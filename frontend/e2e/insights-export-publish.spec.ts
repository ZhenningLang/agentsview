import { expect, test, type BrowserContext, type Page } from "@playwright/test";

/**
 * Browser acceptance for insight HTML export and Gist publish.
 *
 * Every API call is answered locally: the point of this spec is that the app
 * is actually wired end to end, not that GitHub behaves. Payload shape and
 * error mapping belong to the Go httptest.Server tests. A request that
 * escaped to a real host would be both a leak and a wrong oracle, so the spec
 * records every request the page makes and asserts none of them reached
 * github.com.
 */

interface InsightFixture {
  id: number;
  type: string;
  date_from: string;
  date_to: string;
  project: string | null;
  agent: string;
  model: string | null;
  prompt: string | null;
  content: string;
  created_at: string;
}

const DAILY: InsightFixture = {
  id: 501,
  type: "daily_activity",
  date_from: "2026-08-18",
  date_to: "2026-08-18",
  project: "agentsview",
  agent: "claude",
  model: "claude-opus-5",
  prompt: null,
  content: "# Daily insight\n\nFixture body for the daily insight.",
  created_at: "2026-08-18T09:00:00Z",
};

const RANGE: InsightFixture = {
  id: 502,
  type: "daily_activity",
  date_from: "2026-08-10",
  date_to: "2026-08-16",
  project: null,
  agent: "codex",
  model: null,
  prompt: null,
  content: "# Range insight\n\nFixture body for the range insight.",
  created_at: "2026-08-17T09:00:00Z",
};

interface Recorder {
  /** Every request the page issued, as method + path + search. */
  requests: string[];
  /** Anything that tried to leave for GitHub, which must stay empty. */
  githubRequests: string[];
  /** Uncaught page errors, kept out of the request log. */
  pageErrors: string[];
}

const EXPORT_HTML =
  "<!doctype html><html><body><h1>Daily insight</h1></body></html>";

async function installMocks(
  context: BrowserContext,
  page: Page,
): Promise<Recorder> {
  const recorder: Recorder = {
    requests: [],
    githubRequests: [],
    pageErrors: [],
  };

  context.on("request", (request) => {
    if (/github\.com/i.test(request.url())) {
      recorder.githubRequests.push(request.url());
    }
  });

  // Routed on the context, not the page, so the new tab that a local HTML
  // export opens is intercepted too.
  await context.route(/\/api\/v1\/.*/, async (route) => {
    const request = route.request();
    const url = new URL(request.url());
    const path = url.pathname;
    recorder.requests.push(
      `${request.method()} ${path}${url.search}`,
    );

    if (path === "/api/v1/insights") {
      await route.fulfill({ json: { insights: [DAILY, RANGE] } });
      return;
    }

    if (path === "/api/v1/projects") {
      await route.fulfill({ json: { projects: [] } });
      return;
    }

    if (path === "/api/v1/config/github") {
      await route.fulfill({ json: { configured: true } });
      return;
    }

    const exportMatch = path.match(
      /^\/api\/v1\/insights\/(\d+)\/export$/,
    );
    if (exportMatch) {
      await route.fulfill({
        status: 200,
        contentType: "text/html; charset=utf-8",
        headers: {
          "content-disposition":
            `attachment; filename="insight-${exportMatch[1]}.html"`,
        },
        body: EXPORT_HTML,
      });
      return;
    }

    const publishMatch = path.match(
      /^\/api\/v1\/insights\/(\d+)\/publish$/,
    );
    if (publishMatch) {
      const id = publishMatch[1];
      const secret = url.searchParams.get("secret") === "true";
      const slug = `${secret ? "secret" : "public"}-${id}`;
      await route.fulfill({
        json: {
          gist_id: slug,
          gist_url: `https://gist.example.test/${slug}`,
          raw_url: `https://gist.example.test/${slug}/raw`,
          view_url: `https://viewer.example.test/${slug}`,
        },
      });
      return;
    }

    await route.fallback();
  });

  // Nothing should reach GitHub, but if the wiring regresses, fail on a local
  // abort rather than letting the test machine talk to the real API.
  await context.route(/https?:\/\/([a-z.]*\.)?github\.com\/.*/i, (route) =>
    route.abort(),
  );

  page.on("pageerror", (error) => {
    recorder.pageErrors.push(error.message);
  });

  return recorder;
}

function exportButton(page: Page) {
  return page.getByRole("button", { name: "Export insight as HTML" });
}

function publicPublishButton(page: Page) {
  return page.getByRole("button", {
    name: "Publish insight as public Gist",
  });
}

