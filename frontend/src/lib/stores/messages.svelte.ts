import {
  SessionsService,
} from "../api/generated/index";
import type {
  Message,
  MessagesResponse,
  Session,
  ToolCall,
  ToolResultEvent,
} from "../api/types.js";
import {
  configureGeneratedClient,
  isAbortError,
  withAbort,
} from "../api/runtime.js";
import { clearContentCaches } from "../utils/content-parser.js";
import { computeMainModel } from "../utils/model.js";
import { readProgress } from "./read-progress.svelte.js";

const MESSAGE_PAGE_SIZE = 1000;
const FULL_SESSION_MESSAGE_THRESHOLD = 3_000;

interface FetchPageOptions {
  from: number;
  limit: number;
  direction: "asc" | "desc";
  signal: AbortSignal;
}

function normalizeToken(
  token: string | null | undefined,
): string | null {
  if (typeof token !== "string") return null;
  const trimmed = token.trim();
  return trimmed === "" ? null : trimmed;
}

function resultEventsEqual(
  a: ToolResultEvent[] | undefined,
  b: ToolResultEvent[] | undefined,
): boolean {
  const left = a ?? [];
  const right = b ?? [];
  if (left.length !== right.length) return false;
  for (let i = 0; i < left.length; i++) {
    const x = left[i]!;
    const y = right[i]!;
    if (
      x.tool_use_id !== y.tool_use_id ||
      x.agent_id !== y.agent_id ||
      x.subagent_session_id !== y.subagent_session_id ||
      x.source !== y.source ||
      x.status !== y.status ||
      x.content !== y.content ||
      x.timestamp !== y.timestamp ||
      x.event_index !== y.event_index
    ) {
      return false;
    }
  }
  return true;
}

function toolCallsEqual(
  a: ToolCall[] | undefined,
  b: ToolCall[] | undefined,
): boolean {
  const left = a ?? [];
  const right = b ?? [];
  if (left.length !== right.length) return false;
  for (let i = 0; i < left.length; i++) {
    const x = left[i]!;
    const y = right[i]!;
    if (
      x.tool_name !== y.tool_name ||
      x.category !== y.category ||
      x.tool_use_id !== y.tool_use_id ||
      x.input_json !== y.input_json ||
      x.skill_name !== y.skill_name ||
      x.result_content !== y.result_content ||
      x.subagent_session_id !== y.subagent_session_id
    ) {
      return false;
    }
    if (!resultEventsEqual(x.result_events, y.result_events)) {
      return false;
    }
  }
  return true;
}

/**
 * Compare the rendered surface of two messages. Storage and sync
 * bookkeeping — row id, derived lengths, raw token payloads — is
 * deliberately excluded: a re-parse that renumbers rows must not read as
 * new content, or every sync would manufacture a fake unread marker.
 */
function visibleMessageEqual(a: Message, b: Message): boolean {
  return (
    a.role === b.role &&
    a.content === b.content &&
    a.timestamp === b.timestamp &&
    a.has_thinking === b.has_thinking &&
    a.thinking_text === b.thinking_text &&
    a.has_tool_use === b.has_tool_use &&
    a.model === b.model &&
    a.context_tokens === b.context_tokens &&
    a.output_tokens === b.output_tokens &&
    a.has_context_tokens === b.has_context_tokens &&
    a.has_output_tokens === b.has_output_tokens &&
    a.is_system === b.is_system &&
    a.is_compact_boundary === b.is_compact_boundary &&
    a.source_subtype === b.source_subtype &&
    toolCallsEqual(a.tool_calls, b.tool_calls)
  );
}

/**
 * Lowest ordinal whose visible content differs between two transcript
 * windows, or null when they render identically. Ordinals present on only
 * one side count as changed, so a shortened transcript is detected even
 * when the message count is unchanged.
 */
export function earliestChangedOrdinal(
  before: Message[],
  after: Message[],
): number | null {
  const beforeByOrdinal = new Map(
    before.map((m) => [m.ordinal, m]),
  );
  const afterOrdinals = new Set(after.map((m) => m.ordinal));

  let earliest: number | null = null;
  const consider = (ordinal: number) => {
    if (earliest === null || ordinal < earliest) {
      earliest = ordinal;
    }
  };

  for (const message of after) {
    const previous = beforeByOrdinal.get(message.ordinal);
    if (!previous || !visibleMessageEqual(previous, message)) {
      consider(message.ordinal);
    }
  }
  for (const message of before) {
    if (!afterOrdinals.has(message.ordinal)) {
      consider(message.ordinal);
    }
  }

  return earliest;
}

