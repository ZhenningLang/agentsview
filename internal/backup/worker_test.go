package backup

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

// mockRunner scripts git/gh responses keyed by a prefix of the joined args, and
// records every call so a test can assert which commands ran (e.g. that push was
// NOT called on a rejected cycle). The first matching response (registration
// order) wins; an unmatched call defaults to success/exit 0.
type mockRunner struct {
	responses []mockResponse
	calls     [][]string
	fail      error // when set, every Run returns this spawn error
	// dynamic, when set, is consulted before the static responses. It may model
	// state-changing commands (e.g. `remote add` making a later `get-url`
	// resolve). Returning handled=false falls through to responses/default.
	dynamic   func(args []string) (stdout string, code int, handled bool)
	originSet bool // helper state for dynamic mocks modeling `remote add`
}

type mockResponse struct {
	prefix string
	stdout string
	code   int
}

func (m *mockRunner) Run(_ context.Context, args ...string) (string, int, error) {
	m.calls = append(m.calls, append([]string(nil), args...))
	if m.fail != nil {
		return "", -1, m.fail
	}
	if m.dynamic != nil {
		if stdout, code, handled := m.dynamic(args); handled {
			return stdout, code, nil
		}
	}
	joined := strings.Join(args, " ")
	for _, r := range m.responses {
		if strings.Contains(joined, r.prefix) {
			return r.stdout, r.code, nil
		}
	}
	return "", 0, nil
}

func (m *mockRunner) called(substr string) bool {
	for _, c := range m.calls {
		if strings.Contains(strings.Join(c, " "), substr) {
			return true
		}
	}
	return false
}

// validRemoteGit returns a git mock whose remote get-url reports the configured
// repo, so ensureWorkspace sees a matching existing origin and verifyRemote
// passes. Other git commands default to success.
func validRemoteGit(repo string) *mockRunner {
	return &mockRunner{responses: []mockResponse{
		{prefix: "remote get-url", stdout: remoteURL(repo) + "\n", code: 0},
	}}
}

// freshRemoteGit returns a git mock modeling a workspace with NO origin yet:
// the first `remote get-url` reports origin absent (exit 2) so ensureWorkspace
// adds it; once added, subsequent `remote get-url` resolves to the configured
// repo so verifyRemote passes. This exercises the real first-cycle data flow
// (add origin) end to end rather than asserting verifyRemote in isolation.
func freshRemoteGit(repo string) *mockRunner {
	m := &mockRunner{}
	m.dynamic = func(args []string) (string, int, bool) {
		joined := strings.Join(args, " ")
		switch {
		case strings.Contains(joined, "remote add origin"):
			m.originSet = true
			return "", 0, true
		case strings.Contains(joined, "remote get-url"):
			if m.originSet {
				return remoteURL(repo) + "\n", 0, true
			}
			// origin not configured yet: git exits non-zero.
			return "", 2, true
		}
		return "", 0, false
	}
	return m
}

func authOKGH() *mockRunner {
	return &mockRunner{responses: []mockResponse{
		{prefix: "auth status", code: 0},
	}}
}

func newTestWorker(t *testing.T, repo string, git, gh *mockRunner) (*Worker, *StatusStore) {
	t.Helper()
	ws := t.TempDir()
	cross := t.TempDir()
	writeFile(t, filepath.Join(cross, "note.md"), "x")
	st := NewStatusStore(StatusPath(t.TempDir()))
	w := NewWorker(Config{
		Workspace:     ws,
		Repo:          repo,
		CrossAgentDir: cross,
		CCRoot:        "",
	}, git, gh, st)
	return w, st
}

