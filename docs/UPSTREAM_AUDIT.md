# UPSTREAM_AUDIT — apply-failed fix/perf ledger

## Metadata
- **fork_point:** `56218f22d552b339b6ecdca2e5beaa8161485f69`
- **upstream_head:** `ba60e2ce41646895d3a45e9bed48b9b2ce66876d`
- **local_head:** `3ebf2fad80ba03d7554da587eacadba1072cf7e4`
- **audited_at:** `2026-08-13T01:55:00+08:00`
- **last_audited_upstream_sha:** `ba60e2ce41646895d3a45e9bed48b9b2ce66876d`
- **candidate_algorithm_version:** `apply-failed-fix-perf-v1`
- **subject_regex:** `^(fix|perf)(\([^)]*\))?!?(:[[:space:]]+|[[:space:]]+)`
- **primary_scope:** `apply-failed fix/perf commits only`
- **subject_matches:** `206`
- **apply_clean:** `19`
- **apply_failed:** `187`
- **empty_commits:** `0`
- **binary_patches:** `1`
- **spot_check_sample:** `12`
- **spot_check_errors:** `3`
- **spot_check_error_rate:** `25%`
- **spot_check_reviewer_date:** `phase_reviewer_b, 2026-08-13`

## Current Status
- **Status:** `v1 reviewed with known residual accuracy risk`.
- **Semantically reviewed primary records currently written:** `187`.
- This v1 ledger has not received a full manual semantic review. Round-2 reviewer spot-check sampled 12 records and found 3 errors (25%); the quality follow-up is recorded in `docs/UPSTREAM_BACKLOG.md`.

## Scope And Candidate Contract
- Merge commits are excluded with `git log --no-merges`; this range currently has zero merges, but the rule is explicit for the next audit.
- Revert/backout commits are not included by identity. They enter only when their subject matches the exact fix/perf regex; related reverts and follow-ups are still lineage evidence for individual records.
- Empty commits with matching subjects are counted in `empty_commits` and excluded from primary records; this run has zero.
- Binary patches are not excluded. `git format-patch --binary --full-index` is used before `git apply --cached --check`; this run has one binary patch and it is apply-clean, so it is outside primary scope.
- `git apply --check` is only a port-cost signal. It is not a value filter and must not be used to dismiss high-value conflicted fixes.
- Primary scope is apply-failed fix/perf commits only. Apply-clean fix/perf commits are outside this primary ledger; the prior apply-clean triage context is in `/Users/zhenninglang/Projects/agentsview/.long-loop/20260812_review-brief/review_a.md`, `/Users/zhenninglang/Projects/agentsview/.long-loop/20260812_review-brief/review_b.md`, and this run's phase 03/04/05 landed records. Because `.long-loop/` is not committed, the next audit should promote those decisions into a committed complete upstream view.
- Apply-clean and non-fix/perf high-risk commits remain explicit scope holes. Next audit should use `last_audited_upstream_sha` for incrementality and evaluate whether to merge apply-clean, apply-failed, and non-fix/perf high-risk commits into one complete upstream view.
- Local actual-agent distribution used for prioritization comes from the phase spec: claude 5983, droid 3890, kilo 3539, vscode-copilot 607, openclaw 101, kimicode 67, opencode 31, copilot 10, codex 3.

## Generation Command
```bash
python3 - <<'PY'
import os, re, subprocess, tempfile
fork = '56218f22d552b339b6ecdca2e5beaa8161485f69'
upstream = 'ba60e2ce41646895d3a45e9bed48b9b2ce66876d'
local_head = '3ebf2fad80ba03d7554da587eacadba1072cf7e4'
subject_re = re.compile(r'^(fix|perf)(\([^)]*\))?!?(:[\s]+|[\s]+)', re.I)
for line in subprocess.check_output(['git','log','--no-merges','--format=%H%x00%s', f'{fork}..{upstream}'], text=True).splitlines():
    sha, subject = line.split('\0', 1)
    if not subject_re.search(subject):
        continue
    names = subprocess.check_output(['git','diff-tree','--root','--no-commit-id','--name-only','-r', sha], text=True).splitlines()
    if not names:
        print(sha, subject, 'empty', 'text', sep='\t')
        continue
    patch = subprocess.check_output(['git','format-patch','--stdout','--binary','--full-index','-1', sha])
    binary = 'binary' if b'GIT binary patch' in patch or b'Binary files ' in patch else 'text'
    fd, idx = tempfile.mkstemp(prefix='agentsview-upstream-audit-')
    os.close(fd)
    os.unlink(idx)
    env = os.environ.copy()
    env['GIT_INDEX_FILE'] = idx
    subprocess.check_call(['git','read-tree', local_head], env=env, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    result = subprocess.run(['git','apply','--cached','--check'], input=patch, env=env, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    try:
        os.unlink(idx)
    except FileNotFoundError:
        pass
    print(sha, subject, 'clean' if result.returncode == 0 else 'failed', binary, sep='\t')
PY
```

## Primary Findings

### [primary] `ba60e2ce41646895d3a45e9bed48b9b2ce66876d` — `fix(vector): recover invalid Ollama embeddings on CPU (#1378)`
- **Category:** `archive-schema`
- **Decision:** `defer`
- **Evidence:** Defer platform/CLI/server behavior: `git show --name-status ba60e2ce41646895d3a45e9bed48b9b2ce66876d` touches `cmd/agentsview/embeddings.go`, `cmd/agentsview/embeddings_test.go`, `docs/semantic-search.md`, `docs/superpowers/plans/2026-07-29-request-pricing-bands.md`, `docs/superpowers/plans/2026-07-30-sidebar-toggle-placement.md`, ... (16 paths total); local counterpart/equivalent surface starts at `internal/config/config.go:635`. Needs a scoped acceptance test before adoption.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `A local archive, stats, usage, search, pricing, config, server, or schema bug maps to this upstream lineage.`

### [primary] `ced46761734b5e6e4c6f9ee76064c66706fff0f8` — `fix(frontend): keep root sessions landing unfiltered (#1368)`
- **Category:** `ui`
- **Decision:** `defer`
- **Evidence:** Defer UI behavior change: `git show --name-status ced46761734b5e6e4c6f9ee76064c66706fff0f8` touches `frontend/src/App.svelte`, `frontend/src/App.test.ts`, `frontend/src/lib/components/analytics/AnalyticsPage.svelte`, `frontend/src/lib/components/analytics/AnalyticsPage.test.ts`, `frontend/src/lib/components/analytics/TopSessions.test.ts`, ... (11 paths total); local counterpart/equivalent surface starts at `frontend/src/App.svelte:325`. Needs frontend tests or visual review before adoption.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `User-facing navigation/filter/rendering issue is reproduced or requested in this fork UI.`

### [primary] `e00959e36a172e09e0d386527e94209fca882e11` — `fix(parser): trim replayed prefix of backgrounded Claude sessions (#1373)`
- **Category:** `archive-schema`
- **Decision:** `defer`
- **Evidence:** Defer archive/schema/data behavior change: `git show --name-status e00959e36a172e09e0d386527e94209fca882e11` touches `docs/internal/session-format-sources.md`, `internal/db/db.go`, `internal/db/db_test.go`, `internal/parser/claude.go`, `internal/parser/claude_lineage.go`, ... (7 paths total); local counterpart/equivalent surface starts at `internal/db/db.go:21`. Needs data-preservation and backend-parity tests before adoption.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `A local archive, stats, usage, search, pricing, or schema bug maps to this upstream lineage.`

### [primary] `001dd6bd2bff5c583101726dd5d99dd1e62c05eb` — `fix(sync): stop probing macOS protected folders during discovery (#1366)`
- **Category:** `macos-runtime`
- **Decision:** `defer`
- **Evidence:** Defer macOS runtime/discovery fix: `git show --name-status 001dd6bd2bff5c583101726dd5d99dd1e62c05eb` touches `cmd/agentsview/archive_query_backend.go`, `cmd/agentsview/archive_write_backend.go`, `cmd/agentsview/main.go`, `cmd/agentsview/parse_diff.go`, `cmd/agentsview/session_sync.go`, ... (20 paths total); local counterpart exists at `cmd/agentsview/main.go:14`. Needs a focused macOS filesystem/watch regression, not a ledger-only change.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `A macOS protected-directory, watcher, or media-folder failure is reproduced locally.`

### [primary] `14992d32da35a40c666eaaf3fa54a3d59f54d25f` — `fix(opencode): keep the session container off the recursive watch budget (#1355)`
- **Category:** `sync-correctness`
- **Decision:** `defer`
- **Evidence:** Defer sync/backend correctness change: `git show --name-status 14992d32da35a40c666eaaf3fa54a3d59f54d25f` touches `cmd/agentsview/main_test.go`, `internal/parser/opencode_coverage_units_test.go`, `internal/parser/opencode_provider.go`, `internal/parser/opencode_provider_test.go`, `internal/sync/watch_backend.go`, ... (9 paths total); local counterpart/equivalent surface starts at `cmd/agentsview/main_test.go:67`. Needs backend parity or sync regression tests before adoption.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `A local sync, watcher, PG/DuckDB, daemon, or remote-sync regression maps to this upstream lineage.`

### [primary] `a18b57d8efd7eb1bbadb7e6513153e0afea7ac46` — `fix(deps): update dependency mermaid to v11.16.1 [security] (#1357)`
- **Category:** `security`
- **Decision:** `not-applicable`
- **Evidence:** Not applicable: upstream bumps Mermaid from an existing Mermaid dependency, but this fork has no Mermaid dependency. `frontend/package.json:1` has dependencies for lucide, virtual-core, dompurify, marked, and shiki; `frontend/package-lock.json` has zero `mermaid` matches.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `none`

### [primary] `7d3b4805e5e898366d67b8363ea038d50177cd38` — `fix(deps): update dependency dompurify to v3.4.13 [security] (#1359)`
- **Category:** `security`
- **Decision:** `adopt`
- **Evidence:** Phase 05 landed dompurify exact 3.4.13: `3ebf2fad80ba03d7554da587eacadba1072cf7e4`. Local anchor `frontend/package.json:1`; `docs/dompurify-3.4.13-advisories.md` records first-hand advisory review.
- **Port cost:** `conflict`
- **Delivery:** `landed:3ebf2fad80ba03d7554da587eacadba1072cf7e4/phase-05`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `none`

### [primary] `91e14328bcf6b0a72aa818631f741f84074558b6` — `Fix Kimi Work tool-step usage and K2.6 pricing (#1338)`
- **Category:** `active-agent`
- **Decision:** `defer`
- **Evidence:** Round-1 correction: this is applicable to fork Kimi paths, not `not-applicable`. Local Kimi Code support exists at `internal/parser/kimicode.go:15`; local tree has no `k2d6-agent` or K2.6 canonicalization match in `internal/pricing/supplemental.go`.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `Re-evaluate with Kimi Work/K2.6 usage fixtures and pricing parity tests.`

### [primary] `e51f68bb0e7b6fc01b42ca44b8f7642586cce823` — `Fix nested subagent hierarchy: re-parent depth>=2 subagents to their spawner (#1320)`
- **Category:** `sync-correctness`
- **Decision:** `defer`
- **Evidence:** Defer sync/backend correctness change: `git show --name-status e51f68bb0e7b6fc01b42ca44b8f7642586cce823` touches `internal/db/db.go`, `internal/db/db_test.go`, `internal/db/legacy_schema_test.go`, `internal/db/link_subagent_nested_test.go`, `internal/db/orphaned.go`, ... (17 paths total); local counterpart/equivalent surface starts at `internal/db/db.go:103`. Needs backend parity or sync regression tests before adoption.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `A local sync, watcher, PG/DuckDB, daemon, or remote-sync regression maps to this upstream lineage.`

### [primary] `deff98ae37936d83a57209ff600c4269b19d92ac` — `fix(sync): bound agent-scoped reconciliation to its requested roots (#1319)`
- **Category:** `sync-correctness`
- **Decision:** `defer`
- **Evidence:** Defer platform/CLI/server behavior: `git show --name-status deff98ae37936d83a57209ff600c4269b19d92ac` touches `docs/internal/session-format-sources.md`, `internal/db/sessions.go`, `internal/db/stored_source_scope_test.go`, `internal/db/virtual_member_freshness_test.go`, `internal/parser/capabilities_sync_test.go`, ... (32 paths total); local counterpart/equivalent surface starts at `internal/db/sessions.go:30`. Needs a scoped acceptance test before adoption.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `A local sync, watcher, PG/DuckDB, daemon, or remote-sync regression maps to this upstream lineage.`

### [primary] `782b2c9584c62dd2715ece1c9989f7f31e2f62d8` — `fix(sync): suppress OpenCode SHM-only daemon watch pushes (#1321)`
- **Category:** `sync-correctness`
- **Decision:** `defer`
- **Evidence:** Provider-facade path absence is not enough to reject this behavior. Local fork still treats OpenCode SQLite container sidecar events as dirty: `internal/sync/engine.go:1285` matches `opencode.db-*`, and `cmd/agentsview/pg_watch.go:258` notifies dirty after `SyncPaths`. This is a live Kilo/OpenCode-family path, so defer to a sync/provider compatibility phase.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `Re-evaluate with an OpenCode/Kilo `opencode.db-shm` watch event fixture and PG push dirty-notification regression.`

### [primary] `5bc9617c701b3fbde8f204c26bcb8c7f1fddaf52` — `fix(devin): treat session timestamps as epoch seconds (#1322)`
- **Category:** `sync-correctness`
- **Decision:** `defer`
- **Evidence:** Defer sync/backend correctness change: `git show --name-status 5bc9617c701b3fbde8f204c26bcb8c7f1fddaf52` touches `docs/internal/session-format-sources.md`, `internal/db/db.go`, `internal/db/db_test.go`, `internal/parser/devin.go`, `internal/parser/devin_provider.go`, ... (11 paths total); local counterpart/equivalent surface starts at `internal/db/db.go:78`. Needs backend parity or sync regression tests before adoption.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `A local sync, watcher, PG/DuckDB, daemon, or remote-sync regression maps to this upstream lineage.`

### [primary] `7ddf624cc7aa142d16d05aca26b325aee9117446` — `fix(daemon): scope degraded-coverage polling to one provider per pass (#1307)`
- **Category:** `macos-runtime`
- **Decision:** `defer`
- **Evidence:** Defer macOS runtime/discovery fix: `git show --name-status 7ddf624cc7aa142d16d05aca26b325aee9117446` touches `cmd/agentsview/archive_write_backend.go`, `cmd/agentsview/archive_write_backend_test.go`, `cmd/agentsview/main.go`, `cmd/agentsview/main_test.go`, `cmd/agentsview/poll_coordinator_scope_test.go`, ... (20 paths total); local counterpart exists at `cmd/agentsview/main.go:110`. Needs a focused macOS filesystem/watch regression, not a ledger-only change.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `A macOS protected-directory, watcher, or media-folder failure is reproduced locally.`

### [primary] `7449a0140b5360c9b915d540a72a8c29e2a58195` — `fix(frontend): move sidebar toggle beside filters (#1312)`
- **Category:** `ui`
- **Decision:** `defer`
- **Evidence:** Defer UI behavior change: `git show --name-status 7449a0140b5360c9b915d540a72a8c29e2a58195` touches `cmd/agentsview/pricing_schedule_test.go`, `docs/superpowers/plans/2026-07-30-sidebar-toggle-placement.md`, `frontend/messages/en.json`, `frontend/messages/fr.json`, `frontend/messages/ko.json`, ... (23 paths total); local counterpart/equivalent surface starts at `frontend/src/lib/components/analytics/AnalyticsPage.svelte:59`. Needs frontend tests or visual review before adoption.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `User-facing navigation/filter/rendering issue is reproduced or requested in this fork UI.`

### [primary] `57075427c2663c2871d5daeea09aac3b20b61768` — `fix(signals): treat raw terminal API errors as errored outcomes (#1275)`
- **Category:** `sync-correctness`
- **Decision:** `defer`
- **Evidence:** Defer sync/backend correctness change: `git show --name-status 57075427c2663c2871d5daeea09aac3b20b61768` touches `internal/db/sessions.go`, `internal/signals/outcome.go`, `internal/signals/outcome_terminal_api_error_test.go`, `internal/sync/recompute_memory_test.go`; local counterpart/equivalent surface starts at `internal/db/sessions.go:42`. Needs backend parity or sync regression tests before adoption.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `A local sync, watcher, PG/DuckDB, daemon, or remote-sync regression maps to this upstream lineage.`

### [primary] `3205da842e770d42bd1d286bd8532d3f10331fd6` — `fix(copilot): use execution events for tool timing (#1290)`
- **Category:** `sync-correctness`
- **Decision:** `defer`
- **Evidence:** Defer sync/backend correctness change: `git show --name-status 3205da842e770d42bd1d286bd8532d3f10331fd6` touches `docs/internal/session-format-sources.md`, `internal/db/db.go`, `internal/db/db_test.go`, `internal/db/timing.go`, `internal/db/timing_test.go`, ... (14 paths total); local counterpart/equivalent surface starts at `internal/db/db.go:75`. Needs backend parity or sync regression tests before adoption.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `A local sync, watcher, PG/DuckDB, daemon, or remote-sync regression maps to this upstream lineage.`

### [primary] `9762387848333d8c2592227af8d14de61380df52` — `fix(deps): update javascript dependencies (#1300)`
- **Category:** `security`
- **Decision:** `defer`
- **Evidence:** Defer dependency maintenance: `git show --name-status 9762387848333d8c2592227af8d14de61380df52` touches dependency manifests `frontend/package-lock.json`, `frontend/package.json`; this needs a dedicated advisory/lockfile/build phase rather than phase 06 docs. Local anchor: `frontend/package-lock.json:2984`.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `Dependency update is requested or a security advisory affects the locked package range.`

