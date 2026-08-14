// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from "vitest";
import { mount, tick, unmount } from "svelte";
import type { Message } from "../../api/types.js";
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
    sessions: [],
    activeSession: null,
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

afterEach(() => {
  document.body.innerHTML = "";
  vi.clearAllMocks();
  uiState.codeVisible = true;
  uiState.theme = "light";
});

describe("MessageContent", () => {
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
