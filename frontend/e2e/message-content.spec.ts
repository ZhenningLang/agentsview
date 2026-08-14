import { test, expect, type Page } from "@playwright/test";
import { RuntimeErrorMonitor } from "./helpers/runtime-error-monitor";

const LOC = {
  sessionItem: ".session-item",
  sessionProject: ".session-project",
  sessionCount: ".session-count",
  listScroll: ".message-list-scroll",
  row: ".virtual-row",
} as const;

const BETA_7 = {
  project: "project-beta",
  count: 3, // user_message_count shown in sidebar
  displayRows: 6,
};

function getSessionItem(
  page: Page,
  project: string,
  count: number,
) {
  return page
    .locator(LOC.sessionItem)
    .filter({
      has: page.locator(
        `${LOC.sessionProject}:text-is("${project}")`,
      ),
    })
    .filter({
      has: page.locator(
        `${LOC.sessionCount}:text-is("${count}")`,
      ),
    });
}

async function selectSession(
  page: Page,
  project: string,
  count: number,
): Promise<string> {
  const item = getSessionItem(page, project, count);
  const sessionId = await item.getAttribute("data-session-id");
  expect(sessionId).toBeTruthy();
  await item.click();
  await expect(item).toHaveClass(/active/);
  return sessionId!;
}

async function expectSessionLoaded(
  page: Page,
  sessionId: string,
  expectedRows?: number,
) {
  const messageList = page.locator(LOC.listScroll);
  await expect(messageList).toHaveAttribute(
    "data-session-id",
    sessionId,
  );
  await expect(messageList).toHaveAttribute(
    "data-messages-session-id",
    sessionId,
  );
  await expect(messageList).toHaveAttribute(
    "data-loaded",
    "true",
  );

  if (expectedRows !== undefined) {
    await expect(page.locator(LOC.row)).toHaveCount(expectedRows);
  } else {
    await expect(
      page.locator(LOC.row).first(),
    ).toBeVisible();
  }
}

