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
			FeedbackVote: "down", FeedbackComment: "原因: 过度合并", FeedbackStatus: "pending",
			Body:       "the quick brown fox",
			BodyTokens: 5, SourceMtime: 100,
			SyncedAt: "2026-06-23T00:00:00.000Z",
		},
		{
			RelPath: "beta.md", Title: "Beta note", Date: "2026-06-24",
			ProblemType: "incident", Type: "episodic", Status: "archived",
			FeedbackVote: "up", FeedbackStatus: "handled",
			OriginSession: "sess-b", Body: "lazy dog jumps over",
			BodyTokens: 4, SourceMtime: 200,
			SyncedAt: "2026-06-23T00:00:00.000Z",
		},
	}
}

func TestOpenMigratesLegacyMemoryFeedbackColumns(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy-memory-feedback.db")
	d, err := Open(path)
	require.NoError(t, err)
	require.NoError(t, d.ReplaceMemories(context.Background(), sampleMemories()))
	require.NoError(t, d.Close())

	conn, err := sql.Open("sqlite3", makeDSN(path, false))
	require.NoError(t, err)
	_, err = conn.Exec("DROP INDEX idx_memory_feedback_vote")
	require.NoError(t, err)
	_, err = conn.Exec("DROP INDEX idx_memory_feedback_status")
	require.NoError(t, err)
	_, err = conn.Exec("ALTER TABLE memory DROP COLUMN feedback_vote")
	require.NoError(t, err)
	_, err = conn.Exec("ALTER TABLE memory DROP COLUMN feedback_comment")
	require.NoError(t, err)
	_, err = conn.Exec("ALTER TABLE memory DROP COLUMN feedback_status")
	require.NoError(t, err)
	require.NoError(t, conn.Close())

	d, err = Open(path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = d.Close() })
	cols := sqliteLLMTableColumns(t, d, "memory")
	assert.Equal(t, sqliteLLMColumnInfo{Type: "TEXT", NotNull: true, DefaultValue: "''"}, cols["feedback_vote"])
	assert.Equal(t, sqliteLLMColumnInfo{Type: "TEXT", NotNull: true, DefaultValue: "''"}, cols["feedback_comment"])
	assert.Equal(t, sqliteLLMColumnInfo{Type: "TEXT", NotNull: true, DefaultValue: "''"}, cols["feedback_status"])
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

func TestCanonicalMemoryProvenanceRoundTrips(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	raw := Memory{
		RelPath: "assist-mem/raw.jsonl", Source: SourceAssistMem,
		Title: "Raw", Date: "2026-07-01", Body: "raw body",
		SyncedAt: "2026-07-01T00:00:00.000Z",
	}
	canonical := Memory{
		RelPath: "canonical/entrypoint.json", Source: SourceCanonical,
		Title: "Entrypoint", Date: "2026-07-02", Status: "active",
		CanonicalCoveredRefs: `[{"source":"assist-mem","rel_path":"assist-mem/raw.jsonl"}]`,
		CanonicalProvenance:  `{"topic":"entrypoint","sources":["assist-mem"]}`,
		Body:                 "canonical body",
		LLMEmbedding:         []float32{1, 0},
		SyncedAt:             "2026-07-02T00:00:00.000Z",
	}
	require.NoError(t, d.ReplaceMemoriesBySource(ctx, SourceAssistMem, []Memory{raw}))
	require.NoError(t, d.ReplaceMemoriesBySource(ctx, SourceCanonical, []Memory{canonical}))

	all, err := d.ListMemories(ctx, MemoryFilter{})
	require.NoError(t, err)
	require.Len(t, all, 2)
	canonicalRows, err := d.ListMemories(ctx, MemoryFilter{Source: SourceCanonical})
	require.NoError(t, err)
	require.Len(t, canonicalRows, 1)
	assert.Equal(t, canonical.CanonicalCoveredRefs, canonicalRows[0].CanonicalCoveredRefs)
	assert.Equal(t, canonical.CanonicalProvenance, canonicalRows[0].CanonicalProvenance)

	fetched, err := d.GetMemory(ctx, canonical.RelPath)
	require.NoError(t, err)
	require.NotNil(t, fetched)
	assert.Equal(t, canonical.CanonicalCoveredRefs, fetched.CanonicalCoveredRefs)
	assert.Equal(t, canonical.CanonicalProvenance, fetched.CanonicalProvenance)

	embeddings, err := d.MemoryEmbeddings(ctx, MemoryFilter{Source: SourceCanonical})
	require.NoError(t, err)
	require.Len(t, embeddings, 1)
	assert.Equal(t, canonical.CanonicalCoveredRefs, embeddings[0].CanonicalCoveredRefs)
	assert.Equal(t, canonical.CanonicalProvenance, embeddings[0].CanonicalProvenance)
	assert.Equal(t, []float32{1, 0}, embeddings[0].LLMEmbedding)

	rawRows, err := d.ListMemories(ctx, MemoryFilter{Source: SourceAssistMem})
	require.NoError(t, err)
	require.Len(t, rawRows, 1)
	assert.Empty(t, rawRows[0].CanonicalCoveredRefs)
	assert.Empty(t, rawRows[0].CanonicalProvenance)
}

