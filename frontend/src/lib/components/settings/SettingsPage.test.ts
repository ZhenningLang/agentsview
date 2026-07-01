// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from "vitest";
import { mount, tick, unmount } from "svelte";

const mocks = vi.hoisted(() => ({
  settings: {
    loading: false,
    needsAuth: false,
    error: null as string | null,
    load: vi.fn(),
  },
  sync: { readOnly: false },
  ui: { activeModal: "" },
  getAuthToken: vi.fn(() => ""),
  setAuthToken: vi.fn(),
  setServerUrl: vi.fn(),
  isRemoteConnection: vi.fn(() => false),
}));

vi.mock("../../stores/settings.svelte.js", () => ({ settings: mocks.settings }));
vi.mock("../../stores/sync.svelte.js", () => ({ sync: mocks.sync }));
vi.mock("../../stores/ui.svelte.js", () => ({ ui: mocks.ui }));
vi.mock("../../api/runtime.js", () => ({
  getAuthToken: mocks.getAuthToken,
  setAuthToken: mocks.setAuthToken,
  setServerUrl: mocks.setServerUrl,
  isRemoteConnection: mocks.isRemoteConnection,
}));
vi.mock("../../i18n/index.svelte", () => ({ t: (key: string) => key }));

vi.mock("./AppearanceSettings.svelte", () => ({ default: () => "appearance" }));
vi.mock("./AgentDirSettings.svelte", () => ({ default: () => "agent-dirs" }));
vi.mock("./TerminalSettings.svelte", () => ({ default: () => "terminal" }));
vi.mock("./GithubSettings.svelte", () => ({ default: () => "github" }));
vi.mock("./RemoteSettings.svelte", () => ({ default: () => "remote" }));
vi.mock("./WorktreeMappingSettings.svelte", () => ({ default: () => "worktree" }));
vi.mock("./LLMEnrichmentSettings.svelte", () => ({ default: () => "llm" }));
vi.mock("./MemoryBackupSettings.svelte", () => ({ default: () => "memory-backup" }));
vi.mock("./LanguageSwitcher.svelte", () => ({ default: () => "language" }));

// @ts-ignore - svelte component import
import SettingsPage from "./SettingsPage.svelte";

async function flush() {
  await Promise.resolve();
  await tick();
  await Promise.resolve();
  await tick();
}

describe("SettingsPage", () => {
  let component: ReturnType<typeof mount> | undefined;

  afterEach(() => {
    if (component) unmount(component);
    component = undefined;
    document.body.innerHTML = "";
    mocks.settings.loading = false;
    mocks.settings.needsAuth = false;
    mocks.settings.error = null;
    mocks.settings.load.mockClear();
  });

  it("does not render removed automatic memory extract/consolidate settings", async () => {
    component = mount(SettingsPage, { target: document.body });
    await flush();

    expect(document.body.textContent).not.toContain("Memory Extraction");
    expect(document.body.textContent).not.toContain("Memory Consolidation");
  });
});
