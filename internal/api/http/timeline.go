package httpapi

import (
	"encoding/json"
	"net/http"
	"strconv"

	"go-agent/internal/model"
	"go-agent/internal/types"
)

func handleGetTimeline(deps Dependencies, sessionID string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if deps.Store == nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}

		items, err := deps.Store.ListConversationItems(r.Context(), sessionID)
		if err != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		events, err := deps.Store.ListSessionEvents(r.Context(), sessionID, 0)
		if err != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		latestSeq, err := deps.Store.LatestSessionEventSeq(r.Context(), sessionID)
		if err != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}

		blocks := normalizeTimelineBlocks(items, events)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(types.SessionTimelineResponse{
			Blocks:    blocks,
			LatestSeq: latestSeq,
		})
	}
}

func normalizeTimelineBlocks(items []model.ConversationItem, events []types.Event) []types.TimelineBlock {
	blocks := make([]types.TimelineBlock, 0, len(items))
	var currentAssistant *types.TimelineBlock

	flushAssistant := func() {
		if currentAssistant == nil {
			return
		}
		blocks = append(blocks, *currentAssistant)
		currentAssistant = nil
	}

	ensureAssistant := func(id string) *types.TimelineBlock {
		if currentAssistant != nil {
			return currentAssistant
		}
		currentAssistant = &types.TimelineBlock{
			ID:     id,
			Kind:   "assistant_message",
			Status: "completed",
		}
		return currentAssistant
	}

	for idx, item := range items {
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
			blocks = append(blocks, types.TimelineBlock{
				ID:   blockID,
				Kind: "user_message",
				Text: item.Text,
			})
		case model.ConversationItemAssistantText:
			assistant := ensureAssistant(blockID)
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
			assistant := ensureAssistant(item.ToolCall.ID)
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
				content.Status = "completed"
			}
		default:
			continue
		}
	}
	flushAssistant()

	for _, event := range events {
		if event.Type != types.EventSystemNotice {
			continue
		}
		var p types.NoticePayload
		if err := json.Unmarshal(event.Payload, &p); err != nil {
			continue
		}
		blocks = append(blocks, types.TimelineBlock{
			ID:   "notice_" + strconv.FormatInt(event.Seq, 10),
			Kind: "notice",
			Text: p.Text,
		})
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
