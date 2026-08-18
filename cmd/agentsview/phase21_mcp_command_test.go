package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.kenn.io/kit/daemon"

	"go.kenn.io/agentsview/internal/config"
	"go.kenn.io/agentsview/internal/db"
	"go.kenn.io/agentsview/internal/service"
)

// phase21MCPFakeService records what the MCP backend forwards. Only the
// methods under test are implemented; anything else panics on the nil
// embedded interface rather than returning a silent zero value.
type phase21MCPFakeService struct {
	service.SessionService
	lists    int
	pairwise []service.UsagePairwiseComparisonRequest
}

func (f *phase21MCPFakeService) List(
	context.Context, service.ListFilter,
) (*service.SessionList, error) {
	f.lists++
	return &service.SessionList{}, nil
}

func (f *phase21MCPFakeService) UsagePairwiseComparison(
	_ context.Context, req service.UsagePairwiseComparisonRequest,
) (*service.PairwiseComparisonResponse, error) {
	f.pairwise = append(f.pairwise, req)
	return &service.PairwiseComparisonResponse{}, nil
}

// phase21MCPRuntime builds a runtime record pointing at host:port.
func phase21MCPRuntime(port int) *DaemonRuntime {
	return &DaemonRuntime{
		Record: daemon.RuntimeRecord{PID: os.Getpid()},
		Host:   "127.0.0.1",
		Port:   port,
	}
}

// --- QA9: command surface ---------------------------------------------

func TestPhase21MCPCommandRegistersFlags(t *testing.T) {
	mcpCmd := findPhase21Command(t, "mcp")
	for _, name := range []string{
		"http", "http-allow-insecure", "server", "server-token-file", "pg",
	} {
		assert.NotNil(t, mcpCmd.Flags().Lookup(name),
			"mcp must expose --%s", name)
	}
	assert.Contains(t, mcpCmd.Short, "read-only")
}

func findPhase21Command(t *testing.T, name string) *cobra.Command {
	t.Helper()
	for _, cmd := range newRootCommand().Commands() {
		if cmd.Name() == name {
			return cmd
		}
	}
	t.Fatalf("command %q is not registered", name)
	return nil
}

func TestPhase21MCPCommandRejectsConflictingFlags(t *testing.T) {
	tests := []struct {
		name    string
		opts    mcpOptions
		wantErr string
	}{
		{
			name:    "server and pg",
			opts:    mcpOptions{ServerURL: "http://127.0.0.1:8080", PG: true},
			wantErr: "only one",
		},
		{
			name:    "token file without server",
			opts:    mcpOptions{ServerTokenFile: "/tmp/token"},
			wantErr: "requires --server",
		},
		{
			name:    "insecure without http",
			opts:    mcpOptions{HTTPAllowInsecure: true},
			wantErr: "requires --http",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.opts.validate()
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}

	for _, ok := range []mcpOptions{
		{},
		{HTTPAddr: ":8085"},
		{HTTPAddr: "0.0.0.0:8085", HTTPAllowInsecure: true},
		{ServerURL: "http://127.0.0.1:8080", ServerTokenFile: "/tmp/t"},
		{PG: true},
	} {
		assert.NoError(t, ok.validate())
	}
}

func TestPhase21MCPCommandSelectsBackendByFlags(t *testing.T) {
	var called []string
	restore := mcpBackends
	t.Cleanup(func() { mcpBackends = restore })
	mcpBackends = mcpBackendHooks{
		local: func(config.Config) (service.SessionService, func(), error) {
			called = append(called, "local")
			return &phase21MCPFakeService{}, func() {}, nil
		},
		server: func(
			config.Config, mcpOptions,
		) (service.SessionService, func(), error) {
			called = append(called, "server")
			return &phase21MCPFakeService{}, func() {}, nil
		},
		pg: func(config.Config) (service.SessionService, func(), error) {
			called = append(called, "pg")
			return &phase21MCPFakeService{}, func() {}, nil
		},
	}

	tests := []struct {
		name string
		opts mcpOptions
		want string
	}{
		{"default", mcpOptions{}, "local"},
		{"explicit server", mcpOptions{ServerURL: "http://x"}, "server"},
		{"postgres", mcpOptions{PG: true}, "pg"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			called = nil
			_, cleanup, err := newMCPService(config.Config{}, tt.opts)
			require.NoError(t, err)
			cleanup()
			assert.Equal(t, []string{tt.want}, called,
				"exactly one backend policy applies")
		})
	}
}

func TestPhase21MCPCommandSignalContextStopsOnTerm(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("SIGTERM cannot be delivered to self on Windows")
	}
	ctx, stop := mcpSignalContext(context.Background())
	defer stop()

	proc, err := os.FindProcess(os.Getpid())
	require.NoError(t, err)
	require.NoError(t, proc.Signal(syscall.SIGTERM))

	select {
	case <-ctx.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("SIGTERM did not cancel the mcp context")
	}
}

