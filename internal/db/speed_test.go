package db

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// speedMessagesWithLag mirrors the SQL contract: assistant rows only, each
// carrying the timestamp of its direct predecessor of any role.
func speedMessagesWithLag(msgs []SpeedMessage) []SpeedMessage {
	out := make([]SpeedMessage, 0, len(msgs))
	var lagTs time.Time
	lagValid := false
	for _, m := range msgs {
		if m.Role == "assistant" {
			m.PreviousTimestamp = lagTs
			m.PreviousTimestampValid = lagValid
			out = append(out, m)
		}
		lagTs, lagValid = m.Timestamp, m.TimestampValid
	}
	return out
}

func TestSpeedSamplesUseDirectOrdinalPredecessor(t *testing.T) {
	base := time.Date(2026, time.July, 15, 12, 0, 0, 0, time.UTC)
	samples := SpeedEventsFromMessages(speedMessagesWithLag([]SpeedMessage{
		{SessionID: "s", Agent: "claude", Ordinal: 0, Role: "user", Timestamp: base, TimestampValid: true},
		{SessionID: "s", Agent: "claude", Ordinal: 1, Role: "assistant", Timestamp: base.Add(10 * time.Second), TimestampValid: true, OutputTokens: 12, HasOutputTokens: true},
		{SessionID: "s", Agent: "claude", Ordinal: 2, Role: "assistant", Timestamp: base.Add(20 * time.Second), TimestampValid: true, OutputTokens: 100, HasOutputTokens: true},
		{SessionID: "s", Agent: "claude", Ordinal: 3, Role: "user", Timestamp: base.Add(30 * time.Second), TimestampValid: true},
		{SessionID: "s", Agent: "claude", Ordinal: 4, Role: "assistant", Timestamp: base.Add(1830 * time.Second), TimestampValid: true, OutputTokens: 32, HasOutputTokens: true},
		{SessionID: "s", Agent: "claude", Ordinal: 5, Role: "assistant", Timestamp: base.Add(3631 * time.Second), TimestampValid: true, OutputTokens: 64, HasOutputTokens: true},
	}))

	require.Len(t, samples, 2)
	assert.InDelta(t, 10, samples[0].Rate, 1e-12)
	assert.Equal(t, 100, samples[0].OutputTokens)
	assert.InDelta(t, 32.0/1800, samples[1].Rate, 1e-12)
}

func TestSpeedSamplesRejectInvalidWindowsAndCoverage(t *testing.T) {
	base := time.Date(2026, time.July, 15, 12, 0, 0, 0, time.UTC)
	for _, tt := range []struct {
		name string
		cur  SpeedMessage
	}{
		{"zero window", SpeedMessage{Role: "assistant", Timestamp: base, TimestampValid: true, OutputTokens: 32, HasOutputTokens: true}},
		{"negative window", SpeedMessage{Role: "assistant", Timestamp: base.Add(-time.Second), TimestampValid: true, OutputTokens: 32, HasOutputTokens: true}},
		{"over max window", SpeedMessage{Role: "assistant", Timestamp: base.Add(1801 * time.Second), TimestampValid: true, OutputTokens: 32, HasOutputTokens: true}},
		{"short output", SpeedMessage{Role: "assistant", Timestamp: base.Add(time.Second), TimestampValid: true, OutputTokens: 31, HasOutputTokens: true}},
		{"missing output tokens", SpeedMessage{Role: "assistant", Timestamp: base.Add(time.Second), TimestampValid: true, OutputTokens: 100, HasOutputTokens: false}},
		{"missing timestamp", SpeedMessage{Role: "assistant", TimestampValid: false, OutputTokens: 100, HasOutputTokens: true}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			current := tt.cur
			current.SessionID = "s"
			current.Ordinal = 1
			samples := SpeedEventsFromMessages(speedMessagesWithLag([]SpeedMessage{
				{SessionID: "s", Ordinal: 0, Role: "user", Timestamp: base, TimestampValid: true},
				current,
			}))
			assert.Empty(t, samples)
		})
	}
}

