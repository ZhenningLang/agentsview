<script lang="ts">
  import { onMount } from "svelte";
  import { ApiError, isRemoteConnection } from "../../api/runtime.js";
  import {
    fetchEnrichStatus,
    triggerEnrich,
    type LLMEnrichResponse,
    type LLMEnrichmentStatusReport,
  } from "../../api/llm.js";
  import { sync } from "../../stores/sync.svelte.js";
  import SettingsSection from "./SettingsSection.svelte";

  let status: LLMEnrichmentStatusReport | null = $state(null);
  let result: LLMEnrichResponse | null = $state(null);
  let loading = $state(false);
  let running = $state(false);
  let error = $state("");
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
  }
</style>
