package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.kenn.io/agentsview/internal/config"
)

func TestEnrichCommandRegistrationAndFlags(t *testing.T) {
	help, err := executeCommand(newRootCommand(), "--help")
	require.NoError(t, err)
	assert.Contains(t, help, "enrich")

	enrichHelp, err := executeCommand(newRootCommand(), "enrich", "--help")
	require.NoError(t, err)
	for _, want := range []string{"--all", "--project", "--force", "--limit"} {
		assert.Contains(t, enrichHelp, want)
	}
}

func TestEnrichCommandRejectsInvalidFlags(t *testing.T) {
	_, err := executeCommand(newRootCommand(), "enrich", "--limit", "-1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--limit must be >= 0")

	_, err = executeCommand(newRootCommand(), "enrich", "--all", "--limit", "1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--all cannot be combined")
}

func TestEnrichCommandDisabled(t *testing.T) {
	cfg, err := config.Default()
	require.NoError(t, err)
	cfg.DataDir = t.TempDir()
	cfg.DBPath = cfg.DataDir + "/sessions.db"
	cmd := newEnrichCommand()
	err = runEnrich(cmd, cfg, enrichCLIOptions{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "disabled")
}