### [primary] `277f126ff95095cec97f3d19a1076773059ffc90` — `fix: improve worktree path readability (#1311)`
- **Category:** `ui`
- **Decision:** `defer`
- **Evidence:** Defer UI behavior change: `git show --name-status 277f126ff95095cec97f3d19a1076773059ffc90` touches `cmd/testfixture/main.go`, `docs/superpowers/plans/2026-07-30-worktree-tooltip-width.md`, `docs/superpowers/specs/2026-07-30-worktree-tooltip-width-design.md`, `frontend/e2e/session-timing.spec.ts`, `frontend/src/lib/components/content/SessionVitals.svelte`, ... (6 paths total); local counterpart/equivalent surface starts at `cmd/testfixture/main.go:53`. Needs frontend tests or visual review before adoption.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `User-facing navigation/filter/rendering issue is reproduced or requested in this fork UI.`

### [primary] `958534bbb931b81c2ba11777846054c2da6afe29` — `fix(ci): restore cross-platform main checks (#1313)`
- **Category:** `sync-correctness`
- **Decision:** `defer`
- **Evidence:** Mixed CI plus sync change: `.github/workflows/ci.yml` exists locally, while upstream also touches absent `internal/sync/activity_hint_reader.go`. Because the commit includes sync production behavior, not just CI, defer to a scoped cross-platform sync/CI check instead of marking path absence.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `Re-evaluate when cross-platform CI or activity-hint reader behavior is in scope.`

### [primary] `d00ae43bff3755473a0ec19a1370f7d7d294efe4` — `fix(sync): preserve retries across repeated stale paths (#1310)`
- **Category:** `sync-correctness`
- **Decision:** `not-applicable`
- **Evidence:** Not applicable to the current fork because the upstream target path set is absent at frozen `local_head`; examples from `git show --name-only`: `internal/sync/live_activity.go`, `internal/sync/live_activity_test.go`. Verified absence anchor: path absent at frozen local_head: `internal/sync/live_activity.go`, `internal/sync/live_activity_test.go`.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `none`

### [primary] `d322d64cfb0fa9be73c3f75c2229c523e833340d` — `fix(pricing): apply LiteLLM request bands per request (#1305)`
- **Category:** `archive-schema`
- **Decision:** `defer`
- **Evidence:** Defer platform/CLI/server behavior: `git show --name-status d322d64cfb0fa9be73c3f75c2229c523e833340d` touches `cmd/agentsview/export.go`, `cmd/agentsview/export_sessions_test.go`, `cmd/agentsview/usage.go`, `cmd/agentsview/usage_test.go`, `docs/activity.md`, ... (70 paths total); local counterpart/equivalent surface starts at `cmd/agentsview/usage.go:16`. Needs a scoped acceptance test before adoption.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `A local archive, stats, usage, search, pricing, config, server, or schema bug maps to this upstream lineage.`

### [primary] `3c33e111579ac361a616a0030cbe93c5ea662a20` — `fix: keep active Codex sessions current (#1308)`
- **Category:** `macos-runtime`
- **Decision:** `defer`
- **Evidence:** Defer macOS runtime/discovery fix: `git show --name-status 3c33e111579ac361a616a0030cbe93c5ea662a20` touches `cmd/agentsview/live_activity.go`, `cmd/agentsview/live_activity_test.go`, `cmd/agentsview/main.go`, `docs/configuration.md`, `docs/internal/session-format-sources.md`, ... (16 paths total); local counterpart exists at `cmd/agentsview/main.go:56`. Needs a focused macOS filesystem/watch regression, not a ledger-only change.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `A macOS protected-directory, watcher, or media-folder failure is reproduced locally.`

### [primary] `5a12c42071d23223425df861c84cece1537467bb` — `fix(duckdb): preserve source machine attribution (#1302)`
- **Category:** `sync-correctness`
- **Decision:** `defer`
- **Evidence:** Defer sync/backend correctness change: `git show --name-status 5a12c42071d23223425df861c84cece1537467bb` touches `internal/duckdb/connect.go`, `internal/duckdb/probe.go`, `internal/duckdb/push.go`, `internal/duckdb/rebuild.go`, `internal/duckdb/schema.go`, ... (7 paths total); local counterpart/equivalent surface starts at `internal/duckdb/connect.go:1`. Needs backend parity or sync regression tests before adoption.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `A local sync, watcher, PG/DuckDB, daemon, or remote-sync regression maps to this upstream lineage.`

### [primary] `a421fe8d11022374ac42c36f4bab1aba80ca8b15` — `fix(pricing): refresh daemon catalog daily (#1285)`
- **Category:** `ui`
- **Decision:** `defer`
- **Evidence:** Defer platform/CLI/server behavior: `git show --name-status a421fe8d11022374ac42c36f4bab1aba80ca8b15` touches `cmd/agentsview/main.go`, `cmd/agentsview/pricing_schedule.go`, `cmd/agentsview/pricing_schedule_test.go`, `cmd/agentsview/usage.go`, `internal/pricingrefresh/refresh.go`, ... (6 paths total); local counterpart/equivalent surface starts at `cmd/agentsview/main.go:212`. Needs a scoped acceptance test before adoption.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `User-facing navigation/filter/rendering issue is reproduced or requested in this fork UI.`

### [primary] `b0b0553e85a94c5d4bbdb9959982d9344374affe` — `fix(duckdb): give local push-watch deferred scopes a polling owner (#1298)`
- **Category:** `sync-correctness`
- **Decision:** `defer`
- **Evidence:** Defer sync/backend correctness change: `git show --name-status b0b0553e85a94c5d4bbdb9959982d9344374affe` touches `cmd/agentsview/archive_write_backend.go`, `cmd/agentsview/archive_write_backend_test.go`, `cmd/agentsview/duckdb.go`; local counterpart/equivalent surface starts at `cmd/agentsview/duckdb.go:19`. Needs backend parity or sync regression tests before adoption.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `A local sync, watcher, PG/DuckDB, daemon, or remote-sync regression maps to this upstream lineage.`

### [primary] `dc0a980106bd7fd190ec46e9762888c124ff1d0a` — `fix(parser): attribute Git worktrees to repositories (#1294)`
- **Category:** `archive-schema`
- **Decision:** `defer`
- **Evidence:** Defer archive/schema/data behavior change: `git show --name-status dc0a980106bd7fd190ec46e9762888c124ff1d0a` touches `internal/db/db.go`, `internal/db/db_test.go`, `internal/parser/project.go`, `internal/parser/project_git_test.go`; local counterpart/equivalent surface starts at `internal/db/db.go:21`. Needs data-preservation and backend-parity tests before adoption.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `A local archive, stats, usage, search, pricing, or schema bug maps to this upstream lineage.`

### [primary] `a27aab7e4113a132a516c1885cdd0f5eab87bf4c` — `fix(cli): expand leading tilde in DuckDB mirror paths (#1274)`
- **Category:** `sync-correctness`
- **Decision:** `defer`
- **Evidence:** Defer sync/backend correctness change: `git show --name-status a27aab7e4113a132a516c1885cdd0f5eab87bf4c` touches `cmd/agentsview/duckdb.go`, `cmd/agentsview/import.go`, `cmd/agentsview/recall.go`, `cmd/agentsview/recall_test.go`, `cmd/agentsview/session.go`, ... (15 paths total); local counterpart/equivalent surface starts at `cmd/agentsview/duckdb.go:19`. Needs backend parity or sync regression tests before adoption.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `A local sync, watcher, PG/DuckDB, daemon, or remote-sync regression maps to this upstream lineage.`

### [primary] `e9703c2083882d5bc62dce5a073fd45d1f62567a` — `fix(vector): let kit drop blank embedding inputs instead of classifying rejections (#1286)`
- **Category:** `security`
- **Decision:** `defer`
- **Evidence:** Defer platform/CLI/server behavior: `git show --name-status e9703c2083882d5bc62dce5a073fd45d1f62567a` touches `go.mod`, `go.sum`, `internal/vector/build.go`, `internal/vector/encoder.go`, `internal/vector/repair.go`, ... (7 paths total); local counterpart/equivalent surface starts at `go.mod:1`. Needs a scoped acceptance test before adoption.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `Dependency update is requested or a security advisory affects the locked package range.`

### [primary] `285bfa3430dadd6220c237d2ec19f373ec209416` — `fix(test): synchronize daemon startup wait (#1263)`
- **Category:** `platform-other`
- **Decision:** `not-applicable`
- **Evidence:** Not applicable to the current fork because the upstream target path set is absent at frozen `local_head`; examples from `git show --name-only`: `cmd/agentsview/serve_background.go`, `cmd/agentsview/serve_background_test.go`. Verified absence anchor: path absent at frozen local_head: `cmd/agentsview/serve_background.go`, `cmd/agentsview/serve_background_test.go`.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `none`

### [primary] `6a0e54671ae38026eff33277a861315054e711b7` — `fix(test): filter recall evidence log capture to recall lines (#1266)`
- **Category:** `platform-other`
- **Decision:** `not-applicable`
- **Evidence:** Not applicable to the current fork because the upstream target path set is absent at frozen `local_head`; examples from `git show --name-only`: `internal/db/recall_evidence_window_test.go`. Verified absence anchor: path absent at frozen local_head: `internal/db/recall_evidence_window_test.go`.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `none`

### [primary] `9e5324c9ec8b28f4e5d19b673b7e8929d33e0ac8` — `fix(claude): keep ide context out of session titles (#1252)`
- **Category:** `sync-correctness`
- **Decision:** `defer`
- **Evidence:** Defer sync/backend correctness change: `git show --name-status 9e5324c9ec8b28f4e5d19b673b7e8929d33e0ac8` touches `docs/internal/session-format-sources.md`, `internal/db/db.go`, `internal/db/db_test.go`, `internal/parser/claude.go`, `internal/parser/claude_parser_test.go`, ... (8 paths total); local counterpart/equivalent surface starts at `internal/db/db.go:57`. Needs backend parity or sync regression tests before adoption.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `A local sync, watcher, PG/DuckDB, daemon, or remote-sync regression maps to this upstream lineage.`

### [primary] `e7adb12b5b55283171671f8041eb4d6cb1650f33` — `fix(ssh): deduplicate resolved Poolside targets (#1260)`
- **Category:** `active-agent`
- **Decision:** `defer`
- **Evidence:** Defer active-agent parser behavior: `git show --name-status e7adb12b5b55283171671f8041eb4d6cb1650f33` touches `internal/ssh/resolve.go`, `internal/ssh/resolve_test.go`; local counterpart/equivalent parser surface starts at `internal/ssh/resolve.go:12`. Needs agent-specific fixtures before adoption.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `A fixture or real sample for this agent demonstrates the upstream behavior gap in the fork.`

### [primary] `8aa989f45943d1fc195f04583456899e7473e795` — `fix(parser): detect opencode invalid tool calls as failures (#1255)`
- **Category:** `archive-schema`
- **Decision:** `defer`
- **Evidence:** Defer archive/schema/data behavior change: `git show --name-status 8aa989f45943d1fc195f04583456899e7473e795` touches `docs/internal/session-format-sources.md`, `internal/db/db.go`, `internal/db/db_test.go`, `internal/parser/opencode.go`, `internal/parser/opencode_test.go`; local counterpart/equivalent surface starts at `internal/db/db.go:21`. Needs data-preservation and backend-parity tests before adoption.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `A local archive, stats, usage, search, pricing, or schema bug maps to this upstream lineage.`

### [primary] `14ef5729a9973f164989d523c34380aac59c2859` — `fix(parser): bound Gemini reconciliation work (#1259)`
- **Category:** `active-agent`
- **Decision:** `defer`
- **Evidence:** Provider-facade path absence does not make Gemini behavior inapplicable. Local fork supports Gemini at `internal/parser/types.go:115`; upstream touches provider files absent in the fork, so this belongs to the provider-facade defer bucket.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `Re-evaluate when Gemini reconciliation work is reproduced locally or provider-facade migration is revisited.`

### [primary] `0d7e0611201b6fed6c2a2db850a8b44508e1b579` — `fix(poolside): correct Linux default directory to .local/state/poolside (#1258)`
- **Category:** `platform-other`
- **Decision:** `not-applicable`
- **Evidence:** Not applicable to runtime/data correctness for this fork phase: `git show --name-status 0d7e0611201b6fed6c2a2db850a8b44508e1b579` touches `README.md`, `docs/configuration.md`, `internal/parser/poolside_provider.go`, `internal/parser/poolside_test.go`, `internal/parser/types.go`, ... (7 paths total); local counterpart/equivalent surface starts at `README.md:30`. No runtime deliverable is in this phase scope.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `none`

### [primary] `602210eb6fc0145fd352be23b42168a8d7d546b1` — `fix(parser): prefer OpenCode session.directory over project worktree (#1237)`
- **Category:** `sync-correctness`
- **Decision:** `defer`
- **Evidence:** Defer sync/backend correctness change: `git show --name-status 602210eb6fc0145fd352be23b42168a8d7d546b1` touches `docs/internal/session-format-sources.md`, `internal/db/db.go`, `internal/db/db_test.go`, `internal/parser/opencode.go`, `internal/parser/opencode_test.go`, ... (6 paths total); local counterpart/equivalent surface starts at `internal/db/db.go:21`. Needs backend parity or sync regression tests before adoption.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `A local sync, watcher, PG/DuckDB, daemon, or remote-sync regression maps to this upstream lineage.`

### [primary] `dc7d627e685140eb9636c5dad052c8e214c57cea` — `Fix browser-timezone session date filters (#1245)`
- **Category:** `ui`
- **Decision:** `defer`
- **Evidence:** Defer UI behavior change: `git show --name-status dc7d627e685140eb9636c5dad052c8e214c57cea` touches `frontend/src/lib/api/generated/services/SearchService.ts`, `frontend/src/lib/api/generated/services/SessionsService.ts`, `frontend/src/lib/stores/sessions.svelte.ts`, `frontend/src/lib/stores/sessions.test.ts`, `internal/db/filter_test.go`, ... (27 paths total); local counterpart/equivalent surface starts at `frontend/src/lib/api/generated/services/SearchService.ts:12`. Needs frontend tests or visual review before adoption.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `User-facing navigation/filter/rendering issue is reproduced or requested in this fork UI.`

### [primary] `a2c1baa7ccb4a00f1ebdef68e250c6ab73a57dd6` — `fix(postgres): incrementalize change-triggered vector reconciliation (#1207)`
- **Category:** `sync-correctness`
- **Decision:** `defer`
- **Evidence:** Defer platform/CLI/server behavior: `git show --name-status a2c1baa7ccb4a00f1ebdef68e250c6ab73a57dd6` touches `cmd/agentsview/archive_write_backend.go`, `cmd/agentsview/archive_write_backend_test.go`, `cmd/agentsview/daemon_push.go`, `cmd/agentsview/pg.go`, `cmd/agentsview/pg_test.go`, ... (23 paths total); local counterpart/equivalent surface starts at `cmd/agentsview/pg.go:18`. Needs a scoped acceptance test before adoption.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `A local sync, watcher, PG/DuckDB, daemon, or remote-sync regression maps to this upstream lineage.`

### [primary] `537aeeb51b9c252f5e2768be299c15bf1e36fadb` — `fix(duckdb): fail fast on unresponsive remote Quack attach (#1164)`
- **Category:** `sync-correctness`
- **Decision:** `defer`
- **Evidence:** Defer sync/backend correctness change: `git show --name-status 537aeeb51b9c252f5e2768be299c15bf1e36fadb` touches `docs/duckdb.md`, `internal/config/config.go`, `internal/duckdb/attach_timeout_test.go`, `internal/duckdb/connect.go`, `internal/duckdb/driver_windows_arm64_test.go`, ... (6 paths total); local counterpart/equivalent surface starts at `internal/config/config.go:68`. Needs backend parity or sync regression tests before adoption.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `A local sync, watcher, PG/DuckDB, daemon, or remote-sync regression maps to this upstream lineage.`

### [primary] `7546ebffc113e0b5f508c9fd5ba1ec2266362bf2` — `fix(daemon): preserve full resync progress detail (#1231)`
- **Category:** `sync-correctness`
- **Decision:** `not-applicable`
- **Evidence:** Not applicable to the current fork because the upstream target path set is absent at frozen `local_head`; examples from `git show --name-only`: `cmd/agentsview/startup_state.go`, `cmd/agentsview/startup_state_test.go`, `cmd/agentsview/startup_worker_test.go`. Verified absence anchor: path absent at frozen local_head: `cmd/agentsview/startup_state.go`, `cmd/agentsview/startup_state_test.go`, `cmd/agentsview/startup_worker_test.go`.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `none`

### [primary] `b88c54911ed48d375d71562d7951816465f725fb` — `fix(grok): emit one usage event per turn instead of last-snapshot-per-session (#1228)`
- **Category:** `archive-schema`
- **Decision:** `defer`
- **Evidence:** Defer archive/schema/data behavior change: `git show --name-status b88c54911ed48d375d71562d7951816465f725fb` touches `internal/db/db.go`, `internal/db/db_test.go`, `internal/db/usage_test.go`, `internal/parser/grok.go`, `internal/parser/grok_test.go`; local counterpart/equivalent surface starts at `internal/db/db.go:62`. Needs data-preservation and backend-parity tests before adoption.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `A local archive, stats, usage, search, pricing, or schema bug maps to this upstream lineage.`

