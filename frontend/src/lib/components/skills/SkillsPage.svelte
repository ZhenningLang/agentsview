<script lang="ts">
  import { onMount } from "svelte";
  import {
    fetchSkills,
    fetchSkillCost,
    fetchSkillHealth,
    fetchSkill,
    type Skill,
    type SkillTokenCostReport,
    type SkillHealthReport,
  } from "../../api/skills";

  type Tab = "catalog" | "cost" | "health";
  type SortKey =
    | "name"
    | "domain"
    | "description_tokens"
    | "invocation_count"
    | "total_prompt_tokens";

  // Reference context window used to express resident cost as a share.
  const REFERENCE_WINDOW = 200_000;

  let tab = $state<Tab>("catalog");
  let loading = $state(true);
  let error = $state<string | null>(null);

  let skills = $state<Skill[]>([]);
  let cost = $state<SkillTokenCostReport | null>(null);
  let health = $state<SkillHealthReport | null>(null);

  async function load() {
    loading = true;
    error = null;
    try {
      const [s, c, h] = await Promise.all([
        fetchSkills(),
        fetchSkillCost(),
        fetchSkillHealth(),
      ]);
      skills = s;
      cost = c;
      health = h;
    } catch (e) {
      error = e instanceof Error ? e.message : String(e);
    } finally {
      loading = false;
    }
  }

  onMount(load);

  const windowPct = $derived(
    cost ? (cost.total_tokens / REFERENCE_WINDOW) * 100 : 0,
  );
  const maxDomainTokens = $derived(
    cost ? Math.max(1, ...(cost.by_domain ?? []).map((d) => d.tokens)) : 1,
  );

  // Sortable per-skill cost table.
  let sortKey = $state<SortKey>("total_prompt_tokens");
  let sortDir = $state<"asc" | "desc">("desc");

  function toggleSort(key: SortKey) {
    if (sortKey === key) {
      sortDir = sortDir === "asc" ? "desc" : "asc";
    } else {
      sortKey = key;
      sortDir = key === "name" || key === "domain" ? "asc" : "desc";
    }
  }

  const sortedCostSkills = $derived(
    [...(cost?.skills ?? [])].sort((a, b) => {
      const av = a[sortKey];
      const bv = b[sortKey];
      let cmp: number;
      if (typeof av === "string" && typeof bv === "string") {
        cmp = av.localeCompare(bv);
      } else {
        cmp = (av as number) - (bv as number);
      }
      return sortDir === "asc" ? cmp : -cmp;
    }),
  );

  function sortIndicator(key: SortKey): string {
    if (sortKey !== key) return "";
    return sortDir === "asc" ? " ▲" : " ▼";
  }

  // Skill detail (prompt audit) modal.
  let detailSkill = $state<Skill | null>(null);
  let detailLoading = $state(false);
  let detailError = $state<string | null>(null);

  async function openDetail(name: string) {
    detailLoading = true;
    detailError = null;
    detailSkill = null;
    try {
      detailSkill = await fetchSkill(name);
    } catch (e) {
      detailError = e instanceof Error ? e.message : String(e);
    } finally {
      detailLoading = false;
    }
  }

  function closeDetail() {
    detailSkill = null;
    detailError = null;
    detailLoading = false;
  }

  function severityClass(sev: string): string {
    if (sev === "error") return "sev-error";
    if (sev === "warn") return "sev-warn";
    return "sev-info";
  }
</script>

