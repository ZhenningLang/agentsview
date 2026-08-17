package db

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPhase24BranchTokenRoundTrip(t *testing.T) {
	tokens := JoinBranchFilterTokens(
		EncodeBranchFilterToken("alpha,repo", "feat,x"),
		EncodeBranchFilterToken("beta", "main"),
		EncodeBranchFilterToken("alpha", ""),
	)

	pairs := SplitBranchFilterTokens(tokens)
	require.Len(t, pairs, 3)
	assert.Equal(t, BranchInfo{Project: "alpha,repo", Branch: "feat,x", Token: EncodeBranchFilterToken("alpha,repo", "feat,x")}, pairs[0])
	assert.Equal(t, BranchInfo{Project: "beta", Branch: "main", Token: EncodeBranchFilterToken("beta", "main")}, pairs[1])
	assert.Equal(t, BranchInfo{Project: "alpha", Branch: "", Token: EncodeBranchFilterToken("alpha", "")}, pairs[2])
}

func TestPhase24BranchPredicateFailClosedAndParameterized(t *testing.T) {
	b := NewQueryBuilder(PostgresQueryDialect(), 2)
	pred := BranchPairPredicate("s.project", "s.git_branch", JoinBranchFilterTokens(
		EncodeBranchFilterToken("alpha,repo", "feat,x"),
		EncodeBranchFilterToken("beta", "main"),
	), func(v string) string { return b.Add(v) })

	assert.Equal(t, "((s.project = $3 AND s.git_branch = $4) OR (s.project = $5 AND s.git_branch = $6))", pred)
	assert.Equal(t, []any{"alpha,repo", "feat,x", "beta", "main"}, b.Args())
	assert.Equal(t, "1 = 0", BranchPairPredicate("project", "git_branch", "not-a-token", func(v string) string {
		return "?"
	}))
}

func TestPhase24BranchSessionFilterSQLiteBranchPairs(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	seedPhase24BranchSession(t, d, "alpha-main", "alpha", "main")
	seedPhase24BranchSession(t, d, "beta-main", "beta", "main")
	seedPhase24BranchSession(t, d, "alpha-empty", "alpha", "")
	seedPhase24BranchSession(t, d, "alpha-unknown", "alpha", "unknown")

	branches, err := d.GetBranches(ctx, false, false)
	require.NoError(t, err)
	assert.Equal(t, []BranchInfo{
		{Project: "alpha", Branch: "", Token: EncodeBranchFilterToken("alpha", "")},
		{Project: "alpha", Branch: "main", Token: EncodeBranchFilterToken("alpha", "main")},
		{Project: "alpha", Branch: "unknown", Token: EncodeBranchFilterToken("alpha", "unknown")},
		{Project: "beta", Branch: "main", Token: EncodeBranchFilterToken("beta", "main")},
	}, branches)

	page, err := d.ListSessions(ctx, SessionFilter{
		GitBranch: EncodeBranchFilterToken("alpha", "main"),
		Limit:     10,
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"alpha-main"}, phase24SessionIDs(page.Sessions))

	page, err = d.ListSessions(ctx, SessionFilter{
		GitBranch: JoinBranchFilterTokens(
			EncodeBranchFilterToken("alpha", ""),
			EncodeBranchFilterToken("beta", "main"),
		),
		Limit: 10,
	})
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"alpha-empty", "beta-main"}, phase24SessionIDs(page.Sessions))

	page, err = d.ListSessions(ctx, SessionFilter{GitBranch: "invalid", Limit: 10})
	require.NoError(t, err)
	assert.Empty(t, page.Sessions)
}

