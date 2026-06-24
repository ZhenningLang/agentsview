//go:build pgtest

package postgres

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.kenn.io/agentsview/internal/db"
)

func TestStoreSessionEmbeddings(t *testing.T) {
	pgURL := testPGURL(t)
	cleanPGSchema(t, pgURL)
	t.Cleanup(func() { cleanPGSchema(t, pgURL) })
	ctx := context.Background()
	pg, err := Open(pgURL, "agentsview", true)
	require.NoError(t, err)
	defer pg.Close()
	syncer := &Sync{pg: pg, schema: "agentsview"}
	require.NoError(t, syncer.EnsureSchema(ctx))
	encoded, err := db.EncodeEmbedding([]float32{1, 0})
	require.NoError(t, err)
	deletedEncoded, err := db.EncodeEmbedding([]float32{0, 1})
	require.NoError(t, err)
	_, err = pg.ExecContext(ctx, `
		INSERT INTO sessions (
			id, machine, project, agent, first_message, started_at, ended_at,
			message_count, user_message_count, llm_title, llm_embedding, llm_embedding_dim
		) VALUES
			('alpha-embed', 'test-machine', 'alpha', 'claude', 'alpha first', '2026-01-10T00:00:00Z', '2026-01-10T00:01:00Z', 1, 1, 'LLM Alpha', $1, 2),
			('deleted-embed', 'test-machine', 'alpha', 'claude', 'deleted first', '2026-01-10T00:00:00Z', '2026-01-10T00:01:00Z', 1, 1, 'Deleted', $2, 2),
			('no-embed', 'test-machine', 'beta', 'claude', 'beta first', '2026-01-11T00:00:00Z', '2026-01-11T00:01:00Z', 1, 1, '', NULL, 0)`,
		encoded, deletedEncoded)
	require.NoError(t, err)
	_, err = pg.ExecContext(ctx, `UPDATE sessions SET deleted_at = '2026-01-12T00:00:00Z' WHERE id = 'deleted-embed'`)
	require.NoError(t, err)
	store, err := NewStore(pgURL, "agentsview", true)
	require.NoError(t, err)
	defer store.Close()

	embeddings, err := store.SessionEmbeddings(ctx, db.EmbeddingFilter{})
	require.NoError(t, err)
	require.Len(t, embeddings, 1)
	got := embeddings[0]
	assert.Equal(t, "alpha-embed", got.SessionID)
	assert.Equal(t, "alpha", got.Project)
	assert.Equal(t, "claude", got.Agent)
	assert.Equal(t, "alpha first", got.Name)
	assert.Equal(t, []float32{1, 0}, got.Vector)

	alphaOnly, err := store.SessionEmbeddings(ctx, db.EmbeddingFilter{Project: "alpha"})
	require.NoError(t, err)
	assert.Len(t, alphaOnly, 1)
	betaOnly, err := store.SessionEmbeddings(ctx, db.EmbeddingFilter{Project: "beta"})
	require.NoError(t, err)
	assert.Empty(t, betaOnly)
}
