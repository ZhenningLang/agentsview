<script lang="ts">
  import { onMount } from "svelte";
  import {
    fetchVaultRuns,
    fetchVaultRun,
    type VaultRun,
    type VaultRunDetail,
    type VaultPhase,
  } from "../../api/vault";

  let loading = $state(true);
  let error = $state<string | null>(null);
  let runs = $state<VaultRun[]>([]);

  // Facet filter over the run skill. "" = no filter.
  let skill = $state("");

  // Unfiltered catalog loaded once for the skill facet options, so the
  // dropdown stays stable regardless of the active filter.
  let allRuns = $state<VaultRun[]>([]);

  let reqSeq = 0;

  async function loadCatalog() {
    try {
      allRuns = await fetchVaultRuns({});
    } catch {
      // Non-fatal: facet dropdown falls back to whatever the filtered
      // result yields.
    }
  }

  async function load() {
    const seq = ++reqSeq;
    loading = true;
    error = null;
    try {
      const result = await fetchVaultRuns({
        skill: skill || undefined,
      });
      if (seq !== reqSeq) return;
      runs = result;
    } catch (e) {
      if (seq !== reqSeq) return;
      error = e instanceof Error ? e.message : String(e);
    } finally {
      if (seq === reqSeq) loading = false;
    }
  }

  onMount(async () => {
    await loadCatalog();
    await load();
  });

  function uniqueValues(key: keyof VaultRun): string[] {
    const set = new Set<string>();
    for (const r of allRuns) {
      const v = r[key];
      if (typeof v === "string" && v) set.add(v);
    }
    return [...set].sort((a, b) => a.localeCompare(b));
  }

  const skillOptions = $derived(uniqueValues("skill"));

  function clearFilters() {
    skill = "";
    load();
  }

  const hasFilters = $derived(!!skill);

  // Detail modal: fetch the full run (phases + metrics) by slug.
  let detail = $state<VaultRunDetail | null>(null);
  let detailLoading = $state(false);
  let detailError = $state<string | null>(null);

  async function openDetail(slug: string) {
    detailLoading = true;
    detailError = null;
    detail = null;
    try {
      detail = await fetchVaultRun(slug);
    } catch (e) {
      detailError = e instanceof Error ? e.message : String(e);
    } finally {
      detailLoading = false;
    }
  }

  function closeDetail() {
    detail = null;
    detailError = null;
    detailLoading = false;
  }

  // A run is dev-complete (single pass) when its skill says so, or when it
  // simply has no phases. dev-long-run is multi-phase. We treat "no phases"
  // as the single-pass case for graceful display.
  function isSinglePass(run: VaultRunDetail): boolean {
    if (run.skill === "dev-complete") return true;
    return run.phases.length === 0;
  }

  // Tri-state badge text/class for an optional boolean OK flag.
  function okBadge(ok: boolean | undefined): {
    text: string;
    cls: string;
  } {
    if (ok === undefined) return { text: "—", cls: "neutral" };
    return ok
      ? { text: "pass", cls: "ok" }
      : { text: "fail", cls: "bad" };
  }

  function acceptanceBadge(run: VaultRun): { text: string; cls: string } {
    if (run.acceptance_ok === undefined) {
      return { text: "—", cls: "neutral" };
    }
    return run.acceptance_ok
      ? { text: "accepted", cls: "ok" }
      : { text: "rejected", cls: "bad" };
  }

  // Per-phase verify state for the progress table.
  function phaseVerify(p: VaultPhase): { text: string; cls: string } {
    return okBadge(p.verify_ok);
  }

  function phaseStuck(p: VaultPhase): string | null {
    if (p.stuck_consecutive_fail === undefined) return null;
    const fp = p.stuck_fingerprint ? ` · ${p.stuck_fingerprint}` : "";
    return `${p.stuck_consecutive_fail} 连续失败${fp}`;
  }

  // Metric event display: pick a short, scannable label and a state class.
  function metricState(m: VaultRunDetail["metrics"][number]): {
    text: string;
    cls: string;
  } {
    if (m.ok !== undefined) return okBadge(m.ok);
    return { text: "", cls: "neutral" };
  }

  function goalSnippet(goal: string | undefined): string {
    if (!goal) return "";
    const trimmed = goal.trim().replace(/\s+/g, " ");
    return trimmed.length > 160 ? trimmed.slice(0, 160) + "…" : trimmed;
  }

  // Header rows shown in the detail modal, skipping empty values so the
  // table stays meaningful.
  function metaRows(r: VaultRunDetail): [string, string][] {
    const rows: [string, string][] = [
      ["slug", r.slug],
      ["skill", r.skill],
      ["state", r.state],
      ["branch", r.branch],
      ["goal", r.goal],
      ["repo_root", r.repo_root],
      ["workspace_path", r.workspace_path],
      ["source_path", r.source_path],
      [
        "acceptance",
        r.acceptance_ok === undefined
          ? ""
          : (r.acceptance_ok ? "ok" : "fail") +
            (r.acceptance_exit !== undefined
              ? ` (exit ${r.acceptance_exit})`
              : ""),
      ],
      ["synced_at", r.synced_at],
    ];
    return rows.filter(([, v]) => v !== "" && v !== undefined);
  }
