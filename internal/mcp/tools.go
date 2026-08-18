package mcp

import (
	"context"
	"errors"
	"fmt"
	"strings"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"go.kenn.io/agentsview/internal/db"
	"go.kenn.io/agentsview/internal/service"
)

// errSessionIDRequired is returned before any store work when a tool
// that addresses one session was called without one.
var errSessionIDRequired = errors.New("session_id is required")

// register installs the six read-only tools.
func (s *server) register(srv *mcpsdk.Server) {
	mcpsdk.AddTool(srv, &mcpsdk.Tool{
		Name: ToolSearchSessions,
		Description: "Full-text search across archived agent sessions. " +
			"Returns one best-ranked snippet per matching session. " +
			"The query is matched as a literal phrase, so punctuation " +
			"and quotes are safe to include. Sessions still being " +
			"written are withheld, so a page can be shorter than " +
			"limit; excluded_active says how many.",
		Annotations: readOnly("Search sessions"),
	}, s.handleSearchSessions)

	mcpsdk.AddTool(srv, &mcpsdk.Tool{
		Name: ToolListSessions,
		Description: "List archived agent sessions filtered by project, " +
			"agent, machine, git branch or date range, newest first. " +
			"Sessions still being written are withheld, so a page can " +
			"be shorter than limit; excluded_active says how many.",
		Annotations: readOnly("List sessions"),
	}, s.handleListSessions)

	mcpsdk.AddTool(srv, &mcpsdk.Tool{
		Name: ToolGetSessionOverview,
		Description: "Summarize one session: its metadata plus the first " +
			"and last few messages, both in chronological order. Use " +
			"this before paging a whole transcript.",
		Annotations: readOnly("Session overview"),
	}, s.handleGetSessionOverview)

	mcpsdk.AddTool(srv, &mcpsdk.Tool{
		Name: ToolGetSessionMessages,
		Description: "Read one page of a session transcript. Pass the " +
			"returned next_from back as from to continue; a missing " +
			"next_from means the transcript is exhausted.",
		Annotations: readOnly("Session messages"),
	}, s.handleGetSessionMessages)

	mcpsdk.AddTool(srv, &mcpsdk.Tool{
		Name: ToolListSessionToolCalls,
		Description: "List the tool calls made during one session, in " +
			"transcript order, with their input arguments.",
		Annotations: readOnly("Session tool calls"),
	}, s.handleListSessionToolCalls)

	mcpsdk.AddTool(srv, &mcpsdk.Tool{
		Name: ToolGetUsageSummary,
		Description: "Token and cost usage over a date range, with " +
			"per-project, per-model, per-agent and per-machine totals.",
		Annotations: readOnly("Usage summary"),
	}, s.handleGetUsageSummary)
}

// --- search_sessions ---------------------------------------------------

type searchInput struct {
	Query string `json:"query" jsonschema:"Text to search for. Matched as a literal phrase; punctuation is safe."`
	//nolint:lll // schema descriptions read better unwrapped.
	Project       string `json:"project,omitempty" jsonschema:"Restrict to one project."`
	Sort          string `json:"sort,omitempty" jsonschema:"relevance (default) or recency."`
	Limit         int    `json:"limit,omitempty" jsonschema:"Maximum sessions to return (default 20, maximum 100)."`
	Cursor        int    `json:"cursor,omitempty" jsonschema:"Opaque offset from a previous next_cursor."`
	IncludeActive bool   `json:"include_active,omitempty" jsonschema:"Include sessions still being written, including the caller's own. Off by default."`
}

type searchHit struct {
	SessionID        string  `json:"session_id"`
	Project          string  `json:"project"`
	Agent            string  `json:"agent"`
	Name             string  `json:"name,omitempty"`
	Ordinal          int     `json:"ordinal"`
	EndedAt          string  `json:"ended_at,omitempty"`
	Snippet          string  `json:"snippet"`
	SnippetTruncated bool    `json:"snippet_truncated,omitempty"`
	Rank             float64 `json:"rank"`
}

type searchOutput struct {
	Query          string      `json:"query"`
	Results        []searchHit `json:"results"`
	Count          int         `json:"count"`
	ExcludedActive int         `json:"excluded_active,omitempty"`
	NextCursor     int         `json:"next_cursor,omitempty"`
}

