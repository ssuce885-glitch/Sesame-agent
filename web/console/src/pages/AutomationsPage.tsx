import { useState } from "react";
import type { ReactNode } from "react";
import { useNavigate } from "react-router-dom";
import {
  useAutomationRuns,
  useAutomations,
  useCreateAutomation,
  usePauseAutomation,
  useResumeAutomation,
  useRoles,
} from "../api/queries";
import type { Automation, AutomationRun } from "../api/types";
import { ChevronDown, ChevronRight, Plus, Save, X } from "../components/Icon";
import { useI18n } from "../i18n";

interface AutomationsPageProps {
  workspaceRoot: string | null;
}

interface AutomationFormState {
  title: string;
  goal: string;
  ownerRole: string;
  watcherPath: string;
  watcherCron: string;
}

const defaultForm: AutomationFormState = {
  title: "",
  goal: "",
  ownerRole: "",
  watcherPath: "",
  watcherCron: "@every 5m",
};

export function AutomationsPage({ workspaceRoot }: AutomationsPageProps) {
  const { t } = useI18n();
  const { data: automations = [], isLoading, isError, refetch } = useAutomations(workspaceRoot);
  const { data: roles = [] } = useRoles();
  const createAutomation = useCreateAutomation(workspaceRoot);
  const pause = usePauseAutomation(workspaceRoot);
  const resume = useResumeAutomation(workspaceRoot);
  const [creating, setCreating] = useState(false);
  const [expandedID, setExpandedID] = useState<string | null>(null);

  return (
    <section className="h-full overflow-y-auto p-5" style={{ backgroundColor: "var(--color-bg)" }}>
      <div className="mx-auto flex max-w-6xl flex-col gap-5">
        <header className="flex flex-col gap-3 md:flex-row md:items-end md:justify-between">
          <PageHeader title={t("automations.title")} subtitle={t("automations.subtitle")} />
          <button
            type="button"
            disabled={!workspaceRoot}
            onClick={() => setCreating((value) => !value)}
            className="inline-flex h-9 items-center justify-center gap-2 rounded-md px-3 text-sm font-medium"
            style={{
              border: "1px solid var(--color-accent)",
              backgroundColor: creating ? "var(--color-surface)" : "var(--color-accent)",
              color: creating ? "var(--color-text-secondary)" : "white",
              cursor: workspaceRoot ? "pointer" : "not-allowed",
              opacity: workspaceRoot ? 1 : 0.6,
            }}
          >
            {creating ? <X size={14} /> : <Plus size={14} />}
            {creating ? t("automations.cancelCreate") : t("automations.create")}
          </button>
        </header>

        {creating && workspaceRoot && (
          <AutomationForm
            workspaceRoot={workspaceRoot}
            roles={roles}
            isSubmitting={createAutomation.isPending}
            error={createAutomation.error}
            onSubmit={(automation) => {
              createAutomation.mutate(automation, {
                onSuccess: (created) => {
                  setCreating(false);
                  setExpandedID(created.id);
                },
              });
            }}
          />
        )}

        {!workspaceRoot ? (
          <EmptyState text={t("automations.noWorkspace")} />
        ) : isLoading ? (
          <LoadingRows />
        ) : isError ? (
          <ErrorState text={t("automations.loadFailed")} onRetry={() => void refetch()} />
        ) : automations.length === 0 ? (
          <EmptyState text={t("automations.empty")} />
        ) : (
          <div className="overflow-hidden rounded-md" style={{ border: "1px solid var(--color-border)" }}>
            {automations.map((automation) => (
              <AutomationRow
                key={automation.id}
                automation={automation}
                expanded={expandedID === automation.id}
                isMutating={pause.isPending || resume.isPending}
                onToggle={() => setExpandedID((current) => (current === automation.id ? null : automation.id))}
                onPause={() => void pause.mutateAsync(automation.id)}
                onResume={() => void resume.mutateAsync(automation.id)}
              />
            ))}
          </div>
        )}
      </div>
    </section>
  );
}

