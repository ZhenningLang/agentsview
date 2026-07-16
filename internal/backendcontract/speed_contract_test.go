package backendcontract

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.kenn.io/agentsview/internal/db"
	"go.kenn.io/agentsview/internal/dbtest"
	duckdbstore "go.kenn.io/agentsview/internal/duckdb"
)

type speedContractFixture struct {
	since            time.Time
	until            time.Time
	qualifiedSession string
	sparseSession    string
}

func TestSpeedStoreContract(t *testing.T) {
	t.Run("sqlite", func(t *testing.T) {
		local := dbtest.OpenTestDB(t)
		fixture := seedSpeedContractFixture(t, local)
		assertSpeedStoreContract(t, local, fixture)
	})

	t.Run("duckdb", func(t *testing.T) {
		local := dbtest.OpenTestDB(t)
		fixture := seedSpeedContractFixture(t, local)
		syncer, err := duckdbstore.New(
			filepath.Join(t.TempDir(), "speed-contract.duckdb"),
			local,
			"local",
			duckdbstore.SyncOptions{},
		)
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, syncer.Close()) })
		_, err = syncer.Push(context.Background(), true, nil)
		require.NoError(t, err)
		assertSpeedStoreContract(t, duckdbstore.NewStoreFromDB(syncer.DB()), fixture)
	})
}

func seedSpeedContractFixture(t *testing.T, store *db.DB) speedContractFixture {
	t.Helper()
	base := time.Date(2026, time.January, 15, 12, 0, 0, 0, time.UTC)
	for _, agent := range []string{"a", "b", "c", "d", "e", "f", "g", "h", "i"} {
		seedSpeedContractSession(t, store, "speed-"+agent, agent, 5, base)
	}
	seedSpeedContractSession(t, store, "speed-sparse", "sparse", 4, base)
	seedSpeedBurstSession(t, store, base)
	return speedContractFixture{
		since:            base,
		until:            base.Add(time.Hour),
		qualifiedSession: "speed-a",
		sparseSession:    "speed-sparse",
	}
}

// seedSpeedBurstSession writes one session whose assistant rows must merge
// into a single generation event: two sub-second fragments plus a third row
// joined across a larger gap by a shared request ID.
func seedSpeedBurstSession(t *testing.T, store *db.DB, base time.Time) {
	t.Helper()
	started := base.Format(time.RFC3339Nano)
	session := db.Session{
		ID:              "speed-burst",
		Project:         "speed-contract",
		Machine:         "local",
		Agent:           "burst",
		MessageCount:    4,
		StartedAt:       &started,
		EndedAt:         &started,
		LocalModifiedAt: &started,
	}
	messages := []db.Message{
		{SessionID: "speed-burst", Ordinal: 0, Role: "user",
			Timestamp: base.Format(time.RFC3339Nano)},
		{SessionID: "speed-burst", Ordinal: 1, Role: "assistant",
			Timestamp: base.Add(10 * time.Second).Format(time.RFC3339Nano),
			Model:     "model-burst", OutputTokens: 40, HasOutputTokens: true,
			ClaudeRequestID: "req-1"},
		{SessionID: "speed-burst", Ordinal: 2, Role: "assistant",
			Timestamp: base.Add(11 * time.Second).Format(time.RFC3339Nano),
			Model:     "model-burst", OutputTokens: 60, HasOutputTokens: true,
			ClaudeRequestID: "req-1"},
		{SessionID: "speed-burst", Ordinal: 3, Role: "assistant",
			Timestamp: base.Add(14 * time.Second).Format(time.RFC3339Nano),
			Model:     "model-burst", OutputTokens: 24, HasOutputTokens: true,
			ClaudeRequestID: "req-1"},
	}
	_, err := store.WriteSessionBatchAtomic([]db.SessionBatchWrite{{
		Session:         session,
		Messages:        messages,
		DataVersion:     1,
		ReplaceMessages: true,
	}})
	require.NoError(t, err)
}

