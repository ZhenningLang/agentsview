package db

import (
	"time"
	"unicode/utf8"

	"go.kenn.io/agentsview/internal/parser"
)

const (
	// MaxModelLen bounds stored model identifiers in bytes.
	MaxModelLen      = 128
	minPlausibleYear = 2000
	maxPlausibleYear = 2100
)

// ValidationStats records the number of fields changed by category.
type ValidationStats struct {
	ControlCharsStripped int
	ModelClamped         int
	TokensClamped        int
	RoleCoerced          int
	TimestampsBlanked    int
}

// SanitizedMessages returns a deep-enough copy for in-place validation. Nested
// tool slices are cloned so validation never mutates the caller's input.
func SanitizedMessages(msgs []Message) ([]Message, ValidationStats) {
	clean := make([]Message, len(msgs))
	for i, msg := range msgs {
		clean[i] = msg
		clean[i].TokenUsage = append([]byte(nil), msg.TokenUsage...)
		clean[i].ToolResults = append([]ToolResult(nil), msg.ToolResults...)
		clean[i].ToolCalls = make([]ToolCall, len(msg.ToolCalls))
		for j, call := range msg.ToolCalls {
			clean[i].ToolCalls[j] = call
			clean[i].ToolCalls[j].ResultEvents = append(
				[]ToolResultEvent(nil), call.ResultEvents...,
			)
		}
	}
	stats := ValidateAndSanitize(nil, clean, nil)
	return clean, stats
}

// SanitizedUsageEvents returns a copy of usage events after validation.
func SanitizedUsageEvents(events []UsageEvent) ([]UsageEvent, ValidationStats) {
	clean := append([]UsageEvent(nil), events...)
	stats := ValidateAndSanitize(nil, nil, clean)
	return clean, stats
}

func (s *ValidationStats) add(other ValidationStats) {
	s.ControlCharsStripped += other.ControlCharsStripped
	s.ModelClamped += other.ModelClamped
	s.TokensClamped += other.TokensClamped
	s.RoleCoerced += other.RoleCoerced
	s.TimestampsBlanked += other.TimestampsBlanked
}

// Add merges another stats value into the receiver.
func (s *ValidationStats) Add(other ValidationStats) {
	s.add(other)
}

// Empty reports whether validation changed no fields.
func (s ValidationStats) Empty() bool {
	return s == (ValidationStats{})
}

// ValidateAndSanitize applies the parser-output contract in place. It
// sanitizes or clamps invalid fields rather than dropping rows.
func ValidateAndSanitize(
	s *Session, msgs []Message, events []UsageEvent,
) ValidationStats {
	var stats ValidationStats
	if s != nil {
		stats.add(SanitizeSession(s))
	}
	for i := range msgs {
		stats.add(SanitizeMessage(&msgs[i]))
	}
	for i := range events {
		stats.add(SanitizeUsageEvent(&events[i]))
	}
	return stats
}

// SanitizeMessage validates and sanitizes one message row in place.
func SanitizeMessage(m *Message) ValidationStats {
	var stats ValidationStats
	if !parser.ValidRole(parser.RoleType(m.Role)) {
		m.Role = ""
		stats.RoleCoerced++
	}

	originalContentLen := len(m.Content)
	sanitizeStringField(&m.Content, &stats)
	if removed := originalContentLen - len(m.Content); removed > 0 {
		m.ContentLength -= removed
		if m.ContentLength < 0 {
			m.ContentLength = 0
		}
	}

	originalThinkingLen := len(m.ThinkingText)
	sanitizeStringField(&m.ThinkingText, &stats)
	if removed := originalThinkingLen - len(m.ThinkingText); removed > 0 {
		excess := m.ContentLength - len(m.Content)
		if excess > 0 {
			if removed > excess {
				removed = excess
			}
			m.ContentLength -= removed
		}
	}

	sanitizeStringField(&m.ClaudeMessageID, &stats)
	sanitizeStringField(&m.ClaudeRequestID, &stats)
	sanitizeStringField(&m.SourceType, &stats)
	sanitizeStringField(&m.SourceSubtype, &stats)
	sanitizeStringField(&m.SourceUUID, &stats)
	sanitizeStringField(&m.SourceParentUUID, &stats)
	sanitizeStringField(&m.Model, &stats)
	if ClampModel(&m.Model) {
		stats.ModelClamped++
	}
	if clampTokens(&m.ContextTokens) {
		stats.TokensClamped++
	}
	if clampTokens(&m.OutputTokens) {
		stats.TokensClamped++
	}
	if BlankImplausibleTimestamp(&m.Timestamp) {
		stats.TimestampsBlanked++
	}
	for i := range m.ToolCalls {
		stats.add(sanitizeToolCall(&m.ToolCalls[i]))
	}
	for i := range m.ToolResults {
		sanitizeStringField(&m.ToolResults[i].ToolUseID, &stats)
		originalLen := len(m.ToolResults[i].ContentRaw)
		sanitizeStringField(&m.ToolResults[i].ContentRaw, &stats)
		adjustContentLength(
			&m.ToolResults[i].ContentLength,
			originalLen-len(m.ToolResults[i].ContentRaw), 0,
		)
	}
	return stats
}

func sanitizeToolCall(call *ToolCall) ValidationStats {
	var stats ValidationStats
	for _, field := range []*string{
		&call.ToolName,
		&call.Category,
		&call.ToolUseID,
		&call.InputJSON,
		&call.SkillName,
		&call.SubagentSessionID,
	} {
		sanitizeStringField(field, &stats)
	}
	originalLen := len(call.ResultContent)
	sanitizeStringField(&call.ResultContent, &stats)
	adjustContentLength(
		&call.ResultContentLength,
		originalLen-len(call.ResultContent), 0,
	)
	for i := range call.ResultEvents {
		stats.add(sanitizeToolResultEvent(&call.ResultEvents[i]))
	}
	return stats
}

