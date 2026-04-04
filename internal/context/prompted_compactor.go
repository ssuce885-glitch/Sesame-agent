package contextstate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"go-agent/internal/model"
)

type PromptedCompactor struct {
	client model.StreamingClient
	model  string
}

func NewPromptedCompactor(client model.StreamingClient, compactModel string) *PromptedCompactor {
	return &PromptedCompactor{
		client: client,
		model:  compactModel,
	}
}

func (p *PromptedCompactor) Compact(ctx context.Context, items []model.ConversationItem) (model.Summary, error) {
	req := model.Request{
		Model:        p.model,
		Instructions: promptedSummaryInstructions,
		Stream:       true,
		Items:        cloneConversationItems(items),
	}

	events, errs := p.client.Stream(ctx, req)

	var text strings.Builder
	sawMessageEnd := false
	for event := range events {
		switch event.Kind {
		case model.StreamEventTextDelta:
			text.WriteString(event.TextDelta)
		case model.StreamEventMessageEnd:
			sawMessageEnd = true
		}
	}

	if errs != nil {
		if err := <-errs; err != nil {
			return model.Summary{}, fmt.Errorf("prompted compactor stream failed: %w", err)
		}
	}

	if !sawMessageEnd {
		return model.Summary{}, errors.New("prompted compactor stream ended before message end")
	}

	raw := strings.TrimSpace(text.String())
	if raw == "" {
		return model.Summary{}, errors.New("prompted compactor returned empty summary JSON")
	}

	var payload struct {
		RangeLabel       string   `json:"range_label"`
		UserGoals        []string `json:"user_goals"`
		ImportantChoices []string `json:"important_choices"`
		FilesTouched     []string `json:"files_touched"`
		ToolOutcomes     []string `json:"tool_outcomes"`
		OpenThreads      []string `json:"open_threads"`
	}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return model.Summary{}, fmt.Errorf("prompted compactor: decode summary JSON: %w; payload=%q", err, raw)
	}

	return model.Summary{
		RangeLabel:       payload.RangeLabel,
		UserGoals:        payload.UserGoals,
		ImportantChoices: payload.ImportantChoices,
		FilesTouched:     payload.FilesTouched,
		ToolOutcomes:     payload.ToolOutcomes,
		OpenThreads:      payload.OpenThreads,
	}, nil
}

const promptedSummaryInstructions = `You are summarizing a conversation into a strict JSON object.
Return pure JSON only. Do not use markdown, code fences, or commentary.
The object must contain exactly these keys:
- range_label
- user_goals
- important_choices
- files_touched
- tool_outcomes
- open_threads
Use strings for range_label and arrays of strings for the remaining keys.`
