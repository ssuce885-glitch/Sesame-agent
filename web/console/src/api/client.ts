import type {
  SessionListResponse,
  CreateSessionResponse,
  DeleteSessionResponse,
  TimelineResponse,
  MetricsOverview,
  MetricsTimeseries,
  Workspace,
  RoleListResponse,
  RoleSpec,
} from "./types";

// ─── Base fetch ────────────────────────────────────────────────────────────────

const BASE_URL = (import.meta.env.VITE_API_BASE_URL as string | undefined)?.replace(
  /\/$/,
  "",
) ?? "";

async function apiFetch<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(`${BASE_URL}${path}`, {
    ...init,
    headers: {
      "Content-Type": "application/json",
      ...(init?.headers ?? {}),
    },
  });
  if (!response.ok) {
    throw new Error((await response.text()) || `HTTP ${response.status}`);
  }
  if (response.status === 204) {
    return undefined as T;
  }
  const text = await response.text();
  if (!text) {
    return undefined as T;
  }
  return JSON.parse(text) as T;
}

// ─── Sessions ─────────────────────────────────────────────────────────────────

export function getSessions(): Promise<SessionListResponse> {
  return apiFetch<SessionListResponse>("/v1/sessions");
}

export function createSession(
  workspaceRoot: string,
): Promise<CreateSessionResponse> {
  return apiFetch<CreateSessionResponse>("/v1/sessions", {
    method: "POST",
    body: JSON.stringify({ workspace_root: workspaceRoot }),
  });
}

export function selectSession(sessionId: string): Promise<{ selected_session_id: string }> {
  return apiFetch(`/v1/sessions/${sessionId}/select`, { method: "POST" });
}

export function deleteSession(sessionId: string): Promise<DeleteSessionResponse> {
  return apiFetch<DeleteSessionResponse>(`/v1/sessions/${sessionId}`, {
    method: "DELETE",
  });
}

// ─── Session-scoped (uses X-Session-Binding header) ───────────────────────────

export function getTimeline(sessionId: string): Promise<TimelineResponse> {
  return apiFetch<TimelineResponse>("/v1/session/timeline", {
    headers: { "X-Session-Binding": sessionId },
  });
}

export function getWorkspace(sessionId: string): Promise<Workspace> {
  return apiFetch<Workspace>("/v1/session/workspace", {
    headers: { "X-Session-Binding": sessionId },
  });
}

export function submitMessage(
  sessionId: string,
  message: string,
  clientTurnId: string,
): Promise<{ id: string }> {
  return apiFetch<{ id: string }>("/v1/session/turns", {
    method: "POST",
    headers: { "X-Session-Binding": sessionId },
    body: JSON.stringify({ client_turn_id: clientTurnId, message }),
  });
}

export function submitPermission(
  sessionId: string,
  requestId: string,
  decision: string,
): Promise<{ request: unknown; turn_id: string; resumed: boolean }> {
  return apiFetch(`/v1/permissions/decide`, {
    method: "POST",
    headers: { "X-Session-Binding": sessionId },
    body: JSON.stringify({ request_id: requestId, decision, session_id: sessionId }),
  });
}

export function fileContentUrl(sessionId: string, path: string): string {
  const params = new URLSearchParams({ path });
  return `${BASE_URL}/v1/session/files/content?${params.toString()}`;
}

// ─── SSE stream ────────────────────────────────────────────────────────────────

export function openEventStream(sessionId: string, afterSeq: number): EventSource {
  return new EventSource(
    `${BASE_URL}/v1/session/events?after=${afterSeq}`,
  );
}

// ─── Metrics ──────────────────────────────────────────────────────────────────

export function getMetricsOverview(sessionId?: string): Promise<MetricsOverview> {
  const params = sessionId ? `?session_id=${encodeURIComponent(sessionId)}` : "";
  return apiFetch<MetricsOverview>(`/v1/metrics/overview${params}`);
}

export function getMetricsTimeseries(
  sessionId?: string,
  bucket = "day",
): Promise<MetricsTimeseries> {
  const params = new URLSearchParams({ bucket });
  if (sessionId) params.set("session_id", sessionId);
  return apiFetch<MetricsTimeseries>(`/v1/metrics/timeseries?${params.toString()}`);
}

// ─── Roles ────────────────────────────────────────────────────────────────────

export function listRoles(): Promise<RoleListResponse> {
  return apiFetch<RoleListResponse>("/v1/roles");
}

export function createRole(role: RoleSpec): Promise<RoleSpec> {
  return apiFetch<RoleSpec>("/v1/roles", {
    method: "POST",
    body: JSON.stringify(role),
  });
}

export function updateRole(roleID: string, role: RoleSpec): Promise<RoleSpec> {
  return apiFetch<RoleSpec>(`/v1/roles/${encodeURIComponent(roleID)}`, {
    method: "PUT",
    body: JSON.stringify(role),
  });
}

export function deleteRole(roleID: string): Promise<void> {
  return apiFetch<void>(`/v1/roles/${encodeURIComponent(roleID)}`, {
    method: "DELETE",
  });
}

// ─── Utilities ─────────────────────────────────────────────────────────────────

export function generateClientTurnId(): string {
  return `turn-${crypto.randomUUID()}`;
}
