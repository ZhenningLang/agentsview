<script lang="ts">
  import { onDestroy } from "svelte";
  import type { Virtualizer } from "@tanstack/virtual-core";
  import { messages } from "../../stores/messages.svelte.js";
  import { readProgress } from "../../stores/read-progress.svelte.js";
  import { ui } from "../../stores/ui.svelte.js";
  import { sessions } from "../../stores/sessions.svelte.js";
  import { MessageSquareIcon } from "../../icons.js";
  import { createVirtualizer } from "../../virtual/createVirtualizer.svelte.js";
  import MessageContent from "./MessageContent.svelte";
  import CompactBoundaryDivider from "./CompactBoundaryDivider.svelte";
  import SystemBoundaryCard from "../system/SystemBoundaryCard.svelte";
  import ToolCallGroup from "./ToolCallGroup.svelte";
  import type { Message } from "../../api/types.js";
  import {
    buildDisplayItems,
    type DisplayItem,
  } from "../../utils/display-items.js";
  import { filterDisplayItemsByTranscriptMode } from "../../utils/transcript-mode.js";
  import {
    hasVisibleSegments,
  } from "../../utils/content-parser.js";
  import {
    contextEventSubtype,
    isContextEventMessage,
  } from "../../utils/messages.js";
  import { resolveMessageLayout } from "../../utils/message-layout.js";
  import { inSessionSearch } from "../../stores/inSessionSearch.svelte.js";
  import { sessionActivity } from "../../stores/sessionActivity.svelte.js";
  import SessionFindBar from "./SessionFindBar.svelte";
  import {
    getAlignedOffsetScrollAlign,
    getLatestDisplayIndex,
    type ScrollAlign,
  } from "./message-scroll.js";

  let containerRef: HTMLDivElement | undefined = $state(undefined);
  let scrollRaf: number | null = null;
  let lastScrollRequest = 0;
  let activeFollowScrollRequest: number | null = null;
  let followingScrollRaf: number | null = null;
  let followSettleTimer:
    | ReturnType<typeof setTimeout>
    | null = null;

  let baseMessages: Message[] = $derived.by(() => messages.messages);

  let baseDisplayItemsAsc = $derived(
    buildDisplayItems(baseMessages),
  );

  let filteredDisplayItemsAsc = $derived(
    buildDisplayItems(baseMessages, {
      skipToolGrouping: !ui.isBlockVisible("tool"),
    }),
  );

  function isItemVisible(item: DisplayItem): boolean {
    if (item.kind === "tool-group") {
      return true;
    }
    if (isContextEventMessage(item.message)) {
      return true;
    }
    return hasVisibleSegments(item.message, (type) =>
      ui.isBlockVisible(type),
    );
  }

  let normalDisplayItemsAsc = $derived.by(() => {
    if (!ui.hasBlockFilters) return baseDisplayItemsAsc;
    return filteredDisplayItemsAsc.filter(isItemVisible);
  });

  let displayItemsAsc = $derived.by(() => {
    if (ui.transcriptMode === "normal") {
      return normalDisplayItemsAsc;
    }

    if (!ui.hasBlockFilters) {
      return filterDisplayItemsByTranscriptMode(
        baseDisplayItemsAsc,
        "focused",
      );
    }

    return filterDisplayItemsByTranscriptMode(
      filteredDisplayItemsAsc,
      "focused",
      {
        isMessageVisible: (message) =>
          hasVisibleSegments(message, (type) =>
            ui.isBlockVisible(type),
          ),
      },
    ).filter(isItemVisible);
  });

  function itemAt(index: number) {
    if (ui.sortNewestFirst) {
      const mapped = displayItemsAsc.length - 1 - index;
      return displayItemsAsc[mapped];
    }
    return displayItemsAsc[index];
  }

  const virtualizer = createVirtualizer(() => {
    const count = displayItemsAsc.length;
    const el = containerRef ?? null;
    const sid = sessions.activeSessionId ?? "";
    return {
      count,
      getScrollElement: () => el,
      estimateSize: () => 120,
      overscan: 5,
      useAnimationFrameWithResizeObserver: true,
      measureCacheKey: sid,
      getItemKey: (index: number) => {
        const item = itemAt(index);
        if (!item) return `${sid}-${index}`;
        if (item.kind === "tool-group") {
          return `${sid}-tg-${item.ordinals[0]}`;
        }
        return `${sid}-m-${item.message.ordinal}`;
      },
    };
  });

  /** Svelte action: measure element for variable-height virtualizer */
  function measureElement(
    node: HTMLElement,
    virt: Virtualizer<HTMLElement, HTMLElement> | undefined,
  ) {
    virt?.measureElement(node);
    return {
      update(
        nextVirt:
          | Virtualizer<HTMLElement, HTMLElement>
          | undefined,
      ) {
        nextVirt?.measureElement(node);
      },
      destroy() {
        // Cleanup handled by virtualizer
      },
    };
  }

  // ---- transcript read progress -------------------------------------

  let readCheckRaf: number | null = null;
  /** Ordinals confirmed as displayed for the current session+revision. */
  let seenOrdinals = new Set<number>();
  let seenKey = "";

  let displayedOrdinalsAsc = $derived.by(() => {
    const out: number[] = [];
    for (const item of displayItemsAsc) out.push(...item.ordinals);
    return out.sort((a, b) => a - b);
  });

  let readToken = $derived(messages.activeSessionToken);

  let transcriptHasUnread = $derived(
    messages.sessionId !== null &&
      readProgress.hasUnread(messages.sessionId, readToken),
  );

  /**
   * Ordinal the unread run starts at, mapped onto what is actually
   * displayed. A reload reports the earliest changed ordinal directly; a
   * reopen without that metadata falls back to just past the last
   * confirmed read position, and finally to the oldest displayed message
   * so traversal stays conservative rather than confirming a tail.
   */
  let unreadBoundaryOrdinal = $derived.by(() => {
    if (!transcriptHasUnread) return null;
    const ordinals = displayedOrdinalsAsc;
    if (ordinals.length === 0) return null;

    const reported = messages.activeSessionUnreadOrdinal;
    const lastRead = messages.sessionId
      ? readProgress.lastReadOrdinal(messages.sessionId)
      : null;
    const target = reported ?? (lastRead !== null ? lastRead + 1 : null);
    if (target === null) return ordinals[0] ?? null;
    return ordinals.find((o) => o >= target) ?? null;
  });

  /**
   * Wording follows the sort direction, not just the divider position.
   * Ascending puts the divider above the boundary message, so everything
   * below it is what the user has not read: "New messages". Newest-first
   * reverses the list and puts the divider below the boundary message, so
   * everything below it is what the user already read: "Earlier messages".
   * Keeping "New messages" in both directions labels the older half of a
   * newest-first transcript as new.
   */
  let unreadDividerLabel = $derived(
    ui.sortNewestFirst ? "Earlier messages" : "New messages",
  );

  function isBoundaryItem(item: DisplayItem): boolean {
    return (
      unreadBoundaryOrdinal !== null &&
      item.kind === "message" &&
      item.message.ordinal === unreadBoundaryOrdinal
    );
  }

  /** Child ordinals of a grouped row whose own box is inside the
   *  viewport. A partially visible group must never confirm the children
   *  that are still off screen. */
  function visibleGroupChildOrdinals(index: number): number[] {
    if (!containerRef) return [];
    const row = containerRef.querySelector(
      `.virtual-row[data-index="${index}"]`,
    );
    if (!row) return [];
    const bounds = containerRef.getBoundingClientRect();
    const out: number[] = [];
    for (
      const el of row.querySelectorAll<HTMLElement>(
        "[data-message-ordinal]",
      )
    ) {
      const rect = el.getBoundingClientRect();
      if (rect.height === 0) continue;
      if (rect.bottom <= bounds.top) continue;
      if (rect.top >= bounds.bottom) continue;
      const ordinal = Number(el.dataset.messageOrdinal);
      if (Number.isFinite(ordinal)) out.push(ordinal);
    }
    return out;
  }

  function collectVisibleOrdinals(): number[] {
    const v = virtualizer.instance;
    if (!v || !containerRef) return [];
    const scrollTop = v.scrollOffset ?? 0;
    const height = containerRef.clientHeight;
    const viewportBottom = height > 0
      ? scrollTop + height
      : Number.POSITIVE_INFINITY;

    const out: number[] = [];
    for (const vi of v.getVirtualItems()) {
      if (vi.end <= scrollTop) continue;
      if (vi.start >= viewportBottom) continue;
      const item = itemAt(vi.index);
      if (!item) continue;
      const fullyVisible =
        vi.start >= scrollTop && vi.end <= viewportBottom;
      if (item.kind !== "tool-group" || fullyVisible) {
        out.push(...item.ordinals);
        continue;
      }
      out.push(...visibleGroupChildOrdinals(vi.index));
    }
    return out;
  }

  function checkReadProgress() {
    const sessionId = messages.sessionId;
    const token = readToken;
    if (!sessionId || !token || !containerRef) return;

    const key = `${sessionId}:${token}`;
    if (key !== seenKey) {
      seenKey = key;
      seenOrdinals = new Set();
    }
    const visible = collectVisibleOrdinals();
    for (const ordinal of visible) seenOrdinals.add(ordinal);

    if (!readProgress.markerFor(sessionId)) {
      readProgress.baseline(
        sessionId,
        token,
        visible.length > 0 ? Math.max(...visible) : null,
      );
      return;
    }

    if (!readProgress.hasUnread(sessionId, token)) {
      if (visible.length > 0) {
        readProgress.advanceOrdinal(sessionId, Math.max(...visible));
      }
      return;
    }

    // A transcript with nothing to display — every block filter hiding
    // its content, focused mode with nothing focusable, a system-only
    // transcript whose blocks are off — has no ordinal to traverse and no
    // scroll surface: handleScroll only asks for older history when at
    // least one virtual item exists, so this state cannot resolve itself.
    // Without an explicit completion path the marker would stay pinned to
    // the previous revision and the unread indicator could never be
    // cleared. The user is looking at everything this filter can show, so
    // the revision is confirmed with no ordinal claimed.
    if (displayItemsAsc.length === 0) {
      readProgress.markRead(sessionId, token, null);
      return;
    }

    // Older history that is not loaded yet may hold the real boundary,
    // so the visible window cannot confirm the revision.
    if (messages.hasOlder) return;

    const ordinals = displayedOrdinalsAsc;
    const latest = ordinals[ordinals.length - 1];
    if (latest === undefined || !seenOrdinals.has(latest)) return;
    if (
      unreadBoundaryOrdinal !== null &&
      !seenOrdinals.has(unreadBoundaryOrdinal)
    ) {
      return;
    }

    readProgress.markRead(sessionId, token, latest);
  }

  function queueReadProgressCheck() {
    if (readCheckRaf !== null) return;
    readCheckRaf = requestAnimationFrame(() => {
      readCheckRaf = null;
      checkReadProgress();
    });
  }

  // Re-check whenever the transcript, its revision, the sort order or the
  // active filters change what is on screen.
  $effect(() => {
    void readToken;
    void messages.sessionId;
    void messages.messages.length;
    void displayItemsAsc.length;
    void ui.sortNewestFirst;
    void ui.hasBlockFilters;
    void ui.transcriptMode;
    queueReadProgressCheck();
  });

  function publishVisibleTimestamp() {
    const v = virtualizer.instance;
    if (!v) return;
    const items = v.getVirtualItems();
    // Skip overscanned items above the viewport.
    const scrollTop = v.scrollOffset ?? 0;
    for (const vi of items) {
      if (vi.end <= scrollTop) continue;
      const item =
        displayItemsAsc[
          ui.sortNewestFirst
            ? displayItemsAsc.length - 1 - vi.index
            : vi.index
        ];
      if (!item) continue;
      const ts =
        item.kind === "message"
          ? item.message.timestamp
          : item.timestamp;
      if (ts) {
        sessionActivity.firstVisibleTimestamp = ts;
        return;
      }
    }
    sessionActivity.firstVisibleTimestamp = null;
  }

  // Recompute visible timestamp when minimap opens or
  // message content changes (e.g. SSE reload).
  $effect(() => {
    if (ui.vitalsOpen) {
      // Track message array so the effect re-runs after
      // content changes while the minimap is open.
      void messages.messages.length;
      publishVisibleTimestamp();
    }
  });

  function handleScroll() {
    if (!containerRef) return;
    if (scrollRaf !== null) return;
    scrollRaf = requestAnimationFrame(() => {
      scrollRaf = null;
      if (!containerRef) return;
      const items =
        virtualizer.instance?.getVirtualItems() ?? [];
      if (items.length > 0 && messages.hasOlder) {
        const firstVisible = items[0]!.index;
        const lastVisible =
          items[items.length - 1]!.index;
        const threshold = 30;
        if (
          (ui.sortNewestFirst &&
            lastVisible >=
              displayItemsAsc.length - threshold) ||
          (!ui.sortNewestFirst &&
            firstVisible <= threshold)
        ) {
          messages.loadOlder();
        }
      }

      if (ui.vitalsOpen) {
        publishVisibleTimestamp();
      }

      checkReadProgress();
    });
  }

  function handleManualScrollIntent() {
    if (ui.followLatest) {
      cancelFollowLatestWork();
      ui.setFollowLatest(false);
    }
  }

  function manualScrollIntent(node: HTMLElement) {
    const handleKeydown = (event: KeyboardEvent) => {
      if (
        [
          "ArrowDown",
          "ArrowUp",
          "End",
          "Home",
          "PageDown",
          "PageUp",
          " ",
        ].includes(event.key)
      ) {
        handleManualScrollIntent();
      }
    };
    node.addEventListener("wheel", handleManualScrollIntent, {
      passive: true,
    });
    node.addEventListener("pointerdown", handleManualScrollIntent);
    node.addEventListener("touchmove", handleManualScrollIntent, {
      passive: true,
    });
    node.addEventListener("keydown", handleKeydown);
    return {
      destroy() {
        node.removeEventListener(
          "wheel",
          handleManualScrollIntent,
        );
        node.removeEventListener(
          "pointerdown",
          handleManualScrollIntent,
        );
        node.removeEventListener(
          "touchmove",
          handleManualScrollIntent,
        );
        node.removeEventListener("keydown", handleKeydown);
      },
    };
  }

  onDestroy(() => {
    if (scrollRaf !== null) {
      cancelAnimationFrame(scrollRaf);
      scrollRaf = null;
    }
    if (followingScrollRaf !== null) {
      cancelAnimationFrame(followingScrollRaf);
      followingScrollRaf = null;
    }
    if (followSettleTimer !== null) {
      clearTimeout(followSettleTimer);
      followSettleTimer = null;
    }
    if (readCheckRaf !== null) {
      cancelAnimationFrame(readCheckRaf);
      readCheckRaf = null;
    }
  });

  function cancelFollowLatestWork() {
    if (
      activeFollowScrollRequest !== null &&
      activeFollowScrollRequest === lastScrollRequest
    ) {
      lastScrollRequest += 1;
    }
    activeFollowScrollRequest = null;
    if (followingScrollRaf !== null) {
      cancelAnimationFrame(followingScrollRaf);
      followingScrollRaf = null;
    }
    if (followSettleTimer !== null) {
      clearTimeout(followSettleTimer);
      followSettleTimer = null;
    }
  }

  function scrollToDisplayIndex(
    index: number,
    waitFrames: number = 0,
    scrollRetries: number = 0,
    reqId: number = lastScrollRequest,
    align: ScrollAlign = "start",
  ) {
    if (reqId !== lastScrollRequest) return;

    const v = virtualizer.instance;
    if (!v) return;

    // Phase 1: wait up to 5 frames for virtualCount to sync.
    const desiredCount = displayItemsAsc.length;
    const virtualCount = v.options.count;
    if (
      waitFrames < 5 &&
      (virtualCount !== desiredCount || index >= virtualCount)
    ) {
      requestAnimationFrame(() => {
        scrollToDisplayIndex(
          index, waitFrames + 1, 0, reqId,
          align,
        );
      });
      return;
    }

    // Phase 2a: item already rendered — use exact measured offset.
    const virtualItems = v.getVirtualItems();
    const isRendered = virtualItems.some(
      (vi) => vi.index === index,
    );
    if (isRendered) {
      const offsetAndAlign =
        v.getOffsetForIndex(index, align);
      if (offsetAndAlign) {
        const [offset] = offsetAndAlign;
        v.scrollToOffset(
          Math.round(offset),
          { align: getAlignedOffsetScrollAlign(align) },
        );
      }
      return;
    }

    // Phase 2b: item not yet in render window. scrollToIndex
    // scrolls to an estimated position, but TanStack's reconcile
    // loop exits after 1 stable frame — before ResizeObserver
    // measurements (delayed by bumpVersion's setTimeout(0)) have
    // updated the offsets.
    //
    // Retry in 2 frames: by then ResizeObserver + bumpVersion have
    // fired, measurements are updated, and the next attempt either
    // finds the item rendered (for an exact offset scroll) or
    // repeats with a more accurate estimate. Limit to 15 scroll
    // retries (~480 ms) to avoid looping forever.
    v.scrollToIndex(index, { align });
    if (scrollRetries < 15) {
      requestAnimationFrame(() => {
        requestAnimationFrame(() => {
          scrollToDisplayIndex(
            index,
            waitFrames,
            scrollRetries + 1,
            reqId,
            align,
          );
        });
      });
    }
  }

  function raf(): Promise<void> {
    return new Promise((r) => requestAnimationFrame(() => r()));
  }

  async function scrollToOrdinalInternal(ordinal: number) {
    const reqId = ++lastScrollRequest;
    activeFollowScrollRequest = null;

    const idxAsc = displayItemsAsc.findIndex((item) =>
      item.ordinals.includes(ordinal),
    );
    if (idxAsc >= 0) {
      const idx = ui.sortNewestFirst
        ? displayItemsAsc.length - 1 - idxAsc
        : idxAsc;
      scrollToDisplayIndex(idx, 0, 0, reqId);
      return;
    }

    await messages.ensureOrdinalLoaded(ordinal);
    if (reqId !== lastScrollRequest) return;

    // Let Svelte re-derive displayItemsAsc and the
    // virtualizer update its count after loading.
    // Two frames: one for Svelte reactivity, one for
    // virtualizer resize observation.
    await raf();
    await raf();
    if (reqId !== lastScrollRequest) return;

    const loadedIdxAsc = displayItemsAsc.findIndex(
      (item) => item.ordinals.includes(ordinal),
    );
    if (loadedIdxAsc < 0) return;
    const loadedIdx = ui.sortNewestFirst
      ? displayItemsAsc.length - 1 - loadedIdxAsc
      : loadedIdxAsc;
    scrollToDisplayIndex(loadedIdx, 0, 0, reqId);
  }

  export function scrollToOrdinal(ordinal: number) {
    void scrollToOrdinalInternal(ordinal);
  }

  function scrollToLatestInternal() {
    const reqId = ++lastScrollRequest;
    activeFollowScrollRequest = reqId;
    const idx = getLatestDisplayIndex(
      displayItemsAsc.length,
      ui.sortNewestFirst,
    );
    if (idx < 0) return;
    scrollToDisplayIndex(
      idx,
      0,
      0,
      reqId,
      ui.sortNewestFirst ? "start" : "end",
    );
    startFollowLatestSettle(reqId);
  }

  function forceLatestEdge() {
    if (!containerRef) return;
    containerRef.scrollTop = ui.sortNewestFirst
      ? 0
      : containerRef.scrollHeight;
  }

  function startFollowLatestSettle(reqId: number) {
    if (followSettleTimer !== null) {
      clearTimeout(followSettleTimer);
      followSettleTimer = null;
    }

    const tick = () => {
      followSettleTimer = null;
      if (
        reqId !== lastScrollRequest ||
        !ui.followLatest ||
        !containerRef
      ) {
        return;
      }

      forceLatestEdge();
      followSettleTimer = setTimeout(tick, 100);
    };

    tick();
  }

  function queueFollowLatestScroll() {
    if (!ui.followLatest) return;
    if (followingScrollRaf !== null) {
      cancelAnimationFrame(followingScrollRaf);
    }
    followingScrollRaf = requestAnimationFrame(() => {
      followingScrollRaf = null;
      if (!ui.followLatest) return;
      scrollToLatestInternal();
    });
  }

  function latestDisplaySignature(): string {
    const item = displayItemsAsc[displayItemsAsc.length - 1];
    if (!item) return "";
    if (item.kind === "tool-group") {
      return item.messages
        .map((m) => `${m.ordinal}:${m.content_length}:${m.timestamp}`)
        .join("|");
    }
    const m = item.message;
    return `${m.ordinal}:${m.content_length}:${m.timestamp}`;
  }

  $effect(() => {
    const follow = ui.followLatest;
    if (!follow) {
      cancelFollowLatestWork();
    }
  });

  $effect(() => {
    const follow = ui.followLatest;
    const request = ui.followLatestRequest;
    const count = displayItemsAsc.length;
    const latest = latestDisplaySignature();
    const newestFirst = ui.sortNewestFirst;
    const sessionId = messages.sessionId;
    if (!follow || count === 0 || !sessionId) return;
    void request;
    void latest;
    void newestFirst;
    queueFollowLatestScroll();
  });

  export function scrollToLatest() {
    scrollToLatestInternal();
  }

  export function getDisplayItems(): DisplayItem[] {
    return displayItemsAsc;
  }

  export function getNormalDisplayItems(): DisplayItem[] {
    return normalDisplayItemsAsc;
  }

  let highlightQuery = $derived(
    inSessionSearch.isOpen && inSessionSearch.query.trim().length > 0
      ? inSessionSearch.query
      : "",
  );

  /**
   * Layout actually applied to the transcript. Skim hides the content that
   * search auto-expands, so an active highlight suspends it back to the full
   * layout without touching the stored preference.
   */
  let effectiveLayout = $derived(
    resolveMessageLayout(ui.messageLayout, highlightQuery !== ""),
  );
