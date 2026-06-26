// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { mount, tick, unmount } from "svelte";

const mocks = vi.hoisted(() => ({
  fetchMemoryQuality: vi.fn(),
}));

vi.mock("../../api/memoryQuality", () => ({
  fetchMemoryQuality: mocks.fetchMemoryQuality,
}));

// @ts-ignore
import MemoryQualityPanel from "./MemoryQualityPanel.svelte";

async function flush() {
  await Promise.resolve();
  await tick();
  await Promise.resolve();
  await tick();
}

describe("MemoryQualityPanel", () => {
  let component: ReturnType<typeof mount> | undefined;

  beforeEach(() => {
    mocks.fetchMemoryQuality.mockReset().mockResolvedValue({
      telemetry: {
        capture_attempts: 1,
        candidate_count: 1,
        capture_written: 1,
        injection_count: 1,
        recall_count: 1,
        recall_hit_count: 2,
        fallback_count: 1,
        scores: [0.8, 0.4],
      },
      extract: {
        sessions_scanned: 3,
        candidate_count: 2,
        written: 1,
        deduped: 1,
        rejected: 0,
        drift_refused: 0,
        llm_duration_ms: 12,
        llm_call_count: 2,
        provider_usage: { extract: 1 },
        llm_usage: { prompt_tokens: 10, completion_tokens: 5, total_tokens: 15 },
        llm_cost: { currency: "USD", amount: "0.12" },
      },
      consolidate: {
        candidate_count: 3,
        add_count: 1,
        update_count: 1,
        skip_count: 1,
        committed: 1,
        resynced: 1,
        llm_duration_ms: 20,
        llm_call_count: 1,
        provider_usage: { consolidate: 1 },
        llm_usage: { prompt_tokens: 12, completion_tokens: 6, total_tokens: 18 },
        llm_cost: { currency: "USD", amount: "0.34" },
      },
      telemetry_rows: [],
    });
  });

  afterEach(() => {
    if (component) {
      unmount(component);
      component = undefined;
    }
    document.body.innerHTML = "";
  });

  it("renders caveat and non-zero metrics", async () => {
    component = mount(MemoryQualityPanel, { target: document.body });
    await flush();

    document.querySelector<HTMLButtonElement>('[data-testid="memory-quality-toggle"]')!.click();
    await flush();

    const text = document.body.textContent ?? "";
    expect(text).toContain("非零指标只证明埋点接通，不代表召回质量达标");
    expect(text).toContain("抽取");
    expect(text).toContain("召回");
    expect(text).toContain("write rate: 50%");
    expect(text).toContain("fallback 1");
    expect(text).toContain("capsule route 1");
    expect(text).toContain("tokens: 15 / 18");
    expect(text).toContain("extract USD 0.12");
    expect(text).toContain("consolidate USD 0.34");
    expect(text).toContain("0.80, 0.40");
  });
});
