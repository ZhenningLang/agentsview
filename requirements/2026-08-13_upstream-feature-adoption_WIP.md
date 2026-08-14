# Upstream Feature Adoption

## Goal

Adopt user-relevant **features** (`feat` commits) from upstream
`kenn-io/agentsview` into this fork by semantic port.

This is deliberately separate from `2026-08-12_upstream-catchup_DONE.md`, which
covers the earlier correctness / data-safety / CI rounds over `fix` and `perf`
commits. That initiative is closed; this one carries the feature backlog that
was opened afterwards and is still in progress.

## SSOT

- Ledger: `docs/UPSTREAM_FEATURE_LEDGER.md` — the audited classification of every
  upstream `feat` commit in `56218f22..ba60e2ce`. It is the SSOT for what is a
  candidate, what was excluded, and why.
- Fix/perf ledger for the prior initiative: `docs/UPSTREAM_AUDIT.md`.
- Long-run workspace (process artifacts, not in-repo):
  `/Users/zhenninglang/Projects/agentsview/.long-loop/20260812_upstream-catchup`
  (`REQUIREMENT.md`, `SPEC_OVERVIEW.md`, `fix_plan.md`, `phases/*/spec.md`).

Ledger totals as measured: 145 candidates, 86 value records
(49 `want`, 36 `neutral`, 1 `skip`), 59 excluded, 0 unclassified.

## Locked Decisions

- **No architecture migration.** The parser provider refactor is not adopted as
  a prerequisite. Of the 49 `want` records, 41 have no architecture dependency;
  the remaining 8 (4 `kit-ui`, 3 `provider`, 1 `provider+kit-ui`) are either
  reworked on local architecture or deferred, never adopted by pulling the
  upstream architecture in.
- **`internal/recall` is not adoptable.** The fork's own memory/synthesize
  system is coupled to skills and hooks that live outside this repository, so
  recall would be a competing substrate rather than an upgrade. This is a
  compatibility judgment, not a quality judgment about the upstream code. All
  `feat(recall)` commits and recall-dependent features are excluded in the
  ledger.
- **Semantic port, never cherry-pick.** 84 of the 86 value records fail
  `git apply --check` against local HEAD; only 2 apply cleanly. Every adoption
  is re-implemented against local architecture after reading the upstream diff.
- **Do not introduce `@kenn-io/kit-ui`.** User decision, 2026-08-14. Upstream
  features that depend on it are re-implemented on their underlying library
  directly — Mermaid rendering (`5e702c8a`) landed on the official
  `mermaid@11.16.1` npm package instead.
- Work proceeds on `lr/atier-features`, not `main`.
- The persistent SQLite archive is never deleted, truncated, or recreated; the
  constraints from the prior initiative continue to apply.

## Progress

- **A tier — complete (15 commits, phases 10-13).**
  - Phase 10 CLI: `0386ca3a`, `055aa770`, `9691d0fc`, `24300078`, `646a50c3`,
    `e588acf2`
  - Phase 11 backend: `87d00f8e`, `c8a326f2`, `7bcfa4b9`
  - Phase 12 frontend: `2d709437`, `e0e81238`, `64f4bf4f`, `b6594a76`,
    `9f8ee085`
  - Phase 13 Mermaid: `5e702c8a`
- **B tier — 31 candidates queued, not yet scheduled.** Tier assignment is a
  scheduling decision of this run and is not recorded in the ledger; the ledger
  records value and cost, not batch order.

## Open Decisions

- B-tier batching: which of the 31 remaining candidates go together, and whether
  the large ones (for example `4b736d5c12` activity dashboard, 104 files) become
  standalone initiatives instead.
- The 8 architecture-dependent `want` records still need a per-record call:
  rework on local architecture, or defer to a time-boxed provider-migration
  spike.

## Known Side Effects

- `c8a326f2` (skill-name inference from `SKILL.md` reads) bumps the SQLite data
  version from 39 to 40 (`internal/db/db.go`). The first `Open()` on an existing
  archive therefore triggers a **full resync** — roughly 14459 sessions and
  ~680k messages on this machine. The archive is not deleted: this takes the
  non-destructive `dataStale` path (build fresh DB, copy orphaned sessions,
  atomic swap), which the Phase 01 archive-drop guard hardened and verified.

## Acceptance Criteria

- Every adopted commit has a regression test that fails before the port and
  passes after it.
- Full CI green: Go tests, `go vet`, Go formatting, frontend unit tests,
  `svelte-check`, the Vite build, and Playwright end-to-end specs.
- The real archive `~/.agentsview/sessions.db` keeps its inode across the work;
  no phase writes, deletes, or migrates it in place.
- `docs/UPSTREAM_FEATURE_LEDGER.md` stays consistent with what actually landed,
  and boundary decisions from each phase are recorded before delivery.
</content>
</invoke>
