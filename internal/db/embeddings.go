package db

import (
	"context"
	"fmt"
)

func (db *DB) SessionEmbeddings(ctx context.Context, f EmbeddingFilter) ([]SessionEmbedding, error) {
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
	rows, err := db.getReader().QueryContext(ctx, `
		SELECT id, project, agent,
			COALESCE(display_name, session_name, first_message, '') AS name,
			COALESCE(ended_at, started_at, created_at, '') AS session_ended_at,
			llm_embedding, llm_embedding_dim
		FROM sessions
		WHERE deleted_at IS NULL
			AND llm_embedding IS NOT NULL
			AND llm_embedding_dim > 0
			`+projectClause+`
		ORDER BY COALESCE(ended_at, started_at, created_at, '') DESC, id DESC
		`+limitClause, args...)
	if err != nil {
		return nil, fmt.Errorf("query session embeddings: %w", err)
	}
	defer rows.Close()
	return scanSessionEmbeddings(rows)
}

func scanSessionEmbeddings(rows interface {
	Next() bool
	Scan(...any) error
	Err() error
}) ([]SessionEmbedding, error) {
	var out []SessionEmbedding
	for rows.Next() {
		var e SessionEmbedding
		var data []byte
		var dim int
		if err := rows.Scan(&e.SessionID, &e.Project, &e.Agent, &e.Name, &e.SessionEndedAt, &data, &dim); err != nil {
			return nil, fmt.Errorf("scan session embedding: %w", err)
		}
		vector, err := DecodeEmbedding(data, dim)
		if err != nil {
			return nil, fmt.Errorf("decode session embedding %s: %w", e.SessionID, err)
		}
		e.Vector = vector
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate session embeddings: %w", err)
	}
	return out, nil
}
