import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { InsightsService, OpenAPI } from "./generated/index";
import type { PublishResponse } from "./generated/index";
import insightsServiceSource
  from "./generated/services/InsightsService.ts?raw";
import publishResponseSource
  from "./generated/models/PublishResponse.ts?raw";

let fetchMock: ReturnType<typeof vi.fn>;

function installFetchMock(
  body: unknown,
  contentType = "application/json",
): void {
  fetchMock = vi.fn(async () =>
    new Response(
      typeof body === "string" ? body : JSON.stringify(body),
      { status: 200, headers: { "Content-Type": contentType } },
    ),
  );
  vi.stubGlobal("fetch", fetchMock);
}

function requestedUrl(): URL {
  expect(fetchMock).toHaveBeenCalledTimes(1);
  const [target] = fetchMock.mock.calls[0] as [string | URL];
  return new URL(String(target), "http://localhost");
}

function requestedMethod(): string {
  const [, init] = fetchMock.mock.calls[0] as [unknown, RequestInit];
  return String(init.method);
}

describe("Phase 25 generated insight export and publish transport", () => {
  beforeEach(() => {
    OpenAPI.BASE = "";
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    vi.clearAllMocks();
  });

  it("sends GET /api/v1/insights/{id}/export", async () => {
    installFetchMock("<html></html>", "text/html");

    await InsightsService.getApiV1InsightsIdExport({ id: 42 });

    expect(requestedMethod()).toBe("GET");
    expect(requestedUrl().pathname).toBe("/api/v1/insights/42/export");
    expect(requestedUrl().search).toBe("");
  });

  it("sends GET /api/v1/insights/{id}/md", async () => {
    installFetchMock("# Heading", "text/markdown");

    await InsightsService.getApiV1InsightsIdMd({ id: 42 });

    expect(requestedMethod()).toBe("GET");
    expect(requestedUrl().pathname).toBe("/api/v1/insights/42/md");
    expect(requestedUrl().search).toBe("");
  });

  it.each([
    [true, "true"],
    [false, "false"],
  ])(
    "sends POST /api/v1/insights/{id}/publish with secret=%s",
    async (secret, expected) => {
      installFetchMock({
        gist_id: "g1",
        gist_url: "https://gist.github.com/u/g1",
        raw_url: "https://gist.githubusercontent.com/u/g1/raw/x.html",
        view_url: "https://htmlpreview.github.io/?raw",
      });

      await InsightsService.postApiV1InsightsIdPublish({
        id: 42,
        secret,
      });

      expect(requestedMethod()).toBe("POST");
      expect(requestedUrl().pathname).toBe("/api/v1/insights/42/publish");
      expect(requestedUrl().searchParams.get("secret")).toBe(expected);
    },
  );

  it("returns the publish response body unchanged", async () => {
    const body: PublishResponse = {
      gist_id: "gist-id-123",
      gist_url: "https://gist.github.com/example-user/gist-id-123",
      raw_url:
        "https://gist.githubusercontent.com/example-user/gist-id-123/raw/i.html",
      view_url: "https://htmlpreview.github.io/?raw",
    };
    installFetchMock(body);

    const got = await InsightsService.postApiV1InsightsIdPublish({
      id: 42,
    });

    expect(got).toEqual(body);
  });

  it("honours a configured base URL for a remote server", async () => {
    OpenAPI.BASE = "https://remote.test:9443";
    installFetchMock("<html></html>", "text/html");

    await InsightsService.getApiV1InsightsIdExport({ id: 7 });

    const url = requestedUrl();
    expect(url.origin).toBe("https://remote.test:9443");
    expect(url.pathname).toBe("/api/v1/insights/7/export");
    OpenAPI.BASE = "";
  });

  it("reuses the existing PublishResponse model", () => {
    // The generator is the only writer here: the insight publish route
    // must not introduce a second response shape alongside the session
    // one, and nothing may hand-edit these files.
    expect(insightsServiceSource).toContain(
      "import type { PublishResponse } from '../models/PublishResponse';",
    );
    expect(insightsServiceSource).toContain(
      "CancelablePromise<PublishResponse>",
    );
    expect(insightsServiceSource).toContain(
      "do not edit",
    );
    expect(publishResponseSource).toContain("gist_id: string;");
    expect(publishResponseSource).toContain("gist_url: string;");
    expect(publishResponseSource).toContain("raw_url: string;");
    expect(publishResponseSource).toContain("view_url: string;");
  });
});
