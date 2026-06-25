package vault

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
	runs []db.VaultRun
}

func (w *fakeWriter) ReplaceVaultRuns(
	_ context.Context, runs []db.VaultRun,
) error {
	w.runs = runs
	return nil
}

// writeFile writes content to <dir>/<rel>, creating parent dirs.
func writeFile(t *testing.T, dir, rel, content string) {
	t.Helper()
	full := filepath.Join(dir, rel)
	require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
	require.NoError(t, os.WriteFile(full, []byte(content), 0o644))
}

// bySlug indexes a run slice by slug for assertion lookups.
func bySlug(runs []db.VaultRun) map[string]db.VaultRun {
	m := map[string]db.VaultRun{}
	for _, r := range runs {
		m[r.Slug] = r
	}
	return m
}

// phaseByID indexes a run's phases by phase id.
func phaseByID(ps []db.VaultPhase) map[string]db.VaultPhase {
	m := map[string]db.VaultPhase{}
	for _, p := range ps {
		m[p.PhaseID] = p
	}
	return m
}

// TestSyncDevLongRunMultiPhase covers a full dev-long-run workspace: a
// state.json with no skill field (defaults to dev-long-run), multiple phase
// directories with verify.json + stuck.json, and a metrics.jsonl with a
// schema header that must be skipped plus event rows of every kind.
func TestSyncDevLongRunMultiPhase(t *testing.T) {
	root := t.TempDir()
	ws := filepath.Join(root, ".long-loop", "20260623_memory-vault")

	// state.json omits skill -> defaults to dev-long-run; uses worktree_path.
	writeFile(t, ws, "state.json", `{
		"state": "completed",
		"phase": "08",
		"role_in_flight": "agent_orchestrator",
		"worktree_path": "/tmp/wt",
		"branch": "lr/memory-vault",
		"goal": "cross-agent memory and vault mechanism",
		"repo_root": "/tmp/repo",
		"slug": "memory-vault",
		"dirty_main_at_start": false,
		"in_place": false
	}`)
	writeFile(t, ws, "acceptance.json",
		`{"ok": true, "exit": 0, "output_tail": "ALL PASS"}`)

	// Two phases. Phase 01 passed with no stuck; phase 02 failed and is stuck.
	writeFile(t, ws, "phases/01_spike-gate/verify.json",
		`{"ok": true, "exit": 0, "output_tail": "ok"}`)
	writeFile(t, ws, "phases/01_spike-gate/stuck.json",
		`{"consecutive_fail": 0, "fingerprint": null}`)
	writeFile(t, ws, "phases/02_storage/verify.json",
		`{"ok": false, "exit": 1, "output_tail": "boom"}`)
	writeFile(t, ws, "phases/02_storage/stuck.json",
		`{"consecutive_fail": 2, "fingerprint": "abc123"}`)

	// metrics.jsonl: schema header (skipped) + one of each event kind.
	writeFile(t, ws, "metrics.jsonl",
		`{"_schema":"dotfiles.long_loop.metrics","_version":1}`+"\n"+
			`{"ts":"2026-06-23T23:23:18Z","event":"verify","phase":"01","ok":true,"exit":0,"fail_streak":0,"fingerprint":null}`+"\n"+
			`{"ts":"2026-06-23T23:23:40Z","event":"complete_phase","phase":"01"}`+"\n"+
			`{"ts":"2026-06-24T08:46:49Z","event":"acceptance","ok":false,"exit":1}`+"\n"+
			`{"ts":"2026-06-24T08:50:28Z","event":"complete_run"}`+"\n")

	w := &fakeWriter{}
	s := NewSyncer([]string{root}, w)
	require.NoError(t, s.Sync(context.Background()))

	require.Len(t, w.runs, 1)
	run := w.runs[0]
	assert.Equal(t, "memory-vault", run.Slug)
	assert.Equal(t, "dev-long-run", run.Skill, "blank skill defaults")
	assert.Equal(t, "completed", run.State)
	assert.Equal(t, "lr/memory-vault", run.Branch)
	assert.Equal(t, "/tmp/wt", run.WorkspacePath, "worktree_path used")
	assert.Equal(t, "/tmp/repo", run.RepoRoot)
	assert.Equal(t, ws, run.SourcePath)
	require.NotNil(t, run.AcceptanceOK)
	assert.True(t, *run.AcceptanceOK)
	require.NotNil(t, run.AcceptanceExit)
	assert.Equal(t, 0, *run.AcceptanceExit)

	require.Len(t, run.Phases, 2)
	ph := phaseByID(run.Phases)

	p1 := ph["01_spike-gate"]
	assert.Equal(t, "memory-vault", p1.RunSlug)
	require.NotNil(t, p1.VerifyOK)
	assert.True(t, *p1.VerifyOK)
	require.NotNil(t, p1.VerifyExit)
	assert.Equal(t, 0, *p1.VerifyExit)
	require.NotNil(t, p1.StuckConsecutiveFail)
	assert.Equal(t, 0, *p1.StuckConsecutiveFail)
	assert.Nil(t, p1.StuckFingerprint, "null fingerprint stays nil")

	p2 := ph["02_storage"]
	require.NotNil(t, p2.VerifyOK)
	assert.False(t, *p2.VerifyOK)
	require.NotNil(t, p2.VerifyExit)
	assert.Equal(t, 1, *p2.VerifyExit)
	require.NotNil(t, p2.StuckConsecutiveFail)
	assert.Equal(t, 2, *p2.StuckConsecutiveFail)
	require.NotNil(t, p2.StuckFingerprint)
	assert.Equal(t, "abc123", *p2.StuckFingerprint)

	// Header skipped; 4 event rows captured.
	require.Len(t, run.Metrics, 4)
	assert.Equal(t, "verify", run.Metrics[0].Event)
	assert.Equal(t, "01", run.Metrics[0].Phase)
	require.NotNil(t, run.Metrics[0].Ok)
	assert.True(t, *run.Metrics[0].Ok)
	assert.Equal(t, "complete_phase", run.Metrics[1].Event)
	assert.Nil(t, run.Metrics[1].Ok, "complete_phase has no ok")
	assert.Equal(t, "acceptance", run.Metrics[2].Event)
	require.NotNil(t, run.Metrics[2].Ok)
	assert.False(t, *run.Metrics[2].Ok)
	assert.Equal(t, "complete_run", run.Metrics[3].Event)
	for _, m := range run.Metrics {
		assert.Equal(t, "memory-vault", m.RunSlug)
	}
}

