<script lang="ts">
  import { onMount, onDestroy } from "svelte";
  import { ApiError, isRemoteConnection } from "../../api/runtime.js";
  import {
    fetchEnrichStatus,
    fetchLLMConfig,
    saveLLMConfig,
    testLLMConnection,
    startEnrichJob,
    stopEnrichJob,
    fetchEnrichJob,
    type LLMEnrichJobState,
    type LLMEnrichmentStatusReport,
    type LLMConfigPayload,
    type LLMConfigResponse,
    type LLMTestResponse,
    fetchLLMProviders,
    saveLLMProviders,
    type LLMProvidersResponse,
    type LLMProvidersPayload,
    type LLMProviderConfigPayload,
  } from "../../api/llm.js";
  import { sync } from "../../stores/sync.svelte.js";
  import SettingsSection from "./SettingsSection.svelte";
  import { t } from "../../i18n/index.svelte";

  let status: LLMEnrichmentStatusReport | null = $state(null);
  let job = $state<LLMEnrichJobState | null>(null);
  let loading = $state(false);
  let configLoading = $state(false);
  let saving = $state(false);
  let testing = $state(false);
  let error = $state("");
  let configMessage = $state("");
  let testResult: LLMTestResponse | null = $state(null);
  let pollHandle: ReturnType<typeof setTimeout> | null = null;
  const remote = isRemoteConnection();
  const readOnly = $derived(sync.readOnly);
  const jobRunning = $derived(job?.running ?? false);
  function jobPercent(j: LLMEnrichJobState | null): number {
    if (!j || j.total <= 0) return 0;
    return Math.min(100, Math.round((j.processed / j.total) * 100));
  }
  const progressPct = $derived(jobPercent(job));
  const unavailableReason = $derived(
    remote
      ? "LLM enrichment is available only from the local server connection."
      : readOnly
        ? "LLM enrichment cannot run against a read-only backend."
        : "",
  );
  const canTrigger = $derived(!remote && !readOnly && !jobRunning);
  const canStop = $derived(!remote && !readOnly && jobRunning);
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
    embedBalanceUrl: "",
  });

  // Per-usage model override (merged in): chat/embed are configured by the two
  // provider blocks above; these three usages default to the Chat provider and
  // can optionally point at a named custom provider in the registry.
  type CustomProvider = { base_url: string; api_key: string; model: string; reasoning_effort: string };
  const overrideUsages = ["extract", "consolidate", "recall_rerank"];
  let customProviders = $state<Record<string, CustomProvider>>({});
  let usageOverrides = $state<Record<string, string>>({});
  let originalUsage = $state<Record<string, string>>({});
  let removedCustom = $state<Set<string>>(new Set());
  let usageWarnings = $state<string[]>([]);
  let newCustomName = $state("");
  const visibleCustom = $derived(
    Object.entries(customProviders).filter(([name]) => !removedCustom.has(name)),
  );

  function applyProvidersResponse(resp: LLMProvidersResponse) {
    // Registry providers minus the implicit ones bound to chat/embed: show all
    // named providers as custom (the chat/embed blocks own [llm]/[llm.embed]).
    customProviders = Object.fromEntries(
      Object.entries(resp.providers ?? {}).map(([name, p]) => [
        name,
        {
          base_url: p.base_url ?? "",
          api_key: maskedValue(p.has_api_key, p.api_key_preview),
          model: p.model ?? "",
          reasoning_effort: p.reasoning_effort ?? "",
        },
      ]),
    );
    const usage = resp.usage ?? {};
    usageOverrides = Object.fromEntries(overrideUsages.map((u) => [u, usage[u] ?? ""]));
    originalUsage = { ...usageOverrides };
    usageWarnings = [...(resp.usage_warnings ?? [])];
    removedCustom = new Set();
  }

  function applyOverridePreset(name: string, id: string) {
    const p = chatProviders.find((x) => x.id === id);
    if (!p || p.id === "custom") return;
    const prev: CustomProvider = customProviders[name] ?? { base_url: "", api_key: "", model: "", reasoning_effort: "" };
    customProviders = {
      ...customProviders,
      [name]: { base_url: p.baseUrl, api_key: prev.api_key, model: p.models[0] ?? "", reasoning_effort: prev.reasoning_effort },
    };
  }

  function addCustomProvider() {
    const name = newCustomName.trim();
    if (!name || customProviders[name]) return;
    customProviders = { ...customProviders, [name]: { base_url: "", api_key: "", model: "", reasoning_effort: "" } };
    removedCustom = new Set([...removedCustom].filter((n) => n !== name));
    newCustomName = "";
  }

  function removeCustomProvider(name: string) {
    removedCustom = new Set(removedCustom).add(name);
    const next = { ...usageOverrides };
    for (const u of overrideUsages) if (next[u] === name) next[u] = "";
    usageOverrides = next;
  }

  function providersPayload(): LLMProvidersPayload {
    const providers: Record<string, LLMProviderConfigPayload> = {};
    for (const [name, p] of Object.entries(customProviders)) {
      if (removedCustom.has(name)) continue;
      providers[name] = {
        enabled: true,
        base_url: p.base_url.trim(),
        api_key: keyPayload(p.api_key),
        model: p.model.trim(),
        reasoning_effort: p.reasoning_effort.trim(),
        balance_url: "",
      };
    }
    // Only manage the three override usages; leave enrich/embed bindings alone
    // (backend merge semantics preserve absent keys).
    const usage: Record<string, string> = {};
    for (const u of overrideUsages) {
      const provider = (usageOverrides[u] ?? "").trim();
      const wasBound = (originalUsage[u] ?? "").trim() !== "";
      if (provider === "") {
        if (wasBound) usage[u] = "";
        continue;
      }
      if (!removedCustom.has(provider)) usage[u] = provider;
    }
    return { providers, usage, delete_providers: Array.from(removedCustom) };
  }

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
    form.embedBalanceUrl = config.embed?.balance_url ?? "";
  }

  async function loadConfig() {
    if (remote) return;
    configLoading = true;
    configMessage = "";
    try {
      applyConfig(await fetchLLMConfig());
      applyProvidersResponse(await fetchLLMProviders());
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
        balance_url: form.embedBalanceUrl.trim(),
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
      applyProvidersResponse(await saveLLMProviders(providersPayload()));
      configMessage = t("enrich.saved");
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

  function stopPoll() {
    if (pollHandle) {
      clearTimeout(pollHandle);
      pollHandle = null;
    }
  }

  function schedulePoll() {
    stopPoll();
    pollHandle = setTimeout(async () => {
      pollHandle = null;
      try {
        job = await fetchEnrichJob();
      } catch {
        // Transient poll failure; keep the last known state and retry.
      }
      await loadStatus();
      if (job?.running) schedulePoll();
    }, 1500);
  }

  async function refreshJob() {
    if (remote) return;
    try {
      job = await fetchEnrichJob();
      if (job.running) schedulePoll();
    } catch {
      // No job state available yet; ignore.
    }
  }

  async function startEnrichment() {
    if (!canTrigger) return;
    error = "";
    try {
      job = await startEnrichJob();
      schedulePoll();
    } catch (err) {
      error = err instanceof ApiError ? err.message : "Failed to start LLM enrichment";
    }
  }

  async function stopEnrichment() {
    if (!canStop) return;
    error = "";
    try {
      job = await stopEnrichJob();
      // The job unwinds asynchronously; keep polling until it reports done.
      schedulePoll();
    } catch (err) {
      error = err instanceof ApiError ? err.message : "Failed to stop LLM enrichment";
    }
  }

  onMount(() => {
    loadStatus();
    loadConfig();
    refreshJob();
  });

  onDestroy(stopPoll);

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
  title={t("enrich.title")}
  description={t("enrich.desc")}
>
  {#if remote}
    <p class="muted" data-testid="llm-enrichment-remote">
      {unavailableReason}
    </p>
  {:else}
    <form class="config-form" onsubmit={(event) => { event.preventDefault(); saveConfig(); }}>
      <div class="field-group">
        <h4>{t("enrich.chatProvider")}</h4>
        <p class="block-hint">{t("enrich.chatProviderHint")}</p>
        <label>
          <span>{t("common.provider")}</span>
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
          <span>{t("provider.baseUrl")}</span>
          <input name="base_url" type="url" bind:value={form.baseUrl} placeholder="https://api.deepseek.com/v1" />
        </label>
        <label>
          <span>{t("provider.apiKey")}</span>
          <input name="api_key" type="password" bind:value={form.apiKey} autocomplete="off" />
        </label>
        <label>
          <span>{t("provider.model")}</span>
          <input name="model" type="text" bind:value={form.model} placeholder="deepseek-chat" list="chat-model-options" />
          <datalist id="chat-model-options">
            {#each chatModelSuggestions as m}
              <option value={m}></option>
            {/each}
          </datalist>
        </label>
        <label>
          <span>{t("provider.reasoningEffort")}</span>
          <select name="reasoning_effort" bind:value={form.reasoningEffort}>
            {#each reasoningOptions as option}
              <option value={option}>{option || "default"}</option>
            {/each}
          </select>
        </label>
      </div>

      <div class="field-group">
        <h4>{t("enrich.embedProvider")}</h4>
        <p class="block-hint">{t("enrich.embedProviderHint")}</p>
        <label>
          <span>{t("common.provider")}</span>
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
          <span>{t("provider.baseUrl")}</span>
          <input name="embed_base_url" type="url" bind:value={form.embedBaseUrl} placeholder="defaults to chat base URL" />
        </label>
        <label>
          <span>{t("provider.apiKey")}</span>
          <input name="embed_api_key" type="password" bind:value={form.embedApiKey} autocomplete="off" />
        </label>
        <label>
          <span>{t("provider.model")}</span>
          <input name="embed_model" type="text" bind:value={form.embedModel} placeholder="leave empty to disable embeddings" list="embed-model-options" />
          <datalist id="embed-model-options">
            {#each embedModelSuggestions as m}
              <option value={m}></option>
            {/each}
          </datalist>
        </label>
      </div>

      <div class="field-group">
        <h4>{t("override.title")}</h4>
        <p class="block-hint">{t("override.desc")}</p>

        <div class="usage-list" aria-label={t("override.title")}>
          {#each overrideUsages as usage}
            <div class="usage-row" data-testid={`usage-${usage}`}>
              <div class="usage-meta">
                <span class="usage-name">{t(`usage.${usage}`)} <code>{usage}</code></span>
                <span class="usage-desc">{t(`usage.${usage}.desc`)}</span>
              </div>
              <select bind:value={usageOverrides[usage]} aria-label={t(`usage.${usage}`)}>
                <option value="">{t("override.defaultChat")}</option>
                {#each visibleCustom as [name]}
                  <option value={name}>{name}</option>
                {/each}
              </select>
            </div>
          {/each}
        </div>

        <div class="custom-providers">
          <div class="custom-head">
            <strong>{t("override.custom")}</strong>
            <span class="block-hint">{t("override.customDesc")}</span>
          </div>
          {#if visibleCustom.length === 0}
            <p class="empty" data-testid="custom-empty">{t("override.empty")}</p>
          {:else}
            {#each visibleCustom as [name, prov]}
              <div class="custom-card" data-testid={`custom-${name}`}>
                <div class="custom-card-head">
                  <strong>{name}</strong>
                  <button type="button" class="ghost-btn" onclick={() => removeCustomProvider(name)}>{t("common.remove")}</button>
                </div>
                <div class="custom-grid">
                  <label>
                    <span>{t("common.provider")}</span>
                    <select value={matchProvider(chatProviders, prov.base_url)} onchange={(e) => applyOverridePreset(name, e.currentTarget.value)}>
                      {#each chatProviders as p}<option value={p.id}>{p.label}</option>{/each}
                    </select>
                  </label>
                  <label><span>{t("provider.model")}</span><input type="text" bind:value={prov.model} /></label>
                  <label><span>{t("provider.baseUrl")}</span><input type="url" bind:value={prov.base_url} /></label>
                  <label><span>{t("provider.apiKey")}</span><input type="password" bind:value={prov.api_key} autocomplete="off" /></label>
                  <label><span>{t("provider.reasoningEffort")}</span>
                    <select bind:value={prov.reasoning_effort}>
                      {#each reasoningOptions as option}<option value={option}>{option || "default"}</option>{/each}
                    </select>
                  </label>
                </div>
              </div>
            {/each}
          {/if}
          <div class="add-row">
            <input bind:value={newCustomName} placeholder={t("override.customName")} />
            <button type="button" class="ghost-btn" onclick={addCustomProvider} disabled={!newCustomName.trim()}>+ {t("override.addCustom")}</button>
          </div>
        </div>

        {#if usageWarnings.length}
          <div class="warning-list" role="alert">
            <p class="warning-head">{t("override.dangling")}</p>
            {#each usageWarnings as w}<p>{w}</p>{/each}
          </div>
        {/if}
      </div>

      {#if configLoading}
        <p class="muted">{t("common.loading")}</p>
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
      {#if error}
        <p class="error" role="alert">{error}</p>
      {/if}

      <div class="actions">
        <button class="trigger-btn" type="submit" disabled={!canSaveConfig}>
          {saving ? t("common.saving") : t("enrich.save")}
        </button>
        <button class="refresh-btn" type="button" onclick={testConfig} disabled={!canTestConfig}>
          {testing ? t("enrich.testing") : t("enrich.test")}
        </button>
      </div>
    </form>
  {/if}
</SettingsSection>

<SettingsSection title={t("feature.enrichTitle")} description={t("feature.enrichDesc")}>
  {#if remote}
    <p class="muted" data-testid="llm-enrichment-remote">{unavailableReason}</p>
  {:else}
    <form class="config-form" onsubmit={(event) => { event.preventDefault(); saveConfig(); }}>
      <label class="toggle-row">
        <input name="enabled" type="checkbox" bind:checked={form.enabled} />
        <span>{t("enrich.enable")}</span>
      </label>

      <div class="field-group schedule-grid">
        <h4>{t("enrich.scheduling")}</h4>
        <label>
          <span>{t("enrich.minMsgs")}</span>
          <input name="min_user_messages" type="number" min="0" bind:value={form.minUserMessages} />
        </label>
        <label>
          <span>{t("enrich.reenrichDelta")}</span>
          <input name="reenrich_msg_delta" type="number" min="0" bind:value={form.reenrichMsgDelta} />
        </label>
        <label>
          <span>{t("enrich.idleMinutes")}</span>
          <input name="reenrich_idle_minutes" type="number" min="0" bind:value={form.reenrichIdleMinutes} />
        </label>
        <label>
          <span>{t("enrich.concurrency")}</span>
          <input name="concurrency" type="number" min="0" bind:value={form.concurrency} />
        </label>
        <label class="toggle-row periodic-toggle">
          <input name="periodic" type="checkbox" bind:checked={form.periodic} />
          <span>{t("enrich.runPeriodically")}</span>
        </label>
      </div>

      {#if configMessage}
        <p class="result">{configMessage}</p>
      {/if}

      <div class="actions">
        <button class="trigger-btn" type="submit" disabled={!canSaveConfig}>
          {saving ? t("common.saving") : t("feature.enrichSave")}
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
      <p class="muted">{t("common.loading")}</p>
    {/if}

    {#if unavailableReason}
      <p class="muted" data-testid="llm-enrichment-unavailable">
        {unavailableReason}
      </p>
    {/if}

    {#if job && (jobRunning || job.done_at)}
      <div class="enrich-progress" data-testid="enrich-progress">
        <div
          class="progress-track"
          role="progressbar"
          aria-valuemin="0"
          aria-valuemax="100"
          aria-valuenow={progressPct}
        >
          <div class="progress-fill" style="width: {progressPct}%"></div>
        </div>
        {#if jobRunning}
          <p class="muted" data-testid="enrich-progress-label">
            Enriching {job.processed} / {job.total} ({progressPct}%){job.source === "periodic" ? " - periodic" : ""}...
          </p>
        {:else}
          <p class="result" data-testid="enrich-progress-label">
            Done: {job.succeeded} enriched, {job.failed} failed{job.skipped
              ? `, ${job.skipped} skipped`
              : ""}{job.no_content ? `, ${job.no_content} no content` : ""}.
          </p>
          <p class="muted" data-testid="enrich-cost">
            Tokens: {(job.prompt_tokens + job.completion_tokens).toLocaleString()} chat
            ({job.prompt_tokens.toLocaleString()} in / {job.completion_tokens.toLocaleString()} out){job.embed_tokens
              ? `, ${job.embed_tokens.toLocaleString()} embed`
              : ""}.
            {#if job.cost_spent}
              Chat spend this run: {job.cost_currency}
              {job.cost_spent}{job.balance_end ? ` (balance now ${job.cost_currency} ${job.balance_end})` : ""}.
            {/if}
            {#if job.embed_cost_spent}
              Embed spend this run: {job.embed_cost_currency}
              {job.embed_cost_spent}{job.embed_balance_end
                ? ` (balance now ${job.embed_cost_currency} ${job.embed_balance_end})`
                : ""}.
            {/if}
          </p>
        {/if}
        {#if job.error}
          <p class="error" role="alert">{job.error}</p>
        {/if}
      </div>
    {/if}

    {#if error}
      <p class="error" role="alert">{error}</p>
    {/if}

    <div class="actions">
      {#if jobRunning}
        <button class="refresh-btn" onclick={stopEnrichment} disabled={!canStop}>
          {t("enrich.stop")}
        </button>
      {:else}
        <button class="trigger-btn" onclick={startEnrichment} disabled={!canTrigger}>
          {t("enrich.run")}
        </button>
      {/if}
      <button class="refresh-btn" onclick={loadStatus} disabled={loading}>
        {t("enrich.refresh")}
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

  .enrich-progress {
    display: flex;
    flex-direction: column;
    gap: 4px;
  }

  .progress-track {
    width: 100%;
    height: 6px;
    border-radius: 999px;
    background: var(--bg-inset);
    border: 1px solid var(--border-muted);
    overflow: hidden;
  }

  .progress-fill {
    height: 100%;
    background: var(--accent-blue);
    transition: width 0.3s ease;
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
  .block-hint {
    font-size: 12px;
    color: var(--text-muted);
    margin: 0 0 4px;
  }
  .usage-list {
    display: grid;
    gap: 8px;
  }
  .usage-row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 12px;
    padding: 8px 10px;
    border: 1px solid var(--border-muted);
    border-radius: var(--radius-sm);
    background: var(--bg-surface);
  }
  .usage-meta {
    display: grid;
    gap: 2px;
    min-width: 0;
  }
  .usage-name {
    font-size: 13px;
    font-weight: 500;
    color: var(--text-primary);
  }
  .usage-name code {
    font-size: 12px;
    color: var(--text-muted);
    background: var(--bg-inset);
    padding: 1px 5px;
    border-radius: 4px;
  }
  .usage-desc {
    font-size: 12px;
    color: var(--text-muted);
  }
  .usage-row select {
    flex: 0 0 200px;
    max-width: 200px;
  }
  .custom-providers {
    display: grid;
    gap: 10px;
    margin-top: 12px;
    padding-top: 12px;
    border-top: 1px dashed var(--border-muted);
  }
  .custom-head {
    display: grid;
    gap: 2px;
  }
  .custom-card {
    padding: 10px;
    border: 1px solid var(--border-muted);
    border-radius: var(--radius-sm);
    background: var(--bg-inset);
    display: grid;
    gap: 8px;
  }
  .custom-card-head {
    display: flex;
    align-items: center;
    justify-content: space-between;
  }
  .custom-grid {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: 8px;
  }
  .empty {
    padding: 10px;
    border: 1px dashed var(--border-default);
    border-radius: var(--radius-sm);
    color: var(--text-muted);
    font-size: 13px;
    background: var(--bg-inset);
  }
  .add-row {
    display: flex;
    gap: 8px;
    align-items: center;
  }
  .add-row input {
    flex: 1 1 auto;
  }
  .ghost-btn {
    height: 30px;
    padding: 0 12px;
    border-radius: var(--radius-sm);
    border: 1px solid var(--border-muted);
    background: var(--bg-inset);
    color: var(--text-primary);
    cursor: pointer;
  }
  .warning-list {
    display: grid;
    gap: 4px;
    padding: 8px 10px;
    border: 1px solid var(--accent-amber, #f59e0b);
    border-radius: var(--radius-sm);
    background: color-mix(in srgb, var(--accent-amber, #f59e0b) 12%, transparent);
  }
  .warning-list p { margin: 0; }
  .warning-head { font-weight: 600; }
  @media (max-width: 549px) {
    .custom-grid { grid-template-columns: 1fr; }
    .usage-row { flex-direction: column; align-items: stretch; }
    .usage-row select { flex: 1 1 auto; max-width: none; }
  }
</style>
