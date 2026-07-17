package parser

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/tidwall/gjson"
)

// DiscoverKimiCodeSessions finds all wire.jsonl files under the Kimi
// Code sessions directory. The directory structure is:
// <sessionsDir>/wd_<workspace>_<hash>/session_<uuid>/agents/<agent-id>/wire.jsonl
func DiscoverKimiCodeSessions(sessionsDir string) []DiscoveredFile {
	if sessionsDir == "" {
		return nil
	}

	wsDirs, err := os.ReadDir(sessionsDir)
	if err != nil {
		return nil
	}

	var files []DiscoveredFile
	for _, wsEntry := range wsDirs {
		if !isDirOrSymlink(wsEntry, sessionsDir) {
			continue
		}
		wsDir := filepath.Join(sessionsDir, wsEntry.Name())
		sessionDirs, err := os.ReadDir(wsDir)
		if err != nil {
			continue
		}
		for _, sessEntry := range sessionDirs {
			if !isDirOrSymlink(sessEntry, wsDir) {
				continue
			}
			agentsDir := filepath.Join(
				wsDir, sessEntry.Name(), "agents")
			agentDirs, err := os.ReadDir(agentsDir)
			if err != nil {
				continue
			}
			for _, agentEntry := range agentDirs {
				if !isDirOrSymlink(agentEntry, agentsDir) {
					continue
				}
				wirePath := filepath.Join(
					agentsDir, agentEntry.Name(), "wire.jsonl")
				if _, err := os.Stat(wirePath); err != nil {
					continue
				}
				files = append(files, DiscoveredFile{
					Path:    wirePath,
					Project: wsEntry.Name(),
					Agent:   AgentKimiCode,
				})
			}
		}
	}

	sort.Slice(files, func(i, j int) bool {
		return files[i].Path < files[j].Path
	})
	return files
}

