package extract

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.kenn.io/agentsview/internal/db"
	"go.kenn.io/agentsview/internal/llm"
)

func TestSystemPromptPreservesSourceLanguage(t *testing.T) {
	lower := strings.ToLower(systemPrompt)
	// Memory content must be written in the conversation's language (Chinese
	// sessions -> Chinese notes), not defaulted to English.
	assert.Contains(t, lower, "language")
	assert.Contains(t, systemPrompt, "中文")
}

func TestNewCandidateTagsOriginScope(t *testing.T) {
	now := time.Unix(0, 0).UTC()
	in := LLMCandidate{Summary: "s", Evidence: "e", Implication: "i", Category: "decision", Why: "because"}
	// A project session -> project scope, named by the session's project.
	proj, err := NewCandidate(in, db.Session{ID: "x", Agent: "kilo", Project: "ordo-ai", Cwd: "/Users/x/Projects/ordo-ai"}, nil, now)
	require.NoError(t, err)
	assert.Equal(t, "project", proj.Scope)
	assert.Equal(t, "ordo-ai", proj.OriginProject)
	// The dotfiles repo itself is general/user work.
	usr, err := NewCandidate(in, db.Session{ID: "y", Agent: "cc", Project: ".dotfiles", Cwd: "/Users/x/.dotfiles"}, nil, now)
	require.NoError(t, err)
	assert.Equal(t, "user", usr.Scope)
	assert.Equal(t, "", usr.OriginProject)
	// An agent-config session (~/.claude) is general/user even with a name.
	cfg, err := NewCandidate(in, db.Session{ID: "z", Agent: "cc", Project: "skills", Cwd: "/Users/x/.claude/skills"}, nil, now)
	require.NoError(t, err)
	assert.Equal(t, "user", cfg.Scope)
}

func TestCandidateCanonicalIDMatchesPythonCaptureAlgorithm(t *testing.T) {
	c := fixedCandidate(t)
	canonical, err := CanonicalForHash(c)
	require.NoError(t, err)
	var stable map[string]any
	require.NoError(t, json.Unmarshal([]byte(canonical), &stable))
	assert.NotContains(t, stable, "created_at")
	assert.NotContains(t, stable, "id")

	got := CandidateID(c)
	py := `import hashlib,json,sys; c=json.load(sys.stdin); stable={k:v for k,v in c.items() if k not in {"id","created_at"}}; print(hashlib.sha256(json.dumps(stable, ensure_ascii=False, sort_keys=True, separators=(",", ":")).encode("utf-8")).hexdigest())`
	cmd := exec.Command("python3", "-c", py)
	data, err := json.Marshal(c)
	require.NoError(t, err)
	cmd.Stdin = bytes.NewReader(data)
	out, err := cmd.Output()
	require.NoError(t, err)
	assert.Equal(t, stringsTrim(string(out)), got)
}

func TestCandidateSchemaIncludesPromotionFields(t *testing.T) {
	c := fixedCandidate(t)
	var raw map[string]any
	data, err := marshalJSON(c, false)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(data, &raw))
	for _, field := range []string{"summary", "evidence", "implication", "category", "problem_type", "origin_session", "source_platform", "source_roles", "created_at", "id", "why"} {
		assert.Contains(t, raw, field)
	}
	assert.Equal(t, "decision", raw["category"])
	assert.Equal(t, "decision", raw["problem_type"])
	assert.NotEmpty(t, raw["why"])
}

func TestWriterDedupDriftAndStagingOnly(t *testing.T) {
	root := setupGitRoot(t, true)
	c := fixedCandidate(t)
	w := Writer{Root: root}

	first, err := w.Write(context.Background(), c)
	require.NoError(t, err)
	assert.Equal(t, WriteWritten, first.Status)
	second, err := w.Write(context.Background(), c)
	require.NoError(t, err)
	assert.Equal(t, WriteDeduped, second.Status)
	entries, err := os.ReadDir(RawDir(root))
	require.NoError(t, err)
	jsonFiles := 0
	for _, entry := range entries {
		if filepath.Ext(entry.Name()) == ".json" {
			jsonFiles++
		}
	}
	assert.Equal(t, 1, jsonFiles)

	target := filepath.Join(RawDir(root), c.ID+".json")
	driftText := strings.Replace(string(mustReadFile(t, target)), c.Summary, "decision: use another queue", 1)
	require.NoError(t, os.WriteFile(target, []byte(driftText), 0o600))
	driftResult, err := w.Write(context.Background(), c)
	require.NoError(t, err)
	assert.Equal(t, WriteDriftRefused, driftResult.Status)
	_, err = os.Stat(filepath.Join(root, "memory", "user"))
	assert.True(t, os.IsNotExist(err), "extract must not write memory/user")
}

