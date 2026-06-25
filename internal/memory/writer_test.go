package memory

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fixtureRepo creates a temp memory dir that is a local-only git repo with
// one committed note, and returns the dir and the note's rel_path. It NEVER
// touches the real ~/.dotfiles/memory: every test gets an isolated t.TempDir.
func fixtureRepo(t *testing.T) (dir, relPath string) {
	t.Helper()
	dir = t.TempDir()
	relPath = "alpha.md"

	content := "---\ntitle: Alpha\ndate: 2026-06-20\n" +
		"problem_type: knowledge\nstatus: active\n" +
		"origin_session: sess-a\n---\n\nOriginal alpha body.\n"
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, relPath), []byte(content), 0o644))

	gitInit(t, dir)
	gitCommitAll(t, dir, "initial")
	return dir, relPath
}

func gitInit(t *testing.T, dir string) {
	t.Helper()
	run(t, dir, "init")
	// Identity is required for commits in a clean CI env.
	run(t, dir, "config", "user.email", "test@example.com")
	run(t, dir, "config", "user.name", "Test User")
	// Never let the test repo have a remote; this is a hard local-only guard.
}

func gitCommitAll(t *testing.T, dir, msg string) {
	t.Helper()
	run(t, dir, "add", "-A")
	run(t, dir, "commit", "-m", msg)
}

func run(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git %v: %s", args, out)
	return string(out)
}

func sha(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}

func readFile(t *testing.T, dir, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, rel))
	require.NoError(t, err)
	return string(b)
}

func TestWriteHappyPath(t *testing.T) {
	dir, rel := fixtureRepo(t)
	w := NewWriter(dir)
	ctx := context.Background()

	base := sha(readFile(t, dir, rel))
	newContent := "---\ntitle: Alpha v2\ndate: 2026-06-21\n" +
		"problem_type: knowledge\nstatus: active\n" +
		"origin_session: sess-a\n---\n\nUpdated alpha body.\n"

	gotSHA, err := w.Write(ctx, WriteRequest{
		RelPath: rel, Content: newContent, BaseSHA: base,
	})
	require.NoError(t, err)
	assert.Equal(t, sha(newContent), gotSHA)

	// File on disk now has the new content.
	assert.Equal(t, newContent, readFile(t, dir, rel))

	// INDEX.md was rebuilt and reflects the new title.
	index := readFile(t, dir, indexBasename)
	assert.Contains(t, index, "Alpha v2")
	assert.Contains(t, index, "| File | Title |")

	// A new commit landed in the local repo.
	log := run(t, dir, "log", "--pretty=%s")
	assert.Contains(t, log, "memory: edit alpha.md")

	// No remote was ever configured: local-only invariant.
	remotes := run(t, dir, "remote")
	assert.Empty(t, strings.TrimSpace(remotes))
}

func TestWriteRejectsPathTraversal(t *testing.T) {
	dir, _ := fixtureRepo(t)
	w := NewWriter(dir)
	ctx := context.Background()

	// A sibling file the escape would target, to prove it is NOT written.
	outside := filepath.Join(filepath.Dir(dir), "escaped.md")

	cases := []string{
		"../escaped.md",
		"../../etc/passwd",
		"sub/../../escaped.md",
	}
	for _, rel := range cases {
		_, err := w.Write(ctx, WriteRequest{
			RelPath: rel, Content: "pwned", BaseSHA: "",
		})
		require.ErrorIs(t, err, ErrPathTraversal, "rel=%q", rel)
	}
	_, statErr := os.Stat(outside)
	assert.True(t, os.IsNotExist(statErr),
		"traversal must not create a file outside the memory dir")
}

