//go:build !windows

package main

import (
	"os"
	"os/exec"
	"syscall"
)

func configureServeBackgroundCommand(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}

func terminateProcess(proc *os.Process) error {
	return proc.Signal(syscall.SIGTERM)
}
