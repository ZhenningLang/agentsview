import { test, expect, type Locator, type Page } from "@playwright/test";
import { SessionsPage } from "./pages/sessions-page";

// Phase 19 browser acceptance for the two upstream ports:
//   de6eeaf6 -- skim transcript layout (and its search guard)
//   e65fe7a3 -- UI text-size scaling and high-contrast mode
//
// These behaviors are invisible to jsdom: the skim rules live in a component
// <style> block, the root zoom is a computed inline value, and the contrast
// claims are only meaningful against real computed colors.

/** The fixture session seeded by cmd/testfixture with structured tool calls. */
const SHOWCASE = "test-session-duration-showcase";
const SCROLL = ".message-list-scroll";

/** A token that exists only inside tool result content, never in message text. */
const HIDDEN_TOKEN = "zqhiddenoutput";

function readZoom(page: Page): Promise<string> {
  return page.evaluate(() =>
    document.documentElement.style.getPropertyValue("zoom"),
  );
}

function luminance([r, g, b]: [number, number, number]): number {
  const [rr, gg, bb] = [r, g, b].map((channel) => {
    const value = channel / 255;
    return value <= 0.03928
      ? value / 12.92
      : Math.pow((value + 0.055) / 1.055, 2.4);
  });
  return 0.2126 * rr! + 0.7152 * gg! + 0.0722 * bb!;
}

function contrastRatio(
  foreground: [number, number, number],
  background: [number, number, number],
): number {
  const lighter = Math.max(luminance(foreground), luminance(background));
  const darker = Math.min(luminance(foreground), luminance(background));
  return (lighter + 0.05) / (darker + 0.05);
}

