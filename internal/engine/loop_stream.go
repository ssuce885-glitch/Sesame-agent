package engine

import (
	"context"
	"fmt"
	"strings"

	"go-agent/internal/instructions"
	"go-agent/internal/model"
	"go-agent/internal/skills"
	"go-agent/internal/tools"
	"go-agent/internal/types"
)

type loopUsageTotals struct {
	inputTokens  int
	outputTokens int
	cachedTokens int
	hasUsage     bool
}

func executePreparedLoop(ctx context.Context, e *Engine, in Input, emitter loopEmitter, state *preparedLoopState) error {
	assistantStarted := false
	toolSteps := 0
	usageTotals := loopUsageTotals{}

	for {
		stream, errs := e.model.Stream(ctx, state.req)
		messageEnded := false
		var responseMeta *model.ResponseMetadata
		var toolCalls []model.ToolCallChunk
		orderedAssistantItems := make([]model.ConversationItem, 0, 4)

		for event := range stream {
			switch event.Kind {
			case model.StreamEventThinkingDelta:
				if strings.TrimSpace(event.TextDelta) == "" {
					continue
				}
				lastIndex := len(orderedAssistantItems) - 1
				if lastIndex >= 0 && orderedAssistantItems[lastIndex].Kind == model.ConversationItemAssistantThinking {
					orderedAssistantItems[lastIndex].Text += event.TextDelta
				} else {
					orderedAssistantItems = append(orderedAssistantItems, model.AssistantThinkingItem(event.TextDelta))
				}
			case model.StreamEventTextDelta:
				if !assistantStarted {
					if err := emitter.Emit(ctx, types.EventAssistantStarted, struct{}{}); err != nil {
						return err
					}
					assistantStarted = true
				}
				lastIndex := len(orderedAssistantItems) - 1
				if lastIndex >= 0 && orderedAssistantItems[lastIndex].Kind == model.ConversationItemAssistantText {
					orderedAssistantItems[lastIndex].Text += event.TextDelta
				} else {
					orderedAssistantItems = append(orderedAssistantItems, model.ConversationItem{
						Kind: model.ConversationItemAssistantText,
						Text: event.TextDelta,
					})
				}
				if err := emitter.Emit(ctx, types.EventAssistantDelta, types.AssistantDeltaPayload{Text: event.TextDelta}); err != nil {
					return err
				}
			case model.StreamEventToolCallStart, model.StreamEventToolCallDelta:
				continue
			case model.StreamEventToolCallEnd:
				toolCalls = append(toolCalls, event.ToolCall)
				orderedAssistantItems = append(orderedAssistantItems, model.ConversationItem{
					Kind: model.ConversationItemToolCall,
					ToolCall: model.ToolCallChunk{
						ID:    event.ToolCall.ID,
						Name:  event.ToolCall.Name,
						Input: event.ToolCall.Input,
					},
				})
			case model.StreamEventMessageEnd:
				messageEnded = true
			case model.StreamEventUsage:
				continue
			case model.StreamEventResponseMetadata:
				responseMeta = event.ResponseMetadata
			default:
				return emitter.Fail(ctx, fmt.Errorf("unsupported stream event kind: %s", event.Kind))
			}
		}

		if errs != nil {
			err := <-errs
			if err != nil {
				return emitter.Fail(ctx, err)
			}
		}

		if err := updateLoopCacheHead(ctx, e, state, responseMeta); err != nil {
			return emitter.Fail(ctx, err)
		}
		updateLoopUsageTotals(&usageTotals, responseMeta)

		if len(toolCalls) == 0 {
			if err := completeAssistantOnlyTurn(ctx, e, in, emitter, state, messageEnded, orderedAssistantItems, usageTotals); err != nil {
				return err
			}
			return nil
		}

		var err error
		var interrupted bool
		toolSteps, interrupted, err = executeToolCallBatches(ctx, e, in, emitter, state, toolCalls, orderedAssistantItems, toolSteps)
		if err != nil {
			return err
		}
		if interrupted {
			return nil
		}
	}
}

func updateLoopCacheHead(ctx context.Context, e *Engine, state *preparedLoopState, responseMeta *model.ResponseMetadata) error {
	if responseMeta == nil || state.req.Cache == nil || state.caps.Profile == model.CapabilityProfileNone {
		return nil
	}
	cacheMode := state.req.Cache.Mode
	if state.caps.SupportsSessionCache {
		cacheMode = model.CacheModeSession
	}
	nextHead, ok := nextHeadFromResponse(e, state.sessionID, state.providerName, state.caps, state.cacheHead, state.req.Cache, responseMeta)
	if !ok {
		return nil
	}
	if err := persistProviderCacheHead(ctx, e, nextHead); err != nil {
		return err
	}
	state.cacheHead = nextHead
	if e.runtime != nil {
		state.req.Cache = e.runtime.CacheDirectiveForHead(state.cacheHead, state.caps, cacheMode)
	}
	state.req.Items = nil
	state.req.ToolResults = nil
	state.req.UserMessage = ""
	return nil
}

