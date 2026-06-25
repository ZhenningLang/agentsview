// Package memory syncs the user-memory SSOT (markdown notes with YAML
// frontmatter under ~/.dotfiles/memory/user) into the memory dimension
// table. Like the skills syncer it is reference-data plumbing that runs
// entirely independently of the session sync engine: it never reads or
// writes sessions, messages, or tool_calls, so it cannot pollute the core
// fact domain or its stats triggers. It is strictly read-only against the
// SSOT (this MVP never writes memory files) and uses full-replace
// semantics on each sync, mirroring internal/skills/syncer.go.
package memory

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.kenn.io/agentsview/internal/db"
	"go.kenn.io/agentsview/internal/skills"
	"gopkg.in/yaml.v3"
)

// Writer is the narrow persistence surface the syncers need. It is satisfied
// by *db.DB; the PG/DuckDB read stores receive memory rows via the SQLite
// mirror, not through this path. ReplaceMemories does a whole-table replace;
// ReplaceMemoriesBySource scopes the replace to one data source so the
// cross-agent and cc-native syncers never clobber each other's rows.
type Writer interface {
	ReplaceMemories(ctx context.Context, memories []db.Memory) error
	ReplaceMemoriesBySource(
		ctx context.Context, source string, memories []db.Memory,
	) error
}

// indexBasename is the on-disk index file used as a hint and excluded
// from the synced note set (it is generated, not a memory note itself).
const indexBasename = "INDEX.md"

// frontmatter is the YAML header of a memory note. Only the fields the
// dimension table tracks are decoded; the rest of the schema (trust,
// keywords, related, ...) is ignored by this MVP.
type frontmatter struct {
	Title         string `yaml:"title"`
	Date          string `yaml:"date"`
	ProblemType   string `yaml:"problem_type"`
	Type          string `yaml:"type"`
	Status        string `yaml:"status"`
	OriginSession string `yaml:"origin_session"`
}

// Syncer reads a memory directory and persists the parsed notes.
type Syncer struct {
	dir       string
	tokenizer skills.Tokenizer
	writer    Writer
	now       func() time.Time
}

// NewSyncer builds a Syncer. dir is the memory directory containing the
// *.md notes (e.g. ~/.dotfiles/memory/user). When tk is nil the default
// heuristic tokenizer (shared with the skills syncer) is used.
func NewSyncer(dir string, w Writer, tk skills.Tokenizer) *Syncer {
	if tk == nil {
		tk = skills.NewHeuristicTokenizer()
	}
	return &Syncer{
		dir:       dir,
		tokenizer: tk,
		writer:    w,
		now:       time.Now,
	}
}

// Sync scans the memory directory for *.md notes, parses each one, and
// full-replaces the memory table. It is fail-soft per file: a single
// unreadable or unparseable note is skipped rather than aborting the
// whole run, so one bad file never blanks the whole mirror.
func (s *Syncer) Sync(ctx context.Context) error {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return fmt.Errorf("reading memory dir: %w", err)
	}
	syncedAt := s.now().UTC().Format("2006-01-02T15:04:05.000Z")

	memories := make([]db.Memory, 0, len(entries))
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".md") {
			continue
		}
		// INDEX.md is a generated hint/index, not a memory note.
		if name == indexBasename {
			continue
		}
		path := filepath.Join(s.dir, name)
		info, statErr := e.Info()
		if statErr != nil {
			// Skip entries we cannot stat; fail-soft.
			continue
		}
		m, parseErr := s.parseFile(path, name, info.ModTime().Unix())
		if parseErr != nil {
			// Fail-soft: skip this note, keep the rest.
			continue
		}
		m.SyncedAt = syncedAt
		memories = append(memories, m)
	}

	return s.writer.ReplaceMemoriesBySource(
		ctx, db.SourceCrossAgent, memories)
}

// parseFile reads one memory note, splitting YAML frontmatter from the
// body and computing the body token estimate.
func (s *Syncer) parseFile(
	path, relPath string, mtime int64,
) (db.Memory, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return db.Memory{}, err
	}
	raw := string(data)
	var fm frontmatter
	if block := extractFrontmatterBlock(raw); block != "" {
		if uerr := yaml.Unmarshal([]byte(block), &fm); uerr != nil {
			return db.Memory{}, uerr
		}
	}
	body := extractBody(raw)
	return db.Memory{
		RelPath:       relPath,
		Source:        db.SourceCrossAgent,
		Title:         fm.Title,
		Date:          fm.Date,
		ProblemType:   fm.ProblemType,
		Type:          fm.Type,
		Status:        fm.Status,
		OriginSession: fm.OriginSession,
		Body:          body,
		BodyTokens:    s.tokenizer.Count(body),
		SourceMtime:   mtime,
	}, nil
}

// extractFrontmatterBlock returns the YAML between the leading --- and the
// next --- line, or "" when there is no frontmatter. Mirrors the skills
// syncer parser so both behave identically.
func extractFrontmatterBlock(content string) string {
	content = strings.TrimLeft(content, "\ufeff \t\r\n")
	if !strings.HasPrefix(content, "---") {
		return ""
	}
	lines := strings.Split(content, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return ""
	}
	var body []string
	for _, ln := range lines[1:] {
		if strings.TrimSpace(ln) == "---" {
			return strings.Join(body, "\n")
		}
		body = append(body, ln)
	}
	return ""
}

// extractBody returns the markdown content after the frontmatter block.
// When there is no frontmatter, the whole content is the body.
func extractBody(content string) string {
	trimmed := strings.TrimLeft(content, "\ufeff \t\r\n")
	if !strings.HasPrefix(trimmed, "---") {
		return content
	}
	lines := strings.Split(trimmed, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return content
	}
	for i, ln := range lines[1:] {
		if strings.TrimSpace(ln) == "---" {
			return strings.TrimLeft(
				strings.Join(lines[i+2:], "\n"), "\r\n")
		}
	}
	return content
}
