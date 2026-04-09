package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"go-agent/internal/permissions"
	"go-agent/internal/types"
)

type ToolRunStore interface {
	UpsertToolRun(context.Context, types.ToolRun) error
}

type Runtime struct {
	registry *Registry
	store    ToolRunStore
	locks    *resourceLockManager
}

type PreparedCall struct {
	Original       Call
	Call           Call
	ResolvedName   string
	Tool           Tool
	Definition     Definition
	Decoded        DecodedCall
	Parallel       bool
	ResourceClaims []ResourceClaim
	PrepareErr     error
}

type CallBatch struct {
	Parallel bool
	Calls    []PreparedCall
}

type CallExecution struct {
	Call        Call
	Result      Result
	Output      ToolExecutionResult
	ModelResult ModelToolResult
	Err         error
}

func NewRuntime(registry *Registry, store ToolRunStore) *Runtime {
	return &Runtime{
		registry: registry,
		store:    store,
		locks:    defaultResourceLockManager,
	}
}

func (r *Runtime) VisibleDefinitions(execCtx ExecContext) []Definition {
	if r == nil || r.registry == nil {
		return nil
	}
	defs := append([]Definition(nil), r.registry.VisibleDefinitions(execCtx)...)
	customTools, err := loadCustomTools(r.registry, execCtx)
	if err == nil {
		for _, tool := range customTools {
			if !toolEnabled(tool, execCtx) {
				continue
			}
			if execCtx.PermissionEngine != nil {
				switch execCtx.PermissionEngine.Decide(tool.spec.Name) {
				case permissions.DecisionAllow:
				case permissions.DecisionAsk, permissions.DecisionDeny:
					continue
				}
			}
			defs = append(defs, cloneDefinition(tool.Definition()))
		}
	}
	sort.Slice(defs, func(i, j int) bool {
		left := strings.ToLower(defs[i].Name)
		right := strings.ToLower(defs[j].Name)
		if left == right {
			return defs[i].Name < defs[j].Name
		}
		return left < right
	})
	return defs
}

func (r *Runtime) Execute(ctx context.Context, call Call, execCtx ExecContext) (Result, error) {
	output, err := r.ExecuteRich(ctx, call, execCtx)
	return output.Result, err
}

func (r *Runtime) ExecuteRich(ctx context.Context, call Call, execCtx ExecContext) (ToolExecutionResult, error) {
	return r.executePrepared(ctx, r.prepareCall(call, execCtx), execCtx)
}

func (r *Runtime) prepareCall(call Call, execCtx ExecContext) PreparedCall {
	if r == nil || r.registry == nil {
		return PreparedCall{
			Original:   call,
			Call:       call,
			PrepareErr: errors.New("tool registry is required"),
		}
	}
	prepared := r.prepareRegistryOrCustomCall(call, execCtx)
	prepared.ResourceClaims = resourceClaimsForPrepared(prepared, execCtx)
	prepared.Parallel = prepared.PrepareErr == nil &&
		toolEnabled(prepared.Tool, execCtx) &&
		toolConcurrencySafe(prepared.Tool, prepared.Decoded, execCtx)
	return prepared
}

func (r *Runtime) prepareRegistryOrCustomCall(call Call, execCtx ExecContext) PreparedCall {
	if tool, def, resolvedName, ok := r.registry.lookup(call.Name); ok {
		return r.registry.prepareResolvedCall(tool, def, resolvedName, call)
	}

	customTools, err := loadCustomTools(r.registry, execCtx)
	if err != nil {
		return PreparedCall{
			Original:   call,
			Call:       call,
			PrepareErr: err,
		}
	}
	customTool, ok := customTools[strings.ToLower(call.Name)]
	if !ok {
		return PreparedCall{
			Original:   call,
			Call:       call,
			PrepareErr: fmt.Errorf("unknown tool %q", call.Name),
		}
	}
	def := customTool.Definition()
	return r.registry.prepareResolvedCall(customTool, def, def.Name, call)
}

