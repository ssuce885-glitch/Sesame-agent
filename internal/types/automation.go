package types

import (
	"encoding/json"
	"strings"
	"time"
)

type AutomationState string
type AutomationControlAction string
type AutomationWatcherState string
type AutomationWatcherHoldKind string
type AutomationAssumptionSource string
type ChildAgentOutcome string
type AutomationDetectorStatus string

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
	AutomationWatcherStatePending AutomationWatcherState = "pending"
	AutomationWatcherStateRunning AutomationWatcherState = "running"
	AutomationWatcherStatePaused  AutomationWatcherState = "paused"
	AutomationWatcherStateFailed  AutomationWatcherState = "failed"
	AutomationWatcherStateStopped AutomationWatcherState = "stopped"
)

const (
	AutomationWatcherHoldKindManual AutomationWatcherHoldKind = "manual"
)

const (
	AutomationAssumptionSourceNormalizer  AutomationAssumptionSource = "normalizer"
	AutomationAssumptionSourceSystemSkill AutomationAssumptionSource = "system_skill"
	AutomationAssumptionSourceDomainSkill AutomationAssumptionSource = "domain_skill"
)

const (
	ChildAgentOutcomeSuccess ChildAgentOutcome = "success"
	ChildAgentOutcomeFailure ChildAgentOutcome = "failure"
	ChildAgentOutcomeBlocked ChildAgentOutcome = "blocked"
)

const (
	AutomationDetectorStatusHealthy    AutomationDetectorStatus = "healthy"
	AutomationDetectorStatusRecovered  AutomationDetectorStatus = "recovered"
	AutomationDetectorStatusNeedsAgent AutomationDetectorStatus = "needs_agent"
	AutomationDetectorStatusNeedsHuman AutomationDetectorStatus = "needs_human"
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

type AutomationDetectorActionResult struct {
	Name    string `json:"name"`
	Result  string `json:"result"`
	Summary string `json:"summary,omitempty"`
}

type AutomationDetectorSignal struct {
	Status       AutomationDetectorStatus `json:"status"`
	Summary      string                   `json:"summary"`
	Facts        map[string]any           `json:"facts"`
	ActionsTaken []string                 `json:"actions_taken"`
	Hints        []string                 `json:"hints"`
	DedupeKey    string                   `json:"dedupe_key,omitempty"`
}

type AutomationAssumption struct {
	Key    string                     `json:"key,omitempty"`
	Field  string                     `json:"field"`
	Value  json.RawMessage            `json:"value"`
	Reason string                     `json:"reason,omitempty"`
	Source AutomationAssumptionSource `json:"source,omitempty"`
}

type AutomationSpec struct {
	ID               string                 `json:"id"`
	Title            string                 `json:"title"`
	WorkspaceRoot    string                 `json:"workspace_root"`
	Goal             string                 `json:"goal"`
	State            AutomationState        `json:"state"`
	Mode             AutomationMode         `json:"mode,omitempty"`
	Owner            string                 `json:"owner,omitempty"`
	ReportTarget     string                 `json:"report_target,omitempty"`
	EscalationTarget string                 `json:"escalation_target,omitempty"`
	SimplePolicy     SimpleAutomationPolicy `json:"simple_policy,omitempty"`
	Context          AutomationContext      `json:"context"`
	Signals          []AutomationSignal     `json:"signals"`
	WatcherLifecycle json.RawMessage        `json:"watcher_lifecycle"`
	RetriggerPolicy  json.RawMessage        `json:"retrigger_policy"`
	Assumptions      []AutomationAssumption `json:"assumptions"`
	CreatedAt        time.Time              `json:"created_at,omitempty"`
	UpdatedAt        time.Time              `json:"updated_at,omitempty"`
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

type AutomationWatcherHold struct {
	HoldID       string                    `json:"hold_id"`
	AutomationID string                    `json:"automation_id"`
	WatcherID    string                    `json:"watcher_id"`
	Kind         AutomationWatcherHoldKind `json:"kind"`
	OwnerID      string                    `json:"owner_id"`
	Reason       string                    `json:"reason,omitempty"`
	CreatedAt    time.Time                 `json:"created_at,omitempty"`
	UpdatedAt    time.Time                 `json:"updated_at,omitempty"`
}

type AutomationWatcherRuntime struct {
	ID             string                  `json:"id"`
	AutomationID   string                  `json:"automation_id"`
	WorkspaceRoot  string                  `json:"workspace_root"`
	WatcherID      string                  `json:"watcher_id"`
	State          AutomationWatcherState  `json:"state"`
	EffectiveState AutomationWatcherState  `json:"effective_state,omitempty"`
	Holds          []AutomationWatcherHold `json:"holds,omitempty"`
	ScriptPath     string                  `json:"script_path"`
	StatePath      string                  `json:"state_path,omitempty"`
	TaskID         string                  `json:"task_id,omitempty"`
	Command        string                  `json:"command,omitempty"`
	LastError      string                  `json:"last_error,omitempty"`
	CreatedAt      time.Time               `json:"created_at,omitempty"`
	UpdatedAt      time.Time               `json:"updated_at,omitempty"`
}

type TriggerEvent struct {
	EventID       string          `json:"event_id"`
	AutomationID  string          `json:"automation_id"`
	WorkspaceRoot string          `json:"workspace_root"`
	SignalKind    string          `json:"signal_kind,omitempty"`
	Source        string          `json:"source,omitempty"`
	Summary       string          `json:"summary,omitempty"`
	Payload       json.RawMessage `json:"payload,omitempty"`
	DedupeKey     string          `json:"dedupe_key,omitempty"`
	ObservedAt    time.Time       `json:"observed_at,omitempty"`
	CreatedAt     time.Time       `json:"created_at,omitempty"`
	UpdatedAt     time.Time       `json:"updated_at,omitempty"`
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

type AutomationWatcherFilter struct {
	WorkspaceRoot string                 `json:"workspace_root,omitempty"`
	AutomationID  string                 `json:"automation_id,omitempty"`
	State         AutomationWatcherState `json:"state,omitempty"`
	Limit         int                    `json:"limit,omitempty"`
}

type TriggerEventFilter struct {
	WorkspaceRoot string `json:"workspace_root,omitempty"`
	AutomationID  string `json:"automation_id,omitempty"`
	DedupeKey     string `json:"dedupe_key,omitempty"`
	Limit         int    `json:"limit,omitempty"`
}

type PendingAutomationPermission struct {
	RequestID          string `json:"request_id"`
	WorkspaceRoot      string `json:"workspace_root"`
	PreferredSessionID string `json:"preferred_session_id,omitempty"`
}

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
	Trigger TriggerEvent `json:"trigger"`
}

type RecordAutomationHeartbeatResponse struct {
	Heartbeat AutomationHeartbeat `json:"heartbeat"`
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
