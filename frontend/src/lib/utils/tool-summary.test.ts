import { describe, expect, it } from "vitest";

import type { ToolCall } from "../api/types.js";
import { summarizeToolCall } from "./tool-summary.js";

function call(overrides: Partial<ToolCall>): ToolCall {
  return {
    tool_name: "Bash",
    ...overrides,
  };
}

/** Build a tool call whose input_json is the JSON encoding of `params`. */
function withParams(
  toolName: string,
  params: Record<string, unknown>,
  extra: Partial<ToolCall> = {},
): ToolCall {
  return call({
    tool_name: toolName,
    input_json: JSON.stringify(params),
    ...extra,
  });
}

interface Case {
  name: string;
  toolCall: ToolCall;
  expected: string | null;
}

const MALFORMED_CASES: Case[] = [
  {
    name: "missing input_json yields no structured summary",
    toolCall: call({ tool_name: "Bash" }),
    expected: null,
  },
  {
    name: "empty input_json yields no structured summary",
    toolCall: call({ tool_name: "Bash", input_json: "" }),
    expected: null,
  },
  {
    name: "malformed JSON yields no structured summary",
    toolCall: call({ tool_name: "Bash", input_json: "{not json" }),
    expected: null,
  },
  {
    name: "JSON scalar input yields no structured summary",
    toolCall: call({ tool_name: "Bash", input_json: '"just a string"' }),
    expected: null,
  },
  {
    name: "JSON null input yields no structured summary",
    toolCall: call({ tool_name: "Bash", input_json: "null" }),
    expected: null,
  },
  {
    name: "unknown tool with unknown fields yields no structured summary",
    toolCall: withParams("MysteryTool", { wobble: 3, flange: "yes" }),
    expected: null,
  },
  {
    name: "known tool with only unknown fields yields no structured summary",
    toolCall: withParams("Read", { encoding: "utf8" }),
    expected: null,
  },
];

const BASH_CASES: Case[] = [
  {
    name: "Bash renders the command with a shell prefix",
    toolCall: withParams("Bash", { command: "ls -la /src" }),
    expected: "$ ls -la /src",
  },
  {
    name: "Bash uses the codex cmd alias",
    toolCall: withParams("Bash", { cmd: "git status" }),
    expected: "$ git status",
  },
  {
    name: "Bash prefers command over cmd when both are present",
    toolCall: withParams("Bash", { command: "make test", cmd: "ignored" }),
    expected: "$ make test",
  },
  {
    name: "Bash keeps only the first line of a multi-line command",
    toolCall: withParams("Bash", { command: "cd /src\nmake build\nexit 0" }),
    expected: "$ cd /src",
  },
  {
    name: "Bash never appends a result-derived count",
    toolCall: withParams(
      "Bash",
      { command: "echo hi" },
      { result_content: "hi\nthere\nagain\n" },
    ),
    expected: "$ echo hi",
  },
  {
    name: "Bash resolved by category rather than tool name",
    toolCall: withParams(
      "exec_command",
      { cmd: "pytest -q" },
      { category: "Bash" },
    ),
    expected: "$ pytest -q",
  },
  {
    name: "Bash with an empty command falls through to no summary",
    toolCall: withParams("Bash", { command: "" }),
    expected: null,
  },
];

const READ_CASES: Case[] = [
  {
    name: "Read counts result lines and ignores a single trailing newline",
    toolCall: withParams(
      "Read",
      { file_path: "/src/main.ts" },
      { result_content: "a\nb\nc\n" },
    ),
    expected: "/src/main.ts (3 lines)",
  },
  {
    name: "Read counts internal blank lines",
    toolCall: withParams(
      "Read",
      { file_path: "/src/main.ts" },
      { result_content: "a\n\nb\n" },
    ),
    expected: "/src/main.ts (3 lines)",
  },
  {
    name: "Read counts a single line with no trailing newline",
    toolCall: withParams(
      "Read",
      { file_path: "/src/main.ts" },
      { result_content: "only line" },
    ),
    expected: "/src/main.ts (1 lines)",
  },
  {
    name: "Read on an empty result emits no count",
    toolCall: withParams(
      "Read",
      { file_path: "/src/empty.ts" },
      { result_content: "" },
    ),
    expected: "/src/empty.ts",
  },
  {
    name: "Read on a newline-only result emits no count",
    toolCall: withParams(
      "Read",
      { file_path: "/src/empty.ts" },
      { result_content: "\n" },
    ),
    expected: "/src/empty.ts",
  },
  {
    name: "Read with no result content emits no count",
    toolCall: withParams("Read", { file_path: "/src/pending.ts" }),
    expected: "/src/pending.ts",
  },
  {
    name: "Read accepts the path alias",
    toolCall: withParams("Read", { path: "/src/alias.ts" }),
    expected: "/src/alias.ts",
  },
  {
    name: "Read accepts the filePath alias",
    toolCall: withParams("Read", { filePath: "/src/camel.ts" }),
    expected: "/src/camel.ts",
  },
  {
    name: "Read accepts the file alias",
    toolCall: withParams("Read", { file: "/src/short.ts" }),
    expected: "/src/short.ts",
  },
  {
    name: "Read without any path yields no structured summary",
    toolCall: withParams(
      "Read",
      { offset: 10 },
      { result_content: "a\nb\n" },
    ),
    expected: null,
  },
];

