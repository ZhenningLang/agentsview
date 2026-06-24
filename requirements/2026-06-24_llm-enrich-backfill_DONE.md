# LLM enrichment: background backfill + working periodic loop

Branch `lr/llm-enrichment` (continues the LLM enrichment delivery). Follow-on
to `2026-06-24_llm-config-ui_DONE.md`.

## Problem

The "Run enrichment" button processes a fixed batch of 25 sessions
synchronously per click (`huma_routes_llm.go` `defaultHTTPEnrichLimit`). With
thousands of pending sessions a user must click ~N/25 times; there is no way to
backfill the whole archive from the UI. Separately, the `Run periodically`
config toggle is persisted and round-tripped through the API/TOML but **never
consumed** -- no background loop reads it, so checking it does nothing.

## Goal

1. A background enrichment job that processes all pending candidates with a
   live progress bar and a stop control. Starting it returns immediately; the
   job survives the request and keeps running if the page is closed.
2. Make the `Run periodically` toggle actually drive a background ticker so
   steady-state maintenance works without manual clicks.

## Locked decisions

- One pass, not a re-querying loop: the job runs a single `enrich.Run(Limit=0)`
  so every candidate is processed exactly once. This avoids the death-loop /
  wasted-retry where `error` and `no_content` sessions stay candidates
  (`enrichment.go:77`) and would be re-selected every batch.
- Progress via a new `enrich.Options.OnProgress(done, total)` callback invoked
  per candidate under the existing worker-pool mutex. The server job manager
  translates it into job state polled by the frontend.
- Single job at a time (manual or periodic share one runner). Periodic ticks
  no-op while a job runs.
- Job lifecycle is tied to the server's `baseCtx` (cancelled on shutdown);
  stop cancels the job context.
- Periodic ticker is started in `ListenAndServe`, reads `s.cfg.LLM` under
  `s.mu` each tick so the toggle hot-reloads; gated on
  `Enabled && Periodic && llmWriter != nil` plus key/model present.
- New endpoints are local-only fail-closed, consistent with existing LLM
  routes (`requireLocalWritableLLMRequest`).

## Acceptance

- POST `/api/v1/llm/enrich/start` launches a background job, returns running
  state; a second start while running is a no-op returning the live state.
- GET `/api/v1/llm/enrich/job` returns `{running, processed, succeeded,
  failed, target, ...}`; progress advances during a run.
- POST `/api/v1/llm/enrich/stop` cancels promptly.
- Frontend shows a progress bar + Stop while running, polls job state, and
  refreshes counts on completion.
- Toggling `Run periodically` on causes the ticker to start a job on the next
  tick without a restart; off makes ticks no-op.
- The job reports per-run cost: provider-reported token usage (chat
  prompt/completion + embed, captured from the previously-discarded `usage`
  object) and a chat-provider balance delta for providers with a balance
  endpoint (DeepSeek). The UI shows tokens and chat spend on completion.
  Embedding via a separate provider (OpenRouter, USD) is not in the balance
  delta but its tokens are shown.
- Backend table-driven tests (testify) for the job manager, progress, periodic
  gating, and token/cost reporting; frontend vitest for start/stop/progress and
  cost display. `go fmt`, `go vet`, build clean.

## Not doing

- No three-backend parity (job runs against the local writable `*db.DB`;
  PG/DuckDB read-only paths are blocked by local-only + `llmWriter == nil`).
- No persistent job history across restarts (in-memory state only).
