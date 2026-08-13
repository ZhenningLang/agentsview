// @vitest-environment jsdom
// ABOUTME: Unit tests for ToolBlock's output section behavior.
// ABOUTME: Covers visibility, collapse/expand, and preview of result_content.
import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { mount, unmount, tick } from "svelte";
import type { ToolCall } from "../../api/types.js";

vi.mock("./SubagentInline.svelte", () => ({
  default: {},
}));

const copyToClipboardMock = vi.hoisted(() =>
  vi.fn().mockResolvedValue(true),
);

vi.mock("../../utils/clipboard.js", () => ({
  copyToClipboard: copyToClipboardMock,
}));

import { setLocale } from "../../i18n/index.svelte.js";

// @ts-ignore
import ToolBlock from "./ToolBlock.svelte";

describe("ToolBlock output section", () => {
  let component: ReturnType<typeof mount>;

  afterEach(() => {
    if (component) unmount(component);
    document.body.innerHTML = "";
  });

  it("does not render output-header when toolCall has no result_content", async () => {
    const toolCall: ToolCall = {
      tool_name: "Read",
      category: "file",
    };
    component = mount(ToolBlock, {
      target: document.body,
      props: { content: "some input", toolCall },
    });
    await tick();

    expect(document.querySelector(".output-header")).toBeNull();
  });

  it("does not render output-header when toolCall is absent", async () => {
    component = mount(ToolBlock, {
      target: document.body,
      props: { content: "some input" },
    });
    await tick();

    expect(document.querySelector(".output-header")).toBeNull();
  });

  it("renders output-header after expanding the tool block when result_content is set", async () => {
    const toolCall: ToolCall = {
      tool_name: "Read",
      category: "file",
      result_content: "line one\nline two",
    };
    component = mount(ToolBlock, {
      target: document.body,
      props: { content: "some input", toolCall },
    });
    await tick();

    // Output section is inside the collapsed block — not visible yet.
    expect(document.querySelector(".output-header")).toBeNull();

    // Expand the main tool block.
    const toolHeader = document.querySelector<HTMLButtonElement>(".tool-header");
    expect(toolHeader).not.toBeNull();
    toolHeader!.click();
    await tick();

    expect(document.querySelector(".output-header")).not.toBeNull();
  });

  it("output starts collapsed after expanding the tool block", async () => {
    const toolCall: ToolCall = {
      tool_name: "Read",
      category: "file",
      result_content: "line one\nline two",
    };
    component = mount(ToolBlock, {
      target: document.body,
      props: { content: "some input", toolCall },
    });
    await tick();

    document.querySelector<HTMLButtonElement>(".tool-header")!.click();
    await tick();

    // Output content pre block should not be present when output is collapsed.
    expect(document.querySelector(".output-content")).toBeNull();
  });

  it("expands output content on clicking output-header", async () => {
    const resultText = "line one\nline two\nline three";
    const toolCall: ToolCall = {
      tool_name: "Read",
      category: "file",
      result_content: resultText,
    };
    component = mount(ToolBlock, {
      target: document.body,
      props: { content: "some input", toolCall },
    });
    await tick();

    document.querySelector<HTMLButtonElement>(".tool-header")!.click();
    await tick();

    document.querySelector<HTMLButtonElement>(".output-header")!.click();
    await tick();

    const outputContent = document.querySelector(".output-content");
    expect(outputContent).not.toBeNull();
    expect(outputContent!.textContent).toBe(resultText);
  });

  it("shows first line as preview when output is collapsed", async () => {
    const toolCall: ToolCall = {
      tool_name: "Read",
      category: "file",
      result_content: "first line\nsecond line",
    };
    component = mount(ToolBlock, {
      target: document.body,
      props: { content: "some input", toolCall },
    });
    await tick();

    document.querySelector<HTMLButtonElement>(".tool-header")!.click();
    await tick();

    // Output is collapsed — preview should show first line.
    const outputHeader = document.querySelector(".output-header");
    expect(outputHeader).not.toBeNull();
    const preview = outputHeader!.querySelector(".tool-preview");
    expect(preview).not.toBeNull();
    expect(preview!.textContent).toBe("first line");
  });

  it("renders history after expanding the tool block when result_events are set", async () => {
    const toolCall: ToolCall = {
      tool_name: "wait",
      category: "Other",
      result_content: "latest summary",
      result_events: [
        {
          source: "wait_output",
          status: "completed",
          content: "Finished successfully",
          content_length: 21,
          agent_id: "agent-1",
          event_index: 0,
        },
      ],
    };
    component = mount(ToolBlock, {
      target: document.body,
      props: { content: "some input", toolCall },
    });
    await tick();

    expect(document.querySelector(".history-header")).toBeNull();

    document.querySelector<HTMLButtonElement>(".tool-header")!.click();
    await tick();

    expect(document.querySelector(".history-header")).not.toBeNull();
  });

  it("expands event history and shows chronological event content", async () => {
    const toolCall: ToolCall = {
      tool_name: "wait",
      category: "Other",
      result_content: "agent-a:\nFirst finished\n\nagent-b:\nSecond finished",
      result_events: [
        {
          source: "wait_output",
          status: "completed",
          content: "First finished",
          content_length: 14,
          agent_id: "agent-a",
          event_index: 0,
        },
        {
          source: "subagent_notification",
          status: "completed",
          content: "Second finished",
          content_length: 15,
          agent_id: "agent-b",
          event_index: 1,
        },
      ],
    };
    component = mount(ToolBlock, {
      target: document.body,
      props: { content: "some input", toolCall },
    });
    await tick();

    document.querySelector<HTMLButtonElement>(".tool-header")!.click();
    await tick();
    document.querySelector<HTMLButtonElement>(".history-header")!.click();
    await tick();

    const historyEntries = Array.from(document.querySelectorAll(".history-content"));
    expect(historyEntries).toHaveLength(2);
    expect(historyEntries[0]!.textContent).toBe("First finished");
    expect(historyEntries[1]!.textContent).toBe("Second finished");
  });
});

