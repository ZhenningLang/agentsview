# UPSTREAM_BACKLOG — accepted upstream audit follow-ups

Source ledger: `docs/UPSTREAM_AUDIT.md`. This file contains only records whose ledger decision is `adopt` and whose delivery is `backlog`; landed adopts and `defer` records are intentionally absent.

### `398adee5a174d4668e4f6cd62c5ff40f5308915a` — `fix(db): reuse FTS-safe large-session deletes (#801)`
- **Category:** `archive-schema`
- **Priority:** `P0`
- **Gap / evidence:** Verified backlog gap: `internal/db/session_batch.go:463` still defines `deleteSessionMessagesTx`, and `internal/db/session_batch.go:478` deletes from `messages` directly instead of a shared FTS-safe helper.
- **Proposed acceptance:** Large-session replace/delete/restore paths use one FTS-safe delete helper; regression tests cover batch replace, direct delete, trash, and restore.
- **Dependencies / risks:** SQLite FTS5 trigger behavior and archive data safety.
- **Review after:** `2026-09-12`

### `d37ddc9aa119d4d75d1d2ac35acf78f027881bcf` — `fix(sync): harden incremental JSONL resume state (#800)`
- **Category:** `sync-correctness`
- **Priority:** `P1`
- **Gap / evidence:** Verified backlog gap: local has `LastClaudeMessageID`/fallback behavior but no persisted `last_entry_uuid` or `next_ordinal` boundary; local anchors `internal/db/messages.go:465` and `internal/sync/engine.go:3982`.
- **Proposed acceptance:** Interrupted JSONL incremental sync resumes from persisted UUID/ordinal boundary without duplicates or skips.
- **Dependencies / risks:** Parser/sync state migration; verify Claude/Codex fallback parity.
- **Review after:** `2026-09-12`

### `7388b312a4e48b82471f3c2b44a7cb57195993c1` — `fix(db): tolerate NULL message timestamps in velocity analytics (#705)`
- **Category:** `archive-schema`
- **Priority:** `P1`
- **Gap / evidence:** Verified backlog gap: `internal/db/analytics.go:1991` selects raw nullable `timestamp` into `ts string` at `internal/db/analytics.go:2009`; only `model` has `COALESCE`.
- **Proposed acceptance:** Velocity analytics tolerate NULL timestamps in SQLite and preserve PG/DuckDB observable parity.
- **Dependencies / risks:** Analytics parity fixtures across supported stores.
- **Review after:** `2026-09-12`

### `1139b078e712574f2244f48e16c146e99d46d417` — `fix(parser): support new .kimi-code session layout and add .kimi_openclaw to OpenClaw defaults (#665)`
- **Category:** `active-agent`
- **Priority:** `P2`
- **Gap / evidence:** Verified split finding: local Kimi Code support exists at `internal/parser/types.go:318`, but OpenClaw defaults at `internal/parser/types.go:285` include only `.openclaw/agents`, not `.kimi_openclaw/agents`.
- **Proposed acceptance:** OpenClaw discovery includes `.kimi_openclaw/agents` only if product scope accepts it, with no Kimi Code regression.
- **Dependencies / risks:** Agent discovery naming boundary.
- **Review after:** `2026-09-12`

### `6407da6cfb9edccae01f73f173055fb2ff11eb05` — `Reject archives from newer agentsview binaries (#808)`
- **Category:** `archive-schema`
- **Priority:** `P0`
- **Gap / evidence:** Verified companion gap: `internal/db/db.go:424` reads `user_version`, but `internal/db/db.go:433` rejects future versions only when schema repair is needed; existing test `internal/db/db_test.go:683` allows future versions to reopen.
- **Proposed acceptance:** Opening a schema-current SQLite archive with a future data version, and comparable PG/CLI paths, returns an actionable refusal without modifying archive contents.
- **Dependencies / risks:** Schema-contract change; preserve phase 01 legacy repair and backend parity.
- **Review after:** `2026-09-12`

