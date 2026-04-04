package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	contextstate "go-agent/internal/context"
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
	caps := e.model.Capabilities()
	providerName := providerCacheOwnerForCapabilities(caps)
	usageProvider := strings.TrimSpace(e.meta.Provider)
	if usageProvider == "" {
		usageProvider = providerName
	}
	usageModel := strings.TrimSpace(e.meta.Model)

	if err := emit(types.EventTurnStarted, types.TurnStartedPayload{
		WorkspaceRoot: in.Session.WorkspaceRoot,
	}); err != nil {
		return err
	}

	sessionID := in.Turn.SessionID
	if sessionID == "" {
		sessionID = in.Session.ID
	}

	totalItems, working, err := loadConversationState(ctx, e, in, sessionID)
	if err != nil {
		if emitErr := emitFailed(err.Error()); emitErr != nil {
			return errors.Join(err, emitErr)
		}
		return err
	}

	cacheHeadValue, cacheHeadOK, err := loadProviderCacheHead(ctx, e.store, sessionID, providerName, string(caps.Profile))
	if err != nil {
		if emitErr := emitFailed(err.Error()); emitErr != nil {
			return errors.Join(err, emitErr)
		}
		return err
	}
	var cacheHead *types.ProviderCacheHead
	if cacheHeadOK {
		cacheHead = &cacheHeadValue
	}

	req := e.runtime.PrepareRequest(
		working,
		cacheHead,
		caps,
		model.UserMessageItem(in.Turn.UserMessage),
		buildRuntimeInstructions(in.Session.WorkspaceRoot, working.MemoryRefs),
	)
	req.Stream = true
	req.Tools = buildToolSchemas(e.registry)
	req.ToolChoice = "auto"
	nativeContinuation := req.Cache != nil && caps.Profile != model.CapabilityProfileNone
	assistantStarted := false
	nextPosition := totalItems + 1
	toolSteps := 0
	totalInputTokens := 0
	totalOutputTokens := 0
	totalCachedTokens := 0
	hasUsage := false

	userItem := model.UserMessageItem(in.Turn.UserMessage)
	if err := persistConversationItem(ctx, e.store, sessionID, in.Turn.ID, nextPosition, userItem); err != nil {
		if emitErr := emitFailed(err.Error()); emitErr != nil {
			return errors.Join(err, emitErr)
		}
		return err
	}
	nextPosition++

	for {
		stream, errs := e.model.Stream(ctx, req)
		messageEnded := false
		var responseMeta *model.ResponseMetadata
		var toolCalls []model.ToolCallChunk
		assistantText := strings.Builder{}

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
			case model.StreamEventResponseMetadata:
				responseMeta = event.ResponseMetadata
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

		if responseMeta != nil && req.Cache != nil && caps.Profile != model.CapabilityProfileNone {
			cacheMode := req.Cache.Mode
			if caps.SupportsSessionCache {
				cacheMode = model.CacheModeSession
			}
			nextHead, ok := nextHeadFromResponse(e, sessionID, providerName, caps, cacheHead, req.Cache, responseMeta)
			if ok {
				if err := persistProviderCacheHead(ctx, e, nextHead); err != nil {
					if emitErr := emitFailed(err.Error()); emitErr != nil {
						return errors.Join(err, emitErr)
					}
					return err
				}
				cacheHead = nextHead
				cacheHeadOK = true
				if e.runtime != nil {
					req.Cache = e.runtime.CacheDirectiveForHead(cacheHead, caps, cacheMode)
				}
				req.Items = nil
				req.ToolResults = nil
				req.UserMessage = ""
			}
		}

		if responseMeta != nil {
			totalInputTokens += responseMeta.InputTokens
			totalOutputTokens += responseMeta.OutputTokens
			totalCachedTokens += responseMeta.CachedTokens
			hasUsage = true
		}

		if assistantText.Len() > 0 {
			assistantItem := model.ConversationItem{
				Kind: model.ConversationItemAssistantText,
				Text: assistantText.String(),
			}
			if err := persistConversationItem(ctx, e.store, sessionID, in.Turn.ID, nextPosition, assistantItem); err != nil {
				if emitErr := emitFailed(err.Error()); emitErr != nil {
					return errors.Join(err, emitErr)
				}
				return err
			}
			if !nativeContinuation {
				req.Items = append(req.Items, assistantItem)
			}
			nextPosition++
		}

		if len(toolCalls) == 0 {
			if messageEnded {
				usage := buildTurnUsage(
					hasUsage,
					in.Turn.ID,
					sessionID,
					usageProvider,
					usageModel,
					totalInputTokens,
					totalOutputTokens,
					totalCachedTokens,
				)
				if err := finalizeTurn(ctx, e, in, usage); err != nil {
					return err
				}
			}
			return nil
		}

		for _, call := range toolCalls {
			toolSteps++
			if e.maxToolSteps > 0 && toolSteps > e.maxToolSteps {
				err := fmt.Errorf("turn exceeded max tool steps (%d)", e.maxToolSteps)
				if emitErr := emitFailed(err.Error()); emitErr != nil {
					return errors.Join(err, emitErr)
				}
				return err
			}

			toolCallItem := model.ConversationItem{
				Kind: model.ConversationItemToolCall,
				ToolCall: model.ToolCallChunk{
					ID:    call.ID,
					Name:  call.Name,
					Input: call.Input,
				},
			}
			if err := persistConversationItem(ctx, e.store, sessionID, in.Turn.ID, nextPosition, toolCallItem); err != nil {
				if emitErr := emitFailed(err.Error()); emitErr != nil {
					return errors.Join(err, emitErr)
				}
				return err
			}
			req.Items = append(req.Items, toolCallItem)
			nextPosition++

			payload := types.ToolEventPayload{
				ToolCallID: call.ID,
				ToolName:   call.Name,
				Arguments:  marshalToolArguments(call.Input),
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

			payload.ResultPreview = previewToolResult(result.Text)
			if err := emit(types.EventToolCompleted, payload); err != nil {
				return err
			}

			toolResult := model.ToolResult{
				ToolCallID: call.ID,
				ToolName:   call.Name,
				Content:    result.Text,
				IsError:    false,
			}
			toolResultItem := model.ToolResultItem(toolResult)
			if err := persistConversationItem(ctx, e.store, sessionID, in.Turn.ID, nextPosition, toolResultItem); err != nil {
				if emitErr := emitFailed(err.Error()); emitErr != nil {
					return errors.Join(err, emitErr)
				}
				return err
			}
			req.Items = append(req.Items, toolResultItem)
			nextPosition++

			req.ToolResults = append(req.ToolResults, toolResult)
		}
	}
}