### [primary] `ff6a3e4f6d96789e0d69f6de1049b64ae24244cf` — `fix(sync): bound daemon memory and watcher recovery (#1154)`
- **Category:** `macos-runtime`
- **Decision:** `defer`
- **Evidence:** Defer macOS runtime/discovery fix: `git show --name-status ff6a3e4f6d96789e0d69f6de1049b64ae24244cf` touches `.github/workflows/ci-macos-main.yml`, `.github/workflows/desktop-macos-main.yml`, `cmd/agentsview/archive_audit_test.go`, `cmd/agentsview/archive_write_backend.go`, `cmd/agentsview/archive_write_backend_test.go`, ... (231 paths total); local counterpart exists at `cmd/agentsview/classifier_wiring_test.go:1`. Needs a focused macOS filesystem/watch regression, not a ledger-only change.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `A macOS protected-directory, watcher, or media-folder failure is reproduced locally.`

### [primary] `c664fde9d493b4a6745c3b45ffbe86a297d4aa5a` — `fix(frontend): label inline teammate transcript messages (#1229)`
- **Category:** `ui`
- **Decision:** `defer`
- **Evidence:** Defer UI behavior change: `git show --name-status c664fde9d493b4a6745c3b45ffbe86a297d4aa5a` touches `frontend/src/lib/components/content/MessageContent.svelte`, `frontend/src/lib/components/content/MessageContent.test.ts`; local counterpart/equivalent surface starts at `frontend/src/lib/components/content/MessageContent.svelte:136`. Needs frontend tests or visual review before adoption.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `User-facing navigation/filter/rendering issue is reproduced or requested in this fork UI.`

### [primary] `c8593ec5bd0070bca42d0c7639c51621cb1c1841` — `fix(recall): chunk evidence hydration queries (#1221)`
- **Category:** `archive-schema`
- **Decision:** `not-applicable`
- **Evidence:** Not applicable to the current fork because the upstream target path set is absent at frozen `local_head`; examples from `git show --name-only`: `internal/db/recall.go`, `internal/db/recall_test.go`. Verified absence anchor: path absent at frozen local_head: `internal/db/recall.go`, `internal/db/recall_test.go`.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `none`

### [primary] `77216a2047e986b4b69e7261e06b6875fde35f0c` — `fix(recall): keep large body bound out of request grammar (#1216)`
- **Category:** `platform-other`
- **Decision:** `not-applicable`
- **Evidence:** Not applicable to the current fork because the upstream target path set is absent at frozen `local_head`; examples from `git show --name-only`: `docs/internal/recall-extraction.md`, `internal/recall/extract/client.go`, `internal/recall/extract/client_test.go`, `internal/recall/extract/manager_test.go`. Verified absence anchor: path absent at frozen local_head: `docs/internal/recall-extraction.md`, `internal/recall/extract/client.go`, `internal/recall/extract/client_test.go`.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `none`

### [primary] `c64bacb92e700cd5f94a767f7d55836e0462f798` — `fix(deps): update dependency dompurify to v3.4.12 [security] (#1214)`
- **Category:** `security`
- **Decision:** `adopt`
- **Evidence:** Superseded by phase 05 dompurify 3.4.13 landing `3ebf2fad80ba03d7554da587eacadba1072cf7e4`. Local anchor `frontend/package.json:1`; 3.4.13 is newer than 3.4.12.
- **Port cost:** `conflict`
- **Delivery:** `landed:3ebf2fad80ba03d7554da587eacadba1072cf7e4/phase-05`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `none`

### [primary] `1ee2de88e2dae54326d8b47aeb2de2f58b5944f9` — `fix(frontend): preserve project name when opening search sessions (#1204)`
- **Category:** `ui`
- **Decision:** `defer`
- **Evidence:** Defer UI behavior change: `git show --name-status 1ee2de88e2dae54326d8b47aeb2de2f58b5944f9` touches `frontend/e2e/navigation.spec.ts`, `frontend/src/App.svelte`, `frontend/src/App.test.ts`, `frontend/src/lib/components/command-palette/CommandPalette.svelte`, `frontend/src/lib/components/command-palette/CommandPalette.test.ts`, ... (9 paths total); local counterpart/equivalent surface starts at `frontend/e2e/navigation.spec.ts:28`. Needs frontend tests or visual review before adoption.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `User-facing navigation/filter/rendering issue is reproduced or requested in this fork UI.`

### [primary] `f67657e08a3908a5b999ca2add5aba20eb919160` — `fix(usage): use Copilot-reported billing as authoritative session cost (#1169)`
- **Category:** `ui`
- **Decision:** `defer`
- **Evidence:** Defer UI behavior change: `git show --name-status f67657e08a3908a5b999ca2add5aba20eb919160` touches `README.md`, `cmd/agentsview/export.go`, `cmd/agentsview/export_sessions_test.go`, `cmd/agentsview/session_usage.go`, `cmd/agentsview/session_usage_test.go`, ... (42 paths total); local counterpart/equivalent surface starts at `README.md:21`. Needs frontend tests or visual review before adoption.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `User-facing navigation/filter/rendering issue is reproduced or requested in this fork UI.`

### [primary] `a84564ad6ea35f8fb80905da4693144f89f84229` — `fix(grok): ingest usage snapshots from updates.jsonl (#1206)`
- **Category:** `sync-correctness`
- **Decision:** `defer`
- **Evidence:** Defer sync/backend correctness change: `git show --name-status a84564ad6ea35f8fb80905da4693144f89f84229` touches `internal/parser/grok.go`, `internal/parser/grok_provider.go`, `internal/parser/grok_test.go`, `internal/sync/engine_integration_test.go`; local counterpart/equivalent surface starts at `internal/sync/engine_integration_test.go:1042`. Needs backend parity or sync regression tests before adoption.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `A local sync, watcher, PG/DuckDB, daemon, or remote-sync regression maps to this upstream lineage.`

### [primary] `3f0a3429eca7d1fe337e545760c3e48cd6ec9deb` — `fix(frontend): stop persisting rolling-derived session date bounds (#1194)`
- **Category:** `ui`
- **Decision:** `defer`
- **Evidence:** Defer UI behavior change: `git show --name-status 3f0a3429eca7d1fe337e545760c3e48cd6ec9deb` touches `frontend/src/App.svelte`, `frontend/src/App.test.ts`, `frontend/src/lib/components/analytics/AnalyticsPage.svelte`, `frontend/src/lib/stores/sessionRouteParams.ts`, `frontend/src/lib/stores/sessions.svelte.ts`, ... (6 paths total); local counterpart/equivalent surface starts at `frontend/src/App.svelte:30`. Needs frontend tests or visual review before adoption.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `User-facing navigation/filter/rendering issue is reproduced or requested in this fork UI.`

### [primary] `06140b1fdea08b60a0355926034d2e26c901d436` — `fix(parser): align Grok Build persistence formats (#1188)`
- **Category:** `active-agent`
- **Decision:** `not-applicable`
- **Evidence:** Grok Build parser format paths are absent in this fork: upstream touched `internal/parser/grok.go` and Grok Build testdata/provider files that do not exist at frozen local_head. Local active-agent data distribution provided in the phase spec has no Grok count.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `none`

### [primary] `bf5ad3b5a13da3c0a78022ee356ab2126cfaee37` — `fix(server): add timeout response detail (#1175)`
- **Category:** `archive-schema`
- **Decision:** `defer`
- **Evidence:** Defer platform/CLI/server behavior: `git show --name-status bf5ad3b5a13da3c0a78022ee356ab2126cfaee37` touches `internal/server/helpers_internal_test.go`, `internal/server/huma_routes.go`, `internal/server/middleware.go`, `internal/server/middleware_internal_test.go`, `internal/server/middleware_test.go`, ... (8 paths total); local counterpart/equivalent surface starts at `internal/server/helpers_internal_test.go:1`. Needs a scoped acceptance test before adoption.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `A local archive, stats, usage, search, pricing, config, server, or schema bug maps to this upstream lineage.`

### [primary] `5b0f6bff5b1b963f07836b3f25720cac22742707` — `fix(deps): update javascript dependencies (#1140)`
- **Category:** `security`
- **Decision:** `defer`
- **Evidence:** Defer dependency maintenance: `git show --name-status 5b0f6bff5b1b963f07836b3f25720cac22742707` touches dependency manifests `frontend/package-lock.json`, `frontend/package.json`; this needs a dedicated advisory/lockfile/build phase rather than phase 06 docs. Local anchor: `frontend/package-lock.json:2984`.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `Dependency update is requested or a security advisory affects the locked package range.`

### [primary] `4ea27c06f4a5c5de337480d9e3c3a42c750c4fd0` — `fix(duckdb): point remote schema-incompatible errors at the server (#1165)`
- **Category:** `sync-correctness`
- **Decision:** `defer`
- **Evidence:** Defer sync/backend correctness change: `git show --name-status 4ea27c06f4a5c5de337480d9e3c3a42c750c4fd0` touches `cmd/agentsview/duckdb.go`, `internal/duckdb/schema.go`, `internal/duckdb/schema_test.go`; local counterpart/equivalent surface starts at `cmd/agentsview/duckdb.go:19`. Needs backend parity or sync regression tests before adoption.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `A local sync, watcher, PG/DuckDB, daemon, or remote-sync regression maps to this upstream lineage.`

### [primary] `624747545baaa96f86e8c2499f372eda631445d6` — `fix(duckdb): render named string kinds in remote push arguments (#1161)`
- **Category:** `sync-correctness`
- **Decision:** `defer`
- **Evidence:** Defer sync/backend correctness change: `git show --name-status 624747545baaa96f86e8c2499f372eda631445d6` touches `internal/duckdb/project_identity_upsert.go`, `internal/duckdb/push.go`, `internal/duckdb/quack_smoke_duckdbtest_test.go`, `internal/duckdb/sync_test.go`; local counterpart/equivalent surface starts at `internal/duckdb/push.go:1`. Needs backend parity or sync regression tests before adoption.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `A local sync, watcher, PG/DuckDB, daemon, or remote-sync regression maps to this upstream lineage.`

### [primary] `f6f2afbdadef040b5038717be867db9a37461b16` — `fix(parser): track Hermes skill_view usage (#1173)`
- **Category:** `archive-schema`
- **Decision:** `defer`
- **Evidence:** Defer archive/schema/data behavior change: `git show --name-status f6f2afbdadef040b5038717be867db9a37461b16` touches `internal/db/db.go`, `internal/db/db_test.go`, `internal/parser/hermes.go`, `internal/parser/hermes_test.go`; local counterpart/equivalent surface starts at `internal/db/db.go:21`. Needs data-preservation and backend-parity tests before adoption.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `A local archive, stats, usage, search, pricing, or schema bug maps to this upstream lineage.`

### [primary] `e310f67972e0a0a36365c6785e60935adc52f8df` — `fix(duckdb): reject quack http/https client URL forms with an actionable error (#1163)`
- **Category:** `sync-correctness`
- **Decision:** `defer`
- **Evidence:** Defer sync/backend correctness change: `git show --name-status e310f67972e0a0a36365c6785e60935adc52f8df` touches `README.md`, `cmd/agentsview/duckdb_test.go`, `docs/duckdb.md`, `internal/duckdb/connect.go`, `internal/duckdb/connect_test.go`, ... (6 paths total); local counterpart/equivalent surface starts at `README.md:25`. Needs backend parity or sync regression tests before adoption.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `A local sync, watcher, PG/DuckDB, daemon, or remote-sync regression maps to this upstream lineage.`

### [primary] `a4cb13c4988d580c085e3a7e04d37c87db7666d5` — `fix(i18n): localize the analytics model filter label (#1150)`
- **Category:** `ui`
- **Decision:** `defer`
- **Evidence:** Defer UI behavior change: `git show --name-status a4cb13c4988d580c085e3a7e04d37c87db7666d5` touches `frontend/messages/en.json`, `frontend/messages/fr.json`, `frontend/messages/ko.json`, `frontend/messages/zh-CN.json`, `frontend/messages/zh-TW.json`, ... (6 paths total); local counterpart/equivalent surface starts at `frontend/src/lib/components/analytics/AnalyticsPage.svelte:17`. Needs frontend tests or visual review before adoption.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `User-facing navigation/filter/rendering issue is reproduced or requested in this fork UI.`

### [primary] `7760cf7c4a1fb1710837fdf7fe11074a608dfe80` — `fix(frontend): cancel obsolete UI read queries (#1146)`
- **Category:** `ui`
- **Decision:** `defer`
- **Evidence:** Defer UI behavior change: `git show --name-status 7760cf7c4a1fb1710837fdf7fe11074a608dfe80` touches `frontend/e2e/read-cancellation.spec.ts`, `frontend/scripts/generate-api-client.mjs`, `frontend/src/App.svelte`, `frontend/src/App.test.ts`, `frontend/src/lib/api/generated/core/request.ts`, ... (55 paths total); local counterpart/equivalent surface starts at `frontend/scripts/generate-api-client.mjs:15`. Needs frontend tests or visual review before adoption.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `User-facing navigation/filter/rendering issue is reproduced or requested in this fork UI.`

### [primary] `f73697c7ca97d31e0f82d1619445f94861a83c1a` — `fix(db): preserve legacy archives during schema repair (#1143)`
- **Category:** `archive-schema`
- **Decision:** `adopt`
- **Evidence:** Phase 01 landed the non-destructive legacy schema repair: `41a0a20f4d397709997cbeca1af381f26f60d081`. Local anchor `internal/db/db.go:350` documents preserving existing archives and marking stale data for resync.
- **Port cost:** `conflict`
- **Delivery:** `landed:41a0a20f4d397709997cbeca1af381f26f60d081/phase-01`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `none`

### [primary] `e0c2808fdcc824e28265e0c7513eedea8e123787` — `fix(frontend): preserve remote markdown export links (#1142)`
- **Category:** `ui`
- **Decision:** `defer`
- **Evidence:** Defer UI behavior change: `git show --name-status e0c2808fdcc824e28265e0c7513eedea8e123787` touches `frontend/src/lib/api/client-markdown-export.test.ts`, `frontend/src/lib/api/client.ts`; local counterpart/equivalent surface starts at `frontend/src/lib/api/client-markdown-export.test.ts:3`. Needs frontend tests or visual review before adoption.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `User-facing navigation/filter/rendering issue is reproduced or requested in this fork UI.`

### [primary] `fbde1c7ced95819e9e46fa155c2e8ce599b273b0` — `fix(parser): classify queued system prompts before single-turn counting (#1131) (#1134)`
- **Category:** `ui`
- **Decision:** `defer`
- **Evidence:** Defer UI behavior change: `git show --name-status fbde1c7ced95819e9e46fa155c2e8ce599b273b0` touches `frontend/src/lib/utils/messages.test.ts`, `frontend/src/lib/utils/messages.ts`, `internal/db/db.go`, `internal/db/db_test.go`, `internal/db/search.go`, ... (17 paths total); local counterpart/equivalent surface starts at `frontend/src/lib/utils/messages.test.ts:120`. Needs frontend tests or visual review before adoption.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `User-facing navigation/filter/rendering issue is reproduced or requested in this fork UI.`

### [primary] `f55349bcfb11d164f97456041d7c580790f6ddf0` — `fix(postgres): refresh pricing before pushes (#1144)`
- **Category:** `sync-correctness`
- **Decision:** `defer`
- **Evidence:** Round-1 correction: this is a live PG push path, not `not-applicable`. Local anchors `cmd/agentsview/pg.go:72` and `internal/postgres/push.go:127`; no pricing refresh call was verified in the local pg push entrypoints.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `Re-evaluate with a PG push pricing-refresh regression test.`

### [primary] `1efc4aa1fc2aa4786d42c53166b43a8a27590b4f` — `fix(daemon): report start progress until ready (#1141)`
- **Category:** `sync-correctness`
- **Decision:** `not-applicable`
- **Evidence:** Not applicable to the current fork because the upstream target path set is absent at frozen `local_head`; examples from `git show --name-only`: `cmd/agentsview/daemon.go`, `cmd/agentsview/daemon_test.go`. Verified absence anchor: path absent at frozen local_head: `cmd/agentsview/daemon.go`, `cmd/agentsview/daemon_test.go`.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `none`

### [primary] `e7b39c440829244043cbc08776789f2cc46bed85` — `fix(test): give remotesync liveness guards a Windows-safe deadline (#1138)`
- **Category:** `platform-other`
- **Decision:** `not-applicable`
- **Evidence:** Not applicable to the current fork because the upstream target path set is absent at frozen `local_head`; examples from `git show --name-only`: `internal/remotesync/archive_test.go`, `internal/remotesync/archive_unix_test.go`, `internal/remotesync/http_test.go`. Verified absence anchor: path absent at frozen local_head: `internal/remotesync/archive_test.go`, `internal/remotesync/archive_unix_test.go`, `internal/remotesync/http_test.go`.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `none`

### [primary] `fd576d6e5e5e0873e54e54b610bad4e2596e32fd` — `Fix project selector crash on empty project names (#1133)`
- **Category:** `ui`
- **Decision:** `defer`
- **Evidence:** Defer UI behavior change: `git show --name-status fd576d6e5e5e0873e54e54b610bad4e2596e32fd` touches `frontend/src/lib/components/layout/ProjectTypeahead.svelte`, `frontend/src/lib/components/layout/ProjectTypeahead.test.ts`; local counterpart/equivalent surface starts at `frontend/src/lib/components/layout/ProjectTypeahead.svelte:3`. Needs frontend tests or visual review before adoption.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `User-facing navigation/filter/rendering issue is reproduced or requested in this fork UI.`

### [primary] `1e173b9589a3855d14fdba2fc72d9125301bce36` — `fix(parser): restore Codex subagent lineage and titles (#1125)`
- **Category:** `sync-correctness`
- **Decision:** `defer`
- **Evidence:** Defer sync/backend correctness change: `git show --name-status 1e173b9589a3855d14fdba2fc72d9125301bce36` touches `cmd/agentsview/cli_test.go`, `cmd/agentsview/doctor.go`, `cmd/agentsview/doctor_test.go`, `internal/db/db.go`, `internal/parser/codex.go`, ... (10 paths total); local counterpart/equivalent surface starts at `cmd/agentsview/cli_test.go:1`. Needs backend parity or sync regression tests before adoption.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `A local sync, watcher, PG/DuckDB, daemon, or remote-sync regression maps to this upstream lineage.`