// TestSyncDevCompleteTolerance covers a dev-complete workspace, which has a
// distinct state.json (explicit skill, workspace field instead of
// worktree_path, no phase/role_in_flight) and crucially NO phases/ directory
// and NO metrics.jsonl. Missing optional files must mark it incomplete, not
// fail it. dev-complete emits a root verify.json used as the pass/fail snap.
func TestSyncDevCompleteTolerance(t *testing.T) {
	root := t.TempDir()
	ws := filepath.Join(root, ".long-loop", "20260625_quickfix")

	writeFile(t, ws, "state.json", `{
		"skill": "dev-complete",
		"state": "completed",
		"slug": "quickfix",
		"repo_root": "/tmp/repo",
		"worktree_path": "/tmp/wt-dc",
		"branch": "feat/quickfix",
		"workspace": "/tmp/ws-dc",
		"created_at": "2026-06-25T00:00:00Z"
	}`)
	// No phases/, no metrics.jsonl, no acceptance.json -> root verify.json.
	writeFile(t, ws, "verify.json",
		`{"ok": true, "exit": 0, "output_tail": "all green"}`)

	w := &fakeWriter{}
	s := NewSyncer([]string{root}, w)
	require.NoError(t, s.Sync(context.Background()))

	require.Len(t, w.runs, 1)
	run := w.runs[0]
	assert.Equal(t, "quickfix", run.Slug)
	assert.Equal(t, "dev-complete", run.Skill, "explicit skill preserved")
	assert.Equal(t, "feat/quickfix", run.Branch)
	// worktree_path takes precedence; workspace is the fallback only.
	assert.Equal(t, "/tmp/wt-dc", run.WorkspacePath)
	// Incomplete-ok: no phases and no metrics, not a parse failure.
	assert.Empty(t, run.Phases)
	assert.Empty(t, run.Metrics)
	// Root verify.json fills the pass/fail snapshot.
	require.NotNil(t, run.AcceptanceOK)
	assert.True(t, *run.AcceptanceOK)
	require.NotNil(t, run.AcceptanceExit)
	assert.Equal(t, 0, *run.AcceptanceExit)
}

