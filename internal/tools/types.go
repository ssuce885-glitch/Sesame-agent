package tools

import (
	"context"

	"go-agent/internal/permissions"
)

type Call struct {
	Name  string
	Input map[string]any
}

type Result struct {
	Text         string
	ArtifactPath string
}

type ExecContext struct {
	WorkspaceRoot    string
	PermissionEngine *permissions.Engine
}

type Tool interface {
	Name() string
	IsConcurrencySafe() bool
	Execute(context.Context, Call, ExecContext) (Result, error)
}