### [primary] `2066ea35666f86c07acc3b55281334000b64a429` — `fix(desktop): retry backend stop before update install (#1118)`
- **Category:** `platform-other`
- **Decision:** `not-applicable`
- **Evidence:** Not applicable to this local web-viewer catch-up scope because `git show --name-status 2066ea35666f86c07acc3b55281334000b64a429` is desktop/Windows/AppImage-specific: `desktop/src-tauri/src/lib.rs`. No desktop deliverable is in this run scope.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `none`

### [primary] `0111b99c7ca66330c9d339d8d3ba69cfc2e37462` — `fix(daemon): report restart progress until ready (#1115)`
- **Category:** `ui`
- **Decision:** `defer`
- **Evidence:** Defer UI behavior change: `git show --name-status 0111b99c7ca66330c9d339d8d3ba69cfc2e37462` touches `.gitignore`, `Makefile`, `cmd/agentsview/daemon.go`, `cmd/agentsview/daemon_signal_unix_test.go`, `cmd/agentsview/daemon_test.go`, ... (9 paths total); local counterpart/equivalent surface starts at `.gitignore:14`. Needs frontend tests or visual review before adoption.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `User-facing navigation/filter/rendering issue is reproduced or requested in this fork UI.`

### [primary] `cc7559bcf0ae9e8c785be9bdf9b85eb4f7cfcfeb` — `fix(server): prevent stale SPA asset fallbacks (#1114)`
- **Category:** `ui`
- **Decision:** `adopt`
- **Evidence:** Phase 04 landed missing-asset 404 and no-cache entry semantics: `05d58a5959018a9ed46ed4f1cb96a76648d84028`. Local anchor `internal/server/server.go:1`; phase 04 tests cover `TestSPA`.
- **Port cost:** `conflict`
- **Delivery:** `landed:05d58a5959018a9ed46ed4f1cb96a76648d84028/phase-04`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `none`

### [primary] `dba06c26a5760a7da4ed1e39adbe17cb07375231` — `fix(activity): advance calendar date without refresh (#1112)`
- **Category:** `ui`
- **Decision:** `not-applicable`
- **Evidence:** Not applicable to the current fork because the upstream target path set is absent at frozen `local_head`; examples from `git show --name-only`: `frontend/src/lib/components/activity/ActivityPage.svelte`, `frontend/src/lib/components/activity/ActivityPage.test.ts`. Verified absence anchor: path absent at frozen local_head: `frontend/src/lib/components/activity/ActivityPage.svelte`, `frontend/src/lib/components/activity/ActivityPage.test.ts`.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `none`

### [primary] `1979b0bccf8e00522983a075fdf990393b3837d3` — `fix(ci): retry transient SSH image builds (#1111)`
- **Category:** `platform-other`
- **Decision:** `not-applicable`
- **Evidence:** CI-only retry wrapper is not runtime/data correctness for this fork phase. `git show --name-status 1979b0bccf8e00522983a075fdf990393b3837d3` touches `.github/workflows/ci.yml`, CI retry docs, and `scripts/retry.sh` / `scripts/retry_test.sh`; no application code path is changed.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `none`

### [primary] `0617663269751007fd19ca00573c47e7dde41a60` — `fix(daemon): recover recordless writable daemons after startup-state lock acquisition (#1103)`
- **Category:** `ui`
- **Decision:** `defer`
- **Evidence:** Defer UI behavior change: `git show --name-status 0617663269751007fd19ca00573c47e7dde41a60` touches `cmd/agentsview/daemon.go`, `cmd/agentsview/daemon_runtime.go`, `cmd/agentsview/daemon_runtime_test.go`, `cmd/agentsview/daemon_test.go`, `cmd/agentsview/serve_background.go`, ... (10 paths total); local counterpart/equivalent surface starts at `cmd/agentsview/daemon_runtime.go:1`. Needs frontend tests or visual review before adoption.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `User-facing navigation/filter/rendering issue is reproduced or requested in this fork UI.`

### [primary] `32492eb017b8fff3493310f9013f3362a2d0f116` — `fix(deps): update kit for trusted Windows directory owners (#1107)`
- **Category:** `platform-other`
- **Decision:** `not-applicable`
- **Evidence:** Not applicable to this local web-viewer catch-up scope because `git show --name-status 32492eb017b8fff3493310f9013f3362a2d0f116` is desktop/Windows/AppImage-specific: `go.mod`, `go.sum`. No desktop deliverable is in this run scope.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `none`

### [primary] `81a0e688978c15761fde0d04cbdf5ffb9faa7177` — `fix: import current Grok Build sessions with full transcripts (#1106)`
- **Category:** `ui`
- **Decision:** `defer`
- **Evidence:** Defer UI behavior change: `git show --name-status 81a0e688978c15761fde0d04cbdf5ffb9faa7177` touches `README.md`, `docs/configuration.md`, `internal/parser/grok.go`, `internal/parser/grok_provider.go`, `internal/parser/grok_test.go`; local counterpart/equivalent surface starts at `README.md:213`. Needs frontend tests or visual review before adoption.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `User-facing navigation/filter/rendering issue is reproduced or requested in this fork UI.`

### [primary] `f478a45db461bc17b041670d7fd2e7a2d08a3cc7` — `fix(serve): surface runtime-record write warnings in serve logs (#1097) (#1102)`
- **Category:** `sync-correctness`
- **Decision:** `defer`
- **Evidence:** Defer sync/backend correctness change: `git show --name-status f478a45db461bc17b041670d7fd2e7a2d08a3cc7` touches `cmd/agentsview/duckdb.go`, `cmd/agentsview/main.go`, `cmd/agentsview/main_test.go`, `cmd/agentsview/pg.go`, `cmd/agentsview/runtime_warning.go`; local counterpart/equivalent surface starts at `cmd/agentsview/duckdb.go:20`. Needs backend parity or sync regression tests before adoption.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `A local sync, watcher, PG/DuckDB, daemon, or remote-sync regression maps to this upstream lineage.`

### [primary] `43c5b90b4fb3fc2cc029980762c87c99b48bb32d` — `perf: bound passive daemon memory and warm sync work (#1098)`
- **Category:** `macos-runtime`
- **Decision:** `defer`
- **Evidence:** Defer macOS runtime/discovery fix: `git show --name-status 43c5b90b4fb3fc2cc029980762c87c99b48bb32d` touches `internal/db/automated.go`, `internal/db/automated_audit.go`, `internal/db/automated_backfill_test.go`, `internal/db/connection_test.go`, `internal/db/db.go`, ... (30 paths total); local counterpart exists at `internal/db/automated.go:39`. Needs a focused macOS filesystem/watch regression, not a ledger-only change.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `A macOS protected-directory, watcher, or media-folder failure is reproduced locally.`

### [primary] `5396b6da0755176633693ff3273e99d6747da88a` — `fix(stats): align session counts with list visibility (#1048) (#1088)`
- **Category:** `archive-schema`
- **Decision:** `defer`
- **Evidence:** Defer archive/schema/data behavior change: `git show --name-status 5396b6da0755176633693ff3273e99d6747da88a` touches `cmd/agentsview/stats.go`, `cmd/agentsview/stats_test.go`, `cmd/agentsview/testdata/stats_golden.json`, `internal/db/query_dialect.go`, `internal/db/session_stats.go`, ... (11 paths total); local counterpart/equivalent surface starts at `cmd/agentsview/stats.go:1`. Needs data-preservation and backend-parity tests before adoption.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `A local archive, stats, usage, search, pricing, or schema bug maps to this upstream lineage.`

### [primary] `354a2164d442ba70fa490940e043be72e92ba956` — `perf(parser): reuse JSONL reader workspaces (#1093)`
- **Category:** `active-agent`
- **Decision:** `defer`
- **Evidence:** Defer active-agent parser behavior: `git show --name-status 354a2164d442ba70fa490940e043be72e92ba956` touches `internal/parser/claude.go`, `internal/parser/codex.go`, `internal/parser/commandcode.go`, `internal/parser/copilot.go`, `internal/parser/cowork.go`, ... (23 paths total); local counterpart/equivalent parser surface starts at `internal/parser/claude.go:3`. Needs agent-specific fixtures before adoption.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `A fixture or real sample for this agent demonstrates the upstream behavior gap in the fork.`

### [primary] `cee430217f23518e07053aca0b261e98b5ace0bc` — `fix: show live daemon version in status (#1091)`
- **Category:** `sync-correctness`
- **Decision:** `not-applicable`
- **Evidence:** Not applicable to the current fork because the upstream target path set is absent at frozen `local_head`; examples from `git show --name-only`: `cmd/agentsview/daemon.go`, `cmd/agentsview/daemon_safety_test.go`, `cmd/agentsview/daemon_test.go`, `cmd/agentsview/serve_lifecycle.go`. Verified absence anchor: path absent at frozen local_head: `cmd/agentsview/daemon.go`, `cmd/agentsview/daemon_safety_test.go`, `cmd/agentsview/daemon_test.go`.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `none`

### [primary] `0cb491f8e8e7a39da0d081cea35c5eb66a8de691` — `perf(sync): bound background watcher memory (#1090)`
- **Category:** `sync-correctness`
- **Decision:** `defer`
- **Evidence:** Defer sync/backend correctness change: `git show --name-status 0cb491f8e8e7a39da0d081cea35c5eb66a8de691` touches `AGENTS.md`, `internal/parser/aider_provider.go`, `internal/parser/capabilities.go`, `internal/parser/db_backed_provider.go`, `internal/parser/kiro_provider.go`, ... (13 paths total); local counterpart/equivalent surface starts at `AGENTS.md:54`. Needs backend parity or sync regression tests before adoption.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `A local sync, watcher, PG/DuckDB, daemon, or remote-sync regression maps to this upstream lineage.`

### [primary] `0dc2402984d16e56c1101474a8a5ed206c5fc9b4` — `fix(hermes): export only the requested session transcript (#1084)`
- **Category:** `archive-schema`
- **Decision:** `defer`
- **Evidence:** Defer archive/schema/data behavior change: `git show --name-status 0dc2402984d16e56c1101474a8a5ed206c5fc9b4` touches `cmd/agentsview/session_export.go`, `cmd/agentsview/session_test.go`, `internal/parser/hermes.go`, `internal/parser/hermes_provider.go`, `internal/parser/hermes_test.go`; local counterpart/equivalent surface starts at `cmd/agentsview/session_export.go:1`. Needs data-preservation and backend-parity tests before adoption.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `A local archive, stats, usage, search, pricing, or schema bug maps to this upstream lineage.`

### [primary] `ac4686d97176952855c6276cd8ddcaa77567e9cd` — `perf(claude): apply subagent linkage incrementally (#1078)`
- **Category:** `sync-correctness`
- **Decision:** `defer`
- **Evidence:** Defer sync/backend correctness change: `git show --name-status ac4686d97176952855c6276cd8ddcaa77567e9cd` touches `internal/db/db_test.go`, `internal/db/messages.go`, `internal/db/sessions.go`, `internal/db/validate.go`, `internal/parser/claude.go`, ... (11 paths total); local counterpart/equivalent surface starts at `internal/db/db_test.go:187`. Needs backend parity or sync regression tests before adoption.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `A local sync, watcher, PG/DuckDB, daemon, or remote-sync regression maps to this upstream lineage.`

### [primary] `04254fa28ee2d196d27ad775d6e8998754763bc7` — `perf(sync): reduce background watcher and Codex append work (#1077)`
- **Category:** `sync-correctness`
- **Decision:** `defer`
- **Evidence:** Defer sync/backend correctness change: `git show --name-status 04254fa28ee2d196d27ad775d6e8998754763bc7` touches `cmd/agentsview/archive_write_backend.go`, `cmd/agentsview/main.go`, `cmd/agentsview/main_test.go`, `docs/commands.md`, `docs/configuration.md`, ... (24 paths total); local counterpart/equivalent surface starts at `cmd/agentsview/main.go:14`. Needs backend parity or sync regression tests before adoption.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `A local sync, watcher, PG/DuckDB, daemon, or remote-sync regression maps to this upstream lineage.`

### [primary] `990948a5108323cbb6e0f9b9d9f94fe9cf9796f7` — `fix(deps): update javascript dependencies (#1067)`
- **Category:** `security`
- **Decision:** `defer`
- **Evidence:** Defer dependency maintenance: `git show --name-status 990948a5108323cbb6e0f9b9d9f94fe9cf9796f7` touches dependency manifests `frontend/package-lock.json`, `frontend/package.json`; this needs a dedicated advisory/lockfile/build phase rather than phase 06 docs. Local anchor: `frontend/package-lock.json:2984`.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `Dependency update is requested or a security advisory affects the locked package range.`

### [primary] `8bd41f0b7f77b87725fefbe8433fbce6ffa548dc` — `fix(parser): strip Codex recommended plugin context (#1061)`
- **Category:** `archive-schema`
- **Decision:** `defer`
- **Evidence:** Defer archive/schema/data behavior change: `git show --name-status 8bd41f0b7f77b87725fefbe8433fbce6ffa548dc` touches `internal/db/db.go`, `internal/db/db_test.go`, `internal/parser/codex.go`, `internal/parser/codex_parser_test.go`; local counterpart/equivalent surface starts at `internal/db/db.go:21`. Needs data-preservation and backend-parity tests before adoption.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `A local archive, stats, usage, search, pricing, or schema bug maps to this upstream lineage.`

### [primary] `f13b5fa6ba6d5645e3436d0e86060c1c8bca0e1c` — `fix(sessions): include overnight activity in date filters (#1058)`
- **Category:** `ui`
- **Decision:** `defer`
- **Evidence:** Defer UI behavior change: `git show --name-status f13b5fa6ba6d5645e3436d0e86060c1c8bca0e1c` touches `cmd/agentsview/export.go`, `cmd/agentsview/session_list.go`, `cmd/agentsview/session_search.go`, `docs/commands.md`, `docs/session-api.md`, ... (17 paths total); local counterpart/equivalent surface starts at `cmd/agentsview/session_list.go:1`. Needs frontend tests or visual review before adoption.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `User-facing navigation/filter/rendering issue is reproduced or requested in this fork UI.`

### [primary] `f02741d15209aba3395c9c085e3ef060e84e95fd` — `fix(docs): stop instant navigation freezing after loading the homepage (#1055)`
- **Category:** `platform-other`
- **Decision:** `not-applicable`
- **Evidence:** Not applicable to the current fork because the upstream target path set is absent at frozen `local_head`; examples from `git show --name-only`: `docs/index.md`, `docs/scripts/check_built_site.py`, `scripts/docs_assets_test.go`. Verified absence anchor: path absent at frozen local_head: `docs/index.md`, `docs/scripts/check_built_site.py`, `scripts/docs_assets_test.go`.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `none`

### [primary] `49e26a39c4e93081011937889f099e3eac722cf7` — `fix(cli): report hidden session list defaults (#1053)`
- **Category:** `archive-schema`
- **Decision:** `defer`
- **Evidence:** Defer archive/schema/data behavior change: `git show --name-status 49e26a39c4e93081011937889f099e3eac722cf7` touches `cmd/agentsview/session_list.go`, `cmd/agentsview/session_list_test.go`; local counterpart/equivalent surface starts at `cmd/agentsview/session_list.go:1`. Needs data-preservation and backend-parity tests before adoption.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `A local archive, stats, usage, search, pricing, or schema bug maps to this upstream lineage.`

### [primary] `b930b54f4456687e1049077491c1c5941994d57b` — `perf(sync): stop re-parsing unchanged OpenCode SQLite containers on every periodic sync (#1036)`
- **Category:** `sync-correctness`
- **Decision:** `defer`
- **Evidence:** Defer sync/backend correctness change: `git show --name-status b930b54f4456687e1049077491c1c5941994d57b` touches `internal/parser/opencode.go`, `internal/parser/opencode_provider.go`, `internal/parser/opencode_provider_test.go`, `internal/parser/opencode_storage_state.go`, `internal/parser/opencode_storage_state_test.go`, ... (13 paths total); local counterpart/equivalent surface starts at `internal/parser/opencode.go:101`. Needs backend parity or sync regression tests before adoption.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `A local sync, watcher, PG/DuckDB, daemon, or remote-sync regression maps to this upstream lineage.`

### [primary] `1b5124bfdf3d3247c90522d30c3d5eb66080408f` — `fix(docs): scrub screenshot fixtures and sidebar trees (#1030)`
- **Category:** `archive-schema`
- **Decision:** `adopt`
- **Evidence:** Reviewer-verified local gap: upstream changes production sidebar tree filtering in `internal/db/query_dialect.go`, `internal/db/sessions.go`, and `internal/postgres/sessions.go`. Local recursive CTE at `internal/db/query_dialect.go:281` expands descendants without an automation predicate, and `grep` found no `automationScopePredicate` equivalent.
- **Port cost:** `conflict`
- **Delivery:** `backlog`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `Backlog item accepted; schedule SQLite/PostgreSQL sidebar tree automation-scope regression tests.`

### [primary] `f791cd14abd9a6cf8ae2eec0a3a2762f75fee5f4` — `fix(sync): honor shutdown signal during watcher-driven syncs (#1038)`
- **Category:** `sync-correctness`
- **Decision:** `defer`
- **Evidence:** Defer sync/backend correctness change: `git show --name-status f791cd14abd9a6cf8ae2eec0a3a2762f75fee5f4` touches `cmd/agentsview/main.go`, `cmd/agentsview/main_test.go`, `internal/sync/engine.go`, `internal/sync/engine_integration_test.go`, `internal/sync/watcher.go`, ... (6 paths total); local counterpart/equivalent surface starts at `cmd/agentsview/main.go:14`. Needs backend parity or sync regression tests before adoption.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `A local sync, watcher, PG/DuckDB, daemon, or remote-sync regression maps to this upstream lineage.`

