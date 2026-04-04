package engine

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"go-agent/internal/memory"
	"go-agent/internal/model"
	"go-agent/internal/tools"
	"go-agent/internal/types"
)

func runLoop(ctx context.Context, e *Engine, in Input) error {
	if e == nil {
		return errors.New("engine is required")
	}
	if e.model == nil {
		return errors.New("model client is required")
	}
	if e.registry == nil {
		return errors.New("tool registry is required")
	}
	if e.permission == nil {
		return errors.New("permission engine is required")
	}
	if in.Sink == nil {
		return errors.New("event sink is required")
	}

	emit := func(eventType string, payload any) error {
		event, err := types.NewEvent(in.Session.ID, in.Turn.ID, eventType, payload)
		if err != nil {
			return err
		}
		return in.Sink.Emit(ctx, event)
	}
	emitFailed := func(message string) error {
		return emit(types.EventTurnFailed, types.TurnFailedPayload{Message: message})
	}

	if err := emit(types.EventTurnStarted, types.TurnStartedPayload{
		WorkspaceRoot: in.Session.WorkspaceRoot,
	}); err != nil {
		return err
	}

	sessionID := in.Turn.SessionID
	if sessionID == "" {
		sessionID = in.Session.ID
	}

	totalItems, items, summaries, memoryRefs, err := loadConversationState(ctx, e, in, sessionID)
	if err != nil {
		if emitErr := emitFailed(err.Error()); emitErr != nil {
			return errors.Join(err, emitErr)
		}
		return err
	}

	req := buildRequest(e, in, items, summaries, memoryRefs)
	assistantStarted := false
	assistantText := strings.Builder{}
	nextPosition := totalItems + 1
	toolSteps := 0

	for {
		stream, errs := e.model.Stream(ctx, req)
		messageEnded := false
		var toolCalls []model.ToolCallChunk

		for event := range stream {
			switch event.Kind {
			case model.StreamEventTextDelta:
				if !assistantStarted {
					if err := emit(types.EventAssistantStarted, struct{}{}); err != nil {
						return err
					}
					assistantStarted = true
				}
				assistantText.WriteString(event.TextDelta)
				if err := emit(types.EventAssistantDelta, types.AssistantDeltaPayload{Text: event.TextDelta}); err != nil {
					return err
				}
			case model.StreamEventToolCallStart, model.StreamEventToolCallDelta:
				continue
			case model.StreamEventToolCallEnd:
				toolCalls = append(toolCalls, event.ToolCall)
			case model.StreamEventMessageEnd:
				messageEnded = true
			case model.StreamEventUsage:
				continue
			default:
				err := fmt.Errorf("unsupported stream event kind: %s", event.Kind)
				if emitErr := emitFailed(err.Error()); emitErr != nil {
					return errors.Join(err, emitErr)
				}
				return err
			}
		}

		if errs != nil {
			err := <-errs
			if err != nil {
				if emitErr := emitFailed(err.Error()); emitErr != nil {
					return errors.Join(err, emitErr)
				}
				return err
			}
		}

		if len(toolCalls) == 0 {
			if messageEnded {
				if err := persistAssistantText(ctx, e.store, sessionID, in.Turn.ID, nextPosition, assistantText.String()); err != nil {
					if emitErr := emitFailed(err.Error()); emitErr != nil {
						return errors.Join(err, emitErr)
					}
					return err
				}
				if err := emit(types.EventAssistantCompleted, struct{}{}); err != nil {
					return err
				}
				if err := emit(types.EventTurnCompleted, struct{}{}); err != nil {
					return err
				}
			}
			return nil
		}

		if e.maxToolSteps > 0 && toolSteps+len(toolCalls) > e.maxToolSteps {
			err := fmt.Errorf("tool step limit exceeded")
			if emitErr := emitFailed(err.Error()); emitErr != nil {
				return errors.Join(err, emitErr)
			}
			return err
		}

		for _, call := range toolCalls {
			toolSteps++
			payload := struct {
				ToolCallID string `json:"tool_call_id"`
				ToolName   string `json:"tool_name"`
			}{
				ToolCallID: call.ID,
				ToolName:   call.Name,
			}
			if err := emit(types.EventToolStarted, payload); err != nil {
				return err
			}

			result, err := e.registry.Execute(ctx, tools.Call{
				Name:  call.Name,
				Input: call.Input,
			}, tools.ExecContext{
				WorkspaceRoot:    in.Session.WorkspaceRoot,
				PermissionEngine: e.permission,
			})
			if err != nil {
				if emitErr := emitFailed(err.Error()); emitErr != nil {
					return errors.Join(err, emitErr)
				}
				return err
			}

			if err := emit(types.EventToolCompleted, payload); err != nil {
				return err
			}

			toolResult := model.ToolResult{
				ToolCallID: call.ID,
				ToolName:   call.Name,
				Content:    result.Text,
				IsError:    false,
			}
			if err := persistToolResult(ctx, e.store, sessionID, in.Turn.ID, nextPosition, toolResult); err != nil {
				if emitErr := emitFailed(err.Error()); emitErr != nil {
					return errors.Join(err, emitErr)
				}
				return err
			}
			nextPosition++

			req.ToolResults = append(req.ToolResults, toolResult)
		}
	}
}

