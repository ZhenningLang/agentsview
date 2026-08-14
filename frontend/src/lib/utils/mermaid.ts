import DOMPurify from "dompurify";

/** Resolved app theme; `ui.theme` is the single source of truth. */
export type MermaidTheme = "light" | "dark";

/** Official Mermaid defaults, pinned here so a runtime upgrade cannot silently
 *  widen them. */
export const MERMAID_MAX_TEXT_SIZE = 50_000;
export const MERMAID_MAX_EDGES = 500;

/**
 * Config keys a diagram's own frontmatter/directives must never override.
 *
 * Mermaid merges a diagram's YAML frontmatter into the effective config for
 * every key not listed here, so this list is the actual trust boundary — the
 * app-owned SVG sanitize pass runs afterwards and cannot undo a config choice.
 * Beyond Mermaid's own default `secure` set this locks the theming keys,
 * because `ui.theme` is the single source of truth and `themeCSS` /
 * `themeVariables` are arbitrary CSS injected into the diagram's `<style>`.
 */
export const MERMAID_SECURE_KEYS = [
  "secure",
  "securityLevel",
  "startOnLoad",
  "maxTextSize",
  "maxEdges",
  "suppressErrorRendering",
  "htmlLabels",
  "theme",
  "themeCSS",
  "themeVariables",
  "darkMode",
  "fontFamily",
  "altFontFamily",
] as const;

/** The slice of the Mermaid API this module uses. Kept structural so tests can
 *  inject a fake without pulling the real runtime into the test process. */
export interface MermaidApi {
  initialize(config: Record<string, unknown>): void;
  render(id: string, source: string): Promise<{ svg: string }>;
}

export type MermaidLoader = () => Promise<MermaidApi>;

/** Production loader. The `import()` literal is what makes Vite emit Mermaid as
 *  an async chunk instead of folding it into the entry closure — do not turn it
 *  into a static or template-literal import. */
const defaultLoader: MermaidLoader = async () => {
  const mod = await import("mermaid");
  return (mod.default ?? mod) as unknown as MermaidApi;
};

let loader: MermaidLoader = defaultLoader;
let apiPromise: Promise<MermaidApi> | null = null;

/** Serializes `initialize()` + `render()` pairs. Mermaid's config is global
 *  singleton state, so interleaving two blocks would let one diagram render
 *  with the other's theme. */
let queue: Promise<unknown> = Promise.resolve();

let idCounter = 0;

/** Replace the runtime loader. Passing `null` restores the dynamic import.
 *  Tests use this to count loads and to drive failure paths. */
export function setMermaidLoader(next: MermaidLoader | null): void {
  loader = next ?? defaultLoader;
  apiPromise = null;
}

/** Drop the cached module promise. Tests call this between cases; production
 *  code never needs it. */
export function resetMermaidRuntime(): void {
  apiPromise = null;
  queue = Promise.resolve();
}

/** `true` for a fenced-code label that should render as a diagram. */
export function isMermaidLabel(label: string | undefined | null): boolean {
  return (label ?? "").trim().toLowerCase() === "mermaid";
}

/** The static trusted config handed to `initialize()`. Diagram source never
 *  reaches this object. */
export function trustedMermaidConfig(
  theme: MermaidTheme,
): Record<string, unknown> {
  const dark = theme === "dark";
  return {
    startOnLoad: false,
    securityLevel: "strict",
    suppressErrorRendering: true,
    maxTextSize: MERMAID_MAX_TEXT_SIZE,
    maxEdges: MERMAID_MAX_EDGES,
    theme: dark ? "dark" : "default",
    darkMode: dark,
    // Mermaid's default routes flowchart/class/state labels through
    // `addHtmlSpan`, which emits `<foreignObject><div><span>`. DOMPurify lists
    // `foreignobject` in both svgDisallowed AND DEFAULT_FORBID_CONTENTS, so the
    // app's sanitize pass deletes the element *and its children* — the diagram
    // keeps its shapes and loses every label. Native `<text>`/`<tspan>` labels
    // survive sanitization, so this tightens rather than relaxes the boundary.
    htmlLabels: false,
    secure: [...MERMAID_SECURE_KEYS],
  };
}

/** Coalesced module load. A rejected load resets the cache so a later block can
 *  retry after a transient chunk failure. */
function loadMermaid(): Promise<MermaidApi> {
  if (apiPromise !== null) return apiPromise;

  apiPromise = loader().catch((err) => {
    apiPromise = null;
    throw err;
  });

  return apiPromise;
}

/** Append a unit of work to the serial queue. A rejected unit must not poison
 *  the queue for later blocks. */
function enqueue<T>(task: () => Promise<T>): Promise<T> {
  const run = queue.then(task, task);
  queue = run.then(
    () => undefined,
    () => undefined,
  );
  return run;
}

const SVG_NAMESPACE = "http://www.w3.org/2000/svg";

/**
 * App-owned second sanitization pass over Mermaid's SVG output.
 *
 * Mermaid runs its own DOMPurify pass at `securityLevel: "strict"`, but a
 * third-party string is not trusted just because the library says so — this is
 * the same gate `markdown.ts` applies to rendered Markdown, with no `ADD_TAGS`,
 * `ADD_ATTR`, hooks, `IN_PLACE` or `CUSTOM_ELEMENT_HANDLING` relaxation.
 *
 * The result must be exactly one SVG-namespace `<svg>` element, and only that
 * element's serialization is returned. Checking the first child and returning
 * the whole sanitized string is not equivalent: a trailing `<style>` survives
 * sanitization and its rules apply to the entire document, so a diagram could
 * restyle or hide unrelated page elements. Anything else throws, so callers keep
 * showing the source instead of inserting an arbitrary string.
 */
function sanitizeSvg(svg: string): string {
  const clean = DOMPurify.sanitize(svg);

  const probe = document.createElement("div");
  probe.innerHTML = clean;

  if (probe.children.length > 1) {
    throw new Error(
      "mermaid: sanitized output must be a single <svg> root " +
        `(got ${probe.children.length} top-level elements)`,
    );
  }

  const root = probe.firstElementChild;
  if (
    !root ||
    root.namespaceURI !== SVG_NAMESPACE ||
    root.tagName.toLowerCase() !== "svg"
  ) {
    throw new Error("mermaid: sanitized output has no <svg> root");
  }

  return root.outerHTML;
}

/**
 * Render `source` to a sanitized SVG string for the given resolved theme.
 *
 * Rejects on a missing DOM, a failed runtime load, a Mermaid parse/limit error
 * or an SVG that does not survive sanitization. Callers must keep the original
 * source visible until this resolves.
 */
export function renderMermaid(
  source: string,
  theme: MermaidTheme,
): Promise<string> {
  if (typeof document === "undefined") {
    return Promise.reject(
      new Error("mermaid: rendering requires a DOM document"),
    );
  }

  const id = `mermaid-diagram-${++idCounter}`;

  return enqueue(async () => {
    const api = await loadMermaid();
    api.initialize(trustedMermaidConfig(theme));
    const { svg } = await api.render(id, source);
    return sanitizeSvg(svg);
  });
}
