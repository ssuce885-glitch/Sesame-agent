import { useEffect } from "react";
import { fireEvent, render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { describe, expect, it, vi } from "vitest";
import { I18nProvider } from "./i18n";
import { AppShell } from "./App";

vi.mock("./api/queries", () => ({
  useSession: () => ({
    data: {
      id: "session-1234567890",
      workspace_root: "/tmp/test-workspace",
    },
  }),
}));

vi.mock("./pages/ChatPage", () => ({
  ChatPage: ({ onConnectionChange }: { onConnectionChange?: (value: "open") => void }) => {
    useEffect(() => {
      onConnectionChange?.("open");
    }, [onConnectionChange]);
    return <div>Chat page</div>;
  },
}));

vi.mock("./pages/AutomationsPage", () => ({
  AutomationsPage: () => <div>Automations page</div>,
}));

vi.mock("./pages/WorkflowsPage", () => ({
  WorkflowsPage: () => <div>Workflows page</div>,
}));

vi.mock("./pages/ReportsPage", () => ({
  ReportsPage: () => <div>Reports page</div>,
}));

vi.mock("./pages/TaskTracePage", () => ({
  TaskTracePage: ({ taskId }: { taskId: string }) => <div>Task trace page {taskId}</div>,
}));

vi.mock("./pages/TasksPage", () => ({
  TasksPage: () => <div>Tasks page</div>,
}));

vi.mock("./pages/ContextPage", () => ({
  ContextPage: () => <div>Context page</div>,
}));

vi.mock("./pages/RolesPage", () => ({
  RolesPage: () => <div>Roles page</div>,
}));

describe("AppShell", () => {
  it("resets sidebar connection status to idle when leaving chat", async () => {
    render(
      <I18nProvider>
        <MemoryRouter initialEntries={["/chat"]}>
          <AppShell />
        </MemoryRouter>
      </I18nProvider>,
    );

    expect(await screen.findByText("Chat page")).toBeInTheDocument();

    fireEvent.click(screen.getByTitle("Expand"));
    expect(await screen.findByText(/Connected/)).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Automations" }));

    expect(await screen.findByText("Automations page")).toBeInTheDocument();
    expect(await screen.findByText(/Idle/)).toBeInTheDocument();
  });
});
