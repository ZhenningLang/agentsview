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
import { router } from "../../stores/router.svelte.js";
import { setLocale } from "../../i18n/index.svelte.js";
import type { Session } from "../../api/types.js";

// @ts-ignore
import AppHeader from "./AppHeader.svelte";

function testSession(overrides: Partial<Session> = {}): Session {
  return {
    id: "sess-123",
    project: "agentsview",
    machine: "test-machine",
    agent: "claude",
    first_message: "Synthetic test session",
    started_at: "2026-08-13T12:00:00Z",
    ended_at: "2026-08-13T12:05:00Z",
    message_count: 2,
    user_message_count: 1,
    total_output_tokens: 0,
    peak_context_tokens: 0,
    is_automated: false,
    created_at: "2026-08-13T12:00:00Z",
    ...overrides,
  };
}

function menuButtonByText(text: string): HTMLButtonElement | undefined {
  return Array.from(
    document.querySelectorAll<HTMLButtonElement>("button"),
  ).find((button) => button.textContent?.includes(text));
}

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
    sessions.sessions = [testSession()];
    ui.isMobileViewport = false;
    ui.followLatest = false;
    setLocale("en");
  });

  afterEach(() => {
    if (component) {
      unmount(component);
      component = undefined;
    }
    sessions.sessions = [];
    setLocale("en");
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

  it("copies the active session source path from the export menu", async () => {
    sessions.sessions = [
      testSession({
        file_path: "/tmp/agentsview/sessions/session 123.jsonl",
      }),
    ];

    component = mount(AppHeader, { target: document.body });
    await tick();

    document
      .querySelector<HTMLButtonElement>('button[aria-label="Export session"]')!
      .click();
    await tick();

    const copyPathButton = menuButtonByText("Copy source file path");
    expect(copyPathButton).toBeDefined();

    copyPathButton!.click();
    await tick();
    await Promise.resolve();
    await tick();

    expect(mocks.copyToClipboard).toHaveBeenCalledWith(
      "/tmp/agentsview/sessions/session 123.jsonl",
    );
    // Menu closes on success.
    expect(menuButtonByText("Copy source file path")).toBeUndefined();
  });

  it("copies the active session source path from the narrow overflow menu", async () => {
    sessions.sessions = [
      testSession({ file_path: "/var/log/agent/session.jsonl" }),
    ];

    component = mount(AppHeader, { target: document.body });
    await tick();

    document
      .querySelector<HTMLButtonElement>('button[aria-label="More actions"]')!
      .click();
    await tick();

    const copyPathButton = menuButtonByText("Copy source file path");
    expect(copyPathButton).toBeDefined();

    copyPathButton!.click();
    await tick();
    await Promise.resolve();
    await tick();

    expect(mocks.copyToClipboard).toHaveBeenCalledWith(
      "/var/log/agent/session.jsonl",
    );
    expect(menuButtonByText("Copy source file path")).toBeUndefined();
  });

  it("localizes the source path action", async () => {
    setLocale("zh");
    sessions.sessions = [
      testSession({ file_path: "/var/log/agent/session.jsonl" }),
    ];

    component = mount(AppHeader, { target: document.body });
    await tick();

    document
      .querySelector<HTMLButtonElement>('button[aria-label="Export session"]')!
      .click();
    await tick();

    expect(menuButtonByText("Copy source file path")).toBeUndefined();
    expect(menuButtonByText("复制源文件路径")).toBeDefined();
  });

  it("hides the source path action when the session has no on-disk path", async () => {
    sessions.sessions = [testSession({ file_path: "" })];

    component = mount(AppHeader, { target: document.body });
    await tick();

    document
      .querySelector<HTMLButtonElement>('button[aria-label="Export session"]')!
      .click();
    await tick();
    expect(menuButtonByText("Copy source file path")).toBeUndefined();

    document
      .querySelector<HTMLButtonElement>('button[aria-label="More actions"]')!
      .click();
    await tick();
    expect(menuButtonByText("Copy source file path")).toBeUndefined();
  });

  it("hides the source path action when the active session is not hydrated", async () => {
    sessions.sessions = [
      testSession({
        file_path: "/tmp/skinny.jsonl",
        is_index_only: true,
      }),
    ];

    component = mount(AppHeader, { target: document.body });
    await tick();

    document
      .querySelector<HTMLButtonElement>('button[aria-label="Export session"]')!
      .click();
    await tick();

    expect(menuButtonByText("Copy source file path")).toBeUndefined();
  });

  it("does not report success or close the menu when the clipboard write fails", async () => {
    mocks.copyToClipboard.mockResolvedValueOnce(false);
    sessions.sessions = [
      testSession({ file_path: "/tmp/agentsview/no-clipboard.jsonl" }),
    ];

    component = mount(AppHeader, { target: document.body });
    await tick();

    document
      .querySelector<HTMLButtonElement>('button[aria-label="Export session"]')!
      .click();
    await tick();

    menuButtonByText("Copy source file path")!.click();
    await tick();
    await Promise.resolve();
    await tick();

    expect(mocks.copyToClipboard).toHaveBeenCalledWith(
      "/tmp/agentsview/no-clipboard.jsonl",
    );
    // Still open, still showing the idle label — no fake success.
    expect(menuButtonByText("Copy source file path")).toBeDefined();
    expect(menuButtonByText("Copied source file path")).toBeUndefined();
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

  it("keeps transcript mode controls visually compact", async () => {
    component = mount(AppHeader, { target: document.body });
    await tick();

    const normal = document.querySelector<HTMLButtonElement>(
      'button[aria-label="Normal transcript mode"]',
    );
    const focused = document.querySelector<HTMLButtonElement>(
      'button[aria-label="Focused transcript mode"]',
    );

    expect(normal).not.toBeNull();
    expect(focused).not.toBeNull();
    expect(normal?.textContent?.trim()).toBe("N");
    expect(focused?.textContent?.trim()).toBe("F");
    expect(normal?.textContent).not.toContain("Normal");
    expect(focused?.textContent).not.toContain("Focused");
    expect(normal?.title).toContain("show all messages");
    expect(focused?.title).toContain("user prompts and final answers");
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

  // Without layout (jsdom reports 0 widths) the overflow guard keeps every item
  // inline, so the full primary-nav order is observable here.
  it("renders the primary nav inline in the intended order", async () => {
    component = mount(AppHeader, { target: document.body });
    await tick();

    const labels = Array.from(
      document.querySelectorAll<HTMLButtonElement>(".nav-row > .nav-btn"),
    ).map((b) => b.getAttribute("aria-label"));

    expect(labels).toEqual([
      "Sessions",
      "Usage",
	  "Speed",
      "Memory",
      "Vault",
      "Skills",
      "Trends",
      "Pinned",
      "Insights",
      "Trash",
    ]);

    // Everything fits, so the More menu is collapsed.
    const moreWrap = document.querySelector(".nav-row .more-wrap");
    expect(moreWrap?.classList.contains("nav-hidden")).toBe(true);
  });

  it("navigates when a promoted nav item is clicked", async () => {
    component = mount(AppHeader, { target: document.body });
    await tick();

    const memory = document.querySelector<HTMLButtonElement>(
      '.nav-row > button[aria-label="Memory"]',
    );
    expect(memory).not.toBeNull();

    memory!.click();
    await tick();

    expect(router.route).toBe("memory");
    expect(memory!.classList.contains("active")).toBe(true);
  });
});
