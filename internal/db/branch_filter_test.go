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
