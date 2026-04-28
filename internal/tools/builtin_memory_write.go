package tools

import (
	"context"
	"fmt"
	"strings"

	"go-agent/internal/memory"
	rolectx "go-agent/internal/roles"
	"go-agent/internal/types"
)

type memoryWriteTool struct{}

type MemoryWriteInput struct {
	Content    string `json:"content"`
	Kind       string `json:"kind,omitempty"`
	Scope      string `json:"scope,omitempty"`
	Visibility string `json:"visibility,omitempty"`
}

type MemoryWriteOutput struct {
	ID      string `json:"id"`
	Kind    string `json:"kind"`
	Scope   string `json:"scope"`
	Summary string `json:"summary"`
}

func (memoryWriteTool) IsEnabled(execCtx ExecContext) bool {
	return execCtx.MemoryStore != nil && strings.TrimSpace(execCtx.WorkspaceRoot) != ""
}

func (memoryWriteTool) Definition() Definition {
	return Definition{
		Name:        "memory_write",
		Description: "Record an important finding, decision, pattern, or preference to durable shared memory for later recall.",
		InputSchema: objectSchema(map[string]any{
			"content": map[string]any{
				"type":        "string",
				"description": "Memory content to store.",
			},
			"kind": map[string]any{
				"type":        "string",
				"description": "Optional memory kind: fact, decision, preference, pattern, workspace_overview, or global_preference. Defaults to fact.",
				"enum":        []string{"fact", "decision", "preference", "pattern", "workspace_overview", "global_preference"},
			},
			"scope": map[string]any{
				"type":        "string",
				"description": "Optional memory scope: workspace or global. Defaults to workspace.",
				"enum":        []string{"workspace", "global"},
			},
			"visibility": map[string]any{
				"type":        "string",
				"description": "Optional visibility: shared, private, or promoted. Defaults to shared.",
				"enum":        []string{"shared", "private", "promoted"},
			},
		}, "content"),
		OutputSchema: objectSchema(map[string]any{
			"id":      map[string]any{"type": "string"},
			"kind":    map[string]any{"type": "string"},
			"scope":   map[string]any{"type": "string"},
			"summary": map[string]any{"type": "string"},
		}, "id", "kind", "scope", "summary"),
	}
}

func (memoryWriteTool) IsConcurrencySafe() bool { return true }

func (memoryWriteTool) Decode(call Call) (DecodedCall, error) {
	content := strings.TrimSpace(call.StringInput("content"))
	if content == "" {
		return DecodedCall{}, fmt.Errorf("content is required")
	}
	kind, err := decodeMemoryWriteKind(call.StringInput("kind"))
	if err != nil {
		return DecodedCall{}, err
	}
	scope, err := decodeMemoryWriteScope(call.StringInput("scope"))
	if err != nil {
		return DecodedCall{}, err
	}
	visibility, err := decodeMemoryWriteVisibility(call.StringInput("visibility"))
	if err != nil {
		return DecodedCall{}, err
	}

	normalized := Call{Name: call.Name, Input: map[string]any{
		"content":    content,
		"kind":       string(kind),
		"scope":      string(scope),
		"visibility": string(visibility),
	}}
	return DecodedCall{Call: normalized, Input: MemoryWriteInput{
		Content:    content,
		Kind:       string(kind),
		Scope:      string(scope),
		Visibility: string(visibility),
	}}, nil
}

func (t memoryWriteTool) Execute(ctx context.Context, call Call, execCtx ExecContext) (Result, error) {
	decoded, err := t.Decode(call)
	if err != nil {
		return Result{}, err
	}
	output, err := t.ExecuteDecoded(ctx, decoded, execCtx)
	return output.Result, err
}

