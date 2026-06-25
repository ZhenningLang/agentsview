package backup

import (
	"context"
	"strings"
	"testing"
)

// TestRunCommand_NonZeroSurfacesStderr proves that on a non-zero exit the
// command's stderr (where git/gh write failure diagnostics) is surfaced in the
// returned output, not silently dropped. This is the fail-soft reason the UI's
// last_error must carry.
func TestRunCommand_NonZeroSurfacesStderr(t *testing.T) {
	// Emulate git writing a diagnostic to stderr and exiting non-zero.
	out, code, err := runCommand(context.Background(), "sh", "-c", "echo 'Permission denied (publickey)' 1>&2; exit 128")
	if err != nil {
		t.Fatalf("unexpected spawn error: %v", err)
	}
	if code != 128 {
		t.Fatalf("expected exit code 128, got %d", code)
	}
	if !strings.Contains(out, "Permission denied (publickey)") {
		t.Fatalf("expected stderr diagnostic surfaced, got %q", out)
	}
}

// TestRunCommand_NonZeroCombinesBothStreams proves stdout and stderr are both
// preserved on the error path.
func TestRunCommand_NonZeroCombinesBothStreams(t *testing.T) {
	out, code, err := runCommand(context.Background(), "sh", "-c", "echo out; echo err 1>&2; exit 1")
	if err != nil {
		t.Fatalf("unexpected spawn error: %v", err)
	}
	if code != 1 {
		t.Fatalf("expected exit code 1, got %d", code)
	}
	if !strings.Contains(out, "out") || !strings.Contains(out, "err") {
		t.Fatalf("expected both stdout and stderr in output, got %q", out)
	}
}

// TestRunCommand_SuccessReturnsStdoutOnly proves the success path returns only
// stdout, so callers parsing command output (e.g. `remote get-url`) are not
// polluted by anything written to stderr.
func TestRunCommand_SuccessReturnsStdoutOnly(t *testing.T) {
	out, code, err := runCommand(context.Background(), "sh", "-c", "echo theURL; echo noise 1>&2")
	if err != nil {
		t.Fatalf("unexpected spawn error: %v", err)
	}
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if strings.TrimSpace(out) != "theURL" {
		t.Fatalf("expected stdout-only %q, got %q", "theURL", strings.TrimSpace(out))
	}
}

func TestCombineOutput(t *testing.T) {
	cases := []struct {
		stdout, stderr, want string
	}{
		{"a", "b", "a\nb"},
		{"a", "", "a"},
		{"", "b", "b"},
		{"  ", "  ", ""},
		{" a \n", " b \n", "a\nb"},
	}
	for _, c := range cases {
		if got := combineOutput(c.stdout, c.stderr); got != c.want {
			t.Errorf("combineOutput(%q,%q) = %q, want %q", c.stdout, c.stderr, got, c.want)
		}
	}
}
