package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"go-agent/internal/extensions"
)

type customTool struct {
	spec extensions.ToolSpec
}

type customToolEnvelope struct {
	Text         string          `json:"text"`
	ModelText    string          `json:"model_text"`
	PreviewText  string          `json:"preview_text"`
	ArtifactPath string          `json:"artifact_path"`
	Data         json.RawMessage `json:"data"`
	Metadata     map[string]any  `json:"metadata"`
}

type customToolProcessOutput struct {
	Command          string
	Args             []string
	Workdir          string
	Stdout           string
	Stderr           string
	AggregatedOutput string
	ExitCode         int
	TimedOut         bool
	Duration         time.Duration
	Truncated        bool
}

type customToolCappedBuffer struct {
	buf       bytes.Buffer
	maxBytes  int
	truncated bool
}

type customToolTeeCapture struct {
	aggregate *customToolCappedBuffer
	stream    *customToolCappedBuffer
}

func (w *customToolCappedBuffer) Write(p []byte) (int, error) {
	if w == nil {
		return len(p), nil
	}
	if w.maxBytes <= 0 {
		w.truncated = w.truncated || len(p) > 0
		return len(p), nil
	}
	remaining := w.maxBytes - w.buf.Len()
	if remaining <= 0 {
		w.truncated = w.truncated || len(p) > 0
		return len(p), nil
	}
	if len(p) > remaining {
		_, _ = w.buf.Write(p[:remaining])
		w.truncated = true
		return len(p), nil
	}
	_, _ = w.buf.Write(p)
	return len(p), nil
}

func (w customToolTeeCapture) Write(p []byte) (int, error) {
	if w.aggregate != nil {
		_, _ = w.aggregate.Write(p)
	}
	if w.stream != nil {
		_, _ = w.stream.Write(p)
	}
	return len(p), nil
}

func loadCustomTools(registry *Registry, execCtx ExecContext) (map[string]customTool, error) {
	specs, err := extensions.DiscoverToolSpecs(execCtx.GlobalConfigRoot, execCtx.WorkspaceRoot)
	if err != nil {
		return nil, err
	}
	out := make(map[string]customTool, len(specs))
	for _, spec := range specs {
		if registry != nil {
			if _, _, _, ok := registry.lookup(spec.Name); ok {
				continue
			}
		}
		out[strings.ToLower(spec.Name)] = customTool{spec: spec}
	}
	return out, nil
}

func (t customTool) Definition() Definition {
	return Definition{
		Name:           t.spec.Name,
		Description:    t.spec.Description,
		InputSchema:    cloneStringAnyMap(t.spec.InputSchema),
		OutputSchema:   cloneStringAnyMap(t.spec.OutputSchema),
		MaxInlineBytes: InlineResultLimit,
	}
}

func (t customTool) IsConcurrencySafe() bool {
	return t.spec.ConcurrencySafe
}

func (t customTool) Execute(ctx context.Context, call Call, execCtx ExecContext) (Result, error) {
	output, err := t.ExecuteDecoded(ctx, DecodedCall{
		Call:  call,
		Input: call.Input,
	}, execCtx)
	return output.Result, err
}

func (t customTool) ExecuteDecoded(ctx context.Context, decoded DecodedCall, execCtx ExecContext) (ToolExecutionResult, error) {
	input := decoded.Call.Input
	if input == nil {
		input = map[string]any{}
	}
	payload, err := json.Marshal(input)
	if err != nil {
		return ToolExecutionResult{}, fmt.Errorf("marshal custom tool input: %w", err)
	}

	command, args := t.resolveCommand()
	workdir := t.resolveWorkdir(execCtx)
	runCtx, cancel := context.WithTimeout(ctx, time.Duration(t.spec.TimeoutSeconds)*time.Second)
	defer cancel()
	output, err := runCustomToolProcess(
		runCtx,
		command,
		args,
		workdir,
		payload,
		t.customToolEnv(execCtx),
		t.spec.MaxOutputBytes,
	)
	if err != nil {
		return ToolExecutionResult{}, err
	}
	if output.TimedOut {
		return ToolExecutionResult{}, fmt.Errorf("custom tool %q timed out after %d seconds", t.spec.Name, t.spec.TimeoutSeconds)
	}
	if output.ExitCode != 0 {
		return ToolExecutionResult{}, formatCustomToolExitError(t.spec.Name, output)
	}

	return t.parseOutput(output)
}