func TestWriteConflictOnStaleBase(t *testing.T) {
	dir, rel := fixtureRepo(t)
	w := NewWriter(dir)
	ctx := context.Background()

	// Editor read this base, but the file changes on disk before write.
	staleBase := sha(readFile(t, dir, rel))
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, rel),
		[]byte("changed on disk out of band\n"), 0o644))

	_, err := w.Write(ctx, WriteRequest{
		RelPath: rel, Content: "my edit", BaseSHA: staleBase,
	})
	require.ErrorIs(t, err, ErrConflict)

	// The out-of-band content must be preserved, not clobbered.
	assert.Equal(t, "changed on disk out of band\n", readFile(t, dir, rel))
}

func TestWriteNewFileRejectsNonEmptyBase(t *testing.T) {
	dir, _ := fixtureRepo(t)
	w := NewWriter(dir)
	ctx := context.Background()

	// Claiming a base for a file that does not exist is a conflict.
	_, err := w.Write(ctx, WriteRequest{
		RelPath: "brand-new.md", Content: "x", BaseSHA: "deadbeef",
	})
	require.ErrorIs(t, err, ErrConflict)

	// Empty base creates the new note.
	sha2, err := w.Write(ctx, WriteRequest{
		RelPath: "brand-new.md",
		Content: "---\ntitle: New\ndate: 2026-06-25\n" +
			"problem_type: knowledge\n---\n\nbody\n",
		BaseSHA: "",
	})
	require.NoError(t, err)
	assert.NotEmpty(t, sha2)
	assert.FileExists(t, filepath.Join(dir, "brand-new.md"))
}

func TestHistoryListsCommits(t *testing.T) {
	dir, rel := fixtureRepo(t)
	w := NewWriter(dir)
	ctx := context.Background()

	// Two writes => two more commits on top of the initial one.
	base := sha(readFile(t, dir, rel))
	c1 := "---\ntitle: Alpha\ndate: 2026-06-20\n" +
		"problem_type: knowledge\n---\n\nbody v1\n"
	sha1, err := w.Write(ctx, WriteRequest{
		RelPath: rel, Content: c1, BaseSHA: base})
	require.NoError(t, err)

	c2 := "---\ntitle: Alpha\ndate: 2026-06-20\n" +
		"problem_type: knowledge\n---\n\nbody v2\n"
	_, err = w.Write(ctx, WriteRequest{
		RelPath: rel, Content: c2, BaseSHA: sha1})
	require.NoError(t, err)

	hist, err := w.History(ctx, rel)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(hist), 3,
		"initial + two edits should appear in history")
	for _, h := range hist {
		assert.NotEmpty(t, h.Commit)
		assert.NotEmpty(t, h.Date)
	}
	// Newest first.
	assert.Equal(t, "memory: edit alpha.md", hist[0].Message)
}

func TestHistoryNonRepoReturnsEmpty(t *testing.T) {
	dir := t.TempDir() // plain dir, no git init
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "note.md"), []byte("x"), 0o644))
	w := NewWriter(dir)

	hist, err := w.History(context.Background(), "note.md")
	require.NoError(t, err)
	assert.Empty(t, hist)
}

func TestWriteNonRepoFailSoft(t *testing.T) {
	dir := t.TempDir() // plain dir, no git init
	rel := "note.md"
	orig := "---\ntitle: Note\ndate: 2026-06-25\n" +
		"problem_type: knowledge\n---\n\nbody\n"
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, rel), []byte(orig), 0o644))
	w := NewWriter(dir)

	base := sha(orig)
	newContent := orig + "\nmore\n"
	_, err := w.Write(context.Background(), WriteRequest{
		RelPath: rel, Content: newContent, BaseSHA: base,
	})
	// Non-repo must not error: file write + INDEX still happen.
	require.NoError(t, err)
	assert.Equal(t, newContent, readFile(t, dir, rel))
	assert.FileExists(t, filepath.Join(dir, indexBasename))
	// Still not a git repo afterwards: we never git init.
	assert.NoDirExists(t, filepath.Join(dir, ".git"))
}

