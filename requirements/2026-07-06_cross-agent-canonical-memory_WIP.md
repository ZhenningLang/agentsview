# Cross-Agent Canonical Memory

## Goal

Implement a raw-preserving canonical memory layer across `assist-mem`, `cc-native`, and legacy `cross-agent` memory sources.

## SSOT

- Long-run workspace: `/Users/zhenninglang/Projects/agentsview/.long-loop/20260706_cross-agent-canonical-memory-layer`
- Requirement input: `REQUIREMENT.md`
- Plan: `SPEC_OVERVIEW.md`, `fix_plan.md`, `qa.md`, and `phases/*/spec.md`

## Locked Decisions

- Raw source rows remain preserved and inspectable.
- Canonical memory must be a separate layer/source with provenance, not a destructive rewrite of raw source notes.
- Existing compact-memory retirement semantics are not acceptable for cross-source raw preservation unless replaced with a raw-preserving mode.
- Work proceeds on `lr/cross-agent-canonical-memory-layer`, not `main`.

## Acceptance Criteria

- `assist-mem`, `cc-native`, and `cross-agent` can participate in synthesis.
- Canonical rows record covered raw refs.
- Recall defaults can prefer canonical rows and suppress covered raw duplicates.
- Raw/source-specific queries still expose raw rows.
- UI/API expose enough canonical provenance to understand what was covered.
- `lzn-test/lzn-preview` fixture returns one current entrypoint/canonical fact while keeping security exception and environment fact separate.
- Rollback to raw-only behavior is tested.