const EDIT_CASES: Case[] = [
  {
    name: "Edit reports added and removed line counts",
    toolCall: withParams("Edit", {
      file_path: "/src/app.ts",
      old_string: "one\ntwo",
      new_string: "one\ntwo\nthree\nfour",
    }),
    expected: "/src/app.ts (+4 -2)",
  },
  {
    name: "Edit accepts camelCase string params",
    toolCall: withParams("Edit", {
      file_path: "/src/app.ts",
      oldString: "gone",
      newString: "kept\nadded",
    }),
    expected: "/src/app.ts (+2 -1)",
  },
  {
    name: "Edit reports a pure insertion as removing nothing",
    toolCall: withParams("Edit", {
      file_path: "/src/app.ts",
      old_string: "",
      new_string: "inserted",
    }),
    expected: "/src/app.ts (+1 -0)",
  },
  {
    name: "Edit reports a pure deletion as adding nothing",
    toolCall: withParams("Edit", {
      file_path: "/src/app.ts",
      old_string: "dropped",
      new_string: "",
    }),
    expected: "/src/app.ts (+0 -1)",
  },
  {
    name: "Edit with two empty strings emits no count",
    toolCall: withParams("Edit", {
      file_path: "/src/app.ts",
      old_string: "",
      new_string: "",
    }),
    expected: "/src/app.ts",
  },
  {
    name: "Edit patch-only input does not guess line counts",
    toolCall: withParams(
      "Edit",
      {
        file_path: "/src/app.ts",
        patch: "@@ -1,2 +1,4 @@\n+three\n+four",
      },
      { result_content: "applied\npatch\n" },
    ),
    expected: "/src/app.ts",
  },
  {
    name: "Edit with a non-string old_string does not guess line counts",
    toolCall: withParams("Edit", {
      file_path: "/src/app.ts",
      old_string: 12,
      new_string: "one\ntwo",
    }),
    expected: "/src/app.ts",
  },
  {
    name: "Edit without a path yields no structured summary",
    toolCall: withParams("Edit", {
      old_string: "a",
      new_string: "b",
    }),
    expected: null,
  },
];

const WRITE_CASES: Case[] = [
  {
    name: "Write counts the written content lines",
    toolCall: withParams("Write", {
      file_path: "/src/new.ts",
      content: "one\ntwo\nthree",
    }),
    expected: "/src/new.ts (+3)",
  },
  {
    name: "Write ignores a single trailing newline in content",
    toolCall: withParams("Write", {
      file_path: "/src/new.ts",
      content: "one\ntwo\n",
    }),
    expected: "/src/new.ts (+2)",
  },
  {
    name: "Write with empty content emits no count",
    toolCall: withParams("Write", {
      file_path: "/src/new.ts",
      content: "",
    }),
    expected: "/src/new.ts",
  },
  {
    name: "Write with missing content emits no count",
    toolCall: withParams("Write", { file_path: "/src/new.ts" }),
    expected: "/src/new.ts",
  },
  {
    name: "Write with non-string content emits no count",
    toolCall: withParams("Write", {
      file_path: "/src/new.ts",
      content: { blob: true },
    }),
    expected: "/src/new.ts",
  },
  {
    name: "Write without a path yields no structured summary",
    toolCall: withParams("Write", { content: "orphan" }),
    expected: null,
  },
];

