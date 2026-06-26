package server_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.kenn.io/agentsview/internal/config"
)

// A masked-key provider seeded from [llm] keeps the key via key_from "@llm".
func TestLLMProvidersKeyFromSeedsLegacyLLMKey(t *testing.T) {
	te := setup(t, withLLMConfig(func(c *config.LLMConfig) {
		c.Enabled = true
		c.BaseURL = "https://api.deepseek.com"
		c.APIKey = "real-key"
		c.Model = "deepseek-chat"
	}))

	w := requestJSON(te, http.MethodPatch, "/api/v1/config/llm/providers", `{
		"providers": {"deepseek-1": {"enabled": true, "base_url": "https://api.deepseek.com", "api_key": "********", "key_from": "@llm"}},
		"usage": {"enrich": "deepseek-1"},
		"usage_model": {"enrich": "deepseek-chat"}
	}`)
	assertStatus(t, w, http.StatusOK)
	resp := decode[map[string]any](t, w)
	prov := resp["providers"].(map[string]any)["deepseek-1"].(map[string]any)
	assert.Equal(t, true, prov["has_api_key"], "seeded provider must inherit [llm] key")
	// And the secret is never echoed back.
	assert.NotContains(t, w.Body.String(), "real-key")
}

// Renaming a registry provider with a masked key preserves the key via
// key_from = old name, while the old name is deleted.
func TestLLMProvidersKeyFromCarriesKeyOnRename(t *testing.T) {
	te := setup(t, withLLMConfig(func(c *config.LLMConfig) {
		c.Enabled = true
		c.BaseURL = "https://x/v1"
		c.APIKey = "base"
		c.Model = "m"
		c.Providers = map[string]config.LLMConfig{
			"deepseek-1": {Enabled: true, BaseURL: "https://api.deepseek.com", APIKey: "ds-secret"},
		}
		c.Usage = map[string]string{"enrich": "deepseek-1"}
		c.UsageModel = map[string]string{"enrich": "deepseek-chat"}
	}))

	w := requestJSON(te, http.MethodPatch, "/api/v1/config/llm/providers", `{
		"providers": {"myds": {"enabled": true, "base_url": "https://api.deepseek.com", "api_key": "********", "key_from": "deepseek-1"}},
		"usage": {"enrich": "myds"},
		"usage_model": {"enrich": "deepseek-chat"},
		"delete_providers": ["deepseek-1"]
	}`)
	assertStatus(t, w, http.StatusOK)
	resp := decode[map[string]any](t, w)
	providers := resp["providers"].(map[string]any)
	assert.NotContains(t, providers, "deepseek-1")
	myds := providers["myds"].(map[string]any)
	assert.Equal(t, true, myds["has_api_key"], "renamed provider must carry the key")
	assert.NotContains(t, w.Body.String(), "ds-secret")
}
