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

- Phase 12 made `generateFallbackContent` render a pre-computed `patch` / `patch_text` / `patchText` parameter for Edit-shaped tool calls. This changes the displayed tool input for those calls from a single `patch: <blob>` key-value line to the patch body itself, and the same formatter (uncapped) feeds the new copy button. No parser, schema, or API change.
- Phase 12 gave the Pi `edits[]` display path the 200-line cap (`MAX_DIFF_LINES`) it previously lacked. Before this phase that branch returned its lines uncapped, so a large kilo/Pi edit rendered every line into the DOM; every other tool shape already honoured the cap. Measured on a 500-line single-entry edit: 402 rendered lines with no marker before, 201 lines ending in `... (402 lines total)` after. The copy path is unaffected and still returns all 501 lines, so no content is lost to the user — only the on-screen preview is bounded. The per-edit 400-character truncation is likewise now skipped in copy mode while display behavior at 400 characters is unchanged.
- Phase 12 added `parser_malformed_lines` to the hand-written `Session` type only. The field was already serialized by the API and present in the generated models; no backend, schema, or generated-model change.
- Phase 12's Calls disclosure defaults to expanded, and missing, unparseable, or unavailable LocalStorage also falls back to expanded, so existing users keep today's behavior until they collapse it.
- Phase 12's new labels are the first localized strings in the AppHeader export menus and the shortcuts modal, which are otherwise hard-coded English. Under the default `zh` locale those specific rows render Chinese next to English neighbours; localizing the surrounding surfaces was deliberately left out of scope.

## Acceptance Criteria

- Phase 01 preserves legacy SQLite archive rows during schema repair and creates this initiative entry.
- Phase 02 ports the FTS operator-character search fix without regressing CJK search behavior.
- Phase 03 ports peak-context stats distribution semantics and documents the `claude_only` boundary decision.
- Phase 04 ports static SPA asset fallback/cache behavior.
- Phase 05 updates markdown rendering and DOMPurify with advisory evidence and rebuilt embedded frontend assets.
- Phase 06 creates `docs/UPSTREAM_AUDIT.md` covering all apply-failed fix/perf candidates with evidence-backed decisions.
- Phases 07-08 clear the pre-existing CI failures (Windows portability, lint, svelte-check, PostgreSQL integration, race).
- Phase 10 ports the A-tier CLI group (`0386ca3a`, `055aa770`, `9691d0fc`, `24300078`, `646a50c3`, `e588acf2`).
- Phase 11 ports the A-tier backend group (`87d00f8e`, `c8a326f2`, `7bcfa4b9`).
- Phase 12 ports the A-tier frontend group (`2d709437`, `e0e81238`, `64f4bf4f`, `b6594a76`, `9f8ee085`) without adopting `@kenn-io/kit-ui`.
- Phase 13 evaluates upstream Mermaid rendering (`5e702c8a`), whose upstream implementation depends on `@kenn-io/kit-ui`; this entry stays `WIP` until it lands or is explicitly dropped.
- Relevant Go tests, `go vet`, Go formatting, frontend unit tests, `svelte-check`, the Vite build, Playwright end-to-end specs, and phase-specific verification commands pass before delivery.