// --- QA9: local daemon backend ----------------------------------------

func TestPhase21MCPLocalDaemonResolvesEveryOperation(t *testing.T) {
	backend := &phase21MCPFakeService{}
	var runtime *DaemonRuntime
	starts, finds, builds := 0, 0, 0

	svc := &mcpLocalService{
		cfg: config.Config{DataDir: t.TempDir()},
		find: func(config.Config) *DaemonRuntime {
			finds++
			return runtime
		},
		start: func(cfg config.Config) (daemonStartResult, error) {
			starts++
			runtime = phase21MCPRuntime(18080)
			return daemonStartResult{Runtime: runtime, Cfg: cfg}, nil
		},
		newBackend: func(url, _ string) service.SessionService {
			builds++
			assert.Equal(t, "http://127.0.0.1:18080", url)
			return backend
		},
	}

	ctx := context.Background()
	_, err := svc.List(ctx, service.ListFilter{})
	require.NoError(t, err)
	assert.Equal(t, 1, starts, "no daemon yet, so one is started")

	_, err = svc.List(ctx, service.ListFilter{})
	require.NoError(t, err)
	assert.Equal(t, 1, starts, "a live daemon is reused")
	assert.Equal(t, 2, builds,
		"the backend is rebuilt per operation, never cached across calls")

	// The daemon is not owned by this process: it can be stopped or
	// upgraded between two tool calls. A cached backend would keep
	// talking to a dead port for the rest of the session.
	runtime = nil
	_, err = svc.List(ctx, service.ListFilter{})
	require.NoError(t, err)
	assert.Equal(t, 2, starts, "a vanished daemon is started again")
	assert.Equal(t, 3, backend.lists)
	// Three, not four: a start that returns its runtime is believed
	// rather than re-probed. Re-probing is what loses a token minted
	// during the start.
	assert.GreaterOrEqual(t, finds, 3)
}

