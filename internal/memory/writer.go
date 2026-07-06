package memory

// writer.go adds edit-write-back and git history to the user-memory SSOT.
// The memory SSOT is the on-disk *.md files, not the DB: this package only
// performs file operations (and exec of git / the index builder). The DB is
// a read-only cache that the syncer refreshes after a write lands on disk;
// nothing here touches the DB. It deliberately does not depend on db.Store.
//
// Safety posture:
//   - Path-traversal: a caller-supplied rel_path is joined onto the memory
//     dir, Clean'd, and rejected unless it stays inside the memory dir.
//   - Concurrency: writes are gated on a base sha256 of the current file so a
//     stale editor cannot clobber an on-disk change (409 semantics).
//   - Atomicity: writes go to a temp file in the same dir then os.Rename.
//   - Index: INDEX.md is rebuilt after every write (python builder, else a
//     Go fallback that reproduces its table format).
//   - Local git: the memory dir is committed as a local-only repo. It is
//     NEVER pushed. Non-repo dirs fail soft (file is still written).

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// ErrConflict is returned when the on-disk file no longer matches the
// base sha256 the caller read from, i.e. it was modified out from under the
// editor. Callers map this to HTTP 409.
var ErrConflict = errors.New("modified on disk, reload")

// ErrPathTraversal is returned when a rel_path resolves outside the memory
// directory. Callers map this to HTTP 400.
var ErrPathTraversal = errors.New("memory path escapes memory dir")

// HistoryEntry is one git commit touching a single memory file.
type HistoryEntry struct {
	Commit  string `json:"commit"`
	Date    string `json:"date"`
	Message string `json:"message"`
}

// FileWriter performs file-level edits against a single memory root. It is
// constructed per request from the resolved root; it holds no DB. (Named
// FileWriter to avoid colliding with the syncer's Writer persistence interface
// in this package.)
//
// Two roots are supported, selected by constructor:
//   - The cross-agent SSOT (~/.dotfiles/memory/user): NewWriter. The root
//     directly contains the *.md notes; writes rebuild INDEX.md and commit the
//     local-only git repo, and git history is available.
//   - CC-native auto-memory (~/.claude/projects): NewWriterNoGit. The root is
//     the project-dirs parent, so a note's RelPath spans subdirs
//     (<project>/memory/<file>.md). CC-native dirs are not our git repo and
//     have no INDEX, so writes are content-only: noGit skips both the INDEX
//     rebuild and the commit. History is "not applicable" for this root.
//
// The path-traversal guard is identical for both: a note's resolved absolute
// path must stay inside the root, so a CC-native RelPath cannot escape the
// projects parent even though it legitimately contains slashes.
type FileWriter struct {
	dir string
	// noGit, when true, makes Write content-only: no INDEX rebuild and no git
	// commit. It is set for the CC-native root, which is neither our git repo
	// nor INDEX-managed.
	noGit bool
}

// NewWriter builds a FileWriter rooted at the cross-agent memory directory
// (the directory that directly contains the *.md notes, e.g.
// ~/.dotfiles/memory/user). Writes rebuild INDEX.md and commit the local repo.
func NewWriter(dir string) *FileWriter {
	return &FileWriter{dir: dir}
}

// NewWriterNoGit builds a FileWriter rooted at a directory whose writes are
// content-only: no INDEX rebuild, no git commit, no history. It is used for the
// CC-native root (~/.claude/projects), where notes live across project subdirs
// and the directory is not managed as our git repo.
func NewWriterNoGit(dir string) *FileWriter {
	return &FileWriter{dir: dir, noGit: true}
}

// WriteRequest is the payload for a write-back: the target rel_path, the
// reconstructed full file content, and the base sha256 the editor read.
type WriteRequest struct {
	// RelPath is the note path relative to the memory dir (e.g. "alpha.md").
	RelPath string
	// Content is the full new file content (frontmatter + body already
	// assembled by the caller). It is written verbatim.
	Content string
	// BaseSHA is the sha256 (hex) of the whole file content the editor read.
	// Empty means "new file" and is only accepted when the target does not
	// yet exist.
	BaseSHA string
}

