package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.kenn.io/agentsview/internal/config"
	"go.kenn.io/agentsview/internal/db"
)

func TestRunRemoteHosts_AttemptsAllAndCollectsFailures(t *testing.T) {
	hosts := []config.RemoteHost{
		{Host: "alpha"},
		{Host: "beta", User: "u", Port: 2222},
		{Host: "gamma"},
	}
	failBeta := errors.New("ssh down")

	var attempted []config.RemoteHost
	failures := runRemoteHosts(hosts, true, func(rh config.RemoteHost, full bool) error {
		attempted = append(attempted, rh)
		assert.True(t, full, "full flag should propagate to syncFn")
		if rh.Host == "beta" {
			return failBeta
		}
		return nil
	})

	// Every host attempted, in declared order, even after a failure.
	require.Equal(t, hosts, attempted)
	// Only beta failed; its full RemoteHost (user/port) is preserved.
	require.Len(t, failures, 1)
	assert.Equal(t, hosts[1], failures[0].Host)
	assert.Equal(t, failBeta, failures[0].Err)
}

func TestFullSyncAuditDue(t *testing.T) {
	now := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name string
		last time.Time
		want bool
	}{
		{name: "first periodic pass", want: true},
		{name: "within daily interval", last: now.Add(-time.Hour), want: false},
		{name: "daily audit due", last: now.Add(-fullSyncAuditInterval), want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, fullSyncAuditDue(tt.last, now))
		})
	}
}

func TestRunRemoteHosts_AllSucceedReturnsEmpty(t *testing.T) {
	hosts := []config.RemoteHost{{Host: "alpha"}, {Host: "beta"}}
	failures := runRemoteHosts(hosts, false, func(config.RemoteHost, bool) error {
		return nil
	})
	assert.Empty(t, failures)
}

func TestSyncLocalAndRemotes_ResyncForcesRemoteFull(t *testing.T) {
	tests := []struct {
		name      string
		cfgFull   bool
		didResync bool
		wantFull  bool
	}{
		{"no full, no resync", false, false, false},
		{"automatic resync forces remote full", false, true, true},
		{"cli --full", true, false, true},
		{"both", true, true, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hosts := []config.RemoteHost{{Host: "alpha"}, {Host: "beta"}}
			localCalled := false
			var gotFull []bool
			failures := syncLocalAndRemotes(hosts, tt.cfgFull,
				func() bool { localCalled = true; return tt.didResync },
				func(_ config.RemoteHost, full bool) error {
					gotFull = append(gotFull, full)
					return nil
				})

			require.True(t, localCalled, "local sync must run")
			assert.Empty(t, failures)
			require.Len(t, gotFull, len(hosts))
			for _, full := range gotFull {
				assert.Equal(t, tt.wantFull, full)
			}
		})
	}
}

func TestSyncLocalAndReferences_RunsReferenceSyncAfterLocalSync(t *testing.T) {
	var calls []string
	didResync := syncLocalAndReferences(
		func() bool { calls = append(calls, "local"); return true },
		func() { calls = append(calls, "references") },
	)

	assert.True(t, didResync)
	assert.Equal(t, []string{"local", "references"}, calls)
}

func TestSyncAssistMemOnceMirrorsLedgerIntoMemoryTable(t *testing.T) {
	dataDir := t.TempDir()
	ledgerPath := filepath.Join(dataDir, "entries.jsonl")
	content := `{"created_at":"2026-07-03T15:26:16Z","evidence":"user explicitly asked to remember this git workflow rule","id":"213307d78f007581","project":"Beacon","scope":"project","source":"explicit","status":"active","text":"For the Beacon project, direct push to main is allowed.","triggers":["push main","git push"],"type":"preference"}` + "\n"
	require.NoError(t, os.WriteFile(ledgerPath, []byte(content), 0o644))

	database, err := db.Open(filepath.Join(dataDir, "agentsview.db"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, database.Close()) })

	ok := syncAssistMemOnce(context.Background(), config.Config{AssistMemLedger: ledgerPath}, database)
	require.True(t, ok)

	got, err := database.ListMemories(context.Background(), db.MemoryFilter{Source: db.SourceAssistMem})
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "assist-mem/213307d78f007581.jsonl", got[0].RelPath)
	assert.Equal(t, "Beacon", got[0].OriginProject)
	assert.Contains(t, got[0].Body, "direct push to main")
}

