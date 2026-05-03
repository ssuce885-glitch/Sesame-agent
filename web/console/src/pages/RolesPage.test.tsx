import { fireEvent, render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { describe, expect, it, vi } from "vitest";
import { I18nProvider } from "../i18n";
import { RolesPage } from "./RolesPage";

const mocks = vi.hoisted(() => ({
  createTaskMutate: vi.fn(),
}));

vi.mock("../api/queries", () => ({
  useRoles: () => ({
    data: [
      {
        id: "reviewer",
        name: "Reviewer",
        description: "Reviews runtime changes.",
        system_prompt: "Review carefully.",
        permission_profile: "trusted_local",
        model: "test-model",
        version: 1,
      },
    ],
    isLoading: false,
  }),
  useRole: () => ({
    data: {
      id: "reviewer",
      name: "Reviewer",
      description: "Reviews runtime changes.",
      system_prompt: "Review carefully.",
      permission_profile: "trusted_local",
      model: "test-model",
      version: 1,
    },
    isLoading: false,
    isError: false,
    refetch: vi.fn(),
  }),
  useTasks: () => ({
    data: [
      {
        id: "task_recent",
        workspace_root: "/workspace",
        session_id: "role_session",
        role_id: "reviewer",
        kind: "agent",
        state: "completed",
        prompt: "Recent run",
        final_text: "Recent result",
        created_at: "2026-05-03T00:00:00Z",
        updated_at: "2026-05-03T00:00:01Z",
      },
    ],
    isLoading: false,
    isError: false,
  }),
  useCreateTask: () => ({
    mutate: mocks.createTaskMutate,
    isPending: false,
    error: null,
  }),
  useCreateRole: () => ({ mutate: vi.fn(), isPending: false, error: null }),
  useUpdateRole: () => ({ mutate: vi.fn(), isPending: false, error: null }),
}));

describe("RolesPage", () => {
  it("shows recent role runs and starts a test run", () => {
    render(
      <I18nProvider>
        <MemoryRouter>
          <RolesPage workspaceRoot="/workspace" />
        </MemoryRouter>
      </I18nProvider>,
    );

    expect(screen.getByRole("heading", { name: "Reviewer" })).toBeInTheDocument();
    expect(screen.getByText("Recent result")).toBeInTheDocument();

    fireEvent.change(screen.getByPlaceholderText(/Optional test prompt/), {
      target: { value: "Run diagnostics" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Run Test" }));

    expect(mocks.createTaskMutate).toHaveBeenCalledWith(
      expect.objectContaining({
        workspace_root: "/workspace",
        role_id: "reviewer",
        kind: "agent",
        prompt: "Run diagnostics",
      }),
      expect.any(Object),
    );
  });
});
