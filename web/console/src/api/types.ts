// ─── Session ──────────────────────────────────────────────────────────────────

export interface Session {
  id: string;
  title?: string;
  last_preview?: string;
  workspace_root: string;
  state: string;
  active_turn_id?: string;
  updated_at: string;
  is_selected: boolean;
}

export interface SessionListResponse {
  sessions: Session[];
  selected_session_id?: string;
}

export interface CreateSessionResponse {
  id: string;
  workspace_root: string;
}

export interface DeleteSessionResponse {
  deleted_session_id: string;
  selected_session_id?: string;
}

// ─── Timeline ─────────────────────────────────────────────────────────────────

export interface TimelineBlock {
  id: string;
  turn_id?: string;
  run_id?: string;
  kind:
    | "user_message"
    | "reasoning"
    | "assistant_message"
    | "notice"
    | "error"
    | "plan_block"
    | "task_block"
    | "tool_call"
    | "tool_run_block"
    | "permission_block"
    | "worktree_block";
  status?: string;
  title?: string;
  text?: string;
  tool_call_id?: string;
  tool_run_id?: string;
  tool_name?: string;
  task_id?: string;
  plan_id?: string;
  worktree_id?: string;
  permission_request_id?: string;
  requested_profile?: string;
  decision?: string;
  decision_scope?: string;
  reason?: string;
  path?: string;
  args_preview?: string;
  result_preview?: string;
  content?: ContentBlock[];
  usage?: TokenUsage;
}

export interface TimelineResponse {
  blocks: TimelineBlock[];
  latest_seq: number;
  pending_report_count: number;
  queue: QueueSummary;
}

export interface QueueSummary {
  active_turn_id?: string;
  active_turn_kind?: string;
  queue_depth: number;
  queued_user_turns: number;
  queued_child_report_batches: number;
  pending_child_reports: number;
}

// ─── Content Blocks ────────────────────────────────────────────────────────────

export interface TextContentBlock {
  type: "text";
  text: string;
}

export interface ToolCallContentBlock {
  type: "tool_call";
  tool_call_id: string;
  tool_name: string;
  args_preview?: string;
  result_preview?: string;
  status?: string;
}

export interface ImageContentBlock {
  type: "image";
  path?: string;
  url?: string;
  mime_type?: string;
  width?: number;
  height?: number;
  size_bytes?: number;
}

export type ContentBlock = TextContentBlock | ToolCallContentBlock | ImageContentBlock;

// ─── SSE Events ────────────────────────────────────────────────────────────────

export interface ServerEvent<T = unknown> {
  id: string;
  seq: number;
  session_id: string;
  turn_id?: string;
  type: string;
  time: string;
  payload: T;
}

export interface ToolEventPayload {
  tool_call_id: string;
  tool_name: string;
  arguments?: string;
  arguments_raw?: string;
  arguments_recovery?: string;
  result_preview?: string;
  is_error?: boolean;
}

export interface DeltaPayload {
  text: string;
}

export interface FailurePayload {
  message: string;
}

export interface PermissionRequestPayload {
  request_id?: string;
  tool_run_id?: string;
  tool_call_id?: string;
  tool_name?: string;
  requested_profile: string;
  reason?: string;
  turn_id?: string;
}

export interface PermissionResolvedPayload {
  request_id: string;
  tool_run_id?: string;
  tool_call_id?: string;
  tool_name?: string;
  requested_profile: string;
  decision: string;
  decision_scope?: string;
  effective_profile?: string;
  turn_id?: string;
}

export interface SessionMemoryPayload {
  source_turn_id?: string;
  workspace_root?: string;
  async?: boolean;
  updated?: boolean;
  workspace_entries_upserted?: number;
  global_entries_upserted?: number;
  workspace_entries_pruned?: number;
  message?: string;
}

export interface NoticePayload {
  text: string;
}

// ─── Metrics ──────────────────────────────────────────────────────────────────

export interface MetricsOverview {
  input_tokens: number;
  output_tokens: number;
  cached_tokens: number;
  cache_hit_rate: number;
}

export interface TimeseriesPoint {
  bucket_start: string;
  input_tokens: number;
  output_tokens: number;
  cached_tokens: number;
}

export interface MetricsTimeseries {
  bucket: string;
  points: TimeseriesPoint[];
}

export interface MetricsTurnRow {
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

export interface MetricsTurnsResponse {
  items: MetricsTurnRow[];
  page: number;
  page_size: number;
  total_count: number;
}

// ─── Roles ────────────────────────────────────────────────────────────────────

export interface RoleSpec {
  role_id: string;
  display_name: string;
  description: string;
  prompt: string;
  skills: string[];
}

export interface RoleSummary {
  role_id: string;
  display_name: string;
  description: string;
  skills: string[];
}

export interface RoleDiagnostic {
  role_id: string;
  path: string;
  error: string;
}

export interface RoleListResponse {
  roles: RoleSummary[];
  diagnostics: RoleDiagnostic[];
}

// ─── Token Usage ───────────────────────────────────────────────────────────────

export interface TokenUsage {
  provider?: string;
  model?: string;
  input_tokens: number;
  output_tokens: number;
  cached_tokens: number;
  cache_hit_rate: number;
}

// ─── Workspace ─────────────────────────────────────────────────────────────────

export interface Workspace {
  id?: string;
  name?: string;
  session_id?: string;
  workspace_root: string;
  provider?: string;
  model?: string;
  permission_profile?: string;
  provider_cache_profile?: string;
}
