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
  downloadExport: vi.fn().mockResolvedValue(undefined),
  getMarkdownExportUrl: vi
    .fn()
    .mockReturnValue("/api/v1/sessions/sess-123/md"),
  copyToClipboard: vi.fn().mockResolvedValue(true),
  fetchBalance: vi.fn(),
}));

vi.mock("../../api/client.js", () => ({
  downloadExport: mocks.downloadExport,
  getMarkdownExportUrl: mocks.getMarkdownExportUrl,
}));

vi.mock("../../utils/clipboard.js", () => ({
  copyToClipboard: mocks.copyToClipboard,
}));

vi.mock("../../api/llm.js", () => ({
  fetchBalance: mocks.fetchBalance,
}));

import { sessions } from "../../stores/sessions.svelte.js";
import { ui } from "../../stores/ui.svelte.js";

// @ts-ignore
import AppHeader from "./AppHeader.svelte";

describe("AppHeader export actions", () => {
  let component: ReturnType<typeof mount> | undefined;
  const originalStorage = globalThis.localStorage;
  let store: Map<string, string>;

  beforeEach(() => {
    vi.clearAllMocks();
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
    mocks.fetchBalance.mockResolvedValue({ supported: false, available: false });
    sessions.activeSessionId = "sess-123";
    ui.isMobileViewport = false;
    ui.followLatest = false;
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

  it("copies markdown export link from export menu", async () => {
    component = mount(AppHeader, { target: document.body });
    await tick();

    const exportButton = document.querySelector<HTMLButtonElement>(
      'button[aria-label="Export session"]',
    );
    expect(exportButton).not.toBeNull();

    exportButton!.click();
    await tick();

    const copyButton = Array.from(
      document.querySelectorAll<HTMLButtonElement>("button"),
    ).find((button) =>
      button.textContent?.includes("Copy markdown export link"),
    );
    expect(copyButton).not.toBeNull();

    copyButton!.click();
    await tick();

    expect(mocks.getMarkdownExportUrl).toHaveBeenCalledWith("sess-123");
    expect(mocks.copyToClipboard).toHaveBeenCalledWith(
      "http://localhost:3000/api/v1/sessions/sess-123/md",
    );
  });

  it("toggles follow latest from the session header", async () => {
    component = mount(AppHeader, { target: document.body });
    await tick();

    const followButton = document.querySelector<HTMLButtonElement>(
      'button[aria-label="Follow latest messages"]',
    );
    expect(followButton).not.toBeNull();
    expect(followButton!.classList.contains("active")).toBe(false);

    followButton!.click();
    await tick();

    expect(ui.followLatest).toBe(true);
    expect(followButton!.classList.contains("active")).toBe(true);

    followButton!.click();
    await tick();

    expect(ui.followLatest).toBe(false);
    expect(followButton!.classList.contains("active")).toBe(false);
  });

  it("labels compact title-bar actions with hover hints", async () => {
    component = mount(AppHeader, { target: document.body });
    await tick();

    const moreButton = document.querySelector<HTMLButtonElement>(
      'button[aria-label="More navigation"]',
    );
    const shortcutsButton = document.querySelector<HTMLButtonElement>(
      'button[aria-label="Keyboard shortcuts"]',
    );

    expect(moreButton).not.toBeNull();
    expect(moreButton?.title).toBe("More navigation");
    expect(shortcutsButton).not.toBeNull();
    expect(shortcutsButton?.title).toBe("Keyboard shortcuts (?)");
  });

  it("renders supported LLM balance chip", async () => {
    mocks.fetchBalance.mockResolvedValueOnce({
      supported: true,
      currency: "CNY",
      amount: "12.34",
      available: true,
    });

    component = mount(AppHeader, { target: document.body });
    await tick();
    await Promise.resolve();
    await tick();

    const chip = document.querySelector('[data-testid="llm-balance-chip"]');
    expect(chip).not.toBeNull();
    expect(chip?.textContent).toContain("¥12.34");
  });

  it("does not render unsupported or missing LLM balance", async () => {
    mocks.fetchBalance.mockResolvedValueOnce({
      supported: false,
      available: false,
    });

    component = mount(AppHeader, { target: document.body });
    await tick();
    await Promise.resolve();
    await tick();

    expect(document.querySelector('[data-testid="llm-balance-chip"]')).toBeNull();
  });

  it("skips balance fetch for remote connections", async () => {
    store.set("agentsview-server-url", "http://remote.test");

    component = mount(AppHeader, { target: document.body });
    await tick();

    expect(mocks.fetchBalance).not.toHaveBeenCalled();
    expect(document.querySelector('[data-testid="llm-balance-chip"]')).toBeNull();
  });
});
