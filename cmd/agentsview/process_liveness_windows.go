//go:build windows

package main

import (
	"errors"
	"syscall"

	"golang.org/x/sys/windows"
)

// processIsRunning reports whether a PID belongs to a process that has
// not exited yet.
//
// Windows keeps a process object alive for as long as any handle to it
// is open, so a terminated process stays openable and a liveness check
// built on OpenProcess alone answers "still running" for a process that
// is already dead.
//
// That is not academic here. os.FindProcess opens such a handle on
// Windows and holds it, so stopDaemonProcess was keeping alive the very
// object its own wait-for-exit loop was polling: it could never observe
// the exit it had just caused, burned the full grace period twice, and
// then reported the process as unkillable. On Unix os.FindProcess is a
// no-op wrapper, which is why the same code is correct there.
//
// A process handle is a waitable object that becomes signalled when the
// process exits, so a zero-timeout wait answers the real question no
// matter how many handles are open. GetExitCodeProcess is deliberately
// not used: a process that genuinely exits with code 259 is
// indistinguishable from a running one under STILL_ACTIVE.
func processIsRunning(pid int) bool {
	if pid <= 0 {
		return false
	}
	handle, err := windows.OpenProcess(
		windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid),
	)
	if err != nil {
		// Access denied means the process exists and belongs to another
		// user. Anything else means there is no such process.
		return errors.Is(err, syscall.ERROR_ACCESS_DENIED)
	}
	defer func() { _ = windows.CloseHandle(handle) }()

	state, err := windows.WaitForSingleObject(handle, 0)
	if err != nil {
		// The handle could not be waited on. Treat the process as
		// running rather than reporting an exit that was never
		// observed - a false "dead" would let a second writer start.
		return true
	}
	return state != windows.WAIT_OBJECT_0
}
