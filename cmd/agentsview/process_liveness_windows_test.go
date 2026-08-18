//go:build windows

package main

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPhase21ProcessLivenessWindowsReportsExitWhileAHandleIsOpen pins the
// Windows-specific half of the liveness contract.
//
// A Windows process object survives the process itself for as long as any
// handle to it is open, and os.FindProcess opens one. stopDaemonProcess
// calls os.FindProcess and then waits for the exit it just caused, so a
// liveness check that only asks whether the PID can be opened answers
// "still running" forever: every stop burned its whole grace period twice
// and then reported the process as unkillable. This test recreates that
// exact shape - an extra open handle across the exit - and requires the
// answer to be "not running".
func TestPhase21ProcessLivenessWindowsReportsExitWhileAHandleIsOpen(t *testing.T) {
	child := phase21StartHelperProcess(t, "runtime-caddy-child")
	pid := child.Process.Pid
	require.True(t, processIsRunning(pid), "the helper is not running")

	held, err := os.FindProcess(pid)
	require.NoError(t, err)
	t.Cleanup(func() { _ = held.Release() })

	require.NoError(t, child.Process.Kill())
	_ = child.Wait()

	require.Eventually(t, func() bool {
		return !processIsRunning(pid)
	}, 5*time.Second, 25*time.Millisecond,
		"a terminated process still reported as running while a handle to it was open")
}

func TestPhase21ProcessLivenessWindowsRejectsInvalidPIDs(t *testing.T) {
	assert.False(t, processIsRunning(0))
	assert.False(t, processIsRunning(-1))
	assert.True(t, processIsRunning(os.Getpid()))
}
