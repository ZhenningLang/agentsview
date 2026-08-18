package mcp_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.kenn.io/agentsview/internal/db"
	"go.kenn.io/agentsview/internal/dbtest"
	"go.kenn.io/agentsview/internal/mcp"
	"go.kenn.io/agentsview/internal/service"
)

// phase21Now is the fixed clock every test in this file runs against,
// so "is this session still active" is decided by seeded timestamps
// rather than by when the suite happens to run.
var phase21Now = time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC)

func phase21Clock() time.Time { return phase21Now }

// phase21Stamp formats an offset from the fixed clock the way the
// archive stores timestamps.
func phase21Stamp(d time.Duration) string {
	return phase21Now.Add(d).UTC().Format(time.RFC3339Nano)
}

// phase21Connect starts a real MCP session over the SDK's in-memory
// transport pair: the client performs a genuine initialize handshake
// and every assertion below travels the protocol, so a handler that is
// written but never registered fails here rather than passing a direct
// function call.
func phase21Connect(
	t *testing.T, d *db.DB, opts *mcp.Options,
) *mcpsdk.ClientSession {
	t.Helper()
	if opts == nil {
		opts = &mcp.Options{}
	}
	if opts.Now == nil {
		opts.Now = phase21Clock
	}
	srv := mcp.NewServer(service.NewReadOnlyBackend(d), opts)

	ctx := context.Background()
	clientTransport, serverTransport := mcpsdk.NewInMemoryTransports()
	serverSession, err := srv.Connect(ctx, serverTransport, nil)
	require.NoError(t, err, "connecting MCP server")
	t.Cleanup(func() { _ = serverSession.Close() })

	client := mcpsdk.NewClient(
		&mcpsdk.Implementation{Name: "phase21-test", Version: "test"}, nil,
	)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	require.NoError(t, err, "connecting MCP client")
	t.Cleanup(func() { _ = clientSession.Close() })

	init := clientSession.InitializeResult()
	require.NotNil(t, init, "initialize result")
	require.Equal(t, "agentsview", init.ServerInfo.Name)
	return clientSession
}

// phase21Call calls a tool and requires it to succeed, decoding the
// structured result into out.
func phase21Call(
	t *testing.T, cs *mcpsdk.ClientSession,
	name string, args map[string]any, out any,
) {
	t.Helper()
	res := phase21RawCall(t, cs, name, args)
	require.False(t, res.IsError, "tool %s: %s", name, phase21Text(res))
	require.NotNil(t, res.StructuredContent, "tool %s structured content", name)
	raw, err := json.Marshal(res.StructuredContent)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(raw, out), "decoding %s result", name)
}

func phase21RawCall(
	t *testing.T, cs *mcpsdk.ClientSession,
	name string, args map[string]any,
) *mcpsdk.CallToolResult {
	t.Helper()
	res, err := cs.CallTool(context.Background(), &mcpsdk.CallToolParams{
		Name:      name,
		Arguments: args,
	})
	require.NoError(t, err, "calling %s", name)
	require.NotNil(t, res)
	return res
}

// phase21Text joins the textual content blocks of a result.
func phase21Text(res *mcpsdk.CallToolResult) string {
	var b strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(*mcpsdk.TextContent); ok {
			b.WriteString(tc.Text)
		}
	}
	return b.String()
}

// --- fixtures ----------------------------------------------------------

type phase21SearchOut struct {
	Query          string `json:"query"`
	Count          int    `json:"count"`
	ExcludedActive int    `json:"excluded_active"`
	NextCursor     int    `json:"next_cursor"`
	Results        []struct {
		SessionID string `json:"session_id"`
		Project   string `json:"project"`
		Snippet   string `json:"snippet"`
	} `json:"results"`
}

type phase21ListOut struct {
	Count          int `json:"count"`
	ExcludedActive int `json:"excluded_active"`
	Sessions       []struct {
		ID           string `json:"id"`
		Project      string `json:"project"`
		Agent        string `json:"agent"`
		MessageCount int    `json:"message_count"`
	} `json:"sessions"`
}