func TestPhase24ContentSearchBranchFilterSQLite(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	seedPhase24BranchSession(t, d, "alpha-main", "alpha", "main")
	seedPhase24BranchSession(t, d, "beta-main", "beta", "main")
	require.NoError(t, d.InsertMessages([]Message{
		{SessionID: "alpha-main", Ordinal: 0, Role: "assistant", Content: "phase24 shared needle alpha-only", Timestamp: "2026-08-17T00:01:00Z", ContentLength: 34},
		{SessionID: "beta-main", Ordinal: 0, Role: "assistant", Content: "phase24 shared needle beta-only", Timestamp: "2026-08-17T00:01:00Z", ContentLength: 33},
	}))

	page, err := d.SearchContent(ctx, ContentSearchFilter{
		Pattern:   "phase24 shared needle",
		Mode:      "substring",
		Sources:   []string{"messages"},
		Limit:     20,
		GitBranch: EncodeBranchFilterToken("alpha", "main"),
	})
	require.NoError(t, err)
	require.Len(t, page.Matches, 1)
	assert.Equal(t, "alpha-main", page.Matches[0].SessionID)

	page, err = d.SearchContent(ctx, ContentSearchFilter{
		Pattern:   "phase24 shared needle",
		Mode:      "substring",
		Sources:   []string{"messages"},
		Limit:     20,
		GitBranch: "invalid",
	})
	require.NoError(t, err)
	assert.Empty(t, page.Matches)

	page, err = d.SearchContent(ctx, ContentSearchFilter{
		Pattern: "phase24 shared needle",
		Mode:    "substring",
		Sources: []string{"messages"},
		Limit:   20,
	})
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"alpha-main", "beta-main"}, phase24ContentMatchSessionIDs(page.Matches))
}

func TestPhase24SidebarBranchFilterSQLite(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	seedPhase24BranchSession(t, d, "alpha-main", "alpha", "main")
	seedPhase24BranchSession(t, d, "beta-main", "beta", "main")

	index, err := d.GetSidebarSessionIndex(ctx, SessionFilter{
		GitBranch: EncodeBranchFilterToken("alpha", "main"),
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"alpha-main"}, phase24SidebarSessionIDs(index.Sessions))

	index, err = d.GetSidebarSessionIndex(ctx, SessionFilter{})
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"alpha-main", "beta-main"}, phase24SidebarSessionIDs(index.Sessions))
}

func TestPhase24UsageBranchFilterSQLite(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	seedPhase24BranchSession(t, d, "alpha-main", "alpha", "main")
	seedPhase24BranchSession(t, d, "beta-main", "beta", "main")
	seedPhase24UsageEvent(t, d, "alpha-main", 100, 50, 0.02)
	seedPhase24UsageEvent(t, d, "beta-main", 200, 75, 0.04)

	alpha := UsageFilter{
		From:      "2026-08-17",
		To:        "2026-08-17",
		Timezone:  "UTC",
		GitBranch: EncodeBranchFilterToken("alpha", "main"),
	}
	daily, err := d.GetDailyUsage(ctx, alpha)
	require.NoError(t, err)
	assert.Equal(t, 100, daily.Totals.InputTokens)
	assert.Equal(t, 50, daily.Totals.OutputTokens)
	assert.InDelta(t, 0.02, daily.Totals.TotalCost, 1e-9)
	assert.Equal(t, 1, daily.SessionCounts.Total)
	assert.Equal(t, 1, daily.SessionCounts.ByProject["alpha"])

	counts, err := d.GetUsageSessionCounts(ctx, alpha)
	require.NoError(t, err)
	assert.Equal(t, 1, counts.Total)
	assert.Equal(t, 1, counts.ByProject["alpha"])

	usage, err := d.GetSessionUsage(ctx, "alpha-main")
	require.NoError(t, err)
	require.NotNil(t, usage)
	assert.InDelta(t, 0.02, usage.CostUSD, 1e-9)
	assert.Equal(t, []string{"gpt-5.4"}, usage.Models)

	events, err := d.GetUsageEvents(ctx, "alpha-main")
	require.NoError(t, err)
	require.Len(t, events, 1)
	assert.Equal(t, 100, events[0].InputTokens)
	assert.Equal(t, 50, events[0].OutputTokens)

	beta := alpha
	beta.GitBranch = EncodeBranchFilterToken("beta", "main")
	daily, err = d.GetDailyUsage(ctx, beta)
	require.NoError(t, err)
	assert.Equal(t, 200, daily.Totals.InputTokens)
	assert.Equal(t, 75, daily.Totals.OutputTokens)
	assert.InDelta(t, 0.04, daily.Totals.TotalCost, 1e-9)
	assert.Equal(t, 1, daily.SessionCounts.Total)
	assert.Equal(t, 1, daily.SessionCounts.ByProject["beta"])

	unfiltered := alpha
	unfiltered.GitBranch = ""
	daily, err = d.GetDailyUsage(ctx, unfiltered)
	require.NoError(t, err)
	assert.Equal(t, 300, daily.Totals.InputTokens)
	assert.Equal(t, 125, daily.Totals.OutputTokens)
	assert.InDelta(t, 0.06, daily.Totals.TotalCost, 1e-9)
	assert.Equal(t, 2, daily.SessionCounts.Total)
	assert.Equal(t, 1, daily.SessionCounts.ByProject["alpha"])
	assert.Equal(t, 1, daily.SessionCounts.ByProject["beta"])

	counts, err = d.GetUsageSessionCounts(ctx, unfiltered)
	require.NoError(t, err)
	assert.Equal(t, 2, counts.Total)
	assert.Equal(t, 1, counts.ByProject["alpha"])
	assert.Equal(t, 1, counts.ByProject["beta"])
}

