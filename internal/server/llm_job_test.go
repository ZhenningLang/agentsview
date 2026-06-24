package server_test

import (
	"fmt"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.kenn.io/agentsview/internal/config"
	"go.kenn.io/agentsview/internal/db"
	"go.kenn.io/agentsview/internal/server"
)

func seedEnrichCandidate(t *testing.T, te *testEnv, id string) {
	t.Helper()
	first := id
	ended := "2026-06-24T09:00:00Z"
	require.NoError(t, te.db.UpsertSession(db.Session{
		ID:               id,
		Project:          "proj",
		Machine:          "test",
		Agent:            "claude",
		FirstMessage:     &first,
		EndedAt:          &ended,
		MessageCount:     3,
		UserMessageCount: 2,
	}))
	require.NoError(t, te.db.InsertMessages([]db.Message{
		{SessionID: id, Ordinal: 0, Role: "user", Content: "need auth help", ContentLength: 14},
		{SessionID: id, Ordinal: 1, Role: "assistant", Content: "use tokens", ContentLength: 10},
	}))
}

func enrichEnabledConfig(c *config.LLMConfig) {
	c.Enabled = true
	c.BaseURL = "https://chat.example/v1"
	c.APIKey = "secret"
	c.Model = "chat-model"
	c.MinUserMessages = 2
	c.Concurrency = 2
}

func TestLLMEnrichmentJobRunsToCompletion(t *testing.T) {
	client := llmTestClient(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/v1/chat/completions":
			return jsonResponse(http.StatusOK,
				`{"choices":[{"message":{"content":"{\"title\":\"T\",\"summary\":\"S\",\"keywords\":[\"k\"]}"}}]}`), nil
		case "/v1/embeddings":
			return jsonResponse(http.StatusOK, `{"data":[{"embedding":[0.1,0.2]}]}`), nil
		}
		return jsonResponse(http.StatusNotFound, `{}`), nil
	})
	te := setupWithServerOpts(t,
		[]server.Option{server.WithLLMHTTPClient(client)},
		withLLMConfig(enrichEnabledConfig))

	const n = 3
	for i := 0; i < n; i++ {
		seedEnrichCandidate(t, te, fmt.Sprintf("s%d", i))
	}

	startW := postJSON(te, "/api/v1/llm/enrich/start", `{}`)
	assertStatus(t, startW, http.StatusOK)
	start := decode[map[string]any](t, startW)
	assert.Equal(t, true, start["running"])
	assert.Equal(t, "manual", start["source"])

	require.Eventually(t, func() bool {
		job := decode[map[string]any](t, te.get(t, "/api/v1/llm/enrich/job"))
		return job["running"] == false && job["done_at"] != ""
	}, 5*time.Second, 20*time.Millisecond)

	job := decode[map[string]any](t, te.get(t, "/api/v1/llm/enrich/job"))
	assert.EqualValues(t, n, job["succeeded"])
	assert.EqualValues(t, 0, job["failed"])
	assert.EqualValues(t, n, job["total"])
	assert.Empty(t, job["error"])

	status := decode[db.EnrichmentStatusReport](t, te.get(t, "/api/v1/llm/enrich/status"))
	assert.Equal(t, n, status.Enriched)
}