describe("ToolBlock fallback content", () => {
  let component: ReturnType<typeof mount>;

  afterEach(() => {
    if (component) unmount(component);
    document.body.innerHTML = "";
  });

  it("renders fallback content when content is empty and category matches", async () => {
    // Edit category should show file path from input_json
    const toolCall: ToolCall = {
      tool_name: "custom_edit",
      category: "Edit",
      input_json: JSON.stringify({ file_path: "/path/to/file.txt" }),
    };
    component = mount(ToolBlock, {
      target: document.body,
      props: { content: "", toolCall },
    });
    await tick();

    // Expand to see content
    document.querySelector<HTMLButtonElement>(".tool-header")!.click();
    await tick();

    const toolContent = document.querySelector(".tool-content");
    expect(toolContent).not.toBeNull();
    expect(toolContent!.textContent).toContain("file_path: /path/to/file.txt");
  });

  it("renders fallback content for Write tools", async () => {
    const toolCall: ToolCall = {
      tool_name: "custom_write",
      category: "Write",
      input_json: JSON.stringify({ file_path: "/output.txt", content: "Hello World" }),
    };
    component = mount(ToolBlock, {
      target: document.body,
      props: { content: "", toolCall },
    });
    await tick();

    document.querySelector<HTMLButtonElement>(".tool-header")!.click();
    await tick();

    const diffView = document.querySelector(".diff-view");
    expect(diffView).not.toBeNull();
    expect(diffView!.textContent).toContain("Hello World");
  });

  it("falls back to tool_name when category has no specific pattern", async () => {
    // apply_patch doesn't match Edit pattern (which expects old_string/new_string)
    // so it should fall back to generic key-value output
    const toolCall: ToolCall = {
      tool_name: "apply_patch",
      category: "Edit",
      input_json: JSON.stringify({ patch_file: "/path/to/patch.diff" }),
    };
    component = mount(ToolBlock, {
      target: document.body,
      props: { content: "", toolCall },
    });
    await tick();

    document.querySelector<HTMLButtonElement>(".tool-header")!.click();
    await tick();

    const toolContent = document.querySelector(".tool-content");
    expect(toolContent).not.toBeNull();
    // Should show the generic key-value output with exact format
    expect(toolContent!.textContent).toContain("patch_file: /path/to/patch.diff");
  });

  it("renders fallback content when no category is provided", async () => {
    // Tool without category - should use tool_name directly
    const toolCall: ToolCall = {
      tool_name: "apply_patch",
      input_json: JSON.stringify({ patch_file: "/path/to/patch.diff" }),
    };
    component = mount(ToolBlock, {
      target: document.body,
      props: { content: "", toolCall },
    });
    await tick();

    document.querySelector<HTMLButtonElement>(".tool-header")!.click();
    await tick();

    const toolContent = document.querySelector(".tool-content");
    expect(toolContent).not.toBeNull();
    // apply_patch doesn't have specific handling, so should show generic output
    expect(toolContent!.textContent).toContain("patch_file: /path/to/patch.diff");
  });

  it("falls back to tool_name when category is empty string", async () => {
    // Empty string category should be treated same as no category
    const toolCall: ToolCall = {
      tool_name: "apply_patch",
      category: "",
      input_json: JSON.stringify({ patch_file: "/path/to/patch.diff" }),
    };
    component = mount(ToolBlock, {
      target: document.body,
      props: { content: "", toolCall },
    });
    await tick();

    document.querySelector<HTMLButtonElement>(".tool-header")!.click();
    await tick();

    const toolContent = document.querySelector(".tool-content");
    expect(toolContent).not.toBeNull();
    // Should fall back to tool_name and show generic output
    expect(toolContent!.textContent).toContain("patch_file: /path/to/patch.diff");
  });

  it("does not render fallback content when content is provided", async () => {
    const toolCall: ToolCall = {
      tool_name: "custom_tool",
      input_json: JSON.stringify({ param: "value" }),
    };
    component = mount(ToolBlock, {
      target: document.body,
      props: { content: "Explicit content here", toolCall },
    });
    await tick();

    document.querySelector<HTMLButtonElement>(".tool-header")!.click();
    await tick();

    const toolContent = document.querySelector(".tool-content");
    expect(toolContent).not.toBeNull();
    expect(toolContent!.textContent).toBe("Explicit content here");
  });

  it("does not render fallback content when input_json is empty", async () => {
    const toolCall: ToolCall = {
      tool_name: "custom_tool",
    };
    component = mount(ToolBlock, {
      target: document.body,
      props: { content: "", toolCall },
    });
    await tick();

    document.querySelector<HTMLButtonElement>(".tool-header")!.click();
    await tick();

    const toolContent = document.querySelector(".tool-content");
    expect(toolContent).toBeNull();
  });

  it("does not render fallback content when no toolCall is provided", async () => {
    component = mount(ToolBlock, {
      target: document.body,
      props: { content: "" },
    });
    await tick();

    document.querySelector<HTMLButtonElement>(".tool-header")!.click();
    await tick();

    const toolContent = document.querySelector(".tool-content");
    expect(toolContent).toBeNull();
  });
});

