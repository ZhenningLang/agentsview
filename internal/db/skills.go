package db

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// Skill is one entry in the coding-skills catalog, enriched with its
// SKILL.md frontmatter and a static description token estimate. It is
// reference data, NOT a session: it lives in its own dimension table
// and is populated by the SkillSyncer, never by the session sync path.
type Skill struct {
	Name               string `json:"name"`
	CatalogPath        string `json:"catalog_path"`
	ResolvedPath       string `json:"resolved_path"`
	Domain             string `json:"domain"`
	Role               string `json:"role"`
	MigrationState     string `json:"migration_state,omitempty"`
	MigrationCanonical string `json:"migration_canonical,omitempty"`
	Description        string `json:"description"`
	FrontmatterName    string `json:"frontmatter_name"`
	DescriptionTokens  int    `json:"description_tokens"`
	Tokenizer          string `json:"tokenizer"`
	CatalogPresent     bool   `json:"catalog_present"`
	FilePresent        bool   `json:"file_present"`
	HealthErrorCount   int    `json:"health_error_count"`
	SourceMtime        int64  `json:"source_mtime"`
	SyncedAt           string `json:"synced_at"`
	// Prompt is the full SKILL.md content (for the detail/audit view).
	// PromptTokens is the approximate token size of the skill body that
	// loads on each invocation.
	Prompt       string `json:"prompt,omitempty"`
	PromptTokens int    `json:"prompt_tokens"`
	// InvocationCount and TotalPromptTokens are usage-derived (joined
	// from tool_calls.skill_name at query time, not stored in the
	// dimension table). TotalPromptTokens = InvocationCount * PromptTokens.
	// They count only skill-mechanism invocations, not inline use.
	InvocationCount   int `json:"invocation_count"`
	TotalPromptTokens int `json:"total_prompt_tokens"`
}

// SkillHealth is one health-check finding produced by the skill
// catalog integrity scan (symlink/wiring/duplicate/orphan checks).
type SkillHealth struct {
	ID         int64  `json:"id"`
	SkillName  string `json:"skill_name,omitempty"`
	CheckType  string `json:"check_type"`
	Severity   string `json:"severity"`
	Message    string `json:"message"`
	Detail     string `json:"detail,omitempty"`
	DetectedAt string `json:"detected_at"`
}

// SkillFilter narrows a skills listing. Empty fields = no filter.
type SkillFilter struct {
	Domain string
	Role   string
}

// SkillHealthFilter narrows a health findings listing.
type SkillHealthFilter struct {
	SkillName string
	CheckType string
	Severity  string
}

// SkillHealthReport bundles findings with rollup counts so the UI can
// render a summary without a second round trip.
type SkillHealthReport struct {
	Findings      []SkillHealth  `json:"findings"`
	BySeverity    map[string]int `json:"by_severity"`
	ByCheckType   map[string]int `json:"by_check_type"`
	TotalSkills   int            `json:"total_skills"`
	HealthySkills int            `json:"healthy_skills"`
}

// SkillDomainCost is the static description token cost for one domain.
type SkillDomainCost struct {
	Domain string `json:"domain"`
	Skills int    `json:"skills"`
	Tokens int    `json:"tokens"`
}

// SkillTokenCostReport is the C3 static context-cost view: how many
// resident tokens each skill description costs, the total, and a
// per-domain breakdown. This is computed entirely from the skills
// dimension table and is independent of the usage/cost pipeline (it
// never reads messages.token_usage or usage_events).
type SkillTokenCostReport struct {
	TotalSkills int               `json:"total_skills"`
	TotalTokens int               `json:"total_tokens"`
	Tokenizer   string            `json:"tokenizer"`
	Approximate bool              `json:"approximate"`
	ByDomain    []SkillDomainCost `json:"by_domain"`
	Skills      []Skill           `json:"skills"`
}

const skillCols = `name, catalog_path, resolved_path, domain, role,
	migration_state, migration_canonical, description, frontmatter_name,
	description_tokens, tokenizer, catalog_present, file_present,
	health_error_count, source_mtime, synced_at, prompt, prompt_tokens`

func scanSkill(rows *sql.Rows) (Skill, error) {
	var s Skill
	var catalogPresent, filePresent int
	if err := rows.Scan(
		&s.Name, &s.CatalogPath, &s.ResolvedPath, &s.Domain, &s.Role,
		&s.MigrationState, &s.MigrationCanonical, &s.Description,
		&s.FrontmatterName, &s.DescriptionTokens, &s.Tokenizer,
		&catalogPresent, &filePresent, &s.HealthErrorCount,
		&s.SourceMtime, &s.SyncedAt, &s.Prompt, &s.PromptTokens,
	); err != nil {
		return Skill{}, err
	}
	s.CatalogPresent = catalogPresent != 0
	s.FilePresent = filePresent != 0
	return s, nil
}

