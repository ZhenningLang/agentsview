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
import type { Message, Session } from "../../api/types.js";
import { inSessionSearch } from "../../stores/inSessionSearch.svelte.js";
import { messages } from "../../stores/messages.svelte.js";
import { readProgress } from "../../stores/read-progress.svelte.js";
import { sessions } from "../../stores/sessions.svelte.js";
import { ui } from "../../stores/ui.svelte.js";

const virtualizerMock = vi.hoisted(() => ({
  options: { count: 0 },
  scrollOffset: 0,
  visibleItems: [] as Array<{ index: number; key: string; start: number; end: number }>,
  getVirtualItems: vi.fn(() => virtualizerMock.visibleItems),
  getTotalSize: vi.fn(() => 120),
  measureElement: vi.fn(),
  scrollToIndex: vi.fn(),
  scrollToOffset: vi.fn(),
  getOffsetForIndex: vi.fn(),
}));

const phase18State = vi.hoisted(() => ({
  resume: vi.fn().mockResolvedValue({
    launched: true,
    terminal: "terminal",
    command: "",
  }),
}));

vi.mock("../../api/runtime.js", async (importOriginal) => {
  const actual = await importOriginal<typeof import("../../api/runtime.js")>();
  return {
    ...actual,
    configureGeneratedClient: vi.fn(),
    isRemoteConnection: () => false,
  };
});

vi.mock("../../api/generated/index", async (importOriginal) => {
  const actual =
    await importOriginal<typeof import("../../api/generated/index")>();
  return {
    ...actual,
    SessionsService: {
      ...actual.SessionsService,
      postApiV1SessionsIdResume: phase18State.resume,
    },
  };
});

vi.mock("../../virtual/createVirtualizer.svelte.js", () => ({
  createVirtualizer: (
    optsFn: () => { count: number },
  ) => ({
    get instance() {
      virtualizerMock.options.count = optsFn().count;
      return virtualizerMock;
    },
  }),
}));

// @ts-ignore
import MessageList from "./MessageList.svelte";

function makeMessage(ordinal: number): Message {
  return {
    id: ordinal + 1,
    session_id: "s1",
    ordinal,
    role: ordinal % 2 === 0 ? "user" : "assistant",
    content: `msg ${ordinal}`,
    timestamp: new Date(ordinal * 1000).toISOString(),
    has_thinking: false,
    thinking_text: "",
    has_tool_use: false,
    content_length: 6,
    model: "",
    token_usage: null,
    context_tokens: 0,
    output_tokens: 0,
    has_context_tokens: false,
    has_output_tokens: false,
    is_system: false,
  };
}

function makeSession(overrides: Partial<Session> = {}): Session {
  return {
    id: "s1",
    project: "proj",
    machine: "local",
    agent: "claude",
    first_message: "hello",
    started_at: "2026-02-20T12:30:00Z",
    ended_at: "2026-02-20T12:31:00Z",
    message_count: 2,
    user_message_count: 1,
    total_output_tokens: 0,
    peak_context_tokens: 0,
    is_automated: false,
    created_at: "2026-02-20T12:30:00Z",
    ...overrides,
  };
}

function makeToolMessage(ordinal: number): Message {
  return {
    ...makeMessage(ordinal),
    role: "assistant",
    content: "",
    has_tool_use: true,
    content_length: 0,
    tool_calls: [{
      tool_name: "Bash",
      tool_use_id: `toolu-${ordinal}`,
      input_json: `{"command":"pwd"}`,
      result_content: "ok",
    }],
  };
}

function deferred<T>() {
  let resolve!: (value: T | PromiseLike<T>) => void;
  const promise = new Promise<T>((res) => {
    resolve = res;
  });
  return { promise, resolve };
}