func TestReplaceMemoriesBySourceCanonicalRollbackLeavesRawRows(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	for source, relPath := range map[string]string{
		SourceCrossAgent: "cross.md",
		SourceAssistMem:  "assist-mem/raw.jsonl",
		SourceCCNative:   "proj/memory/raw.md",
		SourceCanonical:  "canonical/current.json",
	} {
		require.NoError(t, d.ReplaceMemoriesBySource(ctx, source, []Memory{{
			RelPath: relPath, Source: source, Title: source,
			Date: "2026-07-01", SyncedAt: "2026-07-01T00:00:00.000Z",
		}}))
	}

	require.NoError(t, d.ReplaceMemoriesBySource(ctx, SourceCanonical, nil))

	canonical, err := d.ListMemories(ctx, MemoryFilter{Source: SourceCanonical})
	require.NoError(t, err)
	assert.Empty(t, canonical)
	for _, source := range []string{SourceCrossAgent, SourceAssistMem, SourceCCNative} {
		rows, err := d.ListMemories(ctx, MemoryFilter{Source: source})
		require.NoError(t, err)
		require.Len(t, rows, 1, source)
		assert.Equal(t, source, rows[0].Source)
	}
}

func TestOpenMigratesLegacyMemoryCanonicalColumns(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy-memory-canonical.db")
	d, err := Open(path)
	require.NoError(t, err)
	require.NoError(t, d.ReplaceMemories(context.Background(), sampleMemories()))
	require.NoError(t, d.Close())

	conn, err := sql.Open("sqlite3", makeDSN(path, false))
	require.NoError(t, err)
	_, err = conn.Exec("ALTER TABLE memory DROP COLUMN canonical_covered_refs")
	require.NoError(t, err)
	_, err = conn.Exec("ALTER TABLE memory DROP COLUMN canonical_provenance")
	require.NoError(t, err)
	require.NoError(t, conn.Close())

	d, err = Open(path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = d.Close() })
	cols := sqliteLLMTableColumns(t, d, "memory")
	assert.Equal(t, sqliteLLMColumnInfo{Type: "TEXT", NotNull: true, DefaultValue: "''"}, cols["canonical_covered_refs"])
	assert.Equal(t, sqliteLLMColumnInfo{Type: "TEXT", NotNull: true, DefaultValue: "''"}, cols["canonical_provenance"])

	got, err := d.ListMemories(context.Background(), MemoryFilter{})
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Empty(t, got[0].CanonicalCoveredRefs)
	assert.Empty(t, got[0].CanonicalProvenance)
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
	assert.Equal(t, "down", all[1].FeedbackVote)
	assert.Equal(t, "原因: 过度合并", all[1].FeedbackComment)
	assert.Equal(t, "pending", all[1].FeedbackStatus)

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
		{"feedback_vote", MemoryFilter{FeedbackVote: "down"}, "alpha.md"},
		{"feedback_status", MemoryFilter{FeedbackStatus: "handled"}, "beta.md"},
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

func TestListMemoriesFullTextSearchEscapesDashQuery(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	memories := []Memory{{
		RelPath: "assist-mem/abd80440ea5d8479.jsonl",
		Source:  SourceAssistMem,
		Title:   "lzn-preview deploy entry",
		Date:    "2026-07-01",
		Status:  "active",
		Body:    "lzn-preview and lzn-test scripts live in ~/Projects/ordo_ai",
	}}
	require.NoError(t, d.ReplaceMemories(ctx, memories))

	got, err := d.ListMemories(ctx, MemoryFilter{Source: SourceAssistMem, Q: "lzn-preview"})
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "assist-mem/abd80440ea5d8479.jsonl", got[0].RelPath)
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

func TestReplaceMemoriesBySourceWorksWhenMemoryFTSMirrorIsUnavailable(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	w := d.getWriter()

	_, err := w.Exec(`DROP TRIGGER IF EXISTS memory_ai`)
	require.NoError(t, err)
	_, err = w.Exec(`DROP TRIGGER IF EXISTS memory_ad`)
	require.NoError(t, err)
	_, err = w.Exec(`DROP TRIGGER IF EXISTS memory_au`)
	require.NoError(t, err)
	_, err = w.Exec(`DROP TABLE IF EXISTS memory_fts`)
	require.NoError(t, err)
	_, err = w.Exec(`CREATE TRIGGER memory_ad AFTER DELETE ON memory BEGIN
		DELETE FROM memory_fts WHERE rel_path = old.rel_path;
	END;`)
	require.NoError(t, err)

	memories := sampleMemories()[:1]
	memories[0].Source = SourceCrossAgent
	require.NoError(t, d.ReplaceMemoriesBySource(ctx, SourceCrossAgent, memories))

	got, err := d.ListMemories(ctx, MemoryFilter{Source: SourceCrossAgent})
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "alpha.md", got[0].RelPath)
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