test.describe("Mixed content rendering", () => {
  test.beforeEach(async ({ page }) => {
    await page.goto("/");
    await expect(
      page.locator(LOC.sessionItem).first(),
    ).toBeVisible({ timeout: 5_000 });
  });

  test("tool group renders for consecutive tool-only messages", async ({
    page,
  }) => {
    const { project, count, displayRows } = BETA_7;
    const sid = await selectSession(page, project, count);
    await expectSessionLoaded(page, sid, displayRows);

    const toolGroup = page.locator(".tool-group");
    await expect(toolGroup).toBeVisible();
    await expect(toolGroup).toContainText(/tool calls?/i);

    const toolGroupBody = page.locator(".tool-group-body");
    await expect(toolGroupBody).toBeVisible();

    // Should contain exactly 2 tool blocks inside the group
    // (Indices 3 and 4 in the fixture are tool calls)
    const toolBlocks = toolGroupBody.locator(".tool-block");
    await expect(toolBlocks).toHaveCount(2);
  });

  test("tool block expands on click and text is selectable", async ({
    page,
  }) => {
    const { project, count, displayRows } = BETA_7;
    const sid = await selectSession(page, project, count);
    await expectSessionLoaded(page, sid, displayRows);

    const toolBlock = page.locator(".tool-block").first();
    await expect(toolBlock).toBeVisible();

    // Tool content should be hidden (collapsed by default)
    const toolContent = toolBlock.locator(".tool-content");
    await expect(toolContent).not.toBeVisible();

    // Click the header to expand
    const toolHeader = toolBlock.locator(".tool-header");
    await toolHeader.click();

    // Content should now be visible
    await expect(toolContent).toBeVisible();

    // Verify text is selectable inside the tool content
    const isSelectable = await toolContent.evaluate((el) => {
      const style = window.getComputedStyle(el);
      return style.userSelect !== "none";
    });
    expect(isSelectable).toBe(true);

    // Verify the tool header button allows text selection
    const headerSelectable = await toolHeader.evaluate((el) => {
      const style = window.getComputedStyle(el);
      return style.userSelect !== "none";
    });
    expect(headerSelectable).toBe(true);
  });

  test("text selection does not collapse tool block", async ({
    page,
  }) => {
    const { project, count, displayRows } = BETA_7;
    const sid = await selectSession(page, project, count);
    await expectSessionLoaded(page, sid, displayRows);

    // Expand the tool block first
    const toolBlock = page.locator(".tool-block").first();
    const toolHeader = toolBlock.locator(".tool-header");
    await toolHeader.click();

    const toolContent = toolBlock.locator(".tool-content");
    await expect(toolContent).toBeVisible();

    // Simulate a text selection then click the header
    // The block should remain expanded because there's a selection
    await toolContent.evaluate((el) => {
      const range = document.createRange();
      range.selectNodeContents(el);
      const sel = window.getSelection()!;
      sel.removeAllRanges();
      sel.addRange(range);
    });
    await toolHeader.click();

    // Tool content should still be visible (click was suppressed)
    await expect(toolContent).toBeVisible();

    // Clear selection and click again - now it should collapse
    await page.evaluate(() =>
      window.getSelection()?.removeAllRanges(),
    );
    await toolHeader.click();
    await expect(toolContent).not.toBeVisible();
  });

  test("thinking block is collapsed by default", async ({
    page,
  }) => {
    const { project, count, displayRows } = BETA_7;
    const sid = await selectSession(page, project, count);
    await expectSessionLoaded(page, sid, displayRows);

    const thinkingBlock = page.locator(".thinking-block").first();
    await expect(thinkingBlock).toBeVisible();

    // Content should be hidden (collapsed by default)
    const thinkingContent = thinkingBlock.locator(".thinking-content");
    await expect(thinkingContent).not.toBeVisible();

    // Click to expand
    const thinkingHeader = thinkingBlock.locator(".thinking-header");
    await thinkingHeader.click();

    // Content should now be visible
    await expect(thinkingContent).toBeVisible();
    await expect(thinkingContent).toContainText(
      "Let me analyze...",
    );
  });

  test("thinking+text message shows response text", async ({
    page,
  }) => {
    const { project, count, displayRows } = BETA_7;
    const sid = await selectSession(page, project, count);
    await expectSessionLoaded(page, sid, displayRows);

    // The response text after thinking should be visible
    await expect(
      page
        .locator(LOC.row)
        .filter({
          hasText: "visible response after thinking",
        }),
    ).toBeVisible();
  });

  test("response text remains after toggling thinking off", async ({
    page,
  }) => {
    const { project, count, displayRows } = BETA_7;
    const sid = await selectSession(page, project, count);
    await expectSessionLoaded(page, sid, displayRows);

    // Open block filter dropdown and toggle thinking off
    await page
      .locator('button[aria-label="Filter block types"]')
      .click();
    await page
      .locator(".block-filter-item")
      .filter({ hasText: "Thinking blocks" })
      .click();

    // Thinking blocks should be hidden
    const thinkingBlocks = page.locator(".thinking-block");
    await expect(thinkingBlocks).toHaveCount(0);

    // Response text should still be visible
    await expect(
      page
        .locator(LOC.row)
        .filter({
          hasText: "visible response after thinking",
        }),
    ).toBeVisible();
  });
});

const VALID_DIAGRAM = "flowchart LR\n  A[Start] --> B[End]";
const MALFORMED_DIAGRAM = "flowchart LR\n  A[[[[ --> ]]]]] %%{ bogus";
// Labels that would become executable markup if anything downstream trusted
// the diagram source or Mermaid's own SVG output.
const ADVERSARIAL_DIAGRAM =
  'flowchart LR\n  X["<img src=x onerror=alert(1)>"] --> ' +
  'Y["<script>window.__xss=1</script>"]\n  click X "javascript:alert(2)"';

function mermaidFence(source: string): string {
  return "```mermaid\n" + source + "\n```";
}

/**
 * Replace the first message's content on the selected session. Rewriting the
 * real response keeps the envelope, ordinals and pagination exactly as the
 * server produces them.
 */
