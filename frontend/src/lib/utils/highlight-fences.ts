import { highlightToHtml } from "./syntax-highlight.js";
import { applyMarks } from "./highlight.js";
import { isMermaidLabel, renderMermaid, type MermaidTheme } from "./mermaid.js";

export interface HighlightCodeFencesParams {
  /** Markdown source rendered via {@html}; passing it makes Svelte call
   * update() when {@html} replaces the DOM children. */
  q?: string;
  content: string;
  current?: boolean;
  /** Resolved app theme, forwarded to the shared Mermaid runtime so an already
   * rendered diagram follows a light/dark switch. Required so a new
   * `renderMarkdown` surface cannot silently fall back to a hard-coded theme —
   * a missed callsite has to be a type error, not a wrong-looking diagram. */
  theme: MermaidTheme;
}

/** Class on the diagram container this action inserts after a Mermaid `<pre>`. */
const MERMAID_CONTAINER_CLASS = "mermaid-fence";
/** Class on the failure status inserted before a Mermaid `<pre>`. */
const MERMAID_ERROR_CLASS = "mermaid-fence-error";

/**
 * Svelte action that post-processes fenced code blocks in rendered markdown,
 * after DOMPurify sanitization.
 *
 * Labeled fences get Shiki highlighting; `mermaid` fences go through the shared
 * Mermaid runtime instead and are replaced by an app-sanitized SVG. Unlabeled
 * or unsupported fences are left as plain escaped text. Re-applies search
 * <mark> nodes after each swap since innerHTML replacement wipes them.
 */
export function highlightCodeFences(
  node: HTMLElement,
  params: HighlightCodeFencesParams,
) {
  // Per-element cancel functions that mark in-flight highlights as stale.
  const cancels = new Map<HTMLElement, () => void>();

  function cancelAll() {
    for (const cancel of cancels.values()) cancel();
    cancels.clear();
  }

  function highlightNode(
    codeEl: HTMLElement,
    lang: string,
    q: string,
    isCurrent: boolean,
  ) {
    cancels.get(codeEl)?.();

    let stale = false;
    cancels.set(codeEl, () => { stale = true; });

    // Read plain text BEFORE any innerHTML swap so we capture the
    // DOMPurify-sanitized text content, not previous Shiki spans.
    const code = codeEl.textContent ?? "";

    highlightToHtml(code, lang).then((html) => {
      if (stale) return;
      cancels.delete(codeEl);
      if (html === null) return;

      codeEl.innerHTML = html;

      // Re-apply search marks to this code element after the innerHTML swap
      // wiped any <mark> nodes that applyHighlight had placed inside it.
      if (q.trim()) applyMarks(codeEl, q, isCurrent);
    }).catch(() => {
      // Any error: leave the plain escaped text as-is.
      cancels.delete(codeEl);
    });
  }

  /** Diagram container that belongs to this `<pre>`, if one is already up. */
  function diagramFor(pre: HTMLElement): HTMLElement | null {
    const next = pre.nextElementSibling;
    return next?.classList.contains(MERMAID_CONTAINER_CLASS)
      ? (next as HTMLElement)
      : null;
  }

  /** Failure status that belongs to this `<pre>`, if one is already up. */
  function statusFor(pre: HTMLElement): HTMLElement | null {
    const prev = pre.previousElementSibling;
    return prev?.classList.contains(MERMAID_ERROR_CLASS)
      ? (prev as HTMLElement)
      : null;
  }

  function showDiagram(pre: HTMLElement, svg: string) {
    statusFor(pre)?.remove();

    let container = diagramFor(pre);
    if (container === null) {
      container = document.createElement("div");
      container.className = MERMAID_CONTAINER_CLASS;
      // Styled inline: this element is created outside any component, so no
      // scoped stylesheet reaches it and the four markdown surfaces would
      // otherwise each need their own global rule.
      container.style.maxWidth = "100%";
      container.style.overflowX = "auto";
      pre.after(container);
    }

    // Already sanitized by the app's own DOMPurify pass in utils/mermaid.ts.
    container.innerHTML = svg;
    // Only hide the source once a diagram is actually in place, so a failure
    // can never leave a blank gap where the fence used to be.
    pre.hidden = true;
  }

  /** Put the fence back to plain source: no diagram, no status, `<pre>` shown. */
  function restoreSource(pre: HTMLElement) {
    diagramFor(pre)?.remove();
    statusFor(pre)?.remove();
    pre.hidden = false;
  }

  function showFailure(pre: HTMLElement) {
    diagramFor(pre)?.remove();
    pre.hidden = false;

    if (statusFor(pre) !== null) return;

    const status = document.createElement("div");
    status.className = MERMAID_ERROR_CLASS;
    status.setAttribute("role", "status");
    status.textContent = "Diagram failed to render — showing source";
    status.style.fontSize = "12px";
    status.style.opacity = "0.75";
    pre.before(status);
  }

  function renderDiagram(
    codeEl: HTMLElement,
    pre: HTMLElement,
    theme: MermaidTheme,
  ) {
    cancels.get(codeEl)?.();

    let stale = false;
    cancels.set(codeEl, () => { stale = true; });

    const source = codeEl.textContent ?? "";

    renderMermaid(source, theme).then((svg) => {
      if (stale) return;
      cancels.delete(codeEl);
      showDiagram(pre, svg);
    }).catch(() => {
      if (stale) return;
      cancels.delete(codeEl);
      showFailure(pre);
    });
  }

  function run(p: HighlightCodeFencesParams) {
    cancelAll();

    const q = p.q ?? "";
    const isCurrent = p.current ?? false;
    const theme = p.theme;

    node
      .querySelectorAll<HTMLElement>("pre > code[class*='language-']")
      .forEach((codeEl) => {
        const cls = codeEl.className;
        const match = /\blanguage-(\S+)/.exec(cls);
        const lang = match?.[1] ?? "";
        if (!lang) return;

        if (isMermaidLabel(lang)) {
          const pre = codeEl.parentElement;
          if (!(pre instanceof HTMLElement)) return;
          // Search keeps the plain source for the same reason MessageContent
          // does: rendered SVG text is not a stable oracle for search marks.
          // This has to actively undo a diagram from an earlier run — on a
          // browse -> search transition the DOM is not rebuilt, so returning
          // early would leave the SVG up and the marked-up source hidden.
          if (q.trim()) {
            restoreSource(pre);
            return;
          }
          renderDiagram(codeEl, pre, theme);
          return;
        }

        highlightNode(codeEl, lang, q, isCurrent);
      });
  }

  run(params);

  return {
    update(p: HighlightCodeFencesParams) {
      run(p);
    },
    destroy() {
      cancelAll();
    },
  };
}
