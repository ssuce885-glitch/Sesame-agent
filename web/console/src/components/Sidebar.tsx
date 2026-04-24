import { useI18n } from "../i18n";
import { useCurrentSession } from "../api/queries";

interface SidebarProps {
  workspaceName?: string;
  workspaceRoot?: string;
  connection?: "idle" | "connecting" | "open" | "reconnecting" | "error";
}

export function Sidebar({ workspaceName, workspaceRoot, connection }: SidebarProps) {
  const { t } = useI18n();
  const { data: currentSession } = useCurrentSession();

  const connColor =
    connection === "open"
      ? "var(--color-success)"
      : connection === "reconnecting" || connection === "connecting"
      ? "var(--color-warning)"
      : connection === "error"
      ? "var(--color-error)"
      : "var(--color-border)";

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

  return (
    <aside
      className="flex w-full shrink-0 flex-col md:h-full md:w-[240px] md:min-w-[240px]"
      style={{
        backgroundColor: "var(--color-surface-2)",
        borderRight: "1px solid var(--color-border)",
      }}
    >
      {/* Header */}
      <div
        className="flex items-center justify-between px-4 py-3"
        style={{ borderBottom: "1px solid var(--color-border)" }}
      >
        <span className="text-sm font-semibold" style={{ color: "var(--color-text-muted)" }}>
          {t("sidebar.title")}
        </span>
      </div>

      <div className="flex-1 px-4 py-4 space-y-4 overflow-y-auto">
        {/* Workspace card */}
        <div
          className="rounded-xl p-4"
          style={{
            backgroundColor: "var(--color-surface)",
            border: "1px solid var(--color-border)",
            transition: "border-color 0.15s",
          }}
          onMouseEnter={(e) => {
            e.currentTarget.style.borderColor = "var(--color-text-muted)";
          }}
          onMouseLeave={(e) => {
            e.currentTarget.style.borderColor = "var(--color-border)";
          }}
        >
          <p className="text-xs uppercase tracking-wide" style={{ color: "var(--color-text-muted)" }}>
            {t("sidebar.currentBinding")}
          </p>
          <p className="mt-2 text-sm font-medium" style={{ color: "var(--color-text)" }}>
            {workspaceName || t("app.currentWorkspace")}
          </p>
          <p className="mt-2 text-xs break-all" style={{ color: "var(--color-text-muted)" }}>
            {workspaceRoot || t("sidebar.waitingMetadata")}
          </p>
          <p className="mt-4 text-xs leading-5" style={{ color: "var(--color-text-muted)" }}>
            {t("sidebar.bindingDescription")}
          </p>
        </div>

        {/* Session status */}
        {currentSession && (
          <div
            className="rounded-xl p-4"
            style={{
              backgroundColor: "var(--color-surface)",
              border: "1px solid var(--color-border)",
            }}
          >
            <p className="text-xs uppercase tracking-wide mb-3" style={{ color: "var(--color-text-muted)" }}>
              {t("sidebar.session")}
            </p>
            <div className="space-y-2">
              <div className="flex items-center justify-between">
                <span className="text-xs" style={{ color: "var(--color-text-muted)" }}>
                  ID
                </span>
                <span
                  className="text-xs font-mono"
                  style={{
                    color: "var(--color-text)",
                    fontFamily: "var(--font-mono)",
                  }}
                >
                  {currentSession.id.length > 12
                    ? currentSession.id.slice(0, 12) + "…"
                    : currentSession.id}
                </span>
              </div>
              {connection && (
                <div className="flex items-center justify-between">
                  <span className="text-xs" style={{ color: "var(--color-text-muted)" }}>
                    {t("sidebar.status")}
                  </span>
                  <span className="flex items-center gap-1.5 text-xs" style={{ color: "var(--color-text-muted)" }}>
                    <span
                      className="inline-block w-1.5 h-1.5 rounded-full"
                      style={{ backgroundColor: connColor }}
                    />
                    {connLabel}
                  </span>
                </div>
              )}
            </div>
          </div>
        )}
      </div>
    </aside>
  );
}
