// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  MERMAID_MAX_EDGES,
  MERMAID_MAX_TEXT_SIZE,
  MERMAID_SECURE_KEYS,
  isMermaidLabel,
  renderMermaid,
  resetMermaidRuntime,
  setMermaidLoader,
  trustedMermaidConfig,
  type MermaidApi,
} from "./mermaid.js";

const SAFE_SVG =
  '<svg xmlns="http://www.w3.org/2000/svg" width="120" height="40">' +
  '<g class="node"><rect width="100" height="30"></rect>' +
  "<text>Start</text></g></svg>";

interface FakeApi extends MermaidApi {
  calls: string[];
  configs: Record<string, unknown>[];
  rendered: string[];
}

/** Fake Mermaid runtime that records the exact interleaving of
 *  initialize()/render() so the serial-queue contract is observable. */
function makeFakeApi(
  render: (id: string, source: string) => Promise<{ svg: string }> = async () => ({
    svg: SAFE_SVG,
  }),
): FakeApi {
  const api: FakeApi = {
    calls: [],
    configs: [],
    rendered: [],
    initialize(config) {
      api.calls.push(`init:${String(config.theme)}`);
      api.configs.push(config);
    },
    async render(id, source) {
      api.calls.push(`render:${source.trim()}`);
      api.rendered.push(id);
      return render(id, source);
    },
  };
  return api;
}

