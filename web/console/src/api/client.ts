import type {
  Automation,
  AutomationRun,
  Memory,
  ProjectState,
  RoleInput,
  ReportsResponse,
  RoleSpec,
  SSEEvent,
  SessionInfo,
  SettingResponse,
  StatusResponse,
  Task,
  TaskListFilters,
  TaskTrace,
  TimelineResponse,
  Turn,
} from "./types";

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

// Sessions
export async function ensureSession(workspaceRoot: string): Promise<SessionInfo> {
  return apiFetch<SessionInfo>("/v2/sessions", {
    method: "POST",
    body: JSON.stringify({ workspace_root: workspaceRoot }),
  });
}

export async function getSession(sessionId: string): Promise<SessionInfo> {
  return apiFetch<SessionInfo>(`/v2/sessions/${encodeURIComponent(sessionId)}`);
}

// Timeline
export async function getTimeline(sessionId: string): Promise<TimelineResponse> {
  return apiFetch<TimelineResponse>(`/v2/sessions/${encodeURIComponent(sessionId)}/timeline`);
}

// Turns
export async function submitTurn(sessionId: string, message: string): Promise<Turn> {
  return apiFetch<Turn>("/v2/turns", {
    method: "POST",
    body: JSON.stringify({ session_id: sessionId, message }),
  });
}

export async function interruptTurn(turnId: string): Promise<{ status: string }> {
  return apiFetch<{ status: string }>(`/v2/turns/${encodeURIComponent(turnId)}/interrupt`, {
    method: "POST",
  });
}

// SSE stream
export function openEventStream(
  sessionId: string,
  afterSeq: number,
  onEvent: (event: SSEEvent) => void,
  onOpen: () => void,
  onError: (err: unknown) => void,
): { close: () => void } {
  const params = new URLSearchParams({
    session_id: sessionId,
    after: String(afterSeq),
  });
  const url = `${BASE_URL}/v2/events?${params.toString()}`;
  const controller = new AbortController();

  fetch(url, { signal: controller.signal })
    .then(async (response) => {
      if (!response.ok || !response.body) {
        throw new Error(`SSE connection failed: ${response.status}`);
      }
      onOpen();
      const reader = response.body.getReader();
      const decoder = new TextDecoder();
      let buffer = "";
      const current: SSERecord = { id: "", event: "", dataLines: [] };

      while (true) {
        const { done, value } = await reader.read();
        if (done) break;
        buffer += decoder.decode(value, { stream: true });
        const lines = buffer.split("\n");
        buffer = lines.pop() ?? "";

        for (const line of lines) {
          consumeSSELine(line, current, onEvent);
        }
      }
    })
    .catch((err) => {
      if ((err as Error)?.name !== "AbortError") {
        onError(err);
      }
    });

  return { close: () => controller.abort() };
}

interface SSERecord {
  id: string;
  event: string;
  dataLines: string[];
}

export function parseSSEText(text: string): SSEEvent[] {
  const events: SSEEvent[] = [];
  const current: SSERecord = { id: "", event: "", dataLines: [] };
  for (const line of text.split("\n")) {
    consumeSSELine(line, current, (event) => events.push(event));
  }
  return events;
}

function consumeSSELine(rawLine: string, current: SSERecord, onEvent: (event: SSEEvent) => void) {
  const line = rawLine.endsWith("\r") ? rawLine.slice(0, -1) : rawLine;
  if (line === "") {
    dispatchSSERecord(current, onEvent);
    current.id = "";
    current.event = "";
    current.dataLines = [];
    return;
  }
  if (line.startsWith(":")) {
    return;
  }
  const colon = line.indexOf(":");
  const field = colon >= 0 ? line.slice(0, colon) : line;
  let value = colon >= 0 ? line.slice(colon + 1) : "";
  if (value.startsWith(" ")) {
    value = value.slice(1);
  }
  switch (field) {
    case "id":
      current.id = value.trim();
      break;
    case "event":
      current.event = value.trim();
      break;
    case "data":
      current.dataLines.push(value);
      break;
  }
}

function dispatchSSERecord(current: SSERecord, onEvent: (event: SSEEvent) => void) {
  if (current.dataLines.length === 0) {
    return;
  }
  try {
    const payload = JSON.parse(current.dataLines.join("\n"));
    onEvent({
      id: current.id,
      seq: parseInt(current.id, 10) || payloadSeq(payload),
      type: current.event || "message",
      payload,
    });
  } catch {
    // Skip malformed event payloads.
  }
}

function payloadSeq(payload: unknown): number {
  if (payload != null && typeof payload === "object" && "seq" in payload) {
    const value = (payload as { seq?: unknown }).seq;
    return typeof value === "number" && Number.isFinite(value) ? value : 0;
  }
  return 0;
}

