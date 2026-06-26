package server_test

import (
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.kenn.io/agentsview/internal/config"
	"go.kenn.io/agentsview/internal/db"
	"go.kenn.io/agentsview/internal/server"
)

func TestMemoryRecallReturnsHybridCrossSourceHitsAndDropsNoise(t *testing.T) {
	client := llmTestClient(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path != "/v1/embeddings" {
			return jsonResponse(http.StatusNotFound, `{}`), nil
		}
		body, err := io.ReadAll(req.Body)
		require.NoError(t, err)
		assert.Contains(t, string(body), "semantic query")
		return jsonResponse(http.StatusOK, `{"data":[{"embedding":[1,0]}]}`), nil
	})
	te := setupWithServerOpts(t, []server.Option{server.WithLLMHTTPClient(client)}, withLLMConfig(func(c *config.LLMConfig) {
		c.Enabled = true
		c.BaseURL = "http://llm.test/v1"
		c.APIKey = "secret-key"
		c.Model = "deepseek-chat"
		c.Embed.Model = "text-embedding"
	}))

	memories := []db.Memory{
		{
			RelPath: "semantic-cross.md", Source: db.SourceCrossAgent, Title: "Semantic Cross",
			Date: "2026-06-24", Status: "active", Body: "zero shared keywords in this note",
			SyncedAt: "2026-06-24T00:00:00.000Z", LLMEmbedding: []float32{1, 0},
		},
		{
			RelPath: "proj/memory/semantic-cc.md", Source: db.SourceCCNative, Title: "Semantic CC",
			Date: "2026-06-24", Status: "active", Body: "another unrelated wording sample",
			SyncedAt: "2026-06-24T00:00:00.000Z", LLMEmbedding: []float32{0.95, 0.05},
		},
		{
			RelPath: "noise.md", Source: db.SourceCrossAgent, Title: "Noise",
			Date: "2026-06-24", Status: "active", Body: "noise",
			SyncedAt: "2026-06-24T00:00:00.000Z", LLMEmbedding: []float32{0, 1},
		},
	}
	require.NoError(t, te.db.ReplaceMemories(t.Context(), memories))

	w := postJSON(te, "/api/v1/memory/recall", `{"query":"semantic query","top_k":5}`)
	assertStatus(t, w, http.StatusOK)
	resp := decode[struct {
		Hits []struct {
			RelPath  string  `json:"rel_path"`
			Source   string  `json:"source"`
			Semantic float64 `json:"semantic"`
		} `json:"hits"`
	}](t, w)
	require.Len(t, resp.Hits, 2)
	paths := []string{resp.Hits[0].RelPath, resp.Hits[1].RelPath}
	assert.Contains(t, paths, "semantic-cross.md")
	assert.Contains(t, paths, "proj/memory/semantic-cc.md")
	assert.NotContains(t, paths, "noise.md")
	assert.ElementsMatch(t, []string{db.SourceCrossAgent, db.SourceCCNative}, []string{resp.Hits[0].Source, resp.Hits[1].Source})
}

func TestMemoryRecallDisabledEnvelopeWhenEmbeddingUnavailable(t *testing.T) {
	called := false
	client := llmTestClient(func(req *http.Request) (*http.Response, error) {
		called = true
		return jsonResponse(http.StatusOK, `{}`), nil
	})
	te := setupWithServerOpts(t, []server.Option{server.WithLLMHTTPClient(client)}, withLLMConfig(func(c *config.LLMConfig) {
		c.Enabled = true
		c.BaseURL = "http://llm.test/v1"
		c.APIKey = "secret-key"
		c.Model = "deepseek-chat"
		c.Embed.Model = ""
	}))

	w := postJSON(te, "/api/v1/memory/recall", `{"query":"semantic query","top_k":5}`)
	assertStatus(t, w, http.StatusOK)
	resp := decode[struct {
		Disabled bool          `json:"disabled"`
		Hits     []interface{} `json:"hits"`
		Count    int           `json:"count"`
	}](t, w)
	assert.True(t, resp.Disabled)
	assert.Empty(t, resp.Hits)
	assert.Zero(t, resp.Count)
	assert.False(t, called, "disabled embedding config must not call provider")
}
