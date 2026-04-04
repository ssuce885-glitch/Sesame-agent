package tools

import (
	"context"
	"time"

	"go-agent/internal/runtime"
)

const shellCommandMaxOutputBytes = 256
const shellCommandTimeoutSeconds = 30

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
	shellCtx, cancel := context.WithTimeout(ctx, shellCommandTimeoutSeconds*time.Second)
	defer cancel()

	output, err := runtime.RunCommand(shellCtx, execCtx.WorkspaceRoot, command, shellCommandMaxOutputBytes)
	if err != nil {
		return Result{}, err
	}

	return Result{Text: string(output)}, nil
}
