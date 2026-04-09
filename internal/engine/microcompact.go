package engine

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"go-agent/internal/model"
	"go-agent/internal/types"
)

const (
	microcompactPreviewChars = 240
	microcompactPreviewLines = 12
)

type persistedMicrocompactPayload struct {
	Version         int                      `json:"version"`
	RecentStart     int                      `json:"recent_start,omitempty"`
	SourcePositions []int                    `json:"source_positions,omitempty"`
	Items           []model.ConversationItem `json:"items,omitempty"`
}

func activeMicrocompactItems(compactions []types.ConversationCompaction) []model.ConversationItem {
	if len(compactions) == 0 {
		return nil
	}

	var active []model.ConversationItem
	var activeEnd int
	for _, compaction := range compactions {
		switch compaction.Kind {
		case types.ConversationCompactionKindMicro:
			payload, err := decodeMicrocompactPayload(compaction.SummaryPayload)
			if err != nil || len(payload.Items) == 0 {
				continue
			}
			active = cloneConversationItemsForPrompt(payload.Items)
			activeEnd = compaction.EndPosition
		case types.ConversationCompactionKindRolling, types.ConversationCompactionKindFull:
			if compaction.EndPosition >= activeEnd {
				active = nil
				activeEnd = 0
			}
		}
	}

	return active
}

func buildMicrocompactPayload(items []model.ConversationItem, positions []int, recentStart int) (persistedMicrocompactPayload, error) {
	sourcePositions := make([]int, 0, len(positions))
	for _, pos := range positions {
		if pos < 0 || pos >= len(items) {
			continue
		}
		sourcePositions = append(sourcePositions, pos+1)
	}
	sort.Ints(sourcePositions)

	payload := persistedMicrocompactPayload{
		Version:         1,
		RecentStart:     recentStart,
		SourcePositions: sourcePositions,
		Items:           buildMicrocompactCarryForwardItems(items, positions),
	}
	return payload, nil
}

func encodeMicrocompactPayload(payload persistedMicrocompactPayload) string {
	raw, err := json.Marshal(payload)
	if err != nil {
		return "{}"
	}
	return string(raw)
}

func decodeMicrocompactPayload(raw string) (persistedMicrocompactPayload, error) {
	var payload persistedMicrocompactPayload
	if strings.TrimSpace(raw) == "" {
		return payload, fmt.Errorf("empty microcompact payload")
	}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return payload, err
	}
	return payload, nil
}

func buildMicrocompactCarryForwardItems(items []model.ConversationItem, positions []int) []model.ConversationItem {
	if len(positions) == 0 {
		return nil
	}

	ordered := append([]int(nil), positions...)
	sort.Ints(ordered)

	seenCallPositions := map[int]struct{}{}
	carry := make([]model.ConversationItem, 0, len(ordered)*2+1)
	summaryLines := make([]string, 0, len(ordered))

	for _, pos := range ordered {
		if pos < 0 || pos >= len(items) {
			continue
		}
		item := items[pos]
		if item.Kind != model.ConversationItemToolResult || item.Result == nil {
			continue
		}

		if callPos := findToolCallPosition(items, pos, item.Result.ToolCallID); callPos >= 0 {
			if _, seen := seenCallPositions[callPos]; !seen {
				seenCallPositions[callPos] = struct{}{}
				carry = append(carry, cloneToolCallForPrompt(items[callPos]))
			}
		}

		carry = append(carry, compactToolResultForPrompt(item, pos+1))
		summaryLines = append(summaryLines, microcompactSummaryLine(item, pos+1))
	}

	if len(carry) == 0 {
		return nil
	}

	boundary := model.ConversationItem{
		Kind: model.ConversationItemSummary,
		Summary: &model.Summary{
			RangeLabel:   "historical compacted tool results",
			ToolOutcomes: summaryLines,
		},
	}

	return append([]model.ConversationItem{boundary}, carry...)
}

func microcompactSummaryLine(item model.ConversationItem, position int) string {
	if item.Result == nil {
		return fmt.Sprintf("conversation item %d compacted", position)
	}
	name := strings.TrimSpace(item.Result.ToolName)
	if name == "" {
		name = "tool"
	}
	return fmt.Sprintf("%s result at item %d compacted from %d bytes", name, position, len(item.Result.Content))
}

