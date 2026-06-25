package backup

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Subdir names inside the backup workspace. The two memory sources land in
// fixed, separate trees so the private repo's layout is stable and auditable.
const (
	// crossAgentSubdir holds a copy of memory/user/* (the cross-agent SSOT).
	crossAgentSubdir = "cross-agent"
	// ccNativeSubdir holds CC-native auto-memory, one tree per project:
	// cc-native/<project>/memory/*.
	ccNativeSubdir = "cc-native"
)

// SyncResult reports how many files were copied from each source in a sync.
type SyncResult struct {
	CrossAgentFiles int
	CCNativeFiles   int
}

// SyncSources copies the two memory sources into the backup workspace, fully
// replacing the managed subtrees so deletions in a source propagate. It NEVER
// touches the workspace's own .git, and it never copies a source's .git (the
// cross-agent memory dir is itself a local git repo whose history must not leak
// into the backup repo). Per the locked spec it takes a fresh snapshot every
// cycle, so CC-native edits that P2 does not commit are still captured here.
//
// crossAgentDir is the resolved memory/user dir (may be ""); ccRoot is the
// resolved CC-native root whose immediate children are project dirs each with a
// memory/ subdir (may be ""). A "" source is skipped (its subtree is cleared so
// a removed source does not linger in the backup).
func SyncSources(workspace, crossAgentDir, ccRoot string) (SyncResult, error) {
	var res SyncResult

	crossDst := filepath.Join(workspace, crossAgentSubdir)
	if err := resetDir(crossDst); err != nil {
		return res, fmt.Errorf("resetting cross-agent dir: %w", err)
	}
	if crossAgentDir != "" {
		n, err := copyTree(crossAgentDir, crossDst, true)
		if err != nil {
			return res, fmt.Errorf("copying cross-agent memory: %w", err)
		}
		res.CrossAgentFiles = n
	}

	ccDst := filepath.Join(workspace, ccNativeSubdir)
	if err := resetDir(ccDst); err != nil {
		return res, fmt.Errorf("resetting cc-native dir: %w", err)
	}
	if ccRoot != "" {
		n, err := copyCCNative(ccRoot, ccDst)
		if err != nil {
			return res, fmt.Errorf("copying cc-native memory: %w", err)
		}
		res.CCNativeFiles = n
	}

	return res, nil
}

// copyCCNative copies each project's memory/ subdir under ccRoot into
// dst/<project>/memory/. Projects without a memory/ subdir are skipped.
func copyCCNative(ccRoot, dst string) (int, error) {
	entries, err := os.ReadDir(ccRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	total := 0
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		srcMem := filepath.Join(ccRoot, e.Name(), "memory")
		info, err := os.Stat(srcMem)
		if err != nil || !info.IsDir() {
			continue
		}
		dstMem := filepath.Join(dst, e.Name(), "memory")
		n, err := copyTree(srcMem, dstMem, false)
		if err != nil {
			return total, err
		}
		total += n
	}
	return total, nil
}

// resetDir removes dir and recreates it empty, so a sync fully replaces the
// managed subtree (deletions in the source propagate to the backup).
func resetDir(dir string) error {
	if err := os.RemoveAll(dir); err != nil {
		return err
	}
	return os.MkdirAll(dir, 0o700)
}

// copyTree recursively copies every regular file under src into dst, preserving
// the relative structure. When skipGit is true a top-level (or nested) .git
// directory in the source is skipped, so a source that is itself a git repo
// never leaks its history into the backup workspace. It returns the number of
// files copied. A missing src is treated as empty (0 files, no error).
func copyTree(src, dst string, skipGit bool) (int, error) {
	info, err := os.Stat(src)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	if !info.IsDir() {
		return 0, nil
	}
	count := 0
	err = filepath.WalkDir(src, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			if skipGit && d.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		// Skip symlinks and other irregular files: only copy real file bytes.
		if !d.Type().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		// Defensive: never copy anything that resolves outside dst.
		if strings.HasPrefix(rel, "..") {
			return nil
		}
		target := filepath.Join(dst, rel)
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			return err
		}
		if err := copyFile(path, target); err != nil {
			return err
		}
		count++
		return nil
	})
	return count, err
}

// copyFile copies a single regular file's contents.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
