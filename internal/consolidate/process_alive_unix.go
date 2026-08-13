//go:build !windows

package consolidate

import (
	"errors"
	"os"
	"syscall"
)

// processAlive reports whether a process with the given pid exists. On Unix,
// FindProcess always succeeds, so probe with signal 0: nil or EPERM both mean
// the process exists.
func processAlive(pid int) bool {
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = p.Signal(syscall.Signal(0))
	if err == nil {
		return true
	}
	return errors.Is(err, os.ErrPermission)
}
