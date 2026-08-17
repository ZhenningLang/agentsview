//go:build pgtest

package postgres

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.kenn.io/agentsview/internal/db"
)

func TestPhase24PostgresSessionBranchFilterCoversTokenContract(t *testing.T) {
	ctx := context.Background()
	store := newPhase24PostgresBranchStore(t)

	page, err := store.ListSessions(ctx, db.SessionFilter{
		GitBranch: db.EncodeBranchFilterToken("alpha", "main"),
		Limit:     20,
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"pg-p24-alpha-main"}, pgSessionIDs(page.Sessions))

	page, err = store.ListSessions(ctx, db.SessionFilter{
		GitBranch: db.JoinBranchFilterTokens(
			db.EncodeBranchFilterToken("alpha", ""),
			db.EncodeBranchFilterToken("beta", "main"),
		),
		Limit: 20,
	})
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{
		"pg-p24-alpha-empty",
		"pg-p24-beta-main",
	}, pgSessionIDs(page.Sessions))

	page, err = store.ListSessions(ctx, db.SessionFilter{
		GitBranch: db.EncodeBranchFilterToken("alpha", "unknown"),
		Limit:     20,
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"pg-p24-alpha-unknown"}, pgSessionIDs(page.Sessions))

	page, err = store.ListSessions(ctx, db.SessionFilter{
		GitBranch: db.EncodeBranchFilterToken("alpha", "feat,x"),
		Limit:     20,
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"pg-p24-alpha-comma"}, pgSessionIDs(page.Sessions))

	page, err = store.ListSessions(ctx, db.SessionFilter{GitBranch: "invalid", Limit: 20})
	require.NoError(t, err)
	assert.Empty(t, page.Sessions)
}

func TestPhase24PostgresGetBranchesIncludesBranchTokens(t *testing.T) {
	ctx := context.Background()
	store := newPhase24PostgresBranchStore(t)

	branches, err := store.GetBranches(ctx, false, false)
	require.NoError(t, err)
	assert.Equal(t, []db.BranchInfo{
		{Project: "alpha", Branch: "", Token: db.EncodeBranchFilterToken("alpha", "")},
		{Project: "alpha", Branch: "feat,x", Token: db.EncodeBranchFilterToken("alpha", "feat,x")},
		{Project: "alpha", Branch: "main", Token: db.EncodeBranchFilterToken("alpha", "main")},
		{Project: "alpha", Branch: "unknown", Token: db.EncodeBranchFilterToken("alpha", "unknown")},
		{Project: "beta", Branch: "main", Token: db.EncodeBranchFilterToken("beta", "main")},
	}, branches)
}

func TestPhase24PostgresContentSearchBranchFilter(t *testing.T) {
	ctx := context.Background()
	store := newPhase24PostgresBranchStore(t)

	page, err := store.SearchContent(ctx, db.ContentSearchFilter{
		Pattern:   "phase24 shared needle",
		Mode:      "substring",
		Sources:   []string{"messages"},
		Limit:     20,
		GitBranch: db.EncodeBranchFilterToken("alpha", "main"),
	})
	require.NoError(t, err)
	require.Len(t, page.Matches, 1)
	assert.Equal(t, "pg-p24-alpha-main", page.Matches[0].SessionID)

	page, err = store.SearchContent(ctx, db.ContentSearchFilter{
		Pattern:   "phase24 shared needle",
		Mode:      "substring",
		Sources:   []string{"messages"},
		Limit:     20,
		GitBranch: "invalid",
	})
	require.NoError(t, err)
	assert.Empty(t, page.Matches)

	page, err = store.SearchContent(ctx, db.ContentSearchFilter{
		Pattern: "phase24 shared needle",
		Mode:    "substring",
		Sources: []string{"messages"},
		Limit:   20,
	})
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{
		"pg-p24-alpha-main",
		"pg-p24-beta-main",
	}, pgContentMatchSessionIDs(page.Matches))
}

