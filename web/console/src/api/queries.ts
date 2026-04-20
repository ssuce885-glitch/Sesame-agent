import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import {
  ensureCurrentSession,
  getTimeline,
  getContextHistory,
  getWorkspace,
  getWorkspaceMailbox,
  getWorkspaceRuntimeGraph,
  loadContextHistory,
  reopenContext,
  submitMessage,
  submitPermission,
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

export function useWorkspaceMailbox() {
  return useQuery({
    queryKey: ["workspace-mailbox"],
    queryFn: getWorkspaceMailbox,
    staleTime: 10_000,
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

export function useSubmitPermission(sessionId: string) {
  return useMutation({
    mutationFn: ({ requestId, decision }: { requestId: string; decision: string }) =>
      submitPermission(sessionId, requestId, decision),
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
