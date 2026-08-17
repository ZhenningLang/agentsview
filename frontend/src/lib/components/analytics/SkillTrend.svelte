<script lang="ts">
  import { analytics } from "../../stores/analytics.svelte.js";
  import { parseLocalDate } from "../../utils/dates.js";
  import GranularityPicker from "../shared/GranularityPicker.svelte";

  const MAX_SERIES = 6;
  const OTHER_KEY = "__other__";
  const PLOT_HEIGHT = 120;
  const PLOT_TOP = 8;
  const LABEL_HEIGHT = 18;
  const SVG_HEIGHT = PLOT_TOP + PLOT_HEIGHT + LABEL_HEIGHT;
  const PAD_X = 10;
  const MAX_X_LABELS = 14;

  interface Series {
    key: string;
    label: string;
    total: number;
    colorIndex: number | null;
    values: number[];
  }

  const trendEntries = $derived(analytics.skills?.trend ?? []);

  const skillTotals = $derived.by(() => {
    const totals = new Map<string, number>();
    for (const entry of trendEntries) {
      for (const [skill, count] of Object.entries(entry.by_skill)) {
        totals.set(skill, (totals.get(skill) ?? 0) + count);
      }
    }
    return totals;
  });

  const topSkills = $derived.by(() => {
    return [...skillTotals.entries()]
      .sort((a, b) => b[1] - a[1] || a[0].localeCompare(b[0]))
      .slice(0, MAX_SERIES)
      .map(([skill]) => skill);
  });

  const otherTotal = $derived.by(() => {
    let total = 0;
    const top = new Set(topSkills);
    for (const [skill, count] of skillTotals) {
      if (!top.has(skill)) total += count;
    }
    return total;
  });

  const allSeries = $derived.by(() => {
    const series: Series[] = topSkills.map((skill, i) => ({
      key: skill,
      label: skill,
      total: skillTotals.get(skill) ?? 0,
      colorIndex: i,
      values: trendEntries.map((entry) => entry.by_skill[skill] ?? 0),
    }));
    if (otherTotal > 0) {
      const top = new Set(topSkills);
      series.push({
        key: OTHER_KEY,
        label: "Other",
        total: otherTotal,
        colorIndex: null,
        values: trendEntries.map((entry) => {
          let sum = 0;
          for (const [skill, count] of Object.entries(entry.by_skill)) {
            if (!top.has(skill)) sum += count;
          }
          return sum;
        }),
      });
    }
    return series;
  });

  let hiddenKeys = $state<string[]>([]);
  const visibleSeries = $derived(
    allSeries.filter((series) => !hiddenKeys.includes(series.key)),
  );

  const maxValue = $derived.by(() => {
    let max = 1;
    for (const series of visibleSeries) {
      for (const value of series.values) {
        if (value > max) max = value;
      }
    }
    return max;
  });

  let measuredWidth = $state(0);
  const chartWidth = $derived(measuredWidth > 0 ? measuredWidth : 600);
  let hoverIndex = $state<number | null>(null);
  let tooltipPos = $state<{ x: number; y: number } | null>(null);

  function toggleSeries(key: string) {
    hiddenKeys = hiddenKeys.includes(key)
      ? hiddenKeys.filter((item) => item !== key)
      : [...hiddenKeys, key];
  }

  function xAt(index: number): number {
    const count = trendEntries.length;
    const span = Math.max(chartWidth - 2 * PAD_X, 0);
    if (count <= 1) return PAD_X + span / 2;
    return PAD_X + (index * span) / (count - 1);
  }

  function yAt(value: number): number {
    return PLOT_TOP + PLOT_HEIGHT - (value / maxValue) * PLOT_HEIGHT;
  }

  function linePath(values: number[]): string {
    return values
      .map((value, index) => {
        const cmd = index === 0 ? "M" : "L";
        return `${cmd}${xAt(index).toFixed(1)},${yAt(value).toFixed(1)}`;
      })
      .join(" ");
  }

  function seriesColor(colorIndex: number | null): string {
    const colors = [
      "var(--accent-green)",
      "var(--accent-blue)",
      "var(--accent-amber)",
      "var(--accent-purple)",
      "var(--accent-coral)",
      "var(--accent-indigo)",
    ];
    if (colorIndex === null) return "var(--text-muted)";
    return colors[colorIndex] ?? "var(--text-muted)";
  }

  const labelStep = $derived(
    Math.max(Math.ceil(trendEntries.length / MAX_X_LABELS), 1),
  );

  function formatDate(value: string, opts: Intl.DateTimeFormatOptions): string {
    const parsed = parseLocalDate(value);
    if (!parsed) return value;
    return parsed.toLocaleDateString(undefined, opts);
  }

  function bucketLabel(date: string, index: number): string {
    if (index % labelStep !== 0) return "";
    if (analytics.skillsGranularity === "month") {
      return formatDate(date, { year: "numeric", month: "short" });
    }
    return formatDate(date, { month: "short", day: "numeric" });
  }

  function bucketDateLabel(date: string): string {
    return formatDate(date, { year: "numeric", month: "short", day: "numeric" });
  }

  function labelAnchor(index: number): string {
    const x = xAt(index);
    if (x < 30) return "start";
    if (x > chartWidth - 30) return "end";
    return "middle";
  }

  function handleMove(e: MouseEvent) {
    const count = trendEntries.length;
    if (count === 0 || chartWidth <= 0) return;
    const rect = (e.currentTarget as SVGElement).getBoundingClientRect();
    const x = e.clientX - rect.left;
    const span = Math.max(chartWidth - 2 * PAD_X, 1);
    const index = Math.min(
      Math.max(Math.round(((x - PAD_X) / span) * (count - 1)), 0),
      count - 1,
    );
    hoverIndex = index;
    tooltipPos = { x: rect.left + xAt(index), y: rect.top + PLOT_TOP - 6 };
  }

  function handleLeave() {
    hoverIndex = null;
    tooltipPos = null;
  }

  function setKeyboardHover(element: HTMLElement, index: number) {
    const rect = element.getBoundingClientRect();
    hoverIndex = index;
    tooltipPos = { x: rect.left + xAt(index), y: rect.top + PLOT_TOP - 6 };
  }

  function handleFocus(e: FocusEvent) {
    if (trendEntries.length === 0) return;
    setKeyboardHover(e.currentTarget as HTMLElement, 0);
  }

  function handleKeydown(e: KeyboardEvent) {
    if (trendEntries.length === 0) return;
    if (e.key !== "ArrowLeft" && e.key !== "ArrowRight") return;
    e.preventDefault();
    const delta = e.key === "ArrowRight" ? 1 : -1;
    const index = Math.min(
      Math.max((hoverIndex ?? 0) + delta, 0),
      trendEntries.length - 1,
    );
    setKeyboardHover(e.currentTarget as HTMLElement, index);
  }

  const hoverReadout = $derived.by(() => {
    if (hoverIndex === null) return [];
    return visibleSeries
      .map((series) => ({
        key: series.key,
        label: series.label,
        colorIndex: series.colorIndex,
        value: series.values[hoverIndex ?? 0] ?? 0,
      }))
      .sort((a, b) => b.value - a.value);
  });
