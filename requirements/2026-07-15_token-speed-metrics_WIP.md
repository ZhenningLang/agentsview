# Token Speed Metrics Requirement

Related specification: `docs/specs/token-speed-metrics.md`

## Goal

Show approximate effective output speed from existing assistant-message token
and timestamp data, so users can compare an agent or model across time without
claiming to measure token decoding speed.

## Locked Decisions

- Metric eligibility, direct ordinal predecessor window, p50/p95 aggregation,
  and null thresholds follow spec section 0.
- SQLite, PostgreSQL, and DuckDB expose the same speed query capability.
- Deliver the Analytics Velocity extension, SessionVitals comparison, and a
  standalone Speed trend page.
- The implementation is read-only: no schema migration, parser change, or
  resync is part of this initiative.

## Open Decisions

- Output-length bucket UI remains backlog work.
- A time-of-day speed heatmap remains backlog work.

## Acceptance Criteria

- QA1-QA9 in `docs/specs/token-speed-metrics-qa.md` pass through `verify.sh`.
- Manual real-database checks QA10-QA12 are recorded before delivery.
