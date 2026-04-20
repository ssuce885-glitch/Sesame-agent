// ─── Current session ──────────────────────────────────────────────────────────

export interface CreateSessionResponse {
  id: string;
  workspace_root: string;
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

// ─── Runtime ──────────────────────────────────────────────────────────────────

export interface HistoryEntry {
  id: string;
  title?: string;
  preview?: string;
  source_kind?: string;
  is_current: boolean;
  created_at: string;
  updated_at: string;
}

export interface ContextHead {
  id: string;
  session_id: string;
  parent_head_id?: string;
  source_kind: string;
  title?: string;
  preview?: string;
  created_at: string;
  updated_at: string;
}

export interface ContextHistoryResponse {
  entries: HistoryEntry[];
  current_head_id?: string;
}

export interface RuntimeTask {
  id: string;
  run_id: string;
  plan_id?: string;
  parent_task_id?: string;
  state: string;
  title?: string;
  description?: string;
  owner?: string;
  kind?: string;
  execution_task_id?: string;
  worktree_id?: string;
  created_at: string;
  updated_at: string;
}

export interface RuntimePermissionRequest {
  id: string;
  session_id: string;
  turn_id: string;
  run_id?: string;
  task_id?: string;
  tool_run_id?: string;
  tool_call_id?: string;
  tool_name?: string;
  requested_profile: string;
  reason?: string;
  status: string;
  decision?: string;
  decision_scope?: string;
  resolved_at?: string;
  created_at: string;
  updated_at: string;
}

export interface RuntimeDiagnostic {
  id: string;
  session_id: string;
  turn_id: string;
  event_type: string;
  reason?: string;
  summary?: string;
  created_at: string;
}

export interface RuntimeGraph {
  runs: Array<{ id: string; session_id: string; turn_id?: string; state: string }>;
  plans: Array<{ id: string; run_id: string; state: string; title?: string }>;
  tasks: RuntimeTask[];
  tool_runs: Array<{ id: string; run_id: string; task_id?: string; state: string; tool_name: string }>;
  worktrees: Array<{ id: string; run_id: string; task_id?: string; state: string; worktree_path: string }>;
  incidents: unknown[];
  dispatch_attempts: unknown[];
  permission_requests: RuntimePermissionRequest[];
  diagnostics?: RuntimeDiagnostic[];
}

export interface WorkspaceRuntimeGraphResponse {
  workspace_root: string;
  graph: RuntimeGraph;
}

export interface ReportEnvelope {
  source?: string;
  status?: string;
  severity?: string;
  title?: string;
  summary?: string;
}

export interface WorkspaceMailboxItem {
  id: string;
  report_id?: string;
  delivery_id?: string;
  workspace_root: string;
  session_id: string;
  source_session_id?: string;
  source_role_id?: string;
  source_kind: string;
  source_id: string;
  channel?: string;
  delivery_state?: string;
  envelope: ReportEnvelope;
  observed_at?: string;
  injected_turn_id?: string;
  injected_at?: string;
  created_at?: string;
  updated_at?: string;
}

export interface WorkspaceMailboxResponse {
  workspace_root: string;
  items: WorkspaceMailboxItem[];
  pending_count: number;
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
