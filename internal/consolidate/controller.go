package consolidate

import (
	"context"
	"log"
	"sync/atomic"
	"time"
)

// fallbackInterval is used only when Run is given a non-positive interval; the
// real default (24h) is resolved by config before Run is called, this is just a
// defensive floor so a misconfigured caller never busy-loops.
const fallbackInterval = 24 * time.Hour

// Controller wraps a Worker with a runtime-toggleable enabled flag and an
// immediate-trigger channel, so the background loop can be started
// unconditionally and the UI/API can arm or disarm it at runtime without a
// process restart (locked decision A2: "UI 能开启 + 开启后自动跑").
//
// When disabled the loop still runs but every tick/trigger is a no-op, so no
// LLM-decided write into memory/user ever happens. When the flag flips from
// off to on, SetEnabled fires an immediate trigger so "开启后即自动跑一次" —
// the user does not wait a full interval to see the first cycle.
type Controller struct {
	worker  *Worker
	enabled atomic.Bool
	// trigger is buffered (cap 1) and coalescing: a pending trigger that has
	// not yet been consumed simply absorbs further requests, so a burst of
	// enables never queues a burst of cycles.
	trigger chan struct{}
}

// NewController builds a Controller around worker with the given initial
// enabled state. The worker may be nil only in tests that never call Run.
func NewController(worker *Worker, enabled bool) *Controller {
	c := &Controller{
		worker:  worker,
		trigger: make(chan struct{}, 1),
	}
	c.enabled.Store(enabled)
	return c
}

// Enabled reports whether the worker is currently armed.
func (c *Controller) Enabled() bool {
	if c == nil {
		return false
	}
	return c.enabled.Load()
}

// SetEnabled flips the runtime enabled flag. When it transitions from off to
// on it fires an immediate (coalesced) trigger so the first cycle runs now
// rather than at the next interval tick. Disabling takes effect on the next
// tick/trigger (an in-flight cycle is allowed to finish).
func (c *Controller) SetEnabled(on bool) {
	if c == nil {
		return
	}
	prev := c.enabled.Swap(on)
	if on && !prev {
		c.fire()
	}
}

// Trigger requests an immediate cycle (no-op when disabled). It is coalescing
// and non-blocking. Exposed for callers that want to force a run without
// changing the enabled state.
func (c *Controller) Trigger() {
	if c == nil || !c.enabled.Load() {
		return
	}
	c.fire()
}

func (c *Controller) fire() {
	select {
	case c.trigger <- struct{}{}:
	default:
	}
}

// Run drives the consolidation loop until ctx is cancelled. The loop is always
// started (regardless of the initial enabled state); each tick or trigger runs
// a cycle only when currently enabled, so toggling at runtime takes effect
// without restarting the process. When enabled at startup it fires one
// immediate cycle. Run blocks; callers start it in a goroutine.
func (c *Controller) Run(ctx context.Context, interval time.Duration) {
	if c.worker == nil {
		<-ctx.Done()
		return
	}
	if interval <= 0 {
		interval = fallbackInterval
	}
	if c.enabled.Load() {
		c.runOnce(ctx)
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if c.enabled.Load() {
				c.runOnce(ctx)
			}
		case <-c.trigger:
			if c.enabled.Load() {
				c.runOnce(ctx)
			}
		}
	}
}

func (c *Controller) runOnce(ctx context.Context) {
	if _, err := c.worker.RunOnce(ctx); err != nil {
		log.Printf("consolidate: %v", err)
	}
}
