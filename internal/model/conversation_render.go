package model

import (
	"encoding/json"
	"strings"
)

func renderSummaryContent(summary *Summary, fallback string) string {
	if summary == nil {
		return strings.TrimSpace(fallback)
	}

	lines := []string{"[Conversation summary]"}
	if label := strings.TrimSpace(summary.RangeLabel); label != "" {
		lines = append(lines, "Range: "+label)
	}
	appendSummarySection(&lines, "User goals", summary.UserGoals)
	appendSummarySection(&lines, "Important choices", summary.ImportantChoices)
	appendSummarySection(&lines, "Files touched", summary.FilesTouched)
	appendSummarySection(&lines, "Tool outcomes", summary.ToolOutcomes)
	appendSummarySection(&lines, "Open threads", summary.OpenThreads)

	return strings.Join(lines, "\n")
}

func appendSummarySection(lines *[]string, title string, values []string) {
	trimmed := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		trimmed = append(trimmed, value)
	}
	if len(trimmed) == 0 {
		return
	}

	*lines = append(*lines, title+":")
	for _, value := range trimmed {
		*lines = append(*lines, "- "+value)
	}
}

func normalizedToolCallArguments(call ToolCallChunk) string {
	input := normalizedToolCallInput(call)
	raw, err := json.Marshal(input)
	if err != nil {
		return "{}"
	}
	return string(raw)
}

func normalizedToolCallInput(call ToolCallChunk) map[string]any {
	if len(call.Input) > 0 {
		return cloneToolCallInput(call.Input)
	}

	chunk := strings.TrimSpace(call.InputChunk)
	if chunk == "" {
		return map[string]any{}
	}

	var input map[string]any
	if err := json.Unmarshal([]byte(chunk), &input); err != nil || input == nil {
		return map[string]any{}
	}
	return input
}

func cloneToolCallInput(input map[string]any) map[string]any {
	if len(input) == 0 {
		return map[string]any{}
	}

	cloned := make(map[string]any, len(input))
	for key, value := range input {
		cloned[key] = value
	}
	return cloned
}
