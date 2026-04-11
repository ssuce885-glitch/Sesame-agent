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
	Workdir        string
	TimeoutSeconds int
	MaxOutputBytes int
}

type ShellCommandOutput struct {
	Command        string `json:"command"`
	Workdir        string `json:"workdir,omitempty"`
	Output         string `json:"output"`
	Stdout         string `json:"stdout"`
	Stderr         string `json:"stderr"`
	ExitCode       int    `json:"exit_code"`
	TimedOut       bool   `json:"timed_out"`
	DurationMs     int64  `json:"duration_ms"`
	Truncated      bool   `json:"truncated"`
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
		Aliases:     []string{"shell"},
		Description: "Run a shell command.",
		InputSchema: objectSchema(map[string]any{
			"command": map[string]any{
				"type":        "string",
				"description": "Shell command to execute.",
			},
			"workdir": map[string]any{
				"type":        "string",
				"description": "Optional working directory relative to the workspace root.",
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
			"workdir":          map[string]any{"type": "string"},
			"output":           map[string]any{"type": "string"},
			"stdout":           map[string]any{"type": "string"},
			"stderr":           map[string]any{"type": "string"},
			"exit_code":        map[string]any{"type": "integer"},
			"timed_out":        map[string]any{"type": "boolean"},
			"duration_ms":      map[string]any{"type": "integer"},
			"truncated":        map[string]any{"type": "boolean"},
			"timeout_seconds":  map[string]any{"type": "integer"},
			"max_output_bytes": map[string]any{"type": "integer"},
			"classification":   map[string]any{"type": "string"},
		}, "command", "output", "stdout", "stderr", "exit_code", "timed_out", "duration_ms", "truncated", "timeout_seconds", "max_output_bytes", "classification"),
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
	workdir := strings.TrimSpace(call.StringInput("workdir"))

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
			"workdir":          workdir,
			"timeout_seconds":  timeoutSeconds,
			"max_output_bytes": maxOutputBytes,
		},
	}
	return DecodedCall{
		Call: normalized,
		Input: shellCommandInput{
			Command:        command,
			Workdir:        workdir,
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
	if input.Workdir == "" {
		input.Workdir = strings.TrimSpace(decoded.Call.StringInput("workdir"))
	}
	if input.TimeoutSeconds <= 0 {
		input.TimeoutSeconds = shellCommandTimeoutSeconds
	}
	if input.MaxOutputBytes <= 0 {
		input.MaxOutputBytes = shellCommandMaxOutputBytes
	}
	shellCtx, cancel := context.WithTimeout(ctx, time.Duration(input.TimeoutSeconds)*time.Second)
	defer cancel()

	commandWorkdir := execCtx.WorkspaceRoot
	if input.Workdir != "" {
		commandWorkdir = resolveWorkspacePath(execCtx.WorkspaceRoot, input.Workdir)
		if err := runtime.WithinWorkspace(execCtx.WorkspaceRoot, commandWorkdir); err != nil {
			return ToolExecutionResult{}, err
		}
	}

	output, err := runtime.RunCommandWithEnv(shellCtx, commandWorkdir, input.Command, input.MaxOutputBytes, execCtx.InjectedEnv)
	if err != nil {
		return ToolExecutionResult{}, err
	}

	classification := string(classifyShellCommand(input.Command))
	text := output.AggregatedOutput
	modelText := renderShellCommandModelText(input.Command, commandWorkdir, input.TimeoutSeconds, input.MaxOutputBytes, classification, output)
	previewText := renderShellCommandPreview(input.Command, output)
	return ToolExecutionResult{
		Result: Result{
			Text:      text,
			ModelText: modelText,
		},
		Data: ShellCommandOutput{
			Command:        input.Command,
			Workdir:        commandWorkdir,
			Output:         text,
			Stdout:         output.Stdout,
			Stderr:         output.Stderr,
			ExitCode:       output.ExitCode,
			TimedOut:       output.TimedOut,
			DurationMs:     output.Duration.Milliseconds(),
			Truncated:      output.Truncated,
			TimeoutSeconds: input.TimeoutSeconds,
			MaxOutputBytes: input.MaxOutputBytes,
			Classification: classification,
		},
		PreviewText: previewText,
		Metadata: map[string]any{
			"classification": classification,
			"exit_code":      output.ExitCode,
			"timed_out":      output.TimedOut,
			"truncated":      output.Truncated,
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

func renderShellCommandPreview(command string, output runtime.CommandOutput) string {
	state := fmt.Sprintf("exit=%d", output.ExitCode)
	if output.TimedOut {
		state = "timed_out"
	}
	parts := []string{
		fmt.Sprintf("Command %q finished (%s", command, state),
	}
	if output.Duration > 0 {
		parts[0] += fmt.Sprintf(", %dms", output.Duration.Milliseconds())
	}
	parts[0] += ")"
	if output.Truncated {
		parts = append(parts, "output truncated")
	}
	return strings.Join(parts, "; ")
}

func renderShellCommandModelText(command, workdir string, timeoutSeconds, maxOutputBytes int, classification string, output runtime.CommandOutput) string {
	var lines []string
	lines = append(lines, "Shell command completed.")
	lines = append(lines, fmt.Sprintf("Command: %s", command))
	if strings.TrimSpace(workdir) != "" {
		lines = append(lines, fmt.Sprintf("Working directory: %s", workdir))
	}
	lines = append(lines, fmt.Sprintf("Classification: %s", classification))
	lines = append(lines, fmt.Sprintf("Exit code: %d", output.ExitCode))
	lines = append(lines, fmt.Sprintf("Timed out: %t", output.TimedOut))
	lines = append(lines, fmt.Sprintf("Truncated: %t", output.Truncated))
	lines = append(lines, fmt.Sprintf("Duration: %d ms", output.Duration.Milliseconds()))
	lines = append(lines, fmt.Sprintf("Timeout limit: %d s", timeoutSeconds))
	lines = append(lines, fmt.Sprintf("Output limit: %d bytes", maxOutputBytes))
	lines = append(lines, "stdout:")
	lines = append(lines, shellOutputBlock(output.Stdout))
	lines = append(lines, "stderr:")
	lines = append(lines, shellOutputBlock(output.Stderr))
	return strings.Join(lines, "\n")
}

func shellOutputBlock(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return "<empty>"
	}
	return text
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
