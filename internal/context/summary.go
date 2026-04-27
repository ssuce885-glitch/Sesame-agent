package contextstate

import (
	"encoding/json"
	"strings"

	"go-agent/internal/model"
)

type Summary = model.Summary

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type SummaryBundle struct {
	ContextHeadSummary *model.Summary  `json:"context_head_summary,omitempty"`
	Boundary           *model.Summary  `json:"boundary,omitempty"`
	Rolling            []model.Summary `json:"rolling,omitempty"`
}

type WorkingContext struct {
	CarryForwardItems []model.ConversationItem `json:"carry_forward_items,omitempty"`
	RecentRawItems    []model.ConversationItem `json:"recent_raw_items,omitempty"`
	RecentItems       []model.ConversationItem `json:"recent_items"`
	PromptItems       []model.ConversationItem `json:"prompt_items,omitempty"`
	Summaries         SummaryBundle            `json:"summaries"`
	MemoryRefs        []string                 `json:"memory_refs"`
	RecentMessages    []Message                `json:"recent_messages,omitempty"`
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
	if len(item.Parts) > 0 {
		cloned.Parts = append([]model.ContentPart(nil), item.Parts...)
	}
	if item.Summary != nil {
		summary := cloneSummary(*item.Summary)
		cloned.Summary = &summary
	}
	if item.Result != nil {
		result := *item.Result
		cloned.Result = &result
	}
	if item.ToolCall.Input != nil {
		if input, ok := cloneJSONValue(item.ToolCall.Input).(map[string]any); ok {
			cloned.ToolCall.Input = input
		}
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

func cloneSummaryBundle(bundle SummaryBundle) SummaryBundle {
	var contextHeadSummary *model.Summary
	if bundle.ContextHeadSummary != nil {
		summary := cloneSummary(*bundle.ContextHeadSummary)
		contextHeadSummary = &summary
	}

	var boundary *model.Summary
	if bundle.Boundary != nil {
		summary := cloneSummary(*bundle.Boundary)
		boundary = &summary
	}

	return SummaryBundle{
		ContextHeadSummary: contextHeadSummary,
		Boundary:           boundary,
		Rolling:            cloneSummaries(bundle.Rolling),
	}
}

func flattenSummaryBundle(bundle SummaryBundle) []model.Summary {
	out := make([]model.Summary, 0, 2+len(bundle.Rolling))
	if bundle.ContextHeadSummary != nil {
		out = append(out, cloneSummary(*bundle.ContextHeadSummary))
	}
	if bundle.Boundary != nil {
		out = append(out, cloneSummary(*bundle.Boundary))
	}
	out = append(out, cloneSummaries(bundle.Rolling)...)
	return out
}

func cloneStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	return append([]string(nil), values...)
}

func cloneJSONValue(value any) any {
	if value == nil {
		return nil
	}

	switch v := value.(type) {
	case string, bool,
		int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64, uintptr,
		float32, float64,
		json.Number:
		return v
	case []any:
		out := make([]any, len(v))
		for i, elem := range v {
			out[i] = cloneJSONValue(elem)
		}
		return out
	case []string:
		return append([]string(nil), v...)
	case []map[string]any:
		out := make([]map[string]any, len(v))
		for i, elem := range v {
			out[i] = cloneJSONMap(elem)
		}
		return out
	case map[string]any:
		return cloneJSONMap(v)
	}

	return value
}

func cloneJSONMap(value map[string]any) map[string]any {
	if value == nil {
		return nil
	}

	out := make(map[string]any, len(value))
	for key, elem := range value {
		out[key] = cloneJSONValue(elem)
	}
	return out
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
		case "assistant_thinking":
			out = append(out, model.AssistantThinkingItem(message.Content))
		case "tool_result":
			out = append(out, model.ConversationItem{
				Kind: model.ConversationItemToolResult,
				Result: &model.ToolResult{
					Content: message.Content,
				},
			})
		case "tool_call":
			out = append(out, model.ConversationItem{
				Kind: model.ConversationItemToolCall,
				Text: message.Content,
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
		case model.ConversationItemAssistantThinking:
			out = append(out, Message{Role: "assistant_thinking", Content: item.Text})
		case model.ConversationItemAssistantText:
			out = append(out, Message{Role: "assistant", Content: item.Text})
		case model.ConversationItemToolResult:
			content := item.Text
			if item.Result != nil && item.Result.Content != "" {
				content = item.Result.Content
			}
			out = append(out, Message{Role: "tool_result", Content: content})
		case model.ConversationItemToolCall:
			out = append(out, Message{Role: "tool_call", Content: toolCallMessageContent(item)})
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

func toolCallMessageContent(item model.ConversationItem) string {
	name := item.ToolCall.Name
	if name == "" {
		name = "tool"
	}
	payload := item.ToolCall.Input
	if payload == nil {
		payload = map[string]any{}
	}
	body, err := json.Marshal(payload)
	if err != nil {
		body = []byte("{}")
	}
	if item.ToolCall.ID != "" {
		return item.ToolCall.ID + ": " + name + " " + string(body)
	}
	return name + " " + string(body)
}