func TestPhase24PostgresSidebarBranchFilter(t *testing.T) {
	ctx := context.Background()
	store := newPhase24PostgresBranchStore(t)

	index, err := store.GetSidebarSessionIndex(ctx, db.SessionFilter{
		GitBranch: db.EncodeBranchFilterToken("alpha", "main"),
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"pg-p24-alpha-main"}, pgSidebarSessionIDs(index.Sessions))

	index, err = store.GetSidebarSessionIndex(ctx, db.SessionFilter{})
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{
		"pg-p24-alpha-main",
		"pg-p24-alpha-empty",
		"pg-p24-alpha-unknown",
		"pg-p24-alpha-comma",
		"pg-p24-beta-main",
	}, pgSidebarSessionIDs(index.Sessions))
}

func TestPhase24PostgresAnalyticsBranchFilter(t *testing.T) {
	ctx := context.Background()
	store := newPhase24PostgresBranchStore(t)
	filter := db.AnalyticsFilter{
		From:      "2026-08-17",
		To:        "2026-08-17",
		Timezone:  "UTC",
		GitBranch: db.EncodeBranchFilterToken("alpha", "main"),
	}

	summary, err := store.GetAnalyticsSummary(ctx, filter)
	require.NoError(t, err)
	assert.Equal(t, 1, summary.TotalSessions)
	assert.Equal(t, 2, summary.TotalMessages)

	projects, err := store.GetAnalyticsProjects(ctx, filter)
	require.NoError(t, err)
	require.Len(t, projects.Projects, 1)
	assert.Equal(t, "alpha", projects.Projects[0].Name)

	terms, err := db.ParseTrendTerms([]string{"pg-alpha-main-only"})
	require.NoError(t, err)
	trends, err := store.GetTrendsTerms(ctx, filter, terms, "day")
	require.NoError(t, err)
	require.Len(t, trends.Series, 1)
	assert.Equal(t, 1, trends.Series[0].Total)

	invalid := filter
	invalid.GitBranch = "invalid"
	summary, err = store.GetAnalyticsSummary(ctx, invalid)
	require.NoError(t, err)
	assert.Equal(t, 0, summary.TotalSessions)
}

func TestPhase24PostgresAnalyticsPerConsumerBranchFilter(t *testing.T) {
	ctx := context.Background()
	store := newPhase24PostgresBranchStore(t)
	base := db.AnalyticsFilter{From: "2026-08-17", To: "2026-08-17", Timezone: "UTC"}
	alpha := base
	alpha.GitBranch = db.EncodeBranchFilterToken("alpha", "main")
	beta := base
	beta.GitBranch = db.EncodeBranchFilterToken("beta", "main")

	cases := []struct {
		name                         string
		wantAlpha, wantBeta, wantAll int
		call                         func(context.Context, *Store, db.AnalyticsFilter) (int, error)
	}{
		{"activity", 1, 1, 5, func(ctx context.Context, store *Store, f db.AnalyticsFilter) (int, error) {
			r, err := store.GetAnalyticsActivity(ctx, f, "day")
			if err != nil {
				return 0, err
			}
			count := 0
			for _, entry := range r.Series {
				count += entry.Sessions
			}
			return count, nil
		}},
		{"heatmap", 1, 1, 5, func(ctx context.Context, store *Store, f db.AnalyticsFilter) (int, error) {
			r, err := store.GetAnalyticsHeatmap(ctx, f, "sessions")
			if err != nil {
				return 0, err
			}
			count := 0
			for _, entry := range r.Entries {
				count += entry.Value
			}
			return count, nil
		}},
		{"hour-of-week", 2, 2, 10, func(ctx context.Context, store *Store, f db.AnalyticsFilter) (int, error) {
			r, err := store.GetAnalyticsHourOfWeek(ctx, f)
			if err != nil {
				return 0, err
			}
			count := 0
			for _, cell := range r.Cells {
				count += cell.Messages
			}
			return count, nil
		}},
		{"session-shape", 1, 1, 5, func(ctx context.Context, store *Store, f db.AnalyticsFilter) (int, error) {
			r, err := store.GetAnalyticsSessionShape(ctx, f)
			if err != nil {
				return 0, err
			}
			return r.Count, nil
		}},
		{"signals", 1, 1, 5, func(ctx context.Context, store *Store, f db.AnalyticsFilter) (int, error) {
			r, err := store.GetAnalyticsSignals(ctx, f)
			if err != nil {
				return 0, err
			}
			return r.ScoredSessions + r.UnscoredSessions, nil
		}},
		{"skills", 1, 1, 5, func(ctx context.Context, store *Store, f db.AnalyticsFilter) (int, error) {
			r, err := store.GetAnalyticsSkills(ctx, f, "day")
			if err != nil {
				return 0, err
			}
			return r.TotalSkillCalls, nil
		}},
		{"tools", 1, 1, 5, func(ctx context.Context, store *Store, f db.AnalyticsFilter) (int, error) {
			r, err := store.GetAnalyticsTools(ctx, f)
			if err != nil {
				return 0, err
			}
			return r.TotalCalls, nil
		}},
		{"top-sessions", 1, 1, 5, func(ctx context.Context, store *Store, f db.AnalyticsFilter) (int, error) {
			r, err := store.GetAnalyticsTopSessions(ctx, f, "messages")
			if err != nil {
				return 0, err
			}
			return len(r.Sessions), nil
		}},
		{"velocity", 1, 1, 5, func(ctx context.Context, store *Store, f db.AnalyticsFilter) (int, error) {
			r, err := store.GetAnalyticsVelocity(ctx, f)
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
			alphaCount, err := tc.call(ctx, store, alpha)
			require.NoError(t, err)
			assert.Equal(t, tc.wantAlpha, alphaCount, "alpha branch result")

			betaCount, err := tc.call(ctx, store, beta)
			require.NoError(t, err)
			assert.Equal(t, tc.wantBeta, betaCount, "beta branch result")

			unfilteredCount, err := tc.call(ctx, store, base)
			require.NoError(t, err)
			assert.Equal(t, tc.wantAll, unfilteredCount, "unfiltered result")
		})
	}
}

