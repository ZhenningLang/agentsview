package db

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func sampleMemories() []Memory {
	return []Memory{
		{
			RelPath: "alpha.md", Title: "Alpha note", Date: "2026-06-20",
			ProblemType: "knowledge", Type: "semantic", Status: "active",
			OriginSession: "sess-a", OriginProject: "oss-atlas",
			Body:       "the quick brown fox",
			BodyTokens: 5, SourceMtime: 100,
			SyncedAt: "2026-06-23T00:00:00.000Z",
		},
		{
			RelPath: "beta.md", Title: "Beta note", Date: "2026-06-24",
			ProblemType: "incident", Type: "episodic", Status: "archived",
			OriginSession: "sess-b", Body: "lazy dog jumps over",
			BodyTokens: 4, SourceMtime: 200,
			SyncedAt: "2026-06-23T00:00:00.000Z",
		},
	}
}

func TestOpenMigratesLegacyMemoryEmbeddingColumns(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy-memory.db")
	d, err := Open(path)
	require.NoError(t, err)
	require.NoError(t, d.ReplaceMemories(context.Background(), sampleMemories()))
	require.NoError(t, d.Close())

	conn, err := sql.Open("sqlite3", makeDSN(path, false))
	require.NoError(t, err)
	_, err = conn.Exec("ALTER TABLE memory DROP COLUMN llm_embedding")
	require.NoError(t, err)
	_, err = conn.Exec("ALTER TABLE memory DROP COLUMN llm_embedding_dim")
	require.NoError(t, err)
	require.NoError(t, conn.Close())

	d, err = Open(path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = d.Close() })
	cols := sqliteLLMTableColumns(t, d, "memory")
	assert.Equal(t, sqliteLLMColumnInfo{Type: "BLOB", NotNull: false, DefaultValue: ""}, cols["llm_embedding"])
	assert.Equal(t, sqliteLLMColumnInfo{Type: "INTEGER", NotNull: true, DefaultValue: "0"}, cols["llm_embedding_dim"])
}

func TestMemoryEmbeddingsReturnsLocalVectors(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	memories := sampleMemories()
	memories[0].LLMEmbedding = []float32{1, 0}
	require.NoError(t, d.ReplaceMemories(ctx, memories))

	embeddings, err := d.MemoryEmbeddings(ctx, MemoryFilter{})
	require.NoError(t, err)
	require.Len(t, embeddings, 2)
	assert.Equal(t, "beta.md", embeddings[0].RelPath)
	assert.Empty(t, embeddings[0].LLMEmbedding)
	assert.Equal(t, "alpha.md", embeddings[1].RelPath)
	assert.Equal(t, []float32{1, 0}, embeddings[1].LLMEmbedding)
}

func TestReplaceAndListMemories(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	require.NoError(t, d.ReplaceMemories(ctx, sampleMemories()))

	all, err := d.ListMemories(ctx, MemoryFilter{})
	require.NoError(t, err)
	require.Len(t, all, 2)
	// Ordered by date DESC: beta (2026-06-24) before alpha (2026-06-20).
	assert.Equal(t, "beta.md", all[0].RelPath)
	assert.Equal(t, "alpha.md", all[1].RelPath)
	assert.Equal(t, "the quick brown fox", all[1].Body)
	// origin_project round-trips through the column (alpha tagged, beta General).
	assert.Equal(t, "oss-atlas", all[1].OriginProject)
	assert.Equal(t, "", all[0].OriginProject)

	// Full-replace semantics: a smaller set drops removed rows.
	require.NoError(t, d.ReplaceMemories(ctx, sampleMemories()[:1]))
	all, err = d.ListMemories(ctx, MemoryFilter{})
	require.NoError(t, err)
	assert.Len(t, all, 1)
	assert.Equal(t, "alpha.md", all[0].RelPath)
}

