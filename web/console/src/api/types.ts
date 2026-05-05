// Session
export interface SessionInfo {
  id: string;
  workspace_root: string;
  state: string;
  active_turn_id?: string;
  created_at: string;
  updated_at: string;
  queue?: QueuePayload;
}

export interface QueuePayload {
  active_turn_id?: string;
  active_turn_kind?: string;
  queue_depth: number;
  queued_user_turns: number;
  queued_report_batches: number;
}

// Timeline
export interface TimelineBlock {
  kind: string;
  text?: string;
  title?: string;
  status?: string;
  content?: TimelineContent[];
}

export interface TimelineContent {
  type: string;
  text?: string;
  tool_name?: string;
  args_preview?: string;
  result_preview?: string;
  tool_call_id?: string;
  status?: string;
}

export interface TimelineResponse {
  blocks: TimelineBlock[];
  latest_seq: number;
  queued_report_count: number;
  queue: QueuePayload;
}

// Context Preview
export interface ContextPromptItem {
  role: string;
  source_ref: string;
  content_preview: string;
  approx_tokens: number;
}

export interface ContextPreviewBlock {
  id: string;
  type: string;
  owner: string;
  visibility: string;
  source_ref: string;
  status: "included" | "available" | "excluded" | string;
  reason?: string;
  title?: string;
  summary?: string;
  importance_score?: number;
  updated_at?: string;
}

export interface ContextPreview {
  session_id: string;
  workspace_root: string;
  generated_at: string;
  approx_tokens: number;
  prompt: ContextPromptItem[];
  blocks: ContextPreviewBlock[];
}

export interface ContextBlock {
  id: string;
  workspace_root: string;
  type: string;
  owner: string;
  visibility: string;
  source_ref: string;
  title?: string;
  summary?: string;
  evidence?: string;
  confidence: number;
  importance_score: number;
  expiry_policy?: string;
  expires_at?: string;
  created_at: string;
  updated_at: string;
}

export interface ContextBlockFilters {
  owner?: string;
  visibility?: string;
  type?: string;
  limit?: number;
}

// SSE Events
export interface SSEEvent {
  id: string;
  seq: number;
  type: string;
  payload: Record<string, unknown>;
}

// Turn
export interface Turn {
  id: string;
  session_id: string;
  kind: string;
  state: string;
  user_message: string;
  created_at: string;
  updated_at: string;
}

// Task
export interface Task {
  id: string;
  workspace_root: string;
  session_id: string;
  role_id?: string;
  turn_id?: string;
  parent_session_id?: string;
  parent_turn_id?: string;
  report_session_id?: string;
  kind: string;
  state: string;
  prompt: string;
  output_path?: string;
  final_text?: string;
  outcome?: string;
  created_at: string;
  updated_at: string;
}

export interface TaskListFilters {
  state?: string;
  role_id?: string;
  session_id?: string;
  limit?: number;
}

export interface TaskTraceLink {
  session_id?: string;
  turn_id?: string;
}

export interface TaskTraceRole extends TaskTraceLink {
  id?: string;
}

export interface TaskTraceState {
  task: string;
  turn?: string;
  session?: string;
  queue?: QueuePayload;
}

export interface TaskTraceMessage {
  id?: number;
  session_id: string;
  turn_id?: string;
  role: string;
  content: string;
  tool_call_id?: string;
  position: number;
  created_at: string;
}

export interface TaskTraceEvent {
  id: string;
  seq: number;
  session_id: string;
  turn_id?: string;
  type: string;
  time: string;
  payload: string;
}

export interface TaskTrace {
  task: Task;
  parent: TaskTraceLink;
  role: TaskTraceRole;
  state: TaskTraceState;
  messages: TaskTraceMessage[];
  events: TaskTraceEvent[];
  reports: Report[];
  log_preview?: string;
  log_path?: string;
  log_bytes?: number;
  log_truncated?: boolean;
}

// Role
export interface RoleSpec {
  id: string;
  name: string;
  description: string;
  system_prompt: string;
  permission_profile?: string;
  model?: string;
  max_tool_calls?: number;
  max_runtime?: number;
  max_context_tokens?: number;
  skill_names?: string[];
  denied_tools?: string[];
  allowed_tools?: string[];
  denied_paths?: string[];
  allowed_paths?: string[];
  can_delegate: boolean;
  automation_ownership?: string[];
  version?: number;
}

export interface RoleInput {
  id: string;
  name: string;
  description?: string;
  system_prompt: string;
  permission_profile?: string;
  model?: string;
  max_tool_calls?: number;
  max_runtime?: number;
  max_context_tokens?: number;
  skill_names?: string[];
  denied_tools?: string[];
  allowed_tools?: string[];
  denied_paths?: string[];
  allowed_paths?: string[];
  can_delegate?: boolean;
  automation_ownership?: string[];
}

// Automation
export interface Automation {
  id: string;
  workspace_root: string;
  title: string;
  goal: string;
  state: string;
  owner: string;
  workflow_id?: string;
  watcher_path: string;
  watcher_cron: string;
  created_at: string;
  updated_at: string;
}

export interface AutomationRun {
  automation_id: string;
  dedupe_key: string;
  task_id: string;
  workflow_run_id?: string;
  status: string;
  summary: string;
  created_at: string;
}

// Workflow
export interface Workflow {
  id: string;
  workspace_root: string;
  name: string;
  trigger: string;
  owner_role?: string;
  input_schema?: string;
  steps?: string;
  required_tools?: string;
  approval_policy?: string;
  report_policy?: string;
  failure_policy?: string;
  resume_policy?: string;
  created_at: string;
  updated_at: string;
}

export interface WorkflowRun {
  id: string;
  workflow_id: string;
  workspace_root: string;
  state: string;
  trigger_ref?: string;
  task_ids?: string;
  report_ids?: string;
  approval_ids?: string;
  trace?: string;
  created_at: string;
  updated_at: string;
}

export interface WorkflowTriggerInput {
  trigger_ref?: string;
}

export interface WorkflowRunFilters {
  workflow_id?: string;
  state?: string;
  limit?: number;
}

// Report
export interface Report {
  id: string;
  session_id: string;
  source_kind: string;
  source_id: string;
  title: string;
  summary: string;
  severity: string;
  status: string;
  delivered: boolean;
  created_at: string;
}

export interface ReportsResponse {
  items: Report[];
  queued_count: number;
}

// Project State
export interface ProjectState {
  workspace_root: string;
  summary: string;
  source_session_id?: string;
  source_turn_id?: string;
  created_at: string;
  updated_at: string;
}

// Memory
export interface Memory {
  id: string;
  workspace_root: string;
  kind: string;
  content: string;
  source?: string;
  confidence: number;
  created_at: string;
  updated_at: string;
}

// Status
export interface StatusResponse {
  status: string;
  addr: string;
  model: string;
  permission_profile: string;
  default_session_id: string;
  queue?: QueuePayload;
}

// Setting
export interface SettingResponse {
  key: string;
  value: string;
}