function secretPublishButton(page: Page) {
  return page.getByRole("button", {
    name: "Publish insight as secret Gist",
  });
}

function viewUrlInput(page: Page) {
  return page.locator("#publish-view-url");
}

function gistUrlInput(page: Page) {
  return page.locator("#publish-gist-url");
}

async function selectInsight(page: Page, title: string) {
  await page.locator(".insight-row", { hasText: title }).first().click();
  await expect(exportButton(page)).toBeVisible();
}

async function closePublishModal(page: Page) {
  await page
    .getByRole("button", { name: "Close publish dialog" })
    .click();
  await expect(page.locator(".publish-panel")).toHaveCount(0);
}

test.describe("Insight export and publish", () => {
  test("exports the selected insight through the local export route", async ({
    context,
    page,
  }) => {
    const recorder = await installMocks(context, page);

    await page.goto("/insights");
    await selectInsight(page, "agentsview");

    // A local connection has no auth token, so the export opens a tab. That
    // tab never renders -- the route answers with an attachment -- so the
    // oracle is the request it issued, not the tab's load state.
    const exportRequest = context.waitForEvent(
      "request",
      (request) =>
        new URL(request.url()).pathname ===
        "/api/v1/insights/501/export",
    );
    await exportButton(page).click();
    const request = await exportRequest;

    expect(new URL(request.url()).origin).toBe(
      new URL(page.url()).origin,
    );
    expect(recorder.requests).toContain(
      "GET /api/v1/insights/501/export",
    );
    expect(recorder.githubRequests).toEqual([]);
    expect(recorder.pageErrors).toEqual([]);
    for (const other of context.pages()) {
      if (other !== page) {
        expect(other.url()).not.toContain("github.com");
        await other.close().catch(() => {});
      }
    }
  });

  test("publishes public and secret gists with the right query", async ({
    context,
    page,
  }) => {
    const recorder = await installMocks(context, page);

    await page.goto("/insights");
    await selectInsight(page, "agentsview");

    await publicPublishButton(page).click();
    await expect(viewUrlInput(page)).toHaveValue(
      "https://viewer.example.test/public-501",
    );
    await expect(gistUrlInput(page)).toHaveValue(
      "https://gist.example.test/public-501",
    );
    await closePublishModal(page);

    await secretPublishButton(page).click();
    await expect(viewUrlInput(page)).toHaveValue(
      "https://viewer.example.test/secret-501",
    );
    await closePublishModal(page);

    expect(recorder.requests).toContain(
      "POST /api/v1/insights/501/publish?secret=false",
    );
    expect(recorder.requests).toContain(
      "POST /api/v1/insights/501/publish?secret=true",
    );
    expect(recorder.githubRequests).toEqual([]);
  });

  test("reopening publishes the new selection, not the previous one", async ({
    context,
    page,
  }) => {
    const recorder = await installMocks(context, page);

    await page.goto("/insights");
    await selectInsight(page, "agentsview");

    await publicPublishButton(page).click();
    await expect(viewUrlInput(page)).toHaveValue(
      "https://viewer.example.test/public-501",
    );
    await closePublishModal(page);

    // Same modal, different insight: a reused target or a stale result would
    // publish 501 again, or show 501's URLs for 502.
    await selectInsight(page, "global");
    await publicPublishButton(page).click();
    await expect(viewUrlInput(page)).toHaveValue(
      "https://viewer.example.test/public-502",
    );

    const publishes = recorder.requests.filter((entry) =>
      entry.includes("/publish"),
    );
    expect(publishes).toEqual([
      "POST /api/v1/insights/501/publish?secret=false",
      "POST /api/v1/insights/502/publish?secret=false",
    ]);
    expect(recorder.githubRequests).toEqual([]);
  });

  for (const viewport of [
    { name: "desktop", width: 1280, height: 800 },
    { name: "narrow", width: 400, height: 844 },
  ]) {
    test(`shows all three actions without page overflow on ${viewport.name}`, async ({
      context,
      page,
    }) => {
      await installMocks(context, page);
      await page.setViewportSize({
        width: viewport.width,
        height: viewport.height,
      });

      await page.goto("/insights");
      await selectInsight(page, "agentsview");

      await expect(exportButton(page)).toBeVisible();
      await expect(publicPublishButton(page)).toBeVisible();
      await expect(secretPublishButton(page)).toBeVisible();

      const noOverflow = await page.evaluate(
        () =>
          document.documentElement.scrollWidth <=
          document.documentElement.clientWidth,
      );
      expect(noOverflow).toBe(true);
    });
  }
});
