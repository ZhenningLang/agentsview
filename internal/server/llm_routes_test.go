package server_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.kenn.io/agentsview/internal/config"
	"go.kenn.io/agentsview/internal/consolidate"
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
	called := false
	client := llmTestClient(func(req *http.Request) (*http.Response, error) {
		called = true
		return jsonResponse(http.StatusOK, `{}`), nil
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
		{method: http.MethodGet, path: "/api/v1/config/llm"},
		{method: http.MethodPost, path: "/api/v1/config/llm", body: `{}`},
		{method: http.MethodGet, path: "/api/v1/config/llm/providers"},
		{method: http.MethodPatch, path: "/api/v1/config/llm/providers", body: `{}`},
		{method: http.MethodGet, path: "/api/v1/config/consolidate"},
		{method: http.MethodPatch, path: "/api/v1/config/consolidate", body: `{}`},
		{method: http.MethodPost, path: "/api/v1/llm/test", body: `{}`},
	} {
		w := requestJSON(te, tc.method, tc.path, tc.body, remote)
		assert.Equal(t, http.StatusForbidden, w.Code, "%s %s body: %s", tc.method, tc.path, w.Body.String())
	}
	assert.False(t, called)
	_, err := os.Stat(filepath.Join(te.dataDir, "config.toml"))
	assert.True(t, os.IsNotExist(err), "remote config save must not touch disk")
}

func TestLLMConfigGetMasksAPIKeys(t *testing.T) {
	te := setup(t, withLLMConfig(func(c *config.LLMConfig) {
		c.Enabled = true
		c.BaseURL = "https://chat.example/v1"
		c.APIKey = "chat-secret-1234"
		c.Model = "chat-model"
		c.ReasoningEffort = "low"
		c.MinUserMessages = 3
		c.ReenrichMsgDelta = 4
		c.ReenrichIdleMinutes = 5
		c.Concurrency = 2
		c.Periodic = true
		c.Embed = config.LLMEmbedConfig{
			BaseURL: "https://embed.example/v1",
			APIKey:  "embed-secret-5678",
			Model:   "embed-model",
		}
		c.Providers = map[string]config.LLMConfig{
			"deepseek-chat": {
				BaseURL: "https://deepseek.example/v1",
				APIKey:  "provider-secret-9999",
				Model:   "deepseek-chat",
			},
		}
		c.Usage = map[string]string{"enrich": "deepseek-chat"}
	}))

	w := te.get(t, "/api/v1/config/llm")
	assertStatus(t, w, http.StatusOK)
	assert.NotContains(t, w.Body.String(), "chat-secret-1234")
	assert.NotContains(t, w.Body.String(), "embed-secret-5678")
	assert.NotContains(t, w.Body.String(), "provider-secret-9999")
	resp := decode[map[string]any](t, w)
	assert.Equal(t, true, resp["enabled"])
	assert.Equal(t, "https://chat.example/v1", resp["base_url"])
	assert.Equal(t, true, resp["has_api_key"])
	assert.Equal(t, "1234", resp["api_key_preview"])
	embed := resp["embed"].(map[string]any)
	assert.Equal(t, true, embed["has_api_key"])
	assert.Equal(t, "5678", embed["api_key_preview"])
	providers := resp["providers"].(map[string]any)
	deepseek := providers["deepseek-chat"].(map[string]any)
	assert.Equal(t, true, deepseek["has_api_key"])
	assert.Equal(t, "9999", deepseek["api_key_preview"])
	usage := resp["usage"].(map[string]any)
	assert.Equal(t, "deepseek-chat", usage["enrich"])
}

func TestLLMConfigGetReportsDanglingUsageBindings(t *testing.T) {
	te := setup(t, withLLMConfig(func(c *config.LLMConfig) {
		c.Providers = map[string]config.LLMConfig{
			"deepseek-chat": {BaseURL: "https://deepseek.example/v1", Model: "deepseek-chat"},
		}
		c.Usage = map[string]string{"embed": "typo-provider"}
	}))

	w := te.get(t, "/api/v1/config/llm")
	assertStatus(t, w, http.StatusOK)
	resp := decode[map[string]any](t, w)
	warnings := resp["usage_warnings"].([]any)
	require.Len(t, warnings, 1)
	assert.Contains(t, warnings[0].(string), `usage "embed"`)
	assert.Contains(t, warnings[0].(string), `"typo-provider"`)
}

