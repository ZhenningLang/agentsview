package ghconnect

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
)

// CLIRunner is the production Runner: it shells out to the locally installed,
// already-authenticated `gh` CLI. It is intentionally thin so the state machine
// in ghconnect.go is fully unit-testable against a mock Runner.
type CLIRunner struct {
	// Bin overrides the gh executable name/path. Empty defaults to "gh".
	Bin string
}

// Run executes `gh <args...>`, returning combined stdout and the process exit
// code. A non-zero exit (gh's signal for 404 / not-found / not-authenticated)
// is NOT an error: it is reported via exitCode, mirroring the state machine's
// expectations. Only a genuine spawn failure returns a non-nil error.
func (r CLIRunner) Run(ctx context.Context, args ...string) (string, int, error) {
	bin := r.Bin
	if bin == "" {
		bin = "gh"
	}
	cmd := exec.CommandContext(ctx, bin, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			// Expected: gh signals 404/not-found/not-authenticated via a
			// non-zero exit; surface it without treating it as a spawn error.
			return stdout.String(), exitErr.ExitCode(), nil
		}
		// gh missing / not executable: a real spawn failure.
		return stdout.String(), -1, err
	}
	return stdout.String(), 0, nil
}
