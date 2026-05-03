import { useMemo, useState } from "react";
import type { ReactNode } from "react";
import { useNavigate } from "react-router-dom";
import { useCancelTask, useTasks } from "../api/queries";
import type { Task } from "../api/types";
import { RefreshCw, X } from "../components/Icon";
import { useI18n } from "../i18n";

interface TasksPageProps {
  workspaceRoot: string | null;
}

type TaskFilter = "all" | "active" | "failed" | "completed";
type Tone = "neutral" | "success" | "warning" | "error";

const filterState: Record<TaskFilter, string | undefined> = {
  all: undefined,
  active: "pending,running",
  failed: "failed,cancelled",
  completed: "completed",
};

export function TasksPage({ workspaceRoot }: TasksPageProps) {
  const { t } = useI18n();
  const [filter, setFilter] = useState<TaskFilter>("all");
  const tasksQuery = useTasks(workspaceRoot, {
    state: filterState[filter],
    limit: 200,
  });
  const cancelTask = useCancelTask();
  const tasks = tasksQuery.data ?? [];
  const counts = useMemo(() => countStates(tasks), [tasks]);

  return (
    <section className="h-full overflow-y-auto p-5" style={{ backgroundColor: "var(--color-bg)" }}>
      <div className="mx-auto flex max-w-6xl flex-col gap-5">
        <header className="flex flex-col gap-3 md:flex-row md:items-end md:justify-between">
          <div>
            <h1 className="m-0 text-2xl font-semibold" style={{ color: "var(--color-text)" }}>
              {t("tasks.title")}
            </h1>
            <p className="m-0 mt-1 text-sm" style={{ color: "var(--color-text-tertiary)" }}>
              {t("tasks.subtitle")}
            </p>
          </div>
          <button
            type="button"
            onClick={() => void tasksQuery.refetch()}
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

        <div className="grid gap-3 md:grid-cols-4">
          <Meta label={t("tasks.active")} value={String(counts.active)} tone={counts.active ? "warning" : "neutral"} />
          <Meta label={t("tasks.completed")} value={String(counts.completed)} tone="success" />
          <Meta label={t("tasks.failed")} value={String(counts.failed)} tone={counts.failed ? "error" : "neutral"} />
          <Meta label={t("tasks.total")} value={String(tasks.length)} tone="neutral" />
        </div>

        <div className="flex flex-wrap gap-2">
          {(["all", "active", "failed", "completed"] as TaskFilter[]).map((item) => (
            <button
              key={item}
              type="button"
              onClick={() => setFilter(item)}
              className="rounded-md px-3 py-1.5 text-xs font-medium"
              style={{
                border: filter === item ? "1px solid var(--color-accent)" : "1px solid var(--color-border)",
                backgroundColor: filter === item ? "var(--color-accent-dim)" : "var(--color-surface)",
                color: filter === item ? "var(--color-accent)" : "var(--color-text-secondary)",
                cursor: "pointer",
              }}
            >
              {t(`tasks.filters.${item}`)}
            </button>
          ))}
        </div>

        {!workspaceRoot ? (
          <StateBox tone="neutral" text={t("tasks.noWorkspace")} />
        ) : tasksQuery.isLoading ? (
          <LoadingRows />
        ) : tasksQuery.isError ? (
          <StateBox tone="error" text={t("tasks.listLoadFailed")} />
        ) : tasks.length === 0 ? (
          <StateBox tone="neutral" text={t("tasks.emptyList")} />
        ) : (
          <div className="overflow-hidden rounded-md" style={{ border: "1px solid var(--color-border)" }}>
            {tasks.map((task) => (
              <TaskRow
                key={task.id}
                task={task}
                isCancelling={cancelTask.isPending}
                onCancel={() => cancelTask.mutate(task.id)}
              />
            ))}
          </div>
        )}
      </div>
    </section>
  );
}

