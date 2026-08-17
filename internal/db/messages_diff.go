package db

// transcriptMessagesEqual reports whether two transcripts are the same
// as far as a reader can tell. Comparison is keyed on ordinal rather than
// slice position: the frontend read boundary
// (frontend/src/lib/stores/messages.svelte.ts earliestChangedOrdinal)
// also indexes by ordinal, so an index-based comparison here would make
// the backend revision owner and the browser disagree about what counts
// as a change. Slice order carries no visible meaning; a duplicate or
// missing ordinal does, and is reported as changed.
func transcriptMessagesEqual(a, b []Message) bool {
	if len(a) != len(b) {
		return false
	}
	byOrdinal := make(map[int]Message, len(a))
	for _, msg := range a {
		if _, ok := byOrdinal[msg.Ordinal]; ok {
			return false
		}
		byOrdinal[msg.Ordinal] = msg
	}
	seen := make(map[int]struct{}, len(b))
	for _, msg := range b {
		if _, ok := seen[msg.Ordinal]; ok {
			return false
		}
		seen[msg.Ordinal] = struct{}{}
		other, ok := byOrdinal[msg.Ordinal]
		if !ok {
			return false
		}
		if !transcriptMessageEqual(other, msg) {
			return false
		}
	}
	// Equal lengths plus unique ordinals on both sides plus every b
	// ordinal present in a means the two ordinal sets are identical.
	return true
}

func transcriptMessageEqual(a, b Message) bool {
	if a.Ordinal != b.Ordinal ||
		a.Role != b.Role ||
		a.Content != b.Content ||
		a.ThinkingText != b.ThinkingText ||
		a.Timestamp != b.Timestamp ||
		a.HasThinking != b.HasThinking ||
		a.HasToolUse != b.HasToolUse ||
		a.IsSystem != b.IsSystem ||
		a.Model != b.Model ||
		a.ContextTokens != b.ContextTokens ||
		a.OutputTokens != b.OutputTokens ||
		a.HasContextTokens != b.HasContextTokens ||
		a.HasOutputTokens != b.HasOutputTokens ||
		a.SourceSubtype != b.SourceSubtype ||
		a.IsCompactBoundary != b.IsCompactBoundary ||
		len(a.ToolCalls) != len(b.ToolCalls) {
		return false
	}
	for i := range a.ToolCalls {
		if !transcriptToolCallEqual(a.ToolCalls[i], b.ToolCalls[i]) {
			return false
		}
	}
	return true
}

func transcriptToolCallEqual(a, b ToolCall) bool {
	if a.ToolName != b.ToolName ||
		a.Category != b.Category ||
		a.ToolUseID != b.ToolUseID ||
		a.InputJSON != b.InputJSON ||
		a.SkillName != b.SkillName ||
		a.ResultContent != b.ResultContent ||
		a.SubagentSessionID != b.SubagentSessionID ||
		len(a.ResultEvents) != len(b.ResultEvents) {
		return false
	}
	for i := range a.ResultEvents {
		aEvent := transcriptNormalizedToolResultEvent(a.ResultEvents[i], a, i)
		bEvent := transcriptNormalizedToolResultEvent(b.ResultEvents[i], b, i)
		if !transcriptToolResultEventEqual(aEvent, bEvent) {
			return false
		}
	}
	return true
}

func transcriptNormalizedToolResultEvent(
	event ToolResultEvent, call ToolCall, index int,
) ToolResultEvent {
	event.EventIndex = index
	if event.ToolUseID == "" {
		event.ToolUseID = call.ToolUseID
	}
	if event.SubagentSessionID == "" {
		event.SubagentSessionID = call.SubagentSessionID
	}
	return event
}

func transcriptToolResultEventEqual(a, b ToolResultEvent) bool {
	return a.ToolUseID == b.ToolUseID &&
		a.AgentID == b.AgentID &&
		a.SubagentSessionID == b.SubagentSessionID &&
		a.Source == b.Source &&
		a.Status == b.Status &&
		a.Content == b.Content &&
		a.Timestamp == b.Timestamp &&
		a.EventIndex == b.EventIndex
}
