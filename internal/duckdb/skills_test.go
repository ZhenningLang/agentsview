package duckdb

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.kenn.io/agentsview/internal/db"
)

// TestSyncSkillsMirrorRoundTrip verifies the DuckDB skills mirror keeps
// parity with SQLite/PG: syncSkills full-replaces both tables and the
// DuckDB store reads them back with the same shape, including the
// full-replace shrink semantics.
func TestSyncSkillsMirrorRoundTrip(t *testing.T) {
	local := newLocalDB(t)
	syncer := newTestSync(
		t, filepath.Join(t.TempDir(), "mirror.duckdb"), local, SyncOptions{},
	)
	require.NoError(t, syncer.EnsureSchema(context.Background()))
	ctx := context.Background()

	require.NoError(t, local.ReplaceSkillCatalog(ctx, []db.Skill{
		{
			Name: "guard-check", CatalogPath: "coding-skills/guard-check",
			Domain: "guard", Role: "canonical", Description: "总检查",
			FrontmatterName: "guard-check", DescriptionTokens: 12,
			Tokenizer: "heuristic-v1", CatalogPresent: true, FilePresent: true,
			SyncedAt: "2026-06-23T00:00:00.000Z",
		},
		{
			Name: "think-plan", CatalogPath: "coding-skills/think-plan",
			Domain: "think", Role: "canonical", Description: "写 spec",
			FrontmatterName: "think-plan", DescriptionTokens: 20,
			Tokenizer: "heuristic-v1", CatalogPresent: true, FilePresent: true,
			HealthErrorCount: 1, SyncedAt: "2026-06-23T00:00:00.000Z",
		},
	}, []db.SkillHealth{
		{SkillName: "think-plan", CheckType: "name_mismatch", Severity: "error",
			Message: "x", DetectedAt: "2026-06-23T00:00:00.000Z"},
		{CheckType: "orphan_file", Severity: "warn", Message: "y",
			DetectedAt: "2026-06-23T00:00:00.000Z"},
	}))

	require.NoError(t, syncer.syncSkills(ctx))

	store := NewStoreFromDB(syncer.DB())
	got, err := store.ListSkills(ctx, db.SkillFilter{})
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, "guard-check", got[0].Name)
	assert.Equal(t, 20, got[1].DescriptionTokens)
	assert.True(t, got[0].CatalogPresent)

	cost, err := store.GetSkillTokenCost(ctx)
	require.NoError(t, err)
	assert.Equal(t, 32, cost.TotalTokens)
	assert.NotNil(t, cost.ByDomain)

	health, err := store.GetSkillHealth(ctx, db.SkillHealthFilter{})
	require.NoError(t, err)
	require.Len(t, health.Findings, 2)
	assert.Equal(t, 1, health.BySeverity["error"])
	assert.Equal(t, 2, health.TotalSkills)

	// Full-replace shrink: a smaller local set drops removed mirror rows.
	require.NoError(t, local.ReplaceSkillCatalog(ctx,
		[]db.Skill{{Name: "guard-check", Domain: "guard", Role: "canonical",
			Tokenizer: "heuristic-v1", CatalogPresent: true, FilePresent: true,
			SyncedAt: "2026-06-23T00:00:00.000Z"}}, nil))
	require.NoError(t, syncer.syncSkills(ctx))
	got, err = store.ListSkills(ctx, db.SkillFilter{})
	require.NoError(t, err)
	assert.Len(t, got, 1)
	health, err = store.GetSkillHealth(ctx, db.SkillHealthFilter{})
	require.NoError(t, err)
	assert.Empty(t, health.Findings)
}

// TestSyncSkillsMirrorSkipsEmptyLocal verifies the mirror is not wiped
// when the local catalog has not been synced (empty skills), matching
// the PG push guard.
func TestSyncSkillsMirrorSkipsEmptyLocal(t *testing.T) {
	local := newLocalDB(t)
	syncer := newTestSync(
		t, filepath.Join(t.TempDir(), "mirror.duckdb"), local, SyncOptions{},
	)
	require.NoError(t, syncer.EnsureSchema(context.Background()))
	ctx := context.Background()

	// Seed the mirror, then run syncSkills with an empty local table.
	require.NoError(t, local.ReplaceSkillCatalog(ctx, []db.Skill{
		{Name: "guard-check", Domain: "guard", Role: "canonical",
			Tokenizer: "heuristic-v1", CatalogPresent: true, FilePresent: true,
			SyncedAt: "2026-06-23T00:00:00.000Z"},
	}, nil))
	require.NoError(t, syncer.syncSkills(ctx))

	require.NoError(t, local.ReplaceSkillCatalog(ctx, nil, nil))
	require.NoError(t, syncer.syncSkills(ctx))

	store := NewStoreFromDB(syncer.DB())
	got, err := store.ListSkills(ctx, db.SkillFilter{})
	require.NoError(t, err)
	assert.Len(t, got, 1, "empty local must not wipe the mirror")
}