func TestAggregateSpeedTrendGroupsAndFoldsOther(t *testing.T) {
	base := time.Date(2026, time.July, 15, 12, 0, 0, 0, time.UTC).Unix()
	var samples []SpeedSample
	for i := range 10 {
		key := string(rune('a' + i))
		for range i + 1 {
			samples = append(samples, SpeedSample{Agent: key, Model: key, BucketStart: base, Rate: float64(i + 1), OutputTokens: 32})
		}
	}
	series := AggregateSpeedTrend(samples, "model")
	require.Len(t, series, 9)
	assert.Equal(t, "j", series[0].Key)
	assert.False(t, series[0].IsOther)
	assert.Equal(t, "(rest)", series[8].Key)
	assert.True(t, series[8].IsOther)

	unknown := AggregateSpeedTrend([]SpeedSample{{Agent: "claude", BucketStart: base + 900, Rate: 10, OutputTokens: 32}}, "model")
	require.Len(t, unknown, 1)
	assert.Equal(t, "unknown", unknown[0].Key)
}

func TestBucketSpeedSamplesUsesUTCEpochBoundaries(t *testing.T) {
	base := time.Date(2026, time.July, 15, 12, 14, 59, 0, time.FixedZone("UTC+2", 2*60*60))
	samples := []SpeedSample{
		{timestamp: base},
		{timestamp: base.Add(time.Second)},
		{timestamp: base.Add(time.Minute)},
	}
	BucketSpeedSamples(samples, 900)
	assert.Equal(t, base.UTC().Truncate(15*time.Minute).Unix(), samples[0].BucketStart)
	assert.Equal(t, base.UTC().Add(time.Second).Truncate(15*time.Minute).Unix(), samples[1].BucketStart)
	assert.Equal(t, base.UTC().Add(time.Minute).Truncate(15*time.Minute).Unix(), samples[2].BucketStart)
}

func TestAggregateSpeedTrendOrdersTiesAndRecomputesRest(t *testing.T) {
	base := time.Date(2026, time.July, 15, 12, 0, 0, 0, time.UTC).Unix()
	var samples []SpeedSample
	for _, key := range []string{"i", "h", "g", "f", "e", "d", "c", "b", "a"} {
		for rate := 1; rate <= 5; rate++ {
			samples = append(samples, SpeedSample{Agent: key, BucketStart: base, Rate: float64(rate)})
		}
	}
	series := AggregateSpeedTrend(samples, "agent")
	require.Len(t, series, 9)
	assert.Equal(t, "a", series[0].Key)
	assert.Equal(t, "h", series[7].Key)
	rest := series[8]
	assert.True(t, rest.IsOther)
	require.Len(t, rest.Points, 1)
	assert.Equal(t, 5, rest.Points[0].N)
	require.NotNil(t, rest.Points[0].P50)
	assert.Equal(t, 3.0, *rest.Points[0].P50)
}

func TestAggregateSpeedStatsUsesDiscretePercentilesAndThreshold(t *testing.T) {
	stats := AggregateSpeedStats([]SpeedSample{
		{Rate: 1}, {Rate: 2}, {Rate: 3}, {Rate: 4},
	})
	assert.Equal(t, 4, stats.N)
	assert.Nil(t, stats.P50)
	assert.Nil(t, stats.P95)

	stats = AggregateSpeedStats([]SpeedSample{
		{Rate: 1}, {Rate: 2}, {Rate: 3}, {Rate: 4}, {Rate: 5}, {Rate: 6},
	})
	require.NotNil(t, stats.P50)
	require.NotNil(t, stats.P95)
	assert.Equal(t, 4.0, *stats.P50)
	assert.Equal(t, 6.0, *stats.P95)
}

func TestSessionSpeedUsesRatioOfSumsAndExcludesCurrentBaseline(t *testing.T) {
	speed := SessionSpeedFromSamples([]SpeedSample{
		{OutputTokens: 100, Rate: 10},
		{OutputTokens: 100, Rate: 20},
		{OutputTokens: 100, Rate: 20},
		{OutputTokens: 100, Rate: 20},
		{OutputTokens: 100, Rate: 20},
	})
	require.NotNil(t, speed)
	assert.InDelta(t, 16.6666666667, speed.TokPerSec, 1e-9)

	rates := make([]SpeedSessionRate, 0, 11)
	for i := range 11 {
		rates = append(rates, SpeedSessionRate{
			SessionID: string(rune('a' + i)),
			TokPerSec: float64(i + 1),
		})
	}
	p50, n := SessionSpeedBaselineExcluding(rates, "a")
	assert.Equal(t, 10, n)
	require.NotNil(t, p50)
	assert.Equal(t, 7.0, *p50)
}

