package synthesize

import (
	"context"
	"encoding/json"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.kenn.io/agentsview/internal/db"
)

func TestClusterNotesGroupsSimilarAndDropsSingletons(t *testing.T) {
	notes := []SourceNote{
		{ID: "a", Embedding: []float32{1, 0, 0}},
		{ID: "b", Embedding: []float32{0.99, 0.01, 0}},
		{ID: "c", Embedding: []float32{0, 1, 0}}, // far from a/b -> singleton, dropped
	}
	clusters := ClusterNotes(notes, 0.9)
	require.Len(t, clusters, 1, "only the a/b cluster survives (>=2 members)")
	ids := []string{clusters[0][0].ID, clusters[0][1].ID}
	assert.ElementsMatch(t, []string{"a", "b"}, ids)
}

func TestParseLLMDecisionToleratesCodeFence(t *testing.T) {
	d, err := ParseLLMDecision("```json\n{\"title\":\"主题\",\"insight\":\"要点\",\"problem_type\":\"decision\"}\n```")
	require.NoError(t, err)
	assert.Equal(t, "主题", d.Title)
	assert.Equal(t, "decision", d.ProblemType)
}

func TestEnsureCitationsAppendsMissing(t *testing.T) {
	got := ensureCitations("已有 (because of a) 引用", []string{"a", "b"})
	assert.Contains(t, got, "(because of a)")
	assert.Contains(t, got, "(because of b)")
	assert.Equal(t, 1, strings.Count(got, "(because of a)"), "no duplicate citation")
}

func TestSourceNotesFromMemoriesIncludesTopicsDropsIndexAndEmbeddingless(t *testing.T) {
	mems := []db.Memory{
		{RelPath: "atom.md", Source: db.SourceCrossAgent, Title: "t", OriginSession: "ses_x", LLMEmbedding: []float32{1, 0}},
		{RelPath: "synth.md", Source: db.SourceCrossAgent, OriginSession: "compact-memory:syn-1", LLMEmbedding: []float32{1, 0}}, // included as a topic
		{RelPath: "noembed.md", Source: db.SourceCrossAgent, OriginSession: "ses_y"},                                             // excluded (no embedding)
		{RelPath: "INDEX.md", Source: db.SourceCrossAgent, LLMEmbedding: []float32{1, 0}},                                        // excluded
		{RelPath: "canonical/current.json", Source: db.SourceCanonical, LLMEmbedding: []float32{1, 0}},                           // excluded as raw input
	}
	got := SourceNotesFromMemories(mems)
	require.Len(t, got, 2)
	byID := map[string]SourceNote{}
	for _, n := range got {
		byID[n.ID] = n
	}
	require.Contains(t, byID, "cross-agent:atom.md")
	require.Contains(t, byID, "cross-agent:synth.md")
	assert.False(t, byID["cross-agent:atom.md"].IsTopic, "raw atomic is not a topic")
	assert.True(t, byID["cross-agent:synth.md"].IsTopic, "compact-memory: origin is a topic")
	assert.Equal(t, "cross-agent:atom.md", byID["cross-agent:atom.md"].RawRefID)
}

func TestCanonicalSourceNotesFromMemoriesKeepSourceRelPathIdentity(t *testing.T) {
	mems := []db.Memory{
		{RelPath: "same.md", Source: db.SourceCrossAgent, LLMEmbedding: []float32{1, 0}},
		{RelPath: "same.md", Source: db.SourceAssistMem, LLMEmbedding: []float32{1, 0}},
		{RelPath: "proj/memory/same.md", Source: db.SourceCCNative, LLMEmbedding: []float32{1, 0}},
	}
	notes := SourceNotesFromMemories(mems)
	require.Len(t, notes, 3)
	ids := clusterIDs(notes)
	assert.ElementsMatch(t, []string{
		"cross-agent:same.md",
		"assist-mem:same.md",
		"cc-native:proj/memory/same.md",
	}, ids)
	for _, note := range notes {
		assert.NotEmpty(t, note.Source)
		assert.NotEmpty(t, note.RelPath)
		assert.Equal(t, stableRawRefID(note.Source, note.RelPath), note.RawRefID)
	}
}

// --- worker happy path ---

type fakeStore struct{ mems []db.Memory }

func (f fakeStore) MemoryEmbeddings(context.Context, db.MemoryFilter) ([]db.Memory, error) {
	return f.mems, nil
}

type captureStore struct {
	memsBySource map[string][]db.Memory
	queries      []db.MemoryFilter
	writes       [][]db.Memory
}

