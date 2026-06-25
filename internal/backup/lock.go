package backup

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

// processAlive reports whether a process with the given pid exists. On Unix,
// FindProcess always succeeds, so we probe with signal 0: a nil error or
// EPERM (the process exists but is owned by another user) both mean "alive".
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

// ErrLocked is returned by AcquireLock when another holder owns the lock. The
// caller treats it as "skip this cycle", not a failure.
var ErrLocked = errors.New("backup: lock held by another instance")

// lockFile is the single-flight lock filename created inside the backup
// workspace. A live agentsview instance plus a CLI invocation (or a second
// desktop instance) can race to push the same workspace; the O_EXCL pid file
// makes the loser skip its cycle (locked decision: "周期前抢 .backup.lock,抢不到跳过").
const lockFile = ".backup.lock"

// Lock is an acquired single-flight lock. Release deletes the pid file.
type Lock struct {
	path string
}

// AcquireLock tries to take the backup lock by exclusively creating
// <backupDir>/.backup.lock with the current pid. It returns ErrLocked when the
// file already exists (another holder). A stale lock whose recorded pid is no
// longer running is reclaimed so a crashed run does not wedge the worker forever.
func AcquireLock(backupDir string) (*Lock, error) {
	if err := os.MkdirAll(backupDir, 0o700); err != nil {
		return nil, fmt.Errorf("creating backup dir for lock: %w", err)
	}
	path := filepath.Join(backupDir, lockFile)
	if l, err := tryCreate(path); err == nil {
		return l, nil
	} else if !errors.Is(err, os.ErrExist) {
		return nil, err
	}
	// Lock exists: reclaim it only if its pid is dead.
	if _, dead := lockIsStale(path); dead {
		// Best-effort reclaim: remove the stale file, then retry once.
		_ = os.Remove(path)
		if l, err := tryCreate(path); err == nil {
			return l, nil
		}
	}
	return nil, ErrLocked
}

// tryCreate exclusively creates the pid file. It returns os.ErrExist (wrapped)
// when the file already exists.
func tryCreate(path string) (*Lock, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, err
	}
	_, _ = f.WriteString(strconv.Itoa(os.Getpid()))
	_ = f.Close()
	return &Lock{path: path}, nil
}

// lockIsStale reports whether the lock file records a pid that is no longer a
// live process. An unreadable/empty/unparseable pid is treated as not stale
// (conservative: do not steal a lock we cannot reason about).
func lockIsStale(path string) (pid int, stale bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, false
	}
	pid, err = strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || pid <= 0 {
		return 0, false
	}
	if processAlive(pid) {
		return pid, false
	}
	return pid, true
}

// Release removes the lock pid file. It is safe to call once; a missing file is
// ignored.
func (l *Lock) Release() {
	if l == nil {
		return
	}
	_ = os.Remove(l.path)
}
