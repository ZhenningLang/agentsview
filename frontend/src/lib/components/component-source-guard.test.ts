// ABOUTME: Deterministic source guards over component markup and styles.
// ABOUTME: Phase 19 adds the accent-fill/foreground-token pairing guard.
//
// These are static checks, not rendering tests: they read the .svelte sources
// so a newly written rule is rejected at review time rather than discovered
// visually. Every detector below is a pure function over source text and is
// exercised against both an offending and a non-offending sample, so a regex
// that matches everything (or nothing) fails here instead of silently
// reporting an empty offender list.
import { describe, expect, it } from "vitest";
// @ts-ignore -- @types/node is not in devDependencies; harmless at runtime.
import { readdirSync, readFileSync } from "node:fs";
// @ts-ignore -- @types/node is not in devDependencies; harmless at runtime.
import { relative, resolve } from "node:path";

const componentsRoot = resolve(
  // @ts-ignore -- import.meta.dirname is Node 20.11+, in the supported range.
  import.meta.dirname,
);

/** App.svelte lives outside components/ but paints the same accent fills. */
const appShell = resolve(componentsRoot, "../../App.svelte");

/**
 * Native `<select>` elements predating this guard. The fork shipped these
 * before the upstream "no new native selects" convention was ported, so they
 * are grandfathered rather than rewritten inside a frontend-only phase.
 * The list is exact paths on purpose: a new native select in any other
 * component still fails, and removing one of these must shrink the list.
 */
const GRANDFATHERED_NATIVE_SELECTS = [
  "insights/InsightsPage.svelte",
  "memory/MemoryPage.svelte",
  "settings/LanguageSwitcher.svelte",
  "settings/LLMEnrichmentSettings.svelte",
  "settings/WorktreeMappingSettings.svelte",
  "vault/VaultPage.svelte",
];

function svelteFiles(dir: string): string[] {
  return readdirSync(dir, { withFileTypes: true }).flatMap((entry: any) => {
    const path = `${dir}/${entry.name}`;
    if (entry.isDirectory()) return svelteFiles(path);
    if (entry.isFile() && entry.name.endsWith(".svelte")) return [path];
    return [];
  });
}