type phase21MessageOut struct {
	Ordinal          int    `json:"ordinal"`
	Role             string `json:"role"`
	Content          string `json:"content"`
	ContentTruncated bool   `json:"content_truncated"`
	IsSystem         bool   `json:"is_system"`
}

type phase21MessagesOut struct {
	SessionID string              `json:"session_id"`
	Count     int                 `json:"count"`
	Scanned   int                 `json:"scanned"`
	NextFrom  *int                `json:"next_from"`
	Messages  []phase21MessageOut `json:"messages"`
}

type phase21OverviewOut struct {
	Session struct {
		ID           string `json:"id"`
		Project      string `json:"project"`
		MessageCount int    `json:"message_count"`
	} `json:"session"`
	Head []phase21MessageOut `json:"head"`
	Tail []phase21MessageOut `json:"tail"`
}

type phase21ToolCallsOut struct {
	SessionID string `json:"session_id"`
	Count     int    `json:"count"`
	Matched   int    `json:"matched"`
	ToolCalls []struct {
		Ordinal        int    `json:"ordinal"`
		ToolName       string `json:"tool_name"`
		Category       string `json:"category"`
		Input          string `json:"input"`
		InputTruncated bool   `json:"input_truncated"`
	} `json:"tool_calls"`
}

type phase21UsageOut struct {
	From   string `json:"from"`
	To     string `json:"to"`
	Totals struct {
		InputTokens  int     `json:"inputTokens"`
		OutputTokens int     `json:"outputTokens"`
		TotalCost    float64 `json:"totalCost"`
	} `json:"totals"`
	SessionCounts struct {
		Total     int            `json:"total"`
		ByProject map[string]int `json:"byProject"`
	} `json:"session_counts"`
	ProjectTotals []struct {
		Project     string `json:"project"`
		InputTokens int    `json:"inputTokens"`
	} `json:"project_totals"`
	ModelTotals []struct {
		Model string `json:"model"`
	} `json:"model_totals"`
	Daily []struct {
		Date string `json:"date"`
	} `json:"daily"`
}

// phase21SeedSession creates a session ending at the given offset from
// the fixed clock, with the given messages.
func phase21SeedSession(
	t *testing.T, d *db.DB, id, project string,
	endedOffset time.Duration, msgs ...db.Message,
) {
	t.Helper()
	started := phase21Stamp(endedOffset - time.Hour)
	ended := phase21Stamp(endedOffset)
	dbtest.SeedSession(t, d, id, project, func(s *db.Session) {
		s.StartedAt = &started
		s.EndedAt = &ended
		s.MessageCount = len(msgs)
		s.Agent = "claude"
	})
	if len(msgs) == 0 {
		return
	}
	for i := range msgs {
		msgs[i].SessionID = id
		if msgs[i].Timestamp == "" {
			msgs[i].Timestamp = started
		}
	}
	dbtest.SeedMessages(t, d, msgs...)
}

func phase21UserMsg(ordinal int, content string) db.Message {
	m := dbtest.UserMsg("", ordinal, content)
	return m
}

func phase21AsstMsg(ordinal int, content string) db.Message {
	return dbtest.AsstMsg("", ordinal, content)
}

// --- QA8: tool inventory ----------------------------------------------