func TestLLMConfigGetDoesNotPreviewShortAPIKeys(t *testing.T) {
	te := setup(t, withLLMConfig(func(c *config.LLMConfig) {
		c.APIKey = "abcd"
		c.Embed.APIKey = "xyz"
	}))

	w := te.get(t, "/api/v1/config/llm")
	assertStatus(t, w, http.StatusOK)
	assert.NotContains(t, w.Body.String(), "abcd")
	assert.NotContains(t, w.Body.String(), "xyz")
	resp := decode[map[string]any](t, w)
	assert.Equal(t, true, resp["has_api_key"])
	assert.NotContains(t, resp, "api_key_preview")
	embed := resp["embed"].(map[string]any)
	assert.Equal(t, true, embed["has_api_key"])
	assert.NotContains(t, embed, "api_key_preview")
}

// TestLLMConfigEmbedBalanceURLRoundTrip locks that the embed provider's
// own balance endpoint is configurable via the config patch and returned
// on GET, so the UI can set it independently of the chat balance URL.
func TestLLMConfigEmbedBalanceURLRoundTrip(t *testing.T) {
	te := setupWithServerOpts(t, nil, withLLMConfig(func(c *config.LLMConfig) {
		c.Enabled = true
		c.BaseURL = "https://chat.example/v1"
		c.APIKey = "chat-secret"
		c.Model = "chat-model"
	}))

	w := postJSON(te, "/api/v1/config/llm",
		`{"embed":{"balance_url":"https://embed.example/embed/balance"}}`)
	assertStatus(t, w, http.StatusOK)

	resp := decode[map[string]any](t, te.get(t, "/api/v1/config/llm"))
	embed := resp["embed"].(map[string]any)
	assert.Equal(t, "https://embed.example/embed/balance", embed["balance_url"])
}

func TestLLMConfigAndTestRejectReadOnlyMode(t *testing.T) {
	te := setupPGMode(t)

	for _, tc := range []struct {
		method string
		path   string
		body   string
	}{
		{method: http.MethodGet, path: "/api/v1/config/llm"},
		{method: http.MethodPost, path: "/api/v1/config/llm", body: `{}`},
		{method: http.MethodGet, path: "/api/v1/config/llm/providers"},
		{method: http.MethodPatch, path: "/api/v1/config/llm/providers", body: `{}`},
		{method: http.MethodGet, path: "/api/v1/config/consolidate"},
		{method: http.MethodPatch, path: "/api/v1/config/consolidate", body: `{}`},
		{method: http.MethodPost, path: "/api/v1/llm/test", body: `{}`},
	} {
		w := requestJSON(te, tc.method, tc.path, tc.body)
		assert.Equal(t, http.StatusForbidden, w.Code, "%s %s body: %s", tc.method, tc.path, w.Body.String())
	}
}

