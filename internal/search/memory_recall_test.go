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