const GREP_CASES: Case[] = [
  {
    name: "Grep counts non-empty result lines as matches",
    toolCall: withParams(
      "Grep",
      { pattern: "handleError" },
      { result_content: "a.ts:1:handleError\nb.ts:9:handleError\n" },
    ),
    expected: "handleError (2 matches)",
  },
  {
    name: "Grep ignores blank lines when counting matches",
    toolCall: withParams(
      "Grep",
      { pattern: "handleError" },
      { result_content: "a.ts:1:handleError\n\n   \nb.ts:9:handleError\n" },
    ),
    expected: "handleError (2 matches)",
  },
  {
    name: "Grep with a blank result emits no match count",
    toolCall: withParams(
      "Grep",
      { pattern: "handleError" },
      { result_content: "\n   \n" },
    ),
    expected: "handleError",
  },
  {
    name: "Grep with no result content emits no match count",
    toolCall: withParams("Grep", { pattern: "handleError" }),
    expected: "handleError",
  },
  {
    name: "Grep in count output mode does not recount the summary output",
    toolCall: withParams(
      "Grep",
      { pattern: "handleError", output_mode: "count" },
      { result_content: "a.ts:12\nb.ts:3\n" },
    ),
    expected: "handleError",
  },
  {
    name: "Grep accepts the query alias for the pattern",
    toolCall: withParams(
      "Grep",
      { query: "needle" },
      { result_content: "hit\n" },
    ),
    expected: "needle (1 matches)",
  },
  {
    name: "Grep without a pattern yields no structured summary",
    toolCall: withParams(
      "Grep",
      { output_mode: "content" },
      { result_content: "hit\n" },
    ),
    expected: null,
  },
];

const GLOB_CASES: Case[] = [
  {
    name: "Glob counts non-empty result lines as files",
    toolCall: withParams(
      "Glob",
      { pattern: "**/*.ts" },
      { result_content: "a.ts\nb.ts\nc.ts\n" },
    ),
    expected: "**/*.ts (3 files)",
  },
  {
    name: "Glob with a blank result emits no file count",
    toolCall: withParams(
      "Glob",
      { pattern: "**/*.ts" },
      { result_content: "   \n\n" },
    ),
    expected: "**/*.ts",
  },
  {
    name: "Glob with no result content emits no file count",
    toolCall: withParams("Glob", { pattern: "**/*.ts" }),
    expected: "**/*.ts",
  },
  {
    name: "Glob without a pattern yields no structured summary",
    toolCall: withParams("Glob", { path: "/src" }),
    expected: null,
  },
];

const SPECIAL_CASES: Case[] = [
  {
    name: "TodoWrite surfaces the in-progress todo",
    toolCall: withParams("TodoWrite", {
      todos: [
        { content: "done thing", status: "completed" },
        { content: "current thing", status: "in_progress" },
        { content: "later thing", status: "pending" },
      ],
    }),
    expected: "→ current thing",
  },
  {
    name: "TodoWrite falls back to the last todo when none is in progress",
    toolCall: withParams("TodoWrite", {
      todos: [
        { content: "first", status: "completed" },
        { content: "last", status: "pending" },
      ],
    }),
    expected: "→ last",
  },
  {
    name: "TodoWrite with an empty list yields no structured summary",
    toolCall: withParams("TodoWrite", { todos: [] }),
    expected: null,
  },
  {
    name: "TodoWrite with a non-array todos field yields no summary",
    toolCall: withParams("TodoWrite", { todos: "nope" }),
    expected: null,
  },
  {
    name: "TaskCreate surfaces the subject",
    toolCall: withParams("TaskCreate", { subject: "ship the port" }),
    expected: "ship the port",
  },
  {
    name: "TaskUpdate joins id, status and subject",
    toolCall: withParams("TaskUpdate", {
      taskId: 42,
      status: "in_progress",
      subject: "ship the port",
    }),
    expected: "#42 · in_progress · ship the port",
  },
  {
    name: "TaskUpdate with only a status still summarizes",
    toolCall: withParams("TaskUpdate", { status: "done" }),
    expected: "done",
  },
  {
    name: "TaskUpdate with no known fields yields no summary",
    toolCall: withParams("TaskUpdate", { note: "hi" }),
    expected: null,
  },
  {
    name: "Skill surfaces the skill name",
    toolCall: withParams("Skill", { skill: "dev-tdd" }),
    expected: "dev-tdd",
  },
  {
    name: "lowercase skill surfaces the name alias",
    toolCall: withParams("skill", { name: "guard-review" }),
    expected: "guard-review",
  },
  {
    name: "ToolSearch surfaces the first query line",
    toolCall: withParams("ToolSearch", {
      query: "select:Read,Edit\nignored second line",
    }),
    expected: "select:Read,Edit",
  },
  {
    name: "Task surfaces the description",
    toolCall: withParams("Task", {
      description: "audit session helpers",
      prompt: "long prompt",
    }),
    expected: "audit session helpers",
  },
  {
    name: "Task falls back to the first prompt line",
    toolCall: withParams("Task", {
      prompt: "walk the helpers\nand report back",
    }),
    expected: "walk the helpers",
  },
  {
    name: "Agent is treated as a task call",
    toolCall: withParams("Agent", { description: "spawn a reviewer" }),
    expected: "spawn a reviewer",
  },
  {
    name: "Task category is treated as a task call",
    toolCall: withParams(
      "dispatch",
      { description: "delegated work" },
      { category: "Task" },
    ),
    expected: "delegated work",
  },
  {
    name: "subagent tool names are treated as task calls",
    toolCall: withParams("run_subagent", { description: "nested run" }),
    expected: "nested run",
  },
  {
    name: "task calls without a description or prompt yield no summary",
    toolCall: withParams("Task", { subagent_type: "explore" }),
    expected: null,
  },
];

