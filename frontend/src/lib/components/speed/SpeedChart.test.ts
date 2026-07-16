// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { mount, tick, unmount } from "svelte";

// @ts-ignore Svelte components are compiled by the test runner.
import SpeedChart from "./SpeedChart.svelte";

const sparseSeries = [
  {
    key: "claude",
    is_other: false,
    points: [
      { t: 1_752_562_800, p50: null, p95: null, n: 3 },
      { t: 1_752_566_400, p50: 10, p95: 12, n: 5 },
      { t: 1_752_570_000, p50: 120.4, p95: 150, n: 9 },
    ],
  },
  {
    key: "kilo",
    is_other: false,
    points: [{ t: 1_752_566_400, p50: 4.2, p95: 6, n: 7 }],
  },
];

describe("SpeedChart", () => {
  let component: ReturnType<typeof mount> | undefined;

  beforeEach(() => {
    vi.stubGlobal(
      "ResizeObserver",
      class {
        observe() {}
        disconnect() {}
      },
    );
  });

  afterEach(() => {
    if (component) unmount(component);
    component = undefined;
    document.body.innerHTML = "";
    vi.unstubAllGlobals();
  });

  function mountChart(
    series: typeof sparseSeries,
    concurrency: { t: number; sessions: number }[] = [],
  ) {
    component = mount(SpeedChart, {
      target: document.body,
      props: { series, concurrency, bucketSec: 3600 },
    });
  }

  it("labels values directly on sparse charts and keeps a sane x axis", async () => {
    mountChart(sparseSeries);
    await tick();

    const labels = [...document.querySelectorAll(".value-label")].map(
      (el) => el.textContent?.trim(),
    );
    expect(labels).toEqual(["10.0", "120", "4.2"]);
    expect(document.querySelectorAll("circle.dot").length).toBe(3);
    expect(document.querySelector(".x-label")?.textContent).not.toContain(
      "1970",
    );
  });

  it("hides direct labels on dense charts", async () => {
    const points = Array.from({ length: 30 }, (_, index) => ({
      t: 1_752_562_800 + index * 3600,
      p50: 10 + index,
      p95: 20 + index,
      n: 12,
    }));
    mountChart([{ key: "claude", is_other: false, points }]);
    await tick();

    expect(document.querySelectorAll(".value-label").length).toBe(0);
  });

  it("renders a legend with one color swatch per series", async () => {
    mountChart(sparseSeries);
    await tick();

    const items = [...document.querySelectorAll(".legend-item")];
    expect(items.length).toBe(2);
    expect(items[0]?.textContent).toContain("claude");
    expect(items[1]?.textContent).toContain("kilo");
    expect(document.querySelectorAll(".legend-item .swatch").length).toBe(2);
  });

  it("shows a shared crosshair tooltip for all series at the hovered bucket", async () => {
    mountChart(sparseSeries);
    await tick();

    const overlay = document.querySelector<SVGRectElement>("rect.overlay");
    expect(overlay).not.toBeNull();
    overlay!.dispatchEvent(
      new MouseEvent("mousemove", { bubbles: true, clientX: 60, clientY: 60 }),
    );
    await tick();

    // jsdom reports offsetX 0, which resolves to the first bucket; the
    // tooltip must list every series with a point there.
    const tooltip = document.querySelector(".tooltip");
    expect(tooltip).not.toBeNull();
    expect(tooltip?.textContent).toContain("claude");
    expect(tooltip?.textContent).toContain("insufficient");
    expect(document.querySelector("line.crosshair")).not.toBeNull();

    overlay!.dispatchEvent(new MouseEvent("mouseleave"));
    await tick();
    expect(document.querySelector(".tooltip")).toBeNull();
  });

  it("renders concurrency bars for multi-session buckets and lists them in the tooltip", async () => {
    mountChart(sparseSeries, [
      { t: 1_752_562_800, sessions: 6 },
      { t: 1_752_566_400, sessions: 1 },
    ]);
    await tick();

    // Only the >=2 sessions bucket draws a bar; the solo bucket is noise.
    expect(document.querySelectorAll("rect.concurrency-bar").length).toBe(1);
    expect(document.querySelector(".legend-note")?.textContent).toContain(
      "parallel sessions",
    );

    const overlay = document.querySelector<SVGRectElement>("rect.overlay");
    overlay!.dispatchEvent(
      new MouseEvent("mousemove", { bubbles: true, clientX: 60, clientY: 60 }),
    );
    await tick();
    // jsdom offsetX resolves to the first bucket (t=1_752_562_800).
    expect(document.querySelector(".tooltip-sessions")?.textContent).toContain(
      "6 parallel sessions",
    );
  });
});
