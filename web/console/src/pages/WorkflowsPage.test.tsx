import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { I18nProvider } from "../i18n";
import { WorkflowsPage } from "./WorkflowsPage";

const LOCALE_STORAGE_KEY = "sesame-console.locale";
const KEY_WARNING_PATTERN = /unique\s+"?key"?|Encountered two children|same key/i;

const mocks = vi.hoisted(() => ({
  roles: [{ id: "reviewer", name: "Reviewer" }],
  workflows: [] as Array<Record<string, unknown>>,
  latestRuns: [] as Array<Record<string, unknown>>,
  selectedRuns: [] as Array<Record<string, unknown>>,
  linkedRun: null as Record<string, unknown> | null,
  workflowsLoading: false,
  workflowsError: false,
  selectedRunsLoading: false,
  selectedRunsError: false,
  workflowsRefetch: vi.fn(),
  latestRunsRefetch: vi.fn(),
  selectedRunsRefetch: vi.fn(),
  createMutate: vi.fn(),
  updateMutate: vi.fn(),
  triggerMutate: vi.fn(),
}));

vi.mock("../api/queries", () => ({
  useRoles: () => ({
    data: mocks.roles,
  }),
  useWorkflows: () => ({
    data: mocks.workflows,
    isLoading: mocks.workflowsLoading,
    isError: mocks.workflowsError,
    refetch: mocks.workflowsRefetch,
  }),
  useWorkflowRuns: (_workspaceRoot: string | null, filters: { workflow_id?: string } = {}) => {
    const isSelectedWorkflowRuns = !!filters.workflow_id;
    return {
      data: isSelectedWorkflowRuns ? mocks.selectedRuns : mocks.latestRuns,
      isLoading: isSelectedWorkflowRuns ? mocks.selectedRunsLoading : false,
      isError: isSelectedWorkflowRuns ? mocks.selectedRunsError : false,
      refetch: isSelectedWorkflowRuns ? mocks.selectedRunsRefetch : mocks.latestRunsRefetch,
    };
  },
  useWorkflowRun: (runId: string | null) => ({
    data: runId && mocks.linkedRun && mocks.linkedRun.id === runId ? mocks.linkedRun : undefined,
    isLoading: false,
    isError: false,
  }),
  useCreateWorkflow: () => ({
    mutate: mocks.createMutate,
    isPending: false,
    error: null,
  }),
  useUpdateWorkflow: () => ({
    mutate: mocks.updateMutate,
    isPending: false,
    error: null,
  }),
  useTriggerWorkflow: () => ({
    mutate: mocks.triggerMutate,
    isPending: false,
    error: null,
  }),
}));

function renderPage(route = "/") {
  return render(
    <I18nProvider>
      <MemoryRouter initialEntries={[route]}>
        <WorkflowsPage workspaceRoot="/workspace" />
      </MemoryRouter>
    </I18nProvider>,
  );
}

function workflowFixture(overrides: Record<string, unknown> = {}) {
  return {
    id: "wf_review",
    workspace_root: "/workspace",
    name: "Review workflow",
    trigger: "manual",
    owner_role: "reviewer",
    steps: '[{"kind":"role_task","role_id":"reviewer","prompt":"Review the patch","title":"Review"}]',
    created_at: "2026-05-03T00:00:00Z",
    updated_at: "2026-05-03T00:00:01Z",
    ...overrides,
  };
}

function runFixture(overrides: Record<string, unknown> = {}) {
  return {
    id: "run_review",
    workflow_id: "wf_review",
    workspace_root: "/workspace",
    state: "completed",
    trigger_ref: "manual:web",
    task_ids: '["task_123"]',
    report_ids: '["report_123"]',
    trace: '[{"event":"run_completed","state":"completed","message":"Workflow step complete."}]',
    created_at: "2026-05-03T00:00:02Z",
    updated_at: "2026-05-03T00:00:03Z",
    ...overrides,
  };
}

function formatConsoleMessages(calls: unknown[][]) {
  return calls.map((args) =>
    args
      .map((arg) => {
        if (typeof arg === "string") {
          return arg;
        }
        if (arg instanceof Error) {
          return arg.message;
        }
        return String(arg);
      })
      .join(" "),
  );
}

