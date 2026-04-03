package types

import (
	"encoding/json"
	"time"
)

const (
	EventTurnStarted         = "turn.started"
	EventTurnCompleted       = "turn.completed"
	EventTurnInterrupted     = "turn.interrupted"
	EventAssistantStarted    = "assistant.started"
	EventAssistantDelta      = "assistant.delta"
	EventAssistantCompleted  = "assistant.completed"
	EventToolStarted         = "tool.started"
	EventToolProgress        = "tool.progress"
	EventToolCompleted       = "tool.completed"
	EventPermissionRequested = "permission.requested"
	EventContextCompacted    = "context.compacted"
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

type AssistantDeltaPayload struct {
	Text string `json:"text"`
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
