package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPhase21LoadReadOnlyDoesNotMigrateJSONOrPersistSecret(t *testing.T) {
	dir := setupTestEnv(t)
	jsonPath := filepath.Join(dir, "config.json")
	require.NoError(t, os.WriteFile(jsonPath, []byte(`{"host":"127.0.0.2"}`), 0o600))

	cfg, err := LoadReadOnly()
	require.NoError(t, err)

	assert.Equal(t, dir, cfg.DataDir)
	assert.Empty(t, cfg.CursorSecret)
	assert.NoFileExists(t, filepath.Join(dir, configFileName))
	assert.NoFileExists(t, filepath.Join(dir, "config.json.bak"))
	assert.FileExists(t, jsonPath)
}

func TestPhase21LoadReadOnlyReadsTOMLAndEnvWithoutWrites(t *testing.T) {
	dir := setupTestEnv(t)
	path := filepath.Join(dir, configFileName)
	content := []byte("cursor_secret = \"persisted\"\nauth_token = \"token\"\n")
	require.NoError(t, os.WriteFile(path, content, 0o600))

	cfg, err := LoadReadOnly()
	require.NoError(t, err)

	assert.Equal(t, dir, cfg.DataDir)
	assert.Equal(t, filepath.Join(dir, "sessions.db"), cfg.DBPath)
	assert.Equal(t, "persisted", cfg.CursorSecret)
	assert.Equal(t, "token", cfg.AuthToken)
	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, string(content), string(got))
}
