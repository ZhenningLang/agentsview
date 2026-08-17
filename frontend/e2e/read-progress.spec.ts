import { expect, test, type Page } from "@playwright/test";

/**
 * Browser acceptance for transcript read progress.
 *
 * The server owns `transcript_revision`, a per-session counter that only
 * moves when user-visible transcript content changes. The browser owns the
 * marker that records which revision the user last saw and how far into it
 * they read. These specs drive the real e2e server against the fixture
 * archive and simulate the backend bumping a revision by rewriting that one
 * field on the session payloads the app reads. Everything else — the
 * message pages, the SSE watch stream — reaches the server untouched.
 */

const STORAGE_KEY = "agentsview-read-progress";
const STORAGE_VERSION = 2;

/** 100 messages: several viewports tall, yet still small enough to load in
 *  one page, so the transcript can be traversed end to end. */
const UNREAD_SESSION = "test-session-medium-100";
/** Highest ordinal in that session. */
const UNREAD_SESSION_LATEST_ORDINAL = 99;
/** Any other session; used only to reach the transcript toolbar. */
const NEUTRAL_SESSION = "test-session-small-5";
/** Ordinals 3 and 4 are the fixture's only consecutive tool-only messages,
 *  so they render as the children of a single tool group. */
const TOOL_GROUP_SESSION = "test-session-mixed-content-7";

/** Revision the fixture archive reports for every seeded session. */
const FIXTURE_REVISION = "1";
/** Stands in for "the backend recorded new transcript content". */
const BUMPED_REVISION = "77";

interface Marker {
  token: string;
  ordinal: number | null;
  touched_at: number;
}

function sessionRow(page: Page, id: string) {
  return page.locator(`.session-item[data-session-id="${id}"]`);
}

/** Sidebar unread indicators, addressed by their accessible name rather
 *  than their class so the assertion also covers the a11y contract. */
function unreadDots(page: Page) {
  return page.getByRole("img", { name: "Unread messages" });
}

function unreadDividers(page: Page) {
  return page.getByRole("separator", { name: "Read progress boundary" });
}

async function markerFor(
  page: Page,
  id: string,
): Promise<Marker | null> {
  return await page.evaluate(
    ({ key, id: sessionId }) => {
      let raw: string | null = null;
      try {
        raw = globalThis.localStorage.getItem(key);
      } catch {
        return null;
      }
      if (!raw) return null;
      const parsed = JSON.parse(raw) as {
        sessions?: Record<string, Marker>;
      };
      return parsed.sessions?.[sessionId] ?? null;
    },
    { key: STORAGE_KEY, id },
  );
}

async function markerToken(
  page: Page,
  id: string,
): Promise<string | null> {
  return (await markerFor(page, id))?.token ?? null;
}

function rewriteRevision(
  value: unknown,
  id: string,
  revision: string,
): void {
  if (Array.isArray(value)) {
    for (const entry of value) rewriteRevision(entry, id, revision);
    return;
  }
  if (!value || typeof value !== "object") return;
  const record = value as Record<string, unknown>;
  if (record.id === id && "transcript_revision" in record) {
    record.transcript_revision = revision;
  }
  for (const nested of Object.values(record)) {
    rewriteRevision(nested, id, revision);
  }
}

/**
 * Report `revision` as one session's transcript revision on every session
 * payload the app reads: the sidebar index, the session list page and the
 * session detail the transcript store fetches.
 */
async function serveRevision(
  page: Page,
  id: string,
  revision: string,
): Promise<void> {
  await page.route("**/api/v1/sessions**", async (route) => {
    const path = new URL(route.request().url()).pathname
      .replace(/\/+$/, "");
    const rewritable = path.endsWith("/api/v1/sessions") ||
      path.endsWith("/api/v1/sessions/sidebar-index") ||
      path.endsWith(`/api/v1/sessions/${id}`);
    if (!rewritable) {
      await route.continue();
      return;
    }

    const response = await route.fetch();
    const body = await response.json();
    rewriteRevision(body, id, revision);
    await route.fulfill({
      status: response.status(),
      contentType: "application/json",
      body: JSON.stringify(body),
    });
  });
}

