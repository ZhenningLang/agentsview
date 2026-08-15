import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/svelte";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import WorktreeMappingSettings from "./WorktreeMappingSettings.svelte";
import { SettingsService } from "../../api/generated/index";

vi.mock("../../api/runtime.js", () => ({
  callGenerated: vi.fn((request: () => Promise<unknown>) => request()),
}));

vi.mock("../../api/generated/index", () => ({
  SettingsService: {
    getApiV1SettingsWorktreeMappings: vi.fn(),
    postApiV1SettingsWorktreeMappings: vi.fn(),
    putApiV1SettingsWorktreeMappingsId: vi.fn(),
    deleteApiV1SettingsWorktreeMappingsId: vi.fn(),
    postApiV1SettingsWorktreeMappingsApply: vi.fn(),
  },
}));

const settingsService = SettingsService as unknown as {
  getApiV1SettingsWorktreeMappings: ReturnType<typeof vi.fn>;
  postApiV1SettingsWorktreeMappings: ReturnType<typeof vi.fn>;
};

describe("WorktreeMappingSettings", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    settingsService.getApiV1SettingsWorktreeMappings.mockResolvedValue({
      machine: "test",
      mappings: [],
    });
    settingsService.postApiV1SettingsWorktreeMappings.mockResolvedValue({
      id: 1,
      machine: "test",
      path_prefix: "/tmp/projects",
      layout: "repo_dot_worktrees",
      project: "",
      enabled: true,
      created_at: "2026-08-14T00:00:00.000Z",
      updated_at: "2026-08-14T00:00:00.000Z",
    });
  });

  afterEach(() => {
    cleanup();
  });

  it("saves repo.worktrees layout mappings without requiring project input", async () => {
    render(WorktreeMappingSettings);

    await screen.findByText("Machine");
    await fireEvent.change(screen.getByLabelText("Layout"), {
      target: { value: "repo_dot_worktrees" },
    });
    const pathInput = screen.getByLabelText("Parent directory");
    await fireEvent.input(pathInput, { target: { value: "/tmp/projects" } });

    const projectInput = screen.getByLabelText("Project") as HTMLInputElement;
    expect(projectInput.disabled).toBe(true);

    await fireEvent.click(screen.getByRole("button", { name: "Add mapping" }));

    await waitFor(() => {
      expect(
        settingsService.postApiV1SettingsWorktreeMappings,
      ).toHaveBeenCalledWith({
        requestBody: {
          path_prefix: "/tmp/projects",
          layout: "repo_dot_worktrees",
          project: "",
          enabled: true,
        },
      });
    });
  });
});
