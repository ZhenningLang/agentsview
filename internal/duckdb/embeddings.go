package duckdb

import (
	"context"
	"fmt"

	"go.kenn.io/agentsview/internal/db"
)

func (s *Store) SessionEmbeddings(ctx context.Context, f db.EmbeddingFilter) ([]db.SessionEmbedding, error) {
	args := []any{}
	projectClause := ""
	if f.Project != "" {
		projectClause = "AND project = ?"
		args = append(args, f.Project)
	}
	limitClause := ""
	if f.Limit > 0 {
		limitClause = "LIMIT ?"
		args = append(args, f.Limit)
	}
	rows, err := s.duck.QueryContext(ctx, `
		SELECT id, project, agent,
			COALESCE(display_name, session_name, first_message, '') AS name,
			COALESCE(ended_at, started_at, created_at) AS session_ended_at,
			llm_embedding, llm_embedding_dim
		FROM sessions
		WHERE deleted_at IS NULL
			AND llm_embedding IS NOT NULL
			AND llm_embedding_dim > 0
			`+projectClause+`
		ORDER BY COALESCE(ended_at, started_at, created_at) DESC, id DESC
		`+limitClause, args...)
	if err != nil {
		return nil, fmt.Errorf("duckdb session embeddings: %w", err)
	}
	defer rows.Close()

	var out []db.SessionEmbedding
	for rows.Next() {
		var e db.SessionEmbedding
		var data []byte
		var dim int
		var ended any
		if err := rows.Scan(&e.SessionID, &e.Project, &e.Agent, &e.Name, &ended, &data, &dim); err != nil {
			return nil, fmt.Errorf("scan duckdb session embedding: %w", err)
		}
		vector, err := db.DecodeEmbedding(data, dim)
		if err != nil {
			return nil, fmt.Errorf("decode duckdb session embedding %s: %w", e.SessionID, err)
		}
		e.SessionEndedAt = formatDBTime(ended)
		e.Vector = vector
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate duckdb session embeddings: %w", err)
	}
	return out, nil
}