func seedSpeedContractSession(
	t *testing.T, store *db.DB, sessionID, agent string, sampleCount int, base time.Time,
) {
	t.Helper()
	started := base.Format(time.RFC3339Nano)
	session := db.Session{
		ID:              sessionID,
		Project:         "speed-contract",
		Machine:         "local",
		Agent:           agent,
		MessageCount:    sampleCount + 1,
		StartedAt:       &started,
		EndedAt:         &started,
		LocalModifiedAt: &started,
	}
	messages := []db.Message{{
		SessionID: sessionID,
		Ordinal:   0,
		Role:      "user",
		Timestamp: base.Format(time.RFC3339Nano),
	}}
	for ordinal := 1; ordinal <= sampleCount; ordinal++ {
		messages = append(messages, db.Message{
			SessionID:       sessionID,
			Ordinal:         ordinal,
			Role:            "assistant",
			Timestamp:       base.Add(time.Duration(ordinal) * 10 * time.Second).Format(time.RFC3339Nano),
			Model:           "model-" + agent,
			OutputTokens:    100,
			HasOutputTokens: true,
		})
	}
	_, err := store.WriteSessionBatchAtomic([]db.SessionBatchWrite{{
		Session:         session,
		Messages:        messages,
		DataVersion:     1,
		ReplaceMessages: true,
	}})
	require.NoError(t, err)
}

func assertSpeedStoreContract(t *testing.T, store db.Store, fixture speedContractFixture) {
	t.Helper()
	ctx := context.Background()

	empty, err := store.GetSpeedTrend(ctx, db.SpeedTrendQuery{
		Since: fixture.since, Until: fixture.until, BucketSec: 900, GroupBy: "agent", Agent: "missing",
	})
	require.NoError(t, err)
	assert.Empty(t, empty.Series)

	trend, err := store.GetSpeedTrend(ctx, db.SpeedTrendQuery{
		Since: fixture.since, Until: fixture.until, BucketSec: 900, GroupBy: "agent",
	})
	require.NoError(t, err)
	require.Len(t, trend.Series, 9)
	for index, key := range []string{"a", "b", "c", "d", "e", "f", "g", "h"} {
		assert.Equal(t, key, trend.Series[index].Key)
		assert.False(t, trend.Series[index].IsOther)
	}
	rest := trend.Series[8]
	assert.Equal(t, "(rest)", rest.Key)
	assert.True(t, rest.IsOther)
	require.Len(t, rest.Points, 1)
	// Nine agents fold into rest: sparse (4 events) + burst (1 merged
	// event) + agent i (5 events).
	assert.Equal(t, 10, rest.Points[0].N)
	require.NotNil(t, rest.Points[0].P50)
	assert.InDelta(t, 10, *rest.Points[0].P50, 1e-9)

	// All eleven sessions write inside the first bucket; the concurrency
	// track ignores the agent filter by contract.
	require.Len(t, trend.Concurrency, 1)
	assert.Equal(t, fixture.since.Unix()/900*900, trend.Concurrency[0].T)
	assert.Equal(t, 11, trend.Concurrency[0].Sessions)
	require.Len(t, empty.Concurrency, 1)
	assert.Equal(t, 11, empty.Concurrency[0].Sessions)

	// The burst session's three fragments merge into one event: 124
	// tokens over the 14-second span from the user turn.
	burst, err := store.GetSpeedTrend(ctx, db.SpeedTrendQuery{
		Since: fixture.since, Until: fixture.until, BucketSec: 900, GroupBy: "agent", Agent: "burst",
	})
	require.NoError(t, err)
	require.Len(t, burst.Series, 1)
	require.Len(t, burst.Series[0].Points, 1)
	assert.Equal(t, 1, burst.Series[0].Points[0].N)
	assert.Nil(t, burst.Series[0].Points[0].P50)

	sparse, err := store.GetSpeedTrend(ctx, db.SpeedTrendQuery{
		Since: fixture.since, Until: fixture.until, BucketSec: 900, GroupBy: "agent", Agent: "sparse",
	})
	require.NoError(t, err)
	require.Len(t, sparse.Series, 1)
	require.Len(t, sparse.Series[0].Points, 1)
	assert.Equal(t, 4, sparse.Series[0].Points[0].N)
	assert.Nil(t, sparse.Series[0].Points[0].P50)
	assert.Nil(t, sparse.Series[0].Points[0].P95)

	qualified, err := store.GetSessionSpeed(ctx, fixture.qualifiedSession)
	require.NoError(t, err)
	require.NotNil(t, qualified.Speed)
	assert.Equal(t, 5, qualified.Speed.SampleN)
	assert.InDelta(t, 10, qualified.Speed.TokPerSec, 1e-9)

	sparseSession, err := store.GetSessionSpeed(ctx, fixture.sparseSession)
	require.NoError(t, err)
	assert.Nil(t, sparseSession.Speed)

	rates, err := store.GetSpeedBaselineSessions(ctx, "a", fixture.since, fixture.until)
	require.NoError(t, err)
	require.Len(t, rates, 1)
	assert.Equal(t, fixture.qualifiedSession, rates[0].SessionID)
	assert.InDelta(t, 10, rates[0].TokPerSec, 1e-9)
}
