import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  cancelTask,
  createAutomation,
  createContextBlock,
  createMemory,
  createRole,
  createTask,
  createWorkflow,
  createWorkflowRun,
  deleteContextBlock,
  deleteMemory,
  ensureSession,
  getContextPreview,
  getProjectState,
  getReports,
  getRole,
  getSession,
  getSetting,
  getStatus,
  getTask,
  getTaskTrace,
  getTimeline,
  getWorkflow,
  getWorkflowRun,
  interruptTurn,
  listAutomations,
  listAutomationRuns,
  listContextBlocks,
  listTasks,
  listRoles,
  listWorkflowRuns,
  listWorkflows,
  pauseAutomation,
  resumeAutomation,
  searchMemory,
  setSetting,
  submitTurn,
  triggerWorkflow,
  updateProjectState,
  updateContextBlock,
  updateRole,
  updateWorkflow,
  updateWorkflowRun,
} from "./client";
import type {
  Automation,
  ContextBlock,
  ContextBlockFilters,
  Memory,
  ProjectState,
  RoleInput,
  RoleSpec,
  Task,
  TaskListFilters,
  Workflow,
  WorkflowRun,
  WorkflowRunFilters,
  WorkflowTriggerInput,
} from "./types";

export function useSession(workspaceRoot: string) {
  return useQuery({
    queryKey: ["session", workspaceRoot],
    queryFn: () => ensureSession(workspaceRoot),
    staleTime: 10_000,
  });
}

export function useSessionInfo(sessionId: string | null) {
  return useQuery({
    queryKey: ["session-info", sessionId],
    queryFn: () => getSession(sessionId!),
    enabled: !!sessionId,
    staleTime: 10_000,
  });
}

export function useTimeline(sessionId: string | null) {
  return useQuery({
    queryKey: ["timeline", sessionId],
    queryFn: () => getTimeline(sessionId!),
    enabled: !!sessionId,
    staleTime: Infinity,
  });
}

export function useContextPreview(sessionId: string | null) {
  return useQuery({
    queryKey: ["context-preview", sessionId],
    queryFn: () => getContextPreview(sessionId!),
    enabled: !!sessionId,
    staleTime: 5_000,
  });
}

export function useContextBlocks(workspaceRoot: string | null, filters: ContextBlockFilters = {}) {
  const owner = filters.owner ?? "";
  const visibility = filters.visibility ?? "";
  const type = filters.type ?? "";
  const limit = filters.limit ?? 0;
  return useQuery({
    queryKey: ["context-blocks", workspaceRoot, owner, visibility, type, limit],
    queryFn: () => listContextBlocks(workspaceRoot!, filters),
    enabled: !!workspaceRoot,
    staleTime: 5_000,
  });
}

export function useCreateContextBlock(workspaceRoot: string | null) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (block: Partial<ContextBlock>) => createContextBlock(block),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["context-blocks", workspaceRoot] });
      qc.invalidateQueries({ queryKey: ["context-preview"] });
    },
  });
}

export function useUpdateContextBlock(workspaceRoot: string | null) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, block }: { id: string; block: Partial<ContextBlock> }) => updateContextBlock(id, block),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["context-blocks", workspaceRoot] });
      qc.invalidateQueries({ queryKey: ["context-preview"] });
    },
  });
}

export function useDeleteContextBlock(workspaceRoot: string | null) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => deleteContextBlock(id),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["context-blocks", workspaceRoot] });
      qc.invalidateQueries({ queryKey: ["context-preview"] });
    },
  });
}

export function useRoles() {
  return useQuery({
    queryKey: ["roles"],
    queryFn: listRoles,
    staleTime: 10_000,
  });
}

export function useRole(roleId: string | null) {
  return useQuery({
    queryKey: ["roles", roleId],
    queryFn: () => getRole(roleId!),
    enabled: !!roleId,
    staleTime: 10_000,
  });
}

export function useCreateRole() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (role: RoleInput) => createRole(role),
    onSuccess: (role) => {
      qc.setQueryData<RoleSpec[]>(["roles"], (current = []) => {
        const existing = current.filter((item) => item.id !== role.id);
        return [...existing, role].sort((a, b) => a.id.localeCompare(b.id));
      });
      qc.invalidateQueries({ queryKey: ["roles"] });
      qc.setQueryData(["roles", role.id], role);
    },
  });
}

