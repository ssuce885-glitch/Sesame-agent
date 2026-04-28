package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"go-agent/internal/types"
)

type loadContextTool struct{}

type LoadContextInput struct {
	ContextRef types.ColdContextRef `json:"context_ref"`
}

type LoadContextOutput struct {
	ContextRef types.ColdContextRef             `json:"context_ref"`
	Items      []types.ConversationTimelineItem `json:"items"`
	Count      int                              `json:"count"`
}

func (loadContextTool) IsEnabled(execCtx ExecContext) bool {
	return execCtx.ColdIndexStore != nil && strings.TrimSpace(execCtx.WorkspaceRoot) != ""
}

func (loadContextTool) Definition() Definition {
	return Definition{
		Name:        "load_context",
		Description: "Load full conversation items for a cold archive search result using its context_ref.",
		InputSchema: objectSchema(map[string]any{
			"context_ref": map[string]any{
				"type":        "object",
				"description": "Context reference object from recall_archive result. Copy the entire context_ref object verbatim. Do NOT use the top-level id field as session_id.",
				"properties": map[string]any{
					"session_id":      map[string]any{"type": "string"},
					"context_head_id": map[string]any{"type": "string"},
					"turn_start_pos":  map[string]any{"type": "integer"},
					"turn_end_pos":    map[string]any{"type": "integer"},
					"item_count":      map[string]any{"type": "integer"},
				},
				"additionalProperties": false,
			},
			"session_id": map[string]any{
				"type":        "string",
				"description": "Copy session_id from context_ref verbatim. Do NOT use the recall_archive entry's id field.",
			},
			"context_head_id": map[string]any{
				"type":        "string",
				"description": "Optional context head ID from context_ref.",
			},
			"turn_start_pos": map[string]any{
				"type":        "integer",
				"description": "Inclusive zero-based start position from context_ref.",
			},
			"turn_end_pos": map[string]any{
				"type":        "integer",
				"description": "Exclusive zero-based end position from context_ref.",
			},
			"item_count": map[string]any{
				"type":        "integer",
				"description": "Optional item count from context_ref.",
			},
		}),
		OutputSchema: objectSchema(map[string]any{
			"context_ref": map[string]any{
				"type":                 "object",
				"additionalProperties": true,
			},
			"items": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type":                 "object",
					"additionalProperties": true,
				},
			},
			"count": map[string]any{"type": "integer"},
		}, "context_ref", "items", "count"),
	}
}

func (loadContextTool) IsConcurrencySafe() bool { return true }

func (loadContextTool) Decode(call Call) (DecodedCall, error) {
	ref, err := decodeLoadContextRef(call.Input)
	if err != nil {
		return DecodedCall{}, err
	}
	normalized := Call{Name: call.Name, Input: map[string]any{
		"context_ref": ref,
	}}
	return DecodedCall{Call: normalized, Input: LoadContextInput{ContextRef: ref}}, nil
}

func (t loadContextTool) Execute(ctx context.Context, call Call, execCtx ExecContext) (Result, error) {
	decoded, err := t.Decode(call)
	if err != nil {
		return Result{}, err
	}
	output, err := t.ExecuteDecoded(ctx, decoded, execCtx)
	return output.Result, err
}

func (loadContextTool) ExecuteDecoded(ctx context.Context, decoded DecodedCall, execCtx ExecContext) (ToolExecutionResult, error) {
	if execCtx.ColdIndexStore == nil {
		return ToolExecutionResult{}, fmt.Errorf("cold index store is not configured")
	}
	if strings.TrimSpace(execCtx.WorkspaceRoot) == "" {
		return ToolExecutionResult{}, fmt.Errorf("workspace root is required")
	}
	input, _ := decoded.Input.(LoadContextInput)
	ref := input.ContextRef
	session, ok, err := execCtx.ColdIndexStore.GetSession(ctx, ref.SessionID)
	if err != nil {
		return ToolExecutionResult{}, err
	}
	if !ok {
		return ToolExecutionResult{}, fmt.Errorf("session %q not found", ref.SessionID)
	}
	if strings.TrimSpace(session.WorkspaceRoot) != strings.TrimSpace(execCtx.WorkspaceRoot) {
		return ToolExecutionResult{}, fmt.Errorf("context_ref session is outside the current workspace")
	}
	if ref.TurnStartPos < 0 {
		return ToolExecutionResult{}, fmt.Errorf("turn_start_pos must be non-negative")
	}
	if ref.ItemCount == 0 {
		return emptyLoadContextResult(ref), nil
	}

	var timelineItems []types.ConversationTimelineItem
	if strings.TrimSpace(ref.ContextHeadID) != "" {
		timelineItems, err = execCtx.ColdIndexStore.ListConversationTimelineItemsByContextHead(ctx, ref.SessionID, ref.ContextHeadID)
	} else {
		timelineItems, err = execCtx.ColdIndexStore.ListConversationTimelineItems(ctx, ref.SessionID)
	}
	if err != nil {
		return ToolExecutionResult{}, err
	}
	if len(timelineItems) == 0 {
		return emptyLoadContextResult(ref), nil
	}
	if ref.TurnEndPos <= ref.TurnStartPos {
		return ToolExecutionResult{}, fmt.Errorf("turn_end_pos must be greater than turn_start_pos")
	}
	if ref.TurnEndPos > len(timelineItems) {
		return ToolExecutionResult{}, fmt.Errorf("context range %d:%d exceeds %d conversation items", ref.TurnStartPos, ref.TurnEndPos, len(timelineItems))
	}
	items := append([]types.ConversationTimelineItem(nil), timelineItems[ref.TurnStartPos:ref.TurnEndPos]...)
	output := LoadContextOutput{
		ContextRef: ref,
		Items:      items,
		Count:      len(items),
	}
	text := mustJSON(output)
	return ToolExecutionResult{
		Result:      Result{Text: text, ModelText: text},
		Data:        output,
		PreviewText: fmt.Sprintf("Loaded %d context items", len(items)),
	}, nil
}

