package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.kenn.io/agentsview/internal/config"
	"go.kenn.io/kit/daemon"
)

func TestPhase21DaemonCommandRegistrationAndServeRestartBridge(t *testing.T) {
	root := newRootCommand()
	for _, path := range [][]string{
		{"daemon"},
		{"daemon", "start"},
		{"daemon", "status"},
		{"daemon", "stop"},
		{"daemon", "restart"},
		{"serve", "restart"},
	} {
		cmd, _, err := root.Find(path)
		require.NoError(t, err, strings.Join(path, " "))
		assert.NotNil(t, cmd, strings.Join(path, " "))
	}
}

func TestPhase21DaemonStatusUsesReadOnlyConfigAndDoesNotCreateDataDir(t *testing.T) {
	dataDir := filepath.Join(t.TempDir(), "missing-data")
	t.Setenv("AGENTSVIEW_DATA_DIR", dataDir)

	out, err := executeCommand(newRootCommand(), "daemon", "status")
	require.NoError(t, err)
	assert.Contains(t, out, "No writable agentsview daemon is running.")
	_, statErr := os.Stat(dataDir)
	assert.True(t, os.IsNotExist(statErr), "daemon status created %s", dataDir)
}

func TestPhase21DaemonStartIsConfigOnlyAndIdempotent(t *testing.T) {
	dataDir := runtimeTestDir(t)
	t.Setenv("AGENTSVIEW_DATA_DIR", dataDir)
	settings := withDaemonCommandTestSettings(t)
	settings.start = func(cfg config.Config, _ []string) (*DaemonRuntime, error) {
		settings.started = append(settings.started, cfg)
		host, port := testPingServer(t)
		_, err := WriteDaemonRuntime(dataDir, host, port, "phase21", false)
		require.NoError(t, err)
		return FindDaemonRuntime(dataDir), nil
	}

	out, err := executeCommand(newRootCommand(), "daemon", "start", "--port", "9191")
	require.NoError(t, err)
	assert.Contains(t, out, "agentsview daemon running at")
	require.Len(t, settings.started, 1)
	assert.Equal(t, 8080, settings.started[0].Port, "daemon start must ignore one-off serve flags")

	out, err = executeCommand(newRootCommand(), "daemon", "start", "--port", "9292")
	require.NoError(t, err)
	assert.Contains(t, out, "agentsview daemon already running")
	assert.Len(t, settings.started, 1, "idempotent start spawned again")
}

func TestPhase21DaemonStartFailsClosedOnDataDirMismatchAfterLock(t *testing.T) {
	lockedDir := filepath.Join(t.TempDir(), "locked")
	configuredDir := filepath.Join(t.TempDir(), "configured")
	require.NoError(t, os.MkdirAll(lockedDir, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(lockedDir, "config.toml"), []byte(
		fmt.Sprintf("data_dir = %q\n", configuredDir),
	), 0o600))
	t.Setenv("AGENTSVIEW_DATA_DIR", lockedDir)
	settings := withDaemonCommandTestSettings(t)
	settings.start = func(config.Config, []string) (*DaemonRuntime, error) {
		t.Fatal("daemon start should not run after data-dir mismatch")
		return nil, nil
	}

	_, err := executeCommand(newRootCommand(), "daemon", "start")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "config data_dir changed after locking")
}

func TestPhase21DaemonStopOnlyStopsWritableRuntime(t *testing.T) {
	writableDir := runtimeTestDir(t)
	readOnlyDir := runtimeTestDir(t)
	settings := withDaemonCommandTestSettings(t)
	settings.stop = func(rec daemon.RuntimeRecord) error {
		settings.stopped = append(settings.stopped, rec)
		removeRuntimeRecordFile(rec)
		return nil
	}
	host, port := testPingServer(t)
	_, err := WriteDaemonRuntime(writableDir, host, port, "phase21", false)
	require.NoError(t, err)
	_, err = WriteDaemonRuntime(readOnlyDir, host, port, "phase21", true)
	require.NoError(t, err)

	out := captureStdout(t, func() {
		err = runDaemonStop(config.Config{DataDir: writableDir})
	})
	require.NoError(t, err)
	assert.Contains(t, out, "Stopped writable agentsview daemon")
	require.Len(t, settings.stopped, 1)
	assert.Nil(t, FindDaemonRuntime(writableDir))
	assert.NotNil(t, FindDaemonRuntime(readOnlyDir), "read-only runtime was stopped")
}