func TestPhase21MCPLocalDaemonFailsWithoutAPublishedRuntime(t *testing.T) {
	builds := 0
	svc := &mcpLocalService{
		cfg:  config.Config{DataDir: t.TempDir()},
		find: func(config.Config) *DaemonRuntime { return nil },
		start: func(cfg config.Config) (daemonStartResult, error) {
			return daemonStartResult{Cfg: cfg}, nil
		},
		newBackend: func(string, string) service.SessionService {
			builds++
			return &phase21MCPFakeService{}
		},
	}
	_, err := svc.List(context.Background(), service.ListFilter{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "runtime record")
	assert.Zero(t, builds,
		"a backend must not be built before a runtime is published")
}

func TestPhase21MCPLocalDaemonRefusesWrites(t *testing.T) {
	svc := &mcpLocalService{
		cfg:  config.Config{DataDir: t.TempDir()},
		find: func(config.Config) *DaemonRuntime { return nil },
		start: func(config.Config) (daemonStartResult, error) {
			t.Fatal("a write must not start a daemon")
			return daemonStartResult{}, nil
		},
		newBackend: func(string, string) service.SessionService {
			t.Fatal("a write must not build a backend")
			return nil
		},
	}
	_, err := svc.Sync(context.Background(), service.SyncInput{ID: "x"})
	assert.ErrorIs(t, err, db.ErrReadOnly)
	_, err = svc.ScanSecrets(
		context.Background(), service.SecretScanInput{}, nil)
	assert.ErrorIs(t, err, db.ErrReadOnly)
}

func TestPhase21MCPLocalDaemonNeverOpensTheArchive(t *testing.T) {
	dataDir := t.TempDir()
	svc := &mcpLocalService{
		cfg:  config.Config{DataDir: dataDir, DBPath: filepath.Join(dataDir, "sessions.db")},
		find: func(config.Config) *DaemonRuntime { return nil },
		start: func(cfg config.Config) (daemonStartResult, error) {
			return daemonStartResult{Cfg: cfg}, nil
		},
		newBackend: func(string, string) service.SessionService {
			return &phase21MCPFakeService{}
		},
	}
	_, err := svc.List(context.Background(), service.ListFilter{})
	require.Error(t, err)

	entries, err := os.ReadDir(dataDir)
	require.NoError(t, err)
	assert.Empty(t, entries,
		"failing to reach a daemon must not create an archive")

	// Static half of the same claim: the MCP command path must not
	// contain a direct opener at all. The needles are assembled at
	// runtime so this scan cannot match itself.
	forbidden := []string{
		"db." + "Open(", "sql." + "Open(",
		"openWrite" + "DB(", "openReadOnly" + "DB(",
		"db." + "OpenReadOnly(",
	}
	for _, name := range []string{"mcp.go", "mcp_backend.go"} {
		source, err := os.ReadFile(name)
		require.NoError(t, err)
		for _, needle := range forbidden {
			assert.NotContains(t, string(source), needle,
				"%s must reach the archive only through a daemon", name)
		}
	}
}

// --- QA9: explicit server backend -------------------------------------

func TestPhase21MCPExplicitServerSendsTheConfiguredToken(t *testing.T) {
	var seen []string
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			seen = append(seen, r.Header.Get("Authorization"))
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"sessions":[],"total":0}`))
		}))
	t.Cleanup(srv.Close)

	tokenFile := filepath.Join(t.TempDir(), "token")
	require.NoError(t, os.WriteFile(
		tokenFile, []byte("  file-token\n"), 0o600))

	tests := []struct {
		name string
		cfg  config.Config
		opts mcpOptions
		want string
	}{
		{
			name: "token file wins",
			cfg:  config.Config{AuthToken: "config-token"},
			opts: mcpOptions{ServerURL: srv.URL, ServerTokenFile: tokenFile},
			want: "Bearer file-token",
		},
		{
			name: "config token is the fallback",
			cfg:  config.Config{AuthToken: "config-token"},
			opts: mcpOptions{ServerURL: srv.URL},
			want: "Bearer config-token",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			seen = nil
			svc, cleanup, err := newMCPServerService(tt.cfg, tt.opts)
			require.NoError(t, err)
			t.Cleanup(cleanup)
			_, err = svc.List(context.Background(), service.ListFilter{})
			require.NoError(t, err)
			require.Len(t, seen, 1)
			assert.Equal(t, tt.want, seen[0])

			// The explicit-server backend is read-only even though the
			// remote server would accept a sync.
			_, err = svc.Sync(context.Background(), service.SyncInput{ID: "x"})
			assert.Error(t, err)
			assert.Len(t, seen, 1, "a write must not reach the server")
		})
	}
}

func TestPhase21MCPExplicitServerRejectsUnusableTokenFiles(t *testing.T) {
	dir := t.TempDir()
	empty := filepath.Join(dir, "empty")
	require.NoError(t, os.WriteFile(empty, []byte("  \n"), 0o600))

	_, _, err := newMCPServerService(config.Config{}, mcpOptions{
		ServerURL:       "http://127.0.0.1:8080",
		ServerTokenFile: empty,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty")

	_, _, err = newMCPServerService(config.Config{}, mcpOptions{
		ServerURL:       "http://127.0.0.1:8080",
		ServerTokenFile: filepath.Join(dir, "missing"),
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--server-token-file")
}

// --- QA9: postgres backend --------------------------------------------

func TestPhase21MCPPGRequiresAConfiguredURLAndNeverFallsBack(t *testing.T) {
	dataDir := t.TempDir()
	cfg := config.Config{
		DataDir: dataDir,
		DBPath:  filepath.Join(dataDir, "sessions.db"),
	}
	svc, cleanup, err := newMCPPGService(cfg)
	require.Error(t, err,
		"--pg with no url must fail, not silently read SQLite")
	assert.Nil(t, svc)
	assert.Nil(t, cleanup)
	assert.Contains(t, err.Error(), "PostgreSQL")

	entries, err := os.ReadDir(dataDir)
	require.NoError(t, err)
	assert.Empty(t, entries, "the local archive must not be touched")
}

// --- QA9: Phase 17 pairwise forwarding --------------------------------

func TestPhase21MCPPairwiseComparisonIsForwardedWhole(t *testing.T) {
	backend := &phase21MCPFakeService{}
	svc := &mcpLocalService{
		cfg:  config.Config{DataDir: t.TempDir()},
		find: func(config.Config) *DaemonRuntime { return phase21MCPRuntime(18081) },
		newBackend: func(string, string) service.SessionService {
			return backend
		},
		start: func(config.Config) (daemonStartResult, error) {
			t.Fatal("a live daemon must not be restarted")
			return daemonStartResult{}, nil
		},
	}

	req := service.UsagePairwiseComparisonRequest{
		UsageRequest: service.UsageRequest{
			From:            "2026-08-01",
			To:              "2026-08-18",
			Timezone:        "Asia/Shanghai",
			Project:         "alpha",
			Machine:         "laptop",
			GitBranch:       "main",
			MinUserMessages: 3,
		},
		LeftDimension:  service.PairwiseDimensionModel,
		LeftValue:      "claude-opus-5",
		RightDimension: service.PairwiseDimensionProject,
		RightValue:     "alpha",
	}
	_, err := svc.UsagePairwiseComparison(context.Background(), req)
	require.NoError(t, err)
	require.Len(t, backend.pairwise, 1)
	assert.Equal(t, req, backend.pairwise[0],
		"the Phase 17 comparison must cross the seam unchanged")
}
