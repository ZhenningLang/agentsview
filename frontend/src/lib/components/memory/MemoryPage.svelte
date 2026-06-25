<script lang="ts">
  import { onMount } from "svelte";
  import {
    fetchMemories,
    fetchMemory,
    type Memory,
  } from "../../api/memory";

  type SortKey = "title" | "date" | "problem_type";

  let loading = $state(true);
  let error = $state<string | null>(null);
  let memories = $state<Memory[]>([]);

  // Full-text query (server-side FTS over the body). Empty = list all.
  let query = $state("");
  // Facet filters over frontmatter fields. "" = no filter.
  let problemType = $state("");
  let type = $state("");
  let status = $state("");

  // The body is only fetched on demand for the listing's facet options, but
  // the list endpoint already returns every row's frontmatter, so facet
  // option sets are derived from the unfiltered catalog loaded once.
  let allMemories = $state<Memory[]>([]);

  let reqSeq = 0;

  async function loadCatalog() {
    try {
      allMemories = await fetchMemories({});
    } catch {
      // Non-fatal: facet dropdowns just fall back to whatever the filtered
      // result yields.
    }
  }

  async function load() {
    const seq = ++reqSeq;
    loading = true;
    error = null;
    try {
      const result = await fetchMemories({
        q: query.trim() || undefined,
        problem_type: problemType || undefined,
        type: type || undefined,
        status: status || undefined,
      });
      if (seq !== reqSeq) return;
      memories = result;
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

  // Re-query whenever a facet or the search box changes. $effect tracks the
  // reactive reads below; load() is debounced lightly for the text query.
  let debounce: ReturnType<typeof setTimeout> | null = null;
  function scheduleLoad() {
    if (debounce) clearTimeout(debounce);
    debounce = setTimeout(load, 200);
  }

  function uniqueValues(key: keyof Memory): string[] {
    const set = new Set<string>();
    for (const m of allMemories) {
      const v = m[key];
      if (typeof v === "string" && v) set.add(v);
    }
    return [...set].sort((a, b) => a.localeCompare(b));
  }

  const problemTypeOptions = $derived(uniqueValues("problem_type"));
  const typeOptions = $derived(uniqueValues("type"));
  const statusOptions = $derived(uniqueValues("status"));

  // Client-side sort over the server-filtered rows.
  let sortKey = $state<SortKey>("date");
  let sortDir = $state<"asc" | "desc">("desc");

  function toggleSort(key: SortKey) {
    if (sortKey === key) {
      sortDir = sortDir === "asc" ? "desc" : "asc";
    } else {
      sortKey = key;
      sortDir = key === "date" ? "desc" : "asc";
    }
  }

  function sortIndicator(key: SortKey): string {
    if (sortKey !== key) return "";
    return sortDir === "asc" ? " ▲" : " ▼";
  }

  const sortedMemories = $derived(
    [...memories].sort((a, b) => {
      const av = a[sortKey] ?? "";
      const bv = b[sortKey] ?? "";
      const cmp = String(av).localeCompare(String(bv));
      return sortDir === "asc" ? cmp : -cmp;
    }),
  );

  function clearFilters() {
    query = "";
    problemType = "";
    type = "";
    status = "";
    load();
  }

  const hasFilters = $derived(
    !!(query.trim() || problemType || type || status),
  );

  // Detail modal: fetch the full note (body included) by rel_path.
  let detail = $state<Memory | null>(null);
  let detailLoading = $state(false);
  let detailError = $state<string | null>(null);

  async function openDetail(relPath: string) {
    detailLoading = true;
    detailError = null;
    detail = null;
    try {
      detail = await fetchMemory(relPath);
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

  // Frontmatter rows shown in the detail modal, skipping the body and empty
  // values so the table stays meaningful.
  function frontmatterRows(m: Memory): [string, string][] {
    const rows: [string, string][] = [
      ["rel_path", m.rel_path],
      ["title", m.title],
      ["date", m.date],
      ["problem_type", m.problem_type],
      ["type", m.type],
      ["status", m.status],
      ["origin_session", m.origin_session],
      ["body_tokens", String(m.body_tokens)],
      ["synced_at", m.synced_at],
    ];
    return rows.filter(([, v]) => v !== "" && v !== undefined);
  }

  function bodySnippet(body: string | undefined): string {
    if (!body) return "";
    const trimmed = body.trim().replace(/\s+/g, " ");
    return trimmed.length > 160 ? trimmed.slice(0, 160) + "…" : trimmed;
  }
</script>

<div class="memory-page">
  <header class="memory-header">
    <h1>Memory</h1>
    <p class="subtitle">
      跨 agent user-memory 笔记（只读视图）：全文检索、按 frontmatter facet 过滤、查看正文与元数据。
    </p>
    <div class="controls">
      <input
        class="search"
        type="search"
        placeholder="全文搜索正文…"
        bind:value={query}
        oninput={scheduleLoad}
      />
      <select bind:value={problemType} onchange={load} aria-label="problem_type 过滤">
        <option value="">problem_type: 全部</option>
        {#each problemTypeOptions as opt (opt)}
          <option value={opt}>{opt}</option>
        {/each}
      </select>
      <select bind:value={type} onchange={load} aria-label="type 过滤">
        <option value="">type: 全部</option>
        {#each typeOptions as opt (opt)}
          <option value={opt}>{opt}</option>
        {/each}
      </select>
      <select bind:value={status} onchange={load} aria-label="status 过滤">
        <option value="">status: 全部</option>
        {#each statusOptions as opt (opt)}
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
  {:else if memories.length === 0}
    <div class="state">
      {#if hasFilters}
        没有匹配的笔记。调整搜索词或 facet 过滤。
      {:else}
        未发现 memory 笔记。memory SSOT 为
        <code>~/.dotfiles/memory/user/*.md</code>（local-only），同步后会出现在这里。
      {/if}
    </div>
  {:else}
    <div class="count">{memories.length} 条</div>
    <table class="grid">
      <thead>
        <tr>
          <th class="sortable-th" onclick={() => toggleSort("title")}
            >标题{sortIndicator("title")}</th
          >
          <th class="sortable-th" onclick={() => toggleSort("date")}
            >日期{sortIndicator("date")}</th
          >
          <th class="sortable-th" onclick={() => toggleSort("problem_type")}
            >problem_type{sortIndicator("problem_type")}</th
          >
          <th>type</th>
          <th>status</th>
        </tr>
      </thead>
      <tbody>
        {#each sortedMemories as m (m.rel_path)}
          <tr class="clickable" onclick={() => openDetail(m.rel_path)}>
            <td class="title">
              <div class="title-main">{m.title || m.rel_path}</div>
              {#if m.body}
                <div class="snippet">{bodySnippet(m.body)}</div>
              {/if}
            </td>
            <td class="nowrap">{m.date || "—"}</td>
            <td>
              {#if m.problem_type}
                <span class="badge facet">{m.problem_type}</span>
              {:else}—{/if}
            </td>
            <td>
              {#if m.type}
                <span class="badge facet">{m.type}</span>
              {:else}—{/if}
            </td>
            <td>
              {#if m.status}
                <span class="badge facet">{m.status}</span>
              {:else}—{/if}
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
            <h2>{detail.title || detail.rel_path}</h2>
            <div class="modal-meta">
              {detail.date || "(no date)"}
              {#if detail.problem_type}· {detail.problem_type}{/if}
              {#if detail.type}· {detail.type}{/if}
              {#if detail.status}· {detail.status}{/if}
            </div>
          </div>
          <button class="close-btn" onclick={closeDetail} aria-label="关闭"
            >✕</button
          >
        </div>
        <h4>Frontmatter</h4>
        <table class="fm-grid">
          <tbody>
            {#each frontmatterRows(detail) as [k, v] (k)}
              <tr>
                <td class="fm-key">{k}</td>
                <td class="fm-val">{v}</td>
              </tr>
            {/each}
          </tbody>
        </table>
        <h4>正文</h4>
        <pre class="body">{detail.body || "(无正文)"}</pre>
        <div class="modal-path">{detail.rel_path}</div>
      {/if}
    </div>
  </div>
{/if}

<style>
  .memory-page {
    max-width: 960px;
    margin: 0 auto;
    padding: 1.5rem;
    color: var(--text-primary, #1a1a1a);
  }
  .memory-header h1 {
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
  .controls .search {
    flex: 1 1 14rem;
    min-width: 10rem;
    padding: 0.35rem 0.6rem;
    font-size: 0.85rem;
    border: 1px solid var(--border, #ddd);
    border-radius: 6px;
    background: var(--bg, #fff);
    color: var(--text-primary, #1a1a1a);
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
  td.title {
    font-weight: 600;
  }
  .title-main {
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
  .sortable-th {
    cursor: pointer;
    user-select: none;
    white-space: nowrap;
  }
  .sortable-th:hover {
    color: var(--text-primary, #1a1a1a);
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
  }
  .modal-meta {
    font-size: 0.8rem;
    color: var(--text-secondary, #666);
    margin-top: 0.15rem;
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
  table.fm-grid {
    width: 100%;
    border-collapse: collapse;
    font-size: 0.78rem;
  }
  table.fm-grid td {
    padding: 0.3rem 0.5rem;
    border-bottom: 1px solid var(--border, #eee);
    vertical-align: top;
  }
  .fm-key {
    color: var(--text-secondary, #666);
    width: 9rem;
    white-space: nowrap;
  }
  .fm-val {
    word-break: break-word;
  }
  pre.body {
    background: var(--hover-bg, #f6f8fa);
    color: var(--text-primary, #1a1a1a);
    border: 1px solid var(--border, #e5e7eb);
    border-radius: 6px;
    padding: 0.75rem;
    font-size: 0.78rem;
    line-height: 1.45;
    white-space: pre-wrap;
    word-break: break-word;
    max-height: 50vh;
    overflow-y: auto;
  }
  .modal-path {
    margin-top: 0.5rem;
    font-size: 0.72rem;
    color: var(--text-secondary, #999);
    word-break: break-all;
  }
</style>