func TestPhase21DaemonStopFailsClosedOnMultipleWritableRuntimes(t *testing.T) {
	dataDir := runtimeTestDir(t)
	settings := withDaemonCommandTestSettings(t)
	settings.stop = func(rec daemon.RuntimeRecord) error {
		settings.stopped = append(settings.stopped, rec)
		return nil
	}
	first := phase21StartHelperProcess(t, "runtime-caddy-child")
	second := phase21StartHelperProcess(t, "runtime-caddy-child")
	recs := []daemon.RuntimeRecord{
		phase21DaemonTestRecordForPID(t, dataDir, first.Process.Pid, 49101, false),
		phase21DaemonTestRecordForPID(t, dataDir, second.Process.Pid, 49102, false),
	}
	for _, rec := range recs {
		_, err := writeRuntimeRecordForTest(dataDir, rec)
		require.NoError(t, err)
	}

	err := runDaemonStop(config.Config{DataDir: dataDir})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "multiple writable agentsview daemons")
	assert.Empty(t, settings.stopped)
}

func TestPhase21DaemonRestartPreflightsBeforeStop(t *testing.T) {
	dataDir := runtimeTestDir(t)
	t.Setenv("AGENTSVIEW_DATA_DIR", dataDir)
	settings := withDaemonCommandTestSettings(t)
	settings.checkDataVersion = func(string) error {
		return fmt.Errorf("future data version")
	}
	settings.stop = func(rec daemon.RuntimeRecord) error {
		settings.stopped = append(settings.stopped, rec)
		return nil
	}
	host, port := testPingServer(t)
	_, err := WriteDaemonRuntime(dataDir, host, port, "phase21", false)
	require.NoError(t, err)

	_, err = executeCommand(newRootCommand(), "daemon", "restart")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "future data version")
	assert.Empty(t, settings.stopped, "restart stopped before preflight completed")
}

type daemonCommandTestSettings struct {
	started          []config.Config
	stopped          []daemon.RuntimeRecord
	start            func(config.Config, []string) (*DaemonRuntime, error)
	stop             func(daemon.RuntimeRecord) error
	checkDataVersion func(string) error
}

func withDaemonCommandTestSettings(t *testing.T) *daemonCommandTestSettings {
	t.Helper()
	settings := &daemonCommandTestSettings{}
	old := daemonCommands
	daemonCommands.start = func(cfg config.Config, args []string) (*DaemonRuntime, error) {
		if settings.start != nil {
			return settings.start(cfg, args)
		}
		return nil, nil
	}
	daemonCommands.stop = func(rec daemon.RuntimeRecord) error {
		if settings.stop != nil {
			return settings.stop(rec)
		}
		settings.stopped = append(settings.stopped, rec)
		return nil
	}
	daemonCommands.checkDataVersion = func(path string) error {
		if settings.checkDataVersion != nil {
			return settings.checkDataVersion(path)
		}
		return nil
	}
	t.Cleanup(func() { daemonCommands = old })
	return settings
}

func phase21DaemonTestRecord(t *testing.T, dataDir string, port int, readOnly bool) daemon.RuntimeRecord {
	t.Helper()
	return phase21DaemonTestRecordForPID(t, dataDir, os.Getpid(), port, readOnly)
}

func phase21DaemonTestRecordForPID(t *testing.T, dataDir string, pid int, port int, readOnly bool) daemon.RuntimeRecord {
	t.Helper()
	ct, ok := processCreateTimeMillis(pid)
	require.True(t, ok)
	return daemon.RuntimeRecord{
		PID:     pid,
		Network: daemon.NetworkTCP,
		Address: fmt.Sprintf("127.0.0.1:%d", port),
		Service: daemonService,
		Metadata: map[string]string{
			runtimeAPIVersion:  runtimeAPIVersionValue,
			runtimeDataVersion: "40",
			runtimeCreateTime:  fmt.Sprintf("%d", ct),
			runtimeHost:        "127.0.0.1",
			runtimePort:        fmt.Sprintf("%d", port),
			runtimeReadOnly:    fmt.Sprintf("%t", readOnly),
		},
		StartedAt:  time.Now().UTC(),
		SourcePath: runtimePathForTest(dataDir, pid),
	}
}

var _ = context.Background
