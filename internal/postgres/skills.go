package postgres

import (
	"context"
	"fmt"
	"strings"

	"go.kenn.io/agentsview/internal/db"
)

// Skills read methods mirror the SQLite implementations in
// internal/db/skills.go. The PG store is read-only: skill rows reach PG
// via push sync from SQLite, so there are no writer methods here.

const pgSkillCols = `name, catalog_path, resolved_path, domain, role,
	migration_state, migration_canonical, description, frontmatter_name,
	description_tokens, tokenizer, catalog_present, file_present,
	health_error_count, source_mtime, synced_at`

// ListSkills returns catalog skills ordered by domain then name.
func (s *Store) ListSkills(
	ctx context.Context, f db.SkillFilter,
) ([]db.Skill, error) {
	pb := &paramBuilder{}
	var preds []string
	if f.Domain != "" {
		preds = append(preds, "domain = "+pb.add(f.Domain))
	}
	if f.Role != "" {
		preds = append(preds, "role = "+pb.add(f.Role))
	}
	where := ""
	if len(preds) > 0 {
		where = " WHERE " + strings.Join(preds, " AND ")
	}
	q := "SELECT " + pgSkillCols + " FROM skills" + where +
		" ORDER BY domain, name"
	rows, err := s.pg.QueryContext(ctx, q, pb.args...)
	if err != nil {
		return nil, fmt.Errorf("listing skills: %w", err)
	}
	defer rows.Close()
	out := make([]db.Skill, 0, 64)
	for rows.Next() {
		sk, err := scanPGSkill(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, sk)
	}
	return out, rows.Err()
}

// GetSkill returns one skill by name, or nil when absent.
func (s *Store) GetSkill(
	ctx context.Context, name string,
) (*db.Skill, error) {
	q := "SELECT " + pgSkillCols + " FROM skills WHERE name = $1"
	rows, err := s.pg.QueryContext(ctx, q, name)
	if err != nil {
		return nil, fmt.Errorf("getting skill: %w", err)
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, rows.Err()
	}
	sk, err := scanPGSkill(rows)
	if err != nil {
		return nil, err
	}
	return &sk, nil
}

// GetSkillHealth returns health findings plus rollup counts.
func (s *Store) GetSkillHealth(
	ctx context.Context, f db.SkillHealthFilter,
) (db.SkillHealthReport, error) {
	rep := db.SkillHealthReport{
		BySeverity:  map[string]int{},
		ByCheckType: map[string]int{},
	}
	pb := &paramBuilder{}
	var preds []string
	if f.SkillName != "" {
		preds = append(preds, "skill_name = "+pb.add(f.SkillName))
	}
	if f.CheckType != "" {
		preds = append(preds, "check_type = "+pb.add(f.CheckType))
	}
	if f.Severity != "" {
		preds = append(preds, "severity = "+pb.add(f.Severity))
	}
	where := ""
	if len(preds) > 0 {
		where = " WHERE " + strings.Join(preds, " AND ")
	}
	q := `SELECT id, COALESCE(skill_name, ''), check_type, severity,
		message, detail, detected_at FROM skill_health` + where +
		` ORDER BY severity, check_type, skill_name, id`
	rows, err := s.pg.QueryContext(ctx, q, pb.args...)
	if err != nil {
		return rep, fmt.Errorf("listing skill health: %w", err)
	}
	defer rows.Close()
	rep.Findings = make([]db.SkillHealth, 0, 32)
	for rows.Next() {
		var h db.SkillHealth
		if err := rows.Scan(&h.ID, &h.SkillName, &h.CheckType,
			&h.Severity, &h.Message, &h.Detail, &h.DetectedAt); err != nil {
			return rep, fmt.Errorf("scan skill health: %w", err)
		}
		rep.Findings = append(rep.Findings, h)
		rep.BySeverity[h.Severity]++
		rep.ByCheckType[h.CheckType]++
	}
	if err := rows.Err(); err != nil {
		return rep, err
	}
	if err := s.pg.QueryRowContext(ctx,
		"SELECT COUNT(*), COALESCE(SUM(CASE WHEN health_error_count = 0"+
			" THEN 1 ELSE 0 END), 0) FROM skills",
	).Scan(&rep.TotalSkills, &rep.HealthySkills); err != nil {
		return rep, fmt.Errorf("skill health rollup: %w", err)
	}
	return rep, nil
}

// GetSkillTokenCost aggregates static description token cost from the
// skills table, identical in shape to the SQLite implementation.
func (s *Store) GetSkillTokenCost(
	ctx context.Context,
) (db.SkillTokenCostReport, error) {
	rep := db.SkillTokenCostReport{Approximate: true}
	skills, err := s.ListSkills(ctx, db.SkillFilter{})
	if err != nil {
		return rep, err
	}
	byDomain := map[string]*db.SkillDomainCost{}
	order := make([]string, 0, 16)
	for _, sk := range skills {
		rep.TotalSkills++
		rep.TotalTokens += sk.DescriptionTokens
		if sk.Tokenizer != "" {
			rep.Tokenizer = sk.Tokenizer
		}
		d, ok := byDomain[sk.Domain]
		if !ok {
			d = &db.SkillDomainCost{Domain: sk.Domain}
			byDomain[sk.Domain] = d
			order = append(order, sk.Domain)
		}
		d.Skills++
		d.Tokens += sk.DescriptionTokens
	}
	for _, dom := range order {
		rep.ByDomain = append(rep.ByDomain, *byDomain[dom])
	}
	rep.Skills = skills
	return rep, nil
}

func scanPGSkill(rows interface{ Scan(...any) error }) (db.Skill, error) {
	var sk db.Skill
	var catalogPresent, filePresent int
	if err := rows.Scan(
		&sk.Name, &sk.CatalogPath, &sk.ResolvedPath, &sk.Domain, &sk.Role,
		&sk.MigrationState, &sk.MigrationCanonical, &sk.Description,
		&sk.FrontmatterName, &sk.DescriptionTokens, &sk.Tokenizer,
		&catalogPresent, &filePresent, &sk.HealthErrorCount,
		&sk.SourceMtime, &sk.SyncedAt,
	); err != nil {
		return db.Skill{}, err
	}
	sk.CatalogPresent = catalogPresent != 0
	sk.FilePresent = filePresent != 0
	return sk, nil
}
