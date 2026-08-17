/**
 * Browser-local transcript read progress.
 *
 * The backend owns a per-session `transcript_revision` counter that only
 * moves when user-visible transcript content changes. This store remembers,
 * per session, which revision the user last saw and how far into it they
 * read. Nothing here is sent to the server: read state is per browser.
 *
 * Deliberate constraints:
 *   - The change token is the revision string and nothing else. Falling back
 *     to a modified timestamp would turn renames, signal recomputation and
 *     sync bookkeeping into fake unread badges.
 *   - A session with no stored marker is "read", not "unread". The first
 *     visit establishes a baseline so an existing archive does not light up
 *     wholesale.
 *   - Storage is bounded and every localStorage access fails open: a browser
 *     that denies or exhausts storage degrades to in-memory state instead of
 *     breaking the transcript view.
 */

export const READ_PROGRESS_STORAGE_KEY = "agentsview-read-progress";

/** Storage schema version. Payloads written by any other version are
 *  discarded and re-baselined rather than migrated. */
export const READ_PROGRESS_VERSION = 2;

/** Upper bound on retained sessions; the least recently touched markers
 *  are evicted first. */
export const READ_PROGRESS_MAX_ENTRIES = 500;

export interface ReadProgressMarker {
  /** The `transcript_revision` the user last saw. */
  token: string;
  /** Highest transcript ordinal confirmed as seen for that revision. */
  ordinal: number | null;
  /** Epoch millis of the last touch; drives eviction order. */
  touched_at: number;
}

export interface ReadProgressOptions {
  /** Injectable clock, primarily so tests get deterministic ordering. */
  now?: () => number;
}

interface StoredPayload {
  version: number;
  sessions: Record<string, ReadProgressMarker>;
}

function normalizeToken(token: string | null | undefined): string | null {
  if (typeof token !== "string") return null;
  const trimmed = token.trim();
  return trimmed === "" ? null : trimmed;
}

function normalizeOrdinal(ordinal: number | null | undefined): number | null {
  if (typeof ordinal !== "number") return null;
  if (!Number.isFinite(ordinal)) return null;
  return ordinal;
}

function isMarker(value: unknown): value is ReadProgressMarker {
  if (!value || typeof value !== "object") return false;
  const candidate = value as Record<string, unknown>;
  if (typeof candidate.token !== "string" || candidate.token === "") {
    return false;
  }
  if (
    candidate.ordinal !== null &&
    (typeof candidate.ordinal !== "number" ||
      !Number.isFinite(candidate.ordinal))
  ) {
    return false;
  }
  if (
    typeof candidate.touched_at !== "number" ||
    !Number.isFinite(candidate.touched_at)
  ) {
    return false;
  }
  return true;
}

/** Keep only the most recently touched entries. Ties break on session ID so
 *  eviction is deterministic across reloads. */
function pruneEntries(
  entries: Record<string, ReadProgressMarker>,
): Record<string, ReadProgressMarker> {
  const ids = Object.keys(entries);
  if (ids.length <= READ_PROGRESS_MAX_ENTRIES) return entries;
  ids.sort((a, b) => {
    const diff = entries[b]!.touched_at - entries[a]!.touched_at;
    return diff !== 0 ? diff : (a < b ? -1 : a > b ? 1 : 0);
  });
  const kept: Record<string, ReadProgressMarker> = {};
  for (const id of ids.slice(0, READ_PROGRESS_MAX_ENTRIES)) {
    kept[id] = entries[id]!;
  }
  return kept;
}

function readStoredEntries(): Record<string, ReadProgressMarker> {
  let raw: string | null = null;
  try {
    raw = globalThis.localStorage?.getItem(READ_PROGRESS_STORAGE_KEY) ?? null;
  } catch {
    return {};
  }
  if (!raw) return {};

  let parsed: unknown;
  try {
    parsed = JSON.parse(raw);
  } catch {
    return {};
  }
  if (!parsed || typeof parsed !== "object") return {};

  const payload = parsed as Partial<StoredPayload>;
  if (payload.version !== READ_PROGRESS_VERSION) return {};
  if (!payload.sessions || typeof payload.sessions !== "object") return {};

  const entries: Record<string, ReadProgressMarker> = {};
  for (const [id, marker] of Object.entries(payload.sessions)) {
    if (!id || !isMarker(marker)) continue;
    entries[id] = {
      token: marker.token,
      ordinal: marker.ordinal ?? null,
      touched_at: marker.touched_at,
    };
  }
  return pruneEntries(entries);
}