func TestLLMConfigPostUpdatesHotAndPreservesMaskedKeys(t *testing.T) {
	var gotAuth string
	client := llmTestClient(func(req *http.Request) (*http.Response, error) {
		gotAuth = req.Header.Get("Authorization")
		switch req.URL.Path {
		case "/v1/chat/completions":
			return jsonResponse(http.StatusOK, `{"choices":[{"message":{"content":"ok"}}]}`), nil
		case "/v1/embeddings":
			return jsonResponse(http.StatusOK, `{"data":[{"embedding":[0.1]}]}`), nil
		default:
			return jsonResponse(http.StatusNotFound, `{}`), nil
		}
	})
	te := setupWithServerOpts(t, []server.Option{server.WithLLMHTTPClient(client)}, withLLMConfig(func(c *config.LLMConfig) {
		c.Enabled = false
		c.BaseURL = "https://old-chat.example/v1"
		c.APIKey = "old-chat-secret"
		c.Model = "old-chat-model"
		c.Embed = config.LLMEmbedConfig{
			BaseURL: "https://old-embed.example/v1",
			APIKey:  "old-embed-secret",
			Model:   "old-embed-model",
		}
	}))

	body := `{
		"enabled": true,
		"base_url": "https://new-chat.example/v1",
		"api_key": "********",
		"model": "new-chat-model",
		"reasoning_effort": "medium",
		"min_user_messages": 7,
		"reenrich_msg_delta": 8,
		"reenrich_idle_minutes": 9,
		"concurrency": 3,
		"periodic": true,
		"embed": {
			"base_url": "https://new-embed.example/v1",
			"api_key": "",
			"model": "new-embed-model"
		}
	}`
	w := postJSON(te, "/api/v1/config/llm", body)
	assertStatus(t, w, http.StatusOK)
	assert.NotContains(t, w.Body.String(), "old-chat-secret")
	assert.NotContains(t, w.Body.String(), "old-embed-secret")

	data, err := os.ReadFile(filepath.Join(te.dataDir, "config.toml"))
	require.NoError(t, err)
	assert.Contains(t, string(data), "old-chat-secret")
	assert.Contains(t, string(data), "old-embed-secret")
	assert.Contains(t, string(data), "new-chat-model")

	w = postJSON(te, "/api/v1/llm/test", `{"embed":{"model":""}}`)
	assertStatus(t, w, http.StatusOK)
	assert.Equal(t, "Bearer old-chat-secret", gotAuth)

	w = postJSON(te, "/api/v1/config/llm", `{
		"api_key": "new-chat-secret",
		"embed": {"api_key": "new-embed-secret"}
	}`)
	assertStatus(t, w, http.StatusOK)
	data, err = os.ReadFile(filepath.Join(te.dataDir, "config.toml"))
	require.NoError(t, err)
	assert.Contains(t, string(data), "new-chat-secret")
	assert.Contains(t, string(data), "new-embed-secret")
	assert.NotContains(t, w.Body.String(), "new-chat-secret")
	assert.NotContains(t, w.Body.String(), "new-embed-secret")
}

func TestLLMConfigProvidersPatchUpdatesHotAndPreservesMaskedKeys(t *testing.T) {
	te := setup(t, withLLMConfig(func(c *config.LLMConfig) {
		c.Providers = map[string]config.LLMConfig{
			"deepseek-chat": {
				BaseURL: "https://old.example/v1",
				APIKey:  "old-provider-secret",
				Model:   "old-model",
			},
		}
		c.Usage = map[string]string{"enrich": "deepseek-chat"}
	}))

	body := `{
		"providers": {
			"deepseek-chat": {
				"base_url": "https://new.example/v1",
				"api_key": "********",
				"model": "new-model",
				"reasoning_effort": "medium"
			},
			"openrouter-embed": {
				"base_url": "https://embed.example/v1",
				"api_key": "embed-provider-secret",
				"model": "text-embedding-3-large"
			}
		},
		"usage": {
			"enrich": "deepseek-chat",
			"embed": "openrouter-embed"
		}
	}`
	w := requestJSON(te, http.MethodPatch, "/api/v1/config/llm/providers", body)
	assertStatus(t, w, http.StatusOK)
	assert.NotContains(t, w.Body.String(), "old-provider-secret")
	assert.NotContains(t, w.Body.String(), "embed-provider-secret")

	data, err := os.ReadFile(filepath.Join(te.dataDir, "config.toml"))
	require.NoError(t, err)
	assert.Contains(t, string(data), "old-provider-secret")
	assert.Contains(t, string(data), "embed-provider-secret")
	assert.Contains(t, string(data), "openrouter-embed")

	resp := decode[map[string]any](t, te.get(t, "/api/v1/config/llm"))
	providers := resp["providers"].(map[string]any)
	deepseek := providers["deepseek-chat"].(map[string]any)
	assert.Equal(t, "https://new.example/v1", deepseek["base_url"])
	assert.Equal(t, "new-model", deepseek["model"])
	assert.Equal(t, true, deepseek["has_api_key"])
	usage := resp["usage"].(map[string]any)
	assert.Equal(t, "deepseek-chat", usage["enrich"])
	assert.Equal(t, "openrouter-embed", usage["embed"])
}

