// @vitest-environment jsdom
import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import {
  downloadExport,
  downloadInsightExport,
  getInsightExportUrl,
  getInsightMarkdownExportUrl,
  getMarkdownExportUrl,
} from "./client.js";
import { ApiError } from "./runtime.js";

const storage = {
  getItem: vi.fn().mockReturnValue(""),
  setItem: vi.fn(),
  removeItem: vi.fn(),
  clear: vi.fn(),
};

describe("markdown export URLs", () => {
  beforeEach(() => {
    vi.stubGlobal("localStorage", storage);
    storage.getItem.mockReturnValue("");
    document.head.innerHTML = "";
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("builds markdown export URL with optional depth", () => {
    expect(getMarkdownExportUrl("sess-123")).toBe(
      "/api/v1/sessions/sess-123/md",
    );
    expect(getMarkdownExportUrl("sess-123", "all")).toBe(
      "/api/v1/sessions/sess-123/md?depth=all",
    );
    expect(getMarkdownExportUrl("sess-123", 1)).toBe(
      "/api/v1/sessions/sess-123/md?depth=1",
    );
  });
});

describe("Phase 25 insight export URLs", () => {
  beforeEach(() => {
    vi.stubGlobal("localStorage", storage);
    storage.getItem.mockReturnValue("");
    document.head.innerHTML = "";
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("builds local insight HTML and Markdown URLs", () => {
    expect(getInsightExportUrl(42)).toBe(
      "/api/v1/insights/42/export",
    );
    expect(getInsightMarkdownExportUrl(42)).toBe(
      "/api/v1/insights/42/md",
    );
  });

  it("honours a reverse-proxy base path", () => {
    document.head.innerHTML = '<base href="/agentsview/">';

    expect(getInsightExportUrl(42)).toBe(
      "/agentsview/api/v1/insights/42/export",
    );
    expect(getInsightMarkdownExportUrl(42)).toBe(
      "/agentsview/api/v1/insights/42/md",
    );
  });

  it("targets the configured remote server", () => {
    storage.getItem.mockImplementation((key: string) =>
      key === "agentsview-server-url" ? "https://remote.test:9443" : ""
    );

    expect(getInsightExportUrl(42)).toBe(
      "https://remote.test:9443/api/v1/insights/42/export",
    );
    expect(getInsightMarkdownExportUrl(42)).toBe(
      "https://remote.test:9443/api/v1/insights/42/md",
    );
  });
});

describe("Phase 25 authenticated export download", () => {
  const SERVER = "https://remote.test:9443";
  const TOKEN = "phase25-browser-token";

  let openMock: ReturnType<typeof vi.fn>;
  let fetchMock: ReturnType<typeof vi.fn>;
  let anchors: HTMLAnchorElement[];
  let clickMock: ReturnType<typeof vi.fn<() => void>>;

  function useRemoteAuth(): void {
    storage.getItem.mockImplementation((key: string) => {
      if (key === "agentsview-server-url") return SERVER;
      if (key === `agentsview-auth-token::${SERVER}`) return TOKEN;
      return "";
    });
  }

  function respondWith(
    status: number,
    headers: Record<string, string> = {},
  ): void {
    fetchMock.mockResolvedValue({
      ok: status >= 200 && status < 300,
      status,
      headers: new Headers(headers),
      blob: async () => new Blob(["<html></html>"], { type: "text/html" }),
    });
  }

  beforeEach(() => {
    vi.stubGlobal("localStorage", storage);
    storage.getItem.mockReturnValue("");
    document.head.innerHTML = "";

    openMock = vi.fn();
    fetchMock = vi.fn();
    vi.stubGlobal("open", openMock);
    vi.stubGlobal("fetch", fetchMock);
    vi.stubGlobal("URL", Object.assign(URL, {
      createObjectURL: vi.fn(() => "blob:phase25"),
      revokeObjectURL: vi.fn(),
    }));

    anchors = [];
    clickMock = vi.fn<() => void>();
    const realCreateElement = document.createElement.bind(document);
    vi.spyOn(document, "createElement").mockImplementation(
      (tag: string, ...rest: unknown[]) => {
        const el = realCreateElement(
          tag,
          ...(rest as []),
        ) as HTMLElement;
        if (tag === "a") {
          const anchor = el as HTMLAnchorElement;
          anchor.click = clickMock;
          anchors.push(anchor);
        }
        return el;
      },
    );
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });

  it("opens a new tab locally instead of fetching", async () => {
    await downloadInsightExport(42);

    expect(openMock).toHaveBeenCalledWith(
      "/api/v1/insights/42/export",
      "_blank",
    );
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("sends the token in the header and never in the URL", async () => {
    useRemoteAuth();
    respondWith(200);

    await downloadInsightExport(42);

    expect(openMock).not.toHaveBeenCalled();
    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit];
    expect(url).toBe(`${SERVER}/api/v1/insights/42/export`);
    expect(url).not.toContain(TOKEN);
    expect(new Headers(init.headers).get("Authorization")).toBe(
      `Bearer ${TOKEN}`,
    );
  });

  it("prefers the Content-Disposition filename", async () => {
    useRemoteAuth();
    respondWith(200, {
      "Content-Disposition":
        'attachment; filename="insight-daily_activity-my-app-20250115.html"',
    });

    await downloadInsightExport(42);

    expect(anchors).toHaveLength(1);
    expect(anchors[0]?.download).toBe(
      "insight-daily_activity-my-app-20250115.html",
    );
    expect(clickMock).toHaveBeenCalledTimes(1);
  });

  it("falls back to insight-{id}.html when the header is absent",
    async () => {
      useRemoteAuth();
      respondWith(200);

      await downloadInsightExport(42);

      expect(anchors[0]?.download).toBe("insight-42.html");
    });

  it("falls back to session-{id}.html for a session HTML export",
    async () => {
      useRemoteAuth();
      respondWith(200);

      await downloadExport("sess-123");

      const [url] = fetchMock.mock.calls[0] as [string];
      expect(url).toBe(`${SERVER}/api/v1/sessions/sess-123/export`);
      // The route serves HTML, so the fallback name must not claim .md.
      expect(anchors[0]?.download).toBe("session-sess-123.html");
    });

  it("still prefers Content-Disposition for a session export",
    async () => {
      useRemoteAuth();
      respondWith(200, {
        "Content-Disposition": 'attachment; filename="proj-20250115.html"',
      });

      await downloadExport("sess-123");

      expect(anchors[0]?.download).toBe("proj-20250115.html");
    });

  it.each([401, 404, 500, 502])(
    "throws ApiError on %i",
    async (status) => {
      useRemoteAuth();
      respondWith(status);

      await expect(downloadInsightExport(42)).rejects.toBeInstanceOf(
        ApiError,
      );
      expect(anchors).toHaveLength(0);
    },
  );
});