func (memoryWriteTool) ExecuteDecoded(ctx context.Context, decoded DecodedCall, execCtx ExecContext) (ToolExecutionResult, error) {
	if execCtx.MemoryStore == nil {
		return ToolExecutionResult{}, fmt.Errorf("memory store is not configured")
	}
	workspaceRoot := strings.TrimSpace(execCtx.WorkspaceRoot)
	if workspaceRoot == "" {
		return ToolExecutionResult{}, fmt.Errorf("workspace root is required")
	}
	input, _ := decoded.Input.(MemoryWriteInput)
	kind, err := decodeMemoryWriteKind(input.Kind)
	if err != nil {
		return ToolExecutionResult{}, err
	}
	scope, err := decodeMemoryWriteScope(input.Scope)
	if err != nil {
		return ToolExecutionResult{}, err
	}
	visibility, err := decodeMemoryWriteVisibility(input.Visibility)
	if err != nil {
		return ToolExecutionResult{}, err
	}
	content := strings.TrimSpace(input.Content)
	if content == "" {
		return ToolExecutionResult{}, fmt.Errorf("content is required")
	}

	now := timeNowUTC()
	entry := types.MemoryEntry{
		ID:          types.NewID("mem"),
		Scope:       scope,
		WorkspaceID: workspaceRoot,
		Kind:        kind,
		OwnerRoleID: strings.TrimSpace(rolectx.SpecialistRoleIDFromContext(ctx)),
		Visibility:  visibility,
		Status:      types.MemoryStatusActive,
		Content:     content,
		CreatedAt:   now,
		UpdatedAt:   now,
		LastUsedAt:  now,
	}
	entry.Confidence = memory.ComputeConfidence(entry, now)
	if err := execCtx.MemoryStore.UpsertMemoryEntry(ctx, entry); err != nil {
		return ToolExecutionResult{}, err
	}

	output := MemoryWriteOutput{
		ID:      entry.ID,
		Kind:    string(entry.Kind),
		Scope:   string(entry.Scope),
		Summary: summarizeMemoryWriteContent(content),
	}
	text := mustJSON(output)
	return ToolExecutionResult{
		Result:      Result{Text: text, ModelText: text},
		Data:        output,
		PreviewText: "Recorded memory " + entry.ID,
	}, nil
}

func (memoryWriteTool) MapModelResult(output ToolExecutionResult) ModelToolResult {
	return defaultStructuredModelResult(output)
}

func decodeMemoryWriteKind(raw string) (types.MemoryKind, error) {
	switch kind := types.MemoryKind(strings.TrimSpace(raw)); kind {
	case "":
		return types.MemoryKindFact, nil
	case types.MemoryKindFact,
		types.MemoryKindDecision,
		types.MemoryKindPreference,
		types.MemoryKindPattern,
		types.MemoryKindWorkspaceOverview,
		types.MemoryKindGlobalPreference:
		return kind, nil
	default:
		return "", fmt.Errorf("kind must be one of fact, decision, preference, pattern, workspace_overview, or global_preference")
	}
}

func decodeMemoryWriteScope(raw string) (types.MemoryScope, error) {
	switch scope := types.MemoryScope(strings.TrimSpace(raw)); scope {
	case "":
		return types.MemoryScopeWorkspace, nil
	case types.MemoryScopeWorkspace, types.MemoryScopeGlobal:
		return scope, nil
	default:
		return "", fmt.Errorf("scope must be workspace or global")
	}
}

func decodeMemoryWriteVisibility(raw string) (types.MemoryVisibility, error) {
	switch visibility := types.MemoryVisibility(strings.TrimSpace(raw)); visibility {
	case "":
		return types.MemoryVisibilityShared, nil
	case types.MemoryVisibilityShared, types.MemoryVisibilityPrivate, types.MemoryVisibilityPromoted:
		return visibility, nil
	default:
		return "", fmt.Errorf("visibility must be shared, private, or promoted")
	}
}

func summarizeMemoryWriteContent(content string) string {
	content = strings.Join(strings.Fields(content), " ")
	const maxRunes = 200
	runes := []rune(content)
	if len(runes) <= maxRunes {
		return content
	}
	return string(runes[:maxRunes])
}
