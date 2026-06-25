package backup

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// remoteName is the git remote the backup workspace pushes to. The workspace is
// created and owned entirely by this package (locked decision A3: P4 only stores
// the repo name; P5 owns workspace creation + remote setup + push).
const remoteName = "origin"

// defaultBranch is the branch the backup is committed/pushed on.
const defaultBranch = "main"

// Config holds the inputs one backup cycle needs. All paths are absolute.
type Config struct {
	// Workspace is the isolated git working dir (~/.agentsview/memory-backup).
	// This package owns its .git entirely.
	Workspace string
	// Repo is the `<owner>/<name>` private backup target validated by Phase 04.
	Repo string
	// CrossAgentDir is the resolved memory/user dir (the cross-agent SSOT). May
	// be "" when not configured (its subtree is then cleared).
	CrossAgentDir string
	// CCRoot is the resolved CC-native root (~/.claude/projects). May be "".
	CCRoot string
}

// Worker performs one backup cycle: lock -> ensure workspace+remote -> verify
// the remote still points at the validated private repo -> snapshot the two
// sources -> commit -> push -> record status. Every step is fail-soft: a
// recoverable failure is recorded in the status (UI turns red) and the cycle
// ends cleanly; the next cycle retries.
type Worker struct {
	cfg    Config
	git    GitRunner
	gh     GHRunner
	status *StatusStore
	now    func() time.Time
}

// NewWorker builds a Worker. git/gh are injected so tests mock the command
// runners (no real process, no network). status may be nil (no persistence).
func NewWorker(cfg Config, git GitRunner, gh GHRunner, status *StatusStore) *Worker {
	return &Worker{cfg: cfg, git: git, gh: gh, status: status, now: time.Now}
}

// RunOnce performs a single backup cycle. It returns ErrLocked (and records a
// skip) when another holder owns the single-flight lock. Any other failure is
// returned AND recorded in the status, but never panics (fail-soft).
func (w *Worker) RunOnce(ctx context.Context) error {
	startedAt := w.clock().UTC().Format(time.RFC3339)

	lock, err := AcquireLock(w.cfg.Workspace)
	if err != nil {
		if errors.Is(err, ErrLocked) {
			// Another holder is mid-cycle: skip silently, do not clobber status.
			return ErrLocked
		}
		return w.fail(startedAt, fmt.Sprintf("acquiring lock: %v", err))
	}
	defer lock.Release()

	if strings.TrimSpace(w.cfg.Repo) == "" {
		return w.fail(startedAt, "no backup repo configured")
	}

	// Ensure the isolated workspace is a git repo with the remote pointing at
	// the configured private repo. This package creates the workspace + remote
	// (P4 never did). ensureWorkspace verifies any EXISTING origin against the
	// configured repo BEFORE it would set anything, and only sets origin when it
	// is absent — so an out-of-band remote rewrite is detected and rejected here
	// (the tamper defense), never silently overwritten and pushed to.
	if err := w.ensureWorkspace(ctx); err != nil {
		return w.fail(startedAt, fmt.Sprintf("preparing workspace: %v", err))
	}

	// Post-condition: confirm origin now resolves to the exact `<owner>/<name>`
	// Phase 04 validated as PRIVATE before pushing, so memory can never be pushed
	// to a swapped-in (possibly public) repo. Read back from git, not our config.
	if err := w.verifyRemote(ctx); err != nil {
		return w.fail(startedAt, fmt.Sprintf("verifying remote: %v", err))
	}

	// gh auth health check before any push (locked key fact). A non-zero exit
	// or spawn failure is fail-soft: record and retry next cycle.
	if err := w.checkGHAuth(ctx); err != nil {
		return w.fail(startedAt, err.Error())
	}

	syncRes, err := SyncSources(w.cfg.Workspace, w.cfg.CrossAgentDir, w.cfg.CCRoot)
	if err != nil {
		return w.fail(startedAt, fmt.Sprintf("syncing sources: %v", err))
	}

	committed, err := w.commit(ctx)
	if err != nil {
		return w.fail(startedAt, fmt.Sprintf("committing backup: %v", err))
	}

	// Even with nothing new to commit we push: a prior cycle may have committed
	// but failed to push (fail-soft), so an unchanged cycle still flushes it.
	if err := w.push(ctx); err != nil {
		return w.fail(startedAt, fmt.Sprintf("pushing backup: %v", err))
	}

	return w.succeed(startedAt, syncRes, committed)
}