func (r *Runtime) executePrepared(ctx context.Context, prepared PreparedCall, execCtx ExecContext) (ToolExecutionResult, error) {
	var (
		runRecord *types.ToolRun
		lockStats ResourceLockStats
	)
	if strings.TrimSpace(prepared.ResolvedName) != "" {
		runRecord = r.startToolRun(ctx, prepared.ResolvedName, prepared.Call, execCtx)
		if runRecord != nil {
			now := time.Now().UTC()
			runRecord.CreatedAt = now
			runRecord.UpdatedAt = now
			runRecord.State = types.ToolRunStatePending
			if err := r.store.UpsertToolRun(ctx, *runRecord); err != nil {
				return ToolExecutionResult{}, err
			}
			emitTimelineBlockEvent(ctx, execCtx, types.EventToolRunUpdated, types.TimelineBlockFromToolRun(*runRecord))
		}
	}

	var releaseLocks func()
	if r != nil && r.locks != nil && len(prepared.ResourceClaims) > 0 {
		waitStarted := time.Now()
		release, err := r.locks.acquire(ctx, prepared.ResourceClaims)
		if err != nil {
			if runRecord != nil {
				runRecord.State = types.ToolRunStateFailed
				runRecord.Error = err.Error()
				runRecord.UpdatedAt = time.Now().UTC()
				runRecord.CompletedAt = runRecord.UpdatedAt
				_ = r.store.UpsertToolRun(ctx, *runRecord)
				emitTimelineBlockEvent(ctx, execCtx, types.EventToolRunUpdated, types.TimelineBlockFromToolRun(*runRecord))
			}
			return ToolExecutionResult{}, err
		}
		releaseLocks = release
		lockStats = ResourceLockStats{
			Waited:   time.Since(waitStarted),
			Acquired: normalizeResourceClaims(prepared.ResourceClaims),
		}
	}
	if releaseLocks != nil {
		defer releaseLocks()
	}
	startedAt := time.Now().UTC()
	if runRecord != nil {
		runRecord.StartedAt = startedAt
		runRecord.UpdatedAt = startedAt
		runRecord.State = types.ToolRunStateRunning
		runRecord.LockWaitMs = lockStats.Waited.Milliseconds()
		runRecord.ResourceKeys = claimKeys(lockStats.Acquired)
		if err := r.store.UpsertToolRun(ctx, *runRecord); err != nil {
			return ToolExecutionResult{}, err
		}
		emitTimelineBlockEvent(ctx, execCtx, types.EventToolRunUpdated, types.TimelineBlockFromToolRun(*runRecord))
	}

	output := ToolExecutionResult{}
	execErr := prepared.PrepareErr
	if execErr == nil {
		execToolCtx := execCtx
		if runRecord != nil {
			execToolCtx.ToolRunID = runRecord.ID
		}
		output, execErr = r.registry.executePreparedRich(ctx, prepared, execToolCtx)
	}
	if execErr == nil {
		output = normalizeToolResult(output, prepared.Definition, prepared.ResolvedName, execCtx)
	}
	if runRecord != nil {
		completedAt := time.Now().UTC()
		runRecord.UpdatedAt = completedAt
		if execErr != nil {
			runRecord.State = types.ToolRunStateFailed
			runRecord.Error = execErr.Error()
			runRecord.OutputJSON = ""
			runRecord.CompletedAt = completedAt
		} else {
			runRecord.Error = ""
			if output.Interrupt != nil && strings.TrimSpace(output.Interrupt.EventType) == types.EventPermissionRequested {
				runRecord.State = types.ToolRunStateWaitingPermission
				runRecord.CompletedAt = time.Time{}
				runRecord.OutputJSON = marshalToolRunOutput(output)
				runRecord.PermissionRequestID = firstMetadataString(output.Metadata, "permission_request_id")
			} else {
				runRecord.State = types.ToolRunStateCompleted
				runRecord.CompletedAt = completedAt
				runRecord.OutputJSON = marshalToolRunOutput(output)
			}
		}
		if err := r.store.UpsertToolRun(ctx, *runRecord); err != nil {
			if execErr != nil {
				execErr = errors.Join(execErr, err)
			} else {
				return ToolExecutionResult{}, err
			}
		} else {
			emitTimelineBlockEvent(ctx, execCtx, types.EventToolRunUpdated, types.TimelineBlockFromToolRun(*runRecord))
		}
	}

	return output, execErr
}

