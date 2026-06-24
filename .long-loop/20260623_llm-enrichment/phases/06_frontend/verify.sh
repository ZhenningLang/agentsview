#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../../.." && pwd)"
FRONTEND="$ROOT/frontend"

run_frontend() {
  local label="$1"
  shift
  printf '\n[%s] %s\n' "$label" "$*"
  (cd "$FRONTEND" && "$@")
}

run_root() {
  local label="$1"
  shift
  printf '\n[%s] %s\n' "$label" "$*"
  (cd "$ROOT" && "$@")
}

# QA1: LLM API helpers match the Phase 05 frontend contract.
run_frontend "QA1" npm run test -- --run src/lib/api/llm.test.ts

# QA2: Title selection logic preserves original title semantics and only uses
# llm_title under the persisted toggle.
run_frontend "QA2" npm run test -- --run src/lib/utils/session-title.test.ts src/lib/stores/ui.test.ts src/lib/components/sidebar/SessionItem.test.ts

# QA3: Balance chip renders only when supported and amount is present.
run_frontend "QA3" npm run test -- --run src/lib/components/layout/AppHeader.test.ts

# QA4: Enrichment settings UI loads status, triggers a run, refreshes counts,
# and surfaces backend errors.
run_frontend "QA4" npm run test -- --run src/lib/components/settings/LLMEnrichmentSettings.test.ts

# QA4b: Sidebar index includes llm_title across local read backends.
run_root "QA4b" env CGO_ENABLED=1 go test -tags "fts5,kit_posthog_disabled" ./internal/db ./internal/duckdb ./internal/postgres

if command -v docker >/dev/null 2>&1 && docker info >/dev/null 2>&1; then
  run_root "QA4c" make test-postgres
else
  printf '\n[QA4c] skip make test-postgres: docker is not available\n'
fi

# QA5: Svelte and TypeScript validation pass for the frontend.
run_frontend "QA5" npm run check

# QA6: Frontend regression tests pass for changed and adjacent behavior.
run_frontend "QA6" npm run test
