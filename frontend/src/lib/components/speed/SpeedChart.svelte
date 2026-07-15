<script lang="ts">
  import type { SpeedTrendSeries } from "../../api/types/analytics.js";

  interface Props {
    series: SpeedTrendSeries[];
  }

  let { series }: Props = $props();
  const colors = ["#2563eb", "#d97706", "#7c3aed", "#059669", "#db2777", "#0891b2", "#64748b", "#dc2626", "#52525b"];
  const HEIGHT = 300;
  const LEFT = 52;
  const RIGHT = 16;
  const TOP = 28;
  const BOTTOM = 34;
  let container = $state<HTMLDivElement>();
  let width = $state(760);
  let hovered = $state<{ key: string; p50: number | null; p95: number | null; n: number } | null>(null);

  $effect(() => {
    if (!container) return;
    const observer = new ResizeObserver(([entry]) => {
      if (entry) width = Math.max(320, Math.floor(entry.contentRect.width));
    });
    observer.observe(container);
    return () => observer.disconnect();
  });

  const allPoints = $derived(series.flatMap((item) => item.points));
  const minT = $derived(Math.min(...allPoints.map((point) => point.t)));
  const maxT = $derived(Math.max(...allPoints.map((point) => point.t), minT + 1));
  const maxY = $derived(Math.max(...allPoints.map((point) => point.p50 ?? 0), 1));
  const plotW = $derived(Math.max(width - LEFT - RIGHT, 1));
  const plotH = HEIGHT - TOP - BOTTOM;

  function x(t: number): number {
    return LEFT + ((t - minT) / Math.max(maxT - minT, 1)) * plotW;
  }
  function y(value: number): number {
    return TOP + plotH - (value / maxY) * plotH;
  }
  function path(points: SpeedTrendSeries["points"]): string {
    let started = false;
    return points.map((point) => {
      if (point.p50 == null) {
        started = false;
        return "";
      }
      const command = started ? "L" : "M";
      started = true;
      return `${command}${x(point.t)},${y(point.p50)}`;
    }).join("");
  }
  function labelFor(t: number): string {
    return new Date(t * 1000).toLocaleString(undefined, {
      month: "short",
      day: "numeric",
      hour: "numeric",
    });
  }

  function pointLabel(item: SpeedTrendSeries, point: SpeedTrendSeries["points"][number]): string {
    const p50 = point.p50 == null ? "insufficient data" : `${point.p50.toFixed(1)} tok/s`;
    return `${item.key}: p50 ${p50}, ${point.n} samples`;
  }
</script>

<div class="chart" bind:this={container}>
  {#if allPoints.length === 0}
    <div class="empty">No speed samples in this range</div>
  {:else}
    <svg viewBox={`0 0 ${width} ${HEIGHT}`} role="img" aria-label="Approximate output speed p50 trend">
      <text class="axis-title" x={LEFT} y="14">Output speed p50 (approx. tok/s)</text>
      {#each [0, 0.25, 0.5, 0.75, 1] as ratio}
        {@const value = maxY * ratio}
        {@const yy = y(value)}
        <line x1={LEFT} x2={width - RIGHT} y1={yy} y2={yy} class="grid" />
        <text x={LEFT - 8} y={yy + 4} class="label">{value.toFixed(1)}</text>
      {/each}
      {#each [0, 0.5, 1] as ratio}
        {@const t = Math.round(minT + (maxT - minT) * ratio)}
        <text x={x(t)} y={HEIGHT - 8} class="x-label">{labelFor(t)}</text>
      {/each}
      {#each series as item, index}
        <path d={path(item.points)} fill="none" stroke={colors[index % colors.length]} stroke-width="2.5" stroke-linecap="round" />
        {#each item.points as point}
          <circle
            cx={x(point.t)}
            cy={point.p50 == null ? TOP + plotH : y(point.p50)}
            r="6"
            fill={point.p50 == null ? "var(--border-muted)" : "transparent"}
            role="button"
            tabindex="0"
            aria-label={pointLabel(item, point)}
            onmouseenter={() => hovered = { key: item.key, p50: point.p50, p95: point.p95, n: point.n }}
            onmouseleave={() => hovered = null}
          />
        {/each}
      {/each}
    </svg>
  {/if}
  {#if hovered}
    <div class="tooltip"><strong>{hovered.key}</strong><br />p50 {hovered.p50?.toFixed(1) ?? "insufficient data"}<br />p95 {hovered.p95?.toFixed(1) ?? "insufficient data"}<br />n {hovered.n}</div>
  {/if}
</div>

<style>
  .chart {
    position: relative;
    min-height: 300px;
    overflow: hidden;
    border: 1px solid var(--border-default);
    border-radius: 8px;
    background: var(--bg-surface);
  }

  svg {
    display: block;
    width: 100%;
    height: 300px;
  }

  .grid {
    stroke: var(--border-muted);
  }

  .label,
  .axis-title,
  .x-label {
    fill: var(--text-muted);
    font-size: 10px;
    text-anchor: end;
  }

  .axis-title {
    font-weight: 600;
    text-anchor: start;
  }

  .x-label {
    text-anchor: middle;
  }

  .empty {
    display: grid;
    height: 300px;
    color: var(--text-muted);
    font-size: 13px;
    place-items: center;
  }

  .tooltip {
    position: absolute;
    top: 28px;
    right: 18px;
    padding: 8px 10px;
    border: 1px solid var(--border-default);
    border-radius: 6px;
    background: var(--bg-inset);
    color: var(--text-secondary);
    font-size: 11px;
    line-height: 1.5;
    pointer-events: none;
  }
</style>
