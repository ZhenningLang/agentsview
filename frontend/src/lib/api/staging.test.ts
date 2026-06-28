// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { fetchStagingPool } from "./staging.js";
import { setAuthToken, setServerUrl } from "./runtime.js";

describe("staging pool API", () => {
  const originalFetch = globalThis.fetch;
  let fetchMock: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    setAuthToken("secret-token");
    fetchMock = vi.fn();
    globalThis.fetch = fetchMock as unknown as typeof fetch;
  });

  afterEach(() => {
    globalThis.fetch = originalFetch;
    setAuthToken("");
    setServerUrl("");
  });

  it("fetches the full pool and parses the scope split", async () => {
    fetchMock.mockResolvedValueOnce(
      new Response(
        JSON.stringify({
          available: true,
          total: 3,
          by_scope: { user: 2, project: 1 },
          projects: { "oss-atlas": 1 },
          candidates: [
            { id: "a", summary: "s", category: "preference", scope: "user", origin_project: "", origin_session: "x", created_at: "t" },
          ],
        }),
        { status: 200 },
      ),
    );

    await expect(fetchStagingPool()).resolves.toMatchObject({
      available: true,
      total: 3,
      by_scope: { user: 2, project: 1 },
      projects: { "oss-atlas": 1 },
    });
    const url = fetchMock.mock.calls[0]?.[0] as string;
    expect(url).toContain("/staging/candidates");
    expect(url).not.toContain("scope=");
  });

  it("passes the scope filter and limit as query params", async () => {
    fetchMock.mockResolvedValueOnce(
      new Response(
        JSON.stringify({ available: true, total: 3, by_scope: {}, projects: {}, candidates: [] }),
        { status: 200 },
      ),
    );

    await fetchStagingPool("project", 10);
    const url = fetchMock.mock.calls[0]?.[0] as string;
    expect(url).toContain("scope=project");
    expect(url).toContain("limit=10");
  });
});
