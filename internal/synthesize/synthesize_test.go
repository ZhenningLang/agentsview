package synthesize

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

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
		{RelPath: "atom.md", Title: "t", OriginSession: "ses_x", LLMEmbedding: []float32{1, 0}},
		{RelPath: "synth.md", OriginSession: "compact-memory:syn-1", LLMEmbedding: []float32{1, 0}}, // included as a topic
		{RelPath: "noembed.md", OriginSession: "ses_y"},                                             // excluded (no embedding)
		{RelPath: "INDEX.md", LLMEmbedding: []float32{1, 0}},                                         // excluded
	}
	got := SourceNotesFromMemories(mems)
	require.Len(t, got, 2)
	byID := map[string]SourceNote{}
	for _, n := range got {
		byID[n.ID] = n
	}
	require.Contains(t, byID, "atom")
	require.Contains(t, byID, "synth")
	assert.False(t, byID["atom"].IsTopic, "raw atomic is not a topic")
	assert.True(t, byID["synth"].IsTopic, "compact-memory: origin is a topic")
}

// --- worker happy path ---

type fakeStore struct{ mems []db.Memory }

func (f fakeStore) MemoryEmbeddings(context.Context, db.MemoryFilter) ([]db.Memory, error) {
	return f.mems, nil
}

type fakeLLM struct{ resp string }

func (f fakeLLM) ChatJSON(context.Context, string, string) (string, error) { return f.resp, nil }

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
