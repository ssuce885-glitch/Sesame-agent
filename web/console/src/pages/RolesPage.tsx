import { useEffect, useState } from "react";
import type { ReactNode } from "react";
import { useNavigate } from "react-router-dom";
import { useCreateRole, useCreateTask, useRole, useRoles, useTasks, useUpdateRole } from "../api/queries";
import type { RoleInput, RoleSpec, Task } from "../api/types";
import { RoleList } from "../components/roles/RoleList";
import { Play, Save, X, Wrench } from "../components/Icon";
import { useI18n } from "../i18n";

type RolePageMode = "view" | "create" | "edit";
type Tone = "neutral" | "success" | "warning" | "error";

export function RolesPage({ workspaceRoot }: { workspaceRoot: string | null }) {
  const { t } = useI18n();
  const { data: roles = [], isLoading } = useRoles();
  const [selectedRoleID, setSelectedRoleID] = useState<string | null>(null);
  const [mode, setMode] = useState<RolePageMode>("view");
  const selectedRoleDetails = useRole(selectedRoleID);
  const selectedRole = selectedRoleDetails.data ?? roles.find((role) => role.id === selectedRoleID) ?? null;
  const createRole = useCreateRole();
  const updateRole = useUpdateRole();

  useEffect(() => {
    if (mode === "create") {
      return;
    }
    if (!roles.length) {
      setSelectedRoleID(null);
      return;
    }
    if (!selectedRoleID || !roles.some((role) => role.id === selectedRoleID)) {
      setSelectedRoleID(roles[0].id);
    }
  }, [mode, roles, selectedRoleID]);

  const mutationError = createRole.error ?? updateRole.error;

  return (
    <div className="flex h-full min-h-0 flex-col overflow-hidden lg:flex-row">
      <RoleList
        roles={roles}
        selectedRoleID={selectedRoleID}
        isLoading={isLoading}
        onSelectRole={(roleID) => {
          setSelectedRoleID(roleID);
          setMode("view");
        }}
        onCreateRole={() => {
          setSelectedRoleID(null);
          setMode("create");
        }}
      />
      <section className="flex-1 overflow-y-auto p-5" style={{ backgroundColor: "var(--color-bg)" }}>
        {mode === "create" ? (
          <RoleForm
            title={t("roles.createRole")}
            isSubmitting={createRole.isPending}
            error={mutationError}
            onCancel={() => setMode("view")}
            onSubmit={(input) => {
              createRole.mutate(input, {
                onSuccess: (role) => {
                  setSelectedRoleID(role.id);
                  setMode("view");
                },
              });
            }}
          />
        ) : mode === "edit" && selectedRole ? (
          <RoleForm
            title={t("roles.editRole")}
            role={selectedRole}
            isSubmitting={updateRole.isPending}
            error={mutationError}
            onCancel={() => setMode("view")}
            onSubmit={(input) => {
              updateRole.mutate({ roleId: selectedRole.id, role: input }, {
                onSuccess: (role) => {
                  setSelectedRoleID(role.id);
                  setMode("view");
                },
              });
            }}
          />
        ) : selectedRoleDetails.isLoading && selectedRoleID ? (
          <div className="flex h-full items-center justify-center">
            <div className="animate-shimmer rounded-lg" style={{ width: 200, height: 16, backgroundColor: "var(--color-surface)" }} />
          </div>
        ) : selectedRoleDetails.isError ? (
          <div
            className="max-w-xl rounded-lg p-4 text-sm"
            style={{
              backgroundColor: "var(--color-error-dim)",
              border: "1px solid rgba(239,68,68,0.15)",
              color: "var(--color-error)",
            }}
          >
            <div role="alert">{t("roles.loadFailed", { roleID: selectedRoleID ?? "" })}</div>
            <button
              type="button"
              className="mt-3 rounded-md px-3 py-1.5 text-xs font-medium"
              onClick={() => {
                void selectedRoleDetails.refetch();
              }}
              style={{
                border: "1px solid var(--color-error)",
                backgroundColor: "transparent",
                color: "var(--color-error)",
                cursor: "pointer",
              }}
            >
              {t("roles.retry")}
            </button>
          </div>
        ) : selectedRole ? (
          <RoleDetail role={selectedRole} workspaceRoot={workspaceRoot} onEdit={() => setMode("edit")} />
        ) : (
          <div className="text-sm" style={{ color: "var(--color-text-tertiary)" }}>
            {t("roles.empty")}
          </div>
        )}
      </section>
    </div>
  );
}

