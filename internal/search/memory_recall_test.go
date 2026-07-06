package search

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.kenn.io/agentsview/internal/config"
	"go.kenn.io/agentsview/internal/db"
)

func TestFTSQueryFromText_ExtractsSafeORTerms(t *testing.T) {
	// A candidate blob with code identifiers + FTS-hostile punctuation (':',
	// '(', ')', CJK). The lexical query must become a quoted OR of significant
	// terms so it (a) never throws an FTS5 syntax error and (b) matches notes
	// sharing the same identifiers — even cross-language (ZH note body that also
	// contains `commitPoolItemsEqual`).
	in := "decision: commitPoolItemsEqual 比较了 signed URL 字段 (lzn-preview 部署)"
	q := ftsQueryFromText(in)

	assert.NotContains(t, q, ":", "FTS operator chars must not leak")
	assert.NotContains(t, q, "(")
	assert.Contains(t, q, `"commitPoolItemsEqual"`, "identifier must be a quoted term")
	assert.Contains(t, q, " OR ", "terms must be OR-joined, not implicit-AND")
	assert.Contains(t, q, `"preview"`)

	// Pure punctuation / no extractable term yields an empty query (skip lexical).
	assert.Equal(t, "", ftsQueryFromText("：、（）"))
}

type fakeMemoryRecallEmbedder struct {
	vector []float32
	called bool
}

func (f *fakeMemoryRecallEmbedder) Embed(context.Context, string) ([]float32, error) {
	f.called = true
	return f.vector, nil
}

func TestMemoryRecallAppliesSourceFilterBeforeTopK(t *testing.T) {
	ctx := context.Background()
	store, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, store.Close()) })

	require.NoError(t, store.ReplaceMemories(ctx, []db.Memory{
		{
			RelPath: "project/memory/cc-native.md", Source: db.SourceCCNative, Title: "CC native",
			Date: "2026-06-28", Status: "active", Body: "cc native should outrank if unfiltered",
			SyncedAt: "2026-06-28T00:00:00Z", LLMEmbedding: []float32{1, 0},
		},
		{
			RelPath: "cross-agent.md", Source: db.SourceCrossAgent, Title: "Cross agent",
			Date: "2026-06-28", Status: "active", Body: "cross agent survives filtering",
			SyncedAt: "2026-06-28T00:00:00Z", LLMEmbedding: []float32{0.95, 0.05},
		},
	}))

	embedder := &fakeMemoryRecallEmbedder{vector: []float32{1, 0}}
	resp, err := MemoryRecall(ctx, store, embedder, enabledEmbeddingConfig(), MemoryRecallRequest{
		Query:  "semantic query",
		TopK:   1,
		Filter: db.MemoryFilter{Source: db.SourceCrossAgent},
	})

	require.NoError(t, err)
	require.Len(t, resp.Hits, 1)
	assert.Equal(t, "cross-agent.md", resp.Hits[0].RelPath)
	assert.Equal(t, db.SourceCrossAgent, resp.Hits[0].Source)
}