func (f *captureStore) MemoryEmbeddings(_ context.Context, filter db.MemoryFilter) ([]db.Memory, error) {
	f.queries = append(f.queries, filter)
	mems := f.memsBySource[filter.Source]
	out := make([]db.Memory, 0, len(mems))
	for _, m := range mems {
		if filter.Status != "" && m.Status != filter.Status {
			continue
		}
		out = append(out, m)
	}
	return out, nil
}

func (f *captureStore) ReplaceMemoriesBySource(_ context.Context, source string, memories []db.Memory) error {
	if source != db.SourceCanonical {
		return assert.AnError
	}
	f.writes = append(f.writes, append([]db.Memory(nil), memories...))
	f.memsBySource[source] = append([]db.Memory(nil), memories...)
	return nil
}

type fakeLLM struct{ resp string }

func (f fakeLLM) ChatJSON(context.Context, string, string) (string, error) { return f.resp, nil }

type sequenceLLM struct {
	responses []string
	idx       int
}

func (f *sequenceLLM) ChatJSON(context.Context, string, string) (string, error) {
	if f.idx >= len(f.responses) {
		return f.responses[len(f.responses)-1], nil
	}
	resp := f.responses[f.idx]
	f.idx++
	return resp, nil
}

type fakeScript struct {
	res    ScriptResult
	called int
	gotDF  string
}

func (f *fakeScript) Run(_ context.Context, _ string, decisionFile string) (ScriptResult, error) {
	f.called++
	f.gotDF = decisionFile
	return f.res, nil
}

type fakeCommitter struct{ called bool }

func (f *fakeCommitter) Commit(context.Context, string) error { f.called = true; return nil }

type fakeResyncer struct{ called bool }

func (f *fakeResyncer) Resync(context.Context) error { f.called = true; return nil }

func canonicalTestWorker(store *captureStore, resp string) *Worker {
	return &Worker{
		Store:          store,
		CanonicalStore: store,
		LLM:            fakeLLM{resp: resp},
		now:            func() time.Time { return time.Date(2026, 7, 6, 1, 2, 3, 0, time.UTC) },
	}
}

func memoryFixture(source, relPath, title string, embedding []float32) db.Memory {
	return db.Memory{
		RelPath:      relPath,
		Source:       source,
		Title:        title,
		Date:         "2026-07-06",
		ProblemType:  "knowledge",
		Status:       "active",
		Body:         title,
		LLMEmbedding: embedding,
		SyncedAt:     "2026-07-06T00:00:00Z",
	}
}

func memoryWithProblemType(source, relPath, title, problemType string, embedding []float32) db.Memory {
	m := memoryFixture(source, relPath, title, embedding)
	m.ProblemType = problemType
	return m
}

func frontmatterlessCCNativeFixture(relPath, title string, embedding []float32) db.Memory {
	m := memoryFixture(db.SourceCCNative, relPath, title, embedding)
	m.Status = ""
	return m
}

func queriedSources(filters []db.MemoryFilter) []string {
	out := make([]string, 0, len(filters))
	for _, filter := range filters {
		out = append(out, filter.Source)
	}
	return out
}

func TestWorkerSynthesizesCommitsAndAudits(t *testing.T) {
	store := fakeStore{mems: []db.Memory{
		{RelPath: "a.md", Title: "Signed URL deep compare", Body: "...", Status: "active", Source: db.SourceCrossAgent, OriginSession: "s1", LLMEmbedding: []float32{1, 0, 0}},
		{RelPath: "b.md", Title: "Strip signed URL before compare", Body: "...", Status: "active", Source: db.SourceCrossAgent, OriginSession: "s2", LLMEmbedding: []float32{0.99, 0.01, 0}},
	}}
	script := &fakeScript{res: ScriptResult{Stdout: "write syn-x memory/user/topic.md sources=a,b\n", ExitCode: 0}}
	commit := &fakeCommitter{}
	resync := &fakeResyncer{}
	w := &Worker{
		Root:   "/df",
		Store:  store,
		LLM:    fakeLLM{resp: `{"skip":false,"title":"比较前剥离 signed URL","insight":"对象比较忽略 signed URL","problem_type":"decision","keywords":["compare"]}`},
		Script: script,
		Commit: commit,
		Resync: resync,
		Audit:  NewAuditLog(t.TempDir() + "/.synthesize-audit.jsonl"),
	}

	rec, err := w.RunOnce(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 2, rec.NoteCount)
	assert.Equal(t, 1, rec.ClusterCount)
	assert.Equal(t, 1, rec.WriteCount)
	assert.True(t, rec.Committed)
	assert.True(t, rec.Resynced)
	assert.Equal(t, 1, script.called)
	require.Len(t, rec.Topics, 1)
	assert.ElementsMatch(t, []string{"a", "b"}, rec.Topics[0].SourceIDs)

	recs, err := w.Audit.Read(0)
	require.NoError(t, err)
	require.Len(t, recs, 1)
	assert.Equal(t, 1, recs[0].WriteCount)
}

