import { useState } from "react";
import type { CSSProperties, ReactNode } from "react";
import type {
  HistoryEntry,
  RuntimeDiagnostic,
  RuntimeTask,
  RuntimeToolRun,
  RuntimeWorktree,
  WorkspaceReportDeliveryItem,
} from "../api/types";
import {
  GitBranch,
  Layers,
  Cpu,
  FileText,
  Wrench,
  AlertTriangle,
  Check,
  X,
  Circle,
  Play,
  ChevronDown,
  ChevronUp,
  Clock,
  Mail,
  Database,
} from "../components/Icon";

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

/* ─── SummaryCard (KPI style) ──────────────────────────────────── */

export function SummaryCard({
  label,
  value,
  detail,
  accent = "accent",
}: {
  label: string;
  value: string;
  detail: string;
  accent?: "accent" | "success" | "warning" | "error";
}) {
  const accentMap = {
    accent: "var(--color-accent)",
    success: "var(--color-success)",
    warning: "var(--color-warning)",
    error: "var(--color-error)",
  };
  const color = accentMap[accent];
  return (
    <div
      className="rounded-lg px-4 py-3 relative overflow-hidden"
      style={{ backgroundColor: "var(--color-surface)", border: "1px solid var(--color-border)" }}
    >
      <div
        className="absolute bottom-0 left-0 right-0 h-0.5"
        style={{ backgroundColor: color }}
      />
      <div className="text-[11px] font-semibold uppercase tracking-wider" style={{ color: "var(--color-text-tertiary)" }}>
        {label}
      </div>
      <div className="mt-1 text-2xl font-bold tabular-nums" style={{ color: "var(--color-text)" }}>
        {value}
      </div>
      <div className="mt-1 text-xs" style={{ color: "var(--color-text-secondary)" }}>
        {detail}
      </div>
    </div>
  );
}

/* ─── Panel (collapsible widget) ───────────────────────────────── */

export function Panel({
  title,
  subtitle,
  emptyText,
  collapsible = false,
  children,
}: {
  title: string;
  subtitle: string;
  emptyText: string;
  collapsible?: boolean;
  children: ReactNode;
}) {
  const [collapsed, setCollapsed] = useState(false);
  const items = Array.isArray(children) ? children.filter(Boolean) : children ? [children] : [];

  return (
    <section
      className="rounded-lg overflow-hidden"
      style={{ backgroundColor: "var(--color-surface)", border: "1px solid var(--color-border)" }}
    >
      <div
        className="flex items-center justify-between px-4 py-2.5"
        style={{ borderBottom: items.length > 0 && !collapsed ? "1px solid var(--color-border)" : "none" }}
      >
        <div>
          <h2 className="text-xs font-semibold uppercase tracking-wider m-0" style={{ color: "var(--color-text)" }}>
            {title}
          </h2>
          <p className="text-[11px] m-0 mt-0.5" style={{ color: "var(--color-text-tertiary)" }}>
            {subtitle}
          </p>
        </div>
        {collapsible && (
          <button
            type="button"
            onClick={() => setCollapsed((v) => !v)}
            className="flex items-center justify-center w-6 h-6 rounded"
            style={{ backgroundColor: "transparent", border: "none", color: "var(--color-text-tertiary)", cursor: "pointer" }}
          >
            {collapsed ? <ChevronDown size={14} /> : <ChevronUp size={14} />}
          </button>
        )}
      </div>
      {!collapsed && (
        <div className="px-2 py-2">
          {items.length > 0 ? (
            <div className="flex flex-col gap-1">{items}</div>
          ) : (
            <div
              className="rounded border border-dashed px-4 py-4 text-xs text-center"
              style={{ color: "var(--color-text-tertiary)", borderColor: "var(--color-border)" }}
            >
              {emptyText}
            </div>
          )}
        </div>
      )}
    </section>
  );
}

