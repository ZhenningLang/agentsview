package db

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"strings"
)

// Memory source kinds. A memory note originates from the legacy cross-agent
// SSOT (~/.dotfiles/memory/user), the explicit assist-mem ledger, CC-native
// auto-memory directories (~/.claude/projects/<project>/memory), or generated
// canonical rows. The source column lets the single memory table hold multiple
// data sources and the UI filter between them.
const (
	// SourceCrossAgent is the existing cross-agent user-memory SSOT.
	SourceCrossAgent = "cross-agent"
	// SourceAssistMem is the explicit assist-mem JSONL ledger.
	SourceAssistMem = "assist-mem"
	// SourceCCNative is CC-native auto-memory scanned across project dirs.
	SourceCCNative = "cc-native"
	// SourceCanonical is generated current-memory output with raw provenance.
	SourceCanonical = "canonical"
)

// Memory is one user-memory note: a markdown file under a memory data source
// (the cross-agent SSOT ~/.dotfiles/memory/user/*.md, or CC-native
// auto-memory ~/.claude/projects/<project>/memory/*.md) with optional YAML
// frontmatter and a body. Like Skill it is slowly-changing reference data
// that lives in its own dimension table and is populated by a MemorySyncer,
// never by the session sync path. The store is read-only for callers other
// than the syncer; PG/DuckDB receive rows via the SQLite mirror.
type Memory struct {
	RelPath       string `json:"rel_path"`
	Source        string `json:"source"`
	Title         string `json:"title"`
	Date          string `json:"date"`
	ProblemType   string `json:"problem_type"`
	Type          string `json:"type"`
	Status        string `json:"status"`
	OriginSession string `json:"origin_session"`
	// OriginProject is the project a note belongs to ("" = the General bucket:
	// user-global or cross-project notes). It drives the /memories project facet.
	OriginProject        string    `json:"origin_project"`
	FeedbackVote         string    `json:"feedback_vote"`
	FeedbackComment      string    `json:"feedback_comment"`
	FeedbackStatus       string    `json:"feedback_status"`
	CanonicalCoveredRefs string    `json:"canonical_covered_refs"`
	CanonicalProvenance  string    `json:"canonical_provenance"`
	Body                 string    `json:"body,omitempty"`
	BodyTokens           int       `json:"body_tokens"`
	SourceMtime          int64     `json:"source_mtime"`
	SyncedAt             string    `json:"synced_at"`
	LLMEmbedding         []float32 `json:"-"`
	LLMEmbeddingDim      int       `json:"-"`
}

// MemoryFilter narrows a memory listing. Empty fields = no filter. Q is a
// full-text query over the note body (FTS5 MATCH on SQLite, dialect-
// specific elsewhere). Source filters by data source (cross-agent vs
// cross-agent, assist-mem, cc-native, or canonical).
type MemoryFilter struct {
	Source         string
	ProblemType    string
	Type           string
	Status         string
	OriginSession  string
	OriginProject  string
	FeedbackVote   string
	FeedbackStatus string
	Q              string
}

// memoryCols lists the memory columns in the canonical order shared by
// every backend's scan helper.
const memoryCols = `rel_path, source, title, date, problem_type, type, status,
	origin_session, origin_project, feedback_vote, feedback_comment,
	feedback_status, canonical_covered_refs, canonical_provenance, body,
	body_tokens, source_mtime, synced_at`

const memoryEmbeddingCols = `llm_embedding, llm_embedding_dim`

func scanMemory(rows *sql.Rows) (Memory, error) {
	var m Memory
	if err := rows.Scan(
		&m.RelPath, &m.Source, &m.Title, &m.Date, &m.ProblemType, &m.Type,
		&m.Status, &m.OriginSession, &m.OriginProject, &m.FeedbackVote,
		&m.FeedbackComment, &m.FeedbackStatus, &m.CanonicalCoveredRefs,
		&m.CanonicalProvenance, &m.Body, &m.BodyTokens, &m.SourceMtime,
		&m.SyncedAt,
	); err != nil {
		return Memory{}, err
	}
	return m, nil
}

