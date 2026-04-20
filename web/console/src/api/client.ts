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

async function getRuntimeWorkspace(): Promise<Workspace> {
  return apiFetch<Workspace>("/v1/workspace");
}

function isNotFoundError(err: unknown): boolean {
  return err instanceof Error && (err.message.includes("HTTP 404") || err.message.includes("not found"));
}

// ─── Sessions ─────────────────────────────────────────────────────────────────

export async function getSessions(): Promise<SessionListResponse> {
  try {
    return await apiFetch<SessionListResponse>("/v1/sessions");
  } catch (err) {
    if (!isNotFoundError(err)) {
      throw err;
    }

    let workspace: Workspace;
    try {
      workspace = await getRuntimeWorkspace();
    } catch {
      throw new Error("Sessions unavailable and workspace endpoint not reachable.");
    }
    const session = await createSession(workspace.workspace_root);
    return {
      sessions: [
        {
          id: session.id,
          title: workspace.name || "Main session",
          workspace_root: session.workspace_root,
          state: "idle",
          updated_at: "1970-01-01T00:00:00Z",
          is_selected: true,
        },
      ],
      selected_session_id: session.id,
    };
  }
}

export async function createSession(
  workspaceRoot: string,
): Promise<CreateSessionResponse> {
  const resolvedWorkspaceRoot =
    workspaceRoot.trim() !== ""
      ? workspaceRoot.trim()
      : (await getRuntimeWorkspace()).workspace_root;

  return apiFetch<CreateSessionResponse>("/v1/session/ensure", {
    method: "POST",
    body: JSON.stringify({ workspace_root: resolvedWorkspaceRoot }),
  });
}

export async function selectSession(sessionId: string): Promise<{ selected_session_id: string }> {
  try {
    return await apiFetch(`/v1/sessions/${sessionId}/select`, { method: "POST" });
  } catch (err) {
    throw err;
  }
}

export function deleteSession(sessionId: string): Promise<DeleteSessionResponse> {
  return apiFetch<DeleteSessionResponse>(`/v1/sessions/${sessionId}`, {
    method: "DELETE",
  });
}

// ─── Session-scoped ────────────────────────────────────────────────────────────

export function getTimeline(sessionId: string): Promise<TimelineResponse> {
  return apiFetch<TimelineResponse>("/v1/session/timeline", {
    headers: { "X-Sesame-Context-Binding": sessionId },
  });
}

export function getWorkspace(sessionId: string): Promise<Workspace> {
  return apiFetch<Workspace>("/v1/session/workspace", {
    headers: { "X-Sesame-Context-Binding": sessionId },
  });
}

export function submitMessage(
  sessionId: string,
  message: string,
  clientTurnId: string,
): Promise<{ id: string }> {
  return apiFetch<{ id: string }>("/v1/session/turns", {
    method: "POST",
    headers: { "X-Sesame-Context-Binding": sessionId },
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
    headers: { "X-Sesame-Context-Binding": sessionId },
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
