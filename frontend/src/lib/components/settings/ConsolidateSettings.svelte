<script lang="ts">
  import { onMount } from "svelte";
  import { ApiError, isRemoteConnection } from "../../api/runtime.js";
  import {
    fetchLLMProviders,
    fetchConsolidateConfig,
    saveLLMProviders,
    saveConsolidateConfig,
    type ConsolidateConfigResponse,
    type LLMProvidersResponse,
    type LLMProvidersPayload,
    type LLMProviderConfigPayload,
  } from "../../api/llm";
  import { setConsolidateEnabled } from "../../api/consolidate";
  import SettingsSection from "./SettingsSection.svelte";

  type ProviderForm = {
    enabled: boolean;
    base_url: string;
    api_key: string;
    model: string;
    reasoning_effort: string;
    balance_url: string;
  };

  const keySentinel = "********";
  const usageOptions = ["enrich", "extract", "consolidate", "embed", "recall_rerank"];
  const usageLabels: Record<string, string> = {
    enrich: "enrich",
    extract: "extract",
    consolidate: "consolidate",
    embed: "embed",
    recall_rerank: "recall_rerank",
  };

  let loading = $state(false);
  let saving = $state(false);
  let error = $state("");
  let message = $state("");
  let consolidateState: ConsolidateConfigResponse | null = $state(null);
  let providerForms = $state<Record<string, ProviderForm>>({});
  let usageBindings = $state<Record<string, string>>({});
  let originalUsageBindings = $state<Record<string, string>>({});
  let usageWarnings = $state<string[]>([]);
  let removedProviders = $state<Set<string>>(new Set());
  let newProviderName = $state("");
  let newProviderUsage = $state("consolidate");
  let consolidateForm = $state({ enabled: false, interval: "24h" });
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

  function normalizeInterval(value: string): string {
    return value.trim().replace(/^(\d+)h0m0s$/, "$1h");
  }

  async function load() {
    if (remote) return;
    loading = true;
    error = "";
    try {
      applyProvidersResponse(await fetchLLMProviders());
      consolidateState = await fetchConsolidateConfig();
      consolidateForm.enabled = consolidateState.enabled;
      consolidateForm.interval = normalizeInterval(consolidateState.interval);
    } catch (err) {
      error = err instanceof ApiError ? err.message : "Failed to load consolidate settings";
    } finally {
      loading = false;
    }
  }

  function saveProvidersPayload(): LLMProvidersPayload {
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
    const usage: Record<string, string> = {};
    for (const name of usageOptions) {
      const provider = (usageBindings[name] ?? "").trim();
      const wasBound = (originalUsageBindings[name] ?? "").trim() !== "";
      if (provider === "") {
        if (wasBound) {
          usage[name] = "";
        }
        continue;
      }
      if (!removedProviders.has(provider)) {
        usage[name] = provider;
      }
    }
    return {
      providers,
      usage,
      delete_providers: Array.from(removedProviders),
    };
  }

  async function save() {
    if (!canEdit || saving) return;
    saving = true;
    error = "";
    message = "";
    try {
      consolidateState = await saveConsolidateConfig({ interval: normalizeInterval(consolidateForm.interval) });
      consolidateForm.interval = normalizeInterval(consolidateState.interval);
      applyProvidersResponse(await saveLLMProviders(saveProvidersPayload()));
      message = "Consolidate settings saved";
    } catch (err) {
      error = err instanceof ApiError ? err.message : "Failed to save consolidate settings";
    } finally {
      saving = false;
    }
  }

  async function toggleEnabled() {
    if (!canEdit || saving) return;
    saving = true;
    error = "";
    message = "";
    try {
      const result = await setConsolidateEnabled(!consolidateForm.enabled);
      consolidateForm.enabled = result.enabled;
      consolidateState = {
        ...(consolidateState ?? { interval: consolidateForm.interval }),
        enabled: result.enabled,
      };
      message = "Consolidate worker state saved";
    } catch (err) {
      error = err instanceof ApiError ? err.message : "Failed to toggle consolidate worker";
    } finally {
      saving = false;
    }
  }

  function addProvider() {
    const name = newProviderName.trim();
    if (!name || providerForms[name]) return;
    providerForms = {
      ...providerForms,
      [name]: {
        enabled: true,
        base_url: "",
        api_key: "",
        model: "",
        reasoning_effort: "",
        balance_url: "",
      },
    };
    usageBindings = { ...usageBindings, [newProviderUsage]: name };
    newProviderName = "";
  }

  function removeProvider(name: string) {
    const next = new Set(removedProviders);
    next.add(name);
    removedProviders = next;
    for (const [usage, provider] of Object.entries(usageBindings)) {
      if (provider === name) {
        const nextUsage = { ...usageBindings };
        delete nextUsage[usage];
        usageBindings = nextUsage;
      }
    }
  }

  function visibleProviderEntries(): [string, ProviderForm][] {
    return Object.entries(providerForms).filter(([name]) => !removedProviders.has(name));
  }

  onMount(load);
