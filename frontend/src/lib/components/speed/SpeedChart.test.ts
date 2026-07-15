// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { mount, tick, unmount } from "svelte";

// @ts-ignore Svelte components are compiled by the test runner.
import SpeedChart from "./SpeedChart.svelte";

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

  it("renders sparse buckets as a disconnected insufficient-data point", async () => {
    component = mount(SpeedChart, {
      target: document.body,
      props: {
        series: [
          {
            key: "claude",
            is_other: false,
            points: [
              { t: 1_752_562_800, p50: null, p95: null, n: 3 },
              { t: 1_752_566_400, p50: 10, p95: 12, n: 5 },
            ],
          },
        ],
      },
    });
    await tick();

    const sparsePoint = document.querySelector<SVGCircleElement>(
      'circle[aria-label*="insufficient data"]',
    );
    expect(sparsePoint).not.toBeNull();
    expect(sparsePoint?.getAttribute("cx")).toBe("52");
    expect(document.querySelector(".x-label")?.textContent).not.toContain("1970");
  });
});
