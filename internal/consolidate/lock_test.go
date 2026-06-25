package consolidate

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

// spec verify ②: the second holder of the lock skips its cycle.
func TestAcquireLock_SecondHolderSkips(t *testing.T) {
	staging := t.TempDir()
	first, err := AcquireLock(staging)
	if err != nil {
		t.Fatalf("first AcquireLock: %v", err)
	}
	defer first.Release()

	if _, err := AcquireLock(staging); !errors.Is(err, ErrLocked) {
		t.Fatalf("second AcquireLock err = %v, want ErrLocked", err)
	}

	// After release, the lock is acquirable again.
	first.Release()
	second, err := AcquireLock(staging)
	if err != nil {
		t.Fatalf("AcquireLock after release: %v", err)
	}
	second.Release()
}

// A lock left by a dead pid is reclaimed rather than wedging the worker.
func TestAcquireLock_ReclaimsStalePid(t *testing.T) {
	staging := t.TempDir()
	// Write a lock file with a pid that is almost certainly dead.
	stale := filepath.Join(staging, lockFile)
	if err := os.WriteFile(stale, []byte(strconv.Itoa(2_000_000_000)), 0o600); err != nil {
		t.Fatal(err)
	}
	l, err := AcquireLock(staging)
	if err != nil {
		t.Fatalf("AcquireLock should reclaim a stale lock: %v", err)
	}
	l.Release()
}

// The worker skips (and audits) the cycle when the lock is already held.
func TestWorker_RunOnce_SkipsWhenLocked(t *testing.T) {
	script := &fakeScript{}
	w, rawDir := newTestWorker(t,
		fakeLLM{resp: `{"c1":{"action":"ADD"}}`}, script, &fakeCommitter{}, &fakeResyncer{})
	writeCandidate(t, rawDir, "c1.json", map[string]any{"id": "c1", "summary": "s"})

	held, err := AcquireLock(w.StagingDir)
	if err != nil {
		t.Fatalf("pre-acquire lock: %v", err)
	}
	defer held.Release()

	rec, err := w.RunOnce(context.Background())
	if !errors.Is(err, ErrLocked) {
		t.Fatalf("RunOnce err = %v, want ErrLocked", err)
	}
	if !rec.Skipped {
		t.Error("locked cycle should be marked skipped")
	}
	if script.called {
		t.Error("script must not run while the lock is held by another holder")
	}
}

// The decision file the worker hands the script is the normalized per-candidate
// JSON map the script's contract expects.
func TestWorker_WritesDecisionFile(t *testing.T) {
	script := &fakeScript{res: ScriptResult{Stdout: "skip c1 decision_skip:x\n"}}
	w, rawDir := newTestWorker(t,
		fakeLLM{resp: `{"c1":{"action":"add","reason":"new"}}`}, script, &fakeCommitter{}, &fakeResyncer{})
	writeCandidate(t, rawDir, "c1.json", map[string]any{"id": "c1", "summary": "s"})

	// Capture the decision-file content before the worker removes it.
	var captured []byte
	script.onRun = func(decisionFile string) {
		captured, _ = os.ReadFile(decisionFile)
	}
	if _, err := w.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	var decoded map[string]Decision
	if err := json.Unmarshal(captured, &decoded); err != nil {
		t.Fatalf("decision file not valid JSON map: %v (%q)", err, captured)
	}
	if decoded["c1"].Action != ActionADD {
		t.Errorf("decision file c1 action = %q, want ADD", decoded["c1"].Action)
	}
}