// resolvePath validates relPath against the memory dir and returns the
// cleaned absolute path. It rejects any path that, after cleaning, escapes
// the memory dir, and any path containing a ".." segment.
func (w *FileWriter) resolvePath(relPath string) (string, error) {
	if relPath == "" {
		return "", ErrPathTraversal
	}
	// Reject explicit parent traversal segments outright. filepath.Clean
	// would collapse some of these, but an explicit reject is clearer and
	// closes off encodings the Join/Clean check might otherwise normalize.
	for _, seg := range strings.Split(filepath.ToSlash(relPath), "/") {
		if seg == ".." {
			return "", ErrPathTraversal
		}
	}
	cleanDir := filepath.Clean(w.dir)
	full := filepath.Clean(filepath.Join(cleanDir, relPath))
	// full must be inside cleanDir. Compare with a trailing separator so a
	// sibling dir sharing a prefix (memory-evil vs memory) cannot pass.
	if full != cleanDir &&
		!strings.HasPrefix(full, cleanDir+string(os.PathSeparator)) {
		return "", ErrPathTraversal
	}
	// The memory dir itself is not a writable note target.
	if full == cleanDir {
		return "", ErrPathTraversal
	}
	return full, nil
}

// sha256Hex returns the lowercase hex sha256 of b.
func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// Read returns the verbatim on-disk content of a memory note plus the
// sha256 the editor must echo back as base_sha on the next write. The DB
// mirror drops untracked frontmatter keys and normalizes the body, so the
// editor reads the raw file (the SSOT) to round-trip edits losslessly and
// to obtain a base_sha that actually matches what Write compares against.
func (w *FileWriter) Read(_ context.Context, relPath string) (string, string, error) {
	full, err := w.resolvePath(relPath)
	if err != nil {
		return "", "", err
	}
	data, err := os.ReadFile(full)
	if err != nil {
		return "", "", err
	}
	return string(data), sha256Hex(data), nil
}

// Write performs the safe write-back path: path validation, optimistic-
// concurrency check against BaseSHA, atomic write, INDEX rebuild, and a
// local-only git commit. It returns the new file's sha256 on success.
func (w *FileWriter) Write(ctx context.Context, req WriteRequest) (string, error) {
	full, err := w.resolvePath(req.RelPath)
	if err != nil {
		return "", err
	}

	// Concurrency: compare the on-disk content against the editor's base.
	existing, readErr := os.ReadFile(full)
	switch {
	case readErr == nil:
		if sha256Hex(existing) != req.BaseSHA {
			return "", ErrConflict
		}
	case errors.Is(readErr, os.ErrNotExist):
		// New file: only allowed when the caller did not claim a base.
		if req.BaseSHA != "" {
			return "", ErrConflict
		}
	default:
		return "", fmt.Errorf("reading current memory file: %w", readErr)
	}

	if err := w.atomicWrite(full, []byte(req.Content)); err != nil {
		return "", err
	}

	// CC-native (noGit) writes are content-only: the root is not our git repo
	// and has no INDEX, so neither side effect applies. The atomic write above
	// is the entire write-back for that root.
	if !w.noGit {
		w.rebuildIndex(ctx)
		w.commit(ctx, fmt.Sprintf("memory: edit %s", req.RelPath))
	}

	return sha256Hex([]byte(req.Content)), nil
}

func (w *FileWriter) Delete(ctx context.Context, relPath, baseSHA string) error {
	full, err := w.resolvePath(relPath)
	if err != nil {
		return err
	}
	existing, readErr := os.ReadFile(full)
	if readErr != nil {
		return fmt.Errorf("reading current memory file: %w", readErr)
	}
	if baseSHA != "" && sha256Hex(existing) != baseSHA {
		return ErrConflict
	}
	if err := os.Remove(full); err != nil {
		return err
	}
	if !w.noGit {
		w.rebuildIndex(ctx)
		w.commit(ctx, fmt.Sprintf("memory: delete %s", relPath))
	}
	return nil
}

// atomicWrite writes data to a temp file in the same directory and renames
// it over dst, so a reader never observes a partial file.
func (w *FileWriter) atomicWrite(dst string, data []byte) error {
	dir := filepath.Dir(dst)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating memory dir: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".memory-write-*.tmp")
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("writing temp file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("syncing temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("closing temp file: %w", err)
	}
	if err := os.Rename(tmpName, dst); err != nil {
		return fmt.Errorf("renaming temp file: %w", err)
	}
	return nil
}

// rebuildIndex regenerates INDEX.md. It prefers the python builder
// (scripts/build_memory_index.py) which expects a --root whose
// <root>/memory/user is the memory dir; when the layout does not match or
// the script is unavailable it falls back to a Go reimplementation of the
// same table format. Failures are fail-soft: a stale INDEX never blocks the
// write that already landed.
func (w *FileWriter) rebuildIndex(ctx context.Context) {
	if root, ok := pythonRootFor(w.dir); ok {
		script := filepath.Join(root, "scripts", "build_memory_index.py")
		if _, err := os.Stat(script); err == nil {
			cmd := exec.CommandContext(
				ctx, "python3", script, "--root", root)
			if err := cmd.Run(); err == nil {
				return
			}
			// Fall through to the Go builder on any python failure.
		}
	}
	_ = w.rebuildIndexGo()
}

