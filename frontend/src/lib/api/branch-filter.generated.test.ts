import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  AnalyticsService,
  MetadataService,
  OpenAPI,
  SearchService,
  SessionsService,
  TrendsService,
  UsageService,
} from "./generated/index";

const BRANCH_TOKEN = "alpha\u001ffeat,x";

let fetchMock: ReturnType<typeof vi.fn>;

function installFetchMock() {
  fetchMock = vi.fn(async () =>
    new Response("{}", {
      status: 200,
      headers: { "Content-Type": "application/json" },
    }),
  );
  vi.stubGlobal("fetch", fetchMock);
}

function lastRequestURL(): URL {
  expect(fetchMock).toHaveBeenCalledTimes(1);
  const [input] = fetchMock.mock.calls[0] ?? [];
  return new URL(String(input), "http://agentsview.test");
}

async function expectGitBranchQuery(
  call: () => Promise<unknown>,
  path: string,
) {
  await call();

  const url = lastRequestURL();
  expect(url.pathname).toBe(path);
  expect(url.searchParams.get("git_branch")).toBe(BRANCH_TOKEN);
}

describe("Phase 24 generated branch filter transport", () => {
  beforeEach(() => {
    OpenAPI.BASE = "";
    installFetchMock();
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    vi.clearAllMocks();
  });

  it("MetadataService requests branch metadata tokens", async () => {
    await MetadataService.getApiV1Branches({ includeAutomated: true });

    const url = lastRequestURL();
    expect(url.pathname).toBe("/api/v1/branches");
    expect(url.searchParams.get("include_automated")).toBe("true");
  });

  it("SessionsService sends git_branch in the sessions query", async () => {
    await expectGitBranchQuery(
      () => SessionsService.getApiV1Sessions({ gitBranch: BRANCH_TOKEN }),
      "/api/v1/sessions",
    );
  });

  it("SearchService sends git_branch in the content search query", async () => {
    await expectGitBranchQuery(
      () =>
        SearchService.getApiV1SearchContent({
          pattern: "error",
          gitBranch: BRANCH_TOKEN,
        }),
      "/api/v1/search/content",
    );
  });

  it("AnalyticsService sends git_branch in the analytics query", async () => {
    await expectGitBranchQuery(
      () =>
        AnalyticsService.getApiV1AnalyticsSummary({
          gitBranch: BRANCH_TOKEN,
        }),
      "/api/v1/analytics/summary",
    );
  });

  it("TrendsService sends git_branch in the trends query", async () => {
    await expectGitBranchQuery(
      () => TrendsService.getApiV1TrendsTerms({ gitBranch: BRANCH_TOKEN }),
      "/api/v1/trends/terms",
    );
  });

  it("UsageService sends git_branch in the usage query", async () => {
    await expectGitBranchQuery(
      () => UsageService.getApiV1UsageSummary({ gitBranch: BRANCH_TOKEN }),
      "/api/v1/usage/summary",
    );
  });
});
