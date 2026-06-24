import { describe, expect, it } from "vitest";
import { sessionTitle } from "./session-title.js";

describe("sessionTitle", () => {
  const base = {
    project: "proj-a",
    display_name: "Original name",
    first_message: "first message title",
    llm_title: "LLM title",
  };

  it("uses original fallback when the toggle is off", () => {
    expect(sessionTitle(base, false)).toEqual({
      text: "Original name",
      isShell: false,
      source: "display_name",
    });
  });

  it("uses a non-empty LLM title when the toggle is on", () => {
    expect(sessionTitle(base, true)).toEqual({
      text: "LLM title",
      isShell: false,
      source: "llm_title",
    });
  });

  it("falls back when the LLM title is empty, whitespace, null, or missing", () => {
    for (const llm_title of ["", "   ", null, undefined]) {
      expect(sessionTitle({ ...base, display_name: null, llm_title }, true))
        .toMatchObject({
          text: "first message title",
          source: "first_message",
        });
    }
  });

  it("keeps teammate title extraction when not using LLM title", () => {
    const teammate = {
      project: "proj-a",
      first_message:
        '<teammate-message>You are a teammate on this team. Task #2: Align ROADMAP.md with implementation. 1. Read docs</teammate-message>',
      llm_title: "LLM teammate title",
    };

    expect(sessionTitle(teammate, false)).toMatchObject({
      text: "Align ROADMAP.md with implementation.",
      source: "first_message",
    });
  });
});