// FindKimiCodeSourceFile locates a Kimi Code session wire file by its
// raw session ID (without the "kimicode:" prefix). The raw ID has the
// format "session_<uuid>" for the main agent, or
// "session_<uuid>:<agent-id>" for a subagent. Because the workspace
// directory name is not part of the ID, workspace dirs are scanned in
// sorted order and the first match wins.
func FindKimiCodeSourceFile(sessionsDir, rawID string) string {
	if sessionsDir == "" {
		return ""
	}

	sessionDirName, agentID, _ := strings.Cut(rawID, ":")
	if agentID == "" {
		agentID = "main"
	}
	if !IsValidSessionID(sessionDirName) ||
		!IsValidSessionID(agentID) {
		return ""
	}

	wsDirs, err := os.ReadDir(sessionsDir)
	if err != nil {
		return ""
	}
	names := make([]string, 0, len(wsDirs))
	for _, e := range wsDirs {
		if isDirOrSymlink(e, sessionsDir) {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)

	for _, ws := range names {
		candidate := filepath.Join(
			sessionsDir, ws, sessionDirName,
			"agents", agentID, "wire.jsonl",
		)
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return ""
}

// kimiCodeState mirrors the session-level state.json file written by
// Kimi Code next to the agents/ directory.
type kimiCodeState struct {
	Title      string `json:"title"`
	WorkDir    string `json:"workDir"`
	ForkedFrom string `json:"forkedFrom"`
}

// ParseKimiCodeSession parses one Kimi Code wire.jsonl file (wire
// protocol 1.4). The path layout is
// <sessionsDir>/wd_<workspace>_<hash>/session_<uuid>/agents/<agent-id>/wire.jsonl.
// Session metadata is enriched from the sibling state.json when present.
func ParseKimiCodeSession(
	path, project, machine string,
) (*ParseResult, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("stat %s: %w", path, err)
	}

	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()

	// Derive IDs from the path: .../session_<uuid>/agents/<agent-id>/wire.jsonl
	agentDir := filepath.Dir(path)
	agentID := filepath.Base(agentDir)
	sessionDirName := filepath.Base(filepath.Dir(filepath.Dir(agentDir)))

	canonicalID := "kimicode:" + sessionDirName
	parentSessionID := ""
	relType := RelNone
	if agentID != "main" {
		canonicalID += ":" + agentID
		parentSessionID = "kimicode:" + sessionDirName
		relType = RelSubagent
	}

	// Best-effort state.json enrichment; absence is not an error.
	var state kimiCodeState
	if data, err := os.ReadFile(
		filepath.Join(filepath.Dir(filepath.Dir(agentDir)), "state.json"),
	); err == nil {
		_ = json.Unmarshal(data, &state)
	}
	if agentID == "main" && state.ForkedFrom != "" &&
		IsValidSessionID(state.ForkedFrom) {
		parentSessionID = "kimicode:" + state.ForkedFrom
		relType = RelFork
	}

	lr := newLineReader(f, maxLineSize)

	var (
		messages     []ParsedMessage
		usageEvents  []ParsedUsageEvent
		firstMessage string

		startTime time.Time
		endTime   time.Time

		pendingText     []string
		pendingThinking []string
		pendingToolCall []ParsedToolCall
		pendingStop     string
		hasThinking     bool
		hasToolUse      bool
		pendingTS       time.Time

		currentTurnID    string
		lastAssistantIdx = -1

		malformed     int
		usageSeq      int
		stepEndUsages []kimiCodeStepUsage
	)

	flushAssistant := func() {
		content := strings.Join(pendingText, "\n")
		thinking := strings.Join(pendingThinking, "\n")
		if strings.TrimSpace(content) == "" &&
			len(pendingToolCall) == 0 && thinking == "" {
			pendingText = nil
			pendingThinking = nil
			pendingToolCall = nil
			pendingStop = ""
			pendingTS = time.Time{}
			hasThinking = false
			hasToolUse = false
			return
		}
		messages = append(messages, ParsedMessage{
			Ordinal:       len(messages),
			Role:          RoleAssistant,
			Content:       content,
			ThinkingText:  thinking,
			Timestamp:     pendingTS,
			HasThinking:   hasThinking,
			HasToolUse:    hasToolUse,
			ContentLength: len(content),
			ToolCalls:     pendingToolCall,
			StopReason:    pendingStop,
		})
		lastAssistantIdx = len(messages) - 1
		pendingText = nil
		pendingThinking = nil
		pendingToolCall = nil
		pendingStop = ""
		pendingTS = time.Time{}
		hasThinking = false
		hasToolUse = false
	}

	noteTS := func(t time.Time) {
		if t.IsZero() {
			return
		}
		if startTime.IsZero() || t.Before(startTime) {
			startTime = t
		}
		if t.After(endTime) {
			endTime = t
		}
	}

	for {
		line, ok := lr.next()
		if !ok {
			break
		}
		if !gjson.Valid(line) {
			malformed++
			continue
		}
		root := gjson.Parse(line)

		ts := kimiCodeEventTime(root)
		noteTS(ts)

		switch root.Get("type").Str {
		case "metadata":
			noteTS(kimiCodeMillisTime(root.Get("created_at")))

		case "turn.prompt", "turn.steer":
			origin := root.Get("origin.kind").Str
			text := kimiCodeInputText(root.Get("input"))
			if text == "" {
				continue
			}
			if origin != "" && origin != "user" {
				flushAssistant()
				messages = append(messages, kimiCodeContextCard(
					text, origin, ts, len(messages)))
				continue
			}
			flushAssistant()
			if firstMessage == "" {
				firstMessage = truncate(
					strings.ReplaceAll(text, "\n", " "), 300)
			}
			messages = append(messages, ParsedMessage{
				Ordinal:       len(messages),
				Role:          RoleUser,
				Content:       text,
				Timestamp:     ts,
				ContentLength: len(text),
			})

		case "turn.cancel":
			flushAssistant()
			messages = append(messages, kimiCodeContextCard(
				"Turn cancelled", "turn_cancel", ts, len(messages)))

		case "context.append_message":
			msg := root.Get("message")
			origin := msg.Get("origin.kind").Str
			// User-originated rows mirror turn.prompt; skip to
			// avoid duplicates. Anything else is context.
			if origin == "" || origin == "user" {
				continue
			}
			var parts []string
			msg.Get("content").ForEach(
				func(_, block gjson.Result) bool {
					if block.Get("type").Str == "text" {
						if t := block.Get("text").Str; t != "" {
							parts = append(parts, t)
						}
					}
					return true
				},
			)
			text := strings.Join(parts, "\n")
			if text == "" {
				continue
			}
			flushAssistant()
			messages = append(messages, kimiCodeContextCard(
				text, origin, ts, len(messages)))

		case "context.append_loop_event":
			ev := root.Get("event")
			switch ev.Get("type").Str {
			case "step.begin":
				if turnID := ev.Get("turnId").Str; turnID != currentTurnID {
					flushAssistant()
					currentTurnID = turnID
				}

			case "content.part":
				part := ev.Get("part")
				switch part.Get("type").Str {
				case "text":
					if t := part.Get("text").Str; t != "" {
						if pendingTS.IsZero() {
							pendingTS = ts
						}
						pendingText = append(pendingText, t)
					}
				case "think":
					if t := part.Get("think").Str; t != "" {
						if pendingTS.IsZero() {
							pendingTS = ts
						}
						hasThinking = true
						pendingThinking = append(pendingThinking, t)
					}
				}

			case "tool.call":
				if pendingTS.IsZero() {
					pendingTS = ts
				}
				hasToolUse = true
				name := ev.Get("name").Str
				args := ev.Get("args")
				toolUseID := ev.Get("toolCallId").Str
				if toolUseID == "" {
					toolUseID = ev.Get("uuid").Str
				}
				pendingToolCall = append(pendingToolCall, ParsedToolCall{
					ToolUseID: toolUseID,
					ToolName:  name,
					Category:  NormalizeToolCategory(name),
					InputJSON: args.Raw,
				})
				pendingText = append(pendingText,
					formatKimiToolUse(name, args))

			case "tool.result":
				flushAssistant()
				result := ev.Get("result")
				output := extractKimiToolOutput(result.Get("output"))
				if result.Get("is_error").Bool() && output == "" {
					output = "[error]"
				}
				quoted, err := json.Marshal(output)
				if err != nil {
					continue
				}
				messages = append(messages, ParsedMessage{
					Ordinal:   len(messages),
					Role:      RoleUser,
					Timestamp: ts,
					ToolResults: []ParsedToolResult{{
						ToolUseID:     ev.Get("toolCallId").Str,
						ContentRaw:    string(quoted),
						ContentLength: len(output),
					}},
				})

			case "step.end":
				usage := ev.Get("usage")
				if usage.Exists() {
					stepEndUsages = append(stepEndUsages,
						kimiCodeStepUsage{
							inputOther:    int(usage.Get("inputOther").Int()),
							output:        int(usage.Get("output").Int()),
							cacheRead:     int(usage.Get("inputCacheRead").Int()),
							cacheCreation: int(usage.Get("inputCacheCreation").Int()),
							ts:            ts,
						})
				}
				fr := ev.Get("finishReason").Str
				if fr != "" {
					if len(pendingText) > 0 ||
						len(pendingThinking) > 0 ||
						len(pendingToolCall) > 0 {
						pendingStop = fr
					} else if lastAssistantIdx >= 0 {
						messages[lastAssistantIdx].StopReason = fr
					}
				}
			}

		case "usage.record":
			usage := root.Get("usage")
			usageEvents = append(usageEvents, ParsedUsageEvent{
				SessionID:                canonicalID,
				Source:                   "usage.record",
				Model:                    root.Get("model").Str,
				InputTokens:              int(usage.Get("inputOther").Int()),
				OutputTokens:             int(usage.Get("output").Int()),
				CacheReadInputTokens:     int(usage.Get("inputCacheRead").Int()),
				CacheCreationInputTokens: int(usage.Get("inputCacheCreation").Int()),
				OccurredAt:               timeString(ts, startTime),
				DedupKey: fmt.Sprintf(
					"turn:%s:%d", canonicalID, usageSeq),
			})
			usageSeq++
		}
	}

	if err := lr.Err(); err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}

	flushAssistant()

	if len(messages) == 0 {
		return nil, nil
	}

	// Fallback: some files may carry usage only on step.end. Consume
	// those only when no usage.record events exist, to avoid double
	// counting (the two are 1:1 mirrors in protocol 1.4).
	if len(usageEvents) == 0 && len(stepEndUsages) > 0 {
		for _, u := range stepEndUsages {
			usageEvents = append(usageEvents, ParsedUsageEvent{
				SessionID:                canonicalID,
				Source:                   "step.end",
				InputTokens:              u.inputOther,
				OutputTokens:             u.output,
				CacheReadInputTokens:     u.cacheRead,
				CacheCreationInputTokens: u.cacheCreation,
				OccurredAt:               timeString(u.ts, startTime),
				DedupKey: fmt.Sprintf(
					"step:%s:%d", canonicalID, usageSeq),
			})
			usageSeq++
		}
	}

	sess := ParsedSession{
		ID:               canonicalID,
		Project:          kimiCodeProjectName(project, state.WorkDir),
		Machine:          machine,
		Agent:            AgentKimiCode,
		ParentSessionID:  parentSessionID,
		RelationshipType: relType,
		Cwd:              state.WorkDir,
		SessionName:      state.Title,
		FirstMessage:     firstMessage,
		StartedAt:        startTime,
		EndedAt:          endTime,
		MessageCount:     len(messages),
		UserMessageCount: countKimiCodeUsers(messages),
		MalformedLines:   malformed,
		File: FileInfo{
			Path:  path,
			Size:  info.Size(),
			Mtime: info.ModTime().UnixNano(),
		},
	}
	if len(usageEvents) > 0 {
		applyUsageEventTokenTotals(&sess, usageEvents)
		sess.aggregateTokenPresenceKnown = true
	}

	return &ParseResult{
		Session:     sess,
		Messages:    messages,
		UsageEvents: usageEvents,
	}, nil
}

