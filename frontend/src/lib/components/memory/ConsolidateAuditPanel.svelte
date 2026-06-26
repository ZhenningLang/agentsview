<script lang="ts">
  import { onMount } from "svelte";
  import {
    fetchConsolidateAudit,
    setConsolidateEnabled,
    type ConsolidateRun,
    type ConsolidateDecision,
  } from "../../api/consolidate";

  let loading = $state(true);
  let error = $state<string | null>(null);
  let enabled = $state(false);
  // available is true when the backend exposes a runtime toggle (a running
  // worker controller). When false we fall back to the config/env hint.
  let available = $state(false);
  let toggling = $state(false);
  let runs = $state<ConsolidateRun[]>([]);
  let open = $state(false);

  async function load() {
    loading = true;
    error = null;
    try {
      const audit = await fetchConsolidateAudit(50);
      enabled = audit.enabled;
      available = audit.available;
      runs = audit.records ?? [];
    } catch (e) {
      error = e instanceof Error ? e.message : String(e);
    } finally {
      loading = false;
    }
  }

  // toggle arms/disarms the background worker. Enabling fires an immediate
  // server-side cycle, so after the request resolves we reload the audit to
  // surface that first run (locked decision A2: UI 能开启 + 开启后自动跑).
  async function toggle() {
    if (toggling) return;
    toggling = true;
    error = null;
    try {
      const res = await setConsolidateEnabled(!enabled);
      enabled = res.enabled;
      available = res.available;
      await load();
    } catch (e) {
      error = e instanceof Error ? e.message : String(e);
    } finally {
      toggling = false;
    }
  }

  onMount(load);

  // A decision is "rejected" when the safety script skipped it rather than
  // writing/updating. We surface these distinctly so the audit shows what was
  // kept out of memory and why.
  function isRejected(d: ConsolidateDecision): boolean {
    return (d.result ?? "").startsWith("skip ");
  }

  function decisionLabel(d: ConsolidateDecision): string {
    const action = d.action || "?";
    if (isRejected(d)) return `${action} → rejected`;
    return action;
  }

  function runSummary(r: ConsolidateRun): string {
    if (r.skipped) return r.note || r.error || "skipped";
    const wrote = (r.decisions ?? []).filter(
      (d) => !((d.result ?? "").startsWith("skip ")) && (d.result ?? "") !== "",
    ).length;
    const rejected = (r.decisions ?? []).filter(isRejected).length;
    const parts = [`${r.candidate_count} candidate(s)`, `${wrote} written`];
    if (rejected) parts.push(`${rejected} rejected`);
    if (r.committed) parts.push("committed");
    if (r.error) parts.push(`error: ${r.error}`);
    return parts.join(" · ");
  }
</script>

