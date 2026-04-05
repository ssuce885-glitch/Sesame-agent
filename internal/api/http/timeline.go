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
	toolIndexByCallID := map[string]int{}

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
			blocks = append(blocks, types.TimelineBlock{
				ID:   blockID,
				Kind: "user_message",
				Text: item.Text,
			})
		case model.ConversationItemAssistantText:
			blocks = append(blocks, types.TimelineBlock{
				ID:   blockID,
				Kind: "assistant_output",
				Text: item.Text,
			})
		case model.ConversationItemToolCall:
			blocks = append(blocks, types.TimelineBlock{
				ID:          item.ToolCall.ID,
				Kind:        "tool_call",
				Status:      "completed",
				ToolName:    item.ToolCall.Name,
				ArgsPreview: marshalPreviewJSON(item.ToolCall.Input),
			})
			toolIndexByCallID[item.ToolCall.ID] = len(blocks) - 1
		case model.ConversationItemToolResult:
			if item.Result == nil {
				continue
			}
			if toolIndex, ok := toolIndexByCallID[item.Result.ToolCallID]; ok {
				blocks[toolIndex].ResultPreview = clampPreview(item.Result.Content)
				continue
			}
			blocks = append(blocks, types.TimelineBlock{
				ID:            item.Result.ToolCallID,
				Kind:          "notice",
				Status:        "completed",
				ToolName:      item.Result.ToolName,
				ResultPreview: clampPreview(item.Result.Content),
			})
		default:
			continue
		}
	}

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