func TestWorkerPreviewUsesDefaultSourceAllowlistAndDoesNotWrite(t *testing.T) {
	store := &captureStore{memsBySource: map[string][]db.Memory{
		db.SourceAssistMem:  {memoryFixture(db.SourceAssistMem, "ledger/entrypoint-1.jsonl", "当前入口", []float32{1, 0, 0})},
		db.SourceCCNative:   {memoryFixture(db.SourceCCNative, "project/memory/entrypoint.md", "入口路径", []float32{0.99, 0.01, 0})},
		db.SourceCrossAgent: {memoryFixture(db.SourceCrossAgent, "env.md", "环境事实", []float32{0, 1, 0})},
	}}
	w := canonicalTestWorker(store, `{"skip":false,"title":"当前入口","insight":"入口点是 lzn-preview","problem_type":"knowledge"}`)

	rec, err := w.Preview(context.Background())
	require.NoError(t, err)
	assert.True(t, rec.DryRun)
	assert.Equal(t, 0, rec.WriteCount)
	assert.Equal(t, 0, rec.CanonicalWriteCount)
	assert.Empty(t, store.writes, "preview must not write canonical rows")
	assert.ElementsMatch(t, []string{db.SourceAssistMem, db.SourceCCNative, db.SourceCrossAgent}, queriedSources(store.queries))
	assert.Equal(t, map[string]int{db.SourceAssistMem: 1, db.SourceCCNative: 1, db.SourceCrossAgent: 1}, rec.SourceCounts)
	assert.Equal(t, map[string]int{db.SourceAssistMem: 1, db.SourceCCNative: 1, db.SourceCrossAgent: 1}, rec.EligibleSourceCounts)
	assert.Equal(t, 1, rec.PlannedCanonicalCount)
	require.Len(t, rec.Topics, 1)
	assert.ElementsMatch(t, []RawRef{
		{Source: db.SourceAssistMem, RelPath: "ledger/entrypoint-1.jsonl"},
		{Source: db.SourceCCNative, RelPath: "project/memory/entrypoint.md"},
	}, rec.Topics[0].CoveredRefs)
	assert.Contains(t, rec.ConflictSamples, "separate singleton: cross-agent:env.md")
}

func TestWorkerIncludesFrontmatterlessCCNativeRowsAsActiveInput(t *testing.T) {
	store := &captureStore{memsBySource: map[string][]db.Memory{
		db.SourceAssistMem: {memoryFixture(db.SourceAssistMem, "entrypoint.jsonl", "当前入口", []float32{1, 0})},
		db.SourceCCNative:  {frontmatterlessCCNativeFixture("project/memory/entrypoint.md", "Claude 入口", []float32{0.99, 0.01})},
	}}
	w := canonicalTestWorker(store, `{"skip":false,"title":"当前入口","insight":"入口点","problem_type":"knowledge"}`)

	rec, err := w.Preview(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 1, rec.SourceCounts[db.SourceCCNative])
	assert.Equal(t, 1, rec.EligibleSourceCounts[db.SourceCCNative])
	require.Len(t, rec.Topics, 1)
	assert.Contains(t, rec.Topics[0].SourceIDs, "cc-native:project/memory/entrypoint.md")
}

func TestWorkerSourceAllowlistControlsInputAndExcludesCanonical(t *testing.T) {
	store := &captureStore{memsBySource: map[string][]db.Memory{
		db.SourceAssistMem:  {memoryFixture(db.SourceAssistMem, "a.jsonl", "A", []float32{1, 0})},
		db.SourceCCNative:   {memoryFixture(db.SourceCCNative, "b.md", "B", []float32{1, 0})},
		db.SourceCanonical:  {memoryFixture(db.SourceCanonical, "canonical/old.json", "Old", []float32{1, 0})},
		db.SourceCrossAgent: {memoryFixture(db.SourceCrossAgent, "c.md", "C", []float32{1, 0})},
	}}
	w := canonicalTestWorker(store, `{"skip":false,"title":"AB","insight":"AB","problem_type":"knowledge"}`)
	w.SourceAllowlist = []string{db.SourceAssistMem, db.SourceCCNative, db.SourceCanonical, db.SourceAssistMem}

	rec, err := w.Preview(context.Background())
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{db.SourceAssistMem, db.SourceCCNative}, queriedSources(store.queries))
	assert.Equal(t, map[string]int{db.SourceAssistMem: 1, db.SourceCCNative: 1}, rec.SourceCounts)
	assert.Equal(t, 1, rec.PlannedCanonicalCount)
}