func sanitizeToolResultEvent(event *ToolResultEvent) ValidationStats {
	var stats ValidationStats
	for _, field := range []*string{
		&event.ToolUseID,
		&event.AgentID,
		&event.SubagentSessionID,
		&event.Source,
		&event.Status,
	} {
		sanitizeStringField(field, &stats)
	}
	originalLen := len(event.Content)
	sanitizeStringField(&event.Content, &stats)
	adjustContentLength(
		&event.ContentLength,
		originalLen-len(event.Content), 0,
	)
	if BlankImplausibleTimestamp(&event.Timestamp) {
		stats.TimestampsBlanked++
	}
	return stats
}

func adjustContentLength(length *int, removed, minimum int) {
	if removed <= 0 {
		return
	}
	*length -= removed
	if *length < minimum {
		*length = minimum
	}
}

// SanitizeUsageEvent validates and sanitizes one usage event in place.
func SanitizeUsageEvent(event *UsageEvent) ValidationStats {
	var stats ValidationStats
	sanitizeStringField(&event.Source, &stats)
	sanitizeStringField(&event.CostStatus, &stats)
	sanitizeStringField(&event.CostSource, &stats)
	sanitizeStringField(&event.DedupKey, &stats)
	sanitizeStringField(&event.Model, &stats)
	if ClampModel(&event.Model) {
		stats.ModelClamped++
	}

	for _, field := range []*int{
		&event.InputTokens,
		&event.OutputTokens,
		&event.CacheCreationInputTokens,
		&event.CacheReadInputTokens,
		&event.ReasoningTokens,
	} {
		if clampUsageEventTokens(event.Source, field) {
			stats.TokensClamped++
		}
	}
	if BlankImplausibleTimestamp(&event.OccurredAt) {
		stats.TimestampsBlanked++
	}
	return stats
}

func clampUsageEventTokens(source string, value *int) bool {
	if UsageSourceIsSessionSummary(source) {
		if *value < 0 {
			*value = 0
			return true
		}
		return false
	}
	return clampTokens(value)
}

// UsageSourceIsSessionSummary identifies events whose token values are
// cumulative session totals rather than one row-level generation.
func UsageSourceIsSessionSummary(source string) bool {
	switch source {
	case "session", "droid-settings", "shutdown":
		return true
	default:
		return false
	}
}

// SanitizeSession validates and sanitizes one session row in place. Aggregate
// token totals are intentionally left to the write-path reconciliation logic.
func SanitizeSession(s *Session) ValidationStats {
	var stats ValidationStats
	for _, field := range []*string{
		&s.Project,
		&s.Machine,
		&s.Agent,
		&s.Cwd,
		&s.GitBranch,
		&s.SourceSessionID,
		&s.SourceVersion,
	} {
		sanitizeStringField(field, &stats)
	}
	sanitizeStringPtrField(s.FirstMessage, &stats)
	sanitizeStringPtrField(s.SessionName, &stats)
	sanitizeStringPtrField(s.TerminationStatus, &stats)
	if next, changed := BlankImplausibleTimestampPtr(s.StartedAt); changed {
		s.StartedAt = next
		stats.TimestampsBlanked++
	}
	if next, changed := BlankImplausibleTimestampPtr(s.EndedAt); changed {
		s.EndedAt = next
		stats.TimestampsBlanked++
	}
	return stats
}

func sanitizeStringField(value *string, stats *ValidationStats) {
	clean := SanitizeUTF8(*value)
	if clean == *value {
		return
	}
	*value = clean
	stats.ControlCharsStripped++
}

func sanitizeStringPtrField(value *string, stats *ValidationStats) {
	if value != nil {
		sanitizeStringField(value, stats)
	}
}

// ClampModel truncates a model ID at a valid UTF-8 boundary.
func ClampModel(model *string) bool {
	if len(*model) <= MaxModelLen {
		return false
	}
	cut := MaxModelLen
	for cut > 0 && !utf8.RuneStart((*model)[cut]) {
		cut--
	}
	*model = (*model)[:cut]
	return true
}

func clampTokens(value *int) bool {
	clamped := ClampPlausibleTokens(*value)
	if clamped == *value {
		return false
	}
	*value = clamped
	return true
}

// BlankImplausibleTimestamp clears parseable timestamps outside 2000..2100.
func BlankImplausibleTimestamp(value *string) bool {
	if *value == "" {
		return false
	}
	timestamp, ok := ParseStoredTimestamp(*value)
	if !ok {
		return false
	}
	year := timestamp.UTC().Year()
	if year >= minPlausibleYear && year <= maxPlausibleYear {
		return false
	}
	*value = ""
	return true
}

// BlankImplausibleTimestampPtr returns nil for an implausible timestamp.
func BlankImplausibleTimestampPtr(value *string) (*string, bool) {
	if value == nil {
		return nil, false
	}
	next := *value
	if !BlankImplausibleTimestamp(&next) {
		return value, false
	}
	return nil, true
}

// ParseStoredTimestamp parses timestamp formats accepted by SQLite reads.
func ParseStoredTimestamp(value string) (time.Time, bool) {
	for _, layout := range []string{
		time.RFC3339Nano,
		"2006-01-02 15:04:05",
	} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed, true
		}
	}
	return time.Time{}, false
}