async function injectMessageContent(page: Page, content: string) {
  await page.route("**/api/v1/sessions/*/messages*", async (route) => {
    const response = await route.fetch();
    const body = await response.json();
    const list = body.messages ?? [];
    if (list.length > 0) {
      const first = list[0];
      first.content = content;
      first.content_length = first.content.length;
      first.has_tool_use = false;
      first.tool_calls = [];
    }
    const headers = { ...response.headers() };
    // The rewritten body has a different length; a stale content-length would
    // truncate the response.
    delete headers["content-length"];
    headers["content-type"] = "application/json";
    await route.fulfill({
      status: response.status(),
      headers,
      body: JSON.stringify(body),
    });
  });
}

async function injectMermaidMessages(page: Page) {
  await injectMessageContent(
    page,
    "Valid diagram:\n\n" +
      mermaidFence(VALID_DIAGRAM) +
      "\n\nBroken diagram:\n\n" +
      mermaidFence(MALFORMED_DIAGRAM) +
      "\n\nAdversarial diagram:\n\n" +
      mermaidFence(ADVERSARIAL_DIAGRAM),
  );
}

/** Open the fixture session that every Mermaid spec uses. */
async function openFixtureSession(page: Page) {
  await page.goto("/");
  await expect(page.locator(LOC.sessionItem).first()).toBeVisible({
    timeout: 5_000,
  });
  const { project, count } = BETA_7;
  const sid = await selectSession(page, project, count);
  await expectSessionLoaded(page, sid);
}

/** Assert no executable markup reached the document. */
async function expectNoDangerousDOM(page: Page) {
  const findings = await page.evaluate(() => {
    const scripts = document.querySelectorAll(
      ".message-body script, .message-body iframe, .message-body object",
    ).length;
    const handlers = Array.from(
      document.querySelectorAll(".message-body *"),
    ).filter((el) =>
      Array.from(el.attributes).some((a) =>
        a.name.toLowerCase().startsWith("on"),
      ),
    ).length;
    const jsUrls = Array.from(
      document.querySelectorAll(".message-body [href], .message-body [xlink\\:href]"),
    ).filter((el) => {
      const raw =
        el.getAttribute("href") ?? el.getAttribute("xlink:href") ?? "";
      return raw.replace(/\s/g, "").toLowerCase().startsWith("javascript:");
    }).length;
    return {
      scripts,
      handlers,
      jsUrls,
      pwned: (window as unknown as { __xss?: unknown }).__xss ?? null,
    };
  });

  expect(findings.scripts).toBe(0);
  expect(findings.handlers).toBe(0);
  expect(findings.jsUrls).toBe(0);
  expect(findings.pwned).toBeNull();
}