// pythonRootFor returns the --root the python builder expects, when the
// memory dir matches the builder's hard-coded <root>/memory/user layout.
func pythonRootFor(memoryDir string) (string, bool) {
	clean := filepath.Clean(memoryDir)
	if filepath.Base(clean) != "user" {
		return "", false
	}
	parent := filepath.Dir(clean)
	if filepath.Base(parent) != "memory" {
		return "", false
	}
	return filepath.Dir(parent), true
}

// rebuildIndexGo regenerates INDEX.md in the same format the python builder
// emits, scanning the *.md notes in the memory dir. It is a fallback for
// fixtures and non-standard layouts and never errors fatally for the caller.
func (w *FileWriter) rebuildIndexGo() error {
	entries, err := os.ReadDir(w.dir)
	if err != nil {
		return err
	}
	type row struct {
		file, title, problemType, status, keywords, origin string
	}
	var rows []row
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".md") ||
			name == indexBasename {
			continue
		}
		data, rerr := os.ReadFile(filepath.Join(w.dir, name))
		if rerr != nil {
			continue
		}
		fm := parseIndexFrontmatter(string(data))
		keywords := fm["keywords"]
		if keywords == "" {
			keywords = fm["tags"]
		}
		status := fm["status"]
		if status == "" {
			status = "active"
		}
		// Parity with the python builder: the INDEX is the active-recall index;
		// stale/archived notes are excluded (recall skips them and the DB reads
		// the files directly), so listing them only bloats the index.
		if status != "active" {
			continue
		}
		rows = append(rows, row{
			file:        name,
			title:       fm["title"],
			problemType: fm["problem_type"],
			status:      status,
			keywords:    keywords,
			origin:      fm["origin_session"],
		})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].file < rows[j].file })

	var b strings.Builder
	b.WriteString("# Memory Index\n\n")
	b.WriteString("> Generated by `scripts/build_memory_index.py`; " +
		"do not edit by hand.\n\n")
	b.WriteString(
		"| File | Title | Problem Type | Status | Keywords | Origin |\n")
	b.WriteString("|---|---|---|---|---|---|\n")
	for _, r := range rows {
		b.WriteString(fmt.Sprintf("| %s | %s | %s | %s | %s | %s |\n",
			indexCell(r.file), indexCell(r.title), indexCell(r.problemType),
			indexCell(r.status), indexCell(r.keywords), indexCell(r.origin)))
	}
	out := strings.TrimRight(b.String(), "\n") + "\n"
	return os.WriteFile(
		filepath.Join(w.dir, indexBasename), []byte(out), 0o644)
}

// indexCell escapes a value for a markdown table cell, matching the python
// builder's table_cell.
func indexCell(v string) string {
	v = strings.ReplaceAll(v, "|", `\|`)
	return strings.ReplaceAll(v, "\n", " ")
}

// parseIndexFrontmatter extracts simple key: value frontmatter pairs used by
// the index, mirroring the python builder's normalize_value for [a, b] lists.
func parseIndexFrontmatter(content string) map[string]string {
	out := map[string]string{}
	block := extractFrontmatterBlock(content)
	if block == "" {
		return out
	}
	sc := bufio.NewScanner(strings.NewReader(block))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		v = strings.Trim(strings.TrimSpace(v), `"'`)
		if strings.HasPrefix(v, "[") && strings.HasSuffix(v, "]") {
			inner := strings.TrimSuffix(strings.TrimPrefix(v, "["), "]")
			var items []string
			for _, it := range strings.Split(inner, ",") {
				it = strings.Trim(strings.TrimSpace(it), `"'`)
				if it != "" {
					items = append(items, it)
				}
			}
			v = strings.Join(items, ", ")
		}
		out[k] = v
	}
	return out
}

// isGitRepo reports whether dir is inside a git work tree.
func (w *FileWriter) isGitRepo(ctx context.Context) bool {
	cmd := exec.CommandContext(
		ctx, "git", "-C", w.dir, "rev-parse", "--is-inside-work-tree")
	out, err := cmd.Output()
	return err == nil && strings.TrimSpace(string(out)) == "true"
}

