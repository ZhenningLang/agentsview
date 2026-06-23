package db

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func sampleSkills() []Skill {
	return []Skill{
		{
			Name: "guard-check", CatalogPath: "coding-skills/guard-check",
			Domain: "guard", Role: "canonical", Description: "总检查",
			FrontmatterName: "guard-check", DescriptionTokens: 12,
			Tokenizer: "heuristic-v1", CatalogPresent: true,
			FilePresent: true, SyncedAt: "2026-06-23T00:00:00.000Z",
		},
		{
			Name: "think-plan", CatalogPath: "coding-skills/think-plan",
			Domain: "think", Role: "canonical", Description: "写 spec",
			FrontmatterName: "think-plan", DescriptionTokens: 20,
			Tokenizer: "heuristic-v1", CatalogPresent: true,
			FilePresent: true, HealthErrorCount: 1,
			SyncedAt: "2026-06-23T00:00:00.000Z",
		},
	}
}

func TestReplaceAndListSkills(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	require.NoError(t, d.ReplaceSkills(ctx, sampleSkills()))

	all, err := d.ListSkills(ctx, SkillFilter{})
	require.NoError(t, err)
	require.Len(t, all, 2)
	// Ordered by domain then name: guard < think.
	assert.Equal(t, "guard-check", all[0].Name)
	assert.True(t, all[0].CatalogPresent)
	assert.Equal(t, 20, all[1].DescriptionTokens)

	// Domain filter.
	guard, err := d.ListSkills(ctx, SkillFilter{Domain: "guard"})
	require.NoError(t, err)
	require.Len(t, guard, 1)
	assert.Equal(t, "guard-check", guard[0].Name)

	// Full-replace semantics: a smaller set drops removed rows.
	require.NoError(t, d.ReplaceSkills(ctx, sampleSkills()[:1]))
	all, err = d.ListSkills(ctx, SkillFilter{})
	require.NoError(t, err)
	assert.Len(t, all, 1)
}

func TestGetSkillAndTokenCost(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	require.NoError(t, d.ReplaceSkills(ctx, sampleSkills()))

	sk, err := d.GetSkill(ctx, "think-plan")
	require.NoError(t, err)
	require.NotNil(t, sk)
	assert.Equal(t, "think", sk.Domain)

	missing, err := d.GetSkill(ctx, "nope")
	require.NoError(t, err)
	assert.Nil(t, missing)

	cost, err := d.GetSkillTokenCost(ctx)
	require.NoError(t, err)
	assert.Equal(t, 2, cost.TotalSkills)
	assert.Equal(t, 32, cost.TotalTokens)
	assert.True(t, cost.Approximate)
	assert.Equal(t, "heuristic-v1", cost.Tokenizer)
	assert.Len(t, cost.ByDomain, 2)
}

func TestReplaceAndGetSkillHealth(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()
	require.NoError(t, d.ReplaceSkills(ctx, sampleSkills()))
	require.NoError(t, d.ReplaceSkillHealth(ctx, []SkillHealth{
		{SkillName: "think-plan", CheckType: "name_mismatch",
			Severity: "error", Message: "x", DetectedAt: "2026-06-23T00:00:00.000Z"},
		{CheckType: "orphan_file", Severity: "warn", Message: "y",
			DetectedAt: "2026-06-23T00:00:00.000Z"},
	}))

	rep, err := d.GetSkillHealth(ctx, SkillHealthFilter{})
	require.NoError(t, err)
	require.Len(t, rep.Findings, 2)
	assert.Equal(t, 1, rep.BySeverity["error"])
	assert.Equal(t, 1, rep.BySeverity["warn"])
	assert.Equal(t, 2, rep.TotalSkills)
	assert.Equal(t, 1, rep.HealthySkills, "one skill has zero error count")

	// Catalog-level finding has empty skill name preserved.
	byType, err := d.GetSkillHealth(ctx, SkillHealthFilter{CheckType: "orphan_file"})
	require.NoError(t, err)
	require.Len(t, byType.Findings, 1)
	assert.Empty(t, byType.Findings[0].SkillName)
}

// TestSkillSyncDoesNotPolluteCoreStats proves the skills dimension is
// fully isolated from the session/message fact domain: writing skills
// and health findings must not change session/message counts or insert
// any sessions/messages/tool_calls rows.
func TestSkillSyncDoesNotPolluteCoreStats(t *testing.T) {
	d := testDB(t)
	ctx := context.Background()

	before, err := d.GetStats(ctx, false, false)
	require.NoError(t, err)

	require.NoError(t, d.ReplaceSkills(ctx, sampleSkills()))
	require.NoError(t, d.ReplaceSkillHealth(ctx, []SkillHealth{
		{SkillName: "think-plan", CheckType: "name_mismatch",
			Severity: "error", DetectedAt: "2026-06-23T00:00:00.000Z"},
	}))

	after, err := d.GetStats(ctx, false, false)
	require.NoError(t, err)
	assert.Equal(t, before.SessionCount, after.SessionCount,
		"skill sync must not change session count")
	assert.Equal(t, before.MessageCount, after.MessageCount,
		"skill sync must not change message count")

	for _, table := range []string{"sessions", "messages", "tool_calls"} {
		var n int
		require.NoError(t, d.getReader().QueryRowContext(ctx,
			"SELECT COUNT(*) FROM "+table).Scan(&n))
		assert.Equal(t, 0, n, "skill sync must not insert into "+table)
	}
}
