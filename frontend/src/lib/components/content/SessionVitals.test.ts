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
});

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
