package types

import (
	"encoding/json"
	"strings"
	"time"
)

type AutomationState string
type AutomationControlAction string
type IncidentControlAction string
type AutomationIncidentStatus string
type AutomationWatcherState string
type AutomationPhaseName string
type AutomationPhaseTransitionAction string
type IncidentPhaseReduction string
type IncidentPhaseStatus string
type AutomationAssumptionSource string
type DispatchAttemptStatus string
type DeliveryChannelStatus string
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
	IncidentControlActionAck      IncidentControlAction = "ack"
	IncidentControlActionClose    IncidentControlAction = "close"
	IncidentControlActionReopen   IncidentControlAction = "reopen"
	IncidentControlActionEscalate IncidentControlAction = "escalate"
)

const (
	AutomationIncidentStatusOpen       AutomationIncidentStatus = "open"
	AutomationIncidentStatusSuppressed AutomationIncidentStatus = "suppressed"
	AutomationIncidentStatusQueued     AutomationIncidentStatus = "queued"
	AutomationIncidentStatusActive     AutomationIncidentStatus = "active"
	AutomationIncidentStatusMonitoring AutomationIncidentStatus = "monitoring"
	AutomationIncidentStatusResolved   AutomationIncidentStatus = "resolved"
	AutomationIncidentStatusEscalated  AutomationIncidentStatus = "escalated"
	AutomationIncidentStatusFailed     AutomationIncidentStatus = "failed"
	AutomationIncidentStatusCanceled   AutomationIncidentStatus = "canceled"
	AutomationIncidentStatusClosed     AutomationIncidentStatus = "closed"
)

const (
	AutomationWatcherStatePending AutomationWatcherState = "pending"
	AutomationWatcherStateRunning AutomationWatcherState = "running"
	AutomationWatcherStatePaused  AutomationWatcherState = "paused"
	AutomationWatcherStateFailed  AutomationWatcherState = "failed"
	AutomationWatcherStateStopped AutomationWatcherState = "stopped"
)

const ResponsePlanSchemaVersionV2 = "sesame.response_plan/v2"

const (
	AutomationPhaseDiagnose  AutomationPhaseName = "diagnose"
	AutomationPhaseRemediate AutomationPhaseName = "remediate"
	AutomationPhaseVerify    AutomationPhaseName = "verify"
	AutomationPhaseEscalate  AutomationPhaseName = "escalate"
)

const (
	AutomationPhaseTransitionNextPhase AutomationPhaseTransitionAction = "next_phase"
	AutomationPhaseTransitionComplete  AutomationPhaseTransitionAction = "complete"
	AutomationPhaseTransitionEscalate  AutomationPhaseTransitionAction = "escalate"
	AutomationPhaseTransitionCancel    AutomationPhaseTransitionAction = "cancel"
)

const (
	IncidentPhaseReductionAllMustSucceed IncidentPhaseReduction = "all_must_succeed"
	IncidentPhaseReductionAnySuccess     IncidentPhaseReduction = "any_success"
	IncidentPhaseReductionBestEffort     IncidentPhaseReduction = "best_effort"
)

const (
	IncidentPhaseStatusPending          IncidentPhaseStatus = "pending"
	IncidentPhaseStatusRunning          IncidentPhaseStatus = "running"
	IncidentPhaseStatusAwaitingApproval IncidentPhaseStatus = "awaiting_approval"
	IncidentPhaseStatusCompleted        IncidentPhaseStatus = "completed"
	IncidentPhaseStatusFailed           IncidentPhaseStatus = "failed"
	IncidentPhaseStatusCanceled         IncidentPhaseStatus = "canceled"
)

const (
	AutomationAssumptionSourceNormalizer  AutomationAssumptionSource = "normalizer"
	AutomationAssumptionSourceSystemSkill AutomationAssumptionSource = "system_skill"
	AutomationAssumptionSourceDomainSkill AutomationAssumptionSource = "domain_skill"
)

const (
	DispatchAttemptStatusPlanned          DispatchAttemptStatus = "planned"
	DispatchAttemptStatusAwaitingApproval DispatchAttemptStatus = "awaiting_approval"
	DispatchAttemptStatusRunning          DispatchAttemptStatus = "running"
	DispatchAttemptStatusInterrupted      DispatchAttemptStatus = "interrupted"
	DispatchAttemptStatusCompleted        DispatchAttemptStatus = "completed"
	DispatchAttemptStatusFailed           DispatchAttemptStatus = "failed"
	DispatchAttemptStatusTimedOut         DispatchAttemptStatus = "timed_out"
	DispatchAttemptStatusCanceled         DispatchAttemptStatus = "canceled"
)