test.describe("Mermaid diagrams", () => {
  test("renders a valid diagram, keeps broken source, and stays XSS-safe", async ({
    page,
  }) => {
    const monitor = new RuntimeErrorMonitor(page);
    await injectMermaidMessages(page);
    await page.goto("/");
    await expect(page.locator(LOC.sessionItem).first()).toBeVisible({
      timeout: 5_000,
    });

    const { project, count } = BETA_7;
    const sid = await selectSession(page, project, count);
    await expectSessionLoaded(page, sid);

    // The valid and adversarial diagrams both render; the malformed one does
    // not, so exactly two diagrams are expected.
    const diagrams = page.locator(".mermaid-diagram svg");
    await expect(diagrams).toHaveCount(2, { timeout: 15_000 });
    await expect(diagrams.first()).toBeVisible();

    // The broken fence keeps its complete source plus a visible status, with
    // no blank area and no Mermaid default error diagram.
    const status = page.locator('.mermaid-block [role="status"]');
    await expect(status).toHaveCount(1);
    await expect(status).toContainText(/failed to render/i);
    const failedBlock = page
      .locator(".mermaid-block")
      .filter({ has: page.locator('[role="status"]') });
    await expect(failedBlock.locator(".code-content")).toContainText(
      "%%{ bogus",
    );

    await expectNoDangerousDOM(page);

    // No CSP violation, unhandled rejection or uncaught Mermaid error.
    expect(
      monitor.matching(/Content Security Policy|Refused to|Unhandled|mermaid/i),
    ).toEqual([]);
  });

  test("search falls back to the diagram source", async ({ page }) => {
    await injectMermaidMessages(page);
    await page.goto("/");
    await expect(page.locator(LOC.sessionItem).first()).toBeVisible({
      timeout: 5_000,
    });

    const { project, count } = BETA_7;
    const sid = await selectSession(page, project, count);
    await expectSessionLoaded(page, sid);
    await expect(page.locator(".mermaid-diagram svg").first()).toBeVisible({
      timeout: 15_000,
    });

    await page.keyboard.press("ControlOrMeta+f");
    const input = page.locator('input[aria-label="Search query"]');
    await expect(input).toBeVisible();
    await input.fill("A[Start]");

    // Diagrams give way to searchable source with highlight marks on it.
    await expect(page.locator(".mermaid-diagram svg")).toHaveCount(0);
    const marked = page.locator(".code-content mark.search-highlight");
    await expect(marked.first()).toBeVisible({ timeout: 10_000 });
  });

  test("copies the exact diagram source", async ({ page, context }) => {
    // WebKit does not implement the clipboard-write permission, so the
    // portable oracle is the exact string handed to the Clipboard API (which
    // is the branch `copyToClipboard` takes in a secure context). Chromium
    // additionally reads the real clipboard back.
    let realClipboard = false;
    try {
      await context.grantPermissions(["clipboard-read", "clipboard-write"]);
      realClipboard = true;
    } catch {
      realClipboard = false;
    }

    await page.addInitScript(() => {
      const w = window as unknown as { __copied: string[] };
      w.__copied = [];
      const clipboard = navigator.clipboard;
      if (!clipboard) return;
      const original = clipboard.writeText?.bind(clipboard);
      Object.defineProperty(clipboard, "writeText", {
        configurable: true,
        value: async (text: string) => {
          w.__copied.push(text);
          if (original) await original(text);
        },
      });
    });

    await injectMermaidMessages(page);
    await page.goto("/");
    await expect(page.locator(LOC.sessionItem).first()).toBeVisible({
      timeout: 5_000,
    });

    const { project, count } = BETA_7;
    const sid = await selectSession(page, project, count);
    await expectSessionLoaded(page, sid);

    const firstBlock = page.locator(".mermaid-block").first();
    // Scope to the diagram container: `.mermaid-block svg` also matches the
    // copy button's icon, so it resolves to two elements once the diagram
    // lands and trips Playwright strict mode — intermittently, depending on
    // which of the two rendered first.
    await expect(firstBlock.locator(".mermaid-diagram svg")).toBeVisible({
      timeout: 15_000,
    });
    await firstBlock
      .locator('button[aria-label="Copy diagram source"]')
      .click();

    const recorded = await page.evaluate(
      () => (window as unknown as { __copied: string[] }).__copied,
    );
    expect(recorded).toEqual([VALID_DIAGRAM + "\n"]);

    if (realClipboard) {
      const copied = await page.evaluate(() => navigator.clipboard.readText());
      expect(copied).toBe(VALID_DIAGRAM + "\n");
    }
  });

  test("applies the dark Mermaid theme, not just a fresh render", async ({
    page,
  }) => {
    await injectMermaidMessages(page);
    await openFixtureSession(page);

    const svg = page.locator(".mermaid-diagram svg").first();
    await expect(svg).toBeVisible({ timeout: 15_000 });

    // `outerHTML` is NOT an oracle here: renderMermaid stamps an incrementing
    // id into every render and Mermaid embeds it in the diagram's <style>
    // selectors, so two renders of the *same* theme already differ. Assert on
    // what the theme actually determines — the label colour, plus the
    // id-normalized stylesheet.
    const readTheme = () =>
      page.evaluate(() => {
        const root = document.querySelector(".mermaid-diagram");
        const label = root?.querySelector("text, tspan, .nodeLabel");
        if (!root || !label) return null;
        const styleText = Array.from(root.querySelectorAll("style"))
          .map((el) => el.textContent ?? "")
          .join("")
          .replace(/mermaid-diagram-\d+/g, "DIAGRAM_ID");
        return { fill: getComputedStyle(label).fill, styleText };
      });

    const light = await readTheme();
    expect(light).not.toBeNull();
    expect(light!.styleText.length).toBeGreaterThan(0);

    // Cycle light -> dark.
    await page.locator('button[aria-label^="Theme:"]').click();
    await expect(page.locator('button[aria-label="Theme: dark"]')).toBeVisible();

    await expect
      .poll(async () => (await readTheme())?.fill, { timeout: 15_000 })
      .not.toBe(light!.fill);

    const dark = await readTheme();
    expect(dark).not.toBeNull();

    // The id-normalized stylesheet must differ too, so a same-theme re-render
    // cannot satisfy this test.
    expect(dark!.styleText).not.toBe(light!.styleText);

    // Direction matters: the dark theme must produce a *lighter* label than the
    // light theme. A swapped or dropped mapping fails here even if both themes
    // happen to render.
    const luminance = (rgb: string): number => {
      const [r, g, b] = (rgb.match(/[\d.]+/g) ?? []).map(Number);
      expect(r).not.toBeUndefined();
      return 0.2126 * r! + 0.7152 * g! + 0.0722 * b!;
    };
    expect(luminance(dark!.fill)).toBeGreaterThan(luminance(light!.fill));

    // Still a readable diagram, not an empty shell.
    await expect(svg).toBeVisible();
    await expect(page.locator(".mermaid-diagram").first()).toContainText("Start");
  });

  test("diagram does not overflow a 400px viewport", async ({ page }) => {
    await injectMermaidMessages(page);
    await page.goto("/");
    await expect(page.locator(LOC.sessionItem).first()).toBeVisible({
      timeout: 5_000,
    });

    const { project, count } = BETA_7;
    const sid = await selectSession(page, project, count);
    await expectSessionLoaded(page, sid);
    await expect(page.locator(".mermaid-diagram svg").first()).toBeVisible({
      timeout: 15_000,
    });

    // Narrow the viewport after the session is open: at 400px the sidebar
    // collapses and the session list is no longer reachable.
    await page.setViewportSize({ width: 400, height: 800 });
    await expect(page.locator(".mermaid-diagram svg").first()).toBeVisible();

    const overflow = await page.evaluate(() => ({
      body: document.body.scrollWidth - document.body.clientWidth,
      scrollable: Array.from(
        document.querySelectorAll(".mermaid-diagram"),
      ).every((el) => getComputedStyle(el).overflowX === "auto"),
    }));

    expect(overflow.body).toBeLessThanOrEqual(0);
    expect(overflow.scrollable).toBe(true);
  });
});

