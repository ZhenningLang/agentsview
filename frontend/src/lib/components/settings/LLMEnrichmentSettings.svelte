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
    type LLMTestChannelResult,
    type LLMTestRequest,
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
  let savingEnrich = $state(false);
  let error = $state("");
  let configMessage = $state("");
  let enrichMessage = $state("");
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
  const canSaveEnrich = $derived(!remote && !readOnly && !savingEnrich);
  const canTest = $derived(!remote && !readOnly);

  const keySentinel = "********";
  // The five usages, rendered in this order in the assignment table.
  const USAGES = ["enrich", "embed", "extract", "consolidate", "recall_rerank"] as const;
  type UsageKey = (typeof USAGES)[number];

  // Known OpenAI-compatible vendors. The vendor determines the base URL; only
  // "custom" exposes a base URL field. Model names are vendor-specific.
  type Vendor = { id: string; label: string; baseUrl: string; chatModels: string[]; embedModels: string[] };
  const VENDORS: Vendor[] = [
    { id: "deepseek", label: "DeepSeek", baseUrl: "https://api.deepseek.com", chatModels: ["deepseek-chat", "deepseek-reasoner"], embedModels: [] },
    { id: "openai", label: "OpenAI", baseUrl: "https://api.openai.com/v1", chatModels: ["gpt-4o-mini", "gpt-4o"], embedModels: ["text-embedding-3-small", "text-embedding-3-large"] },
    { id: "openrouter", label: "OpenRouter", baseUrl: "https://openrouter.ai/api/v1", chatModels: ["deepseek/deepseek-chat", "openai/gpt-4o-mini", "anthropic/claude-3.5-sonnet"], embedModels: ["openai/text-embedding-3-large", "openai/text-embedding-3-small"] },
    { id: "moonshot", label: "Moonshot (Kimi)", baseUrl: "https://api.moonshot.cn/v1", chatModels: ["moonshot-v1-8k", "moonshot-v1-32k"], embedModels: [] },
    { id: "ollama", label: "Ollama (local)", baseUrl: "http://localhost:11434/v1", chatModels: ["qwen2.5", "llama3.1"], embedModels: ["bge-m3", "nomic-embed-text"] },
    { id: "custom", label: "Custom", baseUrl: "", chatModels: [], embedModels: [] },
  ];
  const CUSTOM_VENDOR = VENDORS[VENDORS.length - 1]!;
  function vendorById(id: string): Vendor {
    return VENDORS.find((v) => v.id === id) ?? CUSTOM_VENDOR;
  }
  function vendorLabel(v: Vendor): string {
    return v.id === "custom" ? t("providers.customVendor") : v.label;
  }
  function matchVendor(baseUrl: string): string {
    const url = baseUrl.trim().replace(/\/+$/, "");
    if (!url) return "custom";
    const hit = VENDORS.find((v) => v.baseUrl && v.baseUrl.replace(/\/+$/, "") === url);
    return hit ? hit.id : "custom";
  }
  function defaultChatModel(vendorId: string): string {
    return vendorById(vendorId).chatModels[0] ?? "gpt-4o-mini";
  }

  // form holds [llm] base (enable + scheduling, plus the connection fields used
  // only to seed providers on load) and [llm.embed] base. The connection fields
  // are NOT rendered as inputs in the new layout; providers own connections.
  let form = $state({
    enabled: false,
    baseUrl: "",
    apiKey: "",
    model: "",
    hasKey: false,
    keyPreview: "",
    minUserMessages: 0,
    reenrichMsgDelta: 0,
    reenrichIdleMinutes: 0,
    concurrency: 0,
    periodic: false,
    embedBaseUrl: "",
    embedModel: "",
    embedHasKey: false,
    embedKeyPreview: "",
  });

  type Provider = { uid: number; name: string; vendor: string; base_url: string; api_key: string };
  type Binding = { providerUid: number | null; model: string };
  let providers = $state<Provider[]>([]);
  let bindings = $state<Record<UsageKey, Binding>>({
    enrich: { providerUid: null, model: "" },
    embed: { providerUid: null, model: "" },
    extract: { providerUid: null, model: "" },
    consolidate: { providerUid: null, model: "" },
    recall_rerank: { providerUid: null, model: "" },
  });
  let usageWarnings = $state<string[]>([]);
  let loadedRegistryNames = $state<Set<string>>(new Set());
  let uidCounter = 0;

  // Per-target test state. Keys: "provider:<uid>", "usage:<usage>".
  let testResults = $state<Record<string, LLMTestChannelResult>>({});
  let testingTarget = $state<string | null>(null);

  function maskedValue(hasKey: boolean, preview?: string): string {
    if (!hasKey) return "";
    return `${keySentinel}${preview ?? ""}`;
  }
  function keyPayload(value: string): string {
    const trimmed = value.trim();
    return trimmed.startsWith(keySentinel) ? keySentinel : trimmed;
  }
  function autoName(vendorId: string): string {
    const taken = new Set(providers.map((p) => p.name));
    for (let n = 1; ; n++) {
      const candidate = `${vendorId}-${n}`;
      if (!taken.has(candidate)) return candidate;
    }
  }

  function applyConfig(config: LLMConfigResponse) {
    form.enabled = config.enabled;
    form.baseUrl = config.base_url ?? "";
    form.model = config.model ?? "";
    form.hasKey = config.has_api_key;
    form.keyPreview = config.api_key_preview ?? "";
    form.minUserMessages = config.min_user_messages;
    form.reenrichMsgDelta = config.reenrich_msg_delta;
    form.reenrichIdleMinutes = config.reenrich_idle_minutes;
    form.concurrency = config.concurrency;
    form.periodic = config.periodic;
    form.embedBaseUrl = config.embed?.base_url ?? "";
    form.embedModel = config.embed?.model ?? "";
    form.embedHasKey = config.embed?.has_api_key ?? false;
    form.embedKeyPreview = config.embed?.api_key_preview ?? "";
  }

  // Seed a provider from a legacy [llm]/[llm.embed] connection if no existing
  // provider already covers that base URL. Returns the provider uid to bind to.
  function seedProvider(list: Provider[], baseUrl: string, hasKey: boolean, preview: string): number | null {
    const url = (baseUrl ?? "").trim();
    if (!url) return null;
    const norm = url.replace(/\/+$/, "");
    const existing = list.find((p) => p.base_url.trim().replace(/\/+$/, "") === norm);
    if (existing) return existing.uid;
    const vendor = matchVendor(url);
    const taken = new Set(list.map((p) => p.name));
    let name = "";
    for (let n = 1; ; n++) {
      const candidate = `${vendor}-${n}`;
      if (!taken.has(candidate)) { name = candidate; break; }
    }
    const uid = uidCounter++;
    list.push({ uid, name, vendor, base_url: url, api_key: maskedValue(hasKey, preview) });
    return uid;
  }

  function applyProvidersResponse(cfg: LLMConfigResponse, resp: LLMProvidersResponse) {
    const list: Provider[] = [];
    const regNames = new Set<string>();
    for (const [name, p] of Object.entries(resp.providers ?? {})) {
      regNames.add(name);
      list.push({
        uid: uidCounter++,
        name,
        vendor: matchVendor(p.base_url ?? ""),
        base_url: p.base_url ?? "",
        api_key: maskedValue(p.has_api_key, p.api_key_preview),
      });
    }
    loadedRegistryNames = regNames;
    // Migrate legacy defaults into the unified list (idempotent by base URL).
    const chatSeed = seedProvider(list, cfg.base_url ?? "", cfg.has_api_key, cfg.api_key_preview ?? "");
    const embedSeed = seedProvider(list, cfg.embed?.base_url ?? "", cfg.embed?.has_api_key ?? false, cfg.embed?.api_key_preview ?? "");
    providers = list;

    const usage = resp.usage ?? {};
    const usageModel = resp.usage_model ?? {};
    const next: Record<UsageKey, Binding> = {
      enrich: { providerUid: null, model: "" },
      embed: { providerUid: null, model: "" },
      extract: { providerUid: null, model: "" },
      consolidate: { providerUid: null, model: "" },
      recall_rerank: { providerUid: null, model: "" },
    };
    for (const u of USAGES) {
      const boundName = (usage[u] ?? "").trim();
      if (boundName) {
        const found = list.find((p) => p.name === boundName);
        next[u] = { providerUid: found ? found.uid : null, model: usageModel[u] ?? "" };
      } else if (u === "embed") {
        next[u] = { providerUid: embedSeed, model: cfg.embed?.model ?? "" };
      } else {
        next[u] = { providerUid: chatSeed, model: cfg.model ?? "" };
      }
    }
    bindings = next;
    usageWarnings = [...(resp.usage_warnings ?? [])];
  }

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

  async function loadConfig() {
    if (remote) return;
    configLoading = true;
    configMessage = "";
    try {
      const cfg = await fetchLLMConfig();
      applyConfig(cfg);
      applyProvidersResponse(cfg, await fetchLLMProviders());
    } catch (err) {
      error = err instanceof ApiError ? err.message : "Failed to load LLM config";
    } finally {
      configLoading = false;
    }
  }

  // --- Provider editing ---
  function addProvider() {
    const vendor = "deepseek";
    providers = [...providers, { uid: uidCounter++, name: autoName(vendor), vendor, base_url: vendorById(vendor).baseUrl, api_key: "" }];
  }
  function removeProvider(uid: number) {
    const remaining = providers.filter((p) => p.uid !== uid);
    providers = remaining;
    const fallback = remaining[0]?.uid ?? null;
    const next = { ...bindings };
    for (const u of USAGES) {
      if (next[u]?.providerUid === uid) next[u] = { providerUid: fallback, model: next[u].model };
    }
    bindings = next;
  }
  function onVendorChange(uid: number, vendorId: string) {
    providers = providers.map((p) => {
      if (p.uid !== uid) return p;
      const base_url = vendorId === "custom" ? p.base_url : vendorById(vendorId).baseUrl;
      return { ...p, vendor: vendorId, base_url };
    });
  }
  function providerByUid(uid: number | null): Provider | undefined {
    return uid == null ? undefined : providers.find((p) => p.uid === uid);
  }

  function providersPayload(): LLMProvidersPayload {
    const provs: Record<string, LLMProviderConfigPayload> = {};
    const nameByUid = new Map<number, string>();
    for (const p of providers) {
      const name = p.name.trim();
      if (!name) continue;
      nameByUid.set(p.uid, name);
      provs[name] = {
        enabled: true,
        base_url: p.base_url.trim(),
        api_key: keyPayload(p.api_key),
        model: "",
        reasoning_effort: "",
        balance_url: "",
      };
    }
    const usage: Record<string, string> = {};
    const usage_model: Record<string, string> = {};
    for (const u of USAGES) {
      const b = bindings[u];
      const name = b?.providerUid != null ? nameByUid.get(b.providerUid) : undefined;
      if (name) {
        usage[u] = name;
        usage_model[u] = (b.model ?? "").trim();
      }
    }
    const currentNames = new Set(Object.keys(provs));
    const delete_providers = [...loadedRegistryNames].filter((n) => !currentNames.has(n));
    return { providers: provs, usage, usage_model, delete_providers };
  }

  async function saveProviders() {
    if (!canSaveConfig) return;
    saving = true;
    error = "";
    configMessage = "";
    try {
      const cfg = await fetchLLMConfig();
      applyProvidersResponse(cfg, await saveLLMProviders(providersPayload()));
      configMessage = t("enrich.saved");
    } catch (err) {
      error = err instanceof ApiError ? err.message : "Failed to save LLM config";
    } finally {
      saving = false;
    }
  }

  function enrichPayload(): LLMConfigPayload {
    // Connection fields are intentionally omitted so the server's pointer-patch
    // preserves [llm]/[llm.embed]; only enable + scheduling are written here.
    return {
      enabled: form.enabled,
      min_user_messages: Number(form.minUserMessages) || 0,
      reenrich_msg_delta: Number(form.reenrichMsgDelta) || 0,
      reenrich_idle_minutes: Number(form.reenrichIdleMinutes) || 0,
      concurrency: Number(form.concurrency) || 0,
      periodic: form.periodic,
    };
  }
  async function saveEnrich() {
    if (!canSaveEnrich) return;
    savingEnrich = true;
    error = "";
    enrichMessage = "";
    try {
      applyConfig(await saveLLMConfig(enrichPayload()));
      enrichMessage = t("enrich.saved");
    } catch (err) {
      error = err instanceof ApiError ? err.message : "Failed to save LLM config";
    } finally {
      savingEnrich = false;
    }
  }

  // --- Testing ---
  async function runTest(id: string, req: LLMTestRequest, channel: "chat" | "embed") {
    if (!canTest) return;
    testingTarget = id;
    error = "";
    try {
      const resp = await testLLMConnection(req);
      testResults = { ...testResults, [id]: channel === "embed" ? resp.embed : resp.chat };
    } catch (err) {
      testResults = { ...testResults, [id]: { ok: false, message: err instanceof ApiError ? err.message : "test failed" } };
    } finally {
      testingTarget = null;
    }
  }
  function testProvider(p: Provider) {
    return runTest(
      `provider:${p.uid}`,
      { provider: p.name.trim(), channel: "chat", base_url: p.base_url.trim(), api_key: keyPayload(p.api_key), model: defaultChatModel(p.vendor) },
      "chat",
    );
  }
  function testUsage(usage: UsageKey) {
    const b = bindings[usage];
    const model = (b?.model ?? "").trim();
    if (usage === "embed") {
      return runTest(`usage:${usage}`, { usage, channel: "embed", embed: model ? { model } : undefined }, "embed");
    }
    return runTest(`usage:${usage}`, { usage, channel: "chat", model: model || undefined }, "chat");
  }
  function testLabel(r: LLMTestChannelResult | undefined): string {
    if (!r) return "";
    if (r.disabled) return r.message || "disabled";
    if (r.ok) return r.message && r.message !== "ok" ? `ok · ${r.message}` : "ok";
    return r.message || "error";
  }
  function testClass(r: LLMTestChannelResult | undefined): string {
    if (!r) return "";
    if (r.disabled) return "muted";
    return r.ok ? "ok" : "error";
  }

  // --- Enrichment job polling ---
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
        // Transient poll failure; keep the last known state.
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
      // No job yet.
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

  type CountCard = readonly [string, number];
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

  function usageModelSuggestions(usage: UsageKey): string[] {
    const p = providerByUid(bindings[usage]?.providerUid ?? null);
    if (!p) return [];
    const vendor = vendorById(p.vendor);
    return usage === "embed" ? vendor.embedModels : vendor.chatModels;
  }
