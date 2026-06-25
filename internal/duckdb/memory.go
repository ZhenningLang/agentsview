package duckdb

import (
	"context"
	"fmt"
	"strings"

	"go.kenn.io/agentsview/internal/db"
)

// Memory read methods mirror the SQLite implementations in
// internal/db/memory.go. Memory rows reach the DuckDB mirror via the
// SQLite push, so there are no writer methods. SQLite uses an FTS5 MATCH
// for the full-text Q filter; DuckDB has no FTS5 table here, so it uses
// ILIKE substring search (same dialect the message search uses).

const duckMemoryCols = `rel_path, title, date, problem_type, type, status,
	origin_session, body, body_tokens, source_mtime, synced_at`

func (s *Store) ListMemories(
	ctx context.Context, f db.MemoryFilter,
) ([]db.Memory, error) {
	var preds []string
	var args []any
	if f.ProblemType != "" {
		preds = append(preds, "problem_type = ?")
		args = append(args, f.ProblemType)
	}
	if f.Type != "" {
		preds = append(preds, "type = ?")
		args = append(args, f.Type)
	}
	if f.Status != "" {
		preds = append(preds, "status = ?")
		args = append(args, f.Status)
	}
	if f.OriginSession != "" {
		preds = append(preds, "origin_session = ?")
		args = append(args, f.OriginSession)
	}
	if f.Q != "" {
		preds = append(preds, "body ILIKE '%' || ? || '%'")
		args = append(args, f.Q)
	}
	where := ""
	if len(preds) > 0 {
		where = " WHERE " + strings.Join(preds, " AND ")
	}
	q := "SELECT " + duckMemoryCols + " FROM memory" + where +
		" ORDER BY date DESC, rel_path"
	rows, err := s.duck.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("listing memories: %w", err)
	}
	defer rows.Close()
	out := make([]db.Memory, 0, 64)
	for rows.Next() {
		m, err := scanDuckMemory(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (s *Store) GetMemory(
	ctx context.Context, relPath string,
) (*db.Memory, error) {
	q := "SELECT " + duckMemoryCols + " FROM memory WHERE rel_path = ?"
	rows, err := s.duck.QueryContext(ctx, q, relPath)
	if err != nil {
		return nil, fmt.Errorf("getting memory: %w", err)
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, rows.Err()
	}
	m, err := scanDuckMemory(rows)
	if err != nil {
		return nil, err
	}
	return &m, nil
}

func scanDuckMemory(
	rows interface{ Scan(...any) error },
) (db.Memory, error) {
	var m db.Memory
	if err := rows.Scan(
		&m.RelPath, &m.Title, &m.Date, &m.ProblemType, &m.Type,
		&m.Status, &m.OriginSession, &m.Body, &m.BodyTokens,
		&m.SourceMtime, &m.SyncedAt,
	); err != nil {
		return db.Memory{}, err
	}
	return m, nil
}
