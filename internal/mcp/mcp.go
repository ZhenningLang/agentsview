// Package mcp exposes the local session archive as a read-only Model
// Context Protocol server.
//
// Every tool in this package is a read: there is no sync trigger, no
// write, no delete. That is not an accident of the current tool list,
// it is the contract. An MCP client is an LLM acting on its own
// judgement; giving it a mutation path into the archive would let a
// prompt injection in someone else's transcript rewrite the archive
// that stores it. All six tools therefore carry ReadOnlyHint, and the
// inventory test asserts both the exact set and the annotation.
//
// The package talks to the archive only through service.SessionService.
// That keeps one read model for the CLI, the HTTP API and MCP, and it
// is what lets the local backend hand in a lazy wrapper that re-resolves
// a daemon per call instead of opening the SQLite file directly.
package mcp

import (
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"go.kenn.io/agentsview/internal/service"
)

// Tool names. These are wire identifiers: an MCP client config pins
// them, so renaming one is a breaking change for every configured
// client, not an internal refactor.
const (
	ToolSearchSessions       = "search_sessions"
	ToolListSessions         = "list_sessions"
	ToolGetSessionOverview   = "get_session_overview"
	ToolGetSessionMessages   = "get_session_messages"
	ToolListSessionToolCalls = "list_session_tool_calls"
	ToolGetUsageSummary      = "get_usage_summary"
)

// ServerName is the MCP implementation name reported at initialize.
const ServerName = "agentsview"

// DefaultActiveWindow is how recently a session must have been written
// to count as "still active" and be held back from results.
//
// The caller of these tools is usually an agent whose own transcript is
// being appended to this archive right now. Returning that session
// makes the agent read its own in-flight context back as if it were
// evidence: it is the one session guaranteed to match whatever the
// agent is currently thinking about, and it inflates every search for
// the task at hand. Ten minutes covers a live session between two
// writes without hiding genuinely finished work.
const DefaultActiveWindow = 10 * time.Minute

// ToolNames returns the complete tool inventory in registration order.
// Exported so a command, a doc generator or a test can assert the set
// without reaching into a live server.
func ToolNames() []string {
	return []string{
		ToolSearchSessions,
		ToolListSessions,
		ToolGetSessionOverview,
		ToolGetSessionMessages,
		ToolListSessionToolCalls,
		ToolGetUsageSummary,
	}
}

// Options configures a server. The zero value (or nil) is valid.
type Options struct {
	// Version is reported to clients at initialize.
	Version string

	// Now is the clock used to decide whether a session is still
	// active. Tests inject a fixed clock; production leaves it nil.
	Now func() time.Time

	// ActiveWindow overrides DefaultActiveWindow. A negative value
	// disables self-reference exclusion entirely, which is only
	// sensible for a backend that no live agent writes to.
	ActiveWindow time.Duration
}

// server holds the dependencies shared by all tool handlers.
type server struct {
	svc          service.SessionService
	now          func() time.Time
	activeWindow time.Duration
}

// NewServer builds an MCP server over svc with the six read-only tools
// registered. svc must not be nil.
func NewServer(svc service.SessionService, opts *Options) *mcpsdk.Server {
	s := &server{
		svc:          svc,
		now:          time.Now,
		activeWindow: DefaultActiveWindow,
	}
	version := "dev"
	if opts != nil {
		if opts.Now != nil {
			s.now = opts.Now
		}
		if opts.ActiveWindow != 0 {
			s.activeWindow = opts.ActiveWindow
		}
		if opts.Version != "" {
			version = opts.Version
		}
	}
	srv := mcpsdk.NewServer(&mcpsdk.Implementation{
		Name:    ServerName,
		Version: version,
	}, nil)
	s.register(srv)
	return srv
}

// readOnly builds the annotation block every tool in this package
// shares. ReadOnlyHint is the load-bearing field; OpenWorldHint is
// false because the archive is a closed, local corpus, which tells a
// client it can cache and reason about results without assuming an
// external world changed underneath them.
func readOnly(title string) *mcpsdk.ToolAnnotations {
	closedWorld := false
	return &mcpsdk.ToolAnnotations{
		Title:         title,
		ReadOnlyHint:  true,
		OpenWorldHint: &closedWorld,
	}
}