func TestWorkerCanonicalWritePreservesRawRowsAndIsIdempotent(t *testing.T) {
	store := &captureStore{memsBySource: map[string][]db.Memory{
		db.SourceAssistMem: {memoryFixture(db.SourceAssistMem, "entrypoint-1.jsonl", "当前入口", []float32{1, 0})},
		db.SourceCCNative:  {memoryFixture(db.SourceCCNative, "entrypoint-2.md", "当前入口", []float32{0.99, 0.01})},
	}}
	w := canonicalTestWorker(store, `{"skip":false,"title":"当前入口","insight":"统一后的入口事实","problem_type":"knowledge"}`)

	first, err := w.RunOnce(context.Background())
	require.NoError(t, err)
	second, err := w.RunOnce(context.Background())
	require.NoError(t, err)
	require.Len(t, store.writes, 2)
	require.Len(t, store.writes[0], 1)
	require.Len(t, store.writes[1], 1)
	assert.Equal(t, store.writes[0][0].RelPath, store.writes[1][0].RelPath)
	assert.Equal(t, store.writes[0][0].CanonicalCoveredRefs, store.writes[1][0].CanonicalCoveredRefs)
	assert.Equal(t, store.writes[0][0].CanonicalProvenance, store.writes[1][0].CanonicalProvenance)
	assert.Equal(t, 1, first.CanonicalWriteCount)
	assert.Equal(t, 1, second.CanonicalWriteCount)

	rawAssist := store.memsBySource[db.SourceAssistMem]
	rawCC := store.memsBySource[db.SourceCCNative]
	require.Len(t, rawAssist, 1)
	require.Len(t, rawCC, 1)
	assert.Equal(t, db.SourceAssistMem, rawAssist[0].Source)
	assert.Equal(t, db.SourceCCNative, rawCC[0].Source)
	assert.Empty(t, rawAssist[0].CanonicalCoveredRefs)
	canonical := store.writes[0][0]
	assert.Equal(t, db.SourceCanonical, canonical.Source)
	assert.Equal(t, "canonical", canonical.Type)
	assert.JSONEq(t, `[{"source":"assist-mem","rel_path":"entrypoint-1.jsonl"},{"source":"cc-native","rel_path":"entrypoint-2.md"}]`, canonical.CanonicalCoveredRefs)
	assert.Contains(t, canonical.CanonicalProvenance, canonicalSynthesisVersion)
}

func TestWorkerClearsCanonicalRowsWhenCurrentPlanIsEmpty(t *testing.T) {
	store := &captureStore{memsBySource: map[string][]db.Memory{
		db.SourceCanonical: {{RelPath: "canonical/stale.json", Source: db.SourceCanonical, Title: "stale", Status: "active"}},
	}}
	w := canonicalTestWorker(store, `{"skip":false,"title":"unused","insight":"unused","problem_type":"knowledge"}`)

	rec, err := w.RunOnce(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 0, rec.PlannedCanonicalCount)
	assert.Equal(t, 1, rec.CanonicalWriteCount, "empty current plan should still replace canonical source with empty rows")
	require.Len(t, store.writes, 1)
	assert.Empty(t, store.writes[0])
	assert.Empty(t, store.memsBySource[db.SourceCanonical])
}

