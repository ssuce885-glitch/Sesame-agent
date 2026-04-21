package types

type NativeDetectorKind string
type NativeDispatchMode string
type NativeActionKind string

const (
	NativeDetectorKindFile    NativeDetectorKind = "file"
	NativeDetectorKindCommand NativeDetectorKind = "command"
	NativeDetectorKindHealth  NativeDetectorKind = "health"
)

const (
	NativeDispatchModeNotifyOnly NativeDispatchMode = "notify_only"
	NativeDispatchModeRunTask    NativeDispatchMode = "run_task"
)

const (
	NativeActionKindDeleteFile NativeActionKind = "delete_file"
	NativeActionKindRunScript  NativeActionKind = "run_script"
	NativeActionKindSendEmail  NativeActionKind = "send_email"
	NativeActionKindNotifyOnly NativeActionKind = "notify_only"
)

type NativeDetectorBuilderInput struct {
	AutomationID string             `json:"automation_id"`
	Title        string             `json:"title"`
	DetectorKind NativeDetectorKind `json:"detector_kind"`
	Target       map[string]any     `json:"target,omitempty"`
	Schedule     map[string]any     `json:"schedule,omitempty"`
	Condition    map[string]any     `json:"condition,omitempty"`
	FactsSchema  map[string]string  `json:"facts_schema,omitempty"`
	Dedupe       map[string]any     `json:"dedupe,omitempty"`
	State        string             `json:"state,omitempty"`
}

type NativeIncidentPolicyInput struct {
	AutomationID     string         `json:"automation_id"`
	CreateIncidentOn string         `json:"create_incident_on"`
	SummaryTemplate  string         `json:"summary_template,omitempty"`
	DedupePolicy     map[string]any `json:"dedupe_policy,omitempty"`
	Severity         string         `json:"severity,omitempty"`
	AutoCloseMinutes int            `json:"auto_close_minutes,omitempty"`
}

type NativeDispatchPolicyInput struct {
	AutomationID string             `json:"automation_id"`
	DispatchMode NativeDispatchMode `json:"dispatch_mode"`
	ActionKind   NativeActionKind   `json:"action_kind"`
	ActionArgs   map[string]string  `json:"action_args,omitempty"`
	Verification map[string]any     `json:"verification,omitempty"`
	Reporting    map[string]any     `json:"reporting,omitempty"`
}