export function useUpdateRole() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ roleId, role }: { roleId: string; role: RoleInput }) => updateRole(roleId, role),
    onSuccess: (role) => {
      qc.setQueryData<RoleSpec[]>(["roles"], (current = []) => current.map((item) => (item.id === role.id ? role : item)));
      qc.invalidateQueries({ queryKey: ["roles"] });
      qc.setQueryData(["roles", role.id], role);
    },
  });
}

export function useReports(workspaceRoot: string | null) {
  return useQuery({
    queryKey: ["reports", workspaceRoot],
    queryFn: () => getReports(workspaceRoot!),
    enabled: !!workspaceRoot,
    staleTime: 10_000,
  });
}

export function useAutomations(workspaceRoot: string | null) {
  return useQuery({
    queryKey: ["automations", workspaceRoot],
    queryFn: () => listAutomations(workspaceRoot!),
    enabled: !!workspaceRoot,
    staleTime: 10_000,
  });
}

export function useAutomationRuns(automationId: string | null) {
  return useQuery({
    queryKey: ["automation-runs", automationId],
    queryFn: () => listAutomationRuns(automationId!, 20),
    enabled: !!automationId,
    staleTime: 5_000,
  });
}

export function useWorkflows(workspaceRoot: string | null) {
  return useQuery({
    queryKey: ["workflows", "list", workspaceRoot],
    queryFn: () => listWorkflows(workspaceRoot!),
    enabled: !!workspaceRoot,
    staleTime: 10_000,
  });
}

export function useWorkflow(workflowId: string | null) {
  return useQuery({
    queryKey: ["workflows", "detail", workflowId],
    queryFn: () => getWorkflow(workflowId!),
    enabled: !!workflowId,
    staleTime: 10_000,
  });
}

export function useWorkflowRuns(workspaceRoot: string | null, filters: WorkflowRunFilters = {}) {
  const workflowID = filters.workflow_id ?? "";
  const state = filters.state ?? "";
  const limit = filters.limit ?? 0;
  return useQuery({
    queryKey: ["workflow-runs", "list", workspaceRoot, workflowID, state, limit],
    queryFn: () => listWorkflowRuns(workspaceRoot!, filters),
    enabled: !!workspaceRoot,
    staleTime: 5_000,
  });
}

export function useWorkflowRun(runId: string | null) {
  return useQuery({
    queryKey: ["workflow-runs", "detail", runId],
    queryFn: () => getWorkflowRun(runId!),
    enabled: !!runId,
    staleTime: 5_000,
  });
}

export function useProjectState(workspaceRoot: string | null) {
  return useQuery({
    queryKey: ["project-state", workspaceRoot],
    queryFn: () => getProjectState(workspaceRoot!),
    enabled: !!workspaceRoot,
    staleTime: 10_000,
  });
}

export function useMemories(workspaceRoot: string | null, query: string) {
  return useQuery({
    queryKey: ["memories", workspaceRoot, query],
    queryFn: () => searchMemory(query, workspaceRoot!, 80),
    enabled: !!workspaceRoot,
    staleTime: 5_000,
  });
}

export function useCreateMemory(workspaceRoot: string | null, query = "") {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (memory: Partial<Memory>) => createMemory(memory),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["memories", workspaceRoot] });
      qc.invalidateQueries({ queryKey: ["memories", workspaceRoot, query] });
    },
  });
}

export function useDeleteMemory(workspaceRoot: string | null, query = "") {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => deleteMemory(id),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["memories", workspaceRoot] });
      qc.invalidateQueries({ queryKey: ["memories", workspaceRoot, query] });
    },
  });
}

export function useSetting(key: string) {
  return useQuery({
    queryKey: ["settings", key],
    queryFn: () => getSetting(key),
    staleTime: 10_000,
  });
}

export function useSetSetting(key: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (value: string) => setSetting(key, value),
    onSuccess: (setting) => {
      qc.setQueryData(["settings", key], setting);
      qc.invalidateQueries({ queryKey: ["settings", key] });
    },
  });
}

export function useStatus() {
  return useQuery({
    queryKey: ["status"],
    queryFn: getStatus,
    staleTime: 10_000,
  });
}

export function useTask(taskId: string | null) {
  return useQuery({
    queryKey: ["tasks", taskId],
    queryFn: () => getTask(taskId!),
    enabled: !!taskId,
    staleTime: 10_000,
  });
}

