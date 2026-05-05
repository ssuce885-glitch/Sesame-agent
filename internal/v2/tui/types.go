package tui

import "time"

type Event struct {
	ID        string
	Seq       int64
	SessionID string `json:"session_id"`
	TurnID    string `json:"turn_id,omitempty"`
	Type      string
	Time      time.Time
	Payload   []byte
}

type SubmitTurnRequest struct {
	SessionID string `json:"session_id,omitempty"`
	Message   string `json:"message"`
	Kind      string `json:"kind,omitempty"`
}

type Turn struct {
	ID        string `json:"id"`
	SessionID string `json:"session_id"`
	State     string `json:"state,omitempty"`
}

type StatusResponse struct {
	Status            string              `json:"status"`
	Addr              string              `json:"addr,omitempty"`
	Provider          string              `json:"provider,omitempty"`
	Model             string              `json:"model,omitempty"`
	PermissionProfile string              `json:"permission_profile,omitempty"`
	DefaultSessionID  string              `json:"default_session_id,omitempty"`
	Queue             SessionQueuePayload `json:"queue,omitempty"`
	PID               int                 `json:"pid,omitempty"`
}

type SessionInfo struct {
	ID            string              `json:"id"`
	WorkspaceRoot string              `json:"workspace_root"`
	State         string              `json:"state"`
	ActiveTurnID  string              `json:"active_turn_id,omitempty"`
	CreatedAt     time.Time           `json:"created_at"`
	UpdatedAt     time.Time           `json:"updated_at"`
	Queue         SessionQueuePayload `json:"queue,omitempty"`
}

type SessionTimelineResponse struct {
	LatestSeq         int64           `json:"latest_seq"`
	Blocks            []TimelineBlock `json:"blocks"`
	QueuedReportCount int             `json:"queued_report_count"`
	Queue             QueueSummary    `json:"queue"`
}

type TimelineBlock struct {
	Kind    string         `json:"kind"`
	Text    string         `json:"text,omitempty"`
	Title   string         `json:"title,omitempty"`
	Path    string         `json:"path,omitempty"`
	Status  string         `json:"status,omitempty"`
	Content []ContentBlock `json:"content,omitempty"`
}

type ContentBlock struct {
	Type          string `json:"type"`
	Text          string `json:"text,omitempty"`
	ToolName      string `json:"tool_name,omitempty"`
	ArgsPreview   string `json:"args_preview,omitempty"`
	ResultPreview string `json:"result_preview,omitempty"`
	Status        string `json:"status,omitempty"`
	ToolCallID    string `json:"tool_call_id,omitempty"`
}

type ReportsResponse struct {
	Items       []ReportDeliveryItem `json:"items"`
	QueuedCount int                  `json:"queued_count"`
}

type AutomationResponse struct {
	ID            string    `json:"id"`
	WorkspaceRoot string    `json:"workspace_root"`
	Title         string    `json:"title"`
	Goal          string    `json:"goal"`
	State         string    `json:"state"`
	Owner         string    `json:"owner"`
	WorkflowID    string    `json:"workflow_id,omitempty"`
	WatcherPath   string    `json:"watcher_path"`
	WatcherCron   string    `json:"watcher_cron"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type ProjectStateResponse struct {
	WorkspaceRoot   string    `json:"workspace_root"`
	Summary         string    `json:"summary"`
	SourceSessionID string    `json:"source_session_id,omitempty"`
	SourceTurnID    string    `json:"source_turn_id,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type SettingResponse struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type ReportDeliveryItem struct {
	ID         string         `json:"id"`
	SessionID  string         `json:"session_id,omitempty"`
	SourceKind string         `json:"source_kind"`
	SourceID   string         `json:"source_id,omitempty"`
	Title      string         `json:"title,omitempty"`
	Summary    string         `json:"summary,omitempty"`
	Severity   string         `json:"severity,omitempty"`
	Status     string         `json:"status,omitempty"`
	Delivered  bool           `json:"delivered,omitempty"`
	CreatedAt  time.Time      `json:"created_at"`
	Envelope   ReportEnvelope `json:"envelope,omitempty"`
}

type ReportEnvelope struct {
	Title    string `json:"title,omitempty"`
	Summary  string `json:"summary,omitempty"`
	Status   string `json:"status,omitempty"`
	Severity string `json:"severity,omitempty"`
	Source   string `json:"source,omitempty"`
}

type SessionQueuePayload struct {
	ActiveTurnID        string `json:"active_turn_id,omitempty"`
	ActiveTurnKind      string `json:"active_turn_kind,omitempty"`
	QueueDepth          int    `json:"queue_depth"`
	QueuedUserTurns     int    `json:"queued_user_turns"`
	QueuedReportBatches int    `json:"queued_report_batches"`
}

type AssistantDeltaPayload struct {
	Text string `json:"text"`
}

type ToolEventPayload struct {
	ToolCallID    string         `json:"tool_call_id,omitempty"`
	ID            string         `json:"id,omitempty"`
	ToolName      string         `json:"tool_name,omitempty"`
	Name          string         `json:"name,omitempty"`
	Arguments     string         `json:"arguments,omitempty"`
	Args          map[string]any `json:"args,omitempty"`
	ResultPreview string         `json:"result_preview,omitempty"`
	Output        string         `json:"output,omitempty"`
	IsError       bool           `json:"is_error,omitempty"`
}

type NoticePayload struct {
	Text string `json:"text"`
}

type TurnFailedPayload struct {
	Message string `json:"message,omitempty"`
	Error   string `json:"error,omitempty"`
}