// ensureWorkspace initializes the workspace as a git repo (idempotent) and makes
// origin point at the configured private repo's ssh URL.
//
// Tamper defense: it reads the EXISTING origin first. If origin already exists
// but does not match the configured `<owner>/<name>`, it is REJECTED (not
// overwritten) so an out-of-band remote rewrite is detected before any push.
// origin is only set when it is absent (first cycle / fresh workspace); a
// matching existing origin is left untouched.
func (w *Worker) ensureWorkspace(ctx context.Context) error {
	// `git -C <ws> init -b main` is idempotent on an existing repo.
	if _, code, err := w.git.Run(ctx, "-C", w.cfg.Workspace, "init", "-b", defaultBranch); err != nil {
		return fmt.Errorf("git init: %w", err)
	} else if code != 0 {
		return fmt.Errorf("git init exited %d", code)
	}
	// Keep transient runtime state out of the backup repo. The single-flight
	// lock pid file lives at the workspace root and is present during commit
	// (`git add -A`); without this the changing pid would be committed/pushed
	// every cycle, leaking runtime state and producing churn on restart.
	if err := w.writeGitignore(); err != nil {
		return fmt.Errorf("writing .gitignore: %w", err)
	}
	url := remoteURL(w.cfg.Repo)
	// Read the existing origin. A non-zero exit means origin is absent.
	out, code, err := w.git.Run(ctx, "-C", w.cfg.Workspace, "remote", "get-url", remoteName)
	if err != nil {
		return fmt.Errorf("git remote get-url: %w", err)
	}
	if code == 0 {
		// origin exists: it MUST match the configured repo. A mismatch is an
		// out-of-band tamper — reject it rather than silently overwriting and
		// pushing memory to a swapped-in (possibly public) repo.
		got := strings.TrimSpace(out)
		if !remoteMatchesRepo(got, w.cfg.Repo) {
			return fmt.Errorf("existing origin %q does not point at the configured backup repo %q (refusing to overwrite)", got, w.cfg.Repo)
		}
		return nil
	}
	// origin absent: set it for the first time (authoritative).
	if _, code, err := w.git.Run(ctx, "-C", w.cfg.Workspace, "remote", "add", remoteName, url); err != nil {
		return fmt.Errorf("git remote add: %w", err)
	} else if code != 0 {
		return fmt.Errorf("git remote add exited %d", code)
	}
	return nil
}

// gitignoreContents lists workspace-root runtime artifacts that must never be
// committed into the backup repo. The leading entry is the single-flight lock
// pid file; the trailing patterns reserve room for future lock/temp files.
const gitignoreContents = lockFile + "\n*.lock\n*.tmp\n"

// writeGitignore writes the workspace .gitignore (idempotent). It rewrites only
// when the content differs so an unchanged cycle stays a true no-op (no churn
// commit). The file itself is intentionally tracked: it is stable content, so it
// commits once and never churns.
func (w *Worker) writeGitignore() error {
	path := filepath.Join(w.cfg.Workspace, ".gitignore")
	if existing, err := os.ReadFile(path); err == nil && string(existing) == gitignoreContents {
		return nil
	}
	return os.WriteFile(path, []byte(gitignoreContents), 0o600)
}

// verifyRemote confirms origin resolves to the configured `<owner>/<name>`.
// It re-reads the remote from git (not from our own config) so a remote rewritten
// out-of-band is caught before a push leaks memory to the wrong repo.
func (w *Worker) verifyRemote(ctx context.Context) error {
	out, code, err := w.git.Run(ctx, "-C", w.cfg.Workspace, "remote", "get-url", remoteName)
	if err != nil {
		return fmt.Errorf("git remote get-url: %w", err)
	}
	if code != 0 {
		return fmt.Errorf("no %q remote configured", remoteName)
	}
	got := strings.TrimSpace(out)
	if !remoteMatchesRepo(got, w.cfg.Repo) {
		return fmt.Errorf("remote %q does not point at the configured backup repo %q", got, w.cfg.Repo)
	}
	return nil
}