function RoleDetail({ role, workspaceRoot, onEdit }: { role: RoleSpec; workspaceRoot: string | null; onEdit: () => void }) {
  const { t } = useI18n();
  const navigate = useNavigate();
  const createTask = useCreateTask();
  const recentTasks = useTasks(workspaceRoot, { role_id: role.id, limit: 5 });
  const [testPrompt, setTestPrompt] = useState("");
  const runError = createTask.error;

  function runTest() {
    if (!workspaceRoot || createTask.isPending) {
      return;
    }
    const prompt = testPrompt.trim() || t("roles.defaultTestPrompt", { roleID: role.id });
    createTask.mutate(
      {
        workspace_root: workspaceRoot,
        role_id: role.id,
        kind: "agent",
        prompt,
      },
      {
        onSuccess: (task) => navigate(`/tasks/${encodeURIComponent(task.id)}`),
      },
    );
  }

  return (
    <div className="mx-auto flex max-w-4xl flex-col gap-5">
      <header className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <div className="text-xs font-medium uppercase" style={{ color: "var(--color-text-tertiary)" }}>
            {role.id}
          </div>
          <h1 className="m-0 mt-1 text-2xl font-semibold" style={{ color: "var(--color-text)" }}>
            {role.name}
          </h1>
          <p className="mt-2 max-w-2xl text-sm leading-6" style={{ color: "var(--color-text-secondary)" }}>
            {role.description}
          </p>
        </div>
        <button
          type="button"
          onClick={onEdit}
          className="inline-flex h-9 items-center justify-center gap-2 rounded-md px-3 text-sm font-medium"
          style={{
            border: "1px solid var(--color-border)",
            backgroundColor: "var(--color-surface)",
            color: "var(--color-text)",
            cursor: "pointer",
          }}
        >
          <Wrench size={15} color="currentColor" />
          {t("roles.edit")}
        </button>
      </header>

      <div className="grid gap-3 md:grid-cols-3">
        <Meta label="Model" value={role.model ?? "-"} />
        <Meta label="Permission" value={role.permission_profile ?? "-"} />
        <Meta label="Version" value={role.version ? `v${role.version}` : "-"} />
        <Meta label="Max Tools" value={role.max_tool_calls ? String(role.max_tool_calls) : "-"} />
        <Meta label="Context" value={role.max_context_tokens ? String(role.max_context_tokens) : "-"} />
        <Meta label="Can Delegate" value={role.can_delegate ? "yes" : "no"} />
      </div>

      <Section title={t("roles.testRun")}>
        <div className="flex flex-col gap-3 rounded-md p-4" style={{ backgroundColor: "var(--color-bg-elevated)", border: "1px solid var(--color-border)" }}>
          <textarea
            value={testPrompt}
            onChange={(event) => setTestPrompt(event.target.value)}
            rows={4}
            placeholder={t("roles.testPromptPlaceholder", { roleID: role.id })}
            className="w-full resize-y rounded-md p-3 text-sm leading-6"
            style={{ ...inputStyle, fontFamily: "var(--font-mono)" }}
          />
          {runError ? (
            <div className="rounded-md p-3 text-sm" style={{ backgroundColor: "var(--color-error-dim)", border: "1px solid rgba(239,68,68,0.15)", color: "var(--color-error)" }}>
              {runError instanceof Error ? runError.message : String(runError)}
            </div>
          ) : null}
          <div className="flex justify-end">
            <button
              type="button"
              disabled={!workspaceRoot || createTask.isPending}
              onClick={runTest}
              className="inline-flex h-9 items-center justify-center gap-2 rounded-md px-3 text-sm font-medium"
              style={{
                border: "1px solid var(--color-accent)",
                backgroundColor: "var(--color-accent)",
                color: "white",
                cursor: !workspaceRoot || createTask.isPending ? "not-allowed" : "pointer",
                opacity: !workspaceRoot || createTask.isPending ? 0.65 : 1,
              }}
            >
              <Play size={14} color="currentColor" />
              {createTask.isPending ? t("roles.starting") : t("roles.runTest")}
            </button>
          </div>
        </div>
      </Section>

      <Section title={t("roles.recentRuns")}>
        {recentTasks.isLoading ? (
          <div className="animate-shimmer rounded-md" style={{ height: 86, backgroundColor: "var(--color-surface)" }} />
        ) : recentTasks.isError ? (
          <p className="m-0 text-sm" style={{ color: "var(--color-error)" }}>{t("roles.recentRunsFailed")}</p>
        ) : recentTasks.data?.length ? (
          <div className="overflow-hidden rounded-md" style={{ border: "1px solid var(--color-border)" }}>
            {recentTasks.data.map((task) => (
              <RoleTaskRow key={task.id} task={task} onOpen={() => navigate(`/tasks/${encodeURIComponent(task.id)}`)} />
            ))}
          </div>
        ) : (
          <p className="m-0 text-sm" style={{ color: "var(--color-text-tertiary)" }}>{t("roles.noRecentRuns")}</p>
        )}
      </Section>

      <Section title="Prompt">
        <pre
          className="m-0 whitespace-pre-wrap rounded-md p-4 text-sm leading-6"
          style={{
            backgroundColor: "var(--color-bg-elevated)",
            border: "1px solid var(--color-border)",
            color: "var(--color-text-secondary)",
            fontFamily: "var(--font-mono)",
          }}
        >
          {role.system_prompt}
        </pre>
      </Section>

      <Section title="Skills">
        <TagList values={role.skill_names ?? []} empty="No skills configured." />
      </Section>

      <Section title="Automation Ownership">
        <TagList values={role.automation_ownership ?? []} empty="No automation ownership configured." />
      </Section>

      <Section title="Allowed Tools">
        <TagList values={role.allowed_tools ?? []} empty="No explicit allow list." />
      </Section>

      <Section title="Denied Tools">
        <TagList values={role.denied_tools ?? []} empty="No explicit deny list." />
      </Section>

      <Section title="Allowed Paths">
        <TagList values={role.allowed_paths ?? []} empty="No explicit path allow list." />
      </Section>

      <Section title="Denied Paths">
        <TagList values={role.denied_paths ?? []} empty="No explicit path deny list." />
      </Section>
    </div>
  );
}

