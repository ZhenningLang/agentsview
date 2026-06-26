<script lang="ts">
  import { onMount } from "svelte";
  import { ApiError, isRemoteConnection } from "../../api/runtime.js";
  import {
    fetchLLMProviders,
    saveLLMProviders,
    type LLMProvidersResponse,
    type LLMProvidersPayload,
    type LLMProviderConfigPayload,
  } from "../../api/llm";
  import SettingsSection from "./SettingsSection.svelte";
  import { t } from "../../i18n/index.svelte";

  type ProviderForm = {
    enabled: boolean;
    base_url: string;
    api_key: string;
    model: string;
    reasoning_effort: string;
    balance_url: string;
  };

  const keySentinel = "********";
  // Business usages, in display order. Labels/descriptions come from i18n.
  const usageOptions = ["enrich", "extract", "consolidate", "embed", "recall_rerank"];

  let loading = $state(false);
  let saving = $state(false);
  let error = $state("");
  let message = $state("");
  let providerForms = $state<Record<string, ProviderForm>>({});
  let usageBindings = $state<Record<string, string>>({});
  let originalUsageBindings = $state<Record<string, string>>({});
  let usageWarnings = $state<string[]>([]);
  let removedProviders = $state<Set<string>>(new Set());
  let newProviderName = $state("");
  const remote = isRemoteConnection();
  const canEdit = $derived(!remote);

  function maskedValue(hasKey: boolean, preview?: string): string {
    return hasKey ? `${keySentinel}${preview ?? ""}` : "";
  }

  function applyProvidersResponse(resp: LLMProvidersResponse) {
    providerForms = Object.fromEntries(
      Object.entries(resp.providers ?? {}).map(([name, provider]) => [
        name,
        {
          enabled: provider.enabled,
          base_url: provider.base_url ?? "",
          api_key: maskedValue(provider.has_api_key, provider.api_key_preview),
          model: provider.model ?? "",
          reasoning_effort: provider.reasoning_effort ?? "",
          balance_url: provider.balance_url ?? "",
        },
      ]),
    );
    usageBindings = { ...(resp.usage ?? {}) };
    originalUsageBindings = { ...(resp.usage ?? {}) };
    usageWarnings = [...(resp.usage_warnings ?? [])];
    removedProviders = new Set();
  }

  async function load() {
    if (remote) return;
    loading = true;
    error = "";
    try {
      applyProvidersResponse(await fetchLLMProviders());
    } catch (err) {
      error = err instanceof ApiError ? err.message : t("llm.loadFailed");
    } finally {
      loading = false;
    }
  }

  function buildPayload(): LLMProvidersPayload {
    const providers: Record<string, LLMProviderConfigPayload> = {};
    for (const [name, provider] of Object.entries(providerForms)) {
      if (removedProviders.has(name)) continue;
      providers[name] = {
        enabled: provider.enabled,
        base_url: provider.base_url.trim(),
        api_key: provider.api_key.trim().startsWith(keySentinel) ? keySentinel : provider.api_key.trim(),
        model: provider.model.trim(),
        reasoning_effort: provider.reasoning_effort.trim(),
        balance_url: provider.balance_url.trim(),
      };
    }
    // Usage map: send explicit "" for a previously-bound usage now set to
    // default, so the backend deletes the binding (merge semantics).
    const usage: Record<string, string> = {};
    for (const name of usageOptions) {
      const provider = (usageBindings[name] ?? "").trim();
      const wasBound = (originalUsageBindings[name] ?? "").trim() !== "";
      if (provider === "") {
        if (wasBound) usage[name] = "";
        continue;
      }
      if (!removedProviders.has(provider)) usage[name] = provider;
    }
    return { providers, usage, delete_providers: Array.from(removedProviders) };
  }

  async function save() {
    if (!canEdit || saving) return;
    saving = true;
    error = "";
    message = "";
    try {
      applyProvidersResponse(await saveLLMProviders(buildPayload()));
      message = t("llm.saved");
    } catch (err) {
      error = err instanceof ApiError ? err.message : t("llm.saveFailed");
    } finally {
      saving = false;
    }
  }

  function addProvider() {
    const name = newProviderName.trim();
    if (!name || providerForms[name]) return;
    providerForms = {
      ...providerForms,
      [name]: { enabled: true, base_url: "", api_key: "", model: "", reasoning_effort: "", balance_url: "" },
    };
    removedProviders = new Set([...removedProviders].filter((n) => n !== name));
    newProviderName = "";
  }

  function removeProvider(name: string) {
    removedProviders = new Set(removedProviders).add(name);
    const next = { ...usageBindings };
    for (const [usage, provider] of Object.entries(next)) {
      if (provider === name) delete next[usage];
    }
    usageBindings = next;
  }

  const visibleProviders = $derived(
    Object.entries(providerForms).filter(([name]) => !removedProviders.has(name)),
  );

  onMount(load);