/* ─── Row components (list-item style) ─────────────────────────── */

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
    <button type="button" onClick={onSelect} className="w-full text-left flex items-center gap-3 px-3 py-2.5 rounded-md"
      style={rowBaseStyle(selected)} onMouseEnter={(e) => rowHoverIn(e, selected)} onMouseLeave={(e) => rowHoverOut(e, selected)}>
      <GitBranch size={14} color={entry.is_current ? "var(--color-accent)" : "var(--color-text-tertiary)"} />
      <div className="flex-1 min-w-0">
        <div className="text-sm font-medium truncate" style={{ color: "var(--color-text)" }}>
          {entry.title || entry.id}
        </div>
        {entry.preview && (
          <div className="text-xs truncate mt-0.5" style={{ color: "var(--color-text-tertiary)" }}>
            {entry.preview}
          </div>
        )}
      </div>
      <StatusBadge tone={entry.is_current ? "accent" : "neutral"}>
        {entry.is_current ? "Current" : entry.source_kind || "history"}
      </StatusBadge>
      <span className="text-[11px] shrink-0" style={{ color: "var(--color-text-tertiary)" }}>
        {formatTimestamp(entry.updated_at)}
      </span>
    </button>
  );
}

export function TaskRow({ task, selected, onSelect }: { task: RuntimeTask; selected: boolean; onSelect: () => void }) {
  return (
    <button type="button" onClick={onSelect} className="w-full text-left flex items-center gap-3 px-3 py-2.5 rounded-md"
      style={rowBaseStyle(selected)} onMouseEnter={(e) => rowHoverIn(e, selected)} onMouseLeave={(e) => rowHoverOut(e, selected)}>
      <Cpu size={14} color="var(--color-text-tertiary)" />
      <div className="flex-1 min-w-0">
        <div className="text-sm font-medium truncate" style={{ color: "var(--color-text)" }}>
          {task.title || task.id}
        </div>
        {task.description && (
          <div className="text-xs truncate mt-0.5" style={{ color: "var(--color-text-tertiary)" }}>
            {task.description}
          </div>
        )}
      </div>
      <div className="flex items-center gap-3 shrink-0">
        <span className="text-[11px]" style={{ color: "var(--color-text-tertiary)" }}>
          {task.owner || "runtime"}
        </span>
        <StatusBadge tone={toneFromState(task.state)}>{task.state}</StatusBadge>
        <span className="text-[11px]" style={{ color: "var(--color-text-tertiary)" }}>
          {formatTimestamp(task.updated_at)}
        </span>
      </div>
    </button>
  );
}

export function ToolRunRow({ toolRun, selected, onSelect }: { toolRun: RuntimeToolRun; selected: boolean; onSelect: () => void }) {
  return (
    <button type="button" onClick={onSelect} className="w-full text-left flex items-center gap-3 px-3 py-2.5 rounded-md"
      style={rowBaseStyle(selected)} onMouseEnter={(e) => rowHoverIn(e, selected)} onMouseLeave={(e) => rowHoverOut(e, selected)}>
      <Wrench size={14} color="var(--color-text-tertiary)" />
      <div className="flex-1 min-w-0">
        <div className="text-sm font-medium truncate" style={{ color: "var(--color-text)" }}>
          {toolRun.tool_name}
        </div>
        {toolRun.input_json && (
          <div className="text-xs truncate mt-0.5 font-mono" style={{ color: "var(--color-text-tertiary)" }}>
            {toolRun.input_json.length > 80 ? toolRun.input_json.slice(0, 80) + "…" : toolRun.input_json}
          </div>
        )}
      </div>
      <div className="flex items-center gap-3 shrink-0">
        <span className="text-[11px]" style={{ color: "var(--color-text-tertiary)" }}>
          {toolRun.task_id || "—"}
        </span>
        <StatusBadge tone={toneFromState(toolRun.state)}>{toolRun.state}</StatusBadge>
        <span className="text-[11px]" style={{ color: "var(--color-text-tertiary)" }}>
          {formatTimestamp(toolRun.updated_at || toolRun.created_at)}
        </span>
      </div>
    </button>
  );
}

export function WorktreeRow({ worktree, selected, onSelect }: { worktree: RuntimeWorktree; selected: boolean; onSelect: () => void }) {
  return (
    <button type="button" onClick={onSelect} className="w-full text-left flex items-center gap-3 px-3 py-2.5 rounded-md"
      style={rowBaseStyle(selected)} onMouseEnter={(e) => rowHoverIn(e, selected)} onMouseLeave={(e) => rowHoverOut(e, selected)}>
      <Layers size={14} color="var(--color-text-tertiary)" />
      <div className="flex-1 min-w-0">
        <div className="text-sm font-medium truncate" style={{ color: "var(--color-text)" }}>
          {worktree.worktree_branch || worktree.id}
        </div>
        <div className="text-xs truncate mt-0.5 font-mono" style={{ color: "var(--color-text-tertiary)" }}>
          {worktree.worktree_path}
        </div>
      </div>
      <div className="flex items-center gap-3 shrink-0">
        <span className="text-[11px]" style={{ color: "var(--color-text-tertiary)" }}>
          {worktree.task_id || "—"}
        </span>
        <StatusBadge tone={toneFromState(worktree.state)}>{worktree.state}</StatusBadge>
        <span className="text-[11px]" style={{ color: "var(--color-text-tertiary)" }}>
          {formatTimestamp(worktree.updated_at || worktree.created_at)}
        </span>
      </div>
    </button>
  );
}

