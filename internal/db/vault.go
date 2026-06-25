package db

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// VaultRun is one dev-workflow run record discovered under a configured
// root as `.long-loop/<slug>/`. Like Memory and Skill it is slowly-changing
// reference data that lives in its own dimension table and is populated by
// the VaultSyncer, never by the session sync path. The store is read-only
// for callers other than the syncer; PG/DuckDB receive the tables for
// schema parity but rows only land in the local SQLite store.
//
// A run is either a dev-long-run (multi-phase, Phases + Metrics populated)
// or a dev-complete (single workspace, no phases/metrics). Skill
// distinguishes them; a run whose state.json omits the skill field is
// treated as "dev-long-run".
type VaultRun struct {
	Slug          string `json:"slug"`
	Skill         string `json:"skill"`
	State         string `json:"state"`
	Branch        string `json:"branch"`
	Goal          string `json:"goal"`
	RepoRoot      string `json:"repo_root"`
	WorkspacePath string `json:"workspace_path"`
	SourcePath    string `json:"source_path"`
	// AcceptanceOK/AcceptanceExit mirror the workspace acceptance.json
	// snapshot. Nil when no acceptance.json was present (incomplete run).
	AcceptanceOK   *bool `json:"acceptance_ok,omitempty"`
	AcceptanceExit *int  `json:"acceptance_exit,omitempty"`
	SyncedAt       string `json:"synced_at"`
	// Phases and Metrics are populated by GetVaultRun only; ListVaultRuns
	// returns the run header without children.
	Phases  []VaultPhase  `json:"phases,omitempty"`
	Metrics []VaultMetric `json:"metrics,omitempty"`
}

// VaultPhase is one phase directory of a dev-long-run. VerifyOK/VerifyExit
// come from phases/<id>/verify.json; StuckConsecutiveFail/StuckFingerprint
// from phases/<id>/stuck.json. Pointer fields are nil when the
// corresponding optional file was absent.
type VaultPhase struct {
	RunSlug              string  `json:"run_slug"`
	PhaseID             string  `json:"phase_id"`
	VerifyOK            *bool   `json:"verify_ok,omitempty"`
	VerifyExit          *int    `json:"verify_exit,omitempty"`
	StuckConsecutiveFail *int    `json:"stuck_consecutive_fail,omitempty"`
	StuckFingerprint    *string `json:"stuck_fingerprint,omitempty"`
}

// VaultMetric is one event record from metrics.jsonl (header rows excluded).
// dev-complete runs have no metrics so produce no rows. Ok/Exit/Fingerprint
// are nil for events that do not carry them (e.g. complete_phase).
type VaultMetric struct {
	RunSlug     string  `json:"run_slug"`
	TS          string  `json:"ts"`
	Event       string  `json:"event"`
	Phase       string  `json:"phase,omitempty"`
	Ok          *bool   `json:"ok,omitempty"`
	Exit        *int    `json:"exit,omitempty"`
	Fingerprint *string `json:"fingerprint,omitempty"`
}

// VaultFilter narrows a vault run listing. Empty fields = no filter.
type VaultFilter struct {
	Skill string
}

const vaultRunCols = `slug, skill, state, branch, goal, repo_root,
	workspace_path, source_path, acceptance_ok, acceptance_exit, synced_at`

func scanVaultRun(rows *sql.Rows) (VaultRun, error) {
	var r VaultRun
	var accOK sql.NullBool
	var accExit sql.NullInt64
	if err := rows.Scan(
		&r.Slug, &r.Skill, &r.State, &r.Branch, &r.Goal, &r.RepoRoot,
		&r.WorkspacePath, &r.SourcePath, &accOK, &accExit, &r.SyncedAt,
	); err != nil {
		return VaultRun{}, err
	}
	if accOK.Valid {
		v := accOK.Bool
		r.AcceptanceOK = &v
	}
	if accExit.Valid {
		v := int(accExit.Int64)
		r.AcceptanceExit = &v
	}
	return r, nil
}