describe("WorkflowsPage", () => {
  afterEach(() => {
    cleanup();
  });

  beforeEach(() => {
    const restoreStack: Array<() => void> = [];
    const previousLocale = window.localStorage.getItem(LOCALE_STORAGE_KEY);

    restoreStack.push(() => {
      if (previousLocale === null) {
        window.localStorage.removeItem(LOCALE_STORAGE_KEY);
      } else {
        window.localStorage.setItem(LOCALE_STORAGE_KEY, previousLocale);
      }
    });

    window.localStorage.setItem(LOCALE_STORAGE_KEY, "en-US");
    mocks.workflows = [];
    mocks.latestRuns = [];
    mocks.selectedRuns = [];
    mocks.linkedRun = null;
    mocks.workflowsLoading = false;
    mocks.workflowsError = false;
    mocks.selectedRunsLoading = false;
    mocks.selectedRunsError = false;
    mocks.workflowsRefetch.mockReset();
    mocks.latestRunsRefetch.mockReset();
    mocks.selectedRunsRefetch.mockReset();
    mocks.createMutate.mockReset();
    mocks.updateMutate.mockReset();
    mocks.triggerMutate.mockReset();

    return () => {
      while (restoreStack.length > 0) {
        restoreStack.pop()?.();
      }
    };
  });

  it("renders the workflow list and recent runs", async () => {
    mocks.workflows = [
      {
        id: "wf_review",
        workspace_root: "/workspace",
        name: "Review workflow",
        trigger: "manual",
        owner_role: "reviewer",
        steps: '[{"kind":"role_task","role_id":"reviewer","prompt":"Review the patch","title":"Review"}]',
        created_at: "2026-05-03T00:00:00Z",
        updated_at: "2026-05-03T00:00:01Z",
      },
    ];
    mocks.latestRuns = [
      {
        id: "run_latest",
        workflow_id: "wf_review",
        workspace_root: "/workspace",
        state: "completed",
        trigger_ref: "manual:web",
        task_ids: '["task_latest"]',
        report_ids: '["report_latest"]',
        trace: '[{"event":"run_completed","state":"completed","message":"Done"}]',
        created_at: "2026-05-03T00:00:02Z",
        updated_at: "2026-05-03T00:00:03Z",
      },
    ];
    mocks.selectedRuns = [
      {
        id: "run_selected",
        workflow_id: "wf_review",
        workspace_root: "/workspace",
        state: "completed",
        trigger_ref: "manual:web",
        task_ids: '["task_123"]',
        report_ids: '["report_123"]',
        trace: '[{"event":"run_completed","state":"completed","message":"Workflow step complete."}]',
        created_at: "2026-05-03T00:00:02Z",
        updated_at: "2026-05-03T00:00:03Z",
      },
    ];

    renderPage();

    expect(await screen.findByDisplayValue("Review workflow")).toBeInTheDocument();
    expect(screen.getAllByText("completed").length).toBeGreaterThan(0);
    expect(screen.getByText("manual:web")).toBeInTheDocument();
    expect(screen.getByText("task_123")).toBeInTheDocument();
    expect(screen.getByText(/Workflow step complete/)).toBeInTheDocument();
  });

  it("renders structured trace fields for workflow runs", async () => {
    mocks.workflows = [workflowFixture()];
    mocks.selectedRuns = [
      runFixture({
        id: "run_structured_trace",
        state: "waiting_approval",
        task_ids: '["task_summary_123"]',
        report_ids: '["report_summary_123"]',
        trace: '[{"event":"task_created","state":"running","kind":"role_task","task_id":"task_trace_123","message":"Review started","time":"2026-05-03T00:00:02Z"},{"event":"approval_requested","state":"pending","kind":"approval","approval_id":"approval_trace_123","message":"Approve the release","time":"2026-05-03T00:00:03Z"}]',
      }),
    ];

    renderPage();

    expect(await screen.findByDisplayValue("Review workflow")).toBeInTheDocument();
    expect(screen.getByText("Event")).toBeInTheDocument();
    expect(screen.getByText("task_created")).toBeInTheDocument();
    expect(screen.getByText("approval_requested")).toBeInTheDocument();
    expect(screen.getByText("Review started")).toBeInTheDocument();
    expect(screen.getByText("Approve the release")).toBeInTheDocument();
    expect(screen.getByText("task_trace_123")).toBeInTheDocument();
    expect(screen.getByText("approval_trace_123")).toBeInTheDocument();
  });

  it("shows an empty value for empty array trace payloads", async () => {
    mocks.workflows = [workflowFixture()];
    mocks.selectedRuns = [
      runFixture({
        id: "run_empty_trace",
        task_ids: '["task_empty_trace"]',
        report_ids: '["report_empty_trace"]',
        trace: "[]",
      }),
    ];

    renderPage();

    expect(await screen.findByDisplayValue("Review workflow")).toBeInTheDocument();
    expect(screen.queryByText("Event")).not.toBeInTheDocument();

    const traceField = screen.getByText("Trace").parentElement;
    expect(traceField).not.toBeNull();
    expect(within(traceField as HTMLElement).getByText("-")).toBeInTheDocument();
  });

  it("falls back to inline text for invalid trace JSON", async () => {
    mocks.workflows = [workflowFixture()];
    mocks.selectedRuns = [
      runFixture({
        id: "run_invalid_trace",
        trace: "{bad json",
      }),
    ];

    renderPage();

    expect(await screen.findByDisplayValue("Review workflow")).toBeInTheDocument();
    expect(screen.queryByText("Event")).not.toBeInTheDocument();
    expect(screen.getByText("{bad json")).toBeInTheDocument();
  });

  it("falls back to inline text for non-array trace JSON", async () => {
    mocks.workflows = [workflowFixture()];
    mocks.selectedRuns = [
      runFixture({
        id: "run_object_trace",
        trace: '{"event":"task_created"}',
      }),
    ];

    renderPage();

    expect(await screen.findByDisplayValue("Review workflow")).toBeInTheDocument();
    expect(screen.queryByText("Event")).not.toBeInTheDocument();
    expect(screen.getByText('{"event":"task_created"}')).toBeInTheDocument();
  });

  it("keeps duplicate trace event timestamps renderable without key collisions", async () => {
    mocks.workflows = [workflowFixture()];
    mocks.selectedRuns = [
      runFixture({
        id: "run_duplicate_trace_keys",
        trace: '[{"event":"task_created","state":"running","task_id":"task_a","time":"2026-05-03T00:00:02Z"},{"event":"task_created","state":"running","task_id":"task_b","time":"2026-05-03T00:00:02Z"}]',
      }),
    ];

    const errorSpy = vi.spyOn(console, "error");
    const warnSpy = vi.spyOn(console, "warn");

    try {
      renderPage();

      expect(await screen.findByDisplayValue("Review workflow")).toBeInTheDocument();

      const traceTable = screen.getByRole("table");
      expect(within(traceTable).getAllByText("task_created")).toHaveLength(2);
      expect(within(traceTable).getByText("task_a")).toBeInTheDocument();
      expect(within(traceTable).getByText("task_b")).toBeInTheDocument();
      const keyRelatedConsoleMessages = [...formatConsoleMessages(errorSpy.mock.calls), ...formatConsoleMessages(warnSpy.mock.calls)].filter(
        (message) => KEY_WARNING_PATTERN.test(message),
      );

      expect(keyRelatedConsoleMessages).toEqual([]);
    } finally {
      errorSpy.mockRestore();
      warnSpy.mockRestore();
    }
  });

  it("renders only the last 200 structured trace events", async () => {
    const trace = JSON.stringify(
      Array.from({ length: 205 }, (_, index) => ({
        event: "task_created",
        state: "running",
        task_id: `task_${index}`,
        message: `Message ${index}`,
        time: "2026-05-03T00:00:02Z",
      })),
    );
    mocks.workflows = [workflowFixture()];
    mocks.selectedRuns = [
      runFixture({
        id: "run_large_trace",
        trace,
      }),
    ];

    renderPage();

    expect(await screen.findByDisplayValue("Review workflow")).toBeInTheDocument();
    expect(screen.queryByText("task_4")).not.toBeInTheDocument();
    expect(screen.getByText("task_5")).toBeInTheDocument();
    expect(screen.getByText("task_204")).toBeInTheDocument();
  });

  it("slices trace events before filtering invalid tail entries", async () => {
    const earlyEvents = Array.from({ length: 20 }, (_, index) =>
      index % 2 === 0
        ? {
            event: "task_created",
            state: "running",
            task_id: `task_early_${index}`,
            message: `Early message ${index}`,
            time: "2026-05-03T00:00:02Z",
          }
        : null,
    );
    const tailEvents = [
      ...Array.from({ length: 180 }, (_, index) => ({
        event: "task_created",
        state: "running",
        task_id: `task_tail_${index}`,
        message: `Tail message ${index}`,
        time: "2026-05-03T00:00:02Z",
      })),
      ...Array.from({ length: 20 }, () => ({})),
    ];

    mocks.workflows = [workflowFixture()];
    mocks.selectedRuns = [
      runFixture({
        id: "run_slice_before_filter",
        trace: JSON.stringify([...earlyEvents, ...tailEvents]),
      }),
    ];

    renderPage();

    expect(await screen.findByDisplayValue("Review workflow")).toBeInTheDocument();

    const traceTable = screen.getByRole("table");
    const renderedRows = traceTable.querySelectorAll("tbody tr");

    expect(screen.queryByText("task_early_0")).not.toBeInTheDocument();
    expect(within(traceTable).getByText("task_tail_0")).toBeInTheDocument();
    expect(within(traceTable).getByText("task_tail_179")).toBeInTheDocument();
    expect(renderedRows.length).toBe(180);
  });

  it("creates a workflow from the form", () => {
    renderPage();

    fireEvent.change(screen.getByLabelText("Name"), { target: { value: "Workspace review" } });
    fireEvent.change(screen.getByLabelText("Template prompt"), { target: { value: "Inspect runtime health" } });
    fireEvent.click(screen.getByRole("button", { name: "Save workflow" }));

    expect(mocks.createMutate).toHaveBeenCalledWith(
      expect.objectContaining({
        workspace_root: "/workspace",
        name: "Workspace review",
        trigger: "manual",
        owner_role: "reviewer",
        steps: expect.stringContaining("Inspect runtime health"),
      }),
      expect.any(Object),
    );
  });

  it("triggers the selected workflow and shows the new run after refresh", async () => {
    mocks.workflows = [
      {
        id: "wf_review",
        workspace_root: "/workspace",
        name: "Review workflow",
        trigger: "manual",
        owner_role: "reviewer",
        steps: '[{"kind":"role_task","role_id":"reviewer","prompt":"Review the patch","title":"Review"}]',
        created_at: "2026-05-03T00:00:00Z",
        updated_at: "2026-05-03T00:00:01Z",
      },
    ];
    mocks.triggerMutate.mockImplementation((_input, options?: { onSuccess?: (run: Record<string, unknown>) => void }) => {
      const run = {
        id: "run_triggered",
        workflow_id: "wf_review",
        workspace_root: "/workspace",
        state: "completed",
        trigger_ref: "manual:web",
        task_ids: '["task_triggered"]',
        report_ids: '["report_triggered"]',
        trace: '[{"event":"run_completed","state":"completed","message":"Triggered from web"}]',
        created_at: "2026-05-03T00:00:04Z",
        updated_at: "2026-05-03T00:00:05Z",
      };
      mocks.selectedRuns = [run];
      mocks.latestRuns = [run];
      options?.onSuccess?.(run);
    });

    const view = renderPage();

    expect(await screen.findByDisplayValue("Review workflow")).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Trigger" }));

    expect(mocks.triggerMutate).toHaveBeenCalledWith(
      {
        workflowId: "wf_review",
        input: { trigger_ref: "manual:web" },
      },
      expect.any(Object),
    );
    expect(mocks.selectedRunsRefetch).toHaveBeenCalled();

    view.rerender(
      <I18nProvider>
        <MemoryRouter>
          <WorkflowsPage workspaceRoot="/workspace" />
        </MemoryRouter>
      </I18nProvider>,
    );

    expect(screen.getByText("task_triggered")).toBeInTheDocument();
    expect(screen.getByText(/Triggered from web/)).toBeInTheDocument();
  });

  it("shows empty and error states", () => {
    const view = renderPage();

    expect(screen.getByText("No workflows configured.")).toBeInTheDocument();

    mocks.workflowsError = true;
    view.rerender(
      <I18nProvider>
        <MemoryRouter>
          <WorkflowsPage workspaceRoot="/workspace" />
        </MemoryRouter>
      </I18nProvider>,
    );

    expect(screen.getByText("Failed to load workflows.")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Retry" }));
    expect(mocks.workflowsRefetch).toHaveBeenCalled();
  });

  it("selects the linked workflow run from the query string", async () => {
    mocks.workflows = [
      {
        id: "wf_review",
        workspace_root: "/workspace",
        name: "Review workflow",
        trigger: "manual",
        owner_role: "reviewer",
        steps: '[{"kind":"role_task","role_id":"reviewer","prompt":"Review the patch","title":"Review"}]',
        created_at: "2026-05-03T00:00:00Z",
        updated_at: "2026-05-03T00:00:01Z",
      },
    ];
    mocks.linkedRun = {
      id: "run_linked",
      workflow_id: "wf_review",
      workspace_root: "/workspace",
      state: "running",
      trigger_ref: "automation:docs-stale",
      task_ids: '["task_linked"]',
      report_ids: '["report_linked"]',
      trace: '[{"event":"task_created","state":"running","message":"Linked run"}]',
      created_at: "2026-05-03T00:00:02Z",
      updated_at: "2026-05-03T00:00:03Z",
    };

    renderPage("/workflows?run_id=run_linked");

    expect(await screen.findByDisplayValue("Review workflow")).toBeInTheDocument();
    expect(screen.getByText("task_linked")).toBeInTheDocument();
  });
});
