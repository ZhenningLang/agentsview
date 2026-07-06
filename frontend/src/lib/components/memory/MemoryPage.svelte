<script lang="ts">
  import { onMount } from "svelte";
  import {
    fetchMemories,
    fetchMemory,
    fetchMemoryRaw,
    putMemory,
    deleteMemory,
    fetchMemoryHistory,
    fetchMemoryAtCommit,
    revertMemory,
    type Memory,
    type MemoryHistoryEntry,
  } from "../../api/memory";
  import { ApiError } from "../../api/runtime";

  type SortKey = "title" | "date" | "problem_type";
  type TierFilter = "" | "atomic" | "topic";
  type LifecycleFilter = "" | "folded";
  type CanonicalCoveredRef = { source: string; rel_path: string };

  let loading = $state(true);
  let error = $state<string | null>(null);
  let memories = $state<Memory[]>([]);

  // Full-text query (server-side FTS over the body). Empty = list all.
  let query = $state("");
  // New long-term memories live in the explicit assist-mem ledger. Legacy
  // sources remain queryable from the dropdown for migration/debug only.
  let source = $state("assist-mem");
  // Facet filters over frontmatter fields. "" = no filter.
  let problemType = $state("");
  let type = $state("");
  let status = $state("");
  // Project facet. "" = all; GENERAL sentinel = the General bucket (notes with
  // an empty origin_project: user-global or cross-project); else a project name.
  // Project filtering/grouping is done client-side over the loaded rows so the
  // empty-string "General" bucket needs no API sentinel.
  const GENERAL = "__general__";
  let projectFilter = $state("");
  let tierFilter = $state<TierFilter>("");
  let lifecycleFilter = $state<LifecycleFilter>("");
  let groupByProject = $state(false);

  // Human-readable label for a note's project ("" = General bucket).
  function projectLabel(p: string): string {
    return p ? p : "通用";
  }

  // Human-readable label for a memory's data source.
  function sourceLabel(s: string): string {
    if (s === "assist-mem") return "Assist Mem";
    if (s === "cc-native") return "CC 原生";
    if (s === "cross-agent") return "跨 agent";
    if (s === "canonical") return "Canonical generated";
    return s || "—";
  }

  function isCanonical(m: Memory | null | undefined): boolean {
    return m?.source === "canonical";
  }

  function tierOf(m: Memory): "topic" | "atomic" {
    return m.origin_session?.startsWith("compact-memory:") ? "topic" : "atomic";
  }

  function tierLabel(m: Memory): string {
    return tierOf(m) === "topic" ? "主题" : "原子";
  }

  function isActive(m: Memory): boolean {
    return (m.status || "active") === "active";
  }

  function isFolded(m: Memory): boolean {
    return m.status === "stale" || m.status === "archived";
  }

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
        source: source || undefined,
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
  // Non-empty project names for the facet; the General bucket is offered
  // separately when any note has an empty origin_project.
  const projectOptions = $derived(uniqueValues("origin_project"));
  const hasGeneral = $derived(allMemories.some((m) => !m.origin_project));
  const assistMemCount = $derived(
    allMemories.filter((m) => m.source === "assist-mem" && isActive(m)).length,
  );
  const canonicalCount = $derived(
    allMemories.filter((m) => m.source === "canonical" && isActive(m)).length,
  );
  const allSourceCount = $derived(allMemories.length);

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

  // Apply the project facet client-side (handles the empty-string General
  // bucket without an API sentinel).
  const visibleMemories = $derived(
    sortedMemories.filter((m) => {
      if (projectFilter === "") return true;
      if (projectFilter === GENERAL) return !m.origin_project;
      if (m.origin_project !== projectFilter) return false;
      return true;
    }).filter((m) => {
      if (tierFilter && tierOf(m) !== tierFilter) return false;
      if (lifecycleFilter === "folded" && !isFolded(m)) return false;
      return true;
    }),
  );

  // Group visible rows by project for the "按项目分组" view. Named projects come
  // first (alpha), the General bucket last. Returns [label, project, rows].
  const groupedMemories = $derived.by(() => {
    const groups = new Map<string, Memory[]>();
    for (const m of visibleMemories) {
      const key = m.origin_project || "";
      const list = groups.get(key);
      if (list) list.push(m);
      else groups.set(key, [m]);
    }
    return [...groups.entries()]
      .sort((a, b) => {
        if (a[0] === "") return 1;
        if (b[0] === "") return -1;
        return a[0].localeCompare(b[0]);
      })
      .map(([project, rows]) => ({ project, label: projectLabel(project), rows }));
  });

  function clearFilters() {
    query = "";
    source = "assist-mem";
    problemType = "";
    type = "";
    status = "";
    projectFilter = "";
    tierFilter = "";
    lifecycleFilter = "";
    load();
  }

  function showAssistMem() {
    source = "assist-mem";
    status = "active";
    tierFilter = "";
    lifecycleFilter = "";
    load();
  }

  function showCanonical() {
    source = "canonical";
    status = "active";
    tierFilter = "";
    lifecycleFilter = "";
    load();
  }

  function showAllSources() {
    status = "";
    source = "";
    tierFilter = "";
    lifecycleFilter = "";
    load();
  }

  const hasFilters = $derived(
    !!(
      query.trim() ||
      source ||
      problemType ||
      type ||
      status ||
      projectFilter ||
      tierFilter ||
      lifecycleFilter
    ),
  );

  // Detail modal: fetch the full note (body included) by rel_path.
  let detail = $state<Memory | null>(null);
  let detailLoading = $state(false);
  let detailError = $state<string | null>(null);

  // CC-native notes live in scattered ~/.claude/projects dirs with no git repo,
  // and assist-mem rows are synthetic views over a JSONL ledger entry. History
  // does not apply to either source.
  let detailIsCCNative = $derived(detail?.source === "cc-native");
  let detailIsAssistMem = $derived(detail?.source === "assist-mem");
  let detailIsCanonical = $derived(isCanonical(detail));
  let detailCanEdit = $derived(!detailIsCanonical);
  let detailHistoryUnsupported = $derived(
    detailIsCCNative || detailIsAssistMem || detailIsCanonical,
  );

  // The rel_path whose detail modal is open, kept separately so edit/history
  // actions have the key even while detail is being refetched.
  let activePath = $state<string | null>(null);

  async function openDetail(relPath: string) {
    activePath = relPath;
    detailLoading = true;
    detailError = null;
    detail = null;
    resetEdit();
    resetHistory();
    try {
      const loaded = await fetchMemory(relPath);
      detail = loaded;
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
    activePath = null;
    resetEdit();
    resetHistory();
  }

  // ── Edit mode ─────────────────────────────────────────────────────────
  // The editor works on the verbatim on-disk file (frontmatter + body) so it
  // round-trips untracked frontmatter keys and uses a base_sha that matches
  // the backend's optimistic-concurrency gate. base_sha is captured when the
  // edit form loads; a stale base yields a 409 we surface (never drop).
  let editing = $state(false);
  let editContent = $state("");
  let editBaseSha = $state("");
  let editLoading = $state(false);
  let editSaving = $state(false);
  let editError = $state<string | null>(null);
  let editConflict = $state(false);
  let deleteLoading = $state(false);
  let deleteError = $state<string | null>(null);

  function resetEdit() {
    editing = false;
    editContent = "";
    editBaseSha = "";
    editLoading = false;
    editSaving = false;
    editError = null;
    editConflict = false;
    deleteLoading = false;
    deleteError = null;
  }

  async function startEdit() {
    if (!activePath) return;
    editing = true;
    editLoading = true;
    editError = null;
    editConflict = false;
    try {
      const raw = await fetchMemoryRaw(activePath);
      editContent = raw.content;
      editBaseSha = raw.sha;
    } catch (e) {
      editError = e instanceof Error ? e.message : String(e);
      editing = false;
    } finally {
      editLoading = false;
    }
  }

  function cancelEdit() {
    resetEdit();
  }

  async function saveEdit() {
    if (!activePath) return;
    editSaving = true;
    editError = null;
    editConflict = false;
    try {
      await putMemory(activePath, editContent, editBaseSha);
      // Saved: leave edit mode, refresh detail + list so the new content and
      // any frontmatter changes are reflected.
      const path = activePath;
      resetEdit();
      await openDetail(path);
      await load();
      await loadCatalog();
    } catch (e) {
      if (e instanceof ApiError && e.status === 409) {
        editConflict = true;
        editError = "已被磁盘上的改动修改，请重载后再编辑。";
      } else {
        editError = e instanceof Error ? e.message : String(e);
      }
    } finally {
      editSaving = false;
    }
  }

  // Reload the on-disk content into the editor after a 409, picking up the
  // current disk state and a fresh base_sha so the next save can succeed.
  async function reloadForEdit() {
    if (!activePath) return;
    editConflict = false;
    await startEdit();
  }

  async function deleteDetail() {
    if (!activePath || !detail || !detailCanEdit) return;
    if (!confirm(`确认删除 ${detail.title || activePath}?\nAssist Mem 会归档 ledger entry；文件来源会删除对应文件。`)) {
      return;
    }
    deleteLoading = true;
    deleteError = null;
    try {
      const raw = await fetchMemoryRaw(activePath);
      await deleteMemory(activePath, raw.sha);
      closeDetail();
      await load();
      await loadCatalog();
    } catch (e) {
      deleteError = e instanceof Error ? e.message : String(e);
    } finally {
      deleteLoading = false;
    }
  }

  // ── History ───────────────────────────────────────────────────────────
  let historyOpen = $state(false);
  let historyLoading = $state(false);
  let historyError = $state<string | null>(null);
  let history = $state<MemoryHistoryEntry[]>([]);
  // The commit whose content is being inspected, with a simple line diff
  // against the current note body.
  let viewedCommit = $state<string | null>(null);
  let commitContent = $state("");
  let commitLoading = $state(false);
  let commitError = $state<string | null>(null);
  let reverting = $state(false);

  function resetHistory() {
    historyOpen = false;
    historyLoading = false;
    historyError = null;
    history = [];
    viewedCommit = null;
    commitContent = "";
    commitLoading = false;
    commitError = null;
    reverting = false;
  }

  async function toggleHistory() {
    if (historyOpen) {
      historyOpen = false;
      return;
    }
    historyOpen = true;
    if (history.length === 0 && !historyError) {
      await loadHistory();
    }
  }

  async function loadHistory() {
    if (!activePath) return;
    historyLoading = true;
    historyError = null;
    try {
      history = await fetchMemoryHistory(activePath);
    } catch (e) {
      historyError = e instanceof Error ? e.message : String(e);
    } finally {
      historyLoading = false;
    }
  }

  async function viewCommit(commit: string) {
    if (!activePath) return;
    if (viewedCommit === commit) {
      viewedCommit = null;
      commitContent = "";
      return;
    }
    viewedCommit = commit;
    commitLoading = true;
    commitError = null;
    commitContent = "";
    try {
      commitContent = await fetchMemoryAtCommit(activePath, commit);
    } catch (e) {
      commitError = e instanceof Error ? e.message : String(e);
    } finally {
      commitLoading = false;
    }
  }

  // A minimal line-level diff between the current note body and a past
  // commit's content: each line tagged context / added / removed. This is a
  // display aid, not a real LCS diff — it walks both line lists in parallel.
  type DiffLine = { kind: "ctx" | "add" | "del"; text: string };

  function lineDiff(oldText: string, newText: string): DiffLine[] {
    const oldLines = oldText.split("\n");
    const newLines = newText.split("\n");
    const out: DiffLine[] = [];
    const max = Math.max(oldLines.length, newLines.length);
    for (let i = 0; i < max; i++) {
      const o = oldLines[i];
      const n = newLines[i];
      if (o === n) {
        if (o !== undefined) out.push({ kind: "ctx", text: o });
      } else {
        if (o !== undefined) out.push({ kind: "del", text: o });
        if (n !== undefined) out.push({ kind: "add", text: n });
      }
    }
    return out;
  }

  // Diff the viewed commit (old) against the current on-disk content. We use
  // the raw current file when available (edit base) else the parsed body.
  const commitDiff = $derived<DiffLine[]>(
    viewedCommit && !commitLoading && !commitError
      ? lineDiff(commitContent, editBaseSha ? editContent : (detail?.body ?? ""))
      : [],
  );

  async function doRevert(commit: string) {
    if (!activePath) return;
    if (
      !confirm(
        "确认回退到该 commit?\n会把当前文件内容覆盖为该版本并生成一次新提交。",
      )
    ) {
      return;
    }
    reverting = true;
    try {
      // base_sha guards against a concurrent on-disk change. Fetch the current
      // on-disk sha right before reverting so the gate is against live state.
      const raw = await fetchMemoryRaw(activePath);
      await revertMemory(activePath, commit, raw.sha);
      const path = activePath;
      resetHistory();
      resetEdit();
      await openDetail(path);
      await load();
      await loadCatalog();
    } catch (e) {
      if (e instanceof ApiError && e.status === 409) {
        historyError =
          "已被磁盘上的改动修改，请重载后再回退。";
      } else {
        historyError = e instanceof Error ? e.message : String(e);
      }
    } finally {
      reverting = false;
    }
  }

  function shortCommit(c: string): string {
    return c.slice(0, 8);
  }

  // Frontmatter rows shown in the detail modal, skipping the body and empty
  // values so the table stays meaningful.
  function frontmatterRows(m: Memory): [string, string][] {
    const rows: [string, string][] = [
      ["rel_path", m.rel_path],
      ["source", m.source],
      ["title", m.title],
      ["date", m.date],
      ["problem_type", m.problem_type],
      ["type", m.type],
      ["status", m.status],
      ["origin_project", m.origin_project || "通用"],
      ["origin_session", m.origin_session],
      ["body_tokens", String(m.body_tokens)],
      ["synced_at", m.synced_at],
    ];
    return rows.filter(([, v]) => v !== "" && v !== undefined);
  }

  function parseCanonicalCoveredRefs(m: Memory | null | undefined): CanonicalCoveredRef[] {
    if (!m?.canonical_covered_refs?.trim()) return [];
    try {
      const parsed = JSON.parse(m.canonical_covered_refs) as unknown;
      if (!Array.isArray(parsed)) return [];
      return parsed.flatMap((item) => {
        if (!item || typeof item !== "object") return [];
        const ref = item as Record<string, unknown>;
        const refSource = typeof ref.source === "string" ? ref.source.trim() : "";
        const relPath = typeof ref.rel_path === "string" ? ref.rel_path.trim() : "";
        if (!refSource || !relPath) return [];
        return [{ source: refSource, rel_path: relPath }];
      });
    } catch {
      return [];
    }
  }

  function canonicalCoverageCount(m: Memory): number {
    return parseCanonicalCoveredRefs(m).length;
  }

  function canonicalProvenanceRows(m: Memory | null | undefined): string[] {
    if (!m?.canonical_provenance?.trim()) return [];
    try {
      const parsed = JSON.parse(m.canonical_provenance) as unknown;
      if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) {
        return [m.canonical_provenance];
      }
      const obj = parsed as Record<string, unknown>;
      const preferredKeys = ["topic", "sources", "version", "cluster_key"];
      const extraKeys = Object.keys(obj)
        .filter((key) => !preferredKeys.includes(key))
        .sort((a, b) => a.localeCompare(b));
      return [...preferredKeys, ...extraKeys].flatMap((key) => {
        const value = obj[key];
        if (value === undefined || value === null || value === "") return [];
        const rendered =
          typeof value === "object" ? JSON.stringify(value) : String(value);
        return [`${key}: ${Array.isArray(value) ? value.join(", ") : rendered}`];
      });
    } catch {
      return [m.canonical_provenance];
    }
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
      显式长期记忆 ledger：默认只展示通过 /assist-mem 写入的 memory。
    </p>
    <section class="pipeline-card" aria-label="Assist Mem 概览">
      <div class="pipeline-head">
        <div>
          <div class="eyebrow">Assist Mem</div>
          <h2>Explicit Ledger Only</h2>
        </div>
        <div class="pipeline-note">以后新增 memory 只走 /assist-mem，不再走自动候选、Evidence 或 Knowledge 合成。</div>
      </div>
      <div class="pipeline-steps">
        <article class="knowledge-step">
          <span class="step-label">Ledger</span>
          <strong>{assistMemCount}</strong>
          <p>{assistMemCount} active assist-mem entries</p>
          <button type="button" onclick={showAssistMem}>看 assist-mem</button>
        </article>
        <article class="canonical-step">
          <span class="step-label">Canonical</span>
          <strong>{canonicalCount}</strong>
          <p>generated current-memory rows</p>
          <button type="button" onclick={showCanonical}>看 canonical</button>
        </article>
        <article class="legacy-step">
          <span class="step-label">All sources</span>
          <strong>{allSourceCount}</strong>
          <p>全部 raw/canonical 来源用于核对</p>
          <button type="button" onclick={showAllSources}>看全部来源</button>
        </article>
      </div>
    </section>
    <div class="controls">
      <input
        class="search"
        type="search"
        placeholder="全文搜索正文…"
        bind:value={query}
        oninput={scheduleLoad}
      />
      <select bind:value={source} onchange={load} aria-label="source 过滤">
        <option value="">来源: 全部</option>
        <option value="assist-mem">Assist Mem</option>
        <option value="canonical">Canonical generated</option>
        <option value="cross-agent">跨 agent</option>
        <option value="cc-native">CC 原生</option>
      </select>
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
      <select bind:value={projectFilter} aria-label="项目过滤">
        <option value="">项目: 全部</option>
        {#if hasGeneral}
          <option value={GENERAL}>通用</option>
        {/if}
        {#each projectOptions as opt (opt)}
          <option value={opt}>{opt}</option>
        {/each}
      </select>
      <select bind:value={tierFilter} aria-label="层级过滤">
        <option value="">层级: 全部</option>
        <option value="atomic">原子</option>
        <option value="topic">主题</option>
      </select>
      <label class="group-toggle" title="按项目分组显示">
        <input type="checkbox" bind:checked={groupByProject} />
        按项目分组
      </label>
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
  {:else if visibleMemories.length === 0}
    <div class="state">
      {#if hasFilters}
        没有匹配的笔记。调整搜索词或 facet 过滤。
      {:else}
        未发现 memory 笔记。memory SSOT 为
        <code>~/.dotfiles/memory/user/*.md</code>（local-only），同步后会出现在这里。
      {/if}
    </div>
  {:else}
    <div class="count">{visibleMemories.length} 条</div>
    {#if lifecycleFilter === "folded"}
      <div class="active-filter-note">当前显示 stale 或 archived 的已折叠来源。</div>
    {/if}
    {#if groupByProject}
      {#each groupedMemories as g (g.project)}
        <div class="project-group">
          <h3 class="project-head">
            <span class="badge project" class:general={!g.project}>{g.label}</span>
            <span class="project-count">{g.rows.length} 条</span>
          </h3>
          <div class="table-scroll">
            {@render memTable(g.rows)}
          </div>
        </div>
      {/each}
    {:else}
      <div class="table-scroll">
        {@render memTable(visibleMemories)}
      </div>
    {/if}
  {/if}
</div>

{#snippet memTable(list: Memory[])}
  <table class="grid">
    <thead>
      <tr>
        <th class="sortable-th" onclick={() => toggleSort("title")}
          >标题{sortIndicator("title")}</th
        >
        <th class="sortable-th" onclick={() => toggleSort("date")}
          >日期{sortIndicator("date")}</th
        >
        <th>来源</th>
        <th>项目</th>
        <th>层级</th>
        <th class="sortable-th" onclick={() => toggleSort("problem_type")}
          >problem_type{sortIndicator("problem_type")}</th
        >
        <th>type</th>
        <th>status</th>
      </tr>
    </thead>
    <tbody>
      {#each list as m (m.rel_path)}
        <tr class="clickable" onclick={() => openDetail(m.rel_path)}>
          <td class="title">
            <div class="title-main">{m.title || m.rel_path}</div>
            {#if isCanonical(m)}
              <div class="canonical-subtext">
                coverage {canonicalCoverageCount(m)}
              </div>
            {/if}
            {#if m.body}
              <div class="snippet">{bodySnippet(m.body)}</div>
            {/if}
          </td>
          <td class="nowrap">{m.date || "—"}</td>
          <td>
            <span class="badge source source-{m.source}"
              >{sourceLabel(m.source)}</span
            >
          </td>
          <td>
            <span class="badge project" class:general={!m.origin_project}
              >{projectLabel(m.origin_project)}</span
            >
          </td>
          <td>
            <span class="badge tier" class:topic={tierOf(m) === "topic"}>{tierLabel(m)}</span>
          </td>
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
{/snippet}

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
              {#if detailIsCanonical}· Canonical generated{/if}
            </div>
          </div>
          <div class="modal-actions">
            {#if !editing}
              {#if detailCanEdit}
                <button class="action-btn" onclick={startEdit}>编辑</button>
                <button
                  class="action-btn danger"
                  onclick={deleteDetail}
                  disabled={deleteLoading}
                  >{deleteLoading ? "删除中…" : "删除"}</button
                >
              {/if}
            {/if}
            {#if detailHistoryUnsupported}
              <span class="no-history" title="该来源不支持 git 历史"
                >{detailIsCanonical
                  ? "Canonical generated/read-only"
                  : detailIsAssistMem
                    ? "Assist Mem 只读"
                    : "CC 原生不支持历史"}</span
              >
            {:else}
              <button
                class="action-btn"
                class:active={historyOpen}
                onclick={toggleHistory}>历史</button
              >
            {/if}
            <button class="close-btn" onclick={closeDetail} aria-label="关闭"
              >✕</button
            >
          </div>
        </div>

        {#if editing}
          <!-- Edit mode: raw file content (frontmatter + body) in one editor.
               The textarea holds verbatim file bytes; the backend reassembles
               nothing — content is written as-is. -->
          <div class="edit-bar">
            <span class="edit-hint"
              >编辑整文件（frontmatter + 正文）。保存写回磁盘 SSOT。</span
            >
            <div class="edit-buttons">
              <button
                class="action-btn primary"
                onclick={saveEdit}
                disabled={editSaving || editLoading}
                >{editSaving ? "保存中…" : "保存"}</button
              >
              <button
                class="action-btn"
                onclick={cancelEdit}
                disabled={editSaving}>取消</button
              >
            </div>
          </div>
          {#if editConflict}
            <div class="conflict-banner">
              <span>已被磁盘上的改动修改，请重载后再编辑。</span>
              <button
                class="action-btn"
                onclick={reloadForEdit}
                disabled={editLoading}>重载</button
              >
            </div>
          {:else if editError}
            <div class="state error">{editError}</div>
          {/if}
          {#if deleteError}
            <div class="state error">{deleteError}</div>
          {/if}
          {#if editLoading}
            <div class="state">加载文件中…</div>
          {:else}
            <textarea
              class="edit-area"
              bind:value={editContent}
              spellcheck="false"
              aria-label="文件内容编辑器"
            ></textarea>
          {/if}
        {:else}
          {#if deleteError}
            <div class="state error">{deleteError}</div>
          {/if}
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
          {#if detailIsCanonical}
            {@const coveredRefs = parseCanonicalCoveredRefs(detail)}
            {@const provenanceRows = canonicalProvenanceRows(detail)}
            <section class="canonical-detail" aria-label="Canonical coverage">
              <h4>Canonical coverage</h4>
              <div class="canonical-summary">
                {coveredRefs.length} covered raw {coveredRefs.length === 1 ? "ref" : "refs"}
              </div>
              {#if coveredRefs.length > 0}
                <ul class="coverage-list">
                  {#each coveredRefs as ref (`${ref.source}:${ref.rel_path}`)}
                    <li>
                      <span class="badge source source-{ref.source}">{sourceLabel(ref.source)}</span>
                      <code>{ref.rel_path}</code>
                    </li>
                  {/each}
                </ul>
              {:else}
                <div class="canonical-empty">No covered raw refs recorded.</div>
              {/if}
              <h4>Provenance</h4>
              {#if provenanceRows.length > 0}
                <ul class="provenance-list">
                  {#each provenanceRows as row (row)}
                    <li>{row}</li>
                  {/each}
                </ul>
              {:else}
                <div class="canonical-empty">No provenance metadata recorded.</div>
              {/if}
            </section>
          {/if}
          <h4>正文</h4>
          <pre class="body">{detail.body || "(无正文)"}</pre>
        {/if}

        {#if historyOpen}
          <h4>历史</h4>
          {#if historyLoading}
            <div class="state">加载历史中…</div>
          {:else if historyError}
            <div class="state error">{historyError}</div>
          {:else if history.length === 0}
            <div class="state">无 git 历史（memory dir 非 git repo 或文件未提交）。</div>
          {:else}
            <ul class="history-list">
              {#each history as h (h.commit)}
                <li class="history-item">
                  <button
                    class="history-row"
                    class:active={viewedCommit === h.commit}
                    onclick={() => viewCommit(h.commit)}
                  >
                    <span class="hist-date">{h.date}</span>
                    <span class="hist-msg">{h.message}</span>
                    <span class="hist-sha">{shortCommit(h.commit)}</span>
                  </button>
                  <button
                    class="action-btn revert-btn"
                    onclick={() => doRevert(h.commit)}
                    disabled={reverting}>回退</button
                  >
                  {#if viewedCommit === h.commit}
                    <div class="commit-view">
                      {#if commitLoading}
                        <div class="state">加载该版本…</div>
                      {:else if commitError}
                        <div class="state error">{commitError}</div>
                      {:else}
                        <div class="diff-caption">
                          该 commit（红）对比当前内容（绿）：
                        </div>
                        <pre class="diff">{#each commitDiff as dl, i (i)}<span
                              class="diff-line {dl.kind}"
                              >{dl.kind === "add"
                                ? "+"
                                : dl.kind === "del"
                                  ? "-"
                                  : " "}{dl.text}</span
                            >{/each}</pre>
                      {/if}
                    </div>
                  {/if}
                </li>
              {/each}
            </ul>
          {/if}
        {/if}

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
  .pipeline-card {
    margin: 0.75rem 0 1rem;
    padding: 0.9rem;
    border: 1px solid var(--border-default);
    border-radius: 10px;
    background: linear-gradient(
      135deg,
      color-mix(in srgb, var(--accent-blue) 8%, var(--bg-surface)),
      var(--bg-surface) 42%
    );
  }
  .pipeline-head {
    display: flex;
    justify-content: space-between;
    gap: 1rem;
    align-items: flex-start;
    margin-bottom: 0.75rem;
  }
  .eyebrow {
    margin-bottom: 0.15rem;
    color: var(--accent-blue);
    font-size: 0.68rem;
    font-weight: 700;
    letter-spacing: 0.08em;
    text-transform: uppercase;
  }
  .pipeline-head h2 {
    margin: 0;
    font-size: 1.05rem;
  }
  .pipeline-note {
    max-width: 18rem;
    color: var(--text-secondary, #666);
    font-size: 0.78rem;
    line-height: 1.45;
    text-align: right;
  }
  .pipeline-steps {
    display: grid;
    grid-template-columns: repeat(3, minmax(0, 1fr));
    gap: 0.55rem;
  }
  .pipeline-steps article {
    min-width: 0;
    padding: 0.7rem;
    border: 1px solid var(--border-default);
    border-radius: 8px;
    background: color-mix(in srgb, var(--bg-surface) 92%, transparent);
  }
  .pipeline-steps .knowledge-step {
    border-color: color-mix(in srgb, var(--accent-blue) 35%, var(--border-default));
    background: color-mix(in srgb, var(--accent-blue) 9%, var(--bg-surface));
  }
  .pipeline-steps .legacy-step {
    opacity: 0.72;
  }
  .pipeline-steps .canonical-step {
    border-color: color-mix(in srgb, var(--accent-indigo) 36%, var(--border-default));
    background: color-mix(in srgb, var(--accent-indigo) 9%, var(--bg-surface));
  }
  .step-label {
    display: block;
    color: var(--text-secondary, #666);
    font-size: 0.72rem;
    font-weight: 700;
    letter-spacing: 0.04em;
    text-transform: uppercase;
  }
  .pipeline-steps strong {
    display: block;
    margin-top: 0.15rem;
    color: var(--text-primary, #1a1a1a);
    font-size: 1.55rem;
    line-height: 1;
    font-variant-numeric: tabular-nums;
  }
  .pipeline-steps p {
    margin: 0.35rem 0 0;
    color: var(--text-secondary, #666);
    font-size: 0.76rem;
    line-height: 1.35;
  }
  .pipeline-steps button {
    margin-top: 0.55rem;
    padding: 0.25rem 0.45rem;
    border: 1px solid var(--border-default);
    border-radius: 6px;
    background: var(--bg-surface);
    color: var(--text-secondary, #555);
    cursor: pointer;
    font: inherit;
    font-size: 0.74rem;
  }
  .pipeline-steps button:hover,
  .pipeline-steps button:focus-visible {
    border-color: color-mix(in srgb, var(--accent-blue) 45%, var(--border-default));
    color: var(--text-primary, #1a1a1a);
    outline: none;
  }
  .controls .search {
    flex: 1 1 14rem;
    min-width: 10rem;
    padding: 0.35rem 0.6rem;
    font-size: 0.85rem;
    border: 1px solid var(--border-default);
    border-radius: 6px;
    background: var(--bg-surface);
    color: var(--text-primary, #1a1a1a);
  }
  .controls select {
    padding: 0.35rem 0.5rem;
    font-size: 0.82rem;
    border: 1px solid var(--border-default);
    border-radius: 6px;
    background: var(--bg-surface);
    color: var(--text-primary, #1a1a1a);
  }
  .controls .clear,
  .controls .refresh {
    background: none;
    border: 1px solid var(--border-default);
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
  .active-filter-note {
    margin: -0.1rem 0 0.5rem;
    color: var(--text-secondary, #666);
    font-size: 0.78rem;
  }
  .state {
    padding: 2rem;
    text-align: center;
    color: var(--text-secondary, #666);
  }
  .state.error {
    color: var(--accent-red);
  }
  table.grid {
    width: 100%;
    border-collapse: collapse;
    font-size: 0.82rem;
  }
  .table-scroll {
    width: 100%;
    overflow-x: auto;
  }
  .table-scroll table.grid {
    min-width: 760px;
  }
  table.grid th,
  table.grid td {
    text-align: left;
    padding: 0.45rem 0.6rem;
    border-bottom: 1px solid var(--border-default);
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
  .canonical-subtext {
    margin-top: 0.12rem;
    color: var(--accent-indigo);
    font-size: 0.72rem;
    font-weight: 500;
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
    background: color-mix(in srgb, var(--accent-indigo) 16%, transparent);
    color: var(--accent-indigo);
  }
  .badge.project {
    white-space: nowrap;
    background: color-mix(in srgb, var(--accent-blue) 14%, transparent);
    color: var(--accent-blue);
  }
  .badge.project.general {
    background: var(--bg-inset);
    color: var(--text-secondary, #666);
  }
  .group-toggle {
    display: inline-flex;
    align-items: center;
    gap: 0.3rem;
    font-size: 0.82rem;
    color: var(--text-secondary, #666);
    cursor: pointer;
    white-space: nowrap;
  }
  .project-group {
    margin-bottom: 1.25rem;
  }
  .project-head {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    margin: 0.75rem 0 0.4rem;
    font-size: 0.9rem;
  }
  .project-count {
    font-size: 0.74rem;
    color: var(--text-secondary, #888);
    font-weight: 400;
  }
  .badge.source {
    white-space: nowrap;
    background: #f1f5f9;
    color: #334155;
  }
  .badge.source.source-cc-native {
    background: #fef3c7;
    color: #92400e;
  }
  .badge.source.source-cross-agent {
    background: #dcfce7;
    color: #166534;
  }
  .badge.source.source-assist-mem {
    background: #e0e7ff;
    color: #3730a3;
  }
  .badge.source.source-canonical {
    background: #ede9fe;
    color: #6d28d9;
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
    background: var(--bg-surface-hover);
  }
  code {
    font-size: 0.78em;
    background: var(--bg-inset);
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
    background: var(--bg-surface);
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
    border: 1px solid var(--border-default);
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
    border-bottom: 1px solid var(--border-default);
    vertical-align: top;
  }
  .canonical-detail {
    margin-top: 0.8rem;
    padding: 0.7rem;
    border: 1px solid color-mix(in srgb, var(--accent-indigo) 28%, var(--border-default));
    border-radius: 8px;
    background: color-mix(in srgb, var(--accent-indigo) 7%, var(--bg-surface));
  }
  .canonical-detail h4:first-child {
    margin-top: 0;
  }
  .canonical-summary,
  .canonical-empty {
    color: var(--text-secondary, #666);
    font-size: 0.78rem;
  }
  .coverage-list,
  .provenance-list {
    margin: 0.45rem 0 0;
    padding-left: 1rem;
    font-size: 0.78rem;
  }
  .coverage-list li,
  .provenance-list li {
    margin: 0.3rem 0;
  }
  .coverage-list code {
    margin-left: 0.35rem;
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
    background: var(--bg-surface-hover);
    color: var(--text-primary, #1a1a1a);
    border: 1px solid var(--border-default);
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
  .modal-actions {
    display: flex;
    gap: 0.4rem;
    align-items: flex-start;
    flex-shrink: 0;
  }
  .action-btn {
    background: none;
    border: 1px solid var(--border-default);
    border-radius: 6px;
    cursor: pointer;
    padding: 0.25rem 0.6rem;
    color: var(--text-secondary, #555);
    font-size: 0.8rem;
  }
  .action-btn:hover:not(:disabled) {
    color: var(--text-primary, #1a1a1a);
    background: var(--bg-surface-hover);
  }
  .no-history {
    font-size: 0.75rem;
    color: var(--text-secondary, #888);
    align-self: center;
    white-space: nowrap;
  }
  .action-btn:disabled {
    opacity: 0.5;
    cursor: default;
  }
  .action-btn.active {
    background: color-mix(in srgb, var(--accent-indigo) 16%, transparent);
    color: var(--accent-indigo);
    border-color: color-mix(in srgb, var(--accent-indigo) 40%, transparent);
  }
  .action-btn.primary {
    background: var(--accent-blue);
    color: #fff;
    border-color: var(--accent-blue);
  }
  .action-btn.primary:hover:not(:disabled) {
    background: color-mix(in srgb, var(--accent-blue) 82%, black);
    color: #fff;
  }
  .action-btn.danger {
    color: #b42318;
    border-color: color-mix(in srgb, #b42318 35%, var(--border-default));
  }
  .action-btn.danger:hover:not(:disabled) {
    color: #7a271a;
    background: color-mix(in srgb, #f04438 8%, var(--bg-surface));
  }

  .edit-bar {
    display: flex;
    justify-content: space-between;
    align-items: center;
    gap: 0.75rem;
    margin: 0.9rem 0 0.5rem;
    flex-wrap: wrap;
  }
  .edit-hint {
    font-size: 0.78rem;
    color: var(--text-secondary, #666);
  }
  .edit-buttons {
    display: flex;
    gap: 0.4rem;
  }
  .edit-area {
    width: 100%;
    box-sizing: border-box;
    min-height: 22rem;
    max-height: 55vh;
    font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
    font-size: 0.78rem;
    line-height: 1.5;
    padding: 0.75rem;
    border: 1px solid var(--border-default);
    border-radius: 6px;
    background: var(--bg-surface);
    color: var(--text-primary, #1a1a1a);
    resize: vertical;
    white-space: pre;
    overflow: auto;
  }
  .conflict-banner {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 0.75rem;
    background: color-mix(in srgb, var(--accent-red) 12%, transparent);
    border: 1px solid color-mix(in srgb, var(--accent-red) 30%, transparent);
    color: var(--accent-red);
    border-radius: 6px;
    padding: 0.5rem 0.75rem;
    font-size: 0.8rem;
    margin-bottom: 0.5rem;
  }
  .history-list {
    list-style: none;
    margin: 0.4rem 0 0;
    padding: 0;
    border: 1px solid var(--border-default);
    border-radius: 6px;
    overflow: hidden;
  }
  .history-item {
    display: grid;
    grid-template-columns: 1fr auto;
    align-items: center;
    border-bottom: 1px solid var(--border-default);
  }
  .history-item:last-child {
    border-bottom: none;
  }
  .history-row {
    display: flex;
    gap: 0.6rem;
    align-items: baseline;
    background: none;
    border: none;
    cursor: pointer;
    text-align: left;
    padding: 0.45rem 0.6rem;
    width: 100%;
    color: var(--text-primary, #1a1a1a);
    font-size: 0.8rem;
  }
  .history-row:hover,
  .history-row.active {
    background: var(--bg-surface-hover);
  }
  .hist-date {
    color: var(--text-secondary, #666);
    white-space: nowrap;
    font-variant-numeric: tabular-nums;
    font-size: 0.74rem;
  }
  .hist-msg {
    flex: 1;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .hist-sha {
    font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
    color: var(--text-secondary, #999);
    font-size: 0.72rem;
  }
  .revert-btn {
    margin: 0 0.5rem;
    flex-shrink: 0;
  }
  .commit-view {
    grid-column: 1 / -1;
    padding: 0 0.6rem 0.6rem;
  }
  .diff-caption {
    font-size: 0.74rem;
    color: var(--text-secondary, #666);
    margin: 0.3rem 0;
  }
  pre.diff {
    margin: 0;
    background: var(--bg-surface-hover);
    border: 1px solid var(--border-default);
    border-radius: 6px;
    font-size: 0.74rem;
    line-height: 1.4;
    max-height: 40vh;
    overflow: auto;
    padding: 0.4rem 0;
  }
  .diff-line {
    display: block;
    padding: 0 0.6rem;
    white-space: pre-wrap;
    word-break: break-word;
  }
  .diff-line.add {
    background: color-mix(in srgb, var(--accent-green) 16%, transparent);
    color: var(--accent-green);
  }
  .diff-line.del {
    background: color-mix(in srgb, var(--accent-red) 16%, transparent);
    color: var(--accent-red);
  }

  @media (max-width: 700px) {
    .memory-page {
      padding: 1rem 1.25rem;
    }
    .pipeline-head {
      display: block;
    }
    .pipeline-note {
      max-width: none;
      margin-top: 0.35rem;
      text-align: left;
    }
    .pipeline-steps {
      grid-template-columns: 1fr;
    }
    .pipeline-steps article {
      padding: 0.65rem;
    }
    .pipeline-steps button {
      width: 100%;
      min-height: 2rem;
    }
  }

  @media (max-width: 420px) {
    .memory-page {
      padding: 1rem;
    }
    .pipeline-card {
      padding: 0.75rem;
    }
    .pipeline-head h2 {
      font-size: 0.98rem;
    }
    .pipeline-steps {
      gap: 0.45rem;
    }
    .pipeline-steps strong {
      font-size: 1.35rem;
    }
  }
</style>
