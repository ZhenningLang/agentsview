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
	"go.kenn.io/agentsview/internal/skills"
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

// failingEmbedder rejects exactly one body, mirroring a provider that refuses
// a single oversized note while the rest of the batch embeds fine.
type failingEmbedder struct {
	vector   []float32
	failBody string
	err      error
	calls    int
}

func (e *failingEmbedder) Embed(_ context.Context, input string) ([]float32, error) {
	e.calls++
	if input == e.failBody {
		return nil, e.err
	}
	return e.vector, nil
}

// recordingEmbedder captures the last input so tests can assert on what was
// actually sent to the provider.
type recordingEmbedder struct {
	vector    []float32
	lastInput string
	calls     int
}

func (e *recordingEmbedder) Embed(_ context.Context, input string) ([]float32, error) {
	e.calls++
	e.lastInput = input
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

func TestSyncWithEmbedderKeepsMirrorOnEmbeddingFailure(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "memory")
	writeNote(t, dir, "broken.md",
		"title: Broken\ndate: 2026-06-20\nproblem_type: knowledge\nstatus: active",
		"body to embed")
	w := &fakeWriter{}
	s := NewSyncerWithEmbedder(dir, w, nil, &fakeEmbedder{err: errors.New("embed failed")})

	require.NoError(t, s.Sync(context.Background()),
		"a provider rejection must not abort the run")
	require.Len(t, w.memories, 1,
		"the note stays mirrored lexically even without a vector")
	assert.Empty(t, w.memories[0].LLMEmbedding)
}

// A single unembeddable note used to abort the whole run before
// ReplaceMemoriesBySource, so no vector was ever persisted and every later
// sync re-embedded every note from scratch.
func TestSyncOneFailedEmbeddingDoesNotDiscardOtherVectors(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "memory")
	for _, name := range []string{"a.md", "oversized.md", "z.md"} {
		writeNote(t, dir, name,
			"title: "+name+"\ndate: 2026-06-20\nproblem_type: knowledge\nstatus: active",
			"body of "+name)
	}
	w := &fakeWriter{}
	embedder := &failingEmbedder{
		vector:   []float32{1, 0},
		failBody: "body of oversized.md\n",
		err:      errors.New("maximum context length is 8192 tokens"),
	}
	s := NewSyncerWithEmbedder(dir, w, nil, embedder)

	require.NoError(t, s.Sync(context.Background()))
	require.Len(t, w.memories, 3)
	withVector := 0
	for _, m := range w.memories {
		if len(m.LLMEmbedding) > 0 {
			withVector++
		}
	}
	assert.Equal(t, 2, withVector,
		"the two healthy notes keep their fresh vectors")
}

// The reuse path only works if the previous run actually persisted vectors,
// which is what makes the fail-soft fix stop the repeated-billing loop.
func TestSyncReusesVectorsAfterEarlierFailure(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "memory")
	writeNote(t, dir, "good.md",
		"title: Good\ndate: 2026-06-20\nproblem_type: knowledge\nstatus: active",
		"stable body")
	writeNote(t, dir, "oversized.md",
		"title: Oversized\ndate: 2026-06-20\nproblem_type: knowledge\nstatus: active",
		"too long")
	w := &fakeWriter{}
	embedder := &failingEmbedder{
		vector:   []float32{1, 0},
		failBody: "too long\n",
		err:      errors.New("maximum context length is 8192 tokens"),
	}
	s := NewSyncerWithEmbedder(dir, w, nil, embedder)

	require.NoError(t, s.Sync(context.Background()))
	firstRun := embedder.calls
	require.NoError(t, s.Sync(context.Background()))

	assert.Equal(t, firstRun+1, embedder.calls,
		"only the still-failing note is retried; healthy vectors are reused")
}

// A provider outage must not blank vectors that are already stored.
func TestSyncFailedEmbeddingCarriesPreviousVectorForward(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "memory")
	writeNote(t, dir, "stable.md",
		"title: Stable\ndate: 2026-06-20\nproblem_type: knowledge\nstatus: active",
		"changed body")
	info, err := os.Stat(filepath.Join(dir, "stable.md"))
	require.NoError(t, err)
	w := &fakeWriter{memories: []db.Memory{{
		RelPath: "stable.md", Source: db.SourceCrossAgent, Body: "old body\n",
		SourceMtime: info.ModTime().Unix(), LLMEmbedding: []float32{0.25, 0.75}, LLMEmbeddingDim: 2,
	}}}
	s := NewSyncerWithEmbedder(dir, w, nil, &fakeEmbedder{err: errors.New("provider down")})

	require.NoError(t, s.Sync(context.Background()))
	require.Len(t, w.memories, 1)
	assert.Equal(t, []float32{0.25, 0.75}, w.memories[0].LLMEmbedding,
		"an outage keeps the stored vector rather than downgrading to lexical-only")
}

func TestTruncateForEmbeddingFitsBudget(t *testing.T) {
	tk := skills.NewHeuristicTokenizer()
	long := strings.Repeat("语义记忆内容", 20000)
	require.Greater(t, tk.Count(long), maxEmbedInputTokens)

	got := truncateForEmbedding(tk, long)

	assert.LessOrEqual(t, tk.Count(got), maxEmbedInputTokens)
	assert.NotEmpty(t, got)
}

