package db

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPhase23MessageScopeReducerPairsEmptyModelUserWithSelectedAssistant(t *testing.T) {
	var got []ScopedMessage
	reducer := NewScopeReducer(ScopeFilter{Models: map[string]struct{}{"model-a": {}}}, func(row ScopedMessage) {
		got = append(got, row)
	})

	pushScopeRows(t, reducer,
		MessageInput{SessionID: "s", Ordinal: 0, Role: "user", Content: "prompt"},
		MessageInput{SessionID: "s", Ordinal: 1, Role: "assistant", Model: "model-a", HasToolUse: true},
	)

	require.Len(t, got, 2)
	assert.Equal(t, []string{"user", "assistant"}, scopeRoles(got))
	assert.Equal(t, MessageStats{Messages: 2, UserMessages: 1, AssistantMessages: 1, ToolUseMessages: 1}, ScopeStats(got))
}

func TestPhase23MessageScopeReducerClearsPendingUserOnUnselectedAssistant(t *testing.T) {
	var got []ScopedMessage
	reducer := NewScopeReducer(ScopeFilter{Models: map[string]struct{}{"model-a": {}}}, func(row ScopedMessage) {
		got = append(got, row)
	})

	pushScopeRows(t, reducer,
		MessageInput{SessionID: "s", Ordinal: 0, Role: "user", Content: "stale"},
		MessageInput{SessionID: "s", Ordinal: 1, Role: "assistant", Model: "model-b"},
		MessageInput{SessionID: "s", Ordinal: 2, Role: "assistant", Model: "model-a"},
	)

	require.Len(t, got, 1)
	assert.Equal(t, "assistant", got[0].Role)
}

func TestPhase23MessageScopeReducerAppliesDayHourToSameMatchedRow(t *testing.T) {
	hour := 9
	day := 0 // Monday
	var got []ScopedMessage
	reducer := NewScopeReducer(ScopeFilter{
		Models: map[string]struct{}{"model-a": {}}, DayOfWeek: &day, Hour: &hour,
	}, func(row ScopedMessage) { got = append(got, row) })

	pushScopeRows(t, reducer,
		MessageInput{SessionID: "s", Ordinal: 0, Role: "assistant", Model: "model-a", HasLocalTime: false},
		MessageInput{SessionID: "s", Ordinal: 1, Role: "assistant", Model: "model-a", HasLocalTime: true, LocalTime: time.Date(2024, 6, 3, 9, 0, 0, 0, time.UTC)},
		MessageInput{SessionID: "s", Ordinal: 2, Role: "assistant", Model: "model-a", HasLocalTime: true, LocalTime: time.Date(2024, 6, 3, 10, 0, 0, 0, time.UTC)},
	)

	require.Len(t, got, 1)
	assert.Equal(t, 1, got[0].Ordinal)
}

func TestPhase23MessageScopeReducerRejectsUngroupedOrOutOfOrderRows(t *testing.T) {
	reducer := NewScopeReducer(ScopeFilter{Models: map[string]struct{}{"model-a": {}}}, func(ScopedMessage) {})
	require.NoError(t, reducer.Push(MessageInput{SessionID: "a", Ordinal: 1, Role: "assistant", Model: "model-a"}))
	require.Error(t, reducer.Push(MessageInput{SessionID: "a", Ordinal: 0, Role: "assistant", Model: "model-a"}))

	reducer = NewScopeReducer(ScopeFilter{Models: map[string]struct{}{"model-a": {}}}, func(ScopedMessage) {})
	require.NoError(t, reducer.Push(MessageInput{SessionID: "a", Ordinal: 0, Role: "assistant", Model: "model-a"}))
	require.NoError(t, reducer.Push(MessageInput{SessionID: "b", Ordinal: 0, Role: "assistant", Model: "model-a"}))
	require.Error(t, reducer.Push(MessageInput{SessionID: "a", Ordinal: 1, Role: "assistant", Model: "model-a"}))
}

func TestPhase23MessageScopeReducerTimingSortsByOrdinal(t *testing.T) {
	rows := []ScopedMessage{
		{Ordinal: 2, Role: "assistant", LocalTime: time.Date(2024, 6, 1, 0, 2, 0, 0, time.UTC), HasLocalTime: true, ContentLength: 2},
		{Ordinal: 1, Role: "user", LocalTime: time.Date(2024, 6, 1, 0, 1, 0, 0, time.UTC), HasLocalTime: true, ContentLength: 1},
	}

	timing := ScopeTiming(rows)
	require.Len(t, timing, 2)
	assert.Equal(t, "user", timing[0].Role)
	assert.Equal(t, "assistant", timing[1].Role)
}

func pushScopeRows(t *testing.T, reducer *ScopeReducer, rows ...MessageInput) {
	t.Helper()
	for _, row := range rows {
		require.NoError(t, reducer.Push(row))
	}
}

func scopeRoles(rows []ScopedMessage) []string {
	roles := make([]string, len(rows))
	for i, row := range rows {
		roles[i] = row.Role
	}
	return roles
}