func (s *server) handleSearchSessions(
	ctx context.Context, _ *mcpsdk.CallToolRequest, in searchInput,
) (*mcpsdk.CallToolResult, searchOutput, error) {
	sort := strings.ToLower(strings.TrimSpace(in.Sort))
	switch sort {
	case "", "relevance", "recency":
	default:
		return nil, searchOutput{}, fmt.Errorf(
			"invalid sort %q: use relevance or recency", in.Sort,
		)
	}
	// The query is handed over raw. db.PrepareFTSQuery inside the
	// service is the single place that turns caller text into FTS5
	// syntax; quoting it a second time here would search for the quote
	// characters themselves.
	res, err := s.svc.Search(ctx, service.SearchRequest{
		Query:   in.Query,
		Project: strings.TrimSpace(in.Project),
		Sort:    sort,
		Limit:   clampLimit(in.Limit, defaultSearchLimit, maxSearchLimit),
		Cursor:  in.Cursor,
	})
	if err != nil {
		return nil, searchOutput{}, err
	}
	out := searchOutput{
		Query:      res.Query,
		Results:    make([]searchHit, 0, len(res.Results)),
		NextCursor: res.NextCursor,
	}
	for _, r := range res.Results {
		if !in.IncludeActive && s.isActive(r.SessionEndedAt, "") {
			out.ExcludedActive++
			continue
		}
		snippet, truncated := truncateRunes(r.Snippet, maxSnippetRunes)
		out.Results = append(out.Results, searchHit{
			SessionID:        r.SessionID,
			Project:          r.Project,
			Agent:            r.Agent,
			Name:             r.Name,
			Ordinal:          r.Ordinal,
			EndedAt:          r.SessionEndedAt,
			Snippet:          snippet,
			SnippetTruncated: truncated,
			Rank:             r.Rank,
		})
	}
	out.Count = len(out.Results)
	return nil, out, nil
}

// --- list_sessions -----------------------------------------------------

type listInput struct {
	Project       string `json:"project,omitempty" jsonschema:"Restrict to one project."`
	Agent         string `json:"agent,omitempty" jsonschema:"Restrict to one agent, for example claude or codex."`
	Machine       string `json:"machine,omitempty" jsonschema:"Restrict to one machine."`
	GitBranch     string `json:"git_branch,omitempty" jsonschema:"Restrict to one git branch."`
	DateFrom      string `json:"date_from,omitempty" jsonschema:"Earliest session date, YYYY-MM-DD."`
	DateTo        string `json:"date_to,omitempty" jsonschema:"Latest session date, YYYY-MM-DD."`
	Limit         int    `json:"limit,omitempty" jsonschema:"Maximum sessions to return (default 20, maximum 100)."`
	Cursor        string `json:"cursor,omitempty" jsonschema:"Opaque cursor from a previous next_cursor."`
	IncludeActive bool   `json:"include_active,omitempty" jsonschema:"Include sessions still being written, including the caller's own. Off by default."`
	// IncludeOneShot defaults to true, which is the opposite of the
	// web UI. Hiding single-prompt sessions is a browsing ergonomic:
	// for a machine reader it means a session it can search for and
	// read by id is missing from the listing of its own project, with
	// nothing in the response to say so.
	IncludeOneShot   *bool `json:"include_one_shot,omitempty" jsonschema:"Include single-prompt sessions. On by default."`
	IncludeAutomated bool  `json:"include_automated,omitempty" jsonschema:"Include sessions started by automation. Off by default."`
}

type listOutput struct {
	Sessions       []sessionView `json:"sessions"`
	Count          int           `json:"count"`
	ExcludedActive int           `json:"excluded_active,omitempty"`
	NextCursor     string        `json:"next_cursor,omitempty"`
}

func (s *server) handleListSessions(
	ctx context.Context, _ *mcpsdk.CallToolRequest, in listInput,
) (*mcpsdk.CallToolResult, listOutput, error) {
	res, err := s.svc.List(ctx, service.ListFilter{
		Project:   strings.TrimSpace(in.Project),
		Agent:     strings.TrimSpace(in.Agent),
		Machine:   strings.TrimSpace(in.Machine),
		GitBranch: strings.TrimSpace(in.GitBranch),
		DateFrom:  strings.TrimSpace(in.DateFrom),
		DateTo:    strings.TrimSpace(in.DateTo),
		Limit:     clampLimit(in.Limit, defaultListLimit, maxListLimit),
		Cursor:    strings.TrimSpace(in.Cursor),
		IncludeOneShot: in.IncludeOneShot == nil ||
			*in.IncludeOneShot,
		IncludeAutomated: in.IncludeAutomated,
	})
	if err != nil {
		return nil, listOutput{}, err
	}
	out := listOutput{
		Sessions:   make([]sessionView, 0, len(res.Sessions)),
		NextCursor: res.NextCursor,
	}
	for _, sess := range res.Sessions {
		if !in.IncludeActive &&
			s.isActive(deref(sess.EndedAt), deref(sess.StartedAt)) {
			out.ExcludedActive++
			continue
		}
		out.Sessions = append(out.Sessions, newSessionView(sess))
	}
	out.Count = len(out.Sessions)
	return nil, out, nil
}

