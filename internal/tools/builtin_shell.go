package tools

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go-agent/internal/permissions"
	"go-agent/internal/runtime"
)

const defaultShellCommandMaxOutputBytes = 256
const defaultShellCommandTimeoutSeconds = 30

var (
	shellCommandMaxOutputBytes = defaultShellCommandMaxOutputBytes
	shellCommandTimeoutSeconds = defaultShellCommandTimeoutSeconds
)

func SetShellCommandGuardrails(maxOutputBytes, timeoutSeconds int) {
	shellCommandMaxOutputBytes = maxOutputBytes
	shellCommandTimeoutSeconds = timeoutSeconds
}

type shellTool struct{}

type shellCommandInput struct {
	Command        string
	TimeoutSeconds int
	MaxOutputBytes int
}

type ShellCommandOutput struct {
	Command        string `json:"command"`
	Output         string `json:"output"`
	TimeoutSeconds int    `json:"timeout_seconds"`
	MaxOutputBytes int    `json:"max_output_bytes"`
	Classification string `json:"classification"`
}

type shellCommandSafety string

const (
	shellCommandSafetyUnknown     shellCommandSafety = "unknown"
	shellCommandSafetyReadOnly    shellCommandSafety = "read_only"
	shellCommandSafetyMutating    shellCommandSafety = "mutating"
	shellCommandSafetyDestructive shellCommandSafety = "destructive"
)

func (shellTool) Definition() Definition {
	return Definition{
		Name:        "shell_command",
		Description: "Run a shell command.",
		InputSchema: objectSchema(map[string]any{
			"command": map[string]any{
				"type":        "string",
				"description": "Shell command to execute.",
			},
			"timeout_seconds": map[string]any{
				"type":        "integer",
				"description": "Optional timeout in seconds, capped by runtime guardrails.",
			},
			"max_output_bytes": map[string]any{
				"type":        "integer",
				"description": "Optional stdout/stderr cap in bytes, capped by runtime guardrails.",
			},
		}, "command"),
		OutputSchema: objectSchema(map[string]any{
			"command":          map[string]any{"type": "string"},
			"output":           map[string]any{"type": "string"},
			"timeout_seconds":  map[string]any{"type": "integer"},
			"max_output_bytes": map[string]any{"type": "integer"},
			"classification":   map[string]any{"type": "string"},
		}, "command", "output", "timeout_seconds", "max_output_bytes", "classification"),
	}
}

func (shellTool) IsConcurrencySafe() bool { return false }

func (t shellTool) Decode(call Call) (DecodedCall, error) {
	if call.Input == nil {
		call.Input = map[string]any{}
	}
	command := strings.TrimSpace(call.StringInput("command"))
	if command == "" {
		return DecodedCall{}, fmt.Errorf("shell command is required")
	}

	timeoutSeconds, err := decodeShellPositiveInt(call.Input["timeout_seconds"], shellCommandTimeoutSeconds)
	if err != nil {
		return DecodedCall{}, fmt.Errorf("timeout_seconds %w", err)
	}
	if timeoutSeconds > shellCommandTimeoutSeconds {
		return DecodedCall{}, fmt.Errorf("timeout_seconds exceeds max allowed (%d)", shellCommandTimeoutSeconds)
	}

	maxOutputBytes, err := decodeShellPositiveInt(call.Input["max_output_bytes"], shellCommandMaxOutputBytes)
	if err != nil {
		return DecodedCall{}, fmt.Errorf("max_output_bytes %w", err)
	}
	if maxOutputBytes > shellCommandMaxOutputBytes {
		return DecodedCall{}, fmt.Errorf("max_output_bytes exceeds max allowed (%d)", shellCommandMaxOutputBytes)
	}

	normalized := Call{
		Name: call.Name,
		Input: map[string]any{
			"command":          command,
			"timeout_seconds":  timeoutSeconds,
			"max_output_bytes": maxOutputBytes,
		},
	}
	return DecodedCall{
		Call: normalized,
		Input: shellCommandInput{
			Command:        command,
			TimeoutSeconds: timeoutSeconds,
			MaxOutputBytes: maxOutputBytes,
		},
	}, nil
}

func (t shellTool) Execute(ctx context.Context, call Call, execCtx ExecContext) (Result, error) {
	decoded, err := t.Decode(call)
	if err != nil {
		return Result{}, err
	}
	output, err := t.ExecuteDecoded(ctx, decoded, execCtx)
	return output.Result, err
}

func (shellTool) ExecuteDecoded(ctx context.Context, decoded DecodedCall, execCtx ExecContext) (ToolExecutionResult, error) {
	input, _ := decoded.Input.(shellCommandInput)
	if input.Command == "" {
		input.Command = strings.TrimSpace(decoded.Call.StringInput("command"))
	}
	if input.TimeoutSeconds <= 0 {
		input.TimeoutSeconds = shellCommandTimeoutSeconds
	}
	if input.MaxOutputBytes <= 0 {
		input.MaxOutputBytes = shellCommandMaxOutputBytes
	}
	shellCtx, cancel := context.WithTimeout(ctx, time.Duration(input.TimeoutSeconds)*time.Second)
	defer cancel()

	output, err := runtime.RunCommand(shellCtx, execCtx.WorkspaceRoot, input.Command, input.MaxOutputBytes)
	if err != nil {
		return ToolExecutionResult{}, err
	}

	classification := string(classifyShellCommand(input.Command))
	text := string(output)
	return ToolExecutionResult{
		Result: Result{
			Text:      text,
			ModelText: text,
		},
		Data: ShellCommandOutput{
			Command:        input.Command,
			Output:         text,
			TimeoutSeconds: input.TimeoutSeconds,
			MaxOutputBytes: input.MaxOutputBytes,
			Classification: classification,
		},
		PreviewText: PreviewText(text, 256),
		Metadata: map[string]any{
			"classification": classification,
		},
	}, nil
}

