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

func TestOutputContractJSONRoundTrip(t *testing.T) {
	contract := OutputContract{
		ContractID: "ops-monitor-v1",
		Intent:     "Operator-facing health summary",
		Sections: []ContractSection{
			{ID: "overall_status", Title: "Overall Status", Required: true, MaxItems: 1},
			{ID: "action_items", Title: "Action Items", MaxItems: 3},
		},
		Rules: OutputContractRules{
			IncludeSeverity: true,
			MustBeConcise:   true,
		},
		Tone: "concise_operator",
		UIHints: OutputContractUIHints{
			RenderAs:   "task_card",
			Expandable: true,
		},
	}

	raw, err := json.Marshal(contract)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	var decoded OutputContract
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	if decoded.ContractID != contract.ContractID || decoded.Tone != contract.Tone || len(decoded.Sections) != 2 || !decoded.Rules.IncludeSeverity || !decoded.UIHints.Expandable {
		t.Fatalf("decoded = %#v, want %#v", decoded, contract)
	}
}

func TestChildAgentResultJSONRoundTrip(t *testing.T) {
	result := ChildAgentResult{
		ResultID:        "result_docker_1",
		SessionID:       "sess_reporting",
		AgentID:         "docker-check",
		ContractID:      "ops-monitor-v1",
		ReportGroupRefs: []string{"ops-daily"},
		Envelope: ReportEnvelope{
			Source:   "docker",
			Status:   "warning",
			Severity: "warning",
			Title:    "1 unhealthy container",
			Summary:  "api container restarted 3 times in the last hour.",
			Sections: []ReportSectionContent{
				{ID: "overall_status", Title: "Overall Status", Text: "1 container unhealthy"},
				{ID: "action_items", Title: "Action Items", Items: []string{"Inspect api container logs"}},
			},
			Payload: json.RawMessage(`{"containers":[{"name":"api","status":"restarting"}]}`),
		},
	}

	raw, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	var decoded ChildAgentResult
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	if decoded.ResultID != result.ResultID || decoded.SessionID != result.SessionID || decoded.AgentID != result.AgentID || decoded.Envelope.Status != "warning" || len(decoded.Envelope.Sections) != 2 || string(decoded.Envelope.Payload) != string(result.Envelope.Payload) {
		t.Fatalf("decoded = %#v, want %#v", decoded, result)
	}
}