func TestStartAssistMemSyncMirrorsLedgerChangesWithoutWaitingForPeriodicSync(t *testing.T) {
	dataDir := t.TempDir()
	ledgerPath := filepath.Join(dataDir, "entries.jsonl")
	first := `{"created_at":"2026-07-10T15:17:35Z","id":"first","scope":"global","source":"explicit","status":"active","text":"first memory","type":"entrypoint"}` + "\n"
	require.NoError(t, os.WriteFile(ledgerPath, []byte(first), 0o644))

	database, err := db.Open(filepath.Join(dataDir, "agentsview.db"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, database.Close()) })

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	stop := startAssistMemSync(ctx, config.Config{AssistMemLedger: ledgerPath}, database)
	t.Cleanup(stop)

	second := `{"created_at":"2026-07-10T15:18:35Z","id":"second","scope":"global","source":"explicit","status":"active","text":"second memory","type":"entrypoint"}` + "\n"
	file, err := os.OpenFile(ledgerPath, os.O_APPEND|os.O_WRONLY, 0o644)
	require.NoError(t, err)
	_, err = file.WriteString(second)
	require.NoError(t, err)
	require.NoError(t, file.Close())

	require.Eventually(t, func() bool {
		got, listErr := database.ListMemories(context.Background(), db.MemoryFilter{Source: db.SourceAssistMem})
		return listErr == nil && len(got) == 2
	}, 2*time.Second, 20*time.Millisecond)
}

func TestStartAssistMemSyncWatchesLedgerCreatedAfterStartup(t *testing.T) {
	dataDir := t.TempDir()
	ledgerDir := filepath.Join(dataDir, "memory", "ledger")
	ledgerPath := filepath.Join(ledgerDir, "entries.jsonl")

	database, err := db.Open(filepath.Join(dataDir, "agentsview.db"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, database.Close()) })

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	stop := startAssistMemSync(ctx, config.Config{AssistMemLedger: ledgerPath}, database)
	t.Cleanup(stop)

	entry := `{"created_at":"2026-07-10T15:17:35Z","id":"first","scope":"global","source":"explicit","status":"active","text":"first memory","type":"entrypoint"}` + "\n"
	require.NoError(t, os.MkdirAll(ledgerDir, 0o755))
	require.NoError(t, os.WriteFile(ledgerPath, []byte(entry), 0o644))

	require.Eventually(t, func() bool {
		got, listErr := database.ListMemories(context.Background(), db.MemoryFilter{Source: db.SourceAssistMem})
		return listErr == nil && len(got) == 1
	}, 2*time.Second, 20*time.Millisecond)

	second := `{"created_at":"2026-07-10T15:18:35Z","id":"second","scope":"global","source":"explicit","status":"active","text":"second memory","type":"entrypoint"}` + "\n"
	file, err := os.OpenFile(ledgerPath, os.O_APPEND|os.O_WRONLY, 0o644)
	require.NoError(t, err)
	_, err = file.WriteString(second)
	require.NoError(t, err)
	require.NoError(t, file.Close())

	require.Eventually(t, func() bool {
		got, listErr := database.ListMemories(context.Background(), db.MemoryFilter{Source: db.SourceAssistMem})
		return listErr == nil && len(got) == 2
	}, 2*time.Second, 20*time.Millisecond)
}

func TestStartAssistMemSyncRearmsAfterLedgerDirectoryRename(t *testing.T) {
	dataDir := t.TempDir()
	memoryDir := filepath.Join(dataDir, "memory")
	ledgerDir := filepath.Join(memoryDir, "ledger")
	ledgerPath := filepath.Join(ledgerDir, "entries.jsonl")
	relocatedDir := filepath.Join(memoryDir, "ledger-old")
	require.NoError(t, os.MkdirAll(ledgerDir, 0o755))

	first := `{"created_at":"2026-07-10T15:17:35Z","id":"first","scope":"global","source":"explicit","status":"active","text":"first memory","type":"entrypoint"}` + "\n"
	require.NoError(t, os.WriteFile(ledgerPath, []byte(first), 0o644))

	database, err := db.Open(filepath.Join(dataDir, "agentsview.db"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, database.Close()) })

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	rearmed := make(chan string, 4)
	stop := startAssistMemSyncWithRearmHook(
		ctx, config.Config{AssistMemLedger: ledgerPath}, database,
		func(root string, watched bool) {
			if !watched {
				return
			}
			select {
			case rearmed <- filepath.Clean(root):
			default:
			}
		},
	)
	t.Cleanup(stop)
	require.Equal(t, filepath.Clean(ledgerDir), <-rearmed)

	require.Eventually(t, func() bool {
		got, listErr := database.ListMemories(context.Background(), db.MemoryFilter{Source: db.SourceAssistMem})
		return listErr == nil && len(got) == 1
	}, 2*time.Second, 20*time.Millisecond)

	require.NoError(t, os.Rename(ledgerDir, relocatedDir))
	require.Eventually(t, func() bool {
		select {
		case root := <-rearmed:
			return root == filepath.Clean(memoryDir)
		default:
			return false
		}
	}, 3*time.Second, 20*time.Millisecond)
	require.NoError(t, os.MkdirAll(ledgerDir, 0o755))

	second := `{"created_at":"2026-07-10T15:18:35Z","id":"second","scope":"global","source":"explicit","status":"active","text":"second memory","type":"entrypoint"}` + "\n"
	require.NoError(t, os.WriteFile(ledgerPath, []byte(first+second), 0o644))

	require.Eventually(t, func() bool {
		got, listErr := database.ListMemories(context.Background(), db.MemoryFilter{Source: db.SourceAssistMem})
		return listErr == nil && len(got) == 2
	}, 3*time.Second, 20*time.Millisecond)
}