func TestMemoryRecallPreferCanonicalSuppressesCoveredRaw(t *testing.T) {
	ctx := context.Background()
	store, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, store.Close()) })

	require.NoError(t, store.ReplaceMemories(ctx, []db.Memory{
		{
			RelPath: "assist-mem/entrypoint.jsonl", Source: db.SourceAssistMem,
			Title: "Raw assist entrypoint", Date: "2026-07-01", Status: "active",
			Body:     "lzn preview entrypoint lives in cmd/agentsview main",
			SyncedAt: "2026-07-01T00:00:00Z", LLMEmbedding: []float32{1, 0},
		},
		{
			RelPath: "proj/memory/entrypoint.md", Source: db.SourceCCNative,
			Title: "Raw cc entrypoint", Date: "2026-07-01", Status: "active",
			Body:     "lzn preview entrypoint duplicate from cc native memory",
			SyncedAt: "2026-07-01T00:00:00Z", LLMEmbedding: []float32{0.98, 0.02},
		},
		{
			RelPath: "security.md", Source: db.SourceCrossAgent,
			Title: "Security exception", Date: "2026-07-01", Status: "active",
			Body:     "lzn preview security exception stays separate",
			SyncedAt: "2026-07-01T00:00:00Z", LLMEmbedding: []float32{0.9, 0.1},
		},
		{
			RelPath: "canonical/entrypoint.json", Source: db.SourceCanonical,
			Title: "Canonical entrypoint", Date: "2026-07-02", Status: "active",
			Body: "lzn preview current entrypoint is cmd/agentsview main",
			CanonicalCoveredRefs: `[{"source":"assist-mem","rel_path":"assist-mem/entrypoint.jsonl"},` +
				`{"source":"cc-native","rel_path":"proj/memory/entrypoint.md"}]`,
			CanonicalProvenance: `{"topic":"entrypoint"}`,
			SyncedAt:            "2026-07-02T00:00:00Z", LLMEmbedding: []float32{1, 0},
		},
	}))

	resp, err := MemoryRecall(ctx, store, &fakeMemoryRecallEmbedder{vector: []float32{1, 0}}, enabledEmbeddingConfig(), MemoryRecallRequest{
		Query:           "lzn preview entrypoint",
		TopK:            3,
		PreferCanonical: true,
	})

	require.NoError(t, err)
	paths := hitPaths(resp.Hits)
	assert.Contains(t, paths, "canonical/entrypoint.json")
	assert.Contains(t, paths, "security.md")
	assert.NotContains(t, paths, "assist-mem/entrypoint.jsonl")
	assert.NotContains(t, paths, "proj/memory/entrypoint.md")
	require.NotEmpty(t, resp.Hits)
	canonical := findHit(t, resp.Hits, "canonical/entrypoint.json")
	assert.Equal(t, db.SourceCanonical, canonical.Source)
	assert.JSONEq(t, `[{"source":"assist-mem","rel_path":"assist-mem/entrypoint.jsonl"},{"source":"cc-native","rel_path":"proj/memory/entrypoint.md"}]`, canonical.CanonicalCoveredRefs)
	assert.JSONEq(t, `{"topic":"entrypoint"}`, canonical.CanonicalProvenance)
}

func TestMemoryRecallPreferCanonicalFindsCanonicalWithoutEmbedding(t *testing.T) {
	ctx := context.Background()
	store, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, store.Close()) })

	require.NoError(t, store.ReplaceMemories(ctx, []db.Memory{
		{
			RelPath: "assist-mem/entrypoint.jsonl", Source: db.SourceAssistMem,
			Title: "Raw assist entrypoint", Date: "2026-07-01", Status: "active",
			Body:     "lzn preview entrypoint lives in cmd/agentsview main",
			SyncedAt: "2026-07-01T00:00:00Z", LLMEmbedding: []float32{1, 0},
		},
		{
			RelPath: "canonical/entrypoint.json", Source: db.SourceCanonical,
			Title: "Canonical entrypoint", Date: "2026-07-02", Status: "active",
			Body:                 "lzn preview current entrypoint is cmd/agentsview main",
			CanonicalCoveredRefs: `[{"source":"assist-mem","rel_path":"assist-mem/entrypoint.jsonl"}]`,
			CanonicalProvenance:  `{"topic":"entrypoint"}`,
			SyncedAt:             "2026-07-02T00:00:00Z",
			// Phase 03 canonical rows are currently written without embeddings.
			LLMEmbedding: nil,
		},
	}))

	resp, err := MemoryRecall(ctx, store, &fakeMemoryRecallEmbedder{vector: []float32{1, 0}}, enabledEmbeddingConfig(), MemoryRecallRequest{
		Query:           "lzn preview entrypoint",
		TopK:            5,
		PreferCanonical: true,
	})

	require.NoError(t, err)
	paths := hitPaths(resp.Hits)
	assert.Contains(t, paths, "canonical/entrypoint.json")
	assert.NotContains(t, paths, "assist-mem/entrypoint.jsonl")
	canonical := findHit(t, resp.Hits, "canonical/entrypoint.json")
	assert.Zero(t, canonical.Semantic, "non-embedded canonical must come from lexical/list recall")
	assert.Positive(t, canonical.Lexical)
}