// TestRunOnce_HappyPathPushes proves a full cycle on an existing matching
// workspace inits (idempotent), leaves the already-correct origin untouched,
// syncs, commits, and pushes; the status records a success with no error.
func TestRunOnce_HappyPathPushes(t *testing.T) {
	repo := "alice/agent-memory"
	git := validRemoteGit(repo)
	gh := authOKGH()
	w, st := newTestWorker(t, repo, git, gh)

	if err := w.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if !git.called("push origin main") {
		t.Fatal("expected a push to origin main")
	}
	// A matching existing origin must be left as-is: do NOT rewrite it.
	if git.called("remote add origin") || git.called("remote set-url origin") {
		t.Fatal("must not rewrite an already-correct origin")
	}
	s, _ := st.Read()
	if s.LastSuccessAt == "" {
		t.Fatalf("expected LastSuccessAt to be set, got %+v", s)
	}
	if s.LastError != "" {
		t.Fatalf("expected no error, got %q", s.LastError)
	}
	if s.CrossAgentFiles != 1 {
		t.Fatalf("expected 1 cross-agent file synced, got %d", s.CrossAgentFiles)
	}
}

// TestRunOnce_FreshWorkspaceAddsRemote proves that on a fresh workspace with no
// origin yet, the cycle ADDS origin pointing at the configured repo, then pushes
// successfully. This exercises the first-cycle remote-setup path end to end.
func TestRunOnce_FreshWorkspaceAddsRemote(t *testing.T) {
	repo := "alice/agent-memory"
	git := freshRemoteGit(repo)
	gh := authOKGH()
	w, st := newTestWorker(t, repo, git, gh)

	if err := w.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if !git.called("remote add origin " + remoteURL(repo)) {
		t.Fatalf("expected origin added as %s", remoteURL(repo))
	}
	if !git.called("push origin main") {
		t.Fatal("expected a push to origin main")
	}
	s, _ := st.Read()
	if s.LastSuccessAt == "" || s.LastError != "" {
		t.Fatalf("expected success with no error, got %+v", s)
	}
}

// TestRunOnce_RemoteTamperedRejected proves that if an EXISTING workspace origin
// was rewritten out-of-band to a different (attacker) repo, the cycle refuses to
// overwrite or push and records the rejection — memory never leaks to a swapped
// repo. The mock models a persistent out-of-band origin: every `remote get-url`
// returns the attacker URL regardless of any `remote add`/`set-url`, so the test
// fails if production ever blindly overwrites the tampered origin before reading
// it back (the previous dead-code defense).
func TestRunOnce_RemoteTamperedRejected(t *testing.T) {
	repo := "alice/agent-memory"
	const attacker = "git@github.com:mallory/public-leak.git"
	git := &mockRunner{}
	git.dynamic = func(args []string) (string, int, bool) {
		joined := strings.Join(args, " ")
		// The attacker origin is sticky: it is NOT replaced by add/set-url.
		if strings.Contains(joined, "remote add origin") || strings.Contains(joined, "remote set-url origin") {
			return "", 0, true
		}
		if strings.Contains(joined, "remote get-url") {
			return attacker + "\n", 0, true
		}
		return "", 0, false
	}
	gh := authOKGH()
	w, st := newTestWorker(t, repo, git, gh)

	err := w.RunOnce(context.Background())
	if err == nil {
		t.Fatal("expected rejection when remote does not match configured repo")
	}
	if git.called("push") {
		t.Fatal("must NOT push when the remote does not match the configured repo")
	}
	s, _ := st.Read()
	if s.LastError == "" || s.LastSuccessAt != "" {
		t.Fatalf("expected recorded error and no success, got %+v", s)
	}
}

// TestRunOnce_GHNotAuthenticatedFailsSoft proves an unauthenticated gh fails
// soft: no push, an error recorded for the UI, and no panic.
func TestRunOnce_GHNotAuthenticatedFailsSoft(t *testing.T) {
	repo := "alice/agent-memory"
	git := validRemoteGit(repo)
	gh := &mockRunner{responses: []mockResponse{{prefix: "auth status", code: 1}}}
	w, st := newTestWorker(t, repo, git, gh)

	err := w.RunOnce(context.Background())
	if err == nil {
		t.Fatal("expected error when gh is not authenticated")
	}
	if git.called("push") {
		t.Fatal("must NOT push when gh is unauthenticated")
	}
	s, _ := st.Read()
	if s.LastError == "" {
		t.Fatal("expected the auth failure recorded in status")
	}
	if !strings.Contains(s.LastError, "gh") {
		t.Fatalf("expected gh in error, got %q", s.LastError)
	}
}

