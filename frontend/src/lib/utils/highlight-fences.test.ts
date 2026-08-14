// @vitest-environment jsdom
import { afterEach, beforeEach, describe, it, expect, vi } from "vitest";
import { highlightCodeFences } from "./highlight-fences.js";
import { applyHighlight } from "./highlight.js";

/** Stands in for the shared Mermaid runtime so these tests never load the real
 *  one; jsdom has no SVG layout engine to render against. */
const renderMermaidMock = vi.hoisted(() =>
  vi.fn<(source: string, theme: string) => Promise<string>>(),
);

vi.mock("./mermaid.js", async () => {
  const actual = await vi.importActual<typeof import("./mermaid.js")>(
    "./mermaid.js",
  );
  return { ...actual, renderMermaid: renderMermaidMock };
});

function makeDiv(html: string): HTMLElement {
  const div = document.createElement("div");
  div.innerHTML = html;
  return div;
}

function marks(el: HTMLElement): string[] {
  return Array.from(el.querySelectorAll("mark.search-highlight")).map(
    (m) => m.textContent ?? "",
  );
}

function styledSpans(el: HTMLElement): HTMLSpanElement[] {
  return Array.from(el.querySelectorAll("span")).filter(
    (s) => s.style.color !== "",
  ) as HTMLSpanElement[];
}

function makeMarkdownCodeBlock(lang: string, code: string): string {
  const cls = lang ? ` class="language-${lang}"` : "";
  return `<pre><code${cls}>${code}\n</code></pre>`;
}

