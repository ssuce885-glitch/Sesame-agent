package httpapi

import (
	"encoding/json"
	"strconv"
	"strings"

	"go-agent/internal/model"
	"go-agent/internal/types"
)

func normalizeTimelineBlocks(items []types.ConversationTimelineItem, events []types.Event) []types.TimelineBlock {
	blocks := make([]types.TimelineBlock, 0, len(items))
	var currentAssistant *types.TimelineBlock

	flushAssistant := func() {
		if currentAssistant == nil {
			return
		}
		blocks = append(blocks, *currentAssistant)
		currentAssistant = nil
	}

	ensureAssistant := func(id string, turnID string) *types.TimelineBlock {
		if currentAssistant != nil {
			if currentAssistant.TurnID != "" && turnID != "" && currentAssistant.TurnID != turnID {
				flushAssistant()
			}
		}
		if currentAssistant != nil {
			if currentAssistant.TurnID == "" && turnID != "" {
				currentAssistant.TurnID = turnID
			}
			return currentAssistant
		}
		currentAssistant = &types.TimelineBlock{
			ID:     id,
			TurnID: turnID,
			Kind:   "assistant_message",
			Status: "completed",
		}
		return currentAssistant
	}

	for idx, entry := range items {
		item := entry.Item
		blockID := item.ToolCall.ID
		if blockID == "" && item.Result != nil {
			blockID = item.Result.ToolCallID
		}
		if blockID == "" {
			blockID = "item_" + strconv.Itoa(idx+1)
		}

		switch item.Kind {
		case model.ConversationItemUserMessage:
			flushAssistant()
			block := types.TimelineBlock{
				ID:     blockID,
				TurnID: entry.TurnID,
				Kind:   "user_message",
				Text:   item.Text,
			}
			if len(item.Parts) > 0 {
				block.Content = timelineContentFromParts(item.Parts)
				if block.Text == "" {
					block.Text = timelineTextFromParts(item.Parts)
				}
			}
			blocks = append(blocks, block)
		case model.ConversationItemAssistantText:
			assistant := ensureAssistant(blockID, entry.TurnID)
			lastIndex := len(assistant.Content) - 1
			if lastIndex >= 0 && assistant.Content[lastIndex].Type == "text" {
				assistant.Content[lastIndex].Text += item.Text
				continue
			}
			assistant.Content = append(assistant.Content, types.TimelineContentBlock{
				Type: "text",
				Text: item.Text,
			})
		case model.ConversationItemToolCall:
			assistant := ensureAssistant(item.ToolCall.ID, entry.TurnID)
			assistant.Content = append(assistant.Content, types.TimelineContentBlock{
				Type:        "tool_call",
				ToolCallID:  item.ToolCall.ID,
				ToolName:    item.ToolCall.Name,
				ArgsPreview: marshalPreviewJSON(item.ToolCall.Input),
				Status:      "completed",
			})
		case model.ConversationItemToolResult:
			if item.Result == nil {
				continue
			}
			if content := findToolCallContent(currentAssistant, blocks, item.Result.ToolCallID); content != nil {
				content.ResultPreview = clampPreview(item.Result.Content)
				if item.Result.IsError {
					content.Status = "failed"
				} else {
					content.Status = "completed"
				}
			}
		}
	}
	flushAssistant()

	for _, event := range events {
		switch event.Type {
		case types.EventSystemNotice:
			var payload types.NoticePayload
			if err := json.Unmarshal(event.Payload, &payload); err != nil {
				continue
			}
			blocks = append(blocks, types.TimelineBlock{
				ID:     "notice_" + strconv.FormatInt(event.Seq, 10),
				TurnID: event.TurnID,
				Kind:   "notice",
				Text:   payload.Text,
			})
		case types.EventContextHeadSummaryFailed:
			var payload types.ContextHeadSummaryEventPayload
			if err := json.Unmarshal(event.Payload, &payload); err != nil {
				continue
			}
			text := "Context head summary refresh failed."
			if payload.Message != "" {
				text = "Context head summary refresh failed: " + payload.Message
			}
			blocks = append(blocks, types.TimelineBlock{
				ID:     "context_head_summary_failed_" + strconv.FormatInt(event.Seq, 10),
				TurnID: event.TurnID,
				Kind:   "notice",
				Text:   text,
			})
		case types.EventTaskResultReady:
			var block types.TimelineBlock
			if err := json.Unmarshal(event.Payload, &block); err != nil {
				continue
			}
			blocks = mergeRuntimeTimelineBlocks(blocks, []types.TimelineBlock{block})
		}
	}

	return blocks
}

func findToolCallContent(currentAssistant *types.TimelineBlock, blocks []types.TimelineBlock, toolCallID string) *types.TimelineContentBlock {
	if currentAssistant != nil {
		if content := findToolCallContentInMessage(currentAssistant.Content, toolCallID); content != nil {
			return content
		}
	}
	for index := len(blocks) - 1; index >= 0; index-- {
		if blocks[index].Kind != "assistant_message" {
			continue
		}
		if content := findToolCallContentInMessage(blocks[index].Content, toolCallID); content != nil {
			return content
		}
	}
	return nil
}