func TestRevertRestoresOldContent(t *testing.T) {
	dir, rel := fixtureRepo(t)
	w := NewWriter(dir)
	ctx := context.Background()

	original := readFile(t, dir, rel)
	base := sha(original)

	// Edit the note.
	edited := "---\ntitle: Alpha edited\ndate: 2026-06-22\n" +
		"problem_type: knowledge\n---\n\nedited body\n"
	editedSHA, err := w.Write(ctx, WriteRequest{
		RelPath: rel, Content: edited, BaseSHA: base})
	require.NoError(t, err)
	require.Equal(t, edited, readFile(t, dir, rel))

	// Find the initial commit (oldest) to revert to.
	hist, err := w.History(ctx, rel)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(hist), 2)
	initialCommit := hist[len(hist)-1].Commit

	// Sanity: FileAtCommit returns the original content.
	atInitial, err := w.FileAtCommit(ctx, rel, initialCommit)
	require.NoError(t, err)
	assert.Equal(t, original, atInitial)

	// Revert, passing the current (edited) sha as the concurrency base.
	revSHA, err := w.Revert(ctx, rel, initialCommit, editedSHA)
	require.NoError(t, err)
	assert.Equal(t, sha(original), revSHA)
	assert.Equal(t, original, readFile(t, dir, rel))

	// A revert commit landed.
	log := run(t, dir, "log", "--pretty=%s")
	assert.Contains(t, log, "memory: revert alpha.md")
}

func TestRevertConflictOnStaleBase(t *testing.T) {
	dir, rel := fixtureRepo(t)
	w := NewWriter(dir)
	ctx := context.Background()

	hist, err := w.History(ctx, rel)
	require.NoError(t, err)
	require.NotEmpty(t, hist)
	commit := hist[len(hist)-1].Commit

	_, err = w.Revert(ctx, rel, commit, "stale-sha-does-not-match")
	require.ErrorIs(t, err, ErrConflict)
}

func TestFileAtCommitInvalidRef(t *testing.T) {
	dir, rel := fixtureRepo(t)
	w := NewWriter(dir)
	_, err := w.FileAtCommit(context.Background(), rel, "--output=/etc/x")
	require.Error(t, err)
}

func TestPythonRootFor(t *testing.T) {
	root, ok := pythonRootFor("/home/u/.dotfiles/memory/user")
	require.True(t, ok)
	assert.Equal(t, "/home/u/.dotfiles", root)

	_, ok = pythonRootFor("/tmp/some/fixture")
	assert.False(t, ok)
}

// ── CC-native (no-git, multi-root) write-back ─────────────────────────────

// ccFixture creates a temp CC-native root that mirrors the on-disk layout the
// CC syncer scans: <root>/<project>/memory/<file>.md across projects. It returns
// the root and one note's rel_path (relative to the root, spanning subdirs).
// It is a plain dir, NOT a git repo, matching the real ~/.claude/projects.
func ccFixture(t *testing.T) (root, relPath string) {
	t.Helper()
	root = t.TempDir()
	relPath = filepath.Join("proj-a", "memory", "note.md")
	full := filepath.Join(root, relPath)
	require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
	require.NoError(t, os.WriteFile(
		full, []byte("# CC note\n\noriginal cc body.\n"), 0o644))
	// A second project so the root genuinely spans multiple project dirs.
	other := filepath.Join(root, "proj-b", "memory", "other.md")
	require.NoError(t, os.MkdirAll(filepath.Dir(other), 0o755))
	require.NoError(t, os.WriteFile(other, []byte("# other\n"), 0o644))
	return root, relPath
}