// --- get_session_overview ----------------------------------------------

type overviewInput struct {
	SessionID     string `json:"session_id" jsonschema:"Session identifier."`
	HeadLimit     int    `json:"head_limit,omitempty" jsonschema:"How many opening messages to include (default 5, maximum 50)."`
	TailLimit     int    `json:"tail_limit,omitempty" jsonschema:"How many closing messages to include (default 5, maximum 50)."`
	IncludeSystem bool   `json:"include_system,omitempty" jsonschema:"Include system-injected messages. Off by default."`
}

type overviewOutput struct {
	Session sessionView   `json:"session"`
	Head    []messageView `json:"head"`
	Tail    []messageView `json:"tail"`
}

func (s *server) handleGetSessionOverview(
	ctx context.Context, _ *mcpsdk.CallToolRequest, in overviewInput,
) (*mcpsdk.CallToolResult, overviewOutput, error) {
	id := strings.TrimSpace(in.SessionID)
	if id == "" {
		return nil, overviewOutput{}, errSessionIDRequired
	}
	detail, err := s.svc.Get(ctx, id)
	if err != nil {
		return nil, overviewOutput{}, err
	}
	if detail == nil {
		return nil, overviewOutput{}, fmt.Errorf("session %q not found", id)
	}
	headLimit := clampLimit(
		in.HeadLimit, defaultOverviewLimit, maxOverviewLimit,
	)
	tailLimit := clampLimit(
		in.TailLimit, defaultOverviewLimit, maxOverviewLimit,
	)
	start := 0
	head, err := s.svc.Messages(ctx, id, service.MessageFilter{
		From: &start, Limit: headLimit, Direction: "asc",
	})
	if err != nil {
		return nil, overviewOutput{}, err
	}
	// From is left nil so the store starts at the newest message. The
	// page comes back newest-first; an overview is read top to bottom,
	// so it is restored to chronological order before shaping.
	tail, err := s.svc.Messages(ctx, id, service.MessageFilter{
		Limit: tailLimit, Direction: "desc",
	})
	if err != nil {
		return nil, overviewOutput{}, err
	}
	out := overviewOutput{
		Session: newSessionView(detail.Session),
		Head:    messageViews(head.Messages, in.IncludeSystem),
		Tail: messageViews(
			dropOverlap(reverse(tail.Messages), head.Messages),
			in.IncludeSystem,
		),
	}
	return nil, out, nil
}

// reverse returns msgs in the opposite order, without mutating the
// input slice.
func reverse(msgs []db.Message) []db.Message {
	out := make([]db.Message, len(msgs))
	for i, m := range msgs {
		out[len(msgs)-1-i] = m
	}
	return out
}

// dropOverlap removes messages already covered by head. A session
// shorter than head_limit plus tail_limit otherwise reports the same
// messages twice, which reads to a model as a repeated exchange rather
// than as one short session.
func dropOverlap(tail, head []db.Message) []db.Message {
	if len(head) == 0 {
		return tail
	}
	lastHead := head[len(head)-1].Ordinal
	out := make([]db.Message, 0, len(tail))
	for _, m := range tail {
		if m.Ordinal <= lastHead {
			continue
		}
		out = append(out, m)
	}
	return out
}

// --- get_session_messages ----------------------------------------------

type messagesInput struct {
	SessionID string `json:"session_id" jsonschema:"Session identifier."`
	// From is a pointer so an explicit ordinal 0 - the first message of
	// every transcript - is distinguishable from an omitted field. As a
	// plain int the two collapse, and descending paging would restart
	// at the newest message every time the caller asked for ordinal 0.
	From          *int   `json:"from,omitempty" jsonschema:"Ordinal to start at, inclusive. Omit to start at the beginning (ascending) or the end (descending). Ordinal 0 is a valid value."`
	Limit         int    `json:"limit,omitempty" jsonschema:"Maximum messages to return (default 50, maximum 200)."`
	Direction     string `json:"direction,omitempty" jsonschema:"asc (default) or desc."`
	IncludeSystem bool   `json:"include_system,omitempty" jsonschema:"Include system-injected messages. Off by default."`
}