func loadConversationState(ctx context.Context, e *Engine, in Input, sessionID string) (int, []model.ConversationItem, []model.Summary, []string, error) {
	if e.store == nil || e.ctxManager == nil {
		return 0, nil, nil, nil, nil
	}

	items, err := e.store.ListConversationItems(ctx, sessionID)
	if err != nil {
		return 0, nil, nil, nil, err
	}
	totalItems := len(items)

	summaries, err := e.store.ListConversationSummaries(ctx, sessionID)
	if err != nil {
		return 0, nil, nil, nil, err
	}

	entries, err := e.store.ListMemoryEntriesByWorkspace(ctx, in.Session.WorkspaceRoot)
	if err != nil {
		return 0, nil, nil, nil, err
	}

	recalled := memory.Recall(in.Turn.UserMessage, entries, 3)
	memoryRefs := make([]string, 0, len(recalled))
	for _, entry := range recalled {
		memoryRefs = append(memoryRefs, entry.Content)
	}

	working := e.ctxManager.Build(in.Turn.UserMessage, items, summaries, memoryRefs)
	if working.NeedsCompact && e.compactor != nil {
		summary, err := e.compactor.Compact(ctx, items[:working.CompactionStart])
		if err != nil {
			return 0, nil, nil, nil, err
		}
		if err := e.store.InsertConversationSummary(ctx, sessionID, working.CompactionStart, summary); err != nil {
			return 0, nil, nil, nil, err
		}
		summaries = append(summaries, summary)
		working = e.ctxManager.Build(in.Turn.UserMessage, items, summaries, memoryRefs)
	}

	return totalItems, working.RecentItems, working.Summaries, working.MemoryRefs, nil
}

func buildRequest(e *Engine, in Input, items []model.ConversationItem, summaries []model.Summary, memoryRefs []string) model.Request {
	reqItems := append([]model.ConversationItem(nil), items...)
	for _, summary := range summaries {
		summary := summary
		reqItems = append(reqItems, model.ConversationItem{
			Kind:    model.ConversationItemSummary,
			Summary: &summary,
		})
	}
	reqItems = append(reqItems, model.UserMessageItem(in.Turn.UserMessage))

	return model.Request{
		UserMessage:  in.Turn.UserMessage,
		Instructions: buildRuntimeInstructions(in.Session.WorkspaceRoot, memoryRefs),
		Stream:       true,
		Items:        reqItems,
		Tools:        buildToolSchemas(e.registry),
		ToolChoice:   "auto",
	}
}

func buildRuntimeInstructions(workspaceRoot string, memoryRefs []string) string {
	base := fmt.Sprintf("workspace_root=%s\nUse local tools when needed.", workspaceRoot)
	if len(memoryRefs) == 0 {
		return base
	}
	return base + "\nRelevant memory:\n- " + strings.Join(memoryRefs, "\n- ")
}

func buildToolSchemas(registry *tools.Registry) []model.ToolSchema {
	if registry == nil {
		return nil
	}

	defs := registry.Definitions()
	if len(defs) == 0 {
		return nil
	}

	schemas := make([]model.ToolSchema, 0, len(defs))
	for _, def := range defs {
		schemas = append(schemas, model.ToolSchema{
			Name:        def.Name,
			Description: def.Description,
			InputSchema: def.InputSchema,
		})
	}
	return schemas
}

func persistAssistantText(ctx context.Context, store ConversationStore, sessionID, turnID string, position int, text string) error {
	if store == nil || strings.TrimSpace(text) == "" {
		return nil
	}

	return store.InsertConversationItem(ctx, sessionID, turnID, position, model.ConversationItem{
		Kind: model.ConversationItemAssistantText,
		Text: text,
	})
}

func persistToolResult(ctx context.Context, store ConversationStore, sessionID, turnID string, position int, result model.ToolResult) error {
	if store == nil {
		return nil
	}
	return store.InsertConversationItem(ctx, sessionID, turnID, position, model.ToolResultItem(result))
}
