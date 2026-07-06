# Canonical Memory Runbook

## Purpose

This runbook covers local operation and rollback for the raw-preserving canonical
memory layer. It is intentionally separate from automated verification because
packaging, install, restart, and local persistent DB changes are runtime side
effects that require explicit user approval.

## Safe Defaults

- Raw memory rows stay in their source buckets: `assist-mem`, `cc-native`, and
  `cross-agent`.
- Generated current-memory rows use source `canonical`.
- Default recall remains raw-compatible unless canonical preference is requested
  explicitly through config/API/UI controls.
- Automated acceptance must not run `make install`, restart local services, push,
  deploy, or mutate a persistent local production DB.

## Enable Canonical Preference

1. Validate the committed branch with the long-run acceptance verifier.
2. Start AgentsView from the validated binary or approved local install.
3. Use the memory UI/API canonical controls explicitly:
   `/api/v1/memory/recall` with `prefer_canonical=true`, or the UI canonical
   source/preference control.
4. Confirm canonical recall hits include `canonical_covered_refs` and
   `canonical_provenance` metadata.
5. Confirm raw source filters still return raw rows for `assist-mem`,
   `cc-native`, and `cross-agent`.

## Disable Canonical Preference

1. Stop sending `prefer_canonical=true` in recall requests, or turn off the UI
   canonical preference control.
2. Use explicit raw source filters when auditing source notes.
3. Leave `source='canonical'` rows in place if only preference rollback is
   needed; raw-compatible default behavior excludes canonical rows unless they
   are requested.

## Canonical-Only Cleanup

Use this only against an isolated test DB or after backing up the local
persistent DB.

1. Stop the local AgentsView process or make a WAL-safe backup first.
2. Verify the target DB path before running cleanup.
3. Delete or replace only `source='canonical'` rows. Do not delete raw
   `assist-mem`, `cc-native`, or `cross-agent` rows.
4. Restart or resync if needed.
5. Verify raw-only behavior with memory list filters and recall without
   canonical preference.

## Approval-Gated Local Rollout

Local app packaging/restart is not part of automated acceptance. Execute it only
after explicit user approval, committed code, and passing acceptance.

1. Record the approved branch/commit.
2. Back up the local AgentsView data directory or DB.
3. Build/package using the repository Makefile command approved for the local
   workflow, for example `make build-release`, `make install`, or a desktop
   bundle target.
4. Restart the local AgentsView process through the approved local service or
   app workflow.
5. Verify `/api/v1/version` responds from the newly started app.
6. Verify memory list with `source=canonical` and each raw source filter.
7. Verify recall with `prefer_canonical=true`, then verify raw bypass with an
   explicit source filter.

## Live PostgreSQL Parity

Default Phase 06 acceptance runs non-`pgtest` PostgreSQL unit tests for canonical
metadata scanning and schema DDL. Live PostgreSQL integration remains a stronger
optional verifier through `make test-postgres`, which requires Docker or a real
PostgreSQL instance and is not run by the default acceptance script.

## Rollback

1. Disable canonical preference first. This returns recall to raw-compatible
   behavior without deleting generated canonical rows.
2. If generated rows must be removed, clear only `source='canonical'` rows after
   backup or in an isolated DB.
3. Reinstall or run the prior AgentsView binary if the local app rollout caused
   a runtime regression.
4. Re-run raw source filters to confirm raw `assist-mem`, `cc-native`, and
   `cross-agent` rows remain inspectable.
