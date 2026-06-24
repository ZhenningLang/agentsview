<script lang="ts">
  import { onMount } from "svelte";
  import { ApiError, isRemoteConnection } from "../../api/runtime.js";
  import {
    fetchEnrichStatus,
    fetchLLMConfig,
    saveLLMConfig,
    testLLMConnection,
    triggerEnrich,
    type LLMEnrichResponse,
    type LLMEnrichmentStatusReport,
    type LLMConfigPayload,
    type LLMConfigResponse,
    type LLMTestResponse,
  } from "../../api/llm.js";
  import { sync } from "../../stores/sync.svelte.js";
  import SettingsSection from "./SettingsSection.svelte";

  let status: LLMEnrichmentStatusReport | null = $state(null);
  let result: LLMEnrichResponse | null = $state(null);
  let loading = $state(false);
  let configLoading = $state(false);
  let running = $state(false);
  let saving = $state(false);
  let testing = $state(false);
  let error = $state("");
  let configMessage = $state("");
  let testResult: LLMTestResponse | null = $state(null);
  const remote = isRemoteConnection();
  const readOnly = $derived(sync.readOnly);
  const unavailableReason = $derived(
    remote
      ? "LLM enrichment is available only from the local server connection."
      : readOnly
        ? "LLM enrichment cannot run against a read-only backend."
        : "",
  );
  const canTrigger = $derived(!remote && !readOnly && !running);
  const canSaveConfig = $derived(!remote && !readOnly && !saving);
  const canTestConfig = $derived(!remote && !testing);

  const keySentinel = "********";
  const reasoningOptions = ["", "low", "medium", "high"];

  type ProviderPreset = {
    id: string;
    label: string;
    baseUrl: string;
    models: string[];
  };

  // Known OpenAI-compatible providers. Selecting one fills the base URL and
  // suggests models; "Custom" leaves the fields for manual entry. Model names
  // are base-URL specific: direct OpenAI uses bare ids ("text-embedding-3-large"),
  // while OpenRouter requires the namespaced form ("openai/text-embedding-3-large").
  // OpenRouter embeddings are confirmed working via a live Test connection call;
  // it ignores the dimensions param and returns 3072-dim vectors, which is fine
  // here since we store the full vector + dimension.
  const chatProviders: ProviderPreset[] = [
    { id: "deepseek", label: "DeepSeek", baseUrl: "https://api.deepseek.com", models: ["deepseek-chat", "deepseek-reasoner"] },
    { id: "openai", label: "OpenAI", baseUrl: "https://api.openai.com/v1", models: ["gpt-4o-mini", "gpt-4o"] },
    { id: "openrouter", label: "OpenRouter", baseUrl: "https://openrouter.ai/api/v1", models: ["deepseek/deepseek-chat", "openai/gpt-4o-mini", "anthropic/claude-3.5-sonnet"] },
    { id: "moonshot", label: "Moonshot (Kimi)", baseUrl: "https://api.moonshot.cn/v1", models: ["moonshot-v1-8k", "moonshot-v1-32k"] },
    { id: "ollama", label: "Ollama (local)", baseUrl: "http://localhost:11434/v1", models: ["qwen2.5", "llama3.1"] },
    { id: "custom", label: "Custom", baseUrl: "", models: [] },
  ];
  const embedProviders: ProviderPreset[] = [
    { id: "openai", label: "OpenAI", baseUrl: "https://api.openai.com/v1", models: ["text-embedding-3-small", "text-embedding-3-large"] },
    { id: "openrouter", label: "OpenRouter", baseUrl: "https://openrouter.ai/api/v1", models: ["openai/text-embedding-3-large", "openai/text-embedding-3-small"] },
    { id: "ollama", label: "Ollama (local)", baseUrl: "http://localhost:11434/v1", models: ["bge-m3", "nomic-embed-text"] },
    { id: "custom", label: "Custom", baseUrl: "", models: [] },
  ];

  function matchProvider(presets: ProviderPreset[], baseUrl: string): string {
    const url = baseUrl.trim().replace(/\/+$/, "");
    const hit = presets.find((p) => p.baseUrl && p.baseUrl.replace(/\/+$/, "") === url);
    return hit ? hit.id : "custom";
  }
  // Picking a preset (an explicit user action; not fired on initial load)
  // fills the base URL and the provider's default model. The user can then
  // edit the model via the datalist. "Custom" leaves both fields untouched.
  function applyChatProvider(id: string) {
    const p = chatProviders.find((x) => x.id === id);
    if (!p || p.id === "custom") return;
    form.baseUrl = p.baseUrl;
    form.model = p.models[0] ?? "";
  }
  function applyEmbedProvider(id: string) {
    const p = embedProviders.find((x) => x.id === id);
    if (!p || p.id === "custom") return;
    form.embedBaseUrl = p.baseUrl;
    form.embedModel = p.models[0] ?? "";
  }
  const chatModelSuggestions = $derived(
    chatProviders.find((p) => p.id === matchProvider(chatProviders, form.baseUrl))
      ?.models ?? [],
  );
  const embedModelSuggestions = $derived(
    embedProviders.find((p) => p.id === matchProvider(embedProviders, form.embedBaseUrl))
      ?.models ?? [],
  );

  let form = $state({
    enabled: false,
    baseUrl: "",
    apiKey: "",
    model: "",
    reasoningEffort: "",
    minUserMessages: 0,
    reenrichMsgDelta: 0,
    reenrichIdleMinutes: 0,
    concurrency: 0,
    periodic: false,
    balanceUrl: "",
    embedBaseUrl: "",
    embedApiKey: "",
    embedModel: "",
  });

  type CountCard = readonly [string, number];

  async function loadStatus() {
    if (remote) return;
    loading = true;
    error = "";
    try {
      status = await fetchEnrichStatus();
    } catch (err) {
      error = err instanceof ApiError ? err.message : "Failed to load LLM status";
    } finally {
      loading = false;
    }
  }

  function maskedValue(hasKey: boolean, preview?: string): string {
    if (!hasKey) return "";
    return `${keySentinel}${preview ?? ""}`;
  }

  function applyConfig(config: LLMConfigResponse) {
    form.enabled = config.enabled;
    form.baseUrl = config.base_url ?? "";
    form.apiKey = maskedValue(config.has_api_key, config.api_key_preview);
    form.model = config.model ?? "";
    form.reasoningEffort = config.reasoning_effort ?? "";
    form.minUserMessages = config.min_user_messages;
    form.reenrichMsgDelta = config.reenrich_msg_delta;
    form.reenrichIdleMinutes = config.reenrich_idle_minutes;
    form.concurrency = config.concurrency;
    form.periodic = config.periodic;
    form.balanceUrl = config.balance_url ?? "";
    form.embedBaseUrl = config.embed?.base_url ?? "";
    form.embedApiKey = maskedValue(config.embed?.has_api_key ?? false, config.embed?.api_key_preview);
    form.embedModel = config.embed?.model ?? "";
  }

  async function loadConfig() {
    if (remote) return;
    configLoading = true;
    configMessage = "";
    try {
      applyConfig(await fetchLLMConfig());
    } catch (err) {
      error = err instanceof ApiError ? err.message : "Failed to load LLM config";
    } finally {
      configLoading = false;
    }
  }

  function keyPayload(value: string): string {
    const trimmed = value.trim();
    return trimmed.startsWith(keySentinel) ? keySentinel : trimmed;
  }

  function formPayload(): LLMConfigPayload {
    return {
      enabled: form.enabled,
      base_url: form.baseUrl.trim(),
      api_key: keyPayload(form.apiKey),
      model: form.model.trim(),
      reasoning_effort: form.reasoningEffort,
      min_user_messages: Number(form.minUserMessages) || 0,
      reenrich_msg_delta: Number(form.reenrichMsgDelta) || 0,
      reenrich_idle_minutes: Number(form.reenrichIdleMinutes) || 0,
      concurrency: Number(form.concurrency) || 0,
      periodic: form.periodic,
      balance_url: form.balanceUrl.trim(),
      embed: {
        base_url: form.embedBaseUrl.trim(),
        api_key: keyPayload(form.embedApiKey),
        model: form.embedModel.trim(),
      },
    };
  }

  async function saveConfig() {
    if (!canSaveConfig) return;
    saving = true;
    error = "";
    configMessage = "";
    try {
      applyConfig(await saveLLMConfig(formPayload()));
      configMessage = "LLM config saved";
    } catch (err) {
      error = err instanceof ApiError ? err.message : "Failed to save LLM config";
    } finally {
      saving = false;
    }
  }

  async function testConfig() {
    if (!canTestConfig) return;
    testing = true;
    error = "";
    testResult = null;
    try {
      testResult = await testLLMConnection(formPayload());
    } catch (err) {
      error = err instanceof ApiError ? err.message : "Failed to test LLM connection";
    } finally {
      testing = false;
    }
  }

  async function runEnrichment() {
    if (!canTrigger) return;
    running = true;
    error = "";
    result = null;
    try {
      result = await triggerEnrich({ limit: 25 });
      status = await fetchEnrichStatus();
    } catch (err) {
      error = err instanceof ApiError ? err.message : "Failed to trigger LLM enrichment";
    } finally {
      running = false;
    }
  }

  onMount(() => {
    loadStatus();
    loadConfig();
  });

  function countCards(report: LLMEnrichmentStatusReport | null): CountCard[] {
    return [
      ["Total", report?.total ?? 0],
      ["Enriched", report?.enriched ?? 0],
      ["Pending", report?.pending ?? 0],
      ["Skipped", report?.skipped_too_short ?? 0],
      ["No content", report?.no_content ?? 0],
      ["Errors", report?.errors ?? 0],
    ];
  }

  function channelText(label: string, result: LLMTestResponse["chat"]): string {
    if (result.disabled) return `${label}: disabled`;
    return `${label}: ${result.ok ? "ok" : "error"}${result.message ? ` - ${result.message}` : ""}`;
  }