func updateLoopUsageTotals(totals *loopUsageTotals, responseMeta *model.ResponseMetadata) {
	if responseMeta == nil {
		return
	}
	totals.inputTokens += responseMeta.InputTokens
	totals.outputTokens += responseMeta.OutputTokens
	totals.cachedTokens += responseMeta.CachedTokens
	totals.hasUsage = true
}

func completeAssistantOnlyTurn(
	ctx context.Context,
	e *Engine,
	in Input,
	emitter loopEmitter,
	state *preparedLoopState,
	messageEnded bool,
	orderedAssistantItems []model.ConversationItem,
	usageTotals loopUsageTotals,
) error {
	writeContextHeadID := in.Turn.ContextHeadID
	if e.store != nil {
		resolvedContextHeadID, err := resolveConversationWriteContextHeadID(ctx, e.store, in.Turn.ContextHeadID)
		if err != nil {
			return emitter.Fail(ctx, err)
		}
		writeContextHeadID = resolvedContextHeadID
	}
	nextPositionBeforeFlush := state.nextPosition
	var err error
	state.nextPosition, _, err = flushAssistantItems(ctx, e.store, state.sessionID, writeContextHeadID, in.Turn.ID, state.nextPosition, orderedAssistantItems, 0, "", &state.req, state.nativeContinuation)
	if err != nil {
		return emitter.Fail(ctx, err)
	}
	if !messageEnded {
		return emitter.Fail(ctx, fmt.Errorf("model stream ended without message_end signal"))
	}
	usage := buildTurnUsage(
		usageTotals.hasUsage,
		in.Turn.ID,
		state.sessionID,
		state.usageProvider,
		state.usageModel,
		usageTotals.inputTokens,
		usageTotals.outputTokens,
		usageTotals.cachedTokens,
	)
	parentReplyCommitted, err := buildParentReplyCommittedPayload(ctx, e.store, in.Session, in.Turn, writeContextHeadID, nextPositionBeforeFlush, orderedAssistantItems)
	if err != nil {
		return emitter.Fail(ctx, err)
	}
	return finalizeTurn(ctx, e, in, usage, parentReplyCommitted)
}

func executeToolCallBatches(
	ctx context.Context,
	e *Engine,
	in Input,
	emitter loopEmitter,
	state *preparedLoopState,
	toolCalls []model.ToolCallChunk,
	orderedAssistantItems []model.ConversationItem,
	toolSteps int,
) (int, bool, error) {
	callInputs := make([]tools.Call, 0, len(toolCalls))
	for _, call := range toolCalls {
		callInputs = append(callInputs, tools.Call{
			ID:    call.ID,
			Name:  call.Name,
			Input: call.Input,
		})
	}
	if state.turnCtx.CurrentRunID == "" && e.runtimeService != nil {
		if _, err := e.runtimeService.EnsureRun(ctx, state.turnCtx, state.sessionID, in.Turn.ID, "Turn tool execution"); err != nil {
			return toolSteps, false, emitter.Fail(ctx, err)
		}
	}

	callOffset := 0
	assistantCursor := 0
	persistRemainingAssistantItems := true
	interrupted := false
	for _, batch := range state.toolRuntime.PlanBatches(callInputs, state.toolExecCtx) {
		if callOffset+len(batch.Calls) > len(toolCalls) {
			return toolSteps, false, emitter.Fail(ctx, fmt.Errorf("tool batch size mismatch"))
		}

		batchToolCalls := toolCalls[callOffset : callOffset+len(batch.Calls)]
		stepLimitExceededAfterBatch := false
		if e.maxToolSteps > 0 {
			remainingSteps := e.maxToolSteps - toolSteps
			if remainingSteps <= 0 {
				return toolSteps, false, emitter.Fail(ctx, fmt.Errorf("turn exceeded max tool steps (%d)", e.maxToolSteps))
			}
			if remainingSteps < len(batch.Calls) {
				batch.Calls = batch.Calls[:remainingSteps]
				batchToolCalls = batchToolCalls[:remainingSteps]
				batch.Parallel = batch.Parallel && len(batch.Calls) > 1
				stepLimitExceededAfterBatch = true
			}
		}

		for _, call := range batchToolCalls {
			toolSteps++
			payload := types.ToolEventPayload{
				ToolCallID:        call.ID,
				ToolName:          call.Name,
				Arguments:         marshalToolArguments(call.Input),
				ArgumentsRaw:      strings.TrimSpace(call.InputRaw),
				ArgumentsRecovery: strings.TrimSpace(call.InputRecovery),
			}
			if err := emitter.Emit(ctx, types.EventToolStarted, payload); err != nil {
				return toolSteps, false, err
			}
		}

		executed, err := state.toolRuntime.ExecuteBatch(ctx, batch, state.toolExecCtx)
		if err != nil {
			return toolSteps, false, emitter.Fail(ctx, err)
		}

		stopAfterBatch, batchInterrupted, nextPosition, nextCursor, err := applyExecutedToolBatch(
			ctx,
			e,
			in,
			emitter,
			state,
			batchToolCalls,
			executed,
			orderedAssistantItems,
			assistantCursor,
		)
		if err != nil {
			return toolSteps, false, err
		}
		state.nextPosition = nextPosition
		assistantCursor = nextCursor
		callOffset += len(batch.Calls)

		if stepLimitExceededAfterBatch {
			return toolSteps, false, emitter.Fail(ctx, fmt.Errorf("turn exceeded max tool steps (%d)", e.maxToolSteps))
		}
		if batchInterrupted {
			interrupted = true
			persistRemainingAssistantItems = false
			break
		}
		if stopAfterBatch {
			persistRemainingAssistantItems = false
			break
		}
	}
	if persistRemainingAssistantItems {
		var err error
		state.nextPosition, _, err = flushAssistantItems(ctx, e.store, state.sessionID, in.Turn.ContextHeadID, in.Turn.ID, state.nextPosition, orderedAssistantItems, assistantCursor, "", &state.req, state.nativeContinuation)
		if err != nil {
			return toolSteps, false, emitter.Fail(ctx, err)
		}
	}
	return toolSteps, interrupted, nil
}

