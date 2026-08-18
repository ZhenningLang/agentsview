package service_test

import (
	"context"
	"encoding/json"
	"net/url"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.kenn.io/agentsview/internal/db"
	"go.kenn.io/agentsview/internal/dbtest"
	"go.kenn.io/agentsview/internal/postgres"
	"go.kenn.io/agentsview/internal/service"
)

// The usage read path talks to db.Store and nothing else, so the
// PostgreSQL read store is a valid backing store for it by construction.
// Live PostgreSQL behaviour is exercised by the pgtest-tagged suite; this
// assertion is what keeps the seam from growing a *db.DB dependency that
// would silently exclude PG.
var _ = service.NewReadOnlyBackend((*postgres.Store)(nil))

// phase21UsageStoreSpy records the filters the usage path builds. Only
// GetDailyUsage is implemented: any other Store call would panic on the
// embedded nil interface instead of quietly returning a zero value.
type phase21UsageStoreSpy struct {
	db.Store
	calls  []db.UsageFilter
	result db.DailyUsageResult
	err    error
}

func (s *phase21UsageStoreSpy) GetDailyUsage(
	_ context.Context, f db.UsageFilter,
) (db.DailyUsageResult, error) {
	s.calls = append(s.calls, f)
	return s.result, s.err
}

func newPhase21UsageSvc(
	t *testing.T,
) (service.SessionService, *phase21UsageStoreSpy) {
	t.Helper()
	spy := &phase21UsageStoreSpy{result: service.EmptyUsageResult()}
	return service.NewReadOnlyBackend(spy), spy
}

// ---------------------------------------------------------------------
// Defaults and validation. These are the values every usage number is
// computed over, so a silent change here changes every reported figure.
// ---------------------------------------------------------------------

func TestPhase21UsageSummaryAppliesDefaultRange(t *testing.T) {
	t.Parallel()
	svc, spy := newPhase21UsageSvc(t)

	res, err := svc.UsageSummary(context.Background(), service.UsageRequest{})
	require.NoError(t, err)
	require.Len(t, spy.calls, 1)

	wantTo := time.Now().UTC().Format("2006-01-02")
	wantFrom := time.Now().UTC().AddDate(0, 0, -30).Format("2006-01-02")
	assert.Equal(t, wantFrom, spy.calls[0].From)
	assert.Equal(t, wantTo, spy.calls[0].To)
	assert.Equal(t, "UTC", spy.calls[0].Timezone,
		"an unset timezone must default to UTC, not the empty string")
	assert.True(t, spy.calls[0].Breakdowns,
		"the summary needs per-dimension breakdowns to fold")

	// The response echoes the defaulted range, not the empty input.
	assert.Equal(t, wantFrom, res.From)
	assert.Equal(t, wantTo, res.To)
}

func TestPhase21UsageSummaryDefaultsOneShotAndAutomated(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name                            string
		oneShot, automated              *bool
		wantExcludeOne, wantExcludeAuto bool
	}{
		{"unset matches the wire defaults", nil, nil, false, true},
		{"explicit include", new(true), new(true), false, false},
		{"explicit exclude", new(false), new(false), true, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			svc, spy := newPhase21UsageSvc(t)
			_, err := svc.UsageSummary(context.Background(), service.UsageRequest{
				IncludeOneShot: tc.oneShot, IncludeAutomated: tc.automated,
			})
			require.NoError(t, err)
			require.Len(t, spy.calls, 1)
			assert.Equal(t, tc.wantExcludeOne, spy.calls[0].ExcludeOneShot)
			assert.Equal(t, tc.wantExcludeAuto, spy.calls[0].ExcludeAutomated)
		})
	}
}

