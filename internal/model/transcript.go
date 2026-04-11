package model

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

func ValidateConversationItems(items []ConversationItem) error {
	if len(items) == 0 {
		return nil
	}

	pending := make(map[string]struct{})
	for idx, item := range items {
		position := idx + 1
		switch item.Kind {
		case ConversationItemToolCall:
			id := strings.TrimSpace(item.ToolCall.ID)
			if id == "" {
				return fmt.Errorf("tool call at item %d is missing an id", position)
			}
			pending[id] = struct{}{}
		case ConversationItemToolResult:
			if item.Result == nil {
				return fmt.Errorf("tool result at item %d is missing payload", position)
			}
			id := strings.TrimSpace(item.Result.ToolCallID)
			if id == "" {
				return fmt.Errorf("tool result at item %d is missing tool_call_id", position)
			}
			if _, ok := pending[id]; !ok {
				return fmt.Errorf("tool result at item %d does not follow an open tool call (%s)", position, id)
			}
			delete(pending, id)
		case ConversationItemUserMessage:
			if len(pending) > 0 {
				return fmt.Errorf("user message at item %d appears before pending tool results resolve (%s)", position, formatPendingToolIDs(pending))
			}
		case ConversationItemAssistantText, ConversationItemAssistantThinking, ConversationItemSummary:
			if len(pending) > 0 {
				return fmt.Errorf("assistant content at item %d splits an open tool exchange (%s)", position, formatPendingToolIDs(pending))
			}
		}
	}

	if len(pending) > 0 {
		return fmt.Errorf("conversation ends with unresolved tool calls (%s)", formatPendingToolIDs(pending))
	}
	return nil
}

func NearestSafeConversationBoundary(items []ConversationItem, preferred int) int {
	if preferred <= 0 || len(items) == 0 {
		return 0
	}
	if preferred > len(items) {
		preferred = len(items)
	}

	safe := 0
	pending := make(map[string]struct{})
	for idx := 0; idx < preferred; idx++ {
		item := items[idx]
		switch item.Kind {
		case ConversationItemToolCall:
			id := strings.TrimSpace(item.ToolCall.ID)
			if id == "" {
				return safe
			}
			pending[id] = struct{}{}
		case ConversationItemToolResult:
			if item.Result == nil {
				return safe
			}
			id := strings.TrimSpace(item.Result.ToolCallID)
			if id == "" {
				return safe
			}
			if _, ok := pending[id]; !ok {
				return safe
			}
			delete(pending, id)
		case ConversationItemUserMessage, ConversationItemAssistantText, ConversationItemAssistantThinking, ConversationItemSummary:
			if len(pending) > 0 {
				return safe
			}
		}
		if len(pending) == 0 {
			safe = idx + 1
		}
	}
	return safe
}

func immediateStreamError(ctx context.Context, err error) (<-chan StreamEvent, <-chan error) {
	events := make(chan StreamEvent)
	errs := make(chan error, 1)
	close(events)
	select {
	case <-ctx.Done():
		errs <- ctx.Err()
	default:
		errs <- err
	}
	close(errs)
	return events, errs
}

func formatPendingToolIDs(pending map[string]struct{}) string {
	if len(pending) == 0 {
		return ""
	}
	ids := make([]string, 0, len(pending))
	for id := range pending {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return strings.Join(ids, ", ")
}
