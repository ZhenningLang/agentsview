# Kimi Code Support Requirement

Related specification: `docs/specs/kimi-code-support.md`

## Goal

Add Kimi Code (`~/.kimi-code/sessions/`, wire protocol 1.4) as a
first-class agent so its sessions — including subagent sessions — are
discovered, parsed, searchable, and included in token/cost stats like
every other supported agent.

## Locked Decisions

- New agent `kimicode` ("Kimi Code"), `KIMI_CODE_DIR`,
  `kimi_code_dirs`, default `.kimi-code/sessions`, ID prefix
  `kimicode:`; legacy `kimi` parser untouched.
- Canonical session ID `kimicode:session_<uuid>` with subagent suffix
  `:<agent-id>` (spec 方案 A).
- Subagent sessions and context-card rendering of non-user origins are
  in scope (user-approved 2026-07-17).
- Resume command `kimi --session session_<uuid>` in scope
  (user-approved 2026-07-17; ID form verified via `kimi export`).
- Tokens consume `usage.record` events; `step.end.usage` is only a
  zero-`usage.record` fallback.

## Open Decisions

- None.

## Acceptance Criteria

- All checks in the spec-contract block of
  `docs/specs/kimi-code-support.md` pass, including the real-data
  acceptance run against `~/.kimi-code`.
- `make test`, `go fmt ./...`, and `go vet ./...` are clean.