function RoleTaskRow({ task, onOpen }: { task: Task; onOpen: () => void }) {
  return (
    <button
      type="button"
      onClick={onOpen}
      className="grid w-full gap-3 p-3 text-left md:grid-cols-[minmax(0,1fr)_120px]"
      style={{
        backgroundColor: "var(--color-bg-elevated)",
        border: "none",
        borderBottom: "1px solid var(--color-border)",
        color: "var(--color-text)",
        cursor: "pointer",
      }}
    >
      <div className="min-w-0">
        <div className="flex flex-wrap items-center gap-2">
          <Badge value={task.state} tone={stateTone(task.state)} />
          <Badge value={task.kind} tone="neutral" />
          <span className="text-[11px]" style={{ color: "var(--color-text-tertiary)" }}>{task.id}</span>
        </div>
        <p className="m-0 mt-1 line-clamp-2 text-xs leading-5" style={{ color: "var(--color-text-tertiary)" }}>
          {task.final_text || task.prompt}
        </p>
      </div>
      <div className="text-xs" style={{ color: "var(--color-text-secondary)" }}>
        {formatDate(task.updated_at)}
      </div>
    </button>
  );
}

function RoleForm({
  title,
  role,
  isSubmitting,
  error,
  onCancel,
  onSubmit,
}: {
  title: string;
  role?: RoleSpec;
  isSubmitting: boolean;
  error: unknown;
  onCancel: () => void;
  onSubmit: (input: RoleInput) => void;
}) {
  const { t } = useI18n();
  const [form, setForm] = useState<RoleInput>(() => roleToInput(role));

  useEffect(() => {
    setForm(roleToInput(role));
  }, [role?.id]);

  function setField<K extends keyof RoleInput>(key: K, value: RoleInput[K]) {
    setForm((current) => ({ ...current, [key]: value }));
  }

  return (
    <form
      className="mx-auto flex max-w-4xl flex-col gap-5"
      onSubmit={(event) => {
        event.preventDefault();
        onSubmit(normalizeRoleInput(form));
      }}
    >
      <header className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <div className="text-xs font-medium uppercase" style={{ color: "var(--color-text-tertiary)" }}>
            {role?.id ?? "new"}
          </div>
          <h1 className="m-0 mt-1 text-2xl font-semibold" style={{ color: "var(--color-text)" }}>
            {title}
          </h1>
        </div>
        <div className="flex gap-2">
          <button
            type="button"
            onClick={onCancel}
            className="inline-flex h-9 items-center justify-center gap-2 rounded-md px-3 text-sm font-medium"
            style={{
              border: "1px solid var(--color-border)",
              backgroundColor: "transparent",
              color: "var(--color-text-secondary)",
              cursor: "pointer",
            }}
          >
            <X size={15} color="currentColor" />
            {t("roles.cancel")}
          </button>
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
            <Save size={15} color="currentColor" />
            {isSubmitting ? t("roles.saving") : t("roles.save")}
          </button>
        </div>
      </header>

      {error ? (
        <div
          className="rounded-md p-3 text-sm"
          style={{
            backgroundColor: "var(--color-error-dim)",
            border: "1px solid rgba(239,68,68,0.15)",
            color: "var(--color-error)",
          }}
        >
          {error instanceof Error ? error.message : String(error)}
        </div>
      ) : null}

      <div className="grid gap-4 md:grid-cols-2">
        <Field label="Role ID">
          <input
            value={form.id}
            disabled={!!role}
            onChange={(event) => setField("id", event.target.value)}
            required
            pattern="[a-z][a-z0-9_-]{0,63}"
            className="h-10 w-full rounded-md px-3 text-sm"
            style={inputStyle}
          />
        </Field>
        <Field label="Name">
          <input
            value={form.name}
            onChange={(event) => setField("name", event.target.value)}
            required
            className="h-10 w-full rounded-md px-3 text-sm"
            style={inputStyle}
          />
        </Field>
      </div>

      <Field label="Description">
        <input
          value={form.description ?? ""}
          onChange={(event) => setField("description", event.target.value)}
          className="h-10 w-full rounded-md px-3 text-sm"
          style={inputStyle}
        />
      </Field>

      <Field label="System Prompt">
        <textarea
          value={form.system_prompt}
          onChange={(event) => setField("system_prompt", event.target.value)}
          required
          rows={10}
          className="w-full resize-y rounded-md p-3 text-sm leading-6"
          style={{ ...inputStyle, fontFamily: "var(--font-mono)" }}
        />
      </Field>

      <div className="grid gap-4 md:grid-cols-2">
        <Field label="Model">
          <input
            value={form.model ?? ""}
            onChange={(event) => setField("model", event.target.value)}
            className="h-10 w-full rounded-md px-3 text-sm"
            style={inputStyle}
          />
        </Field>
        <Field label="Permission Profile">
          <input
            value={form.permission_profile ?? ""}
            onChange={(event) => setField("permission_profile", event.target.value)}
            className="h-10 w-full rounded-md px-3 text-sm"
            style={inputStyle}
          />
        </Field>
      </div>

      <div className="grid gap-4 md:grid-cols-2">
        <Field label="Max Tool Calls">
          <input
            type="number"
            min={0}
            value={form.max_tool_calls ?? ""}
            onChange={(event) => setField("max_tool_calls", numberValue(event.target.value))}
            className="h-10 w-full rounded-md px-3 text-sm"
            style={inputStyle}
          />
        </Field>
        <Field label="Max Runtime Seconds">
          <input
            type="number"
            min={0}
            value={form.max_runtime ?? ""}
            onChange={(event) => setField("max_runtime", numberValue(event.target.value))}
            className="h-10 w-full rounded-md px-3 text-sm"
            style={inputStyle}
          />
        </Field>
      </div>

      <div className="grid gap-4 md:grid-cols-2">
        <Field label="Max Context Tokens">
          <input
            type="number"
            min={0}
            value={form.max_context_tokens ?? ""}
            onChange={(event) => setField("max_context_tokens", numberValue(event.target.value))}
            className="h-10 w-full rounded-md px-3 text-sm"
            style={inputStyle}
          />
        </Field>
        <Field label="Can Delegate">
          <div
            className="flex h-10 items-center gap-2 rounded-md px-3 text-sm"
            style={inputStyle}
          >
            <input
              type="checkbox"
              checked={!!form.can_delegate}
              onChange={(event) => setField("can_delegate", event.target.checked)}
            />
            <span style={{ color: "var(--color-text-secondary)" }}>Enabled</span>
          </div>
        </Field>
      </div>

      <div className="grid gap-4 md:grid-cols-2">
        <ListField label="Skills" values={form.skill_names ?? []} onChange={(values) => setField("skill_names", values)} />
        <ListField label="Automation Ownership" values={form.automation_ownership ?? []} onChange={(values) => setField("automation_ownership", values)} />
        <ListField label="Allowed Tools" values={form.allowed_tools ?? []} onChange={(values) => setField("allowed_tools", values)} />
        <ListField label="Denied Tools" values={form.denied_tools ?? []} onChange={(values) => setField("denied_tools", values)} />
        <ListField label="Allowed Paths" values={form.allowed_paths ?? []} onChange={(values) => setField("allowed_paths", values)} />
        <ListField label="Denied Paths" values={form.denied_paths ?? []} onChange={(values) => setField("denied_paths", values)} />
      </div>
    </form>
  );
}

