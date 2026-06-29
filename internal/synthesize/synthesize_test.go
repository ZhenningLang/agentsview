package synthesize

import (
	"context"
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

func TestSourceNotesFromMemoriesExcludesSynthesizedAndEmbeddingless(t *testing.T) {
	mems := []db.Memory{
		{RelPath: "atom.md", Title: "t", OriginSession: "ses_x", LLMEmbedding: []float32{1, 0}},
		{RelPath: "synth.md", OriginSession: "compact-memory:syn-1", LLMEmbedding: []float32{1, 0}}, // excluded
		{RelPath: "noembed.md", OriginSession: "ses_y"},                                             // excluded (no embedding)
		{RelPath: "INDEX.md", LLMEmbedding: []float32{1, 0}},                                         // excluded
	}
	got := SourceNotesFromMemories(mems)
	require.Len(t, got, 1)
	assert.Equal(t, "atom", got[0].ID)
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
