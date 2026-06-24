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

const mocks = vi.hoisted(() => ({
  fetchEnrichStatus: vi.fn(),
  triggerEnrich: vi.fn(),
}));

vi.mock("../../api/llm.js", () => ({
  fetchEnrichStatus: mocks.fetchEnrichStatus,
  triggerEnrich: mocks.triggerEnrich,
}));

import { sync } from "../../stores/sync.svelte.js";

// @ts-ignore
import LLMEnrichmentSettings from "./LLMEnrichmentSettings.svelte";

async function flush() {
  await Promise.resolve();
  await tick();
}

describe("LLMEnrichmentSettings", () => {
  let component: ReturnType<typeof mount> | undefined;
  const originalStorage = globalThis.localStorage;
  let store: Map<string, string>;

  beforeEach(() => {
    sync.serverVersion = null;
    store = new Map();
    Object.defineProperty(globalThis, "localStorage", {
      value: {
        getItem: vi.fn((key: string) => store.get(key) ?? null),
        setItem: vi.fn((key: string, value: string) => {
          store.set(key, value);
        }),
        removeItem: vi.fn((key: string) => {
          store.delete(key);
        }),
        clear: vi.fn(() => {
          store.clear();
        }),
      },
      writable: true,
      configurable: true,
    });
    mocks.fetchEnrichStatus.mockReset().mockResolvedValue({
      total: 10,
      enriched: 4,
      pending: 3,
      skipped_too_short: 1,
      no_content: 1,
      errors: 1,
      by_status: { ok: 4 },
    });
    mocks.triggerEnrich.mockReset().mockResolvedValue({
      enriched: 2,
      skipped: 1,
      no_content: 0,
      errors: 0,
      candidates: 3,
      elapsed_ms: 20,
    });
  });

  afterEach(() => {
    if (component) {
      unmount(component);
      component = undefined;
    }
    document.body.innerHTML = "";
    Object.defineProperty(globalThis, "localStorage", {
      value: originalStorage,
      writable: true,
      configurable: true,
    });
  });

  it("loads and renders enrichment status counts", async () => {
    component = mount(LLMEnrichmentSettings, { target: document.body });
    await flush();

    expect(mocks.fetchEnrichStatus).toHaveBeenCalledTimes(1);
    expect(document.body.textContent).toContain("Total");
    expect(document.body.textContent).toContain("10");
    expect(document.body.textContent).toContain("Enriched");
    expect(document.body.textContent).toContain("4");
    expect(document.body.textContent).toContain("Pending");
    expect(document.body.textContent).toContain("3");
  });

  it("triggers enrichment and refreshes status", async () => {
    component = mount(LLMEnrichmentSettings, { target: document.body });
    await flush();

    const button = Array.from(document.querySelectorAll("button")).find((btn) =>
      btn.textContent?.includes("Run enrichment"),
    ) as HTMLButtonElement | undefined;
    expect(button).toBeTruthy();

    button!.click();
    await flush();

    expect(mocks.triggerEnrich).toHaveBeenCalledWith({ limit: 25 });
    expect(mocks.fetchEnrichStatus).toHaveBeenCalledTimes(2);
    expect(document.body.textContent).toContain("Enriched 2 of 3 candidates");
  });

  it("surfaces backend trigger errors", async () => {
    mocks.triggerEnrich.mockRejectedValueOnce(new Error("LLM is disabled"));
    component = mount(LLMEnrichmentSettings, { target: document.body });
    await flush();

    const button = Array.from(document.querySelectorAll("button")).find((btn) =>
      btn.textContent?.includes("Run enrichment"),
    ) as HTMLButtonElement | undefined;
    button!.click();
    await flush();

    expect(document.querySelector('[role="alert"]')?.textContent).toContain(
      "Failed to trigger LLM enrichment",
    );
  });

  it("shows remote mode as unavailable and does not fetch", async () => {
    store.set("agentsview-server-url", "http://remote.test");
    component = mount(LLMEnrichmentSettings, { target: document.body });
    await flush();

    expect(mocks.fetchEnrichStatus).not.toHaveBeenCalled();
    expect(document.body.textContent).toContain("local server connection");
    expect(document.body.textContent).not.toContain("Run enrichment");
  });

  it("disables the trigger in read-only mode and does not call enrichment", async () => {
    sync.serverVersion = {
      version: "test",
      commit: "abc123",
      build_date: "2026-06-24T00:00:00Z",
      read_only: true,
    };
    component = mount(LLMEnrichmentSettings, { target: document.body });
    await flush();

    const button = Array.from(document.querySelectorAll("button")).find((btn) =>
      btn.textContent?.includes("Run enrichment"),
    ) as HTMLButtonElement | undefined;
    expect(button).toBeTruthy();
    expect(button!.disabled).toBe(true);
    expect(document.body.textContent).toContain("read-only backend");

    button!.click();
    await flush();

    expect(mocks.triggerEnrich).not.toHaveBeenCalled();
  });
});
