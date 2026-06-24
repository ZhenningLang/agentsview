# Handoff

(phase coder 每轮交接: 做了什么 / 下一步 / 验证证据)

## Phase 01 config-and-llm-client

做了什么:

- Added `LLMConfig` and `LLMEmbedConfig` to `internal/config`, including default OFF behavior, DeepSeek-oriented defaults, env-wins loading, TOML loading, secret-safe JSON serialization, and `ResolveLLM()` embed fallback.
- Added `internal/llm` with a stdlib OpenAI-compatible HTTP client for chat JSON and embeddings.
- Added `ParseEnrichment` for normal JSON, fenced JSON, noisy text, missing summary, and keyword array/string normalization.
- Added unit tests for config defaults/env merge/embed fallback/secret redaction, client chat retries/reasoning fallback, embedding sentinel errors, and enrichment parsing.
- Wrote phase verifier at `.long-loop/20260623_llm-enrichment/phases/01_config-and-llm-client/verify.sh`.

下一步:

- Orchestrator should launch reviewers for Phase 01 against the uncommitted diff.
- If review asks for changes, coder should update `ack.md`, fix accepted findings, rerun `verify.sh`, then commit only this phase's files.

验证证据:

- `.long-loop/20260623_llm-enrichment/phases/01_config-and-llm-client/verify.sh` passed.
- Covered QA1-QA8: config LLM tests, `internal/llm` tests, `go build ./...`, and `go vet ./...`.

Review 收口:

- Fixed A:B1 with a transport/network-error retry regression test.
- Fixed A:B2 with an empty embedding-vector rejection regression test.
- Fixed review B shoulds for verify.sh full-package fallback, `llm.New` embed fallback normalization, reasoning TOML merge consistency, dead `llmEnvPeriodicSet`, and configured API-key secret assertions.

## Phase 02 schema-and-parity

做了什么:

- Added 10 LLM enrichment columns to SQLite `sessions` DDL and idempotent non-destructive migrations without bumping `dataVersion`.
- Added LLM scalar fields plus `LLMEmbedding []byte` to `db.Session`; ordinary SQLite/PG/DuckDB list/detail reads expose lightweight LLM metadata and intentionally exclude the large `llm_embedding` payload.
- Added `sessionSyncCols` so PG/DuckDB push paths can read and propagate `llm_embedding` without putting it in ordinary API reads.
- Extended PostgreSQL `coreDDL`, ALTER migrations, schema compatibility probe, session scanner, and `pushSession` INSERT/UPDATE/change-detection paths with all 10 LLM columns.
- Extended DuckDB schema to version `4`, column repair list, scanner, mirror upsert, and session fingerprint fields with all 10 LLM columns.
- Added SQLite schema/migration/read tests, DuckDB mirror insert/update tests, and pgtest coverage for PG LLM push propagation.
- Wrote phase verifier at `.long-loop/20260623_llm-enrichment/phases/02_schema-and-parity/verify.sh`.

下一步:

- Orchestrator should launch reviewers for Phase 02 against the uncommitted diff.
- If review asks for changes, coder should write `ack.md`, fix accepted findings, rerun `verify.sh`, then commit only Phase 02 files.
- PG integration coverage remains environment-gated: run `make test-postgres` or set `TEST_PG_URL` to execute `TestPushPropagatesLLMFieldsOnInsertAndUpdate` against a real PostgreSQL instance.

验证证据:

- `.long-loop/20260623_llm-enrichment/phases/02_schema-and-parity/verify.sh` passed through formatting, build, vet, and full `make test`.
- Focused evidence:
  - `ok  	go.kenn.io/agentsview/internal/db	0.652s`
  - `ok  	go.kenn.io/agentsview/internal/postgres	0.342s`
  - `ok  	go.kenn.io/agentsview/internal/duckdb	1.959s`
- Full-suite evidence: verifier output ended with `PASS` and `ok  	go.kenn.io/agentsview/internal/web	2.284s`.
- QA6 caveat: verifier printed `WARN: PG round-trip UNVERIFIED, run make test-postgres or set TEST_PG_URL`; pgtest was added but not executed locally due missing real PG URL.