const inputStyle = {
  backgroundColor: "var(--color-bg-elevated)",
  border: "1px solid var(--color-border)",
  color: "var(--color-text)",
} as const;

function Field({ label, children }: { label: string; children: ReactNode }) {
  return (
    <label className="flex flex-col gap-1.5">
      <span className="text-xs font-semibold uppercase" style={{ color: "var(--color-text-tertiary)" }}>
        {label}
      </span>
      {children}
    </label>
  );
}

function ListField({ label, values, onChange }: { label: string; values: string[]; onChange: (values: string[]) => void }) {
  return (
    <Field label={label}>
      <textarea
        value={values.join("\n")}
        onChange={(event) => onChange(parseList(event.target.value))}
        rows={4}
        className="w-full resize-y rounded-md p-3 text-sm leading-5"
        style={{ ...inputStyle, fontFamily: "var(--font-mono)" }}
      />
    </Field>
  );
}

function Meta({ label, value }: { label: string; value: string }) {
  return (
    <div
      className="rounded-md p-3"
      style={{
        backgroundColor: "var(--color-bg-elevated)",
        border: "1px solid var(--color-border)",
      }}
    >
      <div className="text-[11px] font-medium uppercase" style={{ color: "var(--color-text-tertiary)" }}>
        {label}
      </div>
      <div className="mt-1 truncate text-sm" style={{ color: "var(--color-text)" }}>
        {value}
      </div>
    </div>
  );
}