class MessagesStore {
  messages: Message[] = $state([]);
  loading: boolean = $state(false);
  sessionId: string | null = $state(null);
  messageCount: number = $state(0);
  hasOlder: boolean = $state(false);
  loadingOlder: boolean = $state(false);
  /** Revision token of the transcript currently rendered. Published only
   *  once the matching message window is in `messages`, so consumers can
   *  never pair a new revision with a stale window. */
  activeSessionToken: string | null = $state(null);
  /** Earliest ordinal whose visible content changed when the active token
   *  was published, or null when the boundary is unknown. */
  activeSessionUnreadOrdinal: number | null = $state(null);
  /** Token withheld until the older history it applies to is loaded. */
  private pendingToken: string | null = $state(null);
  private pendingUnreadOrdinal: number | null = $state(null);
  private _stableMainModel: string = $state("");
  mainModel: string = $derived(
    this.loading
      ? this._stableMainModel
      : this.messages.length > 0
        ? computeMainModel(this.messages)
        : "",
  );
  private abortController: AbortController | null = null;
  private reloadPromise: Promise<void> | null = null;
  private reloadSessionId: string | null = null;
  private pendingReload: boolean = false;
  private loadOlderPromise: Promise<void> | null = null;

  async loadSession(id: string) {
    if (
      this.sessionId === id &&
      (this.messages.length > 0 || this.loading)
    ) {
      return;
    }
    this.clear();
    this._stableMainModel = "";
    this.sessionId = id;
    this.loading = true;

    const ac = new AbortController();
    this.abortController = ac;

    try {
      let countHint: number | null = null;
      let token: string | null = null;
      try {
        configureGeneratedClient();
        const sess = await withAbort(
          SessionsService.getApiV1SessionsId({ id }) as unknown as Promise<Session>,
          ac.signal,
        );
        countHint = sess.message_count ?? 0;
        token = normalizeToken(sess.transcript_revision);
      } catch (err) {
        if (isAbortError(err)) return;
        console.warn(
          "Failed to fetch session metadata:",
          err,
        );
      }

      if (
        countHint !== null &&
        countHint > FULL_SESSION_MESSAGE_THRESHOLD
      ) {
        await this.loadProgressively(id, ac.signal);
      } else {
        await this.loadAllMessages(
          id,
          ac.signal,
          countHint ?? undefined,
        );
      }

      // A first open has no previous window to diff against, so the
      // unread boundary stays unknown and consumers fall back to the
      // oldest displayed message.
      this.publishToken(id, token, null);
    } catch (err) {
      if (isAbortError(err)) return;
      console.warn("Failed to load session messages:", err);
    } finally {
      if (this.sessionId === id) {
        this.loading = false;
        this._stableMainModel =
          this.messages.length > 0
            ? computeMainModel(this.messages)
            : "";
      }
    }
  }

  reload(): Promise<void> {
    if (!this.sessionId) return Promise.resolve();

    if (
      this.reloadPromise &&
      this.reloadSessionId === this.sessionId
    ) {
      this.pendingReload = true;
      return this.reloadPromise;
    }

    const id = this.sessionId;
    this.reloadSessionId = id;

    const promise = this.reloadNow(id).finally(async () => {
      if (this.reloadPromise === promise) {
        this.reloadPromise = null;
        this.reloadSessionId = null;
      }
      if (this.pendingReload && this.sessionId === id) {
        this.pendingReload = false;
        await this.reload();
      }
    });
    this.reloadPromise = promise;
    return promise;
  }

  clear() {
    this.abortController?.abort();
    this.abortController = null;
    this.messages = [];
    clearContentCaches();
    this.sessionId = null;
    this.loading = false;
    this._stableMainModel = "";
    this.messageCount = 0;
    this.hasOlder = false;
    this.loadingOlder = false;
    this.reloadPromise = null;
    this.reloadSessionId = null;
    this.pendingReload = false;
    this.loadOlderPromise = null;
    this.activeSessionToken = null;
    this.activeSessionUnreadOrdinal = null;
    this.pendingToken = null;
    this.pendingUnreadOrdinal = null;
  }

