package engine

import (
	"testing"

	"go-agent/internal/model"
)

func TestShouldRefreshContextHeadSummaryExistingRequiresCooldown(t *testing.T) {
	items := make([]model.ConversationItem, contextHeadSummaryBootstrapMinItems)
	for i := range items {
		items[i] = model.ConversationItem{Kind: model.ConversationItemUserMessage, Text: "small update"}
	}

	if shouldRefreshContextHeadSummary(true, items) {
		t.Fatalf("existing summary refreshed after %d low-signal items, want cooldown", len(items))
	}

	items = append(items, make([]model.ConversationItem, contextHeadSummaryCooldownMinItems-len(items))...)
	for i := range items {
		if items[i].Kind == "" {
			items[i] = model.ConversationItem{Kind: model.ConversationItemUserMessage, Text: "small update"}
		}
	}
	if !shouldRefreshContextHeadSummary(true, items) {
		t.Fatalf("existing summary did not refresh after %d items", len(items))
	}
}

func TestShouldRefreshContextHeadSummaryBootstrapKeepsLowerThreshold(t *testing.T) {
	items := make([]model.ConversationItem, contextHeadSummaryBootstrapMinItems)
	for i := range items {
		items[i] = model.ConversationItem{Kind: model.ConversationItemUserMessage, Text: "small update"}
	}

	if !shouldRefreshContextHeadSummary(false, items) {
		t.Fatalf("bootstrap summary did not refresh after %d items", len(items))
	}
}