func (r *Runtime) PlanBatches(calls []Call, execCtx ExecContext) []CallBatch {
	if len(calls) == 0 {
		return nil
	}

	batches := make([]CallBatch, 0, len(calls))
	for _, call := range calls {
		prepared := r.prepareCall(call, execCtx)
		parallel := prepared.Parallel
		if parallel && len(batches) > 0 && batches[len(batches)-1].Parallel {
			batches[len(batches)-1].Calls = append(batches[len(batches)-1].Calls, prepared)
			continue
		}
		batches = append(batches, CallBatch{
			Parallel: parallel,
			Calls:    []PreparedCall{prepared},
		})
	}

	return batches
}

func (r *Runtime) ExecuteBatch(ctx context.Context, batch CallBatch, execCtx ExecContext) ([]CallExecution, error) {
	if r == nil || r.registry == nil {
		return nil, errors.New("tool registry is required")
	}
	if len(batch.Calls) == 0 {
		return nil, nil
	}

	out := make([]CallExecution, len(batch.Calls))
	runOne := func(runCtx context.Context, index int, prepared PreparedCall) {
		output, err := r.executePrepared(runCtx, prepared, execCtx)
		out[index] = CallExecution{
			Call:        prepared.Original,
			Result:      output.Result,
			Output:      output,
			ModelResult: mapToolModelResult(prepared.Tool, output),
			Err:         err,
		}
	}

	if !batch.Parallel || len(batch.Calls) == 1 {
		for index, prepared := range batch.Calls {
			runOne(ctx, index, prepared)
		}
		return out, nil
	}

	batchCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(len(batch.Calls))
	for index, prepared := range batch.Calls {
		go func(i int, current PreparedCall) {
			defer wg.Done()
			runOne(batchCtx, i, current)
			if out[i].Err != nil {
				cancel()
			}
		}(index, prepared)
	}
	wg.Wait()
	return out, nil
}

func (r *Runtime) ExecuteCalls(ctx context.Context, calls []Call, execCtx ExecContext) ([]CallExecution, error) {
	if r == nil || r.registry == nil {
		return nil, errors.New("tool registry is required")
	}

	results := make([]CallExecution, 0, len(calls))
	for _, batch := range r.PlanBatches(calls, execCtx) {
		executed, err := r.ExecuteBatch(ctx, batch, execCtx)
		if err != nil {
			return results, err
		}
		results = append(results, executed...)

		for _, item := range executed {
			if item.Err != nil {
				return results, nil
			}
		}
	}
	return results, nil
}

func (r *Runtime) startToolRun(_ context.Context, resolvedName string, call Call, execCtx ExecContext) *types.ToolRun {
	if r == nil || r.store == nil || execCtx.TurnContext == nil {
		return nil
	}
	runID := strings.TrimSpace(execCtx.TurnContext.CurrentRunID)
	if runID == "" {
		return nil
	}

	return &types.ToolRun{
		ID:         types.NewID("tool_run"),
		RunID:      runID,
		TaskID:     strings.TrimSpace(execCtx.TurnContext.CurrentTaskID),
		ToolName:   resolvedName,
		ToolCallID: strings.TrimSpace(call.ID),
		InputJSON:  marshalCallInput(call.Input),
	}
}

