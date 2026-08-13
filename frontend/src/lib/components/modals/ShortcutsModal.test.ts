// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { mount, tick, unmount } from "svelte";
import { setLocale } from "../../i18n/index.svelte.js";
// @ts-ignore
import ShortcutsModal from "./ShortcutsModal.svelte";

function rows(): Array<{ key: string; action: string }> {
  return Array.from(
    document.querySelectorAll<HTMLElement>(".shortcut-row"),
  ).map((row) => ({
    key: row.querySelector(".shortcut-key")?.textContent?.trim() ?? "",
    action: row.querySelector(".shortcut-action")?.textContent?.trim() ?? "",
  }));
}

describe("ShortcutsModal", () => {
  let component: ReturnType<typeof mount> | undefined;

  beforeEach(() => {
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

  async function open() {
    component = mount(ShortcutsModal, { target: document.body });
    await tick();
  }

  it("documents the user prompt navigation shortcuts", async () => {
    await open();

    expect(rows()).toEqual(
      expect.arrayContaining([
        { key: "Shift+J", action: "Next user prompt" },
        { key: "Shift+K", action: "Previous user prompt" },
      ]),
    );
  });

  it("localizes the new rows without dropping the existing ones", async () => {
    setLocale("zh");
    await open();

    const all = rows();
    expect(all).toEqual(
      expect.arrayContaining([
        { key: "Shift+J", action: "下一条用户提问" },
        { key: "Shift+K", action: "上一条用户提问" },
      ]),
    );
    // Existing rows are untouched English literals.
    expect(all.map((r) => r.key)).toEqual(
      expect.arrayContaining(["j / ↓", "k / ↑", "]", "[", "?"]),
    );
  });

  it("places the prompt shortcuts next to plain message navigation", async () => {
    await open();

    const keys = rows().map((r) => r.key);
    expect(keys.indexOf("Shift+J")).toBe(keys.indexOf("k / ↑") + 1);
    expect(keys.indexOf("Shift+K")).toBe(keys.indexOf("Shift+J") + 1);
  });
});
