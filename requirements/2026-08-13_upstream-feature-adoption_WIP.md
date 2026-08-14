# Upstream Feature Adoption

## Goal

Adopt user-relevant **features** (`feat` commits) from upstream
`kenn-io/agentsview` into this fork by semantic port.

This is deliberately separate from `2026-08-12_upstream-catchup_DONE.md`, which
covers phases 01-08: the correctness / data-safety rounds over `fix` and `perf`
commits plus the CI cleanup that followed. That initiative is closed. The
feature work (phases 09-13, and every later tier) belongs here, including the
phase 10-13 boundary decisions that were originally recorded in the closed
entry and have since been moved into this file.

## SSOT

- **This file is the SSOT for feature adoption from upstream** — A tier, B tier,
  and anything scheduled after them. Scope, progress, boundary decisions, and
  acceptance for phases 09-13 live here, not in the closed fix/perf entry.
- Ledger: `docs/UPSTREAM_FEATURE_LEDGER.md` — the classification of every
  upstream `feat` commit in `56218f22..ba60e2ce`. It is the SSOT for what is a
  candidate, what was excluded, and why. Its accuracy limits are stated in its
  own header; see the Known Limitations note there before using it as a
  scheduling source.
- Fix/perf ledger for the prior initiative: `docs/UPSTREAM_AUDIT.md`.
- Accepted follow-ups from both ledgers: `docs/UPSTREAM_BACKLOG.md`.
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

- **A tier — complete (15 upstream targets, phases 10-13).** These are upstream
  commit targets, not local commit counts; the local branch landed them in a
  different number of commits.
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
- **Mixed-locale menus (needs a user call).** Phase 12 added the first localized
  strings to the AppHeader export menus and the shortcuts modal, which are
  otherwise hard-coded English. Under the default `zh` locale a Chinese row now
  sits next to English neighbours — for example `AppHeader.svelte:693`
  `Copy markdown export link` above `:703` `t("header.copySourcePath")`. Phase 13
  added two more hard-coded English strings on the failure path
  (`highlight-fences.ts:127`, `MermaidBlock.svelte:101-102`). The two options are
  localizing the surrounding surfaces (roughly 25 strings) or reverting the new
  labels to hard-coded English. This is the only delivery defect a user sees on
  first open; it is tracked as P12-2 in the run BACKLOG and is raised here
  because it needs a decision rather than another round of deferral.

## Known Side Effects

- `c8a326f2` (skill-name inference from `SKILL.md` reads) bumps the SQLite data
  version from 39 to 40 (`internal/db/db.go`). The first `Open()` on an existing
  archive therefore triggers a **full resync** — roughly 14459 sessions and
  ~680k messages on this machine, so first startup after this delivery takes a
  visible amount of time proportional to archive size. The archive is not
  deleted: this takes the non-destructive `dataStale` path (build fresh DB, copy
  orphaned sessions, atomic swap via `internal/sync/engine.go` `os.Rename`),
  which the Phase 01 archive-drop guard hardened and verified.
- Nothing in this initiative reaches a running install by itself. The embedded
  SPA under `internal/web/dist/` is a build product and is not tracked by Git,
  so Mermaid, the DOMPurify bump, and every phase 12/13 frontend change require
  `make build` plus a redeploy before they exist in `/Applications/AgentsView.app`.
  Deployment is an explicit non-goal of this run.

## Boundary Decisions

- Phase 12 made `generateFallbackContent` render a pre-computed `patch` /
  `patch_text` / `patchText` parameter for Edit-shaped tool calls. This changes
  the displayed tool input for those calls from a single `patch: <blob>`
  key-value line to the patch body itself, and the same formatter (uncapped)
  feeds the new copy button. No parser, schema, or API change.
- Phase 12 gave the Pi `edits[]` display path the 200-line cap (`MAX_DIFF_LINES`)
  it previously lacked. Before this phase that branch returned its lines
  uncapped, so a large kilo/Pi edit rendered every line into the DOM; every other
  tool shape already honoured the cap. Measured on a 500-line single-entry edit:
  402 rendered lines with no marker before, 201 lines ending in
  `... (402 lines total)` after. The copy path is unaffected and still returns
  all 501 lines, so no content is lost to the user — only the on-screen preview
  is bounded. The per-edit 400-character truncation is likewise now skipped in
  copy mode while display behavior at 400 characters is unchanged.
