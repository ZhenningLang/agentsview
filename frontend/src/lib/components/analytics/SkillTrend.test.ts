// @vitest-environment jsdom
import {
  afterEach,
  beforeEach,
  describe,
  expect,
  it,
  vi,
} from "vitest";
import { mount, tick, unmount } from "svelte";
// @ts-ignore
import SkillTrend from "./SkillTrend.svelte";
import { analytics } from "../../stores/analytics.svelte.js";

describe("SkillTrend", () => {
  beforeEach(() => {
    vi.stubGlobal(
      "ResizeObserver",
      class {
        observe() {}
        unobserve() {}
        disconnect() {}
      },
    );
  });

  afterEach(() => {
    analytics.skills = null;
    analytics.skillsGranularity = "week";
    analytics.loading = { ...analytics.loading, skills: false };
    analytics.querying = { ...analytics.querying, skills: false };
    // @ts-ignore
    analytics.errors = {
      ...analytics.errors,
      skills: null,
    };
    document.body.innerHTML = "";
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
  });

  function skillsResponse(
    trend: { date: string; by_skill: Record<string, number> }[],
  ) {
    return {
      total_skill_calls: 0,
      distinct_skills: 0,
      by_skill: [],
      trend,
    };
  }

  function mountWithData() {
    analytics.skills = skillsResponse([
      {
        date: "2024-01-01",
        by_skill: { commit: 4, review: 2 },
      },
      {
        date: "2024-01-08",
        by_skill: { commit: 6, deploy: 1 },
      },
    ]);
    // @ts-ignore
    analytics.errors = {
      ...analytics.errors,
      skills: null,
    };

    return mount(SkillTrend, { target: document.body });
  }

  it("renders one line per series with a legend", async () => {
    const component = mountWithData();
    await tick();

    expect(document.body.textContent).toContain("Skill Usage Over Time");
    const chips = document.querySelectorAll<HTMLButtonElement>(
      ".legend-chip",
    );
    expect(chips).toHaveLength(3);
    expect(chips[0]!.textContent).toContain("commit");
    expect(chips[0]!.textContent).toContain("10");
    expect(chips[1]!.textContent).toContain("review");
    expect(chips[2]!.textContent).toContain("deploy");
    expect(document.querySelectorAll(".series-line")).toHaveLength(3);
    expect(document.body.textContent).toContain("Jan 1");
    expect(document.body.textContent).toContain("Jan 8");

    await unmount(component);
  });

  it("hides a series line without changing survivor colors", async () => {
    const component = mountWithData();
    await tick();

    const lineStyles = () =>
      [...document.querySelectorAll<SVGPathElement>(".series-line")]
        .map((line) => line.getAttribute("style") ?? "");
    expect(lineStyles()[1]).toContain("--accent-blue");

    const chips = document.querySelectorAll<HTMLButtonElement>(
      ".legend-chip",
    );
    expect(chips[0]!.getAttribute("aria-pressed")).toBe("true");
    chips[0]!.click();
    await tick();

    expect(chips[0]!.getAttribute("aria-pressed")).toBe("false");
    expect(document.querySelectorAll(".series-line")).toHaveLength(2);
    expect(lineStyles()[0]).toContain("--accent-blue");

    await unmount(component);
  });

  it("folds skills past the series cap into Other", async () => {
    const bySkill: Record<string, number> = {};
    for (let i = 0; i < 8; i++) {
      bySkill[`skill-${i}`] = 8 - i;
    }
    analytics.skills = skillsResponse([
      { date: "2024-01-01", by_skill: bySkill },
      { date: "2024-01-08", by_skill: bySkill },
    ]);
    const component = mount(SkillTrend, { target: document.body });
    await tick();

    const chips = document.querySelectorAll<HTMLButtonElement>(
      ".legend-chip",
    );
    expect(chips).toHaveLength(7);
    expect(chips[6]!.textContent).toContain("Other");
    expect(chips[6]!.textContent).toContain("6");
    expect(document.querySelectorAll(".series-line")).toHaveLength(7);

    await unmount(component);
  });

  it("shows a crosshair tooltip listing every visible series", async () => {
    const component = mountWithData();
    await tick();

    const svg = document.querySelector<SVGElement>(".chart-svg")!;
    svg.dispatchEvent(
      new MouseEvent("mousemove", {
        bubbles: true,
        clientX: 0,
        clientY: 20,
      }),
    );
    await tick();

    const tooltip = document.querySelector(".tooltip")!;
    expect(tooltip).toBeTruthy();
    expect(tooltip.textContent).toContain("Jan 1, 2024");
    const rows = tooltip.querySelectorAll(".tooltip-row");
    expect(rows).toHaveLength(3);
    expect(rows[0]!.textContent).toContain("commit");
    expect(rows[0]!.textContent).toContain("4");
    expect(rows[1]!.textContent).toContain("review");
    expect(rows[2]!.textContent).toContain("deploy");
    expect(document.querySelectorAll(".crosshair")).toHaveLength(1);

    document.querySelector<HTMLElement>(".chart")!
      .dispatchEvent(new MouseEvent("mouseleave"));
    await tick();
    expect(document.querySelector(".tooltip")).toBeNull();

    await unmount(component);
  });

  it("exposes trend buckets to keyboard and assistive technology", async () => {
    const component = mountWithData();
    await tick();

    const chart = document.querySelector<HTMLElement>(".chart")!;
    expect(chart.getAttribute("role")).toBe("slider");
    expect(chart.getAttribute("tabindex")).toBe("0");
    expect(chart.getAttribute("aria-describedby")).toBe(
      "skill-trend-data",
    );
    const dataTable = document.querySelector("#skill-trend-data")!;
    expect(dataTable.textContent).toContain("commit");
    expect(dataTable.textContent).toContain("Jan 1, 2024");
    expect(dataTable.textContent).toContain("4");

    chart.dispatchEvent(new FocusEvent("focus"));
    await tick();
    expect(document.querySelector(".tooltip-date")?.textContent)
      .toContain("Jan 1, 2024");

    chart.dispatchEvent(
      new KeyboardEvent("keydown", {
        key: "ArrowRight",
        bubbles: true,
      }),
    );
    await tick();
    expect(document.querySelector(".tooltip-date")?.textContent)
      .toContain("Jan 8, 2024");

    await unmount(component);
  });

  it("requests granularity changes through the shared picker", async () => {
    const fetchSpy = vi
      .spyOn(analytics, "fetchSkills")
      .mockResolvedValue(undefined);
    const component = mountWithData();
    await tick();

    const monthBtn = [
      ...document.querySelectorAll<HTMLButtonElement>(
        ".trend-header button",
      ),
    ].find((button) => button.textContent?.trim() === "Month");
    expect(monthBtn).toBeTruthy();
    monthBtn!.click();
    await tick();

    expect(fetchSpy).toHaveBeenCalledOnce();
    expect(fetchSpy).toHaveBeenCalledWith("month");

    await unmount(component);
  });

  it("disables granularity picker while skills request is pending", async () => {
    analytics.skills = skillsResponse([
      { date: "2024-01-01", by_skill: { commit: 1 } },
    ]);
    analytics.querying = { ...analytics.querying, skills: true };
    const component = mount(SkillTrend, { target: document.body });
    await tick();

    const buttons = document.querySelectorAll<HTMLButtonElement>(
      ".trend-header button",
    );
    expect([...buttons].every((button) => button.disabled)).toBe(true);

    await unmount(component);
  });

  it("renders empty and error states", async () => {
    analytics.skills = skillsResponse([]);
    const emptyComponent = mount(SkillTrend, { target: document.body });
    await tick();
    expect(document.body.textContent).toContain("No skill usage data");
    await unmount(emptyComponent);
    document.body.innerHTML = "";

    analytics.skills = null;
    // @ts-ignore
    analytics.errors = {
      ...analytics.errors,
      skills: "Failed to load",
    };
    const retrySpy = vi
      .spyOn(analytics, "fetchSkills")
      .mockResolvedValue(undefined);
    const errorComponent = mount(SkillTrend, { target: document.body });
    await tick();

    expect(document.body.textContent).toContain("Failed to load");
    document.querySelector<HTMLButtonElement>(".retry-btn")!.click();
    await tick();
    expect(retrySpy).toHaveBeenCalledOnce();

    await unmount(errorComponent);
  });
});
