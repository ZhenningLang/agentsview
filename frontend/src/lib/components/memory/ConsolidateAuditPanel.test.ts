// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { mount, tick, unmount } from "svelte";

const mocks = vi.hoisted(() => ({
  fetchConsolidateAudit: vi.fn(),
  setConsolidateEnabled: vi.fn(),
}));

vi.mock("../../api/consolidate", () => ({
  fetchConsolidateAudit: mocks.fetchConsolidateAudit,
  setConsolidateEnabled: mocks.setConsolidateEnabled,
}));

// @ts-ignore
import ConsolidateAuditPanel from "./ConsolidateAuditPanel.svelte";

async function flush() {
  await Promise.resolve();
  await tick();
  await Promise.resolve();
  await tick();
}

describe("ConsolidateAuditPanel", () => {
  let component: ReturnType<typeof mount> | undefined;

  beforeEach(() => {
    mocks.fetchConsolidateAudit.mockReset().mockResolvedValue({
      enabled: false,
      available: true,
      records: [],
    });
    mocks.setConsolidateEnabled.mockReset().mockResolvedValue({
      enabled: true,
      available: true,
    });
  });

  afterEach(() => {
    if (component) {
      unmount(component);
      component = undefined;
    }
    document.body.innerHTML = "";
  });

  it("reloads audit after enabling and renders real decision counts", async () => {
    mocks.fetchConsolidateAudit
      .mockResolvedValueOnce({ enabled: false, available: true, records: [] })
      .mockResolvedValueOnce({ enabled: false, available: true, records: [] })
      .mockResolvedValueOnce({
        enabled: true,
        available: true,
        records: [
          {
            started_at: "2026-06-26T00:00:00Z",
            candidate_count: 3,
            decisions: [
              { candidate_id: "c-add", action: "ADD", result: "added note" },
              { candidate_id: "c-update", action: "UPDATE", result: "updated note" },
              { candidate_id: "c-skip", action: "SKIP", result: "skip c-skip duplicate" },
            ],
            committed: true,
            resynced: true,
          },
        ],
      });

    component = mount(ConsolidateAuditPanel, { target: document.body });
    await flush();

    const disclosure = document.querySelector<HTMLButtonElement>(
      '[data-testid="consolidate-audit-toggle"]',
    )!;
    disclosure.click();
    await flush();

    const enable = document.querySelector<HTMLButtonElement>(
      '[data-testid="consolidate-enable-toggle"]',
    )!;
    enable.click();
    await flush();

    expect(mocks.setConsolidateEnabled).toHaveBeenCalledWith(true);
    expect(mocks.fetchConsolidateAudit).toHaveBeenCalledTimes(3);
    const text = document.body.textContent ?? "";
    expect(text).toContain("3 candidate(s)");
    expect(text).toContain("2 written");
    expect(text).toContain("1 rejected");
    expect(text).toContain("ADD");
    expect(text).toContain("UPDATE");
    expect(text).toContain("SKIP");
  });
});
