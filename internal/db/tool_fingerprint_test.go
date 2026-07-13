package db

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestComputeToolDataFingerprintUsesExactFixedPointContent(t *testing.T) {
	dirtyCalls := []ToolCallFingerprintRow{{
		MessageOrdinal: 1,
		CallIndex:      0,
		Call: ToolCall{
			ToolName: "R\x07ead", Category: "Read",
			ToolUseID: "tool\x00id", InputJSON: "{\"x\":1}\x1b",
			ResultContent: "same", ResultContentLength: 4,
		},
	}}
	dirtyEvents := []ToolResultEventFingerprintRow{{
		MessageOrdinal: 1,
		CallIndex:      0,
		Event: ToolResultEvent{
			ToolUseID: "tool\x00id", Source: "custom\x07",
			Status: "completed", Content: "equal",
			ContentLength: 5,
			Timestamp:     "2026-07-13T12:00:00.123456789Z",
			EventIndex:    0,
		},
	}}
	cleanCalls := []ToolCallFingerprintRow{{
		MessageOrdinal: 1,
		CallIndex:      0,
		Call: ToolCall{
			ToolName: "Read", Category: "Read",
			ToolUseID: "toolid", InputJSON: "{\"x\":1}",
			ResultContent: "same", ResultContentLength: 4,
		},
	}}
	cleanEvents := []ToolResultEventFingerprintRow{{
		MessageOrdinal: 1,
		CallIndex:      0,
		Event: ToolResultEvent{
			ToolUseID: "toolid", Source: "custom",
			Status: "completed", Content: "equal",
			ContentLength: 5,
			Timestamp:     "2026-07-13T12:00:00.123456Z",
			EventIndex:    0,
		},
	}}

	dirtyFixedPoint := ComputeToolDataFingerprint(
		dirtyCalls, dirtyEvents, true,
	)
	cleanFixedPoint := ComputeToolDataFingerprint(
		cleanCalls, cleanEvents, true,
	)
	require.Equal(t, cleanFixedPoint, dirtyFixedPoint)
	assert.NotEqual(t, dirtyFixedPoint, ComputeToolDataFingerprint(
		dirtyCalls, dirtyEvents, false,
	))

	changed := append([]ToolCallFingerprintRow(nil), cleanCalls...)
	changed[0].Call.ToolName = "Grep" // Same byte length as Read.
	assert.NotEqual(t, cleanFixedPoint, ComputeToolDataFingerprint(
		changed, cleanEvents, true,
	))
}

func TestFingerprintTimestampAcceptsSQLiteSpaceFormat(t *testing.T) {
	assert.Equal(t,
		"2026-01-01T00:00:00Z",
		normalizeToolFingerprintTimestamp("2026-01-01 00:00:00"))
}

func TestLocalToolDataFingerprintNormalizesHistoricalRawRows(t *testing.T) {
	database := testDB(t)
	insertSession(t, database, "tool-fp", "proj")
	require.NoError(t, database.InsertMessages([]Message{{
		SessionID: "tool-fp", Ordinal: 0,
		Role: "assistant", Content: "answer", ContentLength: 6,
		ToolCalls: []ToolCall{{
			ToolName: "Read", Category: "Read", ToolUseID: "tool-id",
			ResultContent: "same", ResultContentLength: 4,
		}},
	}}))

	clean, err := database.ToolDataFingerprint("tool-fp")
	require.NoError(t, err)
	_, err = database.getWriter().Exec(`
		UPDATE tool_calls
		SET tool_name = 'R' || char(7) || 'ead',
			tool_use_id = 'tool' || char(0) || '-id'
		WHERE session_id = ?`, "tool-fp")
	require.NoError(t, err)

	dirtyFixedPoint, err := database.ToolDataFingerprint("tool-fp")
	require.NoError(t, err)
	assert.Equal(t, clean, dirtyFixedPoint)
	msgs, err := database.GetAllMessages(context.Background(), "tool-fp")
	require.NoError(t, err)
	require.Len(t, msgs, 1)
	require.Len(t, msgs[0].ToolCalls, 1)
	assert.Equal(t, "R\x07ead", msgs[0].ToolCalls[0].ToolName,
		"fingerprinting must not mutate historical SQLite rows")
}
