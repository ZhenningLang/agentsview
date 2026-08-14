package main

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.kenn.io/agentsview/internal/db"
	"go.kenn.io/agentsview/internal/service"
)

func TestSessionAge(t *testing.T) {
	now := time.Date(2026, 8, 13, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name string
		ts   string
		want string
	}{
		{name: "empty", want: "-"},
		{name: "invalid", ts: "yesterday", want: "-"},
		{name: "future", ts: "2026-08-13T12:01:00Z", want: "now"},
		{name: "seconds", ts: "2026-08-13T11:59:30Z", want: "30s"},
		{name: "minutes", ts: "2026-08-13T11:45:00Z", want: "15m"},
		{name: "hours", ts: "2026-08-13T08:00:00Z", want: "4h"},
		{name: "days", ts: "2026-08-10T12:00:00Z", want: "3d"},
		{name: "old same year", ts: "2026-01-02T00:00:00Z", want: "Jan 02"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, humanizeSessionAge(tt.ts, now))
		})
	}
}

func TestSessionActivityTime(t *testing.T) {
	created := "2026-08-13T09:00:00Z"
	started := "2026-08-13T10:00:00Z"
	ended := "2026-08-13T11:00:00Z"
	assert.Equal(t, ended, sessionActivityTime(db.Session{CreatedAt: created, StartedAt: &started, EndedAt: &ended}))
	assert.Equal(t, started, sessionActivityTime(db.Session{CreatedAt: created, StartedAt: &started}))
	assert.Equal(t, created, sessionActivityTime(db.Session{CreatedAt: created}))
}

func TestSessionDisplayName(t *testing.T) {
	name := "  Named session  "
	first := "first message"
	assert.Equal(t, "Named session", sessionDisplayName(db.Session{ID: "id", DisplayName: &name, FirstMessage: &first}))
	assert.Equal(t, "first message", sessionDisplayName(db.Session{ID: "id", FirstMessage: &first}))
	assert.Equal(t, "id", sessionDisplayName(db.Session{ID: "id"}))
}

func TestCollapseHome(t *testing.T) {
	assert.Equal(t, "~/proj", collapseHome("/Users/me/proj", "/Users/me"))
	assert.Equal(t, "/opt/proj", collapseHome("/opt/proj", "/Users/me"))
	assert.Equal(t, "~", collapseHome("/Users/me", "/Users/me"))
	// A cwd recorded on this host collapses too, and the rendered form uses
	// forward slashes whatever the host separator is.
	home := filepath.Join(string(filepath.Separator), "Users", "me")
	assert.Equal(t, "~/proj", collapseHome(filepath.Join(home, "proj"), home))
	assert.Equal(t, "~", collapseHome(home, home))
}

func TestTruncName(t *testing.T) {
	assert.Equal(t, "abcd", truncName("abcd", 4))
	assert.Equal(t, "abc...", truncName("abcdefg", 6))
	assert.Equal(t, "你好...", truncName("你好世界", 7))
}

func TestSessionList_ResumeHumanOutput(t *testing.T) {
	now := time.Date(2026, 8, 13, 12, 0, 0, 0, time.UTC)
	activity := "2026-08-13T11:58:00Z"
	stale := "2026-08-13T11:00:00Z"
	name := "Fresh session"
	list := &service.SessionList{Sessions: []db.Session{
		{ID: "fresh", Project: "proj", Agent: "codex", StartedAt: &activity, EndedAt: &activity, MessageCount: 4, UserMessageCount: 2, DisplayName: &name, Cwd: "/Users/me/proj", GitBranch: "main"},
		{ID: "stale", Project: "p2", Agent: "claude", StartedAt: &stale, EndedAt: &stale, MessageCount: 3, UserMessageCount: 2, Cwd: "/tmp/other"},
	}}
	var out strings.Builder
	assert.NoError(t, printSessionListHumanAt(&out, list, now, "/Users/me"))
	got := out.String()
	for _, want := range []string{"*", "ID", "AGE", "AGENT", "PROJECT", "BRANCH", "MSGS", "NAME", "CWD", "2m", "Fresh session", "~/proj"} {
		assert.Contains(t, got, want)
	}
}

func TestSessionRecentlyActive(t *testing.T) {
	now := time.Date(2026, 8, 13, 12, 0, 0, 0, time.UTC)
	fresh := "2026-08-13T11:50:00Z"
	old := "2026-08-13T11:40:00Z"
	assert.True(t, sessionRecentlyActive(db.Session{EndedAt: &fresh}, now))
	assert.False(t, sessionRecentlyActive(db.Session{EndedAt: &old}, now))
	assert.False(t, sessionRecentlyActive(db.Session{CreatedAt: "bad"}, now))
}