type messagesOutput struct {
	SessionID string        `json:"session_id"`
	Messages  []messageView `json:"messages"`
	Count     int           `json:"count"`
	Scanned   int           `json:"scanned"`
	NextFrom  *int          `json:"next_from,omitempty"`
}

func (s *server) handleGetSessionMessages(
	ctx context.Context, _ *mcpsdk.CallToolRequest, in messagesInput,
) (*mcpsdk.CallToolResult, messagesOutput, error) {
	id := strings.TrimSpace(in.SessionID)
	if id == "" {
		return nil, messagesOutput{}, errSessionIDRequired
	}
	direction := strings.ToLower(strings.TrimSpace(in.Direction))
	switch direction {
	case "", "asc", "desc":
	default:
		return nil, messagesOutput{}, fmt.Errorf(
			"invalid direction %q: use asc or desc", in.Direction,
		)
	}
	limit := clampLimit(in.Limit, defaultMessageLimit, maxMessageLimit)
	page, err := s.svc.Messages(ctx, id, service.MessageFilter{
		From:      in.From,
		Limit:     limit,
		Direction: direction,
	})
	if err != nil {
		return nil, messagesOutput{}, err
	}
	raw := page.Messages
	out := messagesOutput{
		SessionID: id,
		Messages:  messageViews(raw, in.IncludeSystem),
		Scanned:   len(raw),
		NextFrom:  nextFrom(raw, limit, direction != "desc"),
	}
	out.Count = len(out.Messages)
	return nil, out, nil
}

// --- list_session_tool_calls -------------------------------------------

type toolCallsInput struct {
	SessionID string `json:"session_id" jsonschema:"Session identifier."`
	ToolName  string `json:"tool_name,omitempty" jsonschema:"Restrict to one tool name, case-insensitive."`
	Category  string `json:"category,omitempty" jsonschema:"Restrict to one tool category, case-insensitive."`
	Limit     int    `json:"limit,omitempty" jsonschema:"Maximum tool calls to return (default 50, maximum 200)."`
}

type toolCallView struct {
	Ordinal           int    `json:"ordinal"`
	Timestamp         string `json:"timestamp,omitempty"`
	ToolName          string `json:"tool_name"`
	Category          string `json:"category,omitempty"`
	Input             string `json:"input,omitempty"`
	InputTruncated    bool   `json:"input_truncated,omitempty"`
	SkillName         string `json:"skill_name,omitempty"`
	SubagentSessionID string `json:"subagent_session_id,omitempty"`
	ResultLength      int    `json:"result_length,omitempty"`
}

type toolCallsOutput struct {
	SessionID string         `json:"session_id"`
	ToolCalls []toolCallView `json:"tool_calls"`
	Count     int            `json:"count"`
	Matched   int            `json:"matched"`
}

func (s *server) handleListSessionToolCalls(
	ctx context.Context, _ *mcpsdk.CallToolRequest, in toolCallsInput,
) (*mcpsdk.CallToolResult, toolCallsOutput, error) {
	id := strings.TrimSpace(in.SessionID)
	if id == "" {
		return nil, toolCallsOutput{}, errSessionIDRequired
	}
	list, err := s.svc.ToolCalls(ctx, id)
	if err != nil {
		return nil, toolCallsOutput{}, err
	}
	limit := clampLimit(in.Limit, defaultToolCallLimit, maxToolCallLimit)
	wantName := strings.ToLower(strings.TrimSpace(in.ToolName))
	wantCategory := strings.ToLower(strings.TrimSpace(in.Category))
	out := toolCallsOutput{
		SessionID: id,
		ToolCalls: make([]toolCallView, 0, limit),
	}
	for _, c := range list.ToolCalls {
		if wantName != "" && !strings.EqualFold(c.ToolName, wantName) {
			continue
		}
		if wantCategory != "" &&
			!strings.EqualFold(c.Category, wantCategory) {
			continue
		}
		// Matched counts every call that passed the filters, including
		// the ones past the page limit, so a caller can tell a complete
		// answer from a truncated one.
		out.Matched++
		if len(out.ToolCalls) >= limit {
			continue
		}
		input, truncated := truncateRunes(c.InputJSON, maxToolInputRunes)
		out.ToolCalls = append(out.ToolCalls, toolCallView{
			Ordinal:           c.Ordinal,
			Timestamp:         c.Timestamp,
			ToolName:          c.ToolName,
			Category:          c.Category,
			Input:             input,
			InputTruncated:    truncated,
			SkillName:         c.SkillName,
			SubagentSessionID: c.SubagentSessionID,
			ResultLength:      c.ResultLength,
		})
	}
	out.Count = len(out.ToolCalls)
	return nil, out, nil
}

