// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { mount, tick, unmount } from "svelte";

const mocks = vi.hoisted(() => ({
  fetchMemories: vi.fn(),
  fetchMemory: vi.fn(),
  fetchMemoryRaw: vi.fn(),
  putMemory: vi.fn(),
  setMemoryFeedback: vi.fn(),
  fetchMemoryHistory: vi.fn(),
  fetchMemoryAtCommit: vi.fn(),
  revertMemory: vi.fn(),
  fetchStagingPool: vi.fn(),
  fetchMemoryQuality: vi.fn(),
  fetchConsolidateAudit: vi.fn(),
}));

vi.mock("../../api/memory", () => ({
  fetchMemories: mocks.fetchMemories,
  fetchMemory: mocks.fetchMemory,
  fetchMemoryRaw: mocks.fetchMemoryRaw,
  putMemory: mocks.putMemory,
  setMemoryFeedback: mocks.setMemoryFeedback,
  fetchMemoryHistory: mocks.fetchMemoryHistory,
  fetchMemoryAtCommit: mocks.fetchMemoryAtCommit,
  revertMemory: mocks.revertMemory,
}));

vi.mock("../../api/staging", () => ({
  fetchStagingPool: mocks.fetchStagingPool,
}));

vi.mock("../../api/memoryQuality", () => ({
  fetchMemoryQuality: mocks.fetchMemoryQuality,
}));

vi.mock("../../api/consolidateAudit", () => ({
  fetchConsolidateAudit: mocks.fetchConsolidateAudit,
}));

// @ts-ignore
import MemoryPage from "./MemoryPage.svelte";

async function flush() {
  await Promise.resolve();
  await tick();
  await Promise.resolve();
  await tick();
}

async function waitForText(text: string) {
  for (let i = 0; i < 20; i += 1) {
    if ((document.body.textContent ?? "").includes(text)) return;
    await new Promise((resolve) => setTimeout(resolve, 0));
    await flush();
  }
}

describe("MemoryPage", () => {
  let component: ReturnType<typeof mount> | undefined;

  beforeEach(() => {
    mocks.fetchMemories.mockReset().mockResolvedValue([
      {
        rel_path: "assist-mem/e979b3764a5954c4.jsonl",
        source: "assist-mem",
        title: "Use assist-mem for long-term memory",
        date: "2026-07-01",
        problem_type: "explicit",
        type: "preference",
        status: "active",
        origin_session: "assist-mem:e979b3764a5954c4",
        origin_project: "",
        feedback_vote: "",
        feedback_comment: "",
        feedback_status: "",
        body: "Use /assist-mem for long-term memory.",
        body_tokens: 12,
        source_mtime: 1,
        synced_at: "2026-07-01T00:00:00.000Z",
      },
      {
        rel_path: "diff-ssot.md",
        source: "cross-agent",
        title: "Diff SSOT",
        date: "2026-07-01",
        problem_type: "knowledge",
        type: "semantic",
        status: "active",
        origin_session: "compact-memory:topic-preview",
        origin_project: "",
        feedback_vote: "",
        feedback_comment: "",
        feedback_status: "",
        body: "Use the requested diff as the review SSOT.",
        body_tokens: 12,
        source_mtime: 1,
        synced_at: "2026-07-01T00:00:00.000Z",
      },
      {
        rel_path: "signed-url.md",
        source: "cross-agent",
        title: "Signed URL comparison",
        date: "2026-06-30",
        problem_type: "knowledge",
        type: "semantic",
        status: "active",
        origin_session: "assist-learn:abc",
        origin_project: "",
        feedback_vote: "",
        feedback_comment: "",
        feedback_status: "",
        body: "Exclude signed URLs from state comparisons.",
        body_tokens: 9,
        source_mtime: 2,
        synced_at: "2026-07-01T00:00:00.000Z",
      },
      {
        rel_path: "commit-diff.md",
        source: "cross-agent",
        title: "Commit diff",
        date: "2026-06-20",
        problem_type: "knowledge",
        type: "semantic",
        status: "stale",
        origin_session: "assist-learn:def",
        origin_project: "",
        feedback_vote: "",
        feedback_comment: "",
        feedback_status: "",
        body: "Old atomic source folded into a topic.",
        body_tokens: 8,
        source_mtime: 3,
        synced_at: "2026-07-01T00:00:00.000Z",
      },
      {
        rel_path: "license-2.md",
        source: "cross-agent",
        title: "License old topic",
        date: "2026-06-10",
        problem_type: "knowledge",
        type: "semantic",
        status: "archived",
        origin_session: "compact-memory:old",
        origin_project: "",
        feedback_vote: "",
        feedback_comment: "",
        feedback_status: "",
        body: "Superseded topic.",
        body_tokens: 4,
        source_mtime: 4,
        synced_at: "2026-07-01T00:00:00.000Z",
      },
    ]);
    mocks.fetchStagingPool.mockReset().mockResolvedValue({
      available: true,
      total: 7,
      by_scope: { user: 5, project: 2 },
      projects: {},
      candidates: [],
    });
    mocks.fetchMemoryQuality.mockReset().mockResolvedValue(null);
    mocks.fetchConsolidateAudit.mockReset().mockResolvedValue({ available: false, entries: [] });
  });

  afterEach(() => {
    if (component) {
      unmount(component);
      component = undefined;
    }
    document.body.innerHTML = "";
  });

  it("summarizes the inbox evidence knowledge pipeline", async () => {
    component = mount(MemoryPage, { target: document.body });
    await flush();

    const text = document.body.textContent ?? "";
    expect(text).toContain("Inbox → Evidence → Knowledge");
    expect(text).toContain("Evidence");
    expect(text).toContain("2 active atomics");
    expect(text).toContain("Knowledge");
    expect(text).toContain("1 active topics");
    expect(text).toContain("2 folded / archived");
  });

  it("offers assist-mem in the source filter", async () => {
    component = mount(MemoryPage, { target: document.body });
    await waitForText("Use assist-mem for long-term memory");

    const sourceFilter = document.body.querySelector(
      'select[aria-label="source 过滤"]',
    ) as HTMLSelectElement;
    const options = [...sourceFilter.options].map((option) => ({
      value: option.value,
      text: option.textContent,
    }));

    expect(options).toContainEqual({ value: "assist-mem", text: "Assist Mem" });
  });

  it("uses readable labels and hides redundant assist-mem explicit badges", async () => {
    component = mount(MemoryPage, { target: document.body });
    await waitForText("Use assist-mem for long-term memory");

    const text = document.body.textContent ?? "";
    expect(text).toContain("来源类型");
    expect(text).toContain("记忆类型");
    expect(text).toContain("preference");
    expect(text).not.toContain("problem_type");

    const assistMemRow = [...document.body.querySelectorAll("tr")].find((row) =>
      row.textContent?.includes("Use assist-mem for long-term memory"),
    );
    expect(assistMemRow).toBeTruthy();
    expect(assistMemRow?.textContent ?? "").toContain("preference");
    expect(assistMemRow?.textContent ?? "").not.toContain("explicit");
  });
});