func TestWorkerKeepsPreviousCanonicalRowsWhenAnyClusterFails(t *testing.T) {
	previous := []db.Memory{{RelPath: "canonical/previous.json", Source: db.SourceCanonical, Title: "previous", Status: "active"}}
	store := &captureStore{memsBySource: map[string][]db.Memory{
		db.SourceAssistMem: {
			memoryFixture(db.SourceAssistMem, "entrypoint-a.jsonl", "当前入口 A", []float32{1, 0, 0}),
			memoryFixture(db.SourceAssistMem, "entrypoint-b.jsonl", "当前入口 B", []float32{0.99, 0.01, 0}),
			memoryFixture(db.SourceAssistMem, "failure-a.jsonl", "失败簇 A", []float32{0, 1, 0}),
			memoryFixture(db.SourceAssistMem, "failure-b.jsonl", "失败簇 B", []float32{0.01, 0.99, 0}),
		},
		db.SourceCanonical: previous,
	}}
	w := &Worker{
		Store:          store,
		CanonicalStore: store,
		LLM:            &sequenceLLM{responses: []string{`{"skip":false,"title":"当前入口","insight":"入口点","problem_type":"knowledge"}`, `{"skip":true}`}},
		now:            func() time.Time { return time.Date(2026, 7, 6, 1, 2, 3, 0, time.UTC) },
	}

	rec, err := w.RunOnce(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 1, rec.PlannedCanonicalCount)
	assert.Equal(t, 1, rec.SkippedCount)
	assert.Equal(t, 1, rec.FailedCount)
	assert.Equal(t, 0, rec.CanonicalWriteCount)
	assert.Empty(t, store.writes, "partial LLM failure must keep previous canonical rows")
	assert.Equal(t, previous, store.memsBySource[db.SourceCanonical])
	require.Len(t, rec.Topics, 2)
	assert.True(t, slices.ContainsFunc(rec.Topics, func(topic TopicRecord) bool { return topic.Skipped && strings.Contains(topic.Result, "llm_skip") }))
}

func TestWorkerLznLikeFixtureKeepsEntrypointSeparateFromExceptionAndEnvironment(t *testing.T) {
	store := &captureStore{memsBySource: map[string][]db.Memory{
		db.SourceAssistMem: {
			memoryFixture(db.SourceAssistMem, "lzn-entrypoint.jsonl", "lzn-preview current entrypoint", []float32{1, 0, 0}),
			memoryFixture(db.SourceAssistMem, "security-exception.jsonl", "允许特定安全例外", []float32{0, 1, 0}),
		},
		db.SourceCCNative: {
			memoryFixture(db.SourceCCNative, "lzn-entrypoint.md", "lzn-test uses lzn-preview", []float32{0.99, 0.01, 0}),
		},
		db.SourceCrossAgent: {
			memoryFixture(db.SourceCrossAgent, "environment.md", "本地环境事实", []float32{0, 0, 1}),
		},
	}}
	w := canonicalTestWorker(store, `{"skip":false,"title":"lzn 当前入口","insight":"当前入口是 lzn-preview","problem_type":"knowledge"}`)

	rec, err := w.Preview(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, rec.PlannedCanonicalCount)
	require.Len(t, rec.Topics, 1)
	assert.ElementsMatch(t, []RawRef{
		{Source: db.SourceAssistMem, RelPath: "lzn-entrypoint.jsonl"},
		{Source: db.SourceCCNative, RelPath: "lzn-entrypoint.md"},
	}, rec.Topics[0].CoveredRefs)
	assert.NotContains(t, rec.Topics[0].SourceIDs, "assist-mem:security-exception.jsonl")
	assert.NotContains(t, rec.Topics[0].SourceIDs, "cross-agent:environment.md")
	assert.Contains(t, rec.ConflictSamples, "separate singleton: assist-mem:security-exception.jsonl")
	assert.Contains(t, rec.ConflictSamples, "separate singleton: cross-agent:environment.md")
}

func TestWorkerLznLikeFixtureSeparatesExceptionAndEnvironmentWithSimilarVectors(t *testing.T) {
	store := &captureStore{memsBySource: map[string][]db.Memory{
		db.SourceAssistMem: {
			memoryFixture(db.SourceAssistMem, "lzn-entrypoint.jsonl", "lzn-preview current entrypoint", []float32{1, 0, 0}),
			memoryWithProblemType(db.SourceAssistMem, "security-exception.jsonl", "security exception for lzn-preview", "security-exception", []float32{0.999, 0.001, 0}),
		},
		db.SourceCCNative: {
			memoryFixture(db.SourceCCNative, "lzn-entrypoint.md", "lzn-test uses lzn-preview", []float32{0.998, 0.002, 0}),
		},
		db.SourceCrossAgent: {
			memoryWithProblemType(db.SourceCrossAgent, "environment.md", "lzn-preview local environment", "environment", []float32{0.997, 0.003, 0}),
		},
	}}
	w := canonicalTestWorker(store, `{"skip":false,"title":"lzn 当前入口","insight":"当前入口是 lzn-preview","problem_type":"knowledge"}`)

	rec, err := w.Preview(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, rec.PlannedCanonicalCount)
	require.Len(t, rec.Topics, 1)
	assert.ElementsMatch(t, []RawRef{
		{Source: db.SourceAssistMem, RelPath: "lzn-entrypoint.jsonl"},
		{Source: db.SourceCCNative, RelPath: "lzn-entrypoint.md"},
	}, rec.Topics[0].CoveredRefs)
	assert.NotContains(t, rec.Topics[0].SourceIDs, "assist-mem:security-exception.jsonl")
	assert.NotContains(t, rec.Topics[0].SourceIDs, "cross-agent:environment.md")
	assert.GreaterOrEqual(t, rec.ConflictCount, 1)
	assert.Contains(t, rec.ConflictSamples, "separate guard: assist-mem:security-exception.jsonl")
	assert.Contains(t, rec.ConflictSamples, "separate guard: cross-agent:environment.md")
}