<section class="consolidate-audit">
  <button
    class="toggle"
    data-testid="consolidate-audit-toggle"
    onclick={() => {
      open = !open;
      if (open && runs.length === 0 && !loading) load();
    }}
    aria-expanded={open}
  >
    <span class="chevron" class:open>▶</span>
    巩固审计
    <span class="status" class:on={enabled}>{enabled ? "已开启" : "未开启"}</span>
  </button>

  {#if open}
    <div class="body">
      <div class="control">
        {#if available}
          <button
            class="enable-toggle"
            class:on={enabled}
            data-testid="consolidate-enable-toggle"
            onclick={toggle}
            disabled={toggling}
            role="switch"
            aria-checked={enabled}
          >
            <span class="knob"></span>
            <span class="enable-label">
              {toggling ? "处理中…" : enabled ? "已开启" : "开启自动巩固"}
            </span>
          </button>
          <span class="control-hint">
            {enabled
              ? "已开启：按周期自动巩固，开启时已立即跑一次。"
              : "开启后立即跑一次，并按周期自动把 staging 候选巩固进 memory/user。"}
          </span>
        {:else}
          <span class="control-hint">
            当前模式不支持页面开关（无可写 memory 目录）。请用配置
            AGENTSVIEW_CONSOLIDATE_ENABLED 开启。
          </span>
        {/if}
      </div>
      {#if loading}
        <p class="muted">加载中…</p>
      {:else if error}
        <p class="err">{error}</p>
      {:else if !enabled && runs.length === 0}
        <p class="muted">
          后台巩固默认关闭。开启后将按周期自动把 staging 候选巩固进
          memory/user。
        </p>
      {:else if runs.length === 0}
        <p class="muted">尚无巩固记录。</p>
      {:else}
        <ul class="runs">
          {#each runs as r (r.started_at)}
            <li class="run">
              <div class="run-head">
                <time>{r.started_at}</time>
                <span class="run-summary">{runSummary(r)}</span>
              </div>
              {#if r.decisions && r.decisions.length}
                <ul class="decisions">
                  {#each r.decisions as d (d.candidate_id)}
                    <li class="decision" class:rejected={isRejected(d)}>
                      <code>{d.candidate_id}</code>
                      <span class="action">{decisionLabel(d)}</span>
                      {#if d.result}<span class="result">{d.result}</span>{/if}
                    </li>
                  {/each}
                </ul>
              {/if}
              {#if r.script_errors && r.script_errors.length}
                <ul class="script-errors">
                  {#each r.script_errors as se}
                    <li>{se}</li>
                  {/each}
                </ul>
              {/if}
            </li>
          {/each}
        </ul>
      {/if}
    </div>
  {/if}
</section>

<style>
  .consolidate-audit {
    margin: 0.5rem 0 1rem;
    border: 1px solid var(--border-default);
    border-radius: 6px;
  }
  .toggle {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    width: 100%;
    padding: 0.5rem 0.75rem;
    background: none;
    border: none;
    color: inherit;
    cursor: pointer;
    font: inherit;
    text-align: left;
  }
  .chevron {
    transition: transform 0.15s ease;
    font-size: 0.7em;
  }
  .chevron.open {
    transform: rotate(90deg);
  }
  .status {
    margin-left: auto;
    font-size: 0.8em;
    opacity: 0.7;
  }
  .status.on {
    color: #4caf50;
    opacity: 1;
  }
  .body {
    padding: 0 0.75rem 0.75rem;
  }
  .control {
    display: flex;
    align-items: center;
    gap: 0.6rem;
    flex-wrap: wrap;
    padding: 0.5rem 0 0.6rem;
    border-bottom: 1px solid var(--border-default);
    margin-bottom: 0.5rem;
  }
  .enable-toggle {
    display: inline-flex;
    align-items: center;
    gap: 0.45rem;
    padding: 0.3rem 0.6rem 0.3rem 0.35rem;
    border: 1px solid var(--border-default);
    border-radius: 999px;
    background: none;
    color: inherit;
    cursor: pointer;
    font: inherit;
    font-size: 0.85em;
  }
  .enable-toggle:disabled {
    opacity: 0.6;
    cursor: progress;
  }
  .enable-toggle .knob {
    width: 0.85rem;
    height: 0.85rem;
    border-radius: 50%;
    background: #888;
    transition: background 0.15s ease;
  }
  .enable-toggle.on {
    border-color: #4caf50;
  }
  .enable-toggle.on .knob {
    background: #4caf50;
  }
  .enable-label {
    line-height: 1;
  }
  .control-hint {
    font-size: 0.8em;
    opacity: 0.7;
  }
  .muted {
    opacity: 0.7;
    font-size: 0.9em;
  }
  .err {
    color: #e57373;
  }
  .runs,
  .decisions,
  .script-errors {
    list-style: none;
    margin: 0;
    padding: 0;
  }
  .run {
    padding: 0.4rem 0;
    border-top: 1px solid var(--border-default);
  }
  .run-head {
    display: flex;
    gap: 0.75rem;
    align-items: baseline;
    flex-wrap: wrap;
  }
  .run-head time {
    font-size: 0.8em;
    opacity: 0.7;
  }
  .run-summary {
    font-size: 0.85em;
  }
  .decisions {
    margin-top: 0.25rem;
    padding-left: 0.5rem;
  }
  .decision {
    display: flex;
    gap: 0.5rem;
    align-items: baseline;
    font-size: 0.82em;
    padding: 0.1rem 0;
  }
  .decision.rejected .action {
    color: #e57373;
  }
  .decision .result {
    opacity: 0.6;
  }
  .script-errors {
    margin-top: 0.25rem;
    color: #e57373;
    font-size: 0.8em;
  }
</style>
