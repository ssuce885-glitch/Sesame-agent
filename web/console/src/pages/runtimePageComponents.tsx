import type { CSSProperties, ReactNode } from "react";
import type {
  HistoryEntry,
  RuntimeDiagnostic,
  RuntimeTask,
  RuntimeToolRun,
  RuntimeWorktree,
  WorkspaceReportDeliveryItem,
} from "../api/types";

export interface DetailItem {
  label: string;
  value: string;
}

export interface SelectionDetailCardAction {
  label: string;
  onClick: () => void | Promise<void>;
  disabled?: boolean;
}

export interface SelectionDetailCardData {
  title: string;
  kindLabel: string;
  summary?: string;
  items: DetailItem[];
}

export function SummaryCard({ label, value, detail }: { label: string; value: string; detail: string }) {
  return (
    <div
      className="rounded-xl px-5 py-4"
      style={{ backgroundColor: "var(--color-surface)", border: "1px solid var(--color-border)" }}
    >
      <div className="text-xs font-medium uppercase tracking-wide" style={{ color: "var(--color-text-muted)" }}>
        {label}
      </div>
      <div className="mt-3 text-4xl font-semibold" style={{ color: "var(--color-text)" }}>
        {value}
      </div>
      <div className="mt-2 text-sm" style={{ color: "var(--color-text-muted)" }}>
        {detail}
      </div>
    </div>
  );
}

export function Panel({
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
      <div className="mb-4" style={{ borderBottom: "1px solid var(--color-border)", paddingBottom: 12 }}>
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

export function ContextHeadRow({
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
      style={rowBaseStyle(selected)}
      onMouseEnter={(e) => rowHoverIn(e, selected)}
      onMouseLeave={(e) => rowHoverOut(e, selected)}
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

export function TaskRow({ task, selected, onSelect }: { task: RuntimeTask; selected: boolean; onSelect: () => void }) {
  return (
    <button
      type="button"
      onClick={onSelect}
      aria-pressed={selected}
      className="rounded-xl px-4 py-3"
      style={rowBaseStyle(selected)}
      onMouseEnter={(e) => rowHoverIn(e, selected)}
      onMouseLeave={(e) => rowHoverOut(e, selected)}
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

export function ToolRunRow({
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
      style={rowBaseStyle(selected)}
      onMouseEnter={(e) => rowHoverIn(e, selected)}
      onMouseLeave={(e) => rowHoverOut(e, selected)}
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

export function WorktreeRow({
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
      style={rowBaseStyle(selected)}
      onMouseEnter={(e) => rowHoverIn(e, selected)}
      onMouseLeave={(e) => rowHoverOut(e, selected)}
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

export function ReportRow({
  item,
  selected,
  onSelect,
}: {
  item: WorkspaceReportDeliveryItem;
  selected: boolean;
  onSelect: () => void;
}) {
  return (
    <button
      type="button"
      onClick={onSelect}
      aria-pressed={selected}
      className="rounded-xl px-4 py-3"
      style={rowBaseStyle(selected)}
      onMouseEnter={(e) => rowHoverIn(e, selected)}
      onMouseLeave={(e) => rowHoverOut(e, selected)}
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

export function DiagnosticRow({
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
      style={rowBaseStyle(selected)}
      onMouseEnter={(e) => rowHoverIn(e, selected)}
      onMouseLeave={(e) => rowHoverOut(e, selected)}
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

export function SelectionDetailCard({
  detail,
  actions,
}: {
  detail: SelectionDetailCardData;
  actions: SelectionDetailCardAction[];
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

function toneFromSeverity(severity?: string): "accent" | "success" | "warning" | "danger" | "neutral" {
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

function rowBaseStyle(selected: boolean): CSSProperties {
  return {
    backgroundColor: selected ? "rgba(62, 130, 247, 0.08)" : "var(--color-surface-2)",
    border: `1px solid ${selected ? "var(--color-accent)" : "var(--color-border)"}`,
    textAlign: "left",
    transition: "background-color 0.15s, border-color 0.15s",
    cursor: "pointer",
  };
}

function rowHoverIn(e: React.MouseEvent<HTMLButtonElement>, selected: boolean) {
  if (selected) return;
  e.currentTarget.style.backgroundColor = "rgba(255,255,255,0.03)";
  e.currentTarget.style.borderColor = "var(--color-text-muted)";
}

function rowHoverOut(e: React.MouseEvent<HTMLButtonElement>, selected: boolean) {
  if (selected) return;
  e.currentTarget.style.backgroundColor = "var(--color-surface-2)";
  e.currentTarget.style.borderColor = "var(--color-border)";
}

export function sortByUpdatedAtDesc<T extends { updated_at?: string; created_at?: string }>(a: T, b: T) {
  return compareTimestamps(b.updated_at || b.created_at, a.updated_at || a.created_at);
}

export function sortByCreatedAtDesc<T extends { created_at?: string }>(a: T, b: T) {
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

export function formatTimestamp(value?: string) {
  if (!value) {
    return "unknown";
  }
  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime())) {
    return value;
  }
  return parsed.toLocaleString();
}
