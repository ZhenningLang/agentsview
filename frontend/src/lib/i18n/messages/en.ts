// English messages. Keys mirror zh.ts; en is also the fallback locale.
export const en: Record<string, string> = {
  // common
  "common.save": "Save",
  "common.saving": "Saving…",
  "common.add": "Add",
  "common.remove": "Remove",
  "common.enabled": "Enabled",
  "common.disabled": "Disabled",
  "common.loading": "Loading…",
  "common.localOnly": "This setting is available only from the local server connection.",
  "common.provider": "Provider",
  "common.test": "Test",
  "common.testing": "Testing…",

  // LLM enrichment (the single merged LLM config section)
  "enrich.title": "LLM Configuration",
  "enrich.desc": "One place for model sources (Providers) and usage assignment: configure providers, then pick one per usage.",
  "feature.enrichTitle": "Session Enrichment",
  "feature.enrichDesc": "Generate titles / summaries / keywords for local sessions. Model used: the “Session enrichment” usage assignment in LLM Configuration above.",
  "feature.enrichSave": "Save enrichment settings",
  "enrich.enable": "Enable LLM enrichment",
  "enrich.scheduling": "Scheduling",
  "enrich.minMsgs": "Min user messages",
  "enrich.reenrichDelta": "Re-enrich message delta",
  "enrich.idleMinutes": "Idle minutes",
  "enrich.concurrency": "Concurrency",
  "enrich.runPeriodically": "Run periodically",
  "enrich.save": "Save LLM config",
  "enrich.test": "Test connection",
  "enrich.testing": "Testing…",
  "enrich.saved": "LLM config saved",
  "enrich.run": "Run enrichment",
  "enrich.stop": "Stop",
  "enrich.refresh": "Refresh status",

  // Providers (model sources · unified list)
  "providers.title": "Providers · Model sources",
  "providers.desc": "Configure every model source (URL / key / model) here. Usage assignment below picks one per usage.",
  "providers.defaultChat": "Default chat model",
  "providers.defaultChatHint": "Fallback for chat usages: used by enrich / extract / consolidate / rerank when not assigned a specific provider.",
  "providers.defaultEmbed": "Default embedding model",
  "providers.defaultEmbedHint": "Vector model: used by semantic search / recall when not assigned. Leave empty to disable embeddings.",
  "providers.badgeChat": "chat",
  "providers.badgeEmbed": "embed",
  "providers.builtin": "built-in",
  "providers.add": "Add provider",
  "providers.addName": "provider name (e.g. cheap-consolidate)",
  "providers.noCustom": "No extra providers yet. Add one when a usage needs a different model.",

  // Usage assignment (pick one provider per usage)
  "assign.title": "Usage assignment",
  "assign.desc": "Which model each usage uses. Defaults to the matching built-in; pick one of the Providers above to differentiate.",
  "assign.defaultChat": "Default chat model",
  "assign.defaultEmbed": "Default embedding model",
  "assign.dangling": "These usages are bound to a missing provider and fell back to default:",

  // language switch
  "lang.label": "Language",
  "lang.zh": "中文",
  "lang.en": "English",

  // provider fields
  "provider.baseUrl": "Base URL",
  "provider.apiKey": "API Key",
  "provider.model": "Model",
  "provider.reasoningEffort": "Reasoning effort",
  "provider.balanceUrl": "Balance URL",
  "provider.enabled": "Enabled",

  // usages (business)
  "usage.enrich": "Session enrichment",
  "usage.enrich.desc": "Generates title / summary / keywords for sessions.",
  "usage.extract": "Memory extraction",
  "usage.extract.desc": "LLM-extracts candidate memories from sessions.",
  "usage.consolidate": "Memory consolidation",
  "usage.consolidate.desc": "Decision model that promotes staged candidates into long-term memory.",
  "usage.embed": "Embedding",
  "usage.embed.desc": "Generates vectors for semantic search / recall (usually a dedicated embedding provider).",
  "usage.recall_rerank": "Recall rerank",
  "usage.recall_rerank.desc": "Reranks recalled memories by relevance (optional; empty = no rerank).",

  // consolidate card (slimmed)
  "consolidate.title": "Memory Consolidation",
  "consolidate.desc": "Background worker that periodically consolidates staged candidates into long-term memory. Off by default; runs on the interval once enabled.",
  "consolidate.interval": "Interval",
  "consolidate.modelHint": "Which model? Set the consolidate binding in “LLM Configuration” above.",
  "consolidate.stateSaved": "Consolidate worker state saved",
  "consolidate.intervalSaved": "Consolidate interval saved",
  "consolidate.toggleFailed": "Failed to toggle consolidate worker",
  "consolidate.saveFailed": "Failed to save consolidate settings",

  // memory quality panel
  "quality.title": "Memory Pipeline Health",
  "quality.caveat": "Non-zero metrics only prove the instrumentation is wired, not that recall quality is good.",
};
