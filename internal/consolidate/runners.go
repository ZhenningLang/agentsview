package consolidate

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"path/filepath"
)

// scriptRelPath is the path to assist_consolidate.py relative to the dotfiles
// root. The script owns every safety gate; the worker only invokes it.
const scriptRelPath = "coding-skills/assist-learn/scripts/assist_consolidate.py"

// rawDirRel is the staging raw dir relative to the dotfiles root, passed to the
// script as --raw-dir (the script resolves it against --root).
const rawDirRel = "memory/.staging/raw_memories"

// PythonScriptRunner runs assist_consolidate.py via python3. It is the
// production ScriptRunner; tests use a fake to avoid spawning a process.
type PythonScriptRunner struct {
	// Python is the interpreter to use. Empty defaults to "python3".
	Python string
}

// Run invokes `python3 <root>/<script> --root <root> --raw-dir <rawDirRel>
// --decision-file <decisionFile>`. A non-zero exit is NOT returned as an error
// (the script exits non-zero when it rejects a candidate, which is expected);
// it is surfaced in ScriptResult.ExitCode. A genuine spawn failure (missing
// python, missing script) is returned as an error.
func (r PythonScriptRunner) Run(
	ctx context.Context, root, rawDir, decisionFile string,
) (ScriptResult, error) {
	python := r.Python
	if python == "" {
		python = "python3"
	}
	script := filepath.Join(root, scriptRelPath)
	cmd := exec.CommandContext(ctx, python, script,
		"--root", root,
		"--raw-dir", rawDirRel,
		"--decision-file", decisionFile,
	)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	res := ScriptResult{
		Stdout: stdout.String(),
		Stderr: stderr.String(),
	}
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			// Expected: the script signals rejections via a non-zero exit.
			res.ExitCode = exitErr.ExitCode()
			return res, nil
		}
		// Spawn failure (python/script missing): a real error.
		return res, err
	}
	return res, nil
}

// GitCommitter commits a directory as a local-only git repo. It NEVER pushes.
// It is a thin wrapper so the worker stays decoupled from exec for tests.
type GitCommitter struct {
	Dir string
}

// Commit stages everything under the dir and commits with the given message.
// A repo with nothing to commit (git commit exits non-zero) is swallowed: an
// empty cycle is not an error. It never pushes.
func (g GitCommitter) Commit(ctx context.Context, message string) error {
	if g.Dir == "" {
		return nil
	}
	add := exec.CommandContext(ctx, "git", "-C", g.Dir, "add", "-A")
	if err := add.Run(); err != nil {
		// Not a git repo or add failed: nothing we can safely commit.
		return nil
	}
	commit := exec.CommandContext(ctx, "git", "-C", g.Dir, "commit", "-m", message)
	_ = commit.Run()
	return nil
}

// ResyncFunc adapts a plain function to the Resyncer interface so the wiring in
// main.go can pass the existing memory syncer's Sync without a named type.
type ResyncFunc func(ctx context.Context) error

// Resync calls the wrapped function.
func (f ResyncFunc) Resync(ctx context.Context) error {
	if f == nil {
		return nil
	}
	return f(ctx)
}