// commit stages and commits the whole memory dir to its local-only repo.
// It is fail-soft: a non-repo dir is left as a plain file write (never
// git-init'd) and any git failure is swallowed. It NEVER pushes.
func (w *FileWriter) commit(ctx context.Context, message string) {
	if !w.isGitRepo(ctx) {
		return
	}
	add := exec.CommandContext(ctx, "git", "-C", w.dir, "add", "-A")
	if err := add.Run(); err != nil {
		return
	}
	commit := exec.CommandContext(
		ctx, "git", "-C", w.dir, "commit", "-m", message)
	_ = commit.Run() // nothing-to-commit etc. is fine.
}

// History returns the git log for a single memory file, newest first. A
// non-repo dir yields an empty list with no error.
func (w *FileWriter) History(
	ctx context.Context, relPath string,
) ([]HistoryEntry, error) {
	full, err := w.resolvePath(relPath)
	if err != nil {
		return nil, err
	}
	if !w.isGitRepo(ctx) {
		return []HistoryEntry{}, nil
	}
	// %x1f unit separator between fields, %x1e record separator between
	// commits, so messages containing newlines or pipes stay intact.
	const format = "%H%x1f%cI%x1f%s%x1e"
	cmd := exec.CommandContext(ctx, "git", "-C", w.dir, "log",
		"--pretty=format:"+format, "--", full)
	out, err := cmd.Output()
	if err != nil {
		// File never committed, or other git error: empty, not fatal.
		return []HistoryEntry{}, nil
	}
	entries := []HistoryEntry{}
	for _, rec := range strings.Split(string(out), "\x1e") {
		rec = strings.Trim(rec, "\n")
		if rec == "" {
			continue
		}
		fields := strings.Split(rec, "\x1f")
		if len(fields) < 3 {
			continue
		}
		entries = append(entries, HistoryEntry{
			Commit:  fields[0],
			Date:    fields[1],
			Message: fields[2],
		})
	}
	return entries, nil
}

// FileAtCommit returns the file's content at a specific commit (git show
// <commit>:<relpath>). The relPath is validated, and the commit ref is
// validated to be a plausible git object name to keep it out of arg
// injection territory.
func (w *FileWriter) FileAtCommit(
	ctx context.Context, relPath, commit string,
) (string, error) {
	if _, err := w.resolvePath(relPath); err != nil {
		return "", err
	}
	if !validCommitRef(commit) {
		return "", fmt.Errorf("invalid commit ref")
	}
	if !w.isGitRepo(ctx) {
		return "", fmt.Errorf("not a git repo")
	}
	// git show takes the path relative to the repo root; w.dir is the repo
	// root for the memory repo, so the rel_path is correct as-is.
	spec := commit + ":" + filepath.ToSlash(filepath.Clean(relPath))
	cmd := exec.CommandContext(ctx, "git", "-C", w.dir, "show", spec)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git show: %s",
			strings.TrimSpace(stderr.String()))
	}
	return stdout.String(), nil
}

// Revert writes the file's content at the given commit back to the current
// file, going through the same safe write path (atomic write, INDEX rebuild,
// commit). The caller-supplied baseSHA still guards against a concurrent
// on-disk change between the user viewing and reverting.
func (w *FileWriter) Revert(
	ctx context.Context, relPath, commit, baseSHA string,
) (string, error) {
	content, err := w.FileAtCommit(ctx, relPath, commit)
	if err != nil {
		return "", err
	}
	full, err := w.resolvePath(relPath)
	if err != nil {
		return "", err
	}
	// Concurrency gate identical to Write.
	existing, readErr := os.ReadFile(full)
	switch {
	case readErr == nil:
		if sha256Hex(existing) != baseSHA {
			return "", ErrConflict
		}
	case errors.Is(readErr, os.ErrNotExist):
		if baseSHA != "" {
			return "", ErrConflict
		}
	default:
		return "", fmt.Errorf("reading current memory file: %w", readErr)
	}
	if err := w.atomicWrite(full, []byte(content)); err != nil {
		return "", err
	}
	w.rebuildIndex(ctx)
	w.commit(ctx, fmt.Sprintf("memory: revert %s to %s", relPath, commit))
	return sha256Hex([]byte(content)), nil
}

// validCommitRef accepts hex object names and a small set of safe ref
// characters. It rejects anything that could be an option (leading '-') or
// shell-ish input, even though exec.Command does not use a shell.
func validCommitRef(ref string) bool {
	if ref == "" || strings.HasPrefix(ref, "-") {
		return false
	}
	for _, r := range ref {
		switch {
		case r >= 'a' && r <= 'z',
			r >= 'A' && r <= 'Z',
			r >= '0' && r <= '9',
			r == '_' || r == '-' || r == '/' || r == '.' || r == '~' ||
				r == '^':
			continue
		default:
			return false
		}
	}
	return true
}
