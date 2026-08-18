package main

import (
	"bytes"
	"context"
	"go/ast"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.kenn.io/agentsview/internal/config"
	"go.kenn.io/agentsview/internal/service"
)

// holdBackgroundLaunchLock takes the launch lock the way a competing
// starter would, so a test can observe what the entrypoint under test
// does when it loses the race.
func holdBackgroundLaunchLock(t *testing.T, dataDir string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(dataDir, 0o700))
	lock, ok := acquireBackgroundLaunchLock(dataDir)
	require.True(t, ok, "test could not take the launch lock first")
	t.Cleanup(func() { _ = lock.Unlock() })
}

// assertNoBackgroundSpawn fails when a background child was launched.
// startServeBackgroundProcess writes its start banner to serve.log
// before handing the process to exec, so the log file existing is
// evidence that a spawn was attempted.
func assertNoBackgroundSpawn(t *testing.T, dataDir string) {
	t.Helper()
	_, err := os.Stat(filepath.Join(dataDir, "serve.log"))
	assert.True(t, os.IsNotExist(err),
		"a background child was spawned while another launch held the lock")
}

// helperChildArgs keeps a spawn that should not happen harmless: if the
// entrypoint under test bypasses the lock, the child is this test
// binary restricted to the quick-exit helper instead of a real server.
func helperChildArgs(t *testing.T, dataDir string) []string {
	t.Helper()
	t.Setenv("AGENTSVIEW_PHASE21_HELPER_MODE", "quick-exit")
	t.Setenv("AGENTSVIEW_PHASE21_HELPER_DATA_DIR", dataDir)
	return []string{"-test.run=^TestPhase21ServeBackgroundHelperProcess$", "--background"}
}

func TestPhase21CanonicalDaemonStartAcquiresLaunchLock(t *testing.T) {
	dataDir := runtimeTestDir(t)
	holdBackgroundLaunchLock(t, dataDir)
	args := helperChildArgs(t, dataDir)

	var res daemonStartResult
	var err error
	out := &bytes.Buffer{}
	stdout := captureStdout(t, func() {
		res, err = daemonCommands.start(config.Config{DataDir: dataDir}, args, out)
	})
	require.NoError(t, err)
	assert.Nil(t, res.Runtime)
	assert.Empty(t, stdout, "the start wrote to global stdout instead of its writer")
	assert.Contains(t, out.String(), "already in progress")
	assertNoBackgroundSpawn(t, dataDir)
}

func TestPhase21MCPLocalWakeUpAcquiresLaunchLock(t *testing.T) {
	dataDir := runtimeTestDir(t)
	holdBackgroundLaunchLock(t, dataDir)
	helperChildArgs(t, dataDir)

	svc, cleanup, err := newMCPLocalService(config.Config{DataDir: dataDir})
	require.NoError(t, err)
	t.Cleanup(cleanup)

	var listErr error
	var stderr string
	stdout := captureStdout(t, func() {
		stderr = captureStderr(t, func() {
			_, listErr = svc.List(context.Background(), service.ListFilter{})
		})
	})
	require.Error(t, listErr)
	assert.Contains(t, listErr.Error(), "did not publish a runtime record")
	assert.Empty(t, stdout, "the wake-up wrote to the MCP frame stream")
	assert.Contains(t, stderr, "already in progress")
	assertNoBackgroundSpawn(t, dataDir)
}

// TestPhase21BackgroundLaunchInventoryKeepsTheSpawnUnderTheLock is the
// regression guard for the bypass: the behavior tests above prove the
// two non-CLI entrypoints serialize today, and this one proves there is
// still exactly one spawn site and that it sits behind the lock, so a
// later caller cannot reach exec without taking it.
func TestPhase21BackgroundLaunchInventoryKeepsTheSpawnUnderTheLock(t *testing.T) {
	got := phase21Inventory(t, phase21BackgroundLaunchKind,
		func(kind, key string) string { return kind })

	want := map[string]string{
		"serve_background.go:startBackgroundServe:acquireBackgroundLaunchLock":       "acquireBackgroundLaunchLock",
		"serve_background.go:startBackgroundServeLocked:startServeBackgroundProcess": "startServeBackgroundProcess",
	}

	assert.Equal(t, want, got)
}

func phase21BackgroundLaunchKind(call *ast.CallExpr) string {
	ident, ok := call.Fun.(*ast.Ident)
	if !ok {
		return ""
	}
	switch ident.Name {
	case "acquireBackgroundLaunchLock", "startServeBackgroundProcess":
		return ident.Name
	default:
		return ""
	}
}