### [primary] `d5e662a37092d0f13cb73ca3ffbc04412e576d50` — `perf(sync): release large backfill heaps after signal recompute (#1037)`
- **Category:** `sync-correctness`
- **Decision:** `defer`
- **Evidence:** Defer sync/backend correctness change: `git show --name-status d5e662a37092d0f13cb73ca3ffbc04412e576d50` touches `cmd/agentsview/main.go`, `internal/db/search_content_bench_test.go`, `internal/sync/engine.go`, `internal/sync/parsediff_integration_test.go`, `internal/sync/recompute_memory.go`, ... (6 paths total); local counterpart/equivalent surface starts at `cmd/agentsview/main.go:14`. Needs backend parity or sync regression tests before adoption.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `A local sync, watcher, PG/DuckDB, daemon, or remote-sync regression maps to this upstream lineage.`

### [primary] `87cf12e1395ff6b8ebd8e9ad36bd575fa318e8c2` — `fix(parser): parse Codex custom tool calls (#1025)`
- **Category:** `active-agent`
- **Decision:** `defer`
- **Evidence:** Defer active-agent parser behavior: `git show --name-status 87cf12e1395ff6b8ebd8e9ad36bd575fa318e8c2` touches `internal/parser/codex.go`, `internal/parser/codex_parser_test.go`; local counterpart/equivalent parser surface starts at `internal/parser/codex.go:1`. Needs agent-specific fixtures before adoption.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `A fixture or real sample for this agent demonstrates the upstream behavior gap in the fork.`

### [primary] `4c8e5db5a4a533f44943f8c566849e5c7f10bf2e` — `fix(deps): update go dependencies (#1020)`
- **Category:** `security`
- **Decision:** `defer`
- **Evidence:** Defer dependency maintenance: `git show --name-status 4c8e5db5a4a533f44943f8c566849e5c7f10bf2e` touches dependency manifests `go.mod`, `go.sum`; this needs a dedicated advisory/lockfile/build phase rather than phase 06 docs. Local anchor: `go.mod:32`.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `Dependency update is requested or a security advisory affects the locked package range.`

### [primary] `71231bf7e1f678bfdfd204bb1d292bbde6fef7f8` — `fix(deps): update javascript dependencies (#1021)`
- **Category:** `platform-other`
- **Decision:** `not-applicable`
- **Evidence:** Not applicable to this local web-viewer catch-up scope because `git show --name-status 71231bf7e1f678bfdfd204bb1d292bbde6fef7f8` is desktop/Windows/AppImage-specific: `desktop/package-lock.json`, `frontend/package-lock.json`, `frontend/package.json`. No desktop deliverable is in this run scope.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `none`

### [primary] `b67c396fbc6943303eeb1ac5f4edb2436db1cf18` — `perf(push): ignore volatile stat fields for session candidacy (#1014)`
- **Category:** `sync-correctness`
- **Decision:** `defer`
- **Evidence:** Defer sync/backend correctness change: `git show --name-status b67c396fbc6943303eeb1ac5f4edb2436db1cf18` touches `internal/duckdb/sync.go`, `internal/duckdb/sync_fastpath_test.go`, `internal/postgres/push.go`, `internal/postgres/push_fingerprint.go`, `internal/postgres/push_test.go`; local counterpart/equivalent surface starts at `internal/duckdb/sync.go:19`. Needs backend parity or sync regression tests before adoption.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `A local sync, watcher, PG/DuckDB, daemon, or remote-sync regression maps to this upstream lineage.`

### [primary] `af0288f555e7f4d7b5ec69d5c963794f4b4d4a1c` — `fix(config): apply port from config.toml to Config struct (#1005)`
- **Category:** `archive-schema`
- **Decision:** `defer`
- **Evidence:** Defer platform/CLI/server behavior: `git show --name-status af0288f555e7f4d7b5ec69d5c963794f4b4d4a1c` touches `internal/config/config.go`, `internal/config/config_test.go`; local counterpart/equivalent surface starts at `internal/config/config.go:1`. Needs a scoped acceptance test before adoption.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `A local archive, stats, usage, search, pricing, config, server, or schema bug maps to this upstream lineage.`

### [primary] `05f7d70edcf55978cb3418486c3c7bfbfca28c84` — `fix(usage): wire agent exclusions through usage filters (#972)`
- **Category:** `ui`
- **Decision:** `defer`
- **Evidence:** Defer UI behavior change: `git show --name-status 05f7d70edcf55978cb3418486c3c7bfbfca28c84` touches `frontend/src/lib/components/filters/SessionActiveFilters.svelte`, `frontend/src/lib/components/filters/SessionActiveFilters.test.ts`, `frontend/src/lib/components/usage/AttributionPanel.test.ts`, `frontend/src/lib/components/usage/UsagePage.svelte`, `frontend/src/lib/components/usage/UsagePage.test.ts`, ... (7 paths total); local counterpart/equivalent surface starts at `frontend/src/lib/components/filters/SessionActiveFilters.svelte:4`. Needs frontend tests or visual review before adoption.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `User-facing navigation/filter/rendering issue is reproduced or requested in this fork UI.`

### [primary] `fd40fced3b3660aaf32763ec43e4d552303ad81e` — `fix(frontend): preserve calendar range picker selections (#1016)`
- **Category:** `ui`
- **Decision:** `defer`
- **Evidence:** Defer UI behavior change: `git show --name-status fd40fced3b3660aaf32763ec43e4d552303ad81e` touches `frontend/src/lib/components/shared/rangeSelection.test.ts`, `frontend/src/lib/components/shared/rangeSelection.ts`, `frontend/src/lib/components/trends/TrendsPage.test.ts`; local counterpart/equivalent surface starts at `frontend/src/lib/components/trends/TrendsPage.test.ts:1`. Needs frontend tests or visual review before adoption.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `User-facing navigation/filter/rendering issue is reproduced or requested in this fork UI.`

### [primary] `4914e43fcfd46f19fb39c0e34beb15ddf24a5b99` — `fix(sync): drop unchanged opencode-family container sessions (#1015)`
- **Category:** `sync-correctness`
- **Decision:** `defer`
- **Evidence:** Defer sync/backend correctness change: `git show --name-status 4914e43fcfd46f19fb39c0e34beb15ddf24a5b99` touches `internal/db/project_identity_test.go`, `internal/insight/generate_test.go`, `internal/parser/devin_test.go`, `internal/parser/opencode.go`, `internal/parser/opencode_provider.go`, ... (7 paths total); local counterpart/equivalent surface starts at `internal/insight/generate_test.go:1`. Needs backend parity or sync regression tests before adoption.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `A local sync, watcher, PG/DuckDB, daemon, or remote-sync regression maps to this upstream lineage.`

### [primary] `4a5cca97b0b22a8b98a96899aa19390858f5fc01` — `fix(sync): skip local git discovery for foreign-machine sessions (#1008)`
- **Category:** `sync-correctness`
- **Decision:** `defer`
- **Evidence:** Defer sync/backend correctness change: `git show --name-status 4a5cca97b0b22a8b98a96899aa19390858f5fc01` touches `internal/db/project_identity.go`, `internal/export/project_identity.go`, `internal/export/project_identity_test.go`, `internal/sync/engine.go`, `internal/sync/engine_test.go`; local counterpart/equivalent surface starts at `internal/sync/engine.go:1`. Needs backend parity or sync regression tests before adoption.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `A local sync, watcher, PG/DuckDB, daemon, or remote-sync regression maps to this upstream lineage.`

### [primary] `6a40a4fcd82d9b5c2416896f863bbaef58af8285` — `fix(activity): count subagent sessions in activity report cost (#1006)`
- **Category:** `ui`
- **Decision:** `defer`
- **Evidence:** Defer UI behavior change: `git show --name-status 6a40a4fcd82d9b5c2416896f863bbaef58af8285` touches `docs/activity.md`, `internal/activity/parity_pgtest_test.go`, `internal/db/activityreport.go`, `internal/db/activityreport_test.go`, `internal/db/analytics.go`, ... (12 paths total); local counterpart/equivalent surface starts at `internal/db/analytics.go:811`. Needs frontend tests or visual review before adoption.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `User-facing navigation/filter/rendering issue is reproduced or requested in this fork UI.`

### [primary] `f356e52de0e76bcdb3a6094605fbd284dd6b29cb` — `fix(deps): update javascript dependencies and kit-ui conformance (#996)`
- **Category:** `security`
- **Decision:** `defer`
- **Evidence:** Defer dependency maintenance: `git show --name-status f356e52de0e76bcdb3a6094605fbd284dd6b29cb` touches dependency manifests `docs/screenshots/package-lock.json`, `docs/screenshots/package.json`, `frontend/messages/en.json`, `frontend/messages/zh-CN.json`, `frontend/messages/zh-TW.json`, ... (31 paths total); this needs a dedicated advisory/lockfile/build phase rather than phase 06 docs. Local anchor: `frontend/package-lock.json:2984`.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `Dependency update is requested or a security advisory affects the locked package range.`

### [primary] `06f456e9bf52b76c2f1bfed49e519edcddede358` — `fix(parser): map OMP parentSession header to ParentSessionID (#995)`
- **Category:** `archive-schema`
- **Decision:** `defer`
- **Evidence:** Defer archive/schema/data behavior change: `git show --name-status 06f456e9bf52b76c2f1bfed49e519edcddede358` touches `internal/parser/pi.go`, `internal/parser/pi_test.go`; local counterpart/equivalent surface starts at `internal/parser/pi.go:1`. Needs data-preservation and backend-parity tests before adoption.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `A local archive, stats, usage, search, pricing, or schema bug maps to this upstream lineage.`

### [primary] `3ce0acd2457362a69f36e81ad4f5caa3e3e9ffb6` — `perf: reduce DuckDB push write amplification (#992)`
- **Category:** `sync-correctness`
- **Decision:** `defer`
- **Evidence:** Defer sync/backend correctness change: `git show --name-status 3ce0acd2457362a69f36e81ad4f5caa3e3e9ffb6` touches `internal/db/messages.go`, `internal/duckdb/checkpoint.go`, `internal/duckdb/connect.go`, `internal/duckdb/push.go`, `internal/duckdb/push_fingerprint.go`, ... (12 paths total); local counterpart/equivalent surface starts at `internal/db/messages.go:542`. Needs backend parity or sync regression tests before adoption.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `A local sync, watcher, PG/DuckDB, daemon, or remote-sync regression maps to this upstream lineage.`

### [primary] `3b26fee85fd477d83e7dea8aff4a76ac457958a1` — `fix(postgres): scope alias backfill marker per target (#971)`
- **Category:** `sync-correctness`
- **Decision:** `defer`
- **Evidence:** Defer sync/backend correctness change: `git show --name-status 3b26fee85fd477d83e7dea8aff4a76ac457958a1` touches `internal/postgres/push.go`, `internal/postgres/push_test.go`, `internal/postgres/sync.go`; local counterpart/equivalent surface starts at `internal/postgres/push.go:1`. Needs backend parity or sync regression tests before adoption.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `A local sync, watcher, PG/DuckDB, daemon, or remote-sync regression maps to this upstream lineage.`

### [primary] `5dd1db648e8a69a053b765bc115338e6395824f4` — `fix(parser): discover OMP sessions with a leading title slot (#980)`
- **Category:** `archive-schema`
- **Decision:** `defer`
- **Evidence:** Defer archive/schema/data behavior change: `git show --name-status 5dd1db648e8a69a053b765bc115338e6395824f4` touches `internal/parser/discovery.go`, `internal/parser/discovery_test.go`, `internal/parser/pi.go`, `internal/parser/pi_provider_test.go`, `internal/parser/pi_test.go`; local counterpart/equivalent surface starts at `internal/parser/discovery.go:1`. Needs data-preservation and backend-parity tests before adoption.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `A local archive, stats, usage, search, pricing, or schema bug maps to this upstream lineage.`

### [primary] `167075861ff2595c69d4269f3ca38c627ad21ed8` — `fix(sync): reparse same-size same-mtime in-place Claude rewrites (#975)`
- **Category:** `sync-correctness`
- **Decision:** `defer`
- **Evidence:** Defer sync/backend correctness change: `git show --name-status 167075861ff2595c69d4269f3ca38c627ad21ed8` touches `internal/sync/engine.go`, `internal/sync/engine_integration_test.go`; local counterpart/equivalent surface starts at `internal/sync/engine.go:1`. Needs backend parity or sync regression tests before adoption.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `A local sync, watcher, PG/DuckDB, daemon, or remote-sync regression maps to this upstream lineage.`

### [primary] `ca0b5c6ed62925251749017191d4ef3c29afabff` — `Fix DuckDB push schema setup through Quack (#945)`
- **Category:** `sync-correctness`
- **Decision:** `defer`
- **Evidence:** Defer sync/backend correctness change: `git show --name-status ca0b5c6ed62925251749017191d4ef3c29afabff` touches `cmd/agentsview/archive_write_backend.go`, `cmd/agentsview/archive_write_backend_test.go`, `cmd/agentsview/duckdb.go`, `cmd/agentsview/duckdb_test.go`, `internal/db/db.go`, ... (34 paths total); local counterpart/equivalent surface starts at `cmd/agentsview/duckdb.go:19`. Needs backend parity or sync regression tests before adoption.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `A local sync, watcher, PG/DuckDB, daemon, or remote-sync regression maps to this upstream lineage.`

### [primary] `6dfaef6734b83c3bfcabeb04416ae4f50d6c255b` — `fix(cli): avoid legacy Copilot billing wording (#952)`
- **Category:** `active-agent`
- **Decision:** `defer`
- **Evidence:** Defer active-agent parser behavior: `git show --name-status 6dfaef6734b83c3bfcabeb04416ae4f50d6c255b` touches `cmd/agentsview/usage.go`, `cmd/agentsview/usage_test.go`; local counterpart/equivalent parser surface starts at `cmd/agentsview/usage.go:28`. Needs agent-specific fixtures before adoption.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `A fixture or real sample for this agent demonstrates the upstream behavior gap in the fork.`

### [primary] `b496abb065d8bb03e2946a57354b0808d4e2197a` — `fix(usage): surface unsupported Copilot usage filters (#947)`
- **Category:** `ui`
- **Decision:** `defer`
- **Evidence:** Defer UI behavior change: `git show --name-status b496abb065d8bb03e2946a57354b0808d4e2197a` touches `frontend/messages/en.json`, `frontend/messages/zh-CN.json`, `frontend/messages/zh-TW.json`, `frontend/src/lib/api/generated/index.ts`, `frontend/src/lib/api/generated/models/UnsupportedUsage.ts`, ... (25 paths total); local counterpart/equivalent surface starts at `frontend/src/lib/api/generated/index.ts:26`. Needs frontend tests or visual review before adoption.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `User-facing navigation/filter/rendering issue is reproduced or requested in this fork UI.`

### [primary] `9e1369cd55143eeed24cd9bfdd08dd7fc91e5fd1` — `fix(test): keep a slow template checkpoint from poisoning the Windows suite (#951)`
- **Category:** `platform-other`
- **Decision:** `not-applicable`
- **Evidence:** Not applicable to runtime/data correctness for this fork phase: `git show --name-status 9e1369cd55143eeed24cd9bfdd08dd7fc91e5fd1` shows this commit is limited to CI, docs, screenshots, or test harness paths: `internal/db/db_test.go`, `internal/db/store_contract_test.go`, `internal/db/template_fallback_test.go`, `internal/dbtest/dbtest.go`, `internal/dbtest/dbtest_fallback_test.go`, ... (6 paths total).
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `none`

### [primary] `5fc0891b872c9d7b79f6b0511d91f5ceb52e843a` — `perf: reduce daemon sync CPU on streaming sessions (#954)`
- **Category:** `sync-correctness`
- **Decision:** `defer`
- **Evidence:** Defer sync/backend correctness change: `git show --name-status 5fc0891b872c9d7b79f6b0511d91f5ceb52e843a` touches `cmd/agentsview/archive_query_backend.go`, `cmd/agentsview/archive_write_backend.go`, `cmd/agentsview/cli.go`, `cmd/agentsview/main.go`, `cmd/agentsview/session_sync.go`, ... (25 paths total); local counterpart/equivalent surface starts at `cmd/agentsview/cli.go:59`. Needs backend parity or sync regression tests before adoption.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `A local sync, watcher, PG/DuckDB, daemon, or remote-sync regression maps to this upstream lineage.`

### [primary] `b2b63da9dcde76c948de942abb0510396a70b7bf` — `fix(parser): support Visual Studio 2026 Copilot sessions (#919)`
- **Category:** `active-agent`
- **Decision:** `defer`
- **Evidence:** Visual Studio Copilot session support maps to an active local agent family, not generic sync. Local registry includes VSCode Copilot defaults at `internal/parser/types.go:221`; port needs Visual Studio 2026 fixture coverage.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `Re-evaluate with Visual Studio 2026 Copilot session fixtures or user data.`

### [primary] `307568e08ab1a884e478237e234d9c4c6d7c7780` — `fix(postgres): don't let skipped ownership conflicts block the session-alias backfill marker (#940)`
- **Category:** `sync-correctness`
- **Decision:** `defer`
- **Evidence:** Defer sync/backend correctness change: `git show --name-status 307568e08ab1a884e478237e234d9c4c6d7c7780` touches `internal/postgres/push.go`, `internal/postgres/push_test.go`; local counterpart/equivalent surface starts at `internal/postgres/push.go:1`. Needs backend parity or sync regression tests before adoption.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `A local sync, watcher, PG/DuckDB, daemon, or remote-sync regression maps to this upstream lineage.`