describe("MessageList follow cancellation", () => {
  let component: ReturnType<typeof mount> | undefined;
  let rafSpy: ReturnType<typeof vi.spyOn>;

  beforeEach(() => {
    vi.clearAllMocks();
    virtualizerMock.visibleItems = [];
    messages.clear();
    sessions.sessions = [];
    sessions.activeSessionId = "s1";
    messages.sessionId = "s1";
    messages.messages = [makeMessage(10)];
    messages.messageCount = 11;
    messages.hasOlder = true;
    ui.followLatest = true;
    ui.followLatestRequest = 1;
    ui.sortNewestFirst = false;
    ui.selectedOrdinal = null;
    ui.pendingScrollOrdinal = null;
    ui.pendingScrollSession = null;
    rafSpy = vi
      .spyOn(window, "requestAnimationFrame")
      .mockImplementation((cb: FrameRequestCallback) => {
        window.setTimeout(() => cb(performance.now()), 0);
        return 1;
      });
  });

  afterEach(() => {
    if (component) {
      unmount(component);
      component = undefined;
    }
    rafSpy.mockRestore();
    messages.clear();
    sessions.sessions = [];
    sessions.activeSessionId = null;
    ui.followLatest = false;
    document.body.innerHTML = "";
  });

  it("keeps delayed ordinal navigation alive after follow latest is disabled", async () => {
    const loaded = deferred<void>();
    const ensureSpy = vi
      .spyOn(messages, "ensureOrdinalLoaded")
      .mockImplementation(async () => {
        await loaded.promise;
        messages.messages = [makeMessage(0), makeMessage(10)];
      });

    component = mount(MessageList, { target: document.body });
    await tick();

    ui.setFollowLatest(false);
    (
      component as ReturnType<typeof mount> & {
        scrollToOrdinal: (ordinal: number) => void;
      }
    ).scrollToOrdinal(0);
    await tick();

    loaded.resolve();
    await tick();
    await vi.waitFor(() => {
      expect(virtualizerMock.scrollToIndex).toHaveBeenCalled();
    });

    expect(ensureSpy).toHaveBeenCalledWith(0);
    expect(virtualizerMock.scrollToIndex).toHaveBeenCalledWith(0, {
      align: "start",
    });
  });

  it("phase18 exposes fork action through the default tool-group render path", async () => {
    sessions.sessions = [makeSession({ id: "s1", agent: "claude" })];
    sessions.activeSessionId = "s1";
    messages.sessionId = "s1";
    messages.messages = [makeToolMessage(7)];
    messages.messageCount = 1;
    virtualizerMock.visibleItems = [{ index: 0, key: "row-0", start: 0, end: 80 }];

    component = mount(MessageList, { target: document.body });
    await tick();
    document.querySelector<HTMLButtonElement>(
      'button[aria-label="Fork from this message"]',
    )!.click();
    await tick();
    await Promise.resolve();

    expect(phase18State.resume).toHaveBeenCalledWith({
      id: "s1",
      requestBody: {
        from_ordinal: 7,
        fork_session: true,
      },
    });
  });
});

