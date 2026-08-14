// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { mount, tick, unmount } from "svelte";
import { cleanup, render } from "@testing-library/svelte";
import { ui } from "../../stores/ui.svelte.js";
// @ts-ignore
import MermaidBlock from "./MermaidBlock.svelte";

const copyToClipboardMock = vi.hoisted(() =>
  vi.fn().mockResolvedValue(true),
);

const renderMermaidMock = vi.hoisted(() =>
  vi.fn<(source: string, theme: string) => Promise<string>>(),
);

// The real UIStore is used deliberately: a hand-rolled theme mock cannot prove
// that a *live* preference change re-renders an already-mounted diagram, which
// is exactly the "read the root class once at mount" bug this guards against.
vi.mock("../../utils/clipboard.js", () => ({
  copyToClipboard: copyToClipboardMock,
}));

vi.mock("../../utils/mermaid.js", async () => {
  const actual = await vi.importActual<
    typeof import("../../utils/mermaid.js")
  >("../../utils/mermaid.js");
  return { ...actual, renderMermaid: renderMermaidMock };
});

const SOURCE = "flowchart LR\n  A[Start] --> B[End]\n";

function svgFor(label: string): string {
  return `<svg data-variant="${label}" xmlns="http://www.w3.org/2000/svg"><text>${label}</text></svg>`;
}

/** A promise plus its resolvers, for pinning the loading state open. */
function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (err: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  // Nothing else awaits these promises, so an unobserved rejection would
  // otherwise surface as an unhandled rejection warning.
  promise.catch(() => {});
  return { promise, resolve, reject };
}

async function settle() {
  await tick();
  await Promise.resolve();
  await Promise.resolve();
  await tick();
}

function diagramSvg(): SVGElement | null {
  return document.querySelector(".mermaid-diagram svg");
}

function sourceText(): string {
  return document.querySelector(".code-block")?.textContent ?? "";
}

beforeEach(() => {
  renderMermaidMock.mockReset();
  renderMermaidMock.mockResolvedValue(svgFor("light"));
  ui.setThemePreference("light");
});

afterEach(() => {
  document.body.innerHTML = "";
  vi.clearAllMocks();
  ui.setThemePreference("light");
});

