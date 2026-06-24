package server_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.kenn.io/agentsview/internal/config"
	"go.kenn.io/agentsview/internal/db"
	"go.kenn.io/agentsview/internal/server"
)

func TestSemanticSearchDisabledDoesNotCallProvider(t *testing.T) {
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
	}))

	w := te.get(t, "/api/v1/search/semantic?q=auth")
	assertStatus(t, w, http.StatusOK)
	resp := decode[map[string]any](t, w)
	assert.Equal(t, true, resp["disabled"])
	assert.Equal(t, float64(0), resp["count"])
	assert.False(t, called)

	statusW := te.get(t, "/api/v1/search/semantic/status")
	assertStatus(t, statusW, http.StatusOK)
	status := decode[map[string]any](t, statusW)
	assert.Equal(t, false, status["available"])
}

func TestSemanticSearchReturnsCosineTopK(t *testing.T) {
	client := llmTestClient(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path != "/v1/embeddings" {
			return jsonResponse(http.StatusNotFound, `{}`), nil
		}
		body, err := io.ReadAll(req.Body)
		require.NoError(t, err)
		assert.NotContains(t, string(body), "secret-key")
		return jsonResponse(http.StatusOK, `{"data":[{"embedding":[1,0]}]}`), nil
	})
	te := setupWithServerOpts(t, []server.Option{server.WithLLMHTTPClient(client)}, withLLMConfig(func(c *config.LLMConfig) {
		c.Enabled = true
		c.BaseURL = "http://llm.test/v1"
		c.APIKey = "secret-key"
		c.Model = "deepseek-chat"
		c.Embed.Model = "text-embedding"
	}))
	seedSemanticSession(t, te, "best", "proj", []float32{1, 0})
	seedSemanticSession(t, te, "second", "proj", []float32{0.5, 0.5})
	seedSemanticSession(t, te, "other", "other", []float32{1, 0})

	w := te.get(t, "/api/v1/search/semantic?q=auth&project=proj&k=1")
	assertStatus(t, w, http.StatusOK)
	resp := decode[searchResponse](t, w)
	require.Len(t, resp.Results, 1)
	assert.Equal(t, "best", resp.Results[0].SessionID)
	assert.Equal(t, -1, resp.Results[0].Ordinal)
	assert.True(t, strings.Contains(resp.Results[0].Snippet, "Semantic"))

	statusW := te.get(t, "/api/v1/search/semantic/status")
	assertStatus(t, statusW, http.StatusOK)
	status := decode[map[string]any](t, statusW)
	assert.Equal(t, true, status["available"])
}

func TestSemanticSearchAllowsNoAPIKeyEmbeddingProvider(t *testing.T) {
	client := llmTestClient(func(req *http.Request) (*http.Response, error) {
		assert.Empty(t, req.Header.Get("Authorization"))
		return jsonResponse(http.StatusOK, `{"data":[{"embedding":[1,0]}]}`), nil
	})
	te := setupWithServerOpts(t, []server.Option{server.WithLLMHTTPClient(client)}, withLLMConfig(func(c *config.LLMConfig) {
		c.Enabled = true
		c.BaseURL = "http://chat.test/v1"
		c.APIKey = "chat-secret"
		c.Model = "deepseek-chat"
		c.Embed.BaseURL = "http://localhost:11434/v1"
		c.Embed.Model = "nomic-embed-text"
		c.Embed.APIKey = ""
	}))
	seedSemanticSession(t, te, "best", "proj", []float32{1, 0})

	statusW := te.get(t, "/api/v1/search/semantic/status")
	assertStatus(t, statusW, http.StatusOK)
	status := decode[map[string]any](t, statusW)
	assert.Equal(t, true, status["available"])

	w := te.get(t, "/api/v1/search/semantic?q=auth")
	assertStatus(t, w, http.StatusOK)
	resp := decode[searchResponse](t, w)
	require.Len(t, resp.Results, 1)
	assert.Equal(t, "best", resp.Results[0].SessionID)
}

func TestSemanticSearchRejectsRemoteRequestsBeforeProviderCall(t *testing.T) {
	called := false
	client := llmTestClient(func(req *http.Request) (*http.Response, error) {
		called = true
		return jsonResponse(http.StatusOK, `{"data":[{"embedding":[1,0]}]}`), nil
	})
	te := setupWithServerOpts(t, []server.Option{server.WithLLMHTTPClient(client)}, withLLMConfig(func(c *config.LLMConfig) {
		c.Enabled = true
		c.BaseURL = "http://llm.test/v1"
		c.APIKey = "secret-key"
		c.Model = "deepseek-chat"
		c.Embed.Model = "text-embedding"
	}))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/search/semantic?q=auth", nil)
	req.Header.Set("X-Forwarded-For", "203.0.113.9")
	w := httptest.NewRecorder()
	te.handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code, w.Body.String())
	assert.False(t, called)
}

func seedSemanticSession(t *testing.T, te *testEnv, id, project string, vector []float32) {
	t.Helper()
	first := id + " first"
	started := "2026-06-24T10:00:00Z"
	ended := "2026-06-24T11:00:00Z"
	require.NoError(t, te.db.UpsertSession(db.Session{
		ID:               id,
		Project:          project,
		Machine:          "test",
		Agent:            "claude",
		FirstMessage:     &first,
		StartedAt:        &started,
		EndedAt:          &ended,
		MessageCount:     1,
		UserMessageCount: 1,
	}))
	require.NoError(t, te.db.WriteEnrichment(t.Context(), id, db.EnrichmentWrite{
		Title: "LLM " + id, Summary: "summary", Keywords: []string{"auth"}, Model: "model", MessageCnt: 1,
		Embedding: vector, HasEmbedding: true,
	}))
}
