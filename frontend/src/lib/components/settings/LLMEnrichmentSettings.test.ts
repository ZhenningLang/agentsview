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
  fetchLLMConfig: vi.fn(),
  saveLLMConfig: vi.fn(),
  testLLMConnection: vi.fn(),
  startEnrichJob: vi.fn(),
  stopEnrichJob: vi.fn(),
  fetchEnrichJob: vi.fn(),
  fetchLLMProviders: vi.fn(),
  saveLLMProviders: vi.fn(),
}));

vi.mock("../../api/llm.js", () => ({
  fetchEnrichStatus: mocks.fetchEnrichStatus,
  fetchLLMConfig: mocks.fetchLLMConfig,
  saveLLMConfig: mocks.saveLLMConfig,
  testLLMConnection: mocks.testLLMConnection,
  startEnrichJob: mocks.startEnrichJob,
  stopEnrichJob: mocks.stopEnrichJob,
  fetchEnrichJob: mocks.fetchEnrichJob,
  fetchLLMProviders: mocks.fetchLLMProviders,
  saveLLMProviders: mocks.saveLLMProviders,
}));

import { sync } from "../../stores/sync.svelte.js";

// @ts-ignore
import LLMEnrichmentSettings from "./LLMEnrichmentSettings.svelte";
import { setLocale } from "../../i18n/index.svelte";

async function flush() {
  // Several cycles: the save path now awaits saveLLMConfig then saveLLMProviders.
  for (let i = 0; i < 4; i++) {
    await Promise.resolve();
    await tick();
  }
}