func TestPhase21UsageSummaryRejectsBadInput(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		req  service.UsageRequest
		msg  string
	}{
		{"unknown timezone",
			service.UsageRequest{Timezone: "Mars/Olympus"},
			"invalid timezone: Mars/Olympus"},
		{"malformed date",
			service.UsageRequest{From: "06-01-2024", To: "2024-06-15"},
			"invalid date format: use YYYY-MM-DD"},
		{"reversed range",
			service.UsageRequest{From: "2024-06-15", To: "2024-06-01"},
			"from must not be after to"},
		{"malformed active_since",
			service.UsageRequest{ActiveSince: "yesterday"},
			"invalid active_since: use RFC3339 timestamp"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			svc, spy := newPhase21UsageSvc(t)
			res, err := svc.UsageSummary(context.Background(), tc.req)
			require.Error(t, err)
			var inputErr *service.UsageInputError
			require.ErrorAs(t, err, &inputErr)
			assert.Equal(t, tc.msg, inputErr.Msg)
			assert.Nil(t, res)
			assert.Empty(t, spy.calls, "invalid input must not reach the store")
		})
	}
}

// TestPhase21UsageSummaryForwardsEveryFilterField is the half-wired gate:
// a dropped filter field compiles, returns data, and quietly reports the
// wrong range or the wrong project.
func TestPhase21UsageSummaryForwardsEveryFilterField(t *testing.T) {
	t.Parallel()
	svc, spy := newPhase21UsageSvc(t)

	_, err := svc.UsageSummary(context.Background(), service.UsageRequest{
		From:             "2024-06-01",
		To:               "2024-06-15",
		Timezone:         "America/New_York",
		Agent:            "claude",
		Project:          "my-app",
		Machine:          "laptop",
		GitBranch:        "branch-token",
		ExcludeProject:   "other-app",
		ExcludeAgent:     "codex",
		ExcludeModel:     "old-model",
		Model:            "some-model",
		MinUserMessages:  3,
		ActiveSince:      "2024-06-10T00:00:00Z",
		IncludeOneShot:   new(false),
		IncludeAutomated: new(true),
	})
	require.NoError(t, err)
	require.Len(t, spy.calls, 1)

	assert.Equal(t, db.UsageFilter{
		From:             "2024-06-01",
		To:               "2024-06-15",
		Timezone:         "America/New_York",
		Agent:            "claude",
		Project:          "my-app",
		Machine:          "laptop",
		GitBranch:        "branch-token",
		ExcludeProject:   "other-app",
		ExcludeAgent:     "codex",
		ExcludeModel:     "old-model",
		Model:            "some-model",
		MinUserMessages:  3,
		ActiveSince:      "2024-06-10T00:00:00Z",
		ExcludeOneShot:   true,
		ExcludeAutomated: false,
		Breakdowns:       true,
	}, spy.calls[0])
}

// ---------------------------------------------------------------------
// Folds and session counts.
// ---------------------------------------------------------------------

func phase21UsageResult() db.DailyUsageResult {
	return db.DailyUsageResult{
		Totals: db.UsageTotals{
			InputTokens: 100, OutputTokens: 50,
			CacheCreationTokens: 10, CacheReadTokens: 300,
			TotalCost: 9, HasCost: true, CacheSavings: 4.5,
		},
		SessionCounts: db.UsageSessionCounts{
			Total:     3,
			ByProject: map[string]int{"cheap": 1, "pricey": 2},
			ByAgent:   map[string]int{"claude": 3},
		},
		Daily: []db.DailyUsageEntry{
			{
				Date: "2024-06-01",
				ProjectBreakdowns: []db.ProjectBreakdown{
					{Project: "cheap", InputTokens: 10, Cost: 1},
					{Project: "pricey", InputTokens: 20, Cost: 5},
				},
				ModelBreakdowns: []db.ModelBreakdown{
					{ModelName: "sonnet", OutputTokens: 5, Cost: 2},
				},
				AgentBreakdowns: []db.AgentBreakdown{
					{Agent: "claude", CacheReadTokens: 7, Cost: 3},
				},
				MachineBreakdowns: []db.MachineBreakdown{
					{Machine: "laptop", CacheCreationTokens: 2, Cost: 1},
				},
			},
			{
				Date: "2024-06-02",
				ProjectBreakdowns: []db.ProjectBreakdown{
					{Project: "cheap", InputTokens: 3, Cost: 0.5},
				},
				ModelBreakdowns: []db.ModelBreakdown{
					{ModelName: "sonnet", OutputTokens: 1, Cost: 1},
					{ModelName: "opus", OutputTokens: 9, Cost: 6},
				},
				AgentBreakdowns: []db.AgentBreakdown{
					{Agent: "claude", CacheReadTokens: 1, Cost: 1},
				},
				MachineBreakdowns: []db.MachineBreakdown{
					{Machine: "desktop", CacheCreationTokens: 4, Cost: 3},
				},
			},
		},
	}
}

