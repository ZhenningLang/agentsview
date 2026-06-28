package synthesize

import (
	"context"
	"log"
	"sync/atomic"
	"time"
)

const fallbackInterval = 24 * time.Hour

// Controller wraps a Worker with a runtime-toggleable enabled flag and an
// immediate-trigger channel, mirroring consolidate.Controller: the loop is
// always started, ticks/triggers are no-ops while disabled, and flipping on
// fires one immediate cycle.
type Controller struct {
	worker  *Worker
	enabled atomic.Bool
	trigger chan struct{}
}

func NewController(worker *Worker, enabled bool) *Controller {
	c := &Controller{worker: worker, trigger: make(chan struct{}, 1)}
	c.enabled.Store(enabled)
	return c
}

func (c *Controller) Enabled() bool {
	if c == nil {
		return false
	}
	return c.enabled.Load()
}

func (c *Controller) SetEnabled(on bool) {
	if c == nil {
		return
	}
	prev := c.enabled.Swap(on)
	if on && !prev {
		c.fire()
	}
}

func (c *Controller) fire() {
	select {
	case c.trigger <- struct{}{}:
	default:
	}
}

// Run drives the synthesis loop until ctx is cancelled. Fires one immediate
// cycle when enabled at startup. Callers start it in a goroutine.
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
		log.Printf("synthesize: %v", err)
	}
}