describe("Phase 19 MessageList skim layout", () => {
  let component: ReturnType<typeof mount> | undefined;

  beforeEach(() => {
    vi.clearAllMocks();
    virtualizerMock.visibleItems = [
      { index: 0, key: "row-0", start: 0, end: 80 },
    ];
    messages.clear();
    sessions.sessions = [makeSession({ id: "s1", agent: "claude" })];
    sessions.activeSessionId = "s1";
    messages.sessionId = "s1";
    messages.messages = [makeToolMessage(7)];
    messages.messageCount = 1;
    messages.loading = false;
    ui.followLatest = false;
    ui.selectedOrdinal = null;
    inSessionSearch.close();
  });

  afterEach(() => {
    if (component) {
      unmount(component);
      component = undefined;
    }
    inSessionSearch.close();
    ui.setLayout("default");
    messages.clear();
    sessions.sessions = [];
    sessions.activeSessionId = null;
    document.body.innerHTML = "";
  });

  function scrollClasses(): string {
    return (
      document.querySelector(".message-list-scroll")?.className ?? ""
    );
  }

  it("applies layout-skim to the transcript root when skim is selected", async () => {
    ui.setLayout("skim");
    component = mount(MessageList, { target: document.body });
    await tick();

    expect(scrollClasses()).toContain("layout-skim");
    expect(scrollClasses()).not.toContain("layout-default");
  });

  it("suspends skim to the default layout while a search highlight is active", async () => {
    ui.setLayout("skim");
    component = mount(MessageList, { target: document.body });
    await tick();
    expect(scrollClasses()).toContain("layout-skim");

    inSessionSearch.open();
    inSessionSearch.query = "pwd";
    await tick();

    expect(scrollClasses()).toContain("layout-default");
    expect(scrollClasses()).not.toContain("layout-skim");
    // The stored preference must not be rewritten -- only the applied class.
    expect(ui.messageLayout).toBe("skim");
  });

  it("restores layout-skim once the search is closed", async () => {
    ui.setLayout("skim");
    component = mount(MessageList, { target: document.body });
    await tick();

    inSessionSearch.open();
    inSessionSearch.query = "pwd";
    await tick();
    expect(scrollClasses()).toContain("layout-default");

    inSessionSearch.close();
    await tick();

    expect(scrollClasses()).toContain("layout-skim");
    expect(ui.messageLayout).toBe("skim");
  });

  it("treats a whitespace-only query as no highlight", async () => {
    ui.setLayout("skim");
    component = mount(MessageList, { target: document.body });
    await tick();

    inSessionSearch.open();
    inSessionSearch.query = "   ";
    await tick();

    expect(scrollClasses()).toContain("layout-skim");
  });

  it("leaves non-skim layouts untouched while searching", async () => {
    ui.setLayout("compact");
    component = mount(MessageList, { target: document.body });
    await tick();

    inSessionSearch.open();
    inSessionSearch.query = "pwd";
    await tick();

    expect(scrollClasses()).toContain("layout-compact");
  });

  it("keeps the tool summary in the skim row and the row clickable", async () => {
    ui.setLayout("skim");
    component = mount(MessageList, { target: document.body });
    await tick();

    expect(
      document.querySelector(".tool-header .tool-preview")?.textContent,
    ).toBe("$ pwd");

    document.querySelector<HTMLElement>(".virtual-row")!.click();
    await tick();

    expect(ui.selectedOrdinal).toBe(7);
    expect(
      document.querySelector(".virtual-row.selected"),
    ).not.toBeNull();
  });
});

