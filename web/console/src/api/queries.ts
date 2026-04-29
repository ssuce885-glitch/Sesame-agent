import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import {
  ensureCurrentSession,
  getTimeline,
  getContextHistory,
  getWorkspace,
  getWorkspaceReports,
  getWorkspaceRuntimeGraph,
  getFileCheckpointDiff,
  loadContextHistory,
  listFileCheckpoints,
  reopenContext,
  rollbackFileCheckpoint,
  submitMessage,
  getMetricsOverview,
  getMetricsTimeseries,
  getRole,
  listRoleVersions,
  listRoles,
  createRole,
  updateRole,
  deleteRole,
  generateClientTurnId,
} from "./client";

// ─── Current workspace/session queries ─────────────────────────────────────────

export function useWorkspaceMeta() {
  return useQuery({
    queryKey: ["workspace"],
    queryFn: getWorkspace,
    staleTime: 30_000,
  });
}

export function useCurrentSession() {
  return useQuery({
    queryKey: ["session", "current"],
    queryFn: ensureCurrentSession,
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

export function useContextHistory(sessionId: string | null) {
  return useQuery({
    queryKey: ["history", sessionId],
    queryFn: () => getContextHistory(sessionId!),
    enabled: !!sessionId,
    staleTime: 10_000,
  });
}

export function useWorkspaceRuntimeGraph() {
  return useQuery({
    queryKey: ["runtime-graph"],
    queryFn: getWorkspaceRuntimeGraph,
    staleTime: 10_000,
  });
}

export function useWorkspaceReports() {
  return useQuery({
    queryKey: ["workspace-reports"],
    queryFn: getWorkspaceReports,
    staleTime: 10_000,
  });
}

export function useFileCheckpoints(sessionId: string | null) {
  return useQuery({
    queryKey: ["file-checkpoints", sessionId],
    queryFn: () => listFileCheckpoints(sessionId!),
    enabled: !!sessionId,
    staleTime: 10_000,
  });
}

export function useFileCheckpointDiff(sessionId: string | null, checkpointId: string | null) {
  return useQuery({
    queryKey: ["file-checkpoints", sessionId, checkpointId, "diff"],
    queryFn: () => getFileCheckpointDiff(sessionId!, checkpointId!),
    enabled: !!sessionId && !!checkpointId,
    staleTime: 30_000,
  });
}

export function useReopenContext(sessionId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () => reopenContext(sessionId),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["history", sessionId] });
      qc.invalidateQueries({ queryKey: ["timeline", sessionId] });
    },
  });
}

export function useLoadContextHistory(sessionId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (headId: string) => loadContextHistory(sessionId, headId),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["history", sessionId] });
      qc.invalidateQueries({ queryKey: ["timeline", sessionId] });
    },
  });
}

export function useRollbackFileCheckpoint(sessionId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (checkpointId: string) => rollbackFileCheckpoint(sessionId, checkpointId),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["file-checkpoints", sessionId] });
      qc.invalidateQueries({ queryKey: ["timeline", sessionId] });
    },
  });
}

// ─── Metrics queries ────────────────────────────────────────────────────────────

export function useMetricsOverview(sessionId?: string) {
  return useQuery({
    queryKey: ["metrics", "overview", sessionId],
    queryFn: () => getMetricsOverview(sessionId),
    staleTime: 30_000,
  });
}

export function useMetricsTimeseries(sessionId?: string) {
  return useQuery({
    queryKey: ["metrics", "timeseries", sessionId],
    queryFn: () => getMetricsTimeseries(sessionId),
    staleTime: 30_000,
  });
}

// ─── Role queries ───────────────────────────────────────────────────────────────

export function useRoles() {
  return useQuery({
    queryKey: ["roles"],
    queryFn: listRoles,
    staleTime: 10_000,
  });
}

export function useRole(roleID: string | null) {
  return useQuery({
    queryKey: ["roles", roleID],
    queryFn: () => getRole(roleID!),
    enabled: !!roleID,
    staleTime: 10_000,
  });
}

export function useRoleVersions(roleID: string | null) {
  return useQuery({
    queryKey: ["roles", roleID, "versions"],
    queryFn: () => listRoleVersions(roleID!),
    enabled: !!roleID,
    staleTime: 10_000,
  });
}

// ─── Mutations ─────────────────────────────────────────────────────────────────

export function useSubmitMessage(sessionId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (message: string) =>
      submitMessage(sessionId, message, generateClientTurnId()),
    onSuccess: () => {
      // Invalidate timeline so it refetches latest messages
      qc.invalidateQueries({ queryKey: ["timeline", sessionId] });
    },
  });
}

export function useCreateRole() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: createRole,
    onSuccess: () => qc.invalidateQueries({ queryKey: ["roles"] }),
  });
}

export function useUpdateRole() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ roleID, role }: { roleID: string; role: import("./types").RoleSpec }) =>
      updateRole(roleID, role),
    onSuccess: (_data, vars) => {
      qc.invalidateQueries({ queryKey: ["roles"] });
      qc.invalidateQueries({ queryKey: ["roles", vars.roleID, "versions"] });
    },
  });
}

export function useDeleteRole() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (roleID: string) => deleteRole(roleID),
    onSuccess: (_data, roleID) => {
      qc.invalidateQueries({ queryKey: ["roles"] });
      qc.removeQueries({ queryKey: ["roles", roleID, "versions"] });
    },
  });
}
