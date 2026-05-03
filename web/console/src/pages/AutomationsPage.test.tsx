import { fireEvent, render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { describe, expect, it, vi } from "vitest";
import { I18nProvider } from "../i18n";
import { AutomationsPage } from "./AutomationsPage";

const mocks = vi.hoisted(() => ({
  createMutate: vi.fn(),
  pause: vi.fn(),
  resume: vi.fn(),
}));

vi.mock("../api/queries", () => ({
  useAutomations: () => ({
    data: [
      {
        id: "automation_1",
        workspace_root: "/workspace",
        title: "Watch docs",
        goal: "Keep docs fresh",
        state: "active",
        owner: "role:reviewer",
        watcher_path: "roles/reviewer/automations/watch.sh",
        watcher_cron: "@every 5m",
        created_at: "2026-05-03T00:00:00Z",
        updated_at: "2026-05-03T00:00:01Z",
      },
    ],
    isLoading: false,
    isError: false,
    refetch: vi.fn(),
  }),
  useAutomationRuns: () => ({
    data: [
      {
        automation_id: "automation_1",
        dedupe_key: "docs-stale",
        task_id: "task_123",
        status: "needs_agent",
        summary: "Docs changed.",
        created_at: "2026-05-03T00:00:02Z",
      },
    ],
    isLoading: false,
    isError: false,
  }),
  useRoles: () => ({
    data: [{ id: "reviewer", name: "Reviewer" }],
  }),
  useCreateAutomation: () => ({
    mutate: mocks.createMutate,
    isPending: false,
    error: null,
  }),
  usePauseAutomation: () => ({
    mutateAsync: mocks.pause,
    isPending: false,
  }),
  useResumeAutomation: () => ({
    mutateAsync: mocks.resume,
    isPending: false,
  }),
}));

describe("AutomationsPage", () => {
  it("creates automations and shows run details", () => {
    render(
      <I18nProvider>
        <MemoryRouter>
          <AutomationsPage workspaceRoot="/workspace" />
        </MemoryRouter>
      </I18nProvider>,
    );

    fireEvent.click(screen.getByRole("button", { name: "Create automation" }));
    fireEvent.change(screen.getByLabelText("Title"), { target: { value: "Monitor docs" } });
    fireEvent.change(screen.getByLabelText("Goal"), { target: { value: "Find stale docs" } });
    fireEvent.change(screen.getByLabelText("Watcher path"), {
      target: { value: "roles/reviewer/automations/watch.sh" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Save automation" }));

    expect(mocks.createMutate).toHaveBeenCalledWith(
      expect.objectContaining({
        workspace_root: "/workspace",
        title: "Monitor docs",
        goal: "Find stale docs",
        owner: "role:reviewer",
        watcher_path: "roles/reviewer/automations/watch.sh",
      }),
      expect.any(Object),
    );

    fireEvent.click(screen.getByRole("button", { name: /Watch docs/ }));

    expect(screen.getByText("Docs changed.")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "task_123" })).toBeInTheDocument();
  });
});
