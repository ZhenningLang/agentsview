package server_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.kenn.io/agentsview/internal/db"
)

func TestWorktreeMappingsAPIUsesCurrentMachine(t *testing.T) {
	te := setup(t)
	prefix := filepath.Join(t.TempDir(), "app.worktrees")

	created := postWorktreeMapping(t, te, map[string]any{
		"path_prefix": prefix,
		"project":     "canonical-app",
		"machine":     "other-machine",
	})
	require.Equal(t, "test", created.Machine)
	require.Equal(t, "canonical_app", created.Project)
	require.True(t, created.Enabled, "created mapping should default enabled")

	var list struct {
		Machine  string                      `json:"machine"`
		Mappings []db.WorktreeProjectMapping `json:"mappings"`
	}
	w := te.get(t, "/api/v1/settings/worktree-mappings")
	assertStatus(t, w, http.StatusOK)
	decodeInto(t, w, &list)
	require.Equal(t, "test", list.Machine)
	require.Len(t, list.Mappings, 1)

	updated := putWorktreeMapping(t, te, created.ID, map[string]any{
		"path_prefix": prefix,
		"project":     "disabled-app",
		"enabled":     false,
	})
	assert.False(t, updated.Enabled, "updated mapping should be disabled")
	assert.Equal(t, "disabled_app", updated.Project)

	req := httptest.NewRequest(
		http.MethodDelete,
		"/api/v1/settings/worktree-mappings/"+
			strconv.FormatInt(created.ID, 10),
		nil,
	)
	req.Header.Set("Origin", "http://127.0.0.1:0")
	delW := httptest.NewRecorder()
	te.handler.ServeHTTP(delW, req)
	assertStatus(t, delW, http.StatusNoContent)

	w = te.get(t, "/api/v1/settings/worktree-mappings")
	assertStatus(t, w, http.StatusOK)
	decodeInto(t, w, &list)
	assert.Empty(t, list.Mappings, "mappings after delete should be empty")
}

func TestWorktreeMappingsAPIHandlesRepoDotWorktreesLayout(t *testing.T) {
	te := setup(t)
	root := t.TempDir()
	layoutPrefix := filepath.Join(root, "service")

	created := postWorktreeMappingMap(t, te, map[string]any{
		"path_prefix": layoutPrefix,
		"layout":      "repo_dot_worktrees",
	})
	assert.Equal(t, "test", created["machine"])
	assert.Equal(t, "repo_dot_worktrees", created["layout"])
	assert.Equal(t, "", created["project"])
	assert.Equal(t, true, created["enabled"])

	updated := putWorktreeMappingMap(t, te, int64(created["id"].(float64)), map[string]any{
		"path_prefix": layoutPrefix,
		"layout":      "repo_dot_worktrees",
		"project":     "stale-form-value",
		"enabled":     false,
	})
	assert.Equal(t, "repo_dot_worktrees", updated["layout"])
	assert.Equal(t, "", updated["project"], "repo layout must not persist stale form project")
	assert.Equal(t, false, updated["enabled"])

	updated = putWorktreeMappingMap(t, te, int64(created["id"].(float64)), map[string]any{
		"path_prefix": layoutPrefix,
		"project":     "legacy-client-value",
		"enabled":     true,
	})
	assert.Equal(t, "repo_dot_worktrees", updated["layout"], "omitted PUT layout preserves existing layout")
	assert.Equal(t, "", updated["project"], "preserved repo layout ignores legacy project")
	assert.Equal(t, true, updated["enabled"])

	w := postRawWorktreeMapping(t, te, map[string]any{
		"path_prefix": filepath.Join(root, "invalid"),
		"layout":      "not_a_layout",
	})
	assertStatus(t, w, http.StatusBadRequest)

	w = postRawWorktreeMapping(t, te, map[string]any{
		"path_prefix": filepath.Join(root, "explicit"),
		"layout":      "explicit",
	})
	assertStatus(t, w, http.StatusBadRequest)

	w = postRawWorktreeMapping(t, te, map[string]any{
		"path_prefix": layoutPrefix,
		"layout":      "repo_dot_worktrees",
	})
	assertStatus(t, w, http.StatusConflict)
}