func TestPhase21UsageSummaryFoldsBreakdowns(t *testing.T) {
	t.Parallel()
	spy := &phase21UsageStoreSpy{result: phase21UsageResult()}
	svc := service.NewReadOnlyBackend(spy)

	res, err := svc.UsageSummary(context.Background(), service.UsageRequest{
		From: "2024-06-01", To: "2024-06-02",
	})
	require.NoError(t, err)

	// Range-wide sums, ordered by cost descending.
	assert.Equal(t, []service.ProjectTotal{
		{Project: "pricey", InputTokens: 20, Cost: 5},
		{Project: "cheap", InputTokens: 13, Cost: 1.5},
	}, res.ProjectTotals)
	assert.Equal(t, []service.ModelTotal{
		{Model: "opus", OutputTokens: 9, Cost: 6},
		{Model: "sonnet", OutputTokens: 6, Cost: 3},
	}, res.ModelTotals)
	assert.Equal(t, []service.AgentTotal{
		{Agent: "claude", CacheReadTokens: 8, Cost: 4},
	}, res.AgentTotals)
	assert.Equal(t, []service.MachineTotal{
		{Machine: "desktop", CacheCreationTokens: 4, Cost: 3},
		{Machine: "laptop", CacheCreationTokens: 2, Cost: 1},
	}, res.MachineTotals)

	// Totals, session counts and daily rows pass through untouched.
	assert.Equal(t, 3, res.SessionCounts.Total)
	assert.Equal(t, map[string]int{"cheap": 1, "pricey": 2},
		res.SessionCounts.ByProject)
	assert.Equal(t, 9.0, res.Totals.TotalCost)
	assert.Len(t, res.Daily, 2)

	// Cache stats are derived, not copied.
	assert.Equal(t, 300, res.CacheStats.CacheReadTokens)
	assert.Equal(t, 100, res.CacheStats.UncachedInputTokens)
	assert.Equal(t, 4.5, res.CacheStats.SavingsVsUncached)
	assert.InDelta(t, 300.0/400.0, res.CacheStats.HitRate, 1e-9)
}

// ---------------------------------------------------------------------
// Pairwise: Phase 17 metrics semantics plus the empty-intersection rule.
// ---------------------------------------------------------------------

func phase21PairwiseReq(
	left, right string,
) service.UsagePairwiseComparisonRequest {
	return service.UsagePairwiseComparisonRequest{
		UsageRequest: service.UsageRequest{
			From: "2024-06-01", To: "2024-06-15",
		},
		LeftDimension:  service.PairwiseDimensionModel,
		LeftValue:      left,
		RightDimension: service.PairwiseDimensionModel,
		RightValue:     right,
	}
}

func TestPhase21PairwiseNarrowsEachSideAndDropsBreakdowns(t *testing.T) {
	t.Parallel()
	svc, spy := newPhase21UsageSvc(t)

	_, err := svc.UsagePairwiseComparison(
		context.Background(), phase21PairwiseReq("opus", "sonnet"))
	require.NoError(t, err)
	require.Len(t, spy.calls, 2, "each side is one store call")

	assert.Equal(t, "opus", spy.calls[0].Model)
	assert.Equal(t, "sonnet", spy.calls[1].Model)
	for i, call := range spy.calls {
		assert.False(t, call.Breakdowns,
			"side %d must not pay for breakdowns it never folds", i)
	}
}

func TestPhase21PairwiseProjectDimensionNarrowsProject(t *testing.T) {
	t.Parallel()
	svc, spy := newPhase21UsageSvc(t)

	req := phase21PairwiseReq("", "")
	req.LeftDimension = service.PairwiseDimensionProject
	req.LeftValue = "alpha"
	req.RightDimension = service.PairwiseDimensionProject
	req.RightValue = "beta"

	_, err := svc.UsagePairwiseComparison(context.Background(), req)
	require.NoError(t, err)
	require.Len(t, spy.calls, 2)
	assert.Equal(t, "alpha", spy.calls[0].Project)
	assert.Equal(t, "beta", spy.calls[1].Project)
}

