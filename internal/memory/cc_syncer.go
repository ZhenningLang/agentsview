package memory

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.kenn.io/agentsview/internal/db"
	"go.kenn.io/agentsview/internal/skills"
	"gopkg.in/yaml.v3"
)

// ccIndexBasename is the per-project CC-native index/directory file. It lists
// the other memory files rather than holding a memory fact itself, so it is
// excluded from the synced note set (and thus never exposed as an editable
// memory entry in later phases).
const ccIndexBasename = "MEMORY.md"

// CCSyncer mirrors CC-native auto-memory into the shared memory dimension
// table as the second data source. CC-native memory lives across many project
// directories under a single root (default ~/.claude/projects), with each
// project's notes in <project>/memory/*.md. Every synced note is tagged
// source=cc-native; its RelPath is the path relative to the root
// (<project>/memory/<file>.md), which is unique across projects and serves as
// the stable natural key plus the write-back locator for P2.
//
// Like the cross-agent Syncer it is read-only against disk (this phase never
// writes CC-native files) and full-replaces its own source on each run via the
// shared ReplaceMemories writer.
type CCSyncer struct {
	root      string
	tokenizer skills.Tokenizer
	writer    Writer
	embedder  Embedder
	now       func() time.Time
}

// NewCCSyncer builds a CCSyncer. root is the directory whose immediate
// children are project dirs each containing a memory/ subdir (e.g.
// ~/.claude/projects). When tk is nil the default heuristic tokenizer is used.
func NewCCSyncer(root string, w Writer, tk skills.Tokenizer) *CCSyncer {
	if tk == nil {
		tk = skills.NewHeuristicTokenizer()
	}
	return &CCSyncer{
		root:      root,
		tokenizer: tk,
		writer:    w,
		now:       time.Now,
	}
}

// NewCCSyncerWithEmbedder mirrors NewSyncerWithEmbedder for CC-native rows.
func NewCCSyncerWithEmbedder(root string, w Writer, tk skills.Tokenizer, e Embedder) *CCSyncer {
	s := NewCCSyncer(root, w, tk)
	s.embedder = e
	return s
}

// Sync scans <root>/*/memory/*.md, parses each note, and full-replaces the
// CC-native rows. It is fail-open on a missing/unreadable root (no rows, no
// error, matching the skills/vault syncers) and fail-soft per file: one
// unreadable or unparseable note is skipped, never aborting the whole run.
// It replaces only the cc-native source so it never disturbs cross-agent rows
// written by the sibling Syncer — both share the single memory table.
func (s *CCSyncer) Sync(ctx context.Context) error {
	projects, err := os.ReadDir(s.root)
	if err != nil {
		// Fail-open: a missing root means CC-native memory is simply absent.
		return s.writer.ReplaceMemoriesBySource(ctx, db.SourceCCNative, nil)
	}
	syncedAt := s.now().UTC().Format("2006-01-02T15:04:05.000Z")
	previous := loadPreviousEmbeddings(ctx, s.writer, db.SourceCCNative, s.embedder)

	memories := make([]db.Memory, 0, 64)
	for _, proj := range projects {
		if !proj.IsDir() {
			continue
		}
		memDir := filepath.Join(s.root, proj.Name(), "memory")
		entries, derr := os.ReadDir(memDir)
		if derr != nil {
			// No memory/ subdir for this project; skip it.
			continue
		}
		for _, e := range entries {
			name := e.Name()
			if e.IsDir() || !strings.HasSuffix(name, ".md") {
				continue
			}
			// MEMORY.md is the index/directory file, not a memory fact.
			if name == ccIndexBasename {
				continue
			}
			path := filepath.Join(memDir, name)
			relPath := filepath.Join(proj.Name(), "memory", name)
			info, statErr := e.Info()
			if statErr != nil {
				continue
			}
			m, parseErr := s.parseFile(path, relPath, info.ModTime().Unix())
			if parseErr != nil {
				// Fail-soft: skip this note, keep the rest.
				continue
			}
			m.SyncedAt = syncedAt
			if err := populateMemoryEmbedding(ctx, s.embedder, &m, previous); err != nil {
				return err
			}
			memories = append(memories, m)
		}
	}

	return s.writer.ReplaceMemoriesBySource(ctx, db.SourceCCNative, memories)
}

// parseFile reads one CC-native note. CC-native auto-memory is plain markdown
// that often has no YAML frontmatter, so parsing is tolerant: a missing or
// malformed-but-absent frontmatter just yields empty fields with the whole
// file as the body. A present-but-broken frontmatter still returns an error so
// the caller can fail-soft skip it.
func (s *CCSyncer) parseFile(
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
		Source:        db.SourceCCNative,
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
