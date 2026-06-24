#!/usr/bin/env bash
set -euo pipefail

cd "$(git rev-parse --show-toplevel)"

echo "QA1-QA7: focused LLM route contracts"
CGO_ENABLED=1 go test -tags "fts5,kit_posthog_disabled" ./internal/server -run 'TestLLM' -count=1

echo "QA5: enrichment status store reads"
CGO_ENABLED=1 go test -tags "fts5,kit_posthog_disabled" ./internal/db ./internal/duckdb -run 'TestGetEnrichmentStatus|TestStoreGetEnrichmentStatus|TestDuckDBStoreContract' -count=1

echo "QA5: PostgreSQL enrichment status integration"
make test-postgres

echo "QA8: formatting"
go fmt ./...

echo "QA8: vet"
go vet ./...

echo "QA8: full repository tests"
make test
