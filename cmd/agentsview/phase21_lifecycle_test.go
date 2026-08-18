package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gofrs/flock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.kenn.io/agentsview/internal/config"
	"go.kenn.io/kit/daemon"
)

func TestPhase21ServeBackgroundHelperProcess(t *testing.T) {
	mode := os.Getenv("AGENTSVIEW_PHASE21_HELPER_MODE")
	if mode == "" {
		return
	}
	dataDir := os.Getenv("AGENTSVIEW_PHASE21_HELPER_DATA_DIR")
	if dataDir == "" {
		os.Exit(2)
	}
	switch mode {
	case "background-runtime", "runtime-caddy-child":
		host, port := testPingServer(t)
		if mode == "background-runtime" {
			_, err := WriteDaemonRuntime(dataDir, host, port, "phase21-helper", false)
			if err != nil {
				os.Exit(3)
			}
		}
		_, _ = os.Stdout.Write([]byte("ready\n"))
		select {}
	case "quick-exit":
		_, _ = os.Stdout.Write([]byte("quick-exit\n"))
		return
	default:
		os.Exit(4)
	}
}

func TestPhase21ServeBackgroundStartsStatusStopsAndAppendsLog(t *testing.T) {
	dataDir := filepath.Join(t.TempDir(), "data")
	require.NoError(t, os.MkdirAll(dataDir, 0o700))
	logPath := filepath.Join(dataDir, "serve.log")
	require.NoError(t, os.WriteFile(logPath, []byte("existing log\n"), 0o600))
	t.Setenv("AGENTSVIEW_PHASE21_HELPER_MODE", "background-runtime")
	t.Setenv("AGENTSVIEW_PHASE21_HELPER_DATA_DIR", dataDir)

	child, gotLogPath, err := startServeBackgroundProcess(
		config.Config{DataDir: dataDir},
		[]string{"-test.run=^TestPhase21ServeBackgroundHelperProcess$", "--background"},
	)
	require.NoError(t, err)
	require.Equal(t, logPath, gotLogPath)
	t.Cleanup(func() { _ = child.Process.Kill(); _ = child.Wait() })

	require.Eventually(t, func() bool {
		return FindDaemonRuntime(dataDir, "") != nil
	}, 5*time.Second, 25*time.Millisecond)
	waitDone := make(chan error, 1)
	go func() { waitDone <- child.Wait() }()

	status := &syncBuffer{}
	runServeStatus(status, config.Config{DataDir: dataDir})
	assert.Contains(t, status.String(), "agentsview running at")
	assert.Contains(t, status.String(), fmt.Sprintf("pid:     %d", child.Process.Pid))

	// The stop runs in a goroutine so the test can time-box it, and it
	// writes into a buffer of its own rather than swapping the global
	// os.Stdout. When this test timed out on Windows, an abandoned
	// capture goroutine left os.Stdout pointing at a pipe nobody was
	// reading, and the next test that printed anything blocked forever
	// once that pipe filled - which is how two failures became a
	// ten-minute suite hang.
	stopOut := &syncBuffer{}
	stopDone := make(chan struct{}, 1)
	go func() {
		runServeStop(stopOut, config.Config{DataDir: dataDir})
		stopDone <- struct{}{}
	}()
	select {
	case <-waitDone:
	case <-time.After(5 * time.Second):
		t.Fatal("background child did not exit after stop")
	}
	select {
	case <-stopDone:
	case <-time.After(5 * time.Second):
		t.Fatal("serve stop did not return")
	}
	assert.Contains(t, stopOut.String(),
		fmt.Sprintf("Stopped agentsview (pid %d).", child.Process.Pid))
	require.Eventually(t, func() bool {
		return !processIsRunning(child.Process.Pid)
	}, 5*time.Second, 25*time.Millisecond)
	assert.Nil(t, FindDaemonRuntime(dataDir, ""))

	logBody, err := os.ReadFile(logPath)
	require.NoError(t, err)
	assert.Contains(t, string(logBody), "existing log")
	assert.Contains(t, string(logBody), "agentsview serve background start")
}

