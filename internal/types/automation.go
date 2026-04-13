package types

import (
	"encoding/json"
	"strings"
	"time"
)

type AutomationState string
type AutomationControlAction string
type AutomationIncidentStatus string
type AutomationWatcherState string
type AutomationPhase string
type DispatchAttemptStatus string
type DeliveryChannelStatus string

const (
	AutomationStateActive AutomationState = "active"
	AutomationStatePaused AutomationState = "paused"
)

const (
	AutomationControlActionPause  AutomationControlAction = "pause"
	AutomationControlActionResume AutomationControlAction = "resume"
)

const (
	AutomationControlPause  AutomationControlAction = AutomationControlActionPause
	AutomationControlResume AutomationControlAction = AutomationControlActionResume
)

const (
	AutomationIncidentStatusOpen AutomationIncidentStatus = "open"
)

const (
	AutomationWatcherStatePending AutomationWatcherState = "pending"
	AutomationWatcherStateRunning AutomationWatcherState = "running"
	AutomationWatcherStatePaused  AutomationWatcherState = "paused"
	AutomationWatcherStateFailed  AutomationWatcherState = "failed"
	AutomationWatcherStateStopped AutomationWatcherState = "stopped"
)

const (
	AutomationPhaseDiagnose  AutomationPhase = "diagnose"
	AutomationPhaseRemediate AutomationPhase = "remediate"
	AutomationPhaseVerify    AutomationPhase = "verify"
)

const (
	ResponsePlanV2Schema = "sesame.response_plan/v2"
)

const (
	DispatchAttemptStatusAwaitingApproval DispatchAttemptStatus = "awaiting_approval"
)

const (
	DeliveryChannelStatusPending  DeliveryChannelStatus = "pending"
	DeliveryChannelStatusReady    DeliveryChannelStatus = "ready"
	DeliveryChannelStatusDisabled DeliveryChannelStatus = "disabled"
)

type AutomationContext struct {
	Targets     []string          `json:"targets"`
	Labels      map[string]string `json:"labels"`
	Owner       string            `json:"owner"`
	Environment string            `json:"environment"`
}

type AutomationSignal struct {
	Kind     string          `json:"kind,omitempty"`
	Source   string          `json:"source,omitempty"`
	Selector string          `json:"selector,omitempty"`
	Payload  json.RawMessage `json:"payload,omitempty"`
}

type AutomationAssumption struct {
	Field  string `json:"field,omitempty"`
	Value  string `json:"value,omitempty"`
	Reason string `json:"reason,omitempty"`
}

type ResponsePlanPhase struct {
	Phase                  AutomationPhase `json:"phase,omitempty"`
	ChildAgentTemplateRefs []string        `json:"child_agent_template_refs,omitempty"`
}

type ResponsePlanV2 struct {
	SchemaVersion          string              `json:"schema_version,omitempty"`
	Mode                   string              `json:"mode,omitempty"`
	ChildAgentTemplateRefs []string            `json:"child_agent_template_refs,omitempty"`
	Phases                 []ResponsePlanPhase `json:"phases,omitempty"`
}

type AutomationSpec struct {
	ID               string             `json:"id"`
	Title            string             `json:"title"`
	WorkspaceRoot    string             `json:"workspace_root"`
	Goal             string             `json:"goal"`
	State            AutomationState    `json:"state"`
	Context          AutomationContext  `json:"context"`
	Signals          []AutomationSignal `json:"signals"`
	IncidentPolicy   json.RawMessage    `json:"incident_policy"`
	ResponsePlan     json.RawMessage    `json:"response_plan"`
	VerificationPlan json.RawMessage    `json:"verification_plan"`
	EscalationPolicy json.RawMessage    `json:"escalation_policy"`
	DeliveryPolicy   json.RawMessage    `json:"delivery_policy"`
	RuntimePolicy    json.RawMessage    `json:"runtime_policy"`
	WatcherLifecycle json.RawMessage    `json:"watcher_lifecycle"`
	RetriggerPolicy  json.RawMessage    `json:"retrigger_policy"`
	RunPolicy        json.RawMessage    `json:"run_policy"`
	Assumptions      []AutomationAssumption `json:"assumptions"`
	CreatedAt        time.Time          `json:"created_at,omitempty"`
	UpdatedAt        time.Time          `json:"updated_at,omitempty"`
}

type AutomationIncident struct {
	ID            string                   `json:"id"`
	AutomationID  string                   `json:"automation_id"`
	WorkspaceRoot string                   `json:"workspace_root"`
	Status        AutomationIncidentStatus `json:"status"`
	SignalKind    string                   `json:"signal_kind,omitempty"`
	Source        string                   `json:"source,omitempty"`
	Summary       string                   `json:"summary,omitempty"`
	Payload       json.RawMessage          `json:"payload,omitempty"`
	ObservedAt    time.Time                `json:"observed_at,omitempty"`
	CreatedAt     time.Time                `json:"created_at,omitempty"`
	UpdatedAt     time.Time                `json:"updated_at,omitempty"`
}

type AutomationHeartbeat struct {
	AutomationID  string          `json:"automation_id"`
	WatcherID     string          `json:"watcher_id"`
	WorkspaceRoot string          `json:"workspace_root"`
	Status        string          `json:"status,omitempty"`
	Payload       json.RawMessage `json:"payload,omitempty"`
	ObservedAt    time.Time       `json:"observed_at,omitempty"`
	CreatedAt     time.Time       `json:"created_at,omitempty"`
	UpdatedAt     time.Time       `json:"updated_at,omitempty"`
}

