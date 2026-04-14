package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"go-agent/internal/config"
	contextstate "go-agent/internal/context"
	"go-agent/internal/instructions"
	"go-agent/internal/intent"
	"go-agent/internal/model"
	"go-agent/internal/permissions"
	"go-agent/internal/runtimegraph"
	"go-agent/internal/skills"
	"go-agent/internal/toolrouter"
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
	turnCtx := &runtimegraph.TurnContext{
		CurrentSessionID: sessionID,
		CurrentTurnID:    in.Turn.ID,
	}
	if in.Resume != nil {
		turnCtx.CurrentRunID = strings.TrimSpace(in.Resume.RunID)
		turnCtx.CurrentTaskID = strings.TrimSpace(in.Resume.TaskID)
	}
	permissionEngine := effectivePermissionEngine(e.permission, in)
	toolExecCtx := tools.ExecContext{
		WorkspaceRoot:     in.Session.WorkspaceRoot,
		GlobalConfigRoot:  e.globalConfigRoot,
		PermissionEngine:  permissionEngine,
		AutomationService: e.automationService,
		TaskManager:       e.taskManager,
		RuntimeService:    e.runtimeService,
		SchedulerService:  e.schedulerService,
		TurnContext:       turnCtx,
		EventSink:         in.Sink,
	}
	toolRuntime := tools.NewRuntime(e.registry, toolRunStoreFromConversationStore(e.store))

	totalItems, working, completionNotices, err := loadConversationState(ctx, e, in, sessionID)
	if err != nil {
		if emitErr := emitFailed(err.Error()); emitErr != nil {
			return errors.Join(err, emitErr)
		}
		return err
	}
	reportMailboxItems, err := loadPendingReportMailboxItems(ctx, e.store, sessionID, in.Turn.ID)
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

	runtimeInstructions, err := buildRuntimeInstructionsWithMaxBytes(in.Session, e.basePrompt, working.MemoryRefs, e.maxWorkspacePromptBytes)
	if err != nil {
		if emitErr := emitFailed(err.Error()); emitErr != nil {
			return errors.Join(err, emitErr)
		}
		return err
	}
	catalog, err := skills.LoadCatalog(e.globalConfigRoot, in.Session.WorkspaceRoot)
	if err != nil {
		if emitErr := emitFailed(err.Error()); emitErr != nil {
			return errors.Join(err, emitErr)
		}
		return err
	}
	plan, err := resolveIntentPlan(ctx, e.store, sessionID, in.Turn.UserMessage, catalog)
	if err != nil {
		if emitErr := emitFailed(err.Error()); emitErr != nil {
			return errors.Join(err, emitErr)
		}
		return err
	}
	if plan.NeedsConfirm {
		return emitPlanningConfirmationTurn(ctx, e, in, emit, totalItems, sessionID, plan)
	}

	baseInstructionsText := runtimeInstructions.Text
	resolution := skills.Resolve(plan, catalog, in.ActivatedSkillNames)
	toolState := resolveTurnToolState(plan, toolRuntime, toolExecCtx, resolution)
	activeSkills := toolState.ActiveSkills
	toolExecCtx.ActiveSkillNames = activatedSkillNames(activeSkills)
	toolExecCtx.InjectedEnv, err = loadActivatedSkillEnv(e.globalConfigRoot, activeSkills)
	if err != nil {
		if emitErr := emitFailed(err.Error()); emitErr != nil {
			return errors.Join(err, emitErr)
		}
		return err
	}
	toolDecision := toolState.Decision
	visibleDefs := toolState.VisibleDefs
	instructionBundle := instructions.Compile(instructions.CompileInput{
		BaseText:               baseInstructionsText,
		Catalog:                catalog,
		Message:                in.Turn.UserMessage,
		Policy:                 toolDecision.Summary,
		VisibleTools:           toolState.VisibleToolNames,
		ActiveSkills:           activeSkills,
		SuggestedSkills:        resolution.Suggested,
		ActiveSkillTokenBudget: e.activeSkillTokenBudget,
	})
	runtimeInstructions.Text = appendTurnSupplementalPrompts(instructionBundle.Render(), completionNotices, reportMailboxItems)
	runtimeInstructions.Notices = append(runtimeInstructions.Notices, instructionBundle.Notices...)
	runtimeInstructions.Notices = append(runtimeInstructions.Notices, completionNotices...)
	req := e.runtime.PrepareRequest(
		working,
		cacheHead,
		caps,
		resumeAwareUserItem(in),
		runtimeInstructions.Text,
	)
	for _, notice := range runtimeInstructions.Notices {
		if err := emit(types.EventSystemNotice, types.NoticePayload{Text: notice}); err != nil {
			return err
		}
	}
	req.Stream = true
	req.Tools = buildToolSchemas(visibleDefs)
	req.ToolChoice = "auto"
	nativeContinuation := req.Cache != nil && caps.Profile != model.CapabilityProfileNone
	assistantStarted := false
	nextPosition := totalItems + 1
	toolSteps := 0
	totalInputTokens := 0
	totalOutputTokens := 0
	totalCachedTokens := 0
	hasUsage := false

	if in.Resume == nil {
		userItem := model.UserMessageItem(in.Turn.UserMessage)
		if err := persistConversationItem(ctx, e.store, sessionID, in.Turn.ID, nextPosition, userItem); err != nil {
			if emitErr := emitFailed(err.Error()); emitErr != nil {
				return errors.Join(err, emitErr)
			}
			return err
		}
		nextPosition++
	} else {
		toolResultItem, toolResult := resumeToolResultItem(in.Resume)
		if err := persistConversationItem(ctx, e.store, sessionID, in.Turn.ID, nextPosition, toolResultItem); err != nil {
			if emitErr := emitFailed(err.Error()); emitErr != nil {
				return errors.Join(err, emitErr)
			}
			return err
		}
		req.Items = append(req.Items, toolResultItem)
		req.ToolResults = append(req.ToolResults, toolResult)
		nextPosition++
	}

	for {
		stream, errs := e.model.Stream(ctx, req)
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
					if err := emit(types.EventAssistantStarted, struct{}{}); err != nil {
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
				if err := emit(types.EventAssistantDelta, types.AssistantDeltaPayload{Text: event.TextDelta}); err != nil {
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

		if len(toolCalls) == 0 {
			nextPosition, _, err = flushAssistantItems(ctx, e.store, sessionID, in.Turn.ID, nextPosition, orderedAssistantItems, 0, "", &req, nativeContinuation)
			if err != nil {
				if emitErr := emitFailed(err.Error()); emitErr != nil {
					return errors.Join(err, emitErr)
				}
				return err
			}
			if !messageEnded {
				err := fmt.Errorf("model stream ended without message_end signal")
				if emitErr := emitFailed(err.Error()); emitErr != nil {
					return errors.Join(err, emitErr)
				}
				return err
			}
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
			return nil
		}

		callInputs := make([]tools.Call, 0, len(toolCalls))
		for _, call := range toolCalls {
			callInputs = append(callInputs, tools.Call{
				ID:    call.ID,
				Name:  call.Name,
				Input: call.Input,
			})
		}
		if turnCtx.CurrentRunID == "" && e.runtimeService != nil {
			if _, err := e.runtimeService.EnsureRun(ctx, turnCtx, sessionID, in.Turn.ID, "Turn tool execution"); err != nil {
				if emitErr := emitFailed(err.Error()); emitErr != nil {
					return errors.Join(err, emitErr)
				}
				return err
			}
		}

		callOffset := 0
		assistantCursor := 0
		persistRemainingAssistantItems := true
		for _, batch := range toolRuntime.PlanBatches(callInputs, toolExecCtx) {
			if callOffset+len(batch.Calls) > len(toolCalls) {
				err := fmt.Errorf("tool batch size mismatch")
				if emitErr := emitFailed(err.Error()); emitErr != nil {
					return errors.Join(err, emitErr)
				}
				return err
			}

			batchToolCalls := toolCalls[callOffset : callOffset+len(batch.Calls)]
			stepLimitExceededAfterBatch := false
			if e.maxToolSteps > 0 {
				remainingSteps := e.maxToolSteps - toolSteps
				if remainingSteps <= 0 {
					err := fmt.Errorf("turn exceeded max tool steps (%d)", e.maxToolSteps)
					if emitErr := emitFailed(err.Error()); emitErr != nil {
						return errors.Join(err, emitErr)
					}
					return err
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
					ToolCallID: call.ID,
					ToolName:   call.Name,
					Arguments:  marshalToolArguments(call.Input),
				}
				if err := emit(types.EventToolStarted, payload); err != nil {
					return err
				}
			}

			executed, err := toolRuntime.ExecuteBatch(ctx, batch, toolExecCtx)
			if err != nil {
				if emitErr := emitFailed(err.Error()); emitErr != nil {
					return errors.Join(err, emitErr)
				}
				return err
			}

			stopAfterBatch := false
			for index, execResult := range executed {
				call := batchToolCalls[index]
				nextPosition, assistantCursor, err = flushAssistantItems(ctx, e.store, sessionID, in.Turn.ID, nextPosition, orderedAssistantItems, assistantCursor, call.ID, &req, nativeContinuation)
				if err != nil {
					if emitErr := emitFailed(err.Error()); emitErr != nil {
						return errors.Join(err, emitErr)
					}
					return err
				}
				result := execResult.Result
				output := execResult.Output
				execErr := execResult.Err

				modelToolResult := execResult.ModelResult
				var toolResultText string
				toolIsError := execErr != nil
				if execErr != nil {
					toolResultText = execErr.Error()
					if modelToolResult.Structured == nil {
						modelToolResult.Structured = structuredToolError(execErr)
					}
				} else {
					toolResultText = result.Text
					if strings.TrimSpace(output.PreviewText) != "" {
						toolResultText = output.PreviewText
					}
				}
				if modelToolResult.IsError {
					toolIsError = true
				}
				if toolIsError {
					stopAfterBatch = true
				}
				modelToolResultText := toolResultText
				if strings.TrimSpace(modelToolResult.Text) != "" {
					modelToolResultText = modelToolResult.Text
				} else if strings.TrimSpace(result.ModelText) != "" {
					modelToolResultText = result.ModelText
				}

				payload := types.ToolEventPayload{
					ToolCallID:    call.ID,
					ToolName:      call.Name,
					Arguments:     marshalToolArguments(call.Input),
					ResultPreview: previewToolResult(toolResultText),
				}
				if err := emit(types.EventToolCompleted, payload); err != nil {
					return err
				}

				toolResult := model.ToolResult{
					ToolCallID:     call.ID,
					ToolName:       call.Name,
					Content:        modelToolResultText,
					StructuredJSON: marshalStructuredToolResult(modelToolResult.Structured),
					IsError:        toolIsError,
				}
				persistToolResult := output.Interrupt == nil || !output.Interrupt.DeferToolResult
				if persistToolResult {
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

				for _, item := range output.NewItems {
					if err := persistConversationItem(ctx, e.store, sessionID, in.Turn.ID, nextPosition, item); err != nil {
						if emitErr := emitFailed(err.Error()); emitErr != nil {
							return errors.Join(err, emitErr)
						}
						return err
					}
					req.Items = append(req.Items, item)
					nextPosition++
				}
				if !toolIsError {
					if activatedNames := activatedSkillNamesFromMetadata(output.Metadata); len(activatedNames) > 0 {
						activeSkills = skills.MergeActivatedSkills(
							activeSkills,
							skills.SelectByNames(catalog, activatedNames, skills.ActivationReasonToolUse),
						)
						toolExecCtx.ActiveSkillNames = activatedSkillNames(activeSkills)
						toolExecCtx.InjectedEnv, err = loadActivatedSkillEnv(e.globalConfigRoot, activeSkills)
						if err != nil {
							if emitErr := emitFailed(err.Error()); emitErr != nil {
								return errors.Join(err, emitErr)
							}
							return err
						}
						resolution.Activated = activeSkills
						toolState = resolveTurnToolState(plan, toolRuntime, toolExecCtx, resolution)
						activeSkills = toolState.ActiveSkills
						toolDecision = toolState.Decision
						visibleDefs = toolState.VisibleDefs
						req.Tools = buildToolSchemas(visibleDefs)
						req.Instructions = appendTurnSupplementalPrompts(instructions.Compile(instructions.CompileInput{
							BaseText:               baseInstructionsText,
							Catalog:                catalog,
							Message:                in.Turn.UserMessage,
							Policy:                 toolDecision.Summary,
							VisibleTools:           toolState.VisibleToolNames,
							ActiveSkills:           activeSkills,
							SuggestedSkills:        resolution.Suggested,
							ActiveSkillTokenBudget: e.activeSkillTokenBudget,
						}).Render(), completionNotices, reportMailboxItems)
					}
				}

				if output.Interrupt != nil {
					if strings.TrimSpace(output.Interrupt.EventType) == types.EventPermissionRequested {
						if err := persistPermissionPause(ctx, e, in, turnCtx, call, output); err != nil {
							if emitErr := emitFailed(err.Error()); emitErr != nil {
								return errors.Join(err, emitErr)
							}
							return err
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
						if err := emit(eventType, payload); err != nil {
							return err
						}
					}
					if notice := strings.TrimSpace(output.Interrupt.Notice); notice != "" {
						if err := emit(types.EventSystemNotice, types.NoticePayload{Text: notice}); err != nil {
							return err
						}
					}
					reason := strings.TrimSpace(output.Interrupt.Reason)
					if reason == "" {
						reason = "tool_interrupted"
					}
					if err := emit(types.EventTurnInterrupted, map[string]string{"reason": reason}); err != nil {
						return err
					}
					return nil
				}
			}

			callOffset += len(batch.Calls)
			if stepLimitExceededAfterBatch {
				err := fmt.Errorf("turn exceeded max tool steps (%d)", e.maxToolSteps)
				if emitErr := emitFailed(err.Error()); emitErr != nil {
					return errors.Join(err, emitErr)
				}
				return err
			}
			if stopAfterBatch {
				persistRemainingAssistantItems = false
				break
			}
		}
		if persistRemainingAssistantItems {
			nextPosition, _, err = flushAssistantItems(ctx, e.store, sessionID, in.Turn.ID, nextPosition, orderedAssistantItems, assistantCursor, "", &req, nativeContinuation)
			if err != nil {
				if emitErr := emitFailed(err.Error()); emitErr != nil {
					return errors.Join(err, emitErr)
				}
				return err
			}
		}
	}
}

func effectivePermissionEngine(base *permissions.Engine, in Input) *permissions.Engine {
	if in.Resume != nil && strings.TrimSpace(in.Resume.EffectivePermissionProfile) != "" {
		return permissions.NewEngine(in.Resume.EffectivePermissionProfile)
	}
	if strings.TrimSpace(in.Session.PermissionProfile) != "" {
		return permissions.NewEngine(in.Session.PermissionProfile)
	}
	return base
}

func resumeAwareUserItem(in Input) model.ConversationItem {
	if in.Resume != nil {
		return model.ConversationItem{}
	}
	return model.UserMessageItem(in.Turn.UserMessage)
}

func resumeToolResultItem(resume *types.TurnResume) (model.ConversationItem, model.ToolResult) {
	if resume == nil {
		return model.ConversationItem{}, model.ToolResult{}
	}
	content := fmt.Sprintf("Permission request resolved: %s.", resume.Decision)
	isError := resume.Decision == types.PermissionDecisionDeny
	if resume.DecisionScope != "" {
		content += " Scope: " + resume.DecisionScope + "."
	}
	if resume.RequestedProfile != "" {
		content += " Requested profile: " + resume.RequestedProfile + "."
	}
	if resume.Reason != "" {
		content += " Reason: " + resume.Reason + "."
	}
	if resume.EffectivePermissionProfile != "" {
		content += " Effective profile: " + resume.EffectivePermissionProfile + "."
	}
	result := model.ToolResult{
		ToolCallID: resume.ToolCallID,
		ToolName:   resume.ToolName,
		Content:    content,
		StructuredJSON: marshalStructuredToolResult(map[string]any{
			"status":                       map[bool]string{true: "denied", false: "resolved"}[isError],
			"decision":                     resume.Decision,
			"decision_scope":               resume.DecisionScope,
			"requested_profile":            resume.RequestedProfile,
			"effective_permission_profile": resume.EffectivePermissionProfile,
		}),
		IsError: isError,
	}
	return model.ToolResultItem(result), result
}

func loadConversationState(ctx context.Context, e *Engine, in Input, sessionID string) (int, contextstate.WorkingSet, []string, error) {
	if e.store == nil || e.ctxManager == nil {
		return 0, contextstate.WorkingSet{}, nil, nil
	}

	items, err := e.store.ListConversationItems(ctx, sessionID)
	if err != nil {
		return 0, contextstate.WorkingSet{}, nil, err
	}
	totalItems := len(items)

	summaries, err := e.store.ListConversationSummaries(ctx, sessionID)
	if err != nil {
		return 0, contextstate.WorkingSet{}, nil, err
	}
	sessionMemory, hasSessionMemory, err := loadSessionMemorySummary(ctx, e.store, sessionID)
	if err != nil {
		return 0, contextstate.WorkingSet{}, nil, err
	}
	if hasSessionMemory {
		summaries = prependSessionMemorySummary(summaries, sessionMemory)
	}
	summaryBundle := selectPromptSummaries(summaries, hasSessionMemory)
	compactions, err := e.store.ListConversationCompactions(ctx, sessionID)
	if err != nil {
		return 0, contextstate.WorkingSet{}, nil, err
	}

	entries, err := e.store.ListMemoryEntriesByWorkspace(ctx, in.Session.WorkspaceRoot)
	if err != nil {
		return 0, contextstate.WorkingSet{}, nil, err
	}

	memoryRefs := buildMemoryRefs(entries, hasSessionMemory, in.Session.WorkspaceRoot, in.Turn.UserMessage)

	persistedMicroItems := activeMicrocompactItems(compactions)
	working := e.ctxManager.Build(in.Turn.UserMessage, items, summaryBundle, memoryRefs)
	working = setPromptItems(working, persistedMicroItems, in.Turn.UserMessage)
	if e.compactor != nil {
		switch working.Action.Kind {
		case contextstate.CompactionActionRolling:
			working, summaryBundle, err = applySummaryCompaction(ctx, e, sessionID, in.Turn.UserMessage, items, summaryBundle, memoryRefs, working, len(compactions)+1, types.ConversationCompactionKindRolling, "rolling_summary")
			if err != nil {
				return 0, contextstate.WorkingSet{}, nil, err
			}
		case contextstate.CompactionActionMicrocompact:
			candidatePayload, candidatePromptItems, ok := buildAppliedMicrocompact(items, working.Action.MicrocompactPositions, working.CompactionStart)
			if ok {
				candidateEstimate := contextstate.EstimatePromptTokens(in.Turn.UserMessage, candidatePromptItems, summaryBundle, memoryRefs)
				if candidateEstimate <= e.ctxManager.Config().MaxEstimatedTokens {
					if err := e.store.InsertConversationCompaction(ctx, types.ConversationCompaction{
						ID:              types.NewID("compact"),
						SessionID:       sessionID,
						Kind:            types.ConversationCompactionKindMicro,
						Generation:      len(compactions) + 1,
						StartPosition:   firstPayloadPosition(candidatePayload),
						EndPosition:     lastPayloadPosition(candidatePayload),
						SummaryPayload:  encodeMicrocompactPayload(candidatePayload),
						Reason:          "microcompact_tool_results",
						ProviderProfile: string(e.model.Capabilities().Profile),
						CreatedAt:       time.Now().UTC(),
					}); err != nil {
						return 0, contextstate.WorkingSet{}, nil, err
					}
					working = setPromptItems(working, candidatePayload.Items, in.Turn.UserMessage)
					working.EstimatedTokens = candidateEstimate
					working.CompactionApplied = true
					break
				}
			}
			working, summaryBundle, err = applySummaryCompaction(ctx, e, sessionID, in.Turn.UserMessage, items, summaryBundle, memoryRefs, working, len(compactions)+1, types.ConversationCompactionKindRolling, "microcompact_escalated_to_rolling")
			if err != nil {
				return 0, contextstate.WorkingSet{}, nil, err
			}
		}
	}

	completionNotices, err := loadPendingTaskCompletionNotices(ctx, e.store, sessionID, in.Turn.ID)
	if err != nil {
		return 0, contextstate.WorkingSet{}, nil, err
	}

	return totalItems, working, completionNotices, nil
}

func applySummaryCompaction(
	ctx context.Context,
	e *Engine,
	sessionID string,
	userMessage string,
	items []model.ConversationItem,
	summaryBundle SummaryBundle,
	memoryRefs []string,
	working contextstate.WorkingSet,
	generation int,
	kind types.ConversationCompactionKind,
	reason string,
) (contextstate.WorkingSet, SummaryBundle, error) {
	cutoff := working.CompactionStart
	if cutoff < 0 {
		cutoff = 0
	}
	if cutoff > len(items) {
		cutoff = len(items)
	}
	cutoff = model.NearestSafeConversationBoundary(items, cutoff)
	if cutoff == 0 {
		return working, summaryBundle, nil
	}

	summary, err := e.compactor.Compact(ctx, items[:cutoff])
	if err != nil {
		return contextstate.WorkingSet{}, SummaryBundle{}, err
	}
	if err := e.store.InsertConversationSummary(ctx, sessionID, cutoff, summary); err != nil {
		return contextstate.WorkingSet{}, SummaryBundle{}, err
	}
	if err := e.store.InsertConversationCompaction(ctx, types.ConversationCompaction{
		ID:              types.NewID("compact"),
		SessionID:       sessionID,
		Kind:            kind,
		Generation:      generation,
		StartPosition:   0,
		EndPosition:     cutoff,
		SummaryPayload:  marshalCompactionSummary(summary),
		Reason:          reason,
		ProviderProfile: string(e.model.Capabilities().Profile),
		CreatedAt:       time.Now().UTC(),
	}); err != nil {
		return contextstate.WorkingSet{}, SummaryBundle{}, err
	}

	summaryBundle.Rolling = append(summaryBundle.Rolling, summary)
	working = e.ctxManager.Build(userMessage, items, summaryBundle, memoryRefs)
	working.CompactionApplied = true
	return working, summaryBundle, nil
}

func marshalCompactionSummary(summary model.Summary) string {
	raw, err := json.Marshal(summary)
	if err != nil {
		return summary.RangeLabel
	}
	return string(raw)
}

func setPromptItems(working contextstate.WorkingSet, carryForwardItems []model.ConversationItem, userMessage string) contextstate.WorkingSet {
	working.PromptItems = appendPromptItems(carryForwardItems, working.RecentItems)
	working.EstimatedTokens = contextstate.EstimatePromptTokens(userMessage, working.PromptItems, working.Summaries, working.MemoryRefs)
	return working
}

func appendPromptItems(carryForwardItems, recentItems []model.ConversationItem) []model.ConversationItem {
	if len(carryForwardItems) == 0 {
		return cloneConversationItemsForPrompt(recentItems)
	}

	out := make([]model.ConversationItem, 0, len(carryForwardItems)+len(recentItems))
	out = append(out, cloneConversationItemsForPrompt(carryForwardItems)...)
	out = append(out, cloneConversationItemsForPrompt(recentItems)...)
	return out
}

func buildAppliedMicrocompact(items []model.ConversationItem, positions []int, recentStart int) (persistedMicrocompactPayload, []model.ConversationItem, bool) {
	payload, err := buildMicrocompactPayload(items, positions, recentStart)
	if err != nil || len(payload.Items) == 0 {
		return persistedMicrocompactPayload{}, nil, false
	}
	promptItems := appendPromptItems(payload.Items, items[recentStart:])
	return payload, promptItems, len(promptItems) > 0
}

func firstPayloadPosition(payload persistedMicrocompactPayload) int {
	if len(payload.SourcePositions) == 0 {
		return 0
	}
	return payload.SourcePositions[0]
}

type permissionPauseStore interface {
	UpsertPermissionRequest(context.Context, types.PermissionRequest) error
	UpsertTurnContinuation(context.Context, types.TurnContinuation) error
	UpdateTurnState(context.Context, string, types.TurnState) error
	UpdateSessionState(context.Context, string, types.SessionState, string) error
}

func persistPermissionPause(ctx context.Context, e *Engine, in Input, turnCtx *runtimegraph.TurnContext, call model.ToolCallChunk, output tools.ToolExecutionResult) error {
	store, ok := e.store.(permissionPauseStore)
	if !ok {
		return nil
	}
	payload, ok := output.Interrupt.EventPayload.(types.PermissionRequestedPayload)
	if !ok {
		return nil
	}
	now := time.Now().UTC()
	request := types.PermissionRequest{
		ID:               payload.RequestID,
		SessionID:        in.Session.ID,
		TurnID:           in.Turn.ID,
		RunID:            turnCtx.CurrentRunID,
		TaskID:           turnCtx.CurrentTaskID,
		ToolRunID:        payload.ToolRunID,
		ToolCallID:       firstNonEmpty(payload.ToolCallID, call.ID),
		ToolName:         firstNonEmpty(payload.ToolName, call.Name),
		RequestedProfile: payload.RequestedProfile,
		Reason:           payload.Reason,
		Status:           types.PermissionRequestStatusRequested,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	if request.ID == "" {
		request.ID = types.NewID("perm")
	}
	if err := store.UpsertPermissionRequest(ctx, request); err != nil {
		return err
	}
	continuation := types.TurnContinuation{
		ID:                  types.NewID("cont"),
		SessionID:           in.Session.ID,
		TurnID:              in.Turn.ID,
		RunID:               turnCtx.CurrentRunID,
		TaskID:              turnCtx.CurrentTaskID,
		PermissionRequestID: request.ID,
		ToolRunID:           request.ToolRunID,
		ToolCallID:          request.ToolCallID,
		ToolName:            request.ToolName,
		RequestedProfile:    request.RequestedProfile,
		Reason:              request.Reason,
		State:               types.TurnContinuationStatePending,
		CreatedAt:           now,
		UpdatedAt:           now,
	}
	if err := store.UpsertTurnContinuation(ctx, continuation); err != nil {
		return err
	}
	if err := store.UpdateTurnState(ctx, in.Turn.ID, types.TurnStateAwaitingPermission); err != nil {
		return err
	}
	return store.UpdateSessionState(ctx, in.Session.ID, types.SessionStateAwaitingPermission, in.Turn.ID)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

type pendingTaskCompletionStore interface {
	ClaimPendingTaskCompletionsForTurn(context.Context, string, string) ([]types.PendingTaskCompletion, error)
}

type pendingReportMailboxStore interface {
	ClaimPendingReportMailboxItemsForTurn(context.Context, string, string) ([]types.ReportMailboxItem, error)
}

func loadPendingTaskCompletionNotices(ctx context.Context, store ConversationStore, sessionID, turnID string) ([]string, error) {
	claimStore, ok := store.(pendingTaskCompletionStore)
	if !ok || strings.TrimSpace(sessionID) == "" || strings.TrimSpace(turnID) == "" {
		return nil, nil
	}
	completions, err := claimStore.ClaimPendingTaskCompletionsForTurn(ctx, sessionID, turnID)
	if err != nil {
		return nil, err
	}
	return buildPendingTaskCompletionNotices(completions), nil
}

func loadPendingReportMailboxItems(ctx context.Context, store ConversationStore, sessionID, turnID string) ([]types.ReportMailboxItem, error) {
	claimStore, ok := store.(pendingReportMailboxStore)
	if !ok || strings.TrimSpace(sessionID) == "" || strings.TrimSpace(turnID) == "" {
		return nil, nil
	}
	return claimStore.ClaimPendingReportMailboxItemsForTurn(ctx, sessionID, turnID)
}

func buildPendingTaskCompletionNotices(completions []types.PendingTaskCompletion) []string {
	if len(completions) == 0 {
		return nil
	}
	notices := make([]string, 0, len(completions))
	for _, completion := range completions {
		taskID := firstNonEmpty(completion.TaskID, completion.ID)
		if taskID == "" {
			continue
		}
		lines := []string{
			"[Child task completion available]",
			"Task ID: " + taskID,
		}
		if completion.TaskType != "" {
			lines = append(lines, "Task type: "+completion.TaskType)
		}
		if completion.ParentTurnID != "" {
			lines = append(lines, "Origin turn: "+completion.ParentTurnID)
		}
		if !completion.ObservedAt.IsZero() {
			lines = append(lines, "Observed at: "+completion.ObservedAt.UTC().Format(time.RFC3339))
		}
		if preview := firstNonEmpty(completion.ResultPreview, completion.ResultText); preview != "" {
			lines = append(lines, "Result preview:")
			lines = append(lines, preview)
		}
		lines = append(lines, "Use task_result with this task_id if you need the full final result.")
		notices = append(notices, strings.Join(lines, "\n"))
	}
	return notices
}

func pendingTaskCompletionPromptSection(notices []string) string {
	if len(notices) == 0 {
		return ""
	}
	return "Pending child task completions:\n\n" + strings.Join(notices, "\n\n")
}

func pendingReportMailboxPromptSection(items []types.ReportMailboxItem) string {
	if len(items) == 0 {
		return ""
	}

	sections := make([]string, 0, len(items))
	for _, item := range items {
		lines := []string{"[Pending report delivered by runtime]"}
		if item.ID != "" {
			lines = append(lines, "Mailbox ID: "+item.ID)
		}
		if item.SourceKind != "" {
			lines = append(lines, "Source kind: "+string(item.SourceKind))
		}
		if item.SourceID != "" {
			lines = append(lines, "Source id: "+item.SourceID)
		}
		if !item.ObservedAt.IsZero() {
			lines = append(lines, "Observed at: "+item.ObservedAt.UTC().Format(time.RFC3339))
		}
		if severity := strings.TrimSpace(item.Envelope.Severity); severity != "" {
			lines = append(lines, "Severity: "+severity)
		}
		if title := strings.TrimSpace(item.Envelope.Title); title != "" {
			lines = append(lines, "Title: "+title)
		}
		if summary := strings.TrimSpace(item.Envelope.Summary); summary != "" {
			lines = append(lines, "Summary:")
			lines = append(lines, summary)
		}
		for _, section := range item.Envelope.Sections {
			if title := strings.TrimSpace(section.Title); title != "" {
				lines = append(lines, "Section: "+title)
			}
			if text := strings.TrimSpace(section.Text); text != "" {
				lines = append(lines, text)
			}
			if len(section.Items) > 0 {
				for _, entry := range section.Items {
					entry = strings.TrimSpace(entry)
					if entry == "" {
						continue
					}
					lines = append(lines, "- "+entry)
				}
			}
		}
		lines = append(lines, "This report arrived asynchronously. Use it as background context for the user's current turn when relevant.")
		sections = append(sections, strings.Join(lines, "\n"))
	}

	return "Pending reports delivered for this turn:\n\n" + strings.Join(sections, "\n\n")
}

func lastPayloadPosition(payload persistedMicrocompactPayload) int {
	if len(payload.SourcePositions) == 0 {
		return 0
	}
	return payload.SourcePositions[len(payload.SourcePositions)-1]
}

func buildToolSchemas(defs []tools.Definition) []model.ToolSchema {
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

type turnToolState struct {
	ActiveSkills     []skills.ActivatedSkill
	Decision         toolrouter.Decision
	VisibleDefs      []tools.Definition
	VisibleToolNames []string
}

func resolveTurnToolState(
	plan intent.Plan,
	toolRuntime *tools.Runtime,
	toolExecCtx tools.ExecContext,
	resolution skills.Resolution,
) turnToolState {
	activeSkills := append([]skills.ActivatedSkill(nil), resolution.Activated...)
	decision := toolrouter.Decide(plan, resolution)
	visibleDefs := decision.FilterDefinitions(toolRuntime.VisibleDefinitions(toolExecCtx))
	visibleToolNames := make([]string, 0, len(visibleDefs))
	for _, def := range visibleDefs {
		visibleToolNames = append(visibleToolNames, def.Name)
	}
	return turnToolState{
		ActiveSkills:     activeSkills,
		Decision:         decision,
		VisibleDefs:      visibleDefs,
		VisibleToolNames: visibleToolNames,
	}
}

type pendingIntentConfirmationStore interface {
	UpsertIntentConfirmation(context.Context, types.IntentConfirmation) error
	GetIntentConfirmation(context.Context, string) (types.IntentConfirmation, bool, error)
	DeleteIntentConfirmation(context.Context, string) error
}

type pendingConfirmationResolution int

const (
	pendingConfirmationKeep pendingConfirmationResolution = iota
	pendingConfirmationResolved
	pendingConfirmationReplaced
)

func resolveIntentPlan(ctx context.Context, store ConversationStore, sessionID, userMessage string, catalog skills.Catalog) (intent.Plan, error) {
	if confirmationStore, ok := store.(pendingIntentConfirmationStore); ok {
		pending, found, err := confirmationStore.GetIntentConfirmation(ctx, sessionID)
		if err != nil {
			return intent.Plan{}, err
		}
		if found {
			plan, resolution := resolvePendingIntentConfirmation(pending, userMessage, catalog)
			if resolution == pendingConfirmationResolved || resolution == pendingConfirmationReplaced {
				if err := confirmationStore.DeleteIntentConfirmation(ctx, sessionID); err != nil {
					return intent.Plan{}, err
				}
			}
			return plan, nil
		}
	}
	return intent.Resolve(intent.Scan(userMessage, catalog)), nil
}

func resolvePendingIntentConfirmation(pending types.IntentConfirmation, reply string, catalog skills.Catalog) (intent.Plan, pendingConfirmationResolution) {
	plan := intent.Resolve(intent.Scan(pending.RawMessage, catalog))
	if strings.TrimSpace(pending.ConfirmText) != "" {
		plan.ConfirmText = pending.ConfirmText
	}
	trimmedReply := strings.TrimSpace(reply)
	reply = strings.ToLower(trimmedReply)
	switch {
	case reply == "", reply == "?" || reply == "不确定":
		plan.NeedsConfirm = true
		return plan, pendingConfirmationKeep
	case strings.Contains(reply, "automation"), strings.Contains(reply, "自动化"), strings.Contains(reply, "脚本"), strings.Contains(reply, "第一个"), reply == "1", reply == "yes", reply == "是":
		plan.NeedsConfirm = false
		if strings.TrimSpace(pending.RecommendedProfile) != "" {
			plan.Profile = intent.CapabilityProfile(pending.RecommendedProfile)
		}
		return plan, pendingConfirmationResolved
	case strings.Contains(reply, "schedule"), strings.Contains(reply, "scheduled"), strings.Contains(reply, "定时"), strings.Contains(reply, "第二个"), reply == "2":
		plan.NeedsConfirm = false
		if strings.TrimSpace(pending.FallbackProfile) != "" {
			plan.Profile = intent.CapabilityProfile(pending.FallbackProfile)
		} else {
			plan.Profile = intent.ProfileScheduledReport
		}
		return plan, pendingConfirmationResolved
	default:
		if looksLikeUnrelatedIntentRequest(trimmedReply, catalog) {
			return intent.Resolve(intent.Scan(trimmedReply, catalog)), pendingConfirmationReplaced
		}
		plan.NeedsConfirm = true
		return plan, pendingConfirmationKeep
	}
}

func looksLikeUnrelatedIntentRequest(reply string, catalog skills.Catalog) bool {
	reply = strings.TrimSpace(reply)
	if reply == "" {
		return false
	}
	lower := strings.ToLower(reply)
	for _, marker := range []string{
		"帮我", "看下", "看看", "解释", "说明", "写", "改", "修", "查", "打开", "创建",
		"review", "fix", "implement", "check", "show", "inspect", "explain", "open", "create",
	} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	signal := intent.Scan(reply, catalog)
	if len(signal.ExplicitSkills) > 0 || len(signal.NameMatches) > 0 {
		return true
	}
	for flag, score := range signal.Strength {
		if flag == intent.FlagCodeEdit && score == 1 {
			continue
		}
		if score > 0 {
			return true
		}
	}
	return false
}

func emitPlanningConfirmationTurn(
	ctx context.Context,
	e *Engine,
	in Input,
	emit func(string, any) error,
	totalItems int,
	sessionID string,
	plan intent.Plan,
) error {
	if confirmationStore, ok := e.store.(pendingIntentConfirmationStore); ok {
		pending := types.IntentConfirmation{
			SessionID:          sessionID,
			SourceTurnID:       in.Turn.ID,
			RawMessage:         plan.Signal.Raw,
			ConfirmText:        plan.ConfirmText,
			RecommendedProfile: string(plan.Profile),
			FallbackProfile:    fallbackConfirmationProfile(plan),
		}
		if err := confirmationStore.UpsertIntentConfirmation(ctx, pending); err != nil {
			return err
		}
	}

	nextPosition := totalItems + 1
	if in.Resume == nil {
		if err := persistConversationItem(ctx, e.store, sessionID, in.Turn.ID, nextPosition, model.UserMessageItem(in.Turn.UserMessage)); err != nil {
			return err
		}
		nextPosition++
	}
	if err := emit(types.EventAssistantStarted, struct{}{}); err != nil {
		return err
	}
	if text := strings.TrimSpace(plan.ConfirmText); text != "" {
		if err := persistConversationItem(ctx, e.store, sessionID, in.Turn.ID, nextPosition, model.ConversationItem{
			Kind: model.ConversationItemAssistantText,
			Text: text,
		}); err != nil {
			return err
		}
		if err := emit(types.EventAssistantDelta, types.AssistantDeltaPayload{Text: text}); err != nil {
			return err
		}
	}
	return finalizeTurn(ctx, e, in, nil)
}

func fallbackConfirmationProfile(plan intent.Plan) string {
	if plan.Signal.Flags[intent.FlagScheduling] {
		return string(intent.ProfileScheduledReport)
	}
	return string(plan.Fallback)
}

func appendTurnSupplementalPrompts(text string, completionNotices []string, reportMailboxItems []types.ReportMailboxItem) string {
	text = strings.TrimSpace(text)
	if completionPrompt := pendingTaskCompletionPromptSection(completionNotices); strings.TrimSpace(completionPrompt) != "" {
		if text == "" {
			text = completionPrompt
		} else {
			text += "\n\n" + completionPrompt
		}
	}
	if reportPrompt := pendingReportMailboxPromptSection(reportMailboxItems); strings.TrimSpace(reportPrompt) != "" {
		if text == "" {
			text = reportPrompt
		} else {
			text += "\n\n" + reportPrompt
		}
	}
	return text
}

func activatedSkillNamesFromMetadata(metadata map[string]any) []string {
	if len(metadata) == 0 {
		return nil
	}
	raw, ok := metadata["activated_skill_names"]
	if !ok || raw == nil {
		return nil
	}
	switch typed := raw.(type) {
	case []string:
		return append([]string(nil), typed...)
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			name, _ := item.(string)
			name = strings.TrimSpace(name)
			if name != "" {
				out = append(out, name)
			}
		}
		return out
	default:
		return nil
	}
}

func activatedSkillNames(activated []skills.ActivatedSkill) []string {
	if len(activated) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(activated))
	names := make([]string, 0, len(activated))
	for _, item := range activated {
		name := strings.TrimSpace(item.Skill.Name)
		if name == "" {
			continue
		}
		key := strings.ToLower(name)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		names = append(names, name)
	}
	return names
}

func loadActivatedSkillEnv(globalConfigRoot string, activated []skills.ActivatedSkill) (map[string]string, error) {
	if len(activated) == 0 {
		return nil, nil
	}
	names := make([]string, 0, len(activated))
	for _, item := range activated {
		name := strings.TrimSpace(item.Skill.Name)
		if name != "" {
			names = append(names, name)
		}
	}
	return config.MergedSkillEnv(globalConfigRoot, names)
}

func marshalStructuredToolResult(value any) string {
	if value == nil {
		return ""
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	return string(raw)
}

func structuredToolError(err error) any {
	var validation *types.AutomationValidationError
	if errors.As(err, &validation) {
		return validation
	}
	return nil
}

func persistConversationItem(ctx context.Context, store ConversationStore, sessionID, turnID string, position int, item model.ConversationItem) error {
	if store == nil {
		return nil
	}
	if (item.Kind == model.ConversationItemAssistantText || item.Kind == model.ConversationItemAssistantThinking) && strings.TrimSpace(item.Text) == "" {
		return nil
	}
	return store.InsertConversationItem(ctx, sessionID, turnID, position, item)
}

func flushAssistantItems(
	ctx context.Context,
	store ConversationStore,
	sessionID string,
	turnID string,
	nextPosition int,
	items []model.ConversationItem,
	cursor int,
	targetToolCallID string,
	req *model.Request,
	nativeContinuation bool,
) (int, int, error) {
	targetToolCallID = strings.TrimSpace(targetToolCallID)
	foundTarget := targetToolCallID == ""
	for cursor < len(items) {
		item := items[cursor]
		if err := persistConversationItem(ctx, store, sessionID, turnID, nextPosition, item); err != nil {
			return nextPosition, cursor, err
		}
		appendAssistantItemToRequest(req, item, nativeContinuation)
		nextPosition++
		cursor++
		if targetToolCallID != "" && item.Kind == model.ConversationItemToolCall && strings.TrimSpace(item.ToolCall.ID) == targetToolCallID {
			foundTarget = true
			break
		}
	}
	if !foundTarget {
		return nextPosition, cursor, fmt.Errorf("assistant tool call %q not found in ordered items", targetToolCallID)
	}
	return nextPosition, cursor, nil
}

func appendAssistantItemToRequest(req *model.Request, item model.ConversationItem, nativeContinuation bool) {
	if req == nil {
		return
	}
	if item.Kind == model.ConversationItemToolCall || !nativeContinuation {
		req.Items = append(req.Items, item)
	}
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
		if err := sink.FinalizeTurn(ctx, usage, finalEvents); err != nil {
			return err
		}
	} else {
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
	}

	if e != nil && e.sessionMemoryAsync {
		if e.sessionMemoryWorker != nil {
			e.sessionMemoryWorker.Enqueue(ctx, e, in)
		} else {
			startAsyncSessionMemoryRefresh(ctx, e, in)
		}
	} else {
		_ = maybeRefreshSessionMemory(ctx, e, in)
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
	return tools.PreviewText(result, 200)
}

type toolRunStore interface {
	UpsertToolRun(context.Context, types.ToolRun) error
}

func toolRunStoreFromConversationStore(store ConversationStore) toolRunStore {
	if store == nil {
		return nil
	}
	runtimeStore, ok := any(store).(toolRunStore)
	if !ok {
		return nil
	}
	return runtimeStore
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