func TestSQLiteSpeedTrendAndSessionSpeed(t *testing.T) {
	database := testDB(t)
	ctx := context.Background()
	base := time.Date(2026, time.July, 14, 10, 0, 0, 0, time.UTC)
	started := base.Format(time.RFC3339)
	insertSession(t, database, "speed-session", "project", func(session *Session) {
		session.Agent = "claude"
		session.StartedAt = &started
		session.EndedAt = &started
		session.MessageCount = 6
	})
	messages := make([]Message, 0, 6)
	for ordinal := range 6 {
		message := Message{
			SessionID: "speed-session",
			Ordinal:   ordinal,
			Role:      "assistant",
			Timestamp: base.Add(time.Duration(ordinal) * 10 * time.Second).Format(time.RFC3339),
			Model:     "claude-test",
		}
		if ordinal == 0 {
			message.Role = "user"
		} else {
			message.OutputTokens = 100
			message.HasOutputTokens = true
		}
		messages = append(messages, message)
	}
	insertMessages(t, database, messages...)

	trend, err := database.GetSpeedTrend(ctx, SpeedTrendQuery{
		Since:     base,
		Until:     base.Add(time.Hour),
		BucketSec: 3600,
		GroupBy:   "agent",
	})
	require.NoError(t, err)
	require.Len(t, trend.Series, 1)
	require.Len(t, trend.Series[0].Points, 1)
	assert.Equal(t, 5, trend.Series[0].Points[0].N)
	require.NotNil(t, trend.Series[0].Points[0].P50)
	assert.Equal(t, 10.0, *trend.Series[0].Points[0].P50)

	outside, err := database.GetSpeedTrend(ctx, SpeedTrendQuery{
		Since:     base.Add(time.Hour),
		Until:     base.Add(2 * time.Hour),
		BucketSec: 3600,
		GroupBy:   "agent",
	})
	require.NoError(t, err)
	assert.Empty(t, outside.Series)

	result, err := database.GetSessionSpeed(ctx, "speed-session")
	require.NoError(t, err)
	require.NotNil(t, result.Speed)
	assert.Equal(t, "claude", result.Agent)
	assert.Equal(t, 5, result.Speed.SampleN)
	assert.Equal(t, 10.0, result.Speed.TokPerSec)
}

func TestSQLiteSpeedTrendKeepsPreviousMessageOutsideRange(t *testing.T) {
	database := testDB(t)
	ctx := context.Background()
	base := time.Date(2026, time.July, 15, 12, 0, 0, 123_456_789, time.FixedZone("UTC+2", 2*60*60))
	started := base.Format(time.RFC3339Nano)
	insertSession(t, database, "speed-range", "project", func(session *Session) {
		session.Agent = "claude"
		session.StartedAt = &started
		session.EndedAt = &started
		session.MessageCount = 6
	})
	messages := []Message{{
		SessionID: "speed-range", Ordinal: 0, Role: "user", Timestamp: base.Format(time.RFC3339Nano),
	}}
	for ordinal := 1; ordinal <= 5; ordinal++ {
		messages = append(messages, Message{
			SessionID: "speed-range", Ordinal: ordinal, Role: "assistant",
			Timestamp:    base.Add(time.Duration(ordinal) * 10 * time.Second).Format(time.RFC3339Nano),
			OutputTokens: 100, HasOutputTokens: true,
		})
	}
	insertMessages(t, database, messages...)

	trend, err := database.GetSpeedTrend(ctx, SpeedTrendQuery{
		Since: base.Add(10 * time.Second), Until: base.Add(time.Minute), BucketSec: 3600, GroupBy: "agent",
	})
	require.NoError(t, err)
	require.Len(t, trend.Series, 1)
	require.Len(t, trend.Series[0].Points, 1)
	assert.Equal(t, 5, trend.Series[0].Points[0].N)
	require.NotNil(t, trend.Series[0].Points[0].P50)
	assert.InDelta(t, 10, *trend.Series[0].Points[0].P50, 1e-9)
}