func compactToolResultForPrompt(item model.ConversationItem, position int) model.ConversationItem {
	result := model.ToolResult{}
	if item.Result != nil {
		result = *item.Result
	}
	original := result.Content
	if strings.TrimSpace(result.ToolName) == "" {
		result.ToolName = "tool"
	}

	lineCount := 0
	if original != "" {
		lineCount = strings.Count(original, "\n") + 1
	}
	preview := summarizeToolResultPreview(original)
	result.Content = fmt.Sprintf(
		"[Microcompacted historical tool result]\nTool: %s\nOriginal size: %d bytes\nApprox lines: %d\nConversation item: %d\nPreview:\n%s",
		result.ToolName,
		len(original),
		lineCount,
		position,
		preview,
	)
	result.StructuredJSON = buildMicrocompactStructuredJSON(result, len(original), lineCount, position, preview)

	return model.ConversationItem{
		Kind:   model.ConversationItemToolResult,
		Result: &result,
	}
}

func buildMicrocompactStructuredJSON(result model.ToolResult, originalBytes, lineCount, position int, preview string) string {
	payload := map[string]any{
		"microcompact":      true,
		"tool_name":         result.ToolName,
		"tool_call_id":      result.ToolCallID,
		"original_bytes":    originalBytes,
		"approx_line_count": lineCount,
		"conversation_item": position,
	}
	if result.IsError {
		payload["is_error"] = true
	}
	if preview != "" {
		payload["preview"] = preview
	}

	raw, err := json.Marshal(payload)
	if err != nil {
		return ""
	}
	return string(raw)
}

func summarizeToolResultPreview(content string) string {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return "[empty]"
	}

	lines := strings.Split(trimmed, "\n")
	if len(lines) > microcompactPreviewLines {
		lines = lines[:microcompactPreviewLines]
	}
	preview := strings.Join(lines, "\n")
	runes := []rune(preview)
	if len(runes) > microcompactPreviewChars {
		preview = string(runes[:microcompactPreviewChars]) + "..."
	}
	return preview
}

func findToolCallPosition(items []model.ConversationItem, resultPos int, toolCallID string) int {
	if toolCallID == "" {
		return -1
	}
	for i := resultPos - 1; i >= 0; i-- {
		item := items[i]
		if item.Kind != model.ConversationItemToolCall {
			continue
		}
		if item.ToolCall.ID == toolCallID {
			return i
		}
	}
	return -1
}

func cloneToolCallForPrompt(item model.ConversationItem) model.ConversationItem {
	out := model.ConversationItem{
		Kind: model.ConversationItemToolCall,
		ToolCall: model.ToolCallChunk{
			ID:         item.ToolCall.ID,
			Name:       item.ToolCall.Name,
			InputChunk: item.ToolCall.InputChunk,
			Input:      cloneJSONMapForPrompt(item.ToolCall.Input),
		},
	}
	return out
}

func cloneConversationItemsForPrompt(items []model.ConversationItem) []model.ConversationItem {
	if len(items) == 0 {
		return nil
	}
	out := make([]model.ConversationItem, len(items))
	for i, item := range items {
		out[i] = cloneConversationItemForPrompt(item)
	}
	return out
}

func cloneConversationItemForPrompt(item model.ConversationItem) model.ConversationItem {
	out := item
	if item.Summary != nil {
		summary := *item.Summary
		summary.UserGoals = append([]string(nil), summary.UserGoals...)
		summary.ImportantChoices = append([]string(nil), summary.ImportantChoices...)
		summary.FilesTouched = append([]string(nil), summary.FilesTouched...)
		summary.ToolOutcomes = append([]string(nil), summary.ToolOutcomes...)
		summary.OpenThreads = append([]string(nil), summary.OpenThreads...)
		out.Summary = &summary
	}
	if item.Result != nil {
		result := *item.Result
		out.Result = &result
	}
	out.ToolCall.Input = cloneJSONMapForPrompt(item.ToolCall.Input)
	return out
}

func cloneJSONMapForPrompt(input map[string]any) map[string]any {
	if len(input) == 0 {
		return nil
	}
	out := make(map[string]any, len(input))
	for key, value := range input {
		out[key] = cloneJSONValueForPrompt(value)
	}
	return out
}

func cloneJSONValueForPrompt(value any) any {
	switch v := value.(type) {
	case map[string]any:
		return cloneJSONMapForPrompt(v)
	case []any:
		out := make([]any, len(v))
		for i, elem := range v {
			out[i] = cloneJSONValueForPrompt(elem)
		}
		return out
	case []string:
		return append([]string(nil), v...)
	default:
		return v
	}
}
