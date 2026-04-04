package contextstate

import (
	"testing"

	"go-agent/internal/model"
)

func TestBuilderKeepsRecentTailAndSummaries(t *testing.T) {
	builder := NewBuilder(3)
	messages := []Message{
		{Role: "user", Content: "u1"},
		{Role: "assistant", Content: "a1"},
		{Role: "user", Content: "u2"},
		{Role: "assistant", Content: "a2"},
		{Role: "user", Content: "u3"},
	}
	summary := model.Summary{RangeLabel: "finish task"}

	ctx := builder.Build(messages, []Summary{summary}, nil)
	if len(ctx.RecentMessages) != 3 {
		t.Fatalf("len(RecentMessages) = %d, want 3", len(ctx.RecentMessages))
	}
	if len(ctx.Summaries) != 1 {
		t.Fatalf("len(Summaries) = %d, want 1", len(ctx.Summaries))
	}
}