describe("ToolBlock show-more for long content", () => {
  let component: ReturnType<typeof mount>;

  afterEach(() => {
    if (component) unmount(component);
    document.body.innerHTML = "";
  });

  it("shows 'show all' button for long Bash fallback content", async () => {
    const longCommand = Array.from({ length: 30 }, (_, i) => `echo line${i}`).join("\n");
    const toolCall: ToolCall = {
      tool_name: "Bash",
      category: "Bash",
      input_json: JSON.stringify({ command: longCommand }),
    };
    component = mount(ToolBlock, {
      target: document.body,
      props: { content: "", toolCall },
    });
    await tick();

    document.querySelector<HTMLButtonElement>(".tool-header")!.click();
    await tick();

    const showMoreBtn = document.querySelector(".show-more-btn");
    expect(showMoreBtn).not.toBeNull();
    expect(showMoreBtn!.textContent).toContain("show all");
  });

  it("expands to full content when 'show all' is clicked", async () => {
    const longCommand = Array.from({ length: 30 }, (_, i) => `echo line${i}`).join("\n");
    const toolCall: ToolCall = {
      tool_name: "Bash",
      category: "Bash",
      input_json: JSON.stringify({ command: longCommand }),
    };
    component = mount(ToolBlock, {
      target: document.body,
      props: { content: "", toolCall },
    });
    await tick();

    document.querySelector<HTMLButtonElement>(".tool-header")!.click();
    await tick();

    const contentBefore = document.querySelector(".tool-content")!.textContent!;
    expect(contentBefore).not.toContain("echo line29");

    document.querySelector<HTMLButtonElement>(".show-more-btn")!.click();
    await tick();

    const contentAfter = document.querySelector(".tool-content")!.textContent!;
    expect(contentAfter).toContain("echo line29");

    const showMoreBtn = document.querySelector(".show-more-btn");
    expect(showMoreBtn!.textContent).toContain("show less");
  });

  it("does not show 'show all' button for short content", async () => {
    const toolCall: ToolCall = {
      tool_name: "Bash",
      category: "Bash",
      input_json: JSON.stringify({ command: "npm test" }),
    };
    component = mount(ToolBlock, {
      target: document.body,
      props: { content: "", toolCall },
    });
    await tick();

    document.querySelector<HTMLButtonElement>(".tool-header")!.click();
    await tick();

    expect(document.querySelector(".show-more-btn")).toBeNull();
  });

  it("auto-expands hidden Bash fallback content on search match", async () => {
    const longCommand = Array.from(
      { length: 30 },
      (_, i) => `echo hidden-line-${i}`,
    ).join("\n");
    const toolCall: ToolCall = {
      tool_name: "Bash",
      category: "Bash",
      input_json: JSON.stringify({ command: longCommand }),
    };
    component = mount(ToolBlock, {
      target: document.body,
      props: {
        content: "",
        toolCall,
        highlightQuery: "hidden-line-29",
      },
    });
    await tick();

    const toolContent = document.querySelector(".tool-content");
    expect(toolContent).not.toBeNull();
    expect(toolContent!.textContent).toContain("hidden-line-29");
    expect(document.querySelector(".show-more-btn")!.textContent).toContain(
      "show less",
    );
  });
});

