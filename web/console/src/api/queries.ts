import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  cancelTask,
  createAutomation,
  createMemory,
  createRole,
  createTask,
  deleteMemory,
  ensureSession,
  getProjectState,
  getReports,
  getRole,
  getSession,
  getSetting,
  getStatus,
  getTask,
  getTaskTrace,
  getTimeline,
  interruptTurn,
  listAutomations,
  listAutomationRuns,
  listTasks,
  listRoles,
  pauseAutomation,
  resumeAutomation,
  searchMemory,
  setSetting,
  submitTurn,
  updateProjectState,
  updateRole,
} from "./client";
import type { Automation, Memory, ProjectState, RoleInput, RoleSpec, Task, TaskListFilters } from "./types";

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

export function useUpdateProjectState(workspaceRoot: string | null) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (state: Partial<ProjectState>) => updateProjectState(state),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["project-state", workspaceRoot] }),
  });
}
