// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { mount, tick, unmount } from "svelte";

const mocks = vi.hoisted(() => ({
  fetchLLMProviders: vi.fn(),
  fetchConsolidateConfig: vi.fn(),
  saveLLMProviders: vi.fn(),
  saveConsolidateConfig: vi.fn(),
  setConsolidateEnabled: vi.fn(),
}));

vi.mock("../../api/llm", () => ({
  fetchLLMProviders: mocks.fetchLLMProviders,
  fetchConsolidateConfig: mocks.fetchConsolidateConfig,
  saveLLMProviders: mocks.saveLLMProviders,
  saveConsolidateConfig: mocks.saveConsolidateConfig,
}));

vi.mock("../../api/consolidate", () => ({
  setConsolidateEnabled: mocks.setConsolidateEnabled,
}));

// @ts-ignore
import ConsolidateSettings from "./ConsolidateSettings.svelte";

async function flush() {
  await Promise.resolve();
  await tick();
  await Promise.resolve();
  await tick();
}

describe("ConsolidateSettings", () => {
  let component: ReturnType<typeof mount> | undefined;
  const originalStorage = globalThis.localStorage;
  let store: Map<string, string>;

  beforeEach(() => {
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
    mocks.fetchLLMProviders.mockReset().mockResolvedValue({
      enabled: false,
      min_user_messages: 0,
      reenrich_msg_delta: 0,
      reenrich_idle_minutes: 0,
      concurrency: 0,
      periodic: false,
      has_api_key: false,
      embed: { has_api_key: false },
      providers: {
        "deepseek-chat": {
          enabled: true,
          base_url: "https://deepseek.example/v1",
          model: "deepseek-chat",
          has_api_key: true,
          api_key_preview: "1234",
        },
        "old-chat": {
          enabled: true,
          base_url: "https://old.example/v1",
          model: "old-model",
          has_api_key: false,
        },
      },
      usage: { consolidate: "deepseek-chat", embed: "old-chat" },
      usage_warnings: [],
    });
    mocks.fetchConsolidateConfig.mockReset().mockResolvedValue({
      enabled: false,
      interval: "24h0m0s",
    });
    mocks.saveLLMProviders.mockReset().mockImplementation(async (payload) => ({
      enabled: false,
      min_user_messages: 0,
      reenrich_msg_delta: 0,
      reenrich_idle_minutes: 0,
      concurrency: 0,
      periodic: false,
      has_api_key: false,
      embed: { has_api_key: false },
      providers: payload.providers,
      usage: payload.usage,
      usage_warnings: [],
    }));
    mocks.saveConsolidateConfig.mockReset().mockResolvedValue({
      enabled: false,
      interval: "90m0s",
    });
    mocks.setConsolidateEnabled.mockReset().mockResolvedValue({
      enabled: true,
      available: true,
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

  it("loads provider usage bindings and consolidate interval", async () => {
    component = mount(ConsolidateSettings, { target: document.body });
    await flush();

    expect(mocks.fetchLLMProviders).toHaveBeenCalledTimes(1);
    expect(mocks.fetchConsolidateConfig).toHaveBeenCalledTimes(1);
    expect(document.querySelector('[data-testid="provider-deepseek-chat"]')).toBeTruthy();
    const interval = document.querySelector<HTMLInputElement>('input[placeholder="24h"]');
    expect(interval?.value).toBe("24h");
    const usageSelects = Array.from(document.querySelectorAll<HTMLSelectElement>(".usage-grid select"));
    expect(usageSelects.some((select) => select.value === "deepseek-chat")).toBe(true);
  });

  it("renders dangling usage warnings returned by the backend", async () => {
    mocks.fetchLLMProviders.mockResolvedValueOnce({
      enabled: false,
      min_user_messages: 0,
      reenrich_msg_delta: 0,
      reenrich_idle_minutes: 0,
      concurrency: 0,
      periodic: false,
      has_api_key: false,
      embed: { has_api_key: false },
      providers: {
        "deepseek-chat": {
          enabled: true,
          base_url: "https://deepseek.example/v1",
          model: "deepseek-chat",
          has_api_key: false,
        },
      },
      usage: { consolidate: "missing-provider" },
      usage_warnings: ['usage "consolidate" references unknown provider "missing-provider"'],
    });

    component = mount(ConsolidateSettings, { target: document.body });
    await flush();

    expect(document.body.textContent).toContain('usage "consolidate" references unknown provider "missing-provider"');
  });

  it("saves provider edits, usage bindings, removed providers, and interval", async () => {
    component = mount(ConsolidateSettings, { target: document.body });
    await flush();

    const interval = document.querySelector<HTMLInputElement>('input[placeholder="24h"]')!;
    interval.value = "90m";
    interval.dispatchEvent(new InputEvent("input", { bubbles: true }));

    const consolidateSelect = Array.from(document.querySelectorAll<HTMLSelectElement>(".usage-grid select")).find(
      (select) => select.value === "deepseek-chat",
    )!;
    consolidateSelect.value = "old-chat";
    consolidateSelect.dispatchEvent(new Event("change", { bubbles: true }));

    const oldCard = document.querySelector('[data-testid="provider-old-chat"]')!;
    const remove = Array.from(oldCard.querySelectorAll("button")).find((button) =>
      button.textContent?.includes("Remove"),
    ) as HTMLButtonElement;
    remove.click();
    await flush();

    const save = Array.from(document.querySelectorAll("button")).find((button) =>
      button.textContent?.includes("Save consolidate settings"),
    ) as HTMLButtonElement;
    save.click();
    await flush();

    expect(mocks.saveLLMProviders).toHaveBeenCalledTimes(1);
    expect(mocks.saveLLMProviders).toHaveBeenCalledWith(
      expect.objectContaining({
        usage: expect.not.objectContaining({ embed: "old-chat" }),
        delete_providers: ["old-chat"],
      }),
    );
    expect(mocks.saveConsolidateConfig).toHaveBeenCalledWith({ interval: "90m" });
    expect(document.body.textContent).toContain("Consolidate settings saved");
  });

  it("persists legacy fallback by sending an explicit empty usage binding", async () => {
    component = mount(ConsolidateSettings, { target: document.body });
    await flush();

    const consolidateSelect = Array.from(document.querySelectorAll<HTMLSelectElement>(".usage-grid select")).find(
      (select) => select.value === "deepseek-chat",
    )!;
    consolidateSelect.value = "";
    consolidateSelect.dispatchEvent(new Event("change", { bubbles: true }));

    const save = Array.from(document.querySelectorAll("button")).find((button) =>
      button.textContent?.includes("Save consolidate settings"),
    ) as HTMLButtonElement;
    save.click();
    await flush();

    expect(mocks.saveLLMProviders).toHaveBeenCalledWith(
      expect.objectContaining({
        usage: expect.objectContaining({ consolidate: "" }),
      }),
    );
  });

  it("validates interval before saving provider edits", async () => {
    mocks.saveConsolidateConfig.mockRejectedValueOnce(new Error("invalid consolidate interval"));
    component = mount(ConsolidateSettings, { target: document.body });
    await flush();

    const interval = document.querySelector<HTMLInputElement>('input[placeholder="24h"]')!;
    interval.value = "0s";
    interval.dispatchEvent(new InputEvent("input", { bubbles: true }));

    const save = Array.from(document.querySelectorAll("button")).find((button) =>
      button.textContent?.includes("Save consolidate settings"),
    ) as HTMLButtonElement;
    save.click();
    await flush();

    expect(mocks.saveConsolidateConfig).toHaveBeenCalledWith({ interval: "0s" });
    expect(mocks.saveLLMProviders).not.toHaveBeenCalled();
    expect(document.body.textContent).toContain("Failed to save consolidate settings");
  });

  it("toggles enabled state through the runtime endpoint and applies response", async () => {
    component = mount(ConsolidateSettings, { target: document.body });
    await flush();

    const toggle = Array.from(document.querySelectorAll("button")).find((button) =>
      button.textContent?.includes("Disabled"),
    ) as HTMLButtonElement;
    toggle.click();
    await flush();

    expect(mocks.setConsolidateEnabled).toHaveBeenCalledWith(true);
    expect(toggle.textContent).toContain("Enabled");
  });
});
