package db

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestComputeMessageDataFingerprintCoversAllPersistedFields(t *testing.T) {
	base := Message{
		Ordinal: 1, Role: "assistant", Content: "same",
		ThinkingText: "plan", Timestamp: "2026-07-13T12:00:00.123456789Z",
		HasThinking: true, HasToolUse: true, ContentLength: 8,
		IsSystem: true, Model: "model", TokenUsage: []byte(`{"input_tokens":1}`),
		ContextTokens: 1, OutputTokens: 2,
		HasContextTokens: true, HasOutputTokens: true,
		ClaudeMessageID: "msg", ClaudeRequestID: "req",
		SourceType: "assistant", SourceSubtype: "message",
		SourceUUID: "uuid", SourceParentUUID: "parent",
		IsSidechain: true, IsCompactBoundary: true,
	}
	want := ComputeMessageDataFingerprint([]Message{base}, true)

	tests := []struct {
		name   string
		mutate func(*Message)
	}{
		{name: "same-length content", mutate: func(m *Message) { m.Content = "diff" }},
		{name: "role", mutate: func(m *Message) { m.Role = "user" }},
		{name: "thinking", mutate: func(m *Message) { m.ThinkingText = "idea" }},
		{name: "timestamp", mutate: func(m *Message) { m.Timestamp = "2026-07-13T12:00:01Z" }},
		{name: "has thinking", mutate: func(m *Message) { m.HasThinking = false }},
		{name: "has tool use", mutate: func(m *Message) { m.HasToolUse = false }},
		{name: "is system", mutate: func(m *Message) { m.IsSystem = false }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			changed := base
			tt.mutate(&changed)
			assert.NotEqual(t, want,
				ComputeMessageDataFingerprint([]Message{changed}, true))
		})
	}

	dirty := base
	dirty.Role = "assistant\x07"
	dirty.Content = "sa\x07me"
	dirty.ThinkingText = "pl\x1ban"
	dirty.Model = "mo\x00del"
	dirty.Timestamp = "2026-07-13T12:00:00.123456999Z"
	dirty.ContentLength = 10
	fixedPoint := base
	fixedPoint.Role = ""
	assert.Equal(t,
		ComputeMessageDataFingerprint([]Message{fixedPoint}, true),
		ComputeMessageDataFingerprint([]Message{dirty}, true))
	assert.NotEqual(t,
		ComputeMessageDataFingerprint([]Message{fixedPoint}, true),
		ComputeMessageDataFingerprint([]Message{dirty}, false))
}

func TestExactFingerprintsHaveStableEmptyValues(t *testing.T) {
	assert.Equal(t,
		ComputeMessageDataFingerprint(nil, true),
		ComputeMessageDataFingerprint(nil, false))
	assert.Equal(t,
		ComputeToolDataFingerprint(nil, nil, true),
		ComputeToolDataFingerprint(nil, nil, false))
	assert.Equal(t,
		ComputeUsageEventFingerprint(nil, true),
		ComputeUsageEventFingerprint(nil, false))
}