/** Pre-seed a marker so a test can start from "read this far, under an
 *  older revision" without driving the whole flow first. */
async function seedMarker(
  page: Page,
  id: string,
  marker: Marker,
): Promise<void> {
  await page.addInitScript(
    ({ key, version, id: sessionId, marker: seeded }) => {
      globalThis.localStorage.setItem(
        key,
        JSON.stringify({
          version,
          sessions: { [sessionId]: seeded },
        }),
      );
    },
    { key: STORAGE_KEY, version: STORAGE_VERSION, id, marker },
  );
}

async function openSessionList(page: Page): Promise<void> {
  await page.goto("/");
  await expect(page.locator(".session-item").first()).toBeVisible({
    timeout: 10_000,
  });
}

async function openTranscript(page: Page, id: string): Promise<void> {
  await sessionRow(page, id).click();
  await expect(
    page.locator(
      `.message-list-scroll[data-messages-session-id="${id}"]` +
        `[data-loaded="true"]`,
    ),
  ).toBeVisible();
  await expect(page.locator(".virtual-row").first()).toBeVisible();
}

async function scrollTranscript(
  page: Page,
  to: "start" | "end",
): Promise<void> {
  await page.locator(".message-list-scroll").evaluate(
    (el, target) => {
      el.scrollTop = target === "end" ? el.scrollHeight : 0;
    },
    to,
  );
}

/** Walk the transcript to the far end until the session reports itself as
 *  read. Rows are measured lazily, so the end keeps moving until the last
 *  row has been rendered — hence the retry rather than one scroll. */
async function traverseToFarEnd(page: Page): Promise<void> {
  await expect(async () => {
    await scrollTranscript(page, "end");
    await expect(unreadDots(page)).toHaveCount(0, { timeout: 1_000 });
  }).toPass({ timeout: 12_000 });
}

/** First visit baselines the archive; then the backend moves one session's
 *  revision. Leaves the page on the session list with that one row unread. */
async function baselineThenBump(page: Page): Promise<void> {
  await openSessionList(page);
  await expect
    .poll(() => markerToken(page, UNREAD_SESSION))
    .toBe(FIXTURE_REVISION);

  await serveRevision(page, UNREAD_SESSION, BUMPED_REVISION);
  await page.reload();
  await expect(page.locator(".session-item").first()).toBeVisible();
}

