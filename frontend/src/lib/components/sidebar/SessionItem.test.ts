// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { mount, tick, unmount } from "svelte";
import { ui } from "../../stores/ui.svelte.js";

// @ts-ignore
import SessionItem from "./SessionItem.svelte";

describe("SessionItem", () => {
  let component: ReturnType<typeof mount> | undefined;

  beforeEach(() => {
    ui.setUseLlmTitle(true);
  });

  afterEach(() => {
    if (component) {
      unmount(component);
      component = undefined;
    }
    document.body.innerHTML = "";
    ui.setUseLlmTitle(false);
  });

  it("does not seed rename input from LLM title", async () => {
    component = mount(SessionItem, {
      target: document.body,
      props: {
        session: {
          id: "session-1",
          project: "proj-a",
          machine: "local",
          agent: "claude",
          first_message: "Original first message",
          display_name: null,
          llm_title: "LLM sidebar title",
          started_at: "2026-06-24T00:00:00Z",
          ended_at: "2026-06-24T00:01:00Z",
          created_at: "2026-06-24T00:00:00Z",
          message_count: 2,
          user_message_count: 1,
          is_automated: false,
        },
      },
    });
    await tick();

    const title = document.querySelector<HTMLElement>(".session-name");
    expect(title?.textContent).toContain("LLM sidebar title");

    title!.dispatchEvent(new MouseEvent("dblclick", { bubbles: true }));
    await tick();

    const input = document.querySelector<HTMLInputElement>(".rename-input");
    expect(input).not.toBeNull();
    expect(input!.value).toBe("Original first message");
  });
});