func applyExecutedToolBatch(
	ctx context.Context,
	e *Engine,
	in Input,
	emitter loopEmitter,
	state *preparedLoopState,
	batchToolCalls []model.ToolCallChunk,
	executed []tools.CallExecution,
	orderedAssistantItems []model.ConversationItem,
	assistantCursor int,
) (bool, bool, int, int, error) {
	stopAfterBatch := false
	interrupted := false
	nextPosition := state.nextPosition
	for index, execResult := range executed {
		call := batchToolCalls[index]
		var err error
		nextPosition, assistantCursor, err = flushAssistantItems(ctx, e.store, state.sessionID, in.Turn.ContextHeadID, in.Turn.ID, nextPosition, orderedAssistantItems, assistantCursor, call.ID, &state.req, state.nativeContinuation)
		if err != nil {
			return false, false, state.nextPosition, assistantCursor, emitter.Fail(ctx, err)
		}
		shouldStop, toolInterrupted, err := applyExecutedToolResult(ctx, e, in, emitter, state, call, execResult, &nextPosition)
		if err != nil {
			return false, false, state.nextPosition, assistantCursor, err
		}
		if shouldStop {
			stopAfterBatch = true
		}
		if toolInterrupted {
			interrupted = true
		}
	}
	return stopAfterBatch, interrupted, nextPosition, assistantCursor, nil
}