func findToolCallContentInMessage(content []types.TimelineContentBlock, toolCallID string) *types.TimelineContentBlock {
	for index := len(content) - 1; index >= 0; index-- {
		if content[index].Type == "tool_call" && content[index].ToolCallID == toolCallID {
			return &content[index]
		}
	}
	return nil
}

func marshalPreviewJSON(value any) string {
	if value == nil {
		return ""
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	return string(raw)
}

func buildRuntimeTimelineBlocks(graph types.RuntimeGraph) []types.TimelineBlock {
	blocks := make([]types.TimelineBlock, 0, len(graph.Plans)+len(graph.Tasks)+len(graph.ToolRuns)+len(graph.Worktrees))
	for _, plan := range graph.Plans {
		blocks = append(blocks, types.TimelineBlockFromPlan(plan))
	}
	for _, task := range graph.Tasks {
		blocks = append(blocks, types.TimelineBlockFromTask(task))
	}
	for _, toolRun := range graph.ToolRuns {
		blocks = append(blocks, types.TimelineBlockFromToolRun(toolRun))
	}
	for _, worktree := range graph.Worktrees {
		blocks = append(blocks, types.TimelineBlockFromWorktree(worktree))
	}
	return blocks
}

func mergeRuntimeTimelineBlocks(base []types.TimelineBlock, runtimeBlocks []types.TimelineBlock) []types.TimelineBlock {
	if len(runtimeBlocks) == 0 {
		return base
	}
	merged := append([]types.TimelineBlock(nil), base...)
	indexByID := make(map[string]int, len(merged))
	for index, block := range merged {
		if strings.TrimSpace(block.ID) == "" {
			continue
		}
		indexByID[block.ID] = index
	}
	for _, block := range runtimeBlocks {
		if index, ok := indexByID[block.ID]; ok {
			merged[index] = mergeTimelineBlock(merged[index], block)
			continue
		}
		indexByID[block.ID] = len(merged)
		merged = append(merged, block)
	}
	return merged
}

func mergeTimelineBlock(current types.TimelineBlock, update types.TimelineBlock) types.TimelineBlock {
	if current.ID == "" {
		return update
	}
	merged := current
	if update.RunID != "" {
		merged.RunID = update.RunID
	}
	if update.TurnID != "" {
		merged.TurnID = update.TurnID
	}
	if update.Kind != "" {
		merged.Kind = update.Kind
	}
	if update.Status != "" {
		merged.Status = update.Status
	}
	if update.Title != "" {
		merged.Title = update.Title
	}
	if update.Text != "" {
		merged.Text = update.Text
	}
	if update.ToolCallID != "" {
		merged.ToolCallID = update.ToolCallID
	}
	if update.ToolRunID != "" {
		merged.ToolRunID = update.ToolRunID
	}
	if update.ToolName != "" {
		merged.ToolName = update.ToolName
	}
	if update.TaskID != "" {
		merged.TaskID = update.TaskID
	}
	if update.PlanID != "" {
		merged.PlanID = update.PlanID
	}
	if update.WorktreeID != "" {
		merged.WorktreeID = update.WorktreeID
	}
	if update.Reason != "" {
		merged.Reason = update.Reason
	}
	if update.Path != "" {
		merged.Path = update.Path
	}
	if update.ArgsPreview != "" {
		merged.ArgsPreview = update.ArgsPreview
	}
	if update.ResultPreview != "" {
		merged.ResultPreview = update.ResultPreview
	}
	if len(update.Content) > 0 {
		merged.Content = update.Content
	}
	if update.Usage != nil {
		merged.Usage = update.Usage
	}
	return merged
}

func timelineContentFromParts(parts []model.ContentPart) []types.TimelineContentBlock {
	out := make([]types.TimelineContentBlock, 0, len(parts))
	for _, part := range parts {
		switch part.Type {
		case model.ContentPartText:
			if strings.TrimSpace(part.Text) == "" {
				continue
			}
			out = append(out, types.TimelineContentBlock{Type: "text", Text: part.Text})
		case model.ContentPartImage:
			out = append(out, types.TimelineContentBlock{
				Type:      "image",
				Path:      part.Path,
				URL:       "",
				MimeType:  part.MimeType,
				Width:     part.Width,
				Height:    part.Height,
				SizeBytes: part.SizeBytes,
			})
		}
	}
	return out
}

func timelineTextFromParts(parts []model.ContentPart) string {
	for _, part := range parts {
		if part.Type == model.ContentPartText && strings.TrimSpace(part.Text) != "" {
			return part.Text
		}
		if part.Type == model.ContentPartImage && strings.TrimSpace(part.Path) != "" {
			return "Image attached: " + part.Path
		}
	}
	return ""
}
