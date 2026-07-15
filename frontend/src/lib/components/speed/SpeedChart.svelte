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
  // Below these point counts the chart labels values directly instead of
  // relying on the tooltip alone.
  const SPARSE_PER_SERIES = 16;
  const SPARSE_TOTAL = 48;

  let container = $state<HTMLDivElement>();
  let width = $state(760);
  let hoverT = $state<number | null>(null);
  let cursor = $state<{ x: number; y: number } | null>(null);

  $effect(() => {
    if (!container) return;
    const observer = new ResizeObserver(([entry]) => {
      if (entry) width = Math.max(320, Math.floor(entry.contentRect.width));
    });
    observer.observe(container);
    return () => observer.disconnect();
  });

  const allPoints = $derived(series.flatMap((item) => item.points));
  const bucketTs = $derived(
    [...new Set(allPoints.map((point) => point.t))].sort((a, b) => a - b),
  );
  const minT = $derived(Math.min(...allPoints.map((point) => point.t)));
  const maxT = $derived(Math.max(...allPoints.map((point) => point.t), minT + 1));
  const maxY = $derived(Math.max(...allPoints.map((point) => point.p50 ?? 0), 1));
  const plotW = $derived(Math.max(width - LEFT - RIGHT, 1));
  const plotH = HEIGHT - TOP - BOTTOM;

  const validCounts = $derived(
    series.map((item) => item.points.filter((point) => point.p50 != null).length),
  );
  const sparse = $derived.by(() => {
    const total = validCounts.reduce((sum, count) => sum + count, 0);
    if (total === 0 || total > SPARSE_TOTAL) return false;
    return validCounts.every((count) => count <= SPARSE_PER_SERIES);
  });

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
  function formatRate(value: number): string {
    return value >= 100 ? value.toFixed(0) : value.toFixed(1);
  }

  function nearestBucket(px: number): number | null {
    if (bucketTs.length === 0) return null;
    const t = minT + ((px - LEFT) / plotW) * (maxT - minT);
    let best = bucketTs[0]!;
    let bestDist = Math.abs(best - t);
    for (const candidate of bucketTs) {
      const dist = Math.abs(candidate - t);
      if (dist < bestDist) {
        best = candidate;
        bestDist = dist;
      }
    }
    return best;
  }

  function handleMove(event: MouseEvent) {
    hoverT = nearestBucket(event.offsetX);
    cursor = { x: event.clientX, y: event.clientY };
  }
  function handleLeave() {
    hoverT = null;
    cursor = null;
  }

  interface HoverRow {
    key: string;
    isOther: boolean;
    color: string;
    p50: number | null;
    p95: number | null;
    n: number;
  }
  const hoverRows = $derived.by((): HoverRow[] => {
    if (hoverT == null) return [];
    const rows: HoverRow[] = [];
    series.forEach((item, index) => {
      const point = item.points.find((candidate) => candidate.t === hoverT);
      if (!point) return;
      rows.push({
        key: item.key,
        isOther: item.is_other,
        color: colors[index % colors.length]!,
        p50: point.p50,
        p95: point.p95,
        n: point.n,
      });
    });
    return rows;
  });

  const tooltipStyle = $derived.by(() => {
    if (!cursor) return "";
    const left = Math.min(cursor.x + 14, window.innerWidth - 230);
    const estimated = 44 + hoverRows.length * 18;
    const top = Math.min(cursor.y + 14, window.innerHeight - estimated);
    return `left: ${left}px; top: ${top}px;`;
  });
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
      {#if hoverT != null}
        <line
          class="crosshair"
          x1={x(hoverT)}
          x2={x(hoverT)}
          y1={TOP}
          y2={TOP + plotH}
        />
      {/if}
      {#each series as item, index}
        {@const color = colors[index % colors.length]}
        <path d={path(item.points)} fill="none" stroke={color} stroke-width="2.5" stroke-linecap="round" />
        {#if sparse}
          {#each item.points as point}
            {#if point.p50 != null}
              <circle class="dot" cx={x(point.t)} cy={y(point.p50)} r="3" fill={color} />
              <text class="value-label" x={x(point.t)} y={y(point.p50) - 8} fill={color}>
                {formatRate(point.p50)}
              </text>
            {/if}
          {/each}
        {/if}
        {#if hoverT != null}
          {@const hoverPoint = item.points.find((candidate) => candidate.t === hoverT)}
          {#if hoverPoint && hoverPoint.p50 != null}
            <circle class="marker" cx={x(hoverPoint.t)} cy={y(hoverPoint.p50)} r="4" fill={color} />
          {/if}
        {/if}
      {/each}
      <rect
        class="overlay"
        role="presentation"
        x={LEFT}
        y={TOP}
        width={plotW}
        height={plotH}
        onmousemove={handleMove}
        onmouseleave={handleLeave}
      />
    </svg>
  {/if}
  <div class="legend">
    {#each series as item, index}
      <span class="legend-item">
        <span class="swatch" style:background={colors[index % colors.length]}></span>
        {item.key}{item.is_other ? " (combined)" : ""}
      </span>
    {/each}
  </div>
  {#if hoverT != null && cursor && hoverRows.length > 0}
    <div class="tooltip" style={tooltipStyle}>
      <div class="tooltip-time">{labelFor(hoverT)}</div>
      {#each hoverRows as row}
        <div class="tooltip-row">
          <span class="swatch" style:background={row.color}></span>
          <span class="tooltip-key">{row.key}{row.isOther ? " (combined)" : ""}</span>
          {#if row.p50 == null}
            <span class="tooltip-value muted">n={row.n} · insufficient</span>
          {:else}
            <span class="tooltip-value">
              {formatRate(row.p50)} tok/s
              <span class="muted">p95 {row.p95 == null ? "-" : formatRate(row.p95)} · n={row.n}</span>
            </span>
          {/if}
        </div>
      {/each}
    </div>
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

  .crosshair {
    stroke: var(--border-default);
    stroke-dasharray: 3 3;
  }

  .overlay {
    fill: transparent;
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

  .value-label {
    font-size: 9.5px;
    font-weight: 600;
    text-anchor: middle;
  }

  .empty {
    display: grid;
    height: 300px;
    color: var(--text-muted);
    font-size: 13px;
    place-items: center;
  }

  .legend {
    display: flex;
    flex-wrap: wrap;
    gap: 4px 14px;
    padding: 8px 12px 10px;
    color: var(--text-secondary);
    font-size: 11px;
  }

  .legend-item {
    display: inline-flex;
    align-items: center;
    gap: 6px;
  }

  .swatch {
    display: inline-block;
    flex-shrink: 0;
    width: 10px;
    height: 3px;
    border-radius: 2px;
  }

  .tooltip {
    position: fixed;
    z-index: 100;
    min-width: 170px;
    padding: 8px 10px;
    border: 1px solid var(--border-default);
    border-radius: 6px;
    background: var(--bg-inset);
    color: var(--text-secondary);
    font-size: 11px;
    line-height: 1.5;
    pointer-events: none;
  }

  .tooltip-time {
    margin-bottom: 4px;
    color: var(--text-muted);
    font-size: 10px;
  }

  .tooltip-row {
    display: flex;
    align-items: center;
    gap: 6px;
    white-space: nowrap;
  }

  .tooltip-key {
    font-weight: 600;
  }

  .tooltip-value {
    margin-left: auto;
    padding-left: 10px;
  }

  .muted {
    color: var(--text-muted);
    font-weight: 400;
  }
</style>
