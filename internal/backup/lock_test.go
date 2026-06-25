package backup

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func TestAcquireLock_SecondHolderSkips(t *testing.T) {
	dir := t.TempDir()
	l1, err := AcquireLock(dir)
	if err != nil {
		t.Fatalf("first acquire: %v", err)
	}
	defer l1.Release()

	if _, err := AcquireLock(dir); !errors.Is(err, ErrLocked) {
		t.Fatalf("second acquire = %v, want ErrLocked", err)
	}
}

func TestAcquireLock_ReleaseAllowsReacquire(t *testing.T) {
	dir := t.TempDir()
	l1, err := AcquireLock(dir)
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	l1.Release()
	l2, err := AcquireLock(dir)
	if err != nil {
		t.Fatalf("reacquire after release: %v", err)
	}
	l2.Release()
}

func TestAcquireLock_StalePidReclaimed(t *testing.T) {
	dir := t.TempDir()
	// Write a lock file with a pid that is essentially never alive.
	path := filepath.Join(dir, lockFile)
	if err := os.WriteFile(path, []byte(strconv.Itoa(1<<30)), 0o600); err != nil {
		t.Fatal(err)
	}
	l, err := AcquireLock(dir)
	if err != nil {
		t.Fatalf("expected stale lock to be reclaimed, got %v", err)
	}
	l.Release()
}
