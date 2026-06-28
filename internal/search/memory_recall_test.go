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
