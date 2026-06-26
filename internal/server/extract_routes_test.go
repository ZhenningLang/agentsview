package server_test

import (
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.kenn.io/agentsview/internal/config"
	"go.kenn.io/agentsview/internal/db"
	"go.kenn.io/agentsview/internal/extract"
	"go.kenn.io/agentsview/internal/server"
)

func TestExtractAuditUnavailableReturnsEmptyRecords(t *testing.T) {
	memDir := filepath.Join(t.TempDir(), "memory", "user")
	require.NoError(t, os.MkdirAll(memDir, 0o700))
	te := setup(t, withMemoryDir(memDir))

	w := te.get(t, "/api/v1/extract/audit")
	assertStatus(t, w, http.StatusOK)
	resp := decode[map[string]any](t, w)
	assert.Equal(t, false, resp["enabled"])
	assert.Equal(t, false, resp["available"])
	assert.Empty(t, resp["records"].([]any))
}

func TestExtractEnablePersistsAndFlipsController(t *testing.T) {
	memDir := filepath.Join(t.TempDir(), "memory", "user")
	require.NoError(t, os.MkdirAll(memDir, 0o700))
	ctl := extract.NewController(nil, false)
	te := setupWithServerOpts(t, []server.Option{server.WithExtractController(ctl)}, withMemoryDir(memDir))

	w := requestJSON(te, http.MethodPut, "/api/v1/extract/enable", `{"enabled":true}`)
	assertStatus(t, w, http.StatusOK)
	resp := decode[map[string]any](t, w)
	assert.Equal(t, true, resp["enabled"])
	assert.Equal(t, true, resp["available"])
	assert.True(t, ctl.Enabled())

	data, err := os.ReadFile(filepath.Join(te.dataDir, "config.toml"))
	require.NoError(t, err)
	assert.Contains(t, string(data), "extract_enabled = true")
}

func TestExtractEnableReadOnlyRejected(t *testing.T) {
	te := setupWithReadOnlyExtractServer(t)
	w := requestJSON(te, http.MethodPut, "/api/v1/extract/enable", `{"enabled":true}`)
	assert.Equal(t, http.StatusNotImplemented, w.Code, w.Body.String())
}

type readOnlyStore struct{ db.Store }

func (s readOnlyStore) ReadOnly() bool { return true }

func setupWithReadOnlyExtractServer(t *testing.T) *testEnv {
	t.Helper()
	dir := t.TempDir()
	database, err := db.Open(filepath.Join(dir, "test.db"))
	require.NoError(t, err)
	t.Cleanup(func() { database.Close() })
	cfg := config.Config{Host: "127.0.0.1", Port: 0, DataDir: dir, WriteTimeout: 30 * time.Second}
	srv := server.New(cfg, readOnlyStore{Store: database}, nil, server.WithExtractController(extract.NewController(nil, false)))
	defaultHost := net.JoinHostPort(cfg.Host, "0")
	defaultOrigin := "http://" + defaultHost
	baseHandler := srv.Handler()
	wrappedHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Host == "example.com" || r.Host == "" {
			r.Host = defaultHost
		}
		if r.RemoteAddr == "192.0.2.1:1234" {
			r.RemoteAddr = "127.0.0.1:1234"
		}
		if r.Header.Get("Origin") == "" {
			switch r.Method {
			case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
				r.Header.Set("Origin", defaultOrigin)
			}
		}
		baseHandler.ServeHTTP(w, r)
	})
	return &testEnv{handler: wrappedHandler, dataDir: dir, db: database}
}