function TaskRow({
  task,
  isCancelling,
  onCancel,
}: {
  task: Task;
  isCancelling: boolean;
  onCancel: () => void;
}) {
  const { t } = useI18n();
  const navigate = useNavigate();
  const canCancel = task.state === "pending" || task.state === "running";

  return (
    <article
      role="button"
      tabIndex={0}
      onClick={() => navigate(`/tasks/${encodeURIComponent(task.id)}`)}
      onKeyDown={(event) => {
        if (event.key === "Enter" || event.key === " ") {
          event.preventDefault();
          navigate(`/tasks/${encodeURIComponent(task.id)}`);
        }
      }}
      className="grid cursor-pointer gap-3 p-4 md:grid-cols-[minmax(0,1fr)_120px_150px_110px]"
      style={{
        backgroundColor: "var(--color-bg-elevated)",
        borderBottom: "1px solid var(--color-border)",
      }}
    >
      <div className="min-w-0">
        <div className="flex flex-wrap items-center gap-2">
          <h2 className="m-0 truncate text-sm font-semibold" style={{ color: "var(--color-text)" }}>
            {task.role_id || task.kind}
          </h2>
          <Badge value={task.state} tone={stateTone(task.state)} />
          <Badge value={task.kind} tone="neutral" />
        </div>
        <p className="m-0 mt-1 line-clamp-2 text-xs leading-5" style={{ color: "var(--color-text-tertiary)" }}>
          {task.final_text || task.prompt}
        </p>
        <div className="mt-2 flex flex-wrap gap-x-3 gap-y-1 text-[11px]" style={{ color: "var(--color-text-tertiary)" }}>
          <span className="break-all">{task.id}</span>
          {task.turn_id && <span className="break-all">{task.turn_id}</span>}
        </div>
      </div>

      <div className="text-xs" style={{ color: "var(--color-text-secondary)" }}>
        <ColumnLabel>{t("tasks.role")}</ColumnLabel>
        <span className="break-all">{task.role_id || "-"}</span>
      </div>

      <div className="text-xs" style={{ color: "var(--color-text-secondary)" }}>
        <ColumnLabel>{t("tasks.updated")}</ColumnLabel>
        {formatDate(task.updated_at)}
      </div>

      <div className="flex items-start justify-end">
        {canCancel && (
          <button
            type="button"
            disabled={isCancelling}
            onClick={(event) => {
              event.stopPropagation();
              onCancel();
            }}
            className="inline-flex items-center gap-1.5 rounded-md px-2.5 py-1.5 text-xs font-medium"
            style={{
              border: "1px solid rgba(239,68,68,0.35)",
              backgroundColor: "var(--color-error-dim)",
              color: "var(--color-error)",
              cursor: isCancelling ? "not-allowed" : "pointer",
              opacity: isCancelling ? 0.55 : 1,
            }}
          >
            <X size={13} />
            {t("tasks.cancel")}
          </button>
        )}
      </div>
    </article>
  );
}

function Meta({ label, value, tone }: { label: string; value: string; tone: Tone }) {
  return (
    <div className="rounded-md p-3" style={{ backgroundColor: "var(--color-bg-elevated)", border: "1px solid var(--color-border)" }}>
      <div className="mb-1 text-[11px] font-medium uppercase" style={{ color: "var(--color-text-tertiary)" }}>
        {label}
      </div>
      <Badge value={value} tone={tone} />
    </div>
  );
}

function ColumnLabel({ children }: { children: ReactNode }) {
  return (
    <div className="mb-1 text-[11px] font-medium uppercase" style={{ color: "var(--color-text-tertiary)" }}>
      {children}
    </div>
  );
}

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

function LoadingRows() {
  return (
    <div className="space-y-2">
      {[0, 1, 2].map((item) => (
        <div key={item} className="animate-shimmer rounded-md" style={{ height: 98, backgroundColor: "var(--color-surface)" }} />
      ))}
    </div>
  );
}

function countStates(tasks: Task[]) {
  return tasks.reduce(
    (counts, task) => {
      if (task.state === "pending" || task.state === "running") {
        counts.active += 1;
      } else if (task.state === "completed") {
        counts.completed += 1;
      } else if (task.state === "failed" || task.state === "cancelled") {
        counts.failed += 1;
      }
      return counts;
    },
    { active: 0, completed: 0, failed: 0 },
  );
}

function stateTone(value?: string): Tone {
  if (value === "completed" || value === "success") {
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

function formatDate(value: string): string {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return value;
  }
  return date.toLocaleString();
}