func TestWorkerLznAcceptanceFixtureFromCommittedTestdata(t *testing.T) {
	fixture := loadLznAcceptanceFixture(t)
	store := &captureStore{memsBySource: map[string][]db.Memory{}}
	for _, rawMemory := range fixture.Memories {
		memory := rawMemory.dbMemory()
		store.memsBySource[memory.Source] = append(store.memsBySource[memory.Source], memory)
	}
	w := canonicalTestWorker(store, fixture.LLMResponse)

	rec, err := w.Preview(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, rec.PlannedCanonicalCount)
	require.Len(t, rec.Topics, 1)
	assert.ElementsMatch(t, []RawRef{
		{Source: db.SourceAssistMem, RelPath: "lzn-preview/entrypoint.jsonl"},
		{Source: db.SourceCCNative, RelPath: "lzn-test/memory/entrypoint.md"},
	}, rec.Topics[0].CoveredRefs)
	assert.NotContains(t, rec.Topics[0].SourceIDs, "cross-agent:lzn-preview/security-exception.md")
	assert.NotContains(t, rec.Topics[0].SourceIDs, "cc-native:lzn-test/memory/environment.md")
	assert.GreaterOrEqual(t, rec.ConflictCount, 2)
	assert.Contains(t, rec.ConflictSamples, "separate guard: cross-agent:lzn-preview/security-exception.md")
	assert.Contains(t, rec.ConflictSamples, "separate guard: cc-native:lzn-test/memory/environment.md")
	assert.Equal(t, map[string]int{db.SourceAssistMem: 1, db.SourceCCNative: 2, db.SourceCrossAgent: 1}, rec.SourceCounts)
	assert.Equal(t, map[string]int{db.SourceAssistMem: 1, db.SourceCCNative: 2, db.SourceCrossAgent: 1}, rec.EligibleSourceCounts)
}

type lznAcceptanceFixture struct {
	LLMResponse string                `json:"llm_response"`
	Memories    []lznAcceptanceMemory `json:"memories"`
}

type lznAcceptanceMemory struct {
	Source      string    `json:"source"`
	RelPath     string    `json:"rel_path"`
	Title       string    `json:"title"`
	Body        string    `json:"body"`
	ProblemType string    `json:"problem_type"`
	Embedding   []float32 `json:"embedding"`
}

func loadLznAcceptanceFixture(t *testing.T) lznAcceptanceFixture {
	t.Helper()
	data, err := os.ReadFile("testdata/lzn_acceptance.json")
	require.NoError(t, err)
	var raw lznAcceptanceFixture
	require.NoError(t, json.Unmarshal(data, &raw))
	require.NotEmpty(t, raw.LLMResponse)
	require.NotEmpty(t, raw.Memories)

	for _, rawMemory := range raw.Memories {
		require.NotEmpty(t, rawMemory.Source)
		require.NotEmpty(t, rawMemory.RelPath)
		require.NotEmpty(t, rawMemory.Embedding)
	}
	return raw
}

func (m lznAcceptanceMemory) dbMemory() db.Memory {
	memory := memoryWithProblemType(m.Source, m.RelPath, m.Title, m.ProblemType, m.Embedding)
	memory.Body = m.Body
	return memory
}

