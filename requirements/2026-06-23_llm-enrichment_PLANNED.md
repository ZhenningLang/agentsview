# LLM Session Enrichment & Semantic Search Requirement

Status: PLANNED — spec written, awaiting user approval + 3 endpoint spikes
Full spec (SSOT): `docs/specs/llm-enrichment.md`
Origin: port of the mantis LLM summary feature
(`~/Projects/mantis/internal/summary/`) to agentsview's multi-backend
architecture.

## Goal

Improve session search accuracy by letting a configurable OpenAI-compatible
LLM enrich each session with a rewritten title, a one-line summary, and
keywords (plus an optional embedding), then feed those into search via a
text channel (keywords/title in the LIKE name branch) and a vector channel
(embedding + brute-force cosine). Surface an LLM balance indicator when an
API key is configured.

## Background

Search accuracy is limited by two facts in code:

- Titles fall back to `first_message`
  (`internal/db/search.go:180`) — often greeting/error/`<system-reminder>`
  noise that does not summarize the session.
- FTS5 uses `porter unicode61` (`internal/db/db.go:179`); porter stemming is
  English-only and unicode61 does not segment CJK, so topic/synonym recall
  is weak and there is no keyword/semantic dimension to search.

mantis solved this with an offline LLM pass producing
`{title, topics:[{summary, keywords}]}` folded into fuzzy search. This
requirement ports that idea onto agentsview's SQLite/PostgreSQL/DuckDB
backends with FTS + a vector channel.

## Locked Decisions

1. Title is a toggleable display option (original ⇄ LLM title), persisted;
   falls back to original when `llm_title` is empty. `display_name`
   priority semantics unchanged.
2. Trigger is incremental enrichment with four gates:
   floor (size) → delta watermark (coverage) → debounce (cost) →
   terminal top-up (completeness). A session may be enriched multiple
   times over its life, bounded and cost-controlled.
3. Search uses a text channel + a vector channel. Vectors use embeddings +
   Go brute-force cosine — zero new deps, parity across all three backends
   (no sqlite-vec / pgvector).
4. Balance display: shown only when an api key is configured;
   provider-aware; silently hidden when unsupported.
5. Default OFF (`[llm].enabled=false`); unconfigured = byte-identical legacy
   behavior, zero outbound calls, zero cost.
6. New query/embedding capabilities go on the `db.Store` interface
   (`internal/db/store.go:18`), enforced for all backends by
   `backendcontract`.
7. Enrichment is offline (SkillSyncer-style), never on the file-watcher hot
   path; does not touch messages/tool_calls/stats triggers or
   `NormalizeToolCategory`.
8. Sampling/noise filtering reuses the mantis strategy (first 3 + last 3 +
   sampled middle, 500-char truncation, noise filter).

## Open Decisions (block parts of the work)

- D1 [blocks P3]: who provides embeddings? Kimi/moonshot may not expose
  `/v1/embeddings` [unverified]. Recommended: decouple via `[llm.embed]`
  (own base_url/api_key/model, empty = reuse chat; empty model = semantic
  search disabled). P1/P2 do not depend on it.
- D2: how is the reasoning level passed? `reasoning_effort` is an OpenAI
  field; whether Kimi accepts it is unverified. Pass-through + graceful
  degradation on rejection.
- D3: text-search wiring — recommended to extend the existing `Search()`
  LIKE name branch to include `llm_*` columns (CJK-friendly, lowest parity
  cost) rather than a separate FTS table.

## Derisk Spikes (turn assumptions into facts before implementing)

These need the user's real endpoint + api key and cannot be run by the
agent alone:

- spike-1 (P3): does the embed endpoint exist, what dimension?
- spike-2 (P1): does chat accept `reasoning_effort` and return parseable
  JSON?
- spike-3 (P2 balance): moonshot balance endpoint path/shape?
- spike-4 (deferred): batch rate-limit behavior (mitigated by low default
  concurrency + backoff).

## Non-Goals

- No manual edit/review UI for enrichment output (display + re-trigger only).
- No sqlite-vec / pgvector / external vector store.
- No enrichment on the sync hot path or SSE.
- No change to `display_name`/`session_name`/`first_message` priority.
- No score fusion between vector and text search (v1 semantic is a separate
  mode).

## Acceptance Criteria (per phase)

- [ ] A1 (P1): keywords contain the query term, body does not — text search
      finds the session.
- [ ] A2 (P2): title toggle switches display; empty `llm_title` falls back.
- [ ] A3 (P1): msg delta ≥ threshold and idle ≥ debounce re-enriches;
      active sessions do not; below-floor sessions are skipped.
- [ ] A4 (P2): balance shows/hides correctly across configured /
      unconfigured / unsupported.
- [ ] A5 (P1): unconfigured `[llm]` — search results identical to pre-change
      (regression).
- [ ] A6 (P1): same query returns the same result set on SQLite / PG /
      DuckDB.

## Phases

1. P1 — text enrichment loop: config, `internal/llm` client, `internal/enrich`
   scheduler, schema + parity, text search channel, `enrich` CLI.
2. P2 — frontend + triggers + balance: `/llm/enrich`, `/llm/balance` routes,
   title toggle, balance chip, enrichment UI.
3. P3 — vector semantic search (after spike-1): `db.Store.SessionEmbeddings`,
   embedding step in enricher, cosine + `/search/semantic` + frontend mode.
