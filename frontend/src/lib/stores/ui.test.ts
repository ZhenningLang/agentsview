import {
  describe,
  it,
  expect,
  vi,
  beforeEach,
} from "vitest";
import { tick } from "svelte";
import {
  SIDEBAR_WIDTH_DEFAULT,
  SIDEBAR_WIDTH_KEY,
  SIDEBAR_WIDTH_MIN,
  SIDEBAR_WIDTH_STORAGE_MAX,
} from "../components/layout/sidebar-width.js";
import { FONT_SCALE_STEPS, ui } from "./ui.svelte.js";

describe("UIStore", () => {
  beforeEach(() => {
    ui.activeModal = null;
    ui.selectedOrdinal = null;
    ui.pendingScrollOrdinal = null;
    ui.followLatest = false;
    ui.followLatestRequest = 0;
  });

  describe("activeModal", () => {
    it("should default to null", () => {
      expect(ui.activeModal).toBeNull();
    });

    it("should set and clear modal type", () => {
      ui.activeModal = "commandPalette";
      expect(ui.activeModal).toBe("commandPalette");

      ui.activeModal = null;
      expect(ui.activeModal).toBeNull();
    });

    it("should switch between modal types", () => {
      ui.activeModal = "shortcuts";
      expect(ui.activeModal).toBe("shortcuts");

      ui.activeModal = "publish";
      expect(ui.activeModal).toBe("publish");
    });
  });

  describe("closeAll", () => {
    it("should set activeModal to null", () => {
      ui.activeModal = "commandPalette";
      ui.closeAll();
      expect(ui.activeModal).toBeNull();
    });

    it("should be idempotent when already null", () => {
      ui.closeAll();
      expect(ui.activeModal).toBeNull();
    });
  });

  describe("selectedOrdinal null flows", () => {
    it("should default to null", () => {
      expect(ui.selectedOrdinal).toBeNull();
    });

    it("should set ordinal via selectOrdinal", () => {
      ui.selectOrdinal(5);
      expect(ui.selectedOrdinal).toBe(5);
    });

    it("should clear to null via clearSelection", () => {
      ui.selectOrdinal(5);
      ui.clearSelection();
      expect(ui.selectedOrdinal).toBeNull();
    });

    it("should handle ordinal 0 without confusion", () => {
      ui.selectOrdinal(0);
      expect(ui.selectedOrdinal).toBe(0);
    });

    it("clearSelection should be idempotent", () => {
      ui.clearSelection();
      expect(ui.selectedOrdinal).toBeNull();
    });
  });

  describe("pendingScrollOrdinal null flows", () => {
    it("should default to null", () => {
      expect(ui.pendingScrollOrdinal).toBeNull();
    });

    it("should set both selected and pending via scrollToOrdinal", () => {
      ui.scrollToOrdinal(10);
      expect(ui.selectedOrdinal).toBe(10);
      expect(ui.pendingScrollOrdinal).toBe(10);
      expect(ui.pendingScrollSession).toBeNull();
    });

    it("should store session ID when provided", () => {
      ui.scrollToOrdinal(5, "sess-123");
      expect(ui.pendingScrollOrdinal).toBe(5);
      expect(ui.pendingScrollSession).toBe("sess-123");
    });

    it("should allow clearing pending independently", () => {
      ui.scrollToOrdinal(10);
      ui.pendingScrollOrdinal = null;
      expect(ui.pendingScrollOrdinal).toBeNull();
      expect(ui.selectedOrdinal).toBe(10);
    });

    it("should handle ordinal 0", () => {
      ui.scrollToOrdinal(0);
      expect(ui.selectedOrdinal).toBe(0);
      expect(ui.pendingScrollOrdinal).toBe(0);
    });
  });

  describe("followLatest", () => {
    it("defaults to disabled", () => {
      expect(ui.followLatest).toBe(false);
    });

    it("can be enabled and disabled", () => {
      ui.setFollowLatest(true);
      expect(ui.followLatest).toBe(true);

      ui.setFollowLatest(false);
      expect(ui.followLatest).toBe(false);
    });

    it("records a new request when already enabled", () => {
      ui.setFollowLatest(true);
      const first = ui.followLatestRequest;

      ui.setFollowLatest(true);

      expect(ui.followLatest).toBe(true);
      expect(ui.followLatestRequest).toBe(first + 1);
    });

    it("toggles follow latest mode", () => {
      ui.toggleFollowLatest();
      expect(ui.followLatest).toBe(true);
      expect(ui.followLatestRequest).toBe(1);

      ui.toggleFollowLatest();
      expect(ui.followLatest).toBe(false);
      expect(ui.followLatestRequest).toBe(1);
    });

    it("is disabled when jumping to a specific ordinal", () => {
      ui.setFollowLatest(true);
      ui.scrollToOrdinal(10);

      expect(ui.followLatest).toBe(false);
      expect(ui.pendingScrollOrdinal).toBe(10);
    });
  });

  describe("theme initialization", () => {
    it("should fall back to light when stored theme is absent", () => {
      expect(ui.theme).toBeDefined();
      expect(["light", "dark"]).toContain(ui.theme);
    });

    it("should survive when localStorage.getItem is unavailable", async () => {
      const original = globalThis.localStorage;
      // Replace with an object that lacks getItem/setItem
      Object.defineProperty(globalThis, "localStorage", {
        value: {},
        writable: true,
        configurable: true,
      });
      try {
        // @ts-expect-error -- query string busts module cache
        const mod = await import("./ui.svelte.js?noGetItem");
        expect(mod.ui.theme).toBe("light");
      } finally {
        Object.defineProperty(globalThis, "localStorage", {
          value: original,
          writable: true,
          configurable: true,
        });
      }
    });

    it("should survive when localStorage is null", async () => {
      const original = globalThis.localStorage;
      Object.defineProperty(globalThis, "localStorage", {
        value: null,
        writable: true,
        configurable: true,
      });
      try {
        // @ts-expect-error -- query string busts module cache
        const mod = await import("./ui.svelte.js?nullStorage");
        expect(mod.ui.theme).toBe("light");
      } finally {
        Object.defineProperty(globalThis, "localStorage", {
          value: original,
          writable: true,
          configurable: true,
        });
      }
    });

    it("should survive when localStorage is undefined", async () => {
      const original = globalThis.localStorage;
      // @ts-expect-error -- deliberately removing localStorage
      delete globalThis.localStorage;
      try {
        // @ts-expect-error -- query string busts module cache
        const mod = await import("./ui.svelte.js?noStorage");
        expect(mod.ui.theme).toBe("light");
      } finally {
        Object.defineProperty(globalThis, "localStorage", {
          value: original,
          writable: true,
          configurable: true,
        });
      }
    });
  });

  describe("sidebar width", () => {
    it("defaults to the helper default when storage is empty", async () => {
      const original = globalThis.localStorage;
      const getItem = vi.fn(() => null);
      const setItem = vi.fn();

      Object.defineProperty(globalThis, "localStorage", {
        value: { getItem, setItem },
        writable: true,
        configurable: true,
      });

      try {
        // @ts-expect-error -- query string busts module cache
        const mod = await import("./ui.svelte.js?sidebarWidthEmpty");
        expect(getItem.mock.calls).toContainEqual([
          SIDEBAR_WIDTH_KEY,
        ]);
        expect(mod.ui.sidebarWidth).toBe(SIDEBAR_WIDTH_DEFAULT);
      } finally {
        Object.defineProperty(globalThis, "localStorage", {
          value: original,
          writable: true,
          configurable: true,
        });
      }
    });

    it("reads and clamps stored widths including stored strings", async () => {
      const original = globalThis.localStorage;

      try {
        Object.defineProperty(globalThis, "localStorage", {
          value: {
            getItem: vi.fn((key: string) =>
              key === SIDEBAR_WIDTH_KEY
                ? String(SIDEBAR_WIDTH_MIN - 50)
                : null,
            ),
            setItem: vi.fn(),
          },
          writable: true,
          configurable: true,
        });
        // @ts-expect-error -- query string busts module cache
        const minMod = await import("./ui.svelte.js?sidebarWidthStoredMin");

        Object.defineProperty(globalThis, "localStorage", {
          value: {
            getItem: vi.fn((key: string) =>
              key === SIDEBAR_WIDTH_KEY
                ? String(SIDEBAR_WIDTH_STORAGE_MAX + 50)
                : null,
            ),
            setItem: vi.fn(),
          },
          writable: true,
          configurable: true,
        });
        // @ts-expect-error -- query string busts module cache
        const maxMod = await import("./ui.svelte.js?sidebarWidthStoredMax");

        Object.defineProperty(globalThis, "localStorage", {
          value: {
            getItem: vi.fn((key: string) =>
              key === SIDEBAR_WIDTH_KEY ? "300" : null,
            ),
            setItem: vi.fn(),
          },
          writable: true,
          configurable: true,
        });
        // @ts-expect-error -- query string busts module cache
        const stringMod = await import("./ui.svelte.js?sidebarWidthStoredString");

        expect(minMod.ui.sidebarWidth).toBe(SIDEBAR_WIDTH_MIN);
        expect(maxMod.ui.sidebarWidth).toBe(
          SIDEBAR_WIDTH_STORAGE_MAX,
        );
        expect(stringMod.ui.sidebarWidth).toBe(300);
      } finally {
        Object.defineProperty(globalThis, "localStorage", {
          value: original,
          writable: true,
          configurable: true,
        });
      }
    });

    it("persists clamped widths through setSidebarWidth", async () => {
      const original = globalThis.localStorage;
      const setItem = vi.fn();

      Object.defineProperty(globalThis, "localStorage", {
        value: {
          getItem: vi.fn(() => null),
          setItem,
        },
        writable: true,
        configurable: true,
      });

      try {
        // @ts-expect-error -- query string busts module cache
        const mod = await import("./ui.svelte.js?sidebarWidthPersist");
        setItem.mockClear();

        mod.ui.setSidebarWidth(SIDEBAR_WIDTH_MIN - 10);
        await tick();
        expect(mod.ui.sidebarWidth).toBe(SIDEBAR_WIDTH_MIN);
        expect(setItem).toHaveBeenCalledTimes(1);
        expect(setItem).toHaveBeenLastCalledWith(
          SIDEBAR_WIDTH_KEY,
          String(SIDEBAR_WIDTH_MIN),
        );

        setItem.mockClear();
        mod.ui.setSidebarWidth(SIDEBAR_WIDTH_STORAGE_MAX + 10);
        await tick();
        expect(mod.ui.sidebarWidth).toBe(
          SIDEBAR_WIDTH_STORAGE_MAX,
        );
        expect(setItem).toHaveBeenCalledTimes(1);
        expect(setItem).toHaveBeenLastCalledWith(
          SIDEBAR_WIDTH_KEY,
          String(SIDEBAR_WIDTH_STORAGE_MAX),
        );
      } finally {
        Object.defineProperty(globalThis, "localStorage", {
          value: original,
          writable: true,
          configurable: true,
        });
      }
    });

    it("survives when localStorage.getItem is unavailable", async () => {
      const original = globalThis.localStorage;

      Object.defineProperty(globalThis, "localStorage", {
        value: {
          setItem: vi.fn(),
        },
        writable: true,
        configurable: true,
      });

      try {
        // @ts-expect-error -- query string busts module cache
        const mod = await import("./ui.svelte.js?sidebarWidthNoGetItem");
        expect(mod.ui.sidebarWidth).toBe(SIDEBAR_WIDTH_DEFAULT);
      } finally {
        Object.defineProperty(globalThis, "localStorage", {
          value: original,
          writable: true,
          configurable: true,
        });
      }
    });

    it("survives when localStorage.setItem is unavailable", async () => {
      const original = globalThis.localStorage;

      Object.defineProperty(globalThis, "localStorage", {
        value: {
          getItem: vi.fn(() => String(SIDEBAR_WIDTH_DEFAULT + 10)),
        },
        writable: true,
        configurable: true,
      });

      try {
        // @ts-expect-error -- query string busts module cache
        const mod = await import("./ui.svelte.js?sidebarWidthNoSetItem");
        expect(mod.ui.sidebarWidth).toBe(SIDEBAR_WIDTH_DEFAULT + 10);
        expect(() =>
          mod.ui.setSidebarWidth(SIDEBAR_WIDTH_DEFAULT + 20),
        ).not.toThrow();
        expect(mod.ui.sidebarWidth).toBe(SIDEBAR_WIDTH_DEFAULT + 20);
      } finally {
        Object.defineProperty(globalThis, "localStorage", {
          value: original,
          writable: true,
          configurable: true,
        });
      }
    });

    it("survives when localStorage is null", async () => {
      const original = globalThis.localStorage;

      Object.defineProperty(globalThis, "localStorage", {
        value: null,
        writable: true,
        configurable: true,
      });

      try {
        // @ts-expect-error -- query string busts module cache
        const mod = await import("./ui.svelte.js?sidebarWidthNullStorage");
        expect(mod.ui.sidebarWidth).toBe(SIDEBAR_WIDTH_DEFAULT);
        expect(() =>
          mod.ui.setSidebarWidth(SIDEBAR_WIDTH_DEFAULT + 15),
        ).not.toThrow();
      } finally {
        Object.defineProperty(globalThis, "localStorage", {
          value: original,
          writable: true,
          configurable: true,
        });
      }
    });

    it("survives when localStorage is undefined", async () => {
      const original = globalThis.localStorage;
      // @ts-expect-error -- deliberately removing localStorage
      delete globalThis.localStorage;

      try {
        // @ts-expect-error -- query string busts module cache
        const mod = await import("./ui.svelte.js?sidebarWidthNoStorage");
        expect(mod.ui.sidebarWidth).toBe(SIDEBAR_WIDTH_DEFAULT);
        expect(() =>
          mod.ui.setSidebarWidth(SIDEBAR_WIDTH_DEFAULT + 25),
        ).not.toThrow();
      } finally {
        Object.defineProperty(globalThis, "localStorage", {
          value: original,
          writable: true,
          configurable: true,
        });
      }
    });
  });

  describe("system theme preference", () => {
    it("resolves to the OS scheme when preference is system", () => {
      ui.setThemePreference("system");
      ui.prefersDark = true;
      expect(ui.theme).toBe("dark");
      ui.prefersDark = false;
      expect(ui.theme).toBe("light");
    });

    it("ignores the OS scheme for explicit light/dark", () => {
      ui.prefersDark = true;
      ui.setThemePreference("light");
      expect(ui.theme).toBe("light");
      ui.setThemePreference("dark");
      expect(ui.theme).toBe("dark");
    });
  });

  describe("postMessage theme control", () => {
    it("should change theme on valid theme:set message", () => {
      ui.themePreference = "light";
      window.dispatchEvent(
        new MessageEvent("message", {
          data: { type: "theme:set", theme: "dark" },
        }),
      );
      expect(ui.themePreference).toBe("dark");
      expect(ui.theme).toBe("dark");
    });

    it("should accept system via theme:set message", () => {
      ui.themePreference = "light";
      window.dispatchEvent(
        new MessageEvent("message", {
          data: { type: "theme:set", theme: "system" },
        }),
      );
      expect(ui.themePreference).toBe("system");
    });

    it("should ignore invalid theme values", () => {
      ui.themePreference = "light";
      window.dispatchEvent(
        new MessageEvent("message", {
          data: { type: "theme:set", theme: "purple" },
        }),
      );
      expect(ui.themePreference).toBe("light");
    });

    it("should ignore unrelated message types", () => {
      ui.themePreference = "light";
      window.dispatchEvent(
        new MessageEvent("message", {
          data: { type: "some-other-event", theme: "dark" },
        }),
      );
      expect(ui.themePreference).toBe("light");
    });
  });

  describe("toggles", () => {
    it("should cycle theme preference light -> dark -> system", () => {
      ui.themePreference = "light";
      ui.toggleTheme();
      expect(ui.themePreference).toBe("dark");
      ui.toggleTheme();
      expect(ui.themePreference).toBe("system");
      ui.toggleTheme();
      expect(ui.themePreference).toBe("light");
    });

    it("should toggle sortNewestFirst", () => {
      const initial = ui.sortNewestFirst;
      ui.toggleSort();
      expect(ui.sortNewestFirst).toBe(!initial);
    });

    it("should toggle LLM title preference", () => {
      ui.setUseLlmTitle(false);
      expect(ui.useLlmTitle).toBe(false);

      ui.toggleUseLlmTitle();
      expect(ui.useLlmTitle).toBe(true);

      ui.setUseLlmTitle(false);
      expect(ui.useLlmTitle).toBe(false);
    });

    it("should persist LLM title preference", async () => {
      const original = globalThis.localStorage;
      const setItem = vi.fn();
      const getItem = vi.fn((key: string) =>
        key === "agentsview-use-llm-title" ? "true" : null,
      );

      Object.defineProperty(globalThis, "localStorage", {
        value: { getItem, setItem },
        writable: true,
        configurable: true,
      });

      try {
        // @ts-expect-error -- cache bust for fresh UIStore
        const mod = await import("./ui.svelte.js?persistUseLlmTitle");
        expect(mod.ui.useLlmTitle).toBe(true);
        setItem.mockClear();

        mod.ui.setUseLlmTitle(false);
        await tick();
        expect(setItem).toHaveBeenLastCalledWith(
          "agentsview-use-llm-title",
          "false",
        );
      } finally {
        Object.defineProperty(globalThis, "localStorage", {
          value: original,
          writable: true,
          configurable: true,
        });
      }
    });
  });

  describe("session vitals Calls disclosure", () => {
    const KEY = "agentsview-session-vitals-calls-expanded";

    type FreshUI = {
      ui: Record<string, unknown> & { toggleVitalsCalls: () => void };
    };

    async function freshStore(
      storage: unknown,
      load: () => Promise<unknown>,
    ): Promise<FreshUI> {
      const original = globalThis.localStorage;
      Object.defineProperty(globalThis, "localStorage", {
        value: storage,
        writable: true,
        configurable: true,
      });
      try {
        return (await load()) as FreshUI;
      } finally {
        Object.defineProperty(globalThis, "localStorage", {
          value: original,
          writable: true,
          configurable: true,
        });
      }
    }

    it("defaults to expanded when nothing is stored", async () => {
      const mod = await freshStore(
        { getItem: vi.fn(() => null), setItem: vi.fn() },
        // @ts-expect-error -- cache bust for a fresh UIStore instance
        () => import("./ui.svelte.js?callsMissing"),
      );
      expect(mod.ui.vitalsCallsExpanded).toBe(true);
    });

    it('restores the collapsed choice from a stored "false"', async () => {
      const mod = await freshStore(
        {
          getItem: vi.fn((key: string) => (key === KEY ? "false" : null)),
          setItem: vi.fn(),
        },
        // @ts-expect-error -- cache bust for a fresh UIStore instance
        () => import("./ui.svelte.js?callsCollapsed"),
      );
      expect(mod.ui.vitalsCallsExpanded).toBe(false);
    });

    it("falls back to expanded for an unparseable stored value", async () => {
      const mod = await freshStore(
        {
          getItem: vi.fn((key: string) => (key === KEY ? "nope" : null)),
          setItem: vi.fn(),
        },
        // @ts-expect-error -- cache bust for a fresh UIStore instance
        () => import("./ui.svelte.js?callsInvalid"),
      );
      expect(mod.ui.vitalsCallsExpanded).toBe(true);
    });

    it("falls back to expanded when storage is null or has no getItem", async () => {
      const nullStorage = await freshStore(
        null,
        // @ts-expect-error -- cache bust for a fresh UIStore instance
        () => import("./ui.svelte.js?callsNullStorage"),
      );
      expect(nullStorage.ui.vitalsCallsExpanded).toBe(true);

      const noGetItem = await freshStore(
        { setItem: vi.fn() },
        // @ts-expect-error -- cache bust for a fresh UIStore instance
        () => import("./ui.svelte.js?callsNoGetItem"),
      );
      expect(noGetItem.ui.vitalsCallsExpanded).toBe(true);
    });

    it("persists the toggled choice under the namespaced key", async () => {
      const setItem = vi.fn();
      const mod = await freshStore(
        { getItem: vi.fn(() => null), setItem },
        // @ts-expect-error -- cache bust for a fresh UIStore instance
        () => import("./ui.svelte.js?callsPersist"),
      );
      const original = globalThis.localStorage;
      Object.defineProperty(globalThis, "localStorage", {
        value: { getItem: vi.fn(() => null), setItem },
        writable: true,
        configurable: true,
      });
      try {
        setItem.mockClear();
        mod.ui.toggleVitalsCalls();
        await tick();
        expect(mod.ui.vitalsCallsExpanded).toBe(false);
        expect(setItem).toHaveBeenLastCalledWith(KEY, "false");

        mod.ui.toggleVitalsCalls();
        await tick();
        expect(mod.ui.vitalsCallsExpanded).toBe(true);
        expect(setItem).toHaveBeenLastCalledWith(KEY, "true");
      } finally {
        Object.defineProperty(globalThis, "localStorage", {
          value: original,
          writable: true,
          configurable: true,
        });
      }
    });

    it("keeps toggling in-page when persistence throws", async () => {
      // Scoped to this key: the theme effect writes `theme` unguarded (see
      // BACKLOG P12-1), and a blanket throw would surface that instead.
      const setItem = vi.fn((key: string) => {
        if (key === KEY) throw new Error("quota exceeded");
      });
      const mod = await freshStore(
        { getItem: vi.fn(() => null), setItem },
        // @ts-expect-error -- cache bust for a fresh UIStore instance
        () => import("./ui.svelte.js?callsPersistThrows"),
      );
      const original = globalThis.localStorage;
      Object.defineProperty(globalThis, "localStorage", {
        value: { getItem: vi.fn(() => null), setItem },
        writable: true,
        configurable: true,
      });
      try {
        mod.ui.toggleVitalsCalls();
        await tick();
        expect(mod.ui.vitalsCallsExpanded).toBe(false);
      } finally {
        Object.defineProperty(globalThis, "localStorage", {
          value: original,
          writable: true,
          configurable: true,
        });
      }
    });
  });

  describe("block type filtering", () => {
    beforeEach(() => {
      ui.showAllBlocks();
    });

    it("should start with all blocks visible", () => {
      expect(ui.hiddenBlockCount).toBe(0);
      expect(ui.hasBlockFilters).toBe(false);
      expect(ui.isBlockVisible("user")).toBe(true);
      expect(ui.isBlockVisible("tool")).toBe(true);
      expect(ui.isBlockVisible("thinking")).toBe(true);
      expect(ui.isBlockVisible("code")).toBe(true);
      expect(ui.isBlockVisible("assistant")).toBe(true);
    });

    it("should toggle a block type off and on", () => {
      ui.toggleBlock("tool");
      expect(ui.isBlockVisible("tool")).toBe(false);
      expect(ui.hiddenBlockCount).toBe(1);
      expect(ui.hasBlockFilters).toBe(true);

      ui.toggleBlock("tool");
      expect(ui.isBlockVisible("tool")).toBe(true);
      expect(ui.hiddenBlockCount).toBe(0);
    });

    it("should reset all with showAllBlocks", () => {
      ui.toggleBlock("user");
      ui.toggleBlock("tool");
      ui.toggleBlock("code");
      expect(ui.hiddenBlockCount).toBe(3);

      ui.showAllBlocks();
      expect(ui.hiddenBlockCount).toBe(0);
      expect(ui.hasBlockFilters).toBe(false);
    });
  });

  describe("sidebar", () => {
    beforeEach(() => {
      ui.sidebarOpen = true;
    });

    it("should default to open", () => {
      expect(ui.sidebarOpen).toBe(true);
    });

    it("should toggle sidebar", () => {
      ui.toggleSidebar();
      expect(ui.sidebarOpen).toBe(false);

      ui.toggleSidebar();
      expect(ui.sidebarOpen).toBe(true);
    });

    it("should close sidebar", () => {
      ui.closeSidebar();
      expect(ui.sidebarOpen).toBe(false);
    });

    it("closeSidebar should be idempotent", () => {
      ui.closeSidebar();
      ui.closeSidebar();
      expect(ui.sidebarOpen).toBe(false);
    });

    it("isMobileViewport should default to false in test environment", () => {
      // matchMedia is unavailable in test env, so isMobileViewport
      // stays at its initial value (false = desktop assumption).
      expect(ui.isMobileViewport).toBe(false);
    });

    it("should initialize sidebar closed on narrow viewport", async () => {
      const originalMatchMedia = window.matchMedia;
      window.matchMedia = vi.fn().mockReturnValue({
        matches: false,
        addEventListener: vi.fn(),
        removeEventListener: vi.fn(),
      }) as unknown as typeof window.matchMedia;
      try {
        // @ts-expect-error -- cache bust for fresh UIStore
        const mod = await import("./ui.svelte.js?narrowViewport");
        expect(mod.ui.sidebarOpen).toBe(false);
        expect(mod.ui.isMobileViewport).toBe(true);
      } finally {
        window.matchMedia = originalMatchMedia;
      }
    });

    it("should initialize sidebar open on wide viewport", async () => {
      const originalMatchMedia = window.matchMedia;
      window.matchMedia = vi.fn().mockReturnValue({
        matches: true,
        addEventListener: vi.fn(),
        removeEventListener: vi.fn(),
      }) as unknown as typeof window.matchMedia;
      try {
        // @ts-expect-error -- cache bust for fresh UIStore
        const mod = await import("./ui.svelte.js?wideViewport");
        expect(mod.ui.sidebarOpen).toBe(true);
        expect(mod.ui.isMobileViewport).toBe(false);
      } finally {
        window.matchMedia = originalMatchMedia;
      }
    });
  });

  describe("messageLayout", () => {
    beforeEach(() => {
      ui.setLayout("default");
    });

    it("should default to 'default'", () => {
      expect(ui.messageLayout).toBe("default");
    });

    it("should set layout explicitly", () => {
      ui.setLayout("compact");
      expect(ui.messageLayout).toBe("compact");

      ui.setLayout("stream");
      expect(ui.messageLayout).toBe("stream");
    });

    it("should cycle through layouts", () => {
      ui.setLayout("default");
      ui.cycleLayout();
      expect(ui.messageLayout).toBe("compact");

      ui.cycleLayout();
      expect(ui.messageLayout).toBe("stream");

      // Phase 19 (de6eeaf6): skim joins the cycle before wrapping around.
      ui.cycleLayout();
      expect(ui.messageLayout).toBe("skim");

      ui.cycleLayout();
      expect(ui.messageLayout).toBe("default");
    });
  });

  describe("Phase 19 messageLayout skim", () => {
    beforeEach(() => {
      ui.setLayout("default");
    });

    it("accepts skim as an explicit layout", () => {
      ui.setLayout("skim");
      expect(ui.messageLayout).toBe("skim");
    });

    it("restores a stored skim layout", async () => {
      const original = globalThis.localStorage;
      Object.defineProperty(globalThis, "localStorage", {
        value: {
          getItem: vi.fn((key: string) =>
            key === "agentsview-message-layout" ? "skim" : null,
          ),
          setItem: vi.fn(),
        },
        writable: true,
        configurable: true,
      });
      try {
        // @ts-expect-error -- query string busts module cache
        const mod = await import("./ui.svelte.js?storedSkimLayout");
        expect(mod.ui.messageLayout).toBe("skim");
      } finally {
        Object.defineProperty(globalThis, "localStorage", {
          value: original,
          writable: true,
          configurable: true,
        });
      }
    });

    it("falls back to default for an unknown stored layout", async () => {
      const original = globalThis.localStorage;
      Object.defineProperty(globalThis, "localStorage", {
        value: {
          getItem: vi.fn((key: string) =>
            key === "agentsview-message-layout" ? "skimmed" : null,
          ),
          setItem: vi.fn(),
        },
        writable: true,
        configurable: true,
      });
      try {
        // @ts-expect-error -- query string busts module cache
        const mod = await import("./ui.svelte.js?badSkimLayout");
        expect(mod.ui.messageLayout).toBe("default");
      } finally {
        Object.defineProperty(globalThis, "localStorage", {
          value: original,
          writable: true,
          configurable: true,
        });
      }
    });

    it("persists a skim layout selection", async () => {
      const original = globalThis.localStorage;
      const setItem = vi.fn();
      Object.defineProperty(globalThis, "localStorage", {
        value: { getItem: vi.fn(() => null), setItem },
        writable: true,
        configurable: true,
      });
      try {
        // @ts-expect-error -- query string busts module cache
        const mod = await import("./ui.svelte.js?persistSkimLayout");
        setItem.mockClear();
        mod.ui.setLayout("skim");
        await tick();
        expect(setItem).toHaveBeenCalledWith(
          "agentsview-message-layout",
          "skim",
        );
      } finally {
        Object.defineProperty(globalThis, "localStorage", {
          value: original,
          writable: true,
          configurable: true,
        });
      }
    });
  });

  describe("transcriptMode", () => {
    beforeEach(() => {
      ui.setTranscriptMode("normal");
    });

    it("should default to normal", () => {
      expect(ui.transcriptMode).toBe("normal");
    });

    it("should set transcript mode explicitly", () => {
      ui.setTranscriptMode("focused");
      expect(ui.transcriptMode).toBe("focused");
    });

    it("should persist transcript mode changes", async () => {
      const original = globalThis.localStorage;
      const setItem = vi.fn();
      const getItem = vi.fn(() => null);

      Object.defineProperty(globalThis, "localStorage", {
        value: { getItem, setItem },
        writable: true,
        configurable: true,
      });

      try {
        // @ts-expect-error -- cache bust for fresh UIStore
        const mod = await import("./ui.svelte.js?persistTranscriptMode");
        setItem.mockClear();
        mod.ui.setTranscriptMode("focused");
        await Promise.resolve();
        expect(setItem).toHaveBeenLastCalledWith(
          "agentsview-transcript-mode",
          "focused",
        );
      } finally {
        Object.defineProperty(globalThis, "localStorage", {
          value: original,
          writable: true,
          configurable: true,
        });
      }
    });

    it("should fall back to normal for invalid stored transcript mode", async () => {
      const original = globalThis.localStorage;

      Object.defineProperty(globalThis, "localStorage", {
        value: {
          getItem: vi.fn((key: string) =>
            key === "agentsview-transcript-mode"
              ? "detailed"
              : null,
          ),
          setItem: vi.fn(),
        },
        writable: true,
        configurable: true,
      });
      try {
        // @ts-expect-error -- cache bust for fresh UIStore
        const mod = await import("./ui.svelte.js?badTranscriptMode");
        expect(mod.ui.transcriptMode).toBe("normal");
      } finally {
        Object.defineProperty(globalThis, "localStorage", {
          value: original,
          writable: true,
          configurable: true,
        });
      }
    });
  });

  /**
   * Phase 19 (e65fe7a3). These use fresh-module imports because the root zoom
   * and high-contrast class are written by `$effect`s created in the store
   * constructor, so they need a store whose lifetime the test controls.
   */
  describe("Phase 19 fontScale", () => {
    /** Swap in a stub localStorage for the duration of `body`. */
    async function withStorage(
      storage: unknown,
      body: () => Promise<void>,
    ): Promise<void> {
      const original = globalThis.localStorage;
      Object.defineProperty(globalThis, "localStorage", {
        value: storage,
        writable: true,
        configurable: true,
      });
      try {
        await body();
      } finally {
        Object.defineProperty(globalThis, "localStorage", {
          value: original,
          writable: true,
          configurable: true,
        });
        document.documentElement.style.removeProperty("zoom");
      }
    }

    /** A store built against a hostile localStorage must still be usable. */
    async function expectInteractive(mod: {
      ui: { fontScale: number; setFontScale: (n: number) => void };
    }): Promise<void> {
      expect(mod.ui.fontScale).toBe(100);
      mod.ui.setFontScale(110);
      await tick();
      expect(mod.ui.fontScale).toBe(110);
      expect(
        document.documentElement.style.getPropertyValue("zoom"),
      ).toBe("1.1");
    }

    beforeEach(() => {
      ui.setFontScale(100);
    });

    it("defaults to 100", () => {
      expect(ui.fontScale).toBe(100);
    });

    it("exposes the five supported steps", () => {
      expect(FONT_SCALE_STEPS).toEqual([90, 100, 110, 120, 130]);
    });

    it("sets any supported step", () => {
      for (const step of FONT_SCALE_STEPS) {
        ui.setFontScale(step);
        expect(ui.fontScale).toBe(step);
      }
    });

    it("ignores values outside the supported steps", () => {
      ui.setFontScale(120);
      for (const bad of [145, 0, -100, 100.5, Number.NaN]) {
        ui.setFontScale(bad);
        expect(ui.fontScale).toBe(120);
      }
    });

    it("applies the font scale as root zoom on the web", async () => {
      await withStorage(
        { getItem: vi.fn(() => null), setItem: vi.fn() },
        async () => {
          // @ts-expect-error -- query string busts module cache
          const mod = await import("./ui.svelte.js?webFontScale");
          mod.ui.setFontScale(90);
          await tick();
          expect(
            document.documentElement.style.getPropertyValue("zoom"),
          ).toBe("0.9");

          mod.ui.setFontScale(110);
          await tick();
          expect(
            document.documentElement.style.getPropertyValue("zoom"),
          ).toBe("1.1");

          mod.ui.setFontScale(130);
          await tick();
          expect(
            document.documentElement.style.getPropertyValue("zoom"),
          ).toBe("1.3");
        },
      );
    });

    it("multiplies the desktop window zoom with the font scale", async () => {
      window.history.replaceState({}, "", "/?desktop");
      try {
        await withStorage(
          { getItem: vi.fn(() => null), setItem: vi.fn() },
          async () => {
            // @ts-expect-error -- query string busts module cache
            const mod = await import("./ui.svelte.js?desktopCompose");
            mod.ui.zoomLevel = 150;
            mod.ui.setFontScale(120);
            await tick();
            // 1.5 * 1.2 -- a sum would be 2.7 and plain replacement 1.2.
            expect(
              document.documentElement.style.getPropertyValue("zoom"),
            ).toBe("1.8");

            mod.ui.zoomLevel = 200;
            mod.ui.setFontScale(110);
            await tick();
            expect(
              document.documentElement.style.getPropertyValue("zoom"),
            ).toBe("2.2");
          },
        );
      } finally {
        window.history.replaceState({}, "", "/");
      }
    });

    it("keeps a single root zoom writer so font scale wins last", async () => {
      window.history.replaceState({}, "", "/?desktop");
      try {
        await withStorage(
          { getItem: vi.fn(() => null), setItem: vi.fn() },
          async () => {
            // @ts-expect-error -- query string busts module cache
            const mod = await import("./ui.svelte.js?singleZoomWriter");
            mod.ui.zoomLevel = 150;
            await tick();
            expect(
              document.documentElement.style.getPropertyValue("zoom"),
            ).toBe("1.5");
            mod.ui.setFontScale(120);
            await tick();
            // A leftover desktop-only writer would clobber this back to 1.5.
            expect(
              document.documentElement.style.getPropertyValue("zoom"),
            ).toBe("1.8");
          },
        );
      } finally {
        window.history.replaceState({}, "", "/");
      }
    });

    it("persists the font scale on the web", async () => {
      const setItem = vi.fn();
      await withStorage(
        { getItem: vi.fn(() => null), setItem },
        async () => {
          // @ts-expect-error -- query string busts module cache
          const mod = await import("./ui.svelte.js?persistFontScale");
          setItem.mockClear();
          mod.ui.setFontScale(120);
          await tick();
          expect(setItem).toHaveBeenCalledWith(
            "agentsview-font-scale",
            "120",
          );
          // The desktop-only window zoom must not leak onto the web.
          expect(setItem).not.toHaveBeenCalledWith(
            "agentsview-zoom-level",
            expect.anything(),
          );
        },
      );
    });

    it("persists both the font scale and the window zoom on desktop", async () => {
      const setItem = vi.fn();
      window.history.replaceState({}, "", "/?desktop");
      try {
        await withStorage(
          { getItem: vi.fn(() => null), setItem },
          async () => {
            // @ts-expect-error -- query string busts module cache
            const mod = await import("./ui.svelte.js?persistDesktopScale");
            setItem.mockClear();
            mod.ui.setFontScale(130);
            mod.ui.zoomLevel = 125;
            await tick();
            expect(setItem).toHaveBeenCalledWith(
              "agentsview-font-scale",
              "130",
            );
            expect(setItem).toHaveBeenCalledWith(
              "agentsview-zoom-level",
              "125",
            );
          },
        );
      } finally {
        window.history.replaceState({}, "", "/");
      }
    });

    it("restores a stored font scale", async () => {
      await withStorage(
        {
          getItem: vi.fn((key: string) =>
            key === "agentsview-font-scale" ? "130" : null,
          ),
          setItem: vi.fn(),
        },
        async () => {
          // @ts-expect-error -- query string busts module cache
          const mod = await import("./ui.svelte.js?storedFontScale");
          expect(mod.ui.fontScale).toBe(130);
        },
      );
    });

    it("falls back to 100 for an invalid stored font scale", async () => {
      await withStorage(
        {
          getItem: vi.fn((key: string) =>
            key === "agentsview-font-scale" ? "145" : null,
          ),
          setItem: vi.fn(),
        },
        async () => {
          // @ts-expect-error -- query string busts module cache
          const mod = await import("./ui.svelte.js?badFontScale");
          expect(mod.ui.fontScale).toBe(100);
        },
      );
    });

    it("stays interactive when localStorage is undefined", async () => {
      await withStorage(undefined, async () => {
        // @ts-expect-error -- query string busts module cache
        const mod = await import("./ui.svelte.js?noStorage");
        await expectInteractive(mod);
      });
    });

    it("stays interactive when localStorage is null", async () => {
      await withStorage(null, async () => {
        // @ts-expect-error -- query string busts module cache
        const mod = await import("./ui.svelte.js?nullStorage");
        await expectInteractive(mod);
      });
    });

    it("stays interactive when localStorage has no methods", async () => {
      await withStorage({}, async () => {
        // @ts-expect-error -- query string busts module cache
        const mod = await import("./ui.svelte.js?methodlessStorage");
        await expectInteractive(mod);
      });
    });

    it("stays interactive when localStorage throws", async () => {
      await withStorage(
        {
          getItem: () => {
            throw new Error("denied");
          },
          setItem: () => {
            throw new Error("denied");
          },
        },
        async () => {
          // @ts-expect-error -- query string busts module cache
          const mod = await import("./ui.svelte.js?throwingStorage");
          await expectInteractive(mod);
        },
      );
    });
  });

  describe("Phase 19 highContrast", () => {
    async function withStorage(
      storage: unknown,
      body: () => Promise<void>,
    ): Promise<void> {
      const original = globalThis.localStorage;
      Object.defineProperty(globalThis, "localStorage", {
        value: storage,
        writable: true,
        configurable: true,
      });
      try {
        await body();
      } finally {
        Object.defineProperty(globalThis, "localStorage", {
          value: original,
          writable: true,
          configurable: true,
        });
        document.documentElement.classList.remove("high-contrast");
      }
    }

    beforeEach(() => {
      if (ui.highContrast) ui.toggleHighContrast();
    });

    it("defaults to false", () => {
      expect(ui.highContrast).toBe(false);
    });

    it("toggles the value", () => {
      ui.toggleHighContrast();
      expect(ui.highContrast).toBe(true);
      ui.toggleHighContrast();
      expect(ui.highContrast).toBe(false);
    });

    it("toggles the root class and persists both states", async () => {
      const setItem = vi.fn();
      await withStorage(
        { getItem: vi.fn(() => null), setItem },
        async () => {
          // @ts-expect-error -- query string busts module cache
          const mod = await import("./ui.svelte.js?highContrastToggle");
          setItem.mockClear();
          mod.ui.toggleHighContrast();
          await tick();
          expect(
            document.documentElement.classList.contains("high-contrast"),
          ).toBe(true);
          expect(setItem).toHaveBeenCalledWith(
            "agentsview-high-contrast",
            "true",
          );

          mod.ui.toggleHighContrast();
          await tick();
          expect(
            document.documentElement.classList.contains("high-contrast"),
          ).toBe(false);
          expect(setItem).toHaveBeenCalledWith(
            "agentsview-high-contrast",
            "false",
          );
        },
      );
    });

    it("restores a stored high-contrast preference", async () => {
      await withStorage(
        {
          getItem: vi.fn((key: string) =>
            key === "agentsview-high-contrast" ? "true" : null,
          ),
          setItem: vi.fn(),
        },
        async () => {
          // @ts-expect-error -- query string busts module cache
          const mod = await import("./ui.svelte.js?storedHighContrast");
          expect(mod.ui.highContrast).toBe(true);
          await tick();
          expect(
            document.documentElement.classList.contains("high-contrast"),
          ).toBe(true);
        },
      );
    });

    it("composes with the dark theme instead of replacing it", async () => {
      await withStorage(
        { getItem: vi.fn(() => null), setItem: vi.fn() },
        async () => {
          // @ts-expect-error -- query string busts module cache
          const mod = await import("./ui.svelte.js?contrastWithDark");
          mod.ui.toggleHighContrast();
          mod.ui.setThemePreference("dark");
          await tick();
          const classes = document.documentElement.classList;
          expect(classes.contains("dark")).toBe(true);
          expect(classes.contains("high-contrast")).toBe(true);

          mod.ui.setThemePreference("light");
          await tick();
          expect(document.documentElement.classList.contains("dark")).toBe(
            false,
          );
          expect(
            document.documentElement.classList.contains("high-contrast"),
          ).toBe(true);
        },
      );
    });

    it("stays interactive when localStorage throws", async () => {
      await withStorage(
        {
          getItem: () => {
            throw new Error("denied");
          },
          setItem: () => {
            throw new Error("denied");
          },
        },
        async () => {
          // @ts-expect-error -- query string busts module cache
          const mod = await import("./ui.svelte.js?contrastBrokenStorage");
          expect(mod.ui.highContrast).toBe(false);
          mod.ui.toggleHighContrast();
          await tick();
          expect(mod.ui.highContrast).toBe(true);
          expect(
            document.documentElement.classList.contains("high-contrast"),
          ).toBe(true);
        },
      );
    });
  });
});
