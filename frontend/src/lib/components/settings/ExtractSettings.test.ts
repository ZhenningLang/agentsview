// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { mount, tick, unmount } from "svelte";

// Extract card: enable/disable toggle only (interval via config/env; no
// per-interval HTTP route exists for extraction).
const mocks = vi.hoisted(() => ({
  fetchExtractAudit: vi.fn(),
  setExtractEnabled: vi.fn(),
  isRemoteConnection: vi.fn(() => false),
}));

vi.mock("../../api/extract", () => ({
  fetchExtractAudit: mocks.fetchExtractAudit,
  setExtractEnabled: mocks.setExtractEnabled,
}));

vi.mock("../../api/runtime.js", () => ({
  isRemoteConnection: mocks.isRemoteConnection,
  ApiError: class ApiError extends Error {},
}));

// @ts-ignore - svelte component import
import ExtractSettings from "./ExtractSettings.svelte";
import { setLocale } from "../../i18n/index.svelte";

async function flush() {
  await Promise.resolve();
  await tick();
  await Promise.resolve();
  await tick();
}

describe("ExtractSettings (toggle)", () => {
  let component: ReturnType<typeof mount> | undefined;
  let host: HTMLElement;

  beforeEach(() => {
    localStorage.clear();
    setLocale("en");
    host = document.createElement("div");
    document.body.appendChild(host);
    mocks.fetchExtractAudit.mockReset().mockResolvedValue({ enabled: false, available: true, records: [] });
    mocks.setExtractEnabled.mockReset().mockResolvedValue({ enabled: true, available: true });
    mocks.isRemoteConnection.mockReturnValue(false);
  });

  afterEach(() => {
    if (component) unmount(component);
    component = undefined;
    host.remove();
  });

  it("loads the current enabled state from the audit endpoint", async () => {
    mocks.fetchExtractAudit.mockResolvedValue({ enabled: true, available: true, records: [] });
    component = mount(ExtractSettings, { target: host });
    await flush();
    const toggle = host.querySelector('[data-testid="extract-toggle"]') as HTMLButtonElement;
    expect(toggle.getAttribute("aria-pressed")).toBe("true");
  });

  it("toggling calls setExtractEnabled with the flipped value", async () => {
    component = mount(ExtractSettings, { target: host });
    await flush();
    (host.querySelector('[data-testid="extract-toggle"]') as HTMLButtonElement).click();
    await flush();
    expect(mocks.setExtractEnabled).toHaveBeenCalledWith(true);
  });

  it("warns when enabled but the worker is unavailable in this process", async () => {
    mocks.fetchExtractAudit.mockResolvedValue({ enabled: true, available: false, records: [] });
    component = mount(ExtractSettings, { target: host });
    await flush();
    expect(host.querySelector('[data-testid="extract-unavailable"]')).toBeTruthy();
  });

  it("is local-only on a remote connection", async () => {
    mocks.isRemoteConnection.mockReturnValue(true);
    component = mount(ExtractSettings, { target: host });
    await flush();
    expect(host.querySelector('[data-testid="extract-toggle"]')).toBeFalsy();
    expect(mocks.fetchExtractAudit).not.toHaveBeenCalled();
  });
});