  /**
   * Publish a revision token now that its message window is loaded.
   *
   * A token is held back when the loaded window is only the tail of a
   * long transcript AND the stored marker is behind: confirming reads
   * from the tail alone would silently swallow an earlier rewrite the
   * user never scrolled to. A session with no marker (first visit) or an
   * already-current marker has nothing to lose, so it publishes at once
   * rather than forcing a pointless full history load.
   */
  private publishToken(
    id: string,
    token: string | null,
    unreadOrdinal: number | null,
  ) {
    if (this.sessionId !== id) return;

    if (token === null) {
      this.activeSessionToken = null;
      this.activeSessionUnreadOrdinal = null;
      this.pendingToken = null;
      this.pendingUnreadOrdinal = null;
      return;
    }

    if (this.hasOlder && readProgress.hasUnread(id, token)) {
      this.pendingToken = token;
      this.pendingUnreadOrdinal = unreadOrdinal;
      return;
    }

    this.activeSessionToken = token;
    this.activeSessionUnreadOrdinal = unreadOrdinal;
    this.pendingToken = null;
    this.pendingUnreadOrdinal = null;
  }

  /** Publish the reloaded token together with the boundary derived from
   *  the window it replaced. */
  private publishReloadedToken(
    id: string,
    token: string | null,
    previous: Message[],
  ) {
    if (this.sessionId !== id) return;
    this.publishToken(
      id,
      token,
      earliestChangedOrdinal(previous, this.messages),
    );
  }

  /** Release a withheld token once the whole history is loaded. */
  private flushPendingToken(id: string) {
    if (this.sessionId !== id) return;
    if (this.pendingToken === null) return;
    if (this.hasOlder) return;

    this.activeSessionToken = this.pendingToken;
    this.activeSessionUnreadOrdinal = this.pendingUnreadOrdinal;
    this.pendingToken = null;
    this.pendingUnreadOrdinal = null;
  }

  private async fetchPages(
    id: string,
    opts: FetchPageOptions,
  ): Promise<Message[]> {
    const loaded: Message[] = [];
    let from = opts.from;

    for (;;) {
      configureGeneratedClient();
      const res = await withAbort(
        SessionsService.getApiV1SessionsIdMessages({
          id,
          from,
          limit: opts.limit,
          direction: opts.direction,
        }) as unknown as Promise<MessagesResponse>,
        opts.signal,
      );
      if (res.messages.length === 0) break;

      loaded.push(...res.messages);

      if (res.messages.length < opts.limit) break;
      const last = res.messages[res.messages.length - 1];
      if (!last) break;

      const nextFrom =
        opts.direction === "asc"
          ? last.ordinal + 1
          : last.ordinal - 1;
      if (
        opts.direction === "asc"
          ? nextFrom <= from
          : nextFrom >= from
      ) {
        break;
      }
      from = nextFrom;
    }

    return loaded;
  }

  private async loadAllMessages(
    id: string,
    signal: AbortSignal,
    messageCountHint?: number,
  ) {
    let from = 0;
    let loaded: Message[] = [];

    for (;;) {
      configureGeneratedClient();
      const res = await withAbort(
        SessionsService.getApiV1SessionsIdMessages({
          id,
          from,
          limit: MESSAGE_PAGE_SIZE,
          direction: "asc",
        }) as unknown as Promise<MessagesResponse>,
        signal,
      );
      if (res.messages.length === 0) break;

      loaded = [...loaded, ...res.messages];
      this.messages = loaded;

      const newest = loaded[loaded.length - 1];
      this.messageCount =
        messageCountHint ??
        (newest ? newest.ordinal + 1 : loaded.length);
      this.hasOlder = false;

      if (res.messages.length < MESSAGE_PAGE_SIZE) break;
      const last = res.messages[res.messages.length - 1];
      if (!last) break;
      const nextFrom = last.ordinal + 1;
      if (nextFrom <= from) break;
      from = nextFrom;
    }

    const newest = this.messages[this.messages.length - 1];
    this.messageCount =
      messageCountHint ??
      (newest ? newest.ordinal + 1 : this.messages.length);
    this.hasOlder = false;
  }

