package duckdb

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.kenn.io/agentsview/internal/db"
)

func TestDuckDBMemoryCanonicalMetadataRoundTrips(t *testing.T) {
	ctx := context.Background()
	d := openTestDuckDB(t)
	require.NoError(t, EnsureSchema(ctx, d), "EnsureSchema")
	vector, err := db.EncodeEmbedding([]float32{1})
	require.NoError(t, err)
	_, err = d.ExecContext(ctx, `
		INSERT INTO memory (
			rel_path, source, title, date, problem_type, type, status,
			origin_session, origin_project, feedback_vote, feedback_comment,
			feedback_status, canonical_covered_refs, canonical_provenance,
			body, body_tokens, source_mtime, llm_embedding, llm_embedding_dim,
			synced_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"canonical/entrypoint.json", db.SourceCanonical, "Entrypoint", "2026-07-02",
		"knowledge", "semantic", "active", "sess", "oss-atlas", "down",
		"needs split", "pending", `[{"source":"assist-mem"}]`,
		`{"topic":"entrypoint"}`, "canonical body", 2, 100, vector, 1,
		"2026-07-02T00:00:00.000Z")
	require.NoError(t, err)
	store := NewStoreFromDB(d)

	listed, err := store.ListMemories(ctx, db.MemoryFilter{Source: db.SourceCanonical})
	require.NoError(t, err)
	require.Len(t, listed, 1)
	assertDuckCanonicalMemory(t, listed[0])

	filtered, err := store.ListMemories(ctx, db.MemoryFilter{
		OriginProject: "oss-atlas", FeedbackVote: "down", FeedbackStatus: "pending",
	})
	require.NoError(t, err)
	require.Len(t, filtered, 1)
	assert.Equal(t, db.SourceCanonical, filtered[0].Source)

	fetched, err := store.GetMemory(ctx, "canonical/entrypoint.json")
	require.NoError(t, err)
	require.NotNil(t, fetched)
	assertDuckCanonicalMemory(t, *fetched)

	embeddings, err := store.MemoryEmbeddings(ctx, db.MemoryFilter{Source: db.SourceCanonical})
	require.NoError(t, err)
	require.Len(t, embeddings, 1)
	assertDuckCanonicalMemory(t, embeddings[0])
	assert.Equal(t, []float32{1}, embeddings[0].LLMEmbedding)
}

func TestDuckDBEnsureSchemaAddsMemoryParityColumns(t *testing.T) {
	ctx := context.Background()
	d := openTestDuckDB(t)
	require.NoError(t, EnsureSchema(ctx, d), "EnsureSchema")

	for _, column := range []string{
		"origin_project",
		"feedback_vote",
		"feedback_comment",
		"feedback_status",
		"canonical_covered_refs",
		"canonical_provenance",
	} {
		assert.True(t, columnExists(t, d, "memory", column), "missing memory.%s", column)
	}
}

func assertDuckCanonicalMemory(t *testing.T, m db.Memory) {
	t.Helper()
	assert.Equal(t, "canonical/entrypoint.json", m.RelPath)
	assert.Equal(t, db.SourceCanonical, m.Source)
	assert.Equal(t, "oss-atlas", m.OriginProject)
	assert.Equal(t, "down", m.FeedbackVote)
	assert.Equal(t, "needs split", m.FeedbackComment)
	assert.Equal(t, "pending", m.FeedbackStatus)
	assert.Equal(t, `[{"source":"assist-mem"}]`, m.CanonicalCoveredRefs)
	assert.Equal(t, `{"topic":"entrypoint"}`, m.CanonicalProvenance)
}
