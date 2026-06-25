package db

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// Memory is one user-memory note: a markdown file under the memory SSOT
// (~/.dotfiles/memory/user/*.md) with YAML frontmatter and a body. Like
// Skill it is slowly-changing reference data that lives in its own
// dimension table and is populated by the MemorySyncer, never by the
// session sync path. The store is read-only for callers other than the
// syncer; PG/DuckDB receive rows via the SQLite mirror.
type Memory struct {
	RelPath       string `json:"rel_path"`
	Title         string `json:"title"`
	Date          string `json:"date"`
	ProblemType   string `json:"problem_type"`
	Type          string `json:"type"`
	Status        string `json:"status"`
	OriginSession string `json:"origin_session"`
	Body          string `json:"body,omitempty"`
	BodyTokens    int    `json:"body_tokens"`
	SourceMtime   int64  `json:"source_mtime"`
	SyncedAt      string `json:"synced_at"`
}

// MemoryFilter narrows a memory listing. Empty fields = no filter. Q is a
// full-text query over the note body (FTS5 MATCH on SQLite, dialect-
// specific elsewhere).
type MemoryFilter struct {
	ProblemType   string
	Type          string
	Status        string
	OriginSession string
	Q             string
}

// memoryCols lists the memory columns in the canonical order shared by
// every backend's scan helper.
const memoryCols = `rel_path, title, date, problem_type, type, status,
	origin_session, body, body_tokens, source_mtime, synced_at`

func scanMemory(rows *sql.Rows) (Memory, error) {
	var m Memory
	if err := rows.Scan(
		&m.RelPath, &m.Title, &m.Date, &m.ProblemType, &m.Type,
		&m.Status, &m.OriginSession, &m.Body, &m.BodyTokens,
		&m.SourceMtime, &m.SyncedAt,
	); err != nil {
		return Memory{}, err
	}
	return m, nil
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
	from := "memory m"
	if f.Q != "" {
		// FTS5 MATCH join on the standalone memory_fts index, keyed back
		// to memory by rel_path.
		from = "memory m JOIN memory_fts ON memory_fts.rel_path = m.rel_path"
		preds = append(preds, "memory_fts MATCH ?")
		args = append(args, f.Q)
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

// replaceMemoriesTx full-replaces the memory table inside an open tx. The
// AFTER INSERT/DELETE triggers keep memory_fts in sync automatically.
func replaceMemoriesTx(
	ctx context.Context, tx txExec, memories []Memory,
) error {
	if _, err := tx.ExecContext(ctx, "DELETE FROM memory"); err != nil {
		return fmt.Errorf("clearing memory: %w", err)
	}
	cols := strings.ReplaceAll(memoryCols, "\n\t", " ")
	stmt, err := tx.PrepareContext(ctx, `INSERT INTO memory (`+cols+`)
		VALUES (?,?,?,?,?,?,?,?,?,?,?)`)
	if err != nil {
		return fmt.Errorf("prepare memory insert: %w", err)
	}
	defer stmt.Close()
	for _, m := range memories {
		if _, err := stmt.ExecContext(ctx,
			m.RelPath, m.Title, m.Date, m.ProblemType, m.Type,
			m.Status, m.OriginSession, m.Body, m.BodyTokens,
			m.SourceMtime, m.SyncedAt,
		); err != nil {
			return fmt.Errorf("insert memory %q: %w", m.RelPath, err)
		}
	}
	return nil
}

// ReplaceMemories atomically full-replaces the memory table in a single
// transaction. The MemorySyncer uses this so a crash mid-write can never
// leave a partial mirror. Local-only writer (PG/DuckDB receive rows via
// the SQLite mirror, not through this path).
func (db *DB) ReplaceMemories(
	ctx context.Context, memories []Memory,
) error {
	db.mu.Lock()
	defer db.mu.Unlock()
	tx, err := db.getWriter().BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin memory tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := replaceMemoriesTx(ctx, tx, memories); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit memory: %w", err)
	}
	return nil
}
