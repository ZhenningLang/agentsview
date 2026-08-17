// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from "vitest";
import { mount, tick, unmount } from "svelte";
import type { Message, Session } from "../../api/types.js";
// @ts-ignore
import MessageContent from "./MessageContent.svelte";

const copyToClipboardMock = vi.hoisted(() =>
  vi.fn().mockResolvedValue(true),
);

/** Mutable UI state so individual tests can hide code blocks or flip theme
 *  without rebuilding the module mock. */
const uiState = vi.hoisted(() => ({
  codeVisible: true,
  theme: "light" as "light" | "dark",
}));

/** Stands in for the shared Mermaid runtime. Tests assert on the call count to
 *  prove the real runtime is never loaded on the search / hidden-block paths. */
const renderMermaidMock = vi.hoisted(() =>
  vi.fn(async (_source: string, _theme: string) =>
    '<svg class="fake-diagram" xmlns="http://www.w3.org/2000/svg"><text>ok</text></svg>',
  ),
);

const phase18State = vi.hoisted(() => ({
  activeSession: null as Session | null,
  sessions: [] as Session[],
  readOnly: false,
  remote: false,
  resume: vi.fn().mockResolvedValue({
    launched: true,
    terminal: "terminal",
    command: "",
  }),
}));

vi.mock("../../api/runtime.js", () => ({
  configureGeneratedClient: vi.fn(),
  isRemoteConnection: () => phase18State.remote,
}));

vi.mock("../../api/generated/index", async (importOriginal) => {
  const actual =
    await importOriginal<typeof import("../../api/generated/index")>();
  return {
    ...actual,
    SessionsService: {
      postApiV1SessionsIdResume: phase18State.resume,
    },
  };
});

vi.mock("../../stores/messages.svelte.js", () => ({
  messages: {
    sessionId: "",
    mainModel: "",
  },
}));

vi.mock("../../stores/ui.svelte.js", () => ({
  ui: {
    isBlockVisible: (kind: string) =>
      kind === "code" ? uiState.codeVisible : true,
    get theme() {
      return uiState.theme;
    },
  },
}));

vi.mock("../../utils/mermaid.js", async () => {
  const actual = await vi.importActual<
    typeof import("../../utils/mermaid.js")
  >("../../utils/mermaid.js");
  return {
    ...actual,
    renderMermaid: renderMermaidMock,
  };
});

vi.mock("../../stores/pins.svelte.js", () => ({
  pins: {
    isPinned: () => false,
    togglePin: vi.fn().mockResolvedValue(undefined),
  },
}));

vi.mock("../../stores/sessions.svelte.js", () => ({
  sessions: {
    get sessions() {
      return phase18State.sessions;
    },
    get activeSession() {
      return phase18State.activeSession;
    },
  },
}));

vi.mock("../../stores/sync.svelte.js", () => ({
  sync: {
    get readOnly() {
      return phase18State.readOnly;
    },
  },
}));

vi.mock("../../utils/highlight.js", async () => {
  const actual = await vi.importActual<
    typeof import("../../utils/highlight.js")
  >("../../utils/highlight.js");
  return {
    ...actual,
    applyHighlight: () => {},
  };
});

vi.mock("../../utils/clipboard.js", () => ({
  copyToClipboard: copyToClipboardMock,
}));

type MessageWithTokenFlags = Message & {
  has_context_tokens?: boolean;
  has_output_tokens?: boolean;
};

function makeMessage(
  overrides: Partial<MessageWithTokenFlags> = {},
): MessageWithTokenFlags {
  return {
    id: 1,
    session_id: "session-1",
    ordinal: 0,
    role: "assistant",
    content: "Token summary",
    timestamp: "2026-02-20T12:30:00Z",
    has_thinking: false,
    thinking_text: "",
    has_tool_use: false,
    content_length: 13,
    model: "claude-sonnet",
    token_usage: null,
    context_tokens: 0,
    output_tokens: 0,
    is_system: false,
    ...overrides,
  };
}

