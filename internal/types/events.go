package types

import (
	"encoding/json"
	"time"
)

const (
	EventTurnStarted                 = "turn.started"
	EventTurnCompleted               = "turn.completed"
	EventTurnFailed                  = "turn.failed"
	EventTurnInterrupted             = "turn.interrupted"
	EventTurnUsage                   = "turn.usage"
	EventAssistantStarted            = "assistant.started"
	EventAssistantDelta              = "assistant.delta"
	EventAssistantCompleted          = "assistant.completed"
	EventToolStarted                 = "tool.started"
	EventToolProgress                = "tool.progress"
	EventToolCompleted               = "tool.completed"
	EventPlanUpdated                 = "plan.updated"
	EventTaskUpdated                 = "task.updated"
	EventTaskResultReady             = "task.result_ready"
	EventReportReady                 = "report.ready"
	EventToolRunUpdated              = "tool_run.updated"
	EventWorktreeUpdated             = "worktree.updated"
	EventParentReplyCommitted        = "parent.reply_committed"
	EventContextCompacted            = "context.compacted"
	EventContextHeadSummaryStarted   = "context_head_summary.started"
	EventContextHeadSummaryCompleted = "context_head_summary.completed"
	EventContextHeadSummaryFailed    = "context_head_summary.failed"
	EventSessionQueueUpdated         = "session.queue_updated"
	EventSystemNotice                = "system.notice"
)

type Event struct {
	ID        string          `json:"id"`
	Seq       int64           `json:"seq"`
	SessionID string          `json:"session_id"`
	TurnID    string          `json:"turn_id,omitempty"`
	Type      string          `json:"type"`
	Time      time.Time       `json:"time"`
	Payload   json.RawMessage `json:"payload"`
}

type TurnStartedPayload struct {
	WorkspaceRoot string `json:"workspace_root"`
}

type TurnFailedPayload struct {
	Message string `json:"message"`
}

type AssistantDeltaPayload struct {
	Text string `json:"text"`
}

type ParentReplyCommittedPayload struct {
	WorkspaceRoot       string   `json:"workspace_root"`
	SessionID           string   `json:"session_id"`
	TurnID              string   `json:"turn_id"`
	TurnKind            TurnKind `json:"turn_kind,omitempty"`
	SourceParentTurnIDs []string `json:"source_parent_turn_ids,omitempty"`
	SourceTaskIDs       []string `json:"source_task_ids,omitempty"`
	ItemID              int64    `json:"item_id"`
	Text                string   `json:"text"`
	CreatedAt           string   `json:"created_at"`
}

type NoticePayload struct {
	Text string `json:"text"`
}

type ToolEventPayload struct {
	ToolCallID        string `json:"tool_call_id"`
	ToolName          string `json:"tool_name"`
	Arguments         string `json:"arguments,omitempty"`
	ArgumentsRaw      string `json:"arguments_raw,omitempty"`
	ArgumentsRecovery string `json:"arguments_recovery,omitempty"`
	ResultPreview     string `json:"result_preview,omitempty"`
	IsError           bool   `json:"is_error,omitempty"`
}

type ContextHeadSummaryEventPayload struct {
	SourceTurnID             string `json:"source_turn_id,omitempty"`
	WorkspaceRoot            string `json:"workspace_root,omitempty"`
	Async                    bool   `json:"async,omitempty"`
	Updated                  bool   `json:"updated,omitempty"`
	WorkspaceEntriesUpserted int    `json:"workspace_entries_upserted,omitempty"`
	GlobalEntriesUpserted    int    `json:"global_entries_upserted,omitempty"`
	WorkspaceEntriesPruned   int    `json:"workspace_entries_pruned,omitempty"`
	Message                  string `json:"message,omitempty"`
}

type SessionQueuePayload struct {
	ActiveTurnID        string   `json:"active_turn_id,omitempty"`
	ActiveTurnKind      TurnKind `json:"active_turn_kind,omitempty"`
	QueueDepth          int      `json:"queue_depth"`
	QueuedUserTurns     int      `json:"queued_user_turns"`
	QueuedReportBatches int      `json:"queued_report_batches"`
}

func NewEvent(sessionID, turnID, eventType string, payload any) (Event, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return Event{}, err
	}

	return Event{
		ID:        NewID("evt"),
		SessionID: sessionID,
		TurnID:    turnID,
		Type:      eventType,
		Time:      time.Now().UTC(),
		Payload:   raw,
	}, nil
}
