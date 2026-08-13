//go:build windows

package extract

import (
	"os"

	"github.com/gofrs/flock"
)

func acquireFileLock(f *os.File) (func() error, error) {
	lock := flock.New(f.Name())
	if err := lock.Lock(); err != nil {
		return nil, err
	}
	return lock.Unlock, nil
}
