// @vitest-environment jsdom
import {
  afterEach,
  beforeEach,
  describe,
  expect,
  it,
  vi,
} from "vitest";
import { ApiError, setAuthToken } from "./runtime.js";
import {
  fetchBalance,
  fetchEnrichStatus,
  fetchLLMConfig,
  fetchSemanticSearchStatus,
  saveLLMConfig,
  semanticSearch,
  testLLMConnection,
  triggerEnrich,
} from "./llm.js";

describe("LLM API helpers", () => {
  const originalFetch = globalThis.fetch;
  const originalStorage = globalThis.localStorage;
  let fetchMock: ReturnType<typeof vi.fn>;
  let store: Map<string, string>;

  beforeEach(() => {
    store = new Map();
    Object.defineProperty(globalThis, "localStorage", {
      value: {
        getItem: vi.fn((key: string) => store.get(key) ?? null),
        setItem: vi.fn((key: string, value: string) => {
          store.set(key, value);
        }),
        removeItem: vi.fn((key: string) => {
          store.delete(key);
        }),
        clear: vi.fn(() => {
          store.clear();
        }),
      },
      writable: true,
      configurable: true,
    });
    setAuthToken("secret-token");
    fetchMock = vi.fn();
    globalThis.fetch = fetchMock as unknown as typeof fetch;
  });

  afterEach(() => {
    globalThis.fetch = originalFetch;
    Object.defineProperty(globalThis, "localStorage", {
      value: originalStorage,
      writable: true,
      configurable: true,
    });
  });

  it("fetches the balance endpoint with auth headers", async () => {
    fetchMock.mockResolvedValueOnce(
      new Response(
        JSON.stringify({
          supported: true,
          currency: "CNY",
          amount: "12.34",
          available: true,
        }),
        { status: 200 },
      ),
    );

    await expect(fetchBalance()).resolves.toEqual({
      supported: true,
      currency: "CNY",
      amount: "12.34",
      available: true,
    });

    expect(fetchMock).toHaveBeenCalledWith(
      "/api/v1/llm/balance",
      expect.objectContaining({
        headers: expect.any(Headers),
      }),
    );
    const init = fetchMock.mock.calls[0]![1] as RequestInit;
    expect((init.headers as Headers).get("Authorization")).toBe(
      "Bearer secret-token",
    );
  });

  it("posts enrichment requests as JSON", async () => {
    fetchMock.mockResolvedValueOnce(
      new Response(
        JSON.stringify({
          enriched: 2,
          skipped: 1,
          no_content: 0,
          errors: 0,
          candidates: 3,
          elapsed_ms: 25,
        }),
        { status: 200 },
      ),
    );

    await expect(triggerEnrich({ limit: 25, force: true })).resolves.toEqual({
      enriched: 2,
      skipped: 1,
      no_content: 0,
      errors: 0,
      candidates: 3,
      elapsed_ms: 25,
    });

    const [path, init] = fetchMock.mock.calls[0]!;
    expect(path).toBe("/api/v1/llm/enrich");
    expect(init).toMatchObject({ method: "POST" });
    expect(JSON.parse((init as RequestInit).body as string)).toEqual({
      limit: 25,
      force: true,
    });
    expect(((init as RequestInit).headers as Headers).get("Content-Type")).toBe(
      "application/json",
    );
  });

  it("fetches enrichment status counts", async () => {
    fetchMock.mockResolvedValueOnce(
      new Response(
        JSON.stringify({
          total: 10,
          enriched: 4,
          pending: 3,
          skipped_too_short: 1,
          no_content: 1,
          errors: 1,
          by_status: { ok: 4, error: 1 },
        }),
        { status: 200 },
      ),
    );

    await expect(fetchEnrichStatus()).resolves.toEqual({
      total: 10,
      enriched: 4,
      pending: 3,
      skipped_too_short: 1,
      no_content: 1,
      errors: 1,
      by_status: { ok: 4, error: 1 },
    });
    expect(fetchMock).toHaveBeenCalledWith(
      "/api/v1/llm/enrich/status",
      expect.any(Object),
    );
  });

  it("fetches and saves masked LLM config", async () => {
    fetchMock
      .mockResolvedValueOnce(
        new Response(
          JSON.stringify({
            enabled: true,
            base_url: "https://api.deepseek.com/v1",
            model: "deepseek-chat",
            reasoning_effort: "low",
            min_user_messages: 3,
            reenrich_msg_delta: 10,
            reenrich_idle_minutes: 60,
            concurrency: 2,
            periodic: true,
            has_api_key: true,
            api_key_preview: "1234",
            embed: {
              base_url: "https://embed.example/v1",
              model: "embed-model",
              has_api_key: true,
              api_key_preview: "5678",
            },
          }),
          { status: 200 },
        ),
      )
      .mockResolvedValueOnce(
        new Response(
          JSON.stringify({
            enabled: false,
            min_user_messages: 0,
            reenrich_msg_delta: 0,
            reenrich_idle_minutes: 0,
            concurrency: 0,
            periodic: false,
            has_api_key: true,
            api_key_preview: "1234",
            embed: { has_api_key: false },
          }),
          { status: 200 },
        ),
      );

    await expect(fetchLLMConfig()).resolves.toMatchObject({
      enabled: true,
      has_api_key: true,
      api_key_preview: "1234",
      embed: { has_api_key: true, api_key_preview: "5678" },
    });
    await expect(
      saveLLMConfig({ enabled: false, api_key: "********" }),
    ).resolves.toMatchObject({ enabled: false, has_api_key: true });

    expect(fetchMock.mock.calls[0]![0]).toBe("/api/v1/config/llm");
    const [path, init] = fetchMock.mock.calls[1]!;
    expect(path).toBe("/api/v1/config/llm");
    expect(init).toMatchObject({ method: "POST" });
    expect(JSON.parse((init as RequestInit).body as string)).toEqual({
      enabled: false,
      api_key: "********",
    });
  });

  it("posts LLM test requests and surfaces ApiError", async () => {
    fetchMock
      .mockResolvedValueOnce(
        new Response(
          JSON.stringify({
            chat: { ok: true, message: "ok" },
            embed: { ok: false, disabled: true, message: "disabled" },
          }),
          { status: 200 },
        ),
      )
      .mockResolvedValueOnce(
        new Response(JSON.stringify({ error: "not available" }), { status: 403 }),
      );

    await expect(testLLMConnection()).resolves.toEqual({
      chat: { ok: true, message: "ok" },
      embed: { ok: false, disabled: true, message: "disabled" },
    });
    expect(fetchMock.mock.calls[0]![0]).toBe("/api/v1/llm/test");
    expect(JSON.parse((fetchMock.mock.calls[0]![1] as RequestInit).body as string)).toEqual({});

    await expect(testLLMConnection({ base_url: "http://bad.test" })).rejects.toMatchObject({
      name: "ApiError",
      status: 403,
      message: "not available",
    } satisfies Partial<ApiError>);
  });

  it("fetches semantic status and semantic search results", async () => {
    fetchMock
      .mockResolvedValueOnce(
        new Response(JSON.stringify({ available: true }), { status: 200 }),
      )
      .mockResolvedValueOnce(
        new Response(JSON.stringify({ query: "auth", results: [], count: 0 }), { status: 200 }),
      );

    await expect(fetchSemanticSearchStatus()).resolves.toEqual({ available: true });
    await expect(semanticSearch("auth", "proj", 5)).resolves.toEqual({
      query: "auth",
      results: [],
      count: 0,
    });

    expect(fetchMock.mock.calls[0]![0]).toBe("/api/v1/search/semantic/status");
    expect(fetchMock.mock.calls[1]![0]).toBe("/api/v1/search/semantic?q=auth&k=5&project=proj");
  });

  it("throws ApiError with server message on non-OK responses", async () => {
    fetchMock.mockResolvedValueOnce(
      new Response(JSON.stringify({ error: "LLM is disabled" }), {
        status: 409,
      }),
    );

    await expect(triggerEnrich()).rejects.toMatchObject({
      name: "ApiError",
      status: 409,
      message: "LLM is disabled",
    } satisfies Partial<ApiError>);
  });
});