func TestLLMProvidersGetReturnsUsageWarnings(t *testing.T) {
	te := setup(t, withLLMConfig(func(c *config.LLMConfig) {
		c.Providers = map[string]config.LLMConfig{
			"deepseek-chat": {BaseURL: "https://deepseek.example/v1", Model: "deepseek-chat"},
		}
		c.Usage = map[string]string{"consolidate": "missing-provider"}
	}))

	w := te.get(t, "/api/v1/config/llm/providers")
	assertStatus(t, w, http.StatusOK)
	resp := decode[map[string]any](t, w)
	warnings := resp["usage_warnings"].([]any)
	require.Len(t, warnings, 1)
	assert.Contains(t, warnings[0].(string), `usage "consolidate"`)
}

func TestLLMProvidersPatchDeletesProvidersAndClearsUsage(t *testing.T) {
	te := setup(t, withLLMConfig(func(c *config.LLMConfig) {
		c.Providers = map[string]config.LLMConfig{
			"deepseek-chat": {BaseURL: "https://deepseek.example/v1", Model: "deepseek-chat"},
			"old-chat":      {BaseURL: "https://old.example/v1", Model: "old-model"},
		}
		c.Usage = map[string]string{
			"consolidate": "old-chat",
			"enrich":      "deepseek-chat",
		}
	}))

	w := requestJSON(te, http.MethodPatch, "/api/v1/config/llm/providers", `{
		"delete_providers": ["old-chat"]
	}`)
	assertStatus(t, w, http.StatusOK)
	resp := decode[map[string]any](t, w)
	providers := resp["providers"].(map[string]any)
	assert.NotContains(t, providers, "old-chat")
	usage := resp["usage"].(map[string]any)
	assert.Equal(t, "deepseek-chat", usage["enrich"])
	assert.NotContains(t, usage, "consolidate")
}

// TestLLMProvidersPatchRoundTripsUsageModel locks the new model: a provider
// holds only the connection, and the per-usage model is sent via usage_model
// and reflected back on GET.
func TestLLMProvidersPatchRoundTripsUsageModel(t *testing.T) {
	te := setup(t, withLLMConfig(func(c *config.LLMConfig) {
		c.Enabled = true
		c.BaseURL = "https://legacy/v1"
		c.APIKey = "legacy-key"
		c.Model = "legacy-model"
	}))

	w := requestJSON(te, http.MethodPatch, "/api/v1/config/llm/providers", `{
		"providers": {"deepseek-1": {"enabled": true, "base_url": "https://api.deepseek.com", "api_key": "ds-key"}},
		"usage": {"extract": "deepseek-1"},
		"usage_model": {"extract": "deepseek-reasoner"}
	}`)
	assertStatus(t, w, http.StatusOK)
	resp := decode[map[string]any](t, w)
	assert.Equal(t, "deepseek-1", resp["usage"].(map[string]any)["extract"])
	assert.Equal(t, "deepseek-reasoner", resp["usage_model"].(map[string]any)["extract"])

	// GET reflects it back.
	g := requestJSON(te, http.MethodGet, "/api/v1/config/llm/providers", "")
	assertStatus(t, g, http.StatusOK)
	got := decode[map[string]any](t, g)
	assert.Equal(t, "deepseek-reasoner", got["usage_model"].(map[string]any)["extract"])
}

// TestLLMProvidersPatchPrunesUsageModelWhenUnbound locks that clearing a usage
// binding also drops its dangling model.
func TestLLMProvidersPatchPrunesUsageModelWhenUnbound(t *testing.T) {
	te := setup(t, withLLMConfig(func(c *config.LLMConfig) {
		c.Providers = map[string]config.LLMConfig{"deepseek-1": {BaseURL: "https://api.deepseek.com", APIKey: "ds-key"}}
		c.Usage = map[string]string{"extract": "deepseek-1"}
		c.UsageModel = map[string]string{"extract": "deepseek-reasoner"}
	}))

	// Unbind extract (provider ""): its model must be pruned.
	w := requestJSON(te, http.MethodPatch, "/api/v1/config/llm/providers", `{"usage": {"extract": ""}}`)
	assertStatus(t, w, http.StatusOK)
	resp := decode[map[string]any](t, w)
	if um, ok := resp["usage_model"].(map[string]any); ok {
		assert.NotContains(t, um, "extract")
	}
}

