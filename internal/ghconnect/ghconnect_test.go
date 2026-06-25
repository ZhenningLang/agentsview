package ghconnect

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// mockRunner scripts gh responses keyed by a prefix of the joined args, so a
// test declares exactly the gh calls it expects. It records every invocation
// for assertions (e.g. that a marker write happened, or that create was NOT
// called). The first matching key (by registration order) wins.
type mockRunner struct {
	responses []mockResponse
	calls     [][]string
	fail      error // when set, every Run returns this spawn error
}

type mockResponse struct {
	matchPrefix string
	stdout      string
	exitCode    int
}

func (m *mockRunner) Run(_ context.Context, args ...string) (string, int, error) {
	m.calls = append(m.calls, append([]string(nil), args...))
	if m.fail != nil {
		return "", -1, m.fail
	}
	joined := strings.Join(args, " ")
	for _, r := range m.responses {
		if strings.HasPrefix(joined, r.matchPrefix) {
			return r.stdout, r.exitCode, nil
		}
	}
	// Default: pretend success with empty output. Tests that care declare a
	// matching response explicitly.
	return "", 0, nil
}

func (m *mockRunner) called(prefix string) bool {
	for _, c := range m.calls {
		if strings.HasPrefix(strings.Join(c, " "), prefix) {
			return true
		}
	}
	return false
}

func authOK() mockResponse { return mockResponse{matchPrefix: "auth status", exitCode: 0} }

func TestConnect_GhNotAuthenticated(t *testing.T) {
	m := &mockRunner{responses: []mockResponse{
		{matchPrefix: "auth status", exitCode: 1},
	}}
	_, err := New(m).Connect(context.Background(), ConnectRequest{Target: "alice"})
	var ce *ConnectError
	if !errors.As(err, &ce) || ce.Code != CodeNotAuthenticated {
		t.Fatalf("want not_authenticated rejection, got %v", err)
	}
	if m.called("repo view") || m.called("repo create") {
		t.Fatal("must not touch any repo when gh is unauthenticated")
	}
}

func TestConnect_GhSpawnFailureRejected(t *testing.T) {
	m := &mockRunner{fail: errors.New("exec: gh not found")}
	_, err := New(m).Connect(context.Background(), ConnectRequest{Target: "alice"})
	var ce *ConnectError
	if !errors.As(err, &ce) || ce.Code != CodeNotAuthenticated {
		t.Fatalf("want not_authenticated when gh missing, got %v", err)
	}
}