class ReadProgressStore {
  private entries: Record<string, ReadProgressMarker> = $state({});
  private readonly now: () => number;

  constructor(options: ReadProgressOptions = {}) {
    this.now = options.now ?? (() => Date.now());
    this.entries = readStoredEntries();
  }

  get size(): number {
    return Object.keys(this.entries).length;
  }

  markerFor(sessionId: string): ReadProgressMarker | null {
    return this.entries[sessionId] ?? null;
  }

  /** Highest ordinal the user is known to have seen, or null when the
   *  session has no marker or the marker predates ordinal tracking. */
  lastReadOrdinal(sessionId: string): number | null {
    return this.entries[sessionId]?.ordinal ?? null;
  }

  /** True only when a baseline exists and the transcript has moved past it.
   *  Sessions without a revision token or without a marker are never
   *  reported unread. */
  hasUnread(sessionId: string, token: string | null | undefined): boolean {
    const next = normalizeToken(token);
    if (next === null) return false;
    const marker = this.entries[sessionId];
    if (!marker) return false;
    return marker.token !== next;
  }

  /**
   * Record a first-visit baseline. Existing markers are never overwritten:
   * a session already known at an older revision stays unread. Re-seeing a
   * session at its current revision only refreshes eviction order.
   */
  baseline(
    sessionId: string,
    token: string | null | undefined,
    ordinal: number | null = null,
  ) {
    const next = normalizeToken(token);
    if (!sessionId || next === null) return;

    const marker = this.entries[sessionId];
    if (marker) {
      if (marker.token !== next) return;
      this.write({ ...marker, touched_at: this.now() }, sessionId);
      return;
    }

    this.write(
      {
        token: next,
        ordinal: normalizeOrdinal(ordinal),
        touched_at: this.now(),
      },
      sessionId,
    );
  }

  /** Extend how far the user has read within the current revision. Never
   *  rewinds, and never creates a marker on its own. */
  advanceOrdinal(sessionId: string, ordinal: number) {
    const next = normalizeOrdinal(ordinal);
    if (next === null) return;
    const marker = this.entries[sessionId];
    if (!marker) return;
    if (marker.ordinal !== null && marker.ordinal >= next) return;
    this.write({ ...marker, ordinal: next, touched_at: this.now() }, sessionId);
  }

  /**
   * Confirm the session as read at `token`. Within one revision the ordinal
   * only grows; when the revision changes the supplied ordinal is adopted as
   * given, because an earlier rewrite can legitimately shorten a transcript.
   */
  markRead(
    sessionId: string,
    token: string | null | undefined,
    ordinal: number | null = null,
  ) {
    const next = normalizeToken(token);
    if (!sessionId || next === null) return;

    const candidate = normalizeOrdinal(ordinal);
    const marker = this.entries[sessionId];
    let resolved = candidate;
    if (marker && marker.token === next && marker.ordinal !== null) {
      resolved = candidate === null
        ? marker.ordinal
        : Math.max(marker.ordinal, candidate);
    }

    this.write(
      { token: next, ordinal: resolved, touched_at: this.now() },
      sessionId,
    );
  }

  /** Forget one session, e.g. after it is deleted. */
  clear(sessionId: string) {
    if (!(sessionId in this.entries)) return;
    const next = { ...this.entries };
    delete next[sessionId];
    this.entries = next;
    this.persist();
  }

  /** Forget everything and drop the storage key. */
  reset() {
    this.entries = {};
    try {
      globalThis.localStorage?.removeItem(READ_PROGRESS_STORAGE_KEY);
    } catch {
      // Storage unavailable — in-memory reset still stands.
    }
  }

  private write(marker: ReadProgressMarker, sessionId: string) {
    this.entries = pruneEntries({ ...this.entries, [sessionId]: marker });
    this.persist();
  }

  private persist() {
    const payload: StoredPayload = {
      version: READ_PROGRESS_VERSION,
      sessions: this.entries,
    };
    try {
      globalThis.localStorage?.setItem(
        READ_PROGRESS_STORAGE_KEY,
        JSON.stringify(payload),
      );
    } catch {
      // Quota exceeded or storage denied — keep the in-memory state so the
      // session view still behaves correctly for this page load.
    }
  }
}

export type { ReadProgressStore };

export function createReadProgressStore(
  options: ReadProgressOptions = {},
): ReadProgressStore {
  return new ReadProgressStore(options);
}

export const readProgress = createReadProgressStore();
