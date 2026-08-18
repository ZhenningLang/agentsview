//go:build windows

package main

import (
	"errors"
	"syscall"

	"golang.org/x/sys/windows"
)

// stillActiveExitCode is what GetExitCodeProcess reports for a process
// that has not exited (STILL_ACTIVE). It is also a legal exit code, which
// is why the wait below is the primary signal and this is only a
// fallback.
const stillActiveExitCode = 259

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
// matter how many handles are open. That wait needs the SYNCHRONIZE
// access right: asking only for PROCESS_QUERY_LIMITED_INFORMATION makes
// WaitForSingleObject fail with access denied, and a check that treats a
// failed wait as "still running" then reports every process as alive
// forever. That was the first attempt at this function, and it is why
// three Windows tests hung rather than two.
//
// GetExitCodeProcess is the fallback rather than the primary signal: a
// process that genuinely exits with code 259 is indistinguishable from a
// running one under STILL_ACTIVE, so it is only consulted when the wait
// could not be performed at all.
func processIsRunning(pid int) bool {
	if pid <= 0 {
		return false
	}

	handle, err := windows.OpenProcess(
		windows.SYNCHRONIZE|windows.PROCESS_QUERY_LIMITED_INFORMATION,
		false, uint32(pid),
	)
	if err == nil {
		defer func() { _ = windows.CloseHandle(handle) }()
		if state, waitErr := windows.WaitForSingleObject(handle, 0); waitErr == nil {
			// WAIT_OBJECT_0 means the handle is signalled, which for a
			// process handle means it has exited. WAIT_TIMEOUT means it
			// is still running.
			return state != windows.WAIT_OBJECT_0
		}
		return !processExited(handle)
	}

	// SYNCHRONIZE can be denied for a process owned by another user.
	// A query-only handle still answers through the exit code.
	handle, err = windows.OpenProcess(
		windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid),
	)
	if err != nil {
		// Access denied means the process exists and is someone else's.
		// Anything else means there is no such process.
		return errors.Is(err, syscall.ERROR_ACCESS_DENIED)
	}
	defer func() { _ = windows.CloseHandle(handle) }()
	return !processExited(handle)
}

// processExited reports whether a process handle carries a real exit
// code. An unreadable exit code answers "not exited": a false report of
// death would let a second writer open the archive.
func processExited(handle windows.Handle) bool {
	var code uint32
	if err := windows.GetExitCodeProcess(handle, &code); err != nil {
		return false
	}
	return code != stillActiveExitCode
}
