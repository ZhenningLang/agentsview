package consolidate

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// --- decision parsing (spec verify ①) ---

func TestParseDecisions_ValidObject(t *testing.T) {
	raw := `{"a":{"action":"ADD"},"b":{"action":"UPDATE","note_id":"x.md","reason":"newer"}}`
	got, err := ParseDecisions(raw, []string{"a", "b"})
	if err != nil {
		t.Fatalf("ParseDecisions: %v", err)
	}
	if got["a"].Action != ActionADD {
		t.Errorf("a action = %q, want ADD", got["a"].Action)
	}
	if got["b"].Action != ActionUPDATE || got["b"].NoteID != "x.md" {
		t.Errorf("b = %+v, want UPDATE x.md", got["b"])
	}
}

func TestParseDecisions_CodeFenceWrapped(t *testing.T) {
	raw := "Sure!\n```json\n{\"a\": {\"action\": \"add\"}}\n```\n"
	got, err := ParseDecisions(raw, []string{"a"})
	if err != nil {
		t.Fatalf("ParseDecisions: %v", err)
	}
	if got["a"].Action != ActionADD {
		t.Errorf("a action = %q, want ADD (case-normalized)", got["a"].Action)
	}
}

func TestParseDecisions_UnknownActionBecomesSkip(t *testing.T) {
	raw := `{"a":{"action":"FRANBULATE"}}`
	got, err := ParseDecisions(raw, []string{"a"})
	if err != nil {
		t.Fatalf("ParseDecisions: %v", err)
	}
	if got["a"].Action != ActionSKIP {
		t.Errorf("unknown action = %q, want SKIP", got["a"].Action)
	}
}

func TestParseDecisions_DropsHallucinatedIDs(t *testing.T) {
	raw := `{"a":{"action":"ADD"},"ghost":{"action":"ADD"}}`
	got, err := ParseDecisions(raw, []string{"a"})
	if err != nil {
		t.Fatalf("ParseDecisions: %v", err)
	}
	if _, ok := got["ghost"]; ok {
		t.Errorf("hallucinated id should be dropped, got %+v", got)
	}
}

func TestParseDecisions_NonJSON(t *testing.T) {
	for _, raw := range []string{"", "I cannot help with that.", "[1,2,3]", "{not json}"} {
		if _, err := ParseDecisions(raw, nil); err == nil {
			t.Errorf("ParseDecisions(%q) want error, got nil", raw)
		}
	}
}

// --- candidate reading ---

func writeCandidate(t *testing.T, dir, name string, body map[string]any) {
	t.Helper()
	data, _ := json.Marshal(body)
	if err := os.WriteFile(filepath.Join(dir, name), data, 0o600); err != nil {
		t.Fatalf("write candidate: %v", err)
	}
}

