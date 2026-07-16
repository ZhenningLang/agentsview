package parser

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/tidwall/gjson"
)

const droidIDPrefix = "droid:"

// DiscoverDroidSessions finds Droid JSONL sessions under ~/.factory/sessions.
func DiscoverDroidSessions(root string) []DiscoveredFile {
	if root == "" {
		return nil
	}
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		return nil
	}

	var files []DiscoveredFile
	_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return nil
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".jsonl") || strings.HasSuffix(name, ".settings.jsonl") {
			return nil
		}
		project := GetProjectName(filepath.Base(filepath.Dir(path)))
		files = append(files, DiscoveredFile{Path: path, Project: project, Agent: AgentDroid})
		return nil
	})
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files
}

func FindDroidSourceFile(root, sessionID string) string {
	if !IsValidSessionID(sessionID) {
		return ""
	}
	for _, file := range DiscoverDroidSessions(root) {
		id := strings.TrimSuffix(filepath.Base(file.Path), ".jsonl")
		if id == sessionID {
			return file.Path
		}
	}
	return ""
}

func ParseDroidSession(path, project, machine string) (*ParseResult, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("stat %s: %w", path, err)
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()

	rawID := strings.TrimSuffix(filepath.Base(path), ".jsonl")
	sessionID := droidIDPrefix + rawID
	var (
		cwd         string
		sessionName string
		version     string
		messages    []ParsedMessage
		startedAt   time.Time
		endedAt     time.Time
		firstMsg    string
		userCount   int
		hasTurn     bool
		malformed   int
	)

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, initialScanBufSize), maxLineSize)
	for scanner.Scan() {
		line := scanner.Text()
		if !gjson.Valid(line) {
			malformed++
			continue
		}
		ts := extractTimestamp(line)
		if !ts.IsZero() {
			if startedAt.IsZero() || ts.Before(startedAt) {
				startedAt = ts
			}
			if ts.After(endedAt) {
				endedAt = ts
			}
		}

		switch gjson.Get(line, "type").Str {
		case "session_start":
			if id := gjson.Get(line, "id").Str; id != "" {
				rawID = id
				sessionID = droidIDPrefix + rawID
			}
			cwd = gjson.Get(line, "cwd").Str
			sessionName = gjson.Get(line, "sessionTitle").Str
			if sessionName == "" {
				sessionName = gjson.Get(line, "title").Str
			}
			if v := gjson.Get(line, "version"); v.Exists() {
				version = v.String()
			}
			content := droidSessionStartContext(line)
			if content != "" {
				messages = append(messages, ParsedMessage{
					Ordinal:       len(messages),
					Role:          RoleUser,
					Content:       content,
					Timestamp:     ts,
					IsSystem:      true,
					ContentLength: len(content),
					SourceType:    "system",
					SourceSubtype: "session_start",
					SourceUUID:    gjson.Get(line, "id").Str,
				})
			}
		case "message":
			role := parseDroidRole(gjson.Get(line, "message.role").Str)
			if role == "" {
				continue
			}
			hasTurn = true
			content, thinking, hasThinking, hasToolUse, toolCalls, toolResults := ExtractTextContent(gjson.Get(line, "message.content"))
			// Droid writes the environment tool listing as a separate
			// user row (<system-reminder> prefix) and emits empty user
			// rows for tool plumbing. Neither is something the caller
			// typed: mark them IsSystem (the same convention the Claude
			// parser uses for tool-result carriers) so downstream
			// user-message counting and the automation classifier gate
			// see only substantive user input.
			trimmed := strings.TrimSpace(content)
			syntheticUserRow := role == RoleUser &&
				(trimmed == "" || strings.HasPrefix(trimmed, "<system-reminder>"))
			msg := ParsedMessage{
				Ordinal:       len(messages),
				Role:          role,
				Content:       content,
				ThinkingText:  thinking,
				Timestamp:     ts,
				HasThinking:   hasThinking,
				HasToolUse:    hasToolUse,
				IsSystem:      syntheticUserRow,
				ContentLength: len(content),
				ToolCalls:     toolCalls,
				ToolResults:   toolResults,
				SourceType:    "message",
				SourceUUID:    gjson.Get(line, "id").Str,
			}
			messages = append(messages, msg)
			if role == RoleUser && !syntheticUserRow {
				userCount++
				if firstMsg == "" {
					firstMsg = truncate(strings.ReplaceAll(content, "\n", " "), 300)
				}
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	if !hasTurn || len(messages) == 0 {
		return nil, nil
	}
	if project == "" {
		project = GetProjectName(filepath.Base(filepath.Dir(path)))
	}
	if cwd != "" {
		if p := ExtractProjectFromCwd(cwd); p != "" {
			project = p
		}
	}
	if startedAt.IsZero() {
		startedAt = info.ModTime()
	}
	if endedAt.IsZero() {
		endedAt = startedAt
	}

	usage, meta := parseDroidSettings(path, sessionID, endedAt, startedAt)
	sess := ParsedSession{
		ID:               sessionID,
		Project:          project,
		Machine:          machine,
		Agent:            AgentDroid,
		Cwd:              cwd,
		SourceVersion:    version,
		MalformedLines:   malformed,
		FirstMessage:     firstMsg,
		SessionName:      sessionName,
		StartedAt:        startedAt,
		EndedAt:          endedAt,
		MessageCount:     len(messages),
		UserMessageCount: userCount,
		File: FileInfo{
			Path:  path,
			Size:  info.Size(),
			Mtime: info.ModTime().UnixNano(),
		},
	}
	if meta.hasUsage {
		sess.HasTotalOutputTokens = true
		sess.TotalOutputTokens = meta.outputTokens
		sess.HasPeakContextTokens = true
		sess.PeakContextTokens = meta.inputTokens + meta.cacheCreationTokens + meta.cacheReadTokens
		sess.aggregateTokenPresenceKnown = true
		sess.AggregateTokenSource = TokenAggregateSummary
	}
	return &ParseResult{Session: sess, Messages: messages, UsageEvents: usage}, nil
}

func droidSessionStartContext(line string) string {
	parts := []string{"Droid session start"}
	if title := gjson.Get(line, "title").Str; title != "" {
		parts = append(parts, "title: "+title)
	}
	if sessionTitle := gjson.Get(line, "sessionTitle").Str; sessionTitle != "" {
		parts = append(parts, "sessionTitle: "+sessionTitle)
	}
	if cwd := gjson.Get(line, "cwd").Str; cwd != "" {
		parts = append(parts, "cwd: "+cwd)
	}
	if owner := gjson.Get(line, "owner").Str; owner != "" {
		parts = append(parts, "owner: "+owner)
	}
	if version := gjson.Get(line, "version"); version.Exists() {
		parts = append(parts, "version: "+version.String())
	}
	if len(parts) == 1 {
		return ""
	}
	return strings.Join(parts, "\n")
}

func parseDroidRole(role string) RoleType {
	switch role {
	case "user":
		return RoleUser
	case "assistant":
		return RoleAssistant
	default:
		return ""
	}
}

type droidSettingsUsage struct {
	inputTokens         int
	outputTokens        int
	cacheCreationTokens int
	cacheReadTokens     int
	reasoningTokens     int
	hasUsage            bool
}

func parseDroidSettings(path, sessionID string, endedAt, startedAt time.Time) ([]ParsedUsageEvent, droidSettingsUsage) {
	settingsPath := strings.TrimSuffix(path, ".jsonl") + ".settings.json"
	data, err := os.ReadFile(settingsPath)
	if err != nil || !gjson.ValidBytes(data) {
		return nil, droidSettingsUsage{}
	}
	usage := droidSettingsUsage{
		inputTokens:         int(gjson.GetBytes(data, "tokenUsage.inputTokens").Int()),
		outputTokens:        int(gjson.GetBytes(data, "tokenUsage.outputTokens").Int()),
		cacheCreationTokens: int(gjson.GetBytes(data, "tokenUsage.cacheCreationTokens").Int()),
		cacheReadTokens:     int(gjson.GetBytes(data, "tokenUsage.cacheReadTokens").Int()),
		reasoningTokens:     int(gjson.GetBytes(data, "tokenUsage.thinkingTokens").Int()),
	}
	usage.hasUsage = gjson.GetBytes(data, "tokenUsage").Exists()
	if !usage.hasUsage {
		return nil, usage
	}
	model := gjson.GetBytes(data, "model").Str
	return []ParsedUsageEvent{{
		SessionID:                sessionID,
		Source:                   "droid-settings",
		Model:                    model,
		InputTokens:              usage.inputTokens,
		OutputTokens:             usage.outputTokens,
		CacheCreationInputTokens: usage.cacheCreationTokens,
		CacheReadInputTokens:     usage.cacheReadTokens,
		ReasoningTokens:          usage.reasoningTokens,
		OccurredAt:               timeString(endedAt, startedAt),
		DedupKey:                 "session:" + sessionID,
	}}, usage
}
