package db

import (
	"sort"
	"time"
)

const (
	minSpeedOutputTokens = 32
	maxSpeedWindowSec    = 1800
)

// SpeedMessage is the minimal ordered message representation needed to derive
// approximate output speed. TimestampValid distinguishes a missing timestamp
// from the zero time, and HasOutputTokens preserves parser coverage semantics.
type SpeedMessage struct {
	SessionID              string
	Agent                  string
	Model                  string
	Ordinal                int
	Role                   string
	Timestamp              time.Time
	TimestampValid         bool
	OutputTokens           int
	HasOutputTokens        bool
	PreviousTimestamp      time.Time
	PreviousTimestampValid bool
}

// SpeedSample is one qualifying assistant output observation.
type SpeedSample struct {
	SessionID    string
	Agent        string
	Model        string
	BucketStart  int64
	Rate         float64
	OutputTokens int
	timestamp    time.Time
}

// SpeedStats is a message-level percentile aggregate. P50 and P95 are null
// below five observations so sparse measurements do not look authoritative.
type SpeedStats struct {
	P50 *float64 `json:"p50"`
	P95 *float64 `json:"p95"`
	N   int      `json:"n"`
}

// SpeedPoint is one time bucket in a speed series.
type SpeedPoint struct {
	T   int64    `json:"t"`
	P50 *float64 `json:"p50"`
	P95 *float64 `json:"p95"`
	N   int      `json:"n"`
}

// SpeedTrendSeries is one agent/model line. The rest line is explicitly
// marked instead of relying on its display name as a sentinel.
type SpeedTrendSeries struct {
	Key     string       `json:"key"`
	IsOther bool         `json:"is_other"`
	Points  []SpeedPoint `json:"points"`
}

// SpeedTrendQuery controls the message-timestamp window used by the trend.
type SpeedTrendQuery struct {
	Since     time.Time
	Until     time.Time
	BucketSec int64
	GroupBy   string
	Agent     string
}

// SpeedTrendResponse is the response for the approximate output-speed trend.
type SpeedTrendResponse struct {
	BucketSec int64              `json:"bucket_sec"`
	GroupBy   string             `json:"group_by"`
	Since     time.Time          `json:"since"`
	Until     time.Time          `json:"until"`
	Series    []SpeedTrendSeries `json:"series"`
}

// SessionSpeed is a ratio-of-sums for one session. Baseline fields are filled
// by the HTTP layer because the per-agent baseline is shared across backends.
type SessionSpeed struct {
	TokPerSec   float64  `json:"tok_per_sec"`
	SampleN     int      `json:"sample_n"`
	BaselineP50 *float64 `json:"baseline_p50"`
	BaselineN   int      `json:"baseline_n"`
}

// SessionSpeedResult keeps the agent lookup internal to the server-side
// baseline workflow; SessionSpeed itself remains the API payload shape.
type SessionSpeedResult struct {
	Agent string
	Speed *SessionSpeed
}

// SpeedSessionRate is one qualified session's ratio-of-sums, retained so a
// cached agent baseline can exclude the session being displayed per request.
type SpeedSessionRate struct {
	SessionID string
	TokPerSec float64
}

// NewSpeedSample applies the speed eligibility contract to a current message
// and its direct ordinal predecessor. Callers must not pre-filter the ordered
// sequence before selecting prev.
func NewSpeedSample(current, prev SpeedMessage) (SpeedSample, bool) {
	if current.Role != "assistant" || !current.HasOutputTokens ||
		current.OutputTokens < minSpeedOutputTokens ||
		!current.TimestampValid || !prev.TimestampValid {
		return SpeedSample{}, false
	}
	window := current.Timestamp.Sub(prev.Timestamp).Seconds()
	if window <= 0 || window > maxSpeedWindowSec {
		return SpeedSample{}, false
	}
	return SpeedSample{
		SessionID:    current.SessionID,
		Agent:        current.Agent,
		Model:        current.Model,
		Rate:         float64(current.OutputTokens) / window,
		OutputTokens: current.OutputTokens,
		timestamp:    current.Timestamp,
	}, true
}

// NewSpeedSampleWithPreviousTimestamp uses a SQL LAG result for the direct
// ordinal predecessor. It keeps query implementations from reconstructing a
// partial sequence in Go after a current-message time-window filter.
func NewSpeedSampleWithPreviousTimestamp(current SpeedMessage) (SpeedSample, bool) {
	return NewSpeedSample(current, SpeedMessage{
		Timestamp:      current.PreviousTimestamp,
		TimestampValid: current.PreviousTimestampValid,
	})
}

// BuildSpeedSamples derives samples from complete ordinal sequences. Sessions
// must be contiguous and ordered by ordinal, as all backend queries provide.
func BuildSpeedSamples(messages []SpeedMessage) []SpeedSample {
	samples := make([]SpeedSample, 0)
	var prev SpeedMessage
	hasPrev := false
	for _, current := range messages {
		if !hasPrev || current.SessionID != prev.SessionID {
			prev = current
			hasPrev = true
			continue
		}
		if sample, ok := NewSpeedSample(current, prev); ok {
			samples = append(samples, sample)
		}
		prev = current
	}
	return samples
}

