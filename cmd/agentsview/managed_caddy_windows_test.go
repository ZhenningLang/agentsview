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
	require.NotNil(t, guard,
		"the Unix half returns a usable guard for this input; so must this one")
	assert.NotPanics(t, guard.Close, "closing a guard with no job object")
	assert.NotPanics(t, guard.Close, "closing twice")
}
