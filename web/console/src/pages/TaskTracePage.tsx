import { useMemo } from "react";
import { useNavigate } from "react-router-dom";
import { useTaskTrace } from "../api/queries";
import type { Report, TaskTrace, TaskTraceEvent, TaskTraceMessage } from "../api/types";
import { MarkdownRenderer } from "../components/MarkdownRenderer";
import { ArrowDown, RefreshCw, X } from "../components/Icon";
import { useI18n } from "../i18n";

interface TaskTracePageProps {
  taskId: string;
}

export function TaskTracePage({ taskId }: TaskTracePageProps) {
  const { t } = useI18n();
  const navigate = useNavigate();
  const traceQuery = useTaskTrace(taskId);
  const trace = traceQuery.data ?? null;

  return (
    <section className="h-full overflow-y-auto p-5" style={{ backgroundColor: "var(--color-bg)" }}>
      <div className="mx-auto flex max-w-6xl flex-col gap-5">
        <header className="flex flex-col gap-3 md:flex-row md:items-start md:justify-between">
          <div className="min-w-0">
            <button
              type="button"
              onClick={() => navigate("/tasks")}
              className="mb-3 inline-flex items-center gap-1.5 rounded px-2 py-1 text-xs font-medium"
              style={{
                border: "1px solid var(--color-border)",
                backgroundColor: "var(--color-surface)",
                color: "var(--color-text-secondary)",
                cursor: "pointer",
              }}
            >
              <ArrowDown size={13} className="rotate-90" />
              {t("tasks.backToTasks")}
            </button>
            <h1 className="m-0 text-2xl font-semibold" style={{ color: "var(--color-text)" }}>
              {t("tasks.traceTitle")}
            </h1>
            <p className="m-0 mt-1 break-all text-sm" style={{ color: "var(--color-text-tertiary)" }}>
              {taskId}
            </p>
          </div>
          <button
            type="button"
            onClick={() => void traceQuery.refetch()}
            className="inline-flex h-9 items-center justify-center gap-2 rounded-md px-3 text-sm font-medium"
            style={{
              border: "1px solid var(--color-border)",
              backgroundColor: "var(--color-surface)",
              color: "var(--color-text)",
              cursor: "pointer",
            }}
          >
            <RefreshCw size={15} />
            {t("tasks.refresh")}
          </button>
        </header>

        {traceQuery.isLoading ? (
          <LoadingTrace />
        ) : traceQuery.isError ? (
          <StateBox tone="error" text={t("tasks.loadFailed")} />
        ) : trace ? (
          <TraceBody trace={trace} />
        ) : (
          <StateBox tone="neutral" text={t("tasks.emptyTrace")} />
        )}
      </div>
    </section>
  );
}