func TestReadCandidates_SkipsMalformed(t *testing.T) {
	dir := t.TempDir()
	writeCandidate(t, dir, "good.json", map[string]any{"id": "good", "summary": "s"})
	if err := os.WriteFile(filepath.Join(dir, "bad.json"), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "ignore.txt"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := ReadCandidates(dir)
	if err != nil {
		t.Fatalf("ReadCandidates: %v", err)
	}
	if len(got) != 1 || got[0].effectiveID() != "good" {
		t.Fatalf("want 1 good candidate, got %+v", got)
	}
}

func TestReadCandidates_MissingDir(t *testing.T) {
	got, err := ReadCandidates(filepath.Join(t.TempDir(), "nope"))
	if err != nil {
		t.Fatalf("missing dir should not error: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("want 0 candidates, got %d", len(got))
	}
}

func TestCandidateEffectiveID_FallsBackToStem(t *testing.T) {
	dir := t.TempDir()
	writeCandidate(t, dir, "abc123.json", map[string]any{"summary": "no id field"})
	got, _ := ReadCandidates(dir)
	if len(got) != 1 || got[0].effectiveID() != "abc123" {
		t.Fatalf("want id from stem abc123, got %+v", got)
	}
}

// --- fakes for the worker ---

type fakeLLM struct {
	resp string
	err  error
}

func (f fakeLLM) ChatJSON(_ context.Context, _, _ string) (string, error) {
	return f.resp, f.err
}

type fakeScript struct {
	res    ScriptResult
	err    error
	called bool
	gotDF  string
	onRun  func(decisionFile string)
}

func (f *fakeScript) Run(_ context.Context, _, _, decisionFile string) (ScriptResult, error) {
	f.called = true
	f.gotDF = decisionFile
	if f.onRun != nil {
		f.onRun(decisionFile)
	}
	return f.res, f.err
}

type fakeCommitter struct {
	called bool
	msg    string
	err    error
}

func (f *fakeCommitter) Commit(_ context.Context, message string) error {
	f.called = true
	f.msg = message
	return f.err
}

type fakeResyncer struct{ called bool }

func (f *fakeResyncer) Resync(_ context.Context) error {
	f.called = true
	return nil
}

func newTestWorker(t *testing.T, llm LLMClient, script ScriptRunner, commit Committer, resync Resyncer) (*Worker, string) {
	t.Helper()
	staging := t.TempDir()
	rawDir := filepath.Join(staging, "raw_memories")
	if err := os.MkdirAll(rawDir, 0o700); err != nil {
		t.Fatal(err)
	}
	auditPath := filepath.Join(t.TempDir(), auditBasename)
	w := NewWorker(staging, rawDir, t.TempDir(), llm, script, commit, resync, NewAuditLog(auditPath))
	return w, rawDir
}

// --- worker: happy path (write -> commit -> resync -> audit) ---

func TestWorker_WriteCommitsAndResyncs(t *testing.T) {
	script := &fakeScript{res: ScriptResult{Stdout: "write c1 memory/user/c1.md\n", ExitCode: 0}}
	commit := &fakeCommitter{}
	resync := &fakeResyncer{}
	w, rawDir := newTestWorker(t,
		fakeLLM{resp: `{"c1":{"action":"ADD"}}`}, script, commit, resync)
	writeCandidate(t, rawDir, "c1.json", map[string]any{"id": "c1", "summary": "s"})

	rec, err := w.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if !script.called {
		t.Error("script not called")
	}
	if !commit.called {
		t.Error("commit not called after a write result")
	}
	if !resync.called {
		t.Error("resync not called after a successful commit")
	}
	if !rec.Committed || !rec.Resynced {
		t.Errorf("record flags Committed/Resynced = %v/%v, want true/true", rec.Committed, rec.Resynced)
	}
	if len(rec.Decisions) != 1 || rec.Decisions[0].Result == "" {
		t.Errorf("decision result not merged from stdout: %+v", rec.Decisions)
	}

	// Audit persisted and readable newest-first.
	recs, err := w.Audit.Read(0)
	if err != nil {
		t.Fatalf("audit read: %v", err)
	}
	if len(recs) != 1 || !recs[0].Committed {
		t.Errorf("audit not persisted: %+v", recs)
	}
}

// --- worker: SKIP-only does not commit ---

func TestWorker_AllSkipNoCommit(t *testing.T) {
	script := &fakeScript{res: ScriptResult{Stdout: "skip c1 decision_skip:redundant\n", ExitCode: 0}}
	commit := &fakeCommitter{}
	w, rawDir := newTestWorker(t,
		fakeLLM{resp: `{"c1":{"action":"SKIP"}}`}, script, commit, &fakeResyncer{})
	writeCandidate(t, rawDir, "c1.json", map[string]any{"id": "c1", "summary": "s"})

	rec, err := w.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if commit.called {
		t.Error("commit should NOT be called when nothing was written")
	}
	if rec.Committed {
		t.Error("record should not be marked committed")
	}
}

// --- worker: non-JSON LLM output -> skip cycle + audit, no panic, no exec ---

func TestWorker_NonJSONLLMOutputSkipsCycle(t *testing.T) {
	script := &fakeScript{}
	commit := &fakeCommitter{}
	w, rawDir := newTestWorker(t,
		fakeLLM{resp: "I cannot do that."}, script, commit, &fakeResyncer{})
	writeCandidate(t, rawDir, "c1.json", map[string]any{"id": "c1", "summary": "s"})

	rec, err := w.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce should not error on bad LLM output: %v", err)
	}
	if script.called {
		t.Error("script must NOT run when decisions cannot be parsed")
	}
	if commit.called {
		t.Error("commit must NOT run when decisions cannot be parsed")
	}
	if rec.Error == "" {
		t.Error("record should carry the parse error")
	}
	recs, _ := w.Audit.Read(0)
	if len(recs) != 1 {
		t.Errorf("the skipped cycle should still be audited, got %d records", len(recs))
	}
}

// --- worker: script non-zero exit -> recorded, not fatal (spec verify ③) ---

func TestWorker_ScriptNonZeroExitRecorded(t *testing.T) {
	script := &fakeScript{res: ScriptResult{
		Stdout:   "skip c1 anti_self_poisoning:negative_tool_claim\n",
		Stderr:   "assist_consolidate failed: redact gate rejected candidate c2\n",
		ExitCode: 1,
	}}
	commit := &fakeCommitter{}
	w, rawDir := newTestWorker(t,
		fakeLLM{resp: `{"c1":{"action":"ADD"}}`}, script, commit, &fakeResyncer{})
	writeCandidate(t, rawDir, "c1.json", map[string]any{"id": "c1", "summary": "s"})

	rec, err := w.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce should not error on script non-zero exit: %v", err)
	}
	if rec.ScriptExitCode != 1 {
		t.Errorf("ScriptExitCode = %d, want 1", rec.ScriptExitCode)
	}
	if len(rec.ScriptErrors) == 0 {
		t.Error("script stderr should be captured in ScriptErrors")
	}
	if commit.called {
		t.Error("a rejected (skip-only) cycle must not commit")
	}
}

// --- worker: script spawn error -> recorded, not fatal ---

func TestWorker_ScriptSpawnErrorRecorded(t *testing.T) {
	script := &fakeScript{err: errors.New("exec: python3 not found")}
	w, rawDir := newTestWorker(t,
		fakeLLM{resp: `{"c1":{"action":"ADD"}}`}, script, &fakeCommitter{}, &fakeResyncer{})
	writeCandidate(t, rawDir, "c1.json", map[string]any{"id": "c1", "summary": "s"})

	rec, err := w.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce should swallow spawn errors: %v", err)
	}
	if rec.Error == "" {
		t.Error("spawn error should be recorded")
	}
}

// --- worker: no candidates -> clean skip, no LLM call ---

func TestWorker_NoCandidates(t *testing.T) {
	calledLLM := false
	w, _ := newTestWorker(t,
		llmFunc(func() (string, error) { calledLLM = true; return "", nil }),
		&fakeScript{}, &fakeCommitter{}, &fakeResyncer{})

	rec, err := w.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if calledLLM {
		t.Error("LLM must not be called when there are no candidates")
	}
	if !rec.Skipped {
		t.Error("empty cycle should be marked skipped")
	}
}

type llmFunc func() (string, error)

func (f llmFunc) ChatJSON(_ context.Context, _, _ string) (string, error) { return f() }
