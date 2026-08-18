import {
  expect,
  test,
  type BrowserContext,
  type Locator,
  type Page,
} from "@playwright/test";

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
  /**
   * API calls this spec never stubbed. Must stay empty: an unstubbed call
   * used to be handed to the real e2e Go server, and that server answers
   * /api/v1/update/check by fetching github.com from its own process --
   * an escape no browser-side recorder can observe.
   */
  unstubbed: string[];
}

/**
 * Startup calls this app makes that are inert here: the e2e server answers
 * them from a temp fixture DB whose agent directories all point at an empty
 * dir, so letting them through touches nothing outside the test.
 *
 * The list is explicit on purpose. /api/v1/update/check used to land in a
 * blanket route.fallback() and made the Go process fetch github.com -- an
 * outbound request no browser-side recorder can see. Anything not named here
 * is answered locally and fails the spec, so a new endpoint has to be looked
 * at before it is trusted.
 */
const LOCAL_ONLY_PATHS = new Set([
  "/api/v1/agents",
  "/api/v1/events",
  "/api/v1/llm/balance",
  "/api/v1/sessions/sidebar-index",
  "/api/v1/settings",
  "/api/v1/starred",
  "/api/v1/stats",
  "/api/v1/sync/status",
  "/api/v1/version",
]);

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
    unstubbed: [],
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

    // The app checks for updates on mount. Left to the real server this is
    // an outbound request to github.com made by the Go process, so it is
    // answered here and the server is additionally started with
    // AGENTSVIEW_DISABLE_UPDATE_CHECK=1 (scripts/e2e-server.sh).
    if (path === "/api/v1/update/check") {
      await route.fulfill({
        json: { current_version: "e2e", update_available: false },
      });
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

    // Fail closed: only reviewed, inert endpoints reach the local server.
    if (LOCAL_ONLY_PATHS.has(path)) {
      await route.fallback();
      return;
    }
    // Everything else is answered here and recorded as a failure, so a call
    // this spec has not vetted breaks the test instead of quietly leaving
    // for the network by way of the server process.
    recorder.unstubbed.push(`${request.method()} ${path}`);
    await route.fulfill({ json: {} });
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

/**
 * Assert an action is actually reachable, not merely "visible".
 *
 * Playwright's visibility check passes for an element an ancestor has clipped
 * with overflow:hidden, and a page-level scrollWidth check passes too, because
 * the clip is what stops the page from growing. So this checks the two things
 * that actually decide whether a user can hit the control: its box lies inside
 * the viewport, and the element painted at its own corners is the element
 * itself rather than whatever clipped it.
 */
async function expectReachable(
  page: Page,
  locator: Locator,
  label: string,
) {
  const box = await locator.boundingBox();
  expect(box, `${label}: no layout box`).not.toBeNull();
  const viewport = page.viewportSize();
  expect(viewport, "viewport size is unknown").not.toBeNull();

  expect(box!.width, `${label}: zero width`).toBeGreaterThan(0);
  expect(box!.height, `${label}: zero height`).toBeGreaterThan(0);
  expect(box!.x, `${label}: starts left of the viewport`)
    .toBeGreaterThanOrEqual(0);
  expect(box!.y, `${label}: starts above the viewport`)
    .toBeGreaterThanOrEqual(0);
  expect(
    box!.x + box!.width,
    `${label}: right edge ${box!.x + box!.width} exceeds viewport width ` +
      `${viewport!.width}`,
  ).toBeLessThanOrEqual(viewport!.width);
  expect(
    box!.y + box!.height,
    `${label}: bottom edge exceeds viewport height`,
  ).toBeLessThanOrEqual(viewport!.height);

  const painted = await locator.evaluate((el) => {
    const r = el.getBoundingClientRect();
    // Inset rather than the literal corner: these controls are rounded, so
    // the corner pixel legitimately belongs to the parent even when nothing
    // clipped anything.
    const inset = Math.max(2, Math.min(r.width, r.height) * 0.25);
    const probes: [number, number][] = [
      [r.left + inset, r.top + inset],
      [r.right - inset, r.top + inset],
      [r.left + inset, r.bottom - inset],
      [r.right - inset, r.bottom - inset],
      [r.left + r.width / 2, r.top + r.height / 2],
    ];
    return probes.every(([x, y]) => {
      const top = document.elementFromPoint(x, y);
      // The element itself or something inside it. An ancestor answering
      // here means the ancestor clipped or covered the control, which is
      // exactly the failure this probe exists to catch.
      return top !== null && (top === el || el.contains(top));
    });
  });
  expect(painted, `${label}: clipped or covered at its own corners`).toBe(
    true,
  );
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
    // Polled, not read once: the request event and the route handler that
    // records it are independent, and webkit reaches the event first.
    await expect
      .poll(() => recorder.requests)
      .toContain("GET /api/v1/insights/501/export");
    expect(recorder.githubRequests).toEqual([]);
    expect(recorder.unstubbed).toEqual([]);
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
    expect(recorder.unstubbed).toEqual([]);
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
    expect(recorder.unstubbed).toEqual([]);
  });

  for (const viewport of [
    { name: "desktop", width: 1280, height: 800 },
    { name: "narrow", width: 400, height: 844 },
  ]) {
    test(`shows all three actions without page overflow on ${viewport.name}`, async ({
      context,
      page,
    }) => {
      const recorder = await installMocks(context, page);
      await page.setViewportSize({
        width: viewport.width,
        height: viewport.height,
      });

      await page.goto("/insights");
      await selectInsight(page, "agentsview");

      await expect(exportButton(page)).toBeVisible();
      await expect(publicPublishButton(page)).toBeVisible();
      await expect(secretPublishButton(page)).toBeVisible();

      await expectReachable(page, exportButton(page), "export");
      await expectReachable(
        page,
        publicPublishButton(page),
        "publish public",
      );
      await expectReachable(
        page,
        secretPublishButton(page),
        "publish secret",
      );

      const noOverflow = await page.evaluate(
        () =>
          document.documentElement.scrollWidth <=
          document.documentElement.clientWidth,
      );
      expect(noOverflow).toBe(true);
      expect(recorder.githubRequests).toEqual([]);
      expect(recorder.unstubbed).toEqual([]);
    });
  }
});
