package contextstate

import "go-agent/internal/model"

func EstimatePromptTokens(userText string, items []model.ConversationItem, summaries []model.Summary, memoryRefs []string) int {
	return estimateConversationTokens(userText, items, summaries, memoryRefs)
}

func estimateConversationTokens(userText string, recentItems []model.ConversationItem, summaries []model.Summary, memoryRefs []string) int {
	total := estimateTextTokens(userText) + 2

	for _, item := range recentItems {
		total += estimateConversationItemTokens(item)
	}
	for _, summary := range summaries {
		total += estimateSummaryTokens(summary)
	}
	for _, ref := range memoryRefs {
		total += estimateTextTokens(ref) + 2
	}

	return total
}

func estimateConversationItemTokens(item model.ConversationItem) int {
	switch item.Kind {
	case model.ConversationItemUserMessage, model.ConversationItemAssistantText:
		return estimateTextTokens(item.Text) + 4
	case model.ConversationItemToolResult:
		content := item.Text
		if item.Result != nil && item.Result.Content != "" {
			content = item.Result.Content
		}
		return estimateTextTokens(content) + 6
	case model.ConversationItemToolCall:
		return estimateTextTokens(toolCallMessageContent(item)) + 8
	case model.ConversationItemSummary:
		return estimateSummaryTokens(itemSummaryFromConversationItem(item)) + 6
	default:
		return estimateTextTokens(item.Text) + 4
	}
}

func estimateSummaryTokens(summary model.Summary) int {
	total := 6 + estimateTextTokens(summary.RangeLabel)
	for _, value := range summary.UserGoals {
		total += estimateTextTokens(value)
	}
	for _, value := range summary.ImportantChoices {
		total += estimateTextTokens(value)
	}
	for _, value := range summary.FilesTouched {
		total += estimateTextTokens(value)
	}
	for _, value := range summary.ToolOutcomes {
		total += estimateTextTokens(value)
	}
	for _, value := range summary.OpenThreads {
		total += estimateTextTokens(value)
	}
	return total
}

func itemSummaryFromConversationItem(item model.ConversationItem) model.Summary {
	if item.Summary != nil {
		return *item.Summary
	}
	return model.Summary{RangeLabel: item.Text}
}

func estimateTextTokens(text string) int {
	if text == "" {
		return 0
	}
	tokens := len(text) / 4
	if tokens == 0 {
		return 1
	}
	return tokens
}

func chooseCompactionAction(items []model.ConversationItem, recentStart int, estimated int, cfg Config) CompactionAction {
	if estimated <= cfg.MaxEstimatedTokens && len(items) <= cfg.CompactionThreshold {
		return CompactionAction{Kind: CompactionActionNone}
	}

	var micro []int
	if cfg.MicrocompactBytesThreshold > 0 {
		for i := 0; i < recentStart; i++ {
			item := items[i]
			if item.Kind == model.ConversationItemToolResult && item.Result != nil && len(item.Result.Content) >= cfg.MicrocompactBytesThreshold {
				micro = append(micro, i)
			}
		}
	}
	if len(micro) > 0 {
		return CompactionAction{Kind: CompactionActionMicrocompact, MicrocompactPositions: micro}
	}

	if recentStart > 0 {
		return CompactionAction{Kind: CompactionActionRolling, RangeStart: 0, RangeEnd: recentStart}
	}
	return CompactionAction{Kind: CompactionActionNone}
}