## Phase 03 enricher-and-cli

做了什么:

- Added SQLite-only enrichment writer/query methods on `*db.DB`: candidate selection, too-short marking, and text enrichment/status writeback. These methods intentionally do not extend `db.Store`, PG, or DuckDB writers.
- Added `internal/enrich` with message sampling, prompt construction, bounded-concurrency runner, disabled zero-outbound short-circuit, no-content handling, failure isolation, secret-redacted errors, and LLM JSON parsing via `internal/llm`.
- Added `agentsview enrich [--all] [--project P] [--force] [--limit N]` CLI with disabled nonzero behavior and cost guard validation for negative limit / `--all` plus `--limit`.
- Added table-driven DB, enricher, prompt, sampling, and CLI tests covering QA1-QA10.
- Wrote phase verifier at `.long-loop/20260623_llm-enrichment/phases/03_enricher-and-cli/verify.sh`.

下一步:

- Orchestrator should launch reviewers for Phase 03 against the uncommitted diff.
- If review asks for changes, coder should write `ack.md`, fix accepted findings, rerun `verify.sh`, then commit only Phase 03 files.
- Manual QA11 remains for a user-controlled DeepSeek API key and real SQLite DB copy; do not run it against the production DB directly.

验证证据:

- `.long-loop/20260623_llm-enrichment/phases/03_enricher-and-cli/verify.sh` passed.
- Focused evidence:
  - `ok   go.kenn.io/agentsview/internal/enrich 0.487s` for disabled zero-outbound.
  - `ok   go.kenn.io/agentsview/internal/db 0.411s` for candidate gate matrix.
  - `ok   go.kenn.io/agentsview/cmd/agentsview 1.157s` for CLI registration/guards/disabled.
- Full-suite evidence: `CGO_ENABLED=1 go test -tags "fts5,kit_posthog_disabled" ./...` passed through `ok   go.kenn.io/agentsview/internal/web 3.867s`; `go vet ./...` completed with no output.

## Phase 04 text-search-channel

做了什么:

- Extended SQLite global `Search()` name branch to include `llm_title`, `llm_keywords`, and `llm_summary` in LIKE matching while preserving `Name` fallback and message-branch de-duplication.
- Extended PostgreSQL and DuckDB global search name branches with the same LLM metadata fields and matching-field snippets.
- Added SQLite tests for LLM keyword/title/summary matches, system-only exclusion, and message-vs-metadata de-duplication.
- Added PostgreSQL `pgtest` coverage for LLM metadata search parity and DuckDB sync-backed tests for searchable `llm_*` propagation.
- Wrote phase verifier at `.long-loop/20260623_llm-enrichment/phases/04_text-search-channel/verify.sh`.

下一步:

- Orchestrator should launch reviewers for Phase 04 against the uncommitted diff.
- If review asks for changes, coder should write `ack.md`, fix accepted findings, rerun `verify.sh`, then commit only Phase 04 files.

验证证据:

- RED: `CGO_ENABLED=1 go test -tags "fts5,kit_posthog_disabled" ./internal/db -run 'TestSearch/llm'` failed before implementation because LLM metadata searches returned 0 results.
- Focused SQLite GREEN: `ok   go.kenn.io/agentsview/internal/db 1.511s` for `TestSearch/llm`.
- SQLite regression: `ok   go.kenn.io/agentsview/internal/db 0.861s` for `TestSearch|TestSearchEmptyQueryGuard`.
- DuckDB focused/parity: `ok   go.kenn.io/agentsview/internal/duckdb 2.021s` for `TestSearchGroupsMessagesAndIncludesNameMatches|TestDuckDBStoreContract|Test.*LLM`.
- PostgreSQL integration: `make test-postgres` passed with `ok   go.kenn.io/agentsview/internal/postgres 21.108s`.
- `go vet ./...` completed with no output.
- `make test` passed with output ending at `ok   go.kenn.io/agentsview/internal/web 3.067s`.
- Final phase verifier: `bash .long-loop/20260623_llm-enrichment/phases/04_text-search-channel/verify.sh` passed with output ending at `ok   go.kenn.io/agentsview/internal/web 1.691s`.

