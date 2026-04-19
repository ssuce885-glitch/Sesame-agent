package engine

import (
	"context"
	"strings"

	"go-agent/internal/model"
)

func loadPromptItemsForHead(ctx context.Context, store ConversationStore, sessionID, preferredHeadID string) ([]model.ConversationItem, error) {
	if store == nil || strings.TrimSpace(sessionID) == "" {
		return nil, nil
	}

	headID, err := resolveConversationReadContextHeadID(ctx, store, preferredHeadID)
	if err != nil || strings.TrimSpace(headID) == "" {
		return nil, err
	}
	return store.ListConversationItemsByContextHead(ctx, sessionID, headID)
}
