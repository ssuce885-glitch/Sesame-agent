package engine

import (
	"context"
	"strings"

	"go-agent/internal/model"
)

func loadPromptItemsForCurrentHead(ctx context.Context, store ConversationStore, sessionID string) ([]model.ConversationItem, error) {
	if store == nil {
		return nil, nil
	}

	headID, ok, err := store.GetCurrentContextHeadID(ctx)
	if err != nil {
		return nil, err
	}
	if !ok || strings.TrimSpace(headID) == "" {
		return nil, nil
	}
	return store.ListConversationItemsByContextHead(ctx, sessionID, headID)
}