// TestPhase21PairwiseEmptyIntersectionSkipsTheStore keeps the Phase 17
// rule: a side excluded by the shared filter has no rows by definition,
// so it must report zeroes rather than run an unconstrained query that
// would return the other side's data.
func TestPhase21PairwiseEmptyIntersectionSkipsTheStore(t *testing.T) {
	t.Parallel()
	spy := &phase21UsageStoreSpy{result: phase21UsageResult()}
	svc := service.NewReadOnlyBackend(spy)

	req := phase21PairwiseReq("opus", "sonnet")
	// The shared filter already restricts to opus, so the right side
	// intersects to nothing.
	req.Model = "opus"

	res, err := svc.UsagePairwiseComparison(context.Background(), req)
	require.NoError(t, err)
	require.Len(t, spy.calls, 1, "the empty side must not be queried")
	assert.Equal(t, "opus", spy.calls[0].Model)

	assert.False(t, res.Left.Empty)
	assert.True(t, res.Right.Empty, "the empty side must say so")
	assert.Equal(t, 0, res.RightMetrics.SessionCount)
	assert.Equal(t, 0.0, res.RightMetrics.TotalCost)
	assert.Equal(t, 0, res.RightMetrics.TotalTokens)
	assert.Nil(t, res.RightMetrics.CostPerSession,
		"no sessions means no per-session average, not a division by zero")
}

func TestPhase21PairwiseComputesPhase17Metrics(t *testing.T) {
	t.Parallel()
	result := phase21UsageResult()
	spy := &phase21UsageStoreSpy{result: result}
	svc := service.NewReadOnlyBackend(spy)

	res, err := svc.UsagePairwiseComparison(
		context.Background(), phase21PairwiseReq("opus", "sonnet"))
	require.NoError(t, err)

	// Both sides see the same fixture, so the metrics must match the
	// shared Phase 17 builder exactly and the deltas must be zero.
	want := service.UsageMetricsFromResult(result)
	assert.Equal(t, want, res.LeftMetrics)
	assert.Equal(t, want, res.RightMetrics)
	assert.Equal(t, 0.0, res.Deltas.TotalCost)
	assert.Equal(t, 0, res.Deltas.TotalTokens)
	assert.Equal(t, service.PairwiseSide{
		Dimension: service.PairwiseDimensionModel, Value: "opus",
	}, res.Left)
}

func TestPhase21PairwiseRejectsUnknownDimension(t *testing.T) {
	t.Parallel()
	svc, spy := newPhase21UsageSvc(t)

	req := phase21PairwiseReq("opus", "sonnet")
	req.RightDimension = "machine"

	res, err := svc.UsagePairwiseComparison(context.Background(), req)
	require.Error(t, err)
	var inputErr *service.UsageInputError
	require.ErrorAs(t, err, &inputErr)
	assert.Equal(t, "invalid pairwise dimension: machine", inputErr.Msg)
	assert.Nil(t, res)
	assert.Empty(t, spy.calls,
		"neither side may be queried before both dimensions validate")
}

// ---------------------------------------------------------------------
// HTTP transport: full parameter forwarding and shared validation.
// ---------------------------------------------------------------------

func phase21FullUsageRequest() service.UsageRequest {
	return service.UsageRequest{
		From:             "2024-06-01",
		To:               "2024-06-15",
		Timezone:         "America/New_York",
		Agent:            "claude",
		Project:          "my-app",
		Machine:          "laptop",
		GitBranch:        "branch-token",
		ExcludeProject:   "other-app",
		ExcludeAgent:     "codex",
		ExcludeModel:     "old-model",
		Model:            "some-model",
		MinUserMessages:  3,
		ActiveSince:      "2024-06-10T00:00:00Z",
		IncludeOneShot:   new(false),
		IncludeAutomated: new(true),
		NoCache:          true,
	}
}