func TestPhase21MCPToolsInventoryIsExactlySixReadOnlyTools(t *testing.T) {
	d := dbtest.OpenTestDB(t)
	phase21SeedSession(t, d, "sess-old", "alpha", -48*time.Hour,
		phase21UserMsg(0, "how do I fix the parser"),
		phase21AsstMsg(1, "run the parser tests"),
	)
	cs := phase21Connect(t, d, nil)

	list, err := cs.ListTools(context.Background(), nil)
	require.NoError(t, err)

	names := make([]string, 0, len(list.Tools))
	for _, tool := range list.Tools {
		names = append(names, tool.Name)
	}
	assert.ElementsMatch(t, mcp.ToolNames(), names,
		"registered tools must match the exported inventory")
	require.Len(t, list.Tools, 6, "the MCP surface is exactly six tools")

	mutating := []string{
		"sync", "delete", "write", "update", "create", "push",
		"import", "prune", "resume", "set", "remove", "enrich",
	}
	for _, tool := range list.Tools {
		require.NotNil(t, tool.Annotations, "tool %s annotations", tool.Name)
		assert.True(t, tool.Annotations.ReadOnlyHint,
			"tool %s must be annotated read-only", tool.Name)
		assert.NotEmpty(t, tool.Description, "tool %s description", tool.Name)
		assert.NotNil(t, tool.InputSchema, "tool %s input schema", tool.Name)
		for _, verb := range mutating {
			assert.NotContains(t, tool.Name, verb,
				"read-only server must not expose a mutating tool")
		}
	}
}

func TestPhase21MCPToolsAllSixAnswerACall(t *testing.T) {
	d := dbtest.OpenTestDB(t)
	phase21SeedSession(t, d, "sess-old", "alpha", -48*time.Hour,
		phase21UserMsg(0, "how do I fix the parser"),
		phase21AsstMsg(1, "run the parser tests"),
	)
	cs := phase21Connect(t, d, nil)

	calls := []struct {
		name string
		args map[string]any
	}{
		{mcp.ToolSearchSessions, map[string]any{"query": "parser"}},
		{mcp.ToolListSessions, map[string]any{}},
		{mcp.ToolGetSessionOverview, map[string]any{"session_id": "sess-old"}},
		{mcp.ToolGetSessionMessages, map[string]any{"session_id": "sess-old"}},
		{mcp.ToolListSessionToolCalls, map[string]any{"session_id": "sess-old"}},
		{mcp.ToolGetUsageSummary, map[string]any{
			"from": "2026-08-01", "to": "2026-08-18",
		}},
	}
	require.Len(t, calls, len(mcp.ToolNames()))
	for _, c := range calls {
		t.Run(c.name, func(t *testing.T) {
			res := phase21RawCall(t, cs, c.name, c.args)
			require.False(t, res.IsError, "%s: %s", c.name, phase21Text(res))
			assert.NotNil(t, res.StructuredContent)
		})
	}
}

// --- QA8: search -------------------------------------------------------

func TestPhase21MCPSearchMatchesPunctuationLiterally(t *testing.T) {
	d := dbtest.OpenTestDB(t)
	phase21SeedSession(t, d, "sess-punct", "alpha", -48*time.Hour,
		phase21UserMsg(0, `error: can't parse (config.toml)`),
		phase21AsstMsg(1, "check the quotes"),
	)
	cs := phase21Connect(t, d, nil)

	var out phase21SearchOut
	phase21Call(t, cs, mcp.ToolSearchSessions, map[string]any{
		"query": `can't parse (config.toml)`,
	}, &out)
	require.Len(t, out.Results, 1,
		"a query with FTS punctuation must not be a syntax error")
	assert.Equal(t, "sess-punct", out.Results[0].SessionID)
	assert.Equal(t, `can't parse (config.toml)`, out.Query,
		"the echoed query is the caller's text, not the FTS form")
}

func TestPhase21MCPSearchExcludesTheCallersOwnActiveSession(t *testing.T) {
	d := dbtest.OpenTestDB(t)
	phase21SeedSession(t, d, "sess-live", "alpha", -1*time.Minute,
		phase21UserMsg(0, "investigate the flaky sync"),
	)
	phase21SeedSession(t, d, "sess-done", "alpha", -3*time.Hour,
		phase21UserMsg(0, "investigate the flaky sync"),
	)
	cs := phase21Connect(t, d, nil)

	var out phase21SearchOut
	phase21Call(t, cs, mcp.ToolSearchSessions, map[string]any{
		"query": "flaky sync",
	}, &out)
	require.Len(t, out.Results, 1)
	assert.Equal(t, "sess-done", out.Results[0].SessionID)
	assert.Equal(t, 1, out.ExcludedActive)

	var all phase21SearchOut
	phase21Call(t, cs, mcp.ToolSearchSessions, map[string]any{
		"query":          "flaky sync",
		"include_active": true,
	}, &all)
	ids := make([]string, 0, len(all.Results))
	for _, r := range all.Results {
		ids = append(ids, r.SessionID)
	}
	assert.ElementsMatch(t, []string{"sess-live", "sess-done"}, ids)
	assert.Equal(t, 0, all.ExcludedActive)
}