func TestPhase24PostgresUsageBranchFilter(t *testing.T) {
	ctx := context.Background()
	store := newPhase24PostgresBranchStore(t)
	filter := db.UsageFilter{
		From:      "2026-08-17",
		To:        "2026-08-17",
		Timezone:  "UTC",
		GitBranch: db.EncodeBranchFilterToken("alpha", "main"),
	}

	daily, err := store.GetDailyUsage(ctx, filter)
	require.NoError(t, err)
	assert.Equal(t, 11, daily.Totals.InputTokens)
	assert.Equal(t, 7, daily.Totals.OutputTokens)
	assert.Equal(t, 1, daily.SessionCounts.Total)
	assert.Equal(t, 1, daily.SessionCounts.ByProject["alpha"])

	top, err := store.GetTopSessionsByCost(ctx, filter, 10)
	require.NoError(t, err)
	require.Len(t, top, 1)
	assert.Equal(t, "pg-p24-alpha-main", top[0].SessionID)

	counts, err := store.GetUsageSessionCounts(ctx, filter)
	require.NoError(t, err)
	assert.Equal(t, 1, counts.Total)
	assert.Equal(t, 1, counts.ByProject["alpha"])

	invalid := filter
	invalid.GitBranch = "invalid"
	counts, err = store.GetUsageSessionCounts(ctx, invalid)
	require.NoError(t, err)
	assert.Equal(t, 0, counts.Total)
}

func TestPhase24PostgresBranchIndexDefinitionAndIdempotency(t *testing.T) {
	ctx := context.Background()
	_, store := prepareUsageSchema(t, "agentsview_phase24_branch_index_test")

	first := phase24PostgresBranchIndexDefs(t, store)
	require.Len(t, first, 1)
	assert.Contains(t, first[0], "idx_sessions_project_git_branch")
	assert.Contains(t, first[0], "USING btree (project, git_branch)")
	assert.Contains(t, first[0], "WHERE (git_branch <> ''::text)")

	require.NoError(t, EnsureSchema(ctx, store.DB(), "agentsview_phase24_branch_index_test"), "repeat EnsureSchema")
	second := phase24PostgresBranchIndexDefs(t, store)
	assert.Equal(t, first, second)
}

