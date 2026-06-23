// Package skills syncs the coding-skills catalog (catalog.json plus
// per-skill SKILL.md frontmatter) into the skills/skill_health
// dimension tables. It is reference-data plumbing that runs entirely
// independently of the session sync engine: it never reads or writes
// sessions, messages, or tool_calls, so it cannot pollute the core
// fact domain or its stats triggers.
package skills

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.kenn.io/agentsview/internal/db"
	"gopkg.in/yaml.v3"
)

// Writer is the narrow persistence surface the syncer needs. It is
// satisfied by *db.DB; the PG/DuckDB read stores receive skill rows via
// push sync, not through this path. The single combined method keeps the
// skills and skill_health replacements in one transaction so a crash can
// never leave the badge counts and findings out of sync.
type Writer interface {
	ReplaceSkillCatalog(
		ctx context.Context, skills []db.Skill, findings []db.SkillHealth,
	) error
}

// catalogEntry mirrors one object in catalog.json.
type catalogEntry struct {
	Name      string `json:"name"`
	Path      string `json:"path"`
	Domain    string `json:"domain"`
	Role      string `json:"role"`
	Migration *struct {
		State     string `json:"state"`
		Canonical string `json:"canonical"`
	} `json:"migration"`
}

type catalogFile struct {
	Skills []catalogEntry `json:"skills"`
}

// frontmatter is the YAML header of a SKILL.md file.
type frontmatter struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
}

// Syncer reads a coding-skills catalog directory and persists the
// derived skills + health findings.
type Syncer struct {
	catalogDir string
	tokenizer  Tokenizer
	writer     Writer
	now        func() time.Time
}

// NewSyncer builds a Syncer. catalogDir is the coding-skills directory
// containing catalog.json (e.g. ~/.dotfiles/coding-skills).
func NewSyncer(catalogDir string, w Writer, tk Tokenizer) *Syncer {
	if tk == nil {
		tk = NewHeuristicTokenizer()
	}
	return &Syncer{
		catalogDir: catalogDir,
		tokenizer:  tk,
		writer:     w,
		now:        time.Now,
	}
}

// CatalogPath returns the catalog.json path for this syncer.
func (s *Syncer) CatalogPath() string {
	return filepath.Join(s.catalogDir, "catalog.json")
}

// Sync reads the catalog and replaces the skills + skill_health tables.
// It is fail-soft per skill: a single unreadable entry becomes a health
// finding rather than aborting the whole run.
func (s *Syncer) Sync(ctx context.Context) error {
	entries, err := s.readCatalog()
	if err != nil {
		return fmt.Errorf("reading catalog: %w", err)
	}
	rootDir := filepath.Dir(s.catalogDir)
	syncedAt := s.now().UTC().Format("2006-01-02T15:04:05.000Z")

	skills := make([]db.Skill, 0, len(entries))
	var findings []db.SkillHealth
	catalogNames := map[string]int{}
	catalogBasenames := map[string]bool{}

	for _, e := range entries {
		catalogNames[e.Name]++
		base := filepath.Base(e.Path)
		catalogBasenames[base] = true

		sk := db.Skill{
			Name:           e.Name,
			CatalogPath:    e.Path,
			Domain:         e.Domain,
			Role:           e.Role,
			CatalogPresent: true,
			Tokenizer:      s.tokenizer.Name(),
			SyncedAt:       syncedAt,
		}
		if e.Migration != nil {
			sk.MigrationState = e.Migration.State
			sk.MigrationCanonical = e.Migration.Canonical
		}

		skillDir := filepath.Join(rootDir, e.Path)
		resolved, resolveErr := filepath.EvalSymlinks(skillDir)
		if resolveErr != nil {
			findings = append(findings, finding(e.Name,
				"symlink_broken", "error",
				fmt.Sprintf("catalog path %q does not resolve", e.Path),
				map[string]string{"path": e.Path, "error": resolveErr.Error()}))
			skills = append(skills, sk)
			continue
		}
		sk.ResolvedPath = resolved

		mdPath := filepath.Join(resolved, "SKILL.md")
		info, statErr := os.Stat(mdPath)
		if statErr != nil {
			findings = append(findings, finding(e.Name,
				"orphan_catalog", "error",
				"catalog entry has no SKILL.md on disk",
				map[string]string{"path": mdPath}))
			skills = append(skills, sk)
			continue
		}
		sk.FilePresent = true
		sk.SourceMtime = info.ModTime().Unix()

		fm, fmErr := readFrontmatter(mdPath)
		if fmErr != nil || (fm.Name == "" && fm.Description == "") {
			findings = append(findings, finding(e.Name,
				"missing_frontmatter", "error",
				"SKILL.md has no parseable name/description frontmatter",
				map[string]string{"path": mdPath}))
			skills = append(skills, sk)
			continue
		}
		sk.FrontmatterName = fm.Name
		sk.Description = fm.Description
		sk.DescriptionTokens = s.tokenizer.Count(fm.Description)

		// Wiring: catalog name == frontmatter name == dir basename.
		if (fm.Name != "" && fm.Name != e.Name) || base != e.Name {
			findings = append(findings, finding(e.Name,
				"name_mismatch", "error",
				"catalog name, frontmatter name, and directory differ",
				map[string]string{
					"catalog_name":     e.Name,
					"frontmatter_name": fm.Name,
					"dir_basename":     base,
				}))
		}
		skills = append(skills, sk)
	}

	// Catalog-level: duplicate names.
	for name, n := range catalogNames {
		if n > 1 {
			findings = append(findings, finding(name,
				"duplicate_name", "error",
				fmt.Sprintf("name appears %d times in catalog", n), nil))
		}
	}

	// Catalog-level: legacy entries whose canonical target is missing.
	for _, sk := range skills {
		if sk.Role == "legacy" && sk.MigrationCanonical != "" {
			if catalogNames[sk.MigrationCanonical] == 0 {
				findings = append(findings, finding(sk.Name,
					"legacy_dangling_canonical", "warn",
					"migration canonical target not found in catalog",
					map[string]string{"canonical": sk.MigrationCanonical}))
			}
		}
	}

	// Catalog-level: on-disk skill dirs absent from the catalog.
	findings = append(findings, s.orphanFiles(catalogBasenames)...)

	// Stamp every finding with the sync time so the health panel can
	// show when an issue was detected. Done here (rather than in
	// finding()) so the timestamp matches the skills' synced_at and stays
	// identical across both storage backends.
	for i := range findings {
		findings[i].DetectedAt = syncedAt
	}

	// Roll up per-skill error counts for the clip badge.
	errCounts := map[string]int{}
	for _, f := range findings {
		if f.Severity == "error" && f.SkillName != "" {
			errCounts[f.SkillName]++
		}
	}
	for i := range skills {
		skills[i].HealthErrorCount = errCounts[skills[i].Name]
	}

	return s.writer.ReplaceSkillCatalog(ctx, skills, findings)
}