func TestPhase21MCPSearchClampsLimitAndRejectsBadInput(t *testing.T) {
	d := dbtest.OpenTestDB(t)
	for _, id := range []string{"s1", "s2", "s3"} {
		phase21SeedSession(t, d, id, "alpha", -48*time.Hour,
			phase21UserMsg(0, "shared needle term"),
		)
	}
	cs := phase21Connect(t, d, nil)

	var out phase21SearchOut
	phase21Call(t, cs, mcp.ToolSearchSessions, map[string]any{
		"query": "shared needle term",
		"limit": 2,
	}, &out)
	assert.Len(t, out.Results, 2, "an explicit limit is honored")

	// A limit past the ceiling is folded down, not rejected and not
	// forwarded: the store would otherwise reset it to its own default.
	var wide phase21SearchOut
	phase21Call(t, cs, mcp.ToolSearchSessions, map[string]any{
		"query": "shared needle term",
		"limit": 10000,
	}, &wide)
	assert.Len(t, wide.Results, 3)

	empty := phase21RawCall(t, cs, mcp.ToolSearchSessions,
		map[string]any{"query": "   "})
	assert.True(t, empty.IsError, "an empty query is a tool error")
	assert.Contains(t, phase21Text(empty), "query required")

	badSort := phase21RawCall(t, cs, mcp.ToolSearchSessions,
		map[string]any{"query": "needle", "sort": "sideways"})
	assert.True(t, badSort.IsError)
	assert.Contains(t, phase21Text(badSort), "invalid sort")
}

// --- QA8: list ---------------------------------------------------------

func TestPhase21MCPListFiltersAndExcludesActiveSessions(t *testing.T) {
	d := dbtest.OpenTestDB(t)
	phase21SeedSession(t, d, "alpha-1", "alpha", -30*time.Hour,
		phase21UserMsg(0, "alpha work"))
	phase21SeedSession(t, d, "alpha-2", "alpha", -20*time.Hour,
		phase21UserMsg(0, "more alpha work"))
	phase21SeedSession(t, d, "beta-1", "beta", -10*time.Hour,
		phase21UserMsg(0, "beta work"))
	phase21SeedSession(t, d, "alpha-live", "alpha", -2*time.Minute,
		phase21UserMsg(0, "in flight"))
	cs := phase21Connect(t, d, nil)

	var out phase21ListOut
	phase21Call(t, cs, mcp.ToolListSessions,
		map[string]any{"project": "alpha"}, &out)
	ids := make([]string, 0, len(out.Sessions))
	for _, s := range out.Sessions {
		ids = append(ids, s.ID)
	}
	assert.ElementsMatch(t, []string{"alpha-1", "alpha-2"}, ids)
	assert.Equal(t, 1, out.ExcludedActive)
	assert.Equal(t, len(out.Sessions), out.Count)

	var withActive phase21ListOut
	phase21Call(t, cs, mcp.ToolListSessions, map[string]any{
		"project":        "alpha",
		"include_active": true,
	}, &withActive)
	assert.Len(t, withActive.Sessions, 3)

	// Withholding happens after the store paged, so a page can come
	// back shorter than the limit - here empty, because the single row
	// the store returned was the live session. excluded_active is what
	// tells the caller the archive is not empty, so it is asserted
	// rather than tolerated.
	var limited phase21ListOut
	phase21Call(t, cs, mcp.ToolListSessions, map[string]any{
		"limit": 1,
	}, &limited)
	assert.Empty(t, limited.Sessions)
	assert.Equal(t, 1, limited.ExcludedActive)

	var limitedTwo phase21ListOut
	phase21Call(t, cs, mcp.ToolListSessions, map[string]any{
		"limit": 2,
	}, &limitedTwo)
	assert.Len(t, limitedTwo.Sessions, 1)
	assert.Equal(t, "beta-1", limitedTwo.Sessions[0].ID)
}