func newPhase24PostgresBranchStore(t *testing.T) *Store {
	t.Helper()
	_, store := prepareUsageSchema(t, "agentsview_phase24_branch_filter_test")
	ctx := context.Background()
	_, err := store.DB().ExecContext(ctx, `
		INSERT INTO model_pricing (
			model_pattern, input_per_mtok, output_per_mtok,
			cache_creation_per_mtok, cache_read_per_mtok, updated_at
		) VALUES ('phase24-test-model', 1, 1, 0, 0, 'seed')`)
	require.NoError(t, err, "insert pricing")

	for _, row := range []struct {
		id      string
		project string
		branch  string
		content string
		input   int
		output  int
	}{
		{"pg-p24-alpha-main", "alpha", "main", "phase24 shared needle pg-alpha-main-only", 11, 7},
		{"pg-p24-beta-main", "beta", "main", "phase24 shared needle pg-beta-main-only", 13, 5},
		{"pg-p24-alpha-empty", "alpha", "", "phase24 empty branch", 17, 3},
		{"pg-p24-alpha-unknown", "alpha", "unknown", "phase24 literal unknown branch", 19, 2},
		{"pg-p24-alpha-comma", "alpha", "feat,x", "phase24 comma branch", 23, 1},
	} {
		phase24InsertPostgresBranchSession(t, store, row.id, row.project, row.branch, row.content, row.input, row.output)
	}
	return store
}

func phase24InsertPostgresBranchSession(
	t *testing.T,
	store *Store,
	id, project, branch, content string,
	input, output int,
) {
	t.Helper()
	ctx := context.Background()
	_, err := store.DB().ExecContext(ctx, `
		INSERT INTO sessions (
			id, machine, project, agent, first_message, started_at, ended_at,
			created_at, message_count, user_message_count, relationship_type,
			git_branch, health_score, health_grade, outcome, outcome_confidence,
			tool_failure_signal_count, tool_retry_count, edit_churn_count,
			compaction_count, mid_task_compaction_count, context_pressure_max
		) VALUES (
			$1, 'local', $2, 'claude', 'phase24 seed',
			'2026-08-17T00:00:00Z'::timestamptz,
			'2026-08-17T00:01:00Z'::timestamptz,
			'2026-08-17T00:00:00Z'::timestamptz,
			2, 2, 'root', $3, 80, 'B', 'completed', 'high',
			1, 1, 1, 1, 1, 0.5
		)`, id, project, branch)
	require.NoError(t, err, "insert session %s", id)
	_, err = store.DB().ExecContext(ctx, `
		INSERT INTO messages (
			session_id, ordinal, role, content, timestamp,
			content_length, model, token_usage, has_tool_use
		) VALUES
			($1, 0, 'user', 'prompt',
			 '2026-08-17T00:00:00Z'::timestamptz, 6,
			 'phase24-test-model', '{}', false),
			($1, 1, 'assistant', $2,
			 '2026-08-17T00:01:00Z'::timestamptz, length($2),
			 'phase24-test-model',
			 jsonb_build_object('input_tokens', $3::int, 'output_tokens', $4::int)::text,
			 true)
		`, id, content, input, output)
	require.NoError(t, err, "insert messages %s", id)
	_, err = store.DB().ExecContext(ctx, `
		INSERT INTO tool_calls (
			session_id, message_ordinal, call_index, tool_name, category,
			tool_use_id, skill_name, input_json
		) VALUES (
			$1, 1, 0, 'Skill', 'Task', $2, $3, '{}'
		)`, id, "tool-"+id, "phase24-"+project)
	require.NoError(t, err, "insert tool_call %s", id)
}

func pgContentMatchSessionIDs(matches []db.ContentMatch) []string {
	ids := make([]string, len(matches))
	for i, match := range matches {
		ids[i] = match.SessionID
	}
	return ids
}

func pgSidebarSessionIDs(sessions []db.SidebarSessionIndexRow) []string {
	ids := make([]string, len(sessions))
	for i, session := range sessions {
		ids[i] = session.ID
	}
	return ids
}

func phase24PostgresBranchIndexDefs(t *testing.T, store *Store) []string {
	t.Helper()
	rows, err := store.DB().QueryContext(context.Background(), `
		SELECT indexdef FROM pg_indexes
		WHERE schemaname = current_schema()
		  AND indexname = 'idx_sessions_project_git_branch'
		ORDER BY indexdef
	`)
	require.NoError(t, err, "query branch index definitions")
	defer rows.Close()

	var defs []string
	for rows.Next() {
		var def string
		require.NoError(t, rows.Scan(&def), "scan branch index definition")
		defs = append(defs, def)
	}
	require.NoError(t, rows.Err(), "iterate branch index definitions")
	return defs
}
