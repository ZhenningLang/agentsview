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
