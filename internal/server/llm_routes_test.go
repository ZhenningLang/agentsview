package server_test

import (
	"bytes"
	"errors"
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

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func llmTestClient(f roundTripFunc) *http.Client {
	return &http.Client{Transport: f}
}

func withLLMConfig(fn func(*config.LLMConfig)) setupOption {
	return func(c *config.Config) { fn(&c.LLM) }
}

func postJSON(te *testEnv, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	te.handler.ServeHTTP(w, req)
	return w
}

func requestJSON(te *testEnv, method, path, body string, mods ...func(*http.Request)) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	for _, mod := range mods {
		mod(req)
	}
	w := httptest.NewRecorder()
	te.handler.ServeHTTP(w, req)
	return w
}

func TestLLMRoutes(t *testing.T) {
	te := setup(t)

	statusW := te.get(t, "/api/v1/llm/enrich/status")
	assertStatus(t, statusW, http.StatusOK)
	status := decode[db.EnrichmentStatusReport](t, statusW)
	assert.Equal(t, 0, status.Total)

	balanceW := te.get(t, "/api/v1/llm/balance")
	assertStatus(t, balanceW, http.StatusOK)
	balance := decode[map[string]any](t, balanceW)
	assert.Equal(t, false, balance["supported"])

	enrichW := postJSON(te, "/api/v1/llm/enrich", `{}`)
	assert.Equal(t, http.StatusConflict, enrichW.Code, enrichW.Body.String())
}

func TestLLMRoutesRejectRemoteRequests(t *testing.T) {
	client := llmTestClient(func(req *http.Request) (*http.Response, error) {
		t.Fatal("provider should not be called for remote requests")
		return nil, nil
	})
	te := setupWithServerOpts(t, []server.Option{server.WithLLMHTTPClient(client)}, withLLMConfig(func(c *config.LLMConfig) {
		c.Enabled = true
		c.BaseURL = "https://api.deepseek.com/v1"
		c.APIKey = "secret-key"
		c.Model = "deepseek-chat"
	}))
	remote := func(req *http.Request) { req.Header.Set("X-Forwarded-For", "203.0.113.9") }

	for _, tc := range []struct {
		method string
		path   string
		body   string
	}{
		{method: http.MethodPost, path: "/api/v1/llm/enrich", body: `{}`},
		{method: http.MethodGet, path: "/api/v1/llm/enrich/status"},
		{method: http.MethodGet, path: "/api/v1/llm/balance"},
	} {
		w := requestJSON(te, tc.method, tc.path, tc.body, remote)
		assert.Equal(t, http.StatusForbidden, w.Code, "%s %s body: %s", tc.method, tc.path, w.Body.String())
	}
}

func TestLLMEnrichRejectsDisabledOrUnconfigured(t *testing.T) {
	called := false
	client := llmTestClient(func(req *http.Request) (*http.Response, error) {
		called = true
		return jsonResponse(http.StatusOK, `{}`), nil
	})

	disabled := setupWithServerOpts(t, []server.Option{server.WithLLMHTTPClient(client)})
	w := postJSON(disabled, "/api/v1/llm/enrich", `{}`)
	assert.Equal(t, http.StatusConflict, w.Code, w.Body.String())
	assert.False(t, called)

	missingKey := setupWithServerOpts(t, []server.Option{server.WithLLMHTTPClient(client)}, withLLMConfig(func(c *config.LLMConfig) {
		c.Enabled = true
		c.BaseURL = "http://llm.test/v1"
		c.Model = "deepseek-chat"
	}))
	w = postJSON(missingKey, "/api/v1/llm/enrich", `{}`)
	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	assert.False(t, called)
}

func TestLLMEnrichRejectsReadOnlyMode(t *testing.T) {
	te := setupPGMode(t)

	w := postJSON(te, "/api/v1/llm/enrich", `{}`)
	assert.Equal(t, http.StatusNotImplemented, w.Code, w.Body.String())
}

func TestLLMEnrichRunsMockedBatch(t *testing.T) {
	chatCalls := 0
	client := llmTestClient(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path != "/v1/chat/completions" {
			return jsonResponse(http.StatusNotFound, `{}`), nil
		}
		chatCalls++
		body, err := io.ReadAll(req.Body)
		require.NoError(t, err)
		assert.NotContains(t, string(body), "secret-key")
		return jsonResponse(http.StatusOK, `{"choices":[{"message":{"content":"{\"title\":\"LLM title\",\"summary\":\"LLM summary\",\"keywords\":[\"auth\",\"token\"]}"}}]}`), nil
	})
	te := setupWithServerOpts(t, []server.Option{server.WithLLMHTTPClient(client)}, withLLMConfig(func(c *config.LLMConfig) {
		c.Enabled = true
		c.BaseURL = "http://llm.test/v1"
		c.APIKey = "secret-key"
		c.Model = "deepseek-chat"
		c.MinUserMessages = 3
		c.ReenrichIdleMinutes = 30
	}))
	seedLLMEligibleSession(t, te)

	w := postJSON(te, "/api/v1/llm/enrich", `{"limit":1}`)
	assertStatus(t, w, http.StatusOK)
	resp := decode[map[string]any](t, w)
	assert.Equal(t, float64(1), resp["candidates"])
	assert.Equal(t, float64(1), resp["enriched"])
	assert.Equal(t, float64(0), resp["errors"])
	assert.Equal(t, 1, chatCalls)

	sess, err := te.db.GetSession(t.Context(), "llm-eligible")
	require.NoError(t, err)
	require.NotNil(t, sess)
	assert.Equal(t, "LLM title", sess.LLMTitle)
	assert.Equal(t, "LLM summary", sess.LLMSummary)
	assert.Equal(t, "auth,token", sess.LLMKeywords)
	assert.Equal(t, db.EnrichStatusOK, sess.EnrichStatus)
}

