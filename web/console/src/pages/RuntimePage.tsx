import { useMemo, useState } from "react";
import {
  useContextHistory,
  useFileCheckpointDiff,
  useFileCheckpoints,
  useLoadContextHistory,
  useRollbackFileCheckpoint,
  useWorkspaceReports,
  useWorkspaceRuntimeGraph,
  useReopenContext,
} from "../api/queries";
import type {
  FileCheckpoint,
  HistoryEntry,
  RuntimeDiagnostic,
  RuntimeTask,
  RuntimeToolRun,
  RuntimeWorktree,
  WorkspaceReportDeliveryItem,
} from "../api/types";
import { useI18n } from "../i18n";
import {
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
import { RefreshCw, GitBranch, Database } from "../components/Icon";

interface RuntimePageProps {
  sessionId: string;
}

export function RuntimePage({ sessionId }: RuntimePageProps) {
  const { t } = useI18n();
  const history = useContextHistory(sessionId || null);
  const runtimeGraph = useWorkspaceRuntimeGraph();
  const reports = useWorkspaceReports();
  const checkpoints = useFileCheckpoints(sessionId || null);
  const reopenContext = useReopenContext(sessionId);
  const loadContextHistory = useLoadContextHistory(sessionId);

  const contextEntries = [...(history.data?.entries ?? [])].sort(sortByUpdatedAtDesc);
  const tasks = [...(runtimeGraph.data?.graph.tasks ?? [])].sort(sortByUpdatedAtDesc);
  const toolRuns = [...(runtimeGraph.data?.graph.tool_runs ?? [])].sort(sortByUpdatedAtDesc);
  const worktrees = [...(runtimeGraph.data?.graph.worktrees ?? [])].sort(sortByUpdatedAtDesc);
  const diagnostics = [...(runtimeGraph.data?.graph.diagnostics ?? [])].sort(sortByCreatedAtDesc);
  const reportItems = [...(reports.data?.items ?? [])].sort(sortByUpdatedAtDesc);
  const checkpointItems = [...(checkpoints.data?.checkpoints ?? [])].sort(sortByCreatedAtDesc);
  const activeTaskCount = tasks.filter((task) => task.state === "running" || task.state === "pending").length;
  const [selection, setSelection] = useState<RuntimeSelection | null>(null);
  const selectedDetail = useMemo(
    () =>
      buildSelectionDetail(
        selection,
        contextEntries,
        tasks,
        reportItems,
        diagnostics,
        toolRuns,
        worktrees,
      ),
    [selection, contextEntries, tasks, reportItems, diagnostics, toolRuns, worktrees],
  );

  const isInitialLoading =
    (history.isLoading && !history.data) ||
    (runtimeGraph.isLoading && !runtimeGraph.data) ||
    (reports.isLoading && !reports.data);

  if (isInitialLoading) {
    return (
      <div className="flex h-full items-center justify-center text-sm" style={{ color: "var(--color-text-tertiary)" }}>
        {t("runtime.loading")}
      </div>
    );
  }

  return (
    <div className="flex flex-col gap-4 overflow-y-auto p-4 md:p-5" style={{ backgroundColor: "var(--color-bg)" }}>
      {/* Page header */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-lg font-bold m-0" style={{ color: "var(--color-text)" }}>
            {t("runtime.title")}
          </h1>
          <p className="text-xs mt-0.5 m-0" style={{ color: "var(--color-text-tertiary)" }}>
            {t("runtime.subtitle")}
          </p>
        </div>
        <button
          type="button"
          onClick={() => {
            void runtimeGraph.refetch();
            void reports.refetch();
            void history.refetch();
          }}
          className="flex items-center gap-1.5 rounded-md px-2.5 py-1.5 text-xs font-medium"
          style={{
            backgroundColor: "var(--color-surface)",
            border: "1px solid var(--color-border)",
            color: "var(--color-text-secondary)",
            cursor: "pointer",
          }}
        >
          <RefreshCw size={13} />
          Refresh
        </button>
      </div>

      {/* KPI Summary Cards */}
      <div className="grid grid-cols-2 gap-3 md:grid-cols-4">
        <SummaryCard label={t("runtime.summary.contextHeads")} value={String(contextEntries.length)} detail={t("runtime.summary.contextHeadsDetail")} accent="accent" />
        <SummaryCard label={t("runtime.summary.activeTasks")} value={String(activeTaskCount)} detail={t("runtime.summary.activeTasksDetail", { count: tasks.length })} accent={activeTaskCount > 0 ? "warning" : "success"} />
        <SummaryCard label={t("runtime.summary.diagnostics")} value={String(diagnostics.length)} detail={t("runtime.summary.diagnosticsDetail")} accent={diagnostics.length > 0 ? "warning" : "success"} />
        <SummaryCard label={t("runtime.summary.queuedReports")} value={String(reports.data?.queued_count ?? 0)} detail={t("runtime.summary.queuedReportsDetail", { count: reportItems.length })} accent="accent" />
      </div>

      {/* Diagnostics */}
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

      {/* Checkpoints */}
      <CheckpointsPanel
        sessionId={sessionId}
        checkpoints={checkpointItems}
        isLoading={checkpoints.isLoading && !checkpoints.data}
      />

      {/* Context Heads + Reports */}
      <div className="grid grid-cols-1 gap-4 xl:grid-cols-2">
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
          {reportItems.map((item) => (
            <ReportRow
              key={item.id}
              item={item}
              selected={selection?.kind === "report" && selection.id === item.id}
              onSelect={() => setSelection({ kind: "report", id: item.id })}
            />
          ))}
        </Panel>
      </div>

      {/* Tasks */}
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

      {/* Tool Runs + Worktrees */}
      <div className="grid grid-cols-1 gap-4 xl:grid-cols-2">
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

      {/* Detail panel */}
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
              reportItems,
              toolRuns,
              worktrees,
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

function CheckpointsPanel({
  sessionId,
  checkpoints,
  isLoading,
}: {
  sessionId: string;
  checkpoints: FileCheckpoint[];
  isLoading: boolean;
}) {
  const { t } = useI18n();
  const [expandedID, setExpandedID] = useState<string | null>(null);
  const diff = useFileCheckpointDiff(sessionId || null, expandedID);
  const rollback = useRollbackFileCheckpoint(sessionId);

  async function handleRollback(checkpoint: FileCheckpoint) {
    if (rollback.isPending) return;
    if (!window.confirm(t("runtime.checkpoints.confirmRollback", { tool: checkpoint.tool_name || checkpoint.id }))) return;
    await rollback.mutateAsync(checkpoint.id);
    setExpandedID(null);
  }

  return (
    <Panel
      title={t("runtime.checkpoints.title")}
      subtitle={t("runtime.checkpoints.subtitle")}
      emptyText={isLoading ? t("runtime.checkpoints.loading") : t("runtime.checkpoints.empty")}
    >
      {isLoading
        ? null
        : checkpoints.map((checkpoint) => {
            const expanded = expandedID === checkpoint.id;
            const files = checkpoint.files_changed ?? [];
            return (
              <div
                key={checkpoint.id}
                className="rounded-md overflow-hidden"
                style={{ backgroundColor: "var(--color-bg-elevated)", border: "1px solid var(--color-border)" }}
              >
                <div className="flex items-center justify-between px-3 py-2">
                  <button
                    type="button"
                    onClick={() => setExpandedID(expanded ? null : checkpoint.id)}
                    className="flex items-center gap-2 text-left flex-1"
                    style={{ background: "transparent", border: "none", color: "var(--color-text)", cursor: "pointer", padding: 0 }}
                    aria-expanded={expanded}
                  >
                    <Database size={13} color="var(--color-text-tertiary)" />
                    <span className="text-sm font-medium">{checkpoint.tool_name || t("runtime.checkpoints.unknownTool")}</span>
                    <span className="text-[11px]" style={{ color: "var(--color-text-tertiary)" }}>
                      {formatTimestamp(checkpoint.created_at)}
                    </span>
                    {files.length > 0 && (
                      <span className="text-[11px] px-1.5 py-0.5 rounded" style={{ backgroundColor: "var(--color-surface)", color: "var(--color-text-tertiary)" }}>
                        {files.length} files
                      </span>
                    )}
                  </button>
                  <button
                    type="button"
                    onClick={() => void handleRollback(checkpoint)}
                    disabled={rollback.isPending}
                    className="rounded px-2 py-1 text-[11px] font-medium"
                    style={{
                      backgroundColor: "var(--color-surface)",
                      border: "1px solid var(--color-border)",
                      color: "var(--color-text-secondary)",
                      opacity: rollback.isPending ? 0.6 : 1,
                      cursor: rollback.isPending ? "not-allowed" : "pointer",
                    }}
                  >
                    {t("runtime.checkpoints.rollback")}
                  </button>
                </div>
                {files.length > 0 && (
                  <div className="px-3 pb-2 flex flex-wrap gap-1.5">
                    {files.slice(0, 10).map((file) => (
                      <span
                        key={file}
                        className="rounded px-1.5 py-0.5 text-[10px] font-mono"
                        style={{ backgroundColor: "var(--color-surface)", color: "var(--color-text-tertiary)" }}
                      >
                        {file}
                      </span>
                    ))}
                    {files.length > 10 && (
                      <span className="px-1.5 py-0.5 text-[10px]" style={{ color: "var(--color-text-tertiary)" }}>
                        {t("runtime.checkpoints.moreFiles", { count: files.length - 10 })}
                      </span>
                    )}
                  </div>
                )}
                {expanded && (
                  <pre
                    className="mx-2 mb-2 max-h-80 overflow-auto rounded p-3 text-xs leading-relaxed"
                    style={{
                      backgroundColor: "var(--color-surface)",
                      border: "1px solid var(--color-border)",
                      color: "var(--color-text-secondary)",
                      fontFamily: "var(--font-mono)",
                      whiteSpace: "pre-wrap",
                    }}
                  >
                    {diff.isLoading
                      ? t("runtime.checkpoints.loadingDiff")
                      : diff.data?.diff || checkpoint.diff_summary || t("runtime.checkpoints.noDiff")}
                  </pre>
                )}
              </div>
            );
          })}
    </Panel>
  );
}

function buildSelectionDetail(
  selection: RuntimeSelection | null,
  contextEntries: HistoryEntry[],
  tasks: RuntimeTask[],
  reportItems: WorkspaceReportDeliveryItem[],
  diagnostics: RuntimeDiagnostic[],
  toolRuns: RuntimeToolRun[],
  worktrees: RuntimeWorktree[],
): SelectionDetail | null {
  if (!selection) return null;
  switch (selection.kind) {
    case "context": {
      const entry = contextEntries.find((item) => item.id === selection.id);
      if (!entry) return null;
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
      const relatedReport = reportItems.find((item) => item.source_id === task?.id);
      if (!task) return null;
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
      const item = reportItems.find((entry) => entry.id === selection.id);
      const relatedTask = tasks.find((task) => task.id === item?.source_id);
      if (!item) return null;
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
      if (!diagnostic) return null;
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
    case "tool_run": {
      const toolRun = toolRuns.find((item) => item.id === selection.id);
      if (!toolRun) return null;
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
          { label: "Lock Wait", value: toolRun.lock_wait_ms != null ? `${toolRun.lock_wait_ms} ms` : undefined },
          { label: "Updated", value: formatTimestamp(toolRun.updated_at || toolRun.created_at) },
        ]),
      };
    }
    case "worktree": {
      const worktree = worktrees.find((item) => item.id === selection.id);
      if (!worktree) return null;
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
  reportItems,
  toolRuns,
  worktrees,
  reopenContext,
  loadContextHistory,
  setSelection,
}: {
  selection: RuntimeSelection | null;
  contextEntries: HistoryEntry[];
  tasks: RuntimeTask[];
  reportItems: WorkspaceReportDeliveryItem[];
  toolRuns: RuntimeToolRun[];
  worktrees: RuntimeWorktree[];
  reopenContext: ReturnType<typeof useReopenContext>;
  loadContextHistory: ReturnType<typeof useLoadContextHistory>;
  setSelection: (selection: RuntimeSelection | null) => void;
}): SelectionAction[] {
  if (!selection) return [];
  switch (selection.kind) {
    case "context": {
      const entry = contextEntries.find((item) => item.id === selection.id);
      if (!entry) return [];
      if (entry.is_current) {
        return [{
          label: reopenContext.isPending ? "Reopening..." : "Reopen current head",
          disabled: reopenContext.isPending,
          onClick: async () => {
            const head = await reopenContext.mutateAsync();
            setSelection({ kind: "context", id: head.id });
          },
        }];
      }
      return [{
        label: loadContextHistory.isPending ? "Loading..." : "Load selected head",
        disabled: loadContextHistory.isPending,
        onClick: async () => {
          const head = await loadContextHistory.mutateAsync(entry.id);
          setSelection({ kind: "context", id: head.id });
        },
      }];
    }
    case "task": {
      const task = tasks.find((item) => item.id === selection.id);
      const relatedReport = reportItems.find((item) => item.source_id === task?.id);
      const relatedToolRun = toolRuns.find((item) => item.task_id === task?.id);
      const relatedWorktree = worktrees.find((item) => item.task_id === task?.id || item.id === task?.worktree_id);
      const actions: SelectionAction[] = [];
      if (relatedReport) {
        actions.push({ label: "Open related report", onClick: () => setSelection({ kind: "report", id: relatedReport.id }) });
      }
      if (relatedToolRun) {
        actions.push({ label: "Open tool run", onClick: () => setSelection({ kind: "tool_run", id: relatedToolRun.id }) });
      }
      if (relatedWorktree) {
        actions.push({ label: "Open worktree", onClick: () => setSelection({ kind: "worktree", id: relatedWorktree.id }) });
      }
      return actions;
    }
    case "report": {
      const report = reportItems.find((item) => item.id === selection.id);
      const relatedTask = tasks.find((task) => task.id === report?.source_id);
      if (!relatedTask) return [];
      return [{ label: "Open source task", onClick: () => setSelection({ kind: "task", id: relatedTask.id }) }];
    }
    case "tool_run": {
      const toolRun = toolRuns.find((item) => item.id === selection.id);
      const relatedTask = tasks.find((item) => item.id === toolRun?.task_id);
      const actions: SelectionAction[] = [];
      if (relatedTask) {
        actions.push({ label: "Open related task", onClick: () => setSelection({ kind: "task", id: relatedTask.id }) });
      }
      return actions;
    }
    case "worktree": {
      const worktree = worktrees.find((item) => item.id === selection.id);
      const relatedTask = tasks.find((item) => item.id === worktree?.task_id);
      if (!relatedTask) return [];
      return [{ label: "Open attached task", onClick: () => setSelection({ kind: "task", id: relatedTask.id }) }];
    }
    default:
      return [];
  }
}

function compactDetailItems(items: Array<{ label: string; value?: string }>): DetailItem[] {
  return items.filter((item) => item.value != null && item.value !== "").map((item) => ({ label: item.label, value: item.value! }));
}
