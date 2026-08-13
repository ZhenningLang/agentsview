import { test, expect } from "@playwright/test";
import { SessionsPage } from "./pages/sessions-page";

test.describe("Navigation", () => {
  let sp: SessionsPage;

  test.beforeEach(async ({ page }) => {
    sp = new SessionsPage(page);
    await sp.goto();
  });

  test("keyboard ] navigates to next session", async () => {
    await sp.sessionItems.first().click();
    await expect(sp.sessionItems.first()).toHaveClass(/active/);

    await sp.pressNextSessionShortcut();
    await expect(sp.sessionItems.nth(1)).toHaveClass(/active/);
  });

  test("keyboard [ navigates to previous session", async () => {
    await sp.sessionItems.nth(1).click();
    await expect(sp.sessionItems.nth(1)).toHaveClass(/active/);

    await sp.pressPreviousSessionShortcut();
    await expect(sp.sessionItems.first()).toHaveClass(/active/);
  });

  test("analytics page shows when no session selected", async () => {
    await expect(sp.analyticsPage).toBeVisible();
    await expect(sp.analyticsToolbar).toBeVisible();
    await expect(sp.exportBtn).toContainText("Export CSV");
  });

  test("Shift+J and Shift+K navigate visible user prompts", async ({
    page,
  }) => {
    // mixed-content-7 interleaves user, assistant and tool rows, so plain j
    // and Shift+J land on different rows.
    await page.goto("/sessions/test-session-mixed-content-7");
    await expect(sp.messageRows.first()).toBeVisible({ timeout: 5_000 });

    const users = sp.messageRows.filter({
      has: page.locator(".message.is-user"),
    });
    const others = sp.messageRows.filter({
      has: page.locator(".message:not(.is-user)"),
    });
    await expect(users).not.toHaveCount(0);

    // Plain j moves one display row: from the first user prompt it lands on a
    // non-user row.
    await users.first().click();
    await expect(users.first()).toHaveClass(/selected/);
    await page.keyboard.press("j");
    await expect(others.first()).toHaveClass(/selected/);

    // Shift+J from a non-user row skips straight to the next user prompt.
    await page.keyboard.press("Shift+J");
    await expect(users.nth(1)).toHaveClass(/selected/);
    await page.keyboard.press("Shift+K");
    await expect(users.first()).toHaveClass(/selected/);

    // Newest-first inverts the chronological step so the visual direction of
    // Shift+J stays "down the rendered list".
    await sp.toggleSortOrder();
    await users.first().click();
    await expect(users.first()).toHaveClass(/selected/);
    await page.keyboard.press("Shift+J");
    await expect(users.nth(1)).toHaveClass(/selected/);
    await page.keyboard.press("Shift+K");
    await expect(users.first()).toHaveClass(/selected/);
    await sp.toggleSortOrder();
  });

  test("shortcuts modal documents prompt navigation without overflowing", async ({
    page,
  }) => {
    const rowFor = (key: string) =>
      page.locator(".shortcuts-modal .shortcut-row").filter({
        has: page.locator(".shortcut-key", { hasText: key }),
      });

    // Default locale is zh.
    await page.keyboard.press("?");
    await expect(page.locator(".shortcuts-modal")).toBeVisible();
    await expect(rowFor("Shift+J")).toContainText("下一条用户提问");
    await expect(rowFor("Shift+K")).toContainText("上一条用户提问");

    await page.addInitScript(() => {
      localStorage.setItem("agentsview.locale", "en");
    });
    await page.reload();
    await expect(sp.sessionItems.first()).toBeVisible({ timeout: 5_000 });
    await page.keyboard.press("?");
    const modal = page.locator(".shortcuts-modal");
    await expect(modal).toBeVisible();
    await expect(rowFor("Shift+J")).toContainText("Next user prompt");
    await expect(rowFor("Shift+K")).toContainText("Previous user prompt");

    for (const width of [1280, 768, 400]) {
      await page.setViewportSize({ width, height: 800 });
      const box = await modal.boundingBox();
      expect(box).not.toBeNull();
      expect(box!.x).toBeGreaterThanOrEqual(0);
      expect(box!.x + box!.width).toBeLessThanOrEqual(width);
    }
  });
});
