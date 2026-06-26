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

  // Providers (model sources · vendor+key, multiple instances)
  "providers.title": "Providers · Model sources",
  "providers.desc": "Each = name (editable) + vendor + API key. Multiple per vendor allowed (different keys/accounts). Models are set per usage below.",
  "providers.name": "Name",
  "providers.vendor": "Vendor",
  "providers.customVendor": "Custom",
  "providers.add": "Add provider",
  "providers.empty": "No providers yet. Click “Add provider” to configure one (vendor + key).",
  "providers.linkHint": "Usage assignment below references providers by name, so names must be unique and non-empty.",
  "providers.nameEmpty": "A provider name is empty — please fill it in.",
  "providers.nameDup": "Duplicate provider name: ",

  // Usage assignment (pick a provider + model per usage)
  "assign.title": "Usage assignment",
  "assign.desc": "For each usage: pick a configured provider + the model it runs.",
  "assign.use": "Use",
  "assign.noProvider": "(configure a provider above first)",
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
