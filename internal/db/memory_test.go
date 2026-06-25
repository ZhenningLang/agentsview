package db

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func sampleMemories() []Memory {
	return []Memory{
		{
			RelPath: "alpha.md", Title: "Alpha note", Date: "2026-06-20",
			ProblemType: "knowledge", Type: "semantic", Status: "active",
			OriginSession: "sess-a", Body: "the quick brown fox",
			BodyTokens: 5, SourceMtime: 100,
			SyncedAt: "2026-06-23T00:00:00.000Z",
		},
		{
			RelPath: "beta.md", Title: "Beta note", Date: "2026-06-24",
			ProblemType: "incident", Type: "episodic", Status: "archived",
			OriginSession: "sess-b", Body: "lazy dog jumps over",
			BodyTokens: 4, SourceMtime: 200,
			SyncedAt: "2026-06-23T00:00:00.000Z",
		},
	}
}

func TestReplaceAndListMemories(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	require.NoError(t, d.ReplaceMemories(ctx, sampleMemories()))

	all, err := d.ListMemories(ctx, MemoryFilter{})
	require.NoError(t, err)
	require.Len(t, all, 2)
	// Ordered by date DESC: beta (2026-06-24) before alpha (2026-06-20).
	assert.Equal(t, "beta.md", all[0].RelPath)
	assert.Equal(t, "alpha.md", all[1].RelPath)
	assert.Equal(t, "the quick brown fox", all[1].Body)

	// Full-replace semantics: a smaller set drops removed rows.
	require.NoError(t, d.ReplaceMemories(ctx, sampleMemories()[:1]))
	all, err = d.ListMemories(ctx, MemoryFilter{})
	require.NoError(t, err)
	assert.Len(t, all, 1)
	assert.Equal(t, "alpha.md", all[0].RelPath)
}

func TestListMemoriesFilters(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	require.NoError(t, d.ReplaceMemories(ctx, sampleMemories()))

	cases := []struct {
		name   string
		filter MemoryFilter
		want   string
	}{
		{"problem_type", MemoryFilter{ProblemType: "knowledge"}, "alpha.md"},
		{"type", MemoryFilter{Type: "episodic"}, "beta.md"},
		{"status", MemoryFilter{Status: "active"}, "alpha.md"},
		{"origin_session", MemoryFilter{OriginSession: "sess-b"}, "beta.md"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := d.ListMemories(ctx, tc.filter)
			require.NoError(t, err)
			require.Len(t, got, 1)
			assert.Equal(t, tc.want, got[0].RelPath)
		})
	}
}

func TestListMemoriesFullTextSearch(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	require.NoError(t, d.ReplaceMemories(ctx, sampleMemories()))

	// FTS5 MATCH on the body via the memory_fts mirror.
	got, err := d.ListMemories(ctx, MemoryFilter{Q: "fox"})
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "alpha.md", got[0].RelPath)

	got, err = d.ListMemories(ctx, MemoryFilter{Q: "dog"})
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "beta.md", got[0].RelPath)

	// Combined with a frontmatter filter.
	got, err = d.ListMemories(ctx,
		MemoryFilter{Q: "jumps", Status: "archived"})
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "beta.md", got[0].RelPath)

	// FTS index is kept in sync on full-replace: re-replacing with a set
	// that no longer contains "fox" returns nothing for that term.
	require.NoError(t, d.ReplaceMemories(ctx, sampleMemories()[1:]))
	got, err = d.ListMemories(ctx, MemoryFilter{Q: "fox"})
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestGetMemory(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	require.NoError(t, d.ReplaceMemories(ctx, sampleMemories()))

	m, err := d.GetMemory(ctx, "beta.md")
	require.NoError(t, err)
	require.NotNil(t, m)
	assert.Equal(t, "Beta note", m.Title)
	assert.Equal(t, "episodic", m.Type)
	assert.Equal(t, int64(200), m.SourceMtime)

	// Missing rel_path returns nil, no error.
	missing, err := d.GetMemory(ctx, "does-not-exist.md")
	require.NoError(t, err)
	assert.Nil(t, missing)
}