/** A promise plus its resolvers, for driving deterministic interleavings. */
function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (err: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

beforeEach(() => {
  resetMermaidRuntime();
});

afterEach(() => {
  setMermaidLoader(null);
  resetMermaidRuntime();
  vi.restoreAllMocks();
});

describe("isMermaidLabel", () => {
  it.each(["mermaid", "Mermaid", "MERMAID", " mermaid ", "\tmermaid\n"])(
    "accepts %j",
    (label) => {
      expect(isMermaidLabel(label)).toBe(true);
    },
  );

  it.each(["", " ", "typescript", "mermaid-js", "mer maid", undefined, null])(
    "rejects %j",
    (label) => {
      expect(isMermaidLabel(label)).toBe(false);
    },
  );
});

describe("trustedMermaidConfig", () => {
  it("pins the strict security posture and official limits for light", () => {
    const config = trustedMermaidConfig("light");
    expect(config).toMatchObject({
      startOnLoad: false,
      securityLevel: "strict",
      suppressErrorRendering: true,
      maxTextSize: MERMAID_MAX_TEXT_SIZE,
      maxEdges: MERMAID_MAX_EDGES,
      theme: "default",
      darkMode: false,
    });
    expect(config.secure).toEqual([...MERMAID_SECURE_KEYS]);
    expect(MERMAID_MAX_TEXT_SIZE).toBe(50_000);
    expect(MERMAID_MAX_EDGES).toBe(500);
  });

  it("maps dark to the official dark theme", () => {
    expect(trustedMermaidConfig("dark")).toMatchObject({
      theme: "dark",
      darkMode: true,
      securityLevel: "strict",
    });
  });

  it("returns a fresh object so a caller cannot mutate the shared config", () => {
    const a = trustedMermaidConfig("light");
    (a as { securityLevel: string }).securityLevel = "loose";
    expect(trustedMermaidConfig("light").securityLevel).toBe("strict");
  });
});

describe("runtime loading", () => {
  it("does not touch the loader at module import time", async () => {
    // The module is already imported by this file; a loader installed now must
    // still see zero calls until a render is actually requested.
    const loader = vi.fn(async () => makeFakeApi());
    setMermaidLoader(loader);
    await Promise.resolve();
    expect(loader).not.toHaveBeenCalled();
  });

  it("coalesces concurrent renders onto a single module load", async () => {
    const api = makeFakeApi();
    const loader = vi.fn(async () => api);
    setMermaidLoader(loader);

    await Promise.all([
      renderMermaid("graph TD; A-->B", "light"),
      renderMermaid("graph TD; C-->D", "light"),
      renderMermaid("graph TD; E-->F", "light"),
    ]);

    expect(loader).toHaveBeenCalledTimes(1);
    expect(api.rendered).toHaveLength(3);
  });

  it("retries after a failed load instead of caching the rejection", async () => {
    const api = makeFakeApi();
    const loader = vi
      .fn<() => Promise<MermaidApi>>()
      .mockRejectedValueOnce(new Error("chunk load failed"))
      .mockResolvedValue(api);
    setMermaidLoader(loader);

    await expect(renderMermaid("graph TD; A-->B", "light")).rejects.toThrow(
      "chunk load failed",
    );
    await expect(
      renderMermaid("graph TD; A-->B", "light"),
    ).resolves.toContain("<svg");
    expect(loader).toHaveBeenCalledTimes(2);
  });

  it("gives every diagram a unique render id", async () => {
    const api = makeFakeApi();
    setMermaidLoader(async () => api);

    await Promise.all([
      renderMermaid("graph TD; A-->B", "light"),
      renderMermaid("graph TD; C-->D", "light"),
    ]);

    expect(new Set(api.rendered).size).toBe(api.rendered.length);
  });
});

describe("serial theme queue", () => {
  it("never interleaves initialize and render across concurrent blocks", async () => {
    const gate = deferred<void>();
    let first = true;
    const api = makeFakeApi(async () => {
      if (first) {
        first = false;
        await gate.promise;
      }
      return { svg: SAFE_SVG };
    });
    setMermaidLoader(async () => api);

    const light = renderMermaid("A", "light");
    const dark = renderMermaid("B", "dark");

    // Release the first render only after both requests are queued; if the
    // queue were not serial the second initialize would already have landed.
    await new Promise((r) => setTimeout(r, 0));
    expect(api.calls).toEqual(["init:default", "render:A"]);

    gate.resolve();
    await Promise.all([light, dark]);

    expect(api.calls).toEqual([
      "init:default",
      "render:A",
      "init:dark",
      "render:B",
    ]);
  });

  it("keeps serving later blocks after one render rejects", async () => {
    const api = makeFakeApi(async (_id, source) => {
      if (source === "bad") throw new Error("Parse error on line 1");
      return { svg: SAFE_SVG };
    });
    setMermaidLoader(async () => api);

    const bad = renderMermaid("bad", "light");
    const good = renderMermaid("good", "light");

    await expect(bad).rejects.toThrow("Parse error");
    await expect(good).resolves.toContain("<svg");
    expect(api.calls).toEqual([
      "init:default",
      "render:bad",
      "init:default",
      "render:good",
    ]);
  });

  it("surfaces a theme switch as a fresh initialize", async () => {
    const api = makeFakeApi();
    setMermaidLoader(async () => api);

    await renderMermaid("A", "light");
    await renderMermaid("A", "dark");

    expect(api.configs.map((c) => c.theme)).toEqual(["default", "dark"]);
    expect(api.configs.map((c) => c.darkMode)).toEqual([false, true]);
  });
});

describe("app-owned SVG sanitization", () => {
  async function renderWith(svg: string): Promise<string> {
    setMermaidLoader(async () => makeFakeApi(async () => ({ svg })));
    return renderMermaid("graph TD; A-->B", "light");
  }

  it("strips <script> smuggled into the diagram output", async () => {
    const out = await renderWith(
      '<svg xmlns="http://www.w3.org/2000/svg">' +
        "<script>window.__pwned = 1</script><text>Start</text></svg>",
    );

    expect(out).not.toContain("<script");
    expect(out).not.toContain("__pwned");
    expect(out).toContain("Start");
  });

  it("strips inline event handlers", async () => {
    const out = await renderWith(
      '<svg xmlns="http://www.w3.org/2000/svg">' +
        '<rect onload="alert(1)" onclick="alert(2)" width="10"></rect>' +
        "</svg>",
    );

    expect(out).not.toContain("onload");
    expect(out).not.toContain("onclick");
    expect(out).not.toContain("alert(");
    expect(out).toContain("<rect");
  });

  it("strips javascript: URLs from links", async () => {
    const out = await renderWith(
      '<svg xmlns="http://www.w3.org/2000/svg">' +
        '<a href="javascript:alert(1)"><text>click</text></a>' +
        '<a href="https://example.com/ok"><text>safe</text></a></svg>',
    );

    expect(out.toLowerCase()).not.toContain("javascript:");
    expect(out).toContain("https://example.com/ok");
  });

  it("keeps the markup a valid diagram actually needs", async () => {
    const out = await renderWith(
      '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 120 40">' +
        "<style>.node rect { fill: #eee; }</style>" +
        '<g class="node"><rect width="100" height="30" rx="4"></rect>' +
        '<text x="10" y="20">Start</text></g>' +
        '<path d="M0 0 L10 10" marker-end="url(#arrow)"></path>' +
        "</svg>",
    );

    expect(out).toContain("viewBox");
    expect(out).toContain("<style");
    expect(out).toContain("<path");
    expect(out).toContain("marker-end");
    expect(out).toContain("Start");
  });

  it("rejects output that is not an <svg> root instead of inserting it", async () => {
    await expect(renderWith("<div>not a diagram</div>")).rejects.toThrow(
      /no <svg> root/,
    );
    await expect(renderWith("")).rejects.toThrow(/no <svg> root/);
    await expect(renderWith("<script>alert(1)</script>")).rejects.toThrow(
      /no <svg> root/,
    );
  });
});

describe("config trust boundary", () => {
  it("never forwards diagram source into the config object", async () => {
    const api = makeFakeApi();
    setMermaidLoader(async () => api);

    const source =
      "---\nconfig:\n  securityLevel: loose\n---\nflowchart LR\n A-->B";
    await renderMermaid(source, "light");

    const config = api.configs[0]!;
    expect(config.securityLevel).toBe("strict");
    expect(JSON.stringify(config)).not.toContain("flowchart");
    expect(JSON.stringify(config)).not.toContain("loose");
  });

  it("does not call bindFunctions on the render result", async () => {
    const bindFunctions = vi.fn();
    setMermaidLoader(async () =>
      makeFakeApi(async () => ({ svg: SAFE_SVG, bindFunctions })),
    );

    await renderMermaid("graph TD; A-->B", "light");
    expect(bindFunctions).not.toHaveBeenCalled();
  });
});

describe("failure reporting", () => {
  it("propagates a Mermaid syntax error to the caller", async () => {
    setMermaidLoader(async () =>
      makeFakeApi(async () => {
        throw new Error("Parse error on line 2: expected NEWLINE");
      }),
    );

    await expect(renderMermaid("not a diagram", "light")).rejects.toThrow(
      /Parse error/,
    );
  });

  it("propagates a chunk load failure to the caller", async () => {
    setMermaidLoader(async () => {
      throw new Error("Failed to fetch dynamically imported module");
    });

    await expect(renderMermaid("graph TD; A-->B", "light")).rejects.toThrow(
      /dynamically imported module/,
    );
  });
});

/**
 * Sanitizer-vs-runtime shape contract.
 *
 * These exist because a hand-authored `<text>`-only fixture is exactly what let
 * a real defect through: with Mermaid's default `htmlLabels: true`, flowchart /
 * class / state labels are emitted as `<foreignObject><div><span>`, DOMPurify
 * lists `foreignobject` in both svgDisallowed and DEFAULT_FORBID_CONTENTS, and
 * every label was deleted while every fixture-based assertion stayed green.
 * The authoritative oracle is the browser spec that reads real runtime output;
 * these are the unit-level tripwires for the same regression.
 */
describe("sanitizer vs real runtime output shape", () => {
  async function renderWith(svg: string): Promise<string> {
    setMermaidLoader(async () => makeFakeApi(async () => ({ svg })));
    return renderMermaid("graph TD; A-->B", "light");
  }

  it("pins htmlLabels false so labels are emitted as native SVG text", () => {
    for (const theme of ["light", "dark"] as const) {
      expect(trustedMermaidConfig(theme).htmlLabels).toBe(false);
    }
    // Locked, so a diagram's own frontmatter cannot turn it back on.
    expect(MERMAID_SECURE_KEYS).toContain("htmlLabels");
  });

  it("locks every config key that owns theming or injects CSS", () => {
    for (const key of [
      "theme",
      "themeCSS",
      "themeVariables",
      "darkMode",
      "fontFamily",
      "altFontFamily",
    ]) {
      expect(MERMAID_SECURE_KEYS).toContain(key);
    }
  });

  it("drops a foreignObject label subtree entirely, contents included", async () => {
    // This is the markup `htmlLabels: true` produces. KEEP_CONTENT does not
    // rescue it, so the label text does not survive in any form.
    const out = await renderWith(
      '<svg xmlns="http://www.w3.org/2000/svg">' +
        '<g class="node"><rect width="60" height="30"></rect>' +
        '<foreignObject width="52" height="24">' +
        '<div xmlns="http://www.w3.org/1999/xhtml">' +
        '<span class="nodeLabel"><p>Alpha</p></span></div>' +
        "</foreignObject></g></svg>",
    );

    expect(out.toLowerCase()).not.toContain("foreignobject");
    expect(out).not.toContain("Alpha");
    expect(out).toContain("<rect");
  });

  it("keeps a native text/tspan label subtree, which is what the runtime emits", async () => {
    // Shaped after real mermaid@11.16.1 flowchart output at htmlLabels: false.
    const out = await renderWith(
      '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 284 70">' +
        "<style>#m .nodeLabel{fill:#333;}</style>" +
        '<g class="root"><g class="nodes">' +
        '<g class="node default" id="flowchart-A-0" transform="translate(30,35)">' +
        '<rect class="basic label-container" width="60" height="34" rx="0"></rect>' +
        '<g class="label" transform="translate(-15,-12)">' +
        '<foreignObject width="0" height="0"></foreignObject>' +
        '<text class="nodeLabel"><tspan class="text-outer-tspan">' +
        '<tspan class="text-inner-tspan">Alpha</tspan></tspan></text>' +
        "</g></g></g>" +
        '<g class="edgePaths"><path class="edge-thickness-normal" ' +
        'd="M60,35L120,35" marker-end="url(#arrowhead)"></path></g>' +
        "</g></svg>",
    );

    expect(out).toContain("Alpha");
    expect(out).toContain("<tspan");
    expect(out).toContain("nodeLabel");
    expect(out).toContain("viewBox");
    expect(out).toContain("marker-end");
    expect(out).toContain("<style");
  });
});

/**
 * Single-root contract.
 *
 * DOMPurify keeps a `<style>` element, and CSS in an inserted `<style>` applies
 * to the whole document — not to the diagram it arrived with. So "the first
 * element is an `<svg>`" is not a trust boundary: anything after that root would
 * ride along into the DOM. Only the root's own serialization is returned.
 */
describe("single-root trust boundary", () => {
  async function renderWith(svg: string): Promise<string> {
    setMermaidLoader(async () => makeFakeApi(async () => ({ svg })));
    return renderMermaid("graph TD; A-->B", "light");
  }

  const VALID = '<svg xmlns="http://www.w3.org/2000/svg"><text>ok</text></svg>';

  it("rejects a page-global <style> appended after a valid SVG", async () => {
    await expect(
      renderWith(`${VALID}<style>#victim{visibility:hidden}</style>`),
    ).rejects.toThrow(/single <svg> root/);
  });

  it("rejects any element appended after a valid SVG", async () => {
    await expect(renderWith(`${VALID}<div>tacked on</div>`)).rejects.toThrow(
      /single <svg> root/,
    );
    await expect(renderWith(`${VALID}<p>tacked on</p>`)).rejects.toThrow(
      /single <svg> root/,
    );
  });

  it("rejects multiple SVG roots", async () => {
    await expect(renderWith(`${VALID}${VALID}`)).rejects.toThrow(
      /single <svg> root/,
    );
  });

  it("rejects an element that merely looks like an SVG root", async () => {
    // An HTML-namespace element named "svg" is not an SVG root. Wrapping it in
    // <p> keeps the HTML parser from adopting it into the SVG namespace.
    await expect(renderWith("<p><svg><text>nope</text></svg></p>")).rejects.toThrow(
      /no <svg> root/,
    );
  });

  it("returns only the root serialization, never trailing content", async () => {
    const out = await renderWith(`  ${VALID}\n  <!-- trailing comment -->\n`);
    expect(out.startsWith("<svg")).toBe(true);
    expect(out.endsWith("</svg>")).toBe(true);
    expect(out).not.toContain("trailing comment");

    // And what comes back must insert as exactly one element.
    const host = document.createElement("div");
    host.innerHTML = out;
    expect(host.children).toHaveLength(1);
    expect(host.firstElementChild!.namespaceURI).toBe(
      "http://www.w3.org/2000/svg",
    );
  });
});