func TestCCWriteNoGitHappyPath(t *testing.T) {
	root, rel := ccFixture(t)
	w := NewWriterNoGit(root)
	ctx := context.Background()

	base := sha(readFile(t, root, rel))
	newContent := "# CC note v2\n\nupdated cc body.\n"
	gotSHA, err := w.Write(ctx, WriteRequest{
		RelPath: rel, Content: newContent, BaseSHA: base,
	})
	require.NoError(t, err)
	assert.Equal(t, sha(newContent), gotSHA)

	// The multi-root locator resolved <root>/proj-a/memory/note.md and the
	// new content landed there.
	assert.Equal(t, newContent, readFile(t, root, rel))

	// No-git invariant: a CC-native write never creates a git repo, never
	// writes an INDEX.md, and never commits.
	assert.NoDirExists(t, filepath.Join(root, ".git"))
	assert.NoFileExists(t, filepath.Join(root, indexBasename))
	assert.NoFileExists(t,
		filepath.Join(root, "proj-a", "memory", indexBasename))
}

func TestCCWriteConflictOnStaleBase(t *testing.T) {
	root, rel := ccFixture(t)
	w := NewWriterNoGit(root)
	ctx := context.Background()

	staleBase := sha(readFile(t, root, rel))
	// The file changes on disk after the editor read its base.
	require.NoError(t, os.WriteFile(
		filepath.Join(root, rel), []byte("changed out of band\n"), 0o644))

	_, err := w.Write(ctx, WriteRequest{
		RelPath: rel, Content: "my edit", BaseSHA: staleBase,
	})
	require.ErrorIs(t, err, ErrConflict)
	// Out-of-band content preserved, not clobbered.
	assert.Equal(t, "changed out of band\n", readFile(t, root, rel))
}

func TestCCWriteRejectsTraversalOutsideRoot(t *testing.T) {
	root, _ := ccFixture(t)
	w := NewWriterNoGit(root)
	ctx := context.Background()

	// A sibling file the escape would target, to prove it is NOT written.
	outside := filepath.Join(filepath.Dir(root), "escaped.md")

	cases := []string{
		"../escaped.md",
		"proj-a/../../escaped.md",
		"proj-a/memory/../../../escaped.md",
	}
	for _, rel := range cases {
		_, err := w.Write(ctx, WriteRequest{
			RelPath: rel, Content: "pwned", BaseSHA: "",
		})
		require.ErrorIs(t, err, ErrPathTraversal, "rel=%q", rel)
	}
	_, statErr := os.Stat(outside)
	assert.True(t, os.IsNotExist(statErr),
		"traversal must not create a file outside the CC root")
}

func TestCCWriteAbsolutePathConfinedToRoot(t *testing.T) {
	root, _ := ccFixture(t)
	w := NewWriterNoGit(root)
	// An absolute path is treated as relative components by filepath.Join, so
	// it is confined UNDER the root, never escaping it. The security property
	// that matters: nothing is written outside the root.
	abs := filepath.Join(t.TempDir(), "evil.md")
	_, err := w.Write(context.Background(), WriteRequest{
		RelPath: abs, Content: "confined", BaseSHA: "",
	})
	require.NoError(t, err)
	// The target absolute path itself was NOT written (would be an escape).
	_, statErr := os.Stat(abs)
	assert.True(t, os.IsNotExist(statErr),
		"absolute rel_path must not write to the literal absolute path")
}

// Regression: the cross-agent (git) writer still commits + rebuilds INDEX, so
// the no-git flag did not bleed into the default path.
func TestCrossAgentWriteStillCommits(t *testing.T) {
	dir, rel := fixtureRepo(t)
	w := NewWriter(dir)
	ctx := context.Background()

	base := sha(readFile(t, dir, rel))
	newContent := "---\ntitle: Alpha v2\ndate: 2026-06-21\n" +
		"problem_type: knowledge\n---\n\nbody.\n"
	_, err := w.Write(ctx, WriteRequest{
		RelPath: rel, Content: newContent, BaseSHA: base,
	})
	require.NoError(t, err)

	// INDEX rebuilt and a commit landed (the side effects the no-git path
	// suppresses must remain for the cross-agent root).
	assert.FileExists(t, filepath.Join(dir, indexBasename))
	log := run(t, dir, "log", "--pretty=%s")
	assert.Contains(t, log, "memory: edit alpha.md")
}