</script>

{#if !sessions.activeSessionId}
  <div class="empty-state">
    <div class="empty-icon">
      <MessageSquareIcon size="36" strokeWidth="1.5" aria-hidden="true" />
    </div>
    <p class="empty-text">Select a session to view messages</p>
  </div>
{:else if messages.loading && messages.messages.length === 0}
  <div class="empty-state">
    <p class="empty-text">Loading messages...</p>
  </div>
{:else}
  <SessionFindBar />
  <div
    class="message-list-scroll layout-{effectiveLayout}"
    bind:this={containerRef}
    data-session-id={sessions.activeSessionId}
    data-messages-session-id={messages.sessionId}
    data-loaded={!messages.loading}
    onscroll={handleScroll}
    use:manualScrollIntent
  >
    <div
      style="height: {virtualizer.instance?.getTotalSize() ?? 0}px; width: 100%; position: relative;"
    >
      {#each virtualizer.instance?.getVirtualItems() ?? [] as row (row.key)}
        {@const item = itemAt(row.index)}
        {#if item}
          <!-- svelte-ignore a11y_click_events_have_key_events -->
          <!-- svelte-ignore a11y_no_static_element_interactions -->
          <div
            class="virtual-row"
            class:selected={ui.selectedOrdinal !== null &&
              item.ordinals.includes(ui.selectedOrdinal)}
            data-index={row.index}
            style="position: absolute; top: 0; left: 0; width: 100%; transform: translateY({row.start}px);"
            use:measureElement={virtualizer.instance}
            onclick={() => {
              const sel = window.getSelection();
              if (sel && sel.toString().length > 0) return;
              ui.selectOrdinal(item.ordinals[0]!);
            }}
          >
            {#if isBoundaryItem(item) && !ui.sortNewestFirst}
              <div
                class="unread-divider"
                role="separator"
                aria-label="Read progress boundary"
              >
                <span class="unread-divider-label">
                  {unreadDividerLabel}
                </span>
              </div>
            {/if}
            {#if item.kind === "tool-group"}
              <ToolCallGroup
                messages={item.messages}
                timestamp={item.timestamp}
                session={sessions.activeSession}
                highlightQuery={highlightQuery}
                isCurrentHighlight={item.ordinals.includes(inSessionSearch.currentOrdinal ?? -1)}
                unreadOrdinal={unreadBoundaryOrdinal}
              />
            {:else if item.message.is_compact_boundary}
              <CompactBoundaryDivider message={item.message} />
            {:else if isContextEventMessage(item.message)}
              <SystemBoundaryCard
                subtype={contextEventSubtype(item.message)}
                content={item.message.content}
                timestamp={item.message.timestamp}
              />
            {:else}
              <MessageContent
                message={item.message}
                highlightQuery={highlightQuery}
                isCurrentHighlight={inSessionSearch.currentOrdinal === item.message.ordinal}
              />
            {/if}
            {#if isBoundaryItem(item) && ui.sortNewestFirst}
              <div
                class="unread-divider"
                role="separator"
                aria-label="Read progress boundary"
              >
                <span class="unread-divider-label">
                  {unreadDividerLabel}
                </span>
              </div>
            {/if}
          </div>
        {/if}
      {/each}
    </div>
  </div>
{/if}

<style>
  .message-list-scroll {
    flex: 1;
    overflow-y: auto;
    overflow-x: hidden;
    padding: 8px 0;
    overflow-anchor: none;
  }

  .virtual-row {
    padding: 5px 12px;
    overflow-anchor: none;
  }

  .unread-divider {
    display: flex;
    align-items: center;
    gap: 8px;
    margin: 4px 0 8px;
    color: var(--accent-blue);
    font-size: 11px;
    font-weight: 600;
  }

  .unread-divider::before,
  .unread-divider::after {
    content: "";
    flex: 1;
    height: 1px;
    background: var(--accent-blue);
    opacity: 0.5;
  }

  .virtual-row.selected > :global(*) {
    outline: 2px solid var(--accent-blue);
    outline-offset: -2px;
    border-radius: var(--radius-md, 6px);
  }

  .empty-state {
    flex: 1;
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    color: var(--text-muted);
    gap: 12px;
  }

  .empty-icon {
    opacity: 0.25;
  }

  .empty-text {
    font-size: 14px;
    font-weight: 500;
  }

  /* ── Compact layout ── */
  .layout-compact {
    padding: 4px 0;
  }

  .layout-compact .virtual-row {
    padding: 2px 12px;
  }

  .layout-compact :global(.message) {
    padding: 6px 12px;
    border-left-width: 2px;
    border-radius: 0;
  }

  .layout-compact :global(.message-header) {
    margin-bottom: 4px;
    gap: 6px;
  }

  .layout-compact :global(.role-icon) {
    width: 16px;
    height: 16px;
    font-size: 9px;
  }

  .layout-compact :global(.role-label) {
    font-size: 10px;
    text-transform: uppercase;
    letter-spacing: 0.04em;
    font-weight: 700;
  }

  .layout-compact :global(.timestamp),
  .layout-compact :global(.group-timestamp) {
    font-size: 10px;
  }

  .layout-compact :global(.text-content) {
    font-size: 13px;
    line-height: 1.55;
  }

  .layout-compact :global(.message-body) {
    gap: 4px;
  }

  /* ── Stream layout ── */
  .layout-stream {
    padding: 0;
  }

  .layout-stream .virtual-row {
    padding: 0;
  }

  .layout-stream :global(.message) {
    border-left: none;
    border-radius: 0;
    padding: 16px 24px;
  }

  .layout-stream :global(.message.is-user) {
    background: color-mix(
      in srgb,
      var(--accent-blue) 5%,
      transparent
    ) !important;
  }

  .layout-stream :global(.message:not(.is-user)) {
    background: transparent !important;
  }

  .layout-stream :global(.message-header) {
    display: none;
  }

  .layout-stream :global(.text-content) {
    font-size: 14px;
    line-height: 1.75;
  }

  /* ── Skim layout ── */
  .layout-skim {
    padding: 0;
  }

  .layout-skim .virtual-row {
    padding: 0;
  }

  .layout-skim :global(.message-header) {
    display: none;
  }

  .layout-skim :global(.tool-block) {
    border-left: none;
    border-radius: 0;
  }

  .layout-skim :global(.tool-chevron) {
    display: none;
  }

  /* Keep the one-line summary, but make the header non-interactive so a
     click cannot silently toggle hidden collapse state (which would
     surprise the user on switching back to a full layout). Row selection
     still works via the virtual-row wrapper behind the header. */
  .layout-skim :global(.tool-header) {
    padding: 1px 12px;
    pointer-events: none;
  }

  /* The copy affordance is a sibling of `.tool-header`, not a child, so the
     `pointer-events: none` above never reached it: on hover (and permanently
     on coarse pointers) it stayed visible and its handler calls
     stopPropagation, so a click on the right of a skim row copied instead of
     selecting the row. */
  .layout-skim :global(.tool-copy) {
    display: none;
  }

  .layout-skim :global(.tool-meta),
  .layout-skim :global(.tool-content),
  .layout-skim :global(.diff-view),
  .layout-skim :global(.output-header),
  .layout-skim :global(.history-header),
  .layout-skim :global(.result-history),
  .layout-skim :global(.show-more-btn) {
    display: none;
  }

  .layout-skim :global(.tool-group-header),
  .layout-skim :global(.pg-header) {
    display: none;
  }

  .layout-skim :global(.tool-group),
  .layout-skim :global(.parallel-group) {
    border: none;
    margin: 0;
    padding: 0;
    background: transparent;
  }

  .layout-skim :global(.subagent-inline) {
    display: none;
  }
</style>