type kimiCodeStepUsage struct {
	inputOther    int
	output        int
	cacheRead     int
	cacheCreation int
	ts            time.Time
}

// kimiCodeContextCard builds an IsSystem context-event message for
// non-user wire content (hook results, injections, notifications).
func kimiCodeContextCard(
	content, subtype string, ts time.Time, ordinal int,
) ParsedMessage {
	return ParsedMessage{
		Ordinal:       ordinal,
		Role:          RoleUser,
		Content:       content,
		Timestamp:     ts,
		IsSystem:      true,
		ContentLength: len(content),
		SourceType:    "system",
		SourceSubtype: subtype,
	}
}

// kimiCodeInputText joins the text blocks of a turn.prompt /
// turn.steer input array.
func kimiCodeInputText(input gjson.Result) string {
	var parts []string
	input.ForEach(func(_, block gjson.Result) bool {
		if block.Get("type").Str == "text" {
			if t := block.Get("text").Str; t != "" {
				parts = append(parts, t)
			}
		}
		return true
	})
	return strings.Join(parts, "\n")
}

// kimiCodeEventTime reads the millisecond epoch "time" field present
// on all wire events.
func kimiCodeEventTime(root gjson.Result) time.Time {
	return kimiCodeMillisTime(root.Get("time"))
}

