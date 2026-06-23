package postgres

import (
	"context"
	"fmt"

	"go.kenn.io/agentsview/internal/db"
)

// syncSkills pushes the local skills + skill_health dimension tables to
// PostgreSQL. Skills are small, global reference data (not per-machine,
// not per-session, not project-scoped), so the push is an unconditional
// full replace inside one transaction rather than a watermarked diff.
// This keeps the read-side PG store (pg serve) in sync with whatever the
// local SkillSyncer last wrote, with no fingerprinting needed.
func (s *Sync) syncSkills(ctx context.Context) error {
	skills, err := s.local.ListSkills(ctx, db.SkillFilter{})
	if err != nil {
		return fmt.Errorf("listing local skills: %w", err)
	}
	// Guard against wiping PG skills from a context that never ran the
	// catalog sync. `agentsview pg push` is a standalone command that does
	// not start the SkillSyncer, so the local skills table is empty there.
	// Treating "no local skills" as "replace PG with nothing" would clear
	// skills that an earlier `serve`-driven push had populated. An empty
	// local set almost always means "this invocation did not sync the
	// catalog", not "the catalog is genuinely empty", so skip the push.
	if len(skills) == 0 {
		return nil
	}
	health, err := s.local.GetSkillHealth(ctx, db.SkillHealthFilter{})
	if err != nil {
		return fmt.Errorf("listing local skill health: %w", err)
	}

	tx, err := s.pg.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin skills push tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, "DELETE FROM skill_health"); err != nil {
		return fmt.Errorf("clearing pg skill_health: %w", err)
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM skills"); err != nil {
		return fmt.Errorf("clearing pg skills: %w", err)
	}

	for _, sk := range skills {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO skills (
				name, catalog_path, resolved_path, domain, role,
				migration_state, migration_canonical, description,
				frontmatter_name, description_tokens, tokenizer,
				catalog_present, file_present, health_error_count,
				source_mtime, synced_at, prompt, prompt_tokens
			) VALUES (
				$1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18
			)`,
			sk.Name, sk.CatalogPath, sk.ResolvedPath, sk.Domain, sk.Role,
			sk.MigrationState, sk.MigrationCanonical, sk.Description,
			sk.FrontmatterName, sk.DescriptionTokens, sk.Tokenizer,
			boolToPGInt(sk.CatalogPresent), boolToPGInt(sk.FilePresent),
			sk.HealthErrorCount, sk.SourceMtime, sk.SyncedAt,
			sk.Prompt, sk.PromptTokens,
		); err != nil {
			return fmt.Errorf("inserting pg skill %q: %w", sk.Name, err)
		}
	}

	// skill_health.id is GENERATED ALWAYS AS IDENTITY, so it is omitted
	// here and re-generated PG-side. A nil skill_name maps to SQL NULL
	// for catalog-level findings (orphan dirs, etc.).
	for _, h := range health.Findings {
		var skillName any
		if h.SkillName != "" {
			skillName = h.SkillName
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO skill_health (
				skill_name, check_type, severity, message, detail,
				detected_at
			) VALUES ($1,$2,$3,$4,$5,$6)`,
			skillName, h.CheckType, h.Severity, h.Message, h.Detail,
			h.DetectedAt,
		); err != nil {
			return fmt.Errorf("inserting pg skill_health: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit skills push: %w", err)
	}
	return nil
}

func boolToPGInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
