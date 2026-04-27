package tui

import (
	"time"
)

// Event is a session event from the daemon.
type Event struct {
	ID        string
	Seq       int64
	SessionID string `json:"session_id"`
	TurnID    string `json:"turn_id,omitempty"`
	Type      string
	Time      time.Time
	Payload   []byte
}

// SubmitTurnRequest is sent when the user submits a prompt.
type SubmitTurnRequest struct {
	Message string `json:"message"`
}

// Turn is a turn in the session.
type Turn struct {
	ID        string `json:"id"`
	SessionID string `json:"session_id"`
}

// StatusResponse is the runtime status.
type StatusResponse struct {
	Status               string `json:"status"`
	Provider             string `json:"provider,omitempty"`
	Model                string `json:"model,omitempty"`
	PermissionProfile    string `json:"permission_profile,omitempty"`
	ProviderCacheProfile string `json:"provider_cache_profile,omitempty"`
	PID                  int    `json:"pid,omitempty"`
}

// SessionTimelineResponse is the session timeline.
type SessionTimelineResponse struct {
	LatestSeq         int64           `json:"latest_seq"`
	Blocks            []TimelineBlock `json:"blocks"`
	QueuedReportCount int             `json:"queued_report_count"`
	Queue             QueueSummary    `json:"queue"`
}

// TimelineBlock is a block in the session timeline.
type TimelineBlock struct {
	Kind    string         `json:"kind"`
	Text    string         `json:"text,omitempty"`
	Title   string         `json:"title,omitempty"`
	Path    string         `json:"path,omitempty"`
	Status  string         `json:"status,omitempty"`
	Content []ContentBlock `json:"content,omitempty"`
}

// ContentBlock is a content block within an assistant message.
type ContentBlock struct {
	Type          string `json:"type"`
	Text          string `json:"text,omitempty"`
	ToolName      string `json:"tool_name,omitempty"`
	ArgsPreview   string `json:"args_preview,omitempty"`
	ResultPreview string `json:"result_preview,omitempty"`
	Status        string `json:"status,omitempty"`
	ToolCallID    string `json:"tool_call_id,omitempty"`
}

// ListContextHistoryResponse is the context history listing.
type ListContextHistoryResponse struct {
	Entries []HistoryEntry `json:"entries"`
}

// HistoryEntry is a single history entry.
type HistoryEntry struct {
	ID         string `json:"id"`
	Title      string `json:"title,omitempty"`
	Preview    string `json:"preview,omitempty"`
	SourceKind string `json:"source_kind,omitempty"`
	IsCurrent  bool   `json:"is_current"`
}

// ContextHead is the head of the context history after a load/reopen.
type ContextHead struct {
	ID string `json:"id"`
}

// ReportsResponse is the workspace reports.
type ReportsResponse struct {
	Items       []ReportDeliveryItem `json:"items"`
	QueuedCount int                  `json:"queued_count"`
	Reports     int                  `json:"reports"`
	Deliveries  int                  `json:"deliveries"`
}

// ReportDeliveryItem is a single delivered or queued report item.
type ReportDeliveryItem struct {
	ID             string         `json:"id"`
	SourceKind     string         `json:"source_kind"`
	InjectedTurnID string         `json:"injected_turn_id,omitempty"`
	ObservedAt     time.Time      `json:"observed_at"`
	Envelope       ReportEnvelope `json:"envelope"`
}

// ReportEnvelope is the envelope of a report item.
type ReportEnvelope struct {
	Title    string          `json:"title,omitempty"`
	Summary  string          `json:"summary,omitempty"`
	Status   string          `json:"status,omitempty"`
	Severity string          `json:"severity,omitempty"`
	Source   string          `json:"source,omitempty"`
	Sections []ReportSection `json:"sections,omitempty"`
}

// ReportSection is a section within a report.
type ReportSection struct {
	Title string   `json:"title,omitempty"`
	Text  string   `json:"text,omitempty"`
	Items []string `json:"items,omitempty"`
}

// ReportSectionContent is a flattened version of ReportSection for rendering.
type ReportSectionContent = ReportSection

// RuntimeGraphResponse is the runtime graph response.
type RuntimeGraphResponse struct {
	Graph RuntimeGraph `json:"graph"`
}

// RuntimeGraph is the runtime graph.
type RuntimeGraph struct {
	Runs        []Run               `json:"runs"`
	Diagnostics []RuntimeDiagnostic `json:"diagnostics"`
	Tasks       []Task              `json:"tasks"`
	ToolRuns    []ToolRun           `json:"tool_runs"`
	Worktrees   []Worktree          `json:"worktrees"`
}

// Run is a runtime run.
type Run struct {
	ID        string `json:"id"`
	State     string `json:"state"`
	Objective string `json:"objective,omitempty"`
	Result    string `json:"result,omitempty"`
	Error     string `json:"error,omitempty"`
}

// RuntimeDiagnostic is a runtime diagnostic entry.
type RuntimeDiagnostic struct {
	ID        string `json:"id"`
	EventType string `json:"event_type"`
	Summary   string `json:"summary,omitempty"`
	Reason    string `json:"reason,omitempty"`
	Severity  string `json:"severity,omitempty"`
	Category  string `json:"category,omitempty"`
	AssetKind string `json:"asset_kind,omitempty"`
	AssetID   string `json:"asset_id,omitempty"`
}

