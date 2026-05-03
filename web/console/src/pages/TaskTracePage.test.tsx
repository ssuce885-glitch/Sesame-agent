import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { describe, expect, it, vi } from "vitest";
import { I18nProvider } from "../i18n";
import { TaskTracePage } from "./TaskTracePage";

vi.mock("../api/queries", () => ({
  useTaskTrace: () => ({
    data: {
      task: {
        id: "task_123",
        workspace_root: "/workspace",
        session_id: "role_session",
        role_id: "reviewer",
        turn_id: "role_turn",
        parent_session_id: "main_session",
        parent_turn_id: "main_turn",
        report_session_id: "main_session",
        kind: "agent",
        state: "running",
        prompt: "Inspect the runtime.",
        output_path: "/tmp/task.log",
        created_at: "2026-05-03T00:00:00Z",
        updated_at: "2026-05-03T00:00:01Z",
      },
      parent: { session_id: "main_session", turn_id: "main_turn" },
      role: { id: "reviewer", session_id: "role_session", turn_id: "role_turn" },
      state: { task: "running", turn: "running", session: "running" },
      messages: [
        {
          session_id: "role_session",
          turn_id: "role_turn",
          role: "assistant",
          content: "Checking logs.",
          position: 1,
          created_at: "2026-05-03T00:00:02Z",
        },
      ],
      events: [
        {
          id: "event_1",
          seq: 7,
          session_id: "role_session",
          turn_id: "role_turn",
          type: "tool_call",
          time: "2026-05-03T00:00:03Z",
          payload: "{\"name\":\"shell\"}",
        },
      ],
      reports: [
        {
          id: "report_1",
          session_id: "main_session",
          source_kind: "task_result",
          source_id: "task_123",
          title: "Task result: agent",
          summary: "Still running.",
          severity: "info",
          status: "running",
          delivered: false,
          created_at: "2026-05-03T00:00:04Z",
        },
      ],
      log_preview: "line 1",
      log_bytes: 6,
      log_truncated: false,
    },
    isLoading: false,
    isError: false,
    refetch: vi.fn(),
  }),
}));

describe("TaskTracePage", () => {
  it("renders task linkage, events, messages, reports, and logs", () => {
    render(
      <I18nProvider>
        <MemoryRouter>
          <TaskTracePage taskId="task_123" />
        </MemoryRouter>
      </I18nProvider>,
    );

    expect(screen.getByRole("heading", { name: "Task Trace" })).toBeInTheDocument();
    expect(screen.getByText("reviewer")).toBeInTheDocument();
    expect(screen.getByText(/Parent session:/)).toBeInTheDocument();
    expect(screen.getAllByText("main_session").length).toBeGreaterThan(0);
    expect(screen.getByText("tool_call")).toBeInTheDocument();
    expect(screen.getByText("Checking logs.")).toBeInTheDocument();
    expect(screen.getByText("Still running.")).toBeInTheDocument();
    expect(screen.getByText("line 1")).toBeInTheDocument();
  });
});
