// @vitest-environment jsdom
// ABOUTME: Phase 19 tests for the Appearance settings accessibility controls.
// ABOUTME: Covers the skim layout option, text-size steps, and high contrast.
import { cleanup, fireEvent, render } from "@testing-library/svelte";
import { afterEach, describe, expect, it } from "vitest";

import { FONT_SCALE_STEPS, ui } from "../../stores/ui.svelte.js";
// @ts-ignore -- svelte component import without generated types
import AppearanceSettings from "./AppearanceSettings.svelte";

describe("Phase 19 AppearanceSettings", () => {
  afterEach(() => {
    // Leave no global state behind for other suites in this process.
    ui.setFontScale(100);
    if (ui.highContrast) ui.toggleHighContrast();
    ui.setLayout("default");
    document.documentElement.classList.remove("high-contrast");
    cleanup();
  });

  it("offers the skim message layout and switches the store", async () => {
    const { getByRole } = render(AppearanceSettings);

    const skim = getByRole("button", { name: "Skim" });
    expect(skim.classList.contains("active")).toBe(false);

    await fireEvent.click(skim);

    expect(ui.messageLayout).toBe("skim");
    expect(
      getByRole("button", { name: "Skim" }).classList.contains("active"),
    ).toBe(true);
  });

  it("renders every text-size step and marks the active one", () => {
    ui.setFontScale(110);
    const { getByRole } = render(AppearanceSettings);

    for (const step of FONT_SCALE_STEPS) {
      const button = getByRole("button", { name: `${step}%` });
      expect(button.classList.contains("active")).toBe(step === 110);
      expect(button.getAttribute("aria-pressed")).toBe(
        String(step === 110),
      );
    }
  });

  it("changes the font scale when a text-size option is clicked", async () => {
    const { getByRole } = render(AppearanceSettings);

    await fireEvent.click(getByRole("button", { name: "120%" }));

    expect(ui.fontScale).toBe(120);
    expect(
      getByRole("button", { name: "120%" }).getAttribute("aria-pressed"),
    ).toBe("true");
  });

  it("exposes high contrast as a named pressed-state toggle", async () => {
    const { getByRole } = render(AppearanceSettings);

    const toggle = getByRole("button", { name: "High contrast" });
    expect(toggle.getAttribute("aria-pressed")).toBe("false");
    expect(toggle.textContent?.trim()).toBe("Off");

    await fireEvent.click(toggle);

    expect(ui.highContrast).toBe(true);
    expect(
      getByRole("button", { name: "High contrast" }).getAttribute(
        "aria-pressed",
      ),
    ).toBe("true");
    expect(
      getByRole("button", { name: "High contrast" }).textContent?.trim(),
    ).toBe("On");
  });

  it("reflects an already-enabled high contrast state on render", () => {
    ui.toggleHighContrast();
    const { getByRole } = render(AppearanceSettings);

    expect(
      getByRole("button", { name: "High contrast" }).getAttribute(
        "aria-pressed",
      ),
    ).toBe("true");
  });

  it("keeps the high-contrast toggle distinct from the LLM titles toggle", async () => {
    const { getByRole } = render(AppearanceSettings);

    await fireEvent.click(getByRole("button", { name: "High contrast" }));

    expect(ui.highContrast).toBe(true);
    // A shared "Off" label would have flipped the LLM titles preference too.
    expect(ui.useLlmTitle).toBe(false);
  });
});
