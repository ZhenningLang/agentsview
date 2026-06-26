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
  // Several cycles: the save path awaits saveLLMConfig then saveLLMProviders.
  for (let i = 0; i < 4; i++) {
    await Promise.resolve();
    await tick();
  }
}

function byTestId(id: string): HTMLElement | null {
  return document.querySelector(`[data-testid="${id}"]`);
}

function buttonWithText(text: string): HTMLButtonElement | undefined {
  return Array.from(document.querySelectorAll("button")).find((b) =>
    b.textContent?.includes(text),
  ) as HTMLButtonElement | undefined;
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
  });

  it("renders the two built-in provider cards and the five usage rows", async () => {
    component = mount(LLMEnrichmentSettings, { target: document.body });
    await flush();

    expect(byTestId("provider-chat")).toBeTruthy();
    expect(byTestId("provider-embed")).toBeTruthy();
    for (const u of ["enrich", "embed", "extract", "consolidate", "recall_rerank"]) {
      expect(byTestId(`usage-${u}`)).toBeTruthy();
    }
    // No named providers yet -> empty state.
    expect(byTestId("custom-empty")).toBeTruthy();
  });

  it("loads masked LLM config without rendering plaintext keys", async () => {
    component = mount(LLMEnrichmentSettings, { target: document.body });
    await flush();

    expect(mocks.fetchLLMConfig).toHaveBeenCalledTimes(1);
    const apiKeyInput = document.querySelector<HTMLInputElement>('input[name="api_key"]');
    const embedKeyInput = document.querySelector<HTMLInputElement>('input[name="embed_api_key"]');
    expect(apiKeyInput?.value).toBe("********1234");
    expect(embedKeyInput?.value).toBe("********5678");
  });

  it("provider presets auto-fill base URL and model", async () => {
    component = mount(LLMEnrichmentSettings, { target: document.body });
    await flush();

    const chatProvider = document.querySelector("select[name=chat_provider]") as HTMLSelectElement;
    const embedProvider = document.querySelector("select[name=embed_provider]") as HTMLSelectElement;
    chatProvider.value = "deepseek";
    chatProvider.dispatchEvent(new Event("change", { bubbles: true }));
    await flush();
    embedProvider.value = "openrouter";
    embedProvider.dispatchEvent(new Event("change", { bubbles: true }));
    await flush();

    expect((document.querySelector("input[name=base_url]") as HTMLInputElement).value).toBe("https://api.deepseek.com");
    expect((document.querySelector("input[name=model]") as HTMLInputElement).value).toBe("deepseek-chat");
    expect((document.querySelector("input[name=embed_base_url]") as HTMLInputElement).value).toBe("https://openrouter.ai/api/v1");
    expect((document.querySelector("input[name=embed_model]") as HTMLInputElement).value).toBe("openai/text-embedding-3-large");
  });

  // ---- Per-provider tests ----

  it("tests the default chat provider via its card button (channel chat)", async () => {
    component = mount(LLMEnrichmentSettings, { target: document.body });
    await flush();

    (byTestId("test-__chat__") as HTMLButtonElement).click();
    await flush();

    expect(mocks.testLLMConnection).toHaveBeenCalledWith(
      expect.objectContaining({ channel: "chat", base_url: "https://api.deepseek.com/v1", model: "deepseek-chat" }),
    );
    expect(byTestId("test-result-__chat__")?.textContent).toContain("ok");
  });

  it("tests the default embedding provider via its card button (channel embed)", async () => {
    mocks.testLLMConnection.mockResolvedValueOnce({
      chat: { ok: false, disabled: true, message: "skipped" },
      embed: { ok: true, message: "ok" },
    });
    component = mount(LLMEnrichmentSettings, { target: document.body });
    await flush();

    (byTestId("test-__embed__") as HTMLButtonElement).click();
    await flush();

    expect(mocks.testLLMConnection).toHaveBeenCalledWith(
      expect.objectContaining({ channel: "embed", embed: expect.objectContaining({ model: "embed-model" }) }),
    );
    expect(byTestId("test-result-__embed__")?.textContent).toContain("ok");
  });

  // ---- Per-usage tests ----

  it("tests a usage via its row button (resolves the usage server-side)", async () => {
    component = mount(LLMEnrichmentSettings, { target: document.body });
    await flush();

    (byTestId("test-usage:extract") as HTMLButtonElement).click();
    await flush();

    expect(mocks.testLLMConnection).toHaveBeenCalledWith(
      expect.objectContaining({ usage: "extract", channel: "chat" }),
    );
  });

  it("tests the embed usage with channel embed", async () => {
    mocks.testLLMConnection.mockResolvedValueOnce({
      chat: { ok: false, disabled: true, message: "skipped" },
      embed: { ok: true, message: "ok" },
    });
    component = mount(LLMEnrichmentSettings, { target: document.body });
    await flush();

    (byTestId("test-usage:embed") as HTMLButtonElement).click();
    await flush();

    expect(mocks.testLLMConnection).toHaveBeenCalledWith(
      expect.objectContaining({ usage: "embed", channel: "embed" }),
    );
    expect(byTestId("test-result-usage:embed")?.textContent).toContain("ok");
  });

  // ---- Migration / no data loss ----

  it("migrates existing config into the new layout without losing data", async () => {
    mocks.fetchLLMProviders.mockResolvedValue({
      providers: { "cheap-consolidate": { enabled: true, base_url: "https://x", model: "m", has_api_key: true, api_key_preview: "…9" } },
      usage: { consolidate: "cheap-consolidate" },
      usage_warnings: [],
    });
    component = mount(LLMEnrichmentSettings, { target: document.body });
    await flush();

    // Existing named provider renders, existing binding is reflected.
    expect(byTestId("provider-cheap-consolidate")).toBeTruthy();
    const sel = byTestId("usage-consolidate")!.querySelector("select") as HTMLSelectElement;
    expect(sel.value).toBe("cheap-consolidate");

    // Saving must preserve everything: embed config kept, no deletes.
    buttonWithText("Save LLM config")!.click();
    await flush();

    expect(mocks.saveLLMConfig).toHaveBeenCalledWith(
      expect.objectContaining({
        api_key: "********",
        embed: expect.objectContaining({ model: "embed-model", api_key: "********" }),
      }),
    );
    const providersPayload = mocks.saveLLMProviders.mock.calls[0]![0];
    expect(providersPayload.providers["cheap-consolidate"]).toBeTruthy();
    expect(providersPayload.usage.consolidate).toBe("cheap-consolidate");
    expect(providersPayload.delete_providers).toEqual([]);
  });

  it("saves config and preserves masked keys when nothing is edited", async () => {
    component = mount(LLMEnrichmentSettings, { target: document.body });
    await flush();

    buttonWithText("Save LLM config")!.click();
    await flush();

    expect(mocks.saveLLMConfig).toHaveBeenCalledWith(
      expect.objectContaining({ api_key: "********", embed: expect.objectContaining({ api_key: "********" }) }),
    );
    expect(document.body.textContent).toContain("LLM config saved");
  });

  // ---- Add / remove provider ----

  it("adds a named provider and binds it to a usage in the save payload", async () => {
    component = mount(LLMEnrichmentSettings, { target: document.body });
    await flush();

    const nameInput = document.querySelector(".add-row input") as HTMLInputElement;
    nameInput.value = "cheap";
    nameInput.dispatchEvent(new Event("input", { bubbles: true }));
    await flush();
    (document.querySelector(".add-row button") as HTMLButtonElement).click();
    await flush();
    expect(byTestId("provider-cheap")).toBeTruthy();

    const sel = byTestId("usage-extract")!.querySelector("select") as HTMLSelectElement;
    sel.value = "cheap";
    sel.dispatchEvent(new Event("change", { bubbles: true }));
    await flush();

    buttonWithText("Save LLM config")!.click();
    await flush();

    const payload = mocks.saveLLMProviders.mock.calls[0]![0];
    expect(payload.providers.cheap).toBeTruthy();
    expect(payload.usage.extract).toBe("cheap");
  });

  it("removes a named provider and sends it in delete_providers", async () => {
    mocks.fetchLLMProviders.mockResolvedValue({
      providers: { gone: { enabled: true, base_url: "https://x", model: "m", has_api_key: true, api_key_preview: "…9" } },
      usage: {},
      usage_warnings: [],
    });
    component = mount(LLMEnrichmentSettings, { target: document.body });
    await flush();

    expect(byTestId("provider-gone")).toBeTruthy();
    (byTestId("provider-gone")!.querySelector(".ghost-btn") as HTMLButtonElement).click();
    await flush();
    expect(byTestId("provider-gone")).toBeFalsy();

    buttonWithText("Save LLM config")!.click();
    await flush();
    const payload = mocks.saveLLMProviders.mock.calls[0]![0];
    expect(payload.delete_providers).toContain("gone");
  });

  it("starts a background job and shows progress", async () => {
    component = mount(LLMEnrichmentSettings, { target: document.body });
    await flush();

    buttonWithText("Run enrichment")!.click();
    await flush();

    expect(mocks.startEnrichJob).toHaveBeenCalledTimes(1);
    expect(byTestId("enrich-progress")?.textContent).toContain("Enriching 0 / 3");
  });

  it("surfaces backend start errors", async () => {
    mocks.startEnrichJob.mockRejectedValueOnce(new Error("LLM is disabled"));
    component = mount(LLMEnrichmentSettings, { target: document.body });
    await flush();

    buttonWithText("Run enrichment")!.click();
    await flush();

    expect(document.querySelector('[role="alert"]')?.textContent).toContain("Failed to start LLM enrichment");
  });

  it("shows remote mode as unavailable and does not fetch", async () => {
    store.set("agentsview-server-url", "http://remote.test");
    component = mount(LLMEnrichmentSettings, { target: document.body });
    await flush();

    expect(mocks.fetchEnrichStatus).not.toHaveBeenCalled();
    expect(mocks.fetchLLMConfig).not.toHaveBeenCalled();
    expect(document.body.textContent).toContain("local server connection");
  });

  it("disables the trigger and test buttons in read-only mode", async () => {
    sync.serverVersion = {
      version: "test",
      commit: "abc123",
      build_date: "2026-06-24T00:00:00Z",
      read_only: true,
    };
    component = mount(LLMEnrichmentSettings, { target: document.body });
    await flush();

    const run = buttonWithText("Run enrichment");
    expect(run?.disabled).toBe(true);
    expect((byTestId("test-__chat__") as HTMLButtonElement).disabled).toBe(true);
    expect(document.body.textContent).toContain("read-only backend");
  });
});