</script>

<div class="vault-page">
  <header class="vault-header">
    <h1>Vault</h1>
    <p class="subtitle">
      dev-workflow 长任务运行档案（只读视图）：dev-long-run（多 phase）与
      dev-complete（单 pass）的 phase 进度、verify/acceptance/stuck 与 metrics 时间线。
    </p>
    <div class="controls">
      <select bind:value={skill} onchange={load} aria-label="skill 过滤">
        <option value="">skill: 全部</option>
        {#each skillOptions as opt (opt)}
          <option value={opt}>{opt}</option>
        {/each}
      </select>
      {#if hasFilters}
        <button class="clear" onclick={clearFilters}>清除</button>
      {/if}
      <button class="refresh" onclick={load} title="Reload" aria-label="刷新"
        >↻</button
      >
    </div>
  </header>

  {#if loading}
    <div class="state">加载中…</div>
  {:else if error}
    <div class="state error">加载失败：{error}</div>
  {:else if runs.length === 0}
    <div class="state">
      {#if hasFilters}
        没有匹配的运行。调整 skill 过滤。
      {:else}
        未发现 vault 运行。vault SSOT 为各仓库的
        <code>.long-loop/&lt;slug&gt;/</code>，同步后会出现在这里。
      {/if}
    </div>
  {:else}
    <div class="count">{runs.length} 条</div>
    <table class="grid">
      <thead>
        <tr>
          <th>slug</th>
          <th>skill</th>
          <th>state</th>
          <th>branch</th>
          <th>acceptance</th>
        </tr>
      </thead>
      <tbody>
        {#each runs as r (r.slug)}
          <tr class="clickable" onclick={() => openDetail(r.slug)}>
            <td class="slug">
              <div class="slug-main">{r.slug}</div>
              {#if r.goal}
                <div class="snippet">{goalSnippet(r.goal)}</div>
              {/if}
            </td>
            <td>
              {#if r.skill}
                <span class="badge facet">{r.skill}</span>
              {:else}—{/if}
            </td>
            <td>
              {#if r.state}
                <span class="badge state">{r.state}</span>
              {:else}—{/if}
            </td>
            <td class="nowrap mono">{r.branch || "—"}</td>
            <td>
              <span class="badge {acceptanceBadge(r).cls}"
                >{acceptanceBadge(r).text}</span
              >
            </td>
          </tr>
        {/each}
      </tbody>
    </table>
  {/if}
</div>

{#if detail || detailLoading || detailError}
  <div
    class="modal-backdrop"
    role="button"
    tabindex="0"
    onclick={closeDetail}
    onkeydown={(e) => e.key === "Escape" && closeDetail()}
  >
    <div
      class="modal"
      role="dialog"
      aria-modal="true"
      tabindex="-1"
      onclick={(e) => e.stopPropagation()}
      onkeydown={() => {}}
    >
      {#if detailLoading}
        <div class="state">加载中…</div>
      {:else if detailError}
        <div class="state error">加载失败：{detailError}</div>
        <button class="close-btn" onclick={closeDetail}>关闭</button>
      {:else if detail}
        <div class="modal-head">
          <div>
            <h2>{detail.slug}</h2>
            <div class="modal-meta">
              {#if detail.skill}{detail.skill}{/if}
              {#if detail.state}· {detail.state}{/if}
              {#if detail.branch}· <span class="mono">{detail.branch}</span
                >{/if}
              {#if isSinglePass(detail)}
                · <span class="tag">单 pass</span>
              {:else}
                · <span class="tag">{detail.phases.length} phase</span>
              {/if}
            </div>
          </div>
          <button class="close-btn" onclick={closeDetail} aria-label="关闭"
            >✕</button
          >
        </div>

        {#if detail.goal}
          <h4>Goal</h4>
          <p class="goal">{detail.goal}</p>
        {/if}

        <h4>元数据</h4>
        <table class="meta-grid">
          <tbody>
            {#each metaRows(detail) as [k, v] (k)}
              <tr>
                <td class="meta-key">{k}</td>
                <td class="meta-val">{v}</td>
              </tr>
            {/each}
          </tbody>
        </table>

        <h4>Phase 进度</h4>
        {#if isSinglePass(detail)}
          <div class="empty-block">
            单 pass 运行（{detail.skill || "dev-complete"}），无 phase 拆分。
            {#if detail.acceptance_ok !== undefined}
              {@const ab = acceptanceBadge(detail)}
              acceptance：<span class="badge {ab.cls}">{ab.text}</span>
              {#if detail.acceptance_exit !== undefined}
                <span class="dim">(exit {detail.acceptance_exit})</span>
              {/if}
            {/if}
          </div>
        {:else}
          <table class="phase-grid">
            <thead>
              <tr>
                <th>phase</th>
                <th>verify</th>
                <th>exit</th>
                <th>stuck</th>
              </tr>
            </thead>
            <tbody>
              {#each detail.phases as p (p.phase_id)}
                {@const pv = phaseVerify(p)}
                {@const stuck = phaseStuck(p)}
                <tr>
                  <td class="mono">{p.phase_id}</td>
                  <td><span class="badge {pv.cls}">{pv.text}</span></td>
                  <td class="nowrap mono">
                    {p.verify_exit ?? "—"}
                  </td>
                  <td>
                    {#if stuck}
                      <span class="badge bad">{stuck}</span>
                    {:else}—{/if}
                  </td>
                </tr>
              {/each}
            </tbody>
          </table>
        {/if}

        <h4>Metrics 时间线</h4>
        {#if detail.metrics.length === 0}
          <div class="empty-block">无 metrics 事件（单 pass 运行不产生 metrics）。</div>
        {:else}
          <ol class="timeline">
            {#each detail.metrics as m, i (i)}
              {@const ms = metricState(m)}
              <li class="tl-item">
                <span class="tl-ts mono">{m.ts}</span>
                <span class="tl-event">{m.event}</span>
                {#if m.phase}
                  <span class="badge facet">{m.phase}</span>
                {/if}
                {#if ms.text}
                  <span class="badge {ms.cls}">{ms.text}</span>
                {/if}
                {#if m.exit !== undefined}
                  <span class="dim mono">exit {m.exit}</span>
                {/if}
                {#if m.fingerprint}
                  <span class="dim mono">{m.fingerprint}</span>
                {/if}
              </li>
            {/each}
          </ol>
        {/if}

        <div class="modal-path">{detail.source_path || detail.slug}</div>
      {/if}
    </div>
  </div>
{/if}

<style>
  .vault-page {
    max-width: 960px;
    margin: 0 auto;
    padding: 1.5rem;
    color: var(--text-primary, #1a1a1a);
  }
  .vault-header h1 {
    margin: 0 0 0.25rem;
    font-size: 1.4rem;
  }
  .subtitle {
    margin: 0 0 1rem;
    color: var(--text-secondary, #666);
    font-size: 0.85rem;
  }
  .controls {
    display: flex;
    gap: 0.5rem;
    flex-wrap: wrap;
    align-items: center;
    margin-bottom: 1rem;
  }
  .controls select {
    padding: 0.35rem 0.5rem;
    font-size: 0.82rem;
    border: 1px solid var(--border, #ddd);
    border-radius: 6px;
    background: var(--bg, #fff);
    color: var(--text-primary, #1a1a1a);
  }
  .controls .clear,
  .controls .refresh {
    background: none;
    border: 1px solid var(--border, #ddd);
    border-radius: 6px;
    cursor: pointer;
    padding: 0.35rem 0.55rem;
    color: var(--text-secondary, #666);
    font-size: 0.82rem;
  }
  .controls .refresh {
    margin-left: auto;
  }
  .count {
    font-size: 0.78rem;
    color: var(--text-secondary, #666);
    margin-bottom: 0.4rem;
  }
  .state {
    padding: 2rem;
    text-align: center;
    color: var(--text-secondary, #666);
  }
  .state.error {
    color: #b91c1c;
  }
  table.grid {
    width: 100%;
    border-collapse: collapse;
    font-size: 0.82rem;
  }
  table.grid th,
  table.grid td {
    text-align: left;
    padding: 0.45rem 0.6rem;
    border-bottom: 1px solid var(--border, #eee);
    vertical-align: top;
  }
  td.slug {
    font-weight: 600;
  }
  .slug-main {
    color: var(--text-primary, #1a1a1a);
  }
  .snippet {
    font-weight: 400;
    font-size: 0.74rem;
    color: var(--text-secondary, #888);
    margin-top: 0.15rem;
  }
  td.nowrap {
    white-space: nowrap;
  }
  .mono {
    font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
    font-variant-numeric: tabular-nums;
  }
  .badge {
    display: inline-block;
    padding: 0.1rem 0.4rem;
    border-radius: 4px;
    font-size: 0.72rem;
  }
  .badge.facet {
    background: #e0e7ff;
    color: #3730a3;
  }
  .badge.state {
    background: #f3f4f6;
    color: #374151;
  }
  .badge.ok {
    background: #dcfce7;
    color: #166534;
  }
  .badge.bad {
    background: #fee2e2;
    color: #991b1b;
  }
  .badge.neutral {
    background: #f3f4f6;
    color: #6b7280;
  }
  .tag {
    display: inline-block;
    padding: 0.05rem 0.35rem;
    border-radius: 4px;
    font-size: 0.72rem;
    background: #ede9fe;
    color: #5b21b6;
  }
  .dim {
    color: var(--text-secondary, #888);
    font-size: 0.74rem;
  }
  tr.clickable {
    cursor: pointer;
  }
  tr.clickable:hover {
    background: var(--hover-bg, #f3f4f6);
  }
  code {
    font-size: 0.78em;
    background: var(--code-bg, #f3f4f6);
    padding: 0.05rem 0.25rem;
    border-radius: 3px;
  }
  .modal-backdrop {
    position: fixed;
    inset: 0;
    background: rgba(0, 0, 0, 0.4);
    display: flex;
    align-items: center;
    justify-content: center;
    z-index: 1000;
    padding: 1.5rem;
  }
  .modal {
    background: var(--bg, #fff);
    color: var(--text-primary, #1a1a1a);
    border-radius: 8px;
    max-width: 760px;
    width: 100%;
    max-height: 85vh;
    overflow-y: auto;
    padding: 1.25rem 1.5rem;
    box-shadow: 0 10px 40px rgba(0, 0, 0, 0.25);
  }
  .modal-head {
    display: flex;
    justify-content: space-between;
    align-items: flex-start;
    gap: 1rem;
  }
  .modal-head h2 {
    margin: 0;
    font-size: 1.2rem;
    word-break: break-all;
  }
  .modal-meta {
    font-size: 0.8rem;
    color: var(--text-secondary, #666);
    margin-top: 0.15rem;
    display: flex;
    gap: 0.35rem;
    flex-wrap: wrap;
    align-items: center;
  }
  .close-btn {
    background: none;
    border: 1px solid var(--border, #ddd);
    border-radius: 6px;
    cursor: pointer;
    padding: 0.2rem 0.5rem;
    color: var(--text-secondary, #666);
    flex-shrink: 0;
  }
  .modal h4 {
    margin: 0.9rem 0 0.4rem;
    font-size: 0.9rem;
  }
  .goal {
    margin: 0 0 0.5rem;
    font-size: 0.82rem;
    line-height: 1.5;
    color: var(--text-primary, #1a1a1a);
  }
  table.meta-grid {
    width: 100%;
    border-collapse: collapse;
    font-size: 0.78rem;
  }
  table.meta-grid td {
    padding: 0.3rem 0.5rem;
    border-bottom: 1px solid var(--border, #eee);
    vertical-align: top;
  }
  .meta-key {
    color: var(--text-secondary, #666);
    width: 9rem;
    white-space: nowrap;
  }
  .meta-val {
    word-break: break-word;
  }
  table.phase-grid {
    width: 100%;
    border-collapse: collapse;
    font-size: 0.8rem;
  }
  table.phase-grid th,
  table.phase-grid td {
    text-align: left;
    padding: 0.35rem 0.5rem;
    border-bottom: 1px solid var(--border, #eee);
    vertical-align: top;
  }
  .empty-block {
    font-size: 0.8rem;
    color: var(--text-secondary, #666);
    padding: 0.5rem 0;
  }
  ol.timeline {
    list-style: none;
    margin: 0;
    padding: 0;
    border-left: 2px solid var(--border, #e5e7eb);
  }
  .tl-item {
    display: flex;
    gap: 0.5rem;
    align-items: center;
    flex-wrap: wrap;
    padding: 0.3rem 0.6rem;
    font-size: 0.78rem;
    position: relative;
  }
  .tl-item::before {
    content: "";
    position: absolute;
    left: -5px;
    top: 0.6rem;
    width: 8px;
    height: 8px;
    border-radius: 50%;
    background: var(--border, #cbd5e1);
  }
  .tl-ts {
    color: var(--text-secondary, #888);
    font-size: 0.72rem;
    white-space: nowrap;
  }
  .tl-event {
    font-weight: 600;
  }
  .modal-path {
    margin-top: 0.5rem;
    font-size: 0.72rem;
    color: var(--text-secondary, #999);
    word-break: break-all;
  }
</style>