export function ReportRow({ item, selected, onSelect }: { item: WorkspaceReportDeliveryItem; selected: boolean; onSelect: () => void }) {
  return (
    <button type="button" onClick={onSelect} className="w-full text-left flex items-center gap-3 px-3 py-2.5 rounded-md"
      style={rowBaseStyle(selected)} onMouseEnter={(e) => rowHoverIn(e, selected)} onMouseLeave={(e) => rowHoverOut(e, selected)}>
      <Mail size={14} color="var(--color-text-tertiary)" />
      <div className="flex-1 min-w-0">
        <div className="text-sm font-medium truncate" style={{ color: "var(--color-text)" }}>
          {item.envelope.title || item.source_id}
        </div>
        {item.envelope.summary && (
          <div className="text-xs truncate mt-0.5" style={{ color: "var(--color-text-tertiary)" }}>
            {item.envelope.summary}
          </div>
        )}
      </div>
      <div className="flex items-center gap-3 shrink-0">
        <span className="text-[11px]" style={{ color: "var(--color-text-tertiary)" }}>
          {item.source_role_id || item.source_kind}
        </span>
        <StatusBadge tone={toneFromSeverity(item.envelope.severity)}>
          {item.delivery_state || "queued"}
        </StatusBadge>
        <span className="text-[11px]" style={{ color: "var(--color-text-tertiary)" }}>
          {formatTimestamp(item.updated_at || item.created_at)}
        </span>
      </div>
    </button>
  );
}

export function DiagnosticRow({ diagnostic, selected, onSelect }: { diagnostic: RuntimeDiagnostic; selected: boolean; onSelect: () => void }) {
  return (
    <button type="button" onClick={onSelect} className="w-full text-left flex items-center gap-3 px-3 py-2.5 rounded-md"
      style={rowBaseStyle(selected)} onMouseEnter={(e) => rowHoverIn(e, selected)} onMouseLeave={(e) => rowHoverOut(e, selected)}>
      <AlertTriangle size={14} color={diagnostic.severity === "error" || diagnostic.severity === "critical" ? "var(--color-error)" : "var(--color-warning)"} />
      <div className="flex-1 min-w-0">
        <div className="text-sm font-medium truncate" style={{ color: "var(--color-text)" }}>
          {diagnostic.summary || diagnostic.event_type || diagnostic.id}
        </div>
        {diagnostic.reason && (
          <div className="text-xs truncate mt-0.5" style={{ color: "var(--color-text-tertiary)" }}>
            {diagnostic.reason}
          </div>
        )}
      </div>
      <div className="flex items-center gap-3 shrink-0">
        <span className="text-[11px]" style={{ color: "var(--color-text-tertiary)" }}>
          {diagnostic.category || diagnostic.event_type}
        </span>
        <StatusBadge tone={toneFromSeverity(diagnostic.severity)}>
          {diagnostic.severity || "info"}
        </StatusBadge>
        <span className="text-[11px]" style={{ color: "var(--color-text-tertiary)" }}>
          {formatTimestamp(diagnostic.created_at)}
        </span>
      </div>
    </button>
  );
}

/* ─── SelectionDetailCard ──────────────────────────────────────── */

