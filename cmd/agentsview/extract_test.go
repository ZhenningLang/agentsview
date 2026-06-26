package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.kenn.io/agentsview/internal/config"
	"go.kenn.io/agentsview/internal/db"
)

func TestStartExtractUsesUsageBinding(t *testing.T) {
	dotfiles := setupExtractDotfilesRoot(t)
	database := testCommandDB(t)
	cfg := config.Config{
		MemoryDir: filepath.Join(dotfiles, "memory", "user"),
		LLM: config.LLMConfig{
			Enabled: true,
			BaseURL: "https://legacy.example/v1",
			Model:   "legacy-model",
			Providers: map[string]config.LLMConfig{
				"extract-provider": {BaseURL: "http://llm.test/v1", Model: "extract-model"},
			},
			Usage: map[string]string{"extract": "extract-provider"},
		},
	}
	require.Equal(t, "http://llm.test/v1", cfg.ResolveUsageLLM("extract").BaseURL)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	ctl := startExtract(ctx, cfg, database)
	require.NotNil(t, ctl)
	assert.False(t, ctl.Enabled())
}

func setupExtractDotfilesRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "memory", "user"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(root, ".gitignore"), []byte("memory/.staging/raw_memories/\n"), 0o600))
	require.NoError(t, execCommand(t, root, "git", "init"))
	return root
}

func testCommandDB(t *testing.T) *db.DB {
	t.Helper()
	database, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	require.NoError(t, err)
	t.Cleanup(func() { database.Close() })
	return database
}

func execCommand(t *testing.T, dir, name string, args ...string) error {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	return cmd.Run()
}

func TestStartExtractUnavailableWithoutMemoryDir(t *testing.T) {
	ctl := startExtract(context.Background(), config.Config{}, nil)
	assert.Nil(t, ctl)
}