func applyExecutedToolResult(
	ctx context.Context,
	e *Engine,
	in Input,
	emitter loopEmitter,
	state *preparedLoopState,
	call model.ToolCallChunk,
	execResult tools.CallExecution,
	nextPosition *int,
) (bool, bool, error) {
	result := execResult.Result
	output := execResult.Output
	execErr := execResult.Err

	modelToolResult := execResult.ModelResult
	toolResultText := result.Text
	toolIsError := execErr != nil
	if execErr != nil {
		toolResultText = execErr.Error()
		if modelToolResult.Structured == nil {
			modelToolResult.Structured = structuredToolError(execErr)
		}
	} else if strings.TrimSpace(output.PreviewText) != "" {
		toolResultText = output.PreviewText
	}
	if modelToolResult.IsError {
		toolIsError = true
	}
	modelToolResultText := toolResultText
	if strings.TrimSpace(modelToolResult.Text) != "" {
		modelToolResultText = modelToolResult.Text
	} else if strings.TrimSpace(result.ModelText) != "" {
		modelToolResultText = result.ModelText
	}

	payload := types.ToolEventPayload{
		ToolCallID:        call.ID,
		ToolName:          call.Name,
		Arguments:         marshalToolArguments(call.Input),
		ArgumentsRaw:      strings.TrimSpace(call.InputRaw),
		ArgumentsRecovery: strings.TrimSpace(call.InputRecovery),
		ResultPreview:     previewToolResult(toolResultText),
		IsError:           toolIsError,
	}
	if err := emitter.Emit(ctx, types.EventToolCompleted, payload); err != nil {
		return false, false, err
	}

	toolResult := model.ToolResult{
		ToolCallID:     call.ID,
		ToolName:       call.Name,
		Content:        modelToolResultText,
		StructuredJSON: marshalStructuredToolResult(modelToolResult.Structured),
		IsError:        toolIsError,
	}
	if output.Interrupt == nil || !output.Interrupt.DeferToolResult {
		toolResultItem := model.ToolResultItem(toolResult)
		if err := persistConversationItem(ctx, e.store, state.sessionID, in.Turn.ContextHeadID, in.Turn.ID, *nextPosition, toolResultItem); err != nil {
			return false, false, emitter.Fail(ctx, err)
		}
		state.req.Items = append(state.req.Items, toolResultItem)
		*nextPosition = *nextPosition + 1
		state.req.ToolResults = append(state.req.ToolResults, toolResult)
	}

	for _, item := range output.NewItems {
		if err := persistConversationItem(ctx, e.store, state.sessionID, in.Turn.ContextHeadID, in.Turn.ID, *nextPosition, item); err != nil {
			return false, false, emitter.Fail(ctx, err)
		}
		state.req.Items = append(state.req.Items, item)
		*nextPosition = *nextPosition + 1
	}

	if !toolIsError {
		if err := updateLoopSkillsFromToolMetadata(e, state, output.Metadata); err != nil {
			return false, false, emitter.Fail(ctx, err)
		}
	}

	if output.Interrupt != nil {
		if err := emitToolInterrupt(ctx, e, in, emitter, state, call, output); err != nil {
			return false, false, err
		}
		return false, true, nil
	}
	return toolIsError, false, nil
}

func updateLoopSkillsFromToolMetadata(e *Engine, state *preparedLoopState, metadata map[string]any) error {
	activatedNames := activatedSkillNamesFromMetadata(metadata)
	if len(activatedNames) == 0 {
		return nil
	}
	state.activeSkills = skills.MergeActivatedSkills(
		state.activeSkills,
		skills.SelectByNames(state.skillCatalog, activatedNames, skills.ActivationReasonToolUse),
	)
	state.toolExecCtx.ActiveSkillNames = activatedSkillNames(state.activeSkills)
	injectedEnv, err := loadActivatedSkillEnv(e.globalConfigRoot, state.activeSkills)
	if err != nil {
		return err
	}
	state.toolExecCtx.InjectedEnv = injectedEnv
	state.visibleDefs = state.toolRuntime.VisibleDefinitions(state.toolExecCtx)
	state.req.Tools = buildToolSchemas(state.visibleDefs)
	state.req.Instructions = appendChildReportPromptSection(instructions.Compile(instructions.CompileInput{
		BaseText:     state.baseInstructions,
		Catalog:      state.skillCatalog,
		Message:      state.turnMessage,
		ActiveSkills: state.activeSkills,
	}).Render(), state.childReports)
	return nil
}

func emitToolInterrupt(
	ctx context.Context,
	e *Engine,
	in Input,
	emitter loopEmitter,
	state *preparedLoopState,
	call model.ToolCallChunk,
	output tools.ToolExecutionResult,
) error {
	if strings.TrimSpace(output.Interrupt.EventType) == types.EventPermissionRequested {
		if err := persistPermissionPause(ctx, e, in, state.turnCtx, call, output); err != nil {
			return emitter.Fail(ctx, err)
		}
		if payload, ok := output.Interrupt.EventPayload.(types.PermissionRequestedPayload); ok {
			if payload.ToolCallID == "" {
				payload.ToolCallID = call.ID
			}
			if payload.ToolName == "" {
				payload.ToolName = call.Name
			}
			if payload.TurnID == "" {
				payload.TurnID = in.Turn.ID
			}
			output.Interrupt.EventPayload = payload
		}
	}
	if eventType := strings.TrimSpace(output.Interrupt.EventType); eventType != "" {
		payload := output.Interrupt.EventPayload
		if payload == nil {
			payload = map[string]string{"reason": output.Interrupt.Reason}
		}
		if err := emitter.Emit(ctx, eventType, payload); err != nil {
			return err
		}
	}
	if notice := strings.TrimSpace(output.Interrupt.Notice); notice != "" {
		if err := emitter.Emit(ctx, types.EventSystemNotice, types.NoticePayload{Text: notice}); err != nil {
			return err
		}
	}
	reason := strings.TrimSpace(output.Interrupt.Reason)
	if reason == "" {
		reason = "tool_interrupted"
	}
	return emitter.Emit(ctx, types.EventTurnInterrupted, map[string]string{"reason": reason})
}