### [primary] `9defe23459c6669b4d4ddae24ea1630b273c6bd7` — `fix(cli): require daemon transport for read commands (#933)`
- **Category:** `ui`
- **Decision:** `defer`
- **Evidence:** Defer UI behavior change: `git show --name-status 9defe23459c6669b4d4ddae24ea1630b273c6bd7` touches `cmd/agentsview/archive_query_backend.go`, `cmd/agentsview/archive_query_backend_test.go`, `cmd/agentsview/cli.go`, `cmd/agentsview/daemon_runtime.go`, `cmd/agentsview/daemon_runtime_test.go`, ... (41 paths total); local counterpart/equivalent surface starts at `cmd/agentsview/cli.go:139`. Needs frontend tests or visual review before adoption.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `User-facing navigation/filter/rendering issue is reproduced or requested in this fork UI.`

### [primary] `4f240b390a1bdd5fd7409658ff6921c56fb6fd7d` — `fix(desktop): surface backend startup failures (#932)`
- **Category:** `platform-other`
- **Decision:** `not-applicable`
- **Evidence:** Not applicable to this local web-viewer catch-up scope because `git show --name-status 4f240b390a1bdd5fd7409658ff6921c56fb6fd7d` is desktop/Windows/AppImage-specific: `Makefile`, `cmd/agentsview/serve_lifecycle.go`, `cmd/agentsview/serve_lifecycle_test.go`, `desktop/scripts/run-tauri.sh`, `desktop/src-tauri/src/lib.rs`. No desktop deliverable is in this run scope.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `none`

### [primary] `811dd8a8b0990f230f3810c00e47d740133fff15` — `fix(desktop): validate updater signatures (#931)`
- **Category:** `platform-other`
- **Decision:** `not-applicable`
- **Evidence:** Not applicable to the current fork because the upstream target path set is absent at frozen `local_head`; examples from `git show --name-only`: `desktop/scripts/repair-appimage-diricon.sh`, `desktop/scripts/test-repair-appimage-diricon.sh`, `scripts/check_desktop_release_health.sh`, `scripts/check_desktop_release_health_from_event_test.sh`, `scripts/check_desktop_release_health_test.sh`. Verified absence anchor: path absent at frozen local_head: `desktop/scripts/repair-appimage-diricon.sh`, `desktop/scripts/test-repair-appimage-diricon.sh`, `scripts/check_desktop_release_health.sh`.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `none`

### [primary] `763b196034fdd0f31be2667921618c0ec58402ba` — `fix(desktop): persist sidecar logs for desktop builds (#921)`
- **Category:** `platform-other`
- **Decision:** `not-applicable`
- **Evidence:** Not applicable to this local web-viewer catch-up scope because `git show --name-status 763b196034fdd0f31be2667921618c0ec58402ba` is desktop/Windows/AppImage-specific: `desktop/src-tauri/src/lib.rs`. No desktop deliverable is in this run scope.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `none`

### [primary] `292a4eebfa5a4103450c671fad65f6bc7aadd430` — `fix(import): include Claude.ai export attachments (#913)`
- **Category:** `active-agent`
- **Decision:** `defer`
- **Evidence:** Defer active-agent parser behavior: `git show --name-status 292a4eebfa5a4103450c671fad65f6bc7aadd430` touches `internal/importer/importer.go`, `internal/importer/importer_test.go`, `internal/parser/claude_ai.go`, `internal/parser/claude_ai_test.go`; local counterpart/equivalent parser surface starts at `internal/importer/importer.go:1`. Needs agent-specific fixtures before adoption.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `A fixture or real sample for this agent demonstrates the upstream behavior gap in the fork.`

### [primary] `6804c297c52bfad176baf3e1a1813cbe77e416c4` — `fix(sync,parser): fix resync discovery perf regression + report it correctly (#912)`
- **Category:** `sync-correctness`
- **Decision:** `defer`
- **Evidence:** Defer sync/backend correctness change: `git show --name-status 6804c297c52bfad176baf3e1a1813cbe77e416c4` touches `internal/parser/antigravity_cli.go`, `internal/parser/antigravity_cli_provider.go`, `internal/parser/antigravity_provider_test.go`, `internal/parser/capabilitysupport_enumer.go`, `internal/parser/discovery_test.go`, ... (12 paths total); local counterpart/equivalent surface starts at `internal/parser/antigravity_cli.go:154`. Needs backend parity or sync regression tests before adoption.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `A local sync, watcher, PG/DuckDB, daemon, or remote-sync regression maps to this upstream lineage.`

### [primary] `48e3be802bd2dcde81543865ecf75d02fa2829f2` — `fix(sync): show timed remote daemon progress (#911)`
- **Category:** `sync-correctness`
- **Decision:** `defer`
- **Evidence:** Defer sync/backend correctness change: `git show --name-status 48e3be802bd2dcde81543865ecf75d02fa2829f2` touches `cmd/agentsview/sync.go`, `cmd/agentsview/sync_test.go`; local counterpart/equivalent surface starts at `cmd/agentsview/sync.go:1`. Needs backend parity or sync regression tests before adoption.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `A local sync, watcher, PG/DuckDB, daemon, or remote-sync regression maps to this upstream lineage.`

### [primary] `e336672c718609fec0c8c414699f03f09eba8b53` — `fix(deps): update javascript dependencies (#908)`
- **Category:** `platform-other`
- **Decision:** `not-applicable`
- **Evidence:** Not applicable to this local web-viewer catch-up scope because `git show --name-status e336672c718609fec0c8c414699f03f09eba8b53` is desktop/Windows/AppImage-specific: `desktop/package-lock.json`, `docs/screenshots/package-lock.json`, `docs/screenshots/package.json`, `frontend/package-lock.json`, `frontend/package.json`. No desktop deliverable is in this run scope.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `none`

### [primary] `2ba08c4fa346b12edd23dc2d53dbfdad9f21c4b4` — `fix(deps): update go dependencies (#907)`
- **Category:** `security`
- **Decision:** `defer`
- **Evidence:** Defer dependency maintenance: `git show --name-status 2ba08c4fa346b12edd23dc2d53dbfdad9f21c4b4` touches dependency manifests `go.mod`, `go.sum`; this needs a dedicated advisory/lockfile/build phase rather than phase 06 docs. Local anchor: `go.mod:1`.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `Dependency update is requested or a security advisory affects the locked package range.`

### [primary] `f69873021fa43f0c58cd3a860cb525066d47a26c` — `fix(usage): accept duration syntax for usage daily --since/--until (#905)`
- **Category:** `archive-schema`
- **Decision:** `defer`
- **Evidence:** Defer archive/schema/data behavior change: `git show --name-status f69873021fa43f0c58cd3a860cb525066d47a26c` touches `cmd/agentsview/cli.go`, `cmd/agentsview/usage.go`, `cmd/agentsview/usage_test.go`, `docs/commands.md`, `docs/token-usage.md`, ... (7 paths total); local counterpart/equivalent surface starts at `cmd/agentsview/cli.go:20`. Needs data-preservation and backend-parity tests before adoption.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `A local archive, stats, usage, search, pricing, or schema bug maps to this upstream lineage.`

### [primary] `a4584de40117cb04d478b8bb22a1f817721bcda6` — `fix(sync): repair persisted Codex goal-context rows (#874)`
- **Category:** `ui`
- **Decision:** `defer`
- **Evidence:** Defer UI behavior change: `git show --name-status a4584de40117cb04d478b8bb22a1f817721bcda6` touches `frontend/src/lib/utils/messages.test.ts`, `frontend/src/lib/utils/messages.ts`, `internal/db/db.go`, `internal/db/db_test.go`, `internal/db/search.go`, ... (17 paths total); local counterpart/equivalent surface starts at `frontend/src/lib/utils/messages.test.ts:3`. Needs frontend tests or visual review before adoption.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `User-facing navigation/filter/rendering issue is reproduced or requested in this fork UI.`

### [primary] `18f3acebb5562cc971e13d31389e40798cc705a9` — `fix(config): honor CLAUDE_CONFIG_DIR for Claude session discovery (#897)`
- **Category:** `archive-schema`
- **Decision:** `defer`
- **Evidence:** Defer archive/schema/data behavior change: `git show --name-status 18f3acebb5562cc971e13d31389e40798cc705a9` touches `cmd/agentsview/pg_service_manager.go`, `cmd/agentsview/pg_service_test.go`, `internal/config/config.go`, `internal/config/config_test.go`, `internal/parser/types.go`, ... (7 paths total); local counterpart/equivalent surface starts at `cmd/agentsview/pg_service_manager.go:14`. Needs data-preservation and backend-parity tests before adoption.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `A local archive, stats, usage, search, pricing, or schema bug maps to this upstream lineage.`

### [primary] `529d997e49207df94a523a443d90cad5ab5cf7a7` — `fix(frontend): localize remaining UI copy (#896)`
- **Category:** `ui`
- **Decision:** `defer`
- **Evidence:** Defer UI behavior change: `git show --name-status 529d997e49207df94a523a443d90cad5ab5cf7a7` touches `frontend/messages/en.json`, `frontend/messages/zh-CN.json`, `frontend/src/lib/components/content/MessageContent.test.ts`, `frontend/src/lib/components/content/SkillBlock.svelte`, `frontend/src/lib/components/content/SkillBlock.test.ts`, ... (17 paths total); local counterpart/equivalent surface starts at `frontend/src/lib/components/content/MessageContent.test.ts:8`. Needs frontend tests or visual review before adoption.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `User-facing navigation/filter/rendering issue is reproduced or requested in this fork UI.`

### [primary] `634d930ba7c8dcf2dfd12e381bcd1720a3e0ffd7` — `fix(analytics): define top-session active duration as clamped idle gaps (alt to #867) (#869)`
- **Category:** `ui`
- **Decision:** `defer`
- **Evidence:** Defer UI behavior change: `git show --name-status 634d930ba7c8dcf2dfd12e381bcd1720a3e0ffd7` touches `frontend/messages/en.json`, `frontend/messages/zh-CN.json`, `frontend/src/lib/api/generated/models/DbTopSession.ts`, `frontend/src/lib/api/types/analytics.ts`, `frontend/src/lib/components/analytics/TopSessions.svelte`, ... (14 paths total); local counterpart/equivalent surface starts at `frontend/src/lib/api/generated/models/DbTopSession.ts:5`. Needs frontend tests or visual review before adoption.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `User-facing navigation/filter/rendering issue is reproduced or requested in this fork UI.`

### [primary] `e5bbfd6cb74360c77e9d3873e3a35a23628eaac4` — `fix(postgres): scope filtered push watermarks (#894)`
- **Category:** `sync-correctness`
- **Decision:** `defer`
- **Evidence:** Defer sync/backend correctness change: `git show --name-status e5bbfd6cb74360c77e9d3873e3a35a23628eaac4` touches `cmd/agentsview/cli.go`, `cmd/agentsview/cli_test.go`, `cmd/agentsview/pg.go`, `cmd/agentsview/pg_service.go`, `cmd/agentsview/pg_test.go`, ... (15 paths total); local counterpart/equivalent surface starts at `cmd/agentsview/cli.go:378`. Needs backend parity or sync regression tests before adoption.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `A local sync, watcher, PG/DuckDB, daemon, or remote-sync regression maps to this upstream lineage.`

### [primary] `746242d08c3f9e721792cbdbdfb343bd0fcbfe5d` — `fix(agentsview): skip schema DDL for compatible pg push (#888)`
- **Category:** `sync-correctness`
- **Decision:** `defer`
- **Evidence:** Defer sync/backend correctness change: `git show --name-status 746242d08c3f9e721792cbdbdfb343bd0fcbfe5d` touches `internal/postgres/push.go`, `internal/postgres/schema.go`, `internal/postgres/schema_test.go`, `internal/postgres/sync.go`; local counterpart/equivalent surface starts at `internal/postgres/push.go:14`. Needs backend parity or sync regression tests before adoption.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `A local sync, watcher, PG/DuckDB, daemon, or remote-sync regression maps to this upstream lineage.`

### [primary] `f52291e84ab306a552cbfc889f68b8e2b495c036` — `fix(server): avoid fixed-port race in server tests (#893)`
- **Category:** `archive-schema`
- **Decision:** `defer`
- **Evidence:** Defer platform/CLI/server behavior: `git show --name-status f52291e84ab306a552cbfc889f68b8e2b495c036` touches `internal/server/server.go`, `internal/server/server_test.go`; local counterpart/equivalent surface starts at `internal/server/server.go:1`. Needs a scoped acceptance test before adoption.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `A local archive, stats, usage, search, pricing, config, server, or schema bug maps to this upstream lineage.`

### [primary] `24de50f1b5ea97625e4f0b5b50d8345ab319ea5b` — `fix(analytics): Count subagent sessions in analytics totals (#873)`
- **Category:** `ui`
- **Decision:** `defer`
- **Evidence:** Defer UI behavior change: `git show --name-status 24de50f1b5ea97625e4f0b5b50d8345ab319ea5b` touches `cmd/agentsview/stats.go`, `cmd/agentsview/testdata/stats_golden.json`, `frontend/e2e/session-count-consistency.spec.ts`, `internal/db/analytics.go`, `internal/db/analytics_test.go`, ... (13 paths total); local counterpart/equivalent surface starts at `cmd/agentsview/stats.go:2`. Needs frontend tests or visual review before adoption.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `User-facing navigation/filter/rendering issue is reproduced or requested in this fork UI.`

### [primary] `9d7260990e191d4e74fbeadb62c8173d63d1624f` — `fix(frontend): polish remaining zh-CN localization (#875)`
- **Category:** `ui`
- **Decision:** `defer`
- **Evidence:** Defer UI behavior change: `git show --name-status 9d7260990e191d4e74fbeadb62c8173d63d1624f` touches `frontend/e2e/pages/sessions-page.ts`, `frontend/e2e/termination.spec.ts`, `frontend/e2e/virtual-list.spec.ts`, `frontend/messages/en.json`, `frontend/messages/zh-CN.json`, ... (25 paths total); local counterpart/equivalent surface starts at `frontend/e2e/pages/sessions-page.ts:1`. Needs frontend tests or visual review before adoption.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `User-facing navigation/filter/rendering issue is reproduced or requested in this fork UI.`

### [primary] `5058dc7b9f16ead6db62fe55fe5950a0719e70f0` — `fix(sync): detect Codex title-only renames in full sync regardless of index mtime (#863)`
- **Category:** `sync-correctness`
- **Decision:** `defer`
- **Evidence:** Defer sync/backend correctness change: `git show --name-status 5058dc7b9f16ead6db62fe55fe5950a0719e70f0` touches `internal/sync/engine.go`, `internal/sync/engine_test.go`; local counterpart/equivalent surface starts at `internal/sync/engine.go:1`. Needs backend parity or sync regression tests before adoption.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `A local sync, watcher, PG/DuckDB, daemon, or remote-sync regression maps to this upstream lineage.`

### [primary] `213295f5e276c9fd713e041af988361d5e3bb19d` — `fix(desktop): repair AppImage DirIcon after bundling (#857)`
- **Category:** `platform-other`
- **Decision:** `not-applicable`
- **Evidence:** Not applicable to this local web-viewer catch-up scope because `git show --name-status 213295f5e276c9fd713e041af988361d5e3bb19d` is desktop/Windows/AppImage-specific: `.github/workflows/ci.yml`, `.github/workflows/desktop-artifacts.yml`, `.github/workflows/desktop-release.yml`, `Makefile`, `ci_workflow_test.go`, ... (7 paths total). No desktop deliverable is in this run scope.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `none`

### [primary] `4ae6071fd20bff480f7eae8f822878fd99d53de7` — `fix(sync): stream remote progress through daemon (#854)`
- **Category:** `sync-correctness`
- **Decision:** `defer`
- **Evidence:** Defer sync/backend correctness change: `git show --name-status 4ae6071fd20bff480f7eae8f822878fd99d53de7` touches `cmd/agentsview/sync.go`, `cmd/agentsview/sync_test.go`, `internal/server/huma_routes_sync.go`, `internal/server/huma_routes_sync_internal_test.go`, `internal/ssh/sync.go`; local counterpart/equivalent surface starts at `cmd/agentsview/sync.go:1`. Needs backend parity or sync regression tests before adoption.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `A local sync, watcher, PG/DuckDB, daemon, or remote-sync regression maps to this upstream lineage.`

### [primary] `a612e65ec6b63b18aa03890e408304aef04b8580` — `fix(desktop): use native desktop zoom on Windows (#850)`
- **Category:** `platform-other`
- **Decision:** `not-applicable`
- **Evidence:** Not applicable to this local web-viewer catch-up scope because `git show --name-status a612e65ec6b63b18aa03890e408304aef04b8580` is desktop/Windows/AppImage-specific: `desktop/src-tauri/capabilities/default.json`, `desktop/src-tauri/src/lib.rs`, `desktop/src-tauri/tauri.conf.json`, `frontend/e2e/appearance-a11y.spec.ts`, `frontend/src/lib/stores/ui.svelte.ts`, ... (6 paths total). No desktop deliverable is in this run scope.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `none`

### [primary] `e6852137d921f3df8a979fe5c1d3ed88d2af9733` — `fix(insights): stop forcing Gemini sandboxed insight runs (#852)`
- **Category:** `active-agent`
- **Decision:** `defer`
- **Evidence:** Defer platform/CLI/server behavior: `git show --name-status e6852137d921f3df8a979fe5c1d3ed88d2af9733` touches `.roborev.toml`, `internal/config/config.go`, `internal/config/config_test.go`, `internal/insight/generate.go`, `internal/insight/generate_test.go`, ... (7 paths total); local counterpart/equivalent surface starts at `.roborev.toml:89`. Needs a scoped acceptance test before adoption.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `A fixture or real sample for this agent demonstrates the upstream behavior gap in the fork.`

