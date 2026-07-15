//go:build pgtest

package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.kenn.io/agentsview/internal/db"
)

func TestPGSpeedTrendAndSessionSpeed(t *testing.T) {
	pgURL := testPGURL(t)
	ensureStoreSchema(t, pgURL)
	store, err := NewStore(pgURL, testSchema, true)
	require.NoError(t, err)
	defer store.Close()

	const sessionID = "speed-pg"
	timingResetSession(t, store.DB(), sessionID)
	timingInsertSessionPG(t, store.DB(), sessionID,
		"2026-01-22T00:00:00Z", "2026-01-22T00:01:00Z")
	for ordinal := range 6 {
		role := "assistant"
		if ordinal == 0 {
			role = "user"
		}
		_, err := store.DB().Exec(`
			INSERT INTO messages (
				session_id, ordinal, role, content, timestamp, content_length,
				output_tokens, has_output_tokens, model
			) VALUES ($1, $2, $3, '', $4::timestamptz, 0, $5, $6, 'claude-test')`,
			sessionID, ordinal, role,
			time.Date(2026, time.January, 22, 0, 0, ordinal*10, 0, time.UTC),
			100, ordinal > 0)
		require.NoError(t, err)
	}

	trend, err := store.GetSpeedTrend(context.Background(), db.SpeedTrendQuery{
		Since:     time.Date(2026, time.January, 22, 0, 0, 0, 0, time.UTC),
		Until:     time.Date(2026, time.January, 22, 1, 0, 0, 0, time.UTC),
		BucketSec: 3600,
		GroupBy:   "agent",
	})
	require.NoError(t, err)
	require.Len(t, trend.Series, 1)
	require.Len(t, trend.Series[0].Points, 1)
	assert.Equal(t, 5, trend.Series[0].Points[0].N)
	require.NotNil(t, trend.Series[0].Points[0].P50)
	assert.Equal(t, 10.0, *trend.Series[0].Points[0].P50)

	result, err := store.GetSessionSpeed(context.Background(), sessionID)
	require.NoError(t, err)
	require.NotNil(t, result.Speed)
	assert.Equal(t, 10.0, result.Speed.TokPerSec)
}
