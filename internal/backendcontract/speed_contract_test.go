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
	return speedContractFixture{
		since:            base,
		until:            base.Add(time.Hour),
		qualifiedSession: "speed-a",
		sparseSession:    "speed-sparse",
	}
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
	assert.Equal(t, 9, rest.Points[0].N)
	require.NotNil(t, rest.Points[0].P50)
	assert.InDelta(t, 10, *rest.Points[0].P50, 1e-9)

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