function TraceBody({ trace }: { trace: TaskTrace }) {
  const { t } = useI18n();
  const task = trace.task;
  const recentEvents = useMemo(() => trace.events.slice(-30), [trace.events]);
  const recentMessages = useMemo(() => trace.messages.slice(-20), [trace.messages]);

  return (
    <>
      <div className="grid gap-3 md:grid-cols-4">
        <Meta label={t("tasks.taskState")} value={trace.state.task || task.state} tone={stateTone(trace.state.task || task.state)} />
        <Meta label={t("tasks.turnState")} value={trace.state.turn || "-"} tone={stateTone(trace.state.turn)} />
        <Meta label={t("tasks.sessionState")} value={trace.state.session || "-"} tone={stateTone(trace.state.session)} />
        <Meta label={t("tasks.role")} value={trace.role.id || task.role_id || "-"} />
      </div>

      <Section title={t("tasks.linkage")}>
        <div className="grid gap-3 text-xs md:grid-cols-2" style={{ color: "var(--color-text-secondary)" }}>
          <Field label={t("tasks.parentSession")} value={trace.parent.session_id || task.parent_session_id || "-"} />
          <Field label={t("tasks.parentTurn")} value={trace.parent.turn_id || task.parent_turn_id || "-"} />
          <Field label={t("tasks.roleSession")} value={trace.role.session_id || task.session_id || "-"} />
          <Field label={t("tasks.roleTurn")} value={trace.role.turn_id || task.turn_id || "-"} />
          <Field label={t("tasks.reportSession")} value={task.report_session_id || "-"} />
          <Field label={t("tasks.outputPath")} value={task.output_path || "-"} />
        </div>
      </Section>

      <Section title={t("tasks.prompt")}>
        <Pre text={task.prompt || "-"} />
      </Section>

      {task.final_text && (
        <Section title={t("tasks.finalText")}>
          <div className="rounded-md p-4" style={{ backgroundColor: "var(--color-bg-elevated)", border: "1px solid var(--color-border)" }}>
            <MarkdownRenderer content={task.final_text} />
          </div>
        </Section>
      )}

      <div className="grid gap-5 xl:grid-cols-2">
        <Section title={t("tasks.events")}>
          {recentEvents.length ? (
            <div className="flex flex-col gap-2">
              {recentEvents.map((event) => (
                <EventRow key={`${event.seq}-${event.id}`} event={event} />
              ))}
            </div>
          ) : (
            <StateBox tone="neutral" text={t("tasks.noEvents")} />
          )}
        </Section>

        <Section title={t("tasks.messages")}>
          {recentMessages.length ? (
            <div className="flex flex-col gap-2">
              {recentMessages.map((message) => (
                <MessageRow key={`${message.position}-${message.role}-${message.tool_call_id ?? ""}`} message={message} />
              ))}
            </div>
          ) : (
            <StateBox tone="neutral" text={t("tasks.noMessages")} />
          )}
        </Section>
      </div>

      <Section title={t("tasks.reports")}>
        {trace.reports.length ? (
          <div className="flex flex-col gap-2">
            {trace.reports.map((report) => (
              <ReportSummary key={report.id} report={report} />
            ))}
          </div>
        ) : (
          <StateBox tone="neutral" text={t("tasks.noReports")} />
        )}
      </Section>

      <Section title={t("tasks.logPreview")}>
        {trace.log_preview ? (
          <>
            <Pre text={trace.log_preview} />
            <div className="mt-2 text-xs" style={{ color: "var(--color-text-tertiary)" }}>
              {formatBytes(trace.log_bytes ?? 0)}
              {trace.log_truncated ? ` · ${t("tasks.truncated")}` : ""}
            </div>
          </>
        ) : (
          <StateBox tone="neutral" text={t("tasks.noLog")} />
        )}
      </Section>
    </>
  );
}

function Section({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <section className="flex flex-col gap-3">
      <h2 className="m-0 text-sm font-semibold" style={{ color: "var(--color-text)" }}>
        {title}
      </h2>
      {children}
    </section>
  );
}

function Meta({ label, value, tone = "neutral" }: { label: string; value: string; tone?: Tone }) {
  return (
    <div className="rounded-md p-3" style={{ backgroundColor: "var(--color-bg-elevated)", border: "1px solid var(--color-border)" }}>
      <div className="mb-1 text-[11px] font-medium uppercase" style={{ color: "var(--color-text-tertiary)" }}>
        {label}
      </div>
      <Badge value={value || "-"} tone={tone} />
    </div>
  );
}

function Field({ label, value }: { label: string; value: string }) {
  return (
    <div className="min-w-0">
      <span style={{ color: "var(--color-text-tertiary)" }}>{label}: </span>
      <span className="break-all">{value}</span>
    </div>
  );
}

function EventRow({ event }: { event: TaskTraceEvent }) {
  const payload = formatPayload(event.payload);
  return (
    <article className="rounded-md p-3" style={{ backgroundColor: "var(--color-bg-elevated)", border: "1px solid var(--color-border)" }}>
      <div className="flex flex-wrap items-center gap-2">
        <Badge value={event.type} tone="neutral" />
        <span className="text-[11px]" style={{ color: "var(--color-text-tertiary)" }}>
          #{event.seq} · {formatDate(event.time)}
        </span>
      </div>
      {payload && <Pre text={payload} compact />}
    </article>
  );
}

