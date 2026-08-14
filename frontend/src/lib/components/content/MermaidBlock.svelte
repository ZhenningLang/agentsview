<script lang="ts">
  import { onDestroy } from "svelte";
  import { ui } from "../../stores/ui.svelte.js";
  import { copyToClipboard } from "../../utils/clipboard.js";
  import { renderMermaid } from "../../utils/mermaid.js";
  import CodeBlock from "./CodeBlock.svelte";
  import CopyButton from "../shared/CopyButton.svelte";

  interface Props {
    content: string;
  }

  let { content }: Props = $props();

  /** Result of the newest settled render. `svg: null` means that exact
   *  (source, theme) pair failed. Keyed by both so a stale success can never be
   *  shown for different content, while a theme swap keeps the old diagram
   *  visible instead of flashing back to source. */
  type Outcome = {
    source: string;
    theme: "light" | "dark";
    svg: string | null;
  };

  let outcome = $state<Outcome | null>(null);
  let copied = $state(false);
  let copyTimer: ReturnType<typeof setTimeout> | undefined;
  let destroyed = false;

  let currentOutcome = $derived(
    outcome !== null && outcome.source === content ? outcome : null,
  );

  let svg = $derived(currentOutcome?.svg ?? null);

  /** Only report failure for the theme currently being displayed; a theme
   *  change re-arms the render, so the block goes back to plain source. */
  let failed = $derived(
    currentOutcome !== null &&
      currentOutcome.svg === null &&
      currentOutcome.theme === ui.theme,
  );

  $effect(() => {
    const source = content;
    const theme = ui.theme;
    let cancelled = false;

    renderMermaid(source, theme)
      .then((rendered) => {
        if (cancelled) return;
        outcome = { source, theme, svg: rendered };
      })
      .catch(() => {
        if (cancelled) return;
        outcome = { source, theme, svg: null };
      });

    // Runs both before a re-render and on destroy, so a late result can never
    // write back into a superseded or torn-down block.
    return () => {
      cancelled = true;
    };
  });

  async function handleCopy() {
    const ok = await copyToClipboard(content);
    if (!ok || destroyed) return;

    clearTimeout(copyTimer);
    copied = true;
    copyTimer = setTimeout(() => {
      copied = false;
    }, 1500);
  }

  onDestroy(() => {
    destroyed = true;
    clearTimeout(copyTimer);
  });
</script>

<div class="mermaid-block">
  {#if svg}
    <div class="mermaid-diagram-wrap">
      <CopyButton
        class="mermaid-copy"
        {copied}
        ariaLabel="Copy diagram source"
        copiedAriaLabel="Copied diagram source"
        title="Copy diagram source"
        copiedTitle="Copied!"
        onclick={handleCopy}
      />
      <!-- Already sanitized by the app's own DOMPurify pass in
           utils/mermaid.ts; never insert a raw runtime string here. -->
      <div class="mermaid-diagram">{@html svg}</div>
    </div>
  {:else}
    {#if failed}
      <div class="mermaid-error" role="status">
        Diagram failed to render — showing source
      </div>
    {/if}
    <CodeBlock {content} language="mermaid" />
  {/if}
</div>

<style>
  .mermaid-block {
    margin: 4px 0;
    max-width: 100%;
  }

  .mermaid-diagram-wrap {
    position: relative;
    background: var(--code-bg);
    border-radius: var(--radius-md);
    padding: 12px 16px;
    max-width: 100%;
  }

  :global(.mermaid-copy.copy-btn) {
    position: absolute;
    top: 6px;
    right: 6px;
    z-index: 1;
  }

  .mermaid-diagram-wrap:hover :global(.mermaid-copy.copy-btn) {
    opacity: 1;
  }

  .mermaid-diagram {
    max-width: 100%;
    overflow-x: auto;
  }

  .mermaid-diagram :global(svg) {
    max-width: 100%;
    height: auto;
  }

  .mermaid-error {
    font-size: 12px;
    color: var(--text-muted);
    padding: 4px 2px;
  }
</style>