// TestSyncWorkspaceFieldFallback verifies dev-complete that omits
// worktree_path falls back to the workspace field.
func TestSyncWorkspaceFieldFallback(t *testing.T) {
	root := t.TempDir()
	ws := filepath.Join(root, ".long-loop", "20260625_wsonly")
	writeFile(t, ws, "state.json", `{
		"skill": "dev-complete",
		"state": "running",
		"slug": "wsonly",
		"workspace": "/tmp/only-ws"
	}`)

	w := &fakeWriter{}
	require.NoError(t, NewSyncer([]string{root}, w).Sync(context.Background()))
	require.Len(t, w.runs, 1)
	assert.Equal(t, "/tmp/only-ws", w.runs[0].WorkspacePath)
}

// TestSyncLegacyMetricsNoHeader covers a legacy metrics.jsonl whose first
// line is already an event row (no schema header). All rows must parse.
func TestSyncLegacyMetricsNoHeader(t *testing.T) {
	root := t.TempDir()
	ws := filepath.Join(root, ".long-loop", "20260601_legacy")
	writeFile(t, ws, "state.json",
		`{"state":"completed","slug":"legacy","branch":"b"}`)
	writeFile(t, ws, "metrics.jsonl",
		`{"ts":"2026-06-01T00:00:00Z","event":"verify","phase":"01","ok":true,"exit":0}`+"\n"+
			`{"ts":"2026-06-01T00:01:00Z","event":"complete_run"}`+"\n")

	w := &fakeWriter{}
	require.NoError(t, NewSyncer([]string{root}, w).Sync(context.Background()))
	require.Len(t, w.runs, 1)
	assert.Equal(t, "dev-long-run", w.runs[0].Skill, "no skill -> long-run")
	require.Len(t, w.runs[0].Metrics, 2, "no header line to skip")
	assert.Equal(t, "verify", w.runs[0].Metrics[0].Event)
}

// TestSyncMalformedFilesAreToleratedPerSource verifies that malformed
// optional files (acceptance, a phase verify, a metrics line) are skipped by
// source path without aborting the workspace, and a malformed state.json
// skips only that one workspace while others sync.
func TestSyncMalformedFilesAreToleratedPerSource(t *testing.T) {
	root := t.TempDir()

	// good workspace with one corrupt optional file each.
	good := filepath.Join(root, ".long-loop", "20260620_good")
	writeFile(t, good, "state.json",
		`{"state":"completed","slug":"good"}`)
	writeFile(t, good, "acceptance.json", `{not json`)
	writeFile(t, good, "phases/01_a/verify.json", `{"ok":true,"exit":0}`)
	writeFile(t, good, "phases/01_a/stuck.json", `garbage`)
	writeFile(t, good, "metrics.jsonl",
		`{"ts":"t","event":"verify","ok":true,"exit":0}`+"\n"+
			`{this line is broken`+"\n"+
			`{"ts":"t2","event":"complete_run"}`+"\n")

	// bad workspace: malformed state.json -> skipped entirely.
	bad := filepath.Join(root, ".long-loop", "20260619_bad")
	writeFile(t, bad, "state.json", `{ broken`)

	w := &fakeWriter{}
	require.NoError(t, NewSyncer([]string{root}, w).Sync(context.Background()))

	require.Len(t, w.runs, 1, "bad workspace skipped, good one kept")
	run := bySlug(w.runs)["good"]
	require.Equal(t, "good", run.Slug)
	// Corrupt acceptance -> unknown.
	assert.Nil(t, run.AcceptanceOK)
	// Phase verify good, stuck corrupt -> verify present, stuck nil.
	require.Len(t, run.Phases, 1)
	require.NotNil(t, run.Phases[0].VerifyOK)
	assert.True(t, *run.Phases[0].VerifyOK)
	assert.Nil(t, run.Phases[0].StuckConsecutiveFail, "corrupt stuck dropped")
	// Broken metric line dropped, the two valid ones kept.
	require.Len(t, run.Metrics, 2)
	assert.Equal(t, "verify", run.Metrics[0].Event)
	assert.Equal(t, "complete_run", run.Metrics[1].Event)
}