func TestSpeedEventsMergeSubSecondBursts(t *testing.T) {
	base := time.Date(2026, time.July, 15, 12, 0, 0, 0, time.UTC)
	samples := SpeedEventsFromMessages(speedMessagesWithLag([]SpeedMessage{
		{SessionID: "s", Agent: "claude", Ordinal: 0, Role: "user", Timestamp: base, TimestampValid: true},
		{SessionID: "s", Agent: "claude", Ordinal: 1, Role: "assistant", Timestamp: base.Add(10 * time.Second), TimestampValid: true, OutputTokens: 40, HasOutputTokens: true, Model: "m1"},
		{SessionID: "s", Agent: "claude", Ordinal: 2, Role: "assistant", Timestamp: base.Add(11 * time.Second), TimestampValid: true, OutputTokens: 60, HasOutputTokens: true, Model: "m1"},
		{SessionID: "s", Agent: "claude", Ordinal: 3, Role: "assistant", Timestamp: base.Add(11500 * time.Millisecond), TimestampValid: true, HasOutputTokens: false},
		{SessionID: "s", Agent: "claude", Ordinal: 4, Role: "user", Timestamp: base.Add(20 * time.Second), TimestampValid: true},
		{SessionID: "s", Agent: "claude", Ordinal: 5, Role: "assistant", Timestamp: base.Add(40 * time.Second), TimestampValid: true, OutputTokens: 50, HasOutputTokens: true, Model: "m1"},
	}))

	require.Len(t, samples, 2)
	// Burst: three rows within 2s gaps collapse into one event spanning
	// user@0 -> last member at 11.5s.
	assert.Equal(t, 100, samples[0].OutputTokens)
	assert.InDelta(t, 100.0/11.5, samples[0].Rate, 1e-9)
	// Next event starts fresh; its window runs from the burst's last row
	// (the LAG predecessor at 20s is the user row).
	assert.Equal(t, 50, samples[1].OutputTokens)
	assert.InDelta(t, 50.0/20.0, samples[1].Rate, 1e-9)
}

func TestSpeedEventsMergeByRequestIDAcrossGap(t *testing.T) {
	base := time.Date(2026, time.July, 15, 12, 0, 0, 0, time.UTC)
	samples := SpeedEventsFromMessages(speedMessagesWithLag([]SpeedMessage{
		{SessionID: "s", Agent: "claude", Ordinal: 0, Role: "user", Timestamp: base, TimestampValid: true},
		{SessionID: "s", Agent: "claude", Ordinal: 1, Role: "assistant", Timestamp: base.Add(10 * time.Second), TimestampValid: true, OutputTokens: 40, HasOutputTokens: true, RequestID: "r1"},
		{SessionID: "s", Agent: "claude", Ordinal: 2, Role: "assistant", Timestamp: base.Add(15 * time.Second), TimestampValid: true, OutputTokens: 24, HasOutputTokens: true, RequestID: "r1"},
	}))

	require.Len(t, samples, 1)
	assert.Equal(t, 64, samples[0].OutputTokens)
	assert.InDelta(t, 64.0/15.0, samples[0].Rate, 1e-9)
}

func TestSpeedEventsMergedFragmentsReachTokenFloor(t *testing.T) {
	base := time.Date(2026, time.July, 15, 12, 0, 0, 0, time.UTC)
	samples := SpeedEventsFromMessages(speedMessagesWithLag([]SpeedMessage{
		{SessionID: "s", Agent: "claude", Ordinal: 0, Role: "user", Timestamp: base, TimestampValid: true},
		{SessionID: "s", Agent: "claude", Ordinal: 1, Role: "assistant", Timestamp: base.Add(10 * time.Second), TimestampValid: true, OutputTokens: 20, HasOutputTokens: true},
		{SessionID: "s", Agent: "claude", Ordinal: 2, Role: "assistant", Timestamp: base.Add(11 * time.Second), TimestampValid: true, OutputTokens: 15, HasOutputTokens: true},
	}))

	require.Len(t, samples, 1)
	assert.Equal(t, 35, samples[0].OutputTokens)
}

func TestSpeedEventsDoNotMergeAcrossSessions(t *testing.T) {
	base := time.Date(2026, time.July, 15, 12, 0, 0, 0, time.UTC)
	samples := SpeedEventsFromMessages([]SpeedMessage{
		{SessionID: "a", Agent: "claude", Role: "assistant", Timestamp: base.Add(10 * time.Second), TimestampValid: true, OutputTokens: 40, HasOutputTokens: true, PreviousTimestamp: base, PreviousTimestampValid: true},
		{SessionID: "b", Agent: "claude", Role: "assistant", Timestamp: base.Add(10500 * time.Millisecond), TimestampValid: true, OutputTokens: 60, HasOutputTokens: true, PreviousTimestamp: base, PreviousTimestampValid: true},
	})

	require.Len(t, samples, 2)
	assert.Equal(t, 40, samples[0].OutputTokens)
	assert.Equal(t, 60, samples[1].OutputTokens)
}
