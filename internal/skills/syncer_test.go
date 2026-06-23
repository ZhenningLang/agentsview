package skills

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.kenn.io/agentsview/internal/db"
)

type fakeWriter struct {
	skills   []db.Skill
	findings []db.SkillHealth
}

func (w *fakeWriter) ReplaceSkillCatalog(
	_ context.Context, s []db.Skill, f []db.SkillHealth,
) error {
	w.skills = s
	w.findings = f
	return nil
}

// writeSkill creates <catalogDir>/<dir>/SKILL.md with the given
// frontmatter name and description.
func writeSkill(t *testing.T, catalogDir, dir, fmName, desc string) {
	t.Helper()
	skillDir := filepath.Join(catalogDir, dir)
	require.NoError(t, os.MkdirAll(skillDir, 0o755))
	body := "---\nname: " + fmName + "\ndescription: " + desc + "\n---\n\n# Body\n"
	require.NoError(t, os.WriteFile(
		filepath.Join(skillDir, "SKILL.md"), []byte(body), 0o644))
}

func writeCatalog(t *testing.T, catalogDir, json string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(catalogDir, 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(catalogDir, "catalog.json"), []byte(json), 0o644))
}

func findingTypes(findings []db.SkillHealth) map[string]int {
	m := map[string]int{}
	for _, f := range findings {
		m[f.CheckType]++
	}
	return m
}

func TestSyncHealthyCatalog(t *testing.T) {
	catalogDir := filepath.Join(t.TempDir(), "coding-skills")
	writeSkill(t, catalogDir, "guard-check", "guard-check", "当交付前总检查时使用")
	writeSkill(t, catalogDir, "think-plan", "think-plan", "当需要写 spec 时使用")
	writeCatalog(t, catalogDir, `{"skills":[
		{"name":"guard-check","path":"coding-skills/guard-check","domain":"guard","role":"canonical"},
		{"name":"think-plan","path":"coding-skills/think-plan","domain":"think","role":"canonical"}
	]}`)

	w := &fakeWriter{}
	s := NewSyncer(catalogDir, w, nil)
	require.NoError(t, s.Sync(context.Background()))

	require.Len(t, w.skills, 2)
	assert.Empty(t, w.findings, "healthy catalog should have no findings")
	for _, sk := range w.skills {
		assert.True(t, sk.CatalogPresent)
		assert.True(t, sk.FilePresent)
		assert.NotEmpty(t, sk.Description)
		assert.Greater(t, sk.DescriptionTokens, 0,
			"description should tokenize to >0 tokens")
		assert.Equal(t, "heuristic-v1", sk.Tokenizer)
		assert.Equal(t, 0, sk.HealthErrorCount)
	}
}

func TestSyncDetectsHealthIssues(t *testing.T) {
	catalogDir := filepath.Join(t.TempDir(), "coding-skills")
	// Healthy.
	writeSkill(t, catalogDir, "guard-check", "guard-check", "总检查")
	// Frontmatter name disagrees with catalog/dir.
	writeSkill(t, catalogDir, "think-plan", "think-planning", "写 spec")
	// On-disk skill not referenced by catalog.
	writeSkill(t, catalogDir, "orphan-skill", "orphan-skill", "孤儿")
	// Legacy skill present on disk but pointing at a missing canonical.
	writeSkill(t, catalogDir, "think-refine", "think-refine", "旧版")
	// Duplicate name + a missing dir + legacy dangling canonical
	// are declared only in the catalog.
	writeCatalog(t, catalogDir, `{"skills":[
		{"name":"guard-check","path":"coding-skills/guard-check","domain":"guard","role":"canonical"},
		{"name":"think-plan","path":"coding-skills/think-plan","domain":"think","role":"canonical"},
		{"name":"guard-check","path":"coding-skills/guard-check","domain":"guard","role":"canonical"},
		{"name":"ghost","path":"coding-skills/ghost","domain":"x","role":"canonical"},
		{"name":"think-refine","path":"coding-skills/think-refine","domain":"think","role":"legacy","migration":{"state":"planned","canonical":"does-not-exist"}}
	]}`)

	w := &fakeWriter{}
	s := NewSyncer(catalogDir, w, nil)
	require.NoError(t, s.Sync(context.Background()))

	types := findingTypes(w.findings)
	assert.Equal(t, 1, types["name_mismatch"], "think-plan name mismatch")
	assert.Equal(t, 1, types["duplicate_name"], "guard-check duplicated")
	assert.Equal(t, 1, types["symlink_broken"], "ghost dir missing")
	assert.Equal(t, 1, types["orphan_file"], "orphan-skill on disk only")
	assert.Equal(t, 1, types["legacy_dangling_canonical"],
		"think-refine canonical missing")

	// Error-severity findings roll up into HealthErrorCount; a
	// warn-only finding (legacy dangling canonical) does not.
	byName := map[string]db.Skill{}
	for _, sk := range w.skills {
		byName[sk.Name] = sk
	}
	assert.Positive(t, byName["think-plan"].HealthErrorCount,
		"name_mismatch is an error")
	assert.Positive(t, byName["guard-check"].HealthErrorCount,
		"duplicate_name is an error")
	assert.Equal(t, 0, byName["think-refine"].HealthErrorCount,
		"legacy dangling canonical is only a warning")

	// Every finding must carry a detected_at timestamp so the health
	// panel can show when an issue was found (regression: it was
	// previously left empty and silently overrode the schema default).
	for _, f := range w.findings {
		assert.NotEmpty(t, f.DetectedAt,
			"finding %s/%s missing detected_at", f.CheckType, f.SkillName)
	}
}

func TestSyncMissingSkillFile(t *testing.T) {
	catalogDir := filepath.Join(t.TempDir(), "coding-skills")
	// Directory exists but has no SKILL.md.
	require.NoError(t, os.MkdirAll(filepath.Join(catalogDir, "bare"), 0o755))
	writeCatalog(t, catalogDir, `{"skills":[
		{"name":"bare","path":"coding-skills/bare","domain":"x","role":"canonical"}
	]}`)

	w := &fakeWriter{}
	s := NewSyncer(catalogDir, w, nil)
	require.NoError(t, s.Sync(context.Background()))

	require.Len(t, w.skills, 1)
	assert.False(t, w.skills[0].FilePresent)
	assert.Equal(t, 1, findingTypes(w.findings)["orphan_catalog"])
}

func TestTokenizerApproximatesChineseCost(t *testing.T) {
	tk := NewHeuristicTokenizer()
	assert.Equal(t, 0, tk.Count(""))
	assert.Positive(t, tk.Count("当交付前总检查时使用"))
	// Longer text costs more than shorter text.
	assert.Greater(t,
		tk.Count("当任务完成、准备合并或需要交付前总检查时使用"),
		tk.Count("总检查"))
}
