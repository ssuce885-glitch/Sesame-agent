import { useState } from "react";
import { useI18n } from "../i18n";
import {
  MessageSquare,
  Play,
  FileText,
  Users,
  Activity,
  Database,
  ChevronRight,
  Globe,
} from "./Icon";

interface SidebarProps {
  workspaceName?: string;
  workspaceRoot?: string;
  sessionId?: string;
  connection?: "idle" | "connecting" | "open" | "reconnecting" | "error";
  activePath: string;
  onNavigate: (path: string) => void;
}

export function Sidebar({
  workspaceName,
  workspaceRoot,
  sessionId,
  connection,
  activePath,
  onNavigate,
}: SidebarProps) {
  const { t } = useI18n();
  const [expanded, setExpanded] = useState(false);

  const connColor =
    connection === "open"
      ? "#22c55e"
      : connection === "reconnecting" || connection === "connecting"
        ? "#f59e0b"
        : connection === "error"
          ? "#ef4444"
          : "#5c6370";

  const connLabel =
    connection === "open"
      ? t("sidebar.connected")
      : connection === "connecting"
        ? t("sidebar.connecting")
        : connection === "reconnecting"
          ? t("sidebar.reconnecting")
          : connection === "error"
            ? t("sidebar.error")
            : t("sidebar.idle");

  const navItems = [
    { path: "/chat", label: t("nav.chat"), icon: MessageSquare },
    { path: "/roles", label: t("nav.roles"), icon: Users },
    { path: "/tasks", label: t("nav.tasks"), icon: Activity },
    { path: "/context", label: t("nav.context"), icon: Database },
    { path: "/automations", label: t("nav.automations"), icon: Play },
    { path: "/reports", label: t("nav.reports"), icon: FileText },
  ];

  return (
    <aside
      className="flex flex-col h-full select-none"
      style={{
        width: expanded ? 220 : 60,
        minWidth: expanded ? 220 : 60,
        backgroundColor: "var(--color-bg-elevated)",
        borderRight: "1px solid var(--color-border)",
        transition: "width 0.2s ease",
      }}
    >
      {/* Brand */}
      <div
        className="flex items-center justify-between h-12 px-3"
        style={{ borderBottom: "1px solid var(--color-border)" }}
      >
        <div className="flex items-center gap-2 overflow-hidden">
          <div
            className="w-6 h-6 rounded flex items-center justify-center shrink-0"
            style={{ backgroundColor: "var(--color-accent)" }}
          >
            <span className="text-xs font-bold text-white">S</span>
          </div>
          {expanded && (
            <span
              className="text-sm font-semibold whitespace-nowrap"
              style={{ color: "var(--color-text)" }}
            >
              Sesame
            </span>
          )}
        </div>
        <button
          type="button"
          onClick={() => setExpanded((v) => !v)}
          className="shrink-0 flex items-center justify-center w-6 h-6 rounded"
          style={{
            backgroundColor: "transparent",
            border: "none",
            color: "var(--color-text-tertiary)",
            cursor: "pointer",
            transform: expanded ? "rotate(90deg)" : "rotate(0deg)",
            transition: "transform 0.2s ease",
          }}
          title={expanded ? "Collapse" : "Expand"}
        >
          <ChevronRight size={14} />
        </button>
      </div>

      {/* Navigation */}
      <nav className="flex-1 py-2 flex flex-col gap-0.5">
        {navItems.map((item) => {
          const active = activePath === item.path;
          const Icon = item.icon;
          return (
            <button
              key={item.path}
              type="button"
              onClick={() => onNavigate(item.path)}
              className="flex items-center gap-3 mx-2 rounded-md text-sm font-medium"
              style={{
                height: 36,
                padding: expanded ? "0 10px" : "0",
                justifyContent: expanded ? "flex-start" : "center",
                backgroundColor: active
                  ? "var(--color-accent-dim)"
                  : "transparent",
                color: active
                  ? "var(--color-accent)"
                  : "var(--color-text-secondary)",
                border: "none",
                cursor: "pointer",
                transition: "background-color 0.15s, color 0.15s",
              }}
              onMouseEnter={(e) => {
                if (!active) {
                  e.currentTarget.style.backgroundColor =
                    "var(--color-surface)";
                  e.currentTarget.style.color = "var(--color-text)";
                }
              }}
              onMouseLeave={(e) => {
                if (!active) {
                  e.currentTarget.style.backgroundColor = "transparent";
                  e.currentTarget.style.color =
                    "var(--color-text-secondary)";
                }
              }}
              title={!expanded ? item.label : undefined}
            >
              <Icon size={18} />
              {expanded && (
                <span className="whitespace-nowrap">{item.label}</span>
              )}
            </button>
          );
        })}
      </nav>

      {/* Workspace / Session info */}
      <div
        className="py-2 px-2"
        style={{ borderTop: "1px solid var(--color-border)" }}
      >
        {/* Workspace */}
        <div
          className="flex items-center gap-2 rounded-md px-2"
          style={{ height: 32 }}
          title={workspaceRoot}
        >
          <Globe
            size={14}
            color="var(--color-text-tertiary)"
          />
          {expanded && (
            <span
              className="text-xs truncate"
              style={{ color: "var(--color-text-secondary)" }}
            >
              {workspaceName || "—"}
            </span>
          )}
        </div>

        {/* Session status */}
        {sessionId && (
          <div
            className="flex items-center gap-2 rounded-md px-2"
            style={{ height: 32 }}
          >
            <span
              className="w-2 h-2 rounded-full shrink-0"
              style={{
                backgroundColor: connColor,
                boxShadow: `0 0 6px ${connColor}40`,
              }}
            />
            {expanded && (
              <span
                className="text-xs truncate"
                style={{ color: "var(--color-text-secondary)" }}
              >
                {sessionId.slice(0, 8)}... · {connLabel}
              </span>
            )}
          </div>
        )}
      </div>
    </aside>
  );
}
