package main

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPhase15OutputFormatResolves(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"default", nil, "human"},
		{"format json", []string{"--format", "json"}, "json"},
		{"json alias", []string{"--json"}, "json"},
		{"alias wins", []string{"--json", "--format", "human"}, "json"},
		{"explicit false uses format", []string{"--json=false", "--format", "json"}, "json"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &cobra.Command{Use: "x"}
			registerFormatFlags(cmd.Flags())
			require.NoError(t, cmd.ParseFlags(tt.args))
			assert.Equal(t, tt.want, outputFormat(cmd))
		})
	}
}

func TestPhase15OutputFormatRejectsInvalid(t *testing.T) {
	cmd := &cobra.Command{Use: "x"}
	registerFormatFlags(cmd.Flags())
	err := cmd.ParseFlags([]string{"--format", "yaml"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be human or json")
}

func TestPhase15MachineOutputCommandFlagsArePaired(t *testing.T) {
	paths := [][]string{
		{"version"},
		{"projects"},
		{"health"},
		{"usage", "daily"},
		{"usage", "statusline"},
		{"stats"},
		{"secrets", "list"},
		{"secrets", "scan"},
		{"session", "list"},
		{"session", "get"},
		{"session", "messages"},
		{"session", "tool-calls"},
		{"session", "search"},
		{"session", "usage"},
		{"session", "sync"},
	}
	for _, path := range paths {
		t.Run(strings.Join(path, " "), func(t *testing.T) {
			cmd, _, err := newRootCommand().Find(path)
			require.NoError(t, err)
			assert.NotNil(t, cmd.Flag("format"), path)
			assert.NotNil(t, cmd.Flag("json"), path)

			cmd, _, err = newRootCommand().Find(path)
			require.NoError(t, err)
			require.NoError(t, cmd.ParseFlags([]string{"--format", "json"}))
			assert.Equal(t, "json", outputFormat(cmd))

			cmd, _, err = newRootCommand().Find(path)
			require.NoError(t, err)
			require.NoError(t, cmd.ParseFlags([]string{"--json"}))
			assert.Equal(t, "json", outputFormat(cmd))
		})
	}
}

func TestPhase15FormatAndJSONFlagsArePairedAcrossCommandTree(t *testing.T) {
	jsonOnly := map[string]bool{
		"agentsview openapi":   true,
		"agentsview token-use": true,
	}
	var walk func(*cobra.Command)
	walk = func(cmd *cobra.Command) {
		if cmd.Hidden {
			return
		}
		hasFormat := cmd.Flag("format") != nil
		hasJSON := cmd.Flag("json") != nil
		if !jsonOnly[cmd.CommandPath()] {
			assert.Equal(t, hasFormat, hasJSON, cmd.CommandPath())
		}
		for _, child := range cmd.Commands() {
			walk(child)
		}
	}
	walk(newRootCommand())
}

func TestPhase15VersionRejectsInvalidFormat(t *testing.T) {
	_, err := executeCommand(newRootCommand(), "version", "--format", "yaml")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be human or json")
}

func TestPhase15SessionExportAndWatchRejectFormatFlags(t *testing.T) {
	_, err := executeCommand(newRootCommand(), "session", "export", "abc", "--format", "json")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "session export: streams raw bytes")
	assert.Contains(t, err.Error(), "--format not supported")

	_, err = executeCommand(newRootCommand(), "session", "watch", "abc", "--json")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "session watch: streams NDJSON")
	assert.Contains(t, err.Error(), "--json alias also unsupported")
}