func TestConnect_NamespaceDefaultsToAgentMemory(t *testing.T) {
	m := &mockRunner{responses: []mockResponse{
		authOK(),
		// Target does not exist -> create path.
		{matchPrefix: "repo view alice/agent-memory", exitCode: 1},
		{matchPrefix: "repo create alice/agent-memory --private", exitCode: 0},
		{matchPrefix: "api --method PUT", exitCode: 0},
	}}
	res, err := New(m).Connect(context.Background(), ConnectRequest{Target: "alice"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Repo != "alice/agent-memory" {
		t.Fatalf("repo = %q, want alice/agent-memory", res.Repo)
	}
	if res.Outcome != OutcomeCreated || !res.Private || !res.MarkerWritten {
		t.Fatalf("unexpected result %+v", res)
	}
	if !m.called("repo create alice/agent-memory --private") {
		t.Fatal("expected private repo create")
	}
}

func TestConnect_ExistingPublicRejected(t *testing.T) {
	m := &mockRunner{responses: []mockResponse{
		authOK(),
		{matchPrefix: "repo view alice/agent-memory", stdout: `{"visibility":"PUBLIC","isPrivate":false}`, exitCode: 0},
	}}
	_, err := New(m).Connect(context.Background(), ConnectRequest{Target: "alice/agent-memory"})
	var ce *ConnectError
	if !errors.As(err, &ce) || ce.Code != CodePublicRejected {
		t.Fatalf("want public_rejected, got %v", err)
	}
	if m.called("api --method PUT") {
		t.Fatal("must not write a marker into a public repo")
	}
}

func TestConnect_ExistingPrivateWithMarkerLinks(t *testing.T) {
	m := &mockRunner{responses: []mockResponse{
		authOK(),
		{matchPrefix: "repo view alice/mem", stdout: `{"visibility":"PRIVATE","isPrivate":true}`, exitCode: 0},
		// marker present: contents/.memory-backup-marker => 200/exit 0.
		{matchPrefix: "api /repos/alice/mem/contents/.memory-backup-marker", exitCode: 0},
	}}
	res, err := New(m).Connect(context.Background(), ConnectRequest{Target: "alice/mem"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Outcome != OutcomeLinkedExisting || res.MarkerWritten {
		t.Fatalf("want link without rewriting marker, got %+v", res)
	}
	if m.called("api --method PUT") {
		t.Fatal("must not rewrite an existing marker")
	}
}

func TestConnect_ExistingPrivateNoMarkerEmptyWritesMarker(t *testing.T) {
	m := &mockRunner{responses: []mockResponse{
		authOK(),
		{matchPrefix: "repo view alice/mem", stdout: `{"visibility":"PRIVATE","isPrivate":true}`, exitCode: 0},
		// marker absent: 404 => exit 1.
		{matchPrefix: "api /repos/alice/mem/contents/.memory-backup-marker", exitCode: 1},
		// contents probe: empty array => empty repo.
		{matchPrefix: "api /repos/alice/mem/contents", stdout: "[]", exitCode: 0},
		{matchPrefix: "api --method PUT", exitCode: 0},
	}}
	res, err := New(m).Connect(context.Background(), ConnectRequest{Target: "alice/mem", MarkerContent: "claimed"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Outcome != OutcomeLinkedExisting || !res.MarkerWritten {
		t.Fatalf("want link with marker write, got %+v", res)
	}
	if !m.called("api --method PUT") {
		t.Fatal("expected marker write on empty private repo")
	}
}

func TestConnect_ExistingPrivateNoMarkerNoCommitsTreatedEmpty(t *testing.T) {
	// A freshly-created repo with no commits returns 404 on the contents
	// endpoint; per B3 that must be treated as empty (claimable).
	m := &mockRunner{responses: []mockResponse{
		authOK(),
		{matchPrefix: "repo view alice/mem", stdout: `{"visibility":"PRIVATE","isPrivate":true}`, exitCode: 0},
		{matchPrefix: "api /repos/alice/mem/contents/.memory-backup-marker", exitCode: 1},
		{matchPrefix: "api /repos/alice/mem/contents", exitCode: 1}, // 404, no commits
		{matchPrefix: "api --method PUT", exitCode: 0},
	}}
	res, err := New(m).Connect(context.Background(), ConnectRequest{Target: "alice/mem"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.MarkerWritten {
		t.Fatalf("empty (no-commit) repo should be claimed, got %+v", res)
	}
}

func TestConnect_ExistingPrivateNoMarkerNonEmptyRejected(t *testing.T) {
	m := &mockRunner{responses: []mockResponse{
		authOK(),
		{matchPrefix: "repo view alice/work", stdout: `{"visibility":"PRIVATE","isPrivate":true}`, exitCode: 0},
		{matchPrefix: "api /repos/alice/work/contents/.memory-backup-marker", exitCode: 1},
		// non-empty contents => foreign content.
		{matchPrefix: "api /repos/alice/work/contents", stdout: `[{"name":"README.md"}]`, exitCode: 0},
	}}
	_, err := New(m).Connect(context.Background(), ConnectRequest{Target: "alice/work"})
	var ce *ConnectError
	if !errors.As(err, &ce) || ce.Code != CodeForeignContent {
		t.Fatalf("want foreign_content rejection, got %v", err)
	}
	if m.called("api --method PUT") {
		t.Fatal("must not write a marker into a foreign non-empty repo")
	}
}

func TestConnect_TargetNotExistCreatesPrivate(t *testing.T) {
	m := &mockRunner{responses: []mockResponse{
		authOK(),
		{matchPrefix: "repo view alice/mem", exitCode: 1}, // not found
		{matchPrefix: "repo create alice/mem --private", exitCode: 0},
		{matchPrefix: "api --method PUT", exitCode: 0},
	}}
	res, err := New(m).Connect(context.Background(), ConnectRequest{Target: "alice/mem"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Outcome != OutcomeCreated {
		t.Fatalf("want created outcome, got %+v", res)
	}
	if !m.called("repo create alice/mem --private") {
		t.Fatal("expected private create")
	}
}

func TestConnect_URLTargetParsed(t *testing.T) {
	m := &mockRunner{responses: []mockResponse{
		authOK(),
		{matchPrefix: "repo view alice/mem", stdout: `{"visibility":"PRIVATE","isPrivate":true}`, exitCode: 0},
		{matchPrefix: "api /repos/alice/mem/contents/.memory-backup-marker", exitCode: 0},
	}}
	for _, url := range []string{
		"https://github.com/alice/mem",
		"https://github.com/alice/mem.git",
		"git@github.com:alice/mem.git",
	} {
		res, err := New(m).Connect(context.Background(), ConnectRequest{Target: url})
		if err != nil {
			t.Fatalf("url %q: unexpected error: %v", url, err)
		}
		if res.Repo != "alice/mem" {
			t.Fatalf("url %q: repo = %q, want alice/mem", url, res.Repo)
		}
	}
}

func TestConnect_EmptyTargetRejected(t *testing.T) {
	m := &mockRunner{responses: []mockResponse{authOK()}}
	_, err := New(m).Connect(context.Background(), ConnectRequest{Target: "  "})
	var ce *ConnectError
	if !errors.As(err, &ce) || ce.Code != CodeInvalidTarget {
		t.Fatalf("want invalid_target, got %v", err)
	}
}

func TestConnect_NilRunner(t *testing.T) {
	c := &Connector{}
	if _, err := c.Connect(context.Background(), ConnectRequest{Target: "alice"}); err == nil {
		t.Fatal("want error for nil runner")
	}
}