func scanMemoryWithEmbedding(rows interface {
	Next() bool
	Scan(...any) error
	Err() error
}) ([]Memory, error) {
	var out []Memory
	for rows.Next() {
		var m Memory
		var data []byte
		var dim int
		if err := rows.Scan(
			&m.RelPath, &m.Source, &m.Title, &m.Date, &m.ProblemType, &m.Type,
			&m.Status, &m.OriginSession, &m.OriginProject, &m.FeedbackVote,
			&m.FeedbackComment, &m.FeedbackStatus, &m.CanonicalCoveredRefs,
			&m.CanonicalProvenance, &m.Body, &m.BodyTokens, &m.SourceMtime,
			&m.SyncedAt, &data, &dim,
		); err != nil {
			return nil, err
		}
		if len(data) > 0 || dim > 0 {
			vector, err := DecodeEmbedding(data, dim)
			if err != nil {
				return nil, err
			}
			m.LLMEmbedding = vector
			m.LLMEmbeddingDim = dim
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// ListMemories returns memory notes, optionally filtered by frontmatter
// fields or a full-text query over the body, ordered by date descending
// then rel_path for stable display. The body is included on each row so
// the listing view can render snippets without a second round trip.
func (db *DB) ListMemories(
	ctx context.Context, f MemoryFilter,
) ([]Memory, error) {
	var preds []string
	var args []any
	if f.Source != "" {
		preds = append(preds, "m.source = ?")
		args = append(args, f.Source)
	}
	if f.ProblemType != "" {
		preds = append(preds, "m.problem_type = ?")
		args = append(args, f.ProblemType)
	}
	if f.Type != "" {
		preds = append(preds, "m.type = ?")
		args = append(args, f.Type)
	}
	if f.Status != "" {
		preds = append(preds, "m.status = ?")
		args = append(args, f.Status)
	}
	if f.OriginSession != "" {
		preds = append(preds, "m.origin_session = ?")
		args = append(args, f.OriginSession)
	}
	if f.OriginProject != "" {
		preds = append(preds, "m.origin_project = ?")
		args = append(args, f.OriginProject)
	}
	if f.FeedbackVote != "" {
		preds = append(preds, "m.feedback_vote = ?")
		args = append(args, f.FeedbackVote)
	}
	if f.FeedbackStatus != "" {
		preds = append(preds, "m.feedback_status = ?")
		args = append(args, f.FeedbackStatus)
	}
	from := "memory m"
	if f.Q != "" {
		if db.hasMemoryFTS() {
			// FTS5 MATCH join on the standalone memory_fts index, keyed back
			// to memory by rel_path.
			from = "memory m JOIN memory_fts ON memory_fts.rel_path = m.rel_path"
			preds = append(preds, "memory_fts MATCH ?")
			args = append(args, memoryFTSQuery(f.Q))
		} else {
			preds = append(preds, "m.body LIKE '%' || ? || '%'")
			args = append(args, f.Q)
		}
	}
	where := ""
	if len(preds) > 0 {
		where = " WHERE " + strings.Join(preds, " AND ")
	}
	cols := strings.ReplaceAll(memoryCols, "\n\t", " ")
	q := "SELECT " + prefixCols(cols, "m.") + " FROM " + from + where +
		" ORDER BY m.date DESC, m.rel_path"
	rows, err := db.getReader().QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("listing memories: %w", err)
	}
	defer rows.Close()
	out := make([]Memory, 0, 64)
	for rows.Next() {
		m, err := scanMemory(rows)
		if err != nil {
			return nil, fmt.Errorf("scan memory: %w", err)
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// GetMemory returns a single memory note by its rel_path, or nil if not
// found.
func (db *DB) GetMemory(
	ctx context.Context, relPath string,
) (*Memory, error) {
	q := "SELECT " + memoryCols + " FROM memory WHERE rel_path = ?"
	rows, err := db.getReader().QueryContext(ctx, q, relPath)
	if err != nil {
		return nil, fmt.Errorf("getting memory: %w", err)
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, rows.Err()
	}
	m, err := scanMemory(rows)
	if err != nil {
		return nil, fmt.Errorf("scan memory: %w", err)
	}
	return &m, nil
}

func (db *DB) MemoryEmbeddings(
	ctx context.Context, f MemoryFilter,
) ([]Memory, error) {
	var preds []string
	var args []any
	if f.Source != "" {
		preds = append(preds, "m.source = ?")
		args = append(args, f.Source)
	}
	if f.ProblemType != "" {
		preds = append(preds, "m.problem_type = ?")
		args = append(args, f.ProblemType)
	}
	if f.Type != "" {
		preds = append(preds, "m.type = ?")
		args = append(args, f.Type)
	}
	if f.Status != "" {
		preds = append(preds, "m.status = ?")
		args = append(args, f.Status)
	}
	if f.OriginSession != "" {
		preds = append(preds, "m.origin_session = ?")
		args = append(args, f.OriginSession)
	}
	if f.OriginProject != "" {
		preds = append(preds, "m.origin_project = ?")
		args = append(args, f.OriginProject)
	}
	if f.FeedbackVote != "" {
		preds = append(preds, "m.feedback_vote = ?")
		args = append(args, f.FeedbackVote)
	}
	if f.FeedbackStatus != "" {
		preds = append(preds, "m.feedback_status = ?")
		args = append(args, f.FeedbackStatus)
	}
	from := "memory m"
	if f.Q != "" {
		if db.hasMemoryFTS() {
			from = "memory m JOIN memory_fts ON memory_fts.rel_path = m.rel_path"
			preds = append(preds, "memory_fts MATCH ?")
			args = append(args, memoryFTSQuery(f.Q))
		} else {
			preds = append(preds, "m.body LIKE '%' || ? || '%'")
			args = append(args, f.Q)
		}
	}
	where := ""
	if len(preds) > 0 {
		where = " WHERE " + strings.Join(preds, " AND ")
	}
	cols := strings.ReplaceAll(memoryCols+", "+memoryEmbeddingCols, "\n\t", " ")
	q := "SELECT " + prefixCols(cols, "m.") + " FROM " + from + where +
		" ORDER BY m.date DESC, m.rel_path"
	rows, err := db.getReader().QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("listing memory embeddings: %w", err)
	}
	defer rows.Close()
	return scanMemoryWithEmbedding(rows)
}

// prefixCols prefixes each bare column in a comma-separated list with the
// given table alias prefix. Used so the FTS join query can disambiguate
// the memory columns from the memory_fts columns.
func prefixCols(cols, prefix string) string {
	parts := strings.Split(cols, ",")
	for i, p := range parts {
		parts[i] = prefix + strings.TrimSpace(p)
	}
	return strings.Join(parts, ", ")
}

var memoryFTSTokenPattern = regexp.MustCompile(`[\p{L}\p{N}_]+`)

func memoryFTSQuery(q string) string {
	terms := memoryFTSTokenPattern.FindAllString(q, -1)
	if len(terms) == 0 {
		return quoteFTSTerm(q)
	}
	quoted := make([]string, 0, len(terms))
	for _, term := range terms {
		quoted = append(quoted, quoteFTSTerm(term))
	}
	return strings.Join(quoted, " AND ")
}

func quoteFTSTerm(term string) string {
	return `"` + strings.ReplaceAll(term, `"`, `""`) + `"`
}

// replaceMemoriesTx full-replaces the memory table inside an open tx. The
// AFTER INSERT/DELETE triggers keep memory_fts in sync automatically. When
// source is non-empty only that data source's rows are cleared and replaced,
// leaving the other source's rows untouched (the single memory table holds
// both cross-agent and cc-native rows, each synced by its own syncer).
func replaceMemoriesTx(
	ctx context.Context, tx txExec, source string, memories []Memory,
) error {
	if err := dropMemoryFTSMirrorIfBroken(ctx, tx); err != nil {
		return err
	}
	if source != "" {
		if _, err := tx.ExecContext(ctx,
			"DELETE FROM memory WHERE source = ?", source,
		); err != nil {
			return fmt.Errorf("clearing memory source %q: %w", source, err)
		}
	} else if _, err := tx.ExecContext(ctx, "DELETE FROM memory"); err != nil {
		return fmt.Errorf("clearing memory: %w", err)
	}
	cols := strings.ReplaceAll(memoryCols+", "+memoryEmbeddingCols, "\n\t", " ")
	stmt, err := tx.PrepareContext(ctx, `INSERT INTO memory (`+cols+`)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`)
	if err != nil {
		return fmt.Errorf("prepare memory insert: %w", err)
	}
	defer stmt.Close()
	for _, m := range memories {
		var encoded []byte
		var dim int
		if len(m.LLMEmbedding) > 0 {
			var err error
			encoded, err = EncodeEmbedding(m.LLMEmbedding)
			if err != nil {
				return fmt.Errorf("encode memory embedding %q: %w", m.RelPath, err)
			}
			dim = len(m.LLMEmbedding)
		}
		if _, err := stmt.ExecContext(ctx,
			m.RelPath, m.Source, m.Title, m.Date, m.ProblemType, m.Type,
			m.Status, m.OriginSession, m.OriginProject, m.FeedbackVote,
			m.FeedbackComment, m.FeedbackStatus, m.CanonicalCoveredRefs,
			m.CanonicalProvenance, m.Body, m.BodyTokens, m.SourceMtime,
			m.SyncedAt, encoded, dim,
		); err != nil {
			return fmt.Errorf("insert memory %q: %w", m.RelPath, err)
		}
	}
	return nil
}

func dropMemoryFTSMirrorIfBroken(ctx context.Context, tx txExec) error {
	if _, err := tx.ExecContext(ctx, "SELECT 1 FROM memory_fts LIMIT 1"); err == nil {
		return nil
	}
	stmts := []string{
		"DROP TRIGGER IF EXISTS memory_ai",
		"DROP TRIGGER IF EXISTS memory_ad",
		"DROP TRIGGER IF EXISTS memory_au",
		"DROP TABLE IF EXISTS memory_fts",
	}
	for _, stmt := range stmts {
		if _, err := tx.ExecContext(ctx, stmt); err != nil {
			if strings.HasPrefix(stmt, "DROP TABLE") && strings.Contains(err.Error(), "no such module") {
				continue
			}
			return fmt.Errorf("dropping broken memory fts mirror (%s): %w", stmt, err)
		}
	}
	return nil
}

// ReplaceMemories atomically full-replaces the whole memory table in a single
// transaction (both data sources). It is retained for the combined-replace
// path; the per-source syncers use ReplaceMemoriesBySource so each source can
// sync independently without wiping the other. Local-only writer (PG/DuckDB
// receive rows via the SQLite mirror, not through this path).
func (db *DB) ReplaceMemories(
	ctx context.Context, memories []Memory,
) error {
	return db.replaceMemories(ctx, "", memories)
}

// ReplaceMemoriesBySource atomically replaces only the rows of the given data
// source (cross-agent or cc-native), leaving the other source untouched. Each
// memory syncer owns one source and calls this so the two syncers — sharing
// the single memory table — never clobber each other.
func (db *DB) ReplaceMemoriesBySource(
	ctx context.Context, source string, memories []Memory,
) error {
	return db.replaceMemories(ctx, source, memories)
}

func (db *DB) replaceMemories(
	ctx context.Context, source string, memories []Memory,
) error {
	db.mu.Lock()
	defer db.mu.Unlock()
	tx, err := db.getWriter().BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin memory tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := replaceMemoriesTx(ctx, tx, source, memories); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit memory: %w", err)
	}
	return nil
}