// checkGHAuth runs `gh auth status`; a non-zero exit or spawn failure means gh
// is not usable, so the push must not proceed.
func (w *Worker) checkGHAuth(ctx context.Context) error {
	if w.gh == nil {
		return nil
	}
	_, code, err := w.gh.Run(ctx, "auth", "status")
	if err != nil {
		return fmt.Errorf("gh not available: %v", err)
	}
	if code != 0 {
		return fmt.Errorf("gh not authenticated (run gh auth login)")
	}
	return nil
}

// commit stages everything and commits. It reports whether a commit was made:
// `git commit` exits non-zero with nothing to commit, which is not an error.
func (w *Worker) commit(ctx context.Context) (bool, error) {
	if _, code, err := w.git.Run(ctx, "-C", w.cfg.Workspace, "add", "-A"); err != nil {
		return false, fmt.Errorf("git add: %w", err)
	} else if code != 0 {
		return false, fmt.Errorf("git add exited %d", code)
	}
	msg := fmt.Sprintf("memory backup %s", w.clock().UTC().Format(time.RFC3339))
	_, code, err := w.git.Run(ctx, "-C", w.cfg.Workspace, "commit", "-m", msg)
	if err != nil {
		return false, fmt.Errorf("git commit: %w", err)
	}
	// Non-zero exit here is the "nothing to commit" case: not a failure.
	return code == 0, nil
}

// push pushes the default branch to origin. A non-zero exit is a push failure
// (network/auth/remote): returned so the cycle records it and retries.
func (w *Worker) push(ctx context.Context) error {
	out, code, err := w.git.Run(ctx, "-C", w.cfg.Workspace, "push", remoteName, defaultBranch)
	if err != nil {
		return fmt.Errorf("git push: %w", err)
	}
	if code != 0 {
		msg := strings.TrimSpace(out)
		if msg == "" {
			msg = fmt.Sprintf("git push exited %d", code)
		}
		return errors.New(msg)
	}
	return nil
}

// fail records a failed cycle in the status (UI red) and returns the error.
func (w *Worker) fail(startedAt, msg string) error {
	if w.status != nil {
		prev, _ := w.status.Read()
		prev.Repo = w.cfg.Repo
		prev.LastAttemptAt = startedAt
		prev.LastError = msg
		prev.LastErrorAt = w.clock().UTC().Format(time.RFC3339)
		_ = w.status.Write(prev)
	}
	return errors.New(msg)
}

// succeed records a successful push, clearing the prior error.
func (w *Worker) succeed(startedAt string, sync SyncResult, committed bool) error {
	if w.status != nil {
		now := w.clock().UTC().Format(time.RFC3339)
		_ = w.status.Write(Status{
			Repo:            w.cfg.Repo,
			LastAttemptAt:   startedAt,
			LastSuccessAt:   now,
			CrossAgentFiles: sync.CrossAgentFiles,
			CCNativeFiles:   sync.CCNativeFiles,
		})
	}
	_ = committed
	return nil
}

func (w *Worker) clock() time.Time {
	if w.now != nil {
		return w.now()
	}
	return time.Now()
}

// remoteURL builds the ssh git URL for a `<owner>/<name>` repo. ssh is used so
// the push reuses the gh-configured ssh credentials (locked key fact).
func remoteURL(repo string) string {
	return "git@github.com:" + repo + ".git"
}

// remoteMatchesRepo reports whether a git remote URL (ssh or https form) refers
// to the given `<owner>/<name>` repo, tolerating a trailing `.git`.
func remoteMatchesRepo(remoteURL, repo string) bool {
	want := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(repo), ".git"))
	if want == "" {
		return false
	}
	got := strings.ToLower(strings.TrimSpace(remoteURL))
	got = strings.TrimSuffix(got, ".git")
	// Normalize both ssh (git@github.com:owner/name) and https
	// (https://github.com/owner/name) to the bare owner/name tail.
	if i := strings.Index(got, "github.com"); i >= 0 {
		got = got[i+len("github.com"):]
		got = strings.TrimLeft(got, ":/")
	}
	got = strings.Trim(got, "/")
	return got == want
}