// AggregateSpeedStats computes the repository's discrete percentile rule.
func AggregateSpeedStats(samples []SpeedSample) SpeedStats {
	stats := SpeedStats{N: len(samples)}
	if len(samples) < 5 {
		return stats
	}
	rates := make([]float64, 0, len(samples))
	for _, sample := range samples {
		rates = append(rates, sample.Rate)
	}
	sort.Float64s(rates)
	p50 := percentileFloat(rates, 0.5)
	p95 := percentileFloat(rates, 0.95)
	stats.P50 = &p50
	stats.P95 = &p95
	return stats
}

// SessionSpeedFromSamples computes the session ratio-of-sums and applies the
// five-message qualification threshold used for the timing card and baseline.
func SessionSpeedFromSamples(samples []SpeedSample) *SessionSpeed {
	if len(samples) < 5 {
		return nil
	}
	var tokens int
	var seconds float64
	for _, sample := range samples {
		tokens += sample.OutputTokens
		seconds += float64(sample.OutputTokens) / sample.Rate
	}
	if seconds <= 0 {
		return nil
	}
	return &SessionSpeed{TokPerSec: float64(tokens) / seconds, SampleN: len(samples)}
}

// AggregateSpeedTrend groups samples, retains the eight largest groups, and
// recomputes statistics for the remaining groups in an explicit rest series.
func AggregateSpeedTrend(samples []SpeedSample, groupBy string) []SpeedTrendSeries {
	groups := make(map[string][]SpeedSample)
	for _, sample := range samples {
		key := sample.Agent
		if groupBy == "model" {
			key = sample.Model
			if key == "" {
				key = "unknown"
			}
		}
		groups[key] = append(groups[key], sample)
	}
	if len(groups) == 0 {
		return []SpeedTrendSeries{}
	}

	keys := make([]string, 0, len(groups))
	for key := range groups {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		left, right := len(groups[keys[i]]), len(groups[keys[j]])
		if left != right {
			return left > right
		}
		return keys[i] < keys[j]
	})

	limit := min(len(keys), 8)
	series := make([]SpeedTrendSeries, 0, limit+1)
	for _, key := range keys[:limit] {
		series = append(series, speedTrendSeries(key, false, groups[key]))
	}
	if len(keys) > limit {
		rest := make([]SpeedSample, 0)
		for _, key := range keys[limit:] {
			rest = append(rest, groups[key]...)
		}
		series = append(series, speedTrendSeries("(rest)", true, rest))
	}
	return series
}

// BucketSpeedSamples assigns UTC epoch buckets from the current assistant
// message timestamp retained while deriving each rate.
func BucketSpeedSamples(samples []SpeedSample, bucketSec int64) {
	for i := range samples {
		if bucketSec <= 0 {
			samples[i].BucketStart = 0
			continue
		}
		samples[i].BucketStart = samples[i].timestamp.UTC().Unix() / bucketSec * bucketSec
	}
}

func speedTrendSeries(key string, isOther bool, samples []SpeedSample) SpeedTrendSeries {
	byBucket := make(map[int64][]SpeedSample)
	for _, sample := range samples {
		byBucket[sample.BucketStart] = append(byBucket[sample.BucketStart], sample)
	}
	buckets := make([]int64, 0, len(byBucket))
	for bucket := range byBucket {
		buckets = append(buckets, bucket)
	}
	sort.Slice(buckets, func(i, j int) bool { return buckets[i] < buckets[j] })
	points := make([]SpeedPoint, 0, len(buckets))
	for _, bucket := range buckets {
		stats := AggregateSpeedStats(byBucket[bucket])
		points = append(points, SpeedPoint{T: bucket, P50: stats.P50, P95: stats.P95, N: stats.N})
	}
	return SpeedTrendSeries{Key: key, IsOther: isOther, Points: points}
}

// SessionSpeedBaseline selects the p50 from already-qualified session rates.
func SessionSpeedBaseline(speeds []SessionSpeed) (*float64, int) {
	n := len(speeds)
	if n < 10 {
		return nil, n
	}
	rates := make([]float64, 0, n)
	for _, speed := range speeds {
		rates = append(rates, speed.TokPerSec)
	}
	sort.Float64s(rates)
	p50 := percentileFloat(rates, 0.5)
	return &p50, n
}

// SessionSpeedBaselineExcluding derives the baseline for one session without
// querying again when the agent's qualified session rates are cached.
func SessionSpeedBaselineExcluding(rates []SpeedSessionRate, sessionID string) (*float64, int) {
	speeds := make([]SessionSpeed, 0, len(rates))
	for _, rate := range rates {
		if rate.SessionID != sessionID {
			speeds = append(speeds, SessionSpeed{TokPerSec: rate.TokPerSec})
		}
	}
	return SessionSpeedBaseline(speeds)
}

// SpeedSessionRatesFromSamples reduces qualifying message samples into the
// qualified session-rate population used for the timing-card baseline.
func SpeedSessionRatesFromSamples(samples []SpeedSample) []SpeedSessionRate {
	bySession := make(map[string][]SpeedSample)
	for _, sample := range samples {
		bySession[sample.SessionID] = append(bySession[sample.SessionID], sample)
	}
	rates := make([]SpeedSessionRate, 0, len(bySession))
	for sessionID, sessionSamples := range bySession {
		if speed := SessionSpeedFromSamples(sessionSamples); speed != nil {
			rates = append(rates, SpeedSessionRate{
				SessionID: sessionID,
				TokPerSec: speed.TokPerSec,
			})
		}
	}
	sort.Slice(rates, func(i, j int) bool {
		return rates[i].SessionID < rates[j].SessionID
	})
	return rates
}