<div class="skills-page">
  <header class="skills-header">
    <h1>Skills</h1>
    <p class="subtitle">
      跨 agent skill 体系治理视图：清单、静态 context 成本、健康体检。
    </p>
    <nav class="tabs">
      <button class:active={tab === "catalog"} onclick={() => (tab = "catalog")}>
        清单{skills.length ? ` (${skills.length})` : ""}
      </button>
      <button class:active={tab === "cost"} onclick={() => (tab = "cost")}>
        静态成本
      </button>
      <button class:active={tab === "health"} onclick={() => (tab = "health")}>
        健康体检{health && health.findings.length
          ? ` (${health.findings.length})`
          : ""}
      </button>
      <button class="refresh" onclick={load} title="Reload" aria-label="刷新"
        >↻</button
      >
    </nav>
  </header>

  {#if loading}
    <div class="state">加载中…</div>
  {:else if error}
    <div class="state error">加载失败：{error}</div>
  {:else if tab === "catalog"}
    {#if skills.length === 0}
      <div class="state">
        未配置 skill 目录，或目录中没有 catalog.json。设置
        <code>AGENTSVIEW_SKILLS_DIR</code> 指向 coding-skills 目录后重启。
      </div>
    {:else}
    <table class="grid">
      <thead>
        <tr>
          <th>Name</th><th>Domain</th><th>Role</th>
          <th class="num" title="description 常驻 token 数（近似，非 Anthropic 精确分词器）"
            >Tokens</th
          ><th>状态</th>
        </tr>
      </thead>
      <tbody>
        {#each skills as s (s.name)}
          <tr class="clickable" onclick={() => openDetail(s.name)}>
            <td class="name">{s.name}</td>
            <td>{s.domain}</td>
            <td>{s.role}</td>
            <td class="num">{s.description_tokens}</td>
            <td>
              {#if !s.file_present}
                <span class="badge sev-error">缺文件</span>
              {:else if s.health_error_count > 0}
                <span class="badge sev-error">{s.health_error_count} 问题</span>
              {:else}
                <span class="badge ok">ok</span>
              {/if}
            </td>
          </tr>
        {/each}
      </tbody>
    </table>
    {/if}
  {:else if tab === "cost" && cost}
    <div class="cost-summary">
      <div class="stat">
        <div class="stat-value">{cost.total_tokens.toLocaleString()}</div>
        <div class="stat-label">常驻描述 token 总量</div>
      </div>
      <div class="stat">
        <div class="stat-value">{windowPct.toFixed(2)}%</div>
        <div class="stat-label">占 {REFERENCE_WINDOW.toLocaleString()} 窗口</div>
      </div>
      <div class="stat">
        <div class="stat-value">{cost.total_skills}</div>
        <div class="stat-label">skills</div>
      </div>
    </div>
    <p class="caveat">
      估算口径：tokenizer = <code>{cost.tokenizer}</code>{cost.approximate
        ? "（近似，非 Anthropic 精确分词器，仅用于量级判断）"
        : ""}。占窗口百分比以 {REFERENCE_WINDOW.toLocaleString()} 为固定参考基准，非实际模型上下文长度。
    </p>
    <h3>按域</h3>
    <div class="bars">
      {#each cost.by_domain as d (d.domain)}
        <div class="bar-row">
          <span class="bar-label">{d.domain || "(none)"}</span>
          <div class="bar-track">
            <div
              class="bar-fill"
              style="width: {(d.tokens / maxDomainTokens) * 100}%"
            ></div>
          </div>
          <span class="bar-num">{d.tokens} · {d.skills}</span>
        </div>
      {/each}
    </div>
    <h3>按 skill</h3>
    <p class="caveat">
      点击任意 skill 查看提示词。总次数仅统计走 Skill 机制的调用（不含模型
      inline 完成）；总 token = 总次数 × 提示词 token。
    </p>
    <table class="grid sortable">
      <thead>
        <tr>
          <th class="sortable-th" onclick={() => toggleSort("name")}
            >Name{sortIndicator("name")}</th
          >
          <th class="sortable-th" onclick={() => toggleSort("domain")}
            >Domain{sortIndicator("domain")}</th
          >
          <th class="num sortable-th"
            title="description 常驻 token（近似）"
            onclick={() => toggleSort("description_tokens")}
            >描述token{sortIndicator("description_tokens")}</th
          >
          <th class="num sortable-th"
            onclick={() => toggleSort("invocation_count")}
            >总次数{sortIndicator("invocation_count")}</th
          >
          <th class="num sortable-th"
            title="总次数 × 提示词 token"
            onclick={() => toggleSort("total_prompt_tokens")}
            >总token{sortIndicator("total_prompt_tokens")}</th
          >
        </tr>
      </thead>
      <tbody>
        {#each sortedCostSkills as s (s.name)}
          <tr class="clickable" onclick={() => openDetail(s.name)}>
            <td class="name">{s.name}</td>
            <td>{s.domain}</td>
            <td class="num">{s.description_tokens.toLocaleString()}</td>
            <td class="num">{s.invocation_count.toLocaleString()}</td>
            <td class="num">{s.total_prompt_tokens.toLocaleString()}</td>
          </tr>
        {/each}
      </tbody>
    </table>
  {:else if tab === "health" && health}
    <div class="health-summary">
      <span class="badge ok">{health.healthy_skills}/{health.total_skills} 健康</span>
      {#each Object.entries(health.by_severity) as [sev, n] (sev)}
        <span class="badge {severityClass(sev)}">{sev}: {n}</span>
      {/each}
    </div>
    {#if health.findings.length === 0}
      <div class="state">没有发现问题。catalog wiring / 软链 / 重复检查全部通过。</div>
    {:else}
      <table class="grid">
        <thead>
          <tr><th>Severity</th><th>Check</th><th>Skill</th><th>Message</th></tr>
        </thead>
        <tbody>
          {#each health.findings as f (f.id)}
            <tr>
              <td><span class="badge {severityClass(f.severity)}">{f.severity}</span></td>
              <td><code>{f.check_type}</code></td>
              <td class="name">{f.skill_name || "—"}</td>
              <td>{f.message}</td>
            </tr>
          {/each}
        </tbody>
      </table>
    {/if}
  {/if}
</div>

{#if detailSkill || detailLoading || detailError}
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
      {:else if detailSkill}
        <div class="modal-head">
          <div>
            <h2>{detailSkill.name}</h2>
            <div class="modal-meta">
              {detailSkill.domain} · {detailSkill.role}
              {#if detailSkill.migration_state}
                · {detailSkill.migration_state}{detailSkill.migration_canonical
                  ? ` → ${detailSkill.migration_canonical}`
                  : ""}
              {/if}
            </div>
          </div>
          <button class="close-btn" onclick={closeDetail} aria-label="关闭"
            >✕</button
          >
        </div>
        <div class="modal-stats">
          <span><b>{detailSkill.invocation_count.toLocaleString()}</b> 总次数</span>
          <span><b>{detailSkill.prompt_tokens.toLocaleString()}</b> 提示词token</span>
          <span
            ><b>{detailSkill.total_prompt_tokens.toLocaleString()}</b> 总token</span
          >
          <span
            ><b>{detailSkill.description_tokens.toLocaleString()}</b> 描述token</span
          >
        </div>
        <p class="modal-desc">{detailSkill.description}</p>
        <h4>提示词（SKILL.md）</h4>
        <pre class="prompt">{detailSkill.prompt || "(无正文)"}</pre>
        <div class="modal-path">{detailSkill.resolved_path}</div>
      {/if}
    </div>
  </div>
{/if}

<style>
  .skills-page {
    max-width: 960px;
    margin: 0 auto;
    padding: 1.5rem;
    color: var(--text-primary, #1a1a1a);
  }
  .skills-header h1 {
    margin: 0 0 0.25rem;
    font-size: 1.4rem;
  }
  .subtitle {
    margin: 0 0 1rem;
    color: var(--text-secondary, #666);
    font-size: 0.85rem;
  }
  .tabs {
    display: flex;
    gap: 0.5rem;
    border-bottom: 1px solid var(--border-default);
    margin-bottom: 1rem;
  }
  .tabs button {
    background: none;
    border: none;
    padding: 0.5rem 0.75rem;
    cursor: pointer;
    color: var(--text-secondary, #666);
    border-bottom: 2px solid transparent;
    font-size: 0.85rem;
  }
  .tabs button.active {
    color: var(--text-primary, #1a1a1a);
    border-bottom-color: var(--accent-blue);
  }
  .tabs .refresh {
    margin-left: auto;
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
  table.grid th,
  table.grid td {
    text-align: left;
    padding: 0.4rem 0.6rem;
    border-bottom: 1px solid var(--border-default);
  }
  table.grid th.num,
  table.grid td.num {
    text-align: right;
    font-variant-numeric: tabular-nums;
  }
  td.name {
    font-weight: 600;
  }
  .badge {
    display: inline-block;
    padding: 0.1rem 0.4rem;
    border-radius: 4px;
    font-size: 0.72rem;
  }
  .badge.ok {
    background: color-mix(in srgb, var(--accent-green) 16%, transparent);
    color: var(--accent-green);
  }
  .sev-error {
    background: color-mix(in srgb, var(--accent-red) 16%, transparent);
    color: var(--accent-red);
  }
  .sev-warn {
    background: color-mix(in srgb, var(--accent-amber) 18%, transparent);
    color: var(--accent-amber);
  }
  .sev-info {
    background: color-mix(in srgb, var(--accent-indigo) 16%, transparent);
    color: var(--accent-indigo);
  }
  .cost-summary,
  .health-summary {
    display: flex;
    gap: 1.5rem;
    margin-bottom: 0.75rem;
    flex-wrap: wrap;
    align-items: center;
  }
  .stat-value {
    font-size: 1.5rem;
    font-weight: 700;
  }
  .stat-label {
    font-size: 0.75rem;
    color: var(--text-secondary, #666);
  }
  .caveat {
    font-size: 0.75rem;
    color: var(--text-secondary, #666);
  }
  h3 {
    font-size: 0.95rem;
    margin: 1.25rem 0 0.5rem;
  }
  .bars {
    display: flex;
    flex-direction: column;
    gap: 0.3rem;
    margin-bottom: 0.5rem;
  }
  .bar-row {
    display: grid;
    grid-template-columns: 7rem 1fr 6rem;
    gap: 0.5rem;
    align-items: center;
    font-size: 0.78rem;
  }
  .bar-track {
    background: var(--border-default);
    border-radius: 3px;
    height: 12px;
    overflow: hidden;
  }
  .bar-fill {
    background: var(--accent-blue);
    height: 100%;
  }
  .bar-num {
    text-align: right;
    font-variant-numeric: tabular-nums;
    color: var(--text-secondary, #666);
  }
  code {
    font-size: 0.78em;
    background: var(--bg-inset);
    padding: 0.05rem 0.25rem;
    border-radius: 3px;
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
  .modal-stats {
    display: flex;
    gap: 1.25rem;
    flex-wrap: wrap;
    margin: 0.75rem 0;
    font-size: 0.82rem;
    color: var(--text-secondary, #666);
  }
  .modal-stats b {
    color: var(--text-primary, #1a1a1a);
    font-size: 1rem;
  }
  .modal-desc {
    font-size: 0.85rem;
    color: var(--text-secondary, #666);
    margin: 0.25rem 0 0.75rem;
  }
  .modal h4 {
    margin: 0.5rem 0 0.4rem;
    font-size: 0.9rem;
  }
  pre.prompt {
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
</style>