function AutomationForm({
  workspaceRoot,
  roles,
  isSubmitting,
  error,
  onSubmit,
}: {
  workspaceRoot: string;
  roles: Array<{ id: string; name: string }>;
  isSubmitting: boolean;
  error: unknown;
  onSubmit: (automation: Partial<Automation>) => void;
}) {
  const { t } = useI18n();
  const [form, setForm] = useState<AutomationFormState>(() => ({
    ...defaultForm,
    ownerRole: roles[0]?.id ?? "",
  }));

  function setField<K extends keyof AutomationFormState>(key: K, value: AutomationFormState[K]) {
    setForm((current) => ({ ...current, [key]: value }));
  }

  return (
    <form
      className="rounded-md p-4"
      style={{ backgroundColor: "var(--color-bg-elevated)", border: "1px solid var(--color-border)" }}
      onSubmit={(event) => {
        event.preventDefault();
        onSubmit({
          workspace_root: workspaceRoot,
          title: form.title.trim(),
          goal: form.goal.trim(),
          owner: `role:${form.ownerRole.trim()}`,
          watcher_path: form.watcherPath.trim(),
          watcher_cron: form.watcherCron.trim() || "@every 5m",
          state: "active",
        });
      }}
    >
      <div className="grid gap-4 md:grid-cols-2">
        <FormField label={t("automations.form.title")}>
          <input
            required
            value={form.title}
            onChange={(event) => setField("title", event.target.value)}
            className="h-10 w-full rounded-md px-3 text-sm"
            style={inputStyle}
          />
        </FormField>
        <FormField label={t("automations.form.owner")}>
          <select
            required
            value={form.ownerRole}
            onChange={(event) => setField("ownerRole", event.target.value)}
            className="h-10 w-full rounded-md px-3 text-sm"
            style={inputStyle}
          >
            <option value="">{t("automations.form.selectRole")}</option>
            {roles.map((role) => (
              <option key={role.id} value={role.id}>
                {role.name} ({role.id})
              </option>
            ))}
          </select>
        </FormField>
        <FormField label={t("automations.form.watcherPath")}>
          <input
            required
            value={form.watcherPath}
            onChange={(event) => setField("watcherPath", event.target.value)}
            placeholder="roles/reddit_monitor/automations/watch.sh"
            className="h-10 w-full rounded-md px-3 text-sm"
            style={inputStyle}
          />
        </FormField>
        <FormField label={t("automations.form.watcherCron")}>
          <input
            value={form.watcherCron}
            onChange={(event) => setField("watcherCron", event.target.value)}
            className="h-10 w-full rounded-md px-3 text-sm"
            style={inputStyle}
          />
        </FormField>
      </div>
      <FormField label={t("automations.form.goal")}>
        <textarea
          required
          value={form.goal}
          onChange={(event) => setField("goal", event.target.value)}
          rows={4}
          className="mt-4 w-full resize-y rounded-md p-3 text-sm leading-6"
          style={inputStyle}
        />
      </FormField>
      {error ? (
        <div className="mt-4 rounded-md p-3 text-sm" style={{ backgroundColor: "var(--color-error-dim)", border: "1px solid rgba(239,68,68,0.15)", color: "var(--color-error)" }}>
          {error instanceof Error ? error.message : String(error)}
        </div>
      ) : null}
      <div className="mt-4 flex justify-end">
        <button
          type="submit"
          disabled={isSubmitting}
          className="inline-flex h-9 items-center justify-center gap-2 rounded-md px-3 text-sm font-medium"
          style={{
            border: "1px solid var(--color-accent)",
            backgroundColor: "var(--color-accent)",
            color: "white",
            cursor: isSubmitting ? "default" : "pointer",
            opacity: isSubmitting ? 0.7 : 1,
          }}
        >
          <Save size={14} />
          {isSubmitting ? t("automations.saving") : t("automations.save")}
        </button>
      </div>
    </form>
  );
}

