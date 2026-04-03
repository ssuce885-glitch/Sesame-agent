package tools

import (
	"context"

	"go-agent/internal/runtime"
)

type shellTool struct{}

func (shellTool) Name() string            { return "shell_command" }
func (shellTool) IsConcurrencySafe() bool { return false }

func (shellTool) Execute(ctx context.Context, call Call, execCtx ExecContext) (Result, error) {
	command := call.Input["command"].(string)
	output, err := runtime.RunCommand(ctx, command)
	if err != nil {
		return Result{}, err
	}

	return Result{Text: string(output)}, nil
}