func TestTruncateForEmbeddingLeavesShortBodyIntact(t *testing.T) {
	tk := skills.NewHeuristicTokenizer()
	body := "short enough body"

	assert.Equal(t, body, truncateForEmbedding(tk, body))
}

func TestSyncTruncatesOversizedBodyBeforeEmbedding(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "memory")
	writeNote(t, dir, "huge.md",
		"title: Huge\ndate: 2026-06-20\nproblem_type: knowledge\nstatus: active",
		strings.Repeat("语义记忆内容", 20000))
	w := &fakeWriter{}
	embedder := &recordingEmbedder{vector: []float32{1, 0}}
	s := NewSyncerWithEmbedder(dir, w, nil, embedder)

	require.NoError(t, s.Sync(context.Background()))
	require.Len(t, w.memories, 1)
	assert.NotEmpty(t, w.memories[0].LLMEmbedding)
	assert.LessOrEqual(t,
		skills.NewHeuristicTokenizer().Count(embedder.lastInput), maxEmbedInputTokens)
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

func TestLedgerSyncerWithEmbedderPopulatesMemoryEmbedding(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "entries.jsonl")
	content := `{"created_at":"2026-07-01T13:36:35Z","id":"embedded","project":"ordo_ai","scope":"project","status":"active","text":"assist mem body to embed","type":"entrypoint"}` + "\n"
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))

	w := &fakeWriter{}
	embedder := &fakeEmbedder{vector: []float32{0.25, 0.75, 1}}
	s := NewLedgerSyncerWithEmbedder(path, w, nil, embedder)

	require.NoError(t, s.Sync(context.Background()))
	require.Len(t, w.memories, 1)
	assert.Equal(t, db.SourceAssistMem, w.memories[0].Source)
	assert.Equal(t, []float32{0.25, 0.75, 1}, w.memories[0].LLMEmbedding)
	assert.Equal(t, 3, w.memories[0].LLMEmbeddingDim)
	assert.Equal(t, 1, embedder.calls)
}

func TestLedgerSyncerWithEmbedderReusesUnchangedEmbedding(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "entries.jsonl")
	content := `{"created_at":"2026-07-01T13:36:35Z","id":"stable","project":"ordo_ai","scope":"project","status":"active","text":"stable assist body","type":"entrypoint"}` + "\n"
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	info, err := os.Stat(path)
	require.NoError(t, err)

	w := &fakeWriter{memories: []db.Memory{{
		RelPath: "assist-mem/stable.jsonl", Source: db.SourceAssistMem,
		Body: "stable assist body\n\nScope: project\n", SourceMtime: info.ModTime().Unix(),
		LLMEmbedding: []float32{0.1, 0.2}, LLMEmbeddingDim: 2,
	}}}
	embedder := &fakeEmbedder{vector: []float32{1, 0}}
	s := NewLedgerSyncerWithEmbedder(path, w, nil, embedder)

	require.NoError(t, s.Sync(context.Background()))
	require.Len(t, w.memories, 1)
	assert.Equal(t, []float32{0.1, 0.2}, w.memories[0].LLMEmbedding)
	assert.Zero(t, embedder.calls)
}

func TestLedgerSyncerWithEmbedderDoesNotReuseChangedBodyEmbedding(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "entries.jsonl")
	content := `{"created_at":"2026-07-01T13:36:35Z","id":"changed","project":"ordo_ai","scope":"project","status":"active","text":"changed assist body","type":"entrypoint"}` + "\n"
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	info, err := os.Stat(path)
	require.NoError(t, err)

	w := &fakeWriter{memories: []db.Memory{{
		RelPath: "assist-mem/changed.jsonl", Source: db.SourceAssistMem,
		Body: "old assist body\n\nScope: project\n", SourceMtime: info.ModTime().Unix(),
		LLMEmbedding: []float32{0.1, 0.2}, LLMEmbeddingDim: 2,
	}}}
	embedder := &fakeEmbedder{vector: []float32{1, 0, 0}}
	s := NewLedgerSyncerWithEmbedder(path, w, nil, embedder)

	require.NoError(t, s.Sync(context.Background()))
	require.Len(t, w.memories, 1)
	assert.Equal(t, []float32{1, 0, 0}, w.memories[0].LLMEmbedding)
	assert.Equal(t, 3, w.memories[0].LLMEmbeddingDim)
	assert.Equal(t, 1, embedder.calls)
}

func TestLedgerSyncerWithEmbedderKeepsMirrorOnEmbeddingFailure(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "entries.jsonl")
	content := `{"created_at":"2026-07-01T13:36:35Z","id":"broken","project":"ordo_ai","scope":"project","status":"active","text":"assist body","type":"entrypoint"}` + "\n"
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	w := &fakeWriter{}
	s := NewLedgerSyncerWithEmbedder(path, w, nil, &fakeEmbedder{err: errors.New("embed failed")})

	require.NoError(t, s.Sync(context.Background()),
		"a provider rejection must not abort the run")
	require.Len(t, w.memories, 1)
	assert.Empty(t, w.memories[0].LLMEmbedding)
}