func TestPhase21MCPListIncludesOneShotSessionsByDefault(t *testing.T) {
	d := dbtest.OpenTestDB(t)
	// One user message and nothing else: the web UI hides this as a
	// one-shot, and every session seeded above is one too.
	phase21SeedSession(t, d, "one-shot", "alpha", -30*time.Hour,
		phase21UserMsg(0, "single prompt"))
	cs := phase21Connect(t, d, nil)

	var out phase21ListOut
	phase21Call(t, cs, mcp.ToolListSessions, map[string]any{}, &out)
	require.Len(t, out.Sessions, 1,
		"a session a client can read by id must appear in the listing")
	assert.Equal(t, "one-shot", out.Sessions[0].ID)

	var hidden phase21ListOut
	phase21Call(t, cs, mcp.ToolListSessions, map[string]any{
		"include_one_shot": false,
	}, &hidden)
	assert.Empty(t, hidden.Sessions, "the UI default is still reachable")
}

func TestPhase21MCPListToolCallsFiltersAndTruncatesInput(t *testing.T) {
	d := dbtest.OpenTestDB(t)
	long := strings.Repeat("参数", 900) // 1800 runes, past the 1000 cap
	msg := phase21AsstMsg(1, "running tools")
	msg.HasToolUse = true
	msg.ToolCalls = []db.ToolCall{
		{
			ToolName:  "Bash",
			Category:  "execution",
			ToolUseID: "tu-1",
			InputJSON: `{"command":"go test ./..."}`,
		},
		{
			ToolName:            "Read",
			Category:            "filesystem",
			ToolUseID:           "tu-2",
			InputJSON:           `{"note":"` + long + `"}`,
			ResultContentLength: 42,
		},
	}
	phase21SeedSession(t, d, "sess-tools", "alpha", -5*time.Hour,
		phase21UserMsg(0, "please run the tests"), msg)
	cs := phase21Connect(t, d, nil)

	var out phase21ToolCallsOut
	phase21Call(t, cs, mcp.ToolListSessionToolCalls,
		map[string]any{"session_id": "sess-tools"}, &out)
	require.Len(t, out.ToolCalls, 2)
	assert.Equal(t, 2, out.Matched)
	assert.Equal(t, 1, out.ToolCalls[0].Ordinal)

	read := out.ToolCalls[1]
	assert.True(t, read.InputTruncated)
	assert.True(t, utf8.ValidString(read.Input),
		"truncation must not split a multi-byte rune")
	assert.Equal(t, 1000, utf8.RuneCountInString(read.Input))

	var filtered phase21ToolCallsOut
	phase21Call(t, cs, mcp.ToolListSessionToolCalls, map[string]any{
		"session_id": "sess-tools",
		"tool_name":  "bash",
	}, &filtered)
	require.Len(t, filtered.ToolCalls, 1)
	assert.Equal(t, "Bash", filtered.ToolCalls[0].ToolName)

	var capped phase21ToolCallsOut
	phase21Call(t, cs, mcp.ToolListSessionToolCalls, map[string]any{
		"session_id": "sess-tools",
		"limit":      1,
	}, &capped)
	assert.Len(t, capped.ToolCalls, 1)
	assert.Equal(t, 2, capped.Matched,
		"matched counts calls past the page so a caller sees the cut")
}

// --- QA8: overview -----------------------------------------------------

