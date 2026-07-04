package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

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