func normalizeToolResult(output ToolExecutionResult, def Definition, toolName string, execCtx ExecContext) ToolExecutionResult {
	if output.ModelText == "" {
		output.ModelText = output.Text
	}
	if output.PreviewText == "" {
		source := output.Text
		if source == "" {
			source = output.ModelText
		}
		output.PreviewText = PreviewText(source, 256)
	}
	if output.Result.ArtifactPath == "" && len(output.Artifacts) > 0 {
		output.Result.ArtifactPath = output.Artifacts[0].Path
	}
	if output.Result.ArtifactPath != "" {
		return output
	}

	inlineLimit := def.MaxInlineBytes
	if inlineLimit <= 0 {
		inlineLimit = InlineResultLimit
	}
	if len(output.ModelText) <= inlineLimit {
		return output
	}

	artifactPath, err := writeToolArtifact(execCtx.WorkspaceRoot, toolName, output.ModelText)
	if err != nil {
		return output
	}

	displayPath := artifactPath
	if execCtx.WorkspaceRoot != "" {
		if rel, err := filepath.Rel(execCtx.WorkspaceRoot, artifactPath); err == nil && rel != "" {
			displayPath = rel
		}
	}

	previewSource := output.Text
	if previewSource == "" {
		previewSource = output.ModelText
	}
	output.ArtifactPath = artifactPath
	output.Artifacts = append(output.Artifacts, ArtifactRef{
		Path: artifactPath,
		Kind: "text",
	})
	output.ModelText = fmt.Sprintf(
		"Tool %s produced a large result. Full output is available at %s.\nPreview:\n%s",
		toolName,
		displayPath,
		PreviewText(previewSource, 256),
	)
	output.PreviewText = PreviewText(previewSource, 256)
	return output
}

func writeToolArtifact(workspaceRoot, toolName, content string) (string, error) {
	if strings.TrimSpace(workspaceRoot) == "" {
		return "", fmt.Errorf("workspace root is required for tool artifact persistence")
	}

	dir := filepath.Join(workspaceRoot, ".runtime-data", "tool-results")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}

	filename := fmt.Sprintf("%s-%s.txt", toolName, types.NewID("artifact"))
	path := filepath.Join(dir, filename)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return "", err
	}
	return path, nil
}

func marshalCallInput(input map[string]any) string {
	if len(input) == 0 {
		return ""
	}
	raw, err := json.Marshal(input)
	if err != nil {
		return ""
	}
	return string(raw)
}

func marshalToolRunOutput(result ToolExecutionResult) string {
	payload := map[string]any{
		"text":          PreviewText(result.Text, 512),
		"model_text":    PreviewText(result.ModelText, 512),
		"artifact_path": result.ArtifactPath,
		"preview_text":  PreviewText(result.PreviewText, 512),
		"metadata":      result.Metadata,
		"artifacts":     result.Artifacts,
	}
	if structured, ok := marshalStructuredPreview(result.Data, 2048); ok {
		payload["structured"] = structured
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return ""
	}
	return string(raw)
}

func claimKeys(claims []ResourceClaim) []string {
	if len(claims) == 0 {
		return nil
	}
	out := make([]string, 0, len(claims))
	for _, claim := range claims {
		if strings.TrimSpace(claim.Key) != "" {
			out = append(out, claim.Key)
		}
	}
	return out
}

func firstMetadataString(metadata map[string]any, key string) string {
	if len(metadata) == 0 {
		return ""
	}
	value, _ := metadata[key].(string)
	return strings.TrimSpace(value)
}

func marshalStructuredPreview(value any, maxBytes int) (any, bool) {
	if value == nil {
		return nil, false
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, false
	}
	if maxBytes > 0 && len(raw) > maxBytes {
		return map[string]any{
			"truncated": true,
			"preview":   PreviewText(string(raw), maxBytes),
		}, true
	}
	var decoded any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return nil, false
	}
	return decoded, true
}