func TestPhase21MCPOverviewReturnsTailInChronologicalOrder(t *testing.T) {
	d := dbtest.OpenTestDB(t)
	msgs := make([]db.Message, 0, 12)
	for i := range 12 {
		msgs = append(msgs, phase21UserMsg(i, "turn "+string(rune('a'+i))))
	}
	phase21SeedSession(t, d, "sess-long", "alpha", -6*time.Hour, msgs...)
	cs := phase21Connect(t, d, nil)

	var out phase21OverviewOut
	phase21Call(t, cs, mcp.ToolGetSessionOverview, map[string]any{
		"session_id": "sess-long",
		"head_limit": 3,
		"tail_limit": 3,
	}, &out)

	assert.Equal(t, "sess-long", out.Session.ID)
	assert.Equal(t, "alpha", out.Session.Project)

	require.Len(t, out.Head, 3)
	assert.Equal(t, []int{0, 1, 2}, phase21Ordinals(out.Head))

	require.Len(t, out.Tail, 3)
	assert.Equal(t, []int{9, 10, 11}, phase21Ordinals(out.Tail),
		"the tail is fetched newest-first and must be restored to reading order")
}

func TestPhase21MCPOverviewDropsHeadTailOverlapAndSystemMessages(t *testing.T) {
	d := dbtest.OpenTestDB(t)
	phase21SeedSession(t, d, "sess-short", "alpha", -6*time.Hour,
		phase21UserMsg(0, "first"),
		phase21UserMsg(1, "<command-name>/compact</command-name>"),
		phase21AsstMsg(2, "second"),
	)
	cs := phase21Connect(t, d, nil)

	var out phase21OverviewOut
	phase21Call(t, cs, mcp.ToolGetSessionOverview, map[string]any{
		"session_id": "sess-short",
		"head_limit": 2,
		"tail_limit": 2,
	}, &out)
	assert.Equal(t, []int{0}, phase21Ordinals(out.Head),
		"the system-injected message is filtered out of the head")
	assert.Equal(t, []int{2}, phase21Ordinals(out.Tail),
		"a session shorter than head+tail must not report a message twice")

	var withSystem phase21OverviewOut
	phase21Call(t, cs, mcp.ToolGetSessionOverview, map[string]any{
		"session_id":     "sess-short",
		"head_limit":     2,
		"tail_limit":     2,
		"include_system": true,
	}, &withSystem)
	assert.Equal(t, []int{0, 1}, phase21Ordinals(withSystem.Head))
	require.Len(t, withSystem.Head, 2)
	assert.True(t, withSystem.Head[1].IsSystem)

	missing := phase21RawCall(t, cs, mcp.ToolGetSessionOverview,
		map[string]any{"session_id": "nope"})
	assert.True(t, missing.IsError)
	assert.Contains(t, phase21Text(missing), "not found")
}

func phase21Ordinals(msgs []phase21MessageOut) []int {
	out := make([]int, 0, len(msgs))
	for _, m := range msgs {
		out = append(out, m.Ordinal)
	}
	return out
}

// --- QA8: messages -----------------------------------------------------

func TestPhase21MCPMessagesTreatsOrdinalZeroAsAValue(t *testing.T) {
	d := dbtest.OpenTestDB(t)
	phase21SeedSession(t, d, "sess-page", "alpha", -6*time.Hour,
		phase21UserMsg(0, "zero"),
		phase21AsstMsg(1, "one"),
		phase21UserMsg(2, "two"),
	)
	cs := phase21Connect(t, d, nil)

	// Descending from an explicit ordinal 0 can only return ordinal 0.
	// If "from" collapsed into an unset int the store would start at
	// the newest message and this would return the whole transcript.
	var fromZero phase21MessagesOut
	phase21Call(t, cs, mcp.ToolGetSessionMessages, map[string]any{
		"session_id": "sess-page",
		"from":       0,
		"direction":  "desc",
	}, &fromZero)
	assert.Equal(t, []int{0}, phase21Ordinals(fromZero.Messages))

	var omitted phase21MessagesOut
	phase21Call(t, cs, mcp.ToolGetSessionMessages, map[string]any{
		"session_id": "sess-page",
		"direction":  "desc",
	}, &omitted)
	assert.Equal(t, []int{2, 1, 0}, phase21Ordinals(omitted.Messages),
		"an omitted from still means newest-first")
}