func TestPhase24AnalyticsBranchFilterSQLite(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	seedPhase24BranchSession(t, d, "alpha-main", "alpha", "main")
	seedPhase24BranchSession(t, d, "beta-main", "beta", "main")
	seedPhase24AnalyticsRows(t, d, "alpha-main", "assist-learn", "Read", 80)
	seedPhase24AnalyticsRows(t, d, "beta-main", "write-outline", "Bash", 70)

	base := AnalyticsFilter{From: "2026-08-17", To: "2026-08-17", Timezone: "UTC"}
	alpha := base
	alpha.GitBranch = EncodeBranchFilterToken("alpha", "main")
	beta := base
	beta.GitBranch = EncodeBranchFilterToken("beta", "main")

	cases := []struct {
		name                         string
		wantAlpha, wantBeta, wantAll int
		call                         func(context.Context, *DB, AnalyticsFilter) (int, error)
	}{
		{"activity", 1, 1, 2, func(ctx context.Context, d *DB, f AnalyticsFilter) (int, error) {
			r, err := d.GetAnalyticsActivity(ctx, f, "day")
			if err != nil {
				return 0, err
			}
			count := 0
			for _, entry := range r.Series {
				count += entry.Sessions
			}
			return count, nil
		}},
		{"heatmap", 1, 1, 2, func(ctx context.Context, d *DB, f AnalyticsFilter) (int, error) {
			r, err := d.GetAnalyticsHeatmap(ctx, f, "sessions")
			if err != nil {
				return 0, err
			}
			count := 0
			for _, entry := range r.Entries {
				count += entry.Value
			}
			return count, nil
		}},
		{"hour-of-week", 2, 2, 4, func(ctx context.Context, d *DB, f AnalyticsFilter) (int, error) {
			r, err := d.GetAnalyticsHourOfWeek(ctx, f)
			if err != nil {
				return 0, err
			}
			count := 0
			for _, cell := range r.Cells {
				count += cell.Messages
			}
			return count, nil
		}},
		{"session-shape", 1, 1, 2, func(ctx context.Context, d *DB, f AnalyticsFilter) (int, error) {
			r, err := d.GetAnalyticsSessionShape(ctx, f)
			if err != nil {
				return 0, err
			}
			return r.Count, nil
		}},
		{"signals", 1, 1, 2, func(ctx context.Context, d *DB, f AnalyticsFilter) (int, error) {
			r, err := d.GetAnalyticsSignals(ctx, f)
			if err != nil {
				return 0, err
			}
			return r.ScoredSessions + r.UnscoredSessions, nil
		}},
		{"skills", 1, 1, 2, func(ctx context.Context, d *DB, f AnalyticsFilter) (int, error) {
			r, err := d.GetAnalyticsSkills(ctx, f, "day")
			if err != nil {
				return 0, err
			}
			return r.TotalSkillCalls, nil
		}},
		{"tools", 1, 1, 2, func(ctx context.Context, d *DB, f AnalyticsFilter) (int, error) {
			r, err := d.GetAnalyticsTools(ctx, f)
			if err != nil {
				return 0, err
			}
			return r.TotalCalls, nil
		}},
		{"top-sessions", 1, 1, 2, func(ctx context.Context, d *DB, f AnalyticsFilter) (int, error) {
			r, err := d.GetAnalyticsTopSessions(ctx, f, "messages")
			if err != nil {
				return 0, err
			}
			return len(r.Sessions), nil
		}},
		{"velocity", 1, 1, 2, func(ctx context.Context, d *DB, f AnalyticsFilter) (int, error) {
			r, err := d.GetAnalyticsVelocity(ctx, f)
			if err != nil {
				return 0, err
			}
			count := 0
			for _, row := range r.ByAgent {
				count += row.Sessions
			}
			return count, nil
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			alphaCount, err := tc.call(ctx, d, alpha)
			require.NoError(t, err)
			assert.Equal(t, tc.wantAlpha, alphaCount, "alpha branch result")

			betaCount, err := tc.call(ctx, d, beta)
			require.NoError(t, err)
			assert.Equal(t, tc.wantBeta, betaCount, "beta branch result")

			unfilteredCount, err := tc.call(ctx, d, base)
			require.NoError(t, err)
			assert.Equal(t, tc.wantAll, unfilteredCount, "unfiltered result")
		})
	}
}

