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
	"log"
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

type embeddingReader interface {
	MemoryEmbeddings(ctx context.Context, f db.MemoryFilter) ([]db.Memory, error)
}

// indexBasename is the on-disk index file used as a hint and excluded
// from the synced note set (it is generated, not a memory note itself).
const indexBasename = "INDEX.md"

// frontmatter is the YAML header of a memory note. Only the fields the
// dimension table tracks are decoded; the rest of the schema (trust,
// keywords, related, ...) is ignored by this MVP.
type frontmatter struct {
	Title           string `yaml:"title"`
	Date            string `yaml:"date"`
	ProblemType     string `yaml:"problem_type"`
	Type            string `yaml:"type"`
	Status          string `yaml:"status"`
	OriginSession   string `yaml:"origin_session"`
	OriginProject   string `yaml:"origin_project"`
	FeedbackVote    string `yaml:"feedback_vote"`
	FeedbackComment string `yaml:"feedback_comment"`
	FeedbackStatus  string `yaml:"feedback_status"`
}

// Syncer reads a memory directory and persists the parsed notes.
type Syncer struct {
	dir       string
	tokenizer skills.Tokenizer
	writer    Writer
	embedder  Embedder
	now       func() time.Time
}

type Embedder interface {
	Embed(ctx context.Context, input string) ([]float32, error)
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

// NewSyncerWithEmbedder keeps the default sync path fail-open when embedder is
// nil, while allowing agentsview to persist local-only memory embeddings when
// the usage binding is available.
func NewSyncerWithEmbedder(dir string, w Writer, tk skills.Tokenizer, e Embedder) *Syncer {
	s := NewSyncer(dir, w, tk)
	s.embedder = e
	return s
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
	previous := loadPreviousEmbeddings(ctx, s.writer, db.SourceCrossAgent)

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
			// Fail-soft: skip this note, keep the rest — but never silently.
			// A malformed frontmatter (e.g. an unquoted title with ':'/'#')
			// previously vanished with no trace; log so it is observable.
			log.Printf("memory sync: skipping %q: malformed frontmatter: %v", name, parseErr)
			continue
		}
		if m.Status != "active" {
			continue
		}
		m.SyncedAt = syncedAt
		if err := populateMemoryEmbedding(ctx, s.embedder, &m, previous); err != nil {
			return err
		}
		memories = append(memories, m)
	}

	return s.writer.ReplaceMemoriesBySource(
		ctx, db.SourceCrossAgent, memories)
}

func loadPreviousEmbeddings(
	ctx context.Context, writer Writer, source string,
) map[string]db.Memory {
	reader, ok := writer.(embeddingReader)
	if !ok {
		return nil
	}
	memories, err := reader.MemoryEmbeddings(ctx, db.MemoryFilter{Source: source})
	if err != nil {
		return nil
	}
	out := make(map[string]db.Memory, len(memories))
	for _, m := range memories {
		out[m.RelPath] = m
	}
	return out
}

func populateMemoryEmbedding(
	ctx context.Context, embedder Embedder, m *db.Memory, previous map[string]db.Memory,
) error {
	// Embeddings are derived only from Body. Reuse by rel_path/source/body so
	// frontmatter-only rewrites or lexical fallback resyncs keep safe vectors,
	// while changed bodies drop/recompute the vector instead of keeping stale
	// semantic input for later clustering.
	if reusePreviousEmbedding(m, previous) {
		return nil
	}
	if embedder == nil {
		return nil
	}
	vector, err := embedder.Embed(ctx, m.Body)
	if err != nil {
		return fmt.Errorf("embedding memory %q: %w", m.RelPath, err)
	}
	m.LLMEmbedding = vector
	m.LLMEmbeddingDim = len(vector)
	return nil
}

func reusePreviousEmbedding(m *db.Memory, previous map[string]db.Memory) bool {
	old, ok := previous[m.RelPath]
	if !ok || old.Source != m.Source || old.Body != m.Body || len(old.LLMEmbedding) == 0 {
		return false
	}
	m.LLMEmbedding = old.LLMEmbedding
	m.LLMEmbeddingDim = old.LLMEmbeddingDim
	return true
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
		RelPath:         relPath,
		Source:          db.SourceCrossAgent,
		Title:           fm.Title,
		Date:            fm.Date,
		ProblemType:     fm.ProblemType,
		Type:            fm.Type,
		Status:          fm.Status,
		OriginSession:   fm.OriginSession,
		OriginProject:   fm.OriginProject,
		FeedbackVote:    fm.FeedbackVote,
		FeedbackComment: fm.FeedbackComment,
		FeedbackStatus:  fm.FeedbackStatus,
		Body:            body,
		BodyTokens:      s.tokenizer.Count(body),
		SourceMtime:     mtime,
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
