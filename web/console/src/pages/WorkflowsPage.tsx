import { useEffect, useMemo, useState } from "react";
import type { ReactNode } from "react";
import { useSearchParams } from "react-router-dom";
import {
  useCreateWorkflow,
  useRoles,
  useTriggerWorkflow,
  useUpdateWorkflow,
  useWorkflowRun,
  useWorkflowRuns,
  useWorkflows,
} from "../api/queries";
import type { Workflow, WorkflowRun } from "../api/types";
import { ChevronRight, Play, Plus, RefreshCw, Save } from "../components/Icon";
import { useI18n } from "../i18n";

interface WorkflowsPageProps {
  workspaceRoot: string | null;
}

interface WorkflowFormState {
  name: string;
  trigger: string;
  ownerRole: string;
  prompt: string;
  steps: string;
}

interface TraceEventSummary {
  event: string;
  state: string;
  kind: string;
  taskID: string;
  approvalID: string;
  message: string;
  time: string;
}

interface TraceEventRow extends TraceEventSummary {
  key: string;
}

type ParsedTrace =
  | { mode: "structured"; events: TraceEventRow[] }
  | { mode: "fallback"; summary: string };

type Tone = "neutral" | "success" | "warning" | "error";
type DetailMode = "create" | "edit";

const MANUAL_TRIGGER = "manual";
const MANUAL_TRIGGER_REF = "manual:web";
const MAX_TRACE_EVENTS = 200;
const TRACE_TABLE_COLUMNS = [
  { id: "event", titleKey: "workflows.run.event" },
  { id: "state", titleKey: "workflows.run.state" },
  { id: "kind", titleKey: "workflows.run.kind" },
  { id: "taskId", titleKey: "workflows.run.taskId" },
  { id: "approvalId", titleKey: "workflows.run.approvalId" },
  { id: "message", titleKey: "workflows.run.message" },
  { id: "time", titleKey: "workflows.run.time" },
] as const;

