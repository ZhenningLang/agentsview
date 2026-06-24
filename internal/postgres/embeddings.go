package postgres

import (
	"context"
	"fmt"
	"time"

	"go.kenn.io/agentsview/internal/db"
)

func (s *Store) SessionEmbeddings(ctx context.Context, f db.EmbeddingFilter) ([]db.SessionEmbedding, error) {
	args := []any{}
	projectClause := ""
	if f.Project != "" {
		args = append(args, f.Project)
		projectClause = fmt.Sprintf("AND project = $%d", len(args))
	}
	limitClause := ""
	if f.Limit > 0 {
		args = append(args, f.Limit)
		limitClause = fmt.Sprintf("LIMIT $%d", len(args))
	}
	rows, err := s.pg.QueryContext(ctx, `
		SELECT id, project, agent,
			COALESCE(display_name, session_name, first_message, '') AS name,
			COALESCE(ended_at, started_at, created_at) AS session_ended_at,
			llm_embedding, llm_embedding_dim
		FROM sessions
		WHERE deleted_at IS NULL
			AND llm_embedding IS NOT NULL
			AND llm_embedding_dim > 0
			`+projectClause+`
		ORDER BY COALESCE(ended_at, started_at, created_at) DESC NULLS LAST, id DESC
		`+limitClause, args...)
	if err != nil {
		return nil, fmt.Errorf("pg session embeddings: %w", err)
	}
	defer rows.Close()

	var out []db.SessionEmbedding
	for rows.Next() {
		var e db.SessionEmbedding
		var data []byte
		var dim int
		var ended any
		if err := rows.Scan(&e.SessionID, &e.Project, &e.Agent, &e.Name, &ended, &data, &dim); err != nil {
			return nil, fmt.Errorf("scan pg session embedding: %w", err)
		}
		vector, err := db.DecodeEmbedding(data, dim)
		if err != nil {
			return nil, fmt.Errorf("decode pg session embedding %s: %w", e.SessionID, err)
		}
		e.SessionEndedAt = formatAnyTime(ended)
		e.Vector = vector
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate pg session embeddings: %w", err)
	}
	return out, nil
}

func formatAnyTime(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case []byte:
		return string(t)
	case time.Time:
		return FormatISO8601(t)
	default:
		return fmt.Sprint(t)
	}
}