## Phase 05 server-routes

做了什么:

- Added typed Huma LLM routes under `/api/v1/llm`: `POST /enrich`,
  `GET /enrich/status`, and `GET /balance`.
- Wired a local-only SQLite writer handle into `Server`; enrichment trigger returns
  not-available in PG/read-only mode and does not construct the enricher there.
- Added `db.Store.GetEnrichmentStatus(ctx)` with SQLite, PostgreSQL, and DuckDB
  implementations so the status endpoint remains backend-visible and parity-checked.
- Implemented provider-aware balance lookup with DeepSeek root-domain
  `/user/balance`, best-effort Moonshot/custom `balance_url`, and safe
  `{supported:false}` degradation on disabled config, missing key, unsupported
  provider, provider error, malformed JSON, or network failure.
- Added route tests for registration/envelopes, disabled/missing-key rejection,
  read-only rejection, mocked enrichment run, status counts, DeepSeek balance
  parsing, unsupported/failure balance degradation, local-only route rejection,
  bounded default HTTP enrichment, and secret non-exposure.
- Review fixes made all three LLM routes local-only, set HTTP enrichment default
  `Limit <= 0` to 25 candidates, preserved `available:false` in balance JSON,
  moved status bucket accumulation to `db.AccumulateEnrichmentStatus`, switched
  provider detection to parsed host matching, and added PostgreSQL pgtest status
  coverage.
- Wrote phase verifier at
  `.long-loop/20260623_llm-enrichment/phases/05_server-routes/verify.sh`.

下一步:

- Phase 05 review blockers are fixed and acknowledged in
  `.long-loop/20260623_llm-enrichment/phases/05_server-routes/ack.md`.
- Next phase can consume the `/api/v1/llm/*` server contract; manual QA9/QA10
  still require an operator-controlled API key/local server smoke test.
- Manual QA9/QA10 remain operator-controlled because they require a real DeepSeek
  key or a real local server smoke test.

验证证据:

- Review-fix focused server route tests:
  `ok   go.kenn.io/agentsview/internal/server 1.074s`.
- Review-fix status store tests: `ok   go.kenn.io/agentsview/internal/db 0.646s`,
  `ok   go.kenn.io/agentsview/internal/duckdb 1.892s`, and pgtest
  `ok   go.kenn.io/agentsview/internal/postgres 23.112s` via
  `make test-postgres`.
- Related package + contract run passed:
  `CGO_ENABLED=1 go test -tags "fts5,kit_posthog_disabled" ./internal/backendcontract ./internal/postgres ./internal/duckdb ./internal/server ./internal/db`.
- `go vet ./...` completed with no output.
- `make test` passed with final package `ok   go.kenn.io/agentsview/internal/web 3.121s`.
- Final phase verifier: `bash .long-loop/20260623_llm-enrichment/phases/05_server-routes/verify.sh`
  passed, including `make test-postgres`, `go fmt ./...`, `go vet ./...`, and
  `make test`.

Boundary decisions:

- schema-contract: LLM server routes are exposed as `/api/v1/llm/*`, matching
  existing typed Huma route conventions rather than bare `/llm/*` shorthand.
- operational-side-effect: `POST /api/v1/llm/enrich` runs synchronously and only
  when a local SQLite writer is available; PG/read-only mode returns not available.
- limit-default-fallback: HTTP enrichment defaults `Limit <= 0` to 25 candidates
  so an empty POST body cannot trigger unbounded synchronous provider calls.
- context-surface: all `/api/v1/llm/*` routes are local-only and return 403 for
  remote clients before DB/provider work.
- context-surface: balance provider failures return HTTP 200 `{supported:false}`
  so frontend can hide the balance chip without exposing provider errors or API keys.

