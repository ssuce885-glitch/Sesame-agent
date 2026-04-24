import { useMemo, useState } from "react";
import {
  useContextHistory,
  useLoadContextHistory,
  useWorkspaceMailbox,
  useWorkspaceRuntimeGraph,
  useReopenContext,
} from "../api/queries";
import type {
  HistoryEntry,
  RuntimeDiagnostic,
  RuntimePermissionRequest,
  RuntimeTask,
  RuntimeToolRun,
  RuntimeWorktree,
  WorkspaceMailboxItem,
} from "../api/types";
import { useI18n } from "../i18n";
import {
  ApprovalRow,
  ContextHeadRow,
  DiagnosticRow,
  formatTimestamp,
  Panel,
  ReportRow,
  SelectionDetailCard,
  sortByCreatedAtDesc,
  sortByUpdatedAtDesc,
  SummaryCard,
  TaskRow,
  ToolRunRow,
  WorktreeRow,
} from "./runtimePageComponents";

interface RuntimePageProps {
  sessionId: string;
}

export function RuntimePage({ sessionId }: RuntimePageProps) {
  const { t } = useI18n();
  const history = useContextHistory(sessionId || null);
  const runtimeGraph = useWorkspaceRuntimeGraph();
  const mailbox = useWorkspaceMailbox();
  const reopenContext = useReopenContext(sessionId);
  const loadContextHistory = useLoadContextHistory(sessionId);

  const contextEntries = [...(history.data?.entries ?? [])].sort(sortByUpdatedAtDesc);
  const tasks = [...(runtimeGraph.data?.graph.tasks ?? [])].sort(sortByUpdatedAtDesc);
  const toolRuns = [...(runtimeGraph.data?.graph.tool_runs ?? [])].sort(sortByUpdatedAtDesc);
  const worktrees = [...(runtimeGraph.data?.graph.worktrees ?? [])].sort(sortByUpdatedAtDesc);
  const diagnostics = [...(runtimeGraph.data?.graph.diagnostics ?? [])].sort(sortByCreatedAtDesc);
  const pendingApprovals = (runtimeGraph.data?.graph.permission_requests ?? []).filter(
    (request) => request.status === "requested",
  );
  const mailboxItems = [...(mailbox.data?.items ?? [])].sort(sortByUpdatedAtDesc);
  const activeTaskCount = tasks.filter((task) => task.state === "running" || task.state === "pending").length;
  const [selection, setSelection] = useState<RuntimeSelection | null>(null);
  const selectedDetail = useMemo(
    () =>
      buildSelectionDetail(
        selection,
        contextEntries,
        tasks,
        mailboxItems,
        pendingApprovals,
        diagnostics,
        toolRuns,
        worktrees,
      ),
    [selection, contextEntries, tasks, mailboxItems, pendingApprovals, diagnostics, toolRuns, worktrees],
  );

  const isInitialLoading =
    (history.isLoading && !history.data) ||
    (runtimeGraph.isLoading && !runtimeGraph.data) ||
    (mailbox.isLoading && !mailbox.data);

  if (isInitialLoading) {
    return (
      <div className="flex h-full items-center justify-center text-sm" style={{ color: "var(--color-text-muted)" }}>
        {t("runtime.loading")}
      </div>
    );
  }

  return (
    <div className="flex flex-col gap-6 overflow-y-auto p-4 md:p-6" style={{ backgroundColor: "var(--color-bg)" }}>
      <div className="flex flex-col gap-2">
        <h1 className="text-xl font-bold" style={{ color: "var(--color-text)", borderBottom: "2px solid var(--color-accent)", paddingBottom: 8, display: "inline-block", alignSelf: "flex-start" }}>
          {t("runtime.title")}
        </h1>
        <p className="text-sm" style={{ color: "var(--color-text-muted)" }}>
          {t("runtime.subtitle")}
        </p>
      </div>

      <div className="grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-5">
        <SummaryCard label={t("runtime.summary.contextHeads")} value={String(contextEntries.length)} detail={t("runtime.summary.contextHeadsDetail")} />
        <SummaryCard label={t("runtime.summary.activeTasks")} value={String(activeTaskCount)} detail={t("runtime.summary.activeTasksDetail", { count: tasks.length })} />
        <SummaryCard
          label={t("runtime.summary.diagnostics")}
          value={String(diagnostics.length)}
          detail={t("runtime.summary.diagnosticsDetail")}
        />
        <SummaryCard
          label={t("runtime.summary.pendingReports")}
          value={String(mailbox.data?.pending_count ?? 0)}
          detail={t("runtime.summary.pendingReportsDetail", { count: mailboxItems.length })}
        />
        <SummaryCard
          label={t("runtime.summary.approvals")}
          value={String(pendingApprovals.length)}
          detail={t("runtime.summary.approvalsDetail")}
        />
      </div>

      <Panel
        title={t("runtime.panels.diagnosticsTitle")}
        subtitle={t("runtime.panels.diagnosticsSubtitle")}
        emptyText={t("runtime.panels.diagnosticsEmpty")}
      >
        {diagnostics.map((diagnostic) => (
          <DiagnosticRow
            key={diagnostic.id}
            diagnostic={diagnostic}
            selected={selection?.kind === "diagnostic" && selection.id === diagnostic.id}
            onSelect={() => setSelection({ kind: "diagnostic", id: diagnostic.id })}
          />
        ))}
      </Panel>

      <div className="grid grid-cols-1 gap-6 xl:grid-cols-[1.1fr_1fr]">
        <Panel
          title={t("runtime.panels.contextTitle")}
          subtitle={t("runtime.panels.contextSubtitle")}
          emptyText={t("runtime.panels.contextEmpty")}
        >
          {contextEntries.map((entry) => (
            <ContextHeadRow
              key={entry.id}
              entry={entry}
              selected={selection?.kind === "context" && selection.id === entry.id}
              onSelect={() => setSelection({ kind: "context", id: entry.id })}
            />
          ))}
        </Panel>

        <Panel
          title={t("runtime.panels.reportsTitle")}
          subtitle={t("runtime.panels.reportsSubtitle")}
          emptyText={t("runtime.panels.reportsEmpty")}
        >
          {mailboxItems.map((item) => (
            <ReportRow
              key={item.id}
              item={item}
              selected={selection?.kind === "report" && selection.id === item.id}
              onSelect={() => setSelection({ kind: "report", id: item.id })}
            />
          ))}
        </Panel>
      </div>

      <div className="grid grid-cols-1 gap-6 xl:grid-cols-[1.3fr_0.9fr]">
        <Panel title={t("runtime.panels.tasksTitle")} subtitle={t("runtime.panels.tasksSubtitle")} emptyText={t("runtime.panels.tasksEmpty")}>
          {tasks.map((task) => (
            <TaskRow
              key={task.id}
              task={task}
              selected={selection?.kind === "task" && selection.id === task.id}
              onSelect={() => setSelection({ kind: "task", id: task.id })}
            />
          ))}
        </Panel>

        <Panel
          title={t("runtime.panels.approvalsTitle")}
          subtitle={t("runtime.panels.approvalsSubtitle")}
          emptyText={t("runtime.panels.approvalsEmpty")}
        >
          {pendingApprovals.map((request) => (
            <ApprovalRow
              key={request.id}
              request={request}
              selected={selection?.kind === "approval" && selection.id === request.id}
              onSelect={() => setSelection({ kind: "approval", id: request.id })}
            />
          ))}
        </Panel>
      </div>

      <div className="grid grid-cols-1 gap-6 xl:grid-cols-[1fr_1fr]">
        <Panel
          title={t("runtime.panels.toolRunsTitle")}
          subtitle={t("runtime.panels.toolRunsSubtitle")}
          emptyText={t("runtime.panels.toolRunsEmpty")}
        >
          {toolRuns.map((toolRun) => (
            <ToolRunRow
              key={toolRun.id}
              toolRun={toolRun}
              selected={selection?.kind === "tool_run" && selection.id === toolRun.id}
              onSelect={() => setSelection({ kind: "tool_run", id: toolRun.id })}
            />
          ))}
        </Panel>

        <Panel
          title={t("runtime.panels.worktreesTitle")}
          subtitle={t("runtime.panels.worktreesSubtitle")}
          emptyText={t("runtime.panels.worktreesEmpty")}
        >
          {worktrees.map((worktree) => (
            <WorktreeRow
              key={worktree.id}
              worktree={worktree}
              selected={selection?.kind === "worktree" && selection.id === worktree.id}
              onSelect={() => setSelection({ kind: "worktree", id: worktree.id })}
            />
          ))}
        </Panel>
      </div>

      <Panel
        title={t("runtime.panels.detailTitle")}
        subtitle={t("runtime.panels.detailSubtitle")}
        emptyText={t("runtime.panels.detailEmpty")}
      >
        {selectedDetail ? (
          <SelectionDetailCard
            detail={selectedDetail}
            actions={buildSelectionActions({
              selection,
              contextEntries,
              tasks,
              mailboxItems,
              toolRuns,
              worktrees,
              pendingApprovals,
              reopenContext,
              loadContextHistory,
              setSelection,
            })}
          />
        ) : null}
      </Panel>
    </div>
  );
}

