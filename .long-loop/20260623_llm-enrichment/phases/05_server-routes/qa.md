# Phase 05 QA — Server routes

## 自动化验证

本轮真实执行证据：

- `CGO_ENABLED=1 go test -tags "fts5,kit_posthog_disabled" ./internal/server -run 'TestLLM' -count=1`
  passed with `ok   go.kenn.io/agentsview/internal/server 1.264s`.
- `CGO_ENABLED=1 go test -tags "fts5,kit_posthog_disabled" ./internal/db ./internal/duckdb -run 'TestGetEnrichmentStatus|TestStoreGetEnrichmentStatus|TestDuckDBStoreContract' -count=1`
  passed with `ok   go.kenn.io/agentsview/internal/db 0.788s` and
  `ok   go.kenn.io/agentsview/internal/duckdb 3.018s` in the focused run.
- `CGO_ENABLED=1 go test -tags "fts5,kit_posthog_disabled" ./internal/backendcontract ./internal/postgres ./internal/duckdb ./internal/server ./internal/db && go vet ./...`
  passed; `go vet ./...` completed with no output.
- `make test` passed; full output ended with `ok   go.kenn.io/agentsview/internal/web 4.203s` in the first full run and `ok   go.kenn.io/agentsview/internal/web 2.651s` when rerun via `verify.sh`.
- `bash .long-loop/20260623_llm-enrichment/phases/05_server-routes/verify.sh`
  passed. Evidence from the final review-fix run:
  `ok   go.kenn.io/agentsview/internal/server 1.074s`,
  `ok   go.kenn.io/agentsview/internal/db 0.646s`,
  `ok   go.kenn.io/agentsview/internal/duckdb 1.892s`,
  `ok   go.kenn.io/agentsview/internal/postgres 23.112s` for
  `make test-postgres`, and final `make test` ended with
  `ok   go.kenn.io/agentsview/internal/web 3.121s`.

- QA1 — Route registration and JSON contract.
  Command: `CGO_ENABLED=1 go test -tags "fts5,kit_posthog_disabled" ./internal/server -run 'TestLLMRoutes' -count=1`
  Expected: `/api/v1/llm/enrich`, `/api/v1/llm/enrich/status`, and
  `/api/v1/llm/balance` are registered and return JSON envelopes matching the
  phase contract.
  Review addendum: `TestLLMRoutesRejectRemoteRequests` verifies all three LLM
  routes are local-only and fail closed with 403 for remote clients.

- QA2 — Enrichment trigger rejects disabled or unconfigured LLM.
  Command: `CGO_ENABLED=1 go test -tags "fts5,kit_posthog_disabled" ./internal/server -run 'TestLLMEnrichRejectsDisabledOrUnconfigured' -count=1`
  Expected: disabled config or missing API key returns a clear non-2xx response
  and does not call the provider or write enrichment data.

- QA3 — Enrichment trigger rejects read-only mode.
  Command: `CGO_ENABLED=1 go test -tags "fts5,kit_posthog_disabled" ./internal/server -run 'TestLLMEnrichRejectsReadOnlyMode' -count=1`
  Expected: a server without a local SQLite writer returns read-only/not
  available and does not construct/run the enricher.

- QA4 — Enrichment trigger can run a bounded mocked batch.
  Command: `CGO_ENABLED=1 go test -tags "fts5,kit_posthog_disabled" ./internal/server -run 'TestLLMEnrichRunsMockedBatch' -count=1`
  Expected: with local writer, enabled config, and mocked chat endpoint, POST
  returns enriched/skipped/error counters and writes `llm_title`, `llm_summary`,
  `llm_keywords`, and `enrich_status='ok'` for eligible sessions.
  Review addendum: `TestLLMEnrichDefaultLimitIsBounded` verifies omitted or
  non-positive HTTP `limit` defaults to 25 candidates.

- QA5 — Status endpoint returns enrichment counts.
  Command: `CGO_ENABLED=1 go test -tags "fts5,kit_posthog_disabled" ./internal/server ./internal/db ./internal/duckdb -run 'Test.*EnrichmentStatus|TestLLMEnrichStatus' -count=1`
  Expected: status counts include total, enriched, pending/empty status,
  `skipped_too_short`, `no_content`, and `error`. SQLite and DuckDB read behavior
  matches. PostgreSQL integration coverage is included via pgtest
  `TestStoreGetEnrichmentStatus` and `make test-postgres` in `verify.sh`.

- QA6 — DeepSeek balance parser uses root-domain `/user/balance`.
  Command: `CGO_ENABLED=1 go test -tags "fts5,kit_posthog_disabled" ./internal/server -run 'TestLLMBalanceDeepSeek' -count=1`
  Expected: base URL such as `https://host.example/v1` calls
  `https://host.example/user/balance`, parses `balance_infos[0]`, and returns
  `{supported:true,currency,amount,available}` without exposing the API key.
  Review addendum: provider selection now uses the parsed host, not arbitrary
  `base_url` path substrings.

- QA7 — Unsupported or failed balance checks degrade silently.
  Command: `CGO_ENABLED=1 go test -tags "fts5,kit_posthog_disabled" ./internal/server -run 'TestLLMBalanceUnsupportedOrFailed' -count=1`
  Expected: disabled LLM, missing API key, unknown provider, provider 4xx/5xx,
  malformed JSON, and network errors return HTTP 200 with `{supported:false}`.
  Review addendum: `TestLLMBalanceIncludesUnavailableFalse` verifies supported
  responses preserve `available:false` in JSON.

- QA8 — Standard Go validation passes.
  Command: `go fmt ./... && go vet ./... && make test-postgres && make test`
  Expected: formatting makes no unexpected changes after coder edits; vet and the
  repository test suite pass under the project defaults.

## 人工验证

- QA9 — Real DeepSeek balance smoke check.
  **目的**：验证真实 DeepSeek API key and endpoint can drive the balance route
  described in `REQUIREMENT.md:291`.
  **操作**：
  1. Configure `[llm] enabled=true`, `base_url="https://api.deepseek.com"`,
     `model="deepseek-chat"`, and a real API key via config or
     `AGENTSVIEW_LLM_API_KEY`.
  2. Start the local server.
  3. Call `GET /api/v1/llm/balance` from localhost.
  **观察**：
  1. Response is either `{supported:true,currency:"CNY",amount:<value>,available:<bool>}`
     or `{supported:false}` if the provider/key is unavailable at test time.
  2. The response body and logs do not include the API key.

- QA10 — Real enrichment trigger smoke check.
  **目的**：验证 local-only enrichment trigger can run against the configured LLM
  without using PG/DuckDB as writers.
  **操作**：
  1. Use a local SQLite-backed server with `[llm] enabled=true` and a real or
     intentionally mocked-compatible chat endpoint.
  2. Ensure there is at least one eligible session with enough user messages.
  3. Call `POST /api/v1/llm/enrich` with body `{"limit":1}`.
  4. Call `GET /api/v1/llm/enrich/status`.
  **观察**：
  1. POST returns counters with `candidates` and one terminal outcome
     (`enriched`, `no_content`, or `errors`).
  2. Status counts change consistently with the outcome.
  3. Running the same POST against PG serve/read-only mode returns read-only/not
     available instead of writing.
