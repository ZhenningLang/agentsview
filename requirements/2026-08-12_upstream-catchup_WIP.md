# Upstream Catchup Requirement

## Goal

Semantically port selected upstream `agentsview` correctness and data-safety fixes into this fork while preserving the fork's memory/synthesize system, Droid and KimiCode parsers, and persistent SQLite archive.

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

## Acceptance Criteria

- Phase 01 preserves legacy SQLite archive rows during schema repair and creates this initiative entry.
- Phase 02 ports the FTS operator-character search fix without regressing CJK search behavior.
- Phase 03 ports peak-context stats distribution semantics and documents the `claude_only` boundary decision.
- Phase 04 ports static SPA asset fallback/cache behavior.
- Phase 05 updates markdown rendering and DOMPurify with advisory evidence and rebuilt embedded frontend assets.
- Phase 06 creates `docs/UPSTREAM_AUDIT.md` covering all apply-failed fix/perf candidates with evidence-backed decisions.
- Relevant Go tests, `go vet`, Go formatting, and phase-specific verification commands pass before delivery.