func TestWriterGitignoreRefused(t *testing.T) {
	root := setupGitRoot(t, false)
	got, err := (Writer{Root: root}).Write(context.Background(), fixedCandidate(t))
	require.NoError(t, err)
	assert.Equal(t, WriteGitignoreError, got.Status)
}

func TestWorkerFailOpenInvalidSecretAndLLMError(t *testing.T) {
	root := setupGitRoot(t, true)
	store := fakeStore{sessions: []db.Session{{ID: "s1", Agent: "claude"}}, messages: map[string][]db.Message{"s1": {{SessionID: "s1", Role: "user", Content: "decide to use queues"}}}}

	badJSON := NewWorker(store, fakeLLM{raw: `{"candidates":[{"category":"decision"}]}`}, root, nil)
	rec, err := badJSON.RunOnce(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 1, rec.Rejected)
	assertNoJSONFiles(t, RawDir(root))

	secret := NewWorker(store, fakeLLM{raw: `{"candidates":[{"category":"decision","summary":"decision: keep key","why":"because","evidence":"AWS key AKIA7QHWN2DKR4FYPLJM","implication":"reuse"}]}`}, root, nil)
	rec, err = secret.RunOnce(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 1, rec.Rejected)
	assertNoJSONFiles(t, RawDir(root))

	llmErr := NewWorker(store, fakeLLM{err: assert.AnError}, root, nil)
	rec, err = llmErr.RunOnce(context.Background())
	require.NoError(t, err)
	assert.Contains(t, rec.Error, "llm")
	assertNoJSONFiles(t, RawDir(root))
}

func TestWorkerWritesCandidateToRawStaging(t *testing.T) {
	root := setupGitRoot(t, true)
	store := fakeStore{sessions: []db.Session{{ID: "session 1", Agent: "claude"}}, messages: map[string][]db.Message{"session 1": {{SessionID: "session 1", Role: "user", Content: "We decided to use ResolveUsageLLM for extract."}}}}
	worker := NewWorker(store, fakeLLM{raw: `{"candidates":[{"category":"decision","summary":"decision: use ResolveUsageLLM for extract","why":"keeps provider usage binding","evidence":"User required ResolveUsageLLM for extract","implication":"Future extract worker calls should use the extract usage binding."}]}`}, root, nil)
	worker.now = func() time.Time { return time.Date(2026, 6, 26, 0, 0, 0, 0, time.UTC) }

	rec, err := worker.RunOnce(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 1, rec.Written)
	assert.Equal(t, 1, rec.CandidateCount)
	assert.Equal(t, 1, rec.StagingFiles)
	entries, err := os.ReadDir(RawDir(root))
	require.NoError(t, err)
	var files []string
	for _, entry := range entries {
		if filepath.Ext(entry.Name()) == ".json" {
			files = append(files, entry.Name())
		}
	}
	require.Len(t, files, 1)
	data, err := os.ReadFile(filepath.Join(RawDir(root), files[0]))
	require.NoError(t, err)
	assert.Contains(t, string(data), `"origin_session": "session-1"`)
	assert.Contains(t, string(data), `"source_platform": "claude"`)
	require.Len(t, rec.Candidates, 1)
	assert.Equal(t, filepath.ToSlash(filepath.Join(rawStagingRel, files[0])), rec.Candidates[0].Path)
	assert.NotContains(t, rec.Candidates[0].Path, root)
	assert.NotContains(t, string(data), root)

	if outDir := os.Getenv("EXTRACT_VERIFY_RAW_DIR"); outDir != "" {
		require.NoError(t, os.MkdirAll(outDir, 0o700))
		require.NoError(t, os.WriteFile(filepath.Join(outDir, files[0]), data, 0o600))
	}
}

