package tools

import (
	"context"

	"go-agent/internal/permissions"
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
}

type ExecContext struct {
	WorkspaceRoot    string
	PermissionEngine *permissions.Engine
}

type Tool interface {
	Definition() Definition
	IsConcurrencySafe() bool
	Execute(context.Context, Call, ExecContext) (Result, error)
}
