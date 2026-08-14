package main

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.kenn.io/agentsview/internal/db"
)

func TestSessionList_ResumeFiltersToActiveWindow(t *testing.T) {
	dataDir := t.TempDir()
	t.Setenv("AGENTSVIEW_DATA_DIR", dataDir)
	fresh := time.Now().UTC().Add(-2 * time.Minute).Format(time.RFC3339)
	stale := time.Now().UTC().Add(-2 * time.Hour).Format(time.RFC3339)
	seedSessionWithOpts(t, dataDir, "fresh", "proj", func(s *db.Session) {
		s.StartedAt, s.EndedAt = &fresh, &fresh
	})
	seedSessionWithOpts(t, dataDir, "stale", "proj", func(s *db.Session) {
		s.StartedAt, s.EndedAt = &stale, &stale
	})

	out, err := executeCommand(newRootCommand(), "session", "list", "--resume", "--format", "json")
	require.NoError(t, err)
	assertSessionListIDs(t, out, []string{"fresh"})
}

func TestSessionList_ActiveAliasFiltersToActiveWindow(t *testing.T) {
	dataDir := t.TempDir()
	t.Setenv("AGENTSVIEW_DATA_DIR", dataDir)
	fresh := time.Now().UTC().Add(-2 * time.Minute).Format(time.RFC3339)
	stale := time.Now().UTC().Add(-2 * time.Hour).Format(time.RFC3339)
	seedSessionWithOpts(t, dataDir, "fresh", "proj", func(s *db.Session) { s.StartedAt, s.EndedAt = &fresh, &fresh })
	seedSessionWithOpts(t, dataDir, "stale", "proj", func(s *db.Session) { s.StartedAt, s.EndedAt = &stale, &stale })

	out, err := executeCommand(newRootCommand(), "session", "list", "--active", "--format", "json")
	require.NoError(t, err)
	assertSessionListIDs(t, out, []string{"fresh"})
}

func TestSessionList_NoResumeShowsAll(t *testing.T) {
	dataDir := t.TempDir()
	t.Setenv("AGENTSVIEW_DATA_DIR", dataDir)
	seedSession(t, dataDir, "fresh", "proj")
	seedSession(t, dataDir, "stale", "proj")

	out, err := executeCommand(newRootCommand(), "session", "list", "--format", "json")
	require.NoError(t, err)
	assertSessionListIDs(t, out, []string{"fresh", "stale"})
}

func TestSessionList_ResumeRespectsExplicitActiveSince(t *testing.T) {
	dataDir := t.TempDir()
	t.Setenv("AGENTSVIEW_DATA_DIR", dataDir)
	old := time.Now().UTC().Add(-2 * time.Hour).Format(time.RFC3339)
	seedSessionWithOpts(t, dataDir, "old", "proj", func(s *db.Session) { s.StartedAt, s.EndedAt = &old, &old })

	out, err := executeCommand(newRootCommand(), "session", "list", "--resume", "--active-since", time.Now().UTC().Add(-3*time.Hour).Format(time.RFC3339), "--format", "json")
	require.NoError(t, err)
	assertSessionListIDs(t, out, []string{"old"})
}

func assertSessionListIDs(t *testing.T, out string, want []string) {
	t.Helper()
	var got struct {
		Sessions []struct {
			ID string `json:"id"`
		} `json:"sessions"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &got), "stdout = %q", out)
	ids := make([]string, len(got.Sessions))
	for i, s := range got.Sessions {
		ids[i] = s.ID
	}
	assert.ElementsMatch(t, want, ids)
}