func seedPhase24BranchSession(t *testing.T, d *DB, id, project, branch string) {
	t.Helper()
	started := "2026-08-17T00:00:00Z"
	ended := "2026-08-17T00:01:00Z"
	require.NoError(t, d.UpsertSession(Session{
		ID:               id,
		Project:          project,
		Machine:          "local",
		Agent:            "claude",
		StartedAt:        &started,
		EndedAt:          &ended,
		MessageCount:     2,
		UserMessageCount: 2,
		RelationshipType: "root",
		GitBranch:        branch,
	}), "upsert %s", id)
}

func seedPhase24UsageEvent(t *testing.T, d *DB, sessionID string, input, output int, cost float64) {
	t.Helper()
	ordinal := 3
	err := d.ReplaceSessionUsageEvents(sessionID, []UsageEvent{{
		SessionID:      sessionID,
		MessageOrdinal: &ordinal,
		Source:         "session",
		Model:          "gpt-5.4",
		InputTokens:    input,
		OutputTokens:   output,
		CostUSD:        &cost,
		CostStatus:     "estimated",
		CostSource:     "hermes",
		OccurredAt:     "2026-08-17T00:00:30Z",
		DedupKey:       "session:" + sessionID,
	}})
	require.NoError(t, err, "ReplaceSessionUsageEvents %s", sessionID)
}

func seedPhase24AnalyticsRows(t *testing.T, d *DB, sessionID, skillName, category string, health int) {
	t.Helper()
	require.NoError(t, d.InsertMessages([]Message{
		{
			SessionID:     sessionID,
			Ordinal:       0,
			Role:          "user",
			Content:       "phase24 analytics user",
			ContentLength: 22,
			Timestamp:     "2026-08-17T00:00:00Z",
		},
		{
			SessionID:     sessionID,
			Ordinal:       1,
			Role:          "assistant",
			Content:       "phase24 analytics assistant",
			ContentLength: 27,
			Timestamp:     "2026-08-17T00:00:10Z",
			HasToolUse:    true,
			ToolCalls: []ToolCall{{
				SessionID: sessionID,
				ToolName:  category,
				Category:  category,
				ToolUseID: "tool-" + sessionID,
				SkillName: skillName,
				InputJSON: "{}",
			}},
		},
	}))

	grade := "B"
	_, err := d.getWriter().ExecContext(context.Background(), `
		UPDATE sessions SET
			health_score = ?,
			health_grade = ?,
			outcome = 'completed',
			outcome_confidence = 'high',
			tool_failure_signal_count = 1,
			tool_retry_count = 1,
			edit_churn_count = 1,
			compaction_count = 1,
			mid_task_compaction_count = 1,
			context_pressure_max = 0.5
		WHERE id = ?`, health, grade, sessionID)
	require.NoError(t, err, "update analytics signals %s", sessionID)
}

func phase24SessionIDs(sessions []Session) []string {
	out := make([]string, len(sessions))
	for i, s := range sessions {
		out[i] = s.ID
	}
	return out
}

func phase24SidebarSessionIDs(sessions []SidebarSessionIndexRow) []string {
	out := make([]string, len(sessions))
	for i, s := range sessions {
		out[i] = s.ID
	}
	return out
}

func phase24ContentMatchSessionIDs(matches []ContentMatch) []string {
	out := make([]string, len(matches))
	for i, match := range matches {
		out[i] = match.SessionID
	}
	return out
}