function AutomationRow({
  automation,
  expanded,
  isMutating,
  onToggle,
  onPause,
  onResume,
}: {
  automation: Automation;
  expanded: boolean;
  isMutating: boolean;
  onToggle: () => void;
  onPause: () => void;
  onResume: () => void;
}) {
  const { t } = useI18n();
  const isPaused = automation.state === "paused";
  return (
    <article style={{ backgroundColor: "var(--color-bg-elevated)", borderBottom: "1px solid var(--color-border)" }}>
      <div className="grid gap-3 p-4 md:grid-cols-[1fr_120px_210px]">
        <button type="button" onClick={onToggle} className="min-w-0 text-left" style={{ background: "transparent", border: "none", padding: 0, cursor: "pointer" }}>
          <div className="flex items-center gap-2">
            {expanded ? <ChevronDown size={14} color="var(--color-text-tertiary)" /> : <ChevronRight size={14} color="var(--color-text-tertiary)" />}
            <h2 className="m-0 truncate text-sm font-semibold" style={{ color: "var(--color-text)" }}>
              {automation.title}
            </h2>
            <StatusBadge value={automation.state} />
          </div>
          <p className="m-0 mt-1 truncate text-xs" style={{ color: "var(--color-text-tertiary)" }}>
            {automation.goal}
          </p>
          <div className="mt-3 grid gap-2 text-xs md:grid-cols-4" style={{ color: "var(--color-text-secondary)" }}>
            <Field label="Owner" value={automation.owner} />
            <Field label="Workflow" value={automation.workflow_id ?? ""} />
            <Field label="Path" value={automation.watcher_path} />
            <Field label="Cron" value={automation.watcher_cron} />
          </div>
        </button>
        <div className="text-xs" style={{ color: "var(--color-text-tertiary)" }}>
          {t("automations.updated")}
          <div className="mt-1" style={{ color: "var(--color-text-secondary)" }}>
            {formatDate(automation.updated_at)}
          </div>
        </div>
        <div className="flex items-start justify-end gap-2">
          <button
            type="button"
            disabled={isMutating}
            onClick={isPaused ? onResume : onPause}
            className="rounded-md px-3 py-1.5 text-xs font-medium"
            style={{
              backgroundColor: isPaused ? "var(--color-accent)" : "var(--color-surface)",
              border: isPaused ? "1px solid var(--color-accent)" : "1px solid var(--color-border)",
              color: isPaused ? "#fff" : "var(--color-text-secondary)",
              cursor: isMutating ? "not-allowed" : "pointer",
              opacity: isMutating ? 0.55 : 1,
            }}
          >
            {isPaused ? t("automations.resume") : t("automations.pause")}
          </button>
        </div>
      </div>
      {expanded && <AutomationRuns automationID={automation.id} />}
    </article>
  );
}

function AutomationRuns({ automationID }: { automationID: string }) {
  const { t } = useI18n();
  const navigate = useNavigate();
  const runs = useAutomationRuns(automationID);
  return (
    <div className="border-t p-4" style={{ borderColor: "var(--color-border)", backgroundColor: "var(--color-bg)" }}>
      <h3 className="m-0 mb-3 text-xs font-semibold uppercase" style={{ color: "var(--color-text-tertiary)" }}>
        {t("automations.runs")}
      </h3>
      {runs.isLoading ? (
        <div className="animate-shimmer rounded-md" style={{ height: 72, backgroundColor: "var(--color-surface)" }} />
      ) : runs.isError ? (
        <EmptyState text={t("automations.runsFailed")} />
      ) : runs.data?.length ? (
        <div className="flex flex-col gap-2">
          {runs.data.map((run) => (
            <RunRow
              key={`${run.automation_id}-${run.dedupe_key}`}
              run={run}
              onOpenTask={() => navigate(`/tasks/${encodeURIComponent(run.task_id)}`)}
              onOpenWorkflow={() =>
                navigate(
                  run.workflow_run_id
                    ? `/workflows?run_id=${encodeURIComponent(run.workflow_run_id)}`
                    : "/workflows",
                )
              }
            />
          ))}
        </div>
      ) : (
        <EmptyState text={t("automations.noRuns")} />
      )}
    </div>
  );
}

