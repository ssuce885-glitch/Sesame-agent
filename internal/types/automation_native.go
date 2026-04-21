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
	Kind         NativeDetectorKind `json:"kind"`
	Args         map[string]any     `json:"args,omitempty"`
}

type NativeIncidentPolicyInput struct {
	AutomationID string             `json:"automation_id"`
	DispatchMode NativeDispatchMode `json:"dispatch_mode"`
	Args         map[string]any     `json:"args,omitempty"`
}

type NativeDispatchPolicyInput struct {
	AutomationID string             `json:"automation_id"`
	Mode         NativeDispatchMode `json:"mode"`
	ActionKind   NativeActionKind   `json:"action_kind"`
	ActionArgs   map[string]any     `json:"action_args,omitempty"`
}
