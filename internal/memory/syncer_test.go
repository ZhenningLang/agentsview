package memory

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
	memories []db.Memory
}

func (w *fakeWriter) ReplaceMemories(
	_ context.Context, m []db.Memory,
) error {
	w.memories = m
	return nil
}

// writeNote creates <dir>/<name> with the given frontmatter and body.
func writeNote(t *testing.T, dir, name, frontmatter, body string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(dir, 0o755))
	content := "---\n" + frontmatter + "\n---\n\n" + body + "\n"
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, name), []byte(content), 0o644))
}

func byRelPath(memories []db.Memory) map[string]db.Memory {
	m := map[string]db.Memory{}
	for _, mem := range memories {
		m[mem.RelPath] = mem
	}
	return m
}

func TestSyncParsesFrontmatterAndBody(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "memory")
	writeNote(t, dir, "alpha.md",
		"title: Alpha\ndate: 2026-06-20\nproblem_type: knowledge\n"+
			"type: semantic\nstatus: active\norigin_session: sess-a",
		"This is the alpha body.")
	writeNote(t, dir, "beta.md",
		"title: Beta\ndate: 2026-06-24\nproblem_type: incident\n"+
			"type: episodic\nstatus: archived\norigin_session: sess-b",
		"This is the beta body with more words to tokenize.")

	w := &fakeWriter{}
	s := NewSyncer(dir, w, nil)
	require.NoError(t, s.Sync(context.Background()))

	require.Len(t, w.memories, 2)
	by := byRelPath(w.memories)

	alpha := by["alpha.md"]
	assert.Equal(t, "Alpha", alpha.Title)
	assert.Equal(t, "2026-06-20", alpha.Date)
	assert.Equal(t, "knowledge", alpha.ProblemType)
	assert.Equal(t, "semantic", alpha.Type)
	assert.Equal(t, "active", alpha.Status)
	assert.Equal(t, "sess-a", alpha.OriginSession)
	assert.Contains(t, alpha.Body, "This is the alpha body.")
	assert.Positive(t, alpha.BodyTokens)
	assert.NotEmpty(t, alpha.SyncedAt)

	beta := by["beta.md"]
	assert.Equal(t, "episodic", beta.Type)
	// Frontmatter must not leak into the body.
	assert.NotContains(t, beta.Body, "title:")
	assert.NotContains(t, beta.Body, "---")
}

func TestSyncIgnoresIndexAndNonMarkdown(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "memory")
	writeNote(t, dir, "note.md",
		"title: Note\ndate: 2026-06-20\nproblem_type: knowledge\n"+
			"type: semantic\nstatus: active\norigin_session: s",
		"body")
	// INDEX.md is a generated hint and must be excluded.
	writeNote(t, dir, "INDEX.md", "title: Index", "| a | b |")
	// Non-markdown files are skipped.
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, ".gitignore"), []byte("*.tmp\n"), 0o644))
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "schema.json"), []byte("{}"), 0o644))

	w := &fakeWriter{}
	s := NewSyncer(dir, w, nil)
	require.NoError(t, s.Sync(context.Background()))

	require.Len(t, w.memories, 1)
	assert.Equal(t, "note.md", w.memories[0].RelPath)
}

func TestSyncFailSoftOnBadFrontmatter(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "memory")
	// Valid note.
	writeNote(t, dir, "good.md",
		"title: Good\ndate: 2026-06-20\nproblem_type: knowledge\n"+
			"type: semantic\nstatus: active\norigin_session: s",
		"good body")
	// Malformed YAML frontmatter: a single bad file must not abort the
	// whole run, the good note still syncs.
	bad := "---\ntitle: Bad\n  : : broken yaml [\n---\n\nbad body\n"
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "bad.md"), []byte(bad), 0o644))

	w := &fakeWriter{}
	s := NewSyncer(dir, w, nil)
	require.NoError(t, s.Sync(context.Background()))

	by := byRelPath(w.memories)
	_, goodOK := by["good.md"]
	assert.True(t, goodOK, "good note should sync despite a bad sibling")
	_, badOK := by["bad.md"]
	assert.False(t, badOK, "malformed note should be skipped fail-soft")
}

func TestSyncNoFrontmatterTreatsAllAsBody(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "memory")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "plain.md"),
		[]byte("just a plain note, no frontmatter"), 0o644))

	w := &fakeWriter{}
	s := NewSyncer(dir, w, nil)
	require.NoError(t, s.Sync(context.Background()))

	require.Len(t, w.memories, 1)
	assert.Equal(t, "just a plain note, no frontmatter",
		w.memories[0].Body)
	assert.Empty(t, w.memories[0].Title)
}

func TestSyncMissingDirReturnsError(t *testing.T) {
	w := &fakeWriter{}
	s := NewSyncer(filepath.Join(t.TempDir(), "does-not-exist"), w, nil)
	require.Error(t, s.Sync(context.Background()))
}
