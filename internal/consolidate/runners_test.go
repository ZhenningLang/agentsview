package consolidate

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPythonScriptRunner_NonZeroExitNotError verifies the exec wrapper treats a
// non-zero script exit as a recorded ScriptResult (rejections are expected),
// not a Go error. It uses a tiny fake "python3" on PATH instead of the real
// interpreter so no real consolidation runs. gh/git/python are all mocked.
func TestPythonScriptRunner_NonZeroExitNotError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake shell python not supported on windows")
	}
	binDir := t.TempDir()
	fakePython := filepath.Join(binDir, "fakepython")
	// Print a result line and a stderr error, then exit 1 (a rejection).
	script := "#!/bin/sh\n" +
		"echo 'skip c1 anti_self_poisoning:negative_tool_claim'\n" +
		"echo 'assist_consolidate failed: redact gate rejected candidate c2' 1>&2\n" +
		"exit 1\n"
	if err := os.WriteFile(fakePython, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	r := PythonScriptRunner{Python: fakePython}
	res, err := r.Run(context.Background(), t.TempDir(), "raw", "/tmp/decision.json")
	if err != nil {
		t.Fatalf("non-zero exit must not be a Go error: %v", err)
	}
	if res.ExitCode != 1 {
		t.Errorf("ExitCode = %d, want 1", res.ExitCode)
	}
	if res.Stdout == "" || res.Stderr == "" {
		t.Errorf("stdout/stderr should be captured: %+v", res)
	}
}

func TestPythonScriptRunner_RunSetsChildWorkingDirectoryToRoot(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake shell python not supported on windows")
	}

	binDir := t.TempDir()
	root := t.TempDir()
	recordPath := filepath.Join(t.TempDir(), "cwd.txt")
	fakePython := filepath.Join(binDir, "fakepython")
	script := "#!/bin/sh\n" +
		"pwd > " + shellQuote(recordPath) + "\n" +
		"exit 0\n"
	require.NoError(t, os.WriteFile(fakePython, []byte(script), 0o755))

	r := PythonScriptRunner{Python: fakePython}
	res, err := r.Run(context.Background(), root, "raw", filepath.Join(t.TempDir(), "decision.json"))

	require.NoError(t, err)
	assert.Equal(t, 0, res.ExitCode)
	recorded, err := os.ReadFile(recordPath)
	require.NoError(t, err)
	expectedCWD, err := filepath.EvalSymlinks(root)
	require.NoError(t, err)
	actualCWD, err := filepath.EvalSymlinks(strings.TrimSpace(string(recorded)))
	require.NoError(t, err)
	assert.Equal(t, expectedCWD, actualCWD)
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

// TestPythonScriptRunner_SpawnFailure verifies a missing interpreter surfaces as
// a Go error so the worker records it (and does not silently treat it as exit 0).
func TestPythonScriptRunner_SpawnFailure(t *testing.T) {
	r := PythonScriptRunner{Python: filepath.Join(t.TempDir(), "does-not-exist")}
	if _, err := r.Run(context.Background(), t.TempDir(), "raw", "/tmp/d.json"); err == nil {
		t.Fatal("missing interpreter should return an error")
	}
}
