package tools

import (
	"context"

	"go-agent/internal/runtime"
)

type shellTool struct{}

func (shellTool) Definition() Definition {
	return Definition{
		Name:        "shell_command",
		Description: "Run a shell command.",
		InputSchema: objectSchema(map[string]any{
			"command": map[string]any{
				"type":        "string",
				"description": "Shell command to execute.",
			},
		}, "command"),
	}
}

func (shellTool) IsConcurrencySafe() bool { return false }

func (shellTool) Execute(ctx context.Context, call Call, execCtx ExecContext) (Result, error) {
	command := call.Input["command"].(string)
	output, err := runtime.RunCommand(ctx, command)
	if err != nil {
		return Result{}, err
	}

	return Result{Text: string(output)}, nil
}
