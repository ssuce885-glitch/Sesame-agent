import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import {
  ensureCurrentSession,
  getTimeline,
  getWorkspace,
  submitMessage,
  submitPermission,
  getMetricsOverview,
  getMetricsTimeseries,
  getRole,
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
    onSuccess: () => qc.invalidateQueries({ queryKey: ["roles"] }),
  });
}

export function useDeleteRole() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (roleID: string) => deleteRole(roleID),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["roles"] }),
  });
}
