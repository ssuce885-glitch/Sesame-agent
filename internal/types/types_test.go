package types

import (
	"encoding/json"
	"testing"
)

func TestNewIDUsesPrefix(t *testing.T) {
	id := NewID("sess")
	if len(id) < 6 || id[:5] != "sess_" {
		t.Fatalf("NewID returned %q", id)
	}
}

func TestEventJSONRoundTrip(t *testing.T) {
	event, err := NewEvent("sess_1", "turn_1", EventTurnStarted, TurnStartedPayload{
		WorkspaceRoot: "D:/work/demo",
	})
	if err != nil {
		t.Fatalf("NewEvent() error = %v", err)
	}

	raw, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	var decoded Event
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	if decoded.Type != EventTurnStarted {
		t.Fatalf("decoded.Type = %q, want %q", decoded.Type, EventTurnStarted)
	}
	if decoded.SessionID != "sess_1" {
		t.Fatalf("decoded.SessionID = %q", decoded.SessionID)
	}
}
