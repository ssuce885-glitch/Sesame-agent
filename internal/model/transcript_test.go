package model

import (
	"context"
	"testing"
)

func TestValidateConversationItemsRejectsOrphanToolResult(t *testing.T) {
	err := ValidateConversationItems([]ConversationItem{
		UserMessageItem("news"),
		ToolResultItem(ToolResult{
			ToolCallID: "call_1",
			ToolName:   "web_fetch",
			Content:    "ok",
		}),
	})
	if err == nil {
		t.Fatal("ValidateConversationItems() error = nil, want orphan tool result error")
	}
}

func TestNearestSafeConversationBoundaryBacksOffOpenToolExchange(t *testing.T) {
	items := []ConversationItem{
		UserMessageItem("start"),
		{
			Kind: ConversationItemToolCall,
			ToolCall: ToolCallChunk{
				ID:    "call_1",
				Name:  "web_fetch",
				Input: map[string]any{"url": "https://example.com"},
			},
		},
		ToolResultItem(ToolResult{
			ToolCallID: "call_1",
			ToolName:   "web_fetch",
			Content:    "done",
		}),
	}

	if got := NearestSafeConversationBoundary(items, 2); got != 1 {
		t.Fatalf("NearestSafeConversationBoundary(..., 2) = %d, want 1", got)
	}
	if got := NearestSafeConversationBoundary(items, 3); got != 3 {
		t.Fatalf("NearestSafeConversationBoundary(..., 3) = %d, want 3", got)
	}
}

func TestAdaptiveProviderRejectsInvalidTranscriptBeforeTransport(t *testing.T) {
	transport := NewFakeStreaming([][]StreamEvent{
		{{Kind: StreamEventMessageEnd}},
	})
	provider := NewAdaptiveProvider(ResolvedProviderConfig{
		Profile: ProviderProfile{
			ID:                 ProviderProfileMiniMax,
			StrictToolSequence: true,
		},
	}, transport)

	events, errs := provider.Stream(context.Background(), Request{
		Items: []ConversationItem{
			ToolResultItem(ToolResult{
				ToolCallID: "call_1",
				ToolName:   "web_fetch",
				Content:    "orphan",
			}),
		},
	})

	if _, ok := <-events; ok {
		t.Fatal("events channel yielded data, want closed channel")
	}
	if err := <-errs; err == nil {
		t.Fatal("error = nil, want transcript validation error")
	}
	if got := transport.LastRequest(); len(got.Items) != 0 {
		t.Fatalf("transport LastRequest() = %#v, want no transport call", got)
	}
}
