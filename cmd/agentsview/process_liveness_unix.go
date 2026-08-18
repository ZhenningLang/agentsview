//go:build !windows

package main

import "go.kenn.io/kit/daemon"

// processIsRunning reports whether a PID belongs to a process that has
// not exited yet.
//
// On Unix a pending exit status keeps the PID reserved but signal 0
// still succeeds for it, and kit's check already encodes that plus the
// EPERM case for another user's process. The Windows half of this pair
// has to do considerably more work; see process_liveness_windows.go.
func processIsRunning(pid int) bool {
	return daemon.ProcessAlive(pid)
}
