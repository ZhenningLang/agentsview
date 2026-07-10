// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { mount, tick, unmount } from "svelte";

const mocks = vi.hoisted(() => ({
  fetchMemories: vi.fn(),
  fetchMemory: vi.fn(),
  fetchMemoryRaw: vi.fn(),
  putMemory: vi.fn(),
  deleteMemory: vi.fn(),
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
  deleteMemory: mocks.deleteMemory,
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
  for (let i = 0; i < 20; i++) {
    await flush();
    if ((document.body.textContent ?? "").includes(text)) return;
  }
}

async function clickByText(text: string) {
  const buttons = [...document.querySelectorAll("button")];
  const button = buttons.find((b) => (b.textContent ?? "").includes(text));
  expect(button, `button containing ${text}`).toBeTruthy();
  button?.click();
  await flush();
}

async function clickByAriaLabel(label: string) {
  const button = document.querySelector<HTMLButtonElement>(`button[aria-label="${label}"]`);
  expect(button, `button with aria-label ${label}`).toBeTruthy();
  button?.click();
  await flush();
}

async function selectSource(source: string) {
  const select = document.querySelector<HTMLSelectElement>('select[aria-label="source 过滤"]');
  expect(select).toBeTruthy();
  select!.value = source;
  select!.dispatchEvent(new Event("change", { bubbles: true }));
  await flush();
}

function memory(overrides: Record<string, unknown>) {
  return {
    rel_path: "memory.md",
    source: "assist-mem",
    title: "Memory",
    date: "2026-07-01",
    problem_type: "explicit",
    type: "preference",
    status: "active",
    origin_session: "assist-mem:fixture",
    origin_project: "",
    feedback_vote: "",
    feedback_comment: "",
    feedback_status: "",
    body: "Memory body.",
    body_tokens: 12,
    source_mtime: 1,
    synced_at: "2026-07-01T00:00:00.000Z",
    ...overrides,
  };
}

const assistRaw = memory({
  rel_path: "assist-mem/lzn-entrypoint.jsonl",
  source: "assist-mem",
  title: "lzn-preview entrypoint raw",
  date: "2026-07-01 21:36:35",
  problem_type: "explicit",
  type: "preference",
  origin_session: "assist-mem:lzn-entrypoint",
  body: "Use /assist-mem for long-term memory.",
});

const canonicalEntrypoint = memory({
  rel_path: "canonical/lzn-entrypoint.json",
  source: "canonical",
  title: "lzn-preview current entrypoint",
  date: "2026-07-02",
  problem_type: "canonical",
  type: "generated",
  origin_session: "canonical:entrypoint",
  body: "Current entrypoint is /assist-mem.",
  canonical_covered_refs: JSON.stringify([
    { source: "assist-mem", rel_path: "assist-mem/lzn-entrypoint.jsonl" },
  ]),
  canonical_provenance: JSON.stringify({
    topic: "entrypoint",
    version: "test",
    confidence: "high",
  }),
});

const crossSecurity = memory({
  rel_path: "security-exception.md",
  source: "cross-agent",
  title: "lzn security exception",
  date: "2026-06-30",
  problem_type: "security",
  type: "exception",
  origin_session: "assist-learn:security",
  body: "Keep the lzn-test security exception separate.",
});

const ccEnvironment = memory({
  rel_path: "claude/projects/lzn-preview/environment.md",
  source: "cc-native",
  title: "lzn-preview environment fact",
  date: "2026-06-29",
  problem_type: "environment",
  type: "fact",
  origin_session: "cc-native:lzn-preview",
  body: "lzn-preview runs in the local preview environment.",
});

const foldedLegacy = memory({
  rel_path: "commit-diff.md",
  source: "cross-agent",
  title: "Commit diff",
  date: "2026-06-20",
  problem_type: "knowledge",
  type: "semantic",
  status: "stale",
  origin_session: "assist-learn:def",
  body: "Old atomic source folded into a topic.",
});

const allRows = [assistRaw, canonicalEntrypoint, crossSecurity, ccEnvironment, foldedLegacy];

describe("MemoryPage", () => {
  let component: ReturnType<typeof mount> | undefined;

  beforeEach(() => {
    mocks.fetchMemories.mockReset().mockImplementation((filter = {}) => {
      const source = (filter as { source?: string }).source;
      if (!source) return Promise.resolve(allRows);
      return Promise.resolve(allRows.filter((m) => m.source === source));
    });
    mocks.fetchMemory.mockReset().mockImplementation((relPath: string) => {
      const row = allRows.find((m) => m.rel_path === relPath);
      return Promise.resolve(row ?? memory({ rel_path: relPath }));
    });
    mocks.fetchMemoryRaw.mockReset().mockResolvedValue({ content: "", sha: "sha" });
    mocks.putMemory.mockReset().mockResolvedValue({ sha: "new-sha" });
    mocks.deleteMemory.mockReset().mockResolvedValue({});
    mocks.fetchMemoryHistory.mockReset().mockResolvedValue([]);
    mocks.fetchStagingPool.mockReset();
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

  it("defaults to explicit assist-mem ledger memories", async () => {
    component = mount(MemoryPage, { target: document.body });
    await waitForText("lzn-preview entrypoint raw");

    const text = document.body.textContent ?? "";
    expect(mocks.fetchMemories).toHaveBeenCalledWith({ source: "assist-mem" });
    expect(text).toContain("Explicit Ledger Only");
    expect(text).toContain("active assist-mem entries");
    expect(text).toContain("2026-07-01 21:36:35");
    expect(text).toContain("全部 raw/canonical 来源用于核对");
    expect(text).toContain("lzn-preview entrypoint raw");
    expect(text).not.toContain("lzn-preview current entrypoint");
    expect(text).not.toContain("反馈");
    expect(text).not.toContain("保存反馈");
    expect(text).not.toContain("Inbox → Evidence → Knowledge");
    expect(text).not.toContain("候选入口");
  });

  it("refreshes both the filtered list and the overview catalog", async () => {
    const secondAssist = memory({
      rel_path: "assist-mem/kilo-session.jsonl",
      source: "assist-mem",
      title: "kilo session entrypoint",
      date: "2026-07-10 23:17:35",
      origin_session: "assist-mem:kilo-session",
    });
    let rows = allRows;
    mocks.fetchMemories.mockImplementation((filter = {}) => {
      const source = (filter as { source?: string }).source;
      const result = source ? rows.filter((m) => m.source === source) : rows;
      return Promise.resolve(result);
    });

    component = mount(MemoryPage, { target: document.body });
    await waitForText("1 active assist-mem entries");

    rows = [...allRows, secondAssist];
    await clickByAriaLabel("刷新");

    expect(document.body.textContent ?? "").toContain("2 active assist-mem entries");
    expect(document.body.textContent ?? "").toContain("kilo session entrypoint");
  });

  it("keeps the newest overview catalog when an older request resolves late", async () => {
    const secondAssist = memory({
      rel_path: "assist-mem/kilo-session.jsonl",
      source: "assist-mem",
      title: "kilo session entrypoint",
      date: "2026-07-10 23:17:35",
      origin_session: "assist-mem:kilo-session",
    });
    const newRows = [...allRows, secondAssist];
    let resolveOldCatalog!: (rows: typeof allRows) => void;
    const oldCatalog = new Promise<typeof allRows>((resolve) => {
      resolveOldCatalog = resolve;
    });
    let catalogCalls = 0;
    mocks.fetchMemories.mockImplementation((filter = {}) => {
      const source = (filter as { source?: string }).source;
      if (source) return Promise.resolve(newRows.filter((m) => m.source === source));
      catalogCalls++;
      return catalogCalls === 1 ? oldCatalog : Promise.resolve(newRows);
    });

    component = mount(MemoryPage, { target: document.body });
    await clickByAriaLabel("刷新");
    await waitForText("2 active assist-mem entries");

    resolveOldCatalog(allRows);
    await flush();

    expect(document.body.textContent ?? "").toContain("2 active assist-mem entries");
  });

  it("shows canonical rows only through explicit canonical controls with generated labels", async () => {
    component = mount(MemoryPage, { target: document.body });
    await waitForText("lzn-preview entrypoint raw");

    await clickByText("看 canonical");
    await waitForText("lzn-preview current entrypoint");

    const text = document.body.textContent ?? "";
    expect(mocks.fetchMemories).toHaveBeenCalledWith(
      expect.objectContaining({ source: "canonical", status: "active" }),
    );
    expect(text).toContain("generated current-memory rows");
    expect(text).toContain("Canonical generated");
    expect(text).toContain("coverage 1");
    expect(text).not.toContain("lzn-preview entrypoint raw");
  });

  it("shows canonical rows through the source dropdown option", async () => {
    component = mount(MemoryPage, { target: document.body });
    await waitForText("lzn-preview entrypoint raw");

    await selectSource("canonical");
    await waitForText("lzn-preview current entrypoint");

    const text = document.body.textContent ?? "";
    expect(mocks.fetchMemories).toHaveBeenCalledWith(
      expect.objectContaining({ source: "canonical" }),
    );
    expect(text).toContain("Canonical generated");
    expect(text).not.toContain("lzn-preview entrypoint raw");
  });

  it("opens canonical detail with covered raw refs and provenance metadata", async () => {
    component = mount(MemoryPage, { target: document.body });
    await waitForText("lzn-preview entrypoint raw");
    await clickByText("看 canonical");
    await waitForText("lzn-preview current entrypoint");

    document.querySelector<HTMLTableRowElement>("tr.clickable")?.click();
    await waitForText("Canonical coverage");

    const text = document.body.textContent ?? "";
    expect(mocks.fetchMemory).toHaveBeenCalledWith("canonical/lzn-entrypoint.json");
    expect(text).toContain("Canonical coverage");
    expect(text).toContain("1 covered raw ref");
    expect(text).toContain("Assist Mem");
    expect(text).toContain("assist-mem/lzn-entrypoint.jsonl");
    expect(text).toContain("Provenance");
    expect(text).toContain("topic: entrypoint");
    expect(text).toContain("confidence: high");
  });

  it("keeps raw source views reachable for covered assist, security, and environment facts", async () => {
    component = mount(MemoryPage, { target: document.body });
    await waitForText("lzn-preview entrypoint raw");

    await selectSource("cross-agent");
    await waitForText("lzn security exception");
    expect(document.body.textContent ?? "").toContain("Keep the lzn-test security exception separate.");

    await selectSource("cc-native");
    await waitForText("lzn-preview environment fact");
    expect(document.body.textContent ?? "").toContain("local preview environment");

    await selectSource("assist-mem");
    await waitForText("lzn-preview entrypoint raw");
    expect(document.body.textContent ?? "").toContain("Use /assist-mem for long-term memory.");
  });

  it("allows assist-mem detail edit and delete while keeping canonical generated rows read-only", async () => {
    component = mount(MemoryPage, { target: document.body });
    await waitForText("lzn-preview entrypoint raw");

    document.querySelector<HTMLTableRowElement>("tr.clickable")?.click();
    await waitForText("编辑");
    expect(document.body.textContent ?? "").toContain("删除");
    expect(document.body.textContent ?? "").not.toContain("历史");

    await clickByText("编辑");
    expect(mocks.fetchMemoryRaw).toHaveBeenCalledWith("assist-mem/lzn-entrypoint.jsonl");

    await clickByAriaLabel("关闭");
    await clickByText("看 canonical");
    await waitForText("lzn-preview current entrypoint");
    document.querySelector<HTMLTableRowElement>("tr.clickable")?.click();
    await waitForText("Canonical generated/read-only");
    const text = document.body.textContent ?? "";
    expect(text).not.toContain("编辑");
    expect(text).not.toContain("删除");
    expect(text).not.toContain("历史");
  });

  it("deletes assist-mem through the memory delete API and refreshes the list", async () => {
    vi.spyOn(window, "confirm").mockReturnValue(true);
    component = mount(MemoryPage, { target: document.body });
    await waitForText("lzn-preview entrypoint raw");

    document.querySelector<HTMLTableRowElement>("tr.clickable")?.click();
    await waitForText("删除");
    await clickByText("删除");

    expect(mocks.fetchMemoryRaw).toHaveBeenCalledWith("assist-mem/lzn-entrypoint.jsonl");
    expect(mocks.deleteMemory).toHaveBeenCalledWith("assist-mem/lzn-entrypoint.jsonl", "sha");
    expect(mocks.fetchMemories).toHaveBeenCalledWith({ source: "assist-mem" });
  });
});