describe("highlightCodeFences", () => {
  describe("labeled fences (known language)", () => {
    it("swaps innerHTML of a language-ts code element with <span> tokens", async () => {
      const html = makeMarkdownCodeBlock("ts", "const x = 1;");
      const div = makeDiv(html);
      const action = highlightCodeFences(div, { content: "const x = 1;", theme: "light" });
      const codeEl = div.querySelector("code")!;
      expect(codeEl).not.toBeNull();

      try {
        await vi.waitFor(
          () => {
            if (!codeEl.innerHTML.includes("<span")) throw new Error("not yet");
          },
          { timeout: 10_000 },
        );
        expect(styledSpans(codeEl).length).toBeGreaterThanOrEqual(1);
        const colors = new Set(styledSpans(codeEl).map((s) => s.getAttribute("style")));
        expect(colors.size).toBeGreaterThanOrEqual(2);
      } finally {
        action.destroy();
      }
    });

    it("preserves textContent after the swap (copy still sees full code)", async () => {
      const code = "const greeting = 'hello';";
      const html = makeMarkdownCodeBlock("typescript", code);
      const div = makeDiv(html);
      const action = highlightCodeFences(div, { content: code, theme: "light" });
      const codeEl = div.querySelector("code")!;

      try {
        await vi.waitFor(
          () => {
            if (!codeEl.innerHTML.includes("<span")) throw new Error("not yet");
          },
          { timeout: 10_000 },
        );
        // Text content (what copy reads) must contain the original tokens.
        expect(codeEl.textContent).toContain("greeting");
        expect(codeEl.textContent).toContain("hello");
      } finally {
        action.destroy();
      }
    });

    it("highlights a language-javascript fence", async () => {
      const html = makeMarkdownCodeBlock("javascript", "var a = 1;");
      const div = makeDiv(html);
      const action = highlightCodeFences(div, { content: "var a = 1;", theme: "light" });
      const codeEl = div.querySelector("code")!;

      try {
        await vi.waitFor(
          () => {
            if (!codeEl.innerHTML.includes("<span")) throw new Error("not yet");
          },
          { timeout: 10_000 },
        );
        expect(styledSpans(codeEl).length).toBeGreaterThanOrEqual(1);
        const colors = new Set(styledSpans(codeEl).map((s) => s.getAttribute("style")));
        expect(colors.size).toBeGreaterThanOrEqual(2);
      } finally {
        action.destroy();
      }
    });
  });

  describe("unlabeled and unknown fences", () => {
    it("leaves an unlabeled <pre><code> element untouched", async () => {
      const original = "no lang\n";
      const html = `<pre><code>${original}</code></pre>`;
      const div = makeDiv(html);
      const action = highlightCodeFences(div, { content: original, theme: "light" });

      try {
        // Give any async work time to settle; nothing should change.
        await new Promise((r) => setTimeout(r, 50));
        const codeEl = div.querySelector("code")!;
        // innerHTML must not have been replaced with <span> tokens.
        expect(codeEl.innerHTML).not.toContain("<span");
        expect(codeEl.textContent).toBe(original);
      } finally {
        action.destroy();
      }
    });

    it("leaves a code element with a diff language tag untouched", async () => {
      const code = "-old line\n+new line\n";
      const html = makeMarkdownCodeBlock("diff", code);
      const div = makeDiv(html);
      const action = highlightCodeFences(div, { content: code, theme: "light" });

      try {
        // highlightToHtml returns null for diff (not in preloaded set); wait long
        // enough to confirm the null path does not mutate the DOM.
        await new Promise((r) => setTimeout(r, 200));
        const codeEl = div.querySelector("code")!;
        expect(codeEl.innerHTML).not.toContain("<span");
      } finally {
        action.destroy();
      }
    });
  });

  describe("stale-async guard", () => {
    it("does not apply a highlight from a previous content after content changes", async () => {
      // First render with typescript; immediately update to a different
      // content string before the first highlight resolves.
      const div = makeDiv(makeMarkdownCodeBlock("ts", "const x = 1;"));
      const action = highlightCodeFences(div, { content: "const x = 1;", theme: "light" });

      try {
        // Update with new content (empty, so no fences to highlight).
        div.innerHTML = "<p>plain text, no fences</p>";
        action.update({ content: "plain text, no fences", theme: "light" });

        // Wait well beyond the highlight resolve time to confirm no swap happens.
        await new Promise((r) => setTimeout(r, 500));

        // The first in-flight highlight should have been cancelled; the div
        // now has no code elements so no span swaps should have occurred.
        const codeEl = div.querySelector("code");
        expect(codeEl).toBeNull();
      } finally {
        action.destroy();
      }
    });
  });

  describe("search-highlight interplay", () => {
    it("re-applies search marks inside code after the Shiki swap", async () => {
      const code = "const foo = 1;";
      const html = makeMarkdownCodeBlock("typescript", code);
      const div = makeDiv(html);

      const fenceAction = highlightCodeFences(div, {
        content: code,
        q: "foo",
        current: false,
        theme: "light",
      });

      const codeEl = div.querySelector("code")!;

      try {
        await vi.waitFor(
          () => {
            if (!codeEl.innerHTML.includes("<span")) throw new Error("not yet");
          },
          { timeout: 10_000 },
        );
        expect(styledSpans(codeEl).length).toBeGreaterThanOrEqual(1);
        const codeMarks = Array.from(
          codeEl.querySelectorAll("mark.search-highlight"),
        ).map((m) => m.textContent ?? "");
        expect(codeMarks).toContain("foo");
      } finally {
        fenceAction.destroy();
      }
    });

    it("does not re-apply marks when no search query is active", async () => {
      const code = "const x = 1;";
      const html = makeMarkdownCodeBlock("ts", code);
      const div = makeDiv(html);
      const action = highlightCodeFences(div, { content: code, theme: "light" });
      const codeEl = div.querySelector("code")!;

      try {
        await vi.waitFor(
          () => {
            if (!codeEl.innerHTML.includes("<span")) throw new Error("not yet");
          },
          { timeout: 10_000 },
        );
        // Shiki ran, no marks expected (no query was given).
        expect(codeEl.querySelectorAll("mark.search-highlight")).toHaveLength(0);
      } finally {
        action.destroy();
      }
    });

    it("marks a query that crosses Shiki token boundaries", async () => {
      // "const foo" is split by Shiki into separate <span> tokens;
      // the cross-node applyMarks must still mark the full phrase.
      const code = "const foo = 1;";
      const html = makeMarkdownCodeBlock("typescript", code);
      const div = makeDiv(html);

      const fenceAction = highlightCodeFences(div, {
        content: code,
        q: "const foo",
        current: false,
        theme: "light",
      });

      const codeEl = div.querySelector("code")!;

      try {
        await vi.waitFor(
          () => {
            if (!codeEl.innerHTML.includes("<span")) throw new Error("not yet");
          },
          { timeout: 10_000 },
        );

        const codeMarks = Array.from(
          codeEl.querySelectorAll("mark.search-highlight"),
        );
        // The mark fragments across token boundaries must concatenate to the query.
        const combined = codeMarks.map((m) => m.textContent ?? "").join("");
        expect(combined).toBe("const foo");
      } finally {
        fenceAction.destroy();
      }
    });

    it("applyHighlight and highlightCodeFences co-applied on the same container", async () => {
      // Mirrors the real MessageContent.svelte call pattern: applyHighlight and
      // highlightCodeFences both mounted on the same <div>.
      const code = "const foo = 1;";
      const prose = "<p>search for foo here</p>";
      const fenceHtml = makeMarkdownCodeBlock("typescript", code);
      const content = prose + fenceHtml;
      const div = makeDiv(content);
      const codeEl = div.querySelector("code")!;

      const hlAction = applyHighlight(div, { q: "foo", current: false, content });
      const fenceAction = highlightCodeFences(div, {
        content,
        q: "foo",
        current: false,
        theme: "light",
      });

      try {
        // Wait for Shiki to swap code innerHTML.
        await vi.waitFor(
          () => {
            if (!codeEl.innerHTML.includes("<span")) throw new Error("not yet");
          },
          { timeout: 10_000 },
        );

        // Prose <p> must still have marks (Shiki only touched the code element).
        const proseEl = div.querySelector("p")!;
        expect(marks(proseEl)).toContain("foo");

        // Code element must have marks re-applied after the Shiki innerHTML swap.
        expect(marks(codeEl)).toContain("foo");

        // Call update on both actions (simulates Svelte re-rendering the content).
        hlAction.update({ q: "foo", current: false, content });
        fenceAction.update({ content, q: "foo", current: false, theme: "light" });

        // After update: applyHighlight re-clears and re-marks all text nodes;
        // highlightCodeFences re-runs the async highlight and re-applies marks.
        // Wait for the Shiki swap to settle again.
        await vi.waitFor(
          () => {
            if (!codeEl.innerHTML.includes("<span")) throw new Error("not yet");
          },
          { timeout: 10_000 },
        );

        // Both prose and code marks must be present after the update cycle.
        expect(marks(proseEl)).toContain("foo");
        expect(marks(codeEl)).toContain("foo");
      } finally {
        hlAction.update({ q: "", current: false, content }); // teardown applyHighlight
        fenceAction.destroy();
      }
    });
  });

  describe("destroy", () => {
    it("cancels in-flight highlights on destroy so stale swaps never occur", async () => {
      const code = "const x = 1;";
      const html = makeMarkdownCodeBlock("ts", code);
      const div = makeDiv(html);
      const originalInner = div.querySelector("code")!.innerHTML;

      const action = highlightCodeFences(div, { content: code, theme: "light" });
      // Destroy immediately — should cancel the pending highlight.
      action.destroy();

      // Wait beyond the expected highlight resolution time.
      await new Promise((r) => setTimeout(r, 500));

      const codeEl = div.querySelector("code")!;
      // innerHTML should still be the original plain text, not Shiki spans.
      expect(codeEl.innerHTML).toBe(originalInner);
    });
  });
});

