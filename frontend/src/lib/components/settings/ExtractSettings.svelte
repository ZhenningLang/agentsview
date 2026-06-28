<script lang="ts">
  import { onMount } from "svelte";
  import { ApiError, isRemoteConnection } from "../../api/runtime.js";
  import { fetchExtractAudit, setExtractEnabled } from "../../api/extract";
  import SettingsSection from "./SettingsSection.svelte";
  import { t } from "../../i18n/index.svelte";

  let loading = $state(false);
  let saving = $state(false);
  let error = $state("");
  let message = $state("");
  let available = $state(true);
  let form = $state({ enabled: false });
  const remote = isRemoteConnection();
  const canEdit = $derived(!remote);

  async function load() {
    if (remote) return;
    loading = true;
    error = "";
    try {
      const audit = await fetchExtractAudit();
      form.enabled = audit.enabled;
      available = audit.available;
    } catch (err) {
      error = err instanceof ApiError ? err.message : t("extract.loadFailed");
    } finally {
      loading = false;
    }
  }

  async function toggleEnabled() {
    if (!canEdit || saving) return;
    saving = true;
    error = "";
    message = "";
    try {
      const result = await setExtractEnabled(!form.enabled);
      form.enabled = result.enabled;
      available = result.available;
      message = t("extract.stateSaved");
    } catch (err) {
      error = err instanceof ApiError ? err.message : t("extract.toggleFailed");
    } finally {
      saving = false;
    }
  }

  onMount(load);
</script>

<SettingsSection title={t("extract.title")} description={t("extract.desc")}>
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
        data-testid="extract-toggle"
      >
        {form.enabled ? t("common.enabled") : t("common.disabled")}
      </button>
    </div>

    <p class="hint">{t("extract.modelHint")}</p>
    {#if !available && form.enabled}
      <p class="hint" data-testid="extract-unavailable">{t("extract.unavailable")}</p>
    {/if}

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