export function WorkflowsPage({ workspaceRoot }: WorkflowsPageProps) {
  const { t } = useI18n();
  const defaultPrompt = t("workflows.form.defaultPrompt");
  const [searchParams] = useSearchParams();
  const linkedRunID = searchParams.get("run_id");
  const { data: roles = [] } = useRoles();
  const workflowsQuery = useWorkflows(workspaceRoot);
  const workflows = workflowsQuery.data ?? [];
  const latestRunsQuery = useWorkflowRuns(workspaceRoot, { limit: 200 });
  const createWorkflow = useCreateWorkflow(workspaceRoot);
  const updateWorkflow = useUpdateWorkflow(workspaceRoot);
  const triggerWorkflow = useTriggerWorkflow(workspaceRoot);
  const [detailMode, setDetailMode] = useState<DetailMode>("edit");
  const [selectedWorkflowID, setSelectedWorkflowID] = useState<string | null>(null);
  const [stepsDirty, setStepsDirty] = useState(false);
  const [validationError, setValidationError] = useState<string | null>(null);
  const [form, setForm] = useState<WorkflowFormState>(() => createDraftForm("", defaultPrompt));
  const linkedRunQuery = useWorkflowRun(linkedRunID);

  const selectedWorkflow =
    detailMode === "edit" ? workflows.find((workflow) => workflow.id === selectedWorkflowID) ?? null : null;
  const selectedRunsQuery = useWorkflowRuns(
    workspaceRoot && selectedWorkflowID ? workspaceRoot : null,
    selectedWorkflowID ? { workflow_id: selectedWorkflowID, limit: 20 } : {},
  );
  const selectedRuns = useMemo(() => {
    const items = selectedRunsQuery.data ?? [];
    const linkedRun = linkedRunQuery.data;
    if (!linkedRun || linkedRun.workflow_id !== selectedWorkflowID) {
      return items;
    }
    if (items.some((run) => run.id === linkedRun.id)) {
      return items;
    }
    return [linkedRun, ...items];
  }, [linkedRunQuery.data, selectedRunsQuery.data, selectedWorkflowID]);

  const latestRunByWorkflow = useMemo(() => {
    const mapping = new Map<string, WorkflowRun>();
    for (const run of latestRunsQuery.data ?? []) {
      if (!mapping.has(run.workflow_id)) {
        mapping.set(run.workflow_id, run);
      }
    }
    return mapping;
  }, [latestRunsQuery.data]);

  useEffect(() => {
    if (detailMode === "create") {
      return;
    }
    if (!workflows.length) {
      setSelectedWorkflowID(null);
      setDetailMode("create");
      setForm(createDraftForm(roles[0]?.id ?? "", defaultPrompt));
      setStepsDirty(false);
      return;
    }
    if (!selectedWorkflowID || !workflows.some((workflow) => workflow.id === selectedWorkflowID)) {
      setSelectedWorkflowID(workflows[0].id);
    }
  }, [defaultPrompt, detailMode, roles, selectedWorkflowID, workflows]);

  useEffect(() => {
    if (detailMode !== "edit" || !selectedWorkflow) {
      return;
    }
    setForm(workflowToFormState(selectedWorkflow, defaultPrompt));
    setStepsDirty(false);
    setValidationError(null);
  }, [defaultPrompt, detailMode, selectedWorkflow]);

  useEffect(() => {
    const linkedWorkflowID = detailMode === "edit" ? linkedRunQuery.data?.workflow_id ?? null : null;
    if (
      !linkedWorkflowID ||
      selectedWorkflowID === linkedWorkflowID ||
      !workflows.some((workflow) => workflow.id === linkedWorkflowID)
    ) {
      return;
    }
    setSelectedWorkflowID(linkedWorkflowID);
  }, [detailMode, linkedRunQuery.data, selectedWorkflowID, workflows]);

  useEffect(() => {
    if (detailMode !== "create" || form.ownerRole || !roles[0]?.id) {
      return;
    }
    setForm((current) => {
      const ownerRole = roles[0]?.id ?? "";
      return {
        ...current,
        ownerRole,
        steps: stepsDirty ? current.steps : buildStepsTemplate(ownerRole, current.prompt),
      };
    });
  }, [detailMode, form.ownerRole, roles, stepsDirty]);

  const mutationError = createWorkflow.error ?? updateWorkflow.error;
  const isSaving = createWorkflow.isPending || updateWorkflow.isPending;
  const canTrigger = selectedWorkflow ? isManualTrigger(selectedWorkflow.trigger) : false;

  function startCreate() {
    setDetailMode("create");
    setSelectedWorkflowID(null);
    setForm(createDraftForm(roles[0]?.id ?? "", defaultPrompt));
    setStepsDirty(false);
    setValidationError(null);
  }

  function selectWorkflow(workflowID: string) {
    setDetailMode("edit");
    setSelectedWorkflowID(workflowID);
    setValidationError(null);
  }

  function updateForm<K extends keyof WorkflowFormState>(key: K, value: WorkflowFormState[K]) {
    setForm((current) => {
      const next = { ...current, [key]: value };
      if (detailMode === "create" && !stepsDirty && (key === "ownerRole" || key === "prompt")) {
        next.steps = buildStepsTemplate(
          key === "ownerRole" ? String(value) : current.ownerRole,
          key === "prompt" ? String(value) : current.prompt,
        );
      }
      return next;
    });
    setValidationError(null);
  }

  function regenerateSteps() {
    setForm((current) => ({
      ...current,
      steps: buildStepsTemplate(current.ownerRole, current.prompt),
    }));
    setStepsDirty(false);
    setValidationError(null);
  }

  function saveWorkflow() {
    if (!workspaceRoot) {
      return;
    }
    if (!form.name.trim() || !form.steps.trim()) {
      setValidationError(t("workflows.validation.required"));
      return;
    }
    setValidationError(null);
    const payload: Partial<Workflow> = {
      workspace_root: workspaceRoot,
      name: form.name.trim(),
      trigger: form.trigger.trim() || MANUAL_TRIGGER,
      owner_role: form.ownerRole.trim(),
      steps: form.steps.trim(),
    };

    if (detailMode === "create") {
      createWorkflow.mutate(payload, {
        onSuccess: (workflow) => {
          setDetailMode("edit");
          setSelectedWorkflowID(workflow.id);
          setForm(workflowToFormState(workflow, defaultPrompt));
          setStepsDirty(false);
        },
      });
      return;
    }

    if (!selectedWorkflowID) {
      return;
    }
    updateWorkflow.mutate(
      { id: selectedWorkflowID, workflow: payload },
      {
        onSuccess: (workflow) => {
          setForm(workflowToFormState(workflow, defaultPrompt));
          setStepsDirty(false);
        },
      },
    );
  }

  function runWorkflow() {
    if (!selectedWorkflowID || !canTrigger) {
      return;
    }
    triggerWorkflow.mutate(
      {
        workflowId: selectedWorkflowID,
        input: { trigger_ref: MANUAL_TRIGGER_REF },
      },
      {
        onSuccess: () => {
          void selectedRunsQuery.refetch();
          void latestRunsQuery.refetch();
        },
      },
    );
  }

  return (
    <div className="flex h-full min-h-0 flex-col overflow-hidden lg:flex-row">
      <section
        className="flex w-full min-h-0 flex-col lg:w-[360px] lg:min-w-[360px]"
        style={{
          backgroundColor: "var(--color-bg-elevated)",
          borderRight: "1px solid var(--color-border)",
          borderBottom: "1px solid var(--color-border)",
        }}
      >
        <header className="flex items-center justify-between gap-3 px-4 py-4">
          <div className="min-w-0">
            <h1 className="m-0 text-lg font-semibold" style={{ color: "var(--color-text)" }}>
              {t("workflows.title")}
            </h1>
            <p className="m-0 mt-1 text-xs" style={{ color: "var(--color-text-tertiary)" }}>
              {t("workflows.subtitle")}
            </p>
          </div>
          <button
            type="button"
            onClick={startCreate}
            disabled={!workspaceRoot}
            className="inline-flex h-9 items-center justify-center gap-1.5 rounded-md px-3 text-sm font-medium"
            style={{
              border: "1px solid var(--color-accent)",
              backgroundColor: detailMode === "create" ? "var(--color-accent-dim)" : "var(--color-accent)",
              color: detailMode === "create" ? "var(--color-accent)" : "white",
              cursor: workspaceRoot ? "pointer" : "not-allowed",
              opacity: workspaceRoot ? 1 : 0.65,
            }}
          >
            <Plus size={14} />
            {t("workflows.create")}
          </button>
        </header>

        {!workspaceRoot ? (
          <StateBox tone="neutral" text={t("workflows.noWorkspace")} className="mx-4 mb-4" />
        ) : workflowsQuery.isLoading ? (
          <LoadingList className="px-4 pb-4" />
        ) : workflowsQuery.isError ? (
          <div className="px-4 pb-4">
            <ErrorBox
              text={t("workflows.loadFailed")}
              onRetry={() => {
                void workflowsQuery.refetch();
              }}
            />
          </div>
        ) : workflows.length === 0 ? (
          <StateBox tone="neutral" text={t("workflows.empty")} className="mx-4 mb-4" />
        ) : (
          <div className="min-h-0 flex-1 overflow-y-auto" style={{ borderTop: "1px solid var(--color-border)" }}>
            {workflows.map((workflow) => (
              <WorkflowRow
                key={workflow.id}
                workflow={workflow}
                latestRun={latestRunByWorkflow.get(workflow.id) ?? null}
                selected={selectedWorkflowID === workflow.id && detailMode === "edit"}
                onSelect={() => selectWorkflow(workflow.id)}
              />
            ))}
          </div>
        )}
      </section>

      <section className="flex-1 overflow-y-auto p-5" style={{ backgroundColor: "var(--color-bg)" }}>
        <div className="mx-auto flex max-w-5xl flex-col gap-5">
          <header className="flex flex-col gap-3 md:flex-row md:items-start md:justify-between">
            <div className="min-w-0">
              <div className="text-xs font-medium uppercase" style={{ color: "var(--color-text-tertiary)" }}>
                {detailMode === "create" ? t("workflows.draft") : selectedWorkflow?.id ?? t("workflows.draft")}
              </div>
              <h2 className="m-0 mt-1 text-2xl font-semibold" style={{ color: "var(--color-text)" }}>
                {detailMode === "create" ? t("workflows.createTitle") : form.name || t("workflows.untitled")}
              </h2>
              <p className="m-0 mt-2 text-sm leading-6" style={{ color: "var(--color-text-secondary)" }}>
                {t("workflows.detailSubtitle")}
              </p>
            </div>
            <div className="flex flex-wrap gap-2">
              {detailMode === "edit" && (
                <button
                  type="button"
                  onClick={runWorkflow}
                  disabled={!canTrigger || triggerWorkflow.isPending}
                  className="inline-flex h-9 items-center justify-center gap-2 rounded-md px-3 text-sm font-medium"
                  style={{
                    border: "1px solid var(--color-accent)",
                    backgroundColor: "var(--color-accent)",
                    color: "white",
                    cursor: !canTrigger || triggerWorkflow.isPending ? "not-allowed" : "pointer",
                    opacity: !canTrigger || triggerWorkflow.isPending ? 0.6 : 1,
                  }}
                >
                  <Play size={14} color="currentColor" />
                  {triggerWorkflow.isPending ? t("workflows.triggering") : t("workflows.trigger")}
                </button>
              )}
              <button
                type="button"
                onClick={saveWorkflow}
                disabled={!workspaceRoot || isSaving}
                className="inline-flex h-9 items-center justify-center gap-2 rounded-md px-3 text-sm font-medium"
                style={{
                  border: "1px solid var(--color-border)",
                  backgroundColor: "var(--color-surface)",
                  color: "var(--color-text)",
                  cursor: !workspaceRoot || isSaving ? "not-allowed" : "pointer",
                  opacity: !workspaceRoot || isSaving ? 0.6 : 1,
                }}
              >
                <Save size={14} color="currentColor" />
                {isSaving ? t("workflows.saving") : t("workflows.save")}
              </button>
            </div>
          </header>

          <div className="grid gap-5 xl:grid-cols-[minmax(0,1.4fr)_minmax(320px,0.9fr)]">
            <section className="min-w-0 rounded-md p-4" style={{ backgroundColor: "var(--color-bg-elevated)", border: "1px solid var(--color-border)" }}>
              <div className="grid gap-4 md:grid-cols-2">
                <Field label={t("workflows.form.name")} htmlFor="workflow-name">
                  <input
                    id="workflow-name"
                    value={form.name}
                    onChange={(event) => updateForm("name", event.target.value)}
                    className="h-10 w-full rounded-md px-3 text-sm"
                    style={inputStyle}
                  />
                </Field>
                <Field label={t("workflows.form.trigger")} htmlFor="workflow-trigger">
                  <select
                    id="workflow-trigger"
                    value={form.trigger}
                    onChange={(event) => updateForm("trigger", event.target.value)}
                    className="h-10 w-full rounded-md px-3 text-sm"
                    style={inputStyle}
                  >
                    {["manual", "schedule", "watcher", "webhook", "file_change"].map((trigger) => (
                      <option key={trigger} value={trigger}>
                        {t(`workflows.form.triggerOptions.${trigger}`)}
                      </option>
                    ))}
                  </select>
                </Field>
                <Field label={t("workflows.form.ownerRole")} htmlFor="workflow-owner-role">
                  <select
                    id="workflow-owner-role"
                    value={form.ownerRole}
                    onChange={(event) => updateForm("ownerRole", event.target.value)}
                    className="h-10 w-full rounded-md px-3 text-sm"
                    style={inputStyle}
                  >
                    <option value="">{t("workflows.form.selectRole")}</option>
                    {roles.map((role) => (
                      <option key={role.id} value={role.id}>
                        {role.name} ({role.id})
                      </option>
                    ))}
                  </select>
                </Field>
                <Field label={t("workflows.form.prompt")} htmlFor="workflow-prompt">
                  <textarea
                    id="workflow-prompt"
                    value={form.prompt}
                    onChange={(event) => updateForm("prompt", event.target.value)}
                    rows={4}
                    className="w-full resize-y rounded-md p-3 text-sm leading-6"
                    style={inputStyle}
                  />
                </Field>
              </div>

              <div className="mt-4">
                <div className="mb-2 flex items-center justify-between gap-3">
                  <label htmlFor="workflow-steps" className="text-sm font-medium" style={{ color: "var(--color-text)" }}>
                    {t("workflows.form.steps")}
                  </label>
                  <button
                    type="button"
                    onClick={regenerateSteps}
                    className="inline-flex h-8 items-center justify-center gap-1.5 rounded-md px-2.5 text-xs font-medium"
                    style={{
                      border: "1px solid var(--color-border)",
                      backgroundColor: "var(--color-surface)",
                      color: "var(--color-text-secondary)",
                      cursor: "pointer",
                    }}
                  >
                    <RefreshCw size={13} color="currentColor" />
                    {t("workflows.form.generateTemplate")}
                  </button>
                </div>
                <textarea
                  id="workflow-steps"
                  value={form.steps}
                  onChange={(event) => {
                    setStepsDirty(true);
                    updateForm("steps", event.target.value);
                  }}
                  rows={18}
                  className="w-full resize-y rounded-md p-3 text-sm leading-6"
                  style={{ ...inputStyle, fontFamily: "var(--font-mono)" }}
                />
              </div>

              {(validationError || mutationError || triggerWorkflow.error) && (
                <div
                  className="mt-4 rounded-md p-3 text-sm"
                  style={{
                    backgroundColor: "var(--color-error-dim)",
                    border: "1px solid rgba(239,68,68,0.18)",
                    color: "var(--color-error)",
                  }}
                >
                  {validationError ??
                    formatErrorMessage(mutationError) ??
                    formatErrorMessage(triggerWorkflow.error) ??
                    ""}
                </div>
              )}
            </section>

            <section className="min-w-0 rounded-md p-4" style={{ backgroundColor: "var(--color-bg-elevated)", border: "1px solid var(--color-border)" }}>
              <div className="mb-3 flex items-center justify-between gap-3">
                <div>
                  <h3 className="m-0 text-sm font-semibold" style={{ color: "var(--color-text)" }}>
                    {t("workflows.recentRuns")}
                  </h3>
                  <p className="m-0 mt-1 text-xs" style={{ color: "var(--color-text-tertiary)" }}>
                    {t("workflows.recentRunsSubtitle")}
                  </p>
                </div>
                {detailMode === "edit" && (
                  <button
                    type="button"
                    onClick={() => {
                      void selectedRunsQuery.refetch();
                    }}
                    className="inline-flex h-8 items-center justify-center gap-1.5 rounded-md px-2.5 text-xs font-medium"
                    style={{
                      border: "1px solid var(--color-border)",
                      backgroundColor: "var(--color-surface)",
                      color: "var(--color-text-secondary)",
                      cursor: "pointer",
                    }}
                  >
                    <RefreshCw size={13} color="currentColor" />
                    {t("workflows.refreshRuns")}
                  </button>
                )}
              </div>

              {detailMode === "create" ? (
                <StateBox tone="neutral" text={t("workflows.saveToViewRuns")} />
              ) : selectedRunsQuery.isLoading ? (
                <LoadingList />
              ) : selectedRunsQuery.isError ? (
                <ErrorBox
                  text={t("workflows.runsLoadFailed")}
                  onRetry={() => {
                    void selectedRunsQuery.refetch();
                  }}
                />
              ) : !selectedRuns.length ? (
                <StateBox tone="neutral" text={t("workflows.noRuns")} />
              ) : (
                <div className="overflow-hidden rounded-md" style={{ border: "1px solid var(--color-border)" }}>
                  {selectedRuns.map((run) => (
                    <RunRow key={run.id} run={run} />
                  ))}
                </div>
              )}
            </section>
          </div>
        </div>
      </section>
    </div>
  );
}

