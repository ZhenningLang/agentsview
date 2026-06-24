# Phase 05 Review Ack

## Blocker Resolutions

- [fixed] A:B1 Added balance network-error coverage in `TestLLMBalanceUnsupportedOrFailed`; provider transport errors now return HTTP 200 `{supported:false}` and the test asserts the API key is not leaked.
- [fixed] A:B2 Added fail-closed local-only checks to `POST /api/v1/llm/enrich`, `GET /api/v1/llm/enrich/status`, and `GET /api/v1/llm/balance` using `isLocalhostContext(ctx)`; remote requests return 403 before provider calls or DB reads.
- [fixed] A:B3 Added bounded HTTP enrichment default `defaultHTTPEnrichLimit = 25`; `Limit <= 0` on `POST /api/v1/llm/enrich` now runs at most 25 candidates instead of all candidates.

## Should Resolutions

- [fixed] B:S1 Removed `omitempty` from balance `available` so supported responses keep `available:false` instead of serializing as `undefined` for the frontend.
- [fixed] B:S2 Replaced duplicated enrichment status accumulation in SQLite/PostgreSQL/DuckDB with exported `db.AccumulateEnrichmentStatus` helper.
- [fixed] B:S3 Added PostgreSQL pgtest coverage `TestStoreGetEnrichmentStatus` and included `make test-postgres` in the phase verifier.
- [fixed] A:S1 Changed provider detection from whole-URL substring matching to parsed host matching, so custom path segments do not accidentally select DeepSeek/Moonshot balance endpoints.

## Verification

- `bash .long-loop/20260623_llm-enrichment/phases/05_server-routes/verify.sh` passed.
- The verifier includes focused LLM route tests, SQLite/DuckDB status store tests, `make test-postgres`, `go fmt ./...`, `go vet ./...`, and `make test`.
