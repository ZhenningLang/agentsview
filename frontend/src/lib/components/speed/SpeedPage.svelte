<script lang="ts">
  import { onMount } from "svelte";
  import { speed } from "../../stores/speed.svelte.js";
  import { router } from "../../stores/router.svelte.js";
  import type { SpeedBucket, SpeedGroupBy } from "../../api/types/analytics.js";
  import SpeedChart from "./SpeedChart.svelte";

  const ranges = [
    { label: "24h", hours: 24 },
    { label: "7d", hours: 168 },
    { label: "30d", hours: 720 },
  ];

  function syncUrl() {
    router.replaceParams({
      bucket: speed.bucket,
      group_by: speed.groupBy,
      range: String(speed.rangeHours),
    });
  }

  async function refresh() {
    syncUrl();
    await speed.fetch();
  }

  async function setBucket(bucket: SpeedBucket) {
    speed.bucket = bucket;
    await refresh();
  }

  async function setGroupBy(groupBy: SpeedGroupBy) {
    speed.groupBy = groupBy;
    await refresh();
  }

  async function setRange(hours: number) {
    speed.rangeHours = hours;
    await refresh();
  }

  onMount(() => {
    const { bucket, group_by: groupBy, range } = router.params;
    if (bucket === "15m" || bucket === "hour" || bucket === "day") {
      speed.bucket = bucket;
    }
    if (groupBy === "agent" || groupBy === "model") {
      speed.groupBy = groupBy;
    }
    const hours = Number(range);
    if ([24, 168, 720].includes(hours)) {
      speed.rangeHours = hours;
    }
    void refresh();
  });
</script>

<section class="speed-page">
  <header>
    <div>
      <h1>Speed</h1>
      <p>
        Output speed (approx.) is derived from message timestamp gaps. It is not a decoding rate. Compare one agent over time first; cross-agent comparisons are for reference only.
      </p>
    </div>
    <button onclick={refresh} disabled={speed.loading}>
      {speed.loading ? "Refreshing" : "Refresh"}
    </button>
  </header>
  <div class="toolbar">
    <div class="control">
      <span>Group</span>
      {#each ["agent", "model"] as value}
        <button class:active={speed.groupBy === value} onclick={() => setGroupBy(value as SpeedGroupBy)}>{value}</button>
      {/each}
    </div>
    <div class="control">
      <span>Bucket</span>
      {#each ["15m", "hour", "day"] as value}
        <button class:active={speed.bucket === value} onclick={() => setBucket(value as SpeedBucket)}>{value}</button>
      {/each}
    </div>
    <div class="control">
      <span>Range</span>
      {#each ranges as range}
        <button class:active={speed.rangeHours === range.hours} onclick={() => setRange(range.hours)}>{range.label}</button>
      {/each}
    </div>
  </div>
  {#if speed.error}
    <div class="error">{speed.error}</div>
  {/if}
  <SpeedChart
    series={speed.response?.series ?? []}
    concurrency={speed.response?.concurrency ?? []}
    bucketSec={speed.response?.bucket_sec ?? 3600}
  />
</section>

<style>
  .speed-page {
    max-width: 1180px;
    margin: 0 auto;
    padding: 22px;
    color: var(--text-primary);
  }

  header {
    display: flex;
    justify-content: space-between;
    gap: 16px;
    margin-bottom: 18px;
  }

  h1 {
    margin: 0;
    font-size: 24px;
  }

  p {
    max-width: 700px;
    margin: 5px 0 0;
    color: var(--text-muted);
    font-size: 13px;
    line-height: 1.5;
  }

  button {
    padding: 6px 10px;
    border: 1px solid var(--border-default);
    border-radius: 6px;
    background: var(--bg-surface);
    color: var(--text-primary);
    cursor: pointer;
    font-size: 12px;
  }

  button:hover,
  button.active {
    background: var(--bg-inset);
  }

  button:disabled {
    cursor: default;
    opacity: 0.65;
  }

  .toolbar {
    display: flex;
    flex-wrap: wrap;
    gap: 12px;
    margin-bottom: 16px;
  }

  .control {
    display: flex;
    align-items: center;
    gap: 4px;
  }

  .control span {
    margin-right: 3px;
    color: var(--text-muted);
    font-size: 11px;
  }

  .error {
    margin-bottom: 10px;
    color: var(--accent-red);
    font-size: 12px;
  }

</style>
