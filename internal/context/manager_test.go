package contextstate

import (
	"strings"
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

func TestManagerBuildDoesNotCompactAtThresholdBoundary(t *testing.T) {
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
	}

	got := manager.Build("follow up", items, nil, nil)
	if got.NeedsCompact {
		t.Fatal("NeedsCompact = true, want false at threshold boundary")
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

func TestManagerBuildDeepClonesNestedToolCallInput(t *testing.T) {
	manager := NewManager(Config{
		MaxRecentItems:      4,
		MaxEstimatedTokens:  6000,
		CompactionThreshold: 8,
	})

	nested := map[string]any{
		"query": "alpha",
		"filters": map[string]any{
			"tags": []any{"one", "two"},
		},
		"steps": []any{
			map[string]any{"name": "first"},
			map[string]any{"name": "second"},
		},
		"names": []string{"a", "b"},
		"maps": []map[string]any{
			{"kind": "nested"},
		},
	}
	items := []model.ConversationItem{
		{
			Kind: model.ConversationItemToolCall,
			ToolCall: model.ToolCallChunk{
				ID:    "call-1",
				Name:  "search",
				Input: nested,
			},
		},
	}

	got := manager.Build("follow up", items, nil, nil)
	nested["query"] = "beta"
	nested["filters"].(map[string]any)["tags"].([]any)[0] = "changed"
	nested["steps"].([]any)[0].(map[string]any)["name"] = "changed"
	nested["names"].([]string)[0] = "changed"
	nested["maps"].([]map[string]any)[0]["kind"] = "changed"

	if got.RecentItems[0].ToolCall.Input["query"] != "alpha" {
		t.Fatalf("query = %q, want alpha", got.RecentItems[0].ToolCall.Input["query"])
	}
	filters := got.RecentItems[0].ToolCall.Input["filters"].(map[string]any)
	tags := filters["tags"].([]any)
	if tags[0] != "one" {
		t.Fatalf("tags[0] = %v, want one", tags[0])
	}
	steps := got.RecentItems[0].ToolCall.Input["steps"].([]any)
	if steps[0].(map[string]any)["name"] != "first" {
		t.Fatalf("steps[0].name = %v, want first", steps[0].(map[string]any)["name"])
	}
	names := got.RecentItems[0].ToolCall.Input["names"].([]string)
	if names[0] != "a" {
		t.Fatalf("names[0] = %q, want a", names[0])
	}
	maps := got.RecentItems[0].ToolCall.Input["maps"].([]map[string]any)
	if maps[0]["kind"] != "nested" {
		t.Fatalf("maps[0].kind = %v, want nested", maps[0]["kind"])
	}
}

func TestManagerBuildKeepsToolCallSemanticMessage(t *testing.T) {
	manager := NewManager(Config{
		MaxRecentItems:      4,
		MaxEstimatedTokens:  6000,
		CompactionThreshold: 8,
	})

	got := manager.Build("follow up", []model.ConversationItem{
		{
			Kind: model.ConversationItemToolCall,
			ToolCall: model.ToolCallChunk{
				ID:    "call-1",
				Name:  "search",
				Input: map[string]any{"query": "alpha"},
			},
		},
	}, nil, nil)

	if len(got.RecentMessages) != 1 {
		t.Fatalf("len(RecentMessages) = %d, want 1", len(got.RecentMessages))
	}
	if got.RecentMessages[0].Role != "tool_call" {
		t.Fatalf("RecentMessages[0].Role = %q, want tool_call", got.RecentMessages[0].Role)
	}
	if !strings.Contains(got.RecentMessages[0].Content, "search") || !strings.Contains(got.RecentMessages[0].Content, "\"query\":\"alpha\"") {
		t.Fatalf("RecentMessages[0].Content = %q, want semantic tool call text", got.RecentMessages[0].Content)
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