func TestWorkerRecordsLLMMetricsAdditively(t *testing.T) {
	root := setupGitRoot(t, true)
	store := fakeStore{sessions: []db.Session{{ID: "s1", Agent: "claude"}}, messages: map[string][]db.Message{"s1": {{SessionID: "s1", Role: "user", Content: "We decided to observe extract metrics."}}}}
	worker := NewWorker(store, fakeLLM{
		raw:   `{"candidates":[{"category":"decision","summary":"decision: observe extract metrics","why":"quality proof","evidence":"User asked for metrics","implication":"Future workers should audit calls."}]}`,
		usage: llm.Usage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15},
	}, root, nil)

	rec, err := worker.RunOnce(context.Background())

	require.NoError(t, err)
	assert.Equal(t, 1, rec.LLMCallCount)
	assert.Equal(t, "extract", rec.ProviderUsage)
	require.NotNil(t, rec.LLMUsage)
	assert.Equal(t, 10, rec.LLMUsage.PromptTokens)
	assert.Equal(t, 5, rec.LLMUsage.CompletionTokens)
	assert.Equal(t, 15, rec.LLMUsage.TotalTokens)
}

func TestControllerDisabledTriggerIsNoop(t *testing.T) {
	root := setupGitRoot(t, true)
	llm := &countingLLM{raw: `{"candidates":[{"category":"decision","summary":"decision: x","why":"y","evidence":"z","implication":"q"}]}`}
	store := fakeStore{sessions: []db.Session{{ID: "s1", Agent: "claude"}}, messages: map[string][]db.Message{"s1": {{Role: "user", Content: "hello"}}}}
	ctl := NewController(NewWorker(store, llm, root, nil), false)
	ctl.Trigger()
	assert.Equal(t, 0, llm.calls)
}

type fakeLLM struct {
	raw   string
	err   error
	usage llm.Usage
}

func (f fakeLLM) ChatJSON(context.Context, string, string) (string, error) {
	return f.raw, f.err
}

func (f fakeLLM) ChatJSONUsage(context.Context, string, string) (string, llm.Usage, error) {
	return f.raw, f.usage, f.err
}

type countingLLM struct {
	raw   string
	calls int
}

func (f *countingLLM) ChatJSON(context.Context, string, string) (string, error) {
	f.calls++
	return f.raw, nil
}

type fakeStore struct {
	sessions []db.Session
	messages map[string][]db.Message
}

func (f fakeStore) ListSessions(context.Context, db.SessionFilter) (db.SessionPage, error) {
	return db.SessionPage{Sessions: f.sessions, Total: len(f.sessions)}, nil
}

func (f fakeStore) GetAllMessages(_ context.Context, sessionID string) ([]db.Message, error) {
	return f.messages[sessionID], nil
}

func fixedCandidate(t *testing.T) Candidate {
	t.Helper()
	c := Candidate{
		Summary:        "decision: use ResolveUsageLLM for extract",
		Evidence:       "Evidence: User required ResolveUsageLLM(\"extract\").",
		Implication:    "Future extract worker calls should use the extract usage binding.",
		Category:       "decision",
		ProblemType:    "decision",
		OriginSession:  "session-1",
		SourcePlatform: "claude",
		SourceRoles:    []string{"assistant", "user"},
		Why:            "keeps provider usage binding",
		CreatedAt:      "2026-06-26T00:00:00Z",
	}
	c.ID = CandidateID(c)
	require.NotEmpty(t, c.ID)
	return c
}

func setupGitRoot(t *testing.T, ignored bool) string {
	t.Helper()
	root := t.TempDir()
	require.NoError(t, exec.Command("git", "init", root).Run())
	require.NoError(t, os.MkdirAll(filepath.Join(root, "memory", ".staging", "raw_memories"), 0o700))
	if ignored {
		require.NoError(t, os.WriteFile(filepath.Join(root, ".gitignore"), []byte("memory/.staging/raw_memories/\n"), 0o600))
	}
	return root
}

func assertNoJSONFiles(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return
	}
	require.NoError(t, err)
	for _, entry := range entries {
		assert.NotEqual(t, ".json", filepath.Ext(entry.Name()))
	}
}

func stringsTrim(s string) string { return strings.TrimSpace(s) }

func mustReadFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	return data
}
