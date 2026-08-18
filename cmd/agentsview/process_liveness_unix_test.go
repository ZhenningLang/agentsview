//go:build !windows

package main

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPhase21ProcessLivenessUnixTracksSelfAndAReapedChild is the Unix
// half of the pair. The same contract holds - a reaped child is not
// running - but it comes for free from signal 0, so this exists to keep
// both platforms behaviourally covered rather than only the one that
// needed the work.
func TestPhase21ProcessLivenessUnixTracksSelfAndAReapedChild(t *testing.T) {
	assert.True(t, processIsRunning(os.Getpid()))
	assert.False(t, processIsRunning(0))
	assert.False(t, processIsRunning(-1))

	child := phase21StartHelperProcess(t, "runtime-caddy-child")
	pid := child.Process.Pid
	require.True(t, processIsRunning(pid), "the helper is not running")

	require.NoError(t, child.Process.Kill())
	_ = child.Wait()

	require.Eventually(t, func() bool {
		return !processIsRunning(pid)
	}, 5*time.Second, 25*time.Millisecond,
		"a reaped child still reported as running")
}