// --- get_usage_summary -------------------------------------------------

type usageInput struct {
	From         string `json:"from,omitempty" jsonschema:"Start date, YYYY-MM-DD. Defaults to 30 days before to."`
	To           string `json:"to,omitempty" jsonschema:"End date, YYYY-MM-DD. Defaults to today."`
	Timezone     string `json:"timezone,omitempty" jsonschema:"IANA timezone used to bucket days, for example Asia/Shanghai."`
	Project      string `json:"project,omitempty" jsonschema:"Restrict to one project."`
	Agent        string `json:"agent,omitempty" jsonschema:"Restrict to one agent."`
	Machine      string `json:"machine,omitempty" jsonschema:"Restrict to one machine."`
	Model        string `json:"model,omitempty" jsonschema:"Restrict to one model."`
	GitBranch    string `json:"git_branch,omitempty" jsonschema:"Restrict to one git branch."`
	Top          int    `json:"top,omitempty" jsonschema:"How many rows to keep in each breakdown (default 10, maximum 50)."`
	IncludeDaily bool   `json:"include_daily,omitempty" jsonschema:"Include the per-day series. Off by default because it is large."`
}

type usageOutput struct {
	From          string                 `json:"from"`
	To            string                 `json:"to"`
	Totals        db.UsageTotals         `json:"totals"`
	SessionCounts db.UsageSessionCounts  `json:"session_counts"`
	CacheStats    service.CacheStats     `json:"cache_stats"`
	ProjectTotals []service.ProjectTotal `json:"project_totals"`
	ModelTotals   []service.ModelTotal   `json:"model_totals"`
	AgentTotals   []service.AgentTotal   `json:"agent_totals"`
	MachineTotals []service.MachineTotal `json:"machine_totals"`
	Daily         []db.DailyUsageEntry   `json:"daily,omitempty"`
	Comparison    *service.Comparison    `json:"comparison,omitempty"`
}

func (s *server) handleGetUsageSummary(
	ctx context.Context, _ *mcpsdk.CallToolRequest, in usageInput,
) (*mcpsdk.CallToolResult, usageOutput, error) {
	res, err := s.svc.UsageSummary(ctx, service.UsageRequest{
		From:      strings.TrimSpace(in.From),
		To:        strings.TrimSpace(in.To),
		Timezone:  strings.TrimSpace(in.Timezone),
		Project:   strings.TrimSpace(in.Project),
		Agent:     strings.TrimSpace(in.Agent),
		Machine:   strings.TrimSpace(in.Machine),
		Model:     strings.TrimSpace(in.Model),
		GitBranch: strings.TrimSpace(in.GitBranch),
	})
	if err != nil {
		return nil, usageOutput{}, err
	}
	// Every number below is copied from the service result. The usage
	// read model lives in internal/service and is shared with the HTTP
	// route; recomputing any of it here would give an MCP client
	// different totals than the UI shows for the same range.
	top := clampLimit(in.Top, defaultUsageTopN, maxUsageTopN)
	out := usageOutput{
		From:          res.From,
		To:            res.To,
		Totals:        res.Totals,
		SessionCounts: res.SessionCounts,
		CacheStats:    res.CacheStats,
		ProjectTotals: topN(res.ProjectTotals, top),
		ModelTotals:   topN(res.ModelTotals, top),
		AgentTotals:   topN(res.AgentTotals, top),
		MachineTotals: topN(res.MachineTotals, top),
		Comparison:    res.Comparison,
	}
	if in.IncludeDaily {
		out.Daily = res.Daily
	}
	if out.SessionCounts.ByProject == nil {
		out.SessionCounts.ByProject = map[string]int{}
	}
	if out.SessionCounts.ByAgent == nil {
		out.SessionCounts.ByAgent = map[string]int{}
	}
	return nil, out, nil
}

// topN keeps the first n rows of an already-ordered breakdown. The
// service sorts these descending by cost, so truncation drops the
// smallest rows. The result is never nil: a nil slice marshals to JSON
// null and fails the tool's output schema.
func topN[T any](rows []T, n int) []T {
	if len(rows) > n {
		rows = rows[:n]
	}
	out := make([]T, 0, len(rows))
	return append(out, rows...)
}