type RuntimeSelection =
  | { kind: "context"; id: string }
  | { kind: "task"; id: string }
  | { kind: "report"; id: string }
  | { kind: "diagnostic"; id: string }
  | { kind: "approval"; id: string }
  | { kind: "tool_run"; id: string }
  | { kind: "worktree"; id: string };

interface DetailItem {
  label: string;
  value: string;
}

interface SelectionDetail {
  title: string;
  kindLabel: string;
  summary?: string;
  items: DetailItem[];
}

interface SelectionAction {
  label: string;
  onClick: () => void | Promise<void>;
  disabled?: boolean;
}

function buildSelectionDetail(
  selection: RuntimeSelection | null,
  contextEntries: HistoryEntry[],
  tasks: RuntimeTask[],
  mailboxItems: WorkspaceMailboxItem[],
  pendingApprovals: RuntimePermissionRequest[],
  diagnostics: RuntimeDiagnostic[],
  toolRuns: RuntimeToolRun[],
  worktrees: RuntimeWorktree[],
): SelectionDetail | null {
  if (!selection) {
    return null;
  }
  switch (selection.kind) {
    case "context": {
      const entry = contextEntries.find((item) => item.id === selection.id);
      if (!entry) {
        return null;
      }
      return {
        title: entry.title || entry.id,
        kindLabel: "context head",
        summary: entry.preview,
        items: compactDetailItems([
          { label: "Head ID", value: entry.id },
          { label: "Source", value: entry.source_kind || "history" },
          { label: "Current", value: entry.is_current ? "yes" : "no" },
          { label: "Updated", value: formatTimestamp(entry.updated_at) },
        ]),
      };
    }
    case "task": {
      const task = tasks.find((item) => item.id === selection.id);
      const relatedReport = mailboxItems.find((item) => item.source_id === task?.id);
      if (!task) {
        return null;
      }
      return {
        title: task.title || task.id,
        kindLabel: "task",
        summary: task.description,
        items: compactDetailItems([
          { label: "Task ID", value: task.id },
          { label: "Run ID", value: task.run_id },
          { label: "Kind", value: task.kind || "task" },
          { label: "Owner", value: task.owner || "runtime" },
          { label: "State", value: task.state },
          { label: "Plan ID", value: task.plan_id },
          { label: "Related Report", value: relatedReport?.id },
        ]),
      };
    }
    case "report": {
      const item = mailboxItems.find((entry) => entry.id === selection.id);
      const relatedTask = tasks.find((task) => task.id === item?.source_id);
      if (!item) {
        return null;
      }
      return {
        title: item.envelope.title || item.source_id,
        kindLabel: "report",
        summary: item.envelope.summary,
        items: compactDetailItems([
          { label: "Report ID", value: item.report_id || item.id },
          { label: "Source", value: item.source_kind },
          { label: "Source Role", value: item.source_role_id },
          { label: "Source Task", value: relatedTask?.id || item.source_id },
          { label: "Severity", value: item.envelope.severity || "info" },
          { label: "Delivery State", value: item.delivery_state || "queued" },
          { label: "Updated", value: formatTimestamp(item.updated_at || item.created_at) },
        ]),
      };
    }
    case "diagnostic": {
      const diagnostic = diagnostics.find((item) => item.id === selection.id);
      if (!diagnostic) {
        return null;
      }
      return {
        title: diagnostic.summary || diagnostic.event_type || diagnostic.id,
        kindLabel: "diagnostic",
        summary: diagnostic.reason,
        items: compactDetailItems([
          { label: "Diagnostic ID", value: diagnostic.id },
          { label: "Session ID", value: diagnostic.session_id },
          { label: "Turn ID", value: diagnostic.turn_id },
          { label: "Event Type", value: diagnostic.event_type },
          { label: "Category", value: diagnostic.category },
          { label: "Severity", value: diagnostic.severity },
          { label: "Reason", value: diagnostic.reason },
          { label: "Summary", value: diagnostic.summary },
          { label: "Repair Hint", value: diagnostic.repair_hint },
          { label: "Asset Kind", value: diagnostic.asset_kind },
          { label: "Asset ID", value: diagnostic.asset_id },
          { label: "Created", value: formatTimestamp(diagnostic.created_at) },
        ]),
      };
    }
    case "approval": {
      const request = pendingApprovals.find((item) => item.id === selection.id);
      if (!request) {
        return null;
      }
      return {
        title: request.tool_name || request.requested_profile,
        kindLabel: "approval",
        summary: request.reason || "Awaiting approval to continue execution.",
        items: compactDetailItems([
          { label: "Request ID", value: request.id },
          { label: "Profile", value: request.requested_profile },
          { label: "Turn ID", value: request.turn_id },
          { label: "Task ID", value: request.task_id },
          { label: "Status", value: request.status },
          { label: "Updated", value: formatTimestamp(request.updated_at) },
        ]),
      };
    }
    case "tool_run": {
      const toolRun = toolRuns.find((item) => item.id === selection.id);
      if (!toolRun) {
        return null;
      }
      return {
        title: toolRun.tool_name || toolRun.id,
        kindLabel: "tool run",
        summary: toolRun.error || toolRun.output_json,
        items: compactDetailItems([
          { label: "Tool Run ID", value: toolRun.id },
          { label: "Run ID", value: toolRun.run_id },
          { label: "Task ID", value: toolRun.task_id },
          { label: "State", value: toolRun.state },
          { label: "Tool Call", value: toolRun.tool_call_id },
          { label: "Input", value: toolRun.input_json },
          { label: "Permission Request", value: toolRun.permission_request_id },
          { label: "Lock Wait", value: toolRun.lock_wait_ms != null ? `${toolRun.lock_wait_ms} ms` : undefined },
          { label: "Updated", value: formatTimestamp(toolRun.updated_at || toolRun.created_at) },
        ]),
      };
    }
    case "worktree": {
      const worktree = worktrees.find((item) => item.id === selection.id);
      if (!worktree) {
        return null;
      }
      return {
        title: worktree.worktree_branch || worktree.id,
        kindLabel: "worktree",
        summary: worktree.worktree_path,
        items: compactDetailItems([
          { label: "Worktree ID", value: worktree.id },
          { label: "Run ID", value: worktree.run_id },
          { label: "Task ID", value: worktree.task_id },
          { label: "State", value: worktree.state },
          { label: "Branch", value: worktree.worktree_branch },
          { label: "Path", value: worktree.worktree_path },
          { label: "Updated", value: formatTimestamp(worktree.updated_at || worktree.created_at) },
        ]),
      };
    }
  }
}

