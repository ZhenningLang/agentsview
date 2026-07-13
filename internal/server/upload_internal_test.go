package server

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.kenn.io/agentsview/internal/db"
	"go.kenn.io/agentsview/internal/parser"
)

func TestSessionBatchWriteFromParsedPreservesSessionName(t *testing.T) {
	sess := parser.ParsedSession{
		ID:          "test-session",
		SessionName: "My Renamed Session",
	}
	result := sessionBatchWriteFromParsed(sess, nil)
	require.NotNil(t, result.Session.SessionName,
		"SessionName must be persisted on upload")
	require.Equal(t, "My Renamed Session", *result.Session.SessionName)
	// DisplayName must NOT be set by the converter — only RenameSession sets it.
	assert.Nil(t, result.Session.DisplayName,
		"converter must not set DisplayName")
}

func TestSessionBatchWriteFromParsedNoSessionName(t *testing.T) {
	sess := parser.ParsedSession{
		ID: "test-session-no-name",
	}
	result := sessionBatchWriteFromParsed(sess, nil)
	require.Nil(t, result.Session.SessionName,
		"SessionName must be nil when not set")
	assert.Nil(t, result.Session.DisplayName,
		"DisplayName must be nil when not set")
}

func TestSessionBatchWriteFromParsedSanitizesThroughUploadPersistence(t *testing.T) {
	database, err := db.Open(filepath.Join(t.TempDir(), "upload.db"))
	require.NoError(t, err)
	t.Cleanup(func() { database.Close() })
	longModel := strings.Repeat("m", db.MaxModelLen+10)
	badTime := time.Date(2999, 1, 1, 0, 0, 0, 0, time.UTC)
	sess := parser.ParsedSession{
		ID: "upload-validation", Project: "proj\x07ect",
		Machine: "remote", Agent: parser.AgentClaude,
		FirstMessage: "prompt\x1b", StartedAt: badTime, EndedAt: badTime,
		MessageCount: 1, TotalOutputTokens: 3_000_000,
		PeakContextTokens:    3_000_000,
		HasTotalOutputTokens: true, HasPeakContextTokens: true,
		AggregateTokenSource: parser.TokenAggregateSummary,
	}
	write := sessionBatchWriteFromParsed(sess, []parser.ParsedMessage{{
		Ordinal: 0, Role: parser.RoleType("wizard"),
		Content: "answer\x07", ContentLength: len("answer\x07"),
		Timestamp: badTime, Model: longModel,
		ContextTokens: 3_000_000, OutputTokens: 3_000_000,
		HasContextTokens: true, HasOutputTokens: true,
	}})

	result, err := database.WriteSessionBatchAtomic(
		[]db.SessionBatchWrite{write},
	)
	require.NoError(t, err)
	require.Equal(t, 1, result.WrittenSessions)

	stored, err := database.GetSessionFull(
		context.Background(), sess.ID,
	)
	require.NoError(t, err)
	require.NotNil(t, stored)
	assert.Equal(t, "project", stored.Project)
	assert.Equal(t, 3_000_000, stored.TotalOutputTokens)
	assert.Equal(t, 3_000_000, stored.PeakContextTokens)
	assert.Nil(t, stored.StartedAt)
	assert.Nil(t, stored.EndedAt)
	msgs, err := database.GetAllMessages(context.Background(), sess.ID)
	require.NoError(t, err)
	require.Len(t, msgs, 1)
	assert.Empty(t, msgs[0].Role)
	assert.Equal(t, "answer", msgs[0].Content)
	assert.Len(t, msgs[0].Model, db.MaxModelLen)
	assert.Equal(t, db.MaxPlausibleTokens, msgs[0].ContextTokens)
	assert.Equal(t, db.MaxPlausibleTokens, msgs[0].OutputTokens)
	assert.Empty(t, msgs[0].Timestamp)
}
