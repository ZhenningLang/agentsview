// Package vault syncs dev-workflow run records (the dev-long-run and
// dev-complete `.long-loop/<slug>/` workspaces) into the vault dimension
// tables. Like the skills and memory syncers it is reference-data plumbing
// that runs entirely independently of the session sync engine: it never
// reads or writes sessions, messages, or tool_calls, so it cannot pollute
// the core fact domain or its stats triggers. It is strictly read-only
// against the emitted SSOT and uses full-replace semantics on each sync.
//
// The emitted schema is owned by the dotfiles repo; see
// docs/agentsview-memory-vault-emitted-schema.md. This syncer follows that
// contract's tolerance rules: missing optional files (phases, metrics,
// acceptance, stuck) mark a run incomplete rather than failing it, a
// malformed file is recorded by source path and skipped without aborting
// the rest, and a single bad workspace never blanks the whole mirror.
package vault

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"go.kenn.io/agentsview/internal/db"
)

// Writer is the narrow persistence surface the syncer needs. It is
// satisfied by *db.DB; the PG/DuckDB read stores receive vault rows via the
// SQLite mirror, not through this path.
type Writer interface {
	ReplaceVaultRuns(ctx context.Context, runs []db.VaultRun) error
}

// longLoopDir is the per-root subdirectory that holds run workspaces.
const longLoopDir = ".long-loop"

// defaultSkill is assigned when state.json omits the skill field. Per the
// emitted contract a run with no skill field is a dev-long-run.
const defaultSkill = "dev-long-run"

// Syncer scans configured roots for `.long-loop/<slug>/` workspaces and
// persists the parsed runs.
type Syncer struct {
	roots  []string
	writer Writer
	now    func() time.Time
}

// NewSyncer builds a Syncer scanning the given roots. Each root is probed
// for a `.long-loop` directory; roots without one simply yield no runs.
func NewSyncer(roots []string, w Writer) *Syncer {
	return &Syncer{
		roots:  roots,
		writer: w,
		now:    time.Now,
	}
}

// stateFile mirrors the fields of state.json this dimension tracks.
// dev-long-run and dev-complete share slug/state/branch/goal/repo_root and
// the workspace path (worktree_path for dev-long-run, workspace for
// dev-complete). dev-complete carries an explicit skill field; dev-long-run
// historically omits it, so a blank skill defaults to dev-long-run.
type stateFile struct {
	Skill        string `json:"skill"`
	State        string `json:"state"`
	Branch       string `json:"branch"`
	Goal         string `json:"goal"`
	RepoRoot     string `json:"repo_root"`
	Slug         string `json:"slug"`
	WorktreePath string `json:"worktree_path"`
	Workspace    string `json:"workspace"`
}

// verifyFile is the shape of verify.json and acceptance.json.
type verifyFile struct {
	OK   bool `json:"ok"`
	Exit int  `json:"exit"`
}

// stuckFile is the shape of phases/<id>/stuck.json.
type stuckFile struct {
	ConsecutiveFail int     `json:"consecutive_fail"`
	Fingerprint     *string `json:"fingerprint"`
}

// metricRecord is one metrics.jsonl row. Header rows carry _schema and are
// skipped. Event rows carry ts/event plus optional phase/ok/exit/fingerprint
// depending on the event kind, so the carrying fields are pointers to
// distinguish "absent" from a zero value.
type metricRecord struct {
	Schema      string  `json:"_schema"`
	TS          string  `json:"ts"`
	Event       string  `json:"event"`
	Phase       string  `json:"phase"`
	OK          *bool   `json:"ok"`
	Exit        *int    `json:"exit"`
	Fingerprint *string `json:"fingerprint"`
}