func TestClusterProjectDerivation(t *testing.T) {
	cases := []struct {
		name        string
		cluster     []SourceNote
		wantProject string
		wantScope   string
	}{
		{
			name:        "single shared project",
			cluster:     []SourceNote{{Project: "oss-atlas"}, {Project: "oss-atlas"}},
			wantProject: "oss-atlas",
			wantScope:   "project",
		},
		{
			name:        "spans multiple projects -> general",
			cluster:     []SourceNote{{Project: "oss-atlas"}, {Project: "ordo_ai"}},
			wantProject: "",
			wantScope:   "general",
		},
		{
			name:        "any general source -> general",
			cluster:     []SourceNote{{Project: "oss-atlas"}, {Project: ""}},
			wantProject: "",
			wantScope:   "general",
		},
		{
			name:        "all general -> general",
			cluster:     []SourceNote{{Project: ""}, {Project: ""}},
			wantProject: "",
			wantScope:   "general",
		},
		{
			name:        "topic+atomic spanning projects -> general",
			cluster:     []SourceNote{{Project: "oss-atlas", IsTopic: true}, {Project: "ordo_ai"}},
			wantProject: "",
			wantScope:   "general",
		},
		{
			name:        "topic+atomic same project -> project",
			cluster:     []SourceNote{{Project: "oss-atlas", IsTopic: true}, {Project: "oss-atlas"}},
			wantProject: "oss-atlas",
			wantScope:   "project",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			project, scope := clusterProject(tc.cluster)
			assert.Equal(t, tc.wantProject, project)
			assert.Equal(t, tc.wantScope, scope)
		})
	}
}

func TestSourceNotesFromMemoriesCarriesProject(t *testing.T) {
	mems := []db.Memory{
		{RelPath: "a.md", Title: "A", Body: "b", OriginProject: "oss-atlas", LLMEmbedding: []float32{1, 0}},
	}
	notes := SourceNotesFromMemories(mems)
	require.Len(t, notes, 1)
	assert.Equal(t, "oss-atlas", notes[0].Project)
}

// --- topic-aware clustering (BuildClusters) ---

const testMergeMinSim = defaultMergeSimilarity

// near returns an embedding very close (cosine > 0.99) to {1,0,0}-style base.
func TestBuildClustersAtomicNearTopicMerges(t *testing.T) {
	notes := []SourceNote{
		// two atomics that cluster together (cos ~1) and are ~1 vs the topic.
		{ID: "a1", Embedding: []float32{1, 0, 0}},
		{ID: "a2", Embedding: []float32{0.999, 0.001, 0}},
		// existing topic note basically identical to the atomic theme.
		{ID: "t1", Embedding: []float32{0.998, 0.002, 0}, IsTopic: true},
	}
	clusters := BuildClusters(notes, 0.55, testMergeMinSim)
	require.Len(t, clusters, 1)
	require.True(t, clusterHasTopic(clusters[0]), "cluster should be a MERGE (contains topic)")
	ids := clusterIDs(clusters[0])
	assert.ElementsMatch(t, []string{"a1", "a2", "t1"}, ids)
}

func TestBuildClustersTwoTopicsAboveThresholdMerge(t *testing.T) {
	notes := []SourceNote{
		{ID: "t1", Embedding: []float32{1, 0, 0}, IsTopic: true},
		{ID: "t2", Embedding: []float32{0.999, 0.001, 0}, IsTopic: true},
	}
	clusters := BuildClusters(notes, 0.55, testMergeMinSim)
	require.Len(t, clusters, 1)
	assert.True(t, clusterHasTopic(clusters[0]))
	assert.ElementsMatch(t, []string{"t1", "t2"}, clusterIDs(clusters[0]))
}

func TestBuildClustersTwoTopicsBelowThresholdNotMerged(t *testing.T) {
	// cosine({1,0,0},{0.7,0.714,0}) ~= 0.70 < 0.82 -> distinct-but-related, no merge.
	notes := []SourceNote{
		{ID: "t1", Embedding: []float32{1, 0, 0}, IsTopic: true},
		{ID: "t2", Embedding: []float32{0.7, 0.714, 0}, IsTopic: true},
	}
	clusters := BuildClusters(notes, 0.55, testMergeMinSim)
	assert.Empty(t, clusters, "topics below mergeMinSim must NOT be merged")
}

func TestBuildClustersAllAtomicFarFromTopicsAdds(t *testing.T) {
	// regression: atomic cluster orthogonal to the only topic -> plain ADD.
	notes := []SourceNote{
		{ID: "a1", Embedding: []float32{0, 1, 0}},
		{ID: "a2", Embedding: []float32{0.001, 0.999, 0}},
		{ID: "t1", Embedding: []float32{1, 0, 0}, IsTopic: true}, // cos ~0 with atomics
	}
	clusters := BuildClusters(notes, 0.55, testMergeMinSim)
	require.Len(t, clusters, 1)
	assert.False(t, clusterHasTopic(clusters[0]), "should be a fresh ADD, not a merge")
	assert.ElementsMatch(t, []string{"a1", "a2"}, clusterIDs(clusters[0]))
}

