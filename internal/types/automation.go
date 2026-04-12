package types

import (
	"encoding/json"
	"time"
)

type AutomationState string
type AutomationControlAction string
type AutomationIncidentStatus string

const (
	AutomationStateActive AutomationState = "active"
	AutomationStatePaused AutomationState = "paused"
)

const (
	AutomationControlActionPause  AutomationControlAction = "pause"
	AutomationControlActionResume AutomationControlAction = "resume"
)

const (
	AutomationIncidentStatusOpen AutomationIncidentStatus = "open"
)

type AutomationContext struct {
	Targets     []string          `json:"targets,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
	Owner       string            `json:"owner,omitempty"`
	Environment string            `json:"environment,omitempty"`
}

type AutomationSignal struct {
	Kind     string          `json:"kind,omitempty"`
	Source   string          `json:"source,omitempty"`
	Selector string          `json:"selector,omitempty"`
	Payload  json.RawMessage `json:"payload,omitempty"`
}

type AutomationSpec struct {
	ID               string             `json:"id"`
	Title            string             `json:"title"`
	WorkspaceRoot    string             `json:"workspace_root"`
	Goal             string             `json:"goal"`
	State            AutomationState    `json:"state"`
	Context          AutomationContext  `json:"context,omitempty"`
	Signals          []AutomationSignal `json:"signals,omitempty"`
	IncidentPolicy   json.RawMessage    `json:"incident_policy,omitempty"`
	ResponsePlan     json.RawMessage    `json:"response_plan,omitempty"`
	VerificationPlan json.RawMessage    `json:"verification_plan,omitempty"`
	EscalationPolicy json.RawMessage    `json:"escalation_policy,omitempty"`
	DeliveryPolicy   json.RawMessage    `json:"delivery_policy,omitempty"`
	RuntimePolicy    json.RawMessage    `json:"runtime_policy,omitempty"`
	WatcherLifecycle json.RawMessage    `json:"watcher_lifecycle,omitempty"`
	RetriggerPolicy  json.RawMessage    `json:"retrigger_policy,omitempty"`
	RunPolicy        json.RawMessage    `json:"run_policy,omitempty"`
	Assumptions      []string           `json:"assumptions,omitempty"`
	CreatedAt        time.Time          `json:"created_at,omitempty"`
	UpdatedAt        time.Time          `json:"updated_at,omitempty"`
}

type AutomationIncident struct {
	ID            string                   `json:"id"`
	AutomationID  string                   `json:"automation_id"`
	WorkspaceRoot string                   `json:"workspace_root"`
	Status        AutomationIncidentStatus `json:"status"`
	TriggerKind   string                   `json:"trigger_kind,omitempty"`
	Source        string                   `json:"source,omitempty"`
	Title         string                   `json:"title,omitempty"`
	Summary       string                   `json:"summary,omitempty"`
	Payload       json.RawMessage          `json:"payload,omitempty"`
	ObservedAt    time.Time                `json:"observed_at,omitempty"`
	CreatedAt     time.Time                `json:"created_at,omitempty"`
	UpdatedAt     time.Time                `json:"updated_at,omitempty"`
}

type AutomationHeartbeat struct {
	ID            string          `json:"id"`
	AutomationID  string          `json:"automation_id"`
	WorkspaceRoot string          `json:"workspace_root"`
	Status        string          `json:"status,omitempty"`
	Message       string          `json:"message,omitempty"`
	Payload       json.RawMessage `json:"payload,omitempty"`
	ObservedAt    time.Time       `json:"observed_at,omitempty"`
	CreatedAt     time.Time       `json:"created_at,omitempty"`
	UpdatedAt     time.Time       `json:"updated_at,omitempty"`
}

type AutomationListFilter struct {
	WorkspaceRoot string          `json:"workspace_root,omitempty"`
	State         AutomationState `json:"state,omitempty"`
	Limit         int             `json:"limit,omitempty"`
}

type AutomationIncidentFilter struct {
	WorkspaceRoot string                   `json:"workspace_root,omitempty"`
	AutomationID  string                   `json:"automation_id,omitempty"`
	Status        AutomationIncidentStatus `json:"status,omitempty"`
	Limit         int                      `json:"limit,omitempty"`
}

type AutomationTriggerRequest struct {
	AutomationID string          `json:"automation_id"`
	TriggerKind  string          `json:"trigger_kind,omitempty"`
	Source       string          `json:"source,omitempty"`
	Title        string          `json:"title,omitempty"`
	Summary      string          `json:"summary,omitempty"`
	Payload      json.RawMessage `json:"payload,omitempty"`
	ObservedAt   time.Time       `json:"observed_at,omitempty"`
}

type AutomationHeartbeatRequest struct {
	AutomationID string          `json:"automation_id"`
	Status       string          `json:"status,omitempty"`
	Message      string          `json:"message,omitempty"`
	Payload      json.RawMessage `json:"payload,omitempty"`
	ObservedAt   time.Time       `json:"observed_at,omitempty"`
}

type ApplyAutomationRequest struct {
	Spec AutomationSpec `json:"spec"`
}

type ApplyAutomationResponse struct {
	Automation AutomationSpec `json:"automation"`
}

type ListAutomationsResponse struct {
	Automations []AutomationSpec `json:"automations"`
}

type ControlAutomationRequest struct {
	Action AutomationControlAction `json:"action"`
}

type ControlAutomationResponse struct {
	Automation AutomationSpec `json:"automation"`
}

type EmitAutomationTriggerResponse struct {
	Incident AutomationIncident `json:"incident"`
}

type RecordAutomationHeartbeatResponse struct {
	Heartbeat AutomationHeartbeat `json:"heartbeat"`
}

type ListAutomationIncidentsResponse struct {
	Incidents []AutomationIncident `json:"incidents"`
}