describe("Phase 20 MessageList read progress", () => {
  let component: ReturnType<typeof mount> | undefined;
  let rafSpy: ReturnType<typeof vi.spyOn>;

  function rows(count: number) {
    return Array.from({ length: count }, (_, i) => ({
      index: i,
      key: `row-${i}`,
      start: i * 100,
      end: (i + 1) * 100,
    }));
  }

  beforeEach(() => {
    vi.clearAllMocks();
    readProgress.reset();
    messages.clear();
    sessions.sessions = [makeSession({ id: "s1" })];
    sessions.activeSessionId = "s1";
    messages.sessionId = "s1";
    messages.messages = [
      makeMessage(0),
      makeMessage(1),
      makeMessage(2),
    ];
    messages.messageCount = 3;
    messages.loading = false;
    messages.hasOlder = false;
    messages.activeSessionToken = "2";
    messages.activeSessionUnreadOrdinal = 1;
    virtualizerMock.visibleItems = rows(3);
    virtualizerMock.scrollOffset = 0;
    ui.followLatest = false;
    ui.sortNewestFirst = false;
    ui.selectedOrdinal = null;
    rafSpy = vi
      .spyOn(window, "requestAnimationFrame")
      .mockImplementation((cb: FrameRequestCallback) => {
        window.setTimeout(() => cb(performance.now()), 0);
        return 1;
      });
  });

  afterEach(() => {
    if (component) {
      unmount(component);
      component = undefined;
    }
    rafSpy.mockRestore();
    readProgress.reset();
    messages.clear();
    sessions.sessions = [];
    sessions.activeSessionId = null;
    ui.sortNewestFirst = false;
    document.body.innerHTML = "";
  });

  function dividers(): HTMLElement[] {
    return [
      ...document.querySelectorAll<HTMLElement>(".unread-divider"),
    ];
  }

  it("shows no boundary when the stored token still matches", async () => {
    readProgress.baseline("s1", "2", 2);

    component = mount(MessageList, { target: document.body });
    await tick();

    expect(dividers()).toHaveLength(0);
  });

  it("shows no boundary on a first visit with no marker", async () => {
    component = mount(MessageList, { target: document.body });
    await tick();

    expect(dividers()).toHaveLength(0);
  });

  it("marks the earliest changed ordinal when the token moved", async () => {
    readProgress.baseline("s1", "1", 2);

    component = mount(MessageList, { target: document.body });
    await tick();

    const divider = dividers();
    expect(divider).toHaveLength(1);
    expect(divider[0]!.textContent).toContain("New messages");
    expect(divider[0]!.getAttribute("aria-label")).toBe(
      "Read progress boundary",
    );

    const row = divider[0]!.closest(".virtual-row") as HTMLElement;
    expect(row.dataset.index).toBe("1");
    // Ascending order puts the boundary above the first unread message.
    expect(row.firstElementChild).toBe(divider[0]);
  });

  it("falls back to the message after the last read ordinal", async () => {
    readProgress.baseline("s1", "1", 0);
    messages.activeSessionUnreadOrdinal = null;

    component = mount(MessageList, { target: document.body });
    await tick();

    const row = dividers()[0]!.closest(".virtual-row") as HTMLElement;
    expect(row.dataset.index).toBe("1");
  });

  it("falls back to the oldest displayed message with no read ordinal", async () => {
    readProgress.baseline("s1", "1", null);
    messages.activeSessionUnreadOrdinal = null;

    component = mount(MessageList, { target: document.body });
    await tick();

    const row = dividers()[0]!.closest(".virtual-row") as HTMLElement;
    expect(row.dataset.index).toBe("0");
  });

  it("puts the boundary after the message in newest-first order", async () => {
    readProgress.baseline("s1", "1", 2);
    ui.sortNewestFirst = true;

    component = mount(MessageList, { target: document.body });
    await tick();

    const divider = dividers();
    expect(divider).toHaveLength(1);
    const row = divider[0]!.closest(".virtual-row") as HTMLElement;
    expect(row.lastElementChild).toBe(divider[0]);
  });

  it("confirms the revision once the boundary and the latest are seen", async () => {
    readProgress.baseline("s1", "1", 0);

    component = mount(MessageList, { target: document.body });
    await tick();
    document
      .querySelector<HTMLElement>(".message-list-scroll")!
      .dispatchEvent(new Event("scroll"));

    await vi.waitFor(() => {
      expect(readProgress.hasUnread("s1", "2")).toBe(false);
    });
    expect(readProgress.lastReadOrdinal("s1")).toBe(2);
  });

  it("does not confirm from the newest row alone", async () => {
    readProgress.baseline("s1", "1", 0);
    ui.sortNewestFirst = true;
    // Only the newest message is on screen; the rewritten earlier
    // message was never scrolled to.
    virtualizerMock.visibleItems = [
      { index: 0, key: "row-0", start: 0, end: 100 },
    ];

    component = mount(MessageList, { target: document.body });
    await tick();
    document
      .querySelector<HTMLElement>(".message-list-scroll")!
      .dispatchEvent(new Event("scroll"));
    await new Promise((r) => setTimeout(r, 5));

    expect(readProgress.hasUnread("s1", "2")).toBe(true);
    expect(readProgress.lastReadOrdinal("s1")).toBe(0);
  });

  it("does not confirm while older history is still unloaded", async () => {
    readProgress.baseline("s1", "1", 0);
    messages.hasOlder = true;
    // Scrolling near the top also asks for the previous page; stub it so
    // the assertion is about the read gate, not about pagination.
    vi.spyOn(messages, "loadOlder").mockResolvedValue(undefined);

    component = mount(MessageList, { target: document.body });
    await tick();
    document
      .querySelector<HTMLElement>(".message-list-scroll")!
      .dispatchEvent(new Event("scroll"));
    await new Promise((r) => setTimeout(r, 5));

    expect(readProgress.hasUnread("s1", "2")).toBe(true);
  });

  it("skips rows scrolled above the viewport", async () => {
    readProgress.baseline("s1", "1", 0);
    // Ordinals 0 and 1 sit entirely above the scroll offset.
    virtualizerMock.scrollOffset = 200;

    component = mount(MessageList, { target: document.body });
    await tick();
    document
      .querySelector<HTMLElement>(".message-list-scroll")!
      .dispatchEvent(new Event("scroll"));
    await new Promise((r) => setTimeout(r, 5));

    expect(readProgress.hasUnread("s1", "2")).toBe(true);
  });

  it("accumulates seen ordinals across scrolls", async () => {
    readProgress.baseline("s1", "1", 0);
    virtualizerMock.visibleItems = [
      { index: 0, key: "row-0", start: 0, end: 100 },
      { index: 1, key: "row-1", start: 100, end: 200 },
    ];

    component = mount(MessageList, { target: document.body });
    await tick();
    const scroller = document.querySelector<HTMLElement>(
      ".message-list-scroll",
    )!;
    scroller.dispatchEvent(new Event("scroll"));
    await new Promise((r) => setTimeout(r, 5));
    expect(readProgress.hasUnread("s1", "2")).toBe(true);

    virtualizerMock.visibleItems = [
      { index: 2, key: "row-2", start: 200, end: 300 },
    ];
    virtualizerMock.scrollOffset = 200;
    scroller.dispatchEvent(new Event("scroll"));

    await vi.waitFor(() => {
      expect(readProgress.hasUnread("s1", "2")).toBe(false);
    });
  });

  it("advances the read ordinal without a revision change", async () => {
    readProgress.baseline("s1", "2", 0);

    component = mount(MessageList, { target: document.body });
    await tick();
    document
      .querySelector<HTMLElement>(".message-list-scroll")!
      .dispatchEvent(new Event("scroll"));

    await vi.waitFor(() => {
      expect(readProgress.lastReadOrdinal("s1")).toBe(2);
    });
  });

  it("baselines a session that has never been seen", async () => {
    component = mount(MessageList, { target: document.body });
    await tick();
    document
      .querySelector<HTMLElement>(".message-list-scroll")!
      .dispatchEvent(new Event("scroll"));

    await vi.waitFor(() => {
      expect(readProgress.markerFor("s1")?.token).toBe("2");
    });
    expect(readProgress.hasUnread("s1", "2")).toBe(false);
  });

  it("does not confirm a partially visible tool group wholesale", async () => {
    readProgress.baseline("s1", "1", null);
    messages.messages = [
      makeToolMessage(0),
      makeToolMessage(1),
    ];
    messages.messageCount = 2;
    messages.activeSessionUnreadOrdinal = 0;
    // One grouped row straddling the scroll offset.
    virtualizerMock.visibleItems = [
      { index: 0, key: "row-0", start: 0, end: 300 },
    ];
    virtualizerMock.scrollOffset = 100;

    component = mount(MessageList, { target: document.body });
    await tick();
    document
      .querySelector<HTMLElement>(".message-list-scroll")!
      .dispatchEvent(new Event("scroll"));
    await new Promise((r) => setTimeout(r, 5));

    expect(readProgress.hasUnread("s1", "2")).toBe(true);
  });

  it("shows the boundary inside a grouped tool row", async () => {
    readProgress.baseline("s1", "1", null);
    messages.messages = [
      makeToolMessage(0),
      makeToolMessage(1),
    ];
    messages.messageCount = 2;
    messages.activeSessionUnreadOrdinal = 1;
    virtualizerMock.visibleItems = [
      { index: 0, key: "row-0", start: 0, end: 300 },
    ];

    component = mount(MessageList, { target: document.body });
    await tick();

    const divider = document.querySelector<HTMLElement>(
      ".tool-group .unread-divider",
    );
    expect(divider).not.toBeNull();
    expect(
      (divider!.nextElementSibling as HTMLElement).dataset.messageOrdinal,
    ).toBe("1");
  });

  it("cancels the pending read check on destroy", async () => {
    readProgress.baseline("s1", "1", 0);
    const cancel = vi.spyOn(window, "cancelAnimationFrame");

    component = mount(MessageList, { target: document.body });
    await tick();
    document
      .querySelector<HTMLElement>(".message-list-scroll")!
      .dispatchEvent(new Event("scroll"));

    unmount(component);
    component = undefined;

    expect(cancel).toHaveBeenCalled();
    cancel.mockRestore();
  });
});
