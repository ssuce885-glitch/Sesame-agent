package contextstate

import (
	"testing"

	"go-agent/internal/model"
)

func TestPrepareRequestStripsAnthropicToolContinuationHistoryOnFreshTurn(t *testing.T) {
	runtime := NewRuntime(3600, 3)
	plan := WorkingSet{
		WorkingContext: WorkingContext{
			RecentRawItems: []model.ConversationItem{
				model.UserMessageItem("old request"),
				model.AssistantThinkingBlockItem("private chain", "sig_1"),
				{Kind: model.ConversationItemAssistantText, Text: "I will inspect it."},
				{Kind: model.ConversationItemToolCall, ToolCall: model.ToolCallChunk{ID: "call_1", Name: "list_dir"}},
				model.ToolResultItem(model.ToolResult{ToolCallID: "call_1", ToolName: "list_dir", Content: "result"}),
			},
		},
	}

	req := runtime.PrepareRequest(
		plan,
		nil,
		model.ProviderCapabilities{RequiresThinkingForToolContinuation: true},
		model.UserMessageItem("fresh child report turn"),
		"instructions",
	)

	for _, item := range req.Items {
		switch item.Kind {
		case model.ConversationItemAssistantThinking, model.ConversationItemToolCall, model.ConversationItemToolResult:
			t.Fatalf("fresh Anthropic turn reinjected continuation item %#v", item.Kind)
		}
	}
	if !containsKind(req.Items, model.ConversationItemAssistantText) {
		t.Fatalf("assistant text history was removed; items = %#v", req.Items)
	}
	if got := req.Items[len(req.Items)-1]; got.Kind != model.ConversationItemUserMessage || got.Text != "fresh child report turn" {
		t.Fatalf("last item = %#v, want fresh user item", got)
	}
}

func containsKind(items []model.ConversationItem, kind model.ConversationItemKind) bool {
	for _, item := range items {
		if item.Kind == kind {
			return true
		}
	}
	return false
}