// TestRunOnce_GHSpawnFailureFailsSoft proves a missing gh binary is fail-soft.
func TestRunOnce_GHSpawnFailureFailsSoft(t *testing.T) {
	repo := "alice/agent-memory"
	git := validRemoteGit(repo)
	gh := &mockRunner{fail: errors.New("exec: gh not found")}
	w, _ := newTestWorker(t, repo, git, gh)
	if err := w.RunOnce(context.Background()); err == nil {
		t.Fatal("expected fail-soft error on gh spawn failure")
	}
	if git.called("push") {
		t.Fatal("must NOT push when gh is missing")
	}
}

// TestRunOnce_PushFailureRecorded proves a non-zero push exit is recorded as an
// error (fail-soft) and not a success.
func TestRunOnce_PushFailureRecorded(t *testing.T) {
	repo := "alice/agent-memory"
	git := &mockRunner{responses: []mockResponse{
		{prefix: "remote get-url", stdout: remoteURL(repo) + "\n", code: 0},
		{prefix: "push", stdout: "fatal: unable to access remote\n", code: 1},
	}}
	gh := authOKGH()
	w, st := newTestWorker(t, repo, git, gh)

	if err := w.RunOnce(context.Background()); err == nil {
		t.Fatal("expected push failure to be returned")
	}
	s, _ := st.Read()
	if s.LastError == "" || s.LastSuccessAt != "" {
		t.Fatalf("expected recorded push error and no success, got %+v", s)
	}
}

// TestRunOnce_NoRepoConfiguredFailsSoft proves an unconfigured target is a
// recorded soft failure, not a panic or a push.
func TestRunOnce_NoRepoConfiguredFailsSoft(t *testing.T) {
	git := &mockRunner{}
	gh := authOKGH()
	w, _ := newTestWorker(t, "", git, gh)
	if err := w.RunOnce(context.Background()); err == nil {
		t.Fatal("expected error with no repo configured")
	}
	if git.called("push") {
		t.Fatal("must not push without a configured repo")
	}
}

// TestRunOnce_LockHeldSkips proves a held lock makes the cycle skip with
// ErrLocked and does not clobber the existing status.
func TestRunOnce_LockHeldSkips(t *testing.T) {
	repo := "alice/agent-memory"
	git := validRemoteGit(repo)
	gh := authOKGH()
	w, _ := newTestWorker(t, repo, git, gh)

	// Hold the lock so the worker's own acquire loses.
	held, err := AcquireLock(w.cfg.Workspace)
	if err != nil {
		t.Fatalf("pre-acquire: %v", err)
	}
	defer held.Release()

	if err := w.RunOnce(context.Background()); !errors.Is(err, ErrLocked) {
		t.Fatalf("RunOnce = %v, want ErrLocked", err)
	}
	if git.called("push") {
		t.Fatal("must not push when the lock is held")
	}
}

func TestRemoteMatchesRepo(t *testing.T) {
	cases := []struct {
		url, repo string
		want      bool
	}{
		{"git@github.com:alice/mem.git", "alice/mem", true},
		{"git@github.com:alice/mem", "alice/mem", true},
		{"https://github.com/alice/mem.git", "alice/mem", true},
		{"https://github.com/alice/mem", "alice/mem", true},
		{"git@github.com:mallory/mem.git", "alice/mem", false},
		{"git@github.com:alice/other.git", "alice/mem", false},
		{"", "alice/mem", false},
		{"git@github.com:alice/mem.git", "", false},
	}
	for _, c := range cases {
		if got := remoteMatchesRepo(c.url, c.repo); got != c.want {
			t.Errorf("remoteMatchesRepo(%q,%q) = %v, want %v", c.url, c.repo, got, c.want)
		}
	}
}
