package server_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.kenn.io/agentsview/internal/backup"
	"go.kenn.io/agentsview/internal/server"
)

// putJSON issues a PUT with a JSON body from a localhost origin so the
// local-only enable gate is satisfied (no real git/gh ever runs).
func putJSON(t *testing.T, te *testEnv, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPut, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://127.0.0.1:0")
	w := httptest.NewRecorder()
	te.handler.ServeHTTP(w, req)
	return w
}

// TestBackupPushStatus_NeverRunIsEmpty proves the status endpoint fail-opens to
// the never-run zero state (no error) when the worker has never pushed.
func TestBackupPushStatus_NeverRunIsEmpty(t *testing.T) {
	te := setupWithServerOpts(t, nil)
	w := te.get(t, "/api/v1/config/memory-backup/push-status")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var resp struct {
		Enabled       bool   `json:"enabled"`
		Available     bool   `json:"available"`
		LastSuccessAt string `json:"last_success_at"`
		LastError     string `json:"last_error"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.False(t, resp.Available, "no controller wired => unavailable")
	assert.Empty(t, resp.LastSuccessAt)
	assert.Empty(t, resp.LastError)
}

// TestBackupPushStatus_ReflectsPersistedStatus proves a persisted status file
// (last success + last error) is surfaced for the UI's green/red indicator.
func TestBackupPushStatus_ReflectsPersistedStatus(t *testing.T) {
	te := setupWithServerOpts(t, nil)

	// Write a status file directly into the data dir the server reads from.
	st := backup.NewStatusStore(backup.StatusPath(te.dataDir))
	require.NoError(t, st.Write(backup.Status{
		Repo:          "alice/agent-memory",
		LastSuccessAt: "2026-06-25T00:00:00Z",
		LastError:     "git push exited 1",
		LastErrorAt:   "2026-06-25T01:00:00Z",
	}))

	w := te.get(t, "/api/v1/config/memory-backup/push-status")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var resp struct {
		Repo          string `json:"repo"`
		LastSuccessAt string `json:"last_success_at"`
		LastError     string `json:"last_error"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "alice/agent-memory", resp.Repo)
	assert.Equal(t, "2026-06-25T00:00:00Z", resp.LastSuccessAt)
	assert.Equal(t, "git push exited 1", resp.LastError)
}

// TestBackupPushEnable_RequiresController proves the enable endpoint reports the
// feature unavailable (not a silent no-op) when no worker is wired.
func TestBackupPushEnable_RequiresController(t *testing.T) {
	te := setupWithServerOpts(t, nil)
	w := putJSON(t, te, "/api/v1/config/memory-backup/enable", `{"enabled":true}`)
	assert.Equal(t, http.StatusNotImplemented, w.Code, w.Body.String())
}

// TestBackupPushEnable_TogglesController proves the enable endpoint flips the
// live controller and persists the choice. The controller is built with a nil
// worker so toggling never spawns git/gh — only the armed flag changes.
func TestBackupPushEnable_TogglesController(t *testing.T) {
	ctrl := backup.NewController(nil, false)
	te := setupWithServerOpts(t, []server.Option{server.WithBackupController(ctrl)})

	w := putJSON(t, te, "/api/v1/config/memory-backup/enable", `{"enabled":true}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var resp struct {
		Enabled   bool `json:"enabled"`
		Available bool `json:"available"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.True(t, resp.Enabled)
	assert.True(t, resp.Available)
	assert.True(t, ctrl.Enabled(), "controller should be armed after enable")

	// Status now reflects the live armed state.
	gw := te.get(t, "/api/v1/config/memory-backup/push-status")
	var status struct {
		Enabled   bool `json:"enabled"`
		Available bool `json:"available"`
	}
	require.NoError(t, json.Unmarshal(gw.Body.Bytes(), &status))
	assert.True(t, status.Enabled)
	assert.True(t, status.Available)

	// Disable again.
	dw := putJSON(t, te, "/api/v1/config/memory-backup/enable", `{"enabled":false}`)
	require.Equal(t, http.StatusOK, dw.Code, dw.Body.String())
	assert.False(t, ctrl.Enabled())
}
