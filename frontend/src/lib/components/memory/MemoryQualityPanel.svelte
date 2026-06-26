<script lang="ts">
  import { onMount } from "svelte";
  import { fetchMemoryQuality, type MemoryQualitySummary } from "../../api/memoryQuality";

  let loading = $state(true);
  let error = $state<string | null>(null);
  let quality = $state<MemoryQualitySummary | null>(null);
  let open = $state(false);

  async function load() {
    loading = true;
    error = null;
    try {
      quality = await fetchMemoryQuality(50);
    } catch (e) {
      error = e instanceof Error ? e.message : String(e);
    } finally {
      loading = false;
    }
  }

  onMount(load);

  function fmtPercent(n: number, d: number): string {
    if (!d) return "0%";
    return `${Math.round((n / d) * 100)}%`;
  }

  function costText(label?: { currency?: string; amount?: string }): string {
    if (!label || (!label.currency && !label.amount)) return "unavailable";
    return [label.currency, label.amount].filter(Boolean).join(" ");
  }

  function providerNames(map: Record<string, number>): string {
    const keys = Object.keys(map);
    return keys.length ? keys.join(", ") : "none";
  }

  function totalTokens(label?: { total_tokens?: number }): string {
    if (!label || label.total_tokens == null) return "unavailable";
    return String(label.total_tokens);
  }
</script>

<section class="memory-quality">
  <button class="toggle" data-testid="memory-quality-toggle" onclick={() => (open = !open)} aria-expanded={open}>
    <span class="chevron" class:open>▶</span>
    Memory 机制运行
    {#if quality}
      <span class="status">已加载</span>
    {/if}
  </button>

  {#if open}
    <div class="body">
      <p class="caveat">非零指标只证明埋点接通，不代表召回质量达标。</p>
      {#if loading}
        <p class="muted">加载中…</p>
      {:else if error}
        <p class="err">{error}</p>
      {:else if quality}
        <div class="cards">
          <article><h3>抽取</h3><p>{quality.extract.candidate_count} candidates / {quality.extract.sessions_scanned} sessions</p><p>write rate: {fmtPercent(quality.extract.written, quality.extract.candidate_count || 1)}</p></article>
          <article><h3>晋升</h3><p>ADD {quality.consolidate.add_count}, UPDATE {quality.consolidate.update_count}, SKIP {quality.consolidate.skip_count}</p><p>commit: {quality.consolidate.committed}, resync: {quality.consolidate.resynced}</p></article>
          <article><h3>召回</h3><p>hits {quality.telemetry.recall_hit_count}, fallback {quality.telemetry.fallback_count}</p><p>routes: {quality.telemetry.recall_count}</p></article>
          <article><h3>注入</h3><p>capsule route {quality.telemetry.injection_count}</p><p>capture written {quality.telemetry.capture_written}</p></article>
          <article><h3>LLM Usage</h3><p>extract calls {quality.extract.llm_call_count} / {quality.extract.llm_duration_ms}ms</p><p>consolidate calls {quality.consolidate.llm_call_count} / {quality.consolidate.llm_duration_ms}ms</p><p>tokens: {totalTokens(quality.extract.llm_usage)} / {totalTokens(quality.consolidate.llm_usage)}</p><p>providers: {providerNames(quality.extract.provider_usage)} | {providerNames(quality.consolidate.provider_usage)}</p></article>
          <article><h3>Cost</h3><p>extract {costText(quality.extract.llm_cost)}</p><p>consolidate {costText(quality.consolidate.llm_cost)}</p></article>
        </div>
        <div class="scores">
          <strong>Score distribution</strong>
          {#if quality.telemetry.scores.length === 0}
            <p class="muted">无分数</p>
          {:else}
            <p>{quality.telemetry.scores.map((s) => s.toFixed(2)).join(", ")}</p>
          {/if}
        </div>
      {:else}
        <p class="muted">尚无质量指标。</p>
      {/if}
    </div>
  {/if}
</section>

<style>
  .memory-quality { margin: 0.5rem 0 1rem; border: 1px solid var(--border-default); border-radius: 6px; }
  .toggle { display: flex; align-items: center; gap: 0.5rem; width: 100%; padding: 0.5rem 0.75rem; background: none; border: none; color: inherit; cursor: pointer; font: inherit; text-align: left; }
  .chevron { transition: transform 0.15s ease; font-size: 0.7em; }
  .chevron.open { transform: rotate(90deg); }
  .status { margin-left: auto; font-size: 0.8em; opacity: 0.7; }
  .body { padding: 0 0.75rem 0.75rem; }
  .caveat { margin: 0.25rem 0 0.75rem; padding: 0.4rem 0.5rem; border-left: 3px solid #e6a700; background: rgba(230, 167, 0, 0.08); }
  .cards { display: grid; grid-template-columns: repeat(auto-fit, minmax(180px, 1fr)); gap: 0.5rem; }
  .cards article { border: 1px solid var(--border-default); border-radius: 6px; padding: 0.5rem; }
  .cards h3 { margin: 0 0 0.25rem; font-size: 0.85rem; }
  .muted { opacity: 0.7; }
  .err { color: #e57373; }
  .scores { margin-top: 0.5rem; }
</style>
