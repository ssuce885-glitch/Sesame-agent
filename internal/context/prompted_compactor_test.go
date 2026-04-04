package contextstate

import (
	"context"
	"strings"
	"testing"

	"go-agent/internal/model"
)

func TestPromptedCompactorBuildsStructuredSummary(t *testing.T) {
	client := model.NewFakeStreaming([][]model.StreamEvent{{
		{Kind: model.StreamEventTextDelta, TextDelta: `{"range_label":"turns 1-4","user_goals":["keep working"],`},
		{Kind: model.StreamEventTextDelta, TextDelta: `"important_choices":["picked approach"],"files_touched":["internal/context/prompted_compactor.go"],`},
		{Kind: model.StreamEventTextDelta, TextDelta: `"tool_outcomes":["ran tests"],"open_threads":["follow up"]}`},
		{Kind: model.StreamEventMessageEnd},
	}})

	compactor := NewPromptedCompactor(client, "compact-model")
	items := []model.ConversationItem{
		model.UserMessageItem("please compact this"),
		{
			Kind: model.ConversationItemAssistantText,
			Text: "working on it",
		},
	}

	summary, err := compactor.Compact(context.Background(), items)
	if err != nil {
		t.Fatalf("Compact() error = %v", err)
	}

	if summary.RangeLabel != "turns 1-4" {
		t.Fatalf("RangeLabel = %q, want %q", summary.RangeLabel, "turns 1-4")
	}
	if got := strings.Join(summary.UserGoals, ","); got != "keep working" {
		t.Fatalf("UserGoals = %v, want keep working", summary.UserGoals)
	}
	if got := strings.Join(summary.ImportantChoices, ","); got != "picked approach" {
		t.Fatalf("ImportantChoices = %v, want picked approach", summary.ImportantChoices)
	}
	if got := strings.Join(summary.FilesTouched, ","); got != "internal/context/prompted_compactor.go" {
		t.Fatalf("FilesTouched = %v, want prompted_compactor.go", summary.FilesTouched)
	}
	if got := strings.Join(summary.ToolOutcomes, ","); got != "ran tests" {
		t.Fatalf("ToolOutcomes = %v, want ran tests", summary.ToolOutcomes)
	}
	if got := strings.Join(summary.OpenThreads, ","); got != "follow up" {
		t.Fatalf("OpenThreads = %v, want follow up", summary.OpenThreads)
	}

	req := client.LastRequest()
	if req.Model != "compact-model" {
		t.Fatalf("LastRequest().Model = %q, want %q", req.Model, "compact-model")
	}
	if !req.Stream {
		t.Fatal("LastRequest().Stream = false, want true")
	}
	if len(req.Tools) != 0 {
		t.Fatalf("len(LastRequest().Tools) = %d, want 0", len(req.Tools))
	}
	if req.Cache != nil {
		t.Fatalf("LastRequest().Cache = %#v, want nil", req.Cache)
	}
	if len(req.Items) != len(items) {
		t.Fatalf("len(LastRequest().Items) = %d, want %d", len(req.Items), len(items))
	}
	if req.Instructions == "" {
		t.Fatal("LastRequest().Instructions = empty, want prompted JSON instructions")
	}
	for _, want := range []string{
		"pure JSON",
		"range_label",
		"user_goals",
		"important_choices",
		"files_touched",
		"tool_outcomes",
		"open_threads",
	} {
		if !strings.Contains(req.Instructions, want) {
			t.Fatalf("Instructions missing %q: %q", want, req.Instructions)
		}
	}
}