test.describe("Mermaid lazy loading", () => {
  /** Track every asset request so "was the runtime fetched?" is observable. */
  function trackAssets(page: Page): string[] {
    const urls: string[] = [];
    page.on("request", (req) => urls.push(req.url()));
    return urls;
  }

  const MERMAID_CHUNK = /mermaid|cytoscape|katex/i;

  test("a session without diagrams never fetches the Mermaid runtime", async ({
    page,
  }) => {
    const urls = trackAssets(page);
    await page.goto("/");
    await expect(page.locator(LOC.sessionItem).first()).toBeVisible({
      timeout: 5_000,
    });

    const { project, count, displayRows } = BETA_7;
    const sid = await selectSession(page, project, count);
    await expectSessionLoaded(page, sid, displayRows);
    await expect(page.locator(".virtual-row").first()).toBeVisible();

    expect(urls.filter((u) => MERMAID_CHUNK.test(u))).toEqual([]);
  });

  test("the runtime is fetched once a diagram appears", async ({ page }) => {
    const urls = trackAssets(page);
    await injectMermaidMessages(page);
    await page.goto("/");
    await expect(page.locator(LOC.sessionItem).first()).toBeVisible({
      timeout: 5_000,
    });

    const { project, count } = BETA_7;
    const sid = await selectSession(page, project, count);
    await expectSessionLoaded(page, sid);
    await expect(page.locator(".mermaid-diagram svg").first()).toBeVisible({
      timeout: 15_000,
    });

    expect(
      urls.filter((u) => /mermaid/i.test(u) && u.endsWith(".js")).length,
    ).toBeGreaterThan(0);
  });
});

/**
 * Real-runtime oracles. Every other Mermaid assertion in this file can pass
 * against markup the pinned runtime never emits; these consume what
 * mermaid@11.16.1 actually produces after the app's own DOMPurify pass.
 */