func TestListMemoriesFilters(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	require.NoError(t, d.ReplaceMemories(ctx, sampleMemories()))

	cases := []struct {
		name   string
		filter MemoryFilter
		want   string
	}{
		{"problem_type", MemoryFilter{ProblemType: "knowledge"}, "alpha.md"},
		{"type", MemoryFilter{Type: "episodic"}, "beta.md"},
		{"status", MemoryFilter{Status: "active"}, "alpha.md"},
		{"origin_session", MemoryFilter{OriginSession: "sess-b"}, "beta.md"},
		{"origin_project", MemoryFilter{OriginProject: "oss-atlas"}, "alpha.md"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := d.ListMemories(ctx, tc.filter)
			require.NoError(t, err)
			require.Len(t, got, 1)
			assert.Equal(t, tc.want, got[0].RelPath)
		})
	}
}

func TestListMemoriesFullTextSearch(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	require.NoError(t, d.ReplaceMemories(ctx, sampleMemories()))

	// FTS5 MATCH on the body via the memory_fts mirror.
	got, err := d.ListMemories(ctx, MemoryFilter{Q: "fox"})
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "alpha.md", got[0].RelPath)

	got, err = d.ListMemories(ctx, MemoryFilter{Q: "dog"})
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "beta.md", got[0].RelPath)

	// Combined with a frontmatter filter.
	got, err = d.ListMemories(ctx,
		MemoryFilter{Q: "jumps", Status: "archived"})
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "beta.md", got[0].RelPath)

	// FTS index is kept in sync on full-replace: re-replacing with a set
	// that no longer contains "fox" returns nothing for that term.
	require.NoError(t, d.ReplaceMemories(ctx, sampleMemories()[1:]))
	got, err = d.ListMemories(ctx, MemoryFilter{Q: "fox"})
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestReplaceMemoriesBySourceCoexist(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()

	crossAgent := []Memory{{
		RelPath: "x.md", Source: SourceCrossAgent, Title: "Cross",
		Date: "2026-06-20", Body: "cross agent body", BodyTokens: 3,
		SyncedAt: "2026-06-23T00:00:00.000Z",
	}}
	ccNative := []Memory{{
		RelPath: "projA/memory/a.md", Source: SourceCCNative, Title: "CC",
		Date: "2026-06-24", Body: "cc native body", BodyTokens: 3,
		SyncedAt: "2026-06-23T00:00:00.000Z",
	}}

	require.NoError(t, d.ReplaceMemoriesBySource(ctx, SourceCrossAgent, crossAgent))
	require.NoError(t, d.ReplaceMemoriesBySource(ctx, SourceCCNative, ccNative))

	// Both sources coexist in the single table.
	all, err := d.ListMemories(ctx, MemoryFilter{})
	require.NoError(t, err)
	require.Len(t, all, 2)

	// Source filter narrows to one source.
	cc, err := d.ListMemories(ctx, MemoryFilter{Source: SourceCCNative})
	require.NoError(t, err)
	require.Len(t, cc, 1)
	assert.Equal(t, "projA/memory/a.md", cc[0].RelPath)
	assert.Equal(t, SourceCCNative, cc[0].Source)

	// Re-replacing one source must NOT wipe the other.
	require.NoError(t, d.ReplaceMemoriesBySource(ctx, SourceCCNative, nil))
	remaining, err := d.ListMemories(ctx, MemoryFilter{})
	require.NoError(t, err)
	require.Len(t, remaining, 1)
	assert.Equal(t, SourceCrossAgent, remaining[0].Source)
}

func TestGetMemory(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	require.NoError(t, d.ReplaceMemories(ctx, sampleMemories()))

	m, err := d.GetMemory(ctx, "beta.md")
	require.NoError(t, err)
	require.NotNil(t, m)
	assert.Equal(t, "Beta note", m.Title)
	assert.Equal(t, "episodic", m.Type)
	assert.Equal(t, int64(200), m.SourceMtime)

	// Missing rel_path returns nil, no error.
	missing, err := d.GetMemory(ctx, "does-not-exist.md")
	require.NoError(t, err)
	assert.Nil(t, missing)
}