// Task is a runtime task.
type Task struct {
	ID              string `json:"id"`
	State           string `json:"state"`
	Title           string `json:"title,omitempty"`
	Owner           string `json:"owner,omitempty"`
	Kind            string `json:"kind,omitempty"`
	Description     string `json:"description,omitempty"`
	ExecutionTaskID string `json:"execution_task_id,omitempty"`
}

// ToolRun is a tool run in the runtime graph.
type ToolRun struct {
	ID         string `json:"id"`
	State      string `json:"state"`
	ToolName   string `json:"tool_name"`
	TaskID     string `json:"task_id,omitempty"`
	ToolCallID string `json:"tool_call_id,omitempty"`
	InputJSON  string `json:"input_json,omitempty"`
	OutputJSON string `json:"output_json,omitempty"`
	Error      string `json:"error,omitempty"`
	LockWaitMs int    `json:"lock_wait_ms,omitempty"`
}

// Worktree is a worktree in the runtime graph.
type Worktree struct {
	ID             string `json:"id"`
	State          string `json:"state"`
	WorktreeBranch string `json:"worktree_branch,omitempty"`
	WorktreePath   string `json:"worktree_path,omitempty"`
}

// CronListResponse is the response from listing cron jobs.
type CronListResponse struct {
	Jobs []CronJob `json:"jobs"`
}

// CronJob is a scheduled job.
type CronJob struct {
	ID            string `json:"id"`
	Name          string `json:"name,omitempty"`
	Enabled       bool   `json:"enabled"`
	Schedule      string `json:"schedule"`
	Timezone      string `json:"timezone,omitempty"`
	WorkspaceRoot string `json:"workspace_root,omitempty"`
	NextRunTime   string `json:"next_run_time,omitempty"`
	LastRunTime   string `json:"last_run_time,omitempty"`
	Status        string `json:"status,omitempty"`
	CreatedAt     string `json:"created_at,omitempty"`
}

// ReportingOverview is the reporting overview.
type ReportingOverview struct {
	ChildAgents  []ChildAgentSpec   `json:"child_agents"`
	ReportGroups []ReportGroup      `json:"report_groups"`
	ChildResults []ChildAgentResult `json:"child_results"`
	Digests      []DigestRecord     `json:"digests"`
}

// ChildAgentSpec is a child agent specification.
type ChildAgentSpec struct {
	AgentID  string       `json:"agent_id,omitempty"`
	Purpose  string       `json:"purpose,omitempty"`
	Mode     string       `json:"mode,omitempty"`
	Schedule ScheduleSpec `json:"schedule,omitempty"`
}

// ScheduleSpec is a schedule specification.
type ScheduleSpec struct {
	Kind         string `json:"kind"`
	Expr         string `json:"expr,omitempty"`
	At           string `json:"at,omitempty"`
	EveryMinutes int    `json:"every_minutes,omitempty"`
	Timezone     string `json:"timezone,omitempty"`
}

// ReportGroup is a report group.
type ReportGroup struct {
	GroupID  string       `json:"group_id,omitempty"`
	Title    string       `json:"title,omitempty"`
	Schedule ScheduleSpec `json:"schedule,omitempty"`
	Sources  []string     `json:"sources,omitempty"`
}

// ChildAgentResult is a child agent result.
type ChildAgentResult struct {
	ResultID string         `json:"result_id,omitempty"`
	AgentID  string         `json:"agent_id,omitempty"`
	Envelope ResultEnvelope `json:"envelope"`
}

// ResultEnvelope is the envelope of a result.
type ResultEnvelope struct {
	Title    string `json:"title,omitempty"`
	Summary  string `json:"summary,omitempty"`
	Status   string `json:"status,omitempty"`
	Severity string `json:"severity,omitempty"`
}

// DigestRecord is a digest record.
type DigestRecord struct {
	DigestID string         `json:"digest_id,omitempty"`
	GroupID  string         `json:"group_id,omitempty"`
	Envelope DigestEnvelope `json:"envelope"`
}

// DigestEnvelope is the envelope of a digest.
type DigestEnvelope = ResultEnvelope

// ToolEventPayload is the payload for tool events.
type ToolEventPayload struct {
	ToolCallID        string `json:"tool_call_id"`
	ToolName          string `json:"tool_name"`
	Arguments         string `json:"arguments,omitempty"`
	ArgumentsRaw      string `json:"arguments_raw,omitempty"`
	ArgumentsRecovery string `json:"arguments_recovery,omitempty"`
	ResultPreview     string `json:"result_preview,omitempty"`
	IsError           bool   `json:"is_error,omitempty"`
}

// AssistantDeltaPayload is the payload for assistant delta events.
type AssistantDeltaPayload struct {
	Text string `json:"text"`
}

// NoticePayload is the payload for notice events.
type NoticePayload struct {
	Text string `json:"text"`
}

// TurnFailedPayload is the payload for turn failed events.
type TurnFailedPayload struct {
	Message string `json:"message"`
}

// SessionQueuePayload is the payload for session queue updated events.
type SessionQueuePayload struct {
	ActiveTurnID        string `json:"active_turn_id,omitempty"`
	ActiveTurnKind      string `json:"active_turn_kind,omitempty"`
	QueueDepth          int    `json:"queue_depth"`
	QueuedUserTurns     int    `json:"queued_user_turns"`
	QueuedReportBatches int    `json:"queued_report_batches"`
}

// ContextHeadSummaryEventPayload is the payload for context head summary events.
type ContextHeadSummaryEventPayload struct {
	Message string `json:"message,omitempty"`
}
