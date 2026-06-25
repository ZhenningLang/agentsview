# Kilo + Droid agent support (ported from platform-upgrade)

Branch `feat/kilo-droid-support`, off current `main`. Selective integration of
the still-relevant parts of the stale `lr2` platform-upgrade branch (last
commit 2026-06-12), which could not be merged as-is (11 semantic conflicts vs
the since-merged LLM-enrichment work).

Related: `2026-06-12_agentsview-platform-upgrade_PLANNED.md` (the broader
original initiative; partially delivered here, partially dropped/deferred).

## Delivered (cherry-picked + conflict-resolved)

- **Kilo** first-class agent: opencode-family SQLite ingestion, `kilo:` ID
  prefix, `agent="kilo"` identity, discovery from `~/.local/share/kilo/` with
  `KILO_DIR` / `kilo_dirs` overrides (config carried by the AgentDef, not the
  dropped generated-name config). (`7652d8e`)
- **Droid** first-class agent: JSONL parser + sibling `<id>.settings.json`
  usage ingestion, `~/.factory/sessions/` with `DROID_SESSIONS_DIR` /
  `droid_sessions_dirs`. (`906e2db`)
- **Browser-safe resume commands** for Kilo (POSIX single-quote escaping,
  copy-only, never executed). (`2179a5e`)
- **System / context events**: persisted system/context messages (resume,
  interruption, command messages, stop-hook feedback, Droid `session_start`)
  render as compact context cards; treated as turn boundaries in transcript
  mode. (`906e2db`)

## Dropped (per user decision: keep llm_title, not a second title system)

- `582ce4c` "generated name support" — the on-demand `generated_name` field +
  `/generate-name` endpoint + `[session_name_model]` config + `session_name.go`
  service. It competes with the already-merged `llm_title` title-override for
  the same display slot; user chose option (a): keep only `llm_title`. Also had
  a UTF-8 byte-truncation bug. Not ported.

## Deferred (backlog — semantic collision with merged llm_* search)

- `fe48cb7` "verify kilo search identity" — adds an `agent` search-filter query
  param. Conflicts with main's llm_* search args/SQL across SQLite/PG/DuckDB;
  high-risk hand-merge for a non-essential enhancement (kilo sessions are
  already browsable as `agent="kilo"`). Redo cleanly against current main later.
- `3f6db46` "improve precise search signals" — general search-ranking change,
  same llm_* search collision. Defer.

## Boundary decisions

- Title display: only `llm_title` (background enrichment) drives generated
  titles; the on-demand `generated_name` path was dropped, so there is exactly
  one title-override mechanism. (user-approved)
- Search: kilo/droid sessions are indexed and filterable via the existing
  `agent` identity; the lr2 search refinements are NOT included. (deferred,
  evidence: 11-file conflict with llm_* search)

## Acceptance

- `make test` (full Go suite, SQLite+DuckDB), frontend vitest, svelte-check,
  `go vet`, `gofmt` all clean on current main.
- Kilo + Droid registered in the parser registry; no `generated_name` /
  `session_name_model` / `/generate-name` remnants in the tree.
