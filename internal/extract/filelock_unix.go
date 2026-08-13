//go:build !windows

package extract

import (
	"os"
	"syscall"
)

func acquireFileLock(f *os.File) (func() error, error) {
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		return nil, err
	}
	return func() error {
		return syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	}, nil
}