func TestPhase21ServeBackgroundLogUsesAppendOnRepeatedLaunches(t *testing.T) {
	dataDir := filepath.Join(t.TempDir(), "data")
	require.NoError(t, os.MkdirAll(dataDir, 0o700))
	t.Setenv("AGENTSVIEW_PHASE21_HELPER_MODE", "quick-exit")
	t.Setenv("AGENTSVIEW_PHASE21_HELPER_DATA_DIR", dataDir)

	for range 2 {
		child, _, err := startServeBackgroundProcess(
			config.Config{DataDir: dataDir},
			[]string{"-test.run=^TestPhase21ServeBackgroundHelperProcess$", "--background"},
		)
		require.NoError(t, err)
		require.NoError(t, child.Wait())
	}

	body, err := os.ReadFile(filepath.Join(dataDir, "serve.log"))
	require.NoError(t, err)
	assert.Equal(t, 2, strings.Count(string(body), "agentsview serve background start"))
}

func TestPhase21ServeBackgroundLaunchLockConcurrentAcquireOnlyOneSucceeds(t *testing.T) {
	dataDir := filepath.Join(t.TempDir(), "data")
	require.NoError(t, os.MkdirAll(dataDir, 0o700))
	start := make(chan struct{})
	results := make(chan *flock.Flock, 2)

	for range 2 {
		go func() {
			<-start
			lock, ok := acquireBackgroundLaunchLock(dataDir)
			if !ok {
				results <- nil
				return
			}
			results <- lock
		}()
	}
	close(start)

	var locks []*flock.Flock
	for range 2 {
		lock := <-results
		if lock != nil {
			locks = append(locks, lock)
		}
	}
	for _, lock := range locks {
		require.NoError(t, lock.Unlock())
	}
	assert.Len(t, locks, 1)
}

func TestPhase21ServeBackgroundChildArgsRemoveBackgroundFlagOnly(t *testing.T) {
	args := serveBackgroundChildArgs([]string{
		"serve", "--port", "0", "--background", "--host=127.0.0.1", "--background=true",
	})
	assert.Equal(t, []string{"serve", "--port", "0", "--host=127.0.0.1"}, args)
}

func TestPhase21ServeLifecycleStatusShowsStartupState(t *testing.T) {
	dataDir := runtimeTestDir(t)
	require.NoError(t, writeStartupState(dataDir, startupState{
		PID:     os.Getpid(),
		Phase:   "opening-db",
		LogPath: filepath.Join(dataDir, "serve.log"),
	}))

	out := &syncBuffer{}
	runServeStatus(out, config.Config{DataDir: dataDir})
	assert.Contains(t, out.String(), fmt.Sprintf("agentsview is starting (pid %d, phase opening-db).", os.Getpid()))
	assert.Contains(t, out.String(), filepath.Join(dataDir, "serve.log"))
}

func TestPhase21ServeStopSkipsMismatchedCreateTimeRuntime(t *testing.T) {
	dataDir := runtimeTestDir(t)
	_, err := writeRuntimeRecordForTest(dataDir, daemon.RuntimeRecord{
		PID:     os.Getpid(),
		Network: daemon.NetworkTCP,
		Address: "127.0.0.1:1",
		Service: daemonService,
		Metadata: map[string]string{
			runtimeCreateTime: "1",
		},
	})
	require.NoError(t, err)

	out := &syncBuffer{}
	runServeStop(out, config.Config{DataDir: dataDir})
	assert.Contains(t, out.String(), "No agentsview server is running.")
}

func TestPhase21ManagedCaddyRuntimeMetadataAndConfirmedOrphanCleanup(t *testing.T) {
	dataDir := runtimeTestDir(t)
	child := phase21StartHelperProcess(t, "runtime-caddy-child")
	ct, ok := processCreateTimeMillis(child.Process.Pid)
	require.True(t, ok)
	rec := daemon.RuntimeRecord{Metadata: map[string]string{
		runtimeCaddyPID:        strconv.Itoa(child.Process.Pid),
		runtimeCaddyCreateTime: strconv.FormatInt(ct, 10),
	}}

	caddyOut := &syncBuffer{}
	stopDone := make(chan struct{}, 1)
	go func() {
		stopOrphanedCaddyChild(caddyOut, rec)
		stopDone <- struct{}{}
	}()
	waitDone := make(chan error, 1)
	go func() { waitDone <- child.Wait() }()
	select {
	case <-waitDone:
	case <-time.After(5 * time.Second):
		t.Fatal("managed caddy child did not exit")
	}
	select {
	case <-stopDone:
	case <-time.After(5 * time.Second):
		t.Fatal("managed caddy cleanup did not return")
	}
	require.Eventually(t, func() bool {
		return !processIsRunning(child.Process.Pid)
	}, 5*time.Second, 25*time.Millisecond)

	_, err := WriteDaemonRuntime(dataDir, "127.0.0.1", 1, "phase21", false, daemonRuntimeOptions{CaddyPID: os.Getpid()})
	require.NoError(t, err)
	runtime := FindDaemonRuntime(dataDir, "")
	assert.Nil(t, runtime, "unprobeable runtime must not become confirmed")
}