func loadConversationState(ctx context.Context, e *Engine, in Input, sessionID string) (int, contextstate.WorkingSet, error) {
	if e.store == nil || e.ctxManager == nil {
		return 0, contextstate.WorkingSet{}, nil
	}

	items, err := e.store.ListConversationItems(ctx, sessionID)
	if err != nil {
		return 0, contextstate.WorkingSet{}, err
	}
	totalItems := len(items)

	summaries, err := e.store.ListConversationSummaries(ctx, sessionID)
	if err != nil {
		return 0, contextstate.WorkingSet{}, err
	}

	entries, err := e.store.ListMemoryEntriesByWorkspace(ctx, in.Session.WorkspaceRoot)
	if err != nil {
		return 0, contextstate.WorkingSet{}, err
	}

	recalled := memory.Recall(in.Turn.UserMessage, entries, 3)
	memoryRefs := make([]string, 0, len(recalled))
	for _, entry := range recalled {
		memoryRefs = append(memoryRefs, entry.Content)
	}

	working := e.ctxManager.Build(in.Turn.UserMessage, items, summaries, memoryRefs)
	if working.Action.Kind == contextstate.CompactionActionRolling && e.compactor != nil {
		cutoff := working.CompactionStart
		if cutoff < 0 {
			cutoff = 0
		}
		if cutoff > len(items) {
			cutoff = len(items)
		}
		summary, err := e.compactor.Compact(ctx, items[:cutoff])
		if err != nil {
			return 0, contextstate.WorkingSet{}, err
		}
		if err := e.store.InsertConversationSummary(ctx, sessionID, cutoff, summary); err != nil {
			return 0, contextstate.WorkingSet{}, err
		}
		summaries = append(summaries, summary)
		working = e.ctxManager.Build(in.Turn.UserMessage, items, summaries, memoryRefs)
	}

	return totalItems, working, nil
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

func persistConversationItem(ctx context.Context, store ConversationStore, sessionID, turnID string, position int, item model.ConversationItem) error {
	if store == nil {
		return nil
	}
	if item.Kind == model.ConversationItemAssistantText && strings.TrimSpace(item.Text) == "" {
		return nil
	}
	return store.InsertConversationItem(ctx, sessionID, turnID, position, item)
}

func persistTurnUsage(ctx context.Context, store ConversationStore, usage types.TurnUsage) error {
	if store == nil {
		return nil
	}
	return store.UpsertTurnUsage(ctx, usage)
}

func buildTurnUsage(hasUsage bool, turnID, sessionID, provider, model string, inputTokens, outputTokens, cachedTokens int) *types.TurnUsage {
	if !hasUsage {
		return nil
	}
	cacheHitRate := 0.0
	if inputTokens > 0 {
		cacheHitRate = float64(cachedTokens) / float64(inputTokens)
	}
	now := time.Now().UTC()
	return &types.TurnUsage{
		TurnID:       turnID,
		SessionID:    sessionID,
		Provider:     provider,
		Model:        model,
		InputTokens:  inputTokens,
		OutputTokens: outputTokens,
		CachedTokens: cachedTokens,
		CacheHitRate: cacheHitRate,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
}

func finalizeTurn(ctx context.Context, e *Engine, in Input, usage *types.TurnUsage) error {
	finalEvents := make([]types.Event, 0, 3)

	assistantCompleted, err := types.NewEvent(in.Session.ID, in.Turn.ID, types.EventAssistantCompleted, struct{}{})
	if err != nil {
		return err
	}
	finalEvents = append(finalEvents, assistantCompleted)

	if usage != nil {
		usageEvent, err := types.NewEvent(in.Session.ID, in.Turn.ID, types.EventTurnUsage, types.TurnUsagePayload{
			Provider:     usage.Provider,
			Model:        usage.Model,
			InputTokens:  usage.InputTokens,
			OutputTokens: usage.OutputTokens,
			CachedTokens: usage.CachedTokens,
			CacheHitRate: usage.CacheHitRate,
		})
		if err != nil {
			return err
		}
		finalEvents = append(finalEvents, usageEvent)
	}

	turnCompleted, err := types.NewEvent(in.Session.ID, in.Turn.ID, types.EventTurnCompleted, struct{}{})
	if err != nil {
		return err
	}
	finalEvents = append(finalEvents, turnCompleted)

	if sink, ok := in.Sink.(TurnFinalizingSink); ok {
		return sink.FinalizeTurn(ctx, usage, finalEvents)
	}

	if usage != nil {
		if err := persistTurnUsage(ctx, e.store, *usage); err != nil {
			return err
		}
	}
	for _, event := range finalEvents {
		if err := in.Sink.Emit(ctx, event); err != nil {
			return err
		}
	}
	return nil
}

func marshalToolArguments(input map[string]any) string {
	if len(input) == 0 {
		return ""
	}
	raw, err := json.Marshal(input)
	if err != nil {
		return ""
	}
	return string(raw)
}

func previewToolResult(result string) string {
	const maxLen = 200
	if len(result) <= maxLen {
		return result
	}
	return result[:maxLen] + "..."
}

func providerCacheOwnerForCapabilities(caps model.ProviderCapabilities) string {
	if caps.Profile == model.CapabilityProfileArkResponses {
		return "openai_compatible"
	}
	return ""
}

func loadProviderCacheHead(ctx context.Context, store ConversationStore, sessionID, provider, capabilityProfile string) (types.ProviderCacheHead, bool, error) {
	if store == nil || provider == "" {
		return types.ProviderCacheHead{}, false, nil
	}
	return store.GetProviderCacheHead(ctx, sessionID, provider, capabilityProfile)
}

func persistProviderCacheHead(ctx context.Context, e *Engine, head *types.ProviderCacheHead) error {
	if e == nil || e.store == nil || head == nil {
		return nil
	}

	return e.store.UpsertProviderCacheHead(ctx, *head)
}

func nextHeadFromResponse(e *Engine, sessionID, provider string, caps model.ProviderCapabilities, head *types.ProviderCacheHead, used *model.CacheDirective, meta *model.ResponseMetadata) (*types.ProviderCacheHead, bool) {
	if e == nil || e.runtime == nil || provider == "" || used == nil || meta == nil || meta.ResponseID == "" || caps.Profile == model.CapabilityProfileNone {
		return head, head != nil
	}

	nextHead := e.runtime.NextCacheHead(head, caps, used, meta)
	if nextHead == nil {
		return head, head != nil
	}
	if nextHead.SessionID == "" {
		nextHead.SessionID = sessionID
	}
	if nextHead.Provider == "" {
		nextHead.Provider = provider
	}
	if nextHead.CapabilityProfile == "" {
		nextHead.CapabilityProfile = string(caps.Profile)
	}
	return nextHead, true
}