function WorkflowRow({
  workflow,
  latestRun,
  selected,
  onSelect,
}: {
  workflow: Workflow;
  latestRun: WorkflowRun | null;
  selected: boolean;
  onSelect: () => void;
}) {
  const { t } = useI18n();
  const preview = summarizeWorkflowSteps(workflow.steps, workflow.owner_role);

  return (
    <button
      type="button"
      onClick={onSelect}
      className="grid w-full gap-3 px-4 py-4 text-left"
      style={{
        border: "none",
        borderBottom: "1px solid var(--color-border)",
        backgroundColor: selected ? "var(--color-accent-dim)" : "transparent",
        cursor: "pointer",
      }}
    >
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0">
          <div className="flex items-center gap-2">
            <h2 className="m-0 truncate text-sm font-semibold" style={{ color: "var(--color-text)" }}>
              {workflow.name}
            </h2>
            <ChevronRight size={13} color={selected ? "var(--color-accent)" : "var(--color-text-tertiary)"} />
          </div>
          <div className="mt-2 flex flex-wrap gap-2">
            <Badge
              value={latestRun?.state || t("workflows.neverRun")}
              tone={latestRun ? stateTone(latestRun.state) : "neutral"}
            />
            <Badge value={workflow.trigger || MANUAL_TRIGGER} tone={isManualTrigger(workflow.trigger) ? "success" : "warning"} />
          </div>
        </div>
        <div className="text-right text-[11px]" style={{ color: "var(--color-text-tertiary)" }}>
          <div>{t("workflows.updated")}</div>
          <div className="mt-1">{formatDate(workflow.updated_at)}</div>
        </div>
      </div>

      <div className="flex flex-wrap gap-x-3 gap-y-1 text-[11px]" style={{ color: "var(--color-text-secondary)" }}>
        <span>{t("workflows.ownerRole")}: {workflow.owner_role || "-"}</span>
        <span>{t("workflows.stepCount", { count: preview.count })}</span>
      </div>

      <p className="m-0 line-clamp-2 text-xs leading-5" style={{ color: "var(--color-text-tertiary)" }}>
        {preview.text}
      </p>
    </button>
  );
}