func TestPhase21ManagedCaddyRuntimeUsesCapturedCreateTimeAfterChildExit(t *testing.T) {
	dataDir := runtimeTestDir(t)
	child := phase21StartHelperProcess(t, "runtime-caddy-child")
	ct, ok := processCreateTimeMillis(child.Process.Pid)
	require.True(t, ok)
	caddy := &managedCaddy{pid: child.Process.Pid, createTimeMillis: ct}
	require.NoError(t, child.Process.Kill())
	_ = child.Wait()

	path, err := WriteDaemonRuntime(dataDir, "127.0.0.1", 1, "phase21", false, daemonRuntimeOptions{
		CaddyPID:              caddy.Pid(),
		CaddyCreateTimeMillis: caddy.CreateTimeMillis(),
	})
	require.NoError(t, err)
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var rec daemon.RuntimeRecord
	require.NoError(t, json.Unmarshal(data, &rec))

	assert.Equal(t, strconv.Itoa(child.Process.Pid), rec.Metadata[runtimeCaddyPID])
	assert.Equal(t, strconv.FormatInt(ct, 10), rec.Metadata[runtimeCaddyCreateTime])
}

func TestPhase21ManagedCaddyMetadataOmittedWhenUnconfigured(t *testing.T) {
	dataDir := runtimeTestDir(t)
	path, err := WriteDaemonRuntime(dataDir, "127.0.0.1", 1, "phase21", false)
	require.NoError(t, err)
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var rec daemon.RuntimeRecord
	require.NoError(t, json.Unmarshal(data, &rec))

	assert.NotContains(t, rec.Metadata, runtimeCaddyPID)
	assert.NotContains(t, rec.Metadata, runtimeCaddyCreateTime)
}

func TestPhase21ManagedCaddyStopClosesGuard(t *testing.T) {
	guard := &recordingManagedCaddyGuard{}
	canceled := false
	caddy := &managedCaddy{
		cancel: func() { canceled = true },
		guard:  guard,
	}

	caddy.Stop()
	caddy.Stop()

	assert.True(t, canceled)
	assert.Equal(t, 1, guard.closeCount)
}

func TestPhase21ManagedCaddyMismatchedPIDDoesNotSignal(t *testing.T) {
	child := phase21StartHelperProcess(t, "runtime-caddy-child")
	rec := daemon.RuntimeRecord{Metadata: map[string]string{
		runtimeCaddyPID:        strconv.Itoa(child.Process.Pid),
		runtimeCaddyCreateTime: "1",
	}}

	stopOrphanedCaddyChild(io.Discard, rec)
	assert.True(t, processIsRunning(child.Process.Pid))
}

func phase21StartHelperProcess(t *testing.T, mode string) *exec.Cmd {
	t.Helper()
	dataDir := filepath.Join(t.TempDir(), "data")
	require.NoError(t, os.MkdirAll(dataDir, 0o700))
	cmd := exec.Command(os.Args[0], "-test.run=^TestPhase21ServeBackgroundHelperProcess$")
	cmd.Env = append(os.Environ(),
		"AGENTSVIEW_PHASE21_HELPER_MODE="+mode,
		"AGENTSVIEW_PHASE21_HELPER_DATA_DIR="+dataDir,
	)
	// A plain bytes.Buffer is not safe here: os/exec copies the child's
	// stdout into it from its own goroutine, and the Eventually below
	// reads it from this one. Under -race that is a reported data race,
	// which is how CI caught it and a local run without -race did not.
	stdout := &syncBuffer{}
	cmd.Stdout = stdout
	require.NoError(t, cmd.Start())
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	require.Eventually(t, func() bool {
		return strings.Contains(stdout.String(), "ready")
	}, 5*time.Second, 25*time.Millisecond)
	return cmd
}

// syncBuffer is an io.Writer a test can read while os/exec is still
// writing to it.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

type recordingManagedCaddyGuard struct {
	closeCount int
}

func (g *recordingManagedCaddyGuard) Close() {
	g.closeCount++
}
