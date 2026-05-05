package tools

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"time"

	runtimex "go-agent/internal/runtime"
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
		Capabilities: []string{
			string(contracts.CapabilityExecuteLocal),
			string(contracts.CapabilityDestructive),
		},
		Risk: "high",
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
	command = strings.TrimSpace(command)
	if command == "" {
		return contracts.ToolResult{IsError: true, Output: "command is required", Risk: t.Definition().Risk}, nil
	}
	if decision := EvaluateRoleToolAccess(execCtx.RoleSpec, t.Definition().Name); !decision.Allowed {
		return contracts.ToolResult{IsError: true, Output: decision.Reason, Risk: t.Definition().Risk}, nil
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
			return contracts.ToolResult{IsError: true, Output: err.Error(), Risk: t.Definition().Risk}, nil
		}
	}

	timeout, outputLimit, decision := resolveShellExecutionPolicy(execCtx.RoleSpec, command)
	if !decision.Allowed {
		return contracts.ToolResult{IsError: true, Output: decision.Reason, Risk: t.Definition().Risk}, nil
	}

	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := runtimex.NewBashCommandContext(runCtx, command)
	cmd.Dir = dir
	var stdout, stderr shellOutputBuffer
	stdout.limit = outputLimit
	stderr.limit = outputLimit
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
	if outputLimit > 0 && len([]byte(output)) > outputLimit {
		output = string([]byte(output)[:outputLimit])
		return contracts.ToolResult{Output: output + fmt.Sprintf("\ncommand output truncated after %d bytes", outputLimit), IsError: true, Risk: t.Definition().Risk}, nil
	}
	if runCtx.Err() == context.DeadlineExceeded {
		return contracts.ToolResult{Output: output + fmt.Sprintf("\ncommand timed out after %s", timeout), IsError: true, Risk: t.Definition().Risk}, nil
	}
	if stdout.truncated || stderr.truncated {
		return contracts.ToolResult{Output: output + fmt.Sprintf("\ncommand output truncated after %d bytes", outputLimit), IsError: true, Risk: t.Definition().Risk}, nil
	}
	if err != nil {
		return contracts.ToolResult{Output: fmt.Sprintf("%s\nexit error: %v", output, err), IsError: true, Risk: t.Definition().Risk}, nil
	}
	return contracts.ToolResult{Ok: true, Output: output, Risk: t.Definition().Risk}, nil
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