func (s *Syncer) readCatalog() ([]catalogEntry, error) {
	data, err := os.ReadFile(s.CatalogPath())
	if err != nil {
		return nil, err
	}
	var cf catalogFile
	if err := json.Unmarshal(data, &cf); err != nil {
		return nil, err
	}
	return cf.Skills, nil
}

// orphanFiles scans the catalog directory for subdirectories that
// contain a SKILL.md but are not referenced by any catalog entry.
func (s *Syncer) orphanFiles(known map[string]bool) []db.SkillHealth {
	dirents, err := os.ReadDir(s.catalogDir)
	if err != nil {
		return nil
	}
	var out []db.SkillHealth
	for _, de := range dirents {
		name := de.Name()
		if strings.HasPrefix(name, ".") || known[name] {
			continue
		}
		// Resolve symlinked entries to detect directories too.
		full := filepath.Join(s.catalogDir, name)
		info, err := os.Stat(full)
		if err != nil || !info.IsDir() {
			continue
		}
		if _, err := os.Stat(filepath.Join(full, "SKILL.md")); err != nil {
			continue
		}
		out = append(out, finding(name, "orphan_file", "warn",
			"directory has a SKILL.md but is not in catalog.json",
			map[string]string{"dir": name}))
	}
	return out
}

func readFrontmatter(path string) (frontmatter, error) {
	var fm frontmatter
	data, err := os.ReadFile(path)
	if err != nil {
		return fm, err
	}
	raw := extractFrontmatterBlock(string(data))
	if raw == "" {
		return fm, nil
	}
	if err := yaml.Unmarshal([]byte(raw), &fm); err != nil {
		return fm, err
	}
	return fm, nil
}

// extractFrontmatterBlock returns the YAML between the leading --- and
// the next --- line, or "" when there is no frontmatter.
func extractFrontmatterBlock(content string) string {
	content = strings.TrimLeft(content, "\ufeff \t\r\n")
	if !strings.HasPrefix(content, "---") {
		return ""
	}
	lines := strings.Split(content, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return ""
	}
	var body []string
	for _, ln := range lines[1:] {
		if strings.TrimSpace(ln) == "---" {
			return strings.Join(body, "\n")
		}
		body = append(body, ln)
	}
	return ""
}

func finding(
	skill, checkType, severity, message string, detail map[string]string,
) db.SkillHealth {
	d := ""
	if len(detail) > 0 {
		if b, err := json.Marshal(detail); err == nil {
			d = string(b)
		}
	}
	return db.SkillHealth{
		SkillName: skill,
		CheckType: checkType,
		Severity:  severity,
		Message:   message,
		Detail:    d,
	}
}