</script>

<SettingsSection
  title="Consolidate"
  description="Expose the background memory consolidation worker, its provider bindings, and interval."
>
  {#if remote}
    <p class="muted">Consolidate settings are available only from the local server connection.</p>
  {:else}
    <div class="field-row">
      <button
        type="button"
        class="toggle-btn"
        class:on={consolidateForm.enabled}
        onclick={toggleEnabled}
        disabled={!canEdit || saving}
        aria-pressed={consolidateForm.enabled}
      >
        {consolidateForm.enabled ? "Enabled" : "Disabled"}
      </button>
      <label>
        <span>Interval</span>
        <input type="text" bind:value={consolidateForm.interval} placeholder="24h" />
      </label>
    </div>

    <div class="usage-grid" aria-label="LLM usage bindings">
      {#each usageOptions as usage}
        <label>
          <span>{usageLabels[usage]}</span>
          <select bind:value={usageBindings[usage]}>
            <option value="">legacy fallback</option>
            {#each visibleProviderEntries() as [name]}
              <option value={name}>{name}</option>
            {/each}
          </select>
        </label>
      {/each}
    </div>

    <div class="provider-toolbar">
      <input bind:value={newProviderName} placeholder="provider name" />
      <select bind:value={newProviderUsage}>
        {#each usageOptions as usage}
          <option value={usage}>{usageLabels[usage]}</option>
        {/each}
      </select>
      <button type="button" onclick={addProvider} disabled={!newProviderName.trim()}>Add provider</button>
    </div>

    <div class="provider-list">
      {#each visibleProviderEntries() as [name, provider]}
        <div class="provider-card" data-testid={`provider-${name}`}>
          <div class="provider-head">
            <strong>{name}</strong>
            <button type="button" onclick={() => removeProvider(name)}>Remove</button>
          </div>
          <label><span>Enabled</span><input type="checkbox" bind:checked={provider.enabled} /></label>
          <label><span>Base URL</span><input type="url" bind:value={provider.base_url} /></label>
          <label><span>API key</span><input type="password" bind:value={provider.api_key} /></label>
          <label><span>Model</span><input type="text" bind:value={provider.model} /></label>
          <label><span>Reasoning effort</span><input type="text" bind:value={provider.reasoning_effort} /></label>
          <label><span>Balance URL</span><input type="url" bind:value={provider.balance_url} /></label>
        </div>
      {/each}
    </div>

    {#if loading}
      <p class="muted">Loading consolidate settings...</p>
    {/if}
    {#if message}
      <p class="result">{message}</p>
    {/if}
    {#if error}
      <p class="error" role="alert">{error}</p>
    {/if}
    {#if usageWarnings.length}
      <div class="warning-list" role="alert" aria-label="LLM usage warnings">
        {#each usageWarnings as warning}
          <p>{warning}</p>
        {/each}
      </div>
    {/if}

    <div class="actions">
      <button type="button" class="save-btn" onclick={save} disabled={!canEdit || saving}>
        {saving ? "Saving..." : "Save consolidate settings"}
      </button>
    </div>
  {/if}
</SettingsSection>

<style>
  .field-row,
  .provider-toolbar,
  .provider-head,
  .actions {
    display: flex;
    flex-wrap: wrap;
    gap: 8px;
    align-items: center;
  }

  .usage-grid {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: 8px;
  }

  .field-row label,
  .provider-card label,
  .usage-grid label,
  .provider-toolbar input,
  .provider-toolbar select {
    min-width: 0;
  }

  .field-row label,
  .provider-card label,
  .usage-grid label {
    display: grid;
    gap: 4px;
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
    gap: 8px;
  }

  .provider-card input,
  .usage-grid select,
  .provider-toolbar input,
  .provider-toolbar select,
  .field-row input {
    height: 30px;
    padding: 0 9px;
    border: 1px solid var(--border-muted);
    border-radius: var(--radius-sm);
    background: var(--bg-surface);
    color: var(--text-primary);
  }

  .save-btn,
  .toggle-btn,
  .provider-toolbar button,
  .provider-head button {
    height: 30px;
    padding: 0 12px;
    border-radius: var(--radius-sm);
    border: 1px solid var(--border-muted);
    background: var(--bg-inset);
    color: var(--text-primary);
  }

  .toggle-btn.on {
    color: white;
    background: var(--accent-blue);
    border-color: var(--accent-blue);
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

  .warning-list {
    display: grid;
    gap: 4px;
    padding: 8px 10px;
    border: 1px solid var(--accent-yellow, #f59e0b);
    border-radius: var(--radius-sm);
    color: var(--text-primary);
    background: color-mix(in srgb, var(--accent-yellow, #f59e0b) 12%, transparent);
  }

  .warning-list p {
    margin: 0;
  }

  @media (max-width: 549px) {
    .usage-grid {
      grid-template-columns: 1fr;
    }
  }
</style>
