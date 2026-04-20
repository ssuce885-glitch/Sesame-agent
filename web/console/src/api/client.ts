import type {
  CreateSessionResponse,
  TimelineResponse,
  MetricsOverview,
  MetricsTimeseries,
  Workspace,
  ContextHistoryResponse,
  ContextHead,
  WorkspaceMailboxResponse,
  WorkspaceRuntimeGraphResponse,
  RoleListResponse,
  RoleSpec,
} from "./types";

// ─── Base fetch ────────────────────────────────────────────────────────────────

const BASE_URL = (import.meta.env.VITE_API_BASE_URL as string | undefined)?.replace(
  /\/$/,
  "",
) ?? "";

async function apiFetch<T>(path: string, init?: RequestInit): Promise<T> {
  const headers = new Headers(init?.headers);
  if (init?.body != null && !headers.has("Content-Type")) {
    headers.set("Content-Type", "application/json");
  }

  const response = await fetch(`${BASE_URL}${path}`, {
    ...init,
    headers,
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

function contextBindingHeaders(sessionId: string): HeadersInit {
  return { "X-Sesame-Context-Binding": sessionId };
}

export async function getWorkspace(): Promise<Workspace> {
  return apiFetch<Workspace>("/v1/workspace");
}

function isNotFoundError(err: unknown): boolean {
  return err instanceof Error && (err.message.includes("HTTP 404") || err.message.includes("not found"));
}

export async function createSession(
  workspaceRoot: string,
): Promise<CreateSessionResponse> {
  const resolvedWorkspaceRoot =
    workspaceRoot.trim() !== ""
      ? workspaceRoot.trim()
      : (await getWorkspace()).workspace_root;

  return apiFetch<CreateSessionResponse>("/v1/session/ensure", {
    method: "POST",
    body: JSON.stringify({ workspace_root: resolvedWorkspaceRoot }),
  });
}

export function ensureCurrentSession(): Promise<CreateSessionResponse> {
  return createSession("");
}

// ─── Session-scoped ────────────────────────────────────────────────────────────

export function getTimeline(sessionId: string): Promise<TimelineResponse> {
  return apiFetch<TimelineResponse>("/v1/session/timeline", {
    headers: contextBindingHeaders(sessionId),
  });
}

export function getContextHistory(sessionId: string): Promise<ContextHistoryResponse> {
  return apiFetch<ContextHistoryResponse>("/v1/session/history", {
    headers: contextBindingHeaders(sessionId),
  });
}

export function reopenContext(sessionId: string): Promise<ContextHead> {
  return apiFetch<ContextHead>("/v1/session/reopen", {
    method: "POST",
    headers: contextBindingHeaders(sessionId),
    body: JSON.stringify({}),
  });
}

export function loadContextHistory(sessionId: string, headId: string): Promise<ContextHead> {
  return apiFetch<ContextHead>("/v1/session/history/load", {
    method: "POST",
    headers: contextBindingHeaders(sessionId),
    body: JSON.stringify({ head_id: headId }),
  });
}

export function submitMessage(
  sessionId: string,
  message: string,
  clientTurnId: string,
): Promise<{ id: string }> {
  return apiFetch<{ id: string }>("/v1/session/turns", {
    method: "POST",
    headers: contextBindingHeaders(sessionId),
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
    headers: contextBindingHeaders(sessionId),
    body: JSON.stringify({ request_id: requestId, decision, session_id: sessionId }),
  });
}

export function fileContentUrl(sessionId: string, path: string): string {
  const params = new URLSearchParams({ path });
  return `${BASE_URL}/v1/session/files/content?${params.toString()}`;
}

// ─── SSE stream ────────────────────────────────────────────────────────────────

export function openEventStream(sessionId: string, afterSeq: number): EventSource {
  const params = new URLSearchParams({
    after: String(afterSeq),
    binding: sessionId.trim(),
  });
  return new EventSource(`${BASE_URL}/v1/session/events?${params.toString()}`);
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

export function getWorkspaceRuntimeGraph(): Promise<WorkspaceRuntimeGraphResponse> {
  return apiFetch<WorkspaceRuntimeGraphResponse>("/v1/runtime_graph");
}

export function getWorkspaceMailbox(): Promise<WorkspaceMailboxResponse> {
  return apiFetch<WorkspaceMailboxResponse>("/v1/mailbox");
}

// ─── Roles ────────────────────────────────────────────────────────────────────

export function listRoles(): Promise<RoleListResponse> {
  return apiFetch<RoleListResponse>("/v1/roles");
}

export function getRole(roleID: string): Promise<RoleSpec> {
  return apiFetch<RoleSpec>(`/v1/roles/${encodeURIComponent(roleID)}`);
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