// ListVaultRuns returns vault run headers (no phases/metrics), optionally
// filtered by skill, ordered by slug descending for stable, newest-first
// display (slugs are date-prefixed).
func (db *DB) ListVaultRuns(
	ctx context.Context, f VaultFilter,
) ([]VaultRun, error) {
	var preds []string
	var args []any
	if f.Skill != "" {
		preds = append(preds, "skill = ?")
		args = append(args, f.Skill)
	}
	where := ""
	if len(preds) > 0 {
		where = " WHERE " + strings.Join(preds, " AND ")
	}
	cols := strings.ReplaceAll(vaultRunCols, "\n\t", " ")
	q := "SELECT " + cols + " FROM vault_run" + where +
		" ORDER BY slug DESC"
	rows, err := db.getReader().QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("listing vault runs: %w", err)
	}
	defer rows.Close()
	out := make([]VaultRun, 0, 32)
	for rows.Next() {
		r, err := scanVaultRun(rows)
		if err != nil {
			return nil, fmt.Errorf("scan vault run: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// GetVaultRun returns a single run by slug with its phases and metrics
// attached, or nil if not found.
func (db *DB) GetVaultRun(
	ctx context.Context, slug string,
) (*VaultRun, error) {
	cols := strings.ReplaceAll(vaultRunCols, "\n\t", " ")
	q := "SELECT " + cols + " FROM vault_run WHERE slug = ?"
	rows, err := db.getReader().QueryContext(ctx, q, slug)
	if err != nil {
		return nil, fmt.Errorf("getting vault run: %w", err)
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, rows.Err()
	}
	r, err := scanVaultRun(rows)
	if err != nil {
		return nil, fmt.Errorf("scan vault run: %w", err)
	}
	if cerr := rows.Close(); cerr != nil {
		return nil, cerr
	}
	phases, err := db.listVaultPhases(ctx, slug)
	if err != nil {
		return nil, err
	}
	r.Phases = phases
	metrics, err := db.listVaultMetrics(ctx, slug)
	if err != nil {
		return nil, err
	}
	r.Metrics = metrics
	return &r, nil
}

func (db *DB) listVaultPhases(
	ctx context.Context, slug string,
) ([]VaultPhase, error) {
	q := `SELECT run_slug, phase_id, verify_ok, verify_exit,
		stuck_consecutive_fail, stuck_fingerprint
		FROM vault_phase WHERE run_slug = ? ORDER BY phase_id`
	rows, err := db.getReader().QueryContext(ctx, q, slug)
	if err != nil {
		return nil, fmt.Errorf("listing vault phases: %w", err)
	}
	defer rows.Close()
	out := make([]VaultPhase, 0, 16)
	for rows.Next() {
		p, err := scanVaultPhase(rows)
		if err != nil {
			return nil, fmt.Errorf("scan vault phase: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func scanVaultPhase(rows *sql.Rows) (VaultPhase, error) {
	var p VaultPhase
	var vOK sql.NullBool
	var vExit sql.NullInt64
	var sFail sql.NullInt64
	var sFp sql.NullString
	if err := rows.Scan(
		&p.RunSlug, &p.PhaseID, &vOK, &vExit, &sFail, &sFp,
	); err != nil {
		return VaultPhase{}, err
	}
	if vOK.Valid {
		v := vOK.Bool
		p.VerifyOK = &v
	}
	if vExit.Valid {
		v := int(vExit.Int64)
		p.VerifyExit = &v
	}
	if sFail.Valid {
		v := int(sFail.Int64)
		p.StuckConsecutiveFail = &v
	}
	if sFp.Valid {
		v := sFp.String
		p.StuckFingerprint = &v
	}
	return p, nil
}

func (db *DB) listVaultMetrics(
	ctx context.Context, slug string,
) ([]VaultMetric, error) {
	q := `SELECT run_slug, ts, event, phase, ok, exit, fingerprint
		FROM vault_metric WHERE run_slug = ? ORDER BY id`
	rows, err := db.getReader().QueryContext(ctx, q, slug)
	if err != nil {
		return nil, fmt.Errorf("listing vault metrics: %w", err)
	}
	defer rows.Close()
	out := make([]VaultMetric, 0, 32)
	for rows.Next() {
		m, err := scanVaultMetric(rows)
		if err != nil {
			return nil, fmt.Errorf("scan vault metric: %w", err)
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func scanVaultMetric(rows *sql.Rows) (VaultMetric, error) {
	var m VaultMetric
	var ok sql.NullBool
	var exit sql.NullInt64
	var fp sql.NullString
	if err := rows.Scan(
		&m.RunSlug, &m.TS, &m.Event, &m.Phase, &ok, &exit, &fp,
	); err != nil {
		return VaultMetric{}, err
	}
	if ok.Valid {
		v := ok.Bool
		m.Ok = &v
	}
	if exit.Valid {
		v := int(exit.Int64)
		m.Exit = &v
	}
	if fp.Valid {
		v := fp.String
		m.Fingerprint = &v
	}
	return m, nil
}

// replaceVaultRunsTx full-replaces all three vault tables inside an open
// tx. Phases and metrics are deleted via the ON DELETE CASCADE on vault_run
// plus explicit DELETEs (cascade is not enforced unless foreign_keys is on,
// so we clear children explicitly for correctness across PRAGMA settings).
func replaceVaultRunsTx(
	ctx context.Context, tx txExec, runs []VaultRun,
) error {
	for _, t := range []string{
		"vault_metric", "vault_phase", "vault_run",
	} {
		if _, err := tx.ExecContext(ctx, "DELETE FROM "+t); err != nil {
			return fmt.Errorf("clearing %s: %w", t, err)
		}
	}
	cols := strings.ReplaceAll(vaultRunCols, "\n\t", " ")
	runStmt, err := tx.PrepareContext(ctx, `INSERT INTO vault_run (`+cols+`)
		VALUES (?,?,?,?,?,?,?,?,?,?,?)`)
	if err != nil {
		return fmt.Errorf("prepare vault_run insert: %w", err)
	}
	defer runStmt.Close()
	phaseStmt, err := tx.PrepareContext(ctx, `INSERT INTO vault_phase
		(run_slug, phase_id, verify_ok, verify_exit,
		 stuck_consecutive_fail, stuck_fingerprint)
		VALUES (?,?,?,?,?,?)`)
	if err != nil {
		return fmt.Errorf("prepare vault_phase insert: %w", err)
	}
	defer phaseStmt.Close()
	metricStmt, err := tx.PrepareContext(ctx, `INSERT INTO vault_metric
		(run_slug, ts, event, phase, ok, exit, fingerprint)
		VALUES (?,?,?,?,?,?,?)`)
	if err != nil {
		return fmt.Errorf("prepare vault_metric insert: %w", err)
	}
	defer metricStmt.Close()

	for _, r := range runs {
		if _, err := runStmt.ExecContext(ctx,
			r.Slug, r.Skill, r.State, r.Branch, r.Goal, r.RepoRoot,
			r.WorkspacePath, r.SourcePath,
			boolPtrArg(r.AcceptanceOK), intPtrArg(r.AcceptanceExit),
			r.SyncedAt,
		); err != nil {
			return fmt.Errorf("insert vault_run %q: %w", r.Slug, err)
		}
		for _, p := range r.Phases {
			if _, err := phaseStmt.ExecContext(ctx,
				r.Slug, p.PhaseID,
				boolPtrArg(p.VerifyOK), intPtrArg(p.VerifyExit),
				intPtrArg(p.StuckConsecutiveFail),
				strPtrArg(p.StuckFingerprint),
			); err != nil {
				return fmt.Errorf(
					"insert vault_phase %q/%q: %w",
					r.Slug, p.PhaseID, err)
			}
		}
		for _, m := range r.Metrics {
			if _, err := metricStmt.ExecContext(ctx,
				r.Slug, m.TS, m.Event, m.Phase,
				boolPtrArg(m.Ok), intPtrArg(m.Exit),
				strPtrArg(m.Fingerprint),
			); err != nil {
				return fmt.Errorf(
					"insert vault_metric %q/%s: %w",
					r.Slug, m.Event, err)
			}
		}
	}
	return nil
}

// ReplaceVaultRuns atomically full-replaces all three vault tables in a
// single transaction. The VaultSyncer uses this so a crash mid-write can
// never leave a partial mirror. Local-only writer (PG/DuckDB receive the
// tables for schema parity, not rows through this path).
func (db *DB) ReplaceVaultRuns(
	ctx context.Context, runs []VaultRun,
) error {
	db.mu.Lock()
	defer db.mu.Unlock()
	tx, err := db.getWriter().BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin vault tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := replaceVaultRunsTx(ctx, tx, runs); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit vault: %w", err)
	}
	return nil
}

// boolPtrArg / intPtrArg / strPtrArg turn nil pointers into SQL NULL and
// otherwise into the pointed-to value, so optional emitted fields round-trip
// as NULL instead of zero-valued rows.
func boolPtrArg(b *bool) any {
	if b == nil {
		return nil
	}
	return *b
}

func intPtrArg(i *int) any {
	if i == nil {
		return nil
	}
	return *i
}

func strPtrArg(s *string) any {
	if s == nil {
		return nil
	}
	return *s
}