## Phase 06 frontend

做了什么:

- Added typed frontend LLM API helpers for balance, enrichment trigger, and enrichment status under `frontend/src/lib/api/llm.ts`.
- Added `llm_title` to frontend session/index types and preserved it through sidebar index hydration and detail merges.
- Added a pure session-title helper and wired the persisted `ui.useLlmTitle` preference into sidebar row titles and the detail breadcrumb without changing rename seed semantics.
- Added an Appearance settings toggle for LLM titles, a conditional header balance chip, and a Settings LLM enrichment section with status counts, trigger action, remote-mode gating, and visible backend errors.
- Added frontend tests for LLM API contract, title fallback/toggle behavior, `ui.useLlmTitle` persistence, balance chip gating, and enrichment settings behavior.
- Added a Vitest setup storage polyfill and runtime storage guards so tests and runtime local/remote detection tolerate missing or partial `localStorage` implementations.
- Updated the icon barrel whitelist to include the already-exported `WrenchIcon` used by the header.
- Wrote phase verifier at `.long-loop/20260623_llm-enrichment/phases/06_frontend/verify.sh`.
- Review fixes: sidebar index now carries `llm_title` across SQLite,
  PostgreSQL, and DuckDB; LLM enrichment controls now disable the trigger for
  read-only backends through a shared `canTrigger` derived state; unused
  `renameSeed()` was removed and rename safety is covered through the real
  `SessionItem` rename path.

下一步:

- Phase 06 review blockers were fixed and acknowledged in
  `.long-loop/20260623_llm-enrichment/phases/06_frontend/ack.md`.
- Manual QA7-QA10 remain for a real browser/local server pass with sample enriched sessions, optional DeepSeek balance config, and desktop/mobile screenshots.

验证证据:

- `.long-loop/20260623_llm-enrichment/phases/06_frontend/verify.sh` passed after review fixes.
- Focused frontend evidence:
  - `src/lib/api/llm.test.ts`: `Test Files 1 passed`, `Tests 4 passed`.
  - `src/lib/utils/session-title.test.ts src/lib/stores/ui.test.ts src/lib/components/sidebar/SessionItem.test.ts`: `Test Files 3 passed`, `Tests 60 passed`.
  - `src/lib/components/layout/AppHeader.test.ts`: `Test Files 1 passed`, `Tests 6 passed`.
  - `src/lib/components/settings/LLMEnrichmentSettings.test.ts`: `Test Files 1 passed`, `Tests 5 passed`.
- Backend sidebar index parity evidence:
  - `CGO_ENABLED=1 go test -tags "fts5,kit_posthog_disabled" ./internal/db ./internal/duckdb ./internal/postgres`: `ok` for all three packages.
  - `make test-postgres`: `ok   go.kenn.io/agentsview/internal/postgres 21.759s`.
- `npm run check` passed with `svelte-check found 0 errors and 0 warnings`.
- Full frontend regression: `npm run test` passed with `Test Files 76 passed (76)` and `Tests 1278 passed (1278)`.

Boundary decisions:

- context-surface: frontend never accepts or stores LLM API keys; it only calls server-side `/api/v1/llm/*` routes.
- context-surface: balance chip and enrichment controls are gated in remote mode because Phase 05 routes reject non-local callers.
- limit-default-fallback: Settings trigger sends `{ limit: 25 }`, matching the Phase 05 bounded default and avoiding an unbounded synchronous UI action.
- schema-contract: `llm_title` is used only as optional display metadata; rename seeds remain based on `display_name` or original first-message fallback.
- schema-contract: sidebar index rows now include optional display metadata
  `llm_title` so list-level title switching works before detail hydration.
- operational-side-effect: read-only backends show an unavailable enrichment
  trigger instead of presenting a write action that can only fail after click.

## Phase 07 vector-semantic

做了什么:

- Added `db.EncodeEmbedding` / `db.DecodeEmbedding` for little-endian float32 vector bytes with dimension and non-finite validation.
- Extended `db.Store` with `SessionEmbeddings(ctx, EmbeddingFilter)` and implemented it for SQLite, PostgreSQL, and DuckDB.
- Added store contract coverage for embedding reads, project filtering, deleted-session exclusion, and vector decode consistency.
- Extended `db.EnrichmentWrite` and `internal/enrich` so configured embed providers can write `llm_embedding` / `llm_embedding_dim` after text enrichment succeeds.
- Kept text enrichment successful when embedding is unconfigured or unsupported; embedding failures do not mark `enrich_status=error`.
- Added `internal/search` semantic ranking with cosine similarity, top-K ordering, disabled config gating, and semantic result shape using `ordinal=-1`.
- Added `/api/v1/search/semantic/status` for zero-outbound availability gating and `/api/v1/search/semantic?q=&project=&k=` for enabled semantic search.
- Added frontend semantic search helper, command-palette status gating, and search store `keyword|semantic` mode; DeepSeek-only default keeps semantic mode hidden because `[llm.embed].model` is empty.
- Review fixes: unauthenticated embed providers are supported, semantic routes are local-only before provider calls, non-finite provider vectors are skipped without failing text enrichment, and DuckDB/PostgreSQL now have behavior-level `SessionEmbeddings` parity tests.
- Wrote and ran phase verifier at `.long-loop/20260623_llm-enrichment/phases/07_vector-semantic/verify.sh`.

下一步:

- Phase 07 review blockers are fixed and acknowledged in `.long-loop/20260623_llm-enrichment/phases/07_vector-semantic/ack.md`.
- Manual QA8/QA9 remain operator-controlled: DeepSeek-only browser smoke for hidden semantic mode, and optional real embedding-provider E2E.

验证证据:

- Final phase verifier passed: `bash .long-loop/20260623_llm-enrichment/phases/07_vector-semantic/verify.sh`.
- QA1 vector/search focused tests: `ok   go.kenn.io/agentsview/internal/search 0.509s`, `ok   go.kenn.io/agentsview/internal/db 0.895s`.
- QA2 store parity: SQLite/DuckDB/backendcontract focused tests passed; `make test-postgres` ran Docker-backed PG and ended with `ok   go.kenn.io/agentsview/internal/postgres 25.062s` inside `verify.sh`.
- QA3 enricher embedding: `ok` for `internal/enrich`, `internal/db`, and `internal/llm` focused tests.
- QA4/QA5 semantic route/service: `ok` for `internal/server` and `internal/search` semantic tests.
- QA6 frontend focused semantic tests: `Test Files 3 passed (3)`, `Tests 29 passed (29)`.
- QA7 full regression: `make test`, `make vet`, and `npm --prefix frontend run check` passed inside `verify.sh`; frontend all-tests also passed when rerun alone with `Test Files 76 passed (76)`, `Tests 1283 passed (1283)`.
- Note: a first full frontend test run executed concurrently with heavy Go/PG verification failed in unrelated existing events/usage/highlight tests; rerunning the same frontend command alone passed.

Boundary decisions:

- schema-contract: `SessionEmbeddings` exposes only session-level embedding metadata plus decoded vector through `db.Store`; no new DB columns were added in this phase.
- context-surface: semantic search is local-only and rejects remote clients before provider calls; when local, it sends the user query to the embed provider only when `llm.enabled`, `llm.embed.model`, and `llm.embed.base_url` are configured.
- context-surface: `llm.embed.api_key` is not a generic availability requirement so local unauthenticated providers such as Ollama-style endpoints can be used; provider-specific auth failures are delegated to the provider HTTP response.
- context-surface: `/api/v1/search/semantic/status` performs zero provider calls and gates frontend visibility so DeepSeek-only default does not show semantic mode.
- limit-default-fallback: semantic `k` is clamped with existing search limits; default is `db.DefaultSearchLimit`, max is `db.MaxSearchLimit`.
- schema-contract: semantic results are independent search-mode results with `ordinal=-1` and are not fused into text search ranking.
