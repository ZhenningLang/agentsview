package synthesize

import (
	"context"
	"os/exec"
	"path/filepath"
)

// compactScriptRel is compact_memory.py relative to the dotfiles root.
const compactScriptRel = "coding-skills/compact-memory/scripts/compact_memory.py"

// ScriptResult is the parsed outcome of one compact_memory.py invocation.
type ScriptResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

// ScriptRunner executes compact_memory.py for one decision file. Tests pass a
// fake so no real process runs.
type ScriptRunner interface {
	Run(ctx context.Context, root, decisionFile string) (ScriptResult, error)
}

// PythonScriptRunner shells out to `python3 <root>/compact_memory.py --root
// <root> --decision-file <f>`, with cwd pinned to root so relative imports and
// memory/user resolve correctly.
type PythonScriptRunner struct {
	Python string // empty -> python3
}

func (r PythonScriptRunner) Run(ctx context.Context, root, decisionFile string) (ScriptResult, error) {
	python := r.Python
	if python == "" {
		python = "python3"
	}
	cmd := exec.CommandContext(ctx, python, filepath.Join(root, compactScriptRel),
		"--root", root, "--decision-file", decisionFile)
	cmd.Dir = root
	var res ScriptResult
	out, err := cmd.Output()
	res.Stdout = string(out)
	if exitErr, ok := err.(*exec.ExitError); ok {
		res.Stderr = string(exitErr.Stderr)
		res.ExitCode = exitErr.ExitCode()
		return res, nil // non-zero exit is a per-decision rejection, not fatal
	}
	if err != nil {
		return res, err
	}
	return res, nil
}