func TestLLMEnrichDefaultLimitIsBounded(t *testing.T) {
	chatCalls := 0
	client := llmTestClient(func(req *http.Request) (*http.Response, error) {
		chatCalls++
		return jsonResponse(http.StatusOK, `{"choices":[{"message":{"content":"{\"title\":\"LLM title\",\"summary\":\"LLM summary\",\"keywords\":[\"auth\"]}"}}]}`), nil
	})
	te := setupWithServerOpts(t, []server.Option{server.WithLLMHTTPClient(client)}, withLLMConfig(func(c *config.LLMConfig) {
		c.Enabled = true
		c.BaseURL = "http://llm.test/v1"
		c.APIKey = "secret-key"
		c.Model = "deepseek-chat"
		c.MinUserMessages = 3
		c.ReenrichIdleMinutes = 30
		c.Concurrency = 1
	}))
	for i := 0; i < 30; i++ {
		seedLLMEligibleSessionID(t, te, "llm-eligible-"+string(rune('a'+i)))
	}

	w := postJSON(te, "/api/v1/llm/enrich", `{}`)
	assertStatus(t, w, http.StatusOK)
	resp := decode[map[string]any](t, w)
	assert.Equal(t, float64(25), resp["candidates"])
	assert.Equal(t, float64(25), resp["enriched"])
	assert.Equal(t, 25, chatCalls)
}

func TestLLMEnrichStatus(t *testing.T) {
	te := setup(t)
	seedLLMStatusSession(t, te, "pending", "")
	seedLLMStatusSession(t, te, "ok", db.EnrichStatusOK)
	seedLLMStatusSession(t, te, "short", db.EnrichStatusSkippedTooShort)
	seedLLMStatusSession(t, te, "empty", db.EnrichStatusNoContent)
	seedLLMStatusSession(t, te, "error", db.EnrichStatusError)

	w := te.get(t, "/api/v1/llm/enrich/status")
	assertStatus(t, w, http.StatusOK)
	status := decode[db.EnrichmentStatusReport](t, w)
	assert.Equal(t, 5, status.Total)
	assert.Equal(t, 1, status.Enriched)
	assert.Equal(t, 1, status.Pending)
	assert.Equal(t, 1, status.SkippedTooShort)
	assert.Equal(t, 1, status.NoContent)
	assert.Equal(t, 1, status.Errors)
	assert.Equal(t, 1, status.ByStatus[""])
}

func TestLLMBalanceDeepSeek(t *testing.T) {
	var gotPath string
	var gotAuth string
	client := llmTestClient(func(req *http.Request) (*http.Response, error) {
		gotPath = req.URL.Path
		gotAuth = req.Header.Get("Authorization")
		return jsonResponse(http.StatusOK, `{"is_available":true,"balance_infos":[{"currency":"CNY","total_balance":"12.34"}]}`), nil
	})
	te := setupWithServerOpts(t, []server.Option{server.WithLLMHTTPClient(client)}, withLLMConfig(func(c *config.LLMConfig) {
		c.Enabled = true
		c.BaseURL = "https://api.deepseek.com/v1"
		c.APIKey = "secret-key"
		c.Model = "deepseek-chat"
	}))

	w := te.get(t, "/api/v1/llm/balance")
	assertStatus(t, w, http.StatusOK)
	resp := decode[map[string]any](t, w)
	assert.Equal(t, "/user/balance", gotPath)
	assert.Equal(t, "Bearer secret-key", gotAuth)
	assert.Equal(t, true, resp["supported"])
	assert.Equal(t, "CNY", resp["currency"])
	assert.Equal(t, "12.34", resp["amount"])
	assert.Equal(t, true, resp["available"])
	assert.NotContains(t, w.Body.String(), "secret-key")
}

