//go:build windows

package backup

import (
	"errors"

	"golang.org/x/sys/windows"
)

// processAlive reports whether a process with the given pid exists on Windows.
// os.Process.Signal does not provide Unix signal-0 semantics there, so use the
// Windows process handle and wait APIs directly.
func processAlive(pid int) bool {
	handle, err := windows.OpenProcess(
		windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.SYNCHRONIZE,
		false,
		uint32(pid),
	)
	if err != nil {
		if errors.Is(err, windows.ERROR_ACCESS_DENIED) {
			return true
		}
		return false
	}
	defer windows.CloseHandle(handle)
	event, err := windows.WaitForSingleObject(handle, 0)
	if err != nil {
		return true
	}
	return event == uint32(windows.WAIT_TIMEOUT)
}