function MessageRow({ message }: { message: TaskTraceMessage }) {
  return (
    <article className="rounded-md p-3" style={{ backgroundColor: "var(--color-bg-elevated)", border: "1px solid var(--color-border)" }}>
      <div className="mb-2 flex flex-wrap items-center gap-2">
        <Badge value={message.role} tone={message.role === "assistant" ? "success" : message.role === "tool" ? "warning" : "neutral"} />
        <span className="text-[11px]" style={{ color: "var(--color-text-tertiary)" }}>
          {message.position} · {formatDate(message.created_at)}
        </span>
      </div>
      <Pre text={message.content || "-"} compact />
    </article>
  );
}

function ReportSummary({ report }: { report: Report }) {
  return (
    <article className="rounded-md p-3" style={{ backgroundColor: "var(--color-bg-elevated)", border: "1px solid var(--color-border)" }}>
      <div className="flex flex-wrap items-center gap-2">
        <h3 className="m-0 text-sm font-semibold" style={{ color: "var(--color-text)" }}>
          {report.title}
        </h3>
        <Badge value={report.status} tone={stateTone(report.status)} />
        <Badge value={report.severity} tone={severityTone(report.severity)} />
      </div>
      <p className="m-0 mt-2 text-xs leading-5" style={{ color: "var(--color-text-secondary)" }}>
        {report.summary}
      </p>
    </article>
  );
}

function Pre({ text, compact = false }: { text: string; compact?: boolean }) {
  return (
    <pre
      className={`m-0 overflow-x-auto whitespace-pre-wrap break-words rounded-md ${compact ? "mt-2 p-2 text-[11px]" : "p-4 text-xs"} leading-5`}
      style={{
        backgroundColor: "var(--color-bg-elevated)",
        border: "1px solid var(--color-border)",
        color: "var(--color-text-secondary)",
        fontFamily: "var(--font-mono)",
      }}
    >
      {text}
    </pre>
  );
}

type Tone = "neutral" | "success" | "warning" | "error";

function Badge({ value, tone }: { value: string; tone: Tone }) {
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

function StateBox({ text, tone }: { text: string; tone: "neutral" | "error" }) {
  return (
    <div
      className="rounded-md p-4 text-sm"
      style={{
        backgroundColor: tone === "error" ? "var(--color-error-dim)" : "var(--color-bg-elevated)",
        border: tone === "error" ? "1px solid rgba(239,68,68,0.2)" : "1px solid var(--color-border)",
        color: tone === "error" ? "var(--color-error)" : "var(--color-text-tertiary)",
      }}
    >
      {text}
    </div>
  );
}

function LoadingTrace() {
  return (
    <div className="flex flex-col gap-3">
      {[0, 1, 2, 3].map((item) => (
        <div key={item} className="animate-shimmer rounded-md" style={{ height: item === 0 ? 72 : 120, backgroundColor: "var(--color-surface)" }} />
      ))}
    </div>
  );
}

function stateTone(value?: string): Tone {
  if (value === "completed" || value === "idle" || value === "success") {
    return "success";
  }
  if (value === "pending" || value === "running") {
    return "warning";
  }
  if (value === "failed" || value === "cancelled" || value === "error" || value === "failure") {
    return "error";
  }
  return "neutral";
}

function severityTone(value: string): Tone {
  if (value === "critical" || value === "error" || value === "high") {
    return "error";
  }
  if (value === "warning" || value === "medium") {
    return "warning";
  }
  if (value === "info" || value === "low") {
    return "success";
  }
  return "neutral";
}

function formatPayload(payload: string): string {
  if (!payload) {
    return "";
  }
  try {
    return JSON.stringify(JSON.parse(payload), null, 2);
  } catch {
    return payload;
  }
}

function formatDate(value: string): string {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return value;
  }
  return date.toLocaleString();
}

function formatBytes(value: number): string {
  if (!value) {
    return "0 B";
  }
  if (value < 1024) {
    return `${value} B`;
  }
  if (value < 1024 * 1024) {
    return `${(value / 1024).toFixed(1)} KB`;
  }
  return `${(value / (1024 * 1024)).toFixed(1)} MB`;
}
