package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// spec verify ⑤: memory/user is the same dir AGENTSVIEW_MEMORY_DIR resolves to,
// and the dotfiles root is reverse-derived from it (two levels up).
func TestResolveDotfilesRoot_DerivedFromMemoryDir(t *testing.T) {
	dotfiles := t.TempDir()
	memDir := filepath.Join(dotfiles, "memory", "user")
	mkdirAll(t, memDir)

	cfg := Config{MemoryDir: memDir}
	if got := cfg.ResolveMemoryDir(); got != memDir {
		t.Fatalf("ResolveMemoryDir = %q, want %q", got, memDir)
	}
	if got := cfg.ResolveDotfilesRoot(); got != dotfiles {
		t.Fatalf("ResolveDotfilesRoot = %q, want %q (two levels up from memory dir)", got, dotfiles)
	}
}

func TestResolveDotfilesRoot_ExplicitOverride(t *testing.T) {
	memDir := t.TempDir()
	cfg := Config{MemoryDir: memDir, DotfilesRoot: "/custom/root"}
	if got := cfg.ResolveDotfilesRoot(); got != "/custom/root" {
		t.Fatalf("ResolveDotfilesRoot = %q, want explicit override", got)
	}
}

func TestResolveDotfilesRoot_EmptyWhenNoMemoryDir(t *testing.T) {
	cfg := Config{} // no MemoryDir, no override
	// ResolveMemoryDir probes ~/.dotfiles/memory/user which may or may not
	// exist; only assert the derivation is consistent with that resolution.
	mem := cfg.ResolveMemoryDir()
	got := cfg.ResolveDotfilesRoot()
	if mem == "" && got != "" {
		t.Fatalf("ResolveDotfilesRoot = %q, want empty when no memory dir", got)
	}
}

func TestResolveConsolidateInterval_Default(t *testing.T) {
	cfg := Config{}
	if got := cfg.ResolveConsolidateInterval(); got != 24*time.Hour {
		t.Fatalf("default interval = %v, want 24h", got)
	}
	cfg.ConsolidateInterval = 90 * time.Minute
	if got := cfg.ResolveConsolidateInterval(); got != 90*time.Minute {
		t.Fatalf("configured interval = %v, want 90m", got)
	}
}

// spec verify ④: ConsolidateEnabled defaults OFF.
func TestConsolidateEnabled_DefaultsOff(t *testing.T) {
	cfg, err := Default()
	require.NoError(t, err)
	assert.False(t, cfg.ConsolidateEnabled)
}

func TestExtractEnabled_DefaultsOffAndInterval(t *testing.T) {
	cfg, err := Default()
	require.NoError(t, err)
	assert.False(t, cfg.ExtractEnabled)
	assert.Equal(t, 24*time.Hour, cfg.ResolveExtractInterval())

	cfg.ExtractInterval = 90 * time.Minute
	assert.Equal(t, 90*time.Minute, cfg.ResolveExtractInterval())
}

func TestExtractSettingsPersistAndEnvOverride(t *testing.T) {
	dir := setupTestEnv(t)
	writeConfig(t, dir, map[string]any{
		"extract_enabled":  true,
		"extract_interval": "2h",
	})

	cfg, err := LoadMinimal()
	require.NoError(t, err)
	assert.True(t, cfg.ExtractEnabled)
	assert.Equal(t, 2*time.Hour, cfg.ExtractInterval)

	t.Setenv("AGENTSVIEW_EXTRACT_ENABLED", "false")
	t.Setenv("AGENTSVIEW_EXTRACT_INTERVAL", "30m")
	cfg, err = LoadMinimal()
	require.NoError(t, err)
	assert.False(t, cfg.ExtractEnabled)
	assert.Equal(t, 30*time.Minute, cfg.ExtractInterval)
}

// The consolidate LLM config falls back to the main LLM connection fields when
// its own are unset, and overrides only the fields it sets.
func TestConsolidateLLM_FallbackAndOverride(t *testing.T) {
	cfg := Config{
		LLM: LLMConfig{
			BaseURL: "https://main.example/v1",
			APIKey:  "main-key",
			Model:   "main-model",
		},
		Consolidate: LLMConfig{
			Model: "consolidate-model",
		},
	}
	got := cfg.ConsolidateLLM()
	if got.BaseURL != "https://main.example/v1" {
		t.Errorf("BaseURL = %q, want fallback to main", got.BaseURL)
	}
	if got.APIKey != "main-key" {
		t.Errorf("APIKey = %q, want fallback to main", got.APIKey)
	}
	if got.Model != "consolidate-model" {
		t.Errorf("Model = %q, want override", got.Model)
	}
}

// Consolidation is a semantic-triage classifier, not a reasoning task. It must
// NOT inherit the base [llm] reasoning_effort: deepseek honors it (adds a
// thinking pass that ~tripled latency in practice) and pushed cycles past the
// LLM client timeout, failing the whole cycle. Reasoning stays off here.
func TestConsolidateLLM_DisablesInheritedReasoning(t *testing.T) {
	cfg := Config{
		LLM: LLMConfig{
			BaseURL:         "https://main.example/v1",
			APIKey:          "main-key",
			Model:           "deepseek-chat",
			ReasoningEffort: "medium",
		},
	}
	if got := cfg.ConsolidateLLM(); got.ReasoningEffort != "" {
		t.Errorf("ConsolidateLLM ReasoningEffort = %q, want disabled (\"\")", got.ReasoningEffort)
	}
}

func mkdirAll(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
}