// skillInvocationCounts returns invocation counts per skill name from
// tool_calls. This is the C4 usage join: it counts only invocations that
// went through the Skill tool mechanism (tool_calls.skill_name), not
// inline use, and does not touch NormalizeToolCategory. Works on all
// three backends because tool_calls is mirrored everywhere.
func (db *DB) skillInvocationCounts(ctx context.Context) (map[string]int, error) {
	rows, err := db.getReader().QueryContext(ctx,
		`SELECT skill_name, COUNT(*) FROM tool_calls
		 WHERE skill_name IS NOT NULL AND skill_name != ''
		 GROUP BY skill_name`)
	if err != nil {
		return nil, fmt.Errorf("counting skill invocations: %w", err)
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var name string
		var n int
		if err := rows.Scan(&name, &n); err != nil {
			return nil, err
		}
		out[name] = n
	}
	return out, rows.Err()
}

// ListSkills returns catalog skills, optionally filtered by domain or
// role, ordered by domain then name for stable display.
func (db *DB) ListSkills(
	ctx context.Context, f SkillFilter,
) ([]Skill, error) {
	var preds []string
	var args []any
	if f.Domain != "" {
		preds = append(preds, "domain = ?")
		args = append(args, f.Domain)
	}
	if f.Role != "" {
		preds = append(preds, "role = ?")
		args = append(args, f.Role)
	}
	where := ""
	if len(preds) > 0 {
		where = " WHERE " + strings.Join(preds, " AND ")
	}
	q := "SELECT " + skillCols + " FROM skills" + where +
		" ORDER BY domain, name"
	rows, err := db.getReader().QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("listing skills: %w", err)
	}
	defer rows.Close()
	out := make([]Skill, 0, 64)
	for rows.Next() {
		s, err := scanSkill(rows)
		if err != nil {
			return nil, fmt.Errorf("scan skill: %w", err)
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// GetSkill returns a single skill by name, or nil if not found.
func (db *DB) GetSkill(
	ctx context.Context, name string,
) (*Skill, error) {
	q := "SELECT " + skillCols + " FROM skills WHERE name = ?"
	rows, err := db.getReader().QueryContext(ctx, q, name)
	if err != nil {
		return nil, fmt.Errorf("getting skill: %w", err)
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, rows.Err()
	}
	s, err := scanSkill(rows)
	if err != nil {
		return nil, fmt.Errorf("scan skill: %w", err)
	}
	var n int
	if err := db.getReader().QueryRowContext(ctx,
		`SELECT COUNT(*) FROM tool_calls WHERE skill_name = ?`, name,
	).Scan(&n); err == nil {
		s.InvocationCount = n
		s.TotalPromptTokens = n * s.PromptTokens
	}
	return &s, nil
}

// GetSkillHealth returns health findings plus rollup counts.
func (db *DB) GetSkillHealth(
	ctx context.Context, f SkillHealthFilter,
) (SkillHealthReport, error) {
	rep := SkillHealthReport{
		BySeverity:  map[string]int{},
		ByCheckType: map[string]int{},
	}
	var preds []string
	var args []any
	if f.SkillName != "" {
		preds = append(preds, "skill_name = ?")
		args = append(args, f.SkillName)
	}
	if f.CheckType != "" {
		preds = append(preds, "check_type = ?")
		args = append(args, f.CheckType)
	}
	if f.Severity != "" {
		preds = append(preds, "severity = ?")
		args = append(args, f.Severity)
	}
	where := ""
	if len(preds) > 0 {
		where = " WHERE " + strings.Join(preds, " AND ")
	}
	q := `SELECT id, COALESCE(skill_name, ''), check_type, severity,
		message, detail, detected_at FROM skill_health` + where +
		` ORDER BY severity, check_type, skill_name, id`
	rows, err := db.getReader().QueryContext(ctx, q, args...)
	if err != nil {
		return rep, fmt.Errorf("listing skill health: %w", err)
	}
	defer rows.Close()
	rep.Findings = make([]SkillHealth, 0, 32)
	for rows.Next() {
		var h SkillHealth
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
	// Rollup: total skills and how many have zero error findings.
	if err := db.getReader().QueryRowContext(ctx,
		"SELECT COUNT(*), COALESCE(SUM(CASE WHEN health_error_count = 0"+
			" THEN 1 ELSE 0 END), 0) FROM skills",
	).Scan(&rep.TotalSkills, &rep.HealthySkills); err != nil {
		return rep, fmt.Errorf("skill health rollup: %w", err)
	}
	return rep, nil
}

// GetSkillTokenCost returns the static description token cost report.
func (db *DB) GetSkillTokenCost(
	ctx context.Context,
) (SkillTokenCostReport, error) {
	// Initialize slices so the JSON contract is always arrays, never
	// null, even with zero skills. The frontend maps/spreads these
	// directly and would crash on null.
	rep := SkillTokenCostReport{
		Approximate: true,
		ByDomain:    []SkillDomainCost{},
		Skills:      []Skill{},
	}
	skills, err := db.ListSkills(ctx, SkillFilter{})
	if err != nil {
		return rep, err
	}
	counts, err := db.skillInvocationCounts(ctx)
	if err != nil {
		return rep, err
	}
	byDomain := map[string]*SkillDomainCost{}
	order := make([]string, 0, 16)
	for i := range skills {
		s := &skills[i]
		s.InvocationCount = counts[s.Name]
		s.TotalPromptTokens = s.InvocationCount * s.PromptTokens
		rep.TotalSkills++
		rep.TotalTokens += s.DescriptionTokens
		if s.Tokenizer != "" {
			rep.Tokenizer = s.Tokenizer
		}
		d, ok := byDomain[s.Domain]
		if !ok {
			d = &SkillDomainCost{Domain: s.Domain}
			byDomain[s.Domain] = d
			order = append(order, s.Domain)
		}
		d.Skills++
		d.Tokens += s.DescriptionTokens
	}
	for _, dom := range order {
		rep.ByDomain = append(rep.ByDomain, *byDomain[dom])
	}
	rep.Skills = skills
	return rep, nil
}

// txExec is the subset of *sql.Tx used by the row-replace helpers.
type txExec interface {
	ExecContext(ctx context.Context, q string, args ...any) (sql.Result, error)
	PrepareContext(ctx context.Context, q string) (*sql.Stmt, error)
}

// replaceSkillsTx full-replaces the skills table inside an open tx.
func replaceSkillsTx(ctx context.Context, tx txExec, skills []Skill) error {
	if _, err := tx.ExecContext(ctx, "DELETE FROM skills"); err != nil {
		return fmt.Errorf("clearing skills: %w", err)
	}
	stmt, err := tx.PrepareContext(ctx, `INSERT INTO skills (`+skillCols+`)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`)
	if err != nil {
		return fmt.Errorf("prepare skills insert: %w", err)
	}
	defer stmt.Close()
	for _, s := range skills {
		if _, err := stmt.ExecContext(ctx,
			s.Name, s.CatalogPath, s.ResolvedPath, s.Domain, s.Role,
			s.MigrationState, s.MigrationCanonical, s.Description,
			s.FrontmatterName, s.DescriptionTokens, s.Tokenizer,
			boolToInt(s.CatalogPresent), boolToInt(s.FilePresent),
			s.HealthErrorCount, s.SourceMtime, s.SyncedAt,
			s.Prompt, s.PromptTokens,
		); err != nil {
			return fmt.Errorf("insert skill %q: %w", s.Name, err)
		}
	}
	return nil
}

// replaceSkillHealthTx full-replaces skill_health inside an open tx.
func replaceSkillHealthTx(
	ctx context.Context, tx txExec, findings []SkillHealth,
) error {
	if _, err := tx.ExecContext(ctx, "DELETE FROM skill_health"); err != nil {
		return fmt.Errorf("clearing skill_health: %w", err)
	}
	stmt, err := tx.PrepareContext(ctx, `INSERT INTO skill_health
		(skill_name, check_type, severity, message, detail, detected_at)
		VALUES (?,?,?,?,?,?)`)
	if err != nil {
		return fmt.Errorf("prepare skill_health insert: %w", err)
	}
	defer stmt.Close()
	for _, h := range findings {
		var skillName any
		if h.SkillName != "" {
			skillName = h.SkillName
		}
		if _, err := stmt.ExecContext(ctx,
			skillName, h.CheckType, h.Severity, h.Message, h.Detail,
			h.DetectedAt,
		); err != nil {
			return fmt.Errorf("insert skill_health: %w", err)
		}
	}
	return nil
}

// ReplaceSkillCatalog atomically full-replaces both the skills and
// skill_health tables in a single transaction. The SkillSyncer uses this
// so a crash between the two writes can never leave skills.health_error_count
// pointing at a stale skill_health detail set. Local-only writer.
func (db *DB) ReplaceSkillCatalog(
	ctx context.Context, skills []Skill, findings []SkillHealth,
) error {
	db.mu.Lock()
	defer db.mu.Unlock()
	tx, err := db.getWriter().BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin skill catalog tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := replaceSkillsTx(ctx, tx, skills); err != nil {
		return err
	}
	if err := replaceSkillHealthTx(ctx, tx, findings); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit skill catalog: %w", err)
	}
	return nil
}

// ReplaceSkills full-replaces only the skills table in its own
// transaction. Kept as a granular building block; the production sync
// path uses ReplaceSkillCatalog for atomicity across both tables.
func (db *DB) ReplaceSkills(ctx context.Context, skills []Skill) error {
	db.mu.Lock()
	defer db.mu.Unlock()
	tx, err := db.getWriter().BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin skills tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := replaceSkillsTx(ctx, tx, skills); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit skills: %w", err)
	}
	return nil
}

// ReplaceSkillHealth full-replaces only skill_health in its own tx.
func (db *DB) ReplaceSkillHealth(
	ctx context.Context, findings []SkillHealth,
) error {
	db.mu.Lock()
	defer db.mu.Unlock()
	tx, err := db.getWriter().BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin skill_health tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := replaceSkillHealthTx(ctx, tx, findings); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit skill_health: %w", err)
	}
	return nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