export function useTasks(workspaceRoot: string | null, filters: TaskListFilters = {}) {
  const state = filters.state ?? "";
  const roleID = filters.role_id ?? "";
  const sessionID = filters.session_id ?? "";
  const limit = filters.limit ?? 0;
  return useQuery({
    queryKey: ["tasks", "list", workspaceRoot, state, roleID, sessionID, limit],
    queryFn: () => listTasks(workspaceRoot!, filters),
    enabled: !!workspaceRoot,
    refetchInterval: state === "pending,running" || state === "running" || state === "" ? 3000 : false,
  });
}

export function useTaskTrace(taskId: string | null) {
  return useQuery({
    queryKey: ["tasks", taskId, "trace"],
    queryFn: () => getTaskTrace(taskId!),
    enabled: !!taskId,
    refetchInterval: (query) => {
      const state = query.state.data?.task.state;
      return state === "pending" || state === "running" ? 2000 : false;
    },
  });
}

export function useSubmitTurn(sessionId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (message: string) => submitTurn(sessionId, message),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["session-info", sessionId] });
    },
  });
}

export function useInterruptTurn() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (turnId: string) => interruptTurn(turnId),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["session"] });
      qc.invalidateQueries({ queryKey: ["session-info"] });
    },
  });
}

export function useCreateTask() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (task: Partial<Task>) => createTask(task),
    onSuccess: (task) => {
      qc.invalidateQueries({ queryKey: ["tasks"] });
      qc.invalidateQueries({ queryKey: ["session-info", task.session_id] });
      qc.invalidateQueries({ queryKey: ["timeline", task.session_id] });
    },
  });
}

export function useCancelTask() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (taskId: string) => cancelTask(taskId),
    onSuccess: (task) => {
      qc.invalidateQueries({ queryKey: ["tasks"] });
      qc.invalidateQueries({ queryKey: ["tasks", task.id] });
    },
  });
}

export function useCreateAutomation(workspaceRoot: string | null) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (data: Partial<Automation>) => createAutomation(data),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["automations", workspaceRoot] }),
  });
}

export function usePauseAutomation(workspaceRoot: string | null) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => pauseAutomation(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["automations", workspaceRoot] }),
  });
}

export function useResumeAutomation(workspaceRoot: string | null) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => resumeAutomation(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["automations", workspaceRoot] }),
  });
}

export function useCreateWorkflow(workspaceRoot: string | null) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (workflow: Partial<Workflow>) => createWorkflow(workflow),
    onSuccess: (workflow) => {
      qc.invalidateQueries({ queryKey: ["workflows", "list", workspaceRoot] });
      qc.setQueryData(["workflows", "detail", workflow.id], workflow);
    },
  });
}

export function useUpdateWorkflow(workspaceRoot: string | null) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, workflow }: { id: string; workflow: Partial<Workflow> }) => updateWorkflow(id, workflow),
    onSuccess: (workflow) => {
      qc.invalidateQueries({ queryKey: ["workflows", "list", workspaceRoot] });
      qc.setQueryData(["workflows", "detail", workflow.id], workflow);
    },
  });
}

export function useCreateWorkflowRun(workspaceRoot: string | null) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (run: Partial<WorkflowRun>) => createWorkflowRun(run),
    onSuccess: (run) => {
      qc.invalidateQueries({ queryKey: ["workflow-runs", "list", workspaceRoot] });
      qc.setQueryData(["workflow-runs", "detail", run.id], run);
    },
  });
}

export function useTriggerWorkflow(workspaceRoot: string | null) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ workflowId, input }: { workflowId: string; input?: WorkflowTriggerInput }) =>
      triggerWorkflow(workflowId, input ?? {}),
    onSuccess: (run) => {
      qc.invalidateQueries({ queryKey: ["workflow-runs", "list", workspaceRoot] });
      qc.setQueryData(["workflow-runs", "detail", run.id], run);
      qc.invalidateQueries({ queryKey: ["workflows", "detail", run.workflow_id] });
    },
  });
}

export function useUpdateWorkflowRun(workspaceRoot: string | null) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, run }: { id: string; run: Partial<WorkflowRun> }) => updateWorkflowRun(id, run),
    onSuccess: (run) => {
      qc.invalidateQueries({ queryKey: ["workflow-runs", "list", workspaceRoot] });
      qc.setQueryData(["workflow-runs", "detail", run.id], run);
    },
  });
}

export function useUpdateProjectState(workspaceRoot: string | null) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (state: Partial<ProjectState>) => updateProjectState(state),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["project-state", workspaceRoot] }),
  });
}
