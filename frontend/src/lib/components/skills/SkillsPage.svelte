<script lang="ts">
  import { onMount } from "svelte";
  import {
    fetchSkills,
    fetchSkillCost,
    fetchSkillHealth,
    type Skill,
    type SkillTokenCostReport,
    type SkillHealthReport,
  } from "../../api/skills";

  type Tab = "catalog" | "cost" | "health";

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
  const sortedCostSkills = $derived(
    [...(cost?.skills ?? [])].sort(
      (a, b) => b.description_tokens - a.description_tokens,
    ),
  );

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
          <tr>
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
    <h3>按 skill（降序）</h3>
    <table class="grid">
      <thead>
        <tr><th>Name</th><th>Domain</th><th class="num">Tokens</th></tr>
      </thead>
      <tbody>
        {#each sortedCostSkills as s (s.name)}
          <tr>
            <td class="name">{s.name}</td>
            <td>{s.domain}</td>
            <td class="num">{s.description_tokens}</td>
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
    border-bottom: 1px solid var(--border, #ddd);
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
    border-bottom-color: var(--accent, #3b82f6);
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
    padding: 0.4rem 0.6rem;
    border-bottom: 1px solid var(--border, #eee);
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
    background: #dcfce7;
    color: #166534;
  }
  .sev-error {
    background: #fee2e2;
    color: #991b1b;
  }
  .sev-warn {
    background: #fef9c3;
    color: #854d0e;
  }
  .sev-info {
    background: #e0e7ff;
    color: #3730a3;
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
    background: var(--border, #eee);
    border-radius: 3px;
    height: 12px;
    overflow: hidden;
  }
  .bar-fill {
    background: var(--accent, #3b82f6);
    height: 100%;
  }
  .bar-num {
    text-align: right;
    font-variant-numeric: tabular-nums;
    color: var(--text-secondary, #666);
  }
  code {
    font-size: 0.78em;
    background: var(--code-bg, #f3f4f6);
    padding: 0.05rem 0.25rem;
    border-radius: 3px;
  }
</style>