type AutomationWatcherRuntime struct {
	ID            string                 `json:"id"`
	AutomationID  string                 `json:"automation_id"`
	WorkspaceRoot string                 `json:"workspace_root"`
	WatcherID     string                 `json:"watcher_id"`
	State         AutomationWatcherState `json:"state"`
	ScriptPath    string                 `json:"script_path"`
	StatePath     string                 `json:"state_path,omitempty"`
	TaskID        string                 `json:"task_id,omitempty"`
	Command       string                 `json:"command,omitempty"`
	LastError     string                 `json:"last_error,omitempty"`
	CreatedAt     time.Time              `json:"created_at,omitempty"`
	UpdatedAt     time.Time              `json:"updated_at,omitempty"`
}

type DispatchAttempt struct {
	DispatchID          string               `json:"dispatch_id"`
	IncidentID          string               `json:"incident_id,omitempty"`
	AutomationID        string               `json:"automation_id,omitempty"`
	WorkspaceRoot       string               `json:"workspace_root,omitempty"`
	Phase               AutomationPhase      `json:"phase,omitempty"`
	Attempt             int                  `json:"attempt,omitempty"`
	Status              DispatchAttemptStatus `json:"status,omitempty"`
	TaskID              string               `json:"task_id,omitempty"`
	BackgroundSessionID string               `json:"background_session_id,omitempty"`
	BackgroundTurnID    string               `json:"background_turn_id,omitempty"`
	ContinuationID      string               `json:"continuation_id,omitempty"`
	PermissionRequestID string               `json:"permission_request_id,omitempty"`
	ApprovalQueueKey    string               `json:"approval_queue_key,omitempty"`
	PreferredSessionID  string               `json:"preferred_session_id,omitempty"`
	CreatedAt           time.Time            `json:"created_at,omitempty"`
	UpdatedAt           time.Time            `json:"updated_at,omitempty"`
}

type DeliveryChannelStatusRecord struct {
	Status DeliveryChannelStatus `json:"status,omitempty"`
}

type DeliveryChannelSet struct {
	Notice    DeliveryChannelStatusRecord `json:"notice,omitempty"`
	Mailbox   DeliveryChannelStatusRecord `json:"mailbox,omitempty"`
	Injection DeliveryChannelStatusRecord `json:"injection,omitempty"`
}

type DeliveryRecord struct {
	DeliveryID    string             `json:"delivery_id"`
	WorkspaceRoot string             `json:"workspace_root,omitempty"`
	AutomationID  string             `json:"automation_id,omitempty"`
	IncidentID    string             `json:"incident_id,omitempty"`
	DispatchID    string             `json:"dispatch_id,omitempty"`
	SummaryRef    string             `json:"summary_ref,omitempty"`
	Channels      DeliveryChannelSet `json:"channels,omitempty"`
	CreatedAt     time.Time          `json:"created_at,omitempty"`
	UpdatedAt     time.Time          `json:"updated_at,omitempty"`
}

type AutomationAsset struct {
	Path       string `json:"path"`
	Content    string `json:"content"`
	Executable bool   `json:"executable,omitempty"`
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

type AutomationWatcherFilter struct {
	WorkspaceRoot string                 `json:"workspace_root,omitempty"`
	AutomationID  string                 `json:"automation_id,omitempty"`
	State         AutomationWatcherState `json:"state,omitempty"`
	Limit         int                    `json:"limit,omitempty"`
}

type IncidentListFilter = AutomationIncidentFilter

type AutomationTriggerRequest struct {
	AutomationID string          `json:"automation_id"`
	SignalKind   string          `json:"signal_kind,omitempty"`
	Source       string          `json:"source,omitempty"`
	Summary      string          `json:"summary,omitempty"`
	Payload      json.RawMessage `json:"payload,omitempty"`
	ObservedAt   time.Time       `json:"observed_at,omitempty"`
}

type TriggerEmitRequest = AutomationTriggerRequest

type AutomationHeartbeatRequest struct {
	AutomationID string          `json:"automation_id"`
	WatcherID    string          `json:"watcher_id"`
	Status       string          `json:"status,omitempty"`
	Payload      json.RawMessage `json:"payload,omitempty"`
	ObservedAt   time.Time       `json:"observed_at,omitempty"`
}

type TriggerHeartbeatRequest = AutomationHeartbeatRequest

type ApplyAutomationRequest struct {
	Confirmed bool              `json:"confirmed"`
	Spec      AutomationSpec    `json:"spec"`
	Assets    []AutomationAsset `json:"assets,omitempty"`
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

type GetAutomationWatcherResponse struct {
	Watcher AutomationWatcherRuntime `json:"watcher"`
}

type ListAutomationWatchersResponse struct {
	Watchers []AutomationWatcherRuntime `json:"watchers"`
}

type AutomationValidationError struct {
	Code    string `json:"code"`
	Message string `json:"message,omitempty"`
}

func (e *AutomationValidationError) Error() string {
	if e == nil {
		return ""
	}
	if strings.TrimSpace(e.Message) == "" {
		return strings.TrimSpace(e.Code)
	}
	return strings.TrimSpace(e.Code) + ": " + strings.TrimSpace(e.Message)
}
