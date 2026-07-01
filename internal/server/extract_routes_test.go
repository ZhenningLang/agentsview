package server_test

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractAuditUnavailableReturnsEmptyRecords(t *testing.T) {
	memDir := filepath.Join(t.TempDir(), "memory", "user")
	require.NoError(t, os.MkdirAll(memDir, 0o700))
	te := setup(t, withMemoryDir(memDir))

	w := te.get(t, "/api/v1/extract/audit")
	assertStatus(t, w, http.StatusOK)
	resp := decode[map[string]any](t, w)
	assert.Equal(t, false, resp["enabled"])
	assert.Equal(t, false, resp["available"])
	assert.Empty(t, resp["records"].([]any))
}

func TestExtractEnableRouteReturnsGone(t *testing.T) {
	memDir := filepath.Join(t.TempDir(), "memory", "user")
	require.NoError(t, os.MkdirAll(memDir, 0o700))
	te := setup(t, withMemoryDir(memDir))

	w := requestJSON(te, http.MethodPut, "/api/v1/extract/enable", `{"enabled":true}`)
	assertStatus(t, w, http.StatusGone)
	_, err := os.Stat(filepath.Join(te.dataDir, "config.toml"))
	assert.True(t, os.IsNotExist(err), "removed extract toggle must not write config")
}
