package main

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.kenn.io/agentsview/internal/config"
	"go.kenn.io/kit/daemon"
)

// The boundary this file pins is the one the phase spec declares and the
// one a user is most likely to get wrong: `serve …` is the broad
// lifecycle over every runtime in a data dir, `daemon …` acts only on
// the writable one, and `serve restart` is a bridge to the canonical
// daemon restart rather than a second implementation.
//
// Registration is not the boundary. A previous version of this coverage
// asserted that `serve restart` was findable on the command tree, which
// stays green even when the command does nothing, and it kept the two
// runtimes in separate data dirs, where no selection has to happen.
// These tests execute the commands against one data dir holding both a
// writable and a read-only runtime.

func TestPhase21LifecycleBoundaryServeRestartRunsTheCanonicalRestart(t *testing.T) {
	dataDir := runtimeTestDir(t)
	t.Setenv("AGENTSVIEW_DATA_DIR", dataDir)
	settings := withDaemonCommandTestSettings(t)

	running := newFakeDaemonServer(t, "")
	_, err := WriteDaemonRuntime(dataDir, running.host, running.port, "phase21", false)
	require.NoError(t, err)

	stops := 0
	settings.stop = func(rec daemon.RuntimeRecord) error {
		stops++
		RemoveDaemonRuntime(dataDir)
		return nil
	}
	restarted := newFakeDaemonServer(t, "")
	settings.start = func(cfg config.Config, _ []string) (daemonStartResult, error) {
		_, writeErr := WriteDaemonRuntime(
			dataDir, restarted.host, restarted.port, "phase21", false,
		)
		require.NoError(t, writeErr)
		return daemonStartResult{
			Runtime: FindDaemonRuntime(dataDir, cfg.AuthToken),
			Cfg:     cfg,
		}, nil
	}

	out, err := executeCommand(newRootCommand(), "serve", "restart")
	require.NoError(t, err)
	assert.Equal(t, 1, stops, "serve restart did not stop the running daemon")
	assert.Len(t, settings.started, 0, "started is only recorded by the default stub")
	assert.Contains(t, out, "Stopped writable agentsview daemon")
	assert.Contains(t, out, "agentsview daemon running at")
}

func TestPhase21LifecycleBoundaryDaemonStopLeavesReadOnlyRuntimeInSameDataDir(t *testing.T) {
	dataDir := runtimeTestDir(t)
	writable, readOnly := phase21TwoRuntimesInOneDataDir(t, dataDir)
	settings := withDaemonCommandTestSettings(t)

	out := &bytes.Buffer{}
	require.NoError(t, runDaemonStop(out, config.Config{DataDir: dataDir}))

	require.Len(t, settings.stopped, 1, "daemon stop must act on exactly one runtime")
	assert.Equal(t, writable.PID, settings.stopped[0].PID,
		"daemon stop chose the read-only runtime")
	assert.NotEqual(t, readOnly.PID, settings.stopped[0].PID)
	assert.Contains(t, out.String(), "Stopped writable agentsview daemon")
}

func TestPhase21LifecycleBoundaryServeStopSeesBothRuntimesDaemonStopSeesOne(t *testing.T) {
	dataDir := runtimeTestDir(t)
	writable, readOnly := phase21TwoRuntimesInOneDataDir(t, dataDir)

	// The selection, without signalling anything: serve's stop works
	// from every live record in the dir, daemon's from the writable
	// subset of the same set.
	broad := map[int]bool{}
	for _, rec := range liveDaemonRecords(dataDir) {
		broad[rec.PID] = true
	}
	assert.True(t, broad[writable.PID], "serve stop cannot see the writable runtime")
	assert.True(t, broad[readOnly.PID], "serve stop cannot see the read-only runtime")

	narrow := map[int]bool{}
	for _, target := range listDaemonRuntimeTargets(dataDir, "") {
		if target.Runtime != nil && !target.Runtime.ReadOnly {
			narrow[target.Record.PID] = true
		}
	}
	assert.True(t, narrow[writable.PID])
	assert.False(t, narrow[readOnly.PID], "the daemon lifecycle reached a read-only runtime")
}

func TestPhase21LifecycleBoundaryStatusWidthsDifferOnOneDataDir(t *testing.T) {
	dataDir := runtimeTestDir(t)
	// Without this the `daemon status` command below loads the real
	// config and reports the user's own daemon.
	t.Setenv("AGENTSVIEW_DATA_DIR", dataDir)
	// Only a read-only runtime, the shape `pg serve` and `duckdb serve`
	// leave behind: `serve status` has to report it, `daemon status`
	// has to say there is no writable daemon rather than claim this one.
	child := phase21StartHelperProcess(t, "runtime-caddy-child")
	rec := phase21DaemonTestRecordForPID(t, dataDir, child.Process.Pid, 49301, true)
	_, err := writeRuntimeRecordForTest(dataDir, rec)
	require.NoError(t, err)

	serveOut := captureStdout(t, func() {
		runServeStatus(config.Config{DataDir: dataDir})
	})
	assert.Contains(t, serveOut, "agentsview")
	assert.NotContains(t, serveOut, "No agentsview server is running.",
		"serve status missed a live read-only runtime")

	daemonCmd := newRootCommand()
	daemonOut, err := executeCommand(daemonCmd, "daemon", "status")
	require.NoError(t, err)
	assert.Contains(t, daemonOut, "No writable agentsview daemon is running.")
}

// phase21TwoRuntimesInOneDataDir publishes a writable and a read-only
// runtime in the same data dir, each owned by a live helper process, so
// a lifecycle command has to select between them rather than between
// directories.
func phase21TwoRuntimesInOneDataDir(
	t *testing.T, dataDir string,
) (writable, readOnly daemon.RuntimeRecord) {
	t.Helper()
	writableChild := phase21StartHelperProcess(t, "runtime-caddy-child")
	readOnlyChild := phase21StartHelperProcess(t, "runtime-caddy-child")
	writable = phase21DaemonTestRecordForPID(
		t, dataDir, writableChild.Process.Pid, 49201, false,
	)
	readOnly = phase21DaemonTestRecordForPID(
		t, dataDir, readOnlyChild.Process.Pid, 49202, true,
	)
	for _, rec := range []daemon.RuntimeRecord{writable, readOnly} {
		_, err := writeRuntimeRecordForTest(dataDir, rec)
		require.NoError(t, err)
	}
	return writable, readOnly
}
