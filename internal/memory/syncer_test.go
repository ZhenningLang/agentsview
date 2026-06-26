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

type fakeWriter struct {
	memories []db.Memory
	source   string
}

type fakeEmbedder struct {
	vector []float32
	err    error
	calls  int
}

func (e *fakeEmbedder) Embed(_ context.Context, _ string) ([]float32, error) {
	e.calls++
	if e.err != nil {
		return nil, e.err
	}
	return e.vector, nil
}

func (w *fakeWriter) ReplaceMemories(
	_ context.Context, m []db.Memory,
) error {
	w.memories = m
	return nil
}

func (w *fakeWriter) ReplaceMemoriesBySource(
	_ context.Context, source string, m []db.Memory,
) error {
	w.source = source
	w.memories = m
	return nil
}

func (w *fakeWriter) MemoryEmbeddings(_ context.Context, f db.MemoryFilter) ([]db.Memory, error) {
	var out []db.Memory
	for _, m := range w.memories {
		if f.Source != "" && m.Source != f.Source {
			continue
		}
		out = append(out, m)
	}
	return out, nil
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

func TestSyncWithEmbedderPopulatesMemoryEmbedding(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "memory")
	writeNote(t, dir, "embedded.md",
		"title: Embedded\ndate: 2026-06-20\nproblem_type: knowledge",
		"body to embed")
	w := &fakeWriter{}
	embedder := &fakeEmbedder{vector: []float32{1, 0}}
	s := NewSyncerWithEmbedder(dir, w, nil, embedder)

	require.NoError(t, s.Sync(context.Background()))
	require.Len(t, w.memories, 1)
	assert.Equal(t, []float32{1, 0}, w.memories[0].LLMEmbedding)
	assert.Equal(t, 2, w.memories[0].LLMEmbeddingDim)
	assert.Equal(t, 1, embedder.calls)
}

func TestSyncWithEmbedderReturnsErrorOnEmbeddingFailure(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "memory")
	writeNote(t, dir, "broken.md",
		"title: Broken\ndate: 2026-06-20\nproblem_type: knowledge",
		"body to embed")
	w := &fakeWriter{}
	s := NewSyncerWithEmbedder(dir, w, nil, &fakeEmbedder{err: errors.New("embed failed")})

	err := s.Sync(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "embedding memory")
	assert.Empty(t, w.memories, "failed embed should not write a silent lexical-only replacement")
}

func TestSyncWithEmbedderReusesUnchangedEmbedding(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "memory")
	writeNote(t, dir, "stable.md",
		"title: Stable\ndate: 2026-06-20\nproblem_type: knowledge",
		"stable body")
	info, err := os.Stat(filepath.Join(dir, "stable.md"))
	require.NoError(t, err)
	w := &fakeWriter{memories: []db.Memory{{
		RelPath: "stable.md", Source: db.SourceCrossAgent, Body: "stable body\n",
		SourceMtime: info.ModTime().Unix(), LLMEmbedding: []float32{0.25, 0.75}, LLMEmbeddingDim: 2,
	}}}
	embedder := &fakeEmbedder{vector: []float32{1, 0}}
	s := NewSyncerWithEmbedder(dir, w, nil, embedder)

	require.NoError(t, s.Sync(context.Background()))
	require.Len(t, w.memories, 1)
	assert.Equal(t, []float32{0.25, 0.75}, w.memories[0].LLMEmbedding)
	assert.Zero(t, embedder.calls)
}

func TestSyncMissingDirReturnsError(t *testing.T) {
	w := &fakeWriter{}
	s := NewSyncer(filepath.Join(t.TempDir(), "does-not-exist"), w, nil)
	require.Error(t, s.Sync(context.Background()))
}
