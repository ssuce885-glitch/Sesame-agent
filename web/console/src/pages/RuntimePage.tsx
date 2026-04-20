import type { ReactNode } from "react";
import {
  useContextHistory,
  useWorkspaceMailbox,
  useWorkspaceRuntimeGraph,
} from "../api/queries";
import type { HistoryEntry, RuntimePermissionRequest, RuntimeTask, WorkspaceMailboxItem } from "../api/types";

interface RuntimePageProps {
  sessionId: string;
}

export function RuntimePage({ sessionId }: RuntimePageProps) {
  const history = useContextHistory(sessionId || null);
  const runtimeGraph = useWorkspaceRuntimeGraph();
  const mailbox = useWorkspaceMailbox();

  const contextEntries = [...(history.data?.entries ?? [])].sort(sortByUpdatedAtDesc);
  const tasks = [...(runtimeGraph.data?.graph.tasks ?? [])].sort(sortByUpdatedAtDesc);
  const pendingApprovals = (runtimeGraph.data?.graph.permission_requests ?? []).filter(
    (request) => request.status === "requested",
  );
  const mailboxItems = [...(mailbox.data?.items ?? [])].sort(sortByUpdatedAtDesc);
  const activeTaskCount = tasks.filter((task) => task.state === "running" || task.state === "pending").length;

  const isInitialLoading =
    (history.isLoading && !history.data) ||
    (runtimeGraph.isLoading && !runtimeGraph.data) ||
    (mailbox.isLoading && !mailbox.data);

  if (isInitialLoading) {
    return (
      <div className="flex h-full items-center justify-center text-sm" style={{ color: "var(--color-text-muted)" }}>
        Loading workspace runtime...
      </div>
    );
  }

  return (
    <div className="flex flex-col gap-6 overflow-y-auto p-4 md:p-6" style={{ backgroundColor: "var(--color-bg)" }}>
      <div className="flex flex-col gap-2">
        <h1 className="text-lg font-semibold" style={{ color: "var(--color-text)" }}>
          Workspace Runtime
        </h1>
        <p className="text-sm" style={{ color: "var(--color-text-muted)" }}>
          Context history, task execution, and report delivery for the current workspace.
        </p>
      </div>

      <div className="grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-4">
        <SummaryCard label="Context Heads" value={String(contextEntries.length)} detail="Available history branches" />
        <SummaryCard label="Active Tasks" value={String(activeTaskCount)} detail={`${tasks.length} total tracked tasks`} />
        <SummaryCard
          label="Pending Reports"
          value={String(mailbox.data?.pending_count ?? 0)}
          detail={`${mailboxItems.length} mailbox items`}
        />
        <SummaryCard
          label="Approval Requests"
          value={String(pendingApprovals.length)}
          detail="Runtime permission waits"
        />
      </div>

      <div className="grid grid-cols-1 gap-6 xl:grid-cols-[1.1fr_1fr]">
        <Panel
          title="Context Heads"
          subtitle="Current session history branches"
          emptyText="No context heads recorded yet."
        >
          {contextEntries.map((entry) => (
            <ContextHeadRow key={entry.id} entry={entry} />
          ))}
        </Panel>

        <Panel
          title="Pending Reports"
          subtitle="Workspace mailbox deliveries waiting on the parent flow"
          emptyText="No reports waiting."
        >
          {mailboxItems.map((item) => (
            <ReportRow key={item.id} item={item} />
          ))}
        </Panel>
      </div>

      <div className="grid grid-cols-1 gap-6 xl:grid-cols-[1.3fr_0.9fr]">
        <Panel title="Tasks" subtitle="Workspace execution spine" emptyText="No tasks recorded yet.">
          {tasks.map((task) => (
            <TaskRow key={task.id} task={task} />
          ))}
        </Panel>

        <Panel
          title="Approval Queue"
          subtitle="Permission requests currently blocking execution"
          emptyText="No approval requests are waiting."
        >
          {pendingApprovals.map((request) => (
            <ApprovalRow key={request.id} request={request} />
          ))}
        </Panel>
      </div>
    </div>
  );
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

function ContextHeadRow({ entry }: { entry: HistoryEntry }) {
  return (
    <div
      className="rounded-xl px-4 py-3"
      style={{ backgroundColor: "var(--color-surface-2)", border: "1px solid var(--color-border)" }}
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
    </div>
  );
}

function TaskRow({ task }: { task: RuntimeTask }) {
  return (
    <div
      className="rounded-xl px-4 py-3"
      style={{ backgroundColor: "var(--color-surface-2)", border: "1px solid var(--color-border)" }}
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
    </div>
  );
}

function ReportRow({ item }: { item: WorkspaceMailboxItem }) {
  return (
    <div
      className="rounded-xl px-4 py-3"
      style={{ backgroundColor: "var(--color-surface-2)", border: "1px solid var(--color-border)" }}
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
    </div>
  );
}

function ApprovalRow({ request }: { request: RuntimePermissionRequest }) {
  return (
    <div
      className="rounded-xl px-4 py-3"
      style={{ backgroundColor: "var(--color-surface-2)", border: "1px solid var(--color-border)" }}
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

function sortByUpdatedAtDesc<T extends { updated_at?: string; created_at?: string }>(a: T, b: T) {
  return compareTimestamps(b.updated_at || b.created_at, a.updated_at || a.created_at);
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
