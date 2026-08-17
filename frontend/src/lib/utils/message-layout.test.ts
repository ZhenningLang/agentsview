import { describe, expect, it } from "vitest";

import type { MessageLayout } from "../stores/ui.svelte.js";
import { resolveMessageLayout } from "./message-layout.js";

const NON_SKIM_LAYOUTS: MessageLayout[] = [
  "default",
  "compact",
  "stream",
];

describe("Phase 19 resolveMessageLayout", () => {
  it("keeps skim while no search highlight is active", () => {
    expect(resolveMessageLayout("skim", false)).toBe("skim");
  });

  it("suspends skim back to default while a highlight is active", () => {
    expect(resolveMessageLayout("skim", true)).toBe("default");
  });

  it("leaves every other layout untouched in both search states", () => {
    for (const layout of NON_SKIM_LAYOUTS) {
      expect(resolveMessageLayout(layout, false)).toBe(layout);
      expect(resolveMessageLayout(layout, true)).toBe(layout);
    }
  });

  it("is pure: repeated calls do not drift", () => {
    expect(resolveMessageLayout("skim", true)).toBe("default");
    expect(resolveMessageLayout("skim", false)).toBe("skim");
    expect(resolveMessageLayout("skim", true)).toBe("default");
    expect(resolveMessageLayout("skim", false)).toBe("skim");
  });
});