function RunRow({ run, onOpenTask, onOpenWorkflow }: { run: AutomationRun; onOpenTask: () => void; onOpenWorkflow: () => void }) {
  const linkButtonStyle = {
    border: "1px solid var(--color-border)",
    backgroundColor: "var(--color-surface)",
    color: "var(--color-accent)",
    cursor: "pointer",
  } as const;
  return (
    <div className="grid gap-3 rounded-md p-3 md:grid-cols-[1fr_130px_180px]" style={{ backgroundColor: "var(--color-bg-elevated)", border: "1px solid var(--color-border)" }}>
      <div className="min-w-0">
        <div className="flex flex-wrap items-center gap-2">
          <StatusBadge value={run.status || "unknown"} />
          <span className="text-[11px]" style={{ color: "var(--color-text-tertiary)" }}>
            {run.dedupe_key}
          </span>
        </div>
        <p className="m-0 mt-1 text-xs leading-5" style={{ color: "var(--color-text-secondary)" }}>
          {run.summary || "-"}
        </p>
      </div>
      <div className="text-xs" style={{ color: "var(--color-text-secondary)" }}>
        {formatDate(run.created_at)}
      </div>
      <div className="flex flex-col items-end gap-2">
        {run.workflow_run_id ? (
          <button
            type="button"
            onClick={onOpenWorkflow}
            className="rounded-md px-2.5 py-1.5 text-xs font-medium"
            style={linkButtonStyle}
          >
            {run.workflow_run_id}
          </button>
        ) : null}
        {run.task_id ? (
          <button
            type="button"
            onClick={onOpenTask}
            className="rounded-md px-2.5 py-1.5 text-xs font-medium"
            style={linkButtonStyle}
          >
            {run.task_id}
          </button>
        ) : !run.workflow_run_id ? (
          <span className="text-xs" style={{ color: "var(--color-text-tertiary)" }}>-</span>
        ) : null}
      </div>
    </div>
  );
}

function PageHeader({ title, subtitle }: { title: string; subtitle: string }) {
  return (
    <div>
      <h1 className="m-0 text-2xl font-semibold" style={{ color: "var(--color-text)" }}>
        {title}
      </h1>
      <p className="m-0 mt-1 text-sm" style={{ color: "var(--color-text-tertiary)" }}>
        {subtitle}
      </p>
    </div>
  );
}

function FormField({ label, children }: { label: string; children: ReactNode }) {
  return (
    <label className="flex flex-col gap-1.5">
      <span className="text-xs font-semibold uppercase" style={{ color: "var(--color-text-tertiary)" }}>
        {label}
      </span>
      {children}
    </label>
  );
}

function Field({ label, value }: { label: string; value: string }) {
  return (
    <div className="min-w-0">
      <span style={{ color: "var(--color-text-tertiary)" }}>{label}: </span>
      <span className="break-all">{value || "-"}</span>
    </div>
  );
}

function StatusBadge({ value }: { value: string }) {
  const tone = statusTone(value);
  const styles = {
    neutral: ["var(--color-surface)", "var(--color-text-tertiary)"],
    success: ["var(--color-success-dim)", "var(--color-success)"],
    warning: ["var(--color-warning-dim)", "var(--color-warning)"],
    error: ["var(--color-error-dim)", "var(--color-error)"],
  }[tone];
  return (
    <span className="rounded px-1.5 py-0.5 text-[11px] font-medium" style={{ backgroundColor: styles[0], color: styles[1] }}>
      {value}
    </span>
  );
}

function EmptyState({ text }: { text: string }) {
  return <div className="rounded-md p-4 text-sm" style={{ backgroundColor: "var(--color-bg-elevated)", color: "var(--color-text-tertiary)", border: "1px solid var(--color-border)" }}>{text}</div>;
}

function ErrorState({ text, onRetry }: { text: string; onRetry: () => void }) {
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
        <div key={item} className="animate-shimmer rounded-md" style={{ height: 88, backgroundColor: "var(--color-surface)" }} />
      ))}
    </div>
  );
}

function statusTone(value: string): "neutral" | "success" | "warning" | "error" {
  if (value.startsWith("workflow:")) {
    return statusTone(value.slice("workflow:".length));
  }
  if (value === "active" || value === "success" || value === "completed") {
    return "success";
  }
  if (value === "needs_agent" || value === "queued" || value === "running" || value === "waiting_approval") {
    return "warning";
  }
  if (value === "error" || value === "failed" || value === "failure" || value === "blocked" || value === "interrupted") {
    return "error";
  }
  return "neutral";
}

function formatDate(value: string) {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return value;
  }
  return date.toLocaleString();
}

const inputStyle = {
  backgroundColor: "var(--color-bg)",
  border: "1px solid var(--color-border)",
  color: "var(--color-text)",
} as const;
