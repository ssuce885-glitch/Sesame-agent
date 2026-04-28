package engine

import (
	"context"
	"fmt"
	"sort"
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
			case model.StreamEventThinkingSignature:
				if strings.TrimSpace(event.ThinkingSignature) == "" {
					continue
				}
				lastIndex := len(orderedAssistantItems) - 1
				if lastIndex >= 0 && orderedAssistantItems[lastIndex].Kind == model.ConversationItemAssistantThinking {
					orderedAssistantItems[lastIndex].ThinkingSignature = event.ThinkingSignature
				} else {
					orderedAssistantItems = append(orderedAssistantItems, model.AssistantThinkingBlockItem("", event.ThinkingSignature))
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
		toolSteps, interrupted, err = executeToolCallBatches(ctx, e, in, emitter, state, toolCalls, orderedAssistantItems, toolSteps, usageTotals)
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
	parentReplyCommitted, err := buildParentReplyCommittedPayload(ctx, e.store, in.Session, in.Turn, writeContextHeadID, nextPositionBeforeFlush, orderedAssistantItems, state.reports)
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
	usageTotals loopUsageTotals,
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
	nextPositionBeforeFlush := state.nextPosition
	persistRemainingAssistantItems := true
	interrupted := false
	completeTurnAfterTools := false
	if e.maxToolSteps > 0 && toolSteps+len(toolCalls) > e.maxToolSteps {
		return toolSteps, false, emitter.Fail(ctx, fmt.Errorf("turn exceeded max tool steps (%d)", e.maxToolSteps))
	}
	plannedBatches := state.toolRuntime.PlanBatches(callInputs, state.toolExecCtx)
	turnCompletingBatch := firstTurnCompletingBatchIndex(plannedBatches)
	assistantFlushBatch := len(plannedBatches) - 1
	if turnCompletingBatch >= 0 {
		assistantFlushBatch = turnCompletingBatch
	}
	if assistantFlushBatch >= 0 {
		lastToolCallID, ok := batchLastToolCallID(toolCalls, plannedBatches, assistantFlushBatch)
		if !ok {
			return toolSteps, false, emitter.Fail(ctx, fmt.Errorf("tool batch size mismatch"))
		}
		var err error
		state.nextPosition, assistantCursor, err = flushAssistantItems(ctx, e.store, state.sessionID, in.Turn.ContextHeadID, in.Turn.ID, state.nextPosition, orderedAssistantItems, assistantCursor, lastToolCallID, &state.req, state.nativeContinuation)
		if err != nil {
			return toolSteps, false, emitter.Fail(ctx, err)
		}
	}
	for batchIndex, batch := range plannedBatches {
		if turnCompletingBatch >= 0 && batchIndex > turnCompletingBatch {
			break
		}
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

		stopAfterBatch, batchInterrupted, nextPosition, err := applyExecutedToolBatch(
			ctx,
			e,
			in,
			emitter,
			state,
			batchToolCalls,
			executed,
		)
		if err != nil {
			return toolSteps, false, err
		}
		state.nextPosition = nextPosition
		callOffset += len(batch.Calls)
		if stopAfterBatch {
			completeTurnAfterTools = true
			persistRemainingAssistantItems = false
		}

		if stepLimitExceededAfterBatch {
			return toolSteps, false, emitter.Fail(ctx, fmt.Errorf("turn exceeded max tool steps (%d)", e.maxToolSteps))
		}
		if batchInterrupted {
			interrupted = true
			persistRemainingAssistantItems = false
			break
		}
		if stopAfterBatch {
			break
		}
		if turnCompletingBatch == batchIndex {
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
	if completeTurnAfterTools && !interrupted {
		writeContextHeadID := in.Turn.ContextHeadID
		if e.store != nil {
			resolvedContextHeadID, err := resolveConversationWriteContextHeadID(ctx, e.store, in.Turn.ContextHeadID)
			if err != nil {
				return toolSteps, false, emitter.Fail(ctx, err)
			}
			writeContextHeadID = resolvedContextHeadID
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
		committedAssistantItems := orderedAssistantItems
		if assistantCursor >= 0 && assistantCursor <= len(orderedAssistantItems) {
			committedAssistantItems = orderedAssistantItems[:assistantCursor]
		}
		parentReplyCommitted, err := buildParentReplyCommittedPayload(ctx, e.store, in.Session, in.Turn, writeContextHeadID, nextPositionBeforeFlush, committedAssistantItems, state.reports)
		if err != nil {
			return toolSteps, false, emitter.Fail(ctx, err)
		}
		if err := finalizeTurn(ctx, e, in, usage, parentReplyCommitted); err != nil {
			return toolSteps, false, err
		}
		return toolSteps, true, nil
	}
	return toolSteps, interrupted, nil
}

func firstTurnCompletingBatchIndex(batches []tools.CallBatch) int {
	for index, batch := range batches {
		for _, call := range batch.Calls {
			if tools.CompletesTurnOnSuccess(call.Tool) {
				return index
			}
		}
	}
	return -1
}

func batchLastToolCallID(toolCalls []model.ToolCallChunk, batches []tools.CallBatch, batchIndex int) (string, bool) {
	if batchIndex < 0 || batchIndex >= len(batches) {
		return "", false
	}
	offset := 0
	for index := 0; index < batchIndex; index++ {
		offset += len(batches[index].Calls)
	}
	batchSize := len(batches[batchIndex].Calls)
	if batchSize == 0 {
		return "", false
	}
	lastIndex := offset + batchSize - 1
	if lastIndex < 0 || lastIndex >= len(toolCalls) {
		return "", false
	}
	return toolCalls[lastIndex].ID, true
}

func applyExecutedToolBatch(
	ctx context.Context,
	e *Engine,
	in Input,
	emitter loopEmitter,
	state *preparedLoopState,
	batchToolCalls []model.ToolCallChunk,
	executed []tools.CallExecution,
) (bool, bool, int, error) {
	stopAfterBatch := false
	interrupted := false
	nextPosition := state.nextPosition
	for index, execResult := range executed {
		call := batchToolCalls[index]
		shouldStop, toolInterrupted, err := applyExecutedToolResult(ctx, e, in, emitter, state, call, execResult, &nextPosition)
		if err != nil {
			return false, false, state.nextPosition, err
		}
		if shouldStop {
			stopAfterBatch = true
		}
		if toolInterrupted {
			interrupted = true
		}
	}
	return stopAfterBatch, interrupted, nextPosition, nil
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
	if e.maxToolResultStoreBytes > 0 && len(modelToolResultText) > e.maxToolResultStoreBytes {
		modelToolResultText = modelToolResultText[:e.maxToolResultStoreBytes] + "...[truncated]"
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
	if output.CompleteTurn {
		return true, false, nil
	}
	return false, false, nil
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
	previousDefs := state.visibleDefs
	state.visibleDefs = state.toolRuntime.VisibleDefinitions(state.toolExecCtx)
	clearPreviousResponseWhenVisibleToolsChange(&state.req, previousDefs, state.visibleDefs)
	state.req.Tools = buildToolSchemas(state.visibleDefs)
	state.req.Instructions = appendReportPromptSection(instructions.Render(instructions.RenderInput{
		BaseText:     state.baseInstructions,
		Catalog:      state.skillCatalog,
		Message:      state.turnMessage,
		ActiveSkills: state.activeSkills,
	}).Render(), state.reports)
	return nil
}

func clearPreviousResponseWhenVisibleToolsChange(req *model.Request, before, after []tools.Definition) {
	if req == nil || req.Cache == nil || req.Cache.PreviousResponseID == "" {
		return
	}
	if sameVisibleToolNames(before, after) {
		return
	}
	req.Cache.PreviousResponseID = ""
}

func sameVisibleToolNames(left, right []tools.Definition) bool {
	leftNames := visibleToolNames(left)
	rightNames := visibleToolNames(right)
	if len(leftNames) != len(rightNames) {
		return false
	}
	for i := range leftNames {
		if leftNames[i] != rightNames[i] {
			return false
		}
	}
	return true
}

func visibleToolNames(defs []tools.Definition) []string {
	if len(defs) == 0 {
		return nil
	}
	names := make([]string, 0, len(defs))
	for _, def := range defs {
		if name := strings.TrimSpace(def.Name); name != "" {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
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
