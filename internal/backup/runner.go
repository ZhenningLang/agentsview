package backup

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"strings"
)

// GitRunner abstracts a single `git` invocation. The production implementation
// (CLIGitRunner) shells out to the installed git; tests pass a mock so a backup
// cycle never spawns a process or touches a real repo (Phase 05 hard rule:
// git/gh MOCKED, no real remote actions). Run reports a non-zero exit via
// exitCode (not err); err is reserved for genuine spawn failures.
type GitRunner interface {
	Run(ctx context.Context, args ...string) (stdout string, exitCode int, err error)
}

// GHRunner abstracts a single `gh` invocation, used only for the pre-push
// `gh auth status` health check. It mirrors ghconnect.Runner so the same mocks
// work, but is declared here to keep the backup package self-contained.
type GHRunner interface {
	Run(ctx context.Context, args ...string) (stdout string, exitCode int, err error)
}

// CLIGitRunner is the production GitRunner: it shells out to the local `git`.
// A non-zero exit is reported via exitCode, not as an error, so callers can
// distinguish "nothing to commit" (exit 1) from a spawn failure.
type CLIGitRunner struct {
	// Bin overrides the git executable name/path. Empty defaults to "git".
	Bin string
}

// Run executes `git <args...>`, returning combined stdout and the process exit
// code. Only a genuine spawn failure returns a non-nil error.
func (r CLIGitRunner) Run(ctx context.Context, args ...string) (string, int, error) {
	return runCommand(ctx, orDefault(r.Bin, "git"), args...)
}

// CLIGHRunner is the production GHRunner: it shells out to the local `gh`.
type CLIGHRunner struct {
	// Bin overrides the gh executable name/path. Empty defaults to "gh".
	Bin string
}

// Run executes `gh <args...>`, returning combined stdout and the process exit
// code. A non-zero exit (gh's signal for not-authenticated) is reported via
// exitCode; only a spawn failure returns a non-nil error.
func (r CLIGHRunner) Run(ctx context.Context, args ...string) (string, int, error) {
	return runCommand(ctx, orDefault(r.Bin, "gh"), args...)
}

func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

// runCommand runs bin with args and normalizes the exec result: a non-zero exit
// is surfaced via exitCode (err nil); only a spawn failure returns err.
//
// On a non-zero exit the returned output is stdout AND stderr combined, because
// git/gh write almost all failure diagnostics (e.g. "Permission denied
// (publickey)", "repository not found", "remote rejected") to stderr; callers
// record that string as the user-facing reason, so dropping stderr would leave a
// generic, unactionable error. On success only stdout is returned so callers
// that parse command output (e.g. `remote get-url`) are not polluted by stderr.
func runCommand(ctx context.Context, bin string, args ...string) (string, int, error) {
	cmd := exec.CommandContext(ctx, bin, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return combineOutput(stdout.String(), stderr.String()), exitErr.ExitCode(), nil
		}
		return combineOutput(stdout.String(), stderr.String()), -1, err
	}
	return stdout.String(), 0, nil
}

// combineOutput joins stdout and stderr for an error path, trimming each and
// dropping an empty half so the result is a clean, non-empty diagnostic when at
// least one stream carried text.
func combineOutput(stdout, stderr string) string {
	parts := make([]string, 0, 2)
	if s := strings.TrimSpace(stdout); s != "" {
		parts = append(parts, s)
	}
	if s := strings.TrimSpace(stderr); s != "" {
		parts = append(parts, s)
	}
	return strings.Join(parts, "\n")
}
