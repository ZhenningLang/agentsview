package sync

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.kenn.io/agentsview/internal/db"
)

func TestPhase22SyncValidateSessionPath(t *testing.T) {
	first := "first\x07message"
	started := "1500-01-01T00:00:00Z"
	s := db.Session{
		ID: "phase22-sync-validate-session", Project: "proj\x1bect",
		FirstMessage: &first, StartedAt: &started,
	}

	stats := validateAndSanitize(&s, nil, nil)
	require.False(t, stats.Empty())
	assert.Equal(t, "project", s.Project)
	require.NotNil(t, s.FirstMessage)
	assert.Equal(t, "firstmessage", *s.FirstMessage)
	assert.Nil(t, s.StartedAt)
	assert.Equal(t, validationStats{}, validateAndSanitize(&s, nil, nil))
}

func TestPhase22SyncValidateMessagePath(t *testing.T) {
	msgs := []db.Message{{
		Role: "wizard", Content: "answer\x1bbody",
		ContentLength: len("answer\x1bbody"),
		Model:         strings.Repeat("m", maxModelLen+20),
		ContextTokens: -1, OutputTokens: maxPlausibleTokens + 1,
		HasContextTokens: true, HasOutputTokens: true,
		Timestamp: "2999-01-01T00:00:00Z",
	}}

	stats := validateAndSanitize(nil, msgs, nil)
	require.False(t, stats.Empty())
	assert.Empty(t, msgs[0].Role)
	assert.Equal(t, "answerbody", msgs[0].Content)
	assert.Equal(t, len("answerbody"), msgs[0].ContentLength)
	assert.Len(t, msgs[0].Model, maxModelLen)
	assert.Zero(t, msgs[0].ContextTokens)
	assert.Equal(t, maxPlausibleTokens, msgs[0].OutputTokens)
	assert.Empty(t, msgs[0].Timestamp)
	assert.Equal(t, validationStats{}, validateAndSanitize(nil, msgs, nil))
}

func TestPhase22SyncValidateUsageEventPath(t *testing.T) {
	events := []db.UsageEvent{{
		SessionID: "phase22-sync-validate-usage", Source: "generation\x07",
		Model:       strings.Repeat("u", maxModelLen+1),
		InputTokens: maxPlausibleTokens + 1,
		OccurredAt:  "1800-01-01T00:00:00Z",
	}}

	stats := validateAndSanitize(nil, nil, events)
	require.False(t, stats.Empty())
	assert.Equal(t, "generation", events[0].Source)
	assert.Len(t, events[0].Model, maxModelLen)
	assert.Equal(t, maxPlausibleTokens, events[0].InputTokens)
	assert.Empty(t, events[0].OccurredAt)
	assert.Equal(t, validationStats{}, validateAndSanitize(nil, nil, events))
}

func TestPhase22SyncValidateWrapperIsIdempotentAcrossAllPaths(t *testing.T) {
	first := "first\x07message"
	started := "1500-01-01T00:00:00Z"
	s := db.Session{
		ID: "phase22-sync-validate", Project: "proj\x1bect",
		FirstMessage: &first, StartedAt: &started,
	}
	msgs := []db.Message{{
		Role: "wizard", Content: "answer\x1bbody",
		ContentLength: len("answer\x1bbody"),
		Model:         strings.Repeat("m", maxModelLen+20),
		ContextTokens: -1, OutputTokens: maxPlausibleTokens + 1,
		HasContextTokens: true, HasOutputTokens: true,
		Timestamp: "2999-01-01T00:00:00Z",
	}}
	events := []db.UsageEvent{{
		SessionID: "phase22-sync-validate", Source: "generation\x07",
		Model:       strings.Repeat("u", maxModelLen+1),
		InputTokens: maxPlausibleTokens + 1,
		OccurredAt:  "1800-01-01T00:00:00Z",
	}}

	stats := validateAndSanitize(&s, msgs, events)
	require.False(t, stats.Empty())
	cleanSession := s
	cleanMessages := append([]db.Message(nil), msgs...)
	cleanEvents := append([]db.UsageEvent(nil), events...)
	assert.Equal(t, validationStats{}, validateAndSanitize(&s, msgs, events))
	assert.Equal(t, cleanSession, s)
	assert.Equal(t, cleanMessages, msgs)
	assert.Equal(t, cleanEvents, events)
}