func TestPhase21MCPMessagesPaginatesFromTheRawBoundary(t *testing.T) {
	d := dbtest.OpenTestDB(t)
	// Ordinal 1 is system-injected, so it is filtered out of the
	// response while still being the last row the store scanned.
	phase21SeedSession(t, d, "sess-sys", "alpha", -6*time.Hour,
		phase21UserMsg(0, "real question"),
		phase21UserMsg(1, "<task-notification>agent finished</task-notification>"),
		phase21AsstMsg(2, "real answer"),
	)
	cs := phase21Connect(t, d, nil)

	var page phase21MessagesOut
	phase21Call(t, cs, mcp.ToolGetSessionMessages, map[string]any{
		"session_id": "sess-sys",
		"limit":      2,
	}, &page)
	assert.Equal(t, []int{0}, phase21Ordinals(page.Messages),
		"the system message is filtered from the page")
	assert.Equal(t, 2, page.Scanned)
	assert.Equal(t, 1, page.Count)
	require.NotNil(t, page.NextFrom)
	assert.Equal(t, 2, *page.NextFrom,
		"next_from follows the last scanned row, not the last visible one")

	var next phase21MessagesOut
	phase21Call(t, cs, mcp.ToolGetSessionMessages, map[string]any{
		"session_id": "sess-sys",
		"from":       *page.NextFrom,
		"limit":      2,
	}, &next)
	assert.Equal(t, []int{2}, phase21Ordinals(next.Messages))
	assert.Nil(t, next.NextFrom, "a short page ends the walk")

	var withSystem phase21MessagesOut
	phase21Call(t, cs, mcp.ToolGetSessionMessages, map[string]any{
		"session_id":     "sess-sys",
		"include_system": true,
	}, &withSystem)
	assert.Equal(t, []int{0, 1, 2}, phase21Ordinals(withSystem.Messages))
}

func TestPhase21MCPMessagesTruncatesContentOnRuneBoundaries(t *testing.T) {
	d := dbtest.OpenTestDB(t)
	long := strings.Repeat("汉", 2100)
	phase21SeedSession(t, d, "sess-cjk", "alpha", -6*time.Hour,
		phase21UserMsg(0, long),
	)
	cs := phase21Connect(t, d, nil)

	var out phase21MessagesOut
	phase21Call(t, cs, mcp.ToolGetSessionMessages,
		map[string]any{"session_id": "sess-cjk"}, &out)
	require.Len(t, out.Messages, 1)
	got := out.Messages[0]
	assert.True(t, got.ContentTruncated)
	assert.True(t, utf8.ValidString(got.Content),
		"a byte-wise cut would emit U+FFFD into the client context")
	assert.Equal(t, 2000, utf8.RuneCountInString(got.Content))
	assert.Equal(t, strings.Repeat("汉", 2000), got.Content)
}

func TestPhase21MCPMessagesRejectsBadInput(t *testing.T) {
	d := dbtest.OpenTestDB(t)
	phase21SeedSession(t, d, "sess-page", "alpha", -6*time.Hour,
		phase21UserMsg(0, "zero"))
	cs := phase21Connect(t, d, nil)

	blank := phase21RawCall(t, cs, mcp.ToolGetSessionMessages,
		map[string]any{"session_id": "  "})
	assert.True(t, blank.IsError)
	assert.Contains(t, phase21Text(blank), "session_id is required")

	bad := phase21RawCall(t, cs, mcp.ToolGetSessionMessages, map[string]any{
		"session_id": "sess-page",
		"direction":  "sideways",
	})
	assert.True(t, bad.IsError)
	assert.Contains(t, phase21Text(bad), "invalid direction")
}

