# Phase 07 Review Ack

## Blocker Resolutions

- [fixed] A:B1 semantic availability no longer requires `Embed.APIKey`; unauthenticated embed providers with `Embed.Model` plus resolved `Embed.BaseURL` are available, and provider auth errors remain delegated to the provider HTTP response. Added no-API-key status/search coverage.
- [fixed] B:B1 `SessionEmbeddings` behavior is now covered outside SQLite: `TestDuckDBStoreContract/session_embeddings` and pgtest `TestStoreSessionEmbeddings` assert project filtering, deleted-session exclusion, and vector round-trip parity.

## Should Resolutions

- [fixed] Non-finite embedding vectors are validated before writing; invalid vectors are skipped and text enrichment remains `ok` with `llm_embedding_dim=0`.
- [fixed] Embedding skip paths log a redacted message with error category only; logs do not include API keys, query text, or sampled content.
- [fixed] Semantic search and semantic status routes now use the same local-only boundary as LLM routes before any provider call.

## Nit Decisions

- [fixed] Removed duplicate `disabled(cfg)` call in semantic search.
- [deferred] Min score threshold test and scan-helper reuse remain optional cleanup; neither affects reviewed blockers or accepted shoulds.