export function SelectionDetailCard({
  detail,
  actions,
}: {
  detail: SelectionDetailCardData;
  actions: SelectionDetailCardAction[];
}) {
  return (
    <div className="rounded-lg overflow-hidden" style={{ backgroundColor: "var(--color-bg-elevated)", border: "1px solid var(--color-border)" }}>
      <div className="px-4 py-3" style={{ borderBottom: "1px solid var(--color-border)" }}>
        <div className="flex items-center justify-between gap-3">
          <div className="min-w-0">
            <div className="text-sm font-semibold truncate" style={{ color: "var(--color-text)" }}>
              {detail.title}
            </div>
            {detail.summary && (
              <div className="text-xs mt-0.5 leading-relaxed" style={{ color: "var(--color-text-secondary)" }}>
                {detail.summary}
              </div>
            )}
          </div>
          <StatusBadge tone="accent">{detail.kindLabel}</StatusBadge>
        </div>
      </div>

      {actions.length > 0 && (
        <div className="px-4 py-2 flex flex-wrap gap-2" style={{ borderBottom: "1px solid var(--color-border)" }}>
          {actions.map((action) => (
            <button
              key={action.label}
              type="button"
              onClick={() => void action.onClick()}
              disabled={action.disabled}
              className="rounded px-2.5 py-1 text-xs font-medium"
              style={{
                backgroundColor: "var(--color-surface)",
                border: "1px solid var(--color-border)",
                color: "var(--color-text)",
                opacity: action.disabled ? 0.5 : 1,
                cursor: action.disabled ? "not-allowed" : "pointer",
              }}
            >
              {action.label}
            </button>
          ))}
        </div>
      )}

      <div className="p-4 grid grid-cols-1 gap-x-4 gap-y-3 md:grid-cols-2">
        {detail.items.map((item) => (
          <div key={`${detail.kindLabel}:${item.label}`}>
            <div className="text-[10px] font-semibold uppercase tracking-wider mb-0.5" style={{ color: "var(--color-text-tertiary)" }}>
              {item.label}
            </div>
            <div className="text-xs break-words font-mono leading-relaxed" style={{ color: "var(--color-text-secondary)" }}>
              {item.value}
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}

/* ─── StatusBadge ──────────────────────────────────────────────── */

function StatusBadge({
  children,
  tone,
}: {
  children: ReactNode;
  tone: "accent" | "success" | "warning" | "danger" | "neutral";
}) {
  const palette = {
    accent: { bg: "rgba(59,130,246,0.12)", color: "#60a5fa" },
    success: { bg: "rgba(34,197,94,0.12)", color: "#4ade80" },
    warning: { bg: "rgba(245,158,11,0.12)", color: "#fbbf24" },
    danger: { bg: "rgba(239,68,68,0.12)", color: "#f87171" },
    neutral: { bg: "rgba(92,99,112,0.15)", color: "#9aa2b0" },
  } as const;

  return (
    <span
      className="rounded px-1.5 py-0.5 text-[10px] font-semibold uppercase tracking-wide shrink-0"
      style={{ backgroundColor: palette[tone].bg, color: palette[tone].color }}
    >
      {children}
    </span>
  );
}

/* ─── Helpers ──────────────────────────────────────────────────── */

function toneFromState(state: string): "accent" | "success" | "warning" | "danger" | "neutral" {
  switch (state) {
    case "running": return "accent";
    case "completed": return "success";
    case "pending": return "warning";
    case "failed":
    case "cancelled": return "danger";
    default: return "neutral";
  }
}

function toneFromSeverity(severity?: string): "accent" | "success" | "warning" | "danger" | "neutral" {
  switch (severity) {
    case "critical":
    case "error": return "danger";
    case "warning": return "warning";
    case "ok":
    case "success": return "success";
    default: return "neutral";
  }
}

function rowBaseStyle(selected: boolean): CSSProperties {
  return {
    backgroundColor: selected ? "rgba(59,130,246,0.06)" : "transparent",
    border: `1px solid ${selected ? "rgba(59,130,246,0.2)" : "transparent"}`,
    textAlign: "left",
    transition: "background-color 0.1s, border-color 0.1s",
    cursor: "pointer",
  };
}

function rowHoverIn(e: React.MouseEvent<HTMLButtonElement>, selected: boolean) {
  if (selected) return;
  e.currentTarget.style.backgroundColor = "rgba(255,255,255,0.03)";
}

function rowHoverOut(e: React.MouseEvent<HTMLButtonElement>, selected: boolean) {
  if (selected) return;
  e.currentTarget.style.backgroundColor = "transparent";
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
  if (!value) return 0;
  const parsed = Date.parse(value);
  return Number.isNaN(parsed) ? 0 : parsed;
}

export function formatTimestamp(value?: string) {
  if (!value) return "—";
  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime())) return value;
  return parsed.toLocaleString(undefined, {
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  });
}

