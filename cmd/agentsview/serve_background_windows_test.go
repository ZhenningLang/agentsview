//go:build windows

package main

import (
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPhase21PlatformDetachWindowsCreationFlags(t *testing.T) {
	cmd := exec.Command("agentsview-test-helper.exe")
	configureServeBackgroundCommand(cmd)
	require.NotNil(t, cmd.SysProcAttr)
	assert.NotZero(t, cmd.SysProcAttr.CreationFlags&detachedProcess)
	assert.NotZero(t, cmd.SysProcAttr.CreationFlags&createNewProcessGroup)
}
