export interface 会话项 {
  id: string;
  title?: string;
  last_preview?: string;
  workspace_root: string;
  state: string;
  active_turn_id?: string;
  updated_at: string;
  is_selected: boolean;
}

export interface 会话列表响应 {
  sessions: 会话项[];
  selected_session_id?: string;
}

export interface 删除会话响应 {
  deleted_session_id: string;
  selected_session_id?: string;
}

export interface 时间线块 {
  id: string;
  turn_id?: string;
  kind: "user_message" | "reasoning" | "assistant_message" | "notice" | "error";
  status?: string;
  text?: string;
  tool_call_id?: string;
  tool_name?: string;
  args_preview?: string;
  result_preview?: string;
  content?: AssistantContentBlock[];
  usage?: Token用量;
}

export interface 文本内容块 {
  type: "text";
  text: string;
}

export interface 工具调用内容块 {
  type: "tool_call";
  tool_call_id: string;
  tool_name: string;
  args_preview?: string;
  result_preview?: string;
  status?: string;
}

export type AssistantContentBlock = 文本内容块 | 工具调用内容块;

export interface 时间线响应 {
  blocks: 时间线块[];
  latest_seq: number;
}

export interface 工作区响应 {
  session_id: string;
  workspace_root: string;
  provider?: string;
  model?: string;
  permission_profile?: string;
  provider_cache_profile?: string;
}

export interface Token用量 {
  provider: string;
  model: string;
  input_tokens: number;
  output_tokens: number;
  cached_tokens: number;
  cache_hit_rate: number;
}

export interface 统计概览响应 {
  input_tokens: number;
  output_tokens: number;
  cached_tokens: number;
  cache_hit_rate: number;
}

export interface 统计时序点 {
  bucket_start: string;
  input_tokens: number;
  output_tokens: number;
  cached_tokens: number;
}

export interface 统计时序响应 {
  bucket: string;
  points: 统计时序点[];
}

export interface 统计明细项 {
  session_id: string;
  session_title?: string;
  turn_id: string;
  provider: string;
  model: string;
  input_tokens: number;
  output_tokens: number;
  cached_tokens: number;
  cache_hit_rate: number;
  created_at: string;
}

export interface 统计明细响应 {
  items: 统计明细项[];
  page: number;
  page_size: number;
  total_count: number;
}

export interface 服务端事件<T = unknown> {
  id: string;
  seq: number;
  session_id: string;
  turn_id?: string;
  type: string;
  time: string;
  payload: T;
}

export interface 工具事件负载 {
  tool_call_id: string;
  tool_name: string;
  arguments?: string;
  result_preview?: string;
}

export interface 回复增量负载 {
  text: string;
}

export interface 失败负载 {
  message: string;
}

const 基础地址 = (import.meta.env.VITE_API_BASE_URL as string | undefined)?.replace(/\/$/, "") ?? "";

async function 请求JSON<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(`${基础地址}${path}`, {
    ...init,
    headers: {
      "Content-Type": "application/json",
      ...(init?.headers ?? {}),
    },
  });

  if (!response.ok) {
    throw new Error((await response.text()) || `HTTP ${response.status}`);
  }
  return (await response.json()) as T;
}

export function 打开事件流(sessionId: string, after: number) {
  return new EventSource(`${基础地址}/v1/sessions/${sessionId}/events?after=${after}`);
}

export function 获取会话列表() {
  return 请求JSON<会话列表响应>("/v1/sessions");
}

export function 获取时间线(sessionId: string) {
  return 请求JSON<时间线响应>(`/v1/sessions/${sessionId}/timeline`);
}

export function 获取工作区(sessionId: string) {
  return 请求JSON<工作区响应>(`/v1/sessions/${sessionId}/workspace`);
}

export function 创建会话(workspaceRoot: string) {
  return 请求JSON<{ id: string; workspace_root: string }>(`/v1/sessions`, {
    method: "POST",
    body: JSON.stringify({ workspace_root: workspaceRoot }),
  });
}

export function 选择会话(sessionId: string) {
  return 请求JSON<{ selected_session_id: string }>(`/v1/sessions/${sessionId}/select`, {
    method: "POST",
  });
}

export function 删除会话(sessionId: string) {
  return 请求JSON<删除会话响应>(`/v1/sessions/${sessionId}`, {
    method: "DELETE",
  });
}

export function 提交消息(sessionId: string, message: string) {
  return 请求JSON<{ id: string }>(`/v1/sessions/${sessionId}/turns`, {
    method: "POST",
    body: JSON.stringify({
      client_turn_id: `turn-${Date.now()}`,
      message,
    }),
  });
}

export function 获取统计概览(sessionId?: string) {
  const params = new URLSearchParams();
  if (sessionId) {
    params.set("session_id", sessionId);
  }
  const suffix = params.toString();
  return 请求JSON<统计概览响应>(`/v1/metrics/overview${suffix ? `?${suffix}` : ""}`);
}

export function 获取统计时序(sessionId?: string) {
  const params = new URLSearchParams();
  params.set("bucket", "day");
  if (sessionId) {
    params.set("session_id", sessionId);
  }
  return 请求JSON<统计时序响应>(`/v1/metrics/timeseries?${params.toString()}`);
}

export function 获取统计明细(sessionId?: string, page = 1, pageSize = 20) {
  const params = new URLSearchParams();
  params.set("page", String(page));
  params.set("page_size", String(pageSize));
  if (sessionId) {
    params.set("session_id", sessionId);
  }
  return 请求JSON<统计明细响应>(`/v1/metrics/turns?${params.toString()}`);
}
