// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { mount, tick, unmount } from "svelte";

const mocks = vi.hoisted(() => ({
  fetchLLMProviders: vi.fn(),
  saveLLMProviders: vi.fn(),
  isRemoteConnection: vi.fn(() => false),
}));

vi.mock("../../api/llm", () => ({
  fetchLLMProviders: mocks.fetchLLMProviders,
  saveLLMProviders: mocks.saveLLMProviders,
}));

vi.mock("../../api/runtime.js", () => ({
  isRemoteConnection: mocks.isRemoteConnection,
  ApiError: class ApiError extends Error {},
}));

// @ts-ignore - svelte component import
import LLMProviderSettings from "./LLMProviderSettings.svelte";
import { setLocale } from "../../i18n/index.svelte";

async function flush() {
  await Promise.resolve();
  await tick();
  await Promise.resolve();
  await tick();
}

function emptyResponse() {
  return { providers: {}, usage: {}, usage_warnings: [] };
}

describe("LLMProviderSettings", () => {
  let component: ReturnType<typeof mount> | undefined;
  let host: HTMLElement;

  beforeEach(() => {
    localStorage.clear();
    setLocale("en"); // deterministic English assertions
    host = document.createElement("div");
    document.body.appendChild(host);
    mocks.fetchLLMProviders.mockReset();
    mocks.saveLLMProviders.mockReset();
    mocks.isRemoteConnection.mockReturnValue(false);
  });

  afterEach(() => {
    if (component) unmount(component);
    component = undefined;
    host.remove();
  });

  it("shows the empty state when no providers are configured", async () => {
    mocks.fetchLLMProviders.mockResolvedValue(emptyResponse());
    component = mount(LLMProviderSettings, { target: host });
    await flush();
    expect(host.querySelector('[data-testid="provider-empty"]')).toBeTruthy();
    // all 5 usage rows still render, each with a Default option
    expect(host.querySelectorAll('[data-testid^="usage-"]').length).toBe(5);
  });

  it("renders configured providers and binds a usage to one", async () => {
    mocks.fetchLLMProviders.mockResolvedValue({
      providers: { "deepseek-chat": { enabled: true, base_url: "https://api.deepseek.com", model: "deepseek-chat", has_api_key: true, api_key_preview: "…ab" } },
      usage: { consolidate: "deepseek-chat" },
      usage_warnings: [],
    });
    component = mount(LLMProviderSettings, { target: host });
    await flush();
    expect(host.querySelector('[data-testid="provider-deepseek-chat"]')).toBeTruthy();
    const consolidateSelect = host.querySelector('[data-testid="usage-consolidate"] select') as HTMLSelectElement;
    expect(consolidateSelect.value).toBe("deepseek-chat");
    // default option exists
    const optionValues = Array.from(consolidateSelect.options).map((o) => o.value);
    expect(optionValues).toContain(""); // default
    expect(optionValues).toContain("deepseek-chat");
  });

  it("adds a provider via the add row", async () => {
    mocks.fetchLLMProviders.mockResolvedValue(emptyResponse());
    component = mount(LLMProviderSettings, { target: host });
    await flush();
    const input = host.querySelector(".add-row input") as HTMLInputElement;
    input.value = "openrouter-embed";
    input.dispatchEvent(new Event("input", { bubbles: true }));
    await flush();
    const addBtn = host.querySelector(".add-row button") as HTMLButtonElement;
    addBtn.click();
    await flush();
    expect(host.querySelector('[data-testid="provider-openrouter-embed"]')).toBeTruthy();
  });

  it("save payload sends explicit empty string to unbind a previously bound usage", async () => {
    mocks.fetchLLMProviders.mockResolvedValue({
      providers: { p1: { enabled: true, base_url: "u", model: "m", has_api_key: false } },
      usage: { consolidate: "p1" },
      usage_warnings: [],
    });
    mocks.saveLLMProviders.mockResolvedValue(emptyResponse());
    component = mount(LLMProviderSettings, { target: host });
    await flush();
    // change consolidate back to default ("")
    const sel = host.querySelector('[data-testid="usage-consolidate"] select') as HTMLSelectElement;
    sel.value = "";
    sel.dispatchEvent(new Event("change", { bubbles: true }));
    await flush();
    (host.querySelector(".save-btn") as HTMLButtonElement).click();
    await flush();
    expect(mocks.saveLLMProviders).toHaveBeenCalledTimes(1);
    const payload = mocks.saveLLMProviders.mock.calls[0][0];
    expect(payload.usage.consolidate).toBe(""); // explicit unbind
  });

  it("removing a provider adds it to delete_providers and clears its bindings", async () => {
    mocks.fetchLLMProviders.mockResolvedValue({
      providers: { p1: { enabled: true, base_url: "u", model: "m", has_api_key: false } },
      usage: { extract: "p1" },
      usage_warnings: [],
    });
    mocks.saveLLMProviders.mockResolvedValue(emptyResponse());
    component = mount(LLMProviderSettings, { target: host });
    await flush();
    (host.querySelector('[data-testid="provider-p1"] .ghost-btn') as HTMLButtonElement).click();
    await flush();
    expect(host.querySelector('[data-testid="provider-p1"]')).toBeFalsy();
    (host.querySelector(".save-btn") as HTMLButtonElement).click();
    await flush();
    const payload = mocks.saveLLMProviders.mock.calls[0][0];
    expect(payload.delete_providers).toContain("p1");
  });

  it("renders dangling usage warnings from the backend", async () => {
    mocks.fetchLLMProviders.mockResolvedValue({
      providers: {},
      usage: {},
      usage_warnings: ["usage 'embed' is bound to missing provider 'typo'"],
    });
    component = mount(LLMProviderSettings, { target: host });
    await flush();
    const warn = host.querySelector(".warning-list");
    expect(warn?.textContent).toContain("typo");
  });
});
