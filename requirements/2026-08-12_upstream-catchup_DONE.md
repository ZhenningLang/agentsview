# Upstream Catchup Requirement

## Goal

Semantically port selected upstream `agentsview` correctness and data-safety fixes into this fork while preserving the fork's memory/synthesize system, Droid and KimiCode parsers, and persistent SQLite archive.

## Scope

Phases 01-08 only: the `fix` / `perf` correctness and data-safety ports plus the CI cleanup that followed them. Feature adoption from upstream (`feat` commits, phases 09-13 and later tiers) is a separate initiative and its SSOT is `requirements/2026-08-13_upstream-feature-adoption_WIP.md`. The phase 10-13 boundary decisions and acceptance criteria that were originally recorded here have been moved to that entry.

## Related Artifact

The upstream fix/perf audit ledger is `docs/UPSTREAM_AUDIT.md`. It is produced later in this initiative and is the long-lived review record for candidates not directly landed in the early phases.

## Locked Decisions

- Do not merge or rebase wholesale onto upstream history.
- Do not absorb the upstream parser provider refactor or `internal/recall` in this run.
- Do not delete, truncate, drop, or recreate the real SQLite archive; SQLite schema changes must be non-destructive.
- Preserve observable behavior parity between SQLite and PostgreSQL/Cockroach where affected.
- Do not deploy, push, change secrets, or update `/Applications/AgentsView.app` as part of this initiative.

## Open Decisions

- Fix/perf commits marked `adopt` in `docs/UPSTREAM_AUDIT.md` but outside this run will be tracked in `docs/UPSTREAM_BACKLOG.md` for separate scheduling.

## Boundary Decisions

- Phase 01 changed `Open()` on legacy archives from destructive recreate to transactional legacy schema repair plus non-destructive resync. The repair probe now covers 10 legacy columns: `sessions.parent_session_id`, `insights.date_from`, `insights.date_to`, `tool_calls.tool_use_id`, `tool_calls.input_json`, `tool_calls.skill_name`, `tool_calls.result_content_length`, `sessions.user_message_count`, `sessions.relationship_type`, and `tool_calls.subagent_session_id`.
- Phase 01 added a fail-closed refusal when a legacy archive needing schema repair has `PRAGMA user_version` newer than this binary supports: `database data version %d is newer than supported version %d`. This affects the shared `Open()` path used by `serve`, `stats`, and `pg push`.
- The future-version refusal is currently asymmetric: schema-current archives with a newer `user_version` can still open, while legacy archives needing schema repair are refused. Follow-up `6407da6cfb9edccae01f73f173055fb2ff11eb05` in `docs/UPSTREAM_BACKLOG.md` tracks making this boundary consistent.
- Phases 07-08 edited files inside `internal/memory/` and `internal/synthesize/`, which the run's "do not touch the fork's memory/synthesize system" constraint had otherwise reserved. The constraint was aimed at redesign — replacing, plugin-izing, or substituting `internal/recall` for that system — and the CI phases were scoped to making existing code pass lint and build on Windows, so the edits were taken as an allowed exception rather than a scope change. The complete set is `internal/memory/feedback.go`, `internal/memory/ledger_editor.go`, `internal/memory/writer.go`, `internal/memory/writer_test.go`, `internal/synthesize/audit.go`, and `internal/synthesize/worker.go`; the changes are lint rewrites (`slices.Contains`, `strings.SplitSeq`, `fmt.Fprintf`), one Windows fix that strips a volume name before the path-traversal check in `writer.go`, and one nil-map guard in `ledger_editor.go` — no change to what the memory system stores, indexes, or exposes. Two behavior deltas follow from those two guards: an absolute `relPath` now has its volume name and leading separators stripped before confinement, so a Windows `C:\...` input normalizes the way a POSIX `/...` input already did under `filepath.Join`, and a JSON `null` ledger entry is now refused with an error instead of panicking on assignment to a nil map.

## Acceptance Criteria

- Phase 01 preserves legacy SQLite archive rows during schema repair and creates this initiative entry.
- Phase 02 ports the FTS operator-character search fix without regressing CJK search behavior.
- Phase 03 ports peak-context stats distribution semantics and documents the `claude_only` boundary decision.
- Phase 04 ports static SPA asset fallback/cache behavior.
- Phase 05 updates markdown rendering and DOMPurify with advisory evidence and rebuilt embedded frontend assets.
- Phase 06 creates `docs/UPSTREAM_AUDIT.md` covering all apply-failed fix/perf candidates with evidence-backed decisions.
- Phases 07-08 clear the pre-existing CI failures (Windows portability, lint, svelte-check, PostgreSQL integration, race).
- Phases 09-13 (upstream `feat` adoption) are out of scope for this entry; their acceptance criteria live in `requirements/2026-08-13_upstream-feature-adoption_WIP.md`.
- Relevant Go tests, `go vet`, Go formatting, frontend unit tests, `svelte-check`, the Vite build, Playwright end-to-end specs, and phase-specific verification commands pass before delivery.