describe("ToolBlock collapsed preview", () => {
  let component: ReturnType<typeof mount>;

  afterEach(() => {
    if (component) unmount(component);
    document.body.innerHTML = "";
  });

  it("shows codex bash command (cmd key) when content is empty", async () => {
    const toolCall: ToolCall = {
      tool_name: "exec_command",
      category: "Bash",
      input_json: JSON.stringify({
        cmd: "nl -ba file.md",
        workdir: "/x",
      }),
    };
    component = mount(ToolBlock, {
      target: document.body,
      props: { content: "", label: "Bash", toolCall },
    });
    await tick();

    const preview = document.querySelector(".tool-header .tool-preview");
    expect(preview).not.toBeNull();
    expect(preview!.textContent).toBe("$ nl -ba file.md");
  });

  it("shows claude bash command (command key) when content is empty", async () => {
    const toolCall: ToolCall = {
      tool_name: "Bash",
      category: "Bash",
      input_json: JSON.stringify({ command: "ls -la" }),
    };
    component = mount(ToolBlock, {
      target: document.body,
      props: { content: "", label: "Bash", toolCall },
    });
    await tick();

    const preview = document.querySelector(".tool-header .tool-preview");
    expect(preview).not.toBeNull();
    expect(preview!.textContent).toBe("$ ls -la");
  });

  it("shows only the first line of multi-line bash commands", async () => {
    const toolCall: ToolCall = {
      tool_name: "exec_command",
      category: "Bash",
      input_json: JSON.stringify({
        cmd: "cat <<EOF\nhello\nworld\nEOF",
      }),
    };
    component = mount(ToolBlock, {
      target: document.body,
      props: { content: "", label: "Bash", toolCall },
    });
    await tick();

    const preview = document.querySelector(".tool-header .tool-preview");
    expect(preview).not.toBeNull();
    expect(preview!.textContent).toBe("$ cat <<EOF");
  });

  it("prefers explicit content over command fallback", async () => {
    const toolCall: ToolCall = {
      tool_name: "exec_command",
      category: "Bash",
      input_json: JSON.stringify({ cmd: "from json" }),
    };
    component = mount(ToolBlock, {
      target: document.body,
      props: { content: "$ from content", label: "Bash", toolCall },
    });
    await tick();

    const preview = document.querySelector(".tool-header .tool-preview");
    expect(preview!.textContent).toBe("$ from content");
  });

  it("shows in-progress todo content for TodoWrite", async () => {
    const toolCall: ToolCall = {
      tool_name: "TodoWrite",
      input_json: JSON.stringify({
        todos: [
          { content: "first done task", status: "completed" },
          { content: "current work", status: "in_progress" },
          { content: "future task", status: "pending" },
        ],
      }),
    };
    component = mount(ToolBlock, {
      target: document.body,
      props: { content: "", label: "TodoWrite", toolCall },
    });
    await tick();

    const preview = document.querySelector(".tool-header .tool-preview");
    expect(preview).not.toBeNull();
    expect(preview!.textContent).toBe("→ current work");
  });

  it("falls back to last todo when none are in-progress", async () => {
    const toolCall: ToolCall = {
      tool_name: "TodoWrite",
      input_json: JSON.stringify({
        todos: [
          { content: "task one", status: "completed" },
          { content: "task two", status: "completed" },
        ],
      }),
    };
    component = mount(ToolBlock, {
      target: document.body,
      props: { content: "", label: "TodoWrite", toolCall },
    });
    await tick();

    const preview = document.querySelector(".tool-header .tool-preview");
    expect(preview!.textContent).toBe("→ task two");
  });

  it("prefers TodoWrite synthesis over content first line", async () => {
    const toolCall: ToolCall = {
      tool_name: "TodoWrite",
      input_json: JSON.stringify({
        todos: [{ content: "do thing", status: "in_progress" }],
      }),
    };
    component = mount(ToolBlock, {
      target: document.body,
      props: {
        content: "[Todo List]\n  → do thing",
        label: "TodoWrite",
        toolCall,
      },
    });
    await tick();

    const preview = document.querySelector(".tool-header .tool-preview");
    expect(preview!.textContent).toBe("→ do thing");
  });

  it("shows subject for TaskCreate", async () => {
    const toolCall: ToolCall = {
      tool_name: "TaskCreate",
      input_json: JSON.stringify({
        subject: "Rebuild Companies list table columns",
        description: "long description here",
      }),
    };
    component = mount(ToolBlock, {
      target: document.body,
      props: { content: "", label: "TaskCreate", toolCall },
    });
    await tick();

    const preview = document.querySelector(".tool-header .tool-preview");
    expect(preview!.textContent).toBe("Rebuild Companies list table columns");
  });

  it("shows task id, status, and subject for TaskUpdate", async () => {
    const toolCall: ToolCall = {
      tool_name: "TaskUpdate",
      input_json: JSON.stringify({
        taskId: 29,
        status: "in_progress",
        subject: "Rebuild Companies list table columns",
      }),
    };
    component = mount(ToolBlock, {
      target: document.body,
      props: { content: "", label: "TaskUpdate", toolCall },
    });
    await tick();

    const preview = document.querySelector(".tool-header .tool-preview");
    expect(preview!.textContent).toBe(
      "#29 · in_progress · Rebuild Companies list table columns",
    );
  });

  it("shows just task id and status for TaskUpdate without subject", async () => {
    const toolCall: ToolCall = {
      tool_name: "TaskUpdate",
      input_json: JSON.stringify({
        taskId: 29,
        status: "completed",
      }),
    };
    component = mount(ToolBlock, {
      target: document.body,
      props: { content: "", label: "TaskUpdate", toolCall },
    });
    await tick();

    const preview = document.querySelector(".tool-header .tool-preview");
    expect(preview!.textContent).toBe("#29 · completed");
  });

  it("shows skill name for Skill tool", async () => {
    const toolCall: ToolCall = {
      tool_name: "Skill",
      input_json: JSON.stringify({ skill: "roborev-review-branch" }),
    };
    component = mount(ToolBlock, {
      target: document.body,
      props: { content: "", label: "Skill", toolCall },
    });
    await tick();

    const preview = document.querySelector(".tool-header .tool-preview");
    expect(preview!.textContent).toBe("roborev-review-branch");
  });

  it("shows skill name from name field for lowercase skill tool", async () => {
    const toolCall: ToolCall = {
      tool_name: "skill",
      input_json: JSON.stringify({ name: "my-skill" }),
    };
    component = mount(ToolBlock, {
      target: document.body,
      props: { content: "", label: "skill", toolCall },
    });
    await tick();

    const preview = document.querySelector(".tool-header .tool-preview");
    expect(preview!.textContent).toBe("my-skill");
  });

  it("shows query for ToolSearch", async () => {
    const toolCall: ToolCall = {
      tool_name: "ToolSearch",
      input_json: JSON.stringify({
        query: "select:TaskOutput,TaskGet",
        max_results: 2,
      }),
    };
    component = mount(ToolBlock, {
      target: document.body,
      props: { content: "", label: "ToolSearch", toolCall },
    });
    await tick();

    const preview = document.querySelector(".tool-header .tool-preview");
    expect(preview!.textContent).toBe("select:TaskOutput,TaskGet");
  });

  it("shows description for Task tool", async () => {
    const toolCall: ToolCall = {
      tool_name: "Task",
      input_json: JSON.stringify({
        subagent_type: "Explore",
        description: "Explore agentsview project",
      }),
    };
    component = mount(ToolBlock, {
      target: document.body,
      props: { content: "", label: "Task", toolCall },
    });
    await tick();

    const preview = document.querySelector(".tool-header .tool-preview");
    expect(preview!.textContent).toBe("Explore agentsview project");
  });

  it("falls back to prompt when description is missing for Task", async () => {
    const toolCall: ToolCall = {
      tool_name: "Task",
      input_json: JSON.stringify({
        subagent_type: "Explore",
        prompt: "Find the foo function\nand return its location",
      }),
    };
    component = mount(ToolBlock, {
      target: document.body,
      props: { content: "", label: "Task", toolCall },
    });
    await tick();

    const preview = document.querySelector(".tool-header .tool-preview");
    expect(preview!.textContent).toBe("Find the foo function");
  });

  it("uses Task preview for Agent tool", async () => {
    const toolCall: ToolCall = {
      tool_name: "Agent",
      input_json: JSON.stringify({
        description: "Audit ship readiness",
      }),
    };
    component = mount(ToolBlock, {
      target: document.body,
      props: { content: "", label: "Agent", toolCall },
    });
    await tick();

    const preview = document.querySelector(".tool-header .tool-preview");
    expect(preview!.textContent).toBe("Audit ship readiness");
  });

  it("uses Task preview for subagent-style tool names", async () => {
    const toolCall: ToolCall = {
      tool_name: "Zencoder_subagent__ZencoderSubagent",
      input_json: JSON.stringify({
        description: "Run subagent task",
      }),
    };
    component = mount(ToolBlock, {
      target: document.body,
      props: { content: "", label: "subagent", toolCall },
    });
    await tick();

    const preview = document.querySelector(".tool-header .tool-preview");
    expect(preview!.textContent).toBe("Run subagent task");
  });
});