func TestWorktreeMappingsAPIApply(t *testing.T) {
	te := setup(t)
	prefix := filepath.Join(t.TempDir(), "app.worktrees")
	_ = postWorktreeMapping(t, te, map[string]any{
		"path_prefix": prefix,
		"project":     "canonical-app",
	})
	require.NoError(t, te.db.UpsertSession(db.Session{
		ID:      "s1",
		Machine: "test",
		Agent:   "claude",
		Project: "feature_login",
		Cwd:     filepath.Join(prefix, "feature-login"),
	}))

	w := te.post(t, "/api/v1/settings/worktree-mappings/apply", `{}`)
	assertStatus(t, w, http.StatusOK)
	var resp struct {
		Machine         string `json:"machine"`
		MatchedSessions int    `json:"matched_sessions"`
		UpdatedSessions int    `json:"updated_sessions"`
	}
	decodeInto(t, w, &resp)
	assert.Equal(t, "test", resp.Machine)
	assert.Equal(t, 1, resp.MatchedSessions)
	assert.Equal(t, 1, resp.UpdatedSessions)
	sess, err := te.db.GetSession(context.Background(), "s1")
	require.NoError(t, err)
	assert.Equal(t, "canonical_app", sess.Project)
}

func TestWorktreeMappingsAPIRejectsRemoteMode(t *testing.T) {
	te := setupPGMode(t)
	w := te.get(t, "/api/v1/settings/worktree-mappings")
	assertStatus(t, w, http.StatusNotImplemented)

	w = te.post(t, "/api/v1/settings/worktree-mappings", `{
		"path_prefix": "/tmp/app.worktrees",
		"project": "app"
	}`)
	assertStatus(t, w, http.StatusNotImplemented)

	w = te.post(t, "/api/v1/settings/worktree-mappings/apply", `{}`)
	assertStatus(t, w, http.StatusNotImplemented)
}

func TestWorktreeMappingsAPIMalformedIDIsNotFound(t *testing.T) {
	te := setup(t)
	req := httptest.NewRequest(
		http.MethodPut,
		"/api/v1/settings/worktree-mappings/apply",
		bytes.NewReader([]byte(`{}`)),
	)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://127.0.0.1:0")
	w := httptest.NewRecorder()
	te.handler.ServeHTTP(w, req)
	assertStatus(t, w, http.StatusNotFound)
}

func postWorktreeMapping(
	t *testing.T,
	te *testEnv,
	body map[string]any,
) db.WorktreeProjectMapping {
	t.Helper()
	data, err := json.Marshal(body)
	require.NoError(t, err)
	w := te.post(
		t,
		"/api/v1/settings/worktree-mappings",
		string(data),
	)
	assertStatus(t, w, http.StatusCreated)
	return decode[db.WorktreeProjectMapping](t, w)
}

func postWorktreeMappingMap(
	t *testing.T,
	te *testEnv,
	body map[string]any,
) map[string]any {
	t.Helper()
	w := postRawWorktreeMapping(t, te, body)
	assertStatus(t, w, http.StatusCreated)
	var got map[string]any
	decodeInto(t, w, &got)
	return got
}

func postRawWorktreeMapping(
	t *testing.T,
	te *testEnv,
	body map[string]any,
) *httptest.ResponseRecorder {
	t.Helper()
	data, err := json.Marshal(body)
	require.NoError(t, err)
	return te.post(t, "/api/v1/settings/worktree-mappings", string(data))
}

func putWorktreeMapping(
	t *testing.T,
	te *testEnv,
	id int64,
	body map[string]any,
) db.WorktreeProjectMapping {
	t.Helper()
	data, err := json.Marshal(body)
	require.NoError(t, err)
	req := httptest.NewRequest(
		http.MethodPut,
		"/api/v1/settings/worktree-mappings/"+
			strconv.FormatInt(id, 10),
		bytes.NewReader(data),
	)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://127.0.0.1:0")
	w := httptest.NewRecorder()
	te.handler.ServeHTTP(w, req)
	assertStatus(t, w, http.StatusOK)
	return decode[db.WorktreeProjectMapping](t, w)
}

func putWorktreeMappingMap(
	t *testing.T,
	te *testEnv,
	id int64,
	body map[string]any,
) map[string]any {
	t.Helper()
	data, err := json.Marshal(body)
	require.NoError(t, err)
	req := httptest.NewRequest(
		http.MethodPut,
		"/api/v1/settings/worktree-mappings/"+
			strconv.FormatInt(id, 10),
		bytes.NewReader(data),
	)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://127.0.0.1:0")
	w := httptest.NewRecorder()
	te.handler.ServeHTTP(w, req)
	assertStatus(t, w, http.StatusOK)
	var got map[string]any
	decodeInto(t, w, &got)
	return got
}

func decodeInto(
	t *testing.T,
	w *httptest.ResponseRecorder,
	target any,
) {
	t.Helper()
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), target),
		"decoding JSON; body: %s", w.Body.String())
}