describe("highlightCodeFences mermaid fences", () => {
  const SOURCE = "flowchart LR\n  A[Start] --> B[End]";

  function svgFor(label: string): string {
    return `<svg data-variant="${label}" xmlns="http://www.w3.org/2000/svg"><text>${label}</text></svg>`;
  }

  function deferred<T>() {
    let resolve!: (value: T) => void;
    let reject!: (err: unknown) => void;
    const promise = new Promise<T>((res, rej) => {
      resolve = res;
      reject = rej;
    });
    promise.catch(() => {});
    return { promise, resolve, reject };
  }

  function diagram(el: HTMLElement): SVGElement | null {
    return el.querySelector(".mermaid-fence svg");
  }

  async function waitForDiagram(el: HTMLElement) {
    await vi.waitFor(
      () => {
        if (!diagram(el)) throw new Error("not yet");
      },
      { timeout: 5_000 },
    );
  }

  beforeEach(() => {
    renderMermaidMock.mockReset();
    renderMermaidMock.mockImplementation(async (_source, theme) =>
      svgFor(theme),
    );
  });

  afterEach(() => {
    vi.clearAllMocks();
  });

  it.each(["mermaid", "Mermaid", "MERMAID"])(
    "upgrades a language-%s fence to a sanitized diagram",
    async (lang) => {
      const div = makeDiv(makeMarkdownCodeBlock(lang, SOURCE));
      const action = highlightCodeFences(div, {
        content: SOURCE,
        theme: "light",
      });

      try {
        await waitForDiagram(div);
        expect(renderMermaidMock).toHaveBeenCalledTimes(1);
        expect(renderMermaidMock.mock.calls[0]![0]).toContain(
          "A[Start] --> B[End]",
        );
        // Source is retained (not destroyed) but no longer shown.
        const pre = div.querySelector("pre")!;
        expect(pre).not.toBeNull();
        expect(pre.hidden).toBe(true);
        expect(pre.textContent).toContain("flowchart LR");
        expect(div.querySelector('[role="status"]')).toBeNull();
      } finally {
        action.destroy();
      }
    },
  );

  it("keeps the source visible and shows a status when the render fails", async () => {
    renderMermaidMock.mockRejectedValue(new Error("Parse error on line 1"));
    const div = makeDiv(makeMarkdownCodeBlock("mermaid", SOURCE));
    const action = highlightCodeFences(div, {
      content: SOURCE,
      theme: "light",
    });

    try {
      await vi.waitFor(
        () => {
          if (!div.querySelector('[role="status"]')) throw new Error("not yet");
        },
        { timeout: 5_000 },
      );

      const pre = div.querySelector("pre")!;
      expect(pre.hidden).toBe(false);
      expect(pre.textContent).toContain("A[Start] --> B[End]");
      expect(diagram(div)).toBeNull();
      expect(div.querySelector('[role="status"]')!.textContent).toMatch(
        /failed to render/i,
      );
    } finally {
      action.destroy();
    }
  });

  it("leaves a Mermaid fence as searchable source while a query is active", async () => {
    const div = makeDiv(makeMarkdownCodeBlock("mermaid", SOURCE));
    const action = highlightCodeFences(div, {
      content: SOURCE,
      q: "Start",
      current: false,
      theme: "light",
    });

    try {
      await new Promise((r) => setTimeout(r, 100));
      expect(renderMermaidMock).not.toHaveBeenCalled();
      expect(diagram(div)).toBeNull();
      const pre = div.querySelector("pre")!;
      expect(pre.hidden).toBe(false);
      expect(pre.textContent).toContain("A[Start] --> B[End]");
    } finally {
      action.destroy();
    }
  });

  it("re-renders on a theme update and ignores the superseded result", async () => {
    const first = deferred<string>();
    const second = deferred<string>();
    renderMermaidMock
      .mockReturnValueOnce(first.promise)
      .mockReturnValueOnce(second.promise);

    const div = makeDiv(makeMarkdownCodeBlock("mermaid", SOURCE));
    const action = highlightCodeFences(div, {
      content: SOURCE,
      theme: "light",
    });

    try {
      action.update({ content: SOURCE, theme: "dark" });
      expect(renderMermaidMock).toHaveBeenCalledTimes(2);
      expect(renderMermaidMock.mock.calls[1]![1]).toBe("dark");

      second.resolve(svgFor("dark"));
      await waitForDiagram(div);
      expect(diagram(div)!.getAttribute("data-variant")).toBe("dark");

      // The stale light render lands afterwards and must be ignored.
      first.resolve(svgFor("light"));
      await new Promise((r) => setTimeout(r, 50));
      expect(diagram(div)!.getAttribute("data-variant")).toBe("dark");
    } finally {
      action.destroy();
    }
  });

  it("does not mutate the DOM after destroy", async () => {
    const pending = deferred<string>();
    renderMermaidMock.mockReturnValue(pending.promise);

    const div = makeDiv(makeMarkdownCodeBlock("mermaid", SOURCE));
    const action = highlightCodeFences(div, {
      content: SOURCE,
      theme: "light",
    });
    action.destroy();
    const snapshot = div.innerHTML;

    pending.resolve(svgFor("late"));
    await new Promise((r) => setTimeout(r, 100));

    expect(div.innerHTML).toBe(snapshot);
    expect(diagram(div)).toBeNull();
  });

  it("renders a Mermaid fence and a Shiki fence in the same container", async () => {
    const code = "const foo = 1;";
    const html =
      makeMarkdownCodeBlock("typescript", code) +
      makeMarkdownCodeBlock("mermaid", SOURCE);
    const div = makeDiv(html);
    const action = highlightCodeFences(div, {
      content: html,
      theme: "light",
    });

    try {
      await waitForDiagram(div);
      const codeEl = div.querySelector("code.language-typescript")!;
      await vi.waitFor(
        () => {
          if (!codeEl.innerHTML.includes("<span")) throw new Error("not yet");
        },
        { timeout: 10_000 },
      );

      expect(styledSpans(codeEl as HTMLElement).length).toBeGreaterThanOrEqual(1);
      expect(codeEl.textContent).toContain("foo");
      expect(diagram(div)).not.toBeNull();
    } finally {
      action.destroy();
    }
  });

  it("leaves a failed Mermaid fence from poisoning neighbouring Shiki fences", async () => {
    renderMermaidMock.mockRejectedValue(new Error("boom"));
    const code = "const bar = 2;";
    const html =
      makeMarkdownCodeBlock("mermaid", SOURCE) +
      makeMarkdownCodeBlock("typescript", code);
    const div = makeDiv(html);
    const action = highlightCodeFences(div, {
      content: html,
      theme: "light",
    });

    try {
      const codeEl = div.querySelector("code.language-typescript")!;
      await vi.waitFor(
        () => {
          if (!codeEl.innerHTML.includes("<span")) throw new Error("not yet");
        },
        { timeout: 10_000 },
      );
      expect(div.querySelector('[role="status"]')).not.toBeNull();
      expect(codeEl.textContent).toContain("bar");
    } finally {
      action.destroy();
    }
  });
});

