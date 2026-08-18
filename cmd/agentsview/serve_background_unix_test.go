//go:build !windows

package main

import (
	"os/exec"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPhase21PlatformDetachUnixSetsid(t *testing.T) {
	cmd := exec.Command("agentsview-test-helper")
	configureServeBackgroundCommand(cmd)
	require.NotNil(t, cmd.SysProcAttr)
	require.True(t, cmd.SysProcAttr.Setsid)
}