func TestLLMBalanceUnsupportedOrFailed(t *testing.T) {
	tests := []struct {
		name   string
		cfg    config.LLMConfig
		client *http.Client
	}{
		{name: "disabled", cfg: config.LLMConfig{BaseURL: "https://api.deepseek.com", APIKey: "secret-key"}},
		{name: "missing key", cfg: config.LLMConfig{Enabled: true, BaseURL: "https://api.deepseek.com"}},
		{name: "unknown provider", cfg: config.LLMConfig{Enabled: true, BaseURL: "https://example.com/v1", APIKey: "secret-key"}},
		{name: "provider name only in path", cfg: config.LLMConfig{Enabled: true, BaseURL: "https://example.com/deepseek/v1", APIKey: "secret-key"}},
		{name: "provider error", cfg: config.LLMConfig{Enabled: true, BaseURL: "https://api.deepseek.com", APIKey: "secret-key"}, client: llmTestClient(func(req *http.Request) (*http.Response, error) {
			return jsonResponse(http.StatusInternalServerError, `{"error":"boom secret-key"}`), nil
		})},
		{name: "malformed", cfg: config.LLMConfig{Enabled: true, BaseURL: "https://api.deepseek.com", APIKey: "secret-key"}, client: llmTestClient(func(req *http.Request) (*http.Response, error) {
			return jsonResponse(http.StatusOK, `{bad-json`), nil
		})},
		{name: "network error", cfg: config.LLMConfig{Enabled: true, BaseURL: "https://api.deepseek.com", APIKey: "secret-key"}, client: llmTestClient(func(req *http.Request) (*http.Response, error) {
			return nil, errors.New("dial failed secret-key")
		})},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := tt.client
			if client == nil {
				client = llmTestClient(func(req *http.Request) (*http.Response, error) {
					t.Fatal("provider should not be called")
					return nil, nil
				})
			}
			te := setupWithServerOpts(t, []server.Option{server.WithLLMHTTPClient(client)}, withLLMConfig(func(c *config.LLMConfig) {
				*c = tt.cfg
			}))
			w := te.get(t, "/api/v1/llm/balance")
			assertStatus(t, w, http.StatusOK)
			resp := decode[map[string]any](t, w)
			assert.Equal(t, false, resp["supported"])
			assert.NotContains(t, w.Body.String(), "secret-key")
		})
	}
}

func TestLLMBalanceIncludesUnavailableFalse(t *testing.T) {
	client := llmTestClient(func(req *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusOK, `{"is_available":false,"balance_infos":[{"currency":"CNY","total_balance":0}]}`), nil
	})
	te := setupWithServerOpts(t, []server.Option{server.WithLLMHTTPClient(client)}, withLLMConfig(func(c *config.LLMConfig) {
		c.Enabled = true
		c.BaseURL = "https://api.deepseek.com/v1"
		c.APIKey = "secret-key"
		c.Model = "deepseek-chat"
	}))

	w := te.get(t, "/api/v1/llm/balance")
	assertStatus(t, w, http.StatusOK)
	resp := decode[map[string]any](t, w)
	assert.Equal(t, true, resp["supported"])
	available, ok := resp["available"]
	require.True(t, ok, "available must be present when provider returns false")
	assert.Equal(t, false, available)
	assert.Equal(t, "0", resp["amount"])
}

func jsonResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(bytes.NewBufferString(body)),
	}
}

func seedLLMEligibleSession(t *testing.T, te *testEnv) {
	seedLLMEligibleSessionID(t, te, "llm-eligible")
}

func seedLLMEligibleSessionID(t *testing.T, te *testEnv, id string) {
	t.Helper()
	first := "eligible first message"
	started := "2026-06-24T10:00:00Z"
	ended := "2026-06-24T11:00:00Z"
	require.NoError(t, te.db.UpsertSession(db.Session{
		ID:               id,
		Project:          "proj",
		Machine:          "test",
		Agent:            "claude",
		FirstMessage:     &first,
		StartedAt:        &started,
		EndedAt:          &ended,
		MessageCount:     4,
		UserMessageCount: 3,
	}))
	te.seedMessages(t, id, 4, func(i int, m *db.Message) {
		m.Content = "meaningful user content for enrichment"
		m.ContentLength = len(m.Content)
	})
}

func seedLLMStatusSession(t *testing.T, te *testEnv, id, status string) {
	t.Helper()
	first := id + " first"
	started := "2026-06-24T10:00:00Z"
	require.NoError(t, te.db.UpsertSession(db.Session{
		ID:               id,
		Project:          "proj",
		Machine:          "test",
		Agent:            "claude",
		FirstMessage:     &first,
		StartedAt:        &started,
		MessageCount:     1,
		UserMessageCount: 1,
	}))
	switch status {
	case "":
		return
	case db.EnrichStatusOK:
		require.NoError(t, te.db.WriteEnrichment(t.Context(), id, db.EnrichmentWrite{
			Title: "title", Summary: "summary", Keywords: []string{"key"}, Model: "model", MessageCnt: 1,
		}))
	case db.EnrichStatusSkippedTooShort, db.EnrichStatusNoContent, db.EnrichStatusError:
		require.NoError(t, te.db.WriteEnrichment(t.Context(), id, db.EnrichmentWrite{Status: status, Error: "err"}))
	default:
		t.Fatalf("unsupported test status %q", status)
	}
}
