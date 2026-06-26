// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { mount, tick, unmount } from "svelte";

// Slimmed consolidate card: enable toggle + interval only. Provider/usage
// editing moved to LLMEnrichmentSettings (per-usage override block).
const mocks = vi.hoisted(() => ({
  fetchConsolidateConfig: vi.fn(),
  saveConsolidateConfig: vi.fn(),
  setConsolidateEnabled: vi.fn(),
  isRemoteConnection: vi.fn(() => false),
}));

vi.mock("../../api/llm", () => ({
  fetchConsolidateConfig: mocks.fetchConsolidateConfig,
  saveConsolidateConfig: mocks.saveConsolidateConfig,
}));

vi.mock("../../api/consolidate", () => ({
  setConsolidateEnabled: mocks.setConsolidateEnabled,
}));

vi.mock("../../api/runtime.js", () => ({
  isRemoteConnection: mocks.isRemoteConnection,
  ApiError: class ApiError extends Error {},
}));

// @ts-ignore - svelte component import
import ConsolidateSettings from "./ConsolidateSettings.svelte";
import { setLocale } from "../../i18n/index.svelte";

async function flush() {
  await Promise.resolve();
  await tick();
  await Promise.resolve();
  await tick();
}

describe("ConsolidateSettings (slimmed)", () => {
  let component: ReturnType<typeof mount> | undefined;
  let host: HTMLElement;

  beforeEach(() => {
    localStorage.clear();
    setLocale("en");
    host = document.createElement("div");
    document.body.appendChild(host);
    mocks.fetchConsolidateConfig.mockReset().mockResolvedValue({ enabled: false, interval: "24h" });
    mocks.saveConsolidateConfig.mockReset().mockResolvedValue({ enabled: false, interval: "12h" });
    mocks.setConsolidateEnabled.mockReset().mockResolvedValue({ enabled: true });
    mocks.isRemoteConnection.mockReturnValue(false);
  });

  afterEach(() => {
    if (component) unmount(component);
    component = undefined;
    host.remove();
  });

  it("loads enabled state and interval", async () => {
    mocks.fetchConsolidateConfig.mockResolvedValue({ enabled: true, interval: "24h0m0s" });
    component = mount(ConsolidateSettings, { target: host });
    await flush();
    const toggle = host.querySelector(".toggle-btn") as HTMLButtonElement;
    expect(toggle.getAttribute("aria-pressed")).toBe("true");
    const interval = host.querySelector('input[type="text"]') as HTMLInputElement;
    expect(interval.value).toBe("24h"); // normalized
  });

  it("toggling calls setConsolidateEnabled", async () => {
    component = mount(ConsolidateSettings, { target: host });
    await flush();
    (host.querySelector(".toggle-btn") as HTMLButtonElement).click();
    await flush();
    expect(mocks.setConsolidateEnabled).toHaveBeenCalledWith(true);
  });

  it("saving interval calls saveConsolidateConfig with normalized value", async () => {
    component = mount(ConsolidateSettings, { target: host });
    await flush();
    const interval = host.querySelector('input[type="text"]') as HTMLInputElement;
    interval.value = "6h";
    interval.dispatchEvent(new Event("input", { bubbles: true }));
    await flush();
    (host.querySelector(".save-btn") as HTMLButtonElement).click();
    await flush();
    expect(mocks.saveConsolidateConfig).toHaveBeenCalledWith({ interval: "6h" });
  });

  it("blocks save on an invalid interval and shows an inline error", async () => {
    component = mount(ConsolidateSettings, { target: host });
    await flush();
    const interval = host.querySelector('input[type="text"]') as HTMLInputElement;
    const save = host.querySelector(".save-btn") as HTMLButtonElement;

    for (const bad of ["abc", "0s", "5 minutes", ""]) {
      interval.value = bad;
      interval.dispatchEvent(new Event("input", { bubbles: true }));
      await flush();
      expect(save.disabled, `"${bad}" should disable save`).toBe(true);
      expect(host.querySelector('[data-testid="interval-invalid"]')).toBeTruthy();
    }

    save.click();
    await flush();
    expect(mocks.saveConsolidateConfig).not.toHaveBeenCalled();

    // A valid duration re-enables save and clears the error.
    interval.value = "90m";
    interval.dispatchEvent(new Event("input", { bubbles: true }));
    await flush();
    expect(save.disabled).toBe(false);
    expect(host.querySelector('[data-testid="interval-invalid"]')).toBeFalsy();
  });

  it("does NOT render provider/usage editing (moved to LLM config)", async () => {
    component = mount(ConsolidateSettings, { target: host });
    await flush();
    expect(host.querySelector('[data-testid^="provider-"]')).toBeFalsy();
    expect(host.querySelector('[data-testid^="usage-"]')).toBeFalsy();
  });
});