function makeSession(
  overrides: Partial<Session> = {},
): Session {
  return {
    id: "session-1",
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

afterEach(() => {
  document.body.innerHTML = "";
  vi.clearAllMocks();
  uiState.codeVisible = true;
  uiState.theme = "light";
  phase18State.activeSession = null;
  phase18State.sessions = [];
  phase18State.readOnly = false;
  phase18State.remote = false;
  phase18State.resume.mockResolvedValue({
    launched: true,
    terminal: "terminal",
    command: "",
  });
});

describe("MessageContent", () => {
  it("phase18 sends a message-point fork request from a Claude root message", async () => {
    phase18State.activeSession = makeSession({ id: "session-1", agent: "claude" });
    const component = mount(MessageContent, {
      target: document.body,
      props: {
        message: makeMessage({ session_id: "session-1", ordinal: 3 }),
      },
    });

    await tick();
    document.querySelector<HTMLButtonElement>(
      'button[aria-label="Fork from this message"]',
    )!.click();
    await tick();
    await Promise.resolve();

    expect(phase18State.resume).toHaveBeenCalledWith({
      id: "session-1",
      requestBody: {
        from_ordinal: 3,
        fork_session: true,
      },
    });
    unmount(component);
  });

  it("phase18 copies a command for local read-only message-point fork", async () => {
    phase18State.activeSession = makeSession({ id: "session-1", agent: "claude" });
    phase18State.readOnly = true;
    phase18State.remote = false;
    phase18State.resume.mockResolvedValue({
      launched: false,
      command: "claude < prompt",
    });
    const component = mount(MessageContent, {
      target: document.body,
      props: {
        message: makeMessage({ session_id: "session-1", ordinal: 5 }),
      },
    });

    await tick();
    document.querySelector<HTMLButtonElement>(
      'button[aria-label="Fork from this message"]',
    )!.click();
    await tick();
    await Promise.resolve();

    expect(phase18State.resume).toHaveBeenCalledWith({
      id: "session-1",
      requestBody: {
        command_only: true,
        from_ordinal: 5,
        fork_session: true,
      },
    });
    expect(copyToClipboardMock).toHaveBeenCalledWith("claude < prompt");
    unmount(component);
  });

  it("phase18 hides the action for remote read-only message-point fork", async () => {
    phase18State.activeSession = makeSession({ id: "session-1", agent: "claude" });
    phase18State.readOnly = true;
    phase18State.remote = true;
    const component = mount(MessageContent, {
      target: document.body,
      props: { message: makeMessage({ session_id: "session-1" }) },
    });

    await tick();
    expect(document.querySelector(
      'button[aria-label="Fork from this message"]',
    )).toBeNull();
    expect(phase18State.resume).not.toHaveBeenCalled();
    unmount(component);
  });

  it("phase18 uses explicit embedded child ownership instead of active parent", async () => {
    phase18State.activeSession = makeSession({ id: "parent", agent: "claude" });
    let component = mount(MessageContent, {
      target: document.body,
      props: {
        message: makeMessage({ session_id: "child", ordinal: 1 }),
        session: makeSession({ id: "child", agent: "codex" }),
        isSubagentContext: true,
      },
    });

    await tick();
    expect(document.querySelector(
      'button[aria-label="Fork from this message"]',
    )).toBeNull();
    unmount(component);
    document.body.innerHTML = "";

    phase18State.activeSession = makeSession({ id: "parent", agent: "codex" });
    component = mount(MessageContent, {
      target: document.body,
      props: {
        message: makeMessage({ session_id: "child", ordinal: 2 }),
        session: makeSession({ id: "child", agent: "claude" }),
        isSubagentContext: true,
      },
    });

    await tick();
    expect(document.querySelector(
      'button[aria-label="Fork from this message"]',
    )).not.toBeNull();
    unmount(component);
  });

  it("phase18 does not fall back to active parent while embedded session metadata is missing", async () => {
    phase18State.activeSession = makeSession({ id: "parent", agent: "claude" });
    const component = mount(MessageContent, {
      target: document.body,
      props: {
        message: makeMessage({ session_id: "child", ordinal: 4 }),
        session: null,
        isSubagentContext: true,
      },
    });

    await tick();
    expect(document.querySelector(
      'button[aria-label="Fork from this message"]',
    )).toBeNull();
    expect(phase18State.resume).not.toHaveBeenCalled();
    unmount(component);
  });

  it("renders compact token totals when both token metrics are reported", async () => {
    const component = mount(MessageContent, {
      target: document.body,
      props: {
        message: makeMessage({
          context_tokens: 2400,
          output_tokens: 180,
          has_context_tokens: true,
          has_output_tokens: true,
        }),
      },
    });

    await tick();
    const tokenMeta = document.querySelector(".message-tokens");
    expect(tokenMeta?.textContent?.replace(/\s+/g, " ").trim()).toBe(
      "2.4k ctx / 180 out",
    );

    unmount(component);
  });

  it("renders an explicit missing token placeholder when context tokens are absent", async () => {
    const component = mount(MessageContent, {
      target: document.body,
      props: {
        message: makeMessage({
          context_tokens: 0,
          output_tokens: 180,
          has_context_tokens: false,
          has_output_tokens: true,
        }),
      },
    });

    await tick();
    const tokenMeta = document.querySelector(".message-tokens");
    expect(tokenMeta?.textContent?.replace(/\s+/g, " ").trim()).toBe(
      "— ctx / 180 out",
    );

    unmount(component);
  });

  it("copies the exact raw content from a fenced code block", async () => {
    const code = "const answer = 42;\n";
    const content = `Here is code:\n\n\`\`\`ts\n${code}\`\`\``;
    const component = mount(MessageContent, {
      target: document.body,
      props: {
        message: makeMessage({
          content,
          content_length: content.length,
        }),
      },
    });

    await tick();
    const copyButton = document.querySelector<HTMLButtonElement>(
      'button.copy-btn[aria-label="Copy code block"]',
    );
    expect(copyButton).not.toBeNull();
    expect(copyButton!.querySelector("svg")).not.toBeNull();
    expect(copyButton!.textContent?.trim()).toBe("");

    copyButton!.click();
    await Promise.resolve();
    await tick();

    expect(copyToClipboardMock).toHaveBeenCalledWith(code);
    expect(copyButton!.getAttribute("aria-label")).toBe(
      "Copied code block",
    );
    expect(copyButton!.querySelector("svg")).not.toBeNull();
    expect(copyButton!.textContent?.trim()).toBe("");

    unmount(component);
  });
});

describe("MessageContent Mermaid routing", () => {
  const diagram = "flowchart LR\n  A[Start] --> B[End]\n";

  function mermaidMessage(label: string, source = diagram) {
    const content = `Here is a diagram:\n\n\`\`\`${label}\n${source}\`\`\``;
    return makeMessage({ content, content_length: content.length });
  }

  async function settle() {
    await tick();
    await Promise.resolve();
    await Promise.resolve();
    await tick();
  }

  it.each(["mermaid", "Mermaid", "MERMAID", "  mermaid  "])(
    "renders label %j as a diagram, not a plain code block",
    async (label) => {
      const component = mount(MessageContent, {
        target: document.body,
        props: { message: mermaidMessage(label) },
      });

      await settle();

      expect(renderMermaidMock).toHaveBeenCalledTimes(1);
      expect(renderMermaidMock.mock.calls[0]![0]).toBe(diagram);
      const host = document.querySelector(".mermaid-block");
      expect(host).not.toBeNull();
      expect(host!.querySelector("svg.fake-diagram")).not.toBeNull();
      // Once the diagram is up the raw source block is gone.
      expect(document.querySelector(".code-block")).toBeNull();

      unmount(component);
    },
  );

  it("keeps a non-mermaid fence on the CodeBlock path", async () => {
    const component = mount(MessageContent, {
      target: document.body,
      props: { message: mermaidMessage("typescript", "const x = 1;\n") },
    });

    await settle();

    expect(renderMermaidMock).not.toHaveBeenCalled();
    expect(document.querySelector(".mermaid-block")).toBeNull();
    expect(document.querySelector(".code-block")).not.toBeNull();

    unmount(component);
  });

  it("keeps an unlabeled fence on the CodeBlock path", async () => {
    const content = "text\n\n```\nplain\n```";
    const component = mount(MessageContent, {
      target: document.body,
      props: {
        message: makeMessage({ content, content_length: content.length }),
      },
    });

    await settle();

    expect(renderMermaidMock).not.toHaveBeenCalled();
    expect(document.querySelector(".code-block")).not.toBeNull();

    unmount(component);
  });

  it("falls back to searchable source while a search query is active", async () => {
    const component = mount(MessageContent, {
      target: document.body,
      props: {
        message: mermaidMessage("mermaid"),
        highlightQuery: "Start",
      },
    });

    await settle();

    // The Mermaid runtime must not even be asked for.
    expect(renderMermaidMock).not.toHaveBeenCalled();
    expect(document.querySelector(".mermaid-block")).toBeNull();

    const codeBlock = document.querySelector(".code-block");
    expect(codeBlock).not.toBeNull();
    expect(codeBlock!.textContent).toContain("A[Start] --> B[End]");
    expect(
      codeBlock!.querySelector('button.copy-btn[aria-label="Copy code block"]'),
    ).not.toBeNull();

    unmount(component);
  });

  it("copies the exact diagram source from the search fallback", async () => {
    const component = mount(MessageContent, {
      target: document.body,
      props: {
        message: mermaidMessage("mermaid"),
        highlightQuery: "Start",
      },
    });

    await settle();
    const copyButton = document.querySelector<HTMLButtonElement>(
      'button.copy-btn[aria-label="Copy code block"]',
    );
    expect(copyButton).not.toBeNull();
    copyButton!.click();
    await Promise.resolve();
    await tick();

    expect(copyToClipboardMock).toHaveBeenCalledWith(diagram);

    unmount(component);
  });

  it("does not mount or load Mermaid when code blocks are hidden", async () => {
    uiState.codeVisible = false;
    const component = mount(MessageContent, {
      target: document.body,
      props: { message: mermaidMessage("mermaid") },
    });

    await settle();

    expect(renderMermaidMock).not.toHaveBeenCalled();
    expect(document.querySelector(".mermaid-block")).toBeNull();
    expect(document.querySelector(".code-block")).toBeNull();

    unmount(component);
  });
});