func TestSerialRunnerDoesNotOverlapAssistMemSyncs(t *testing.T) {
	entered := make(chan struct{}, 2)
	release := make(chan struct{}, 2)
	runner := newSerialRunner(func() {
		entered <- struct{}{}
		<-release
	})

	go runner.Run()
	require.Eventually(t, func() bool { return len(entered) == 1 }, time.Second, time.Millisecond)
	go runner.Run()

	assert.Never(t, func() bool { return len(entered) > 1 }, 100*time.Millisecond, 5*time.Millisecond)
	release <- struct{}{}
	require.Eventually(t, func() bool { return len(entered) == 2 }, time.Second, time.Millisecond)
	release <- struct{}{}
}

func TestSyncAssistMemOnceMirrorsLatestLedgerTopicIntoMemoryTable(t *testing.T) {
	dataDir := t.TempDir()
	ledgerPath := filepath.Join(dataDir, "entries.jsonl")
	content := strings.Join([]string{
		`{"created_at":"2026-07-01T13:36:35Z","id":"older","project":"ordo_ai","scope":"project","source":"explicit","status":"active","text":"old lzn deployment entrypoint","topic":"lzn-deploy-entrypoint","type":"entrypoint"}`,
		`{"created_at":"2026-07-05T11:44:20Z","id":"newer","project":"ordo_ai","scope":"project","source":"explicit","status":"active","text":"new lzn deployment entrypoint","topic":"lzn-deploy-entrypoint","type":"entrypoint"}`,
	}, "\n")
	require.NoError(t, os.WriteFile(ledgerPath, []byte(content), 0o644))

	database, err := db.Open(filepath.Join(dataDir, "agentsview.db"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, database.Close()) })

	ok := syncAssistMemOnce(context.Background(), config.Config{AssistMemLedger: ledgerPath}, database)
	require.True(t, ok)

	got, err := database.ListMemories(context.Background(), db.MemoryFilter{Source: db.SourceAssistMem})
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "assist-mem/newer.jsonl", got[0].RelPath)
	assert.Contains(t, got[0].Body, "new lzn deployment entrypoint")
}

func TestSyncRawMemorySourcesMissingEmbedConfigMirrorLexicalRows(t *testing.T) {
	assertRawMemorySourcesLexicalFallback(t, config.LLMConfig{
		Enabled: true,
		Embed: config.LLMEmbedConfig{
			BaseURL: "https://embed.example.invalid/v1",
			// Missing model means embedding sync must be disabled.
		},
	})
}

func TestSyncRawMemorySourcesDisabledEmbedConfigMirrorsLexicalRows(t *testing.T) {
	assertRawMemorySourcesLexicalFallback(t, config.LLMConfig{
		Enabled: false,
		Embed: config.LLMEmbedConfig{
			BaseURL: "https://embed.example.invalid/v1",
			Model:   "text-embedding",
		},
	})
}