function styleBlocks(source: string): string[] {
  return Array.from(
    source.matchAll(/<style\b[^>]*>([\s\S]*?)<\/style>/g),
    // Comments are dropped here so a note above a rule cannot leak into the
    // selector text the detectors report and match on.
  ).map((match) => (match[1] ?? "").replace(/\/\*[\s\S]*?\*\//g, " "));
}

export function oneOffControlStyleOffenders(source: string): string[] {
  return styleBlocks(source).flatMap((style) => {
    const offenders: string[] = [];
    if (/\.[A-Za-z0-9_-]+-select(?=[\s:{,.#>+~[])/.test(style)) {
      offenders.push("component-specific *-select selector");
    }
    if (/\.[A-Za-z0-9_-]+-select-chevron(?=[\s:{,.#>+~[])/.test(style)) {
      offenders.push("manual select chevron selector");
    }
    if (/(?:^|\s)-?(?:webkit-)?appearance\s*:\s*none\b/.test(style)) {
      offenders.push("manual native control appearance reset");
    }
    return offenders;
  });
}

interface CssRule {
  selector: string;
  block: string;
}

/** Split a selector list on top-level commas, keeping `:not(a, b)` intact. */
function selectorList(selector: string): string[] {
  const parts: string[] = [];
  let depth = 0;
  let current = "";
  for (const char of selector) {
    if (char === "(") depth++;
    else if (char === ")") depth = Math.max(0, depth - 1);
    if (char === "," && depth === 0) {
      parts.push(current);
      current = "";
      continue;
    }
    current += char;
  }
  parts.push(current);
  return parts.map((part) => part.trim()).filter(Boolean);
}

/**
 * One entry per selector, not per rule: a grouped rule such as
 * `.a, .b:not(.c) { ... }` has to be read as two independent selectors, or a
 * lift written for the last member silently appears to cover the first.
 */
function cssRules(style: string): CssRule[] {
  return Array.from(style.matchAll(/([^{}]+)\{([^{}]*)\}/g)).flatMap((match) =>
    selectorList((match[1] ?? "").trim().replace(/\s+/g, " ")).map(
      (selector) => ({ selector, block: match[2] ?? "" }),
    ),
  );
}

/** Value of a declaration in a rule body, or null when it is not declared. */
function declaration(block: string, property: string): string | null {
  const match = block.match(
    new RegExp(`(?:^|[;{\\s])${property}\\s*:([^;]*)`, "i"),
  );
  return match ? (match[1] ?? "").trim() : null;
}

/**
 * Whether a rule paints an opaque accent fill.
 *
 * `color-mix()` counts: darkening or lightening an accent token still leaves a
 * saturated fill whose luminance tracks the token, so a foreground pinned to
 * white breaks exactly the same way once the token moves. Mixes toward
 * `transparent` do not count -- those are the tint backgrounds the fork uses
 * behind normal body text, where the surface underneath decides readability.
 */
function paintsAccentFill(block: string): boolean {
  for (const property of ["background", "background-color"]) {
    const value = declaration(block, property);
    if (value === null) continue;
    if (!/var\(--accent-[a-z]+/.test(value)) continue;
    if (/\btransparent\b/.test(value)) continue;
    return true;
  }
  return false;
}

function hardCodesWhiteForeground(block: string): boolean {
  const value = declaration(block, "color");
  return value !== null && /^(?:white|#fff(?:fff)?)$/i.test(value);
}

/**
 * Report style rules that fill a background with an accent token and then
 * hard-code a white foreground. The pairing must go through the
 * `--accent-*-foreground` tokens so high contrast and dark mode can move it.
 *
 * The check is per CSS rule, not per file: a file may legitimately contain a
 * white foreground in one rule and an accent fill in a different one. For the
 * case where two rules land on the *same element* through separate classes,
 * see `combinedClassAccentOffenders` below.
 */
export function accentFillForegroundOffenders(source: string): string[] {
  return styleBlocks(source).flatMap((style) =>
    cssRules(style).flatMap(({ selector, block }) =>
      paintsAccentFill(block) && hardCodesWhiteForeground(block)
        ? [`${selector}: hard-coded white foreground on accent fill`]
        : [],
    ),
  );
}

/** Class names in the last compound selector, e.g. `.a .b.c` -> ["b", "c"]. */
function trailingClasses(selector: string): string[] {
  const compound = selector.split(/[\s>+~]+/).filter(Boolean).pop() ?? "";
  const withoutNegations = compound.replace(/:not\([^)]*\)/g, "");
  return Array.from(withoutNegations.matchAll(/\.([A-Za-z0-9_-]+)/g)).map(
    (match) => match[1]!,
  );
}

/** Class names excluded by `:not(.x)` in the last compound selector. */
function negatedClasses(selector: string): string[] {
  const compound = selector.split(/[\s>+~]+/).filter(Boolean).pop() ?? "";
  return Array.from(compound.matchAll(/:not\(\s*\.([A-Za-z0-9_-]+)\s*\)/g)).map(
    (match) => match[1]!,
  );
}

/** Markup with `<script>`/`<style>` removed, so only the template remains. */
function templateOf(source: string): string {
  return source
    .replace(/<style\b[^>]*>[\s\S]*?<\/style>/g, "")
    .replace(/<script\b[^>]*>[\s\S]*?<\/script>/g, "");
}

/**
 * Class names that land together on one element, one entry per element.
 *
 * Svelte spreads a single element's classes over `class="..."`, interpolated
 * strings and `class:name={cond}` directives, and the tag can span many lines,
 * so the scanner walks each opening tag tracking quotes and brace depth rather
 * than matching a tag in one regex.
 */
export function elementClassGroups(source: string): string[][] {
  const template = templateOf(source);
  const groups: string[][] = [];
  const tagStart = /<([A-Za-z][\w.-]*)/g;
  let tag: RegExpExecArray | null;
  while ((tag = tagStart.exec(template)) !== null) {
    let index = tagStart.lastIndex;
    let depth = 0;
    let quote: string | null = null;
    let end = -1;
    while (index < template.length) {
      const char = template[index]!;
      if (quote !== null) {
        if (char === quote) quote = null;
      } else if (char === '"' || char === "'") {
        quote = char;
      } else if (char === "{") {
        depth++;
      } else if (char === "}") {
        depth = Math.max(0, depth - 1);
      } else if (char === ">" && depth === 0) {
        end = index;
        break;
      }
      index++;
    }
    if (end < 0) continue;
    const attributes = template.slice(tagStart.lastIndex, end);
    const classes = new Set<string>();
    for (const attr of attributes.matchAll(/\bclass\s*=\s*"([^"]*)"/g)) {
      const raw = attr[1] ?? "";
      for (const word of raw.replace(/\{[^{}]*\}/g, " ").split(/\s+/)) {
        if (word) classes.add(word);
      }
      // Class names chosen inside an interpolation, e.g. `{ok ? "a" : "b"}`.
      for (const expression of raw.matchAll(/\{[^{}]*\}/g)) {
        for (const literal of expression[0].matchAll(/["'`]([^"'`]*)["'`]/g)) {
          for (const word of (literal[1] ?? "").split(/\s+/)) {
            if (word) classes.add(word);
          }
        }
      }
    }
    for (const directive of attributes.matchAll(
      /\bclass:([A-Za-z0-9_-]+)/g,
    )) {
      classes.add(directive[1]!);
    }
    if (classes.size > 0) groups.push([...classes]);
  }
  return groups;
}

/**
 * Report elements that end up with an accent fill and a hard-coded white
 * foreground through *different* classes -- the base class carries the text
 * color and a variant class carries the fill, so the per-rule guard above sees
 * nothing wrong while the rendered element is still white-on-accent.
 */
export function combinedClassAccentOffenders(source: string): string[] {
  const whiteByClass = new Map<string, string>();
  const fillByClass = new Map<string, string>();
  for (const style of styleBlocks(source)) {
    for (const { selector, block } of cssRules(style)) {
      const classes = trailingClasses(selector);
      if (classes.length !== 1) continue;
      const name = classes[0]!;
      if (hardCodesWhiteForeground(block)) whiteByClass.set(name, selector);
      if (paintsAccentFill(block)) fillByClass.set(name, selector);
    }
  }
  if (whiteByClass.size === 0 || fillByClass.size === 0) return [];

  const offenders: string[] = [];
  for (const group of elementClassGroups(source)) {
    const white = group.filter((name) => whiteByClass.has(name));
    const fill = group.filter((name) => fillByClass.has(name));
    if (white.length === 0 || fill.length === 0) continue;
    // A single class doing both is the per-rule guard's business, not ours.
    if (white.length === 1 && fill.length === 1 && white[0] === fill[0]) {
      continue;
    }
    offenders.push(
      `${white.map((name) => `.${name}`).join("+")} white foreground combines with accent fill ${fill
        .map((name) => `.${name}`)
        .join("+")}`,
    );
  }
  return [...new Set(offenders)];
}

function expandHex(hex: string): string {
  const body = hex.slice(1);
  return body.length === 3
    ? `#${body[0]!.repeat(2)}${body[1]!.repeat(2)}${body[2]!.repeat(2)}`
    : `#${body}`;
}

function channels(hex: string): [number, number, number] {
  const body = expandHex(hex).slice(1);
  return [0, 2, 4].map((i) => parseInt(body.slice(i, i + 2), 16)) as [
    number,
    number,
    number,
  ];
}

function relativeLuminance(rgb: [number, number, number]): number {
  const [r, g, b] = rgb.map((value) => {
    const channel = value / 255;
    return channel <= 0.03928
      ? channel / 12.92
      : Math.pow((channel + 0.055) / 1.055, 2.4);
  }) as [number, number, number];
  return 0.2126 * r + 0.7152 * g + 0.0722 * b;
}

export function contrastRatio(a: string, b: string): number {
  const first = relativeLuminance(channels(a));
  const second = relativeLuminance(channels(b));
  return (
    (Math.max(first, second) + 0.05) / (Math.min(first, second) + 0.05)
  );
}

/** The two surfaces a literal color has to stay readable on. */
const LIGHT_SURFACE = "#ffffff";
const DARK_SURFACE = "#0d1117";

/** Grey enough that recoloring it cannot destroy encoded meaning. */
function isNeutral(hex: string): boolean {
  const [r, g, b] = channels(hex);
  return Math.max(r, g, b) - Math.min(r, g, b) <= 24;
}

/**
 * Report rules whose text color is a literal neutral grey that no
 * `:global(.high-contrast)` rule in the same file lifts.
 *
 * Every literal grey is theme-blind by construction -- no single grey clears
 * 4.5:1 against both the light and the dark surface -- so one of the two
 * themes is always below AA and high contrast is the mode that has to move it.
 * The guard is limited to neutral greys on purpose: the fork's saturated
 * literals (`#f29070` slow, `#c47a7a` error, `#6ad0a8` live) encode state, and
 * recoloring them would erase meaning rather than restore contrast.
 *
 * Coverage is decided on class sets, not on substrings: a lift written for
 * `.cd.muted` does not cover the bare `.cd` that every row renders, which is
 * precisely the half-wired shape review B found. A high-contrast rule covers a
 * grey rule when its own trailing classes are a subset of the grey rule's and
 * none of its `:not()` exclusions appear there.
 */
export function highContrastGapOffenders(source: string): string[] {
  const lifts: { positive: string[]; negated: string[] }[] = [];
  const greys: { selector: string; hex: string }[] = [];
  for (const style of styleBlocks(source)) {
    for (const { selector, block } of cssRules(style)) {
      if (selector.includes("high-contrast")) {
        lifts.push({
          positive: trailingClasses(selector),
          negated: negatedClasses(selector),
        });
        continue;
      }
      const value = declaration(block, "color");
      if (value === null) continue;
      const hex = value.match(/^#[0-9a-fA-F]{3}(?:[0-9a-fA-F]{3})?$/)?.[0];
      if (!hex || !isNeutral(hex)) continue;
      // White and near-white are foreground literals for filled surfaces, not
      // muted body text; `accentFillForegroundOffenders` owns those.
      if (contrastRatio(hex, LIGHT_SURFACE) < 1.5) continue;
      greys.push({ selector, hex });
    }
  }

  return greys
    .filter(({ selector }) => {
      const own = new Set(trailingClasses(selector));
      return !lifts.some(
        (lift) =>
          lift.positive.length > 0 &&
          lift.positive.every((name) => own.has(name)) &&
          !lift.negated.some((name) => own.has(name)),
      );
    })
    .map(
      ({ selector, hex }) =>
        `${selector}: literal ${hex} has no high-contrast lift`,
    );
}

/**
 * Accent fill tokens declared in a CSS custom-property block that have no
 * matching `-foreground` companion in the same block.
 */
export function missingForegroundTokens(block: string): string[] {
  const fills = new Set<string>();
  const foregrounds = new Set<string>();
  for (const match of block.matchAll(/--accent-([a-z]+)(-foreground)?\s*:/g)) {
    if (match[2]) {
      foregrounds.add(match[1]!);
    } else {
      fills.add(match[1]!);
    }
  }
  return [...fills].filter((fill) => !foregrounds.has(fill)).sort();
}

/** Extract the declaration body of a top-level CSS rule by selector. */
function cssRuleBody(source: string, selector: string): string {
  const index = source.indexOf(`${selector} {`);
  expect(index, `${selector} must exist in app.css`).toBeGreaterThanOrEqual(0);
  const start = source.indexOf("{", index);
  const end = source.indexOf("\n}", start);
  expect(end, `${selector} must be closed`).toBeGreaterThan(start);
  return source.slice(start, end);
}

const OFFENDING_SAMPLE = `
<style>
  .primary-btn {
    background: var(--accent-blue);
    color: #fff;
  }
</style>
`;

/** The same fill hidden inside a color-mix, as MemoryPage's hover state had. */
const OFFENDING_MIX_SAMPLE = `
<style>
  .primary-btn:hover {
    background: color-mix(in srgb, var(--accent-blue) 82%, black);
    color: #fff;
  }
</style>
`;

const CLEAN_SAMPLES = [
  // Foreground token instead of a literal.
  `<style>
     .primary-btn { background: var(--accent-blue); color: var(--accent-blue-foreground); }
   </style>`,
  // Accent dot with no text at all.
  `<style>
     .agent-dot { background: var(--accent-green); width: 6px; height: 6px; }
   </style>`,
  // Progress bar fill, again with no foreground.
  `<style>
     .bar-fill { background: var(--accent-amber); height: 4px; }
   </style>`,
  // White text on a non-accent surface: not this guard's business.
  `<style>
     .badge { background: var(--text-muted); color: #fff; }
   </style>`,
  // A translucent accent tint: the surface underneath, not the token, decides
  // readability, and the fork paints roughly twenty of these.
  `<style>
     .chip-soft { background: color-mix(in srgb, var(--accent-indigo) 12%, transparent); color: #fff; }
   </style>`,
  // A darkened accent fill that did move its foreground onto the token.
  `<style>
     .primary-btn:hover { background: color-mix(in srgb, var(--accent-blue) 82%, black); color: var(--accent-blue-foreground); }
   </style>`,
  // Accent fill and white text in two rules that never meet on one element.
  // `combinedClassAccentOffenders` is what covers the case where they do.
  `<style>
     .chip { background: var(--accent-purple); }
     .note { color: white; }
   </style>`,
];

/** A base class carrying the text color and a variant carrying the fill. */
const OFFENDING_COMBINED_SAMPLE = `
<span class="header-badge" class:badge-red={failed} class:badge-blue={!failed}>
  Generating
</span>
<style>
  .header-badge { color: white; font-size: 9px; }
  .badge-blue { background: var(--accent-blue); }
  .badge-red { background: var(--accent-red); }
</style>
`;

const CLEAN_COMBINED_SAMPLES = [
  // The same markup once the foreground moved onto the variant classes.
  `<span class="header-badge" class:badge-blue={true}>Generating</span>
   <style>
     .header-badge { font-size: 9px; }
     .badge-blue { background: var(--accent-blue); color: var(--accent-blue-foreground); }
   </style>`,
  // Two classes that both exist but never share an element.
  `<span class="chip">a</span><span class="note">b</span>
   <style>
     .chip { background: var(--accent-purple); }
     .note { color: white; }
   </style>`,
  // White text over a neutral surface next to an unrelated accent bar.
  `<div class="toast bar-fill">hi</div>
   <style>
     .toast { background: var(--text-muted); color: #fff; }
     .bar-fill { height: 4px; }
   </style>`,
];

/** A literal grey lifted only through one variant, leaving the base uncovered. */
const OFFENDING_GREY_SAMPLE = `
<style>
  .call .cd { color: #999; }
  .call .cd.muted { color: #666; }
  :global(.high-contrast) .call .cd.muted { color: var(--text-secondary); }
</style>
`;

const CLEAN_GREY_SAMPLES = [
  // The lift matches the base class, so both it and .muted are covered.
  `<style>
     .call .cd { color: #999; }
     .call .cd.muted { color: #666; }
     :global(.high-contrast) .call .cd { color: var(--text-secondary); }
   </style>`,
  // Semantic colors are not neutral, so they are outside the guard.
  `<style>
     .call .cd.slow { color: #f29070; }
     .call .cd.live { color: #6ad0a8; }
   </style>`,
  // A grouped lift still covers each of its own selectors.
  `<style>
     .ca { color: #888; }
     .cd { color: #999; }
     :global(.high-contrast) .ca,
     :global(.high-contrast) .cd { color: var(--text-secondary); }
   </style>`,
  // A literal grey on a non-text property is outside the guard.
  `<style>
     .axis { border-bottom: 1px solid #232323; color: var(--text-muted); }
   </style>`,
];

describe("component source guardrails", () => {
  it("keeps new native select controls out of component source", () => {
    const offenders = svelteFiles(componentsRoot)
      .flatMap((path) => {
        const rel = relative(componentsRoot, path);
        const source = readFileSync(path, "utf8");
        return /<select\b/.test(source) ? [rel] : [];
      })
      .sort((a, b) => a.localeCompare(b));

    expect(offenders).toEqual(GRANDFATHERED_NATIVE_SELECTS);
  });

  it("keeps one-off select chrome out of component styles", () => {
    const offenders = svelteFiles(componentsRoot)
      .flatMap((path) => {
        const rel = relative(componentsRoot, path);
        const source = readFileSync(path, "utf8");
        return oneOffControlStyleOffenders(source).map(
          (reason) => `${rel}: ${reason}`,
        );
      })
      .sort((a, b) => a.localeCompare(b));

    expect(offenders).toEqual([]);
  });
});

describe("Phase 19 accent foreground source guard", () => {
  it("rejects a hard-coded white foreground on an accent fill", () => {
    expect(accentFillForegroundOffenders(OFFENDING_SAMPLE)).toEqual([
      ".primary-btn: hard-coded white foreground on accent fill",
    ]);
  });

  it("does not flag legitimate accent usage", () => {
    for (const sample of CLEAN_SAMPLES) {
      expect(accentFillForegroundOffenders(sample)).toEqual([]);
    }
  });

  it("keeps accent-filled component styles on foreground tokens", () => {
    const offenders = svelteFiles(componentsRoot)
      .flatMap((path) => {
        const rel = relative(componentsRoot, path);
        const source = readFileSync(path, "utf8");
        return accentFillForegroundOffenders(source).map(
          (reason) => `${rel}: ${reason}`,
        );
      })
      .sort((a, b) => a.localeCompare(b));

    expect(offenders).toEqual([]);
  });

  it("scans a non-trivial number of component files", () => {
    // Guards against a broken walker reporting an empty offender list.
    expect(svelteFiles(componentsRoot).length).toBeGreaterThan(50);
  });

  it("keeps the app shell outside components/ on foreground tokens", () => {
    const source = readFileSync(appShell, "utf8");
    expect(source).toContain("<style>");
    expect(accentFillForegroundOffenders(source)).toEqual([]);
  });

  it("rejects a white foreground on an accent fill hidden in a color-mix", () => {
    expect(accentFillForegroundOffenders(OFFENDING_MIX_SAMPLE)).toEqual([
      ".primary-btn:hover: hard-coded white foreground on accent fill",
    ]);
  });
});

describe("Phase 19 combined-class accent foreground guard", () => {
  it("rejects a base-class white foreground over a variant-class accent fill", () => {
    expect(combinedClassAccentOffenders(OFFENDING_COMBINED_SAMPLE)).toEqual([
      ".header-badge white foreground combines with accent fill .badge-red+.badge-blue",
    ]);
  });

  it("does not flag classes that never meet on one element", () => {
    for (const sample of CLEAN_COMBINED_SAMPLES) {
      expect(combinedClassAccentOffenders(sample)).toEqual([]);
    }
  });

  it("reads classes from literals, interpolations and class: directives", () => {
    const groups = elementClassGroups(
      `<span
         class="badge {kind === 'a' ? 'badge-a' : 'badge-b'}"
         class:selected={active}
       >x</span>`,
    );
    // "a" comes from the comparison, not from a class: the scanner takes every
    // string literal inside the interpolation because it does not parse the
    // expression. Over-collecting only ever adds candidate names, so the guard
    // can report a pairing that does not exist but cannot miss one that does.
    expect(groups).toEqual([["badge", "a", "badge-a", "badge-b", "selected"]]);
  });

  it("keeps every rendered element off white-on-accent combinations", () => {
    const offenders = [...svelteFiles(componentsRoot), appShell]
      .flatMap((path) => {
        const rel = relative(componentsRoot, path);
        const source = readFileSync(path, "utf8");
        return combinedClassAccentOffenders(source).map(
          (reason) => `${rel}: ${reason}`,
        );
      })
      .sort((a, b) => a.localeCompare(b));

    expect(offenders).toEqual([]);
  });
});

describe("Phase 19 high-contrast literal-grey coverage", () => {
  it("rejects a lift that only covers a variant of the grey rule", () => {
    expect(highContrastGapOffenders(OFFENDING_GREY_SAMPLE)).toEqual([
      ".call .cd: literal #999 has no high-contrast lift",
    ]);
  });

  it("accepts a lift on the base class, semantic colors and readable greys", () => {
    for (const sample of CLEAN_GREY_SAMPLES) {
      expect(highContrastGapOffenders(sample)).toEqual([]);
    }
  });

  it("computes contrast ratios against both theme surfaces", () => {
    // Anchors the oracle itself: #999 is the value review B measured at
    // 2.85:1 on white, and #2a2f3a is the light high-contrast replacement.
    expect(contrastRatio("#999999", "#ffffff")).toBeCloseTo(2.85, 2);
    expect(contrastRatio("#2a2f3a", "#ffffff")).toBeGreaterThan(4.5);
  });

  it("leaves no literal grey in component styles without a lift", () => {
    const offenders = [...svelteFiles(componentsRoot), appShell]
      .flatMap((path) => {
        const rel = relative(componentsRoot, path);
        const source = readFileSync(path, "utf8");
        return highContrastGapOffenders(source).map(
          (reason) => `${rel}: ${reason}`,
        );
      })
      .sort((a, b) => a.localeCompare(b));

    expect(offenders).toEqual([]);
  });
});

describe("Phase 19 accent foreground token completeness", () => {
  const appCss = readFileSync(
    resolve(componentsRoot, "../../app.css"),
    "utf8",
  );

  it("detects a missing foreground companion", () => {
    expect(
      missingForegroundTokens(
        "--accent-blue: #00f; --accent-blue-foreground: #fff; --accent-teal: #0aa;",
      ),
    ).toEqual(["teal"]);
    expect(
      missingForegroundTokens(
        "--accent-blue: #00f; --accent-blue-foreground: #fff;",
      ),
    ).toEqual([]);
  });

  it("pairs every light-theme accent fill with a foreground", () => {
    const body = cssRuleBody(appCss, ":root");
    expect(missingForegroundTokens(body)).toEqual([]);
    expect(body).toContain("--accent-violet-foreground:");
  });

  it("pairs every dark-theme accent fill with a foreground", () => {
    const body = cssRuleBody(appCss, ":root.dark");
    expect(missingForegroundTokens(body)).toEqual([]);
    expect(body).toContain("--accent-violet-foreground:");
  });

  it("defines high-contrast overrides for both themes", () => {
    const light = cssRuleBody(appCss, ":root.high-contrast");
    const dark = cssRuleBody(appCss, ":root.dark.high-contrast");
    for (const token of [
      "--text-primary",
      "--text-secondary",
      "--text-muted",
      "--border-default",
      "--border-muted",
      "--accent-blue",
    ]) {
      expect(light, `light high contrast ${token}`).toContain(`${token}:`);
      expect(dark, `dark high contrast ${token}`).toContain(`${token}:`);
    }
    expect(appCss).toContain(":root.high-contrast :focus-visible");
  });
});