func TestLLMEnrichmentJobReportsTokensAndCost(t *testing.T) {
	var mu sync.Mutex
	balanceCalls := 0
	balances := []string{"100.0000", "99.5000"}
	client := llmTestClient(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/v1/chat/completions":
			return jsonResponse(http.StatusOK,
				`{"choices":[{"message":{"content":"{\"title\":\"T\",\"summary\":\"S\",\"keywords\":[\"k\"]}"}}],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}`), nil
		case "/v1/embeddings":
			return jsonResponse(http.StatusOK,
				`{"data":[{"embedding":[0.1,0.2]}],"usage":{"prompt_tokens":7,"total_tokens":7}}`), nil
		case "/user/balance":
			mu.Lock()
			amount := balances[len(balances)-1]
			if balanceCalls < len(balances) {
				amount = balances[balanceCalls]
			}
			balanceCalls++
			mu.Unlock()
			return jsonResponse(http.StatusOK,
				`{"is_available":true,"balance_infos":[{"currency":"CNY","total_balance":"`+amount+`"}]}`), nil
		}
		return jsonResponse(http.StatusNotFound, `{}`), nil
	})
	te := setupWithServerOpts(t,
		[]server.Option{server.WithLLMHTTPClient(client)},
		withLLMConfig(func(c *config.LLMConfig) {
			c.Enabled = true
			c.BaseURL = "https://api.deepseek.com/v1"
			c.APIKey = "secret"
			c.Model = "deepseek-chat"
			c.MinUserMessages = 2
			c.Concurrency = 2
			c.Embed = config.LLMEmbedConfig{
				BaseURL: "https://api.deepseek.com/v1",
				APIKey:  "secret",
				Model:   "embed-model",
			}
		}))

	const n = 2
	for i := 0; i < n; i++ {
		seedEnrichCandidate(t, te, fmt.Sprintf("s%d", i))
	}

	startW := postJSON(te, "/api/v1/llm/enrich/start", `{}`)
	assertStatus(t, startW, http.StatusOK)

	require.Eventually(t, func() bool {
		job := decode[map[string]any](t, te.get(t, "/api/v1/llm/enrich/job"))
		return job["running"] == false && job["done_at"] != ""
	}, 5*time.Second, 20*time.Millisecond)

	job := decode[map[string]any](t, te.get(t, "/api/v1/llm/enrich/job"))
	assert.EqualValues(t, n*10, job["prompt_tokens"])
	assert.EqualValues(t, n*5, job["completion_tokens"])
	assert.EqualValues(t, n*7, job["embed_tokens"])
	assert.Equal(t, "CNY", job["cost_currency"])
	assert.Equal(t, "0.5000", job["cost_spent"])
	assert.Equal(t, "99.5000", job["balance_end"])
}

func TestLLMEnrichmentJobStartRejectsDisabled(t *testing.T) {
	te := setup(t)
	w := postJSON(te, "/api/v1/llm/enrich/start", `{}`)
	assert.Equal(t, http.StatusConflict, w.Code, w.Body.String())
}

func TestLLMEnrichmentJobStartRequiresCredentials(t *testing.T) {
	te := setup(t, withLLMConfig(func(c *config.LLMConfig) {
		c.Enabled = true
		c.BaseURL = "https://chat.example/v1"
		c.Model = "chat-model"
		// API key intentionally missing.
	}))
	w := postJSON(te, "/api/v1/llm/enrich/start", `{}`)
	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
}

func TestLLMEnrichmentJobStartRejectsReadOnly(t *testing.T) {
	te := setupPGMode(t)
	w := postJSON(te, "/api/v1/llm/enrich/start", `{}`)
	assert.Equal(t, http.StatusForbidden, w.Code, w.Body.String())
}

func TestLLMEnrichmentJobRoutesRejectRemote(t *testing.T) {
	te := setup(t, withLLMConfig(enrichEnabledConfig))
	remote := func(req *http.Request) { req.Header.Set("X-Forwarded-For", "203.0.113.9") }
	for _, tc := range []struct {
		method string
		path   string
		body   string
	}{
		{http.MethodPost, "/api/v1/llm/enrich/start", `{}`},
		{http.MethodPost, "/api/v1/llm/enrich/stop", `{}`},
		{http.MethodGet, "/api/v1/llm/enrich/job", ""},
	} {
		w := requestJSON(te, tc.method, tc.path, tc.body, remote)
		assert.Equal(t, http.StatusForbidden, w.Code,
			"%s %s: %s", tc.method, tc.path, w.Body.String())
	}
}

func TestLLMEnrichmentJobStopReportsState(t *testing.T) {
	te := setup(t, withLLMConfig(enrichEnabledConfig))
	// No job running: stop is a no-op returning an idle state.
	w := postJSON(te, "/api/v1/llm/enrich/stop", `{}`)
	assertStatus(t, w, http.StatusOK)
	state := decode[map[string]any](t, w)
	assert.Equal(t, false, state["running"])
}
