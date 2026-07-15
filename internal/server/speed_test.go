package server

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.kenn.io/agentsview/internal/db"
)

type speedCountingStore struct {
	db.Store
	sessionResults map[string]db.SessionSpeedResult
	rates          []db.SpeedSessionRate
	baselineCalls  int
}

func (s *speedCountingStore) GetSessionSpeed(
	_ context.Context, sessionID string,
) (db.SessionSpeedResult, error) {
	return s.sessionResults[sessionID], nil
}

func (s *speedCountingStore) GetSpeedBaselineSessions(
	_ context.Context, _ string, _, _ time.Time,
) ([]db.SpeedSessionRate, error) {
	s.baselineCalls++
	return s.rates, nil
}

func TestEnrichSessionTimingSpeedUsesCacheAndExcludesCurrentSession(t *testing.T) {
	rates := make([]db.SpeedSessionRate, 0, 11)
	for index := 0; index < 11; index++ {
		rates = append(rates, db.SpeedSessionRate{
			SessionID: string(rune('a' + index)),
			TokPerSec: float64(index + 1),
		})
	}
	store := &speedCountingStore{
		sessionResults: map[string]db.SessionSpeedResult{
			"a": {Agent: "claude", Speed: &db.SessionSpeed{TokPerSec: 1, SampleN: 5}},
			"k": {Agent: "claude", Speed: &db.SessionSpeed{TokPerSec: 11, SampleN: 5}},
		},
		rates: rates,
	}
	server := &Server{
		db: store,
		speedBaselineCache: newTTLCache[[]db.SpeedSessionRate](
			speedBaselineCacheTTL, speedBaselineCacheMaxEntries,
		),
	}

	first := &db.SessionTiming{SessionID: "a"}
	require.NoError(t, server.enrichSessionTimingSpeed(context.Background(), first))
	require.NotNil(t, first.Speed)
	require.NotNil(t, first.Speed.BaselineP50)
	assert.Equal(t, 7.0, *first.Speed.BaselineP50)
	assert.Equal(t, 10, first.Speed.BaselineN)

	second := &db.SessionTiming{SessionID: "k"}
	require.NoError(t, server.enrichSessionTimingSpeed(context.Background(), second))
	require.NotNil(t, second.Speed)
	require.NotNil(t, second.Speed.BaselineP50)
	assert.Equal(t, 6.0, *second.Speed.BaselineP50)
	assert.Equal(t, 10, second.Speed.BaselineN)
	assert.Equal(t, 1, store.baselineCalls, "same agent must reuse cached rates")
}

func TestEnrichSessionTimingSpeedSkipsBaselineForIneligibleSession(t *testing.T) {
	store := &speedCountingStore{
		sessionResults: map[string]db.SessionSpeedResult{"s": {Agent: "claude"}},
	}
	server := &Server{
		db: store,
		speedBaselineCache: newTTLCache[[]db.SpeedSessionRate](
			speedBaselineCacheTTL, speedBaselineCacheMaxEntries,
		),
	}
	timing := &db.SessionTiming{SessionID: "s"}
	require.NoError(t, server.enrichSessionTimingSpeed(context.Background(), timing))
	assert.Nil(t, timing.Speed)
	assert.Equal(t, 0, store.baselineCalls)
}

func TestSessionTimingSpeedJSONStates(t *testing.T) {
	tests := []struct {
		name  string
		speed *db.SessionSpeed
		want  string
	}{
		{"ineligible session", nil, `"speed":null`},
		{"sparse baseline", &db.SessionSpeed{TokPerSec: 10, SampleN: 5, BaselineN: 9}, `"baseline_p50":null`},
		{"measurable baseline", &db.SessionSpeed{TokPerSec: 10, SampleN: 5, BaselineP50: speedFloat(12), BaselineN: 10}, `"baseline_p50":12`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, err := json.Marshal(db.SessionTiming{SessionID: "session", Speed: tt.speed})
			require.NoError(t, err)
			assert.Contains(t, string(body), tt.want)
		})
	}
}

func speedFloat(value float64) *float64 { return &value }