describe("highlightCodeFences mermaid state transitions", () => {
  const SOURCE = "flowchart LR\n  A[Start] --> B[End]";

  function svgFor(label: string): string {
    return `<svg data-variant="${label}" xmlns="http://www.w3.org/2000/svg"><text>${label}</text></svg>`;
  }

  function deferred<T>() {
    let resolve!: (value: T) => void;
    let reject!: (err: unknown) => void;
    const promise = new Promise<T>((res, rej) => {
      resolve = res;
      reject = rej;
    });
    promise.catch(() => {});
    return { promise, resolve, reject };
  }

  function diagram(el: HTMLElement): SVGElement | null {
    return el.querySelector(".mermaid-fence svg");
  }

  async function waitForDiagram(el: HTMLElement) {
    await vi.waitFor(
      () => {
        if (!diagram(el)) throw new Error("not yet");
      },
      { timeout: 5_000 },
    );
  }

  beforeEach(() => {
    renderMermaidMock.mockReset();
    renderMermaidMock.mockImplementation(async (_source, theme) =>
      svgFor(theme),
    );
  });

  afterEach(() => {
    vi.clearAllMocks();
  });

  it("restores the source when a search query arrives after a diagram rendered", async () => {
    const div = makeDiv(makeMarkdownCodeBlock("mermaid", SOURCE));
    const action = highlightCodeFences(div, {
      content: SOURCE,
      theme: "light",
    });

    try {
      await waitForDiagram(div);
      const pre = div.querySelector("pre")!;
      expect(pre.hidden).toBe(true);

      // Browse -> search. The DOM is NOT rebuilt on this transition, so the
      // action has to actively undo the diagram it put up earlier.
      action.update({ content: SOURCE, q: "Start", current: false, theme: "light" });

      expect(diagram(div)).toBeNull();
      expect(pre.hidden).toBe(false);
      expect(pre.textContent).toContain("A[Start] --> B[End]");
      expect(div.querySelector('[role="status"]')).toBeNull();

      // Search -> browse restores the diagram.
      action.update({ content: SOURCE, q: "", current: false, theme: "light" });
      await waitForDiagram(div);
      expect(div.querySelector("pre")!.hidden).toBe(true);
    } finally {
      action.destroy();
    }
  });

  it("clears a failure status when a search query arrives", async () => {
    renderMermaidMock.mockRejectedValue(new Error("Parse error on line 1"));
    const div = makeDiv(makeMarkdownCodeBlock("mermaid", SOURCE));
    const action = highlightCodeFences(div, {
      content: SOURCE,
      theme: "light",
    });

    try {
      await vi.waitFor(
        () => {
          if (!div.querySelector('[role="status"]')) throw new Error("not yet");
        },
        { timeout: 5_000 },
      );

      action.update({ content: SOURCE, q: "Start", current: false, theme: "light" });

      expect(div.querySelector('[role="status"]')).toBeNull();
      expect(div.querySelector("pre")!.hidden).toBe(false);
    } finally {
      action.destroy();
    }
  });

  it("discards a stale result after the fence content changed", async () => {
    const first = deferred<string>();
    const second = deferred<string>();
    renderMermaidMock
      .mockReturnValueOnce(first.promise)
      .mockReturnValueOnce(second.promise);

    const div = makeDiv(makeMarkdownCodeBlock("mermaid", SOURCE));
    const action = highlightCodeFences(div, {
      content: SOURCE,
      theme: "light",
    });

    try {
      // Svelte replaces the {@html} children when `content` changes; mirror
      // that, then re-run the action with the new source.
      const NEXT = "flowchart TD\n  X[Xray] --> Y[Yankee]";
      div.innerHTML = makeMarkdownCodeBlock("mermaid", NEXT);
      action.update({ content: NEXT, theme: "light" });

      expect(renderMermaidMock).toHaveBeenCalledTimes(2);
      expect(renderMermaidMock.mock.calls[1]![0]).toContain("X[Xray]");

      second.resolve(svgFor("second"));
      await waitForDiagram(div);
      expect(diagram(div)!.getAttribute("data-variant")).toBe("second");

      // The superseded render for the OLD source lands last and must be
      // ignored, or the block shows a diagram of content that is gone.
      first.resolve(svgFor("first"));
      await new Promise((r) => setTimeout(r, 50));

      expect(diagram(div)!.getAttribute("data-variant")).toBe("second");
      expect(div.querySelectorAll(".mermaid-fence")).toHaveLength(1);
    } finally {
      action.destroy();
    }
  });
});