const (
	DeliveryChannelStatusPending  DeliveryChannelStatus = "pending"
	DeliveryChannelStatusEmitted  DeliveryChannelStatus = "emitted"
	DeliveryChannelStatusReady    DeliveryChannelStatus = "ready"
	DeliveryChannelStatusQueued   DeliveryChannelStatus = "queued"
	DeliveryChannelStatusInjected DeliveryChannelStatus = "injected"
	DeliveryChannelStatusDisabled DeliveryChannelStatus = "disabled"
	DeliveryChannelStatusSkipped  DeliveryChannelStatus = "skipped"
	DeliveryChannelStatusFailed   DeliveryChannelStatus = "failed"
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

type ChildAgentTemplate struct {
	AgentID             string   `json:"agent_id"`
	Purpose             string   `json:"purpose,omitempty"`
	PromptTemplate      string   `json:"prompt_template,omitempty"`
	ActivatedSkillNames []string `json:"activated_skill_names,omitempty"`
	OutputContractRef   string   `json:"output_contract_ref,omitempty"`
	TimeoutSeconds      int      `json:"timeout_seconds,omitempty"`
	MaxAttempts         int      `json:"max_attempts,omitempty"`
	Concurrency         int      `json:"concurrency,omitempty"`
	AllowElevation      bool     `json:"allow_elevation,omitempty"`
}

type ChildAgentStrategyEscalationCondition struct {
	WhenStatus []string `json:"when_status"`
}

type ChildAgentStrategyCompletionPolicy struct {
	ResumeWatcherOnSuccess *bool `json:"resume_watcher_on_success"`
	ResumeWatcherOnFailure *bool `json:"resume_watcher_on_failure"`
}

type ChildAgentStrategyFailurePolicy struct {
	HandoffToHuman         *bool `json:"handoff_to_human"`
	KeepPaused             *bool `json:"keep_paused"`
	NotifyViaExternalSkill *bool `json:"notify_via_external_skill"`
}

type ChildAgentTemplateStrategy struct {
	Goal                string                                `json:"goal"`
	EscalationCondition ChildAgentStrategyEscalationCondition `json:"escalation_condition"`
	CompletionPolicy    ChildAgentStrategyCompletionPolicy    `json:"completion_policy"`
	FailurePolicy       ChildAgentStrategyFailurePolicy       `json:"failure_policy"`
}

type ChildAgentTemplateSkills struct {
	Required []string `json:"required"`
	Optional []string `json:"optional"`
}

type AutomationPhasePlan struct {
	Phase       AutomationPhaseName             `json:"phase"`
	ChildAgents []ChildAgentTemplate            `json:"child_agents,omitempty"`
	OnSuccess   AutomationPhaseTransitionAction `json:"on_success,omitempty"`
	OnFailure   AutomationPhaseTransitionAction `json:"on_failure,omitempty"`
}

type ResponsePlanV2 struct {
	SchemaVersion string                `json:"schema_version"`
	Phases        []AutomationPhasePlan `json:"phases"`
}

type AutomationSpec struct {
	ID               string                 `json:"id"`
	Title            string                 `json:"title"`
	WorkspaceRoot    string                 `json:"workspace_root"`
	Goal             string                 `json:"goal"`
	State            AutomationState        `json:"state"`
	Context          AutomationContext      `json:"context"`
	Signals          []AutomationSignal     `json:"signals"`
	IncidentPolicy   json.RawMessage        `json:"incident_policy"`
	ResponsePlan     json.RawMessage        `json:"response_plan"`
	VerificationPlan json.RawMessage        `json:"verification_plan"`
	EscalationPolicy json.RawMessage        `json:"escalation_policy"`
	DeliveryPolicy   json.RawMessage        `json:"delivery_policy"`
	RuntimePolicy    json.RawMessage        `json:"runtime_policy"`
	WatcherLifecycle json.RawMessage        `json:"watcher_lifecycle"`
	RetriggerPolicy  json.RawMessage        `json:"retrigger_policy"`
	RunPolicy        json.RawMessage        `json:"run_policy"`
	Assumptions      []AutomationAssumption `json:"assumptions"`
	CreatedAt        time.Time              `json:"created_at,omitempty"`
	UpdatedAt        time.Time              `json:"updated_at,omitempty"`
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

type TriggerEvent struct {
	EventID       string          `json:"event_id"`
	AutomationID  string          `json:"automation_id"`
	WorkspaceRoot string          `json:"workspace_root"`
	IncidentID    string          `json:"incident_id,omitempty"`
	SignalKind    string          `json:"signal_kind,omitempty"`
	Source        string          `json:"source,omitempty"`
	Summary       string          `json:"summary,omitempty"`
	Payload       json.RawMessage `json:"payload,omitempty"`
	DedupeKey     string          `json:"dedupe_key,omitempty"`
	ObservedAt    time.Time       `json:"observed_at,omitempty"`
	CreatedAt     time.Time       `json:"created_at,omitempty"`
	UpdatedAt     time.Time       `json:"updated_at,omitempty"`
}

type IncidentPhaseState struct {
	IncidentID             string                 `json:"incident_id"`
	AutomationID           string                 `json:"automation_id"`
	WorkspaceRoot          string                 `json:"workspace_root"`
	Phase                  AutomationPhaseName    `json:"phase"`
	Reduction              IncidentPhaseReduction `json:"reduction"`
	Status                 IncidentPhaseStatus    `json:"status"`
	DispatchIDs            []string               `json:"dispatch_ids"`
	ActiveDispatchCount    int                    `json:"active_dispatch_count"`
	CompletedDispatchCount int                    `json:"completed_dispatch_count"`
	FailedDispatchCount    int                    `json:"failed_dispatch_count"`
	CreatedAt              time.Time              `json:"created_at,omitempty"`
	UpdatedAt              time.Time              `json:"updated_at,omitempty"`
}

type DispatchAttempt struct {
	DispatchID          string                `json:"dispatch_id"`
	IncidentID          string                `json:"incident_id"`
	AutomationID        string                `json:"automation_id"`
	WorkspaceRoot       string                `json:"workspace_root"`
	Phase               AutomationPhaseName   `json:"phase"`
	Attempt             int                   `json:"attempt"`
	Status              DispatchAttemptStatus `json:"status"`
	TaskID              string                `json:"task_id,omitempty"`
	BackgroundSessionID string                `json:"background_session_id,omitempty"`
	BackgroundTurnID    string                `json:"background_turn_id,omitempty"`
	ChildAgentID        string                `json:"child_agent_id,omitempty"`
	PromptHash          string                `json:"prompt_hash,omitempty"`
	ActivatedSkillNames []string              `json:"activated_skill_names,omitempty"`
	OutputContractRef   string                `json:"output_contract_ref,omitempty"`
	ContinuationID      string                `json:"continuation_id,omitempty"`
	PermissionRequestID string                `json:"permission_request_id,omitempty"`
	ApprovalQueueKey    string                `json:"approval_queue_key,omitempty"`
	PreferredSessionID  string                `json:"preferred_session_id,omitempty"`
	StartedAt           time.Time             `json:"started_at,omitempty"`
	FinishedAt          time.Time             `json:"finished_at,omitempty"`
	Error               string                `json:"error,omitempty"`
	CreatedAt           time.Time             `json:"created_at,omitempty"`
	UpdatedAt           time.Time             `json:"updated_at,omitempty"`
}

type DeliveryChannelStatusRecord struct {
	Status DeliveryChannelStatus `json:"status"`
}

type DeliveryChannelSet struct {
	Notice    DeliveryChannelStatusRecord `json:"notice"`
	Mailbox   DeliveryChannelStatusRecord `json:"mailbox"`
	Injection DeliveryChannelStatusRecord `json:"injection"`
}

type DeliveryRecord struct {
	DeliveryID    string             `json:"delivery_id"`
	WorkspaceRoot string             `json:"workspace_root"`
	AutomationID  string             `json:"automation_id"`
	IncidentID    string             `json:"incident_id"`
	DispatchID    string             `json:"dispatch_id"`
	SummaryRef    string             `json:"summary_ref,omitempty"`
	Channels      DeliveryChannelSet `json:"channels"`
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

type TriggerEventFilter struct {
	WorkspaceRoot string `json:"workspace_root,omitempty"`
	AutomationID  string `json:"automation_id,omitempty"`
	IncidentID    string `json:"incident_id,omitempty"`
	DedupeKey     string `json:"dedupe_key,omitempty"`
	Limit         int    `json:"limit,omitempty"`
}

type DispatchAttemptFilter struct {
	WorkspaceRoot string                `json:"workspace_root,omitempty"`
	AutomationID  string                `json:"automation_id,omitempty"`
	IncidentID    string                `json:"incident_id,omitempty"`
	Status        DispatchAttemptStatus `json:"status,omitempty"`
	Limit         int                   `json:"limit,omitempty"`
}

type DeliveryRecordFilter struct {
	WorkspaceRoot string `json:"workspace_root,omitempty"`
	AutomationID  string `json:"automation_id,omitempty"`
	IncidentID    string `json:"incident_id,omitempty"`
	DispatchID    string `json:"dispatch_id,omitempty"`
	Limit         int    `json:"limit,omitempty"`
}

type PendingAutomationPermission struct {
	RequestID           string `json:"request_id"`
	WorkspaceRoot       string `json:"workspace_root"`
	AutomationID        string `json:"automation_id"`
	IncidentID          string `json:"incident_id"`
	DispatchID          string `json:"dispatch_id"`
	BackgroundSessionID string `json:"background_session_id"`
	BackgroundTurnID    string `json:"background_turn_id"`
	PreferredSessionID  string `json:"preferred_session_id,omitempty"`
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
