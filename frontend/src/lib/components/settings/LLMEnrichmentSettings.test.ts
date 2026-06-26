// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
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
  for (let i = 0; i < 5; i++) {
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

// Default backend state: legacy [llm] (deepseek) + [llm.embed] (openrouter),
// empty registry — exercises the migration/seed path.
function llmConfig() {
  return {
    enabled: true,
    base_url: "https://api.deepseek.com",
    model: "deepseek-chat",
    reasoning_effort: "medium",
    min_user_messages: 3,
    reenrich_msg_delta: 10,
    reenrich_idle_minutes: 60,
    concurrency: 2,
    periodic: true,
    has_api_key: true,
    api_key_preview: "1234",
    embed: {
      base_url: "https://openrouter.ai/api/v1",
      model: "openai/text-embedding-3-large",
      has_api_key: true,
      api_key_preview: "5678",
    },
  };
}

describe("LLMEnrichmentSettings (provider+model)", () => {
  let component: ReturnType<typeof mount> | undefined;
  const originalStorage = globalThis.localStorage;
  let store: Map<string, string>;

  beforeEach(() => {
    setLocale("en");
    sync.serverVersion = null;
    store = new Map();
    Object.defineProperty(globalThis, "localStorage", {
      value: {
        getItem: vi.fn((k: string) => store.get(k) ?? null),
        setItem: vi.fn((k: string, v: string) => store.set(k, v)),
        removeItem: vi.fn((k: string) => store.delete(k)),
        clear: vi.fn(() => store.clear()),
      },
      writable: true,
      configurable: true,
    });
    mocks.fetchEnrichStatus.mockReset().mockResolvedValue({
      total: 10, enriched: 4, pending: 3, skipped_too_short: 1, no_content: 1, errors: 1, by_status: { ok: 4 },
    });
    mocks.fetchEnrichJob.mockReset().mockResolvedValue({
      running: false, processed: 0, total: 0, succeeded: 0, no_content: 0, failed: 0, skipped: 0,
      prompt_tokens: 0, completion_tokens: 0, embed_tokens: 0,
    });
    mocks.startEnrichJob.mockReset().mockResolvedValue({
      running: true, source: "manual", processed: 0, total: 3, succeeded: 0, no_content: 0, failed: 0, skipped: 0,
      prompt_tokens: 0, completion_tokens: 0, embed_tokens: 0,
    });
    mocks.stopEnrichJob.mockReset().mockResolvedValue({ running: false, processed: 0, total: 0, succeeded: 0, no_content: 0, failed: 0, skipped: 0, prompt_tokens: 0, completion_tokens: 0, embed_tokens: 0 });
    mocks.fetchLLMConfig.mockReset().mockResolvedValue(llmConfig());
    mocks.fetchLLMProviders.mockReset().mockResolvedValue({ ...llmConfig(), providers: {}, usage: {}, usage_model: {}, usage_warnings: [] });
    mocks.saveLLMProviders.mockReset().mockResolvedValue({ ...llmConfig(), providers: {}, usage: {}, usage_model: {}, usage_warnings: [] });
    mocks.saveLLMConfig.mockReset().mockImplementation(async (payload) => ({ ...llmConfig(), ...payload, embed: llmConfig().embed }));
    mocks.testLLMConnection.mockReset().mockResolvedValue({
      chat: { ok: true, message: "ok" },
      embed: { ok: false, disabled: true, message: "disabled" },
    });
  });

  afterEach(() => {
    if (component) { unmount(component); component = undefined; }
    document.body.innerHTML = "";
    Object.defineProperty(globalThis, "localStorage", { value: originalStorage, writable: true, configurable: true });
  });

  it("migrates legacy [llm]/[llm.embed] into a vendor+key provider list", async () => {
    component = mount(LLMEnrichmentSettings, { target: document.body });
    await flush();
    // [llm] -> deepseek-1, [llm.embed] -> openrouter-1
    expect(byTestId("provider-deepseek-1")).toBeTruthy();
    expect(byTestId("provider-openrouter-1")).toBeTruthy();
    // five usage rows
    for (const u of ["enrich", "embed", "extract", "consolidate", "recall_rerank"]) {
      expect(byTestId(`usage-${u}`)).toBeTruthy();
    }
    // models live on the usage rows
    expect((byTestId("usage-model-enrich") as HTMLInputElement).value).toBe("deepseek-chat");
    expect((byTestId("usage-model-embed") as HTMLInputElement).value).toBe("openai/text-embedding-3-large");
  });

  it("does not render plaintext API keys", async () => {
    component = mount(LLMEnrichmentSettings, { target: document.body });
    await flush();
    const keyInputs = Array.from(document.querySelectorAll<HTMLInputElement>('input[type="password"]'));
    expect(keyInputs.length).toBeGreaterThan(0);
    expect(keyInputs[0]!.value).toBe("********1234");
    expect(document.body.textContent).not.toContain("sk-");
  });

  it("save writes providers (no model), usage names, and usage_model, with no deletes", async () => {
    component = mount(LLMEnrichmentSettings, { target: document.body });
    await flush();

    buttonWithText("Save LLM config")!.click();
    await flush();

    expect(mocks.saveLLMProviders).toHaveBeenCalledTimes(1);
    // The LLM-config save must NOT touch [llm] connection fields.
    expect(mocks.saveLLMConfig).not.toHaveBeenCalled();
    const payload = mocks.saveLLMProviders.mock.calls[0]![0];
    expect(payload.providers["deepseek-1"]).toMatchObject({ base_url: "https://api.deepseek.com", api_key: "********", model: "" });
    expect(payload.providers["openrouter-1"]).toMatchObject({ base_url: "https://openrouter.ai/api/v1", model: "" });
    expect(payload.usage.enrich).toBe("deepseek-1");
    expect(payload.usage.embed).toBe("openrouter-1");
    expect(payload.usage_model.enrich).toBe("deepseek-chat");
    expect(payload.usage_model.embed).toBe("openai/text-embedding-3-large");
    expect(payload.delete_providers).toEqual([]);
  });

  it("enrichment-settings save omits connection fields so [llm] survives", async () => {
    component = mount(LLMEnrichmentSettings, { target: document.body });
    await flush();

    buttonWithText("Save enrichment settings")!.click();
    await flush();

    expect(mocks.saveLLMConfig).toHaveBeenCalledTimes(1);
    const payload = mocks.saveLLMConfig.mock.calls[0]![0];
    expect(payload).toHaveProperty("enabled");
    expect(payload).toHaveProperty("min_user_messages");
    // Connection fields must be absent (pointer-patch preserves them server-side).
    expect(payload).not.toHaveProperty("base_url");
    expect(payload).not.toHaveProperty("model");
    expect(payload).not.toHaveProperty("embed");
  });

  it("adds a second provider for the same vendor (auto-named, unique)", async () => {
    component = mount(LLMEnrichmentSettings, { target: document.body });
    await flush();
    buttonWithText("Add provider")!.click();
    await flush();
    // deepseek-1 already exists from migration -> new one is deepseek-2
    expect(byTestId("provider-deepseek-2")).toBeTruthy();
  });

  it("removes a registry provider and lists it in delete_providers on save", async () => {
    mocks.fetchLLMProviders.mockResolvedValue({
      ...llmConfig(),
      providers: { "extra-1": { enabled: true, base_url: "https://api.deepseek.com", has_api_key: true, api_key_preview: "9" } },
      usage: {},
      usage_model: {},
      usage_warnings: [],
    });
    component = mount(LLMEnrichmentSettings, { target: document.body });
    await flush();
    expect(byTestId("provider-extra-1")).toBeTruthy();
    (byTestId("provider-extra-1")!.querySelector(".ghost-btn") as HTMLButtonElement).click();
    await flush();
    expect(byTestId("provider-extra-1")).toBeFalsy();

    buttonWithText("Save LLM config")!.click();
    await flush();
    const payload = mocks.saveLLMProviders.mock.calls[0]![0];
    expect(payload.delete_providers).toContain("extra-1");
  });

  it("tests a provider via its card (channel chat, by provider name)", async () => {
    component = mount(LLMEnrichmentSettings, { target: document.body });
    await flush();
    const btn = byTestId("provider-deepseek-1")!.querySelector('[data-testid^="test-provider:"]') as HTMLButtonElement;
    btn.click();
    await flush();
    expect(mocks.testLLMConnection).toHaveBeenCalledWith(
      expect.objectContaining({ provider: "deepseek-1", channel: "chat" }),
    );
    expect(btn.parentElement?.textContent).toContain("ok");
  });

  it("tests a chat usage via its row (usage + channel chat + current model)", async () => {
    component = mount(LLMEnrichmentSettings, { target: document.body });
    await flush();
    (byTestId("test-usage:extract") as HTMLButtonElement).click();
    await flush();
    expect(mocks.testLLMConnection).toHaveBeenCalledWith(
      expect.objectContaining({ usage: "extract", channel: "chat", model: "deepseek-chat" }),
    );
  });

  it("tests the embed usage with channel embed", async () => {
    mocks.testLLMConnection.mockResolvedValueOnce({ chat: { ok: false, disabled: true, message: "skipped" }, embed: { ok: true, message: "ok" } });
    component = mount(LLMEnrichmentSettings, { target: document.body });
    await flush();
    (byTestId("test-usage:embed") as HTMLButtonElement).click();
    await flush();
    expect(mocks.testLLMConnection).toHaveBeenCalledWith(
      expect.objectContaining({ usage: "embed", channel: "embed", embed: expect.objectContaining({ model: "openai/text-embedding-3-large" }) }),
    );
    expect(byTestId("test-result-usage:embed")?.textContent).toContain("ok");
  });

  it("starts a background job and shows progress", async () => {
    component = mount(LLMEnrichmentSettings, { target: document.body });
    await flush();
    buttonWithText("Run enrichment")!.click();
    await flush();
    expect(mocks.startEnrichJob).toHaveBeenCalledTimes(1);
    expect(byTestId("enrich-progress")?.textContent).toContain("Enriching 0 / 3");
  });

  it("shows remote mode as unavailable and does not fetch", async () => {
    store.set("agentsview-server-url", "http://remote.test");
    component = mount(LLMEnrichmentSettings, { target: document.body });
    await flush();
    expect(mocks.fetchLLMConfig).not.toHaveBeenCalled();
    expect(document.body.textContent).toContain("local server connection");
  });

  it("disables save and test buttons in read-only mode", async () => {
    sync.serverVersion = { version: "t", commit: "c", build_date: "2026-06-24T00:00:00Z", read_only: true };
    component = mount(LLMEnrichmentSettings, { target: document.body });
    await flush();
    expect(buttonWithText("Run enrichment")?.disabled).toBe(true);
    expect((byTestId("test-usage:extract") as HTMLButtonElement).disabled).toBe(true);
    expect(document.body.textContent).toContain("read-only backend");
  });
});
