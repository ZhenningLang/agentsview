# LLM Config UI + Connection Test Requirement

Status: PLANNED — extends the LLM enrichment delivery (branch lr/llm-enrichment)
Related: requirements/2026-06-23_llm-enrichment_PLANNED.md

## Goal

Let users configure LLM enrichment (chat + embed: base_url/api_key/model/
reasoning + scheduling) from the web Settings UI instead of hand-editing TOML,
and add a "Test connection" button that probes both chat and embed endpoints
so users immediately know whether the config works.

## Background

The enrichment feature ships config as TOML/env only (no in-app editor) and has
no connection test. The only implicit signals (balance chip, enrichment status)
cover chat for known providers only; embed misconfig surfaces just as
"unavailable" with no error detail. agentsview already persists UI-edited config
via `writeConfigMap` (0600) + `SaveTerminalConfig` + `POST /api/v1/config/terminal`,
and LLM handlers read `s.cfg.ResolveLLM()` per-request under `s.mu`, so a saved
config hot-applies without restart.

## Locked Decisions

1. Three new endpoints, all local-only fail-closed (consistent with /llm/enrich):
   - GET /api/v1/config/llm — current config, api_key/embed.api_key MASKED.
   - POST /api/v1/config/llm — persist + hot-apply under s.mu; submitting the
     mask sentinel keeps the existing key (no wipe).
   - POST /api/v1/llm/test — minimal real chat + embed probe, per-channel result.
2. api_key never returned in plaintext; TOML written 0600 (existing helper).
3. Persist via writeConfigMap pattern (merge [llm]/[llm.embed] section).
4. Frontend: config form in Settings (enabled, chat base_url/api_key/model/
   reasoning_effort, embed base_url/api_key/model, scheduling) + Save + Test.
5. Lands on branch lr/llm-enrichment (same PR as enrichment).

## Acceptance Criteria

- [ ] Configure chat+embed entirely from Settings UI; Save persists to TOML 0600
      and takes effect without restart.
- [ ] Test button reports chat ok/err and embed ok/disabled/err with messages.
- [ ] GET never leaks api_key; re-saving without retyping key preserves it.
- [ ] All three endpoints reject remote (local-only).
- [ ] make test + frontend check/test green; real browser screenshot of the form
      + a real Test-connection run against DeepSeek.
