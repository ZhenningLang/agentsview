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

func TestMemoryRecallAPIExposesCanonicalPreferenceAndSourceFilter(t *testing.T) {
	te := setupRecallFixture(t)

	wPreferred := postJSON(te, "/api/v1/memory/recall", `{"query":"lzn preview entrypoint","top_k":5,"prefer_canonical":true}`)
	assertStatus(t, wPreferred, http.StatusOK)
	preferred := decode[struct {
		Hits []recallAPIMetadataHit `json:"hits"`
	}](t, wPreferred)

	preferredPaths := recallAPIMetadataPaths(preferred.Hits)
	assert.Contains(t, preferredPaths, "canonical/entrypoint.json")
	assert.NotContains(t, preferredPaths, "assist-mem/entrypoint.jsonl")
	canonical := findRecallAPIHit(t, preferred.Hits, "canonical/entrypoint.json")
	assert.Equal(t, db.SourceCanonical, canonical.Source)
	assert.JSONEq(t, `[{"source":"assist-mem","rel_path":"assist-mem/entrypoint.jsonl"}]`, canonical.CanonicalCoveredRefs)
	assert.JSONEq(t, `{"topic":"entrypoint"}`, canonical.CanonicalProvenance)

	wRaw := postJSON(te, "/api/v1/memory/recall", `{"query":"lzn preview entrypoint","top_k":5,"prefer_canonical":true,"source":"assist-mem"}`)
	assertStatus(t, wRaw, http.StatusOK)
	raw := decode[struct {
		Hits []recallAPIPathHit `json:"hits"`
	}](t, wRaw)
	require.Len(t, raw.Hits, 1)
	assert.Equal(t, "assist-mem/entrypoint.jsonl", raw.Hits[0].RelPath)
	assert.Equal(t, db.SourceAssistMem, raw.Hits[0].Source)

	wTyped := postJSON(te, "/api/v1/memory/recall", `{"query":"lzn preview entrypoint","top_k":5,"prefer_canonical":true,"status":"active","problem_type":"entrypoint"}`)
	assertStatus(t, wTyped, http.StatusOK)
	typed := decode[struct {
		Hits []recallAPIPathHit `json:"hits"`
	}](t, wTyped)
	assert.Equal(t, []string{"canonical/entrypoint.json"}, recallAPIPaths(typed.Hits))
}

func TestMemoryRecallAPILznCanonicalFixture(t *testing.T) {
	te := setupRecallFixture(t)

	wPreferred := postJSON(te, "/api/v1/memory/recall", `{"query":"lzn preview entrypoint","top_k":5,"prefer_canonical":true}`)
	assertStatus(t, wPreferred, http.StatusOK)
	preferred := decode[struct {
		Hits []recallAPIPathHit `json:"hits"`
	}](t, wPreferred)
	preferredPaths := recallAPIPaths(preferred.Hits)
	assert.Contains(t, preferredPaths, "canonical/entrypoint.json")
	assert.NotContains(t, preferredPaths, "assist-mem/entrypoint.jsonl")
	assert.Contains(t, preferredPaths, "security.md")
	assert.Contains(t, preferredPaths, "proj/memory/environment.md")

	wSecurity := postJSON(te, "/api/v1/memory/recall", `{"query":"lzn preview security exception","top_k":5,"prefer_canonical":true,"source":"cross-agent"}`)
	assertStatus(t, wSecurity, http.StatusOK)
	security := decode[struct {
		Hits []recallAPIPathHit `json:"hits"`
	}](t, wSecurity)
	assert.Contains(t, recallAPIPaths(security.Hits), "security.md")

	wEnv := postJSON(te, "/api/v1/memory/recall", `{"query":"lzn preview environment fact","top_k":5,"prefer_canonical":true,"source":"cc-native"}`)
	assertStatus(t, wEnv, http.StatusOK)
	env := decode[struct {
		Hits []recallAPIPathHit `json:"hits"`
	}](t, wEnv)
	assert.Contains(t, recallAPIPaths(env.Hits), "proj/memory/environment.md")
}

func setupRecallFixture(t *testing.T) *testEnv {
	t.Helper()
	client := llmTestClient(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path != "/v1/embeddings" {
			return jsonResponse(http.StatusNotFound, `{}`), nil
		}
		return jsonResponse(http.StatusOK, `{"data":[{"embedding":[1,0]}]}`), nil
	})
	te := setupWithServerOpts(t, []server.Option{server.WithLLMHTTPClient(client)}, withLLMConfig(func(c *config.LLMConfig) {
		c.Enabled = true
		c.BaseURL = "http://llm.test/v1"
		c.APIKey = "secret-key"
		c.Model = "deepseek-chat"
		c.Embed.Model = "text-embedding"
	}))

	require.NoError(t, te.db.ReplaceMemories(t.Context(), []db.Memory{
		{
			RelPath: "assist-mem/entrypoint.jsonl", Source: db.SourceAssistMem,
			Title: "Raw assist entrypoint", Date: "2026-07-01", Status: "active", ProblemType: "entrypoint",
			Body:     "lzn preview entrypoint lives in cmd/agentsview main",
			SyncedAt: "2026-07-01T00:00:00Z", LLMEmbedding: []float32{1, 0},
		},
		{
			RelPath: "security.md", Source: db.SourceCrossAgent,
			Title: "Security exception", Date: "2026-07-01", Status: "active",
			Body:     "lzn preview security exception stays separate",
			SyncedAt: "2026-07-01T00:00:00Z", LLMEmbedding: []float32{0.94, 0.06},
		},
		{
			RelPath: "proj/memory/environment.md", Source: db.SourceCCNative,
			Title: "Environment fact", Date: "2026-07-01", Status: "active",
			Body:     "lzn preview environment fact uses preview namespace",
			SyncedAt: "2026-07-01T00:00:00Z", LLMEmbedding: []float32{0.93, 0.07},
		},
		{
			RelPath: "canonical/entrypoint.json", Source: db.SourceCanonical,
			Title: "Canonical entrypoint", Date: "2026-07-02", Status: "active", ProblemType: "entrypoint",
			Body:                 "lzn preview current entrypoint is cmd/agentsview main",
			CanonicalCoveredRefs: `[{"source":"assist-mem","rel_path":"assist-mem/entrypoint.jsonl"}]`,
			CanonicalProvenance:  `{"topic":"entrypoint"}`,
			SyncedAt:             "2026-07-02T00:00:00Z",
		},
	}))
	return te
}

type recallAPIPathHit struct {
	RelPath string `json:"rel_path"`
	Source  string `json:"source"`
}

type recallAPIMetadataHit struct {
	RelPath              string `json:"rel_path"`
	Source               string `json:"source"`
	CanonicalCoveredRefs string `json:"canonical_covered_refs"`
	CanonicalProvenance  string `json:"canonical_provenance"`
}

func recallAPIPaths(hits []recallAPIPathHit) []string {
	paths := make([]string, 0, len(hits))
	for _, hit := range hits {
		paths = append(paths, hit.RelPath)
	}
	return paths
}

func recallAPIMetadataPaths(hits []recallAPIMetadataHit) []string {
	paths := make([]string, 0, len(hits))
	for _, hit := range hits {
		paths = append(paths, hit.RelPath)
	}
	return paths
}

func findRecallAPIHit(t *testing.T, hits []recallAPIMetadataHit, relPath string) recallAPIMetadataHit {
	t.Helper()
	for _, hit := range hits {
		if hit.RelPath == relPath {
			return hit
		}
	}
	require.FailNowf(t, "missing recall hit", "rel_path %q not found in %#v", relPath, hits)
	return recallAPIMetadataHit{}
}
