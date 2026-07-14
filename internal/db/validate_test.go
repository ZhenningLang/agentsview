package db

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSanitizeMessageAppliesParserOutputContract(t *testing.T) {
	rawContent := "before\x1b]0;title\x07after"
	m := Message{
		Role:              "wizard",
		Content:           rawContent,
		ThinkingText:      "think\u0085more",
		ContentLength:     len(rawContent) + len("think\u0085more") + 20,
		Model:             strings.Repeat("a", MaxModelLen-1) + "étail",
		ContextTokens:     -1,
		OutputTokens:      MaxPlausibleTokens + 1,
		Timestamp:         "2999-01-01T00:00:00Z",
		ClaudeMessageID:   "msg\x00id",
		ClaudeRequestID:   "req\x07id",
		SourceUUID:        "uuid\x1bvalue",
		SourceParentUUID:  "parent\u009fvalue",
		SourceType:        "type\x00",
		SourceSubtype:     "subtype\x07",
		HasContextTokens:  true,
		HasOutputTokens:   true,
		IsCompactBoundary: true,
	}
	originalLength := m.ContentLength
	removed := len(rawContent) - len("before]0;titleafter") +
		len("think\u0085more") - len("thinkmore")

	stats := SanitizeMessage(&m)

	assert.Empty(t, m.Role)
	assert.Equal(t, "before]0;titleafter", m.Content)
	assert.Equal(t, "thinkmore", m.ThinkingText)
	assert.Equal(t, originalLength-removed, m.ContentLength)
	assert.LessOrEqual(t, len(m.Model), MaxModelLen)
	assert.True(t, utf8.ValidString(m.Model))
	assert.Equal(t, 0, m.ContextTokens)
	assert.Equal(t, MaxPlausibleTokens, m.OutputTokens)
	assert.Empty(t, m.Timestamp)
	assert.Equal(t, "msgid", m.ClaudeMessageID)
	assert.Equal(t, "reqid", m.ClaudeRequestID)
	assert.Equal(t, "uuidvalue", m.SourceUUID)
	assert.Equal(t, "parentvalue", m.SourceParentUUID)
	assert.Equal(t, "type", m.SourceType)
	assert.Equal(t, "subtype", m.SourceSubtype)
	assert.Equal(t, ValidationStats{
		ControlCharsStripped: 8,
		ModelClamped:         1,
		TokensClamped:        2,
		RoleCoerced:          1,
		TimestampsBlanked:    1,
	}, stats)

	second := SanitizeMessage(&m)
	assert.Equal(t, ValidationStats{}, second)
}

func TestSanitizeMessagePreservesCleanSemanticLength(t *testing.T) {
	m := Message{
		Role:          "assistant",
		Content:       "clean",
		ThinkingText:  "thought",
		ContentLength: 1234,
		Model:         "claude-opus-4",
		ContextTokens: 100,
		OutputTokens:  20,
		Timestamp:     "2026-07-13T12:00:00Z",
	}
	want := m

	stats := SanitizeMessage(&m)

	assert.Equal(t, ValidationStats{}, stats)
	assert.Equal(t, want, m)
}

func TestSanitizeUsageEventKeepsSessionSummaryUnbounded(t *testing.T) {
	for _, source := range []string{"session", "droid-settings", "shutdown"} {
		t.Run(source, func(t *testing.T) {
			summary := UsageEvent{
				Source:       source,
				Model:        "model",
				InputTokens:  MaxPlausibleTokens + 100,
				OutputTokens: -1,
			}
			stats := SanitizeUsageEvent(&summary)
			assert.Equal(t, MaxPlausibleTokens+100, summary.InputTokens)
			assert.Zero(t, summary.OutputTokens)
			assert.Equal(t, 1, stats.TokensClamped)
		})
	}

	row := UsageEvent{
		Source:                   "generation\x1b",
		Model:                    strings.Repeat("m", MaxModelLen+20),
		InputTokens:              MaxPlausibleTokens + 1,
		OutputTokens:             -1,
		CacheCreationInputTokens: MaxPlausibleTokens + 2,
		CacheReadInputTokens:     5,
		ReasoningTokens:          -2,
		OccurredAt:               "1800-01-01T00:00:00Z",
		CostStatus:               "ok\x07",
		CostSource:               "catalog\x00",
		DedupKey:                 "key\u0085",
	}
	stats := SanitizeUsageEvent(&row)
	assert.Equal(t, "generation", row.Source)
	assert.Len(t, row.Model, MaxModelLen)
	assert.Equal(t, MaxPlausibleTokens, row.InputTokens)
	assert.Zero(t, row.OutputTokens)
	assert.Equal(t, MaxPlausibleTokens, row.CacheCreationInputTokens)
	assert.Equal(t, 5, row.CacheReadInputTokens)
	assert.Zero(t, row.ReasoningTokens)
	assert.Empty(t, row.OccurredAt)
	assert.Equal(t, "ok", row.CostStatus)
	assert.Equal(t, "catalog", row.CostSource)
	assert.Equal(t, "key", row.DedupKey)
	assert.Equal(t, ValidationStats{
		ControlCharsStripped: 4,
		ModelClamped:         1,
		TokensClamped:        4,
		TimestampsBlanked:    1,
	}, stats)
}

func TestValidateAndSanitizeSessionAndAggregateStats(t *testing.T) {
	first := "first\x07message"
	name := "name\u0085"
	started := "1500-01-01T00:00:00Z"
	ended := "not-a-timestamp"
	termination := "awaiting\x1buser"
	s := Session{
		Project:           "project\x00",
		Machine:           "machine\x07",
		Agent:             "claude",
		Cwd:               "/tmp\x1bwork",
		GitBranch:         "main\u009f",
		SourceSessionID:   "source\x00",
		SourceVersion:     "version\x07",
		FirstMessage:      &first,
		SessionName:       &name,
		StartedAt:         &started,
		EndedAt:           &ended,
		TerminationStatus: &termination,
	}
	msgs := []Message{{Role: "bogus", Content: "body\x07"}}
	events := []UsageEvent{{Source: "generation", InputTokens: -1}}

	stats := ValidateAndSanitize(&s, msgs, events)

	assert.Equal(t, "project", s.Project)
	assert.Equal(t, "machine", s.Machine)
	assert.Equal(t, "/tmpwork", s.Cwd)
	assert.Equal(t, "main", s.GitBranch)
	assert.Equal(t, "source", s.SourceSessionID)
	assert.Equal(t, "version", s.SourceVersion)
	require.NotNil(t, s.FirstMessage)
	assert.Equal(t, "firstmessage", *s.FirstMessage)
	require.NotNil(t, s.SessionName)
	assert.Equal(t, "name", *s.SessionName)
	assert.Nil(t, s.StartedAt)
	assert.Nil(t, s.EndedAt)
	require.NotNil(t, s.TerminationStatus)
	assert.Equal(t, "awaitinguser", *s.TerminationStatus)
	assert.Equal(t, "", msgs[0].Role)
	assert.Equal(t, "body", msgs[0].Content)
	assert.Zero(t, events[0].InputTokens)
	assert.Equal(t, ValidationStats{
		ControlCharsStripped: 10,
		TokensClamped:        1,
		RoleCoerced:          1,
		TimestampsBlanked:    2,
	}, stats)

	assert.Equal(t, ValidationStats{}, ValidateAndSanitize(&s, msgs, events))
}