  private async loadProgressively(
    id: string,
    signal: AbortSignal,
  ) {
    configureGeneratedClient();
    const firstRes = await withAbort(
      SessionsService.getApiV1SessionsIdMessages({
        id,
        limit: MESSAGE_PAGE_SIZE,
        direction: "desc",
      }) as unknown as Promise<MessagesResponse>,
      signal,
    );

    this.messages = [...firstRes.messages].reverse();
    const newest = this.messages[this.messages.length - 1];
    this.messageCount = newest ? newest.ordinal + 1 : 0;
    const oldest = this.messages[0]?.ordinal;
    this.hasOlder =
      oldest !== undefined ? oldest > 0 : false;
  }

  private async loadFrom(
    id: string,
    from: number,
    signal: AbortSignal,
  ) {
    const pages = await this.fetchPages(id, {
      from,
      limit: MESSAGE_PAGE_SIZE,
      direction: "asc",
      signal,
    });
    if (pages.length > 0) {
      const updates = new Map(
        pages.map((m) => [m.ordinal, m]),
      );
      const existingOrdinals = new Set(
        this.messages.map((m) => m.ordinal),
      );
      const appended = pages.filter(
        (m) => !existingOrdinals.has(m.ordinal),
      );
      clearContentCaches();
      this.messages = [
        ...this.messages.map((m) => updates.get(m.ordinal) ?? m),
        ...appended,
      ];
    }
  }

  async loadOlder() {
    if (
      !this.sessionId ||
      this.loadOlderPromise ||
      !this.hasOlder ||
      this.messages.length === 0
    ) {
      return this.loadOlderPromise ?? undefined;
    }

    const p = this.doLoadOlder().finally(() => {
      if (this.loadOlderPromise === p) {
        this.loadOlderPromise = null;
      }
    });
    this.loadOlderPromise = p;
    return p;
  }

  private async doLoadOlder() {
    const id = this.sessionId;
    if (!id || this.messages.length === 0) return;

    const oldest = this.messages[0]!.ordinal;
    if (oldest <= 0) {
      this.hasOlder = false;
      return;
    }

    const signal = this.abortController?.signal;
    if (!signal || signal.aborted) return;

    this.loadingOlder = true;
    try {
      configureGeneratedClient();
      const res = await withAbort(
        SessionsService.getApiV1SessionsIdMessages({
          id,
          from: oldest - 1,
          limit: MESSAGE_PAGE_SIZE,
          direction: "desc",
        }) as unknown as Promise<MessagesResponse>,
        signal,
      );
      if (this.sessionId !== id) return;
      if (res.messages.length === 0) {
        this.hasOlder = false;
        return;
      }
      const chunk = [...res.messages].reverse();
      this.messages.unshift(...chunk);
      this.hasOlder = chunk[0]!.ordinal > 0;
      this.flushPendingToken(id);
    } catch (err) {
      if (isAbortError(err)) return;
      console.warn("Failed to load older messages:", err);
    } finally {
      if (this.sessionId === id) {
        this.loadingOlder = false;
      }
    }
  }

  async ensureOrdinalLoaded(targetOrdinal: number) {
    if (!this.sessionId || this.messages.length === 0) return;

    const id = this.sessionId;
    const oldestLoaded = this.messages[0]!.ordinal;
    if (oldestLoaded <= targetOrdinal) return;
    if (!this.hasOlder) return;

    if (this.loadOlderPromise) {
      await this.loadOlderPromise;
      if (!this.sessionId || this.sessionId !== id) return;
      if (this.messages.length === 0) return;
      if (this.messages[0]!.ordinal <= targetOrdinal) return;
    }

    const p = this.doEnsureOrdinal(
      id,
      targetOrdinal,
    ).finally(() => {
      if (this.loadOlderPromise === p) {
        this.loadOlderPromise = null;
      }
    });
    this.loadOlderPromise = p;
    return p;
  }

