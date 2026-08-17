package db

func transcriptMessagesEqual(a, b []Message) bool {
	if !transcriptOrdinalsUnique(a) || !transcriptOrdinalsUnique(b) {
		return false
	}
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !transcriptMessageEqual(a[i], b[i]) {
			return false
		}
	}
	return true
}

func transcriptOrdinalsUnique(msgs []Message) bool {
	seen := make(map[int]struct{}, len(msgs))
	for _, msg := range msgs {
		if _, ok := seen[msg.Ordinal]; ok {
			return false
		}
		seen[msg.Ordinal] = struct{}{}
	}
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