func kimiCodeMillisTime(v gjson.Result) time.Time {
	if !v.Exists() {
		return time.Time{}
	}
	ms := v.Float()
	if ms == 0 {
		return time.Time{}
	}
	sec := int64(ms / 1000)
	nsec := int64(ms-float64(sec*1000)) * 1e6
	return time.Unix(sec, nsec).UTC()
}

// kimiCodeProjectName prefers the state.json workDir base name; when
// absent it strips the "wd_" prefix and "_<12-hex>" suffix from the
// workspace directory name.
func kimiCodeProjectName(project, workDir string) string {
	if workDir != "" {
		if base := filepath.Base(workDir); base != "" && base != "." {
			return base
		}
	}
	p := project
	if strings.HasPrefix(p, "wd_") {
		p = strings.TrimPrefix(p, "wd_")
		if idx := strings.LastIndex(p, "_"); idx > 0 &&
			len(p)-idx-1 == 12 && isKimiCodeHex(p[idx+1:]) {
			p = p[:idx]
		}
	}
	if p == "" {
		p = "kimicode"
	}
	return p
}

func isKimiCodeHex(s string) bool {
	for _, c := range s {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return len(s) > 0
}

// countKimiCodeUsers counts substantive user messages, excluding
// context cards and tool-result carrier rows.
func countKimiCodeUsers(msgs []ParsedMessage) int {
	n := 0
	for _, m := range msgs {
		if m.Role == RoleUser && !m.IsSystem && m.Content != "" &&
			len(m.ToolResults) == 0 {
			n++
		}
	}
	return n
}
