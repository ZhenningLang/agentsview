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
  triggerEnrich: vi.fn(),
}));

vi.mock("../../api/llm.js", () => ({
  fetchEnrichStatus: mocks.fetchEnrichStatus,
  fetchLLMConfig: mocks.fetchLLMConfig,
  saveLLMConfig: mocks.saveLLMConfig,
  testLLMConnection: mocks.testLLMConnection,
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

    expect(mocks.triggerEnrich).not.toHaveBeenCalled();
  });
});
