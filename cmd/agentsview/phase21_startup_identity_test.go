package main

import (
	"bytes"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.kenn.io/agentsview/internal/config"
)

func TestPhase21StartupStateRejectsReusedPIDWithDifferentCreateTime(t *testing.T) {
	dataDir := runtimeTestDir(t)
	require.NoError(t, os.MkdirAll(dataDir, 0o700))

	// A live PID whose creation time does not match the record: the
	// writing process is gone and an unrelated program now owns the PID.
	require.NoError(t, writeStartupState(dataDir, startupState{
		PID:              os.Getpid(),
		Phase:            "opening-db",
		CreateTimeMillis: 1,
	}))

	_, ok := readStartupState(dataDir)
	assert.False(t, ok, "impostor PID was accepted as the startup owner")
	_, statErr := os.Stat(startupStatePath(dataDir))
	assert.True(t, os.IsNotExist(statErr), "stale startup state was not removed")

	out := &bytes.Buffer{}
	runServeStatus(out, config.Config{DataDir: dataDir})
	assert.Contains(t, out.String(), "No agentsview server is running.")
}

func TestPhase21StartupStateRecordsCreateTimeAndAcceptsItsOwner(t *testing.T) {
	dataDir := runtimeTestDir(t)
	require.NoError(t, os.MkdirAll(dataDir, 0o700))
	require.NoError(t, writeStartupState(dataDir, startupState{Phase: "opening-db"}))

	state, ok := readStartupState(dataDir)
	require.True(t, ok, "the writing process was not recognized as the owner")
	assert.Equal(t, os.Getpid(), state.PID)
	live, okCreate := processCreateTimeMillis(os.Getpid())
	require.True(t, okCreate, "process creation time is unavailable")
	assert.Equal(t, live, state.CreateTimeMillis)
}