func clusterIDs(c []SourceNote) []string {
	ids := make([]string, 0, len(c))
	for _, n := range c {
		ids = append(ids, n.ID)
	}
	return ids
}

// --- merge decision wiring through the worker ---

func TestWorkerMergeIncludesTopicInSourcesAndStale(t *testing.T) {
	store := fakeStore{mems: []db.Memory{
		{RelPath: "a1.md", Title: "atomic one", Body: "x", Status: "active", Source: db.SourceCrossAgent, OriginSession: "s1", LLMEmbedding: []float32{1, 0, 0}},
		{RelPath: "a2.md", Title: "atomic two", Body: "y", Status: "active", Source: db.SourceCrossAgent, OriginSession: "s2", LLMEmbedding: []float32{0.999, 0.001, 0}},
		{RelPath: "t1.md", Title: "existing topic", Body: "z", Status: "active", Source: db.SourceCrossAgent, OriginSession: "compact-memory:syn-old", LLMEmbedding: []float32{0.998, 0.002, 0}},
	}}
	script := &captureScript{res: ScriptResult{Stdout: "write syn-x memory/user/topic.md sources=a1,a2,t1\n"}}
	w := &Worker{
		Root:   "/df",
		Store:  store,
		LLM:    fakeLLM{resp: `{"skip":false,"title":"合并后的主题","insight":"综合要点","problem_type":"knowledge","keywords":["k"]}`},
		Script: script,
		Commit: &fakeCommitter{},
		Resync: &fakeResyncer{},
		Audit:  NewAuditLog(t.TempDir() + "/.synthesize-audit.jsonl"),
	}

	rec, err := w.RunOnce(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 1, rec.WriteCount)
	require.Equal(t, 1, script.called)

	var d Decision
	require.NoError(t, json.Unmarshal([]byte(script.decisionJSON), &d))
	assert.Equal(t, "ADD", d.Action, "a merge is an ADD whose sources include the topic")
	assert.ElementsMatch(t, []string{"a1", "a2", "t1"}, d.SourceIDs)
	require.Contains(t, d.StaleSources, "t1")
	assert.Contains(t, d.StaleSources["t1"], "merged into topic", "topic source gets a merge reason")
	assert.Contains(t, d.StaleSources["a1"], "folded into topic", "atomic source gets a fold reason")
	// every source cited in the insight
	for _, id := range []string{"a1", "a2", "t1"} {
		assert.Contains(t, d.Insight, "(because of "+id+")")
	}
}

func TestWorkerMergeSkipWritesNothing(t *testing.T) {
	store := fakeStore{mems: []db.Memory{
		{RelPath: "a1.md", Title: "atomic one", Body: "x", Status: "active", Source: db.SourceCrossAgent, OriginSession: "s1", LLMEmbedding: []float32{1, 0, 0}},
		{RelPath: "a2.md", Title: "atomic two", Body: "y", Status: "active", Source: db.SourceCrossAgent, OriginSession: "s2", LLMEmbedding: []float32{0.999, 0.001, 0}},
		{RelPath: "t1.md", Title: "existing topic", Body: "z", Status: "active", Source: db.SourceCrossAgent, OriginSession: "compact-memory:syn-old", LLMEmbedding: []float32{0.998, 0.002, 0}},
	}}
	script := &captureScript{res: ScriptResult{Stdout: "write should-not-happen\n"}}
	commit := &fakeCommitter{}
	w := &Worker{
		Root:   "/df",
		Store:  store,
		LLM:    fakeLLM{resp: `{"skip":true}`},
		Script: script,
		Commit: commit,
		Resync: &fakeResyncer{},
		Audit:  NewAuditLog(t.TempDir() + "/.synthesize-audit.jsonl"),
	}

	rec, err := w.RunOnce(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 0, rec.WriteCount)
	assert.Equal(t, 0, script.called, "LLM skip must not invoke compact_memory")
	assert.False(t, commit.called, "nothing written -> no commit")
}

// captureScript records the decision file JSON so tests can assert on the
// decision the worker built.
type captureScript struct {
	res          ScriptResult
	called       int
	decisionJSON string
}

func (f *captureScript) Run(_ context.Context, _ string, decisionFile string) (ScriptResult, error) {
	f.called++
	data, err := os.ReadFile(decisionFile)
	if err != nil {
		return ScriptResult{}, err
	}
	f.decisionJSON = string(data)
	return f.res, nil
}