// TestSyncSlugFallbackToDirName verifies a state.json with no slug field
// falls back to the workspace directory name.
func TestSyncSlugFallbackToDirName(t *testing.T) {
	root := t.TempDir()
	ws := filepath.Join(root, ".long-loop", "20260624_noslug")
	writeFile(t, ws, "state.json", `{"state":"running"}`)

	w := &fakeWriter{}
	require.NoError(t, NewSyncer([]string{root}, w).Sync(context.Background()))
	require.Len(t, w.runs, 1)
	assert.Equal(t, "20260624_noslug", w.runs[0].Slug)
}

// TestSyncMissingLongLoopDirIsFailSoft verifies roots without a .long-loop
// directory contribute no runs and do not error.
func TestSyncMissingLongLoopDirIsFailSoft(t *testing.T) {
	w := &fakeWriter{}
	s := NewSyncer([]string{t.TempDir()}, w)
	require.NoError(t, s.Sync(context.Background()))
	assert.Empty(t, w.runs)
}

// TestSyncMultipleRootsDedupeFirstWins verifies multiple roots are scanned
// and the same slug discovered in two roots is deduped (first root wins).
func TestSyncMultipleRootsDedupeFirstWins(t *testing.T) {
	root1 := t.TempDir()
	root2 := t.TempDir()
	ws1 := filepath.Join(root1, ".long-loop", "20260624_dup")
	ws2 := filepath.Join(root2, ".long-loop", "20260624_dup")
	writeFile(t, ws1, "state.json",
		`{"state":"completed","slug":"dup","branch":"first"}`)
	writeFile(t, ws2, "state.json",
		`{"state":"completed","slug":"dup","branch":"second"}`)
	// A distinct run in root2 to confirm both roots are scanned.
	writeFile(t, filepath.Join(root2, ".long-loop", "20260624_other"),
		"state.json", `{"state":"running","slug":"other"}`)

	w := &fakeWriter{}
	require.NoError(t,
		NewSyncer([]string{root1, root2}, w).Sync(context.Background()))

	by := bySlug(w.runs)
	require.Len(t, w.runs, 2)
	require.Contains(t, by, "dup")
	require.Contains(t, by, "other")
	assert.Equal(t, "first", by["dup"].Branch, "first root wins dedupe")
}

// TestSyncOrderingNewestFirst verifies runs are emitted slug-descending so
// the date-prefixed slugs come back newest-first deterministically.
func TestSyncOrderingNewestFirst(t *testing.T) {
	root := t.TempDir()
	for _, slug := range []string{
		"20260601_a", "20260625_c", "20260610_b",
	} {
		writeFile(t, filepath.Join(root, ".long-loop", slug),
			"state.json", `{"state":"x","slug":"`+slug+`"}`)
	}
	w := &fakeWriter{}
	require.NoError(t, NewSyncer([]string{root}, w).Sync(context.Background()))
	require.Len(t, w.runs, 3)
	assert.Equal(t, "20260625_c", w.runs[0].Slug)
	assert.Equal(t, "20260610_b", w.runs[1].Slug)
	assert.Equal(t, "20260601_a", w.runs[2].Slug)
}