func phase21FullUsageQuery() url.Values {
	return url.Values{
		"from":              {"2024-06-01"},
		"to":                {"2024-06-15"},
		"timezone":          {"America/New_York"},
		"agent":             {"claude"},
		"project":           {"my-app"},
		"machine":           {"laptop"},
		"git_branch":        {"branch-token"},
		"exclude_project":   {"other-app"},
		"exclude_agent":     {"codex"},
		"exclude_model":     {"old-model"},
		"model":             {"some-model"},
		"min_user_messages": {"3"},
		"active_since":      {"2024-06-10T00:00:00Z"},
		"include_one_shot":  {"false"},
		"include_automated": {"true"},
		"no_cache":          {"true"},
	}
}

func TestPhase21HTTPUsageSummaryForwardsEveryQueryParam(t *testing.T) {
	t.Parallel()
	cap := &phase21QueryCapture{body: service.UsageSummaryResult{}}
	svc := service.NewHTTPBackend(cap.start(t), "", false)

	_, err := svc.UsageSummary(context.Background(), phase21FullUsageRequest())
	require.NoError(t, err)
	require.Len(t, cap.queries, 1)
	assert.Equal(t, "/api/v1/usage/summary", cap.paths[0])
	assert.Equal(t, phase21FullUsageQuery(), cap.queries[0])
}

// TestPhase21HTTPUsageSummaryOmitsUnsetFlags leaves the route on its own
// documented defaults instead of pinning them from the client, which is
// what keeps an unset request identical on both transports.
func TestPhase21HTTPUsageSummaryOmitsUnsetFlags(t *testing.T) {
	t.Parallel()
	cap := &phase21QueryCapture{body: service.UsageSummaryResult{}}
	svc := service.NewHTTPBackend(cap.start(t), "", false)

	_, err := svc.UsageSummary(context.Background(), service.UsageRequest{
		From: "2024-06-01", To: "2024-06-15",
	})
	require.NoError(t, err)
	require.Len(t, cap.queries, 1)
	assert.Equal(t, url.Values{
		"from": {"2024-06-01"},
		"to":   {"2024-06-15"},
	}, cap.queries[0])
}

func TestPhase21HTTPPairwiseForwardsEveryQueryParam(t *testing.T) {
	t.Parallel()
	cap := &phase21QueryCapture{body: service.PairwiseComparisonResponse{}}
	svc := service.NewHTTPBackend(cap.start(t), "", false)

	req := service.UsagePairwiseComparisonRequest{
		UsageRequest:   phase21FullUsageRequest(),
		LeftDimension:  service.PairwiseDimensionModel,
		LeftValue:      "opus",
		RightDimension: service.PairwiseDimensionProject,
		RightValue:     "my-app",
	}
	_, err := svc.UsagePairwiseComparison(context.Background(), req)
	require.NoError(t, err)
	require.Len(t, cap.queries, 1)
	assert.Equal(t, "/api/v1/usage/pairwise-comparison", cap.paths[0])

	want := phase21FullUsageQuery()
	want.Set("left_dimension", "model")
	want.Set("left_value", "opus")
	want.Set("right_dimension", "project")
	want.Set("right_value", "my-app")
	assert.Equal(t, want, cap.queries[0],
		"the pairwise call carries the whole usage filter, not just the sides")
}

func TestPhase21HTTPUsageValidatesLocally(t *testing.T) {
	t.Parallel()
	cap := &phase21QueryCapture{body: service.UsageSummaryResult{}}
	svc := service.NewHTTPBackend(cap.start(t), "", false)

	_, err := svc.UsageSummary(context.Background(), service.UsageRequest{
		Timezone: "Mars/Olympus",
	})
	var inputErr *service.UsageInputError
	require.ErrorAs(t, err, &inputErr)
	assert.Equal(t, "invalid timezone: Mars/Olympus", inputErr.Msg)
	assert.Empty(t, cap.paths, "invalid input must not reach the daemon")

	_, err = svc.UsagePairwiseComparison(context.Background(),
		service.UsagePairwiseComparisonRequest{
			LeftDimension:  "machine",
			LeftValue:      "laptop",
			RightDimension: service.PairwiseDimensionModel,
			RightValue:     "opus",
		})
	require.ErrorAs(t, err, &inputErr)
	assert.Equal(t, "invalid pairwise dimension: machine", inputErr.Msg)
	assert.Empty(t, cap.paths)
}

