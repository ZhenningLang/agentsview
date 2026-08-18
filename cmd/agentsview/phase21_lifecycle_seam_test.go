package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.kenn.io/agentsview/internal/config"
	"go.kenn.io/agentsview/internal/service"
	"go.kenn.io/kit/daemon"
)

// fakeDaemonServer stands in for a running daemon: it answers the
// runtime ping used for discovery and the one read endpoint the HTTP
// backend calls, and it rejects anything without the bearer token when
// one is configured. Discovery and the backend therefore both have to
// present the right token for a test to pass.
type fakeDaemonServer struct {
	host  string
	port  int
	mu    sync.Mutex
	auths []string
}

func newFakeDaemonServer(t *testing.T, token string) *fakeDaemonServer {
	t.Helper()
	fake := &fakeDaemonServer{}
	ping := daemon.NewPingHandler(daemon.PingHandlerOptions{
		Service: daemonService,
		Version: "test",
	})
	ts := httptest.NewServer(http.HandlerFunc(func(
		w http.ResponseWriter, r *http.Request,
	) {
		fake.mu.Lock()
		fake.auths = append(fake.auths, r.Header.Get("Authorization"))
		fake.mu.Unlock()
		if token != "" && r.Header.Get("Authorization") != "Bearer "+token {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if r.URL.Path == "/api/ping" {
			ping.ServeHTTP(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(service.SessionList{})
	}))
	t.Cleanup(ts.Close)
	u, err := url.Parse(ts.URL)
	require.NoError(t, err)
	host, portText, err := net.SplitHostPort(u.Host)
	require.NoError(t, err)
	port, err := strconv.Atoi(portText)
	require.NoError(t, err)
	fake.host, fake.port = host, port
	return fake
}

func (f *fakeDaemonServer) sawBearer(token string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Contains(f.auths, "Bearer "+token)
}

// TestPhase21DaemonRestartSucceedsWhileTheDaemonHoldsTheWriteOwnerLock
// holds the real write-owner flock for the duration of the restart, the
// way a live daemon does. A restart that takes that lock before
// stopping the daemon can never reach the stop.
func TestPhase21DaemonRestartSucceedsWhileTheDaemonHoldsTheWriteOwnerLock(t *testing.T) {
	dataDir := runtimeTestDir(t)
	t.Setenv("AGENTSVIEW_DATA_DIR", dataDir)
	settings := withDaemonCommandTestSettings(t)

	owner, err := acquireWriteOwnerLock(context.Background(), dataDir)
	require.NoError(t, err)
	released := false
	t.Cleanup(func() {
		if !released {
			_ = owner.Close()
		}
	})
	running := newFakeDaemonServer(t, "")
	_, err = WriteDaemonRuntime(dataDir, running.host, running.port, "phase21", false)
	require.NoError(t, err)

	settings.stop = func(rec daemon.RuntimeRecord) error {
		// A real daemon releases the write-owner lock as it exits.
		require.NoError(t, owner.Close())
		released = true
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

	out, err := executeCommand(newRootCommand(), "daemon", "restart")
	require.NoError(t, err)
	assert.True(t, released, "restart never reached the stop")
	assert.Contains(t, out, "Stopped writable agentsview daemon")
	assert.Contains(t, out, "agentsview daemon running at")
	assert.Contains(t, out, strconv.Itoa(restarted.port))
}

// TestPhase21DaemonStartIsIdempotentWithRequireAuth covers the second
// start against a daemon that requires a token: the pre-lock probe has
// to present the configured token, or the running daemon answers 401,
// looks absent, and the start walks into the lock it holds.
func TestPhase21DaemonStartIsIdempotentWithRequireAuth(t *testing.T) {
	dataDir := runtimeTestDir(t)
	t.Setenv("AGENTSVIEW_DATA_DIR", dataDir)
	settings := withDaemonCommandTestSettings(t)
	writePhase21ConfigFile(t, dataDir, "require_auth = true\nauth_token = \"phase21-token\"\n")

	owner, err := acquireWriteOwnerLock(context.Background(), dataDir)
	require.NoError(t, err)
	t.Cleanup(func() { _ = owner.Close() })
	running := newFakeDaemonServer(t, "phase21-token")
	_, err = WriteDaemonRuntime(
		dataDir, running.host, running.port, "phase21", false,
		daemonRuntimeOptions{RequireAuth: true},
	)
	require.NoError(t, err)

	out, err := executeCommand(newRootCommand(), "daemon", "start")
	require.NoError(t, err)
	assert.Contains(t, out, "agentsview daemon already running at")
	assert.Empty(t, settings.started, "a second start spawned a daemon")
}

// TestPhase21MCPWakeUpKeepsStdoutFreeOfNonProtocolText guards the
// default MCP transport: stdout carries JSON-RPC frames, so a wake-up
// that reports progress there corrupts the session.
func TestPhase21MCPWakeUpKeepsStdoutFreeOfNonProtocolText(t *testing.T) {
	dataDir := runtimeTestDir(t)
	holdBackgroundLaunchLock(t, dataDir)
	helperChildArgs(t, dataDir)

	svc, cleanup, err := newMCPLocalService(config.Config{DataDir: dataDir})
	require.NoError(t, err)
	t.Cleanup(cleanup)

	var stderr string
	stdout := captureStdout(t, func() {
		stderr = captureStderr(t, func() {
			_, _ = svc.List(context.Background(), service.ListFilter{})
		})
	})
	assert.Empty(t, stdout, "wake-up wrote non-protocol text to the MCP stream")
	assert.Contains(t, stderr, "already in progress")
	assertNoBackgroundSpawn(t, dataDir)
}

// TestPhase21MCPWakeUpDiscoversTheDaemonItStartedWithRequireAuth walks
// the whole seam: an MCP server loaded from a read-only config with no
// token starts a daemon, that start mints the token, and discovery plus
// the HTTP backend both have to use the minted one.
func TestPhase21MCPWakeUpDiscoversTheDaemonItStartedWithRequireAuth(t *testing.T) {
	dataDir := runtimeTestDir(t)
	t.Setenv("AGENTSVIEW_DATA_DIR", dataDir)
	settings := withDaemonCommandTestSettings(t)

	var minted string
	var started *fakeDaemonServer
	settings.start = func(cfg config.Config, _ []string) (daemonStartResult, error) {
		// What startBackgroundServe does: mint and persist the token
		// before the child publishes its runtime record, and hand the
		// effective config back to the caller.
		require.NoError(t, cfg.EnsureAuthToken())
		minted = cfg.AuthToken
		require.NotEmpty(t, minted)
		started = newFakeDaemonServer(t, minted)
		_, err := WriteDaemonRuntime(
			dataDir, started.host, started.port, "phase21", false,
			daemonRuntimeOptions{RequireAuth: true},
		)
		require.NoError(t, err)
		return daemonStartResult{
			Runtime: FindDaemonRuntime(dataDir, minted),
			Cfg:     cfg,
		}, nil
	}

	svc, cleanup, err := newMCPLocalService(config.Config{
		DataDir:     dataDir,
		RequireAuth: true,
	})
	require.NoError(t, err)
	t.Cleanup(cleanup)

	list, err := svc.List(context.Background(), service.ListFilter{})
	require.NoError(t, err, "the MCP backend could not reach the daemon it started")
	assert.NotNil(t, list)
	require.NotNil(t, started)
	assert.True(t, started.sawBearer(minted),
		"the daemon never saw the token its own start minted")
}

func writePhase21ConfigFile(t *testing.T, dataDir, body string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(dataDir, 0o700))
	require.NoError(t, os.WriteFile(
		filepath.Join(dataDir, "config.toml"), []byte(body), 0o600,
	))
}

// TestPhase21BackgroundStartReturnsAnErrorInsteadOfExiting covers the
// other half of the stdio contract: a start that fails must come back
// as an error the MCP command can turn into a tool error. Calling
// fatal here would take the whole MCP process down with it, which is
// what round 1 observed when this path ran under a test.
func TestPhase21BackgroundStartReturnsAnErrorInsteadOfExiting(t *testing.T) {
	dataDir := runtimeTestDir(t)
	require.NoError(t, os.MkdirAll(dataDir, 0o700))
	args := helperChildArgs(t, dataDir)

	out := &bytes.Buffer{}
	var res daemonStartResult
	var err error
	stdout := captureStdout(t, func() {
		res, err = startBackgroundServe(config.Config{DataDir: dataDir}, args, out)
	})
	require.Error(t, err, "a child that exits before readiness must be an error")
	assert.Contains(t, err.Error(), "exited before becoming ready")
	assert.Nil(t, res.Runtime)
	assert.Empty(t, stdout, "the failing start wrote to the MCP stream")
}

// TestPhase21DaemonLifecycleCommandsSerializeOnTheirOwnLock covers the
// lock that replaced the write-owner lock in restart: lifecycle
// commands still exclude each other, they just no longer wait on the
// lock the daemon they are managing is holding.
func TestPhase21DaemonLifecycleCommandsSerializeOnTheirOwnLock(t *testing.T) {
	dataDir := runtimeTestDir(t)
	t.Setenv("AGENTSVIEW_DATA_DIR", dataDir)
	settings := withDaemonCommandTestSettings(t)
	settings.start = func(config.Config, []string) (daemonStartResult, error) {
		t.Fatal("a start ran while another lifecycle command held the lock")
		return daemonStartResult{}, nil
	}

	held, err := acquireDaemonLifecycleLock(dataDir)
	require.NoError(t, err)
	t.Cleanup(func() { _ = held.Unlock() })

	for _, sub := range []string{"start", "restart"} {
		_, err := executeCommand(newRootCommand(), "daemon", sub)
		require.Error(t, err, sub)
		assert.Contains(t, err.Error(), "another agentsview daemon lifecycle command is running", sub)
	}

	// The write-owner lock is free the whole time: a live daemon owns
	// it, and lifecycle contention must not be reported through it.
	owner, err := acquireWriteOwnerLock(context.Background(), dataDir)
	require.NoError(t, err, "the lifecycle lock must not be the write-owner lock")
	require.NoError(t, owner.Close())
}