</script>

{#snippet testButton(id: string, run: () => void)}
  <div class="test-cell">
    <button type="button" class="test-btn" data-testid={`test-${id}`} onclick={run} disabled={!canTest || testingTarget !== null}>
      {testingTarget === id ? t("common.testing") : t("common.test")}
    </button>
    {#if testResults[id]}
      <span class={`test-flag ${testClass(testResults[id])}`} data-testid={`test-result-${id}`}>{testLabel(testResults[id])}</span>
    {/if}
  </div>
{/snippet}

<SettingsSection title={t("enrich.title")} description={t("enrich.desc")}>
  {#if remote}
    <p class="muted" data-testid="llm-enrichment-remote">{unavailableReason}</p>
  {:else}
    <form class="config-form" onsubmit={(event) => { event.preventDefault(); saveProviders(); }}>
      <!-- Providers: vendor + key, multiple instances allowed. -->
      <div class="block">
        <div class="block-head">
          <h4>{t("providers.title")}</h4>
          <p class="block-hint">{t("providers.desc")}</p>
        </div>

        {#if providers.length === 0}
          <p class="empty" data-testid="providers-empty">{t("providers.empty")}</p>
        {:else}
          {#each providers as p (p.uid)}
            <div class="provider-card" data-testid={`provider-${p.name}`}>
              <div class="provider-fields">
                <label class="f-name">
                  <span>{t("providers.name")}</span>
                  <input type="text" bind:value={p.name} data-testid={`provider-name-${p.uid}`} />
                </label>
                <label class="f-vendor">
                  <span>{t("providers.vendor")}</span>
                  <select value={p.vendor} onchange={(e) => onVendorChange(p.uid, e.currentTarget.value)}>
                    {#each VENDORS as v}<option value={v.id}>{vendorLabel(v)}</option>{/each}
                  </select>
                </label>
                <label class="f-key">
                  <span>{t("provider.apiKey")}</span>
                  <input type="password" bind:value={p.api_key} autocomplete="off" />
                </label>
                {#if p.vendor === "custom"}
                  <label class="f-url">
                    <span>{t("provider.baseUrl")}</span>
                    <input type="url" bind:value={p.base_url} placeholder="https://host/v1" />
                  </label>
                {/if}
              </div>
              <div class="provider-actions">
                {@render testButton(`provider:${p.uid}`, () => testProvider(p))}
                <button type="button" class="ghost-btn" onclick={() => removeProvider(p.uid)}>{t("common.remove")}</button>
              </div>
            </div>
          {/each}
        {/if}

        <div class="add-row">
          <button type="button" class="ghost-btn" onclick={addProvider}>+ {t("providers.add")}</button>
        </div>
      </div>

      <!-- Usage assignment: provider + model per usage. -->
      <div class="block">
        <div class="block-head">
          <h4>{t("assign.title")}</h4>
          <p class="block-hint">{t("assign.desc")}</p>
        </div>
        <div class="usage-list" aria-label={t("assign.title")}>
          {#each USAGES as usage}
            <div class="usage-row" data-testid={`usage-${usage}`}>
              <div class="usage-meta">
                <span class="usage-name">{t(`usage.${usage}`)} <code>{usage}</code></span>
                <span class="usage-desc">{t(`usage.${usage}.desc`)}</span>
              </div>
              <div class="usage-control">
                <span class="lbl">{t("assign.use")}</span>
                {#if providers.length === 0}
                  <span class="muted">{t("assign.noProvider")}</span>
                {:else}
                  <select bind:value={bindings[usage].providerUid} aria-label={t(`usage.${usage}`)} data-testid={`usage-provider-${usage}`}>
                    {#each providers as p}<option value={p.uid}>{p.name}</option>{/each}
                  </select>
                  <span class="lbl">{t("provider.model")}</span>
                  <input
                    type="text"
                    class="model-input"
                    bind:value={bindings[usage].model}
                    list={`models-${usage}`}
                    data-testid={`usage-model-${usage}`}
                  />
                  <datalist id={`models-${usage}`}>
                    {#each usageModelSuggestions(usage) as m}<option value={m}></option>{/each}
                  </datalist>
                  {@render testButton(`usage:${usage}`, () => testUsage(usage))}
                {/if}
              </div>
            </div>
          {/each}
        </div>

        {#if usageWarnings.length}
          <div class="warning-list" role="alert">
            <p class="warning-head">{t("assign.dangling")}</p>
            {#each usageWarnings as w}<p>{w}</p>{/each}
          </div>
        {/if}
      </div>

      {#if configLoading}<p class="muted">{t("common.loading")}</p>{/if}
      {#if configMessage}<p class="result">{configMessage}</p>{/if}
      {#if error}<p class="error" role="alert">{error}</p>{/if}

      <div class="actions">
        <button class="trigger-btn" type="submit" disabled={!canSaveConfig}>
          {saving ? t("common.saving") : t("enrich.save")}
        </button>
      </div>
    </form>
  {/if}
</SettingsSection>

<SettingsSection title={t("feature.enrichTitle")} description={t("feature.enrichDesc")}>
  {#if remote}
    <p class="muted" data-testid="llm-enrichment-remote">{unavailableReason}</p>
  {:else}
    <form class="config-form" onsubmit={(event) => { event.preventDefault(); saveEnrich(); }}>
      <label class="toggle-row">
        <input name="enabled" type="checkbox" bind:checked={form.enabled} />
        <span>{t("enrich.enable")}</span>
      </label>

      <div class="block schedule-grid">
        <h4>{t("enrich.scheduling")}</h4>
        <label><span>{t("enrich.minMsgs")}</span><input name="min_user_messages" type="number" min="0" bind:value={form.minUserMessages} /></label>
        <label><span>{t("enrich.reenrichDelta")}</span><input name="reenrich_msg_delta" type="number" min="0" bind:value={form.reenrichMsgDelta} /></label>
        <label><span>{t("enrich.idleMinutes")}</span><input name="reenrich_idle_minutes" type="number" min="0" bind:value={form.reenrichIdleMinutes} /></label>
        <label><span>{t("enrich.concurrency")}</span><input name="concurrency" type="number" min="0" bind:value={form.concurrency} /></label>
        <label class="toggle-row periodic-toggle"><input name="periodic" type="checkbox" bind:checked={form.periodic} /><span>{t("enrich.runPeriodically")}</span></label>
      </div>

      {#if enrichMessage}<p class="result">{enrichMessage}</p>{/if}

      <div class="actions">
        <button class="trigger-btn" type="submit" disabled={!canSaveEnrich}>
          {savingEnrich ? t("common.saving") : t("feature.enrichSave")}
        </button>
      </div>
    </form>

    <div class="status-grid" aria-label="LLM enrichment status">
      {#each countCards(status) as [label, value]}
        <div class="status-card"><span class="status-value">{value}</span><span class="status-label">{label}</span></div>
      {/each}
    </div>

    {#if loading}<p class="muted">{t("common.loading")}</p>{/if}
    {#if unavailableReason}<p class="muted" data-testid="llm-enrichment-unavailable">{unavailableReason}</p>{/if}

    {#if job && (jobRunning || job.done_at)}
      <div class="enrich-progress" data-testid="enrich-progress">
        <div class="progress-track" role="progressbar" aria-valuemin="0" aria-valuemax="100" aria-valuenow={progressPct}>
          <div class="progress-fill" style="width: {progressPct}%"></div>
        </div>
        {#if jobRunning}
          <p class="muted" data-testid="enrich-progress-label">Enriching {job.processed} / {job.total} ({progressPct}%){job.source === "periodic" ? " - periodic" : ""}...</p>
        {:else}
          <p class="result" data-testid="enrich-progress-label">Done: {job.succeeded} enriched, {job.failed} failed{job.skipped ? `, ${job.skipped} skipped` : ""}{job.no_content ? `, ${job.no_content} no content` : ""}.</p>
          <p class="muted" data-testid="enrich-cost">
            Tokens: {(job.prompt_tokens + job.completion_tokens).toLocaleString()} chat
            ({job.prompt_tokens.toLocaleString()} in / {job.completion_tokens.toLocaleString()} out){job.embed_tokens ? `, ${job.embed_tokens.toLocaleString()} embed` : ""}.
            {#if job.cost_spent}
              Chat spend this run: {job.cost_currency} {job.cost_spent}{job.balance_end ? ` (balance now ${job.cost_currency} ${job.balance_end})` : ""}.
            {/if}
            {#if job.embed_cost_spent}
              Embed spend this run: {job.embed_cost_currency} {job.embed_cost_spent}{job.embed_balance_end ? ` (balance now ${job.embed_cost_currency} ${job.embed_balance_end})` : ""}.
            {/if}
          </p>
        {/if}
        {#if job.error}<p class="error" role="alert">{job.error}</p>{/if}
      </div>
    {/if}

    {#if error}<p class="error" role="alert">{error}</p>{/if}

    <div class="actions">
      {#if jobRunning}
        <button class="refresh-btn" onclick={stopEnrichment} disabled={!canStop}>{t("enrich.stop")}</button>
      {:else}
        <button class="trigger-btn" onclick={startEnrichment} disabled={!canTrigger}>{t("enrich.run")}</button>
      {/if}
      <button class="refresh-btn" onclick={loadStatus} disabled={loading}>{t("enrich.refresh")}</button>
    </div>
  {/if}
</SettingsSection>

<style>
  .config-form { display: flex; flex-direction: column; gap: 14px; }
  .block { display: flex; flex-direction: column; gap: 10px; }
  .block-head { display: grid; gap: 2px; }
  .block h4 { margin: 0; font-size: 12px; font-weight: 650; color: var(--text-secondary); }
  .block-hint { font-size: 12px; color: var(--text-muted); margin: 0; }

  .provider-card {
    display: flex; align-items: flex-end; gap: 10px; flex-wrap: wrap;
    padding: 10px 12px; border: 1px solid var(--border-muted);
    border-radius: var(--radius-sm); background: var(--bg-inset);
  }
  .provider-fields { display: flex; align-items: flex-end; gap: 8px; flex: 1 1 auto; flex-wrap: wrap; min-width: 0; }
  .provider-fields label { display: grid; gap: 3px; min-width: 0; }
  .provider-fields span, .usage-control .lbl, .toggle-row span { font-size: 11px; color: var(--text-muted); }
  .f-name { flex: 1 1 130px; }
  .f-vendor { flex: 0 0 130px; }
  .f-key { flex: 1 1 150px; }
  .f-url { flex: 1 1 180px; }
  .provider-actions { display: flex; align-items: center; gap: 8px; }

  input, select {
    min-width: 0; height: 30px; padding: 0 9px;
    border: 1px solid var(--border-muted); border-radius: var(--radius-sm);
    background: var(--bg-surface); color: var(--text-primary); font-size: 12px;
  }
  input:focus, select:focus { outline: none; border-color: var(--accent-blue); }

  .test-cell { display: flex; align-items: center; gap: 8px; }
  .test-btn {
    height: 26px; padding: 0 12px; border-radius: var(--radius-sm);
    border: 1px solid var(--border-muted); background: var(--bg-surface);
    color: var(--text-secondary); font-size: 11px; cursor: pointer;
  }
  .test-btn:disabled { opacity: 0.55; cursor: default; }
  .test-flag { font-size: 11px; line-height: 1.3; }
  .test-flag.ok { color: var(--accent-green, #16a34a); }
  .test-flag.error { color: var(--accent-red, #ef4444); }
  .test-flag.muted { color: var(--text-muted); }

  .add-row { display: flex; gap: 8px; }
  .ghost-btn {
    height: 28px; padding: 0 12px; border-radius: var(--radius-sm);
    border: 1px solid var(--border-muted); background: var(--bg-surface);
    color: var(--text-primary); font-size: 11px; cursor: pointer;
  }

  .usage-list { display: grid; gap: 8px; }
  .usage-row {
    display: flex; align-items: center; justify-content: space-between; gap: 12px;
    padding: 8px 10px; border: 1px solid var(--border-muted);
    border-radius: var(--radius-sm); background: var(--bg-inset);
  }
  .usage-meta { display: grid; gap: 2px; min-width: 0; }
  .usage-name { font-size: 13px; font-weight: 500; color: var(--text-primary); }
  .usage-name code { font-size: 11px; color: var(--text-muted); background: var(--bg-surface); padding: 1px 5px; border-radius: 4px; }
  .usage-desc { font-size: 12px; color: var(--text-muted); }
  .usage-control { display: flex; align-items: center; gap: 8px; flex: 0 0 auto; flex-wrap: wrap; justify-content: flex-end; }
  .usage-control select { flex: 0 0 130px; max-width: 130px; }
  .model-input { flex: 0 0 170px; max-width: 170px; }

  .empty {
    padding: 10px; border: 1px dashed var(--border-default); border-radius: var(--radius-sm);
    color: var(--text-muted); font-size: 12px; background: var(--bg-inset); margin: 0;
  }
  .warning-list {
    display: grid; gap: 4px; padding: 8px 10px;
    border: 1px solid var(--accent-amber, #f59e0b); border-radius: var(--radius-sm);
    background: color-mix(in srgb, var(--accent-amber, #f59e0b) 12%, transparent);
  }
  .warning-list p { margin: 0; }
  .warning-head { font-weight: 600; }

  .schedule-grid { padding: 12px; border: 1px solid var(--border-muted); border-radius: var(--radius-sm); background: var(--bg-inset); }
  .schedule-grid label { display: grid; grid-template-columns: minmax(110px, 0.42fr) minmax(0, 1fr); align-items: center; gap: 8px; min-width: 0; }

  .status-grid { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 8px; margin-top: 12px; }
  .status-card { min-width: 0; padding: 10px; border: 1px solid var(--border-muted); border-radius: var(--radius-sm); background: var(--bg-inset); }
  .status-value { display: block; font-size: 18px; font-weight: 650; color: var(--text-primary); line-height: 1.1; }
  .status-label { display: block; margin-top: 3px; font-size: 10px; color: var(--text-muted); white-space: nowrap; }

  .toggle-row { display: flex; align-items: center; gap: 8px; }
  .toggle-row input { margin: 0; }
  .periodic-toggle { grid-column: 1 / -1; }

  .muted, .result, .error { margin: 0; font-size: 12px; line-height: 1.5; }
  .muted { color: var(--text-muted); }
  .result { color: var(--text-secondary); }
  .error { color: var(--accent-red, #ef4444); }

  .enrich-progress { display: flex; flex-direction: column; gap: 4px; }
  .progress-track { width: 100%; height: 6px; border-radius: 999px; background: var(--bg-inset); border: 1px solid var(--border-muted); overflow: hidden; }
  .progress-fill { height: 100%; background: var(--accent-blue); transition: width 0.3s ease; }

  .actions { display: flex; flex-wrap: wrap; gap: 8px; }
  .trigger-btn, .refresh-btn { height: 28px; padding: 0 12px; border-radius: var(--radius-sm); font-size: 12px; font-weight: 500; border: 1px solid var(--border-muted); cursor: pointer; }
  .trigger-btn { color: white; background: var(--accent-blue); border-color: var(--accent-blue); }
  .refresh-btn { color: var(--text-secondary); background: var(--bg-inset); }
  .trigger-btn:disabled, .refresh-btn:disabled { opacity: 0.6; cursor: default; }

  @media (max-width: 549px) {
    .status-grid { grid-template-columns: repeat(2, minmax(0, 1fr)); }
    .provider-card, .usage-row { flex-direction: column; align-items: stretch; }
    .usage-control { justify-content: flex-start; }
    .schedule-grid label { grid-template-columns: 1fr; gap: 4px; }
  }
</style>
