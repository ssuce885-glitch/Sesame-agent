import { fireEvent, render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { describe, expect, it, vi } from "vitest";
import { I18nProvider } from "../i18n";
import { TasksPage } from "./TasksPage";

const cancel = vi.fn();

vi.mock("../api/queries", () => ({
  useTasks: () => ({
    data: [
      {
        id: "task_running",
        workspace_root: "/workspace",
        session_id: "role_session",
        role_id: "reviewer",
        turn_id: "turn_1",
        kind: "agent",
        state: "running",
        prompt: "Inspect runtime",
        created_at: "2026-05-03T00:00:00Z",
        updated_at: "2026-05-03T00:00:01Z",
      },
      {
        id: "task_done",
        workspace_root: "/workspace",
        session_id: "role_session",
        role_id: "writer",
        turn_id: "turn_2",
        kind: "agent",
        state: "completed",
        prompt: "Write report",
        final_text: "Report complete",
        created_at: "2026-05-03T00:00:00Z",
        updated_at: "2026-05-03T00:00:02Z",
      },
    ],
    isLoading: false,
    isError: false,
    refetch: vi.fn(),
  }),
  useCancelTask: () => ({
    mutate: cancel,
    isPending: false,
  }),
}));

describe("TasksPage", () => {
  it("renders task monitor rows and cancels active tasks", () => {
    render(
      <I18nProvider>
        <MemoryRouter>
          <TasksPage workspaceRoot="/workspace" />
        </MemoryRouter>
      </I18nProvider>,
    );

    expect(screen.getByRole("heading", { name: "Tasks" })).toBeInTheDocument();
    expect(screen.getAllByText("reviewer").length).toBeGreaterThan(0);
    expect(screen.getAllByText("writer").length).toBeGreaterThan(0);
    expect(screen.getByText("Report complete")).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));

    expect(cancel).toHaveBeenCalledWith("task_running");
  });
});
