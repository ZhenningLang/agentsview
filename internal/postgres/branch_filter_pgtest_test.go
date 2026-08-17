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
	assert.Equal(t, []string{"pg-p24-alpha-main"}, pgSessionIDs(index.Sessions))

	index, err = store.GetSidebarSessionIndex(ctx, db.SessionFilter{})
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{
		"pg-p24-alpha-main",
		"pg-p24-alpha-empty",
		"pg-p24-alpha-unknown",
		"pg-p24-alpha-comma",
		"pg-p24-beta-main",
	}, pgSessionIDs(index.Sessions))
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
			git_branch
		) VALUES (
			$1, 'local', $2, 'claude', 'phase24 seed',
			'2026-08-17T00:00:00Z'::timestamptz,
			'2026-08-17T00:01:00Z'::timestamptz,
			'2026-08-17T00:00:00Z'::timestamptz,
			2, 2, 'root', $3
		)`, id, project, branch)
	require.NoError(t, err, "insert session %s", id)
	_, err = store.DB().ExecContext(ctx, `
		INSERT INTO messages (
			session_id, ordinal, role, content, timestamp,
			content_length, model, token_usage
		) VALUES
			($1, 0, 'user', 'prompt',
			 '2026-08-17T00:00:00Z'::timestamptz, 6,
			 'phase24-test-model', '{}'),
			($1, 1, 'assistant', $2,
			 '2026-08-17T00:01:00Z'::timestamptz, length($2),
			 'phase24-test-model',
			 jsonb_build_object('input_tokens', $3::int, 'output_tokens', $4::int)::text)
		`, id, content, input, output)
	require.NoError(t, err, "insert messages %s", id)
}

func pgContentMatchSessionIDs(matches []db.ContentMatch) []string {
	ids := make([]string, len(matches))
	for i, match := range matches {
		ids[i] = match.SessionID
	}
	return ids
}