</script>

<SettingsSection
  title="LLM enrichment"
  description="Generate optional titles, summaries, and keywords for local sessions."
>
  {#if remote}
    <p class="muted" data-testid="llm-enrichment-remote">
      {unavailableReason}
    </p>
  {:else}
    <form class="config-form" onsubmit={(event) => { event.preventDefault(); saveConfig(); }}>
      <label class="toggle-row">
        <input name="enabled" type="checkbox" bind:checked={form.enabled} />
        <span>Enable LLM enrichment</span>
      </label>

      <div class="field-group">
        <h4>Chat provider</h4>
        <label>
          <span>Provider</span>
          <select
            name="chat_provider"
            value={matchProvider(chatProviders, form.baseUrl)}
            onchange={(e) => applyChatProvider(e.currentTarget.value)}
          >
            {#each chatProviders as p}
              <option value={p.id}>{p.label}</option>
            {/each}
          </select>
        </label>
        <label>
          <span>Base URL</span>
          <input name="base_url" type="url" bind:value={form.baseUrl} placeholder="https://api.deepseek.com/v1" />
        </label>
        <label>
          <span>API key</span>
          <input name="api_key" type="password" bind:value={form.apiKey} autocomplete="off" />
        </label>
        <label>
          <span>Model</span>
          <input name="model" type="text" bind:value={form.model} placeholder="deepseek-chat" list="chat-model-options" />
          <datalist id="chat-model-options">
            {#each chatModelSuggestions as m}
              <option value={m}></option>
            {/each}
          </datalist>
        </label>
        <label>
          <span>Reasoning effort</span>
          <select name="reasoning_effort" bind:value={form.reasoningEffort}>
            {#each reasoningOptions as option}
              <option value={option}>{option || "default"}</option>
            {/each}
          </select>
        </label>
      </div>

      <div class="field-group">
        <h4>Embedding provider</h4>
        <label>
          <span>Provider</span>
          <select
            name="embed_provider"
            value={matchProvider(embedProviders, form.embedBaseUrl)}
            onchange={(e) => applyEmbedProvider(e.currentTarget.value)}
          >
            {#each embedProviders as p}
              <option value={p.id}>{p.label}</option>
            {/each}
          </select>
        </label>
        <label>
          <span>Base URL</span>
          <input name="embed_base_url" type="url" bind:value={form.embedBaseUrl} placeholder="defaults to chat base URL" />
        </label>
        <label>
          <span>API key</span>
          <input name="embed_api_key" type="password" bind:value={form.embedApiKey} autocomplete="off" />
        </label>
        <label>
          <span>Model</span>
          <input name="embed_model" type="text" bind:value={form.embedModel} placeholder="leave empty to disable embeddings" list="embed-model-options" />
          <datalist id="embed-model-options">
            {#each embedModelSuggestions as m}
              <option value={m}></option>
            {/each}
          </datalist>
        </label>
      </div>

      <div class="field-group schedule-grid">
        <h4>Scheduling</h4>
        <label>
          <span>Min user messages</span>
          <input name="min_user_messages" type="number" min="0" bind:value={form.minUserMessages} />
        </label>
        <label>
          <span>Re-enrich message delta</span>
          <input name="reenrich_msg_delta" type="number" min="0" bind:value={form.reenrichMsgDelta} />
        </label>
        <label>
          <span>Idle minutes</span>
          <input name="reenrich_idle_minutes" type="number" min="0" bind:value={form.reenrichIdleMinutes} />
        </label>
        <label>
          <span>Concurrency</span>
          <input name="concurrency" type="number" min="0" bind:value={form.concurrency} />
        </label>
        <label class="toggle-row periodic-toggle">
          <input name="periodic" type="checkbox" bind:checked={form.periodic} />
          <span>Run periodically</span>
        </label>
      </div>

      {#if configLoading}
        <p class="muted">Loading LLM config...</p>
      {/if}
      {#if configMessage}
        <p class="result">{configMessage}</p>
      {/if}
      {#if testResult}
        <div class="test-result" data-testid="llm-test-result">
          <p>{channelText("chat", testResult.chat)}</p>
          <p>{channelText("embed", testResult.embed)}</p>
        </div>
      {/if}

      <div class="actions">
        <button class="trigger-btn" type="submit" disabled={!canSaveConfig}>
          {saving ? "Saving..." : "Save LLM config"}
        </button>
        <button class="refresh-btn" type="button" onclick={testConfig} disabled={!canTestConfig}>
          {testing ? "Testing..." : "Test connection"}
        </button>
      </div>
    </form>

    <div class="status-grid" aria-label="LLM enrichment status">
      {#each countCards(status) as [label, value]}
        <div class="status-card">
          <span class="status-value">{value}</span>
          <span class="status-label">{label}</span>
        </div>
      {/each}
    </div>

    {#if loading}
      <p class="muted">Loading enrichment status...</p>
    {/if}

    {#if unavailableReason}
      <p class="muted" data-testid="llm-enrichment-unavailable">
        {unavailableReason}
      </p>
    {/if}

    {#if result}
      <p class="result" data-testid="llm-enrichment-result">
        Enriched {result.enriched} of {result.candidates} candidates in {result.elapsed_ms}ms.
        Skipped {result.skipped}, no content {result.no_content}, errors {result.errors}.
      </p>
    {/if}

    {#if error}
      <p class="error" role="alert">{error}</p>
    {/if}

    <div class="actions">
      <button
        class="trigger-btn"
        onclick={runEnrichment}
        disabled={!canTrigger}
      >
        {running ? "Enriching..." : "Run enrichment"}
      </button>
      <button class="refresh-btn" onclick={loadStatus} disabled={loading || running}>
        Refresh status
      </button>
    </div>
  {/if}
</SettingsSection>

<style>
  .status-grid {
    display: grid;
    grid-template-columns: repeat(3, minmax(0, 1fr));
    gap: 8px;
  }

  .config-form,
  .field-group {
    display: flex;
    flex-direction: column;
    gap: 10px;
  }

  .field-group {
    padding: 12px;
    border: 1px solid var(--border-muted);
    border-radius: var(--radius-sm);
    background: var(--bg-inset);
  }

  .field-group h4 {
    margin: 0;
    font-size: 11px;
    font-weight: 650;
    color: var(--text-secondary);
  }

  .field-group label {
    display: grid;
    grid-template-columns: minmax(110px, 0.42fr) minmax(0, 1fr);
    align-items: center;
    gap: 8px;
    min-width: 0;
  }

  .field-group span,
  .toggle-row span {
    font-size: 11px;
    color: var(--text-muted);
  }

  .field-group input,
  .field-group select {
    min-width: 0;
    height: 30px;
    padding: 0 9px;
    border: 1px solid var(--border-muted);
    border-radius: var(--radius-sm);
    background: var(--bg-surface);
    color: var(--text-primary);
    font-size: 12px;
  }

  .field-group input:focus,
  .field-group select:focus {
    outline: none;
    border-color: var(--accent-blue);
  }

  .toggle-row,
  .field-group .toggle-row {
    display: flex;
    align-items: center;
    gap: 8px;
  }

  .toggle-row input {
    margin: 0;
  }

  .periodic-toggle {
    grid-column: 1 / -1;
  }

  .test-result {
    display: flex;
    flex-direction: column;
    gap: 3px;
    padding: 8px 10px;
    border: 1px solid var(--border-muted);
    border-radius: var(--radius-sm);
    color: var(--text-secondary);
    background: var(--bg-inset);
  }

  .test-result p {
    margin: 0;
    font-size: 12px;
    line-height: 1.4;
  }

  .status-card {
    min-width: 0;
    padding: 10px;
    border: 1px solid var(--border-muted);
    border-radius: var(--radius-sm);
    background: var(--bg-inset);
  }

  .status-value {
    display: block;
    font-size: 18px;
    font-weight: 650;
    color: var(--text-primary);
    line-height: 1.1;
  }

  .status-label {
    display: block;
    margin-top: 3px;
    font-size: 10px;
    color: var(--text-muted);
    white-space: nowrap;
  }

  .muted,
  .result,
  .error {
    margin: 0;
    font-size: 12px;
    line-height: 1.5;
  }

  .muted {
    color: var(--text-muted);
  }

  .result {
    color: var(--text-secondary);
  }

  .error {
    color: var(--accent-red, #ef4444);
  }

  .actions {
    display: flex;
    flex-wrap: wrap;
    gap: 8px;
  }

  .trigger-btn,
  .refresh-btn {
    height: 28px;
    padding: 0 12px;
    border-radius: var(--radius-sm);
    font-size: 12px;
    font-weight: 500;
    border: 1px solid var(--border-muted);
    cursor: pointer;
  }

  .trigger-btn {
    color: white;
    background: var(--accent-blue);
    border-color: var(--accent-blue);
  }

  .refresh-btn {
    color: var(--text-secondary);
    background: var(--bg-inset);
  }

  .trigger-btn:disabled,
  .refresh-btn:disabled {
    opacity: 0.6;
    cursor: default;
  }

  @media (max-width: 549px) {
    .status-grid {
      grid-template-columns: repeat(2, minmax(0, 1fr));
    }

    .field-group label {
      grid-template-columns: 1fr;
      gap: 4px;
    }
  }
</style>
