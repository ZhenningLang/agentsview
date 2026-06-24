package db

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEnrichCandidatesGateMatrix(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	now := time.Date(2026, 6, 24, 10, 0, 0, 0, time.UTC)
	old := now.Add(-45 * time.Minute).Format(time.RFC3339)
	recent := now.Add(-5 * time.Minute).Format(time.RFC3339)
	ended := now.Add(-time.Hour).Format(time.RFC3339)

	tests := []struct {
		name       string
		status     string
		messages   int
		users      int
		enriched   int
		endedAt    *string
		messageAt  string
		wantNormal bool
		wantForce  bool
	}{
		{name: "new ended", messages: 4, users: 3, endedAt: &ended, wantNormal: true, wantForce: true},
		{name: "error idle", status: EnrichStatusError, messages: 4, users: 3, messageAt: old, wantNormal: true, wantForce: true},
		{name: "skipped grown", status: EnrichStatusSkippedTooShort, messages: 4, users: 3, messageAt: old, wantNormal: true, wantForce: true},
		{name: "no content retry", status: EnrichStatusNoContent, messages: 4, users: 3, messageAt: old, wantNormal: true, wantForce: true},
		{name: "incremental reached", status: EnrichStatusOK, messages: 25, users: 8, enriched: 4, messageAt: old, wantNormal: true, wantForce: true},
		{name: "incremental below", status: EnrichStatusOK, messages: 9, users: 6, enriched: 4, messageAt: old, wantForce: true},
		{name: "active not idle", messages: 4, users: 3, messageAt: recent, wantForce: true},
		{name: "too short", messages: 2, users: 2, endedAt: &ended},
	}
	for i, tt := range tests {
		id := "s" + string(rune('a'+i))
		insertSession(t, d, id, "proj", func(s *Session) {
			s.MessageCount = tt.messages
			s.UserMessageCount = tt.users
			s.EnrichedMsgCount = tt.enriched
			s.EnrichStatus = tt.status
			s.EndedAt = tt.endedAt
		})
		if tt.messageAt != "" {
			insertMessages(t, d, userMsgAt(id, 0, "hello from user", tt.messageAt))
		}
		if tt.status != "" || tt.enriched > 0 {
			_, err := d.getWriter().Exec(`UPDATE sessions SET enrich_status = ?, enriched_msg_count = ? WHERE id = ?`, tt.status, tt.enriched, id)
			require.NoError(t, err)
		}
	}

	candidates, err := d.EnrichCandidates(ctx, EnrichCandidateOptions{
		Project:             "proj",
		MinUserMessages:     3,
		ReenrichMsgDelta:    20,
		ReenrichIdleMinutes: 30,
		Now:                 now,
	})
	require.NoError(t, err)
	got := candidateIDSet(candidates)
	for i, tt := range tests {
		id := "s" + string(rune('a'+i))
		assert.Equal(t, tt.wantNormal, got[id], tt.name)
	}

	forceCandidates, err := d.EnrichCandidates(ctx, EnrichCandidateOptions{
		Project:         "proj",
		Force:           true,
		MinUserMessages: 3,
		Now:             now,
	})
	require.NoError(t, err)
	forceGot := candidateIDSet(forceCandidates)
	for i, tt := range tests {
		id := "s" + string(rune('a'+i))
		assert.Equal(t, tt.wantForce, forceGot[id], tt.name+" force")
	}
}

func TestSkippedTooShortAndNoContentCanBeReevaluated(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	now := time.Date(2026, 6, 24, 10, 0, 0, 0, time.UTC)
	ended := now.Add(-time.Hour).Format(time.RFC3339)
	insertSession(t, d, "short", "proj", func(s *Session) {
		s.MessageCount = 2
		s.UserMessageCount = 2
		s.EndedAt = &ended
	})

	skipped, err := d.MarkEnrichmentSkippedTooShort(ctx, EnrichCandidateOptions{MinUserMessages: 3})
	require.NoError(t, err)
	assert.Equal(t, 1, skipped)
	skipped, err = d.MarkEnrichmentSkippedTooShort(ctx, EnrichCandidateOptions{MinUserMessages: 3})
	require.NoError(t, err)
	assert.Equal(t, 0, skipped, "already skipped short sessions should not be counted again")
	s := requireSessionExists(t, d, "short")
	assert.Equal(t, EnrichStatusSkippedTooShort, s.EnrichStatus)

	_, err = d.getWriter().Exec(`UPDATE sessions SET message_count = 4, user_message_count = 3 WHERE id = 'short'`)
	require.NoError(t, err)
	candidates, err := d.EnrichCandidates(ctx, EnrichCandidateOptions{MinUserMessages: 3, Now: now})
	require.NoError(t, err)
	assert.True(t, candidateIDSet(candidates)["short"])

	insertSession(t, d, "empty", "proj", func(s *Session) {
		s.MessageCount = 4
		s.UserMessageCount = 3
		s.EndedAt = &ended
	})
	require.NoError(t, d.WriteEnrichment(ctx, "empty", EnrichmentWrite{Status: EnrichStatusNoContent, Error: "empty"}))
	candidates, err = d.EnrichCandidates(ctx, EnrichCandidateOptions{MinUserMessages: 3, Now: now})
	require.NoError(t, err)
	assert.True(t, candidateIDSet(candidates)["empty"])
}