test.describe("Mermaid real-runtime output", () => {
  // Flowchart, class and state diagrams route their labels through
  // `addHtmlSpan` -> <foreignObject>, which DOMPurify both disallows AND lists
  // in FORBID_CONTENTS — so an over-permissive htmlLabels setting deletes the
  // label text outright and leaves visible but empty shapes. Sequence diagrams
  // emit <text> and are unaffected, so both families are covered here.
  const LABELLED = [
    {
      name: "flowchart",
      source: "flowchart LR\n  A[Alpha] --> B[Beta]",
      labels: ["Alpha", "Beta"],
    },
    {
      name: "classDiagram",
      source: "classDiagram\n  class Animal {\n    +String name\n  }",
      labels: ["Animal", "name"],
    },
    {
      name: "stateDiagram",
      source: "stateDiagram-v2\n  Idle --> Busy\n  Busy --> Idle",
      labels: ["Idle", "Busy"],
    },
    {
      name: "sequenceDiagram",
      source: "sequenceDiagram\n  Alice->>Bob: Hello Bob",
      labels: ["Alice", "Bob", "Hello Bob"],
    },
  ] as const;

  test("every rendered diagram keeps its node labels", async ({ page }) => {
    await injectMessageContent(
      page,
      LABELLED.map((d) => `${d.name}:\n\n${mermaidFence(d.source)}`).join("\n\n"),
    );
    await openFixtureSession(page);

    const diagrams = page.locator(".mermaid-diagram");
    await expect(diagrams).toHaveCount(LABELLED.length, { timeout: 15_000 });

    for (const [i, spec] of LABELLED.entries()) {
      const text = await diagrams.nth(i).innerText();
      for (const label of spec.labels) {
        expect(
          text,
          `${spec.name} lost the label ${JSON.stringify(label)} — ` +
            "an SVG that renders shapes with no text still passes " +
            "a presence/visibility assertion",
        ).toContain(label);
      }
    }
  });

  test("frontmatter cannot override the app theme or inject CSS", async ({
    page,
  }) => {
    // A transcript is untrusted input. Mermaid merges a diagram's own YAML
    // frontmatter into the effective config unless the key is in `secure`.
    const attack =
      "---\n" +
      "config:\n" +
      "  theme: dark\n" +
      '  themeCSS: ".nodeLabel { visibility: hidden } * { fill: rgb(1, 2, 3) }"\n' +
      "  securityLevel: loose\n" +
      "  htmlLabels: true\n" +
      "---\n" +
      "flowchart LR\n  A[Alpha] --> B[Beta]";
    await injectMessageContent(
      page,
      `Attack:\n\n${mermaidFence(attack)}\n\nControl:\n\n${mermaidFence(
        "flowchart LR\n  A[Alpha] --> B[Beta]",
      )}`,
    );
    await openFixtureSession(page);

    const diagrams = page.locator(".mermaid-diagram");
    await expect(diagrams).toHaveCount(2, { timeout: 15_000 });

    // The injected CSS must not have reached the document at all.
    const injected = await page.evaluate(() =>
      Array.from(document.querySelectorAll(".mermaid-diagram style")).map(
        (el) => el.textContent ?? "",
      ),
    );
    for (const css of injected) {
      expect(css).not.toContain("rgb(1, 2, 3)");
      expect(css).not.toContain("visibility: hidden");
    }

    // Labels survive, i.e. the attack did not blank the diagram either.
    await expect(diagrams.nth(0)).toContainText("Alpha");

    // The attacked diagram must look exactly like the control one: same theme,
    // same label styling. Comparing computed styles catches a theme swap that
    // a string search for "dark" would miss.
    const styles = await page.evaluate(() => {
      const read = (i: number) => {
        const root = document.querySelectorAll(".mermaid-diagram")[i];
        const label = root?.querySelector("text, .nodeLabel, span, tspan");
        if (!label) return null;
        const cs = getComputedStyle(label);
        return { fill: cs.fill, color: cs.color, visibility: cs.visibility };
      };
      return { attacked: read(0), control: read(1) };
    });
    expect(styles.attacked).not.toBeNull();
    expect(styles.attacked).toEqual(styles.control);
    expect(styles.attacked!.visibility).toBe("visible");
  });
});
