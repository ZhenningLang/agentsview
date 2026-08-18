//go:build windows

package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPhase21ManagedCaddyWindowsGuardHandlesNilProcess(t *testing.T) {
	guard, err := guardManagedCaddyProcess(nil)
	require.NoError(t, err)
	assert.Nil(t, guard)
}
