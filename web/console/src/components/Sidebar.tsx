import { useI18n } from "../i18n";

interface SidebarProps {
  workspaceName?: string;
  workspaceRoot?: string;
}

export function Sidebar({ workspaceName, workspaceRoot }: SidebarProps) {
  const { t } = useI18n();

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

      <div className="flex-1 px-4 py-4">
        <div
          className="rounded-xl p-4"
          style={{
            backgroundColor: "var(--color-surface)",
            border: "1px solid var(--color-border)",
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
      </div>
    </aside>
  );
}