</script>

<SettingsSection title={t("llm.title")} description={t("llm.desc")}>
  {#if remote}
    <p class="muted">{t("common.localOnly")}</p>
  {:else}
    <!-- Block 1: Provider pool (configure sources first) -->
    <div class="block">
      <div class="block-head">
        <h4>{t("llm.providerPool")}</h4>
        <p class="muted">{t("llm.providerPoolDesc")}</p>
      </div>

      {#if visibleProviders.length === 0}
        <p class="empty" data-testid="provider-empty">{t("llm.providerEmptyState")}</p>
      {:else}
        <div class="provider-list">
          {#each visibleProviders as [name, provider]}
            <div class="provider-card" data-testid={`provider-${name}`}>
              <div class="provider-head">
                <strong>{name}</strong>
                <button type="button" class="ghost-btn" onclick={() => removeProvider(name)}>
                  {t("common.remove")}
                </button>
              </div>
              <div class="provider-grid">
                <label><span>{t("provider.baseUrl")}</span><input type="url" bind:value={provider.base_url} /></label>
                <label><span>{t("provider.model")}</span><input type="text" bind:value={provider.model} /></label>
                <label><span>{t("provider.apiKey")}</span><input type="password" bind:value={provider.api_key} /></label>
                <label><span>{t("provider.reasoningEffort")}</span><input type="text" bind:value={provider.reasoning_effort} /></label>
                <label><span>{t("provider.balanceUrl")}</span><input type="url" bind:value={provider.balance_url} /></label>
                <label class="check"><input type="checkbox" bind:checked={provider.enabled} /><span>{t("provider.enabled")}</span></label>
              </div>
            </div>
          {/each}
        </div>
      {/if}

      <div class="add-row">
        <input bind:value={newProviderName} placeholder={t("llm.newProviderName")} />
        <button type="button" class="ghost-btn" onclick={addProvider} disabled={!newProviderName.trim()}>
          + {t("llm.addProvider")}
        </button>
      </div>
    </div>

    <!-- Block 2: Usage bindings (then bind each usage to a provider) -->
    <div class="block">
      <div class="block-head">
        <h4>{t("llm.usageBindings")}</h4>
        <p class="muted">{t("llm.usageBindingsDesc")}</p>
      </div>
      <div class="usage-list" aria-label={t("llm.usageBindings")}>
        {#each usageOptions as usage}
          <div class="usage-row" data-testid={`usage-${usage}`}>
            <div class="usage-meta">
              <span class="usage-name">{t(`usage.${usage}`)} <code>{usage}</code></span>
              <span class="usage-desc">{t(`usage.${usage}.desc`)}</span>
            </div>
            <select bind:value={usageBindings[usage]} aria-label={t(`usage.${usage}`)}>
              <option value="">{t("llm.defaultOption")}</option>
              {#each visibleProviders as [name]}
                <option value={name}>{name}</option>
              {/each}
            </select>
          </div>
        {/each}
      </div>
    </div>

    {#if loading}<p class="muted">{t("common.loading")}</p>{/if}
    {#if message}<p class="result">{message}</p>{/if}
    {#if error}<p class="error" role="alert">{error}</p>{/if}
    {#if usageWarnings.length}
      <div class="warning-list" role="alert">
        <p class="warning-head">{t("llm.dangling")}</p>
        {#each usageWarnings as warning}<p>{warning}</p>{/each}
      </div>
    {/if}

    <div class="actions">
      <button type="button" class="save-btn" onclick={save} disabled={!canEdit || saving}>
        {saving ? t("common.saving") : t("llm.saveConfig")}
      </button>
    </div>
  {/if}
</SettingsSection>

<style>
  .block {
    display: grid;
    gap: 10px;
    padding-bottom: 16px;
    margin-bottom: 16px;
    border-bottom: 1px solid var(--border-muted);
  }
  .block:last-of-type {
    border-bottom: none;
  }
  .block-head h4 {
    margin: 0;
    font-size: 14px;
    font-weight: 600;
    color: var(--text-primary);
  }
  .provider-list {
    display: grid;
    gap: 12px;
  }
  .provider-card {
    padding: 12px;
    border: 1px solid var(--border-muted);
    border-radius: var(--radius-sm);
    background: var(--bg-inset);
    display: grid;
    gap: 10px;
  }
  .provider-head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 8px;
  }
  .provider-grid {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: 8px;
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
  label {
    display: grid;
    gap: 4px;
    min-width: 0;
  }
  label > span {
    font-size: 12px;
    color: var(--text-secondary);
  }
  label.check {
    grid-auto-flow: column;
    align-items: center;
    justify-content: start;
    gap: 6px;
  }
  input,
  select {
    height: 30px;
    padding: 0 9px;
    border: 1px solid var(--border-muted);
    border-radius: var(--radius-sm);
    background: var(--bg-surface);
    color: var(--text-primary);
    min-width: 0;
  }
  .usage-row select {
    flex: 0 0 220px;
    max-width: 220px;
  }
  .add-row {
    display: flex;
    gap: 8px;
    align-items: center;
  }
  .add-row input {
    flex: 1 1 auto;
  }
  .empty {
    padding: 12px;
    border: 1px dashed var(--border-default);
    border-radius: var(--radius-sm);
    color: var(--text-muted);
    font-size: 13px;
    background: var(--bg-inset);
  }
  .save-btn,
  .ghost-btn {
    height: 30px;
    padding: 0 12px;
    border-radius: var(--radius-sm);
    border: 1px solid var(--border-muted);
    background: var(--bg-inset);
    color: var(--text-primary);
    cursor: pointer;
  }
  .save-btn {
    background: var(--accent-blue);
    border-color: var(--accent-blue);
    color: #fff;
  }
  .save-btn:disabled {
    opacity: 0.6;
    cursor: default;
  }
  .actions {
    display: flex;
    gap: 8px;
  }
  .muted {
    color: var(--text-muted);
    font-size: 13px;
    margin: 0;
  }
  .result {
    color: var(--text-secondary);
  }
  .error {
    color: var(--accent-red, #ef4444);
  }
  .warning-list {
    display: grid;
    gap: 4px;
    padding: 8px 10px;
    border: 1px solid var(--accent-amber, #f59e0b);
    border-radius: var(--radius-sm);
    color: var(--text-primary);
    background: color-mix(in srgb, var(--accent-amber, #f59e0b) 12%, transparent);
  }
  .warning-list p {
    margin: 0;
  }
  .warning-head {
    font-weight: 600;
  }
  @media (max-width: 549px) {
    .provider-grid {
      grid-template-columns: 1fr;
    }
    .usage-row {
      flex-direction: column;
      align-items: stretch;
    }
    .usage-row select {
      flex: 1 1 auto;
      max-width: none;
    }
  }
</style>
