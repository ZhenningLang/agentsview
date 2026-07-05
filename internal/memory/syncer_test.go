package memory

import (
	"bytes"
	"context"
	"errors"
	"log"
	"os"
	"path/filepath"
	"strings"
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
			"type: semantic\nstatus: active\norigin_session: sess-a\n"+
			"origin_project: oss-atlas\nfeedback_vote: down\n"+
			"feedback_comment: \"原因: 过度合并\"\nfeedback_status: pending",
		"This is the alpha body.")
	writeNote(t, dir, "beta.md",
		"title: Beta\ndate: 2026-06-24\nproblem_type: incident\n"+
			"type: episodic\nstatus: archived\norigin_session: sess-b",
		"This is the beta body with more words to tokenize.")

	w := &fakeWriter{}
	s := NewSyncer(dir, w, nil)
	require.NoError(t, s.Sync(context.Background()))

	require.Len(t, w.memories, 1)
	by := byRelPath(w.memories)

	alpha := by["alpha.md"]
	assert.Equal(t, "Alpha", alpha.Title)
	assert.Equal(t, "2026-06-20", alpha.Date)
	assert.Equal(t, "knowledge", alpha.ProblemType)
	assert.Equal(t, "semantic", alpha.Type)
	assert.Equal(t, "active", alpha.Status)
	assert.Equal(t, "sess-a", alpha.OriginSession)
	assert.Equal(t, "oss-atlas", alpha.OriginProject)
	assert.Equal(t, "down", alpha.FeedbackVote)
	assert.Equal(t, "原因: 过度合并", alpha.FeedbackComment)
	assert.Equal(t, "pending", alpha.FeedbackStatus)
	assert.Equal(t, "", by["beta.md"].OriginProject)
	assert.Contains(t, alpha.Body, "This is the alpha body.")
	assert.Positive(t, alpha.BodyTokens)
	assert.NotEmpty(t, alpha.SyncedAt)

	_, betaOK := by["beta.md"]
	assert.False(t, betaOK, "archived cross-agent notes should not enter the UI cache")
}

func TestSyncOnlyMirrorsActiveCrossAgentNotes(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "memory")
	writeNote(t, dir, "active.md",
		"title: Active\ndate: 2026-06-20\nproblem_type: knowledge\n"+
			"type: semantic\nstatus: active\norigin_session: s-active",
		"active body")
	writeNote(t, dir, "archived.md",
		"title: Archived\ndate: 2026-06-20\nproblem_type: knowledge\n"+
			"type: semantic\nstatus: archived\norigin_session: s-archived",
		"archived body")
	writeNote(t, dir, "stale.md",
		"title: Stale\ndate: 2026-06-20\nproblem_type: knowledge\n"+
			"type: semantic\nstatus: stale\norigin_session: s-stale",
		"stale body")

	w := &fakeWriter{}
	s := NewSyncer(dir, w, nil)
	require.NoError(t, s.Sync(context.Background()))

	require.Len(t, w.memories, 1)
	assert.Equal(t, db.SourceCrossAgent, w.source)
	assert.Equal(t, "active.md", w.memories[0].RelPath)
	assert.Equal(t, "active", w.memories[0].Status)
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

// A skipped malformed note must be logged, never silently dropped — a silently
// vanished memory is the failure mode this guards against.
func TestSyncLogsSkippedMalformedNote(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "memory")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	bad := "---\ntitle: Bad\n  : : broken yaml [\n---\n\nbad body\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "bad.md"), []byte(bad), 0o644))

	var buf bytes.Buffer
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })

	w := &fakeWriter{}
	s := NewSyncer(dir, w, nil)
	require.NoError(t, s.Sync(context.Background()))

	assert.Contains(t, buf.String(), "bad.md")
	assert.Contains(t, buf.String(), "malformed frontmatter")
}

// A title containing ':' '#' and quotes, when emitted as a YAML double-quoted
// scalar (the form assist_consolidate.py now renders), must parse and round-trip
// — previously such notes broke strict YAML and vanished from the store.
func TestSyncParsesQuotedSpecialCharTitle(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "memory")
	writeNote(t, dir, "tricky.md",
		"title: \"decision: # Scope Alignment: add or \\\"refactor\\\"\"\n"+
			"date: 2026-06-28\nproblem_type: decision\n"+
			"type: semantic\nstatus: active\norigin_session: ses_1",
		"body")

	w := &fakeWriter{}
	s := NewSyncer(dir, w, nil)
	require.NoError(t, s.Sync(context.Background()))

	require.Len(t, w.memories, 1)
	assert.Equal(t, "tricky.md", w.memories[0].RelPath)
	assert.Equal(t, `decision: # Scope Alignment: add or "refactor"`, w.memories[0].Title)
}

