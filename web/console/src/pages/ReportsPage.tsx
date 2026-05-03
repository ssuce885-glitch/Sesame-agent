import { useReports } from "../api/queries";
import type { Report } from "../api/types";
import { useI18n } from "../i18n";
import type { ReactNode } from "react";
import { useNavigate } from "react-router-dom";

interface ReportsPageProps {
  workspaceRoot: string | null;
}

export function ReportsPage({ workspaceRoot }: ReportsPageProps) {
  const { t } = useI18n();
  const { data, isLoading, isError, refetch } = useReports(workspaceRoot);
  const reports = data?.items ?? [];

  return (
    <section className="h-full overflow-y-auto p-5" style={{ backgroundColor: "var(--color-bg)" }}>
      <div className="mx-auto max-w-6xl">
        <header className="mb-5 flex flex-col gap-2 md:flex-row md:items-end md:justify-between">
          <div>
            <h1 className="m-0 text-2xl font-semibold" style={{ color: "var(--color-text)" }}>
              {t("reports.title")}
            </h1>
            <p className="m-0 mt-1 text-sm" style={{ color: "var(--color-text-tertiary)" }}>
              {t("reports.subtitle")}
            </p>
          </div>
          {data && (
            <div className="text-xs" style={{ color: "var(--color-text-tertiary)" }}>
              {t("reports.queued", { count: data.queued_count })}
            </div>
          )}
        </header>

        {!workspaceRoot ? (
          <StateBox text={t("reports.noWorkspace")} />
        ) : isLoading ? (
          <LoadingRows />
        ) : isError ? (
          <ErrorBox text={t("reports.loadFailed")} onRetry={() => void refetch()} />
        ) : reports.length === 0 ? (
          <StateBox text={t("reports.empty")} />
        ) : (
          <div className="overflow-hidden rounded-md" style={{ border: "1px solid var(--color-border)" }}>
            {reports.map((report) => (
              <ReportRow key={report.id} report={report} />
            ))}
          </div>
        )}
      </div>
    </section>
  );
}

function ReportRow({ report }: { report: Report }) {
  const navigate = useNavigate();
  const canOpenTask = report.source_kind === "task_result" && report.source_id;
  return (
    <article
      className="grid gap-3 p-4 md:grid-cols-[1fr_120px_120px_170px]"
      style={{
        backgroundColor: "var(--color-bg-elevated)",
        borderBottom: "1px solid var(--color-border)",
      }}
    >
      <div className="min-w-0">
        <div className="flex flex-wrap items-center gap-2">
          <h2 className="m-0 truncate text-sm font-semibold" style={{ color: "var(--color-text)" }}>
            {report.title}
          </h2>
          <Badge value={report.source_kind} tone="neutral" />
        </div>
        <p className="m-0 mt-1 line-clamp-2 text-xs leading-5" style={{ color: "var(--color-text-tertiary)" }}>
          {report.summary}
        </p>
        {canOpenTask ? (
          <button
            type="button"
            onClick={() => navigate(`/tasks/${encodeURIComponent(report.source_id)}`)}
            className="mt-2 rounded px-2 py-1 text-[11px] font-medium"
            style={{
              border: "1px solid var(--color-border)",
              backgroundColor: "var(--color-surface)",
              color: "var(--color-accent)",
              cursor: "pointer",
            }}
          >
            {report.source_id}
          </button>
        ) : (
          <div className="mt-2 text-[11px]" style={{ color: "var(--color-text-tertiary)" }}>
            {report.source_id}
          </div>
        )}
      </div>
      <div>
        <ColumnLabel>Severity</ColumnLabel>
        <Badge value={report.severity} tone={severityTone(report.severity)} />
      </div>
      <div>
        <ColumnLabel>Status</ColumnLabel>
        <Badge value={report.status} tone={report.delivered ? "success" : "neutral"} />
      </div>
      <div className="text-xs" style={{ color: "var(--color-text-secondary)" }}>
        <ColumnLabel>Created</ColumnLabel>
        {formatDate(report.created_at)}
      </div>
    </article>
  );
}

function ColumnLabel({ children }: { children: ReactNode }) {
  return (
    <div className="mb-1 text-[11px] font-medium uppercase" style={{ color: "var(--color-text-tertiary)" }}>
      {children}
    </div>
  );
}

function Badge({ value, tone }: { value: string; tone: "neutral" | "success" | "warning" | "error" }) {
  const styles = {
    neutral: ["var(--color-surface)", "var(--color-text-tertiary)"],
    success: ["var(--color-success-dim)", "var(--color-success)"],
    warning: ["var(--color-warning-dim)", "var(--color-warning)"],
    error: ["var(--color-error-dim)", "var(--color-error)"],
  }[tone];

  return (
    <span className="inline-flex rounded px-1.5 py-0.5 text-[11px] font-medium" style={{ backgroundColor: styles[0], color: styles[1] }}>
      {value || "-"}
    </span>
  );
}

function severityTone(severity: string): "neutral" | "success" | "warning" | "error" {
  if (severity === "critical" || severity === "error" || severity === "high") {
    return "error";
  }
  if (severity === "warning" || severity === "medium") {
    return "warning";
  }
  if (severity === "info" || severity === "low") {
    return "success";
  }
  return "neutral";
}

function StateBox({ text }: { text: string }) {
  return <div className="rounded-md p-4 text-sm" style={{ backgroundColor: "var(--color-bg-elevated)", color: "var(--color-text-tertiary)", border: "1px solid var(--color-border)" }}>{text}</div>;
}

function ErrorBox({ text, onRetry }: { text: string; onRetry: () => void }) {
  return (
    <div className="rounded-md p-4 text-sm" style={{ backgroundColor: "var(--color-error-dim)", color: "var(--color-error)", border: "1px solid rgba(239,68,68,0.2)" }}>
      <div>{text}</div>
      <button type="button" className="mt-3 rounded px-3 py-1.5 text-xs font-medium" onClick={onRetry} style={{ border: "1px solid var(--color-error)", backgroundColor: "transparent", color: "var(--color-error)", cursor: "pointer" }}>
        Retry
      </button>
    </div>
  );
}

function LoadingRows() {
  return (
    <div className="space-y-2">
      {[0, 1, 2].map((item) => (
        <div key={item} className="animate-shimmer rounded-md" style={{ height: 92, backgroundColor: "var(--color-surface)" }} />
      ))}
    </div>
  );
}

function formatDate(value: string): string {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return value;
  }
  return date.toLocaleString();
}