func TestConsolidateConfigGetAndPatchRoundTrip(t *testing.T) {
	te := setup(t)

	getResp := decode[map[string]any](t, te.get(t, "/api/v1/config/consolidate"))
	assert.Equal(t, false, getResp["enabled"])
	assert.Equal(t, "24h0m0s", getResp["interval"])

	w := requestJSON(te, http.MethodPatch, "/api/v1/config/consolidate", `{"interval":"90m"}`)
	assertStatus(t, w, http.StatusOK)
	resp := decode[map[string]any](t, w)
	assert.Equal(t, "1h30m0s", resp["interval"])

	data, err := os.ReadFile(filepath.Join(te.dataDir, "config.toml"))
	require.NoError(t, err)
	assert.Contains(t, string(data), `consolidate_interval = "1h30m0s"`)

	got := decode[map[string]any](t, te.get(t, "/api/v1/config/consolidate"))
	assert.Equal(t, "1h30m0s", got["interval"])
}

func TestConsolidateConfigPatchRejectsInvalidInterval(t *testing.T) {
	te := setup(t)

	for _, body := range []string{`{"interval":"soon"}`, `{"interval":"0s"}`} {
		w := requestJSON(te, http.MethodPatch, "/api/v1/config/consolidate", body)
		assertStatus(t, w, http.StatusBadRequest)
		assert.Contains(t, w.Body.String(), "invalid consolidate interval")
	}
}

func TestConsolidateAuditRemainsReadOnlyAndEnableRouteReturnsGone(t *testing.T) {
	memoryDir := t.TempDir()
	audit := consolidate.NewAuditLog(consolidate.AuditPath(memoryDir))
	rec := consolidate.RunRecord{
		StartedAt:      "2026-06-26T00:00:00Z",
		CandidateCount: 1,
		Decisions: []consolidate.DecisionRecord{{
			CandidateID: "candidate-1",
			Action:      "ADD",
			Result:      "write note candidate-1",
		}},
		Committed: true,
	}
	require.NoError(t, audit.Append(rec))
	te := setup(t, withMemoryDir(memoryDir))

	enable := requestJSON(te, http.MethodPut, "/api/v1/consolidate/enable", `{"enabled":true}`)
	assertStatus(t, enable, http.StatusGone)

	auditResp := te.get(t, "/api/v1/consolidate/audit?limit=1")
	assertStatus(t, auditResp, http.StatusOK)
	body := decode[map[string]any](t, auditResp)
	assert.Equal(t, false, body["enabled"])
	assert.Equal(t, false, body["available"])
	records := body["records"].([]any)
	require.Len(t, records, 1)
	first := records[0].(map[string]any)
	assert.Equal(t, float64(1), first["candidate_count"])
}

func TestLLMConfigProvidersGetUsesDedicatedRegistryEndpoint(t *testing.T) {
	te := setup(t, withLLMConfig(func(c *config.LLMConfig) {
		c.Providers = map[string]config.LLMConfig{
			"deepseek-chat": {
				BaseURL: "https://deepseek.example/v1",
				APIKey:  "provider-secret-9999",
				Model:   "deepseek-chat",
			},
		}
		c.Usage = map[string]string{"consolidate": "deepseek-chat"}
	}))

	w := te.get(t, "/api/v1/config/llm/providers")
	assertStatus(t, w, http.StatusOK)
	assert.NotContains(t, w.Body.String(), "provider-secret-9999")
	resp := decode[map[string]any](t, w)
	providers := resp["providers"].(map[string]any)
	deepseek := providers["deepseek-chat"].(map[string]any)
	assert.Equal(t, "https://deepseek.example/v1", deepseek["base_url"])
	assert.Equal(t, true, deepseek["has_api_key"])
	usage := resp["usage"].(map[string]any)
	assert.Equal(t, "deepseek-chat", usage["consolidate"])
}