func TestMemoryRecallPreferCanonicalDoesNotSuppressWhenCanonicalOutsideTopK(t *testing.T) {
	ctx := context.Background()
	store, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, store.Close()) })

	require.NoError(t, store.ReplaceMemories(ctx, []db.Memory{
		{
			RelPath: "unrelated-a.md", Source: db.SourceCrossAgent,
			Title: "Unrelated A", Date: "2026-07-03", Status: "active",
			Body:     "different high score fact",
			SyncedAt: "2026-07-03T00:00:00Z", LLMEmbedding: []float32{1, 0},
		},
		{
			RelPath: "assist-mem/covered.jsonl", Source: db.SourceAssistMem,
			Title: "Covered raw", Date: "2026-07-02", Status: "active",
			Body:     "covered high score raw fact",
			SyncedAt: "2026-07-02T00:00:00Z", LLMEmbedding: []float32{0.99, 0.01},
		},
		{
			RelPath: "unrelated-b.md", Source: db.SourceCrossAgent,
			Title: "Unrelated B", Date: "2026-07-01", Status: "active",
			Body:     "second different high score fact",
			SyncedAt: "2026-07-01T00:00:00Z", LLMEmbedding: []float32{0.98, 0.02},
		},
		{
			RelPath: "canonical/covered.json", Source: db.SourceCanonical,
			Title: "Canonical covered", Date: "2026-07-04", Status: "active",
			Body:                 "canonical low score substitute",
			CanonicalCoveredRefs: `[{"source":"assist-mem","rel_path":"assist-mem/covered.jsonl"}]`,
			SyncedAt:             "2026-07-04T00:00:00Z", LLMEmbedding: []float32{0.78, 0.62},
		},
	}))

	resp, err := MemoryRecall(ctx, store, &fakeMemoryRecallEmbedder{vector: []float32{1, 0}}, enabledEmbeddingConfig(), MemoryRecallRequest{
		Query:           "semantic query",
		TopK:            2,
		PreferCanonical: true,
	})

	require.NoError(t, err)
	assert.Equal(t, []string{"unrelated-a.md", "assist-mem/covered.jsonl"}, hitPaths(resp.Hits))
	assert.NotContains(t, hitPaths(resp.Hits), "canonical/covered.json")
}

func TestMemoryRecallExplicitSourceBypassesCanonicalSuppression(t *testing.T) {
	ctx := context.Background()
	store, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, store.Close()) })

	require.NoError(t, store.ReplaceMemories(ctx, []db.Memory{
		{
			RelPath: "assist-mem/entrypoint.jsonl", Source: db.SourceAssistMem,
			Title: "Raw assist entrypoint", Date: "2026-07-01", Status: "active",
			Body:     "lzn preview entrypoint lives in cmd/agentsview main",
			SyncedAt: "2026-07-01T00:00:00Z", LLMEmbedding: []float32{1, 0},
		},
		{
			RelPath: "canonical/entrypoint.json", Source: db.SourceCanonical,
			Title: "Canonical entrypoint", Date: "2026-07-02", Status: "active",
			Body:                 "lzn preview current entrypoint is cmd/agentsview main",
			CanonicalCoveredRefs: `[{"source":"assist-mem","rel_path":"assist-mem/entrypoint.jsonl"}]`,
			SyncedAt:             "2026-07-02T00:00:00Z", LLMEmbedding: []float32{1, 0},
		},
	}))

	resp, err := MemoryRecall(ctx, store, &fakeMemoryRecallEmbedder{vector: []float32{1, 0}}, enabledEmbeddingConfig(), MemoryRecallRequest{
		Query:           "lzn preview entrypoint",
		TopK:            5,
		PreferCanonical: true,
		Filter:          db.MemoryFilter{Source: db.SourceAssistMem},
	})

	require.NoError(t, err)
	require.Len(t, resp.Hits, 1)
	assert.Equal(t, "assist-mem/entrypoint.jsonl", resp.Hits[0].RelPath)
	assert.Equal(t, db.SourceAssistMem, resp.Hits[0].Source)
}

