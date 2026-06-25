package server_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.kenn.io/agentsview/internal/server"
)

// ghMock scripts gh CLI responses for the memory-backup connect endpoint so the
// route is exercised end-to-end without any real gh process or network call
// (Phase 04/05 hard rule: gh/git MOCKED, no real remote actions).
type ghMock struct {
	responses []struct {
		prefix string
		out    string
		code   int
	}
	calls []string
}

func (m *ghMock) add(prefix, out string, code int) {
	m.responses = append(m.responses, struct {
		prefix string
		out    string
		code   int
	}{prefix, out, code})
}

func (m *ghMock) Run(_ context.Context, args ...string) (string, int, error) {
	joined := strings.Join(args, " ")
	m.calls = append(m.calls, joined)
	for _, r := range m.responses {
		if strings.HasPrefix(joined, r.prefix) {
			return r.out, r.code, nil
		}
	}
	return "", 0, nil
}

func (m *ghMock) called(prefix string) bool {
	for _, c := range m.calls {
		if strings.HasPrefix(c, prefix) {
			return true
		}
	}
	return false
}

// TestConnectMemoryBackup_CreatesAndPersists drives the happy path: a bare
// namespace resolves to <owner>/agent-memory, the repo is created private, the
// marker is written, and the resolved repo + linked flag are persisted so the
// GET status endpoint reports them.
func TestConnectMemoryBackup_CreatesAndPersists(t *testing.T) {
	m := &ghMock{}
	m.add("auth status", "", 0)
	m.add("repo view alice/agent-memory", "", 1) // not found
	m.add("repo create alice/agent-memory --private", "", 0)
	m.add("api --method PUT", "", 0)

	te := setupWithServerOpts(t, []server.Option{server.WithGHRunner(m)})

	w := te.post(t, "/api/v1/config/memory-backup/connect",
		`{"namespace_or_url":"alice"}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var resp struct {
		Repo          string `json:"repo"`
		Outcome       string `json:"outcome"`
		Private       bool   `json:"private"`
		MarkerWritten bool   `json:"marker_written"`
		Linked        bool   `json:"linked"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "alice/agent-memory", resp.Repo)
	assert.Equal(t, "created", resp.Outcome)
	assert.True(t, resp.Private)
	assert.True(t, resp.MarkerWritten)
	assert.True(t, resp.Linked)
	assert.True(t, m.called("repo create alice/agent-memory --private"))

	// Status endpoint reflects the persisted link.
	gw := te.get(t, "/api/v1/config/memory-backup")
	require.Equal(t, http.StatusOK, gw.Code, gw.Body.String())
	var status struct {
		Repo   string `json:"repo"`
		Linked bool   `json:"linked"`
	}
	require.NoError(t, json.Unmarshal(gw.Body.Bytes(), &status))
	assert.Equal(t, "alice/agent-memory", status.Repo)
	assert.True(t, status.Linked)
}

// TestConnectMemoryBackup_PublicRejected proves a public target is rejected
// with a 4xx and the reason is surfaced; no marker is written.
func TestConnectMemoryBackup_PublicRejected(t *testing.T) {
	m := &ghMock{}
	m.add("auth status", "", 0)
	m.add("repo view alice/agent-memory", `{"visibility":"PUBLIC","isPrivate":false}`, 0)

	te := setupWithServerOpts(t, []server.Option{server.WithGHRunner(m)})
	w := te.post(t, "/api/v1/config/memory-backup/connect",
		`{"namespace_or_url":"alice"}`)
	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "PUBLIC")
	assert.False(t, m.called("api --method PUT"))

	// Nothing persisted on rejection.
	gw := te.get(t, "/api/v1/config/memory-backup")
	var status struct {
		Repo   string `json:"repo"`
		Linked bool   `json:"linked"`
	}
	require.NoError(t, json.Unmarshal(gw.Body.Bytes(), &status))
	assert.Empty(t, status.Repo)
	assert.False(t, status.Linked)
}

// TestConnectMemoryBackup_NotAuthenticated proves an unauthenticated gh yields
// an explicit rejection rather than a silent failure.
func TestConnectMemoryBackup_NotAuthenticated(t *testing.T) {
	m := &ghMock{}
	m.add("auth status", "", 1)

	te := setupWithServerOpts(t, []server.Option{server.WithGHRunner(m)})
	w := te.post(t, "/api/v1/config/memory-backup/connect",
		`{"namespace_or_url":"alice"}`)
	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "gh")
	assert.False(t, m.called("repo create"))
}
