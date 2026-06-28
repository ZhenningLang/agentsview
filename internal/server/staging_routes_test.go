package server_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeStagingCandidate(t *testing.T, dir, name string, body map[string]any) {
	t.Helper()
	data, err := json.Marshal(body)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), data, 0o600))
}

func TestStagingCandidatesEmptyWhenNoStagingDir(t *testing.T) {
	// A configured dotfiles root whose staging dir does not exist yet: the route
	// is fail-open — available (root known) but an empty pool, not an error.
	dotfiles := t.TempDir()
	memoryDir := filepath.Join(dotfiles, "memory", "user")
	require.NoError(t, os.MkdirAll(memoryDir, 0o700))
	te := setup(t, withDotfilesRoot(dotfiles), withMemoryDir(memoryDir))

	w := te.get(t, "/api/v1/staging/candidates")
	assertStatus(t, w, 200)
	body := decode[map[string]any](t, w)
	assert.Equal(t, true, body["available"])
	assert.Equal(t, float64(0), body["total"])
	assert.Empty(t, body["candidates"].([]any))
}

func TestStagingCandidatesSplitsByScope(t *testing.T) {
	dotfiles := t.TempDir()
	rawDir := filepath.Join(dotfiles, "memory", ".staging", "raw_memories")
	require.NoError(t, os.MkdirAll(rawDir, 0o700))
	memoryDir := filepath.Join(dotfiles, "memory", "user")
	require.NoError(t, os.MkdirAll(memoryDir, 0o700))

	writeStagingCandidate(t, rawDir, "a.json", map[string]any{
		"id": "a", "summary": "user pref", "category": "preference", "scope": "user", "created_at": "2026-06-28T01:00:00Z"})
	writeStagingCandidate(t, rawDir, "b.json", map[string]any{
		"id": "b", "summary": "oss-atlas decision", "category": "decision",
		"scope": "project", "origin_project": "oss-atlas", "created_at": "2026-06-28T02:00:00Z"})
	// Legacy candidate with no scope field defaults to user.
	writeStagingCandidate(t, rawDir, "c.json", map[string]any{
		"id": "c", "summary": "legacy", "category": "fact", "created_at": "2026-06-28T03:00:00Z"})

	te := setup(t, withDotfilesRoot(dotfiles), withMemoryDir(memoryDir))

	w := te.get(t, "/api/v1/staging/candidates")
	assertStatus(t, w, 200)
	body := decode[map[string]any](t, w)
	assert.Equal(t, true, body["available"])
	assert.Equal(t, float64(3), body["total"])
	byScope := body["by_scope"].(map[string]any)
	assert.Equal(t, float64(2), byScope["user"], "a + legacy c default to user")
	assert.Equal(t, float64(1), byScope["project"])
	projects := body["projects"].(map[string]any)
	assert.Equal(t, float64(1), projects["oss-atlas"])
	// Newest-first ordering.
	cands := body["candidates"].([]any)
	require.Len(t, cands, 3)
	assert.Equal(t, "c", cands[0].(map[string]any)["id"])

	// Scope filter returns only project-scoped candidates.
	w = te.get(t, "/api/v1/staging/candidates?scope=project")
	assertStatus(t, w, 200)
	body = decode[map[string]any](t, w)
	cands = body["candidates"].([]any)
	require.Len(t, cands, 1)
	assert.Equal(t, "oss-atlas", cands[0].(map[string]any)["origin_project"])
	// Counts still reflect the full pool, not the filtered view.
	assert.Equal(t, float64(3), body["total"])
}
