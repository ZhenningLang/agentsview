package duckdb

import (
	"context"
	"database/sql"
	"fmt"

	"go.kenn.io/agentsview/internal/db"
)

// Vault read methods mirror the SQLite implementations in
// internal/db/vault.go. Vault rows reach the local SQLite store only (these
// tables exist in the DuckDB mirror for schema parity), so there are no
// writer methods. Reads return whatever rows are present, which is empty in
// pure DuckDB serve mode.

const duckVaultRunCols = `slug, skill, state, branch, goal, repo_root,
	workspace_path, source_path, acceptance_ok, acceptance_exit, synced_at`

func (s *Store) ListVaultRuns(
	ctx context.Context, f db.VaultFilter,
) ([]db.VaultRun, error) {
	var args []any
	where := ""
	if f.Skill != "" {
		where = " WHERE skill = ?"
		args = append(args, f.Skill)
	}
	q := "SELECT " + duckVaultRunCols + " FROM vault_run" + where +
		" ORDER BY slug DESC"
	rows, err := s.duck.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("listing vault runs: %w", err)
	}
	defer rows.Close()
	out := make([]db.VaultRun, 0, 32)
	for rows.Next() {
		r, err := scanDuckVaultRun(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) GetVaultRun(
	ctx context.Context, slug string,
) (*db.VaultRun, error) {
	q := "SELECT " + duckVaultRunCols + " FROM vault_run WHERE slug = ?"
	rows, err := s.duck.QueryContext(ctx, q, slug)
	if err != nil {
		return nil, fmt.Errorf("getting vault run: %w", err)
	}
	r, ok, err := scanDuckVaultRunSingle(rows)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, nil
	}
	phases, err := s.listVaultPhases(ctx, slug)
	if err != nil {
		return nil, err
	}
	r.Phases = phases
	metrics, err := s.listVaultMetrics(ctx, slug)
	if err != nil {
		return nil, err
	}
	r.Metrics = metrics
	return &r, nil
}

func scanDuckVaultRunSingle(rows *sql.Rows) (db.VaultRun, bool, error) {
	defer rows.Close()
	if !rows.Next() {
		return db.VaultRun{}, false, rows.Err()
	}
	r, err := scanDuckVaultRun(rows)
	if err != nil {
		return db.VaultRun{}, false, err
	}
	return r, true, nil
}

func (s *Store) listVaultPhases(
	ctx context.Context, slug string,
) ([]db.VaultPhase, error) {
	q := `SELECT run_slug, phase_id, verify_ok, verify_exit,
		stuck_consecutive_fail, stuck_fingerprint
		FROM vault_phase WHERE run_slug = ? ORDER BY phase_id`
	rows, err := s.duck.QueryContext(ctx, q, slug)
	if err != nil {
		return nil, fmt.Errorf("listing vault phases: %w", err)
	}
	defer rows.Close()
	out := make([]db.VaultPhase, 0, 16)
	for rows.Next() {
		p, err := scanDuckVaultPhase(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Store) listVaultMetrics(
	ctx context.Context, slug string,
) ([]db.VaultMetric, error) {
	q := `SELECT run_slug, ts, event, phase, ok, exit, fingerprint
		FROM vault_metric WHERE run_slug = ? ORDER BY id`
	rows, err := s.duck.QueryContext(ctx, q, slug)
	if err != nil {
		return nil, fmt.Errorf("listing vault metrics: %w", err)
	}
	defer rows.Close()
	out := make([]db.VaultMetric, 0, 32)
	for rows.Next() {
		m, err := scanDuckVaultMetric(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func scanDuckVaultRun(
	rows interface{ Scan(...any) error },
) (db.VaultRun, error) {
	var r db.VaultRun
	var accOK sql.NullBool
	var accExit sql.NullInt64
	if err := rows.Scan(
		&r.Slug, &r.Skill, &r.State, &r.Branch, &r.Goal, &r.RepoRoot,
		&r.WorkspacePath, &r.SourcePath, &accOK, &accExit, &r.SyncedAt,
	); err != nil {
		return db.VaultRun{}, err
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

func scanDuckVaultPhase(
	rows interface{ Scan(...any) error },
) (db.VaultPhase, error) {
	var p db.VaultPhase
	var vOK sql.NullBool
	var vExit, sFail sql.NullInt64
	var sFp sql.NullString
	if err := rows.Scan(
		&p.RunSlug, &p.PhaseID, &vOK, &vExit, &sFail, &sFp,
	); err != nil {
		return db.VaultPhase{}, err
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

func scanDuckVaultMetric(
	rows interface{ Scan(...any) error },
) (db.VaultMetric, error) {
	var m db.VaultMetric
	var ok sql.NullBool
	var exit sql.NullInt64
	var fp sql.NullString
	if err := rows.Scan(
		&m.RunSlug, &m.TS, &m.Event, &m.Phase, &ok, &exit, &fp,
	); err != nil {
		return db.VaultMetric{}, err
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
