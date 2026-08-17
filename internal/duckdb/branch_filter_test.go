package duckdb

import (
	"context"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.kenn.io/agentsview/internal/db"
)

func TestPhase24DuckDBSessionBranchFilterCoversTokenContract(t *testing.T) {
	ctx := context.Background()
	store := newPhase24DuckDBBranchStore(t)

	page, err := store.ListSessions(ctx, db.SessionFilter{
		GitBranch: db.EncodeBranchFilterToken("alpha", "main"),
		Limit:     20,
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"duck-p24-alpha-main"}, duckSessionIDs(page.Sessions))

	page, err = store.ListSessions(ctx, db.SessionFilter{
		GitBranch: db.JoinBranchFilterTokens(
			db.EncodeBranchFilterToken("alpha", ""),
			db.EncodeBranchFilterToken("beta", "main"),
		),
		Limit: 20,
	})
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{
		"duck-p24-alpha-empty",
		"duck-p24-beta-main",
	}, duckSessionIDs(page.Sessions))

	page, err = store.ListSessions(ctx, db.SessionFilter{
		GitBranch: db.EncodeBranchFilterToken("alpha", "unknown"),
		Limit:     20,
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"duck-p24-alpha-unknown"}, duckSessionIDs(page.Sessions))

	page, err = store.ListSessions(ctx, db.SessionFilter{
		GitBranch: db.EncodeBranchFilterToken("alpha", "feat,x"),
		Limit:     20,
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"duck-p24-alpha-comma"}, duckSessionIDs(page.Sessions))

	page, err = store.ListSessions(ctx, db.SessionFilter{GitBranch: "invalid", Limit: 20})
	require.NoError(t, err)
	assert.Empty(t, page.Sessions)
}

func TestPhase24DuckDBGetBranchesIncludesBranchTokens(t *testing.T) {
	ctx := context.Background()
	store := newPhase24DuckDBBranchStore(t)

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

func TestPhase24DuckDBContentSearchBranchFilter(t *testing.T) {
	ctx := context.Background()
	store := newPhase24DuckDBBranchStore(t)

	page, err := store.SearchContent(ctx, db.ContentSearchFilter{
		Pattern:   "phase24 shared needle",
		Mode:      "substring",
		Sources:   []string{"messages"},
		Limit:     20,
		GitBranch: db.EncodeBranchFilterToken("alpha", "main"),
	})
	require.NoError(t, err)
	require.Len(t, page.Matches, 1)
	assert.Equal(t, "duck-p24-alpha-main", page.Matches[0].SessionID)

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
		"duck-p24-alpha-main",
		"duck-p24-beta-main",
	}, duckContentMatchSessionIDs(page.Matches))
}

func TestPhase24DuckDBSidebarBranchFilter(t *testing.T) {
	ctx := context.Background()
	store := newPhase24DuckDBBranchStore(t)

	index, err := store.GetSidebarSessionIndex(ctx, db.SessionFilter{
		GitBranch: db.EncodeBranchFilterToken("alpha", "main"),
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"duck-p24-alpha-main"}, duckSidebarSessionIDs(index.Sessions))

	index, err = store.GetSidebarSessionIndex(ctx, db.SessionFilter{})
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{
		"duck-p24-alpha-main",
		"duck-p24-alpha-empty",
		"duck-p24-alpha-unknown",
		"duck-p24-alpha-comma",
		"duck-p24-beta-main",
	}, duckSidebarSessionIDs(index.Sessions))
}

func TestPhase24DuckDBAnalyticsBranchFilter(t *testing.T) {
	ctx := context.Background()
	store := newPhase24DuckDBBranchStore(t)
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

	terms, err := db.ParseTrendTerms([]string{"duck-alpha-main-only"})
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

func TestPhase24DuckDBUsageBranchFilter(t *testing.T) {
	ctx := context.Background()
	store := newPhase24DuckDBBranchStore(t)
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
	assert.Equal(t, "duck-p24-alpha-main", top[0].SessionID)

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

func newPhase24DuckDBBranchStore(t *testing.T) *Store {
	t.Helper()
	ctx := context.Background()
	local := newLocalDB(t)
	require.NoError(t, local.UpsertModelPricing([]db.ModelPricing{{
		ModelPattern:  "phase24-test-model",
		InputPerMTok:  1,
		OutputPerMTok: 1,
	}}))

	for _, row := range []struct {
		id      string
		project string
		branch  string
		content string
		input   int
		output  int
	}{
		{"duck-p24-alpha-main", "alpha", "main", "phase24 shared needle duck-alpha-main-only", 11, 7},
		{"duck-p24-beta-main", "beta", "main", "phase24 shared needle duck-beta-main-only", 13, 5},
		{"duck-p24-alpha-empty", "alpha", "", "phase24 empty branch", 17, 3},
		{"duck-p24-alpha-unknown", "alpha", "unknown", "phase24 literal unknown branch", 19, 2},
		{"duck-p24-alpha-comma", "alpha", "feat,x", "phase24 comma branch", 23, 1},
	} {
		phase24WriteBranchSession(t, local, row.id, row.project, row.branch, row.content, row.input, row.output)
	}

	syncer := newTestSync(t, filepath.Join(t.TempDir(), "phase24.duckdb"), local, SyncOptions{})
	_, err := syncer.Push(ctx, true, nil)
	require.NoError(t, err)
	return NewStoreFromDB(syncer.DB())
}

func phase24WriteBranchSession(
	t *testing.T,
	local *db.DB,
	id, project, branch, content string,
	input, output int,
) {
	t.Helper()
	started := "2026-08-17T00:00:00.000Z"
	ended := "2026-08-17T00:01:00.000Z"
	usage := []byte(`{"input_tokens":` + strconv.Itoa(input) + `,"output_tokens":` + strconv.Itoa(output) + `}`)
	_, err := local.WriteSessionBatchAtomic([]db.SessionBatchWrite{{
		Session: db.Session{
			ID:               id,
			Project:          project,
			Machine:          "local",
			Agent:            "claude",
			StartedAt:        &started,
			EndedAt:          &ended,
			CreatedAt:        started,
			MessageCount:     2,
			UserMessageCount: 2,
			RelationshipType: "root",
			GitBranch:        branch,
			DataVersion:      db.CurrentDataVersion(),
		},
		Messages: []db.Message{
			{SessionID: id, Ordinal: 0, Role: "user", Content: "prompt", Timestamp: started},
			{
				SessionID: id, Ordinal: 1, Role: "assistant", Content: content,
				Timestamp: ended, Model: "phase24-test-model", TokenUsage: usage,
			},
		},
		ReplaceMessages: true,
		DataVersion:     db.CurrentDataVersion(),
	}})
	require.NoError(t, err)
}

func duckContentMatchSessionIDs(matches []db.ContentMatch) []string {
	ids := make([]string, len(matches))
	for i, match := range matches {
		ids[i] = match.SessionID
	}
	return ids
}
