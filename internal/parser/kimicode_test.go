package parser

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeKimiCodeTestFile(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0o644)
}

func TestDiscoverKimiCodeSessions(t *testing.T) {
	root := t.TempDir()
	wsDir := filepath.Join(root, "wd_my-app_aabbccddeeff")
	sessDir := filepath.Join(
		wsDir, "session_11111111-2222-3333-4444-555555555555")
	mainWire := filepath.Join(sessDir, "agents", "main", "wire.jsonl")
	subWire := filepath.Join(sessDir, "agents", "agent-0", "wire.jsonl")
	otherMain := filepath.Join(
		wsDir, "session_aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee",
		"agents", "main", "wire.jsonl")

	require.NoError(t, writeKimiCodeTestFile(mainWire, "{}\n"))
	require.NoError(t, writeKimiCodeTestFile(subWire, "{}\n"))
	require.NoError(t, writeKimiCodeTestFile(otherMain, "{}\n"))
	// A session dir without any wire.jsonl is skipped.
	require.NoError(t, writeKimiCodeTestFile(
		filepath.Join(wsDir, "session_00000000-0000-0000-0000-000000000000",
			"logs", "kimi-code.log"), "log"))
	// Stray files at the wrong depth are skipped.
	require.NoError(t, writeKimiCodeTestFile(
		filepath.Join(wsDir, "state.json"), "{}"))
	require.NoError(t, writeKimiCodeTestFile(
		filepath.Join(root, "session_index.jsonl"), "{}"))

	files := DiscoverKimiCodeSessions(root)
	require.Len(t, files, 3)
	assert.Equal(t, subWire, files[0].Path)
	assert.Equal(t, mainWire, files[1].Path)
	assert.Equal(t, otherMain, files[2].Path)
	for _, f := range files {
		assert.Equal(t, "wd_my-app_aabbccddeeff", f.Project)
		assert.Equal(t, AgentKimiCode, f.Agent)
	}

	assert.Nil(t, DiscoverKimiCodeSessions(filepath.Join(root, "missing")))
	assert.Nil(t, DiscoverKimiCodeSessions(""))
}

func TestFindKimiCodeSourceFile(t *testing.T) {
	root := t.TempDir()
	wsDir := filepath.Join(root, "wd_my-app_aabbccddeeff")
	mainWire := filepath.Join(
		wsDir, "session_11111111-2222-3333-4444-555555555555",
		"agents", "main", "wire.jsonl")
	subWire := filepath.Join(
		wsDir, "session_11111111-2222-3333-4444-555555555555",
		"agents", "agent-0", "wire.jsonl")
	require.NoError(t, writeKimiCodeTestFile(mainWire, "{}\n"))
	require.NoError(t, writeKimiCodeTestFile(subWire, "{}\n"))

	assert.Equal(t, mainWire, FindKimiCodeSourceFile(
		root, "session_11111111-2222-3333-4444-555555555555"))
	assert.Equal(t, subWire, FindKimiCodeSourceFile(
		root, "session_11111111-2222-3333-4444-555555555555:agent-0"))
	assert.Empty(t, FindKimiCodeSourceFile(
		root, "session_99999999-9999-9999-9999-999999999999"))
	assert.Empty(t, FindKimiCodeSourceFile(root, "../bad"))
	assert.Empty(t, FindKimiCodeSourceFile(root, "session_x:../bad"))
	assert.Empty(t, FindKimiCodeSourceFile("", "session_11111111"))
}
