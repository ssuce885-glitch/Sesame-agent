package tools

import (
	"context"
	"time"

	"go-agent/internal/model"
	"go-agent/internal/permissions"
	"go-agent/internal/roles"
	"go-agent/internal/runtimegraph"
	"go-agent/internal/scheduler"
	"go-agent/internal/session"
	"go-agent/internal/task"
	"go-agent/internal/types"
)

type Call struct {
	ID    string
	Name  string
	Input map[string]any
}

func (c Call) StringInput(key string) string {
	v, _ := c.Input[key].(string)
	return v
}

type Result struct {
	Text         string
	ArtifactPath string
	ModelText    string
}

type ArtifactRef struct {
	Path      string
	Kind      string
	SizeBytes int64
}

type ToolExecutionResult struct {
	Result
	Data         any
	PreviewText  string
	Artifacts    []ArtifactRef
	Metadata     map[string]any
	NewItems     []model.ConversationItem
	Interrupt    *ToolInterrupt
	CompleteTurn bool
}

type ModelToolResult struct {
	Text       string
	Structured any
	IsError    bool
}

type ToolInterrupt struct {
	Reason          string
	Notice          string
	EventType       string
	EventPayload    any
	DeferToolResult bool
}

type ExecContext struct {
	WorkspaceRoot            string
	GlobalConfigRoot         string
	ActiveSkillNames         []string
	InjectedEnv              map[string]string
	PermissionEngine         *permissions.Engine
	AutomationService        AutomationService
	RoleService              RoleService
	SessionDelegationService session.RoleDelegationService
	TaskManager              *task.Manager
	RuntimeService           *runtimegraph.Service
	SchedulerService         *scheduler.Service
	TurnContext              *runtimegraph.TurnContext
	ToolRunID                string
	EventSink                EventSink
}

type EventSink interface {
	Emit(context.Context, types.Event) error
}

type AutomationService interface {
	ApplyRequest(context.Context, types.ApplyAutomationRequest) (types.AutomationSpec, error)
	Apply(context.Context, types.AutomationSpec) (types.AutomationSpec, error)
	Get(context.Context, string) (types.AutomationSpec, bool, error)
	List(context.Context, types.AutomationListFilter) ([]types.AutomationSpec, error)
	Control(context.Context, string, types.AutomationControlAction) (types.AutomationSpec, bool, error)
	Delete(context.Context, string) (bool, error)
	EmitTrigger(context.Context, types.AutomationTriggerRequest) (types.TriggerEvent, error)
	RecordHeartbeat(context.Context, types.AutomationHeartbeatRequest) (types.AutomationHeartbeat, error)
	GetWatcher(context.Context, string) (types.AutomationWatcherRuntime, bool, error)
	ListHeartbeats(context.Context, types.AutomationHeartbeatFilter) ([]types.AutomationHeartbeat, error)
}

type RoleService interface {
	List(string) (roles.Catalog, error)
	Get(string, string) (roles.Spec, error)
	Create(string, roles.UpsertInput) (roles.Spec, error)
	Update(string, roles.UpsertInput) (roles.Spec, error)
}

type ResourceClaimMode string

const (
	ResourceClaimShared    ResourceClaimMode = "shared"
	ResourceClaimExclusive ResourceClaimMode = "exclusive"
)

type ResourceClaim struct {
	Key  string
	Mode ResourceClaimMode
}

type ResourceLockStats struct {
	Waited   time.Duration
	Acquired []ResourceClaim
}

type DecodedCall struct {
	Call  Call
	Input any
}

type Tool interface {
	Definition() Definition
	IsConcurrencySafe() bool
	Execute(context.Context, Call, ExecContext) (Result, error)
}

type enablableTool interface {
	IsEnabled(ExecContext) bool
}

type decodingTool interface {
	Decode(Call) (DecodedCall, error)
}

type decodedExecutor interface {
	ExecuteDecoded(context.Context, DecodedCall, ExecContext) (ToolExecutionResult, error)
}

type concurrencyAwareTool interface {
	IsConcurrencySafeCall(DecodedCall, ExecContext) bool
}

type resourceAwareTool interface {
	ResourceClaims(DecodedCall, ExecContext) []ResourceClaim
}

type permissionAwareTool interface {
	CheckPermission(context.Context, DecodedCall, ExecContext) (permissions.Decision, string, error)
}

type modelResultMapper interface {
	MapModelResult(ToolExecutionResult) ModelToolResult
}

type turnCompletingTool interface {
	CompletesTurnOnSuccess() bool
}

func CompletesTurnOnSuccess(tool Tool) bool {
	terminal, ok := tool.(turnCompletingTool)
	return ok && terminal.CompletesTurnOnSuccess()
}
