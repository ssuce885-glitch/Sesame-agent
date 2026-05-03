package tools

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"time"

	"go-agent/internal/v2/contracts"
)

type shellTool struct{}

const (
	defaultShellToolTimeout = 120 * time.Second
	maxShellToolOutputBytes = 64 * 1024
)

func NewShellTool() contracts.Tool { return &shellTool{} }

func (t *shellTool) Definition() contracts.ToolDefinition {
	return contracts.ToolDefinition{
		Name:        "shell",
		Namespace:   contracts.NamespaceShell,
		Description: "Execute a shell command in the workspace.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"command":     map[string]any{"type": "string", "description": "The shell command to run"},
				"working_dir": map[string]any{"type": "string", "description": "Working directory (defaults to workspace root)"},
			},
			"required": []string{"command"},
		},
	}
}

func (t *shellTool) Execute(ctx context.Context, call contracts.ToolCall, execCtx contracts.ExecContext) (contracts.ToolResult, error) {
	command, _ := call.Args["command"].(string)
	if command == "" {
		return contracts.ToolResult{IsError: true, Output: "command is required"}, nil
	}

	dir, _ := call.Args["working_dir"].(string)
	workspace, err := workspaceRoot(execCtx.WorkspaceRoot)
	if err != nil {
		return contracts.ToolResult{}, err
	}
	if dir == "" {
		dir = workspace
	} else {
		dir, err = resolveWorkspacePath(workspace, dir)
		if err != nil {
			return contracts.ToolResult{IsError: true, Output: err.Error()}, nil
		}
	}

	runCtx, cancel := context.WithTimeout(ctx, defaultShellToolTimeout)
	defer cancel()

	cmd := exec.CommandContext(runCtx, "bash", "-lc", command)
	cmd.Dir = dir
	var stdout, stderr shellOutputBuffer
	stdout.limit = maxShellToolOutputBytes
	stderr.limit = maxShellToolOutputBytes
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err = cmd.Run()

	output := stdout.String()
	if stderr.Len() > 0 {
		if output != "" {
			output += "\n"
		}
		output += stderr.String()
	}
	if runCtx.Err() == context.DeadlineExceeded {
		return contracts.ToolResult{Output: output + fmt.Sprintf("\ncommand timed out after %s", defaultShellToolTimeout), IsError: true}, nil
	}
	if stdout.truncated || stderr.truncated {
		return contracts.ToolResult{Output: output + fmt.Sprintf("\ncommand output truncated after %d bytes", maxShellToolOutputBytes), IsError: true}, nil
	}
	if err != nil {
		return contracts.ToolResult{Output: fmt.Sprintf("%s\nexit error: %v", output, err), IsError: true}, nil
	}
	return contracts.ToolResult{Output: output}, nil
}

type shellOutputBuffer struct {
	bytes.Buffer
	limit     int
	truncated bool
}

func (b *shellOutputBuffer) Write(p []byte) (int, error) {
	if b.limit <= 0 {
		_, _ = b.Buffer.Write(p)
		return len(p), nil
	}
	remaining := b.limit - b.Buffer.Len()
	if remaining <= 0 {
		b.truncated = true
		return len(p), nil
	}
	if len(p) > remaining {
		_, _ = b.Buffer.Write(p[:remaining])
		b.truncated = true
		return len(p), nil
	}
	_, _ = b.Buffer.Write(p)
	return len(p), nil
}
