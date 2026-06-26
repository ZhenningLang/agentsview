package backup

import (
	"os"
	"path/filepath"
	"testing"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func mustExist(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected %s to exist: %v", path, err)
	}
}

func mustNotExist(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err == nil {
		t.Fatalf("expected %s to NOT exist", path)
	}
}

// TestSyncSources_CollectsBothSources proves the two sources land in the right
// subtrees, the cross-agent source's .git is excluded, and CC-native is laid
// out as cc-native/<project>/memory/*.
func TestSyncSources_CollectsBothSources(t *testing.T) {
	cross := t.TempDir()
	writeFile(t, filepath.Join(cross, "user-primary-agents.md"), "a")
	writeFile(t, filepath.Join(cross, "nested", "note.md"), "b")
	// The cross-agent dir is itself a git repo; its history must NOT be copied.
	writeFile(t, filepath.Join(cross, ".git", "config"), "[core]")

	ccRoot := t.TempDir()
	writeFile(t, filepath.Join(ccRoot, "projA", "memory", "MEMORY.md"), "idx")
	writeFile(t, filepath.Join(ccRoot, "projA", "memory", "fact.md"), "x")
	// A project without a memory/ subdir is skipped.
	writeFile(t, filepath.Join(ccRoot, "projB", "sessions.jsonl"), "noise")

	ws := t.TempDir()
	res, err := SyncSources(ws, cross, ccRoot)
	if err != nil {
		t.Fatalf("sync: %v", err)
	}

	mustExist(t, filepath.Join(ws, crossAgentSubdir, "user-primary-agents.md"))
	mustExist(t, filepath.Join(ws, crossAgentSubdir, "nested", "note.md"))
	mustNotExist(t, filepath.Join(ws, crossAgentSubdir, ".git", "config"))
	mustNotExist(t, filepath.Join(ws, crossAgentSubdir, ".git"))

	mustExist(t, filepath.Join(ws, ccNativeSubdir, "projA", "memory", "fact.md"))
	mustExist(t, filepath.Join(ws, ccNativeSubdir, "projA", "memory", "MEMORY.md"))
	mustNotExist(t, filepath.Join(ws, ccNativeSubdir, "projB"))

	if res.CrossAgentFiles != 2 {
		t.Fatalf("cross-agent files = %d, want 2", res.CrossAgentFiles)
	}
	if res.CCNativeFiles != 2 {
		t.Fatalf("cc-native files = %d, want 2", res.CCNativeFiles)
	}
}

// TestSyncSources_ReplacesStaleFiles proves a sync fully replaces the managed
// subtree so a deletion in the source propagates into the backup workspace.
func TestSyncSources_ReplacesStaleFiles(t *testing.T) {
	cross := t.TempDir()
	writeFile(t, filepath.Join(cross, "keep.md"), "1")
	writeFile(t, filepath.Join(cross, "gone.md"), "2")

	ws := t.TempDir()
	if _, err := SyncSources(ws, cross, ""); err != nil {
		t.Fatalf("sync 1: %v", err)
	}
	mustExist(t, filepath.Join(ws, crossAgentSubdir, "gone.md"))

	// Remove a source file; the next sync must remove it from the workspace.
	if err := os.Remove(filepath.Join(cross, "gone.md")); err != nil {
		t.Fatal(err)
	}
	if _, err := SyncSources(ws, cross, ""); err != nil {
		t.Fatalf("sync 2: %v", err)
	}
	mustExist(t, filepath.Join(ws, crossAgentSubdir, "keep.md"))
	mustNotExist(t, filepath.Join(ws, crossAgentSubdir, "gone.md"))
}

// TestSyncSources_EmptySourcesClearSubtrees proves "" sources are skipped and
// their subtrees cleared rather than erroring.
func TestSyncSources_EmptySourcesClearSubtrees(t *testing.T) {
	ws := t.TempDir()
	res, err := SyncSources(ws, "", "")
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if res.CrossAgentFiles != 0 || res.CCNativeFiles != 0 {
		t.Fatalf("unexpected counts %+v", res)
	}
	mustExist(t, filepath.Join(ws, crossAgentSubdir))
	mustExist(t, filepath.Join(ws, ccNativeSubdir))
}

func TestSyncSources_SkipsAuditJsonl(t *testing.T) {
	cross := t.TempDir()
	writeFile(t, filepath.Join(cross, ".extract-audit.jsonl"), "audit")
	writeFile(t, filepath.Join(cross, "note.md"), "keep")

	ws := t.TempDir()
	if _, err := SyncSources(ws, cross, ""); err != nil {
		t.Fatalf("sync: %v", err)
	}
	mustExist(t, filepath.Join(ws, crossAgentSubdir, "note.md"))
	mustNotExist(t, filepath.Join(ws, crossAgentSubdir, ".extract-audit.jsonl"))
}

func TestSyncSources_SkipsDatabaseFiles(t *testing.T) {
	cross := t.TempDir()
	writeFile(t, filepath.Join(cross, "memory.db"), "llm_embedding bytes")
	writeFile(t, filepath.Join(cross, "note.md"), "keep")

	ws := t.TempDir()
	if _, err := SyncSources(ws, cross, ""); err != nil {
		t.Fatalf("sync: %v", err)
	}
	mustExist(t, filepath.Join(ws, crossAgentSubdir, "note.md"))
	mustNotExist(t, filepath.Join(ws, crossAgentSubdir, "memory.db"))
}