func TestSyncNoFrontmatterDoesNotEnterCrossAgentCache(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "memory")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "plain.md"),
		[]byte("just a plain note, no frontmatter"), 0o644))

	w := &fakeWriter{}
	s := NewSyncer(dir, w, nil)
	require.NoError(t, s.Sync(context.Background()))

	assert.Empty(t, w.memories)
}

func TestSyncWithEmbedderPopulatesMemoryEmbedding(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "memory")
	writeNote(t, dir, "embedded.md",
		"title: Embedded\ndate: 2026-06-20\nproblem_type: knowledge\nstatus: active",
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
		"title: Broken\ndate: 2026-06-20\nproblem_type: knowledge\nstatus: active",
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
		"title: Stable\ndate: 2026-06-20\nproblem_type: knowledge\nstatus: active",
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

func TestLedgerSyncerMirrorsActiveAssistMemEntries(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "entries.jsonl")
	content := strings.Join([]string{
		`{"created_at":"2026-07-01T13:36:35Z","evidence":"user explicit remember","id":"abd80440ea5d8479","project":"ordo_ai","scope":"project","source":"explicit","status":"Active","text":"lzn-preview and lzn-test deploy scripts live in ~/Projects/ordo_ai.","triggers":["lzn-preview","lzn-test",".env.lzn"],"type":"entrypoint"}`,
		`{"created_at":"2026-07-01T13:40:00Z","id":"inactive","project":"ordo_ai","scope":"project","status":"archived","text":"old note","type":"entrypoint"}`,
		``,
	}, "\n")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))

	w := &fakeWriter{}
	s := NewLedgerSyncer(path, w, nil)
	require.NoError(t, s.Sync(context.Background()))

	require.Len(t, w.memories, 1)
	got := w.memories[0]
	assert.Equal(t, db.SourceAssistMem, w.source)
	assert.Equal(t, "assist-mem/abd80440ea5d8479.jsonl", got.RelPath)
	assert.Equal(t, db.SourceAssistMem, got.Source)
	assert.Equal(t, "active", got.Status)
	assert.Equal(t, "explicit", got.ProblemType)
	assert.Equal(t, "2026-07-01 21:36:35", got.Date)
	assert.Equal(t, "entrypoint", got.Type)
	assert.Equal(t, "ordo_ai", got.OriginProject)
	assert.Equal(t, "assist-mem:abd80440ea5d8479", got.OriginSession)
	assert.Contains(t, got.Title, "lzn-preview and lzn-test")
	assert.Contains(t, got.Body, "~/Projects/ordo_ai")
	assert.Contains(t, got.Body, "user explicit remember")
	assert.Contains(t, got.Body, "lzn-test")
	assert.NotEmpty(t, got.SyncedAt)
}

func TestLedgerSyncerKeepsLatestActiveAssistMemEntryPerTopic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "entries.jsonl")
	content := strings.Join([]string{
		`{"created_at":"2026-07-01T13:36:35Z","id":"older-topic","project":"ordo_ai","scope":"project","status":"active","text":"old lzn-preview location","topic":"lzn-preview-entrypoint","type":"entrypoint"}`,
		`{"created_at":"2026-07-01T13:40:00Z","id":"newer-topic","project":"ordo_ai","scope":"project","status":"active","text":"current lzn-preview location","topic":"lzn-preview-entrypoint","type":"entrypoint"}`,
		`{"created_at":"2026-07-01T13:45:00Z","id":"archived-topic","project":"ordo_ai","scope":"project","status":"archived","text":"archived lzn-preview location","topic":"lzn-preview-entrypoint","type":"entrypoint"}`,
		`{"created_at":"2026-07-01T13:50:00Z","id":"untopic-a","project":"ordo_ai","scope":"project","status":"active","text":"untopiced first","type":"entrypoint"}`,
		`{"created_at":"2026-07-01T13:55:00Z","id":"untopic-b","project":"ordo_ai","scope":"project","status":"active","text":"untopiced second","type":"entrypoint"}`,
	}, "\n")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))

	w := &fakeWriter{}
	s := NewLedgerSyncer(path, w, nil)
	require.NoError(t, s.Sync(context.Background()))

	require.Len(t, w.memories, 3)
	by := byRelPath(w.memories)
	_, hasOlder := by["assist-mem/older-topic.jsonl"]
	_, hasArchived := by["assist-mem/archived-topic.jsonl"]
	assert.False(t, hasOlder, "older active entries with the same topic should not be mirrored")
	assert.False(t, hasArchived, "archived entries should remain excluded")
	assert.Contains(t, by, "assist-mem/newer-topic.jsonl")
	assert.Contains(t, by, "assist-mem/untopic-a.jsonl")
	assert.Contains(t, by, "assist-mem/untopic-b.jsonl")
	assert.Contains(t, by["assist-mem/newer-topic.jsonl"].Body, "current lzn-preview location")
}