func emptyLoadContextResult(ref types.ColdContextRef) ToolExecutionResult {
	output := LoadContextOutput{
		ContextRef: ref,
		Items:      nil,
		Count:      0,
	}
	text := mustJSON(output)
	return ToolExecutionResult{
		Result:      Result{Text: text, ModelText: text},
		Data:        output,
		PreviewText: "Loaded 0 context items",
	}
}

func (loadContextTool) MapModelResult(output ToolExecutionResult) ModelToolResult {
	return defaultStructuredModelResult(output)
}

func decodeLoadContextRef(input map[string]any) (types.ColdContextRef, error) {
	var ref types.ColdContextRef
	itemCountProvided := false
	if raw, ok := input["context_ref"]; ok && raw != nil {
		itemCountProvided = inputObjectHasKey(raw, "item_count")
		data, err := json.Marshal(raw)
		if err != nil {
			return types.ColdContextRef{}, err
		}
		if err := json.Unmarshal(data, &ref); err != nil {
			return types.ColdContextRef{}, fmt.Errorf("context_ref must match the cold search context_ref shape: %w", err)
		}
	}
	if strings.TrimSpace(ref.SessionID) == "" {
		ref.SessionID = strings.TrimSpace(stringInputFromMap(input, "session_id"))
	}
	if strings.TrimSpace(ref.ContextHeadID) == "" {
		ref.ContextHeadID = strings.TrimSpace(stringInputFromMap(input, "context_head_id"))
	}
	if ref.TurnStartPos == 0 {
		if value, ok, err := optionalIntFromMap(input, "turn_start_pos"); err != nil {
			return types.ColdContextRef{}, fmt.Errorf("turn_start_pos %w", err)
		} else if ok {
			ref.TurnStartPos = value
		}
	}
	if ref.TurnEndPos == 0 {
		if value, ok, err := optionalIntFromMap(input, "turn_end_pos"); err != nil {
			return types.ColdContextRef{}, fmt.Errorf("turn_end_pos %w", err)
		} else if ok {
			ref.TurnEndPos = value
		}
	}
	if value, ok, err := optionalIntFromMap(input, "item_count"); err != nil {
		return types.ColdContextRef{}, fmt.Errorf("item_count %w", err)
	} else if ok {
		ref.ItemCount = value
		itemCountProvided = true
	}
	ref.SessionID = strings.TrimSpace(ref.SessionID)
	ref.ContextHeadID = strings.TrimSpace(ref.ContextHeadID)
	if ref.SessionID == "" {
		return types.ColdContextRef{}, fmt.Errorf("session_id is required")
	}
	if itemCountProvided && ref.ItemCount == 0 && ref.TurnStartPos == 0 && ref.TurnEndPos == 0 {
		ref.TurnEndPos = 1
	}
	if !itemCountProvided && ref.TurnEndPos > ref.TurnStartPos {
		ref.ItemCount = ref.TurnEndPos - ref.TurnStartPos
	}
	if ref.TurnEndPos <= ref.TurnStartPos {
		return types.ColdContextRef{}, fmt.Errorf("turn_start_pos and turn_end_pos are required")
	}
	return ref, nil
}

func inputObjectHasKey(raw any, key string) bool {
	if object, ok := raw.(map[string]any); ok {
		_, exists := object[key]
		return exists
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return false
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(data, &object); err != nil {
		return false
	}
	_, exists := object[key]
	return exists
}

func stringInputFromMap(input map[string]any, key string) string {
	value, _ := input[key].(string)
	return value
}

func optionalIntFromMap(input map[string]any, key string) (int, bool, error) {
	raw, ok := input[key]
	if !ok || raw == nil {
		return 0, false, nil
	}
	switch value := raw.(type) {
	case int:
		return value, true, nil
	case int32:
		return int(value), true, nil
	case int64:
		return int(value), true, nil
	case float64:
		if value != float64(int(value)) {
			return 0, false, fmt.Errorf("must be an integer")
		}
		return int(value), true, nil
	default:
		return 0, false, fmt.Errorf("must be an integer")
	}
}
