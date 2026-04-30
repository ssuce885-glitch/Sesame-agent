package engine

import (
	"encoding/json"
	"strings"

	"go-agent/internal/model"
)

func encodeContextHeadSummaryPayload(summary model.Summary) string {
	raw, _ := json.Marshal(summary)
	return string(raw)
}

func decodeContextHeadSummaryPayload(raw string) (model.Summary, bool, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return model.Summary{}, false, nil
	}

	var summary model.Summary
	if err := json.Unmarshal([]byte(raw), &summary); err != nil {
		return model.Summary{
			RangeLabel:  contextHeadSummaryRangeLabel,
			OpenThreads: []string{raw},
		}, true, nil
	}
	if strings.TrimSpace(summary.RangeLabel) == "" {
		summary.RangeLabel = contextHeadSummaryRangeLabel
	}
	return summary, true, nil
}

type compactionSummaryPayload struct {
	RangeLabel       string   `json:"range_label"`
	UserGoals        []string `json:"user_goals"`
	ImportantChoices []string `json:"important_choices"`
	FilesTouched     []string `json:"files_touched"`
	ToolOutcomes     []string `json:"tool_outcomes"`
	OpenThreads      []string `json:"open_threads"`
}

func decodeCompactionSummaryPayload(raw string) (model.Summary, bool, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return model.Summary{}, false, nil
	}

	var standard model.Summary
	if err := json.Unmarshal([]byte(raw), &standard); err != nil {
		return model.Summary{}, false, err
	}

	var snake compactionSummaryPayload
	if err := json.Unmarshal([]byte(raw), &snake); err != nil {
		return model.Summary{}, false, err
	}

	summary := standard
	if strings.TrimSpace(summary.RangeLabel) == "" {
		summary.RangeLabel = snake.RangeLabel
	}
	if len(summary.UserGoals) == 0 {
		summary.UserGoals = append([]string(nil), snake.UserGoals...)
	}
	if len(summary.ImportantChoices) == 0 {
		summary.ImportantChoices = append([]string(nil), snake.ImportantChoices...)
	}
	if len(summary.FilesTouched) == 0 {
		summary.FilesTouched = append([]string(nil), snake.FilesTouched...)
	}
	if len(summary.ToolOutcomes) == 0 {
		summary.ToolOutcomes = append([]string(nil), snake.ToolOutcomes...)
	}
	if len(summary.OpenThreads) == 0 {
		summary.OpenThreads = append([]string(nil), snake.OpenThreads...)
	}

	if isZeroSummary(summary) {
		return model.Summary{}, false, nil
	}
	return summary, true, nil
}