// ---------------------------------------------------------------------
// Transport parity against a real server over a real SQLite archive.
// ---------------------------------------------------------------------

// seedPhase21Usage writes one session whose assistant messages carry
// token usage, which is what the daily usage query reads.
func seedPhase21Usage(
	t *testing.T, d *db.DB, id, project, model, day string, msgs int,
) {
	t.Helper()
	ts := day + "T09:00:00Z"
	dbtest.SeedSession(t, d, id, project, func(s *db.Session) {
		s.Agent = "claude"
		s.StartedAt = &ts
		s.EndedAt = &ts
		s.MessageCount = msgs
		s.UserMessageCount = msgs
	})
	rows := make([]db.Message, msgs)
	for i := range rows {
		rows[i] = db.Message{
			SessionID:     id,
			Ordinal:       i,
			Role:          "assistant",
			Content:       "reply",
			ContentLength: 5,
			Timestamp:     ts,
			Model:         model,
			TokenUsage: json.RawMessage(
				`{"input_tokens":100,"output_tokens":50,` +
					`"cache_creation_input_tokens":10,` +
					`"cache_read_input_tokens":20}`),
		}
	}
	require.NoError(t, d.ReplaceSessionMessages(id, rows))
}

func TestPhase21UsageTransportParity(t *testing.T) {
	t.Parallel()
	baseURL, d := newHTTPTestServer(t)
	seedPhase21Usage(t, d, "p21-u1", "alpha", "sonnet", "2024-06-02", 2)
	seedPhase21Usage(t, d, "p21-u2", "beta", "opus", "2024-06-03", 3)

	direct := service.NewDirectBackend(d, nil)
	remote := service.NewHTTPBackend(baseURL, "", false)
	ctx := context.Background()

	base := service.UsageRequest{From: "2024-06-01", To: "2024-06-15"}
	reqs := []service.UsageRequest{
		base,
		{From: base.From, To: base.To, Project: "alpha"},
		{From: base.From, To: base.To, Model: "opus"},
		{From: base.From, To: base.To, Timezone: "America/New_York"},
		{From: "2020-01-01", To: "2020-01-02"},
	}
	for i, req := range reqs {
		t.Run("summary/"+strconv.Itoa(i), func(t *testing.T) {
			want, err := direct.UsageSummary(ctx, req)
			require.NoError(t, err)
			got, err := remote.UsageSummary(ctx, req)
			require.NoError(t, err)
			assert.Equal(t, want, got, "transports disagree for %+v", req)
		})
	}

	// A non-empty summary is what makes the parity meaningful.
	summary, err := direct.UsageSummary(ctx, base)
	require.NoError(t, err)
	assert.Equal(t, 2, summary.SessionCounts.Total,
		"fixture should produce usage rows for both sessions")
	assert.NotEmpty(t, summary.ProjectTotals)
	assert.NotEmpty(t, summary.ModelTotals)

	pairwise := []service.UsagePairwiseComparisonRequest{
		{
			UsageRequest:   base,
			LeftDimension:  service.PairwiseDimensionModel,
			LeftValue:      "sonnet",
			RightDimension: service.PairwiseDimensionModel,
			RightValue:     "opus",
		},
		{
			UsageRequest:   base,
			LeftDimension:  service.PairwiseDimensionProject,
			LeftValue:      "alpha",
			RightDimension: service.PairwiseDimensionProject,
			RightValue:     "beta",
		},
		{
			UsageRequest: service.UsageRequest{
				From: base.From, To: base.To, Model: "opus",
			},
			LeftDimension:  service.PairwiseDimensionModel,
			LeftValue:      "opus",
			RightDimension: service.PairwiseDimensionModel,
			RightValue:     "sonnet",
		},
	}
	for i, req := range pairwise {
		t.Run("pairwise/"+strconv.Itoa(i), func(t *testing.T) {
			want, err := direct.UsagePairwiseComparison(ctx, req)
			require.NoError(t, err)
			got, err := remote.UsagePairwiseComparison(ctx, req)
			require.NoError(t, err)
			assert.Equal(t, want, got, "transports disagree for %+v", req)
		})
	}
}
