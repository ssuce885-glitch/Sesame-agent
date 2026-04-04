package contextstate

import (
	"testing"

	"go-agent/internal/model"
)

func TestManagerBuildSelectsRecentItemsAndSummaries(t *testing.T) {
	manager := NewManager(Config{
		MaxRecentItems:      4,
		MaxEstimatedTokens:  6000,
		CompactionThreshold: 8,
	})

	items := []model.ConversationItem{
		model.UserMessageItem("turn 1"),
		model.UserMessageItem("turn 2"),
		model.UserMessageItem("turn 3"),
		model.UserMessageItem("turn 4"),
		model.UserMessageItem("turn 5"),
	}
	summaries := []model.Summary{
		{RangeLabel: "turns 1-2", UserGoals: []string{"explore repo"}},
	}

	got := manager.Build("follow up", items, summaries, nil)
	if len(got.RecentItems) != 4 {
		t.Fatalf("len(RecentItems) = %d, want 4", len(got.RecentItems))
	}
	if len(got.Summaries) != 1 {
		t.Fatalf("len(Summaries) = %d, want 1", len(got.Summaries))
	}
	if !got.NeedsCompact {
		t.Fatal("NeedsCompact = false, want true")
	}
}