function Section({ title, children }: { title: string; children: ReactNode }) {
  return (
    <section>
      <h2 className="m-0 mb-2 text-xs font-semibold uppercase" style={{ color: "var(--color-text-tertiary)" }}>
        {title}
      </h2>
      {children}
    </section>
  );
}

function TagList({ values, empty }: { values: string[]; empty: string }) {
  if (values.length === 0) {
    return <p className="m-0 text-sm" style={{ color: "var(--color-text-tertiary)" }}>{empty}</p>;
  }
  return (
    <div className="flex flex-wrap gap-2">
      {values.map((value) => (
        <span
          key={value}
          className="rounded px-2 py-1 text-xs"
          style={{
            backgroundColor: "var(--color-surface)",
            border: "1px solid var(--color-border)",
            color: "var(--color-text-secondary)",
          }}
        >
          {value}
        </span>
      ))}
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

function roleToInput(role?: RoleSpec): RoleInput {
  return {
    id: role?.id ?? "",
    name: role?.name ?? "",
    description: role?.description ?? "",
    system_prompt: role?.system_prompt ?? "",
    permission_profile: role?.permission_profile ?? "",
    model: role?.model ?? "",
    max_tool_calls: role?.max_tool_calls,
    max_runtime: role?.max_runtime,
    max_context_tokens: role?.max_context_tokens,
    can_delegate: role?.can_delegate ?? false,
    automation_ownership: role?.automation_ownership ?? [],
    skill_names: role?.skill_names ?? [],
    denied_tools: role?.denied_tools ?? [],
    allowed_tools: role?.allowed_tools ?? [],
    denied_paths: role?.denied_paths ?? [],
    allowed_paths: role?.allowed_paths ?? [],
  };
}

function normalizeRoleInput(input: RoleInput): RoleInput {
  return {
    ...input,
    id: input.id.trim(),
    name: input.name.trim(),
    description: input.description?.trim() ?? "",
    system_prompt: input.system_prompt.trim(),
    permission_profile: input.permission_profile?.trim() ?? "",
    model: input.model?.trim() ?? "",
    can_delegate: !!input.can_delegate,
    automation_ownership: input.automation_ownership ?? [],
    skill_names: input.skill_names ?? [],
    denied_tools: input.denied_tools ?? [],
    allowed_tools: input.allowed_tools ?? [],
    denied_paths: input.denied_paths ?? [],
    allowed_paths: input.allowed_paths ?? [],
  };
}

function parseList(value: string): string[] {
  return value
    .split(/\r?\n|,/)
    .map((item) => item.trim())
    .filter(Boolean);
}

function numberValue(value: string): number | undefined {
  if (value.trim() === "") {
    return undefined;
  }
  return Number(value);
}