### `7ee9e4e15b` — `feat(parse-diff): classify incremental-append skew and recommend a resync baseline (#970)`
- **Category:** `parse-diff-foundation`
- **Priority:** `P1`
- **Gap / evidence:** Phase 14 re-read the upstream dependency chain and confirmed this commit is not a standalone 14-file schema feature. It depends on the parse-diff subsystem introduced by upstream `4592129b`, then extended by `b96075cf` and `e359fbc0`. This fork has no local parse-diff subsystem, so adding only `last_write_incremental` would create dead schema with no classifier/report/CLI consumer.
- **Proposed acceptance:** Adopt parse-diff as its own foundation slice first, including usable CLI/report output, live-write race precedence, synthetic corpus coverage, and incremental-skew classification; only then add `last_write_incremental` marker semantics.
- **Dependencies / risks:** Requires parse-diff foundation; migration cost is materially larger than the feature ledger's original `conflict + 14 files` surface.
- **Review after:** `2026-09-13`

### `1b5124bfdf3d3247c90522d30c3d5eb66080408f` — `fix(docs): scrub screenshot fixtures and sidebar trees (#1030)`
- **Category:** `archive-schema`
- **Priority:** `P1`
- **Gap / evidence:** Reviewer-verified local gap: upstream changes production sidebar tree filtering in `internal/db/query_dialect.go`, `internal/db/sessions.go`, and `internal/postgres/sessions.go`. Local recursive CTE at `internal/db/query_dialect.go:281` expands descendants without an automation predicate, and `grep` found no `automationScopePredicate` equivalent.
- **Proposed acceptance:** Sidebar tree queries apply automation scope to recursive descendants in SQLite and PostgreSQL, with regression tests proving automated subagent/fork/continuation rows cannot re-enter the default paged sidebar.
- **Dependencies / risks:** Query-shape parity across SQLite and PostgreSQL; avoid regressing existing orphan/fake-root filters.
- **Review after:** `2026-09-12`

## Ledger v1 semantic accuracy follow-up

### Ledger v1 semantic accuracy follow-up
- **Source:** `docs/UPSTREAM_AUDIT.md` spot check metadata.
- **Observed accuracy:** reviewer B sampled 12 records on 2026-08-13 and found 3 errors, a 25% error rate.
- **Reliability limitation:** natural-language decisions have not reached the per-record reliable audit standard. Category, Evidence wording, and Revisit trigger are partially generated from path buckets; an incorrect bucket can make all three wrong together.
- **Reason:** this v1 ledger combines manual review with path-based conservative triage; it is useful as a committed coverage baseline but has not received full manual semantic review.
- **Priority:** `P0`
- **Proposed acceptance:** before the next upstream increment, perform full semantic re-review of all 187 primary records, record per-SHA conclusions, recompute `spot_check_error_rate`, and fix any failed cohort before using the ledger as a scheduling source.
- **Review after:** `2026-09-12`

## Feature ledger v1 per-record audit

### Feature ledger v1 per-record audit
- **Source:** `docs/UPSTREAM_FEATURE_LEDGER.md` spot check metadata.
- **Observed accuracy:** reviewer A sampled 18 of 145 records and found 1 error (5.6%); reviewer B sampled 20 and found 4 (20%). Both sets of reported errors were fixed in place.
- **Reliability limitation:** the mechanical layer is complete and reproducible — candidate set, value/excluded split, and every SHA come from the recorded `candidate_command`. The judgment columns (`What the diff does`, `Recommendation`, `Reason`) were audited only on the sampled subset, so the unsampled remainder carries the sampled error rate as its expected accuracy. The initiative's acceptance asked for a per-record reliable audit of all 86 value records; that standard is not met.
- **Reason:** the ledger was produced to decide what to schedule next, and the A tier it fed was verified by reading each of the 15 upstream diffs during the port. The remaining records have not had that treatment. A bulk re-run is deliberately not being done as part of this delivery: an unsampled re-review would carry the same unmeasured uncertainty it is meant to remove.
- **Priority:** `P1`
- **Proposed acceptance:** before B tier is scheduled, re-read the upstream diff for every record that B tier draws from and correct its `Recommendation` / `Reason` in place; re-sample at least 20 untouched records afterwards and record the new error rate in the ledger metadata. A record may not enter a batch on the strength of its ledger row alone.
- **Review after:** `2026-09-13`