const GENERIC_CASES: Case[] = [
  {
    name: "unknown tool with a file_path falls back to the path",
    toolCall: withParams("NotebookEdit", {
      file_path: "/src/notebook.ipynb",
    }),
    expected: "/src/notebook.ipynb",
  },
  {
    name: "unknown tool with a command falls back to the shell form",
    toolCall: withParams("run_terminal", { command: "npm test" }),
    expected: "$ npm test",
  },
  {
    name: "unknown tool with a pattern falls back to the pattern",
    toolCall: withParams("codebase_search", { pattern: "func main" }),
    expected: "func main",
  },
];

const CAP_CASES: Case[] = [
  {
    name: "long file paths are capped at 100 characters",
    toolCall: withParams("Read", { file_path: `/${"a".repeat(150)}` }),
    expected: `/${"a".repeat(99)}`,
  },
  {
    name: "long commands are capped at 100 characters after the prefix",
    toolCall: withParams("Bash", { command: "e".repeat(150) }),
    expected: `$ ${"e".repeat(100)}`,
  },
  {
    name: "long patterns are capped at 100 characters",
    toolCall: withParams("Glob", { pattern: "p".repeat(150) }),
    expected: "p".repeat(100),
  },
  {
    name: "long todo text is capped at 100 characters including the arrow",
    toolCall: withParams("TodoWrite", {
      todos: [{ content: "t".repeat(150), status: "in_progress" }],
    }),
    expected: `→ ${"t".repeat(98)}`,
  },
];

const GROUPS: Array<[string, Case[]]> = [
  ["malformed and unknown input", MALFORMED_CASES],
  ["Bash", BASH_CASES],
  ["Read", READ_CASES],
  ["Edit", EDIT_CASES],
  ["Write", WRITE_CASES],
  ["Grep", GREP_CASES],
  ["Glob", GLOB_CASES],
  ["special tools", SPECIAL_CASES],
  ["generic fallback", GENERIC_CASES],
  ["length caps", CAP_CASES],
];

describe("Phase 19 summarizeToolCall", () => {
  for (const [group, cases] of GROUPS) {
    describe(group, () => {
      for (const testCase of cases) {
        it(testCase.name, () => {
          expect(summarizeToolCall(testCase.toolCall)).toBe(
            testCase.expected,
          );
        });
      }
    });
  }

  it("never throws on hostile input", () => {
    const hostile: ToolCall[] = [
      call({ tool_name: "", input_json: "[]" }),
      call({ tool_name: "Read", input_json: "[1,2,3]" }),
      call({ tool_name: "Edit", input_json: '{"file_path":null}' }),
      call({
        tool_name: "Grep",
        input_json: '{"pattern":"x"}',
        result_content: " \n",
      }),
      call({ tool_name: "TodoWrite", input_json: '{"todos":[null]}' }),
      call({ tool_name: "Bash", input_json: '{"command":{"nested":1}}' }),
    ];

    for (const toolCall of hostile) {
      expect(() => summarizeToolCall(toolCall)).not.toThrow();
    }
  });
});