func (t customTool) resolveCommand() (string, []string) {
	command := strings.TrimSpace(t.spec.Command)
	if command == "" {
		return "", append([]string(nil), t.spec.Args...)
	}
	if filepath.IsAbs(command) {
		return command, append([]string(nil), t.spec.Args...)
	}
	if strings.HasPrefix(command, ".") || strings.Contains(command, "/") || strings.Contains(command, `\`) {
		return filepath.Clean(filepath.Join(t.spec.RootDir, command)), append([]string(nil), t.spec.Args...)
	}
	return command, append([]string(nil), t.spec.Args...)
}

func (t customTool) resolveWorkdir(execCtx ExecContext) string {
	if strings.TrimSpace(t.spec.Workdir) == "" {
		if strings.TrimSpace(execCtx.WorkspaceRoot) != "" {
			return execCtx.WorkspaceRoot
		}
		return t.spec.RootDir
	}
	if filepath.IsAbs(t.spec.Workdir) {
		return filepath.Clean(t.spec.Workdir)
	}
	base := t.spec.RootDir
	if strings.TrimSpace(execCtx.WorkspaceRoot) != "" {
		base = execCtx.WorkspaceRoot
	}
	return filepath.Clean(filepath.Join(base, t.spec.Workdir))
}

func (t customTool) customToolEnv(execCtx ExecContext) []string {
	env := []string{
		"SESAME_TOOL_NAME=" + t.spec.Name,
		"SESAME_TOOL_SCOPE=" + t.spec.Scope,
		"SESAME_TOOL_DIR=" + t.spec.RootDir,
		"SESAME_TOOL_MANIFEST=" + t.spec.ManifestPath,
	}
	if strings.TrimSpace(execCtx.WorkspaceRoot) != "" {
		env = append(env, "SESAME_WORKSPACE_ROOT="+execCtx.WorkspaceRoot)
	}
	if strings.TrimSpace(execCtx.ToolRunID) != "" {
		env = append(env, "SESAME_TOOL_RUN_ID="+execCtx.ToolRunID)
	}
	if execCtx.TurnContext != nil {
		if strings.TrimSpace(execCtx.TurnContext.CurrentTurnID) != "" {
			env = append(env, "SESAME_TURN_ID="+execCtx.TurnContext.CurrentTurnID)
		}
		if strings.TrimSpace(execCtx.TurnContext.CurrentSessionID) != "" {
			env = append(env, "SESAME_SESSION_ID="+execCtx.TurnContext.CurrentSessionID)
		}
	}
	return env
}

func (t customTool) parseOutput(output customToolProcessOutput) (ToolExecutionResult, error) {
	metadata := map[string]any{
		"command":       output.Command,
		"args":          append([]string(nil), output.Args...),
		"workdir":       output.Workdir,
		"scope":         t.spec.Scope,
		"tool_path":     t.spec.Path,
		"manifest_path": t.spec.ManifestPath,
		"exit_code":     output.ExitCode,
		"timed_out":     output.TimedOut,
		"duration_ms":   output.Duration.Milliseconds(),
		"truncated":     output.Truncated,
	}
	if trimmed := strings.TrimSpace(output.Stderr); trimmed != "" {
		metadata["stderr"] = trimmed
	}

	stdout := strings.TrimSpace(output.Stdout)
	if stdout == "" {
		return ToolExecutionResult{
			Result:   Result{},
			Metadata: metadata,
		}, nil
	}

	var rawObject map[string]json.RawMessage
	if err := json.Unmarshal([]byte(stdout), &rawObject); err == nil && looksLikeCustomToolEnvelope(rawObject) {
		return t.parseEnvelope(stdout, metadata)
	}

	var structured any
	if err := json.Unmarshal([]byte(stdout), &structured); err == nil {
		if t.spec.OutputSchema != nil {
			if err := validateSchemaValue("output", structured, t.spec.OutputSchema); err != nil {
				return ToolExecutionResult{}, fmt.Errorf("custom tool %q output %w", t.spec.Name, err)
			}
		}
		return ToolExecutionResult{
			Result: Result{
				Text:      stdout,
				ModelText: stdout,
			},
			Data:     structured,
			Metadata: metadata,
		}, nil
	}

	if t.spec.OutputSchema != nil {
		return ToolExecutionResult{}, fmt.Errorf("custom tool %q output must be valid JSON matching output_schema", t.spec.Name)
	}

	return ToolExecutionResult{
		Result: Result{
			Text:      stdout,
			ModelText: stdout,
		},
		Metadata: metadata,
	}, nil
}

func (t customTool) parseEnvelope(stdout string, runtimeMetadata map[string]any) (ToolExecutionResult, error) {
	var envelope customToolEnvelope
	if err := json.Unmarshal([]byte(stdout), &envelope); err != nil {
		return ToolExecutionResult{}, fmt.Errorf("custom tool %q output envelope %w", t.spec.Name, err)
	}

	var structured any
	if len(envelope.Data) > 0 && string(envelope.Data) != "null" {
		if err := json.Unmarshal(envelope.Data, &structured); err != nil {
			return ToolExecutionResult{}, fmt.Errorf("custom tool %q data field %w", t.spec.Name, err)
		}
	}
	if t.spec.OutputSchema != nil {
		if len(envelope.Data) == 0 || string(envelope.Data) == "null" {
			return ToolExecutionResult{}, fmt.Errorf("custom tool %q output is missing data for declared output_schema", t.spec.Name)
		}
		if err := validateSchemaValue("output", structured, t.spec.OutputSchema); err != nil {
			return ToolExecutionResult{}, fmt.Errorf("custom tool %q output %w", t.spec.Name, err)
		}
	}

	text := strings.TrimSpace(envelope.Text)
	modelText := strings.TrimSpace(envelope.ModelText)
	switch {
	case text == "" && modelText == "":
		text = stdout
		modelText = stdout
	case text == "":
		text = modelText
	case modelText == "":
		modelText = text
	}

	artifactPath := strings.TrimSpace(envelope.ArtifactPath)
	if artifactPath != "" && !filepath.IsAbs(artifactPath) {
		workdir, _ := runtimeMetadata["workdir"].(string)
		if strings.TrimSpace(workdir) != "" {
			artifactPath = filepath.Clean(filepath.Join(workdir, artifactPath))
		}
	}

	result := ToolExecutionResult{
		Result: Result{
			Text:         text,
			ModelText:    modelText,
			ArtifactPath: artifactPath,
		},
		Data:        structured,
		PreviewText: strings.TrimSpace(envelope.PreviewText),
		Metadata:    mergeCustomToolMetadata(envelope.Metadata, runtimeMetadata),
	}
	if artifactPath != "" {
		result.Artifacts = append(result.Artifacts, ArtifactRef{
			Path: artifactPath,
			Kind: "file",
		})
	}
	return result, nil
}

func looksLikeCustomToolEnvelope(raw map[string]json.RawMessage) bool {
	for _, key := range []string{"text", "model_text", "preview_text", "artifact_path", "data", "metadata"} {
		if _, ok := raw[key]; ok {
			return true
		}
	}
	return false
}

func mergeCustomToolMetadata(toolMetadata, runtimeMetadata map[string]any) map[string]any {
	if len(toolMetadata) == 0 && len(runtimeMetadata) == 0 {
		return nil
	}
	merged := make(map[string]any, len(toolMetadata)+len(runtimeMetadata))
	for key, value := range toolMetadata {
		merged[key] = value
	}
	for key, value := range runtimeMetadata {
		merged[key] = value
	}
	return merged
}

func runCustomToolProcess(
	ctx context.Context,
	command string,
	args []string,
	workdir string,
	stdin []byte,
	extraEnv []string,
	maxOutputBytes int,
) (customToolProcessOutput, error) {
	timeoutCtx := ctx
	cmd := exec.CommandContext(timeoutCtx, command, args...)
	if strings.TrimSpace(workdir) != "" {
		cmd.Dir = workdir
	}
	cmd.Env = append(os.Environ(), extraEnv...)
	cmd.Stdin = bytes.NewReader(stdin)

	stdout := &customToolCappedBuffer{maxBytes: maxOutputBytes}
	stderr := &customToolCappedBuffer{maxBytes: maxOutputBytes}
	aggregate := &customToolCappedBuffer{maxBytes: maxOutputBytes}
	cmd.Stdout = customToolTeeCapture{aggregate: aggregate, stream: stdout}
	cmd.Stderr = customToolTeeCapture{aggregate: aggregate, stream: stderr}

	startedAt := time.Now()
	runErr := cmd.Run()
	result := customToolProcessOutput{
		Command:          command,
		Args:             append([]string(nil), args...),
		Workdir:          workdir,
		Stdout:           stdout.buf.String(),
		Stderr:           stderr.buf.String(),
		AggregatedOutput: aggregate.buf.String(),
		Duration:         time.Since(startedAt),
		Truncated:        stdout.truncated || stderr.truncated || aggregate.truncated,
	}

	switch {
	case runErr == nil:
		result.ExitCode = 0
		return result, nil
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		result.ExitCode = -1
		result.TimedOut = true
		return result, nil
	case errors.Is(ctx.Err(), context.Canceled):
		return result, ctx.Err()
	}

	var exitErr *exec.ExitError
	if errors.As(runErr, &exitErr) {
		result.ExitCode = exitErr.ExitCode()
		return result, nil
	}
	return result, runErr
}

func formatCustomToolExitError(name string, output customToolProcessOutput) error {
	message := fmt.Sprintf("custom tool %q failed with exit code %d", name, output.ExitCode)
	if detail := strings.TrimSpace(output.Stderr); detail != "" {
		return fmt.Errorf("%s: %s", message, PreviewText(detail, 256))
	}
	if detail := strings.TrimSpace(output.AggregatedOutput); detail != "" {
		return fmt.Errorf("%s: %s", message, PreviewText(detail, 256))
	}
	return fmt.Errorf("%s", message)
}
