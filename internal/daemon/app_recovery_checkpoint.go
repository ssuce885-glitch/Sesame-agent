package daemon

import (
	"context"
	"encoding/json"
	"strings"

	"go-agent/internal/model"
	rolectx "go-agent/internal/roles"
	"go-agent/internal/session"
	"go-agent/internal/sessionrole"
	"go-agent/internal/store/sqlite"
	"go-agent/internal/types"
	"go-agent/internal/workspace"
)

func resumeTurnFromCheckpoint(ctx context.Context, store *sqlite.Store, manager *session.Manager, checkpoint types.TurnCheckpoint) (bool, error) {
	if store == nil || manager == nil || strings.TrimSpace(checkpoint.TurnID) == "" {
		return false, nil
	}
	if checkpoint.State != types.TurnCheckpointStatePostToolBatch {
		return false, nil
	}

	turn, found, err := store.GetTurn(ctx, checkpoint.TurnID)
	if err != nil || !found {
		return false, err
	}
	if isTurnTerminal(turn.State) {
		return false, nil
	}

	sessionID := strings.TrimSpace(firstNonEmptyRecoveryString(checkpoint.SessionID, turn.SessionID))
	sessionRow, found, err := store.GetSession(ctx, sessionID)
	if err != nil || !found {
		return false, err
	}
	if err := ensureCheckpointConversationItems(ctx, store, turn, checkpoint); err != nil {
		return false, err
	}

	replayCtx, ok, err := replayContextForRecoveredTurn(ctx, store, sessionRow)
	if err != nil || !ok {
		return false, err
	}
	_, err = manager.SubmitTurn(replayCtx, sessionRow.ID, session.SubmitTurnInput{Turn: turn})
	if err != nil {
		return false, err
	}
	return true, nil
}

func ensureCheckpointConversationItems(ctx context.Context, store *sqlite.Store, turn types.Turn, checkpoint types.TurnCheckpoint) error {
	assistantItems, err := decodeCheckpointAssistantItems(checkpoint.AssistantItemsJSON)
	if err != nil {
		return err
	}
	toolResults, err := decodeCheckpointToolResults(checkpoint.ToolResultsJSON)
	if err != nil {
		return err
	}
	if len(assistantItems) == 0 && len(toolResults) == 0 {
		return nil
	}

	timeline, err := store.ListConversationTimelineItems(ctx, turn.SessionID)
	if err != nil {
		return err
	}
	existingToolCalls := make(map[string]struct{})
	existingToolResults := make(map[string]struct{})
	for _, item := range timeline {
		if strings.TrimSpace(item.TurnID) != strings.TrimSpace(turn.ID) {
			continue
		}
		switch item.Item.Kind {
		case model.ConversationItemToolCall:
			if id := strings.TrimSpace(item.Item.ToolCall.ID); id != "" {
				existingToolCalls[id] = struct{}{}
			}
		case model.ConversationItemToolResult:
			if item.Item.Result != nil {
				if id := strings.TrimSpace(item.Item.Result.ToolCallID); id != "" {
					existingToolResults[id] = struct{}{}
				}
			}
		}
	}

	maxPosition, ok, err := store.MaxConversationPosition(ctx, turn.SessionID)
	if err != nil {
		return err
	}
	nextPosition := 1
	if ok {
		nextPosition = maxPosition + 1
	}
	contextHeadID := strings.TrimSpace(turn.ContextHeadID)
	if contextHeadID == "" {
		if current, currentOK, err := store.GetCurrentContextHeadID(ctx); err != nil {
			return err
		} else if currentOK {
			contextHeadID = strings.TrimSpace(current)
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
		if _, exists := existingToolCalls[id]; !exists {
			missingAssistantToolCall = true
			break
		}
	}
	if missingAssistantToolCall {
		for _, item := range assistantItems {
			if skipCheckpointConversationItem(item) {
				continue
			}
			if item.Kind == model.ConversationItemToolCall {
				id := strings.TrimSpace(item.ToolCall.ID)
				if id != "" {
					if _, exists := existingToolCalls[id]; exists {
						continue
					}
					existingToolCalls[id] = struct{}{}
				}
			}
			if err := store.InsertConversationItemWithContextHead(ctx, turn.SessionID, contextHeadID, turn.ID, nextPosition, item); err != nil {
				return err
			}
			nextPosition++
		}
	}

	for _, result := range toolResults {
		id := strings.TrimSpace(result.ToolCallID)
		if id == "" {
			continue
		}
		if _, exists := existingToolResults[id]; exists {
			continue
		}
		if err := store.InsertConversationItemWithContextHead(ctx, turn.SessionID, contextHeadID, turn.ID, nextPosition, model.ToolResultItem(result)); err != nil {
			return err
		}
		existingToolResults[id] = struct{}{}
		nextPosition++
	}
	return nil
}

func replayContextForRecoveredTurn(ctx context.Context, store *sqlite.Store, sessionRow types.Session) (context.Context, bool, error) {
	sessionID := strings.TrimSpace(sessionRow.ID)
	if sessionID == "" {
		return nil, false, nil
	}
	if strings.HasPrefix(sessionID, "task_session_") {
		return workspace.WithWorkspaceRoot(sessionrole.WithSessionRole(ctx, types.SessionRoleMainParent), sessionRow.WorkspaceRoot), true, nil
	}

	specialistRoleID, err := store.ResolveSpecialistRoleID(ctx, sessionID, sessionRow.WorkspaceRoot)
	if err != nil {
		return nil, false, err
	}
	role, err := store.ResolveSessionRole(ctx, sessionID, sessionRow.WorkspaceRoot)
	if err != nil {
		return nil, false, err
	}
	if specialistRoleID == "" && role != types.SessionRoleMainParent {
		return nil, false, nil
	}

	replayRole := role
	if specialistRoleID != "" {
		replayRole = types.SessionRoleMainParent
	}
	return workspace.WithWorkspaceRoot(
		rolectx.WithSpecialistRoleID(sessionrole.WithSessionRole(ctx, replayRole), specialistRoleID),
		sessionRow.WorkspaceRoot,
	), true, nil
}

func decodeCheckpointAssistantItems(raw string) ([]model.ConversationItem, error) {
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

func decodeCheckpointToolResults(raw string) ([]model.ToolResult, error) {
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

func skipCheckpointConversationItem(item model.ConversationItem) bool {
	switch item.Kind {
	case model.ConversationItemAssistantText:
		return strings.TrimSpace(item.Text) == ""
	case model.ConversationItemAssistantThinking:
		return strings.TrimSpace(item.Text) == "" && strings.TrimSpace(item.ThinkingSignature) == ""
	default:
		return false
	}
}

func firstNonEmptyRecoveryString(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