func assertRawMemorySourcesLexicalFallback(t *testing.T, llmCfg config.LLMConfig) {
	t.Helper()
	dataDir := t.TempDir()
	memoryDir := filepath.Join(dataDir, "memory")
	ccRoot := filepath.Join(dataDir, "cc-projects")
	assistPath := filepath.Join(dataDir, "entries.jsonl")
	require.NoError(t, os.MkdirAll(memoryDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(memoryDir, "entry.md"), []byte("---\ntitle: Entry\nstatus: active\n---\n\ncross body\n"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(ccRoot, "proj", "memory"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(ccRoot, "proj", "memory", "cc.md"), []byte("cc body"), 0o644))
	require.NoError(t, os.WriteFile(assistPath, []byte(`{"created_at":"2026-07-03T15:26:16Z","id":"assist","project":"Beacon","scope":"project","status":"active","text":"assist body","type":"preference"}`+"\n"), 0o644))

	database, err := db.Open(filepath.Join(dataDir, "agentsview.db"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, database.Close()) })

	called := false
	cfg := config.Config{
		MemoryDir:       memoryDir,
		CCMemoryDir:     ccRoot,
		AssistMemLedger: assistPath,
		LLM:             llmCfg,
	}
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		called = true
		return &http.Response{StatusCode: http.StatusInternalServerError, Body: io.NopCloser(bytes.NewBufferString(`{}`))}, nil
	})}

	assert.True(t, syncMemoryOnceWithHTTPClient(context.Background(), cfg, database, client))
	assert.True(t, syncAssistMemOnceWithHTTPClient(context.Background(), cfg, database, client))
	assert.True(t, syncCCMemoryOnceWithHTTPClient(context.Background(), cfg, database, client))
	assert.False(t, called, "missing embed model must not call provider during lexical sync")

	for _, source := range []string{db.SourceCrossAgent, db.SourceAssistMem, db.SourceCCNative} {
		got, err := database.ListMemories(context.Background(), db.MemoryFilter{Source: source})
		require.NoError(t, err)
		require.Len(t, got, 1, "source %s should sync lexical rows", source)
	}
}

func TestMemoryResyncerMissingEmbedConfigMirrorsLexicalRows(t *testing.T) {
	dataDir := t.TempDir()
	memoryDir := filepath.Join(dataDir, "memory")
	require.NoError(t, os.MkdirAll(memoryDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(memoryDir, "entry.md"), []byte("---\ntitle: Entry\nstatus: active\n---\n\nresync body\n"), 0o644))
	database, err := db.Open(filepath.Join(dataDir, "agentsview.db"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, database.Close()) })

	called := false
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		called = true
		return &http.Response{StatusCode: http.StatusInternalServerError, Body: io.NopCloser(bytes.NewBufferString(`{}`))}, nil
	})}
	cfg := config.Config{
		LLM: config.LLMConfig{
			Enabled: true,
			Embed:   config.LLMEmbedConfig{BaseURL: "https://embed.example.invalid/v1"},
		},
	}

	require.NoError(t, newMemoryResyncer(memoryDir, database, cfg, client).Resync(context.Background()))
	assert.False(t, called, "background resync helper must share missing-config embedder gate")
	got, err := database.ListMemories(context.Background(), db.MemoryFilter{Source: db.SourceCrossAgent})
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Contains(t, got[0].Body, "resync body")
}

func TestSyncAssistMemOnceConfiguredEmbedderPopulatesEmbedding(t *testing.T) {
	dataDir := t.TempDir()
	ledgerPath := filepath.Join(dataDir, "entries.jsonl")
	content := `{"created_at":"2026-07-03T15:26:16Z","id":"assist-embedded","project":"Beacon","scope":"project","status":"active","text":"assist body","type":"preference"}` + "\n"
	require.NoError(t, os.WriteFile(ledgerPath, []byte(content), 0o644))
	database, err := db.Open(filepath.Join(dataDir, "agentsview.db"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, database.Close()) })

	called := 0
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		called++
		assert.Equal(t, "/v1/embeddings", req.URL.Path)
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(bytes.NewBufferString(`{"data":[{"embedding":[0.2,0.8]}]}`))}, nil
	})}
	cfg := config.Config{
		AssistMemLedger: ledgerPath,
		LLM: config.LLMConfig{
			Enabled: true,
			Embed:   config.LLMEmbedConfig{BaseURL: "https://embed.example.test/v1", Model: "text-embedding"},
		},
	}

	assert.True(t, syncAssistMemOnceWithHTTPClient(context.Background(), cfg, database, client))
	got, err := database.MemoryEmbeddings(context.Background(), db.MemoryFilter{Source: db.SourceAssistMem})
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, []float32{0.2, 0.8}, got[0].LLMEmbedding)
	assert.Equal(t, 2, got[0].LLMEmbeddingDim)
	assert.Equal(t, 1, called)
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}
