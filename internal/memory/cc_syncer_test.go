package memory

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.kenn.io/agentsview/internal/db"
)

// writeCCNote creates <root>/<project>/memory/<name> with raw content.
func writeCCNote(t *testing.T, root, project, name, content string) {
	t.Helper()
	dir := filepath.Join(root, project, "memory")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, name), []byte(content), 0o644))
}

// TestCCSyncScansProjectsExcludesIndex is the canonical Phase 01 acceptance
// from verify.sh: a root with projA/memory/{a.md,MEMORY.md} and
// projB/memory/b.md must yield exactly 2 entries (MEMORY.md excluded), each
// tagged source=cc-native with rel_path relative to the CC root.
func TestCCSyncScansProjectsExcludesIndex(t *testing.T) {
	root := filepath.Join(t.TempDir(), "projects")
	writeCCNote(t, root, "projA", "a.md",
		"# fact a\n\nthis is cc-native memory fact a")
	// MEMORY.md is an index/directory file, not a memory fact: excluded.
	writeCCNote(t, root, "projA", "MEMORY.md",
		"# Memory Index\n\n- [a](a.md)")
	writeCCNote(t, root, "projB", "b.md",
		"# fact b\n\nthis is cc-native memory fact b")

	w := &fakeWriter{}
	s := NewCCSyncer(root, w, nil)
	require.NoError(t, s.Sync(context.Background()))

	require.Len(t, w.memories, 2)
	by := byRelPath(w.memories)

	a, okA := by[filepath.Join("projA", "memory", "a.md")]
	require.True(t, okA, "projA/memory/a.md should be synced")
	assert.Equal(t, "cc-native", a.Source)
	assert.Contains(t, a.Body, "this is cc-native memory fact a")
	assert.Positive(t, a.BodyTokens)
	assert.NotEmpty(t, a.SyncedAt)

	b, okB := by[filepath.Join("projB", "memory", "b.md")]
	require.True(t, okB, "projB/memory/b.md should be synced")
	assert.Equal(t, "cc-native", b.Source)

	// MEMORY.md must never appear as a memory entry.
	for _, m := range w.memories {
		assert.NotContains(t, m.RelPath, "MEMORY.md")
	}
}

// TestCCSyncTolerantParse: CC-native auto-memory files often have no YAML
// frontmatter (plain markdown). The syncer must tolerate that without
// failing — the whole file becomes the body and frontmatter fields are empty.
func TestCCSyncTolerantParse(t *testing.T) {
	root := filepath.Join(t.TempDir(), "projects")
	writeCCNote(t, root, "p", "plain.md",
		"just a plain cc-native note, no frontmatter at all")
	// A note WITH frontmatter is still parsed (tolerant, not rejected).
	writeCCNote(t, root, "p", "fm.md",
		"---\ntitle: Has FM\ndate: 2026-06-20\nfeedback_vote: up\n"+
			"feedback_comment: \"useful: yes\"\nfeedback_status: handled\n---\n\nbody text here")

	w := &fakeWriter{}
	s := NewCCSyncer(root, w, nil)
	require.NoError(t, s.Sync(context.Background()))

	require.Len(t, w.memories, 2)
	by := byRelPath(w.memories)

	plain := by[filepath.Join("p", "memory", "plain.md")]
	assert.Equal(t, "just a plain cc-native note, no frontmatter at all",
		plain.Body)
	assert.Empty(t, plain.Title)
	assert.Equal(t, "cc-native", plain.Source)

	fm := by[filepath.Join("p", "memory", "fm.md")]
	assert.Equal(t, "Has FM", fm.Title)
	assert.Equal(t, "2026-06-20", fm.Date)
	assert.Equal(t, "up", fm.FeedbackVote)
	assert.Equal(t, "useful: yes", fm.FeedbackComment)
	assert.Equal(t, "handled", fm.FeedbackStatus)
	assert.NotContains(t, fm.Body, "title:")
}

// TestCCSyncMissingRootIsFailSoft: a non-existent CC root must not error;
// it simply produces an empty set (fail-open, matching skills/vault).
func TestCCSyncMissingRootFailSoft(t *testing.T) {
	w := &fakeWriter{}
	s := NewCCSyncer(filepath.Join(t.TempDir(), "nope"), w, nil)
	require.NoError(t, s.Sync(context.Background()))
	assert.Empty(t, w.memories)
}

// TestCCSyncFailSoftBadFrontmatter: one note with broken YAML must not abort
// the whole run; sibling notes still sync.
func TestCCSyncFailSoftBadFrontmatter(t *testing.T) {
	root := filepath.Join(t.TempDir(), "projects")
	writeCCNote(t, root, "p", "good.md", "# good\n\ngood body")
	writeCCNote(t, root, "p", "bad.md",
		"---\ntitle: Bad\n  : : broken yaml [\n---\n\nbad body")

	w := &fakeWriter{}
	s := NewCCSyncer(root, w, nil)
	require.NoError(t, s.Sync(context.Background()))

	by := byRelPath(w.memories)
	_, goodOK := by[filepath.Join("p", "memory", "good.md")]
	assert.True(t, goodOK, "good note should sync despite a bad sibling")
	_, badOK := by[filepath.Join("p", "memory", "bad.md")]
	assert.False(t, badOK, "malformed note should be skipped fail-soft")
}

// TestCrossAgentSyncerStampsSource: the existing cross-agent syncer must now
// tag its rows source=cross-agent (regression: source column backfill).
func TestCrossAgentSyncerStampsSource(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "memory")
	writeNote(t, dir, "x.md",
		"title: X\ndate: 2026-06-20\nproblem_type: knowledge\n"+
			"type: semantic\nstatus: active\norigin_session: s",
		"cross agent body")

	w := &fakeWriter{}
	s := NewSyncer(dir, w, nil)
	require.NoError(t, s.Sync(context.Background()))

	require.Len(t, w.memories, 1)
	assert.Equal(t, db.SourceCrossAgent, w.memories[0].Source)
}

func TestCCSyncWithEmbedderPopulatesMemoryEmbedding(t *testing.T) {
	root := filepath.Join(t.TempDir(), "projects")
	writeCCNote(t, root, "projA", "a.md", "---\ntitle: A\n---\n\ncc body")
	w := &fakeWriter{}
	embedder := &fakeEmbedder{vector: []float32{0.5, 0.5}}
	s := NewCCSyncerWithEmbedder(root, w, nil, embedder)

	require.NoError(t, s.Sync(context.Background()))
	require.Len(t, w.memories, 1)
	assert.Equal(t, []float32{0.5, 0.5}, w.memories[0].LLMEmbedding)
	assert.Equal(t, 2, w.memories[0].LLMEmbeddingDim)
	assert.Equal(t, 1, embedder.calls)
}

func TestCCSyncWithEmbedderReturnsErrorOnEmbeddingFailure(t *testing.T) {
	root := filepath.Join(t.TempDir(), "projects")
	writeCCNote(t, root, "projA", "a.md", "cc body")
	w := &fakeWriter{}
	s := NewCCSyncerWithEmbedder(root, w, nil, &fakeEmbedder{err: errors.New("embed failed")})

	err := s.Sync(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "embedding memory")
	assert.Empty(t, w.memories)
}