function RunRow({ run }: { run: WorkflowRun }) {
  const { t } = useI18n();
  const taskIDs = parseStringList(run.task_ids);
  const reportIDs = parseStringList(run.report_ids);

  return (
    <article className="grid gap-3 p-4" style={{ backgroundColor: "var(--color-bg-elevated)", borderBottom: "1px solid var(--color-border)" }}>
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0">
          <div className="flex flex-wrap items-center gap-2">
            <h4 className="m-0 text-sm font-semibold" style={{ color: "var(--color-text)" }}>
              {run.id}
            </h4>
            <Badge value={run.state} tone={stateTone(run.state)} />
          </div>
          <div className="mt-1 text-xs" style={{ color: "var(--color-text-tertiary)" }}>
            {formatDate(run.updated_at)}
          </div>
        </div>
        <div className="text-xs" style={{ color: "var(--color-text-secondary)" }}>
          {(run.trigger_ref || "").trim() || MANUAL_TRIGGER_REF}
        </div>
      </div>

      <RunField label={t("workflows.run.taskIds")} value={taskIDs.length ? taskIDs.join(", ") : "-"} />
      <RunField label={t("workflows.run.reportIds")} value={reportIDs.length ? reportIDs.join(", ") : "-"} />
      <TraceField raw={run.trace} />
    </article>
  );
}