// Sync scans every configured root for `.long-loop/<slug>/` workspaces,
// parses each one, and full-replaces the vault tables. It is fail-soft per
// workspace: a single unreadable or malformed workspace is skipped rather
// than aborting the whole run, so one bad run never blanks the mirror.
func (s *Syncer) Sync(ctx context.Context) error {
	syncedAt := s.now().UTC().Format("2006-01-02T15:04:05.000Z")

	runs := make([]db.VaultRun, 0, 32)
	seen := map[string]bool{} // slug dedupe across roots; first root wins
	for _, root := range s.roots {
		loopDir := filepath.Join(root, longLoopDir)
		entries, err := os.ReadDir(loopDir)
		if err != nil {
			// No .long-loop under this root (or unreadable): fail-soft,
			// just contributes no runs.
			continue
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			workspace := filepath.Join(loopDir, e.Name())
			run, ok := s.parseWorkspace(workspace, e.Name())
			if !ok {
				// Workspace had no usable state.json; skip fail-soft.
				continue
			}
			if seen[run.Slug] {
				continue
			}
			seen[run.Slug] = true
			run.SyncedAt = syncedAt
			runs = append(runs, run)
		}
	}
	// Stable, newest-first ordering by slug (slugs are date-prefixed) so the
	// mirror is deterministic regardless of directory iteration order.
	sort.Slice(runs, func(i, j int) bool {
		return runs[i].Slug > runs[j].Slug
	})

	return s.writer.ReplaceVaultRuns(ctx, runs)
}

// parseWorkspace reads one `.long-loop/<dir>/` workspace into a VaultRun.
// The dir name (e.g. 20260623_memory-vault) is the fallback slug when
// state.json is missing or omits its own slug. Returns ok=false only when
// there is no usable state.json at all, since that is the minimum a run
// needs to be meaningful. Optional files (acceptance, phases, metrics) are
// tolerated by their parsers.
func (s *Syncer) parseWorkspace(
	workspace, dirName string,
) (db.VaultRun, bool) {
	st, err := readState(filepath.Join(workspace, "state.json"))
	if err != nil {
		return db.VaultRun{}, false
	}

	skill := strings.TrimSpace(st.Skill)
	if skill == "" {
		skill = defaultSkill
	}
	slug := strings.TrimSpace(st.Slug)
	if slug == "" {
		slug = dirName
	}
	workspacePath := st.WorktreePath
	if workspacePath == "" {
		workspacePath = st.Workspace
	}

	run := db.VaultRun{
		Slug:          slug,
		Skill:         skill,
		State:         st.State,
		Branch:        st.Branch,
		Goal:          st.Goal,
		RepoRoot:      st.RepoRoot,
		WorkspacePath: workspacePath,
		SourcePath:    workspace,
	}

	// Workspace-level pass/fail snapshot (optional). dev-long-run emits
	// acceptance.json at the workspace root; dev-complete emits a root
	// verify.json instead (it has no phases). Prefer acceptance.json and
	// fall back to the root verify.json so the dev-complete signal is not
	// lost. Missing both = unknown (nil pointers, incomplete-ok).
	if acc, ok := readVerify(filepath.Join(workspace, "acceptance.json")); ok {
		okVal := acc.OK
		exitVal := acc.Exit
		run.AcceptanceOK = &okVal
		run.AcceptanceExit = &exitVal
	} else if v, ok := readVerify(filepath.Join(workspace, "verify.json")); ok {
		okVal := v.OK
		exitVal := v.Exit
		run.AcceptanceOK = &okVal
		run.AcceptanceExit = &exitVal
	}

	// Phases (dev-long-run only; dev-complete has none). Each phase dir may
	// carry verify.json and/or stuck.json. A missing phases/ dir or any
	// missing optional file marks the run incomplete, not failed.
	run.Phases = readPhases(filepath.Join(workspace, "phases"))

	// Metrics (dev-long-run only; append-only JSONL with an optional schema
	// header). dev-complete has no metrics file, which is incomplete-ok.
	run.Metrics = readMetrics(filepath.Join(workspace, "metrics.jsonl"), slug)

	// Stamp the run slug onto children so callers can read it off any row.
	for i := range run.Phases {
		run.Phases[i].RunSlug = slug
	}
	for i := range run.Metrics {
		run.Metrics[i].RunSlug = slug
	}

	return run, true
}