// --- QA8: usage --------------------------------------------------------

func TestPhase21MCPUsageSummaryReportsTheServiceReadModel(t *testing.T) {
	d := dbtest.OpenTestDB(t)
	phase21SeedSession(t, d, "usage-1", "alpha", -30*time.Hour,
		phase21UserMsg(0, "alpha usage"))
	phase21SeedSession(t, d, "usage-2", "beta", -30*time.Hour,
		phase21UserMsg(0, "beta usage"))
	phase21SeedUsage(t, d, "usage-1", 100, 50, 0.02)
	phase21SeedUsage(t, d, "usage-2", 200, 75, 0.04)
	cs := phase21Connect(t, d, nil)

	args := map[string]any{
		"from":     "2026-08-17",
		"to":       "2026-08-17",
		"timezone": "UTC",
	}
	var out phase21UsageOut
	phase21Call(t, cs, mcp.ToolGetUsageSummary, args, &out)

	assert.Equal(t, "2026-08-17", out.From)
	assert.Equal(t, "2026-08-17", out.To)
	assert.Equal(t, 300, out.Totals.InputTokens)
	assert.Equal(t, 125, out.Totals.OutputTokens)
	assert.InDelta(t, 0.06, out.Totals.TotalCost, 1e-9)
	assert.Equal(t, 2, out.SessionCounts.Total)
	assert.Equal(t, 1, out.SessionCounts.ByProject["alpha"])
	assert.Len(t, out.ProjectTotals, 2)
	assert.Empty(t, out.Daily, "the day series is opt-in")

	// The tool must report the shared read model rather than folding
	// its own: same request, same numbers as the service backend.
	svc := service.NewReadOnlyBackend(d)
	direct, err := svc.UsageSummary(context.Background(), service.UsageRequest{
		From: "2026-08-17", To: "2026-08-17", Timezone: "UTC",
	})
	require.NoError(t, err)
	assert.Equal(t, direct.Totals.InputTokens, out.Totals.InputTokens)
	assert.Equal(t, direct.Totals.OutputTokens, out.Totals.OutputTokens)
	assert.InDelta(t, direct.Totals.TotalCost, out.Totals.TotalCost, 1e-9)
	assert.Len(t, direct.ProjectTotals, len(out.ProjectTotals))

	var daily phase21UsageOut
	phase21Call(t, cs, mcp.ToolGetUsageSummary, map[string]any{
		"from": "2026-08-17", "to": "2026-08-17",
		"timezone": "UTC", "include_daily": true,
	}, &daily)
	assert.NotEmpty(t, daily.Daily)

	var topped phase21UsageOut
	phase21Call(t, cs, mcp.ToolGetUsageSummary, map[string]any{
		"from": "2026-08-17", "to": "2026-08-17",
		"timezone": "UTC", "top": 1,
	}, &topped)
	assert.Len(t, topped.ProjectTotals, 1,
		"top caps every breakdown so one call cannot flood a context window")

	bad := phase21RawCall(t, cs, mcp.ToolGetUsageSummary,
		map[string]any{"from": "17/08/2026"})
	assert.True(t, bad.IsError)
}

func phase21SeedUsage(
	t *testing.T, d *db.DB, sessionID string,
	input, output int, cost float64,
) {
	t.Helper()
	ordinal := 0
	require.NoError(t, d.ReplaceSessionUsageEvents(sessionID, []db.UsageEvent{{
		SessionID:      sessionID,
		MessageOrdinal: &ordinal,
		Source:         "session",
		Model:          "claude-opus-5",
		InputTokens:    input,
		OutputTokens:   output,
		CostUSD:        &cost,
		CostStatus:     "estimated",
		CostSource:     "pricing",
		OccurredAt:     "2026-08-17T00:00:30Z",
		DedupKey:       "session:" + sessionID,
	}}))
}