function Field({ label, htmlFor, children }: { label: string; htmlFor: string; children: ReactNode }) {
  return (
    <div className="min-w-0">
      <label htmlFor={htmlFor} className="mb-2 block text-sm font-medium" style={{ color: "var(--color-text)" }}>
        {label}
      </label>
      {children}
    </div>
  );
}

function RunField({ label, value, monospace = false }: { label: string; value: string; monospace?: boolean }) {
  return (
    <div className="min-w-0 text-xs">
      <div className="mb-1 font-medium uppercase" style={{ color: "var(--color-text-tertiary)" }}>
        {label}
      </div>
      <div
        className="break-all leading-5"
        style={{
          color: "var(--color-text-secondary)",
          fontFamily: monospace ? "var(--font-mono)" : undefined,
        }}
      >
        {value}
      </div>
    </div>
  );
}

function TraceField({ raw }: { raw?: string }) {
  const { t } = useI18n();
  const trace = useMemo(() => parseTrace(raw), [raw]);
  const label = t("workflows.run.trace");

  if (trace.mode === "fallback") {
    return <RunField label={label} value={trace.summary} monospace />;
  }

  if (!trace.events.length) {
    return <RunField label={label} value="-" />;
  }

  return (
    <div className="min-w-0 text-xs">
      <div className="mb-1 font-medium uppercase" style={{ color: "var(--color-text-tertiary)" }}>
        {label}
      </div>
      <div
        className="overflow-x-auto rounded-md"
        style={{
          border: "1px solid var(--color-border)",
          backgroundColor: "var(--color-surface)",
        }}
      >
        <table className="min-w-full border-collapse text-left">
          <thead style={{ backgroundColor: "var(--color-bg-elevated)" }}>
            <tr>
              {TRACE_TABLE_COLUMNS.map((column) => (
                <th
                  key={column.id}
                  className="px-2 py-1.5 text-[11px] font-medium"
                  style={{ borderBottom: "1px solid var(--color-border)", color: "var(--color-text-tertiary)" }}
                >
                  {t(column.titleKey)}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {trace.events.map((event) => (
              <tr key={event.key}>
                <TraceCell value={event.event} monospace />
                <TraceCell value={event.state} monospace />
                <TraceCell value={event.kind} monospace />
                <TraceCell value={event.taskID} monospace />
                <TraceCell value={event.approvalID} monospace />
                <TraceCell value={event.message} />
                <TraceCell value={formatTraceTime(event.time)} />
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}

function TraceCell({ value, monospace = false }: { value: string; monospace?: boolean }) {
  return (
    <td
      className="max-w-[20rem] break-words px-2 py-1.5 align-top"
      style={{
        borderBottom: "1px solid var(--color-border)",
        color: "var(--color-text-secondary)",
        fontFamily: monospace ? "var(--font-mono)" : undefined,
      }}
    >
      {value || "-"}
    </td>
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
      {value}
    </span>
  );
}

function StateBox({ text, tone, className }: { text: string; tone: "neutral" | "error"; className?: string }) {
  return (
    <div
      className={`rounded-md p-4 text-sm ${className ?? ""}`.trim()}
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

function ErrorBox({ text, onRetry }: { text: string; onRetry: () => void }) {
  const { t } = useI18n();
  return (
    <div
      className="rounded-md p-4 text-sm"
      style={{
        backgroundColor: "var(--color-error-dim)",
        border: "1px solid rgba(239,68,68,0.2)",
        color: "var(--color-error)",
      }}
    >
      <div>{text}</div>
      <button
        type="button"
        onClick={onRetry}
        className="mt-3 inline-flex h-8 items-center justify-center rounded-md px-3 text-xs font-medium"
        style={{
          border: "1px solid var(--color-error)",
          backgroundColor: "transparent",
          color: "var(--color-error)",
          cursor: "pointer",
        }}
      >
        {t("workflows.retry")}
      </button>
    </div>
  );
}

function LoadingList({ className }: { className?: string }) {
  return (
    <div className={`space-y-2 ${className ?? ""}`.trim()}>
      {[0, 1, 2].map((item) => (
        <div key={item} className="animate-shimmer rounded-md" style={{ height: 88, backgroundColor: "var(--color-surface)" }} />
      ))}
    </div>
  );
}

const inputStyle = {
  border: "1px solid var(--color-border)",
  backgroundColor: "var(--color-surface)",
  color: "var(--color-text)",
};

function createDraftForm(ownerRole: string, prompt: string): WorkflowFormState {
  return {
    name: "",
    trigger: MANUAL_TRIGGER,
    ownerRole,
    prompt,
    steps: buildStepsTemplate(ownerRole, prompt),
  };
}

function workflowToFormState(workflow: Workflow, defaultPrompt: string): WorkflowFormState {
  const parsed = parseStepSummary(workflow.steps, workflow.owner_role);
  const prompt = parsed.firstPrompt || defaultPrompt;
  return {
    name: workflow.name,
    trigger: workflow.trigger || MANUAL_TRIGGER,
    ownerRole: workflow.owner_role || parsed.firstRoleID || "",
    prompt,
    steps: workflow.steps || buildStepsTemplate(workflow.owner_role || parsed.firstRoleID || "", prompt),
  };
}

function buildStepsTemplate(ownerRole: string, prompt: string): string {
  return JSON.stringify(
    {
      steps: [
        {
          type: "role_task",
          role_id: ownerRole,
          prompt,
          name: "Primary step",
        },
      ],
    },
    null,
    2,
  );
}

function isManualTrigger(trigger?: string) {
  const normalized = (trigger || MANUAL_TRIGGER).trim();
  return normalized === "" || normalized === MANUAL_TRIGGER;
}

function stateTone(state: string): Tone {
  switch (state) {
    case "completed":
      return "success";
    case "failed":
      return "error";
    case "interrupted":
    case "waiting_approval":
      return "warning";
    default:
      return "neutral";
  }
}

function formatDate(value?: string) {
  if (!value) {
    return "-";
  }
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString();
}

function formatErrorMessage(error: unknown) {
  if (!error) {
    return null;
  }
  return error instanceof Error ? error.message : String(error);
}

function parseStringList(raw?: string) {
  if (!raw) {
    return [];
  }
  try {
    const parsed = JSON.parse(raw);
    if (Array.isArray(parsed)) {
      return parsed.map((item) => String(item)).filter(Boolean);
    }
  } catch {
    return raw
      .split(",")
      .map((item) => item.trim())
      .filter(Boolean);
  }
  return [];
}

function parseTrace(raw?: string): ParsedTrace {
  const trimmed = raw?.trim() || "";
  if (!trimmed) {
    return { mode: "structured", events: [] };
  }
  try {
    const parsed = JSON.parse(trimmed) as unknown;
    if (!Array.isArray(parsed)) {
      return { mode: "fallback", summary: truncateInline(trimmed) };
    }
    if (!parsed.length) {
      return { mode: "structured", events: [] };
    }
    const sliceStart = Math.max(parsed.length - MAX_TRACE_EVENTS, 0);
    const recentEvents = sliceStart === 0 ? parsed : parsed.slice(sliceStart);
    const events = recentEvents
      .map((item, index) => normalizeTraceEvent(item, sliceStart + index))
      .filter((item): item is TraceEventRow => item !== null);
    if (!events.length) {
      return { mode: "fallback", summary: summarizeParsedTrace(parsed, trimmed) };
    }
    return {
      mode: "structured",
      events,
    };
  } catch {
    return { mode: "fallback", summary: truncateInline(trimmed) };
  }
}

function normalizeTraceEvent(item: unknown, index: number): TraceEventRow | null {
  if (!item || typeof item !== "object") {
    return null;
  }
  const event = item as Record<string, unknown>;
  const summary = {
    event: firstText(event.event),
    state: firstText(event.state),
    kind: firstText(event.kind),
    taskID: firstText(event.task_id),
    approvalID: firstText(event.approval_id),
    message: joinTraceMessage(event),
    time: firstText(event.time),
  };
  if (!Object.values(summary).some(Boolean)) {
    return null;
  }
  return {
    key: `trace-row-${index}`,
    ...summary,
  };
}

function summarizeParsedTrace(parsed: unknown[], raw: string) {
  const last = parsed[parsed.length - 1];
  if (!last || typeof last !== "object") {
    return truncateInline(raw);
  }
  const item = last as Record<string, unknown>;
  const parts = [item.event, item.state, item.message, item.error]
    .map((value) => (typeof value === "string" ? value.trim() : ""))
    .filter(Boolean);
  return parts.length ? truncateInline(parts.join(" · "), 140) : truncateInline(raw);
}

function summarizeWorkflowSteps(raw?: string, fallbackRoleID?: string) {
  const summary = parseStepSummary(raw, fallbackRoleID);
  if (!summary.count) {
    return { count: 0, text: raw?.trim() || "-" };
  }
  const previewParts = [summary.firstTitle, summary.firstRoleID, summary.firstPrompt].filter(Boolean);
  return {
    count: summary.count,
    text: truncateInline(previewParts.join(" · "), 140),
  };
}

function parseStepSummary(raw?: string, fallbackRoleID?: string) {
  const payload = parseStepsPayload(raw);
  if (!payload.length) {
    return {
      count: 0,
      firstRoleID: fallbackRoleID?.trim() || "",
      firstPrompt: "",
      firstTitle: "",
    };
  }
  const first = payload[0];
  return {
    count: payload.length,
    firstRoleID: first.roleID || fallbackRoleID?.trim() || "",
    firstPrompt: first.prompt,
    firstTitle: first.title,
  };
}

function parseStepsPayload(raw?: string) {
  const trimmed = raw?.trim() || "";
  if (!trimmed) {
    return [] as Array<{ roleID: string; prompt: string; title: string }>;
  }
  try {
    const parsed = JSON.parse(trimmed) as unknown;
    const steps = Array.isArray(parsed)
      ? parsed
      : parsed && typeof parsed === "object" && Array.isArray((parsed as { steps?: unknown[] }).steps)
        ? (parsed as { steps: unknown[] }).steps
        : [];
    return steps
      .filter((item): item is Record<string, unknown> => !!item && typeof item === "object")
      .map((item) => ({
        roleID: firstText(item.role_id, item.role),
        prompt: firstText(item.prompt),
        title: firstText(item.name, item.title, item.kind, item.type),
      }));
  } catch {
    return [];
  }
}

function firstText(...values: unknown[]) {
  for (const value of values) {
    if (typeof value === "string" && value.trim()) {
      return value.trim();
    }
  }
  return "";
}

function joinTraceMessage(item: Record<string, unknown>) {
  const message = firstText(item.message);
  const error = firstText(item.error);
  if (message && error && error !== message) {
    return `${message} · ${error}`;
  }
  return message || error;
}

function formatTraceTime(value: string) {
  return value ? formatDate(value) : "-";
}

function truncateInline(value: string, max = 120) {
  const normalized = value.replace(/\s+/g, " ").trim();
  if (normalized.length <= max) {
    return normalized;
  }
  return `${normalized.slice(0, max - 3)}...`;
}
