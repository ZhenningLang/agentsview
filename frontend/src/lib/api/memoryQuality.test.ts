// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { fetchMemoryQuality } from "./memoryQuality.js";
import { setAuthToken, setServerUrl } from "./runtime.js";

describe("memory quality API", () => {
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

  it("defaults missing arrays and parses typed metrics", async () => {
    fetchMock.mockResolvedValueOnce(
      new Response(
        JSON.stringify({
          telemetry: { capture_attempts: 1, scores: [0.6, 0.4] },
          extract: { sessions_scanned: 3, provider_usage: { extract: 1 } },
          consolidate: { candidate_count: 2, provider_usage: { consolidate: 1 } },
          telemetry_rows: null,
        }),
        { status: 200 },
      ),
    );

    await expect(fetchMemoryQuality()).resolves.toMatchObject({
      telemetry: { capture_attempts: 1, scores: [0.6, 0.4] },
      extract: { sessions_scanned: 3, provider_usage: { extract: 1 } },
      consolidate: { candidate_count: 2, provider_usage: { consolidate: 1 } },
      telemetry_rows: [],
    });
  });

  it("calls the memory quality route with limit and parses server envelope", async () => {
    fetchMock.mockResolvedValueOnce(
      new Response(
        JSON.stringify({
          telemetry: {
            capture_attempts: 2,
            candidate_count: 2,
            capture_written: 1,
            injection_count: 3,
            recall_count: 4,
            recall_hit_count: 5,
            fallback_count: 1,
            scores: [0.8],
          },
          extract: {
            sessions_scanned: 6,
            candidate_count: 7,
            written: 3,
            deduped: 1,
            rejected: 2,
            drift_refused: 1,
            llm_duration_ms: 11,
            llm_call_count: 2,
            provider_usage: { extract: 2 },
            llm_usage: { prompt_tokens: 10, completion_tokens: 5, total_tokens: 15 },
            llm_cost: { currency: "USD", amount: "0.12" },
          },
          consolidate: {
            candidate_count: 8,
            add_count: 4,
            update_count: 2,
            skip_count: 2,
            committed: 1,
            resynced: 1,
            llm_duration_ms: 13,
            llm_call_count: 1,
            provider_usage: { consolidate: 1 },
            llm_usage: { prompt_tokens: 12, completion_tokens: 6, total_tokens: 18 },
            llm_cost: { currency: "USD", amount: "0.34" },
          },
          telemetry_rows: [
            { schema: "dotfiles.memory.telemetry.v1", ts: "2026-06-26T00:00:00Z", event: "recall", source: "context_capsule" },
          ],
        }),
        { status: 200 },
      ),
    );

    const result = await fetchMemoryQuality(7);

    expect(fetchMock).toHaveBeenCalledTimes(1);
    expect(fetchMock.mock.calls[0]![0]).toBe("/api/v1/memory/quality?limit=7");
    const init = fetchMock.mock.calls[0]![1] as RequestInit;
    expect(new Headers(init.headers).get("Authorization")).toBe("Bearer secret-token");
    expect(result.telemetry).toMatchObject({ recall_hit_count: 5, scores: [0.8] });
    expect(result.extract.llm_usage?.total_tokens).toBe(15);
    expect(result.extract.llm_cost).toEqual({ currency: "USD", amount: "0.12" });
    expect(result.consolidate.llm_usage?.total_tokens).toBe(18);
    expect(result.telemetry_rows).toHaveLength(1);
  });
});
