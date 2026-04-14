package engine

import (
	contextstate "go-agent/internal/context"
	"go-agent/internal/model"
)

func buildReinjectedPromptItems(bundle SummaryBundle, carryForward []model.ConversationItem, recentRaw []model.ConversationItem, userItem model.ConversationItem) []model.ConversationItem {
	return contextstate.BuildReinjectedPromptItems(bundle, carryForward, recentRaw, userItem)
}
