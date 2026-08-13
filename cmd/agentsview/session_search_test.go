package main

import (
	"strings"
	"testing"
	"time"

	"github.com/mattn/go-runewidth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.kenn.io/agentsview/internal/db"
	"go.kenn.io/agentsview/internal/service"
)

func TestSessionSearchFlagValidation(t *testing.T) {
	cmd := newSessionSearchCommand()
	cmd.SetArgs([]string{"needle", "--regex", "--fts"})
	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "mutually exclusive")
}

func TestSessionSearchFTSWithToolSource(t *testing.T) {
	cmd := newSessionSearchCommand()
	cmd.SetArgs([]string{"needle", "--fts", "--in", "tool_result"})
	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "messages only")
}

func TestPrintContentMatchesTableBasic(t *testing.T) {
	res := &service.ContentSearchResult{Matches: []db.ContentMatch{{
		SessionID: "s1", Project: "proj", Location: "tool_input", ToolName: "Bash",
		Ordinal: 7, Timestamp: "2026-08-13T11:00:00Z", Snippet: "hello\nworld",
	}}}
	var out strings.Builder
	require.NoError(t, printContentMatchesHumanAt(&out, res, 0, time.Date(2026, 8, 13, 12, 0, 0, 0, time.UTC)))
	got := out.String()
	assert.Contains(t, got, "ID")
	assert.Contains(t, got, "MATCH")
	assert.Contains(t, got, "AGE")
	assert.Contains(t, got, "PROJECT")
	assert.Contains(t, got, "LOCATION")
	assert.Contains(t, got, "SNIPPET")
	assert.Contains(t, got, "#7")
	assert.Contains(t, got, "1h")
	assert.Contains(t, got, "tool_input:Bash")
	assert.Contains(t, got, "hello world")
}

func TestPrintContentMatchesTableEmptyAndCursor(t *testing.T) {
	var out strings.Builder
	require.NoError(t, printContentMatchesHumanAt(&out, &service.ContentSearchResult{NextCursor: 50}, 0, time.Now()))
	assert.Equal(t, "(no matches)\nMore results: --cursor 50\n", out.String())
}

func TestPrintContentMatchesTableWideRunesAlign(t *testing.T) {
	res := &service.ContentSearchResult{Matches: []db.ContentMatch{
		{SessionID: "s1", Project: "中文项目", Location: "message", Ordinal: 1, Timestamp: "2026-08-13T11:59:00Z", Snippet: "alpha"},
		{SessionID: "s2", Project: "ascii", Location: "message", Ordinal: 2, Timestamp: "2026-08-13T11:58:00Z", Snippet: "beta"},
	}}
	var out strings.Builder
	require.NoError(t, printContentMatchesHumanAt(&out, res, 80, time.Date(2026, 8, 13, 12, 0, 0, 0, time.UTC)))
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	require.Len(t, lines, 3)
	wantCol := displayColumn(lines[1], "message")
	assert.Equal(t, wantCol, displayColumn(lines[2], "message"))
}

func TestPrintContentMatchesTableWideSnippetBudget(t *testing.T) {
	res := &service.ContentSearchResult{Matches: []db.ContentMatch{{
		SessionID: "s1", Project: "中文项目", Location: "message", Ordinal: 1,
		Timestamp: "2026-08-13T11:59:00Z", Snippet: strings.Repeat("界", 80),
	}}}
	var out strings.Builder
	require.NoError(t, printContentMatchesHumanAt(&out, res, 72, time.Date(2026, 8, 13, 12, 0, 0, 0, time.UTC)))
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	require.Len(t, lines, 2)
	assert.LessOrEqual(t, runewidth.StringWidth(lines[1]), 72)
	assert.Contains(t, lines[1], "…")
}

func TestPrintContentMatchesTableLocationCap(t *testing.T) {
	res := &service.ContentSearchResult{Matches: []db.ContentMatch{{
		SessionID: "s1", Project: "proj", Location: "tool_result",
		ToolName: strings.Repeat("tool", 20), Ordinal: 1,
		Timestamp: "2026-08-13T11:59:00Z", Snippet: "needle",
	}}}
	var out strings.Builder
	require.NoError(t, printContentMatchesHumanAt(&out, res, 80, time.Date(2026, 8, 13, 12, 0, 0, 0, time.UTC)))
	line := strings.Split(strings.TrimSpace(out.String()), "\n")[1]
	assert.LessOrEqual(t, runewidth.StringWidth(line), 80)
	assert.Contains(t, line, "…")
}

