# Skill Governance Observation Views Requirement

Status: DONE — delivered in commit `12e6c4c`
Full spec (SSOT): `~/.dotfiles/docs/agentsview-skills-integration-spec.md`
Upstream PRD: `~/.dotfiles/docs/harness-governance-prd-2026-06-23.md` (workflow C)

## Goal

Turn agentsview into the governance observation layer for a cross-agent
coding-skills system: see what skills exist, whether the catalog is
healthy, and how much resident context each skill description costs —
driven by data, not memory.

## Background

The cross-agent skill catalog (`~/.dotfiles/coding-skills/catalog.json`
plus per-skill `SKILL.md` frontmatter) is slowly-changing reference
data, not sessions. Decisions about pruning, renaming, and context cost
had no data behind them. agentsview already parses sessions/messages/
tool_calls and serves a web UI, so it is the natural place to surface
skill-catalog facts.

## Core Requirements (delivered)

1. C1 — Skill inventory: browse name / domain / role / enabled status /
   description token count for every catalog skill.
2. C2 — Catalog health check: detect symlink-broken paths, name/wiring
   mismatches (catalog vs frontmatter vs directory), duplicates, on-disk
   orphans, and legacy entries pointing at a missing canonical.
3. C3 — Static context cost: per-skill description token estimate, total,
   share of a reference window, and a per-domain breakdown. Token counts
   are approximate (heuristic tokenizer, not the Anthropic tokenizer) and
   independent of the session usage/cost pipeline.

## Locked Data-Modeling Decision

- Skills live in their own `skills` + `skill_health` dimension tables,
  synced by a dedicated SkillSyncer that runs independently of the
  session sync engine. It never writes sessions/messages/tool_calls, so
  the stats triggers and all existing analytics stay unpolluted.
- Parity is kept across all three serve backends (SQLite owns the writer;
  PostgreSQL and DuckDB mirror via their push paths and expose the same
  read queries).

## Non-Goals / Deferred

- C4 (skill usage statistics from `tool_calls.skill_name`) is designed in
  the spec but intentionally not implemented in this delivery.
- The general tool-category normalization is unchanged.
- Dynamic per-session capsule-injection tracking is out of scope.

## Acceptance Criteria (all met)

- [x] Browser shows skill inventory + health findings + per-skill static
      description cost.
- [x] Zero pollution: importing skills does not change session/message
      counts and inserts no sessions/messages/tool_calls rows (asserted in
      `internal/db/skills_test.go`).
- [x] SQLite / PostgreSQL / DuckDB return the same skill read shapes;
      PG and DuckDB mirrors round-trip via push (pgtest + duck tests).
- [x] Catalog directory configurable via `AGENTSVIEW_SKILLS_DIR` /
      `skills_catalog_dir`, fail-open when absent.
- [x] `go build/vet ./...`, gofmt, Go unit tests, PG pgtest suite, and
      `svelte-check` all pass; feature verified live in a browser.

## Phases (as delivered)

1. C1 inventory: config, `internal/skills` syncer, schema tables, store
   methods, `/api/v1/skills` route, frontend route + page.
2. C3 static cost: heuristic tokenizer, cost aggregation + UI.
3. C2 health: health checks, `skill_health` table, findings UI.
4. Dual review (kilo + Claude) hardening: PG/DuckDB push guards against
   wiping an empty catalog, full DuckDB mirror, atomic catalog replace,
   empty-slice JSON contract, schema-compat probes, mobile nav, TOML
   config wiring.
5. C4 usage statistics: designed only, not implemented.
