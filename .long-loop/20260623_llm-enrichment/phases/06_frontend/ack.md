# Phase 06 Review Ack

## Blocker Resolutions

- [fixed] opus:B1 Sidebar index now returns `llm_title` from SQLite,
  PostgreSQL, and DuckDB with the same SELECT/Scan column order. Regression
  coverage asserts sidebar index rows expose `LLMTitle` in SQLite/DuckDB
  contract tests and PostgreSQL pgtest coverage.
- [fixed] kilo:B1 LLM enrichment trigger availability now uses a shared
  `canTrigger = !remote && !readOnly && !running` derived state. The read-only
  UI shows an unavailable reason, disables the trigger button, and the click
  guard uses the same state. Regression coverage includes read-only mode.

## Should Resolutions

- [fixed] opus:renameSeed Removed unused `renameSeed()` helper. The rename
  safety assertion now mounts `SessionItem` and exercises the real double-click
  `startRename` path, confirming the rename input uses the original first
  message fallback rather than `llm_title` while LLM title display is enabled.

## Verification

- `bash .long-loop/20260623_llm-enrichment/phases/06_frontend/verify.sh` passed.
- The verifier ran frontend focused tests, backend SQLite/DuckDB/PostgreSQL
  package tests, `make test-postgres`, `npm run check`, and full frontend tests.
