<script lang="ts">
  import { onMount } from "svelte";
  import { fetchStagingPool, type StagingCandidate } from "../../api/staging";

  let loading = $state(true);
  let error = $state<string | null>(null);
  let open = $state(false);
  let available = $state(false);
  let total = $state(0);
  let byScope = $state<Record<string, number>>({});
  let projects = $state<Record<string, number>>({});
  let candidates = $state<StagingCandidate[]>([]);
  // "" = all, else filter to user / project.
  let scopeFilter = $state<"" | "user" | "project">("");

  async function load() {
    loading = true;
    error = null;
    try {
      const pool = await fetchStagingPool(scopeFilter);
      available = pool.available;
      total = pool.total;
      byScope = pool.by_scope ?? {};
      projects = pool.projects ?? {};
      candidates = pool.candidates ?? [];
    } catch (e) {
      error = e instanceof Error ? e.message : String(e);
    } finally {
      loading = false;
    }
  }

  function setScope(s: "" | "user" | "project") {
    if (scopeFilter === s) return;
    scopeFilter = s;
    load();
  }

  const projectEntries = $derived(
    Object.entries(projects).sort((a, b) => b[1] - a[1]),
  );

  onMount(load);
</script>

<section class="staging-pool">
  <button
    class="toggle"
    data-testid="staging-pool-toggle"
    onclick={() => {
      open = !open;
      if (open && !loading && total === 0 && !error) load();
    }}
    aria-expanded={open}
  >
    <span class="chevron" class:open>▶</span>
    备选池（待巩固候选）
    <span class="count">{total}</span>
  </button>

  {#if open}
    <div class="body">
      {#if loading}
        <p class="muted">加载中…</p>
      {:else if error}
        <p class="err">{error}</p>
      {:else if !available}
        <p class="muted">未配置 dotfiles 根目录，无法定位备选池。</p>
      {:else}
        <div class="summary">
          <span class="chip" class:active={scopeFilter === ""} role="button" tabindex="0"
            onclick={() => setScope("")} onkeydown={(e) => e.key === "Enter" && setScope("")}>
            全部 {total}
          </span>
          <span class="chip" class:active={scopeFilter === "user"} role="button" tabindex="0"
            onclick={() => setScope("user")} onkeydown={(e) => e.key === "Enter" && setScope("user")}>
            整体（用户级）{byScope.user ?? 0}
          </span>
          <span class="chip" class:active={scopeFilter === "project"} role="button" tabindex="0"
            onclick={() => setScope("project")} onkeydown={(e) => e.key === "Enter" && setScope("project")}>
            项目级 {byScope.project ?? 0}
          </span>
        </div>

        {#if projectEntries.length}
          <div class="projects">
            {#each projectEntries as [name, n] (name)}
              <span class="proj">{name} <b>{n}</b></span>
            {/each}
          </div>
        {/if}

        {#if candidates.length === 0}
          <p class="muted">该范围暂无候选。</p>
        {:else}
          <ul class="cands">
            {#each candidates as c (c.id)}
              <li class="cand">
                <span class="cat">{c.category}</span>
                <span class="scope" class:project={c.scope === "project"}>
                  {c.scope === "project" ? c.origin_project || "project" : "user"}
                </span>
                <span class="sum">{c.summary}</span>
              </li>
            {/each}
          </ul>
        {/if}
      {/if}
    </div>
  {/if}
</section>

<style>
  .staging-pool {
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
  .count {
    margin-left: auto;
    font-variant-numeric: tabular-nums;
    color: var(--text-muted);
  }
  .body {
    padding: 0 0.75rem 0.75rem;
  }
  .summary {
    display: flex;
    flex-wrap: wrap;
    gap: 0.4rem;
    margin: 0.25rem 0 0.5rem;
  }
  .chip {
    padding: 0.15rem 0.5rem;
    border: 1px solid var(--border-default);
    border-radius: 999px;
    font-size: 0.85em;
    cursor: pointer;
    user-select: none;
  }
  .chip.active {
    background: var(--accent-blue, #3b82f6);
    border-color: var(--accent-blue, #3b82f6);
    color: #fff;
  }
  .projects {
    display: flex;
    flex-wrap: wrap;
    gap: 0.4rem;
    margin-bottom: 0.5rem;
  }
  .proj {
    font-size: 0.8em;
    color: var(--text-muted);
    border: 1px dashed var(--border-default);
    border-radius: 4px;
    padding: 0.1rem 0.4rem;
  }
  .cands {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: 0.25rem;
  }
  .cand {
    display: flex;
    align-items: baseline;
    gap: 0.5rem;
    font-size: 0.88em;
  }
  .cat {
    flex: 0 0 auto;
    min-width: 5.5rem;
    color: var(--text-muted);
    font-variant: small-caps;
  }
  .scope {
    flex: 0 0 auto;
    min-width: 5rem;
    color: var(--text-muted);
    font-size: 0.85em;
  }
  .scope.project {
    color: var(--accent-blue, #3b82f6);
  }
  .sum {
    flex: 1 1 auto;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .muted {
    color: var(--text-muted);
  }
  .err {
    color: var(--accent-red, #ef4444);
  }
</style>
