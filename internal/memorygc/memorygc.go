// Package memorygc runs the dotfiles memory data-lifecycle GC on a timer:
// it expires raw staging candidates, the drained-candidate archive, and
// long-archived user notes. The actual deletion logic lives in the dotfiles
// python scripts (the safety/INDEX SSOT); this package only schedules and
// shells out to them, mirroring how consolidate delegates writes to python.
package memorygc

import (
	"context"
	"fmt"
	"log"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"
)

// ExecRunner is the production CommandRunner: it shells out, with the working
// directory pinned to the dotfiles root so the python scripts resolve relative
// paths (staging dir, memory/user) correctly.
type ExecRunner struct{}

func (ExecRunner) Run(ctx context.Context, dir, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	return string(out), err
}

const (
	captureScriptRel     = "scripts/hooks/memory_capture.py"
	consolidateScriptRel = "coding-skills/assist-learn/scripts/assist_consolidate.py"

	// DefaultInterval is how often the GC runs. GC is cheap and safe (git is the
	// audit trail), so it runs on a steady cadence rather than behind an enable gate.
	DefaultInterval = 24 * time.Hour
	// DefaultArchivedNoteTTLDays is the retention for archived/stale user notes.
	DefaultArchivedNoteTTLDays = 90
)

// CommandRunner executes a subprocess. The production impl shells out; tests
// substitute a fake to assert the exact commands without running python.
type CommandRunner interface {
	Run(ctx context.Context, dir, name string, args ...string) (string, error)
}

// GC expires the three memory data tiers via the dotfiles python scripts.
type GC struct {
	Root                string // dotfiles repo root
	ArchivedNoteTTLDays int    // 0 -> default
	Runner              CommandRunner
}

// RunOnce performs one GC pass: candidates + consumed archive (memory_capture
// --gc) and long-archived user notes (assist_consolidate --gc-archived-notes).
// A failure of one leg is logged and collected but never aborts the other.
func (g GC) RunOnce(ctx context.Context) error {
	ttl := g.ArchivedNoteTTLDays
	if ttl <= 0 {
		ttl = DefaultArchivedNoteTTLDays
	}
	var firstErr error
	note := func(err error) {
		if err != nil && firstErr == nil {
			firstErr = err
		}
	}

	// Tier 0/1: raw candidates (14d) + consumed archive (7d), python defaults.
	if _, err := g.Runner.Run(ctx, g.Root, "python3",
		filepath.Join(g.Root, captureScriptRel), "--root", g.Root, "--gc"); err != nil {
		log.Printf("memory gc: candidate/consumed: %v", err)
		note(fmt.Errorf("candidate gc: %w", err))
	}

	// Tier 2: long-archived user notes.
	rawDir := filepath.Join(g.Root, "memory", ".staging", "raw_memories")
	if _, err := g.Runner.Run(ctx, g.Root, "python3",
		filepath.Join(g.Root, consolidateScriptRel),
		"--root", g.Root, "--raw-dir", rawDir,
		"--gc-archived-notes", "--archived-note-ttl-days", strconv.Itoa(ttl)); err != nil {
		log.Printf("memory gc: archived notes: %v", err)
		note(fmt.Errorf("archived-note gc: %w", err))
	}
	return firstErr
}

// Run drives the GC loop: one pass on startup, then every interval, until ctx
// is cancelled. Callers start it in a goroutine.
func (g GC) Run(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = DefaultInterval
	}
	g.runOnce(ctx)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			g.runOnce(ctx)
		}
	}
}

func (g GC) runOnce(ctx context.Context) {
	_ = g.RunOnce(ctx)
}