test.describe("Transcript read progress", () => {
  test("first visit baselines the archive without unread dots", async ({
    page,
  }) => {
    await openSessionList(page);

    await expect
      .poll(() => markerToken(page, UNREAD_SESSION))
      .toBe(FIXTURE_REVISION);

    // A pre-existing archive must not light up wholesale on first sight.
    await expect(unreadDots(page)).toHaveCount(0);
    const marker = await markerFor(page, UNREAD_SESSION);
    expect(marker?.ordinal).toBeNull();
  });

  test("a bumped revision marks only that session unread", async ({
    page,
  }) => {
    await baselineThenBump(page);

    await expect(
      sessionRow(page, UNREAD_SESSION).getByRole("img", {
        name: "Unread messages",
      }),
    ).toBeVisible();
    // Sessions still at their baseline revision stay quiet.
    await expect(unreadDots(page)).toHaveCount(1);
    expect(await markerToken(page, UNREAD_SESSION)).toBe(
      FIXTURE_REVISION,
    );
  });

  test("opening an unread transcript shows the boundary until it is traversed", async ({
    page,
  }) => {
    await baselineThenBump(page);
    await openTranscript(page, UNREAD_SESSION);

    // Oldest first: the unread run starts at the top of the window.
    const divider = unreadDividers(page);
    await expect(divider).toHaveCount(1);
    await expect(divider.first()).toContainText("New messages");
    // The tail is still below the fold, so nothing is confirmed yet.
    await expect(unreadDots(page)).toHaveCount(1);

    await traverseToFarEnd(page);

    await expect(unreadDividers(page)).toHaveCount(0);
    const marker = await markerFor(page, UNREAD_SESSION);
    expect(marker?.token).toBe(BUMPED_REVISION);
    expect(marker?.ordinal).toBe(UNREAD_SESSION_LATEST_ORDINAL);
  });

  test("newest first: reading the tail alone does not confirm the revision", async ({
    page,
  }) => {
    await baselineThenBump(page);

    // Flip the sort on an unrelated session so the unread transcript opens
    // newest first, with the latest message already on screen.
    await openTranscript(page, NEUTRAL_SESSION);
    await page.getByLabel("Toggle sort order").click();
    await openTranscript(page, UNREAD_SESSION);

    // The unread run starts at the far end, which is off screen.
    await expect(unreadDividers(page)).toHaveCount(0);
    await expect(unreadDots(page)).toHaveCount(1);

    // Re-reading the newest rows must not stand in for the rewrite that
    // happened further back.
    for (let i = 0; i < 4; i++) {
      await page.locator(".message-list-scroll").evaluate((el) => {
        el.scrollTop = 300;
      });
      await scrollTranscript(page, "start");
    }
    await expect(unreadDots(page)).toHaveCount(1);
    expect(await markerToken(page, UNREAD_SESSION)).toBe(
      FIXTURE_REVISION,
    );

    // Reaching the boundary at the far end is what confirms it.
    await traverseToFarEnd(page);
    const marker = await markerFor(page, UNREAD_SESSION);
    expect(marker?.token).toBe(BUMPED_REVISION);
    expect(marker?.ordinal).toBe(UNREAD_SESSION_LATEST_ORDINAL);
  });

  test("the boundary lands inside a tool group between its children", async ({
    page,
  }) => {
    // Read as far as the first tool call, under the previous revision.
    await seedMarker(page, TOOL_GROUP_SESSION, {
      token: "0",
      ordinal: 3,
      touched_at: 1,
    });

    await page.goto(`/sessions/${TOOL_GROUP_SESSION}`);
    await expect(page.locator(".tool-group")).toBeVisible({
      timeout: 10_000,
    });

    const groupBody = page.locator(".tool-group-body");
    const divider = groupBody.getByRole("separator", {
      name: "Read progress boundary",
    });
    await expect(divider).toBeVisible();
    await expect(divider).toContainText("New messages");
    // Grouped rows must not also draw a boundary around the whole group.
    await expect(unreadDividers(page)).toHaveCount(1);

    // Both children stay rendered; the boundary sits between them.
    await expect(
      groupBody.locator('[data-message-ordinal="3"]'),
    ).toBeVisible();
    await expect(
      groupBody.locator('[data-message-ordinal="4"]'),
    ).toBeVisible();
    const layout = await groupBody.evaluate((body) => {
      const out: string[] = [];
      for (const child of Array.from(body.children)) {
        if (child.classList.contains("unread-divider")) {
          out.push("boundary");
          continue;
        }
        const ordinal = (child as HTMLElement).dataset.messageOrdinal;
        if (ordinal !== undefined) out.push(`ordinal:${ordinal}`);
      }
      return out;
    });
    expect(layout).toEqual(["ordinal:3", "boundary", "ordinal:4"]);

    await expect(unreadDots(page)).toHaveCount(1);
    await traverseToFarEnd(page);
    await expect(unreadDividers(page)).toHaveCount(0);
  });

  test("the read marker survives a reload and still detects a later bump", async ({
    page,
  }) => {
    await openSessionList(page);
    await expect
      .poll(() => markerToken(page, UNREAD_SESSION))
      .toBe(FIXTURE_REVISION);

    await page.reload();
    await expect(page.locator(".session-item").first()).toBeVisible();

    // The marker is read back from storage, not re-derived per page load.
    expect(await markerToken(page, UNREAD_SESSION)).toBe(
      FIXTURE_REVISION,
    );
    await expect(unreadDots(page)).toHaveCount(0);

    // And it is the persisted marker that detects the next revision.
    await serveRevision(page, UNREAD_SESSION, BUMPED_REVISION);
    await page.reload();
    await expect(
      sessionRow(page, UNREAD_SESSION).getByRole("img", {
        name: "Unread messages",
      }),
    ).toBeVisible();
  });
});
