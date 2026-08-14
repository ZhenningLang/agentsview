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
import type { SessionTiming } from "../../api/types/timing.js";

const mocks = vi.hoisted(() => {
  const timing: SessionTiming = {
    session_id: "sess-1",
    total_duration_ms: 1200,
    tool_duration_ms: 0,
    turn_count: 1,
    tool_call_count: 0,
    subagent_count: 0,
    slowest_call: null,
    by_category: [],
    turns: [],
    running: false,
    speed: null,
  };

  return {
    fetchSessionTiming: vi.fn().mockResolvedValue(timing),
  };
});

vi.mock("../../api/timing.js", () => ({
  fetchSessionTiming: mocks.fetchSessionTiming,
}));

import { ui } from "../../stores/ui.svelte.js";
import { sessionTiming } from "../../stores/sessionTiming.svelte.js";
// @ts-ignore
import SessionVitals from "./SessionVitals.svelte";

describe("SessionVitals", () => {
  let component: ReturnType<typeof mount> | undefined;

  beforeEach(() => {
    sessionTiming.reset();
    ui.vitalsOpen = true;
  });

  afterEach(() => {
    if (component) {
      unmount(component);
      component = undefined;
    }
    sessionTiming.reset();
    ui.vitalsOpen = false;
    document.body.innerHTML = "";
  });

  it("has an obvious close control inside the analysis pane", async () => {
    component = mount(SessionVitals, {
      target: document.body,
      props: { sessionId: "sess-1" },
    });
    await tick();
    await tick();

    const closeButton = document.querySelector<HTMLButtonElement>(
      'button[aria-label="Close session analysis"]',
    );

    expect(closeButton).not.toBeNull();
    expect(closeButton?.title).toBe("Close session analysis");

    closeButton!.click();
    await tick();

    expect(ui.vitalsOpen).toBe(false);
  });

  it("shows approximate output speed when the baseline exists", async () => {
    mocks.fetchSessionTiming.mockResolvedValueOnce(timingWithSpeed({
      tok_per_sec: 20,
      sample_n: 5,
      baseline_p50: 20,
      baseline_n: 10,
    }));
    component = mount(SessionVitals, {
      target: document.body,
      props: { sessionId: "sess-1" },
    });
    await tick();
    await tick();
    expect(document.body.textContent).toContain("Output speed (approx.)");
    expect(document.body.textContent).toContain("20 tok/s (approx.) · normal vs 30d median");
  });

  it("keeps a measurable speed visible when its baseline is sparse", async () => {
    mocks.fetchSessionTiming.mockResolvedValueOnce(timingWithSpeed({
      tok_per_sec: 12,
      sample_n: 5,
      baseline_p50: null,
      baseline_n: 4,
    }));
    component = mount(SessionVitals, {
      target: document.body,
      props: { sessionId: "sess-1" },
    });
    await tick();
    await tick();
    expect(document.body.textContent).toContain("12 tok/s (approx.) · insufficient baseline");
  });

  describe("Calls disclosure", () => {
    async function mountWithCalls() {
      mocks.fetchSessionTiming.mockResolvedValueOnce(timingWithCalls());
      component = mount(SessionVitals, {
        target: document.body,
        props: { sessionId: "sess-1" },
      });
      await tick();
      await tick();
    }

    function callsHeader(): HTMLElement | null {
      return Array.from(document.querySelectorAll<HTMLElement>(".v-h")).find(
        (h) => h.textContent?.includes("Calls"),
      ) ?? null;
    }

    beforeEach(() => {
      ui.vitalsCallsExpanded = true;
    });

    afterEach(() => {
      ui.vitalsCallsExpanded = true;
    });

    it("renders the Calls header as an expanded disclosure button", async () => {
      await mountWithCalls();

      const header = callsHeader();
      expect(header).not.toBeNull();
      expect(header!.tagName).toBe("BUTTON");
      expect(header!.getAttribute("aria-expanded")).toBe("true");
      expect(header!.textContent).toContain("1 call");
      expect(document.querySelector(".scale-axis")).not.toBeNull();
      expect(document.querySelector(".calls")).not.toBeNull();
    });

    it("hides only the axis and rows when collapsed, then restores them", async () => {
      await mountWithCalls();

      callsHeader()!.click();
      await tick();

      expect(callsHeader()!.getAttribute("aria-expanded")).toBe("false");
      expect(callsHeader()!.textContent).toContain("1 call");
      expect(document.querySelector(".scale-axis")).toBeNull();
      expect(document.querySelector(".calls")).toBeNull();
      expect(ui.vitalsCallsExpanded).toBe(false);

      callsHeader()!.click();
      await tick();

      expect(callsHeader()!.getAttribute("aria-expanded")).toBe("true");
      expect(document.querySelector(".scale-axis")).not.toBeNull();
      expect(document.querySelector(".calls")).not.toBeNull();
    });

    it("starts collapsed when the stored preference is collapsed", async () => {
      ui.vitalsCallsExpanded = false;
      await mountWithCalls();

      expect(callsHeader()!.getAttribute("aria-expanded")).toBe("false");
      expect(document.querySelector(".calls")).toBeNull();
    });

    it("leaves the other analysis section headers as plain headers", async () => {
      await mountWithCalls();

      const timeline = Array.from(
        document.querySelectorAll<HTMLElement>(".v-h"),
      ).find((h) => h.textContent?.includes("Timeline"));
      expect(timeline).toBeDefined();
      expect(timeline!.tagName).toBe("HEADER");
    });
  });
});

function timingWithCalls(): SessionTiming {
  return {
    session_id: "sess-1",
    total_duration_ms: 1200,
    tool_duration_ms: 400,
    turn_count: 1,
    tool_call_count: 1,
    subagent_count: 0,
    slowest_call: null,
    by_category: [
      { category: "shell", duration_ms: 400, call_count: 1 },
    ],
    turns: [
      {
        message_id: 1,
        ordinal: 3,
        started_at: "2026-08-13T12:00:00Z",
        duration_ms: 400,
        primary_category: "shell",
        calls: [
          {
            tool_use_id: "tc-1",
            tool_name: "Bash",
            category: "shell",
            duration_ms: 400,
            is_parallel: false,
            input_preview: "ls -la",
          },
        ],
      },
    ],
    running: false,
    speed: null,
  };
}

function timingWithSpeed(speed: NonNullable<SessionTiming["speed"]>): SessionTiming {
  return {
    session_id: "sess-1",
    total_duration_ms: 1200,
    tool_duration_ms: 0,
    turn_count: 1,
    tool_call_count: 0,
    subagent_count: 0,
    slowest_call: null,
    by_category: [],
    turns: [],
    running: false,
    speed,
  };
}
