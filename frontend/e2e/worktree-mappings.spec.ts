import { expect, test } from "@playwright/test";

test.describe("Worktree mappings settings", () => {
  test.beforeEach(async ({ page }) => {
    await page.route(/\/api\/v1\/.*/, async (route) => {
      const url = new URL(route.request().url());
      const path = url.pathname;

      if (path === "/api/v1/settings") {
        await route.fulfill({
          json: {
            agent_dirs: {},
            github_configured: false,
            host: "127.0.0.1",
            port: 8090,
            require_auth: false,
            terminal: { mode: "auto" },
          },
        });
        return;
      }

      if (path === "/api/v1/config/terminal") {
        await route.fulfill({ json: { mode: "auto" } });
        return;
      }

      if (path === "/api/v1/settings/worktree-mappings") {
        if (route.request().method() === "POST") {
          await route.fulfill({
            json: {
              id: 2,
              machine: "test-machine",
              ...(route.request().postDataJSON() as Record<string, unknown>),
              created_at: "2026-08-14T00:00:00.000Z",
              updated_at: "2026-08-14T00:00:00.000Z",
            },
          });
          return;
        }
        await route.fulfill({
          json: {
            machine: "test-machine",
            mappings: [
              {
                id: 1,
                machine: "test-machine",
                path_prefix: "/tmp/projects",
                layout: "repo_dot_worktrees",
                project: "",
                enabled: true,
                created_at: "2026-08-14T00:00:00.000Z",
                updated_at: "2026-08-14T00:00:00.000Z",
              },
            ],
          },
        });
        return;
      }

      await route.fallback();
    });
  });

  test("repo.worktrees form is usable without horizontal overflow on mobile", async ({
    page,
  }) => {
    const diagnostics: string[] = [];
    page.on("console", (msg) => {
      if (msg.type() === "error") diagnostics.push(`console: ${msg.text()}`);
    });
    page.on("pageerror", (error) => {
      diagnostics.push(`pageerror: ${error.message}`);
    });
    page.on("requestfailed", (request) => {
      diagnostics.push(
        `requestfailed: ${request.url()} ${request.failure()?.errorText}`,
      );
    });
    page.on("response", (response) => {
      if (response.status() >= 400) {
        diagnostics.push(`response: ${response.status()} ${response.url()}`);
      }
    });

    await page.setViewportSize({ width: 400, height: 844 });
    await page.goto("/settings");

    const section = page.locator("section", { hasText: "Worktree mappings" });
    await expect(
      section,
      [
        diagnostics.join("\n"),
        await page.locator("body").evaluate((body) => body.innerHTML),
      ]
        .filter(Boolean)
        .join("\n"),
    ).toBeVisible();
    await expect(section.locator(".mapping-project")).toHaveText(
      "{repo}.worktrees/{branch}",
    );

    await section.getByLabel("Layout").selectOption("repo_dot_worktrees");
    await expect(section.getByLabel("Parent directory")).toBeVisible();
    await expect(section.getByLabel("Project")).toBeDisabled();

    const controlTops = await section
      .locator(".form-grid > label")
      .evaluateAll((labels) =>
        labels.map((label) => label.getBoundingClientRect().top),
      );
    expect(controlTops.length).toBeGreaterThanOrEqual(3);
    expect(new Set(controlTops.slice(0, 3)).size).toBe(3);

    const saveRequest = page.waitForRequest(
      (request) =>
        request.method() === "POST" &&
        new URL(request.url()).pathname ===
          "/api/v1/settings/worktree-mappings",
    );
    await section.getByLabel("Parent directory").fill("/tmp/projects");
    await section.getByRole("button", { name: "Add mapping" }).click();
    expect((await saveRequest).postDataJSON()).toMatchObject({
      path_prefix: "/tmp/projects",
      layout: "repo_dot_worktrees",
      project: "",
      enabled: true,
    });

    const noOverflow = await page.evaluate(
      () =>
        document.documentElement.scrollWidth <=
        document.documentElement.clientWidth,
    );
    expect(noOverflow).toBe(true);
  });
});
