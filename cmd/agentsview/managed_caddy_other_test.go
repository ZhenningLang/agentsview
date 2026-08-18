//go:build !windows

package main

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPhase21ManagedCaddyOtherPlatformGuardIsNoop(t *testing.T) {
	guard, err := guardManagedCaddyProcess(&os.Process{Pid: os.Getpid()})
	require.NoError(t, err)
	require.NotNil(t, guard)
	guard.Close()

	assert.IsType(t, noopManagedCaddyGuard{}, guard)
}