  private async doEnsureOrdinal(
    id: string,
    targetOrdinal: number,
  ) {
    const signal = this.abortController?.signal;
    if (!signal || signal.aborted) return;

    this.loadingOlder = true;
    try {
      let from = this.messages[0]!.ordinal - 1;
      let lastOldest = this.messages[0]!.ordinal;
      const chunks: Message[][] = [];

      while (from >= 0) {
        configureGeneratedClient();
        const res = await withAbort(
          SessionsService.getApiV1SessionsIdMessages({
            id,
            from,
            limit: MESSAGE_PAGE_SIZE,
            direction: "desc",
          }) as unknown as Promise<MessagesResponse>,
          signal,
        );
        if (this.sessionId !== id) return;
        if (res.messages.length === 0) {
          this.hasOlder = false;
          break;
        }

        const chunk = [...res.messages].reverse();
        chunks.push(chunk);
        const chunkOldest = chunk[0]!.ordinal;

        if (chunkOldest <= targetOrdinal) break;
        if (chunkOldest >= lastOldest) break;

        lastOldest = chunkOldest;
        from = chunkOldest - 1;
      }

      if (this.sessionId !== id) return;

      if (chunks.length > 0) {
        const merged = chunks.reverse().flat();
        this.messages = [...merged, ...this.messages];
      }

      const oldestNow = this.messages[0]?.ordinal;
      this.hasOlder =
        oldestNow !== undefined && oldestNow > 0;
      this.flushPendingToken(id);
    } catch (err) {
      if (isAbortError(err)) return;
      console.warn(
        "Failed to load older messages for ordinal:",
        err,
      );
    } finally {
      if (this.sessionId === id) {
        this.loadingOlder = false;
      }
    }
  }

  private async reloadNow(id: string) {
    const signal = this.abortController?.signal;
    if (!signal || signal.aborted) return;

    try {
      configureGeneratedClient();
      const sess = await withAbort(
        SessionsService.getApiV1SessionsId({ id }) as unknown as Promise<Session>,
        signal,
      );
      if (this.sessionId !== id) return;

      const newCount = sess.message_count ?? 0;
      const oldCount = this.messageCount;
      const token = normalizeToken(sess.transcript_revision);
      // Snapshot the window the user is currently looking at; the token
      // is only published after the refreshed window replaces it.
      const previous = [...this.messages];

      if (newCount === oldCount) {
        await this.refreshLoadedWindow(id, signal);
        this.publishReloadedToken(id, token, previous);
        return;
      }

      if (newCount > oldCount && this.messages.length > 0) {
        const oldestOrdinal = this.messages[0]!.ordinal;
        await this.loadFrom(id, oldestOrdinal, signal);
        if (this.sessionId !== id) return;

        const newest =
          this.messages[this.messages.length - 1];
        if (newest && newest.ordinal !== newCount - 1) {
          await this.fullReload(id, signal, newCount);
          this.publishReloadedToken(id, token, previous);
          return;
        }

        this.messageCount = newCount;
        this.publishReloadedToken(id, token, previous);
        return;
      }

      await this.fullReload(id, signal, newCount);
      this.publishReloadedToken(id, token, previous);
    } catch (err) {
      if (isAbortError(err)) return;
      console.warn("Reload failed:", err);
    }
  }

  private async refreshLoadedWindow(
    id: string,
    signal: AbortSignal,
  ) {
    const oldest = this.messages[0];
    const newest = this.messages[this.messages.length - 1];
    if (!oldest || !newest) return;

    const refreshed = await this.fetchPages(id, {
      from: oldest.ordinal,
      limit: MESSAGE_PAGE_SIZE,
      direction: "asc",
      signal,
    });
    if (this.sessionId !== id || refreshed.length === 0) {
      return;
    }

    const updates = new Map(
      refreshed
        .filter(
          (m) =>
            m.ordinal >= oldest.ordinal &&
            m.ordinal <= newest.ordinal,
        )
        .map((m) => [m.ordinal, m]),
    );
    clearContentCaches();
    this.messages = this.messages.map(
      (m) => updates.get(m.ordinal) ?? m,
    );
  }

  private async fullReload(
    id: string,
    signal: AbortSignal,
    messageCountHint?: number,
  ) {
    clearContentCaches();
    this.loading = true;
    try {
      if (
        messageCountHint !== undefined &&
        messageCountHint > FULL_SESSION_MESSAGE_THRESHOLD
      ) {
        await this.loadProgressively(id, signal);
      } else {
        await this.loadAllMessages(
          id,
          signal,
          messageCountHint,
        );
      }
    } finally {
      if (this.sessionId === id) {
        this.loading = false;
        this._stableMainModel =
          this.messages.length > 0
            ? computeMainModel(this.messages)
            : "";
      }
    }
  }
}

export const messages = new MessagesStore();
