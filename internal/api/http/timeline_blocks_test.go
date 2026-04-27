package httpapi

import (
	"testing"

	"go-agent/internal/model"
	"go-agent/internal/types"
)

func TestNormalizeTimelineBlocksSeparatesAssistantItemsAcrossTurns(t *testing.T) {
	blocks := normalizeTimelineBlocks([]types.ConversationTimelineItem{
		{
			TurnID: "turn_user",
			Item:   model.UserMessageItem("start"),
		},
		{
			TurnID: "turn_user",
			Item: model.ConversationItem{
				Kind: model.ConversationItemAssistantText,
				Text: "delegated",
			},
		},
		{
			TurnID: "turn_report",
			Item: model.ConversationItem{
				Kind: model.ConversationItemAssistantText,
				Text: "report handled",
			},
		},
	}, nil)

	if len(blocks) != 3 {
		t.Fatalf("blocks = %d, want 3: %#v", len(blocks), blocks)
	}
	if blocks[1].TurnID != "turn_user" || blocks[1].Content[0].Text != "delegated" {
		t.Fatalf("first assistant block = %#v, want user turn only", blocks[1])
	}
	if blocks[2].TurnID != "turn_report" || blocks[2].Content[0].Text != "report handled" {
		t.Fatalf("second assistant block = %#v, want report turn only", blocks[2])
	}
}