### [primary] `3de4c63c6649f6ece93b1a8e50a49b78ee3521ad` — `fix(postgres): fail blocked pg push and surface push errors (#849)`
- **Category:** `sync-correctness`
- **Decision:** `defer`
- **Evidence:** Defer sync/backend correctness change: `git show --name-status 3de4c63c6649f6ece93b1a8e50a49b78ee3521ad` touches `cmd/agentsview/pg.go`, `cmd/agentsview/pg_test.go`, `internal/db/orphaned.go`, `internal/postgres/push.go`; local counterpart/equivalent surface starts at `cmd/agentsview/pg.go:18`. Needs backend parity or sync regression tests before adoption.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `A local sync, watcher, PG/DuckDB, daemon, or remote-sync regression maps to this upstream lineage.`

### [primary] `caf34f2a9036e5e4404b848600bca8ec3b692047` — `fix(parser): collapse bare roborev CI worktrees (#848)`
- **Category:** `active-agent`
- **Decision:** `defer`
- **Evidence:** Defer platform/CLI/server behavior: `git show --name-status caf34f2a9036e5e4404b848600bca8ec3b692047` touches `internal/parser/project.go`, `internal/parser/project_git_test.go`; local counterpart/equivalent surface starts at `internal/parser/project.go:1`. Needs a scoped acceptance test before adoption.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `A fixture or real sample for this agent demonstrates the upstream behavior gap in the fork.`

### [primary] `b8d8542519aeb51245a07689538ebc3f69408c1c` — `fix(sync): close daemon progress gaps (#845)`
- **Category:** `sync-correctness`
- **Decision:** `defer`
- **Evidence:** Defer sync/backend correctness change: `git show --name-status b8d8542519aeb51245a07689538ebc3f69408c1c` touches `cmd/agentsview/main.go`, `cmd/agentsview/main_test.go`, `cmd/agentsview/sync.go`, `cmd/agentsview/sync_test.go`, `cmd/agentsview/usage.go`, ... (7 paths total); local counterpart/equivalent surface starts at `cmd/agentsview/main.go:14`. Needs backend parity or sync regression tests before adoption.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `A local sync, watcher, PG/DuckDB, daemon, or remote-sync regression maps to this upstream lineage.`

### [primary] `70849048867009d3950a286a8e8b94a25bf9798d` — `fix(parser): compute per-turn Gemini context tokens as delta (#837)`
- **Category:** `archive-schema`
- **Decision:** `defer`
- **Evidence:** Defer archive/schema/data behavior change: `git show --name-status 70849048867009d3950a286a8e8b94a25bf9798d` touches `internal/db/db.go`, `internal/db/db_test.go`, `internal/parser/gemini.go`, `internal/parser/gemini_parser_test.go`; local counterpart/equivalent surface starts at `internal/db/db.go:21`. Needs data-preservation and backend-parity tests before adoption.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `A local archive, stats, usage, search, pricing, or schema bug maps to this upstream lineage.`

### [primary] `a17cd6a226eb77988e962ca2d1ba69cba9072e78` — `fix(postgres): reset push watermarks on target changes (#829)`
- **Category:** `sync-correctness`
- **Decision:** `defer`
- **Evidence:** Defer sync/backend correctness change: `git show --name-status a17cd6a226eb77988e962ca2d1ba69cba9072e78` touches `internal/postgres/connect.go`, `internal/postgres/connect_test.go`, `internal/postgres/push.go`, `internal/postgres/push_test.go`, `internal/postgres/sync.go`, ... (6 paths total); local counterpart/equivalent surface starts at `internal/postgres/connect.go:1`. Needs backend parity or sync regression tests before adoption.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `A local sync, watcher, PG/DuckDB, daemon, or remote-sync regression maps to this upstream lineage.`

### [primary] `f6199d466231cfa28941531f90fb918a598c6144` — `fix(usage): dedupe replayed continued-session rows (#828)`
- **Category:** `sync-correctness`
- **Decision:** `defer`
- **Evidence:** Defer sync/backend correctness change: `git show --name-status f6199d466231cfa28941531f90fb918a598c6144` touches `internal/activity/activity.go`, `internal/activity/usage_test.go`, `internal/db/activityreport.go`, `internal/db/activityreport_test.go`, `internal/db/usage.go`, ... (15 paths total); local counterpart/equivalent surface starts at `internal/db/usage.go:20`. Needs backend parity or sync regression tests before adoption.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `A local sync, watcher, PG/DuckDB, daemon, or remote-sync regression maps to this upstream lineage.`

### [primary] `650aee1236cc8e8a843eee3619c67790d0ed2b3e` — `fix(session): keep pg reads opt-in (#820)`
- **Category:** `sync-correctness`
- **Decision:** `defer`
- **Evidence:** Defer sync/backend correctness change: `git show --name-status 650aee1236cc8e8a843eee3619c67790d0ed2b3e` touches `cmd/agentsview/classifier.go`, `cmd/agentsview/classifier_test.go`, `cmd/agentsview/session.go`, `cmd/agentsview/session_test.go`, `docs/commands.md`, ... (6 paths total); local counterpart/equivalent surface starts at `cmd/agentsview/classifier.go:24`. Needs backend parity or sync regression tests before adoption.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `A local sync, watcher, PG/DuckDB, daemon, or remote-sync regression maps to this upstream lineage.`

### [primary] `1fbd8dbf001e6d1265a19cd130da3820c05a1309` — `fix(sync): avoid photo media prompts during Aider scan (#818)`
- **Category:** `macos-runtime`
- **Decision:** `defer`
- **Evidence:** Defer macOS runtime/discovery fix: `git show --name-status 1fbd8dbf001e6d1265a19cd130da3820c05a1309` touches `internal/parser/aider.go`, `internal/parser/aider_test.go`, `internal/server/export.go`, `internal/server/export_test.go`; local counterpart exists at `internal/server/export.go:607`. Needs a focused macOS filesystem/watch regression, not a ledger-only change.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `A macOS protected-directory, watcher, or media-folder failure is reproduced locally.`

### [primary] `2c427992606dec57a6a4f495335b26cd3f0dd5d1` — `fix(sync): reparse stale roborev CI projects (#814)`
- **Category:** `active-agent`
- **Decision:** `defer`
- **Evidence:** Defer platform/CLI/server behavior: `git show --name-status 2c427992606dec57a6a4f495335b26cd3f0dd5d1` touches `internal/db/sessions.go`, `internal/parser/parser_test.go`, `internal/parser/project.go`, `internal/sync/engine.go`, `internal/sync/engine_test.go`; local counterpart/equivalent surface starts at `internal/db/sessions.go:30`. Needs a scoped acceptance test before adoption.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `A fixture or real sample for this agent demonstrates the upstream behavior gap in the fork.`

### [primary] `edaf475d825eda43f174c87daad286ce1dc26af5` — `Fix stale usage daily refresh and reduce pricing overhead (#817)`
- **Category:** `ui`
- **Decision:** `defer`
- **Evidence:** Defer platform/CLI/server behavior: `git show --name-status edaf475d825eda43f174c87daad286ce1dc26af5` touches `cmd/agentsview/usage.go`, `cmd/agentsview/usage_test.go`, `internal/db/activityreport.go`, `internal/db/pricing_match_test.go`, `internal/db/usage.go`, ... (10 paths total); local counterpart/equivalent surface starts at `cmd/agentsview/usage.go:173`. Needs a scoped acceptance test before adoption.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `User-facing navigation/filter/rendering issue is reproduced or requested in this fork UI.`

### [primary] `1d5b431baad28df4a4f6f77a1b7a6d04b0af4cd1` — `fix(sync): avoid Music prompts during Aider scan (#816)`
- **Category:** `macos-runtime`
- **Decision:** `defer`
- **Evidence:** Defer macOS runtime/discovery fix: `git show --name-status 1d5b431baad28df4a4f6f77a1b7a6d04b0af4cd1` touches `cmd/agentsview/main.go`, `cmd/agentsview/sync_test.go`, `cmd/agentsview/usage.go`, `internal/parser/aider.go`, `internal/parser/aider_test.go`; local counterpart exists at `cmd/agentsview/main.go:14`. Needs a focused macOS filesystem/watch regression, not a ledger-only change.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `A macOS protected-directory, watcher, or media-folder failure is reproduced locally.`

### [primary] `0b1c62e10cf7b175f890f611717665fed6fb028e` — `perf(sync): reduce full resync rebuild costs (#812)`
- **Category:** `ui`
- **Decision:** `defer`
- **Evidence:** Defer UI behavior change: `git show --name-status 0b1c62e10cf7b175f890f611717665fed6fb028e` touches `internal/db/messages_test.go`, `internal/db/session_batch.go`, `internal/signals/heuristics.go`, `internal/signals/heuristics_test.go`; local counterpart/equivalent surface starts at `internal/db/messages_test.go:565`. Needs frontend tests or visual review before adoption.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `User-facing navigation/filter/rendering issue is reproduced or requested in this fork UI.`

### [primary] `d296d07118e9226d42ca3154887e545658000f58` — `fix(search): quote FTS terms so operator-char tokens don't 500 (#804)`
- **Category:** `archive-schema`
- **Decision:** `adopt`
- **Evidence:** Phase 02 landed canonical FTS query quoting: `6ae450f0efb60416b5e0a4cdab7729c91933fe06`. Local anchor `internal/server/search.go:12` delegates to `db.PrepareFTSQuery`.
- **Port cost:** `conflict`
- **Delivery:** `landed:6ae450f0efb60416b5e0a4cdab7729c91933fe06/phase-02`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `none`

### [primary] `4ba4a86d382ab241f3f537b70af6fb2d74df4038` — `fix(parser): parse native kimi code records (#807)`
- **Category:** `active-agent`
- **Decision:** `already-equivalent`
- **Evidence:** Verified equivalent core: local fork has native Kimi Code parser in `internal/parser/kimicode.go:15` and tests beginning at `internal/parser/kimicode_test.go:20`.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `none`

### [primary] `84d55aa6a294757664fc12e5f65bb74eeba11fb0` — `fix(sidebar): add native session links (#460) (#755)`
- **Category:** `ui`
- **Decision:** `defer`
- **Evidence:** Defer UI behavior change: `git show --name-status 84d55aa6a294757664fc12e5f65bb74eeba11fb0` touches `frontend/src/lib/components/sidebar/SessionItem.svelte`, `frontend/src/lib/components/sidebar/SessionList.test.ts`; local counterpart/equivalent surface starts at `frontend/src/lib/components/sidebar/SessionItem.svelte:3`. Needs frontend tests or visual review before adoption.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `User-facing navigation/filter/rendering issue is reproduced or requested in this fork UI.`

### [primary] `398adee5a174d4668e4f6cd62c5ff40f5308915a` — `fix(db): reuse FTS-safe large-session deletes (#801)`
- **Category:** `archive-schema`
- **Decision:** `adopt`
- **Evidence:** Verified backlog gap: `internal/db/session_batch.go:463` still defines `deleteSessionMessagesTx`, and `internal/db/session_batch.go:478` deletes from `messages` directly instead of a shared FTS-safe helper.
- **Port cost:** `conflict`
- **Delivery:** `backlog`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `Backlog item accepted; schedule archive-safety regression tests.`

### [primary] `d37ddc9aa119d4d75d1d2ac35acf78f027881bcf` — `fix(sync): harden incremental JSONL resume state (#800)`
- **Category:** `sync-correctness`
- **Decision:** `adopt`
- **Evidence:** Verified backlog gap: local has `LastClaudeMessageID`/fallback behavior but no persisted `last_entry_uuid` or `next_ordinal` boundary; local anchors `internal/db/messages.go:465` and `internal/sync/engine.go:3982`.
- **Port cost:** `conflict`
- **Delivery:** `backlog`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `Backlog item accepted; schedule parser/sync resume-state phase.`

### [primary] `6784b7e0124d4765daad222ff3f191d065752f99` — `fix(ui): stop Ctrl+K palette from reselecting input on every keystroke (#799)`
- **Category:** `ui`
- **Decision:** `defer`
- **Evidence:** Defer UI behavior change: `git show --name-status 6784b7e0124d4765daad222ff3f191d065752f99` touches `frontend/src/lib/components/command-palette/CommandPalette.svelte`, `frontend/src/lib/components/command-palette/CommandPalette.test.ts`; local counterpart/equivalent surface starts at `frontend/src/lib/components/command-palette/CommandPalette.svelte:244`. Needs frontend tests or visual review before adoption.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `User-facing navigation/filter/rendering issue is reproduced or requested in this fork UI.`

### [primary] `3d070cae6fff66eae044500328ad44383050510d` — `fix(parser): avoid protected home dirs in aider discovery (#793)`
- **Category:** `macos-runtime`
- **Decision:** `not-applicable`
- **Evidence:** Not applicable to the current fork because the upstream target path set is absent at frozen `local_head`; examples from `git show --name-only`: `internal/parser/aider.go`, `internal/parser/aider_test.go`. Verified absence anchor: path absent at frozen local_head: `internal/parser/aider.go`, `internal/parser/aider_test.go`.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `none`

### [primary] `ab75a8bae35f453da1565425d359998fe9c7a79a` — `fix(serve): use CREATE_NO_WINDOW for the Windows background daemon (#786)`
- **Category:** `platform-other`
- **Decision:** `not-applicable`
- **Evidence:** Windows-only background daemon behavior is outside this local web-viewer catch-up scope. `git show --name-status ab75a8bae35f453da1565425d359998fe9c7a79a` touches `.github/workflows/ci.yml`, `ci_workflow_test.go`, and Windows-specific `cmd/agentsview/serve_background_windows.go` / test paths.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `none`

### [primary] `85574232775ffda6b4d6fb11d68385b2dec4f046` — `fix(postgres): batch pg push comparison reads (#331) (#754)`
- **Category:** `sync-correctness`
- **Decision:** `defer`
- **Evidence:** Defer sync/backend correctness change: `git show --name-status 85574232775ffda6b4d6fb11d68385b2dec4f046` touches `internal/postgres/push.go`, `internal/postgres/push_fingerprint.go`, `internal/postgres/push_fingerprint_test.go`, `internal/postgres/push_test.go`; local counterpart/equivalent surface starts at `internal/postgres/push.go:1`. Needs backend parity or sync regression tests before adoption.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `A local sync, watcher, PG/DuckDB, daemon, or remote-sync regression maps to this upstream lineage.`

### [primary] `11a5e97a0de357134aab0fc08ffbefa1e0dd570d` — `fix: run NilAway on pre-push (#750)`
- **Category:** `sync-correctness`
- **Decision:** `defer`
- **Evidence:** Defer sync/backend correctness change: `git show --name-status 11a5e97a0de357134aab0fc08ffbefa1e0dd570d` touches `AGENTS.md`, `Makefile`, `README.md`, `hook_config_test.go`, `prek.toml`; local counterpart/equivalent surface starts at `AGENTS.md:179`. Needs backend parity or sync regression tests before adoption.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `A local sync, watcher, PG/DuckDB, daemon, or remote-sync regression maps to this upstream lineage.`

### [primary] `a88910404834c7eced58a4b23121d5cf4cc58b37` — `fix(frontend): align read-only header refresh label (#745)`
- **Category:** `ui`
- **Decision:** `defer`
- **Evidence:** Defer UI behavior change: `git show --name-status a88910404834c7eced58a4b23121d5cf4cc58b37` touches `frontend/src/lib/components/layout/AppHeader.svelte`, `frontend/src/lib/components/layout/AppHeader.test.ts`; local counterpart/equivalent surface starts at `frontend/src/lib/components/layout/AppHeader.svelte:861`. Needs frontend tests or visual review before adoption.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `User-facing navigation/filter/rendering issue is reproduced or requested in this fork UI.`

### [primary] `86b7f6858ee6e4d4e7d6f63bc1a47a06938139b4` — `fix(frontend): distinguish global sync action (#743)`
- **Category:** `ui`
- **Decision:** `defer`
- **Evidence:** Defer UI behavior change: `git show --name-status 86b7f6858ee6e4d4e7d6f63bc1a47a06938139b4` touches `frontend/src/lib/components/layout/AppHeader.svelte`, `frontend/src/lib/components/layout/AppHeader.test.ts`, `frontend/src/lib/icons.test.ts`, `frontend/src/lib/icons.ts`; local counterpart/equivalent surface starts at `frontend/src/lib/components/layout/AppHeader.svelte:997`. Needs frontend tests or visual review before adoption.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `User-facing navigation/filter/rendering issue is reproduced or requested in this fork UI.`

### [primary] `cefc3ca4d155be0366b79b740420a5f512be64a3` — `fix(activity): align refresh control with filters (#742)`
- **Category:** `ui`
- **Decision:** `not-applicable`
- **Evidence:** Not applicable to the current fork because the upstream target path set is absent at frozen `local_head`; examples from `git show --name-only`: `frontend/src/lib/components/activity/ActivityPage.svelte`, `frontend/src/lib/components/activity/ActivityPage.test.ts`. Verified absence anchor: path absent at frozen local_head: `frontend/src/lib/components/activity/ActivityPage.svelte`, `frontend/src/lib/components/activity/ActivityPage.test.ts`.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `none`

### [primary] `7b0adbe86a5f3c6f27f5cea92df107b9269ad819` — `fix(activity): normalize filter chevrons (#741)`
- **Category:** `ui`
- **Decision:** `defer`
- **Evidence:** Defer UI behavior change: `git show --name-status 7b0adbe86a5f3c6f27f5cea92df107b9269ad819` touches `frontend/src/lib/components/activity/ActivityPage.svelte`, `frontend/src/lib/components/activity/ActivityPage.test.ts`, `frontend/src/lib/components/layout/ProjectTypeahead.svelte`; local counterpart/equivalent surface starts at `frontend/src/lib/components/layout/ProjectTypeahead.svelte:31`. Needs frontend tests or visual review before adoption.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `User-facing navigation/filter/rendering issue is reproduced or requested in this fork UI.`