// readState parses state.json. A missing or malformed file is an error so
// the caller can skip the workspace fail-soft.
func readState(path string) (stateFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return stateFile{}, err
	}
	var st stateFile
	if err := json.Unmarshal(data, &st); err != nil {
		return stateFile{}, fmt.Errorf("malformed %s: %w", path, err)
	}
	return st, nil
}

// readVerify parses a verify.json/acceptance.json snapshot. ok=false means
// the file was absent or malformed; the caller treats that as "unknown".
func readVerify(path string) (verifyFile, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return verifyFile{}, false
	}
	var v verifyFile
	if err := json.Unmarshal(data, &v); err != nil {
		// Malformed optional file: record path, skip, do not abort.
		return verifyFile{}, false
	}
	return v, true
}

// readStuck parses a phases/<id>/stuck.json snapshot. ok=false means absent
// or malformed; the caller leaves the stuck columns nil.
func readStuck(path string) (stuckFile, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return stuckFile{}, false
	}
	var s stuckFile
	if err := json.Unmarshal(data, &s); err != nil {
		return stuckFile{}, false
	}
	return s, true
}

// readPhases enumerates phases/<id>/ directories and parses each one's
// optional verify.json and stuck.json. Phases are returned sorted by id so
// the mirror is deterministic. A missing phases/ directory yields nil
// (dev-complete and not-yet-started dev-long-run both hit this path).
func readPhases(phasesDir string) []db.VaultPhase {
	entries, err := os.ReadDir(phasesDir)
	if err != nil {
		return nil
	}
	out := make([]db.VaultPhase, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		phaseDir := filepath.Join(phasesDir, e.Name())
		p := db.VaultPhase{PhaseID: e.Name()}

		if v, ok := readVerify(filepath.Join(phaseDir, "verify.json")); ok {
			okVal := v.OK
			exitVal := v.Exit
			p.VerifyOK = &okVal
			p.VerifyExit = &exitVal
		}
		if st, ok := readStuck(filepath.Join(phaseDir, "stuck.json")); ok {
			failVal := st.ConsecutiveFail
			p.StuckConsecutiveFail = &failVal
			if st.Fingerprint != nil {
				fp := *st.Fingerprint
				p.StuckFingerprint = &fp
			}
		}
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].PhaseID < out[j].PhaseID
	})
	return out
}

// readMetrics parses metrics.jsonl line by line. The optional first-line
// schema header ({"_schema":...}) is skipped; legacy files without a header
// are accepted (their first row is a normal event). Malformed individual
// lines are skipped without aborting the whole file, and a missing file
// yields nil (dev-complete and not-yet-emitted runs hit this path).
func readMetrics(path, slug string) []db.VaultMetric {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	out := make([]db.VaultMetric, 0, 32)
	sc := bufio.NewScanner(f)
	// Allow long output lines; metrics rows are small but be generous.
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var rec metricRecord
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			// Malformed line: skip fail-soft.
			continue
		}
		// Header rows carry _schema and no event; ignore for run stats.
		if rec.Schema != "" || rec.Event == "" {
			continue
		}
		m := db.VaultMetric{
			RunSlug:     slug,
			TS:          rec.TS,
			Event:       rec.Event,
			Phase:       rec.Phase,
			Fingerprint: rec.Fingerprint,
		}
		if rec.OK != nil {
			okVal := *rec.OK
			m.Ok = &okVal
		}
		if rec.Exit != nil {
			exitVal := *rec.Exit
			m.Exit = &exitVal
		}
		out = append(out, m)
	}
	if err := sc.Err(); err != nil {
		// Partial parse on a read error: keep what we got, fail-soft.
		return out
	}
	return out
}