func TestEnrichCandidatesProjectAndLimit(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	ended := "2026-06-24T09:00:00Z"
	for _, tc := range []struct{ id, project string }{{"a1", "a"}, {"a2", "a"}, {"b1", "b"}} {
		insertSession(t, d, tc.id, tc.project, func(s *Session) {
			s.MessageCount = 4
			s.UserMessageCount = 3
			s.EndedAt = &ended
		})
	}
	candidates, err := d.EnrichCandidates(ctx, EnrichCandidateOptions{Project: "a", Limit: 1, MinUserMessages: 3})
	require.NoError(t, err)
	require.Len(t, candidates, 1)
	assert.Equal(t, "a", candidates[0].Project)
}

func TestWriteEnrichmentSuccessAndFailure(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	insertSession(t, d, "ok", "proj", func(s *Session) {
		s.MessageCount = 5
		s.UserMessageCount = 3
	})
	when := time.Date(2026, 6, 24, 10, 0, 0, 0, time.UTC)
	require.NoError(t, d.WriteEnrichment(ctx, "ok", EnrichmentWrite{
		Title: " Title ", Summary: " Summary ", Keywords: []string{"auth", " token "},
		Model: "deepseek-chat", MessageCnt: 5, EnrichedAt: when,
	}))
	s := requireSessionExists(t, d, "ok")
	assert.Equal(t, "Title", s.LLMTitle)
	assert.Equal(t, "Summary", s.LLMSummary)
	assert.Equal(t, "auth,token", s.LLMKeywords)
	assert.Equal(t, 5, s.EnrichedMsgCount)
	assert.Equal(t, "deepseek-chat", s.EnrichModel)
	assert.Equal(t, EnrichStatusOK, s.EnrichStatus)
	assert.Empty(t, s.EnrichError)
	assert.Zero(t, s.LLMEmbeddingDim)

	require.NoError(t, d.WriteEnrichment(ctx, "ok", EnrichmentWrite{Status: EnrichStatusError, Error: "provider failed"}))
	s = requireSessionExists(t, d, "ok")
	assert.Equal(t, EnrichStatusError, s.EnrichStatus)
	assert.Equal(t, "provider failed", s.EnrichError)
	assert.Equal(t, "Title", s.LLMTitle, "error must not clear previous successful text fields")

	err := d.WriteEnrichment(ctx, "missing", EnrichmentWrite{Title: "x"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "session not found")
}

func TestGetEnrichmentStatus(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	for _, tc := range []struct {
		id     string
		status string
	}{
		{id: "pending"},
		{id: "ok", status: EnrichStatusOK},
		{id: "short", status: EnrichStatusSkippedTooShort},
		{id: "empty", status: EnrichStatusNoContent},
		{id: "error", status: EnrichStatusError},
	} {
		insertSession(t, d, tc.id, "proj")
		switch tc.status {
		case "":
		case EnrichStatusOK:
			require.NoError(t, d.WriteEnrichment(ctx, tc.id, EnrichmentWrite{
				Title: "title", Summary: "summary", Keywords: []string{"key"}, Model: "model", MessageCnt: 1,
			}))
		default:
			require.NoError(t, d.WriteEnrichment(ctx, tc.id, EnrichmentWrite{
				Status: tc.status, Error: "err",
			}))
		}
	}
	insertSession(t, d, "deleted", "proj")
	_, err := d.getWriter().Exec(`UPDATE sessions SET deleted_at = ? WHERE id = ?`, "2026-06-24T10:00:00Z", "deleted")
	require.NoError(t, err)

	status, err := d.GetEnrichmentStatus(ctx)
	require.NoError(t, err)
	assert.Equal(t, 5, status.Total)
	assert.Equal(t, 1, status.Enriched)
	assert.Equal(t, 1, status.Pending)
	assert.Equal(t, 1, status.SkippedTooShort)
	assert.Equal(t, 1, status.NoContent)
	assert.Equal(t, 1, status.Errors)
	assert.Equal(t, 1, status.ByStatus[""])
	assert.Equal(t, 1, status.ByStatus[EnrichStatusOK])
}

func candidateIDSet(candidates []EnrichCandidate) map[string]bool {
	out := map[string]bool{}
	for _, c := range candidates {
		out[c.ID] = true
	}
	return out
}