- Phase 12 added `parser_malformed_lines` to the hand-written `Session` type
  only. The field was already serialized by the API and present in the generated
  models; no backend, schema, or generated-model change.
- Phase 12's Calls disclosure defaults to expanded, and missing, unparseable, or
  unavailable LocalStorage also falls back to expanded, so existing users keep
  today's behavior until they collapse it.
- Phase 12's new labels are the first localized strings in the AppHeader export
  menus and the shortcuts modal, which are otherwise hard-coded English. Under
  the default `zh` locale those specific rows render Chinese next to English
  neighbours; localizing the surrounding surfaces was deliberately left out of
  scope. See Open Decisions — this is now waiting on a user call rather than
  silently accepted.
- Phase 13 adds the first runtime dependency of this initiative: exact
  `mermaid@11.16.1` from `registry.npmjs.org`. `11.16.1` is the patched version
  for GHSA-c4c3-pg64-4m4v (affected `>=11.0.0-alpha.1, <11.16.1`), so upstream's
  own `11.15.0` was not copied. Upstream's `a18b57d8` Mermaid security bump was
  recorded as not-applicable in `docs/UPSTREAM_AUDIT.md` because the fork had no
  Mermaid dependency at the time; that snapshot is left as-is and this entry
  carries the current state.
- Phase 13 treats diagram source from archived transcripts as untrusted input.
  Mermaid is initialized with a static trusted config (`securityLevel: "strict"`,
  `startOnLoad: false`, `suppressErrorRendering: true`, `maxTextSize: 50000`,
  `maxEdges: 500`, explicit `secure` keys), diagram source never reaches the
  config API, `bindFunctions` is never called, and the returned SVG goes through
  a second app-owned `DOMPurify.sanitize` with no
  `ADD_TAGS`/`ADD_ATTR`/hook/`IN_PLACE`/`CUSTOM_ELEMENT_HANDLING` relaxation
  before it enters the DOM.
- Phase 13 pins Mermaid's `htmlLabels: false` and locks the theming keys, which
  has two accepted user-visible costs. A diagram's own `theme` / `themeCSS` /
  `themeVariables` / `darkMode` / font directives are now silently ignored, so a
  transcript that wrote `%%{init: {'theme':'forest'}}%%` renders in the app theme
  with no indication the directive was dropped. And markdown inside a node label
  (`A["**Bold**"]`) renders literally instead of being formatted, because native
  SVG `<text>` has no inline markup. Both are accepted: the alternative is
  letting untrusted transcript content restyle the page, or allowing
  `foreignObject` through the sanitizer.
- Phase 13 keeps the plain source code block whenever an in-session search query
  is active, on both the message code-segment path and the shared Markdown fence
  action, because rendered SVG text is not a stable oracle for search marks.
- Phase 13 loads Mermaid through a single dynamic `import("mermaid")`. The Vite
  manifest shows the runtime is reachable only as a dynamic import of the entry,
  and the entry chunk grows by roughly 4 KB raw / 2 KB gzip — app glue only, no
  runtime. Exact byte counts are deliberately not recorded here: they change with
  every build and the phase verifier already cross-checks them against a freshly
  built manifest, so a copy in this entry can only go stale.

## Acceptance Criteria

- Every adopted commit has a regression test that fails before the port and
  passes after it.
- Full CI green: Go tests, `go vet`, Go formatting, frontend unit tests,
  `svelte-check`, the Vite build, and Playwright end-to-end specs.
- No phase in this run writes, deletes, or migrates the real archive
  `~/.agentsview/sessions.db`; every read is read-only and every write test runs
  against `t.TempDir()` or a copy. This is a development-time invariant only:
  the first post-delivery `Open()` legitimately rebuilds and swaps the archive
  through the hardened `dataStale` resync path, which replaces the file and
  therefore its inode. See Known Side Effects.
- `docs/UPSTREAM_FEATURE_LEDGER.md` records, in its header, which upstream
  targets have actually landed, so the remaining candidate set can be computed
  from the ledger alone.
- The run acceptance runner asserts the A-tier surface, not just the earlier
  fix/perf surface: the feature ledger and this requirement entry are tracked,
  the Mermaid version is pinned in both `package.json` and the lockfile, and the
  15 A-tier upstream SHAs are present in the ledger.
