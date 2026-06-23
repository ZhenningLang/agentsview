//go:build pgtest

package postgres

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.kenn.io/agentsview/internal/db"
)

// TestSyncSkillsRoundTrip verifies that the local skills + skill_health
// dimension tables full-replace into PG via syncSkills and read back
// identically through the PG store, including the full-replace semantics
// (a shrinking local set drops removed PG rows).
func TestSyncSkillsRoundTrip(t *testing.T) {
	pgURL := testPGURL(t)

	const schema = "agentsview_push_skills_test"
	pg, err := Open(pgURL, schema, true)
	require.NoError(t, err, "Open")
	defer pg.Close()

	ctx := context.Background()
	_, err = pg.Exec(`DROP SCHEMA IF EXISTS ` + schema + ` CASCADE`)
	require.NoError(t, err, "drop schema")
	require.NoError(t, EnsureSchema(ctx, pg, schema), "EnsureSchema")

	localDB, err := db.Open(filepath.Join(t.TempDir(), "local.db"))
	require.NoError(t, err, "db.Open")
	defer localDB.Close()

	sync := &Sync{
		pg:         pg,
		local:      localDB,
		machine:    "test-machine",
		schema:     schema,
		schemaDone: true,
	}
	store := &Store{pg: pg}

	skills := []db.Skill{
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
	}
	require.NoError(t, localDB.ReplaceSkills(ctx, skills), "ReplaceSkills")
	require.NoError(t, localDB.ReplaceSkillHealth(ctx, []db.SkillHealth{
		{SkillName: "think-plan", CheckType: "name_mismatch", Severity: "error",
			Message: "x", DetectedAt: "2026-06-23T00:00:00.000Z"},
		{CheckType: "orphan_file", Severity: "warn", Message: "y",
			DetectedAt: "2026-06-23T00:00:00.000Z"},
	}), "ReplaceSkillHealth")

	// First push.
	require.NoError(t, sync.syncSkills(ctx), "syncSkills")

	got, err := store.ListSkills(ctx, db.SkillFilter{})
	require.NoError(t, err, "PG ListSkills")
	require.Len(t, got, 2)
	assert.Equal(t, "guard-check", got[0].Name)
	assert.Equal(t, 20, got[1].DescriptionTokens)
	assert.True(t, got[0].CatalogPresent)

	cost, err := store.GetSkillTokenCost(ctx)
	require.NoError(t, err, "PG GetSkillTokenCost")
	assert.Equal(t, 32, cost.TotalTokens)
	assert.Equal(t, "heuristic-v1", cost.Tokenizer)

	health, err := store.GetSkillHealth(ctx, db.SkillHealthFilter{})
	require.NoError(t, err, "PG GetSkillHealth")
	require.Len(t, health.Findings, 2)
	assert.Equal(t, 1, health.BySeverity["error"])
	assert.Equal(t, 1, health.BySeverity["warn"])
	assert.Equal(t, 2, health.TotalSkills)

	// Catalog-level finding keeps a NULL/empty skill name through PG.
	orphan, err := store.GetSkillHealth(ctx, db.SkillHealthFilter{CheckType: "orphan_file"})
	require.NoError(t, err)
	require.Len(t, orphan.Findings, 1)
	assert.Empty(t, orphan.Findings[0].SkillName)

	// Second push with a shrunk set: full-replace must drop removed rows.
	require.NoError(t, localDB.ReplaceSkills(ctx, skills[:1]), "ReplaceSkills shrink")
	require.NoError(t, localDB.ReplaceSkillHealth(ctx, nil), "clear health")
	require.NoError(t, sync.syncSkills(ctx), "syncSkills second")

	got, err = store.ListSkills(ctx, db.SkillFilter{})
	require.NoError(t, err)
	assert.Len(t, got, 1, "shrunk local set must drop PG rows")
	health, err = store.GetSkillHealth(ctx, db.SkillHealthFilter{})
	require.NoError(t, err)
	assert.Empty(t, health.Findings, "cleared local health must clear PG")
}
