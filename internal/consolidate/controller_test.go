package consolidate

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// countingLLM records how many cycles actually reached the LLM step, which only
// happens when the worker ran with candidates present. It is the observable
// proxy for "a cycle ran".
type countingLLM struct {
	calls atomic.Int64
	resp  string
}

func (c *countingLLM) ChatJSON(_ context.Context, _, _ string) (string, error) {
	c.calls.Add(1)
	return c.resp, nil
}

// newCountingController builds a controller whose worker has one candidate, so
// every executed cycle hits the counting LLM exactly once. callsAfter returns
// the LLM call count, used to assert how many cycles ran.
func newCountingController(t *testing.T, enabled bool) (*Controller, func() int64) {
	t.Helper()
	llm := &countingLLM{resp: `{"c1":{"action":"SKIP"}}`}
	w, rawDir := newTestWorker(t, llm,
		&fakeScript{res: ScriptResult{Stdout: "skip c1 decision_skip:x\n"}},
		&fakeCommitter{}, &fakeResyncer{})
	writeCandidate(t, rawDir, "c1.json", map[string]any{"id": "c1", "summary": "s"})
	return NewController(w, enabled), func() int64 { return llm.calls.Load() }
}

// waitFor polls cond until true or the deadline; returns whether it became true.
func waitFor(cond func() bool, d time.Duration) bool {
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(2 * time.Millisecond)
	}
	return cond()
}

// TestController_DisabledDoesNotRun verifies the locked-decision default: when
// the worker is constructed disabled, the loop starts but never runs a cycle
// (no LLM call, no write into memory/user) — the A2 "OFF first-run safety".
func TestController_DisabledDoesNotRun(t *testing.T) {
	ctrl, calls := newCountingController(t, false)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() { defer wg.Done(); ctrl.Run(ctx, 5*time.Millisecond) }()

	// Give the loop several intervals' worth of ticks while disabled.
	time.Sleep(40 * time.Millisecond)
	if got := calls(); got != 0 {
		t.Fatalf("disabled controller ran %d cycle(s), want 0", got)
	}
	cancel()
	wg.Wait()
}

// TestController_EnabledAtStartRunsImmediately verifies that a controller armed
// at construction runs one cycle right away (no waiting a full interval).
func TestController_EnabledAtStartRunsImmediately(t *testing.T) {
	ctrl, calls := newCountingController(t, true)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go ctrl.Run(ctx, time.Hour) // long interval: only the startup cycle should fire
	if !waitFor(func() bool { return calls() >= 1 }, time.Second) {
		t.Fatalf("enabled controller did not run a startup cycle, calls=%d", calls())
	}
}

// TestController_RuntimeEnableTriggersImmediateRun is the core A2 acceptance:
// a worker that starts DISABLED runs nothing, and flipping it on at runtime
// (as the UI/enable endpoint does) fires an immediate cycle without waiting a
// full interval — proving "UI 能开启 + 开启后自动跑" without a restart.
func TestController_RuntimeEnableTriggersImmediateRun(t *testing.T) {
	ctrl, calls := newCountingController(t, false)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go ctrl.Run(ctx, time.Hour) // long interval so only an enable-trigger can run a cycle

	// Still disabled: nothing runs.
	time.Sleep(20 * time.Millisecond)
	if got := calls(); got != 0 {
		t.Fatalf("pre-enable cycle count = %d, want 0", got)
	}

	// Flip on at runtime — this must fire an immediate cycle.
	ctrl.SetEnabled(true)
	if !ctrl.Enabled() {
		t.Fatal("Enabled() = false after SetEnabled(true)")
	}
	if !waitFor(func() bool { return calls() >= 1 }, time.Second) {
		t.Fatalf("runtime-enable did not trigger an immediate cycle, calls=%d", calls())
	}

	// Disabling stops further cycles: capture the count and assert it stays put.
	ctrl.SetEnabled(false)
	stable := calls()
	time.Sleep(30 * time.Millisecond)
	if got := calls(); got != stable {
		t.Fatalf("disabled controller ran extra cycles: %d -> %d", stable, got)
	}
}

// TestController_NilWorkerNoPanic verifies a nil-worker controller (the
// "feature unavailable" path) blocks until ctx cancel without panicking.
func TestController_NilWorkerNoPanic(t *testing.T) {
	ctrl := NewController(nil, true)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { ctrl.Run(ctx, time.Millisecond); close(done) }()
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("nil-worker controller did not return after ctx cancel")
	}
}
