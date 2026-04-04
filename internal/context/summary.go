package contextstate

import (
	"strings"

	"go-agent/internal/model"
)

type Summary = model.Summary

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type WorkingContext struct {
	RecentItems    []model.ConversationItem `json:"recent_items"`
	Summaries      []model.Summary          `json:"summaries"`
	MemoryRefs     []string                 `json:"memory_refs"`
	RecentMessages []Message                `json:"recent_messages,omitempty"`
}

func cloneSummary(summary model.Summary) model.Summary {
	return model.Summary{
		RangeLabel:       summary.RangeLabel,
		UserGoals:        append([]string(nil), summary.UserGoals...),
		ImportantChoices: append([]string(nil), summary.ImportantChoices...),
		FilesTouched:     append([]string(nil), summary.FilesTouched...),
		ToolOutcomes:     append([]string(nil), summary.ToolOutcomes...),
		OpenThreads:      append([]string(nil), summary.OpenThreads...),
	}
}

func cloneConversationItem(item model.ConversationItem) model.ConversationItem {
	cloned := item
	if item.Summary != nil {
		summary := cloneSummary(*item.Summary)
		cloned.Summary = &summary
	}
	if item.Result != nil {
		result := *item.Result
		cloned.Result = &result
	}
	if item.ToolCall.Input != nil {
		input := make(map[string]any, len(item.ToolCall.Input))
		for key, value := range item.ToolCall.Input {
			input[key] = value
		}
		cloned.ToolCall.Input = input
	}
	return cloned
}

func cloneConversationItems(items []model.ConversationItem) []model.ConversationItem {
	if len(items) == 0 {
		return nil
	}
	out := make([]model.ConversationItem, len(items))
	for i, item := range items {
		out[i] = cloneConversationItem(item)
	}
	return out
}

func cloneSummaries(summaries []model.Summary) []model.Summary {
	if len(summaries) == 0 {
		return nil
	}
	out := make([]model.Summary, len(summaries))
	for i, summary := range summaries {
		out[i] = cloneSummary(summary)
	}
	return out
}

func cloneStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	return append([]string(nil), values...)
}

func messagesToConversationItems(messages []Message) []model.ConversationItem {
	if len(messages) == 0 {
		return nil
	}

	out := make([]model.ConversationItem, 0, len(messages))
	for _, message := range messages {
		switch message.Role {
		case "user":
			out = append(out, model.UserMessageItem(message.Content))
		case "assistant":
			out = append(out, model.ConversationItem{
				Kind: model.ConversationItemAssistantText,
				Text: message.Content,
			})
		case "tool_result":
			out = append(out, model.ConversationItem{
				Kind: model.ConversationItemToolResult,
				Result: &model.ToolResult{
					Content: message.Content,
				},
			})
		case "summary":
			out = append(out, model.ConversationItem{
				Kind: model.ConversationItemSummary,
				Summary: &model.Summary{
					RangeLabel: strings.TrimSpace(message.Content),
				},
			})
		default:
			out = append(out, model.ConversationItem{
				Kind: model.ConversationItemAssistantText,
				Text: message.Content,
			})
		}
	}

	return out
}

func conversationItemsToMessages(items []model.ConversationItem) []Message {
	if len(items) == 0 {
		return nil
	}

	out := make([]Message, 0, len(items))
	for _, item := range items {
		switch item.Kind {
		case model.ConversationItemUserMessage:
			out = append(out, Message{Role: "user", Content: item.Text})
		case model.ConversationItemAssistantText:
			out = append(out, Message{Role: "assistant", Content: item.Text})
		case model.ConversationItemToolResult:
			content := item.Text
			if item.Result != nil && item.Result.Content != "" {
				content = item.Result.Content
			}
			out = append(out, Message{Role: "tool_result", Content: content})
		case model.ConversationItemSummary:
			content := item.Text
			if item.Summary != nil {
				content = item.Summary.RangeLabel
			}
			out = append(out, Message{Role: "summary", Content: content})
		default:
			out = append(out, Message{Role: "assistant", Content: item.Text})
		}
	}

	return out
}