func (shellTool) IsConcurrencySafeCall(decoded DecodedCall, _ ExecContext) bool {
	input, _ := decoded.Input.(shellCommandInput)
	if input.Command == "" {
		input.Command = strings.TrimSpace(decoded.Call.StringInput("command"))
	}
	return classifyShellCommand(input.Command) == shellCommandSafetyReadOnly
}

func (shellTool) CheckPermission(_ context.Context, decoded DecodedCall, _ ExecContext) (permissions.Decision, string, error) {
	input, _ := decoded.Input.(shellCommandInput)
	if input.Command == "" {
		input.Command = strings.TrimSpace(decoded.Call.StringInput("command"))
	}
	if classifyShellCommand(input.Command) == shellCommandSafetyDestructive {
		return permissions.DecisionAsk, "destructive shell command detected", nil
	}
	return permissions.DecisionAllow, "", nil
}

func (shellTool) MapModelResult(output ToolExecutionResult) ModelToolResult {
	text := output.ModelText
	if strings.TrimSpace(text) == "" {
		text = output.Text
	}
	return ModelToolResult{
		Text:       text,
		Structured: output.Data,
	}
}

func decodeShellPositiveInt(raw any, fallback int) (int, error) {
	if raw == nil {
		return fallback, nil
	}
	switch value := raw.(type) {
	case int:
		if value <= 0 {
			return 0, fmt.Errorf("must be greater than zero")
		}
		return value, nil
	case int32:
		if value <= 0 {
			return 0, fmt.Errorf("must be greater than zero")
		}
		return int(value), nil
	case int64:
		if value <= 0 {
			return 0, fmt.Errorf("must be greater than zero")
		}
		return int(value), nil
	case float64:
		if value <= 0 || value != float64(int(value)) {
			return 0, fmt.Errorf("must be a positive integer")
		}
		return int(value), nil
	default:
		return 0, fmt.Errorf("must be a positive integer")
	}
}

func classifyShellCommand(command string) shellCommandSafety {
	segments := splitShellCommandSegments(command)
	if len(segments) == 0 {
		return shellCommandSafetyUnknown
	}

	overall := shellCommandSafetyReadOnly
	for _, segment := range segments {
		classification := classifyShellCommandSegment(segment)
		switch classification {
		case shellCommandSafetyDestructive:
			return classification
		case shellCommandSafetyMutating:
			overall = classification
		case shellCommandSafetyUnknown:
			if overall == shellCommandSafetyReadOnly {
				overall = classification
			}
		}
	}
	return overall
}

func splitShellCommandSegments(command string) []string {
	replaced := strings.NewReplacer(
		"&&", "\n",
		"||", "\n",
		"|", "\n",
		";", "\n",
		"&", "\n",
	).Replace(command)
	parts := strings.Split(replaced, "\n")
	segments := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			segments = append(segments, part)
		}
	}
	return segments
}

func classifyShellCommandSegment(segment string) shellCommandSafety {
	lower := strings.ToLower(strings.TrimSpace(segment))
	if lower == "" {
		return shellCommandSafetyUnknown
	}
	if strings.Contains(lower, ">>") || strings.Contains(lower, ">") {
		return shellCommandSafetyMutating
	}

	fields := strings.Fields(lower)
	if len(fields) == 0 {
		return shellCommandSafetyUnknown
	}

	switch fields[0] {
	case "cd", "echo", "dir", "type", "find", "findstr", "fc", "where", "tree", "sort", "more":
		return shellCommandSafetyReadOnly
	case "copy", "xcopy", "robocopy", "move", "ren", "rename", "mkdir", "md", "mklink", "attrib":
		return shellCommandSafetyMutating
	case "del", "erase", "rmdir", "rd", "format", "shutdown", "taskkill":
		return shellCommandSafetyDestructive
	case "git":
		return classifyGitShellCommand(fields[1:])
	default:
		return shellCommandSafetyUnknown
	}
}

func classifyGitShellCommand(args []string) shellCommandSafety {
	if len(args) == 0 {
		return shellCommandSafetyUnknown
	}
	switch args[0] {
	case "status", "diff", "show", "log", "branch", "rev-parse", "grep", "ls-files", "remote":
		return shellCommandSafetyReadOnly
	case "config":
		for _, arg := range args[1:] {
			if strings.HasPrefix(arg, "--get") {
				return shellCommandSafetyReadOnly
			}
		}
		return shellCommandSafetyMutating
	case "clean", "rm":
		return shellCommandSafetyDestructive
	case "reset":
		for _, arg := range args[1:] {
			if arg == "--hard" {
				return shellCommandSafetyDestructive
			}
		}
		return shellCommandSafetyMutating
	case "add", "apply", "am", "checkout", "clone", "commit", "merge", "pull", "push", "rebase", "restore", "stash", "switch", "tag", "worktree":
		return shellCommandSafetyMutating
	default:
		return shellCommandSafetyUnknown
	}
}