func TestLLMConnectionTestReportsChatAndEmbed(t *testing.T) {
	var paths []string
	var chatBody map[string]any
	client := llmTestClient(func(req *http.Request) (*http.Response, error) {
		paths = append(paths, req.URL.Path)
		switch req.URL.Path {
		case "/v1/chat/completions":
			body, err := io.ReadAll(req.Body)
			require.NoError(t, err)
			require.NoError(t, json.Unmarshal(body, &chatBody))
			return jsonResponse(http.StatusOK, `{"choices":[{"message":{"content":"ok"}}]}`), nil
		case "/v1/embeddings":
			return jsonResponse(http.StatusOK, `{"data":[{"embedding":[0.1,0.2]}]}`), nil
		default:
			return jsonResponse(http.StatusNotFound, `{}`), nil
		}
	})
	te := setupWithServerOpts(t, []server.Option{server.WithLLMHTTPClient(client)}, withLLMConfig(func(c *config.LLMConfig) {
		c.Enabled = true
		c.BaseURL = "http://llm.test/v1"
		c.APIKey = "secret-key"
		c.Model = "chat-model"
		c.Embed.Model = "embed-model"
	}))

	w := postJSON(te, "/api/v1/llm/test", `{}`)
	assertStatus(t, w, http.StatusOK)
	assert.NotContains(t, w.Body.String(), "secret-key")
	resp := decode[map[string]any](t, w)
	chat := resp["chat"].(map[string]any)
	embed := resp["embed"].(map[string]any)
	assert.Equal(t, true, chat["ok"])
	assert.Equal(t, "ok", chat["message"])
	assert.Equal(t, true, embed["ok"])
	assert.Equal(t, "ok", embed["message"])
	assert.Equal(t, []string{"/v1/chat/completions", "/v1/embeddings"}, paths)
	assert.Equal(t, float64(4), chatBody["max_tokens"])
	messages := chatBody["messages"].([]any)
	require.Len(t, messages, 2)
	assert.Contains(t, strings.ToLower(messages[0].(map[string]any)["content"].(string)), "json")
	assert.Contains(t, strings.ToLower(messages[1].(map[string]any)["content"].(string)), "json")
}

func TestLLMConnectionTestReportsEmbedDisabledAndSanitizesErrors(t *testing.T) {
	client := llmTestClient(func(req *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusUnauthorized, `{"error":"bad secret-key"}`), nil
	})
	te := setupWithServerOpts(t, []server.Option{server.WithLLMHTTPClient(client)}, withLLMConfig(func(c *config.LLMConfig) {
		c.Enabled = true
		c.BaseURL = "http://llm.test/v1"
		c.APIKey = "secret-key"
		c.Model = "chat-model"
	}))

	w := postJSON(te, "/api/v1/llm/test", `{}`)
	assertStatus(t, w, http.StatusOK)
	assert.NotContains(t, w.Body.String(), "secret-key")
	resp := decode[map[string]any](t, w)
	chat := resp["chat"].(map[string]any)
	embed := resp["embed"].(map[string]any)
	assert.Equal(t, false, chat["ok"])
	assert.Contains(t, chat["message"], "[redacted]")
	assert.Equal(t, false, embed["ok"])
	assert.Equal(t, true, embed["disabled"])
	assert.Equal(t, "disabled", embed["message"])
}

func TestLLMConnectionTestAcceptsCandidateConfig(t *testing.T) {
	var gotAuth string
	client := llmTestClient(func(req *http.Request) (*http.Response, error) {
		gotAuth = req.Header.Get("Authorization")
		return jsonResponse(http.StatusOK, `{"choices":[{"message":{"content":"ok"}}]}`), nil
	})
	te := setupWithServerOpts(t, []server.Option{server.WithLLMHTTPClient(client)}, withLLMConfig(func(c *config.LLMConfig) {
		c.Enabled = true
		c.BaseURL = "http://saved.test/v1"
		c.APIKey = "saved-secret"
		c.Model = "saved-model"
	}))

	body := map[string]any{
		"base_url": "http://candidate.test/v1",
		"api_key":  "candidate-secret",
		"model":    "candidate-model",
		"embed": map[string]any{
			"model": "",
		},
	}
	data, err := json.Marshal(body)
	require.NoError(t, err)
	w := postJSON(te, "/api/v1/llm/test", string(data))
	assertStatus(t, w, http.StatusOK)
	assert.Equal(t, "Bearer candidate-secret", gotAuth)
}

