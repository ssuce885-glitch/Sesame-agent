import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { I18nProvider } from "../i18n";
import { ContextPage } from "./ContextPage";

const mocks = vi.hoisted(() => ({
  updateProjectState: vi.fn(),
  setSetting: vi.fn(),
  createMemory: vi.fn(),
  deleteMemory: vi.fn(),
  refetchMemories: vi.fn(),
}));

vi.mock("../api/queries", () => ({
  useProjectState: () => ({
    data: {
      workspace_root: "/workspace",
      summary: "Current project state",
      source_session_id: "session_1",
      created_at: "2026-05-03T00:00:00Z",
      updated_at: "2026-05-03T00:00:01Z",
    },
  }),
  useUpdateProjectState: () => ({
    mutate: mocks.updateProjectState,
    isPending: false,
    error: null,
  }),
  useSetting: () => ({
    data: { key: "project_state_auto", value: "true" },
  }),
  useSetSetting: () => ({
    mutate: mocks.setSetting,
  }),
  useMemories: () => ({
    data: [
      {
        id: "memory_1",
        workspace_root: "/workspace",
        kind: "decision",
        content: "Keep runtime context separate from durable memory.",
        source: "session_1",
        confidence: 0.9,
        created_at: "2026-05-03T00:00:02Z",
        updated_at: "2026-05-03T00:00:03Z",
      },
    ],
    isLoading: false,
    isError: false,
    refetch: mocks.refetchMemories,
  }),
  useCreateMemory: () => ({
    mutate: mocks.createMemory,
    isPending: false,
    error: null,
  }),
  useDeleteMemory: () => ({
    mutate: mocks.deleteMemory,
    isPending: false,
  }),
}));

describe("ContextPage", () => {
  it("updates project state and manages memory entries", () => {
    render(
      <I18nProvider>
        <ContextPage workspaceRoot="/workspace" sessionId="session_1" />
      </I18nProvider>,
    );

    fireEvent.change(screen.getByLabelText("Project State"), {
      target: { value: "Updated project state" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Save Project State" }));

    expect(mocks.updateProjectState).toHaveBeenCalledWith({
      workspace_root: "/workspace",
      summary: "Updated project state",
      source_session_id: "session_1",
    });

    fireEvent.click(screen.getByLabelText("Auto-update project state"));
    expect(mocks.setSetting).toHaveBeenCalledWith("false");

    expect(screen.getByText("Keep runtime context separate from durable memory.")).toBeInTheDocument();

    fireEvent.change(screen.getByLabelText("Kind"), { target: { value: "fact" } });
    fireEvent.change(screen.getByLabelText("Content"), { target: { value: "Use one memory table." } });
    fireEvent.change(screen.getByLabelText("Source"), { target: { value: "manual" } });
    fireEvent.click(screen.getByRole("button", { name: "Save Memory" }));

    expect(mocks.createMemory).toHaveBeenCalledWith(
      expect.objectContaining({
        workspace_root: "/workspace",
        kind: "fact",
        content: "Use one memory table.",
        source: "manual",
        confidence: 1,
      }),
      expect.any(Object),
    );

    fireEvent.click(screen.getByRole("button", { name: "Delete" }));
    expect(mocks.deleteMemory).toHaveBeenCalledWith("memory_1");
  });
});
