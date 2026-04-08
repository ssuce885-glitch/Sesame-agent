package tools

import (
	"context"

	"go-agent/internal/model"
	"go-agent/internal/permissions"
	"go-agent/internal/runtimegraph"
	"go-agent/internal/task"
)

type Call struct {
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
	Data        any
	PreviewText string
	Artifacts   []ArtifactRef
	Metadata    map[string]any
	NewItems    []model.ConversationItem
}

type ModelToolResult struct {
	Text       string
	Structured any
	IsError    bool
}

type ExecContext struct {
	WorkspaceRoot    string
	PermissionEngine *permissions.Engine
	TaskManager      *task.Manager
	RuntimeService   *runtimegraph.Service
	TurnContext      *runtimegraph.TurnContext
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

type permissionAwareTool interface {
	CheckPermission(context.Context, DecodedCall, ExecContext) (permissions.Decision, string, error)
}

type modelResultMapper interface {
	MapModelResult(ToolExecutionResult) ModelToolResult
}
