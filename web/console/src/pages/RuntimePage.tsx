import { useMemo, useState } from "react";
import type { ReactNode } from "react";
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
  RuntimeDispatchAttempt,
  RuntimeIncident,
  RuntimePermissionRequest,
  RuntimeTask,
  RuntimeToolRun,
  RuntimeWorktree,
  WorkspaceMailboxItem,
} from "../api/types";
import { useI18n } from "../i18n";

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
  const incidents = [...(runtimeGraph.data?.graph.incidents ?? [])].sort(sortByUpdatedAtDesc);
  const dispatchAttempts = [...(runtimeGraph.data?.graph.dispatch_attempts ?? [])].sort(sortByUpdatedAtDesc);
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
        incidents,
        dispatchAttempts,
      ),
    [selection, contextEntries, tasks, mailboxItems, pendingApprovals, diagnostics, toolRuns, worktrees, incidents, dispatchAttempts],
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
        <h1 className="text-lg font-semibold" style={{ color: "var(--color-text)" }}>
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
          title={t("runtime.panels.incidentsTitle")}
          subtitle={t("runtime.panels.incidentsSubtitle")}
          emptyText={t("runtime.panels.incidentsEmpty")}
        >
          {incidents.map((incident) => (
            <IncidentRow
              key={incident.id}
              incident={incident}
              selected={selection?.kind === "incident" && selection.id === incident.id}
              onSelect={() => setSelection({ kind: "incident", id: incident.id })}
            />
          ))}
        </Panel>

        <Panel
          title={t("runtime.panels.dispatchTitle")}
          subtitle={t("runtime.panels.dispatchSubtitle")}
          emptyText={t("runtime.panels.dispatchEmpty")}
        >
          {dispatchAttempts.map((attempt) => (
            <DispatchAttemptRow
              key={attempt.dispatch_id}
              attempt={attempt}
              selected={selection?.kind === "dispatch" && selection.id === attempt.dispatch_id}
              onSelect={() => setSelection({ kind: "dispatch", id: attempt.dispatch_id })}
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
              incidents,
              dispatchAttempts,
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
  | { kind: "incident"; id: string }
  | { kind: "dispatch"; id: string }
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

function SummaryCard({ label, value, detail }: { label: string; value: string; detail: string }) {
  return (
    <div
      className="rounded-xl px-5 py-4"
      style={{ backgroundColor: "var(--color-surface)", border: "1px solid var(--color-border)" }}
    >
      <div className="text-xs font-medium uppercase tracking-wide" style={{ color: "var(--color-text-muted)" }}>
        {label}
      </div>
      <div className="mt-3 text-3xl font-semibold" style={{ color: "var(--color-text)" }}>
        {value}
      </div>
      <div className="mt-2 text-sm" style={{ color: "var(--color-text-muted)" }}>
        {detail}
      </div>
    </div>
  );
}

function Panel({
  title,
  subtitle,
  emptyText,
  children,
}: {
  title: string;
  subtitle: string;
  emptyText: string;
  children: ReactNode;
}) {
  const items = Array.isArray(children) ? children.filter(Boolean) : children ? [children] : [];

  return (
    <section
      className="rounded-2xl p-5"
      style={{ backgroundColor: "var(--color-surface)", border: "1px solid var(--color-border)" }}
    >
      <div className="mb-4">
        <h2 className="text-sm font-semibold uppercase tracking-wide" style={{ color: "var(--color-text)" }}>
          {title}
        </h2>
        <p className="mt-1 text-sm" style={{ color: "var(--color-text-muted)" }}>
          {subtitle}
        </p>
      </div>
      <div className="flex flex-col gap-3">
        {items.length > 0 ? (
          items
        ) : (
          <div
            className="rounded-xl border border-dashed px-4 py-5 text-sm"
            style={{ color: "var(--color-text-muted)", borderColor: "var(--color-border)" }}
          >
            {emptyText}
          </div>
        )}
      </div>
    </section>
  );
}

function ContextHeadRow({
  entry,
  selected,
  onSelect,
}: {
  entry: HistoryEntry;
  selected: boolean;
  onSelect: () => void;
}) {
  return (
    <button
      type="button"
      onClick={onSelect}
      aria-pressed={selected}
      className="rounded-xl px-4 py-3"
      style={{
        backgroundColor: selected ? "rgba(62, 130, 247, 0.08)" : "var(--color-surface-2)",
        border: `1px solid ${selected ? "var(--color-accent)" : "var(--color-border)"}`,
        textAlign: "left",
      }}
    >
      <div className="flex items-start justify-between gap-3">
        <div>
          <div className="text-sm font-medium" style={{ color: "var(--color-text)" }}>
            {entry.title || entry.id}
          </div>
          {entry.preview && (
            <div className="mt-1 text-sm leading-5" style={{ color: "var(--color-text-muted)" }}>
              {entry.preview}
            </div>
          )}
        </div>
        <StatusBadge tone={entry.is_current ? "accent" : "neutral"}>
          {entry.is_current ? "Current" : entry.source_kind || "history"}
        </StatusBadge>
      </div>
      <div className="mt-3 text-xs" style={{ color: "var(--color-text-muted)" }}>
        Updated {formatTimestamp(entry.updated_at)}
      </div>
    </button>
  );
}

function TaskRow({
  task,
  selected,
  onSelect,
}: {
  task: RuntimeTask;
  selected: boolean;
  onSelect: () => void;
}) {
  return (
    <button
      type="button"
      onClick={onSelect}
      aria-pressed={selected}
      className="rounded-xl px-4 py-3"
      style={{
        backgroundColor: selected ? "rgba(62, 130, 247, 0.08)" : "var(--color-surface-2)",
        border: `1px solid ${selected ? "var(--color-accent)" : "var(--color-border)"}`,
        textAlign: "left",
      }}
    >
      <div className="flex items-start justify-between gap-3">
        <div>
          <div className="text-sm font-medium" style={{ color: "var(--color-text)" }}>
            {task.title || task.id}
          </div>
          {task.description && (
            <div className="mt-1 text-sm leading-5" style={{ color: "var(--color-text-muted)" }}>
              {task.description}
            </div>
          )}
        </div>
        <StatusBadge tone={toneFromState(task.state)}>{task.state}</StatusBadge>
      </div>
      <div className="mt-3 flex flex-wrap gap-x-4 gap-y-1 text-xs" style={{ color: "var(--color-text-muted)" }}>
        <span>Owner: {task.owner || "runtime"}</span>
        <span>Kind: {task.kind || "task"}</span>
        <span>Updated {formatTimestamp(task.updated_at)}</span>
      </div>
    </button>
  );
}

function IncidentRow({
  incident,
  selected,
  onSelect,
}: {
  incident: RuntimeIncident;
  selected: boolean;
  onSelect: () => void;
}) {
  return (
    <button
      type="button"
      onClick={onSelect}
      aria-pressed={selected}
      className="rounded-xl px-4 py-3"
      style={{
        backgroundColor: selected ? "rgba(62, 130, 247, 0.08)" : "var(--color-surface-2)",
        border: `1px solid ${selected ? "var(--color-accent)" : "var(--color-border)"}`,
        textAlign: "left",
      }}
    >
      <div className="flex items-start justify-between gap-3">
        <div>
          <div className="text-sm font-medium" style={{ color: "var(--color-text)" }}>
            {incident.summary || incident.id}
          </div>
          {incident.signal_kind && (
            <div className="mt-1 text-sm leading-5" style={{ color: "var(--color-text-muted)" }}>
              Signal: {incident.signal_kind}
            </div>
          )}
        </div>
        <StatusBadge tone={toneFromState(incident.status)}>{incident.status}</StatusBadge>
      </div>
      <div className="mt-3 flex flex-wrap gap-x-4 gap-y-1 text-xs" style={{ color: "var(--color-text-muted)" }}>
        <span>Automation: {incident.automation_id}</span>
        <span>Source: {incident.source || "runtime"}</span>
        <span>Updated {formatTimestamp(incident.updated_at || incident.created_at)}</span>
      </div>
    </button>
  );
}

function DispatchAttemptRow({
  attempt,
  selected,
  onSelect,
}: {
  attempt: RuntimeDispatchAttempt;
  selected: boolean;
  onSelect: () => void;
}) {
  return (
    <button
      type="button"
      onClick={onSelect}
      aria-pressed={selected}
      className="rounded-xl px-4 py-3"
      style={{
        backgroundColor: selected ? "rgba(62, 130, 247, 0.08)" : "var(--color-surface-2)",
        border: `1px solid ${selected ? "var(--color-accent)" : "var(--color-border)"}`,
        textAlign: "left",
      }}
    >
      <div className="flex items-start justify-between gap-3">
        <div>
          <div className="text-sm font-medium" style={{ color: "var(--color-text)" }}>
            {attempt.outcome_summary || attempt.dispatch_id}
          </div>
          <div className="mt-1 text-sm leading-5" style={{ color: "var(--color-text-muted)" }}>
            Dispatch {attempt.dispatch_id}
          </div>
        </div>
        <StatusBadge tone={toneFromState(attempt.status)}>{attempt.status}</StatusBadge>
      </div>
      <div className="mt-3 flex flex-wrap gap-x-4 gap-y-1 text-xs" style={{ color: "var(--color-text-muted)" }}>
        <span>Phase: {attempt.phase}</span>
        <span>Task: {attempt.task_id || "none"}</span>
        <span>Updated {formatTimestamp(attempt.updated_at || attempt.created_at)}</span>
      </div>
    </button>
  );
}

function ToolRunRow({
  toolRun,
  selected,
  onSelect,
}: {
  toolRun: RuntimeToolRun;
  selected: boolean;
  onSelect: () => void;
}) {
  return (
    <button
      type="button"
      onClick={onSelect}
      aria-pressed={selected}
      className="rounded-xl px-4 py-3"
      style={{
        backgroundColor: selected ? "rgba(62, 130, 247, 0.08)" : "var(--color-surface-2)",
        border: `1px solid ${selected ? "var(--color-accent)" : "var(--color-border)"}`,
        textAlign: "left",
      }}
    >
      <div className="flex items-start justify-between gap-3">
        <div>
          <div className="text-sm font-medium" style={{ color: "var(--color-text)" }}>
            {toolRun.tool_name}
          </div>
          {toolRun.input_json && (
            <div className="mt-1 text-sm leading-5" style={{ color: "var(--color-text-muted)" }}>
              {toolRun.input_json}
            </div>
          )}
        </div>
        <StatusBadge tone={toneFromState(toolRun.state)}>{toolRun.state}</StatusBadge>
      </div>
      <div className="mt-3 flex flex-wrap gap-x-4 gap-y-1 text-xs" style={{ color: "var(--color-text-muted)" }}>
        <span>Task: {toolRun.task_id || "none"}</span>
        <span>Call: {toolRun.tool_call_id || "n/a"}</span>
        <span>Updated {formatTimestamp(toolRun.updated_at || toolRun.created_at)}</span>
      </div>
    </button>
  );
}

function WorktreeRow({
  worktree,
  selected,
  onSelect,
}: {
  worktree: RuntimeWorktree;
  selected: boolean;
  onSelect: () => void;
}) {
  return (
    <button
      type="button"
      onClick={onSelect}
      aria-pressed={selected}
      className="rounded-xl px-4 py-3"
      style={{
        backgroundColor: selected ? "rgba(62, 130, 247, 0.08)" : "var(--color-surface-2)",
        border: `1px solid ${selected ? "var(--color-accent)" : "var(--color-border)"}`,
        textAlign: "left",
      }}
    >
      <div className="flex items-start justify-between gap-3">
        <div>
          <div className="text-sm font-medium" style={{ color: "var(--color-text)" }}>
            {worktree.worktree_branch || worktree.id}
          </div>
          <div className="mt-1 text-sm leading-5" style={{ color: "var(--color-text-muted)" }}>
            {worktree.worktree_path}
          </div>
        </div>
        <StatusBadge tone={toneFromState(worktree.state)}>{worktree.state}</StatusBadge>
      </div>
      <div className="mt-3 flex flex-wrap gap-x-4 gap-y-1 text-xs" style={{ color: "var(--color-text-muted)" }}>
        <span>Task: {worktree.task_id || "none"}</span>
        <span>Updated {formatTimestamp(worktree.updated_at || worktree.created_at)}</span>
      </div>
    </button>
  );
}

function ReportRow({
  item,
  selected,
  onSelect,
}: {
  item: WorkspaceMailboxItem;
  selected: boolean;
  onSelect: () => void;
}) {
  return (
    <button
      type="button"
      onClick={onSelect}
      aria-pressed={selected}
      className="rounded-xl px-4 py-3"
      style={{
        backgroundColor: selected ? "rgba(62, 130, 247, 0.08)" : "var(--color-surface-2)",
        border: `1px solid ${selected ? "var(--color-accent)" : "var(--color-border)"}`,
        textAlign: "left",
      }}
    >
      <div className="flex items-start justify-between gap-3">
        <div>
          <div className="text-sm font-medium" style={{ color: "var(--color-text)" }}>
            {item.envelope.title || item.source_id}
          </div>
          {item.envelope.summary && (
            <div className="mt-1 text-sm leading-5" style={{ color: "var(--color-text-muted)" }}>
              {item.envelope.summary}
            </div>
          )}
        </div>
        <StatusBadge tone={toneFromSeverity(item.envelope.severity)}>{item.delivery_state || "queued"}</StatusBadge>
      </div>
      <div className="mt-3 flex flex-wrap gap-x-4 gap-y-1 text-xs" style={{ color: "var(--color-text-muted)" }}>
        <span>Source: {item.source_role_id || item.source_kind}</span>
        <span>Severity: {item.envelope.severity || "info"}</span>
        <span>Updated {formatTimestamp(item.updated_at || item.created_at)}</span>
      </div>
    </button>
  );
}

function DiagnosticRow({
  diagnostic,
  selected,
  onSelect,
}: {
  diagnostic: RuntimeDiagnostic;
  selected: boolean;
  onSelect: () => void;
}) {
  return (
    <button
      type="button"
      onClick={onSelect}
      aria-pressed={selected}
      className="rounded-xl px-4 py-3"
      style={{
        backgroundColor: selected ? "rgba(62, 130, 247, 0.08)" : "var(--color-surface-2)",
        border: `1px solid ${selected ? "var(--color-accent)" : "var(--color-border)"}`,
        textAlign: "left",
      }}
    >
      <div className="flex items-start justify-between gap-3">
        <div>
          <div className="text-sm font-medium" style={{ color: "var(--color-text)" }}>
            {diagnostic.summary || diagnostic.event_type || diagnostic.id}
          </div>
          {diagnostic.reason && (
            <div className="mt-1 text-sm leading-5" style={{ color: "var(--color-text-muted)" }}>
              {diagnostic.reason}
            </div>
          )}
        </div>
        <StatusBadge tone={toneFromDiagnosticEvent(diagnostic.event_type)}>{diagnostic.event_type}</StatusBadge>
      </div>
      <div className="mt-3 flex flex-wrap gap-x-4 gap-y-1 text-xs" style={{ color: "var(--color-text-muted)" }}>
        <span>Session: {diagnostic.session_id}</span>
        <span>Turn: {diagnostic.turn_id}</span>
        <span>Created {formatTimestamp(diagnostic.created_at)}</span>
      </div>
    </button>
  );
}

function ApprovalRow({
  request,
  selected,
  onSelect,
}: {
  request: RuntimePermissionRequest;
  selected: boolean;
  onSelect: () => void;
}) {
  return (
    <button
      type="button"
      onClick={onSelect}
      aria-pressed={selected}
      className="rounded-xl px-4 py-3"
      style={{
        backgroundColor: selected ? "rgba(62, 130, 247, 0.08)" : "var(--color-surface-2)",
        border: `1px solid ${selected ? "var(--color-accent)" : "var(--color-border)"}`,
        textAlign: "left",
      }}
    >
      <div className="flex items-start justify-between gap-3">
        <div>
          <div className="text-sm font-medium" style={{ color: "var(--color-text)" }}>
            {request.tool_name || request.requested_profile}
          </div>
          <div className="mt-1 text-sm leading-5" style={{ color: "var(--color-text-muted)" }}>
            {request.reason || "Awaiting approval to continue execution."}
          </div>
        </div>
        <StatusBadge tone="warning">{request.status}</StatusBadge>
      </div>
      <div className="mt-3 flex flex-wrap gap-x-4 gap-y-1 text-xs" style={{ color: "var(--color-text-muted)" }}>
        <span>Profile: {request.requested_profile}</span>
        <span>Updated {formatTimestamp(request.updated_at)}</span>
      </div>
    </button>
  );
}

function SelectionDetailCard({
  detail,
  actions,
}: {
  detail: SelectionDetail;
  actions: SelectionAction[];
}) {
  return (
    <div
      className="rounded-xl px-4 py-4"
      style={{ backgroundColor: "var(--color-surface-2)", border: "1px solid var(--color-border)" }}
    >
      <div className="flex items-start justify-between gap-3">
        <div>
          <div className="text-sm font-medium" style={{ color: "var(--color-text)" }}>
            {detail.title}
          </div>
          {detail.summary && (
            <div className="mt-1 text-sm leading-6" style={{ color: "var(--color-text-muted)" }}>
              {detail.summary}
            </div>
          )}
        </div>
        <StatusBadge tone="accent">{detail.kindLabel}</StatusBadge>
      </div>
      {actions.length > 0 ? (
        <div className="mt-4 flex flex-wrap gap-2">
          {actions.map((action) => (
            <button
              key={action.label}
              type="button"
              onClick={() => void action.onClick()}
              disabled={action.disabled}
              className="rounded-full px-3 py-1.5 text-xs font-medium"
              style={{
                backgroundColor: "var(--color-surface)",
                border: "1px solid var(--color-border)",
                color: "var(--color-text)",
                opacity: action.disabled ? 0.6 : 1,
              }}
            >
              {action.label}
            </button>
          ))}
        </div>
      ) : null}
      <div className="mt-4 grid grid-cols-1 gap-3 md:grid-cols-2">
        {detail.items.map((item) => (
          <div
            key={`${detail.kindLabel}:${item.label}`}
            className="rounded-lg px-3 py-3"
            style={{ backgroundColor: "var(--color-surface)", border: "1px solid var(--color-border)" }}
          >
            <div className="text-[11px] uppercase tracking-wide" style={{ color: "var(--color-text-muted)" }}>
              {item.label}
            </div>
            <div className="mt-1 text-sm break-words" style={{ color: "var(--color-text)" }}>
              {item.value}
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}

function StatusBadge({
  children,
  tone,
}: {
  children: ReactNode;
  tone: "accent" | "success" | "warning" | "danger" | "neutral";
}) {
  const palette = {
    accent: { background: "rgba(62, 130, 247, 0.14)", color: "var(--color-accent)" },
    success: { background: "rgba(55, 170, 102, 0.14)", color: "#2f8f57" },
    warning: { background: "rgba(214, 138, 36, 0.16)", color: "#b36b00" },
    danger: { background: "rgba(210, 73, 73, 0.14)", color: "#b64040" },
    neutral: { background: "rgba(120, 132, 152, 0.16)", color: "var(--color-text-muted)" },
  } as const;

  return (
    <span
      className="rounded-full px-2.5 py-1 text-xs font-medium capitalize"
      style={{ backgroundColor: palette[tone].background, color: palette[tone].color }}
    >
      {children}
    </span>
  );
}

function toneFromState(state: string): "accent" | "success" | "warning" | "danger" | "neutral" {
  switch (state) {
    case "running":
      return "accent";
    case "completed":
      return "success";
    case "pending":
      return "warning";
    case "failed":
    case "cancelled":
      return "danger";
    default:
      return "neutral";
  }
}

function toneFromSeverity(
  severity?: string,
): "accent" | "success" | "warning" | "danger" | "neutral" {
  switch (severity) {
    case "critical":
    case "error":
      return "danger";
    case "warning":
      return "warning";
    case "ok":
    case "success":
      return "success";
    default:
      return "neutral";
  }
}

function toneFromDiagnosticEvent(eventType: string): "accent" | "success" | "warning" | "danger" | "neutral" {
  switch (eventType) {
    case "error":
    case "failure":
    case "blocked":
      return "danger";
    case "warning":
    case "degraded":
      return "warning";
    case "ok":
    case "recovered":
      return "success";
    default:
      return "neutral";
  }
}

function sortByUpdatedAtDesc<T extends { updated_at?: string; created_at?: string }>(a: T, b: T) {
  return compareTimestamps(b.updated_at || b.created_at, a.updated_at || a.created_at);
}

function sortByCreatedAtDesc<T extends { created_at?: string }>(a: T, b: T) {
  return compareTimestamps(b.created_at, a.created_at);
}

function compareTimestamps(a?: string, b?: string) {
  return parseTimestamp(a) - parseTimestamp(b);
}

function parseTimestamp(value?: string) {
  if (!value) {
    return 0;
  }
  const parsed = Date.parse(value);
  return Number.isNaN(parsed) ? 0 : parsed;
}

function formatTimestamp(value?: string) {
  if (!value) {
    return "unknown";
  }
  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime())) {
    return value;
  }
  return parsed.toLocaleString();
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
  incidents: RuntimeIncident[],
  dispatchAttempts: RuntimeDispatchAttempt[],
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
    case "incident": {
      const incident = incidents.find((item) => item.id === selection.id);
      if (!incident) {
        return null;
      }
      return {
        title: incident.summary || incident.id,
        kindLabel: "incident",
        summary: incident.source,
        items: compactDetailItems([
          { label: "Incident ID", value: incident.id },
          { label: "Automation", value: incident.automation_id },
          { label: "Status", value: incident.status },
          { label: "Signal", value: incident.signal_kind },
          { label: "Source", value: incident.source },
          { label: "Observed", value: formatTimestamp(incident.observed_at) },
          { label: "Updated", value: formatTimestamp(incident.updated_at || incident.created_at) },
        ]),
      };
    }
    case "dispatch": {
      const attempt = dispatchAttempts.find((item) => item.dispatch_id === selection.id);
      if (!attempt) {
        return null;
      }
      return {
        title: attempt.outcome_summary || attempt.dispatch_id,
        kindLabel: "dispatch",
        summary: attempt.child_agent_id,
        items: compactDetailItems([
          { label: "Dispatch ID", value: attempt.dispatch_id },
          { label: "Incident ID", value: attempt.incident_id },
          { label: "Automation", value: attempt.automation_id },
          { label: "Phase", value: attempt.phase },
          { label: "Attempt", value: String(attempt.attempt) },
          { label: "Status", value: attempt.status },
          { label: "Task ID", value: attempt.task_id },
          { label: "Permission Request", value: attempt.permission_request_id },
          { label: "Skills", value: attempt.activated_skill_names?.join(", ") },
          { label: "Updated", value: formatTimestamp(attempt.updated_at || attempt.created_at) },
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
  incidents,
  dispatchAttempts,
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
  incidents: RuntimeIncident[];
  dispatchAttempts: RuntimeDispatchAttempt[];
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
      const relatedDispatch = dispatchAttempts.find((item) => item.task_id === task?.id);
      const relatedToolRun = toolRuns.find((item) => item.task_id === task?.id);
      const relatedWorktree = worktrees.find((item) => item.task_id === task?.id || item.id === task?.worktree_id);
      const actions: SelectionAction[] = [];
      if (relatedReport) {
        actions.push({
          label: "Open related report",
          onClick: () => setSelection({ kind: "report", id: relatedReport.id }),
        });
      }
      if (relatedDispatch) {
        actions.push({
          label: "Open dispatch",
          onClick: () => setSelection({ kind: "dispatch", id: relatedDispatch.dispatch_id }),
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
    case "incident": {
      const incident = incidents.find((item) => item.id === selection.id);
      const latestDispatch = dispatchAttempts.find((item) => item.incident_id === incident?.id);
      if (!latestDispatch) {
        return [];
      }
      return [
        {
          label: "Open latest dispatch",
          onClick: () => setSelection({ kind: "dispatch", id: latestDispatch.dispatch_id }),
        },
      ];
    }
    case "dispatch": {
      const attempt = dispatchAttempts.find((item) => item.dispatch_id === selection.id);
      const relatedIncident = incidents.find((item) => item.id === attempt?.incident_id);
      const relatedTask = tasks.find((item) => item.id === attempt?.task_id);
      const relatedApproval = pendingApprovals.find((item) => item.id === attempt?.permission_request_id);
      const actions: SelectionAction[] = [];
      if (relatedIncident) {
        actions.push({
          label: "Open incident",
          onClick: () => setSelection({ kind: "incident", id: relatedIncident.id }),
        });
      }
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