describe("MermaidBlock", () => {
  it("shows the source with a copy affordance before the diagram resolves", async () => {
    const pending = deferred<string>();
    renderMermaidMock.mockReturnValue(pending.promise);

    const component = mount(MermaidBlock, {
      target: document.body,
      props: { content: SOURCE },
    });

    await settle();

    expect(diagramSvg()).toBeNull();
    expect(sourceText()).toContain("A[Start] --> B[End]");
    expect(
      document.querySelector('button.copy-btn[aria-label="Copy code block"]'),
    ).not.toBeNull();
    // No failure status while the render is merely in flight.
    expect(document.querySelector('[role="status"]')).toBeNull();

    pending.resolve(svgFor("light"));
    await settle();
    unmount(component);
  });

  it("swaps to the sanitized diagram and drops the source block on success", async () => {
    const component = mount(MermaidBlock, {
      target: document.body,
      props: { content: SOURCE },
    });

    await settle();

    expect(renderMermaidMock).toHaveBeenCalledWith(SOURCE, "light");
    expect(diagramSvg()).not.toBeNull();
    expect(document.querySelector(".code-block")).toBeNull();
    expect(document.querySelector('[role="status"]')).toBeNull();

    unmount(component);
  });

  it("keeps the full source and shows an accessible status on failure", async () => {
    renderMermaidMock.mockRejectedValue(new Error("Parse error on line 2"));

    const component = mount(MermaidBlock, {
      target: document.body,
      props: { content: SOURCE },
    });

    await settle();

    expect(diagramSvg()).toBeNull();
    const status = document.querySelector('[role="status"]');
    expect(status).not.toBeNull();
    expect(status!.textContent).toMatch(/failed to render/i);
    // Full source, not a truncated preview.
    expect(sourceText()).toContain("flowchart LR");
    expect(sourceText()).toContain("A[Start] --> B[End]");

    unmount(component);
  });

  it("copies the exact source from the failure fallback", async () => {
    renderMermaidMock.mockRejectedValue(new Error("boom"));

    const component = mount(MermaidBlock, {
      target: document.body,
      props: { content: SOURCE },
    });

    await settle();

    const copyButton = document.querySelector<HTMLButtonElement>(
      'button.copy-btn[aria-label="Copy code block"]',
    );
    expect(copyButton).not.toBeNull();
    copyButton!.click();
    await settle();

    expect(copyToClipboardMock).toHaveBeenCalledWith(SOURCE);

    unmount(component);
  });

  it("copies the exact source from the rendered diagram", async () => {
    const component = mount(MermaidBlock, {
      target: document.body,
      props: { content: SOURCE },
    });

    await settle();

    const copyButton = document.querySelector<HTMLButtonElement>(
      'button.copy-btn[aria-label="Copy diagram source"]',
    );
    expect(copyButton).not.toBeNull();
    copyButton!.click();
    await settle();

    // Exact source, never the sanitized SVG or a trimmed variant.
    expect(copyToClipboardMock).toHaveBeenCalledWith(SOURCE);
    expect(copyButton!.getAttribute("aria-label")).toBe(
      "Copied diagram source",
    );
    // Copying must not tear the diagram down.
    expect(diagramSvg()).not.toBeNull();

    unmount(component);
  });

  it("does not claim success when the clipboard write fails", async () => {
    copyToClipboardMock.mockResolvedValueOnce(false);

    const component = mount(MermaidBlock, {
      target: document.body,
      props: { content: SOURCE },
    });

    await settle();

    const copyButton = document.querySelector<HTMLButtonElement>(
      'button.copy-btn[aria-label="Copy diagram source"]',
    );
    copyButton!.click();
    await settle();

    expect(copyButton!.getAttribute("aria-label")).toBe(
      "Copy diagram source",
    );

    unmount(component);
  });

  it("re-renders an already-mounted diagram when the theme preference flips", async () => {
    renderMermaidMock.mockImplementation(async (_source, theme) =>
      svgFor(theme),
    );

    const component = mount(MermaidBlock, {
      target: document.body,
      props: { content: SOURCE },
    });

    await settle();
    expect(renderMermaidMock).toHaveBeenCalledWith(SOURCE, "light");
    expect(diagramSvg()?.getAttribute("data-variant")).toBe("light");

    ui.setThemePreference("dark");
    await settle();

    expect(renderMermaidMock).toHaveBeenLastCalledWith(SOURCE, "dark");
    expect(diagramSvg()?.getAttribute("data-variant")).toBe("dark");

    unmount(component);
  });

  it("follows the OS scheme through the system preference", async () => {
    renderMermaidMock.mockImplementation(async (_source, theme) =>
      svgFor(theme),
    );
    ui.setThemePreference("system");
    ui.prefersDark = false;

    const component = mount(MermaidBlock, {
      target: document.body,
      props: { content: SOURCE },
    });

    await settle();
    expect(diagramSvg()?.getAttribute("data-variant")).toBe("light");

    ui.prefersDark = true;
    await settle();

    expect(diagramSvg()?.getAttribute("data-variant")).toBe("dark");

    ui.prefersDark = false;
    unmount(component);
  });

  it("discards a stale result once a newer request has been issued", async () => {
    const first = deferred<string>();
    const second = deferred<string>();
    renderMermaidMock
      .mockReturnValueOnce(first.promise)
      .mockReturnValueOnce(second.promise);

    const component = mount(MermaidBlock, {
      target: document.body,
      props: { content: SOURCE },
    });

    await settle();
    ui.setThemePreference("dark");
    await settle();
    expect(renderMermaidMock).toHaveBeenCalledTimes(2);

    // The newest request settles first, then the superseded one; the stale
    // result must not repaint the block.
    second.resolve(svgFor("dark"));
    await settle();
    first.resolve(svgFor("light"));
    await settle();

    expect(diagramSvg()?.getAttribute("data-variant")).toBe("dark");

    unmount(component);
  });

  it("does not touch the DOM after destroy", async () => {
    const pending = deferred<string>();
    renderMermaidMock.mockReturnValue(pending.promise);

    const component = mount(MermaidBlock, {
      target: document.body,
      props: { content: SOURCE },
    });

    await settle();
    unmount(component);
    const afterUnmount = document.body.innerHTML;

    pending.resolve(svgFor("late"));
    await settle();

    expect(document.body.innerHTML).toBe(afterUnmount);
    expect(document.querySelector("svg[data-variant='late']")).toBeNull();
  });

  it("recovers on a later attempt after a transient failure", async () => {
    renderMermaidMock.mockRejectedValueOnce(new Error("chunk load failed"));

    const component = mount(MermaidBlock, {
      target: document.body,
      props: { content: SOURCE },
    });

    await settle();
    expect(document.querySelector('[role="status"]')).not.toBeNull();
    unmount(component);

    renderMermaidMock.mockResolvedValue(svgFor("retry"));
    const second = mount(MermaidBlock, {
      target: document.body,
      props: { content: SOURCE },
    });

    await settle();
    expect(diagramSvg()?.getAttribute("data-variant")).toBe("retry");
    expect(document.querySelector('[role="status"]')).toBeNull();

    unmount(second);
  });
});

