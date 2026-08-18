package mcp

import (
	"time"
	"unicode/utf8"

	"go.kenn.io/agentsview/internal/db"
)

// Response caps. An MCP result is spent from the client's context
// window, so the shaping here is a budget, not a formatting nicety: an
// unbounded transcript page can evict the very conversation that asked
// for it.
const (
	maxContentRunes   = 2000
	maxSnippetRunes   = 400
	maxFirstMsgRunes  = 300
	maxToolInputRunes = 1000
)

// Per-tool page sizes. These are deliberately smaller than the store's
// own limits (db.DefaultSearchLimit is 50, db.DefaultMessageLimit 100):
// the store's defaults are sized for a UI that paginates on scroll, not
// for a context window.
const (
	defaultSearchLimit = 20
	maxSearchLimit     = 100

	defaultListLimit = 20
	maxListLimit     = 100

	defaultMessageLimit = 50
	maxMessageLimit     = 200

	defaultOverviewLimit = 5
	maxOverviewLimit     = 50

	defaultToolCallLimit = 50
	maxToolCallLimit     = 200

	defaultUsageTopN = 10
	maxUsageTopN     = 50
)

// clampLimit folds a caller-supplied limit into [1, maxLimit], with an
// absent or non-positive value meaning the default.
func clampLimit(requested, def, maxLimit int) int {
	if requested <= 0 {
		return def
	}
	if requested > maxLimit {
		return maxLimit
	}
	return requested
}

// truncateRunes cuts s to at most maxRunes runes and reports whether it
// cut anything.
//
// Runes, not bytes: transcripts in this archive are routinely CJK, and
// a byte slice through a multi-byte character emits U+FFFD into the
// model's context - a corrupted last character that reads as content.
// The cut is still not grapheme-aware, so an emoji sequence can lose a
// modifier, but no invalid UTF-8 is ever produced.
func truncateRunes(s string, maxRunes int) (string, bool) {
	if maxRunes <= 0 || utf8.RuneCountInString(s) <= maxRunes {
		return s, false
	}
	count := 0
	for i := range s {
		if count == maxRunes {
			return s[:i], true
		}
		count++
	}
	return s, false
}