describe("ToolBlock copy affordances", () => {
  let component: ReturnType<typeof mount> | undefined;

  const INPUT_COPY = 'button[aria-label="Copy tool input"]';
  const OUTPUT_COPY = 'button[aria-label="Copy tool output"]';

  beforeEach(() => {
    copyToClipboardMock.mockClear();
    copyToClipboardMock.mockResolvedValue(true);
    setLocale("en");
  });

  afterEach(() => {
    if (component) {
      unmount(component);
      component = undefined;
    }
    setLocale("zh");
    document.body.innerHTML = "";
  });

  function render(props: {
    content: string;
    label?: string;
    toolCall?: ToolCall;
  }) {
    component = mount(ToolBlock, { target: document.body, props });
    return tick();
  }

  async function clickCopy(selector: string) {
    document.querySelector<HTMLButtonElement>(selector)!.click();
    await Promise.resolve();
    await tick();
  }

  it("copies the full Task prompt without expanding the block", async () => {
    const prompt = Array.from(
      { length: 40 },
      (_, i) => `prompt line ${i + 1}`,
    ).join("\n");
    const toolCall: ToolCall = {
      tool_name: "Task",
      input_json: JSON.stringify({ prompt, description: "Do a thing" }),
    };
    await render({ content: "", label: "Task", toolCall });

    expect(document.querySelector(".tool-content")).toBeNull();
    await clickCopy(INPUT_COPY);

    expect(copyToClipboardMock).toHaveBeenCalledWith(prompt);
    // Copying must not expand the block.
    expect(document.querySelector(".tool-content")).toBeNull();
  });

  it("copies the full Bash fallback while the preview stays truncated", async () => {
    const command = Array.from(
      { length: 260 },
      (_, i) => `echo step-${i + 1}`,
    ).join("\n");
    const toolCall: ToolCall = {
      tool_name: "Bash",
      category: "Bash",
      input_json: JSON.stringify({
        command,
        description: "long script",
        agent__intent: "internal-only",
      }),
    };
    await render({ content: "", label: "Bash", toolCall });

    await clickCopy(INPUT_COPY);

    const copied = copyToClipboardMock.mock.calls[0]![0] as string;
    expect(copied).toBe(
      `description: long script\ncommand: ${command}`,
    );
    expect(copied).not.toContain("lines total");
    expect(copied).not.toContain("internal-only");

    // The rendered preview keeps the 20-line display cap.
    document.querySelector<HTMLButtonElement>(".tool-header")!.click();
    await tick();
    const shown = document.querySelector(".tool-content")!.textContent!;
    expect(shown.split("\n").length).toBe(20);
  });

  it("copies the full Edit diff, including a pre-computed patch", async () => {
    const oldStr = Array.from({ length: 150 }, (_, i) => `old ${i}`).join("\n");
    const newStr = Array.from({ length: 150 }, (_, i) => `new ${i}`).join("\n");
    await render({
      content: "",
      label: "Edit",
      toolCall: {
        tool_name: "Edit",
        category: "Edit",
        input_json: JSON.stringify({
          file_path: "/tmp/a.ts",
          old_string: oldStr,
          new_string: newStr,
        }),
      } satisfies ToolCall,
    });

    await clickCopy(INPUT_COPY);
    const copied = copyToClipboardMock.mock.calls[0]![0] as string;
    expect(copied.split("\n").length).toBe(301);
    expect(copied).not.toContain("lines total");
    expect(copied.endsWith("+new 149")).toBe(true);

    unmount(component!);
    component = undefined;
    document.body.innerHTML = "";
    copyToClipboardMock.mockClear();

    const patch = Array.from({ length: 240 }, (_, i) => `+patched ${i}`).join(
      "\n",
    );
    await render({
      content: "",
      label: "Edit",
      toolCall: {
        tool_name: "apply_patch",
        category: "Edit",
        input_json: JSON.stringify({ file_path: "/tmp/a.ts", patch }),
      } satisfies ToolCall,
    });

    await clickCopy(INPUT_COPY);
    expect(copyToClipboardMock).toHaveBeenCalledWith(patch);
  });

  it("copies the full Write content", async () => {
    const content = Array.from({ length: 250 }, (_, i) => `line ${i}`).join(
      "\n",
    );
    await render({
      content: "",
      label: "Write",
      toolCall: {
        tool_name: "Write",
        category: "Write",
        input_json: JSON.stringify({ file_path: "/tmp/a.ts", content }),
      } satisfies ToolCall,
    });

    await clickCopy(INPUT_COPY);
    const copied = copyToClipboardMock.mock.calls[0]![0] as string;
    expect(copied.split("\n").length).toBe(251);
    expect(copied).not.toContain("lines total");
    expect(copied.endsWith("+line 249")).toBe(true);
  });

  it("prefers explicit content over any fallback", async () => {
    await render({
      content: "explicit\nrendered\ncontent",
      label: "Read",
      toolCall: {
        tool_name: "Read",
        category: "Read",
        input_json: JSON.stringify({ file_path: "/tmp/a.ts" }),
      } satisfies ToolCall,
    });

    await clickCopy(INPUT_COPY);
    expect(copyToClipboardMock).toHaveBeenCalledWith(
      "explicit\nrendered\ncontent",
    );
  });

  it("renders no input copy button when there is nothing to copy", async () => {
    await render({ content: "", label: "Read" });
    expect(document.querySelector(INPUT_COPY)).toBeNull();
  });

  it("copies the exact result_content without expanding output", async () => {
    const resultText = "first output line\nsecond output line";
    await render({
      content: "some input",
      toolCall: {
        tool_name: "Read",
        category: "file",
        result_content: resultText,
      } satisfies ToolCall,
    });

    document.querySelector<HTMLButtonElement>(".tool-header")!.click();
    await tick();

    expect(document.querySelector(OUTPUT_COPY)).not.toBeNull();
    expect(document.querySelector(".output-content")).toBeNull();

    await clickCopy(OUTPUT_COPY);

    expect(copyToClipboardMock).toHaveBeenCalledWith(resultText);
    expect(document.querySelector(".output-content")).toBeNull();
    // The tool block itself stays expanded.
    expect(document.querySelector(".output-header")).not.toBeNull();

    document.querySelector<HTMLButtonElement>(".output-header")!.click();
    await tick();
    expect(document.querySelector(".output-content")?.textContent).toBe(
      resultText,
    );
  });

  it("tracks input and output copied state independently", async () => {
    vi.useFakeTimers();
    try {
      await render({
        content: "input text",
        toolCall: {
          tool_name: "Read",
          category: "file",
          result_content: "output text",
        } satisfies ToolCall,
      });
      document.querySelector<HTMLButtonElement>(".tool-header")!.click();
      await tick();

      document.querySelector<HTMLButtonElement>(INPUT_COPY)!.click();
      await Promise.resolve();
      await tick();

      expect(document.querySelector(INPUT_COPY)).toBeNull();
      expect(
        document.querySelector('button[aria-label="Copied tool input"]'),
      ).not.toBeNull();
      // The output button is untouched.
      expect(document.querySelector(OUTPUT_COPY)).not.toBeNull();

      vi.advanceTimersByTime(1600);
      await tick();
      expect(document.querySelector(INPUT_COPY)).not.toBeNull();
    } finally {
      vi.useRealTimers();
    }
  });

  it("does not show a copied state when the clipboard write fails", async () => {
    copyToClipboardMock.mockResolvedValue(false);
    await render({
      content: "input text",
      toolCall: {
        tool_name: "Read",
        category: "file",
        result_content: "output text",
      } satisfies ToolCall,
    });
    document.querySelector<HTMLButtonElement>(".tool-header")!.click();
    await tick();

    await clickCopy(INPUT_COPY);
    await clickCopy(OUTPUT_COPY);

    expect(
      document.querySelector('button[aria-label="Copied tool input"]'),
    ).toBeNull();
    expect(
      document.querySelector('button[aria-label="Copied tool output"]'),
    ).toBeNull();
  });

  it("copies a long Pi edits[] input without truncating it", async () => {
    const longText = "y".repeat(500);
    await render({
      content: "",
      label: "Edit",
      toolCall: {
        tool_name: "pi_edit",
        category: "Edit",
        input_json: JSON.stringify({
          file_path: "/tmp/a.ts",
          edits: [{ op: "replace", pos: "1", lines: [longText] }],
        }),
      } satisfies ToolCall,
    });

    await clickCopy(INPUT_COPY);
    const copied = copyToClipboardMock.mock.calls[0]![0] as string;
    expect(copied).toContain(longText);
    expect(copied).not.toContain("\u2026");

    // The rendered preview keeps the 400-char per-edit truncation.
    document.querySelector<HTMLButtonElement>(".tool-header")!.click();
    await tick();
    expect(document.body.textContent).toContain("\u2026");
  });

  it("does not schedule a copied-state timer after unmount", async () => {
    vi.useFakeTimers();
    const setTimeoutSpy = vi.spyOn(globalThis, "setTimeout");
    let resolveClipboard!: (ok: boolean) => void;
    copyToClipboardMock.mockReturnValue(
      new Promise<boolean>((res) => {
        resolveClipboard = res;
      }),
    );
    try {
      await render({
        content: "input text",
        toolCall: {
          tool_name: "Read",
          category: "file",
          result_content: "output text",
        } satisfies ToolCall,
      });
      document.querySelector<HTMLButtonElement>(".tool-header")!.click();
      await tick();

      // Clipboard write is still in flight when the row is torn down, which is
      // what happens when a session switch unmounts the virtualized list.
      document.querySelector<HTMLButtonElement>(INPUT_COPY)!.click();
      unmount(component!);
      component = undefined;

      setTimeoutSpy.mockClear();
      resolveClipboard(true);
      await Promise.resolve();
      await Promise.resolve();
      await tick();

      // The late continuation must not arm a 1500ms timer nobody can clear.
      const armed = setTimeoutSpy.mock.calls.filter(
        ([, delay]) => delay === 1500,
      );
      expect(armed).toHaveLength(0);
    } finally {
      setTimeoutSpy.mockRestore();
      vi.useRealTimers();
    }
  });

  it("clears pending copied-state timers on unmount", async () => {
    vi.useFakeTimers();
    const clearSpy = vi.spyOn(globalThis, "clearTimeout");
    try {
      await render({
        content: "input text",
        toolCall: {
          tool_name: "Read",
          category: "file",
          result_content: "output text",
        } satisfies ToolCall,
      });
      document.querySelector<HTMLButtonElement>(".tool-header")!.click();
      await tick();
      document.querySelector<HTMLButtonElement>(INPUT_COPY)!.click();
      await Promise.resolve();
      await tick();

      clearSpy.mockClear();
      unmount(component!);
      component = undefined;
      // Svelte's own teardown also calls clearTimeout, so assert the copied
      // state actually stops rather than just that some timer was cleared.
      expect(clearSpy).toHaveBeenCalled();
      vi.advanceTimersByTime(1600);
      await tick();
      expect(document.querySelector(INPUT_COPY)).toBeNull();
      expect(
        document.querySelector('button[aria-label="Copied tool input"]'),
      ).toBeNull();
    } finally {
      clearSpy.mockRestore();
      vi.useRealTimers();
    }
  });
});
