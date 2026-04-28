package engine

import (
	"context"
	"errors"
	"testing"

	contextstate "go-agent/internal/context"
	"go-agent/internal/model"
)

type failingCompactor struct {
	calls int
}

func (c *failingCompactor) Compact(context.Context, []model.ConversationItem) (model.Summary, error) {
	c.calls++
	return model.Summary{}, errors.New("planned compaction failure")
}

func TestRunCompactionPassesSkipsAfterThreeSummaryFailures(t *testing.T) {
	compactor := &failingCompactor{}
	engine := &Engine{
		ctxManager: contextstate.NewManager(contextstate.Config{}),
		compactor:  compactor,
	}
	items := []model.ConversationItem{
		model.UserMessageItem("one"),
		{Kind: model.ConversationItemAssistantText, Text: "two"},
	}
	working := contextstate.WorkingSet{
		CompactionStart: len(items),
		Action: contextstate.CompactionAction{
			Kind: contextstate.CompactionActionRolling,
		},
	}

	for attempt := 1; attempt <= 3; attempt++ {
		if _, _, err := runCompactionPasses(
			context.Background(),
			engine,
			"session_1",
			"/workspace",
			"head_1",
			"user",
			items,
			SummaryBundle{},
			nil,
			nil,
			nil,
			0,
			working,
		); err == nil {
			t.Fatalf("attempt %d returned nil error, want compaction failure", attempt)
		}
	}

	if _, _, err := runCompactionPasses(
		context.Background(),
		engine,
		"session_1",
		"/workspace",
		"head_1",
		"user",
		items,
		SummaryBundle{},
		nil,
		nil,
		nil,
		0,
		working,
	); err != nil {
		t.Fatalf("fourth attempt returned error after circuit opened: %v", err)
	}
	if compactor.calls != 3 {
		t.Fatalf("compactor calls = %d, want 3", compactor.calls)
	}
}