function parseRgb(value: string): [number, number, number] {
  const rgb = value.match(/rgba?\((\d+),\s*(\d+),\s*(\d+)/);
  if (rgb) {
    return [Number(rgb[1]), Number(rgb[2]), Number(rgb[3])];
  }
  // Chromium serializes a resolved color-mix() as `color(srgb r g b)` with
  // 0..1 components rather than as rgb().
  const srgb = value.match(
    /color\(srgb\s+([0-9.]+)\s+([0-9.]+)\s+([0-9.]+)/,
  );
  if (srgb) {
    return [
      Math.round(Number(srgb[1]) * 255),
      Math.round(Number(srgb[2]) * 255),
      Math.round(Number(srgb[3]) * 255),
    ];
  }
  throw new Error(`Expected an rgb() or color(srgb) color, got ${value}`);
}

async function elementColors(locator: Locator): Promise<{
  background: string;
  foreground: string;
}> {
  return locator.evaluate((element) => {
    const styles = getComputedStyle(element);
    return {
      background: styles.backgroundColor,
      foreground: styles.color,
    };
  });
}

function expectReadableContrast(
  colors: { background: string; foreground: string },
  label: string,
) {
  const ratio = contrastRatio(
    parseRgb(colors.foreground),
    parseRgb(colors.background),
  );
  expect(ratio, `${label} contrast (${colors.foreground} on ${colors.background})`)
    .toBeGreaterThanOrEqual(4.5);
}

/**
 * Foreground of an element paired with the first opaque background painted
 * behind it. Transcript text sits on transparent rows, so reading
 * `backgroundColor` off the element itself would compare against rgba(0,0,0,0)
 * and make any color look readable.
 */
async function elementColorsOnSurface(locator: Locator): Promise<{
  background: string;
  foreground: string;
}> {
  return locator.evaluate((element) => {
    const foreground = getComputedStyle(element).color;
    let node: Element | null = element;
    while (node) {
      const background = getComputedStyle(node).backgroundColor;
      const alpha = background.match(/rgba?\([^)]*,\s*([0-9.]+)\s*\)/);
      if (background && (!alpha || Number(alpha[1]) > 0.9)) {
        return { background, foreground };
      }
      node = node.parentElement;
    }
    return { background: "rgb(255, 255, 255)", foreground };
  });
}

/** Computed display of the first matching element, or null when absent. */
function computedDisplay(page: Page, selector: string): Promise<string | null> {
  return page.evaluate((sel) => {
    const el = document.querySelector(sel);
    return el ? getComputedStyle(el).display : null;
  }, selector);
}

async function openShowcase(page: Page) {
  await page.goto(`/sessions/${SHOWCASE}`);
  await expect(page.locator(SCROLL)).toBeVisible({ timeout: 10_000 });
  await expect(page.locator(".virtual-row").first()).toBeVisible({
    timeout: 10_000,
  });
}

/**
 * Give the first tool call some result content carrying HIDDEN_TOKEN, and
 * make the in-session search endpoint report that message as the only match.
 * The fixture ships result lengths but no bodies, and the token has to live
 * somewhere skim actually hides -- the tool output -- for the search guard to
 * be observable. Rewriting the real responses keeps envelope and ordinals
 * exactly as the server produces them.
 */
async function injectHiddenToolOutput(page: Page) {
  let hostOrdinal: number | null = null;

  await page.route("**/api/v1/sessions/*/messages*", async (route) => {
    const response = await route.fetch();
    const body = await response.json();
    for (const message of body.messages ?? []) {
      const calls = message.tool_calls ?? [];
      if (calls.length > 0) {
        calls[0].result_content = `search fixture ${HIDDEN_TOKEN} line\nsecond line`;
        calls[0].result_content_length = calls[0].result_content.length;
        hostOrdinal = message.ordinal;
        break;
      }
    }
    const headers = { ...response.headers() };
    delete headers["content-length"];
    headers["content-type"] = "application/json";
    await route.fulfill({
      status: response.status(),
      headers,
      body: JSON.stringify(body),
    });
  });

  await page.route("**/api/v1/sessions/*/search*", async (route) => {
    const url = new URL(route.request().url());
    const query = url.searchParams.get("q") ?? "";
    const hit = query.length > 0 && HIDDEN_TOKEN.startsWith(query.toLowerCase());
    await route.fulfill({
      status: 200,
      headers: { "content-type": "application/json" },
      body: JSON.stringify({
        ordinals: hit && hostOrdinal !== null ? [hostOrdinal] : [],
      }),
    });
  });
}

test.describe("Phase 19 skim layout", () => {
  test.beforeEach(async ({ page }) => {
    await page.addInitScript(() => {
      localStorage.setItem("agentsview-message-layout", "skim");
      localStorage.setItem("theme", "light");
    });
  });

  test("collapses the transcript to one-line tool summaries", async ({
    page,
  }) => {
    await openShowcase(page);

    await expect(page.locator(SCROLL)).toHaveClass(/layout-skim/);

    // The structured one-liner survives; the surrounding chrome does not.
    const previews = page.locator(".tool-header .tool-preview");
    await expect(previews.first()).toBeVisible();
    expect(await previews.count()).toBeGreaterThan(1);
    await expect(
      page.locator(".tool-header .tool-preview", {
        hasText: "$ go test ./... -count=10",
      }),
    ).toBeVisible();

    expect(await computedDisplay(page, ".message-header")).toBe("none");
    expect(await computedDisplay(page, ".pg-header")).toBe("none");
    expect(await computedDisplay(page, ".subagent-inline")).toBe("none");
    expect(await computedDisplay(page, ".tool-chevron")).toBe("none");
    // Collapsed tool bodies are not rendered at all in this layout.
    expect(await page.locator(".tool-content").count()).toBe(0);
    expect(await page.locator(".output-header").count()).toBe(0);
  });

  test("hides the copy affordance so the row edge stays selectable", async ({
    page,
  }) => {
    await openShowcase(page);

    // The button is rendered -- skim has to hide it, not depend on it being
    // absent -- and it sits beside `.tool-header`, so the header's
    // `pointer-events: none` never covered it.
    const copyButtons = page.locator(".tool-copy");
    expect(await copyButtons.count()).toBeGreaterThan(0);
    expect(await computedDisplay(page, ".tool-copy")).toBe("none");

    // Click where the copy button would have been: the right edge of the row.
    const row = page.locator(".virtual-row").nth(1);
    const box = await row.boundingBox();
    expect(box).not.toBeNull();
    await page.mouse.click(
      box!.x + box!.width - 12,
      box!.y + box!.height / 2,
    );

    await expect(page.locator(".virtual-row.selected")).toHaveCount(1);
    expect(await page.locator(".tool-content").count()).toBe(0);
  });

  test("keeps tool headers inert while row selection still works", async ({
    page,
  }) => {
    await openShowcase(page);

    const pointerEvents = await page.evaluate(() => {
      const el = document.querySelector(".tool-header");
      return el ? getComputedStyle(el).pointerEvents : null;
    });
    expect(pointerEvents).toBe("none");

    const row = page.locator(".virtual-row").nth(1);
    await row.click();
    await expect(page.locator(".virtual-row.selected")).toHaveCount(1);
    // A click that reached the header would have expanded the block.
    expect(await page.locator(".tool-content").count()).toBe(0);
  });

  test("shows the full layout while searching, then restores skim", async ({
    page,
  }) => {
    await injectHiddenToolOutput(page);
    await openShowcase(page);
    await expect(page.locator(SCROLL)).toHaveClass(/layout-skim/);

    await page.locator(".virtual-row").first().click();
    await page.keyboard.press("/");
    const findInput = page.locator(".find-bar .find-input");
    await expect(findInput).toBeVisible();

    await findInput.fill(HIDDEN_TOKEN);

    // Skim would hide the auto-expanded match, so the layout falls back.
    await expect(page.locator(SCROLL)).toHaveClass(/layout-default/);
    await expect(page.locator(SCROLL)).not.toHaveClass(/layout-skim/);
    const highlight = page.locator(".tool-content mark.search-highlight");
    await expect(highlight.first()).toBeVisible();

    // The stored preference is untouched -- only the applied class moved.
    expect(
      await page.evaluate(() =>
        localStorage.getItem("agentsview-message-layout"),
      ),
    ).toBe("skim");

    await page.keyboard.press("Escape");
    await expect(findInput).toBeHidden();
    await expect(page.locator(SCROLL)).toHaveClass(/layout-skim/);
    expect(
      await page.evaluate(() =>
        localStorage.getItem("agentsview-message-layout"),
      ),
    ).toBe("skim");
  });
});

test.describe("Phase 19 text size", () => {
  test("scales the UI to 130% without page-level horizontal overflow", async ({
    page,
  }) => {
    await page.addInitScript(() => {
      localStorage.setItem("agentsview-font-scale", "130");
    });
    const sp = new SessionsPage(page);
    await sp.goto();

    expect(await readZoom(page)).toBe("1.3");

    const desktopOverflow = await page.evaluate(
      () =>
        document.documentElement.scrollWidth -
        document.documentElement.clientWidth,
    );
    expect(desktopOverflow).toBeLessThanOrEqual(2);

    await sp.selectFirstSession();
    await expect(sp.messageRows.first()).toBeVisible();

    await page.setViewportSize({ width: 400, height: 800 });
    const mobileOverflow = await page.evaluate(
      () =>
        document.documentElement.scrollWidth -
        document.documentElement.clientWidth,
    );
    expect(mobileOverflow).toBeLessThanOrEqual(2);
  });

  test("scales the UI to 90% and keeps the transcript usable", async ({
    page,
  }) => {
    await page.addInitScript(() => {
      localStorage.setItem("agentsview-font-scale", "90");
    });
    const sp = new SessionsPage(page);
    await sp.goto();

    expect(await readZoom(page)).toBe("0.9");

    await sp.selectFirstSession();
    await expect(sp.messageRows.first()).toBeVisible();
    await sp.scroller.evaluate((el) => {
      el.scrollTop = 200;
    });
    expect(await sp.scroller.evaluate((el) => el.scrollTop)).toBeGreaterThan(0);
  });

  test("applies a new text size immediately and persists it", async ({
    page,
  }) => {
    const sp = new SessionsPage(page);
    await sp.goto();
    expect(await readZoom(page)).toBe("1");

    await page.locator('button[aria-label="Settings"]').first().click();
    await page.getByRole("button", { name: "120%" }).click();

    // No reload: the effect must write the root zoom straight away.
    expect(await readZoom(page)).toBe("1.2");
    expect(
      await page.evaluate(() => localStorage.getItem("agentsview-font-scale")),
    ).toBe("120");
  });

  test("multiplies the desktop window zoom with the text size", async ({
    page,
  }) => {
    await page.addInitScript(() => {
      localStorage.setItem("agentsview-zoom-level", "150");
      localStorage.setItem("agentsview-font-scale", "120");
    });
    await page.goto("/?desktop");
    const sp = new SessionsPage(page);
    await expect(sp.sessionItems.first()).toBeVisible({ timeout: 10_000 });

    // 1.5 * 1.2; a sum would read 2.7 and a single writer winning would read
    // either 1.5 or 1.2.
    expect(await readZoom(page)).toBe("1.8");
  });
});

test.describe("Phase 19 high contrast", () => {
  test("applies the light root class and overrides the neutral tokens", async ({
    page,
  }) => {
    await page.addInitScript(() => {
      localStorage.setItem("theme", "light");
      localStorage.setItem("agentsview-high-contrast", "true");
    });
    const sp = new SessionsPage(page);
    await sp.goto();

    const classes = await page.evaluate(() => ({
      contrast: document.documentElement.classList.contains("high-contrast"),
      dark: document.documentElement.classList.contains("dark"),
    }));
    expect(classes).toEqual({ contrast: true, dark: false });

    const tokens = await page.evaluate((): Record<string, string> => {
      const expand = (raw: string): string =>
        /^#[0-9a-fA-F]{3}$/.test(raw)
          ? "#" + raw[1]!.repeat(2) + raw[2]!.repeat(2) + raw[3]!.repeat(2)
          : raw;
      const read = (name: string) =>
        expand(
          getComputedStyle(document.documentElement)
            .getPropertyValue(name)
            .trim(),
        );
      return {
        textPrimary: read("--text-primary"),
        textMuted: read("--text-muted"),
        accentBlue: read("--accent-blue"),
      };
    });
    expect(tokens.textPrimary).toBe("#000000");
    expect(tokens.textMuted).toBe("#44495a");
    expect(tokens.accentBlue).toBe("#0a47c2");
  });

  test("composes with the dark theme rather than replacing it", async ({
    page,
  }) => {
    await page.addInitScript(() => {
      localStorage.setItem("theme", "dark");
      localStorage.setItem("agentsview-high-contrast", "true");
    });
    const sp = new SessionsPage(page);
    await sp.goto();

    const classes = await page.evaluate(() => ({
      contrast: document.documentElement.classList.contains("high-contrast"),
      dark: document.documentElement.classList.contains("dark"),
    }));
    expect(classes).toEqual({ contrast: true, dark: true });

    const accentBlue = await page.evaluate(() =>
      getComputedStyle(document.documentElement)
        .getPropertyValue("--accent-blue")
        .trim(),
    );
    expect(accentBlue).toBe("#8ab8ff");
  });

  test("keeps accent-filled controls readable in dark high contrast", async ({
    page,
  }) => {
    await page.addInitScript(() => {
      localStorage.setItem("theme", "dark");
      localStorage.setItem("agentsview-high-contrast", "true");
    });
    await openShowcase(page);

    const primaryButtonColors = await page.evaluate(() => {
      const panel = document.createElement("div");
      panel.className = "modal-panel";
      panel.innerHTML =
        '<button class="modal-btn modal-btn-primary">Save</button>';
      document.body.append(panel);
      const button = panel.querySelector("button")!;
      const styles = getComputedStyle(button);
      const result = {
        background: styles.backgroundColor,
        foreground: styles.color,
      };
      panel.remove();
      return result;
    });
    expectReadableContrast(primaryButtonColors, "modal primary button");

    const agentBadge = page.locator(".agent-badge").first();
    await expect(agentBadge).toBeVisible();
    expectReadableContrast(
      await elementColors(agentBadge),
      "session agent badge",
    );

    // A non-blue agent fill exercises a different token pair than the shell.
    const nonBlueBadge = await page.evaluate(() => {
      const badge = document.createElement("span");
      badge.className = "agent-badge";
      badge.style.background = "var(--accent-green)";
      badge.style.color = "var(--accent-green-foreground)";
      badge.textContent = "Codex";
      document.body.append(badge);
      const styles = getComputedStyle(badge);
      const result = {
        background: styles.backgroundColor,
        foreground: styles.color,
      };
      badge.remove();
      return result;
    });
    expectReadableContrast(nonBlueBadge, "non-blue agent badge");

    const userIcon = page.locator(".role-icon", { hasText: "U" }).first();
    await expect(userIcon).toBeVisible();
    expectReadableContrast(await elementColors(userIcon), "user role icon");

    const assistantIcon = page.locator(".role-icon", { hasText: "A" }).first();
    await expect(assistantIcon).toBeVisible();
    expectReadableContrast(
      await elementColors(assistantIcon),
      "assistant role icon",
    );
  });

  test("keeps Insights status badges readable in dark high contrast", async ({
    page,
  }) => {
    await page.addInitScript(() => {
      localStorage.setItem("theme", "dark");
      localStorage.setItem("agentsview-high-contrast", "true");
    });
    // Two rows so both badge variants render: the fill lives on
    // .badge-blue/.badge-purple while the base .header-badge carries the text.
    await page.route("**/api/v1/insights*", async (route) => {
      await route.fulfill({
        status: 200,
        headers: { "content-type": "application/json" },
        body: JSON.stringify({
          insights: [
            {
              id: 1,
              type: "daily_activity",
              date_from: "2026-08-17",
              date_to: "2026-08-17",
              project: null,
              agent: "claude",
              model: null,
              prompt: null,
              content: "# Daily\n\nnothing to see",
              created_at: "2026-08-17T09:00:00Z",
            },
            {
              id: 2,
              type: "agent_analysis",
              date_from: "2026-08-16",
              date_to: "2026-08-17",
              project: null,
              agent: "claude",
              model: null,
              prompt: null,
              content: "# Analysis\n\nnothing to see",
              created_at: "2026-08-17T10:00:00Z",
            },
          ],
        }),
      });
    });

    await page.goto("/insights");
    const rows = page.locator(".insight-row");
    await expect(rows.first()).toBeVisible({ timeout: 10_000 });

    const badge = page.locator(".header-badge");
    for (const [index, label] of [
      [0, "daily activity badge"],
      [1, "agent analysis badge"],
    ] as const) {
      await rows.nth(index).click();
      await expect(badge).toBeVisible();
      const colors = await elementColors(badge);
      // A base-class white would read as rgb(255, 255, 255) here whatever the
      // variant fill resolved to.
      expect(colors.foreground, label).not.toBe("rgb(255, 255, 255)");
      expectReadableContrast(colors, label);
    }
  });

  test("keeps a color-mix accent hover readable in dark high contrast", async ({
    page,
  }) => {
    await page.addInitScript(() => {
      localStorage.setItem("theme", "dark");
      localStorage.setItem("agentsview-high-contrast", "true");
    });
    const sp = new SessionsPage(page);
    await sp.goto();

    // The engine resolves the same mix MemoryPage's primary hover paints, so
    // this measures the actual blended color rather than trusting arithmetic.
    const hover = await page.evaluate(() => {
      const probe = document.createElement("div");
      probe.style.background =
        "color-mix(in srgb, var(--accent-blue) 82%, black)";
      probe.style.color = "var(--accent-blue-foreground)";
      probe.textContent = "Save";
      document.body.append(probe);
      const styles = getComputedStyle(probe);
      const result = {
        background: styles.backgroundColor,
        foreground: styles.color,
      };
      probe.remove();
      return result;
    });
    expect(hover.foreground).not.toBe("rgb(255, 255, 255)");
    expectReadableContrast(hover, "darkened accent hover fill");
  });

  test("lifts the transcript call durations out of their literal grey", async ({
    page,
  }) => {
    await page.addInitScript(() => {
      localStorage.setItem("theme", "dark");
      localStorage.setItem("agentsview-high-contrast", "true");
      localStorage.setItem("agentsview-session-vitals", "true");
    });
    await openShowcase(page);

    const duration = page.locator(".call .cd").first();
    await expect(duration).toBeVisible({ timeout: 10_000 });
    expectReadableContrast(
      await elementColorsOnSurface(duration),
      "call duration",
    );

    // Every rendered `.cd` also carries one of slow/live/muted, and only
    // `.muted` used to be lifted. Cloning a real node and dropping the state
    // classes keeps the Svelte scope hash while exposing the base `.cd` rule,
    // which is where the literal #999 still lived.
    const bare = await duration.evaluate((element) => {
      const clone = element.cloneNode(true) as HTMLElement;
      clone.classList.remove("muted", "slow", "live");
      element.parentElement!.append(clone);
      const color = getComputedStyle(clone).color;
      clone.remove();
      return color;
    });
    expect(bare).not.toBe("rgb(153, 153, 153)");

    const axis = page.locator(".scale-axis").first();
    await expect(axis).toBeVisible();
    const axisColors = await elementColorsOnSurface(axis);
    expect(axisColors.foreground).not.toBe("rgb(102, 102, 102)");
    expectReadableContrast(axisColors, "call scale axis");
  });

  test("strengthens the keyboard focus ring", async ({ page }) => {
    const sp = new SessionsPage(page);
    await sp.goto();

    const baseline = await page.evaluate(() => {
      const probe = document.createElement("button");
      probe.textContent = "focus probe";
      document.body.append(probe);
      probe.focus({ focusVisible: true } as FocusOptions);
      const styles = getComputedStyle(probe);
      const result = {
        width: styles.outlineWidth,
        offset: styles.outlineOffset,
      };
      probe.remove();
      return result;
    });
    expect(baseline).toEqual({ width: "2px", offset: "1px" });

    const strengthened = await page.evaluate(() => {
      document.documentElement.classList.add("high-contrast");
      const probe = document.createElement("button");
      probe.textContent = "focus probe";
      document.body.append(probe);
      probe.focus({ focusVisible: true } as FocusOptions);
      const styles = getComputedStyle(probe);
      const result = {
        width: styles.outlineWidth,
        offset: styles.outlineOffset,
      };
      probe.remove();
      document.documentElement.classList.remove("high-contrast");
      return result;
    });
    expect(strengthened).toEqual({ width: "3px", offset: "2px" });
  });

  test("survives a reload and clears cleanly when turned off", async ({
    page,
  }) => {
    const sp = new SessionsPage(page);
    await sp.goto();

    await page.locator('button[aria-label="Settings"]').first().click();
    const toggle = page.getByRole("button", { name: "High contrast" });
    await toggle.click();

    await expect
      .poll(() =>
        page.evaluate(() =>
          document.documentElement.classList.contains("high-contrast"),
        ),
      )
      .toBe(true);
    expect(
      await page.evaluate(() =>
        localStorage.getItem("agentsview-high-contrast"),
      ),
    ).toBe("true");

    // Reloading stays on the settings route, so wait for the control itself
    // rather than for the session list.
    await page.reload();
    await expect(
      page.getByRole("button", { name: "High contrast" }),
    ).toBeVisible({ timeout: 10_000 });
    expect(
      await page.evaluate(() =>
        document.documentElement.classList.contains("high-contrast"),
      ),
    ).toBe(true);

    await page.getByRole("button", { name: "High contrast" }).click();

    await expect
      .poll(() =>
        page.evaluate(() =>
          document.documentElement.classList.contains("high-contrast"),
        ),
      )
      .toBe(false);
    expect(
      await page.evaluate(() =>
        localStorage.getItem("agentsview-high-contrast"),
      ),
    ).toBe("false");
  });
});