// deref returns the value behind p, or "" for nil.
func deref(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// messageView is the shaped form of one transcript message. It is a
// deliberate subset of db.Message: token accounting, source UUIDs and
// the raw token_usage payload are noise to a reader and cost context.
type messageView struct {
	Ordinal          int    `json:"ordinal"`
	Role             string `json:"role"`
	Timestamp        string `json:"timestamp,omitempty"`
	Model            string `json:"model,omitempty"`
	Content          string `json:"content"`
	ContentTruncated bool   `json:"content_truncated,omitempty"`
	ContentLength    int    `json:"content_length"`
	HasThinking      bool   `json:"has_thinking,omitempty"`
	HasToolUse       bool   `json:"has_tool_use,omitempty"`
	IsSystem         bool   `json:"is_system,omitempty"`
}

// isSystemMessage reports whether m is a system-injected message rather
// than something a person or the model wrote.
//
// The prefix rule itself is db.IsSystemPrefixed, which is the Go twin
// of the SQL that the search and analytics paths use. Deriving it a
// second time here would not fail anything: it would just make the same
// session look different depending on whether the search index or this
// tool answered. m.IsSystem is the parser's own persisted verdict and
// is honoured as well - it is a stored classification, not a second
// implementation of the prefix rule.
func isSystemMessage(m db.Message) bool {
	return m.IsSystem || db.IsSystemPrefixed(m.Content, m.Role)
}

// messageViews shapes messages, dropping system-injected ones unless
// includeSystem is set. The returned slice is never nil: a nil slice
// marshals to JSON null, which fails the tool's own output schema.
func messageViews(msgs []db.Message, includeSystem bool) []messageView {
	out := make([]messageView, 0, len(msgs))
	for _, m := range msgs {
		system := isSystemMessage(m)
		if system && !includeSystem {
			continue
		}
		content, truncated := truncateRunes(m.Content, maxContentRunes)
		out = append(out, messageView{
			Ordinal:          m.Ordinal,
			Role:             m.Role,
			Timestamp:        m.Timestamp,
			Model:            m.Model,
			Content:          content,
			ContentTruncated: truncated,
			ContentLength:    m.ContentLength,
			HasThinking:      m.HasThinking,
			HasToolUse:       m.HasToolUse,
			IsSystem:         system,
		})
	}
	return out
}

// nextFrom returns the ordinal a follow-up page should start at, or nil
// when this page exhausted the transcript.
//
// It is computed from the raw store page, before system-message
// filtering. Deriving it from what survived the filter would rewind the
// cursor to the last message the caller was allowed to see: a page
// whose tail is entirely system messages would return the same window
// forever, and the caller would loop.
func nextFrom(raw []db.Message, limit int, asc bool) *int {
	if len(raw) == 0 || len(raw) < limit {
		return nil
	}
	last := raw[len(raw)-1].Ordinal
	if asc {
		next := last + 1
		return &next
	}
	if last == 0 {
		return nil
	}
	next := last - 1
	return &next
}

// sessionView is the shaped form of a session row.
type sessionView struct {
	ID                string `json:"id"`
	Project           string `json:"project"`
	Agent             string `json:"agent"`
	Machine           string `json:"machine"`
	Name              string `json:"name,omitempty"`
	FirstMessage      string `json:"first_message,omitempty"`
	StartedAt         string `json:"started_at,omitempty"`
	EndedAt           string `json:"ended_at,omitempty"`
	MessageCount      int    `json:"message_count"`
	UserMessageCount  int    `json:"user_message_count"`
	GitBranch         string `json:"git_branch,omitempty"`
	Outcome           string `json:"outcome,omitempty"`
	OutcomeConfidence string `json:"outcome_confidence,omitempty"`
	TotalOutputTokens int    `json:"total_output_tokens,omitempty"`
	PeakContextTokens int    `json:"peak_context_tokens,omitempty"`
}

// sessionName prefers the user-visible name over the stored one.
func sessionName(s db.Session) string {
	if n := deref(s.DisplayName); n != "" {
		return n
	}
	return deref(s.SessionName)
}

func newSessionView(s db.Session) sessionView {
	first, _ := truncateRunes(deref(s.FirstMessage), maxFirstMsgRunes)
	return sessionView{
		ID:                s.ID,
		Project:           s.Project,
		Agent:             s.Agent,
		Machine:           s.Machine,
		Name:              sessionName(s),
		FirstMessage:      first,
		StartedAt:         deref(s.StartedAt),
		EndedAt:           deref(s.EndedAt),
		MessageCount:      s.MessageCount,
		UserMessageCount:  s.UserMessageCount,
		GitBranch:         s.GitBranch,
		Outcome:           s.Outcome,
		OutcomeConfidence: s.OutcomeConfidence,
		TotalOutputTokens: s.TotalOutputTokens,
		PeakContextTokens: s.PeakContextTokens,
	}
}

// isActive reports whether a session was written to inside the active
// window and should therefore be withheld as a probable self-reference.
// An unparseable or absent timestamp is treated as not active: an
// unreadable timestamp is not evidence of recency, and failing closed
// here would hide the whole archive whenever a backend stored a format
// this build cannot parse.
func (s *server) isActive(endedAt, startedAt string) bool {
	if s.activeWindow < 0 {
		return false
	}
	window := s.activeWindow
	if window == 0 {
		window = DefaultActiveWindow
	}
	stamp := endedAt
	if stamp == "" {
		stamp = startedAt
	}
	if stamp == "" {
		return false
	}
	t, err := time.Parse(time.RFC3339, stamp)
	if err != nil {
		return false
	}
	return !t.Before(s.now().Add(-window))
}
