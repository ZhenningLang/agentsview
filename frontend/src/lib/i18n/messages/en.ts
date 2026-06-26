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

  // language switch
  "lang.label": "Language",
  "lang.zh": "中文",
  "lang.en": "English",

  // LLM config section
  "llm.title": "LLM Configuration",
  "llm.desc": "Configure model sources per usage. Unbound usages fall back to the default ([llm]).",
  "llm.providerPool": "Provider Pool",
  "llm.providerPoolDesc": "Configure model sources (account / URL / model) here. Each usage picks from this pool.",
  "llm.usageBindings": "Usage Bindings",
  "llm.usageBindingsDesc": "Which provider each usage uses. Empty = use the default config.",
  "llm.addProvider": "Add Provider",
  "llm.newProviderName": "provider name (e.g. deepseek-chat)",
  "llm.providerEmptyState": "No providers configured. Without one, every usage uses the default [llm]; click “Add Provider” to give a usage its own model.",
  "llm.defaultOption": "Default ([llm] config)",
  "llm.saveConfig": "Save LLM config",
  "llm.saved": "LLM config saved",
  "llm.saveFailed": "Failed to save LLM config",
  "llm.loadFailed": "Failed to load LLM config",
  "llm.dangling": "These usages are bound to a missing provider and fell back to default:",

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
