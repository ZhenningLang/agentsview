// @vitest-environment jsdom
import {
  afterEach,
  describe,
  expect,
  it,
  vi,
} from "vitest";
import { mount, tick, unmount } from "svelte";
import type { Message, Session } from "../../api/types.js";

// @ts-ignore
import ToolCallGroup from "./ToolCallGroup.svelte";

const copyToClipboardMock = vi.hoisted(() =>
  vi.fn().mockResolvedValue(true),
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

vi.mock("../../utils/clipboard.js", () => ({
  copyToClipboard: copyToClipboardMock,
}));

function makeSession(overrides: Partial<Session> = {}): Session {
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

function makeToolMessage(overrides: Partial<Message> = {}): Message {
  return {
    id: 41,
    session_id: "session-1",
    ordinal: 7,
    role: "assistant",
    content: "",
    timestamp: "2026-02-20T12:31:00Z",
    has_thinking: false,
    thinking_text: "",
    has_tool_use: true,
    content_length: 0,
    model: "claude-opus",
    token_usage: null,
    context_tokens: 0,
    output_tokens: 0,
    has_context_tokens: false,
    has_output_tokens: false,
    is_system: false,
    tool_calls: [{
      tool_name: "Bash",
      tool_use_id: "toolu-1",
      input_json: `{"command":"pwd"}`,
      result_content: "ok",
    }],
    ...overrides,
  };
}

afterEach(() => {
  document.body.innerHTML = "";
  vi.clearAllMocks();
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

describe("ToolCallGroup", () => {
  it("phase18 exposes message-point fork for default grouped tool messages", async () => {
    phase18State.activeSession = makeSession({ id: "session-1", agent: "claude" });
    const component = mount(ToolCallGroup, {
      target: document.body,
      props: {
        messages: [makeToolMessage()],
        timestamp: "2026-02-20T12:31:00Z",
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
        from_ordinal: 7,
        fork_session: true,
      },
    });
    unmount(component);
  });
});