function buildSelectionActions({
  selection,
  contextEntries,
  tasks,
  mailboxItems,
  toolRuns,
  worktrees,
  pendingApprovals,
  reopenContext,
  loadContextHistory,
  setSelection,
}: {
  selection: RuntimeSelection | null;
  contextEntries: HistoryEntry[];
  tasks: RuntimeTask[];
  mailboxItems: WorkspaceMailboxItem[];
  toolRuns: RuntimeToolRun[];
  worktrees: RuntimeWorktree[];
  pendingApprovals: RuntimePermissionRequest[];
  reopenContext: ReturnType<typeof useReopenContext>;
  loadContextHistory: ReturnType<typeof useLoadContextHistory>;
  setSelection: (selection: RuntimeSelection | null) => void;
}): SelectionAction[] {
  if (!selection) {
    return [];
  }
  switch (selection.kind) {
    case "context": {
      const entry = contextEntries.find((item) => item.id === selection.id);
      if (!entry) {
        return [];
      }
      if (entry.is_current) {
        return [
          {
            label: reopenContext.isPending ? "Reopening..." : "Reopen current head",
            disabled: reopenContext.isPending,
            onClick: async () => {
              const head = await reopenContext.mutateAsync();
              setSelection({ kind: "context", id: head.id });
            },
          },
        ];
      }
      return [
        {
          label: loadContextHistory.isPending ? "Loading..." : "Load selected head",
          disabled: loadContextHistory.isPending,
          onClick: async () => {
            const head = await loadContextHistory.mutateAsync(entry.id);
            setSelection({ kind: "context", id: head.id });
          },
        },
      ];
    }
    case "task": {
      const task = tasks.find((item) => item.id === selection.id);
      const relatedReport = mailboxItems.find((item) => item.source_id === task?.id);
      const relatedToolRun = toolRuns.find((item) => item.task_id === task?.id);
      const relatedWorktree = worktrees.find((item) => item.task_id === task?.id || item.id === task?.worktree_id);
      const actions: SelectionAction[] = [];
      if (relatedReport) {
        actions.push({
          label: "Open related report",
          onClick: () => setSelection({ kind: "report", id: relatedReport.id }),
        });
      }
      if (relatedToolRun) {
        actions.push({
          label: "Open tool run",
          onClick: () => setSelection({ kind: "tool_run", id: relatedToolRun.id }),
        });
      }
      if (relatedWorktree) {
        actions.push({
          label: "Open worktree",
          onClick: () => setSelection({ kind: "worktree", id: relatedWorktree.id }),
        });
      }
      return actions;
    }
    case "report": {
      const report = mailboxItems.find((item) => item.id === selection.id);
      const relatedTask = tasks.find((task) => task.id === report?.source_id);
      if (!relatedTask) {
        return [];
      }
      return [
        {
          label: "Open source task",
          onClick: () => setSelection({ kind: "task", id: relatedTask.id }),
        },
      ];
    }
    case "tool_run": {
      const toolRun = toolRuns.find((item) => item.id === selection.id);
      const relatedTask = tasks.find((item) => item.id === toolRun?.task_id);
      const relatedApproval = pendingApprovals.find((item) => item.id === toolRun?.permission_request_id);
      const actions: SelectionAction[] = [];
      if (relatedTask) {
        actions.push({
          label: "Open related task",
          onClick: () => setSelection({ kind: "task", id: relatedTask.id }),
        });
      }
      if (relatedApproval) {
        actions.push({
          label: "Open approval request",
          onClick: () => setSelection({ kind: "approval", id: relatedApproval.id }),
        });
      }
      return actions;
    }
    case "worktree": {
      const worktree = worktrees.find((item) => item.id === selection.id);
      const relatedTask = tasks.find((item) => item.id === worktree?.task_id);
      if (!relatedTask) {
        return [];
      }
      return [
        {
          label: "Open attached task",
          onClick: () => setSelection({ kind: "task", id: relatedTask.id }),
        },
      ];
    }
    default:
      return [];
  }
}

function compactDetailItems(items: Array<{ label: string; value?: string }>): DetailItem[] {
  return items
    .filter((item) => item.value != null && item.value !== "")
    .map((item) => ({ label: item.label, value: item.value! }));
}
