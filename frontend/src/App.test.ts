// @vitest-environment jsdom
import { describe, expect, it } from "vitest";
import type { Message } from "./lib/api/types.js";
import type { DisplayItem } from "./lib/utils/display-items.js";
// @ts-ignore -- module-script export from a Svelte component
import { findUserPromptOrdinal } from "./App.svelte";
import appSource from "./App.svelte?raw";

function message(
  ordinal: number,
  role: Message["role"],
  isSystem = false,
): DisplayItem {
  return {
    kind: "message",
    ordinals: [ordinal],
    message: { role, is_system: isSystem } as Message,
  };
}

function toolGroup(...ordinals: number[]): DisplayItem {
  return {
    kind: "tool-group",
    ordinals,
    messages: [],
    timestamp: "",
  };
}

describe("findUserPromptOrdinal", () => {
  // Chronological ascending, exactly what MessageList.getDisplayItems() returns.
  const items: DisplayItem[] = [
    message(1, "user"),
    message(2, "assistant"),
    toolGroup(3),
    message(4, "user"),
    message(5, "assistant"),
    message(6, "user"),
  ];

  it("moves to the next user prompt from a user row", () => {
    expect(findUserPromptOrdinal(items, 1, 1, true)).toBe(4);
    expect(findUserPromptOrdinal(items, 4, 1, true)).toBe(6);
  });

  it("moves to the next user prompt from assistant and tool rows", () => {
    expect(findUserPromptOrdinal(items, 2, 1, true)).toBe(4);
    expect(findUserPromptOrdinal(items, 3, 1, true)).toBe(4);
    expect(findUserPromptOrdinal(items, 5, 1, true)).toBe(6);
  });

  it("moves backward past assistant and tool rows", () => {
    expect(findUserPromptOrdinal(items, 3, -1, true)).toBe(1);
    expect(findUserPromptOrdinal(items, 5, -1, true)).toBe(4);
    expect(findUserPromptOrdinal(items, 6, -1, true)).toBe(4);
  });

  it("picks an edge prompt when nothing is selected", () => {
    expect(findUserPromptOrdinal(items, null, 1, true)).toBe(1);
    expect(findUserPromptOrdinal(items, null, -1, true)).toBe(6);
  });

  it("treats an off-list selection like no selection", () => {
    expect(findUserPromptOrdinal(items, 99, 1, true)).toBe(1);
    expect(findUserPromptOrdinal(items, 99, -1, true)).toBe(6);
  });

  it("stops at the boundaries instead of wrapping", () => {
    expect(findUserPromptOrdinal(items, 6, 1, true)).toBeUndefined();
    expect(findUserPromptOrdinal(items, 1, -1, true)).toBeUndefined();
  });

  it("returns undefined when there is no user prompt to reach", () => {
    expect(
      findUserPromptOrdinal([message(1, "assistant"), toolGroup(2)], 1, 1, true),
    ).toBeUndefined();
    expect(findUserPromptOrdinal([], null, 1, true)).toBeUndefined();
  });

  it("skips system boundaries that carry the user role", () => {
    expect(
      findUserPromptOrdinal(
        [message(1, "user"), message(2, "user", true), message(3, "user")],
        1,
        1,
        true,
      ),
    ).toBe(3);
  });

  it("navigates nowhere while the user block type is filtered out", () => {
    expect(findUserPromptOrdinal(items, 2, 1, false)).toBeUndefined();
    expect(findUserPromptOrdinal(items, null, 1, false)).toBeUndefined();
    expect(findUserPromptOrdinal(items, 5, -1, false)).toBeUndefined();
  });

  it("is direction-agnostic: the caller inverts delta for newest-first", () => {
    // Rendered newest-first, App.svelte passes -delta, so a visual "next"
    // (Shift+J) walks the chronological list backwards.
    expect(findUserPromptOrdinal(items, 4, -1, true)).toBe(1);
    expect(findUserPromptOrdinal(items, 4, 1, true)).toBe(6);
  });
});

// `findUserPromptOrdinal` is direction-agnostic on purpose: the newest-first
// inversion lives at the single call site. That makes the inversion invisible
// to every assertion above — deleting it leaves them all green and only the
// browser tier (e2e/navigation.spec.ts) notices. This pins the wiring so the
// fast tier can honour QA5's "newest-first direction reversed" fail condition.
describe("navigateUserPrompt wiring", () => {
  const call = appSource
    .split("function navigateUserPrompt")[1]
    ?.split("function navigateToMessageOrdinal")[0] ?? "";

  it("inverts the delta for newest-first before calling the helper", () => {
    expect(call).toContain("ui.sortNewestFirst ? -delta : delta");
  });

  it("drives the helper from the rendered display items and block filter", () => {
    expect(call).toContain("messageListRef?.getDisplayItems()");
    expect(call).toContain('ui.isBlockVisible("user")');
    expect(call).toContain("navigateToMessageOrdinal(ordinal)");
  });
});