func TestMemoryRecallDefaultPreservesRawCompatibleBehavior(t *testing.T) {
	ctx := context.Background()
	store, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, store.Close()) })

	require.NoError(t, store.ReplaceMemories(ctx, []db.Memory{
		{
			RelPath: "assist-mem/entrypoint.jsonl", Source: db.SourceAssistMem,
			Title: "Raw assist entrypoint", Date: "2026-07-01", Status: "active",
			Body:     "lzn preview entrypoint lives in cmd/agentsview main",
			SyncedAt: "2026-07-01T00:00:00Z", LLMEmbedding: []float32{1, 0},
		},
		{
			RelPath: "canonical/entrypoint.json", Source: db.SourceCanonical,
			Title: "Canonical entrypoint", Date: "2026-07-02", Status: "active",
			Body:                 "lzn preview current entrypoint is cmd/agentsview main",
			CanonicalCoveredRefs: `[{"source":"assist-mem","rel_path":"assist-mem/entrypoint.jsonl"}]`,
			SyncedAt:             "2026-07-02T00:00:00Z", LLMEmbedding: []float32{1, 0},
		},
	}))

	resp, err := MemoryRecall(ctx, store, &fakeMemoryRecallEmbedder{vector: []float32{1, 0}}, enabledEmbeddingConfig(), MemoryRecallRequest{
		Query: "lzn preview entrypoint",
		TopK:  5,
	})

	require.NoError(t, err)
	paths := hitPaths(resp.Hits)
	assert.Contains(t, paths, "assist-mem/entrypoint.jsonl")
	assert.NotContains(t, paths, "canonical/entrypoint.json")
}

func TestMemoryRecallMalformedCanonicalCoverageDoesNotSuppress(t *testing.T) {
	ctx := context.Background()
	store, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, store.Close()) })

	require.NoError(t, store.ReplaceMemories(ctx, []db.Memory{
		{
			RelPath: "assist-mem/entrypoint.jsonl", Source: db.SourceAssistMem,
			Title: "Raw assist entrypoint", Date: "2026-07-01", Status: "active",
			Body:     "lzn preview entrypoint lives in cmd/agentsview main",
			SyncedAt: "2026-07-01T00:00:00Z", LLMEmbedding: []float32{0.96, 0.04},
		},
		{
			RelPath: "canonical/entrypoint.json", Source: db.SourceCanonical,
			Title: "Canonical entrypoint", Date: "2026-07-02", Status: "active",
			Body:                 "lzn preview current entrypoint is cmd/agentsview main",
			CanonicalCoveredRefs: `{not json`,
			SyncedAt:             "2026-07-02T00:00:00Z", LLMEmbedding: []float32{1, 0},
		},
	}))

	resp, err := MemoryRecall(ctx, store, &fakeMemoryRecallEmbedder{vector: []float32{1, 0}}, enabledEmbeddingConfig(), MemoryRecallRequest{
		Query:           "lzn preview entrypoint",
		TopK:            5,
		PreferCanonical: true,
	})

	require.NoError(t, err)
	paths := hitPaths(resp.Hits)
	assert.Contains(t, paths, "canonical/entrypoint.json")
	assert.Contains(t, paths, "assist-mem/entrypoint.jsonl")
}

func TestMemoryRecallDisabledConfigDoesNotCallEmbedder(t *testing.T) {
	embedder := &fakeMemoryRecallEmbedder{vector: []float32{1, 0}}

	resp, err := MemoryRecall(context.Background(), nil, embedder, config.LLMConfig{}, MemoryRecallRequest{Query: "semantic query", TopK: 5})

	require.NoError(t, err)
	assert.True(t, resp.Disabled)
	assert.Empty(t, resp.Hits)
	assert.False(t, embedder.called)
}

func enabledEmbeddingConfig() config.LLMConfig {
	return config.LLMConfig{
		Enabled: true,
		Embed: config.LLMEmbedConfig{
			BaseURL: "http://llm.test/v1",
			Model:   "text-embedding",
		},
	}
}

func hitPaths(hits []MemoryRecallHit) []string {
	paths := make([]string, 0, len(hits))
	for _, hit := range hits {
		paths = append(paths, hit.RelPath)
	}
	return paths
}

func findHit(t *testing.T, hits []MemoryRecallHit, relPath string) MemoryRecallHit {
	t.Helper()
	for _, hit := range hits {
		if hit.RelPath == relPath {
			return hit
		}
	}
	require.FailNowf(t, "missing hit", "rel_path %q not found in %#v", relPath, hits)
	return MemoryRecallHit{}
}
