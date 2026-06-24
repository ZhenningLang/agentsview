#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../../.." && pwd)"
cd "$ROOT"

echo "QA1 vector encode/decode, cosine, top-K"
go test -tags "fts5,kit_posthog_disabled" ./internal/search ./internal/db -run 'Test.*(Embedding|Semantic|Cosine|Vector)' -count=1

echo "QA2 SessionEmbeddings store parity, DuckDB, backend contract, PostgreSQL"
go test -tags "fts5,kit_posthog_disabled" ./internal/db ./internal/duckdb ./internal/backendcontract -run 'Test.*(StoreContract|SessionEmbeddings|BackendContract)' -count=1
make test-postgres

echo "QA3 optional embedding write during enrichment"
go test -tags "fts5,kit_posthog_disabled" ./internal/enrich ./internal/db ./internal/llm -run 'Test.*(Embed|Embedding|Enrich)' -count=1

echo "QA4-QA5 semantic search disabled and enabled HTTP paths"
go test -tags "fts5,kit_posthog_disabled" ./internal/server ./internal/search -run 'Test.*Semantic' -count=1

echo "QA6 frontend semantic mode gating and search store"
npm --prefix frontend test -- --run src/lib/api/llm.test.ts src/lib/stores/search.test.ts src/lib/components/command-palette/CommandPalette.test.ts

echo "QA7 formatting, full Go regression, vet, frontend typecheck"
go fmt ./...
make test
make vet
npm --prefix frontend run check

echo "Manual QA reminder: QA8 DeepSeek-only UI hidden and QA9 real embed provider E2E are operator-controlled."
