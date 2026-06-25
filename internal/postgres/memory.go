package postgres

import (
	"context"
	"fmt"
	"strings"

	"go.kenn.io/agentsview/internal/db"
)

// Memory read methods mirror the SQLite implementations in
// internal/db/memory.go. The PG store is read-only: memory rows reach PG
// via the SQLite mirror, so there are no writer methods here. SQLite uses
// an FTS5 MATCH for the full-text Q filter; PG has no FTS5 module, so it
// uses ILIKE substring search (same dialect the message search uses).

const pgMemoryCols = `rel_path, source, title, date, problem_type, type, status,
	origin_session, body, body_tokens, source_mtime, synced_at`

// ListMemories returns memory notes ordered by date descending then
// rel_path, optionally filtered by frontmatter fields or a body substring.
func (s *Store) ListMemories(
	ctx context.Context, f db.MemoryFilter,
) ([]db.Memory, error) {
	pb := &paramBuilder{}
	var preds []string
	if f.Source != "" {
		preds = append(preds, "source = "+pb.add(f.Source))
	}
	if f.ProblemType != "" {
		preds = append(preds, "problem_type = "+pb.add(f.ProblemType))
	}
	if f.Type != "" {
		preds = append(preds, "type = "+pb.add(f.Type))
	}
	if f.Status != "" {
		preds = append(preds, "status = "+pb.add(f.Status))
	}
	if f.OriginSession != "" {
		preds = append(preds, "origin_session = "+pb.add(f.OriginSession))
	}
	if f.Q != "" {
		preds = append(preds,
			"body ILIKE '%' || "+pb.add(f.Q)+" || '%'")
	}
	where := ""
	if len(preds) > 0 {
		where = " WHERE " + strings.Join(preds, " AND ")
	}
	q := "SELECT " + pgMemoryCols + " FROM memory" + where +
		" ORDER BY date DESC, rel_path"
	rows, err := s.pg.QueryContext(ctx, q, pb.args...)
	if err != nil {
		return nil, fmt.Errorf("listing memories: %w", err)
	}
	defer rows.Close()
	out := make([]db.Memory, 0, 64)
	for rows.Next() {
		m, err := scanPGMemory(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// GetMemory returns one memory note by rel_path, or nil when absent.
func (s *Store) GetMemory(
	ctx context.Context, relPath string,
) (*db.Memory, error) {
	q := "SELECT " + pgMemoryCols + " FROM memory WHERE rel_path = $1"
	rows, err := s.pg.QueryContext(ctx, q, relPath)
	if err != nil {
		return nil, fmt.Errorf("getting memory: %w", err)
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, rows.Err()
	}
	m, err := scanPGMemory(rows)
	if err != nil {
		return nil, err
	}
	return &m, nil
}

func scanPGMemory(
	rows interface{ Scan(...any) error },
) (db.Memory, error) {
	var m db.Memory
	if err := rows.Scan(
		&m.RelPath, &m.Source, &m.Title, &m.Date, &m.ProblemType, &m.Type,
		&m.Status, &m.OriginSession, &m.Body, &m.BodyTokens,
		&m.SourceMtime, &m.SyncedAt,
	); err != nil {
		return db.Memory{}, err
	}
	return m, nil
}