// Tasks
export async function listTasks(workspaceRoot: string, filters: TaskListFilters = {}): Promise<Task[]> {
  const params = new URLSearchParams({ workspace_root: workspaceRoot });
  if (filters.state) {
    params.set("state", filters.state);
  }
  if (filters.role_id) {
    params.set("role_id", filters.role_id);
  }
  if (filters.session_id) {
    params.set("session_id", filters.session_id);
  }
  if (filters.limit && filters.limit > 0) {
    params.set("limit", String(filters.limit));
  }
  return apiFetch<Task[]>(`/v2/tasks?${params.toString()}`);
}

export async function createTask(task: Partial<Task>): Promise<Task> {
  return apiFetch<Task>("/v2/tasks", {
    method: "POST",
    body: JSON.stringify(task),
  });
}

export async function getTask(taskId: string): Promise<Task> {
  return apiFetch<Task>(`/v2/tasks/${encodeURIComponent(taskId)}`);
}

export async function getTaskTrace(taskId: string): Promise<TaskTrace> {
  return apiFetch<TaskTrace>(`/v2/tasks/${encodeURIComponent(taskId)}/trace`);
}

export async function cancelTask(taskId: string): Promise<Task> {
  return apiFetch<Task>(`/v2/tasks/${encodeURIComponent(taskId)}/cancel`, {
    method: "POST",
  });
}

// Memory
export async function createMemory(memory: Partial<Memory>): Promise<Memory> {
  return apiFetch<Memory>("/v2/memory", {
    method: "POST",
    body: JSON.stringify(memory),
  });
}

export async function searchMemory(query: string, workspaceRoot: string, limit = 50): Promise<Memory[]> {
  const params = new URLSearchParams({ q: query, workspace_root: workspaceRoot, limit: String(limit) });
  return apiFetch<Memory[]>(`/v2/memory?${params.toString()}`);
}

export async function deleteMemory(id: string): Promise<void> {
  await apiFetch<void>(`/v2/memory/${encodeURIComponent(id)}`, { method: "DELETE" });
}

// Project State
export async function getProjectState(workspaceRoot: string): Promise<ProjectState> {
  const params = new URLSearchParams({ workspace_root: workspaceRoot });
  return apiFetch<ProjectState>(`/v2/project_state?${params.toString()}`);
}

export async function updateProjectState(state: Partial<ProjectState>): Promise<ProjectState> {
  return apiFetch<ProjectState>("/v2/project_state", {
    method: "PUT",
    body: JSON.stringify(state),
  });
}

// Settings
export async function getSetting(key: string): Promise<SettingResponse> {
  return apiFetch<SettingResponse>(`/v2/settings/${encodeURIComponent(key)}`);
}

export async function setSetting(key: string, value: string): Promise<SettingResponse> {
  return apiFetch<SettingResponse>(`/v2/settings/${encodeURIComponent(key)}`, {
    method: "PUT",
    body: JSON.stringify({ value }),
  });
}

// Roles
export async function listRoles(): Promise<RoleSpec[]> {
  return apiFetch<RoleSpec[]>("/v2/roles");
}

export async function getRole(roleId: string): Promise<RoleSpec> {
  return apiFetch<RoleSpec>(`/v2/roles/${encodeURIComponent(roleId)}`);
}

export async function createRole(role: RoleInput): Promise<RoleSpec> {
  return apiFetch<RoleSpec>("/v2/roles", {
    method: "POST",
    body: JSON.stringify(role),
  });
}

export async function updateRole(roleId: string, role: RoleInput): Promise<RoleSpec> {
  return apiFetch<RoleSpec>(`/v2/roles/${encodeURIComponent(roleId)}`, {
    method: "PUT",
    body: JSON.stringify(role),
  });
}

// Automations
export async function listAutomations(workspaceRoot: string): Promise<Automation[]> {
  const params = new URLSearchParams({ workspace_root: workspaceRoot });
  return apiFetch<Automation[]>(`/v2/automations?${params.toString()}`);
}

export async function createAutomation(data: Partial<Automation>): Promise<Automation> {
  return apiFetch<Automation>("/v2/automations", {
    method: "POST",
    body: JSON.stringify(data),
  });
}

export async function listAutomationRuns(id: string, limit = 20): Promise<AutomationRun[]> {
  const params = new URLSearchParams({ limit: String(limit) });
  return apiFetch<AutomationRun[]>(`/v2/automations/${encodeURIComponent(id)}/runs?${params.toString()}`);
}

export async function pauseAutomation(id: string): Promise<Automation> {
  return apiFetch<Automation>(`/v2/automations/${encodeURIComponent(id)}/pause`, {
    method: "POST",
  });
}

export async function resumeAutomation(id: string): Promise<Automation> {
  return apiFetch<Automation>(`/v2/automations/${encodeURIComponent(id)}/resume`, {
    method: "POST",
  });
}

// Reports
export async function getReports(workspaceRoot: string): Promise<ReportsResponse> {
  const params = new URLSearchParams({ workspace_root: workspaceRoot });
  return apiFetch<ReportsResponse>(`/v2/reports?${params.toString()}`);
}

// Status
export async function getStatus(): Promise<StatusResponse> {
  return apiFetch<StatusResponse>("/v2/status");
}