### [primary] `cd59527cc9e1a25c16c8869158ba63f2ae0eaca4` — `fix(deps): update dependency dompurify to v3.4.11 [security] (#732)`
- **Category:** `security`
- **Decision:** `adopt`
- **Evidence:** Superseded by phase 05 dompurify 3.4.13 landing `3ebf2fad80ba03d7554da587eacadba1072cf7e4`. Local anchor `frontend/package.json:1`; 3.4.13 is newer than 3.4.11.
- **Port cost:** `conflict`
- **Delivery:** `landed:3ebf2fad80ba03d7554da587eacadba1072cf7e4/phase-05`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `none`

### [primary] `ebf14c1b43fac6db63fc731494c458d983da5abc` — `fix(postgres): guard pg push against same-id cross-machine row collision (#724)`
- **Category:** `sync-correctness`
- **Decision:** `defer`
- **Evidence:** Defer sync/backend correctness change: `git show --name-status ebf14c1b43fac6db63fc731494c458d983da5abc` touches `cmd/agentsview/pg.go`, `cmd/agentsview/pg_test.go`, `cmd/agentsview/pg_watch.go`, `cmd/agentsview/pg_watch_test.go`, `internal/db/db.go`, ... (16 paths total); local counterpart/equivalent surface starts at `cmd/agentsview/pg.go:18`. Needs backend parity or sync regression tests before adoption.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `A local sync, watcher, PG/DuckDB, daemon, or remote-sync regression maps to this upstream lineage.`

### [primary] `b93deda24c6df13ae96dc176e94f9eb01c7b6b5b` — `fix(db): promote orphan subagent sessions to sidebar roots (#725)`
- **Category:** `archive-schema`
- **Decision:** `defer`
- **Evidence:** Verified product conflict: local tests explicitly exclude orphan subagent/fork fake roots at `internal/db/filter_test.go:499` and `internal/db/filter_test.go:647`, while upstream promotes them.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `User explicitly requests orphan transcript visibility, or archive sampling proves hidden rows are user-visible data loss.`

### [primary] `546db04496cc286dfaf73f8a423547b7a37794cf` — `Fix nested fenced code block parsing (#722)`
- **Category:** `ui`
- **Decision:** `adopt`
- **Evidence:** Phase 05 landed fence-length-aware parsing: `3ebf2fad80ba03d7554da587eacadba1072cf7e4`. Local anchor `frontend/src/lib/utils/content-parser.ts:1` and phase 05 tests cover nested shorter fences.
- **Port cost:** `conflict`
- **Delivery:** `landed:3ebf2fad80ba03d7554da587eacadba1072cf7e4/phase-05`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `none`

### [primary] `1fc423a749c2dc7bf6a4a34f7a7996181bb45e31` — `fix(codex): reparse late token counts (#716)`
- **Category:** `sync-correctness`
- **Decision:** `defer`
- **Evidence:** Defer sync/backend correctness change: `git show --name-status 1fc423a749c2dc7bf6a4a34f7a7996181bb45e31` touches `internal/parser/codex.go`, `internal/parser/codex_parser_test.go`, `internal/sync/engine.go`, `internal/sync/engine_integration_test.go`; local counterpart/equivalent surface starts at `internal/parser/codex.go:20`. Needs backend parity or sync regression tests before adoption.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `A local sync, watcher, PG/DuckDB, daemon, or remote-sync regression maps to this upstream lineage.`

### [primary] `1ab17fedb47203618f8ca9168217dc13a4825ced` — `fix(parser): extract OpenClaw camelCase toolCall/toolResult blocks (#715)`
- **Category:** `active-agent`
- **Decision:** `defer`
- **Evidence:** Defer active-agent parser behavior: `git show --name-status 1ab17fedb47203618f8ca9168217dc13a4825ced` touches `internal/parser/content.go`, `internal/parser/openclaw.go`, `internal/parser/openclaw_test.go`; local counterpart/equivalent parser surface starts at `internal/parser/content.go:1`. Needs agent-specific fixtures before adoption.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `A fixture or real sample for this agent demonstrates the upstream behavior gap in the fork.`

### [primary] `42d0584df69df26e7718a58991d05aaf5625efff` — `fix: base skill analytics on message timestamps (#688)`
- **Category:** `sync-correctness`
- **Decision:** `defer`
- **Evidence:** Defer sync/backend correctness change: `git show --name-status 42d0584df69df26e7718a58991d05aaf5625efff` touches `internal/db/analytics.go`, `internal/db/analytics_test.go`, `internal/duckdb/analytics_usage.go`, `internal/duckdb/store_test.go`, `internal/duckdb/sync_test.go`, ... (8 paths total); local counterpart/equivalent surface starts at `internal/db/analytics.go:1109`. Needs backend parity or sync regression tests before adoption.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `A local sync, watcher, PG/DuckDB, daemon, or remote-sync regression maps to this upstream lineage.`

### [primary] `d8b3a04920396eb57d37e53dfe04add9765c7a2f` — `Fix refresh toolbar affordance (#711)`
- **Category:** `ui`
- **Decision:** `defer`
- **Evidence:** Defer UI behavior change: `git show --name-status d8b3a04920396eb57d37e53dfe04add9765c7a2f` touches `frontend/src/lib/components/analytics/AnalyticsPage.svelte`, `frontend/src/lib/components/analytics/AnalyticsPage.test.ts`, `frontend/src/lib/components/shared/DateRangeSelector.svelte`, `frontend/src/lib/components/shared/dateRangeSelector.test.ts`, `frontend/src/lib/components/usage/UsagePage.svelte`, ... (8 paths total); local counterpart/equivalent surface starts at `frontend/src/lib/components/analytics/AnalyticsPage.svelte:22`. Needs frontend tests or visual review before adoption.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `User-facing navigation/filter/rendering issue is reproduced or requested in this fork UI.`

### [primary] `ce29164540394c04f0788da75f37a1fb9ab13759` — `fix(db): classify automation from transcripts (#712)`
- **Category:** `sync-correctness`
- **Decision:** `defer`
- **Evidence:** Defer sync/backend correctness change: `git show --name-status ce29164540394c04f0788da75f37a1fb9ab13759` touches `internal/db/automated.go`, `internal/db/automated_backfill_test.go`, `internal/db/db.go`, `internal/db/db_test.go`, `internal/db/messages.go`, ... (10 paths total); local counterpart/equivalent surface starts at `internal/db/automated.go:12`. Needs backend parity or sync regression tests before adoption.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `A local sync, watcher, PG/DuckDB, daemon, or remote-sync regression maps to this upstream lineage.`

### [primary] `d303252eca8f4c227e0c091ac0cbafd42d905d9b` — `fix(parser): import Codex renamed session titles (#702)`
- **Category:** `sync-correctness`
- **Decision:** `defer`
- **Evidence:** Defer sync/backend correctness change: `git show --name-status d303252eca8f4c227e0c091ac0cbafd42d905d9b` touches `cmd/agentsview/main.go`, `internal/db/db.go`, `internal/db/db_test.go`, `internal/parser/codex.go`, `internal/parser/codex_parser_test.go`, ... (17 paths total); local counterpart/equivalent surface starts at `cmd/agentsview/main.go:28`. Needs backend parity or sync regression tests before adoption.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `A local sync, watcher, PG/DuckDB, daemon, or remote-sync regression maps to this upstream lineage.`

### [primary] `a553c5d9bb9f7e0132c6a45722001b1017b0ba5e` — `fix(postgres): preserve source machine on pg push (#701)`
- **Category:** `sync-correctness`
- **Decision:** `defer`
- **Evidence:** Defer sync/backend correctness change: `git show --name-status a553c5d9bb9f7e0132c6a45722001b1017b0ba5e` touches `internal/postgres/push.go`, `internal/postgres/push_pgtest_test.go`, `internal/postgres/push_test.go`; local counterpart/equivalent surface starts at `internal/postgres/push.go:1`. Needs backend parity or sync regression tests before adoption.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `A local sync, watcher, PG/DuckDB, daemon, or remote-sync regression maps to this upstream lineage.`

### [primary] `7388b312a4e48b82471f3c2b44a7cb57195993c1` — `fix(db): tolerate NULL message timestamps in velocity analytics (#705)`
- **Category:** `archive-schema`
- **Decision:** `adopt`
- **Evidence:** Verified backlog gap: `internal/db/analytics.go:1991` selects raw nullable `timestamp` into `ts string` at `internal/db/analytics.go:2009`; only `model` has `COALESCE`.
- **Port cost:** `conflict`
- **Delivery:** `backlog`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `Backlog item accepted; schedule SQLite/PG/DuckDB analytics parity tests.`

### [primary] `7c6c2917f6fd03d2458855f0a139bd18e6721706` — `fix(deps): update dependency dompurify to v3.4.9 [security] (#698)`
- **Category:** `security`
- **Decision:** `adopt`
- **Evidence:** Superseded by phase 05 dompurify 3.4.13 landing `3ebf2fad80ba03d7554da587eacadba1072cf7e4`. Local anchor `frontend/package.json:1`; 3.4.13 is newer than 3.4.9.
- **Port cost:** `conflict`
- **Delivery:** `landed:3ebf2fad80ba03d7554da587eacadba1072cf7e4/phase-05`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `none`

### [primary] `1139b078e712574f2244f48e16c146e99d46d417` — `fix(parser): support new .kimi-code session layout and add .kimi_openclaw to OpenClaw defaults (#665)`
- **Category:** `active-agent`
- **Decision:** `adopt`
- **Evidence:** Verified split finding: local Kimi Code support exists at `internal/parser/types.go:318`, but OpenClaw defaults at `internal/parser/types.go:285` include only `.openclaw/agents`, not `.kimi_openclaw/agents`.
- **Port cost:** `conflict`
- **Delivery:** `backlog`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `Backlog item accepted; schedule OpenClaw/Kimi default discovery review.`

### [primary] `59a4d055a41bdcb9eb5a6eb0f36d8a01e6cef2c4` — `fix: support Claude companion session layout (#677)`
- **Category:** `sync-correctness`
- **Decision:** `defer`
- **Evidence:** Defer sync/backend correctness change: `git show --name-status 59a4d055a41bdcb9eb5a6eb0f36d8a01e6cef2c4` touches `internal/db/db.go`, `internal/db/db_test.go`, `internal/parser/claude.go`, `internal/parser/claude_parser_test.go`, `internal/sync/engine_integration_test.go`; local counterpart/equivalent surface starts at `internal/db/db.go:436`. Needs backend parity or sync regression tests before adoption.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `A local sync, watcher, PG/DuckDB, daemon, or remote-sync regression maps to this upstream lineage.`

### [primary] `6333ae157f768b3d79c4a19524f36a72acacbd66` — `fix: render Cursor ApplyPatch tool calls (#670)`
- **Category:** `ui`
- **Decision:** `defer`
- **Evidence:** Defer UI behavior change: `git show --name-status 6333ae157f768b3d79c4a19524f36a72acacbd66` touches `frontend/src/lib/components/content/ToolBlock.test.ts`, `frontend/src/lib/utils/content-parser.test.ts`, `frontend/src/lib/utils/content-parser.ts`, `frontend/src/lib/utils/tool-params.test.ts`, `frontend/src/lib/utils/tool-params.ts`, ... (16 paths total); local counterpart/equivalent surface starts at `frontend/src/lib/components/content/ToolBlock.test.ts:23`. Needs frontend tests or visual review before adoption.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `User-facing navigation/filter/rendering issue is reproduced or requested in this fork UI.`

### [primary] `ec671d9786823569f30c0aa754de1fc3e21810ac` — `fix(frontend): shim vitest localStorage on newer node (#666)`
- **Category:** `ui`
- **Decision:** `defer`
- **Evidence:** Defer platform/CLI/server behavior: `git show --name-status ec671d9786823569f30c0aa754de1fc3e21810ac` touches `.github/workflows/ci.yml`, `frontend/src/vitest-setup.test.ts`, `frontend/src/vitest-setup.ts`, `frontend/vite.config.ts`; local counterpart/equivalent surface starts at `.github/workflows/ci.yml:33`. Needs a scoped acceptance test before adoption.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `User-facing navigation/filter/rendering issue is reproduced or requested in this fork UI.`

### [primary] `85e8bb789b56afcfce5873555ec995ed2e2603db` — `fix(frontend): route command palette session picks (#669)`
- **Category:** `ui`
- **Decision:** `defer`
- **Evidence:** Defer UI behavior change: `git show --name-status 85e8bb789b56afcfce5873555ec995ed2e2603db` touches `frontend/src/lib/components/command-palette/CommandPalette.svelte`, `frontend/src/lib/components/command-palette/CommandPalette.test.ts`; local counterpart/equivalent surface starts at `frontend/src/lib/components/command-palette/CommandPalette.svelte:24`. Needs frontend tests or visual review before adoption.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `User-facing navigation/filter/rendering issue is reproduced or requested in this fork UI.`

### [primary] `0b2d3d105dbf10657ee7a48d69fe2fe476a02de2` — `fix(frontend): skip local-only calls in read-only mode (#667)`
- **Category:** `ui`
- **Decision:** `defer`
- **Evidence:** Defer UI behavior change: `git show --name-status 0b2d3d105dbf10657ee7a48d69fe2fe476a02de2` touches `frontend/src/lib/api/generated/models/SettingsResponse.ts`, `frontend/src/lib/components/settings/SettingsPage.svelte`, `frontend/src/lib/components/settings/SettingsPage.test.ts`, `frontend/src/lib/components/settings/WorktreeMappingSettings.svelte`, `frontend/src/lib/components/settings/WorktreeMappingSettings.test.ts`, ... (13 paths total); local counterpart/equivalent surface starts at `frontend/src/lib/api/generated/models/SettingsResponse.ts:1`. Needs frontend tests or visual review before adoption.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `User-facing navigation/filter/rendering issue is reproduced or requested in this fork UI.`

### [primary] `293342a2a474f70c25d42ef216d5581ca7688528` — `fix(codex): stop double counting history replayed into forked sessions (#657)`
- **Category:** `archive-schema`
- **Decision:** `defer`
- **Evidence:** Defer archive/schema/data behavior change: `git show --name-status 293342a2a474f70c25d42ef216d5581ca7688528` touches `internal/db/db.go`, `internal/db/db_test.go`, `internal/db/orphaned.go`, `internal/parser/codex.go`, `internal/parser/codex_parser_test.go`, ... (6 paths total); local counterpart/equivalent surface starts at `internal/db/db.go:61`. Needs data-preservation and backend-parity tests before adoption.
- **Port cost:** `conflict`
- **Delivery:** `none`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `A local archive, stats, usage, search, pricing, or schema bug maps to this upstream lineage.`

### [primary] `6d44431f87b52a99f38865fc52c440a8e4f1f0e6` — `fix(stats): include non-Claude agents in peak_context_tokens distribution (#653)`
- **Category:** `archive-schema`
- **Decision:** `adopt`
- **Evidence:** Phase 03 landed data-driven peak-context distribution: `0e2594ae44fe07acfb4d40585a321f72edb3584b`. Local anchor `internal/db/session_stats.go:1`; phase handoff records `claude_only=false` compatibility.
- **Port cost:** `conflict`
- **Delivery:** `landed:0e2594ae44fe07acfb4d40585a321f72edb3584b/phase-03`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `none`

## Companion Findings

### [companion] `6407da6cfb9edccae01f73f173055fb2ff11eb05` — `Reject archives from newer agentsview binaries (#808)`
- **Category:** `archive-schema`
- **Decision:** `adopt`
- **Evidence:** Verified companion gap: `internal/db/db.go:424` reads `user_version`, but `internal/db/db.go:433` rejects future versions only when schema repair is needed; existing test `internal/db/db_test.go:683` allows future versions to reopen.
- **Port cost:** `conflict`
- **Delivery:** `backlog`
- **Review after:** `2026-09-12`
- **Revisit trigger:** `Backlog item accepted; schedule schema-contract phase.`

## Provider facade
- **Decision:** `defer`.
- **Core migration chain:** `6c8407ec`, `736e782f`, `23ce56f8`, `a57f24fe`, `8a4ae8c5`, `9fd61a07`, `ebee2ccc`, `72352d0d`, `98ef093c`, `56e3d0f0`.
- **Evidence:** local frozen tree has no `internal/parser/*provider*.go`; the fork registry is still `internal/parser/types.go:73`, with Positron at `internal/parser/types.go:435`, fork-owned Droid at `internal/parser/types.go:138`, and fork-owned Kimi Code at `internal/parser/types.go:318`.
- **Revisit trigger:** re-evaluate when at least 10 post-facade correctness fixes require repeated hand translation, or when 3 security/archive/sync high-risk fixes are blocked by the old architecture.

## Recall primitives
- **Overall `internal/recall`:** `defer`; this fork has no `internal/recall` package and the manager/store/UI boundary overlaps the local memory/synthesize direction.
- **`extract/segment.go`:** candidate for an isolated spike only. Acceptance idea: run it against fork transcript fixtures and compare stable segment IDs and boundaries against existing markdown/content parsers. Revisit trigger: a future memory/synthesize task needs deterministic transcript segmentation and has acceptance fixtures.
- **`rank.go`:** not drop-in because it depends on recall `Entry`, `Query`, status, and evidence semantics. Acceptance idea: define a measurable retrieval gap, then port only the ranking primitive behind that contract. Revisit trigger: local memory retrieval has a measured ranking-quality issue not solved by existing search.
