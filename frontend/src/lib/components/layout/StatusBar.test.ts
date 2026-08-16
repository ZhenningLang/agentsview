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
// @ts-ignore
import StatusBar from "./StatusBar.svelte";
import { sync } from "../../stores/sync.svelte.js";

describe("StatusBar", () => {
  beforeEach(() => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date("2026-04-08T05:00:00Z"));
    const s = sync as unknown as Record<string, unknown>;
    sync.syncing = false;
    sync.backendSyncing = false;
    s.localProgress = null;
    s.backendProgress = null;
    sync.lastSync = "2026-04-08T05:00:00Z";
    sync.stats = null;
    sync.serverVersion = null;
    sync.versionMismatch = false;
    sync.remoteUnreachable = false;
    sync.backendDegraded = false;
    sync.backendDegradedMessage = null;
  });

  afterEach(() => {
    document.body.innerHTML = "";
    vi.useRealTimers();
    sync.lastSync = null;
    sync.stats = null;
    sync.serverVersion = null;
    sync.versionMismatch = false;
    sync.remoteUnreachable = false;
    sync.backendDegraded = false;
    sync.backendDegradedMessage = null;
    const s = sync as unknown as Record<string, unknown>;
    sync.backendSyncing = false;
    s.localProgress = null;
    s.backendProgress = null;
    sync.syncing = false;
  });

  it("refreshes the sync label as time passes", async () => {
    const component = mount(StatusBar, {
      target: document.body,
    });

    await tick();
    const syncLabel = document.querySelector(
      ".status-right span:last-of-type",
    );
    const expectedTitle = new Date(sync.lastSync!).toLocaleString(
      undefined,
      {
        month: "short",
        day: "numeric",
        hour: "2-digit",
        minute: "2-digit",
      },
    );

    expect(document.body.textContent).toContain(
      "synced just now",
    );
    expect(syncLabel?.getAttribute("title")).toBe(expectedTitle);

    await vi.advanceTimersByTimeAsync(70_000);
    await tick();

    expect(document.body.textContent).toContain(
      "synced 1m ago",
    );

    unmount(component);
  });

  it("shows a remote-unreachable indicator only when flagged", async () => {
    sync.remoteUnreachable = true;
    const component = mount(StatusBar, {
      target: document.body,
    });
    await tick();
    expect(document.body.textContent).toContain(
      "remote server unreachable",
    );

    sync.remoteUnreachable = false;
    await tick();
    expect(document.body.textContent).not.toContain(
      "remote server unreachable",
    );

    unmount(component);
  });

  it("shows a sync-not-ready indicator when backend is degraded", async () => {
    sync.backendDegraded = true;
    sync.backendDegradedMessage = "sync not ready";
    const component = mount(StatusBar, {
      target: document.body,
    });
    await tick();

    expect(document.body.textContent).toContain("sync not ready");
    expect(
      document.querySelector(".backend-warn")?.getAttribute("title"),
    ).toBe("sync not ready");

    sync.backendDegraded = false;
    await tick();
    expect(document.body.textContent).not.toContain("sync not ready");

    unmount(component);
  });

  it("shows Phase 22 progress detail with hint", async () => {
    const s = sync as unknown as Record<string, unknown>;
    sync.backendSyncing = true;
    s.backendProgress = {
      phase: "rebuilding_search",
      detail: "Rebuilding search index",
      hint: "Rebuilding the search index may take a while on large archives.",
      resync: true,
      projects_total: 0,
      projects_done: 0,
      sessions_total: 4,
      sessions_done: 3,
      messages_indexed: 10,
    };
    const component = mount(StatusBar, {
      target: document.body,
    });
    await tick();

    expect(document.body.textContent).toContain("Rebuilding search index");
    expect(
      document.querySelector(".sync-progress")?.getAttribute("title"),
    ).toBe("Rebuilding the search index may take a while on large archives.");

    unmount(component);
  });

  it("shows backend polling progress without local sync", async () => {
    const s = sync as unknown as Record<string, unknown>;
    sync.syncing = false;
    sync.backendSyncing = true;
    s.backendProgress = {
      phase: "parse",
      projects_total: 0,
      projects_done: 0,
      sessions_total: 10,
      sessions_done: 4,
      messages_indexed: 20,
    };

    const component = mount(StatusBar, {
      target: document.body,
    });
    await tick();

    expect(document.body.textContent).toContain("Syncing 40% (4/10)");

    unmount(component);
  });
});
