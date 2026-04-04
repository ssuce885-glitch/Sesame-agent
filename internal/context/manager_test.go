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
	if got.CompactionStart != 1 {
		t.Fatalf("CompactionStart = %d, want 1", got.CompactionStart)
	}
	wantRecent := []string{"turn 2", "turn 3", "turn 4", "turn 5"}
	for i, want := range wantRecent {
		if got.RecentItems[i].Text != want {
			t.Fatalf("RecentItems[%d].Text = %q, want %q", i, got.RecentItems[i].Text, want)
		}
		if got.RecentMessages[i].Content != want {
			t.Fatalf("RecentMessages[%d].Content = %q, want %q", i, got.RecentMessages[i].Content, want)
		}
	}
	if len(got.Summaries) != 1 {
		t.Fatalf("len(Summaries) = %d, want 1", len(got.Summaries))
	}
	if got.NeedsCompact {
		t.Fatal("NeedsCompact = true, want false")
	}
}

func TestManagerBuildMarksCompactWhenThresholdExceeded(t *testing.T) {
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
		model.UserMessageItem("turn 6"),
		model.UserMessageItem("turn 7"),
		model.UserMessageItem("turn 8"),
		model.UserMessageItem("turn 9"),
	}

	got := manager.Build("follow up", items, nil, nil)
	if !got.NeedsCompact {
		t.Fatal("NeedsCompact = false, want true")
	}
	if got.CompactionStart != 5 {
		t.Fatalf("CompactionStart = %d, want 5", got.CompactionStart)
	}
	if len(got.RecentItems) != 4 {
		t.Fatalf("len(RecentItems) = %d, want 4", len(got.RecentItems))
	}
	wantRecent := []string{"turn 6", "turn 7", "turn 8", "turn 9"}
	for i, want := range wantRecent {
		if got.RecentItems[i].Text != want {
			t.Fatalf("RecentItems[%d].Text = %q, want %q", i, got.RecentItems[i].Text, want)
		}
		if got.RecentMessages[i].Content != want {
			t.Fatalf("RecentMessages[%d].Content = %q, want %q", i, got.RecentMessages[i].Content, want)
		}
	}
}

func TestManagerBuildSnapshotsWorkingSet(t *testing.T) {
	manager := NewManager(Config{
		MaxRecentItems:      4,
		MaxEstimatedTokens:  6000,
		CompactionThreshold: 8,
	})

	summary := model.Summary{
		RangeLabel:       "turns 1-2",
		UserGoals:        []string{"explore repo"},
		ImportantChoices: []string{"keep it small"},
		FilesTouched:     []string{"README.md"},
		ToolOutcomes:     []string{"search ok"},
		OpenThreads:      []string{"follow up"},
	}
	items := []model.ConversationItem{
		{
			Kind:    model.ConversationItemToolCall,
			Summary: &summary,
			ToolCall: model.ToolCallChunk{
				ID:    "call-1",
				Name:  "search",
				Input: map[string]any{"query": "alpha"},
			},
		},
		{
			Kind:   model.ConversationItemToolResult,
			Result: &model.ToolResult{Content: "done"},
		},
	}
	summaries := []model.Summary{summary}
	memoryRefs := []string{"memory-1"}

	got := manager.Build("follow up", items, summaries, memoryRefs)

	summary.UserGoals[0] = "mutated"
	items[0].ToolCall.Input["query"] = "beta"
	items[1].Result.Content = "changed"
	summaries[0].FilesTouched[0] = "changed.md"
	memoryRefs[0] = "memory-2"

	if got.RecentItems[0].Summary == nil || got.RecentItems[0].Summary.UserGoals[0] != "explore repo" {
		t.Fatalf("RecentItems[0].Summary = %#v, want independent snapshot", got.RecentItems[0].Summary)
	}
	if got.RecentItems[0].ToolCall.Input["query"] != "alpha" {
		t.Fatalf("RecentItems[0].ToolCall.Input[query] = %q, want alpha", got.RecentItems[0].ToolCall.Input["query"])
	}
	if got.RecentItems[1].Result == nil || got.RecentItems[1].Result.Content != "done" {
		t.Fatalf("RecentItems[1].Result = %#v, want independent snapshot", got.RecentItems[1].Result)
	}
	if got.Summaries[0].FilesTouched[0] != "README.md" {
		t.Fatalf("Summaries[0].FilesTouched[0] = %q, want README.md", got.Summaries[0].FilesTouched[0])
	}
	if got.MemoryRefs[0] != "memory-1" {
		t.Fatalf("MemoryRefs[0] = %q, want memory-1", got.MemoryRefs[0])
	}
}

func TestBuilderBuildPopulatesBothViews(t *testing.T) {
	builder := NewBuilder(2)
	messages := []Message{
		{Role: "user", Content: "u1"},
		{Role: "assistant", Content: "a1"},
		{Role: "tool_result", Content: "tool ok"},
	}

	got := builder.Build(messages, nil, nil)
	if len(got.RecentMessages) != 2 {
		t.Fatalf("len(RecentMessages) = %d, want 2", len(got.RecentMessages))
	}
	if len(got.RecentItems) != 2 {
		t.Fatalf("len(RecentItems) = %d, want 2", len(got.RecentItems))
	}
	if got.RecentItems[0].Kind != model.ConversationItemAssistantText || got.RecentItems[0].Text != "a1" {
		t.Fatalf("RecentItems[0] = %#v, want assistant text", got.RecentItems[0])
	}
	if got.RecentItems[1].Kind != model.ConversationItemToolResult || got.RecentItems[1].Result == nil || got.RecentItems[1].Result.Content != "tool ok" {
		t.Fatalf("RecentItems[1] = %#v, want tool result", got.RecentItems[1])
	}
	if got.RecentMessages[0].Content != "a1" || got.RecentMessages[1].Content != "tool ok" {
		t.Fatalf("RecentMessages = %#v, want converted tail", got.RecentMessages)
	}
}
