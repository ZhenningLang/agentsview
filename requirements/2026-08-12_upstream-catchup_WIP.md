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

## Boundary Decisions

- Phase 01 changed `Open()` on legacy archives from destructive recreate to transactional legacy schema repair plus non-destructive resync. The repair probe now covers 10 legacy columns: `sessions.parent_session_id`, `insights.date_from`, `insights.date_to`, `tool_calls.tool_use_id`, `tool_calls.input_json`, `tool_calls.skill_name`, `tool_calls.result_content_length`, `sessions.user_message_count`, `sessions.relationship_type`, and `tool_calls.subagent_session_id`.
- Phase 01 added a fail-closed refusal when a legacy archive needing schema repair has `PRAGMA user_version` newer than this binary supports: `database data version %d is newer than supported version %d`. This affects the shared `Open()` path used by `serve`, `stats`, and `pg push`.
- The future-version refusal is currently asymmetric: schema-current archives with a newer `user_version` can still open, while legacy archives needing schema repair are refused. Follow-up `6407da6cfb9edccae01f73f173055fb2ff11eb05` in `docs/UPSTREAM_BACKLOG.md` tracks making this boundary consistent.

## Acceptance Criteria

- Phase 01 preserves legacy SQLite archive rows during schema repair and creates this initiative entry.
- Phase 02 ports the FTS operator-character search fix without regressing CJK search behavior.
- Phase 03 ports peak-context stats distribution semantics and documents the `claude_only` boundary decision.
- Phase 04 ports static SPA asset fallback/cache behavior.
- Phase 05 updates markdown rendering and DOMPurify with advisory evidence and rebuilt embedded frontend assets.
- Phase 06 creates `docs/UPSTREAM_AUDIT.md` covering all apply-failed fix/perf candidates with evidence-backed decisions.
- Relevant Go tests, `go vet`, Go formatting, and phase-specific verification commands pass before delivery.
