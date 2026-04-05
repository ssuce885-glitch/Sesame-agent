package types

import (
	"encoding/json"
	"time"
)

const (
	EventTurnStarted         = "turn.started"
	EventTurnCompleted       = "turn.completed"
	EventTurnFailed          = "turn.failed"
	EventTurnInterrupted     = "turn.interrupted"
	EventTurnUsage           = "turn.usage"
	EventAssistantStarted    = "assistant.started"
	EventAssistantDelta      = "assistant.delta"
	EventAssistantCompleted  = "assistant.completed"
	EventToolStarted         = "tool.started"
	EventToolProgress        = "tool.progress"
	EventToolCompleted       = "tool.completed"
	EventPermissionRequested = "permission.requested"
	EventContextCompacted    = "context.compacted"
	EventSystemNotice        = "system.notice"
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

type NoticePayload struct {
	Text string `json:"text"`
}

type ToolEventPayload struct {
	ToolCallID    string `json:"tool_call_id"`
	ToolName      string `json:"tool_name"`
	Arguments     string `json:"arguments,omitempty"`
	ResultPreview string `json:"result_preview,omitempty"`
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
