package engine

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"go-agent/internal/model"
	"go-agent/internal/types"
)

func saveTurnCheckpoint(
	ctx context.Context,
	e *Engine,
	in Input,
	state *preparedLoopState,
	checkpointState string,
	toolCalls []model.ToolCallChunk,
	nextPosition int,
	completedToolIDs []string,
	toolResults []model.ToolResult,
	assistantItems []model.ConversationItem,
) error {
	if e == nil || e.store == nil {
		return nil
	}
	toolCallIDs, toolCallNames := checkpointToolCalls(toolCalls)
	checkpoint := types.TurnCheckpoint{
		ID:            types.NewID("chkpt"),
		TurnID:        in.Turn.ID,
		SessionID:     state.sessionID,
		Sequence:      state.nextCheckpointSeq,
		State:         checkpointState,
		ToolCallIDs:   toolCallIDs,
		ToolCallNames: toolCallNames,
		NextPosition:  nextPosition,
		CreatedAt:     time.Now().UTC(),
	}
	if checkpointState == types.TurnCheckpointStatePostToolBatch {
		checkpoint.CompletedToolIDs = completedToolIDs
		checkpoint.ToolResultsJSON = marshalToolResults(toolResults)
		checkpoint.AssistantItemsJSON = marshalAssistantItems(assistantItems)
	}
	return e.store.InsertTurnCheckpoint(ctx, checkpoint)
}

func loadResumeCheckpoint(ctx context.Context, store ConversationStore, turnID string) (*types.TurnCheckpoint, error) {
	if store == nil || strings.TrimSpace(turnID) == "" {
		return nil, nil
	}
	checkpoint, ok, err := store.GetLatestTurnCheckpoint(ctx, turnID)
	if err != nil {
		return nil, err
	}
	if !ok || checkpoint.State != types.TurnCheckpointStatePostToolBatch {
		return nil, nil
	}
	return &checkpoint, nil
}

func requestUserItemForCheckpoint(in Input, checkpoint *types.TurnCheckpoint) model.ConversationItem {
	if checkpoint != nil {
		return model.ConversationItem{}
	}
	return turnEntryUserItem(in)
}

func nextCheckpointSequence(checkpoint *types.TurnCheckpoint) int {
	if checkpoint == nil {
		return 0
	}
	return checkpoint.Sequence + 1
}

func applyCheckpointToRequest(req *model.Request, checkpoint *types.TurnCheckpoint) error {
	if req == nil || checkpoint == nil {
		return nil
	}
	assistantItems, err := unmarshalAssistantItems(checkpoint.AssistantItemsJSON)
	if err != nil {
		return err
	}
	toolResults, err := unmarshalToolResults(checkpoint.ToolResultsJSON)
	if err != nil {
		return err
	}

	existingToolCalls := make(map[string]struct{})
	existingToolResults := make(map[string]struct{})
	for _, item := range req.Items {
		switch item.Kind {
		case model.ConversationItemToolCall:
			if id := strings.TrimSpace(item.ToolCall.ID); id != "" {
				existingToolCalls[id] = struct{}{}
			}
		case model.ConversationItemToolResult:
			if item.Result != nil {
				if id := strings.TrimSpace(item.Result.ToolCallID); id != "" {
					existingToolResults[id] = struct{}{}
				}
			}
		}
	}

	missingAssistantToolCall := false
	for _, item := range assistantItems {
		if item.Kind != model.ConversationItemToolCall {
			continue
		}
		id := strings.TrimSpace(item.ToolCall.ID)
		if id == "" {
			continue
		}
		if _, ok := existingToolCalls[id]; !ok {
			missingAssistantToolCall = true
			break
		}
	}
	if missingAssistantToolCall {
		for _, item := range assistantItems {
			if item.Kind == model.ConversationItemToolCall {
				id := strings.TrimSpace(item.ToolCall.ID)
				if id != "" {
					if _, ok := existingToolCalls[id]; ok {
						continue
					}
					existingToolCalls[id] = struct{}{}
				}
			}
			req.Items = append(req.Items, item)
		}
	}

	for _, result := range toolResults {
		id := strings.TrimSpace(result.ToolCallID)
		if id == "" {
			continue
		}
		if _, ok := existingToolResults[id]; !ok {
			req.Items = append(req.Items, model.ToolResultItem(result))
			existingToolResults[id] = struct{}{}
		}
	}

	existingRequestResults := make(map[string]struct{}, len(req.ToolResults))
	for _, result := range req.ToolResults {
		if id := strings.TrimSpace(result.ToolCallID); id != "" {
			existingRequestResults[id] = struct{}{}
		}
	}
	for _, result := range toolResults {
		id := strings.TrimSpace(result.ToolCallID)
		if id == "" {
			continue
		}
		if _, ok := existingRequestResults[id]; ok {
			continue
		}
		req.ToolResults = append(req.ToolResults, result)
		existingRequestResults[id] = struct{}{}
	}
	return nil
}

func checkpointToolCalls(calls []model.ToolCallChunk) ([]string, []string) {
	if len(calls) == 0 {
		return nil, nil
	}
	ids := make([]string, 0, len(calls))
	names := make([]string, 0, len(calls))
	for _, call := range calls {
		ids = append(ids, strings.TrimSpace(call.ID))
		names = append(names, strings.TrimSpace(call.Name))
	}
	return ids, names
}

func checkpointAssistantItems(items []model.ConversationItem, cursor int) []model.ConversationItem {
	if len(items) == 0 || cursor <= 0 {
		return nil
	}
	if cursor > len(items) {
		cursor = len(items)
	}
	return append([]model.ConversationItem(nil), items[:cursor]...)
}

func checkpointCompletedToolIDs(results []model.ToolResult) []string {
	if len(results) == 0 {
		return nil
	}
	ids := make([]string, 0, len(results))
	for _, result := range results {
		if id := strings.TrimSpace(result.ToolCallID); id != "" {
			ids = append(ids, id)
		}
	}
	return ids
}

func marshalToolResults(results []model.ToolResult) string {
	if results == nil {
		results = []model.ToolResult{}
	}
	raw, err := json.Marshal(results)
	if err != nil {
		return ""
	}
	return string(raw)
}

func unmarshalToolResults(raw string) ([]model.ToolResult, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	var results []model.ToolResult
	if err := json.Unmarshal([]byte(raw), &results); err != nil {
		return nil, err
	}
	return results, nil
}

func marshalAssistantItems(items []model.ConversationItem) string {
	if items == nil {
		items = []model.ConversationItem{}
	}
	raw, err := json.Marshal(items)
	if err != nil {
		return ""
	}
	return string(raw)
}

func unmarshalAssistantItems(raw string) ([]model.ConversationItem, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	var items []model.ConversationItem
	if err := json.Unmarshal([]byte(raw), &items); err != nil {
		return nil, err
	}
	return items, nil
}
