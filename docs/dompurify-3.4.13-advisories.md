# DOMPurify 3.4.11 -> 3.4.13 advisory review

Written while upgrading `frontend/package.json` from `dompurify@3.4.11` to
`dompurify@3.4.13` (upstream parity with `kenn-io/agentsview`).

Sources are the official GitHub Advisory Database entries, read on 2026-08-12.
Upstream renovate commit bodies (`c64bacb9`, `7d3b4805`) were used only to
discover which advisories to look up, not as the statement of fact.

## How this repository calls DOMPurify

- Single call site: `frontend/src/lib/utils/markdown.ts` — `DOMPurify.sanitize(html)`.
- The input is a **string** produced by `marked`, not a DOM node.
- No `DOMPurify.addHook(...)`, no `DOMPurify.setConfig(...)`.
- No `CUSTOM_ELEMENT_HANDLING`, no `IN_PLACE`, no `RETURN_DOM*`.
- Default configuration only; the second argument is never passed.

Machine-checkable form of the same claim (run from the repository root):

```bash
grep -RE 'DOMPurify\.(addHook|setConfig)|CUSTOM_ELEMENT_HANDLING|IN_PLACE' \
  frontend/src --include='*.ts' --include='*.svelte' --include='*.js'
```

That grep must stay empty. The XML-style prompt tag preservation added in the
same change escapes tags at the `marked` token level and deliberately does
**not** widen the sanitizer allowlist, so it does not move this repository into
any of the affected configurations below.

## Advisory 1 — GHSA-c2j3-45gr-mqc4

<https://github.com/advisories/GHSA-c2j3-45gr-mqc4>

| Field | Value |
| ----- | ----- |
| Title | `CUSTOM_ELEMENT_HANDLING` bypasses `afterSanitizeElements` for allowed custom elements |
| Severity | Low (CVSS 2.1) |
| CVE | none assigned |
| Affected | `<= 3.4.11` |
| First patched | `3.4.12` |

Custom elements permitted through `CUSTOM_ELEMENT_HANDLING.tagNameCheck` return
early from `_sanitizeDisallowedNode()` and therefore skip the
`afterSanitizeElements` hook. An application hook that strips security-relevant
attributes from every element would still strip them from ordinary elements but
leave them on allowed custom elements; the attribute value becomes a
second-order XSS vector only if the custom element later writes it into an HTML
sink.

Triggering it requires **all** of: `CUSTOM_ELEMENT_HANDLING` enabled, an
`afterSanitizeElements` hook registered via `addHook()`, and a custom element
that re-injects preserved attributes. The advisory states that standard
`DOMPurify.sanitize(html)` with default configuration is not affected.

**Applicability here: not affected on the call path.** The installed package
version `3.4.11` was inside the affected range, but
`frontend/src/lib/utils/markdown.ts` uses none of the three preconditions.

## Advisory 2 — GHSA-hpcv-96wg-7vj8

<https://github.com/advisories/GHSA-hpcv-96wg-7vj8>

| Field | Value |
| ----- | ----- |
| Title | Cross-realm `IN_PLACE` sanitization leaves executable markup intact via realm-bound `instanceof` checks |
| Severity | Moderate (CVSS 6.1) |
| CVE | CVE-2026-49458 |
| Affected | `<= 3.4.5` |
| First patched | `3.4.6` |

Sanitizing DOM nodes that originate in another JavaScript realm (for example a
same-origin iframe) with `IN_PLACE: true` defeats realm-bound `instanceof`
checks, so foreign-realm `<form>` elements escape the `_isClobbered` check,
foreign-realm `<template>` content is never recursed into, and attached shadow
roots skip the sanitization walk entirely.

Triggering it requires `sanitize(node, { IN_PLACE: true })` with a cross-realm
**node** input. The advisory states string input is not affected, because the
parser builds nodes inside the sanitizer's own realm.

**Applicability here: not affected, on two independent grounds.** The installed
version `3.4.11` was already above the patched `3.4.6`, and the call path passes
a string with default configuration. This advisory is listed only because the
upstream renovate body for the 3.4.12 bump cited it; it is not a reason for this
upgrade.

## Advisory 3 — GHSA-55q2-fjhq-7xh7

<https://github.com/advisories/GHSA-55q2-fjhq-7xh7>

| Field | Value |
| ----- | ----- |
| Title | `IN_PLACE` hook removal leaves a detached subtree executable, causing XSS |
| Severity | Moderate (CVSS 5.1) |
| CVE | none assigned |
| Affected | `<= 3.4.12` |
| First patched | `3.4.13` |

During in-place sanitization, a hook that removes an element causes
`_sanitizeElements()` to return before `_neutralizeSubtree()` runs, so the
removed element's detached descendants keep queued event handlers. An `<img>`
can retain an attacker-supplied `onload` and fire after `sanitize()` returns,
even though the returned root is clean.

Triggering it requires `IN_PLACE: true` plus a `beforeSanitizeElements` or
`uponSanitizeElement` hook that removes a containing element. The advisory
states default `DOMPurify.sanitize(html)` is safe.

**Applicability here: not affected on the call path.** The installed version
`3.4.11` was inside the affected range, but there is no `IN_PLACE` usage and no
hook of any kind.

## Conclusion

All three advisories require non-default DOMPurify configuration that this
repository does not use. **No evidence was found that the standard
`sanitize(html)` call in `frontend/src/lib/utils/markdown.ts` was exploitable at
`3.4.11`, and this upgrade should not be described as fixing a live XSS here.**

The upgrade to `3.4.13` is justified as:

1. moving the dependency out of the affected package ranges of
   GHSA-c2j3-45gr-mqc4 (`<= 3.4.11`) and GHSA-55q2-fjhq-7xh7 (`<= 3.4.12`), so
   audit tooling stops flagging it and a future call-site change cannot silently
   land on a vulnerable version; and
2. dependency parity with upstream `kenn-io/agentsview`.

If a future change introduces `IN_PLACE`, `CUSTOM_ELEMENT_HANDLING`, or any
`addHook` call, revisit this document: the "not affected" conclusions above are
statements about the call path, not about the library.