describe("LLMEnrichmentSettings", () => {
  let component: ReturnType<typeof mount> | undefined;
  const originalStorage = globalThis.localStorage;
  let store: Map<string, string>;

  beforeEach(() => {
    setLocale("en");
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
    mocks.fetchEnrichJob.mockReset().mockResolvedValue({
      running: false,
      processed: 0,
      total: 0,
      succeeded: 0,
      no_content: 0,
      failed: 0,
      skipped: 0,
      prompt_tokens: 0,
      completion_tokens: 0,
      embed_tokens: 0,
    });
    mocks.startEnrichJob.mockReset().mockResolvedValue({
      running: true,
      source: "manual",
      processed: 0,
      total: 3,
      succeeded: 0,
      no_content: 0,
      failed: 0,
      skipped: 0,
      prompt_tokens: 0,
      completion_tokens: 0,
      embed_tokens: 0,
    });
    mocks.stopEnrichJob.mockReset().mockResolvedValue({
      running: false,
      source: "manual",
      processed: 1,
      total: 3,
      succeeded: 1,
      no_content: 0,
      failed: 0,
      skipped: 0,
      prompt_tokens: 0,
      completion_tokens: 0,
      embed_tokens: 0,
      done_at: "2026-06-24T00:00:00Z",
    });
    mocks.fetchLLMConfig.mockReset().mockResolvedValue({
      enabled: true,
      base_url: "https://api.deepseek.com/v1",
      model: "deepseek-chat",
      reasoning_effort: "low",
      min_user_messages: 3,
      reenrich_msg_delta: 10,
      reenrich_idle_minutes: 60,
      concurrency: 2,
      periodic: true,
      has_api_key: true,
      api_key_preview: "1234",
      embed: {
        base_url: "https://embed.example/v1",
        model: "embed-model",
        has_api_key: true,
        api_key_preview: "5678",
      },
    });
    mocks.fetchLLMProviders.mockReset().mockResolvedValue({ providers: {}, usage: {}, usage_warnings: [] });
    mocks.saveLLMProviders.mockReset().mockResolvedValue({ providers: {}, usage: {}, usage_warnings: [] });
    mocks.saveLLMConfig.mockReset().mockImplementation(async (payload) => ({
      enabled: payload.enabled ?? true,
      base_url: payload.base_url ?? "https://api.deepseek.com/v1",
      model: payload.model ?? "deepseek-chat",
      reasoning_effort: payload.reasoning_effort ?? "low",
      min_user_messages: payload.min_user_messages ?? 3,
      reenrich_msg_delta: payload.reenrich_msg_delta ?? 10,
      reenrich_idle_minutes: payload.reenrich_idle_minutes ?? 60,
      concurrency: payload.concurrency ?? 2,
      periodic: payload.periodic ?? true,
      has_api_key: true,
      api_key_preview: "1234",
      embed: {
        base_url: payload.embed?.base_url ?? "https://embed.example/v1",
        model: payload.embed?.model ?? "embed-model",
        has_api_key: true,
        api_key_preview: "5678",
      },
    }));
    mocks.testLLMConnection.mockReset().mockResolvedValue({
      chat: { ok: true, message: "ok" },
      embed: { ok: false, disabled: true, message: "disabled" },
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

  it("loads masked LLM config without rendering plaintext keys", async () => {
    component = mount(LLMEnrichmentSettings, { target: document.body });
    await flush();

    expect(mocks.fetchLLMConfig).toHaveBeenCalledTimes(1);
    const apiKeyInput = document.querySelector<HTMLInputElement>('input[name="api_key"]');
    const embedKeyInput = document.querySelector<HTMLInputElement>('input[name="embed_api_key"]');
    expect(apiKeyInput?.value).toBe("********1234");
    expect(embedKeyInput?.value).toBe("********5678");
    expect(document.body.textContent).not.toContain("chat-secret");
    expect(document.body.textContent).not.toContain("embed-secret");
  });

  it("saves config and preserves masked keys", async () => {
    component = mount(LLMEnrichmentSettings, { target: document.body });
    await flush();

    const enabled = document.querySelector<HTMLInputElement>('input[name="enabled"]')!;
    enabled.checked = false;
    enabled.dispatchEvent(new Event("change", { bubbles: true }));
    const model = document.querySelector<HTMLInputElement>('input[name="model"]')!;
    model.value = "deepseek-reasoner";
    model.dispatchEvent(new InputEvent("input", { bubbles: true }));
    const save = Array.from(document.querySelectorAll("button")).find((btn) =>
      btn.textContent?.includes("Save LLM config"),
    ) as HTMLButtonElement | undefined;
    save!.click();
    await flush();

    expect(mocks.saveLLMConfig).toHaveBeenCalledWith(
      expect.objectContaining({
        enabled: false,
        api_key: "********",
        model: "deepseek-reasoner",
        embed: expect.objectContaining({ api_key: "********" }),
      }),
    );
    expect(document.body.textContent).toContain("LLM config saved");
  });

  it("tests connection and renders per-channel results", async () => {
    component = mount(LLMEnrichmentSettings, { target: document.body });
    await flush();

    const testButton = Array.from(document.querySelectorAll("button")).find((btn) =>
      btn.textContent?.includes("Test connection"),
    ) as HTMLButtonElement | undefined;
    testButton!.click();
    await flush();

    expect(mocks.testLLMConnection).toHaveBeenCalledWith(expect.objectContaining({
      base_url: "https://api.deepseek.com/v1",
      model: "deepseek-chat",
    }));
    expect(document.body.textContent).toContain("chat: ok");
    expect(document.body.textContent).toContain("embed: disabled");
  });

  it("updates enabled config from toggle", async () => {
    component = mount(LLMEnrichmentSettings, { target: document.body });
    await flush();

    const enabled = document.querySelector<HTMLInputElement>('input[name="enabled"]')!;
    expect(enabled.checked).toBe(true);
    enabled.checked = false;
    enabled.dispatchEvent(new Event("change", { bubbles: true }));
    await flush();

    expect(enabled.checked).toBe(false);
  });

  it("starts a background job and shows progress", async () => {
    component = mount(LLMEnrichmentSettings, { target: document.body });
    await flush();

    const button = Array.from(document.querySelectorAll("button")).find((btn) =>
      btn.textContent?.includes("Run enrichment"),
    ) as HTMLButtonElement | undefined;
    expect(button).toBeTruthy();

    button!.click();
    await flush();

    expect(mocks.startEnrichJob).toHaveBeenCalledTimes(1);
    const progress = document.querySelector('[data-testid="enrich-progress"]');
    expect(progress).toBeTruthy();
    expect(progress?.textContent).toContain("Enriching 0 / 3");
    // While running, the primary action becomes a Stop button.
    const stop = Array.from(document.querySelectorAll("button")).find((btn) =>
      btn.textContent?.trim() === "Stop",
    );
    expect(stop).toBeTruthy();
  });

  it("renders token usage and chat cost for a completed job", async () => {
    mocks.fetchEnrichJob.mockResolvedValueOnce({
      running: false,
      source: "manual",
      processed: 3,
      total: 3,
      succeeded: 3,
      no_content: 0,
      failed: 0,
      skipped: 0,
      done_at: "2026-06-24T00:00:00Z",
      prompt_tokens: 1200,
      completion_tokens: 800,
      embed_tokens: 3400,
      cost_currency: "CNY",
      cost_spent: "0.4200",
      balance_end: "98.7600",
    });
    component = mount(LLMEnrichmentSettings, { target: document.body });
    await flush();

    const cost = document.querySelector('[data-testid="enrich-cost"]');
    expect(cost).toBeTruthy();
    const text = (cost?.textContent ?? "").replace(/\s+/g, " ");
    expect(text).toContain("2,000 chat");
    expect(text).toContain("3,400 embed");
    expect(text).toContain("CNY 0.4200");
    expect(text).toContain("balance now CNY 98.7600");
  });

  it("renders embed spend separately from chat spend", async () => {
    mocks.fetchEnrichJob.mockResolvedValueOnce({
      running: false,
      source: "manual",
      processed: 2,
      total: 2,
      succeeded: 2,
      no_content: 0,
      failed: 0,
      skipped: 0,
      done_at: "2026-06-24T00:00:00Z",
      prompt_tokens: 1000,
      completion_tokens: 500,
      embed_tokens: 700,
      cost_currency: "CNY",
      cost_spent: "0.5000",
      balance_end: "99.5000",
      embed_cost_currency: "USD",
      embed_cost_spent: "0.2000",
      embed_balance_end: "49.8000",
    });
    component = mount(LLMEnrichmentSettings, { target: document.body });
    await flush();

    const cost = document.querySelector('[data-testid="enrich-cost"]');
    expect(cost).toBeTruthy();
    const text = (cost?.textContent ?? "").replace(/\s+/g, " ");
    expect(text).toContain("Embed spend this run: USD 0.2000");
    expect(text).toContain("balance now USD 49.8000");
  });

  it("surfaces backend start errors", async () => {
    mocks.startEnrichJob.mockRejectedValueOnce(new Error("LLM is disabled"));
    component = mount(LLMEnrichmentSettings, { target: document.body });
    await flush();

    const button = Array.from(document.querySelectorAll("button")).find((btn) =>
      btn.textContent?.includes("Run enrichment"),
    ) as HTMLButtonElement | undefined;
    button!.click();
    await flush();

    expect(document.querySelector('[role="alert"]')?.textContent).toContain(
      "Failed to start LLM enrichment",
    );
  });

  it("shows remote mode as unavailable and does not fetch", async () => {
    store.set("agentsview-server-url", "http://remote.test");
    component = mount(LLMEnrichmentSettings, { target: document.body });
    await flush();

    expect(mocks.fetchEnrichStatus).not.toHaveBeenCalled();
    expect(mocks.fetchLLMConfig).not.toHaveBeenCalled();
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

    expect(mocks.startEnrichJob).not.toHaveBeenCalled();
  });

  it("provider presets auto-fill base URL and model", async () => {
    component = mount(LLMEnrichmentSettings, { target: document.body });
    await flush();

    const chatProvider = document.querySelector(
      "select[name=chat_provider]",
    ) as HTMLSelectElement;
    const embedProvider = document.querySelector(
      "select[name=embed_provider]",
    ) as HTMLSelectElement;
    expect(chatProvider).toBeTruthy();
    expect(embedProvider).toBeTruthy();

    chatProvider.value = "deepseek";
    chatProvider.dispatchEvent(new Event("change", { bubbles: true }));
    await flush();
    embedProvider.value = "openrouter";
    embedProvider.dispatchEvent(new Event("change", { bubbles: true }));
    await flush();

    const baseUrl = document.querySelector(
      "input[name=base_url]",
    ) as HTMLInputElement;
    const model = document.querySelector(
      "input[name=model]",
    ) as HTMLInputElement;
    const embedBaseUrl = document.querySelector(
      "input[name=embed_base_url]",
    ) as HTMLInputElement;
    const embedModel = document.querySelector(
      "input[name=embed_model]",
    ) as HTMLInputElement;

    expect(baseUrl.value).toBe("https://api.deepseek.com");
    expect(model.value).toBe("deepseek-chat");
    expect(embedBaseUrl.value).toBe("https://openrouter.ai/api/v1");
    expect(embedModel.value).toBe("openai/text-embedding-3-large");
  });

  // ---- Per-usage model override block (merged from the removed LLMProviderSettings) ----

  it("renders the 3 per-usage override rows and the custom-provider empty state", async () => {
    component = mount(LLMEnrichmentSettings, { target: document.body });
    await flush();
    expect(document.querySelector('[data-testid="usage-extract"]')).toBeTruthy();
    expect(document.querySelector('[data-testid="usage-consolidate"]')).toBeTruthy();
    expect(document.querySelector('[data-testid="usage-recall_rerank"]')).toBeTruthy();
    // enrich/embed are NOT in the override block (configured by the provider blocks)
    expect(document.querySelector('[data-testid="usage-enrich"]')).toBeFalsy();
    expect(document.querySelector('[data-testid="usage-embed"]')).toBeFalsy();
    expect(document.querySelector('[data-testid="custom-empty"]')).toBeTruthy();
  });

  it("loads existing custom providers and usage bindings from the registry", async () => {
    mocks.fetchLLMProviders.mockResolvedValue({
      providers: { "cheap-consolidate": { enabled: true, base_url: "https://x", model: "m", has_api_key: true, api_key_preview: "…9" } },
      usage: { consolidate: "cheap-consolidate" },
      usage_warnings: [],
    });
    component = mount(LLMEnrichmentSettings, { target: document.body });
    await flush();
    expect(document.querySelector('[data-testid="custom-cheap-consolidate"]')).toBeTruthy();
    const sel = document.querySelector('[data-testid="usage-consolidate"] select') as HTMLSelectElement;
    expect(sel.value).toBe("cheap-consolidate");
  });

  it("adds a custom provider and saves it with the usage binding payload", async () => {
    component = mount(LLMEnrichmentSettings, { target: document.body });
    await flush();
    // add a custom provider
    const nameInput = document.querySelector(".add-row input") as HTMLInputElement;
    nameInput.value = "cheap";
    nameInput.dispatchEvent(new Event("input", { bubbles: true }));
    await flush();
    (document.querySelector(".add-row button") as HTMLButtonElement).click();
    await flush();
    expect(document.querySelector('[data-testid="custom-cheap"]')).toBeTruthy();
    // bind extract → cheap
    const sel = document.querySelector('[data-testid="usage-extract"] select') as HTMLSelectElement;
    sel.value = "cheap";
    sel.dispatchEvent(new Event("change", { bubbles: true }));
    await flush();
    // save
    const save = Array.from(document.querySelectorAll("button")).find((b) =>
      b.textContent?.includes("Save LLM config"),
    ) as HTMLButtonElement;
    save.click();
    await flush();
    expect(mocks.saveLLMProviders).toHaveBeenCalledTimes(1);
    const payload = mocks.saveLLMProviders.mock.calls[0][0];
    expect(payload.providers.cheap).toBeTruthy();
    expect(payload.usage.extract).toBe("cheap");
  });
});
