<script lang="ts">
  import { onMount } from "svelte";
  import { ApiError, isRemoteConnection } from "../../api/runtime.js";
  import {
    fetchConsolidateConfig,
    saveConsolidateConfig,
    type ConsolidateConfigResponse,
  } from "../../api/llm";
  import { setConsolidateEnabled } from "../../api/consolidate";
  import SettingsSection from "./SettingsSection.svelte";
  import { t } from "../../i18n/index.svelte";

  let loading = $state(false);
  let saving = $state(false);
  let error = $state("");
  let message = $state("");
  let consolidateState: ConsolidateConfigResponse | null = $state(null);
  let form = $state({ enabled: false, interval: "24h" });
  const remote = isRemoteConnection();
  const canEdit = $derived(!remote);

  function normalizeInterval(value: string): string {
    return value.trim().replace(/^(\d+)h0m0s$/, "$1h");
  }

  // Mirror the backend (time.ParseDuration + must be > 0): a sequence of
  // <number><unit> tokens whose total is positive. Returns total ms, or null
  // when malformed, so the UI can validate before hitting the server.
  const DURATION_UNIT_MS: Record<string, number> = {
    ns: 1e-6, us: 1e-3, "µs": 1e-3, ms: 1, s: 1e3, m: 6e4, h: 3.6e6,
  };
  function parseGoDuration(value: string): number | null {
    const str = value.trim();
    if (!/^(\d+(\.\d+)?(ns|us|µs|ms|s|m|h))+$/.test(str)) return null;
    let total = 0;
    for (const match of str.matchAll(/(\d+(?:\.\d+)?)(ns|us|µs|ms|s|m|h)/g)) {
      total += parseFloat(match[1]!) * (DURATION_UNIT_MS[match[2]!] ?? 0);
    }
    return total;
  }
  const intervalValid = $derived.by(() => {
    const ms = parseGoDuration(form.interval);
    return ms != null && ms > 0;
  });

  async function load() {
    if (remote) return;
    loading = true;
    error = "";
    try {
      consolidateState = await fetchConsolidateConfig();
      form.enabled = consolidateState.enabled;
      form.interval = normalizeInterval(consolidateState.interval);
    } catch (err) {
      error = err instanceof ApiError ? err.message : t("consolidate.saveFailed");
    } finally {
      loading = false;
    }
  }

  async function saveInterval() {
    if (!canEdit || saving || !intervalValid) return;
    saving = true;
    error = "";
    message = "";
    try {
      consolidateState = await saveConsolidateConfig({ interval: normalizeInterval(form.interval) });
      form.interval = normalizeInterval(consolidateState.interval);
      message = t("consolidate.intervalSaved");
    } catch (err) {
      error = err instanceof ApiError ? err.message : t("consolidate.saveFailed");
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
      const result = await setConsolidateEnabled(!form.enabled);
      form.enabled = result.enabled;
      consolidateState = { ...(consolidateState ?? { interval: form.interval }), enabled: result.enabled };
      message = t("consolidate.stateSaved");
    } catch (err) {
      error = err instanceof ApiError ? err.message : t("consolidate.toggleFailed");
    } finally {
      saving = false;
    }
  }

  onMount(load);
</script>

<SettingsSection title={t("consolidate.title")} description={t("consolidate.desc")}>
  {#if remote}
    <p class="muted">{t("common.localOnly")}</p>
  {:else}
    <div class="field-row">
      <button
        type="button"
        class="toggle-btn"
        class:on={form.enabled}
        onclick={toggleEnabled}
        disabled={!canEdit || saving}
        aria-pressed={form.enabled}
      >
        {form.enabled ? t("common.enabled") : t("common.disabled")}
      </button>
      <label>
        <span>{t("consolidate.interval")}</span>
        <input
          type="text"
          bind:value={form.interval}
          placeholder="24h"
          aria-invalid={!intervalValid}
          class:invalid={!intervalValid}
        />
      </label>
      <button type="button" class="save-btn" onclick={saveInterval} disabled={!canEdit || saving || !intervalValid}>
        {saving ? t("common.saving") : t("common.save")}
      </button>
    </div>

    {#if !intervalValid}
      <p class="error" role="alert" data-testid="interval-invalid">{t("consolidate.intervalInvalid")}</p>
    {/if}

    <p class="hint">{t("consolidate.modelHint")}</p>

    {#if loading}<p class="muted">{t("common.loading")}</p>{/if}
    {#if message}<p class="result">{message}</p>{/if}
    {#if error}<p class="error" role="alert">{error}</p>{/if}
  {/if}
</SettingsSection>

<style>
  .field-row {
    display: flex;
    flex-wrap: wrap;
    gap: 12px;
    align-items: end;
  }
  .field-row label {
    display: grid;
    gap: 4px;
    min-width: 0;
  }
  .field-row label > span {
    font-size: 12px;
    color: var(--text-secondary);
  }
  input {
    height: 30px;
    padding: 0 9px;
    border: 1px solid var(--border-muted);
    border-radius: var(--radius-sm);
    background: var(--bg-surface);
    color: var(--text-primary);
  }
  input.invalid {
    border-color: var(--accent-red, #ef4444);
  }
  .save-btn,
  .toggle-btn {
    height: 30px;
    padding: 0 12px;
    border-radius: var(--radius-sm);
    border: 1px solid var(--border-muted);
    background: var(--bg-inset);
    color: var(--text-primary);
    cursor: pointer;
  }
  .toggle-btn.on {
    color: #fff;
    background: var(--accent-blue);
    border-color: var(--accent-blue);
  }
  .hint {
    font-size: 12px;
    color: var(--text-muted);
    margin: 10px 0 0;
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
</style>
