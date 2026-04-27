package types

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type AutomationMode string
type AutomationState string
type AutomationControlAction string
type AutomationWatcherState string
type AutomationWatcherHoldKind string
type AutomationAssumptionSource string
type ChildAgentOutcome string
type AutomationDetectorStatus string

const (
	AutomationModeSimple AutomationMode = "simple"
)

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

var canonicalAutomationOwnerRoleIDPattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,63}$`)
var canonicalAutomationIDPattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,127}$`)

type SimpleAutomationPolicy struct {
	OnSuccess string `json:"on_success,omitempty"`
	OnFailure string `json:"on_failure,omitempty"`
	OnBlocked string `json:"on_blocked,omitempty"`
}

type SimpleAutomationBuilderInput struct {
	AutomationID     string                 `json:"automation_id"`
	Owner            string                 `json:"owner"`
	WatchScript      string                 `json:"watch_script"`
	IntervalSeconds  int                    `json:"interval_seconds"`
	Title            string                 `json:"title,omitempty"`
	Goal             string                 `json:"goal,omitempty"`
	TimeoutSeconds   int                    `json:"timeout_seconds,omitempty"`
	ReportTarget     string                 `json:"report_target,omitempty"`
	EscalationTarget string                 `json:"escalation_target,omitempty"`
	SimplePolicy     SimpleAutomationPolicy `json:"simple_policy,omitempty"`
}

type SimpleAutomationRun struct {
	AutomationID string    `json:"automation_id"`
	DedupeKey    string    `json:"dedupe_key"`
	Owner        string    `json:"owner"`
	TaskID       string    `json:"task_id,omitempty"`
	LastStatus   string    `json:"last_status,omitempty"`
	LastSummary  string    `json:"last_summary,omitempty"`
	CreatedAt    time.Time `json:"created_at,omitempty"`
	UpdatedAt    time.Time `json:"updated_at,omitempty"`
}

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

type AutomationHeartbeatFilter struct {
	WorkspaceRoot string `json:"workspace_root,omitempty"`
	AutomationID  string `json:"automation_id,omitempty"`
	WatcherID     string `json:"watcher_id,omitempty"`
	Limit         int    `json:"limit,omitempty"`
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

func (spec *AutomationSpec) UnmarshalJSON(data []byte) error {
	type automationSpecAlias AutomationSpec
	aux := struct {
		*automationSpecAlias
		Assumptions json.RawMessage `json:"assumptions"`
	}{
		automationSpecAlias: (*automationSpecAlias)(spec),
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	assumptions, err := decodeAutomationAssumptionsJSON(aux.Assumptions)
	if err != nil {
		return err
	}
	spec.Assumptions = assumptions
	return nil
}

func NormalizeRoleAutomationOwner(raw string) string {
	raw = strings.TrimSpace(raw)
	if !strings.HasPrefix(raw, "role:") {
		return ""
	}
	roleID := strings.TrimSpace(strings.TrimPrefix(raw, "role:"))
	if !canonicalAutomationOwnerRoleIDPattern.MatchString(roleID) {
		return ""
	}
	return "role:" + roleID
}

func NormalizeAutomationID(raw string) string {
	raw = strings.TrimSpace(raw)
	if !canonicalAutomationIDPattern.MatchString(raw) {
		return ""
	}
	return raw
}

func NormalizeAutomationAssumptions(values []AutomationAssumption) []AutomationAssumption {
	if len(values) == 0 {
		return []AutomationAssumption{}
	}
	out := make([]AutomationAssumption, 0, len(values))
	seenKeys := make(map[string]int, len(values))
	for _, value := range values {
		value.Key = strings.TrimSpace(value.Key)
		value.Field = strings.TrimSpace(value.Field)
		value.Value = normalizeAutomationRawJSONValue(value.Value)
		value.Reason = strings.TrimSpace(value.Reason)
		value.Source = normalizeAutomationAssumptionSource(value.Source)
		if value.Key == "" && value.Field == "" && len(value.Value) == 0 && value.Reason == "" && value.Source == "" {
			continue
		}
		if value.Field == "" || len(value.Value) == 0 || !json.Valid(value.Value) {
			continue
		}
		value.Key = normalizeAutomationAssumptionKey(value.Key, value.Field, seenKeys)
		out = append(out, value)
	}
	if len(out) == 0 {
		return []AutomationAssumption{}
	}
	return out
}

func decodeAutomationAssumptionsJSON(raw json.RawMessage) ([]AutomationAssumption, error) {
	raw = normalizeAutomationRawJSONValue(raw)
	if len(raw) == 0 || string(raw) == "null" {
		return []AutomationAssumption{}, nil
	}

	var structured []AutomationAssumption
	if err := json.Unmarshal(raw, &structured); err == nil {
		return NormalizeAutomationAssumptions(structured), nil
	}

	var legacy []string
	if err := json.Unmarshal(raw, &legacy); err == nil {
		return normalizeLegacyAutomationAssumptions(legacy)
	}

	return nil, fmt.Errorf("decode automation assumptions: unsupported payload %s", string(raw))
}

func normalizeLegacyAutomationAssumptions(values []string) ([]AutomationAssumption, error) {
	out := make([]AutomationAssumption, 0, len(values))
	for index, value := range values {
		value = normalizeAutomationFirstNonEmptyTrimmed(value)
		if value == "" {
			continue
		}
		encoded, err := json.Marshal(value)
		if err != nil {
			return nil, err
		}
		out = append(out, AutomationAssumption{
			Key:    fmt.Sprintf("assumption_legacy_%d", index+1),
			Field:  fmt.Sprintf("legacy_assumptions[%d]", index),
			Value:  encoded,
			Reason: "migrated from legacy assumptions string list",
			Source: AutomationAssumptionSourceNormalizer,
		})
	}
	return NormalizeAutomationAssumptions(out), nil
}

func normalizeAutomationFirstNonEmptyTrimmed(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func normalizeAutomationAssumptionSource(source AutomationAssumptionSource) AutomationAssumptionSource {
	switch strings.ToLower(strings.TrimSpace(string(source))) {
	case string(AutomationAssumptionSourceSystemSkill):
		return AutomationAssumptionSourceSystemSkill
	case string(AutomationAssumptionSourceDomainSkill):
		return AutomationAssumptionSourceDomainSkill
	default:
		return AutomationAssumptionSourceNormalizer
	}
}

func normalizeAutomationAssumptionKey(current string, field string, seen map[string]int) string {
	base := strings.TrimSpace(current)
	if base == "" {
		base = "assumption_" + sanitizeAutomationKeyPart(field)
	}
	if strings.TrimSpace(base) == "" || base == "assumption_" {
		base = "assumption"
	}
	count := seen[base]
	seen[base] = count + 1
	if count == 0 {
		return base
	}
	return base + "_" + strconv.Itoa(count+1)
}

func sanitizeAutomationKeyPart(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return ""
	}
	var builder strings.Builder
	lastUnderscore := false
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			builder.WriteRune(r)
			lastUnderscore = false
			continue
		}
		if !lastUnderscore {
			builder.WriteByte('_')
			lastUnderscore = true
		}
	}
	return strings.Trim(builder.String(), "_")
}

func normalizeAutomationRawJSONValue(raw json.RawMessage) json.RawMessage {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" {
		return nil
	}
	return json.RawMessage(trimmed)
}