/**
 * Content-update lifecycle. `mount()` takes a plain props object, which Svelte
 * 5 does not make reactive, so these use `@testing-library/svelte`'s
 * `rerender()` — the only way to drive a *live* prop change here and the same
 * reason the theme tests use the real `UIStore` instead of a mock.
 */
describe("MermaidBlock content updates", () => {
  function deferred<T>() {
    let resolve!: (value: T) => void;
    const promise = new Promise<T>((res) => {
      resolve = res;
    });
    promise.catch(() => {});
    return { promise, resolve };
  }

  const NEXT_SOURCE = "flowchart TD\n  X[Xray] --> Y[Yankee]\n";

  afterEach(() => {
    cleanup();
  });

  it("re-renders when the content prop changes", async () => {
    renderMermaidMock.mockImplementation(async (source) =>
      svgFor(source.includes("Xray") ? "second" : "first"),
    );

    const { rerender } = render(MermaidBlock, { content: SOURCE });
    await settle();
    expect(diagramSvg()?.getAttribute("data-variant")).toBe("first");

    await rerender({ content: NEXT_SOURCE });
    await settle();

    expect(renderMermaidMock).toHaveBeenLastCalledWith(NEXT_SOURCE, "light");
    expect(diagramSvg()?.getAttribute("data-variant")).toBe("second");
  });

  it("never paints a diagram belonging to superseded content", async () => {
    const first = deferred<string>();
    const second = deferred<string>();
    renderMermaidMock
      .mockReturnValueOnce(first.promise)
      .mockReturnValueOnce(second.promise);

    const { rerender } = render(MermaidBlock, { content: SOURCE });
    await settle();

    await rerender({ content: NEXT_SOURCE });
    await settle();
    expect(renderMermaidMock).toHaveBeenCalledTimes(2);

    // Newest settles first, superseded one lands afterwards.
    second.resolve(svgFor("second"));
    await settle();
    first.resolve(svgFor("first"));
    await settle();

    expect(diagramSvg()?.getAttribute("data-variant")).toBe("second");
    expect(document.querySelectorAll(".mermaid-diagram")).toHaveLength(1);
  });

  it("falls back to the new source while the new render is still pending", async () => {
    const first = deferred<string>();
    const second = deferred<string>();
    renderMermaidMock
      .mockReturnValueOnce(first.promise)
      .mockReturnValueOnce(second.promise);

    const { rerender } = render(MermaidBlock, { content: SOURCE });
    first.resolve(svgFor("first"));
    await settle();
    expect(diagramSvg()).not.toBeNull();

    await rerender({ content: NEXT_SOURCE });
    await settle();

    // The old diagram must not linger over the new source: an outcome keyed to
    // the previous content is not shown for the current one.
    expect(diagramSvg()).toBeNull();
    expect(sourceText()).toContain("X[Xray] --> Y[Yankee]");
  });
});
