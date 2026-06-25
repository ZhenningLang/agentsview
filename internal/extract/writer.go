package extract

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"
)

const rawStagingRel = "memory/.staging/raw_memories"

type WriteStatus string

const (
	WriteWritten        WriteStatus = "written"
	WriteDeduped        WriteStatus = "deduped"
	WriteDriftRefused   WriteStatus = "drift_refused"
	WriteGitignoreError WriteStatus = "gitignore_refused"
)

type WriteResult struct {
	Status      WriteStatus `json:"status"`
	CandidateID string      `json:"candidate_id"`
	Path        string      `json:"path,omitempty"`
	Reason      string      `json:"reason,omitempty"`
}

type Writer struct {
	Root string
}

func RawDir(root string) string {
	return filepath.Join(root, "memory", ".staging", "raw_memories")
}

func (w Writer) Write(ctx context.Context, c Candidate) (WriteResult, error) {
	root, err := filepath.Abs(w.Root)
	if err != nil {
		return WriteResult{}, err
	}
	if err := c.Validate(); err != nil {
		return WriteResult{}, err
	}
	c.ID = CandidateID(c)
	targetDir := RawDir(root)
	target := filepath.Join(targetDir, c.ID+".json")
	targetRel := filepath.ToSlash(filepath.Join(rawStagingRel, c.ID+".json"))
	ignored, err := gitignoreAllows(ctx, root, target)
	if err != nil || !ignored {
		return WriteResult{Status: WriteGitignoreError, CandidateID: c.ID, Reason: "memory/.staging/raw_memories is not ignored"}, nil
	}
	if err := os.MkdirAll(targetDir, 0o700); err != nil {
		return WriteResult{}, err
	}
	text, err := CanonicalJSON(c)
	if err != nil {
		return WriteResult{}, err
	}
	lockPath := filepath.Join(targetDir, ".lock")
	lock, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return WriteResult{}, err
	}
	defer lock.Close()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		return WriteResult{}, err
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)

	if existing, err := os.ReadFile(target); err == nil {
		if sameStableCandidate(string(existing), c) {
			return WriteResult{Status: WriteDeduped, CandidateID: c.ID, Path: targetRel}, nil
		}
		bak := filepath.Join(targetDir, fmt.Sprintf("%s.%d.bak", filepath.Base(target), time.Now().Unix()))
		_ = os.WriteFile(bak, []byte(fmt.Sprintf("{\"status\":\"drift_refused\",\"candidate_id\":\"%s\",\"existing_size\":%d}\n", c.ID, len(existing))), 0o600)
		return WriteResult{Status: WriteDriftRefused, CandidateID: c.ID, Path: targetRel, Reason: "target differs from canonical candidate"}, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return WriteResult{}, err
	}

	tmp, err := os.CreateTemp(targetDir, "."+c.ID+".*.tmp")
	if err != nil {
		return WriteResult{}, err
	}
	tmpName := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpName)
		}
	}()
	if _, err := tmp.WriteString(text); err != nil {
		_ = tmp.Close()
		return WriteResult{}, err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return WriteResult{}, err
	}
	if err := tmp.Close(); err != nil {
		return WriteResult{}, err
	}
	if err := os.Rename(tmpName, target); err != nil {
		return WriteResult{}, err
	}
	cleanup = false
	fSyncDir(targetDir)
	return WriteResult{Status: WriteWritten, CandidateID: c.ID, Path: targetRel}, nil
}

func gitignoreAllows(ctx context.Context, root, target string) (bool, error) {
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return false, err
	}
	cmd := exec.CommandContext(ctx, "git", "check-ignore", "-q", filepath.ToSlash(rel))
	cmd.Dir = root
	err = cmd.Run()
	if err == nil {
		return true, nil
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) && exit.ExitCode() == 1 {
		return false, nil
	}
	return false, err
}

func fSyncDir(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	_ = f.Sync()
}
