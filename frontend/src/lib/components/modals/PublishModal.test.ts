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
  getGithubConfig: vi.fn(),
  saveGithubConfig: vi.fn(),
  publishSession: vi.fn(),
  publishInsight: vi.fn(),
  configureGeneratedClient: vi.fn(),
}));

vi.mock("../../api/generated/index", () => ({
  ConfigService: {
    getApiV1ConfigGithub: mocks.getGithubConfig,
    postApiV1ConfigGithub: mocks.saveGithubConfig,
  },
  SessionsService: {
    postApiV1SessionsIdPublish: mocks.publishSession,
  },
  InsightsService: {
    postApiV1InsightsIdPublish: mocks.publishInsight,
  },
}));

vi.mock("../../api/runtime.js", () => ({
  configureGeneratedClient: mocks.configureGeneratedClient,
}));

import { ui } from "../../stores/ui.svelte.js";
import { sessions } from "../../stores/sessions.svelte.js";

// @ts-ignore
import PublishModal from "./PublishModal.svelte";

interface Deferred<T> {
  promise: Promise<T>;
  resolve: (value: T) => void;
  reject: (reason?: unknown) => void;
}

function deferred<T>(): Deferred<T> {
  let resolve!: (value: T) => void;
  let reject!: (reason?: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

/** Let the component's awaited chain settle before asserting. */
async function flush() {
  for (let i = 0; i < 6; i += 1) {
    await Promise.resolve();
    await tick();
  }
}

function gistResult(id: string) {
  return {
    gist_id: id,
    gist_url: `https://gist.github.com/${id}`,
    raw_url: `https://gist.githubusercontent.com/${id}/raw`,
    view_url: `https://viewer.example/${id}`,
  };
}

function closeButton(): HTMLButtonElement {
  const button = document.querySelector<HTMLButtonElement>(
    'button[aria-label="Close publish dialog"]',
  );
  expect(button).not.toBeNull();
  return button!;
}

function buttonByText(text: string): HTMLButtonElement | undefined {
  return Array.from(
    document.querySelectorAll<HTMLButtonElement>("button"),
  ).find((button) => button.textContent?.trim() === text);
}

function viewUrlValue(): string | null {
  const input = document.querySelector<HTMLInputElement>(
    "#publish-view-url",
  );
  return input ? input.value : null;
}

function errorText(): string | null {
  return (
    document.querySelector(".modal-error")?.textContent?.trim() ?? null
  );
}

describe("Phase 25 PublishModal target dispatch", () => {
  let component: ReturnType<typeof mount> | undefined;

  beforeEach(() => {
    vi.clearAllMocks();
    mocks.getGithubConfig.mockResolvedValue({ configured: true });
    mocks.saveGithubConfig.mockResolvedValue({});
    mocks.publishSession.mockResolvedValue(gistResult("session-gist"));
    mocks.publishInsight.mockResolvedValue(gistResult("insight-gist"));
    sessions.activeSessionId = null;
    ui.activeModal = null;
    ui.publishTarget = null;
    ui.publishSecret = false;
    document.body.innerHTML = "";
  });

  afterEach(() => {
    if (component) {
      unmount(component);
      component = undefined;
    }
    sessions.activeSessionId = null;
    ui.activeModal = null;
    ui.publishTarget = null;
    ui.publishSecret = false;
    document.body.innerHTML = "";
  });

  function mountModal() {
    component = mount(PublishModal, { target: document.body });
  }

  it("publishes a public gist for an insight target", async () => {
    ui.openPublish({ kind: "insight", id: 42 }, false);
    mountModal();
    await flush();

    expect(mocks.publishInsight).toHaveBeenCalledWith({
      id: 42,
      secret: false,
    });
    expect(mocks.publishSession).not.toHaveBeenCalled();
    expect(viewUrlValue()).toBe("https://viewer.example/insight-gist");
  });

  it("publishes a secret gist for an insight target", async () => {
    ui.openPublish({ kind: "insight", id: 7 }, true);
    mountModal();
    await flush();

    expect(mocks.publishInsight).toHaveBeenCalledWith({
      id: 7,
      secret: true,
    });
    expect(mocks.publishSession).not.toHaveBeenCalled();
  });

  it("still publishes sessions through SessionsService", async () => {
    ui.openPublish({ kind: "session", id: "sess-123" }, true);
    mountModal();
    await flush();

    expect(mocks.publishSession).toHaveBeenCalledWith({
      id: "sess-123",
      secret: true,
    });
    expect(mocks.publishInsight).not.toHaveBeenCalled();
    expect(viewUrlValue()).toBe("https://viewer.example/session-gist");
  });

  it("ignores the active session when the target is an insight", async () => {
    sessions.activeSessionId = "sess-999";
    ui.openPublish({ kind: "insight", id: 11 }, false);
    mountModal();
    await flush();

    expect(mocks.publishInsight).toHaveBeenCalledWith({
      id: 11,
      secret: false,
    });
    expect(mocks.publishSession).not.toHaveBeenCalled();
  });

  it("reports an error and publishes nothing without a target", async () => {
    sessions.activeSessionId = "sess-123";
    ui.publishTarget = null;
    ui.activeModal = "publish";
    mountModal();
    await flush();

    expect(mocks.publishSession).not.toHaveBeenCalled();
    expect(mocks.publishInsight).not.toHaveBeenCalled();
    expect(errorText()).toBe("No publish target selected");
  });

  it("keeps the same target across first-time token setup", async () => {
    mocks.getGithubConfig.mockResolvedValue({ configured: false });
    ui.openPublish({ kind: "insight", id: 5 }, true);
    mountModal();
    await flush();

    const input = document.querySelector<HTMLInputElement>(
      "input.token-input",
    );
    expect(input).not.toBeNull();
    input!.value = "phase25-token";
    input!.dispatchEvent(new Event("input", { bubbles: true }));
    await tick();

    // A later store change must not retarget the publish already in flight.
    ui.publishTarget = { kind: "session", id: "sess-later" };
    ui.publishSecret = false;

    buttonByText("Save & Publish")!.click();
    await flush();

    expect(mocks.saveGithubConfig).toHaveBeenCalledWith({
      requestBody: { token: "phase25-token" },
    });
    expect(mocks.publishInsight).toHaveBeenCalledWith({
      id: 5,
      secret: true,
    });
    expect(mocks.publishSession).not.toHaveBeenCalled();
  });

  it("retries the same target after a service error", async () => {
    mocks.publishInsight.mockRejectedValueOnce(new Error("gist boom"));
    ui.openPublish({ kind: "insight", id: 8 }, false);
    mountModal();
    await flush();

    expect(errorText()).toBe("gist boom");

    ui.publishTarget = { kind: "insight", id: 4321 };
    buttonByText("Retry")!.click();
    await flush();

    expect(mocks.publishInsight).toHaveBeenCalledTimes(2);
    expect(mocks.publishInsight).toHaveBeenLastCalledWith({
      id: 8,
      secret: false,
    });
  });

  it("does not publish when closed while the config call is pending", async () => {
    const config = deferred<{ configured: boolean }>();
    mocks.getGithubConfig.mockReturnValue(config.promise);
    ui.openPublish({ kind: "insight", id: 9 }, false);
    mountModal();
    await tick();

    closeButton().click();
    await tick();

    config.resolve({ configured: true });
    await flush();

    expect(mocks.publishInsight).not.toHaveBeenCalled();
    expect(ui.activeModal).toBeNull();
    expect(ui.publishTarget).toBeNull();
  });

  it("does not publish when unmounted while the config call is pending", async () => {
    const config = deferred<{ configured: boolean }>();
    mocks.getGithubConfig.mockReturnValue(config.promise);
    ui.openPublish({ kind: "insight", id: 10 }, false);
    mountModal();
    await tick();

    unmount(component!);
    component = undefined;

    config.resolve({ configured: true });
    await flush();

    expect(mocks.publishInsight).not.toHaveBeenCalled();
  });

  it("does not write success state when closed mid-publish", async () => {
    const publish = deferred<ReturnType<typeof gistResult>>();
    mocks.publishInsight.mockReturnValue(publish.promise);
    ui.openPublish({ kind: "insight", id: 12 }, false);
    mountModal();
    await flush();

    closeButton().click();
    await tick();

    publish.resolve(gistResult("late-gist"));
    await flush();

    expect(viewUrlValue()).toBeNull();
    expect(errorText()).toBeNull();
  });

  it("does not write error state when closed mid-publish", async () => {
    const publish = deferred<ReturnType<typeof gistResult>>();
    mocks.publishInsight.mockReturnValue(publish.promise);
    ui.openPublish({ kind: "insight", id: 13 }, false);
    mountModal();
    await flush();

    closeButton().click();
    await tick();

    publish.reject(new Error("late failure"));
    await flush();

    expect(errorText()).toBeNull();
    expect(viewUrlValue()).toBeNull();
  });

  it("clears the target when the overlay closes the modal", async () => {
    ui.openPublish({ kind: "insight", id: 14 }, false);
    mountModal();
    await flush();

    const overlay = document.querySelector<HTMLElement>(".modal-overlay");
    expect(overlay).not.toBeNull();
    overlay!.click();
    await tick();

    expect(ui.activeModal).toBeNull();
    expect(ui.publishTarget).toBeNull();
  });
});
