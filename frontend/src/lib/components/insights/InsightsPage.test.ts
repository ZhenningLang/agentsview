// @vitest-environment jsdom
import {
  describe,
  it,
  expect,
  vi,
  beforeEach,
  afterEach,
} from "vitest";
import { mount, tick, unmount } from "svelte";

const mocks = vi.hoisted(() => ({
  downloadInsightExport: vi.fn().mockResolvedValue(undefined),
}));

vi.mock("../../api/client.js", async (importOriginal) => {
  const actual =
    await importOriginal<typeof import("../../api/client.js")>();
  return { ...actual, downloadInsightExport: mocks.downloadInsightExport };
});

import { insights } from "../../stores/insights.svelte.js";
import { sessions } from "../../stores/sessions.svelte.js";
import { ui } from "../../stores/ui.svelte.js";
import type { Insight } from "../../api/types.js";

// @ts-ignore
import InsightsPage from "./InsightsPage.svelte";

function testInsight(overrides: Partial<Insight> = {}): Insight {
  return {
    id: 77,
    type: "daily_activity",
    date_from: "2026-08-18",
    date_to: "2026-08-18",
    project: "agentsview",
    agent: "claude",
    model: "claude-opus-5",
    prompt: null,
    content: "# Phase 25 insight\n\nSynthetic body.",
    created_at: "2026-08-18T09:00:00Z",
    ...overrides,
  };
}

function actionButton(label: string): HTMLButtonElement | null {
  return document.querySelector<HTMLButtonElement>(
    `button[aria-label="${label}"]`,
  );
}

async function flush() {
  for (let i = 0; i < 10; i += 1) {
    await Promise.resolve();
    await tick();
  }
}

describe("Phase 25 InsightsPage export and publish actions", () => {
  let component: ReturnType<typeof mount> | undefined;
  const originalFetch = globalThis.fetch;
  // What the stubbed list endpoint returns. load() drops a selectedId that is
  // not in the loaded list, so the fixture has to arrive through the load.
  let listPayload: Insight[] = [];

  beforeEach(() => {
    vi.clearAllMocks();
    listPayload = [];
    // The page loads projects and insights on mount. Answer both from a stub
    // so nothing reaches a real server, then drive the store directly.
    globalThis.fetch = vi.fn(async (input: RequestInfo | URL) => {
      const url = String(
        typeof input === "string" || input instanceof URL
          ? input
          : input.url,
      );
      const body = url.includes("/projects")
        ? { projects: [] }
        : { insights: listPayload };
      return new Response(JSON.stringify(body), {
        status: 200,
        headers: { "content-type": "application/json" },
      });
    }) as typeof fetch;
    insights.items = [];
    insights.selectedId = null;
    insights.selectedTaskId = null;
    insights.tasks = [];
    sessions.activeSessionId = null;
    ui.activeModal = null;
    ui.publishTarget = null;
    ui.publishSecret = false;
    document.body.innerHTML = "";
  });

  afterEach(() => {
    if (component) {
      unmount(component);
      component = undefined;
    }
    globalThis.fetch = originalFetch;
    insights.items = [];
    insights.selectedId = null;
    sessions.activeSessionId = null;
    ui.activeModal = null;
    ui.publishTarget = null;
    ui.publishSecret = false;
    document.body.innerHTML = "";
  });

  async function mountWithSelection(
    item: Insight | null,
    rest: Insight[] = [],
  ) {
    listPayload = item ? [item, ...rest] : rest;
    component = mount(InsightsPage, { target: document.body });
    await flush();
    if (item) {
      insights.selectedId = item.id;
    }
    await flush();
  }

  it("shows export, publish and secret actions for the selected insight", async () => {
    await mountWithSelection(testInsight());

    for (const label of [
      "Export insight as HTML",
      "Publish insight as public Gist",
      "Publish insight as secret Gist",
    ]) {
      const button = actionButton(label);
      expect(button, label).not.toBeNull();
      expect(button!.type).toBe("button");
    }
  });

  it("exports the selected insight by id", async () => {
    await mountWithSelection(testInsight({ id: 91 }));

    actionButton("Export insight as HTML")!.click();
    await flush();

    expect(mocks.downloadInsightExport).toHaveBeenCalledWith(91);
  });

  it("opens a public gist publish for the selected insight", async () => {
    await mountWithSelection(testInsight());

    actionButton("Publish insight as public Gist")!.click();
    await tick();

    expect(ui.publishTarget).toEqual({ kind: "insight", id: 77 });
    expect(ui.publishSecret).toBe(false);
    expect(ui.activeModal).toBe("publish");
  });

  it("opens a secret gist publish for the selected insight", async () => {
    await mountWithSelection(testInsight());

    actionButton("Publish insight as secret Gist")!.click();
    await tick();

    expect(ui.publishTarget).toEqual({ kind: "insight", id: 77 });
    expect(ui.publishSecret).toBe(true);
    expect(ui.activeModal).toBe("publish");
  });

  it("targets the insight even when a session is active", async () => {
    sessions.activeSessionId = "sess-123";
    await mountWithSelection(testInsight({ id: 12 }));

    actionButton("Publish insight as public Gist")!.click();
    await tick();

    expect(ui.publishTarget).toEqual({ kind: "insight", id: 12 });
  });

  it("retargets publish when the selection changes", async () => {
    await mountWithSelection(testInsight({ id: 12 }), [
      testInsight({ id: 34 }),
    ]);
    insights.selectedId = 34;
    await flush();

    actionButton("Publish insight as public Gist")!.click();
    await tick();

    expect(ui.publishTarget).toEqual({ kind: "insight", id: 34 });
  });

  it("offers no insight actions without a selection", async () => {
    await mountWithSelection(null);

    expect(actionButton("Export insight as HTML")).toBeNull();
    expect(actionButton("Publish insight as public Gist")).toBeNull();
    expect(actionButton("Publish insight as secret Gist")).toBeNull();
    expect(mocks.downloadInsightExport).not.toHaveBeenCalled();
    expect(ui.activeModal).toBeNull();
    expect(ui.publishTarget).toBeNull();
  });

  it("keeps the delete action and rendered markdown alongside the new actions", async () => {
    await mountWithSelection(testInsight());

    expect(
      document.querySelector<HTMLButtonElement>("button.delete-btn"),
    ).not.toBeNull();
    const article = document.querySelector(".markdown-body");
    expect(article).not.toBeNull();
    expect(article!.querySelector("h1")?.textContent).toContain(
      "Phase 25 insight",
    );
  });
});
