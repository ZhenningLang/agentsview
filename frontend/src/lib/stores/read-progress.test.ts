import {
  afterEach,
  beforeEach,
  describe,
  expect,
  it,
  vi,
} from "vitest";

const STORAGE_KEY = "agentsview-read-progress";

type ReadProgressModule =
  typeof import("./read-progress.svelte.js");

/** Load a fresh copy of the module so the singleton re-reads
 *  localStorage for every case. */
async function loadModule(): Promise<ReadProgressModule> {
  vi.resetModules();
  return await import("./read-progress.svelte.js");
}

/** Deterministic clock so touched_at ordering is assertable. */
function stepClock(start = 1_000): () => number {
  let value = start;
  return () => ++value;
}

const realStorage = globalThis.localStorage;

function installStorage(impl: Partial<Storage>) {
  Object.defineProperty(globalThis, "localStorage", {
    value: impl,
    writable: true,
    configurable: true,
  });
}

function restoreStorage() {
  Object.defineProperty(globalThis, "localStorage", {
    value: realStorage,
    writable: true,
    configurable: true,
  });
}

function storedPayload(): unknown {
  const raw = globalThis.localStorage.getItem(STORAGE_KEY);
  return raw === null ? null : JSON.parse(raw);
}

describe("Phase 20 read progress store", () => {
  beforeEach(() => {
    restoreStorage();
    globalThis.localStorage.clear();
  });

  afterEach(() => {
    restoreStorage();
    globalThis.localStorage.clear();
    vi.restoreAllMocks();
  });

  describe("token contract", () => {
    it("ignores sessions without a transcript revision token", async () => {
      const { createReadProgressStore } = await loadModule();
      const store = createReadProgressStore();

      store.baseline("s1", null);
      store.baseline("s2", "");
      store.markRead("s3", undefined, 4);

      expect(store.size).toBe(0);
      expect(store.hasUnread("s1", null)).toBe(false);
      expect(store.hasUnread("s2", "")).toBe(false);
      expect(storedPayload()).toBeNull();
    });

    it("never falls back to a modified timestamp as the token", async () => {
      const { createReadProgressStore } = await loadModule();
      const store = createReadProgressStore();

      store.baseline("s1", "7");
      // A metadata-only refresh keeps the same revision token.
      expect(store.hasUnread("s1", "7")).toBe(false);
      // No token at all must never resurrect an unread state.
      expect(store.hasUnread("s1", null)).toBe(false);
      expect(store.markerFor("s1")?.token).toBe("7");
    });
  });

  describe("baseline", () => {
    it("treats a first visit as read instead of unread", async () => {
      const { createReadProgressStore } = await loadModule();
      const store = createReadProgressStore();

      expect(store.hasUnread("fresh", "12")).toBe(false);

      store.baseline("fresh", "12", 40);

      expect(store.hasUnread("fresh", "12")).toBe(false);
      expect(store.markerFor("fresh")).toMatchObject({
        token: "12",
        ordinal: 40,
      });
    });

    it("does not overwrite an existing unread marker", async () => {
      const { createReadProgressStore } = await loadModule();
      const store = createReadProgressStore();

      store.baseline("s1", "1", 10);
      store.baseline("s1", "2", 99);

      expect(store.markerFor("s1")).toMatchObject({
        token: "1",
        ordinal: 10,
      });
      expect(store.hasUnread("s1", "2")).toBe(true);
    });

    it("refreshes touched_at when re-seen at the same token", async () => {
      const { createReadProgressStore } = await loadModule();
      const store = createReadProgressStore({ now: stepClock() });

      store.baseline("s1", "1", 10);
      const first = store.markerFor("s1")!.touched_at;
      store.baseline("s1", "1", 10);
      const second = store.markerFor("s1")!.touched_at;

      expect(second).toBeGreaterThan(first);
      expect(store.markerFor("s1")).toMatchObject({
        token: "1",
        ordinal: 10,
      });
    });
  });

  describe("unread flip", () => {
    it("reports unread only after the revision token changes", async () => {
      const { createReadProgressStore } = await loadModule();
      const store = createReadProgressStore();

      store.baseline("s1", "3", 12);
      expect(store.hasUnread("s1", "3")).toBe(false);

      expect(store.hasUnread("s1", "4")).toBe(true);

      store.markRead("s1", "4", 18);
      expect(store.hasUnread("s1", "4")).toBe(false);
    });
  });

  describe("ordinal monotonicity", () => {
    it("advances but never rewinds within one revision", async () => {
      const { createReadProgressStore } = await loadModule();
      const store = createReadProgressStore();

      store.baseline("s1", "1", 5);
      store.advanceOrdinal("s1", 9);
      expect(store.lastReadOrdinal("s1")).toBe(9);

      store.advanceOrdinal("s1", 2);
      expect(store.lastReadOrdinal("s1")).toBe(9);

      store.markRead("s1", "1", 4);
      expect(store.lastReadOrdinal("s1")).toBe(9);
    });

    it("adopts the new ordinal when the revision token changes", async () => {
      const { createReadProgressStore } = await loadModule();
      const store = createReadProgressStore();

      store.baseline("s1", "1", 30);
      // An earlier rewrite can shrink the transcript; the ordinal must
      // follow the new revision rather than staying pinned high.
      store.markRead("s1", "2", 7);

      expect(store.markerFor("s1")).toMatchObject({
        token: "2",
        ordinal: 7,
      });
    });

    it("ignores advances for sessions with no marker", async () => {
      const { createReadProgressStore } = await loadModule();
      const store = createReadProgressStore();

      store.advanceOrdinal("ghost", 12);

      expect(store.size).toBe(0);
      expect(store.lastReadOrdinal("ghost")).toBeNull();
    });

    it("ignores non-finite ordinals", async () => {
      const { createReadProgressStore } = await loadModule();
      const store = createReadProgressStore();

      store.baseline("s1", "1", 5);
      store.advanceOrdinal("s1", Number.NaN);
      store.advanceOrdinal("s1", Number.POSITIVE_INFINITY);

      expect(store.lastReadOrdinal("s1")).toBe(5);
    });
  });

  describe("clear and reset", () => {
    it("clears one session and resets everything", async () => {
      const { createReadProgressStore } = await loadModule();
      const store = createReadProgressStore();

      store.baseline("s1", "1", 1);
      store.baseline("s2", "1", 1);

      store.clear("s1");
      expect(store.markerFor("s1")).toBeNull();
      expect(store.markerFor("s2")).not.toBeNull();

      store.reset();
      expect(store.size).toBe(0);
      expect(globalThis.localStorage.getItem(STORAGE_KEY)).toBeNull();
    });
  });

  describe("persistence", () => {
    it("round trips markers through a versioned payload", async () => {
      const first = await loadModule();
      expect(first.READ_PROGRESS_VERSION).toBe(2);
      first.readProgress.baseline("s1", "5", 21);

      expect(storedPayload()).toMatchObject({
        version: 2,
        sessions: {
          s1: { token: "5", ordinal: 21 },
        },
      });

      const second = await loadModule();
      expect(second.readProgress.hasUnread("s1", "5")).toBe(false);
      expect(second.readProgress.hasUnread("s1", "6")).toBe(true);
      expect(second.readProgress.lastReadOrdinal("s1")).toBe(21);
    });

    it("discards a version 1 payload and re-baselines", async () => {
      globalThis.localStorage.setItem(
        STORAGE_KEY,
        JSON.stringify({
          version: 1,
          sessions: { s1: { token: "1", ordinal: 3, touched_at: 1 } },
        }),
      );

      const { readProgress } = await loadModule();

      expect(readProgress.size).toBe(0);
      expect(readProgress.hasUnread("s1", "9")).toBe(false);
    });

    it("discards malformed JSON without throwing", async () => {
      globalThis.localStorage.setItem(STORAGE_KEY, "{not json");

      const { readProgress } = await loadModule();

      expect(readProgress.size).toBe(0);
      expect(() => readProgress.baseline("s1", "1", 0)).not.toThrow();
      expect(readProgress.markerFor("s1")?.token).toBe("1");
    });

    it("drops entries whose shape does not match the marker contract", async () => {
      globalThis.localStorage.setItem(
        STORAGE_KEY,
        JSON.stringify({
          version: 2,
          sessions: {
            good: { token: "1", ordinal: 4, touched_at: 10 },
            noToken: { ordinal: 4, touched_at: 10 },
            badOrdinal: { token: "1", ordinal: "4", touched_at: 10 },
            notAnObject: 7,
          },
        }),
      );

      const { readProgress } = await loadModule();

      expect(readProgress.size).toBe(1);
      expect(readProgress.markerFor("good")?.ordinal).toBe(4);
      expect(readProgress.markerFor("noToken")).toBeNull();
      expect(readProgress.markerFor("badOrdinal")).toBeNull();
      expect(readProgress.markerFor("notAnObject")).toBeNull();
    });
  });

  describe("bounded storage", () => {
    it("keeps at most 500 markers, evicting the least recently touched", async () => {
      const { createReadProgressStore, READ_PROGRESS_MAX_ENTRIES } =
        await loadModule();
      expect(READ_PROGRESS_MAX_ENTRIES).toBe(500);

      const store = createReadProgressStore({ now: stepClock() });
      for (let i = 0; i < READ_PROGRESS_MAX_ENTRIES + 25; i++) {
        store.markRead(`s${i}`, "1", i);
      }

      expect(store.size).toBe(READ_PROGRESS_MAX_ENTRIES);
      expect(store.markerFor("s0")).toBeNull();
      expect(store.markerFor("s24")).toBeNull();
      expect(store.markerFor("s25")).not.toBeNull();
      expect(store.markerFor("s524")).not.toBeNull();

      const payload = storedPayload() as {
        sessions: Record<string, unknown>;
      };
      expect(Object.keys(payload.sessions)).toHaveLength(
        READ_PROGRESS_MAX_ENTRIES,
      );
    });

    it("prunes an oversized stored payload on load", async () => {
      const sessions: Record<string, unknown> = {};
      for (let i = 0; i < 640; i++) {
        sessions[`s${i}`] = { token: "1", ordinal: i, touched_at: i };
      }
      globalThis.localStorage.setItem(
        STORAGE_KEY,
        JSON.stringify({ version: 2, sessions }),
      );

      const { readProgress } = await loadModule();

      expect(readProgress.size).toBe(500);
      expect(readProgress.markerFor("s0")).toBeNull();
      expect(readProgress.markerFor("s639")).not.toBeNull();
    });
  });

  describe("storage failures fail open", () => {
    it("survives a throwing getItem at load", async () => {
      installStorage({
        getItem: () => {
          throw new DOMException("SecurityError");
        },
        setItem: () => {},
        removeItem: () => {},
        clear: () => {},
      });

      const { readProgress } = await loadModule();

      expect(readProgress.size).toBe(0);
      expect(() => readProgress.baseline("s1", "1", 2)).not.toThrow();
      expect(readProgress.markerFor("s1")?.token).toBe("1");
    });

    it("keeps in-memory state when setItem throws a quota error", async () => {
      const setItem = vi.fn(() => {
        throw new DOMException("QuotaExceededError");
      });
      installStorage({
        getItem: () => null,
        setItem,
        removeItem: () => {},
        clear: () => {},
      });

      const { createReadProgressStore } = await loadModule();
      const store = createReadProgressStore();

      expect(() => store.markRead("s1", "3", 9)).not.toThrow();
      expect(setItem).toHaveBeenCalled();
      expect(store.hasUnread("s1", "3")).toBe(false);
      expect(store.hasUnread("s1", "4")).toBe(true);
      expect(store.lastReadOrdinal("s1")).toBe(9);
    });

    it("survives a throwing removeItem on reset", async () => {
      installStorage({
        getItem: () => null,
        setItem: () => {},
        removeItem: () => {
          throw new DOMException("SecurityError");
        },
        clear: () => {},
      });

      const { createReadProgressStore } = await loadModule();
      const store = createReadProgressStore();
      store.baseline("s1", "1", 0);

      expect(() => store.reset()).not.toThrow();
      expect(store.size).toBe(0);
    });

    it("survives a missing localStorage entirely", async () => {
      Object.defineProperty(globalThis, "localStorage", {
        value: undefined,
        writable: true,
        configurable: true,
      });

      const { createReadProgressStore } = await loadModule();
      const store = createReadProgressStore();

      expect(() => store.baseline("s1", "1", 0)).not.toThrow();
      expect(store.hasUnread("s1", "2")).toBe(true);
    });
  });
});