</script>

<div class="trend-container">
  <div class="trend-header">
    <h3 class="chart-title">Skill Usage Over Time</h3>
    <GranularityPicker
      value={analytics.skillsGranularity}
      onChange={(granularity) => analytics.setSkillsGranularity(granularity)}
      disabled={analytics.querying.skills}
    />
  </div>

  {#if analytics.errors.skills}
    <div class="error">
      {analytics.errors.skills}
      <button class="retry-btn" onclick={() => analytics.fetchSkills()}>
        Retry
      </button>
    </div>
  {:else if analytics.loading.skills && trendEntries.length === 0}
    <div class="empty">Loading skill usage trend...</div>
  {:else if trendEntries.length > 0 && allSeries.length > 0}
    <div class="legend" role="group" aria-label="Skill trend legend">
      {#each allSeries as series (series.key)}
        <button
          class="legend-chip"
          class:hidden-series={hiddenKeys.includes(series.key)}
          aria-pressed={!hiddenKeys.includes(series.key)}
          onclick={() => toggleSeries(series.key)}
        >
          <span
            class="legend-key"
            style="background: {seriesColor(series.colorIndex)}"
          ></span>
          <span class="legend-name">{series.label}</span>
          <span class="legend-count">{series.total.toLocaleString()}</span>
        </button>
      {/each}
    </div>

    <div
      class="chart"
      bind:clientWidth={measuredWidth}
      role="slider"
      tabindex="0"
      aria-label="Skill usage trend chart"
      aria-describedby="skill-trend-data"
      aria-valuemin="0"
      aria-valuemax={Math.max(trendEntries.length - 1, 0)}
      aria-valuenow={hoverIndex ?? 0}
      aria-valuetext={bucketDateLabel(trendEntries[hoverIndex ?? 0]?.date ?? "")}
      onmousemove={handleMove}
      onmouseleave={handleLeave}
      onfocus={handleFocus}
      onblur={handleLeave}
      onkeydown={handleKeydown}
    >
      <svg width={chartWidth} height={SVG_HEIGHT} class="chart-svg" aria-hidden="true">
        <line
          class="baseline"
          x1={PAD_X}
          y1={PLOT_TOP + PLOT_HEIGHT}
          x2={chartWidth - PAD_X}
          y2={PLOT_TOP + PLOT_HEIGHT}
        />

        {#each visibleSeries as series (series.key)}
          {#if trendEntries.length > 1}
            <path
              class="series-line"
              d={linePath(series.values)}
              style="stroke: {seriesColor(series.colorIndex)}"
            />
          {:else}
            <circle
              class="series-marker"
              cx={xAt(0)}
              cy={yAt(series.values[0] ?? 0)}
              r="4"
              style="fill: {seriesColor(series.colorIndex)}"
            />
          {/if}
        {/each}

        {#if hoverIndex !== null}
          <line
            class="crosshair"
            x1={xAt(hoverIndex)}
            y1={PLOT_TOP}
            x2={xAt(hoverIndex)}
            y2={PLOT_TOP + PLOT_HEIGHT}
          />
          {#each visibleSeries as series (series.key)}
            <circle
              class="series-marker"
              cx={xAt(hoverIndex)}
              cy={yAt(series.values[hoverIndex] ?? 0)}
              r="4"
              style="fill: {seriesColor(series.colorIndex)}"
            />
          {/each}
        {/if}

        {#each trendEntries as entry, index (entry.date)}
          {@const label = bucketLabel(entry.date, index)}
          {#if label}
            <text
              class="x-label"
              x={xAt(index)}
              y={SVG_HEIGHT - 4}
              text-anchor={labelAnchor(index)}
            >
              {label}
            </text>
          {/if}
        {/each}
      </svg>
      <table id="skill-trend-data" class="sr-only">
        <caption>Skill Usage Over Time</caption>
        <thead>
          <tr>
            <th scope="col">Date</th>
            {#each allSeries as series (series.key)}
              <th scope="col">{series.label}</th>
            {/each}
          </tr>
        </thead>
        <tbody>
          {#each trendEntries as entry, index (entry.date)}
            <tr>
              <th scope="row">{bucketDateLabel(entry.date)}</th>
              {#each allSeries as series (series.key)}
                <td>{(series.values[index] ?? 0).toLocaleString()}</td>
              {/each}
            </tr>
          {/each}
        </tbody>
      </table>
    </div>

    {#if hoverIndex !== null && tooltipPos}
      <div
        class="tooltip"
        role="status"
        aria-live="polite"
        style="left: {tooltipPos.x}px; top: {tooltipPos.y}px;"
      >
        <div class="tooltip-date">
          {bucketDateLabel(trendEntries[hoverIndex]?.date ?? "")}
        </div>
        {#each hoverReadout as row (row.key)}
          <div class="tooltip-row">
            <span
              class="tip-key"
              style="background: {seriesColor(row.colorIndex)}"
            ></span>
            <span class="tip-value">{row.value.toLocaleString()}</span>
            <span class="tip-name">{row.label}</span>
          </div>
        {/each}
      </div>
    {/if}
  {:else}
    <div class="empty">No skill usage data</div>
  {/if}
</div>

<style>
  .trend-container {
    position: relative;
    flex: 1;
  }

  .trend-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    margin-bottom: 8px;
    gap: 12px;
  }

  .chart-title {
    font-size: 12px;
    font-weight: 600;
    color: var(--text-primary);
  }

  .legend {
    display: flex;
    align-items: center;
    flex-wrap: wrap;
    gap: 4px;
    margin-bottom: 10px;
  }

  .legend-chip {
    display: inline-flex;
    align-items: center;
    gap: var(--space-2);
    max-width: 220px;
    padding: 2px 7px;
    border: 1px solid var(--border-muted);
    border-radius: var(--radius-sm);
    background: var(--bg-inset);
    color: var(--text-secondary);
    font-size: 10px;
    cursor: pointer;
  }

  .legend-chip:hover {
    background: var(--bg-surface-hover);
  }

  .legend-chip.hidden-series {
    opacity: 0.45;
  }

  .legend-key,
  .tip-key {
    flex-shrink: 0;
    width: 10px;
    height: 2px;
    border-radius: 1px;
  }

  .legend-name {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .legend-count,
  .tip-value {
    font-family: var(--font-mono);
    color: var(--text-muted);
  }

  .chart {
    width: 100%;
  }

  .chart-svg {
    display: block;
  }

  .baseline {
    stroke: var(--border-muted);
    stroke-width: 1;
  }

  .series-line {
    fill: none;
    stroke-width: 2;
    stroke-linecap: round;
    stroke-linejoin: round;
  }

  .series-marker {
    stroke: var(--bg-surface);
    stroke-width: 2;
    pointer-events: none;
  }

  .crosshair {
    stroke: var(--text-muted);
    stroke-width: 1;
    opacity: 0.5;
    pointer-events: none;
  }

  .x-label {
    font-size: 8px;
    fill: var(--text-muted);
  }

  .tooltip {
    position: fixed;
    transform: translateX(-50%) translateY(-100%);
    padding: 5px 8px;
    background: var(--text-primary);
    color: var(--bg-primary);
    font-size: 10px;
    border-radius: var(--radius-sm);
    white-space: nowrap;
    pointer-events: none;
    z-index: 100;
  }

  .tooltip-date {
    font-weight: 600;
    margin-bottom: 3px;
  }

  .tooltip-row {
    display: flex;
    align-items: center;
    gap: var(--space-2);
  }

  .tip-value {
    min-width: 20px;
    text-align: right;
    font-weight: 600;
  }

  .tip-name {
    opacity: 0.8;
  }

  .empty {
    color: var(--text-muted);
    font-size: 12px;
    padding: 24px;
    text-align: center;
  }

  .error {
    color: var(--accent-red);
    font-size: 12px;
    padding: 12px;
    display: flex;
    align-items: center;
    gap: 8px;
  }

  .retry-btn {
    padding: 2px 8px;
    border: 1px solid currentColor;
    border-radius: var(--radius-sm);
    font-size: 11px;
    color: inherit;
    cursor: pointer;
  }
</style>