func TestPrintContentMatchesTableProjectCap(t *testing.T) {
	res := &service.ContentSearchResult{Matches: []db.ContentMatch{{
		SessionID: "s1", Project: strings.Repeat("project", 12), Location: "message",
		Ordinal: 1, Timestamp: "2026-08-13T11:59:00Z", Snippet: "needle",
	}}}
	var out strings.Builder
	require.NoError(t, printContentMatchesHumanAt(&out, res, 80, time.Date(2026, 8, 13, 12, 0, 0, 0, time.UTC)))
	line := strings.Split(strings.TrimSpace(out.String()), "\n")[1]
	assert.LessOrEqual(t, runewidth.StringWidth(line), 80)
	assert.Contains(t, line, "…")
}

func TestPrintContentMatchesTableSnippetFillsWidth(t *testing.T) {
	res := &service.ContentSearchResult{Matches: []db.ContentMatch{{
		SessionID: "s1", Project: "p", Location: "message", Ordinal: 1,
		Timestamp: "2026-08-13T11:59:00Z", Snippet: strings.Repeat("x", 120),
	}}}
	var out strings.Builder
	require.NoError(t, printContentMatchesHumanAt(&out, res, 70, time.Date(2026, 8, 13, 12, 0, 0, 0, time.UTC)))
	line := strings.Split(strings.TrimSpace(out.String()), "\n")[1]
	assert.Equal(t, 70, runewidth.StringWidth(line))
}

func TestPrintContentMatchesTableSnippetExactFit(t *testing.T) {
	res := &service.ContentSearchResult{Matches: []db.ContentMatch{{
		SessionID: "s1", Project: "p", Location: "message", Ordinal: 1,
		Timestamp: "2026-08-13T11:59:00Z", Snippet: "exact",
	}}}
	var out strings.Builder
	require.NoError(t, printContentMatchesHumanAt(&out, res, 80, time.Date(2026, 8, 13, 12, 0, 0, 0, time.UTC)))
	assert.Contains(t, out.String(), "exact")
	assert.NotContains(t, out.String(), "exact…")
}

func TestContentSnippetBudget(t *testing.T) {
	rows := [][]string{{"ID", "MATCH", "AGE", "PROJECT", "LOCATION", "SNIPPET"},
		{"s1", "#1", "1m", "p", "message", strings.Repeat("界", 40)}}
	var out strings.Builder
	writeSearchRows(&out, rows, 64)
	line := strings.Split(strings.TrimSpace(out.String()), "\n")[1]
	assert.LessOrEqual(t, runewidth.StringWidth(line), 64)
}

func TestHumanizeMatchAge(t *testing.T) {
	now := time.Date(2026, 8, 13, 12, 0, 0, 0, time.UTC)
	assert.Equal(t, "now", humanizeMatchAge("2026-08-13T12:00:01Z", now))
	assert.Equal(t, "30s", humanizeMatchAge("2026-08-13T11:59:30Z", now))
	assert.Equal(t, "15m", humanizeMatchAge("2026-08-13T11:45:00Z", now))
	assert.Equal(t, "4h", humanizeMatchAge("2026-08-13T08:00:00Z", now))
	assert.Equal(t, "3d", humanizeMatchAge("2026-08-10T12:00:00Z", now))
	assert.Equal(t, "Jan 02", humanizeMatchAge("2026-01-02T00:00:00Z", now))
	assert.Equal(t, "Jan 2025", humanizeMatchAge("2025-01-02T00:00:00Z", now))
	assert.Equal(t, "-", humanizeMatchAge("", now))
	assert.Equal(t, "-", humanizeMatchAge("bad", now))
}

func TestPrintContentMatchesTableAgeColumn(t *testing.T) {
	res := &service.ContentSearchResult{Matches: []db.ContentMatch{{
		SessionID: "s1", Project: "proj", Location: "message", Ordinal: 1,
		Timestamp: "2026-08-13T11:59:00Z", Snippet: "needle",
	}}}
	var out strings.Builder
	require.NoError(t, printContentMatchesHumanAt(&out, res, 0, time.Date(2026, 8, 13, 12, 0, 0, 0, time.UTC)))
	header := strings.SplitN(out.String(), "\n", 2)[0]
	assert.Less(t, strings.Index(header, "MATCH"), strings.Index(header, "AGE"))
	assert.Less(t, strings.Index(header, "AGE"), strings.Index(header, "PROJECT"))
}

func displayColumn(line, needle string) int {
	i := strings.Index(line, needle)
	if i < 0 {
		return -1
	}
	return runewidth.StringWidth(line[:i])
}