// TestLLMConnectionTestResolvesUsage locks that {"usage":"extract"} tests the
// usage's effective resolved config (the bound named provider), not enrich.
func TestLLMConnectionTestResolvesUsage(t *testing.T) {
	var gotAuth, gotModel string
	client := llmTestClient(func(req *http.Request) (*http.Response, error) {
		gotAuth = req.Header.Get("Authorization")
		body, _ := io.ReadAll(req.Body)
		var parsed map[string]any
		_ = json.Unmarshal(body, &parsed)
		gotModel, _ = parsed["model"].(string)
		return jsonResponse(http.StatusOK, `{"choices":[{"message":{"content":"ok"}}]}`), nil
	})
	te := setupWithServerOpts(t, []server.Option{server.WithLLMHTTPClient(client)}, withLLMConfig(func(c *config.LLMConfig) {
		c.Enabled = true
		c.BaseURL = "http://default.test/v1"
		c.APIKey = "default-secret"
		c.Model = "default-model"
		c.Providers = map[string]config.LLMConfig{
			"cheap": {Enabled: true, BaseURL: "http://cheap.test/v1", APIKey: "cheap-secret", Model: "cheap-model"},
		}
		c.Usage = map[string]string{"extract": "cheap"}
	}))

	w := postJSON(te, "/api/v1/llm/test", `{"usage":"extract","channel":"chat"}`)
	assertStatus(t, w, http.StatusOK)
	assert.NotContains(t, w.Body.String(), "cheap-secret")
	assert.Equal(t, "Bearer cheap-secret", gotAuth)
	assert.Equal(t, "cheap-model", gotModel)
	resp := decode[map[string]any](t, w)
	assert.Equal(t, true, resp["chat"].(map[string]any)["ok"])
	// channel "chat" skips embed.
	assert.Equal(t, true, resp["embed"].(map[string]any)["disabled"])
}

// TestLLMConnectionTestResolvesProviderSecret locks that {"provider":"cheap"}
// with a masked key tests that stored provider's real secret, not enrich's.
func TestLLMConnectionTestResolvesProviderSecret(t *testing.T) {
	var gotAuth string
	client := llmTestClient(func(req *http.Request) (*http.Response, error) {
		gotAuth = req.Header.Get("Authorization")
		return jsonResponse(http.StatusOK, `{"choices":[{"message":{"content":"ok"}}]}`), nil
	})
	te := setupWithServerOpts(t, []server.Option{server.WithLLMHTTPClient(client)}, withLLMConfig(func(c *config.LLMConfig) {
		c.Enabled = true
		c.BaseURL = "http://default.test/v1"
		c.APIKey = "default-secret"
		c.Model = "default-model"
		c.Providers = map[string]config.LLMConfig{
			"cheap": {Enabled: true, BaseURL: "http://cheap.test/v1", APIKey: "cheap-secret", Model: "cheap-model"},
		}
	}))

	// Masked key means "keep stored"; the stored provider secret must be used.
	w := postJSON(te, "/api/v1/llm/test", `{"provider":"cheap","channel":"chat","api_key":"********"}`)
	assertStatus(t, w, http.StatusOK)
	assert.Equal(t, "Bearer cheap-secret", gotAuth)
}

// TestLLMConnectionTestChannelEmbedOnly locks that {"channel":"embed"} pings
// only the embed transport and skips the chat ping entirely.
func TestLLMConnectionTestChannelEmbedOnly(t *testing.T) {
	var paths []string
	client := llmTestClient(func(req *http.Request) (*http.Response, error) {
		paths = append(paths, req.URL.Path)
		if req.URL.Path == "/v1/embeddings" {
			return jsonResponse(http.StatusOK, `{"data":[{"embedding":[0.1,0.2]}]}`), nil
		}
		return jsonResponse(http.StatusOK, `{"choices":[{"message":{"content":"ok"}}]}`), nil
	})
	te := setupWithServerOpts(t, []server.Option{server.WithLLMHTTPClient(client)}, withLLMConfig(func(c *config.LLMConfig) {
		c.Enabled = true
		c.BaseURL = "http://llm.test/v1"
		c.APIKey = "secret-key"
		c.Model = "chat-model"
		c.Embed.Model = "embed-model"
	}))

	w := postJSON(te, "/api/v1/llm/test", `{"channel":"embed"}`)
	assertStatus(t, w, http.StatusOK)
	resp := decode[map[string]any](t, w)
	assert.Equal(t, true, resp["embed"].(map[string]any)["ok"])
	assert.Equal(t, true, resp["chat"].(map[string]any)["disabled"])
	assert.Equal(t, []string{"/v1/embeddings"}, paths)
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
