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
	"go-agent/internal/model"
	"go-agent/internal/permissions"
	"go-agent/internal/runtimegraph"
	"go-agent/internal/sessionrole"
	"go-agent/internal/skills"
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
	turnMessage := effectiveTurnMessage(in.Turn)
	turnCtx := &runtimegraph.TurnContext{
		CurrentSessionID: sessionID,
		CurrentTurnID:    in.Turn.ID,
		CurrentTaskID:    strings.TrimSpace(in.TaskID),
	}
	if in.Resume != nil {
		turnCtx.CurrentRunID = strings.TrimSpace(in.Resume.RunID)
		if turnCtx.CurrentTaskID == "" {
			turnCtx.CurrentTaskID = strings.TrimSpace(in.Resume.TaskID)
		}
	}
	permissionEngine := effectivePermissionEngine(e.permission, in)
	toolExecCtx := tools.ExecContext{
		WorkspaceRoot:            in.Session.WorkspaceRoot,
		GlobalConfigRoot:         e.globalConfigRoot,
		PermissionEngine:         permissionEngine,
		AutomationService:        e.automationService,
		SessionDelegationService: e.sessionDelegationService,
		TaskManager:              e.taskManager,
		RuntimeService:           e.runtimeService,
		SchedulerService:         e.schedulerService,
		TurnContext:              turnCtx,
		EventSink:                in.Sink,
	}
	toolRuntime := tools.NewRuntime(e.registry, toolRunStoreFromConversationStore(e.store))

	_, working, err := loadConversationState(ctx, e, in, sessionID, turnMessage)
	if err != nil {
		if emitErr := emitFailed(err.Error()); emitErr != nil {
			return errors.Join(err, emitErr)
		}
		return err
	}
	turnKind := normalizeTurnKind(in.Turn.Kind)
	var childReports []types.ChildReport
	if turnKind == types.TurnKindChildReportBatch {
		childReports, err = loadPendingChildReports(ctx, e.store, sessionID, in.Turn.ID)
		if err != nil {
			if emitErr := emitFailed(err.Error()); emitErr != nil {
				return errors.Join(err, emitErr)
			}
			return err
		}
	}
	nextPosition, err := nextConversationPosition(ctx, e.store, sessionID)
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
	baseInstructionsText := runtimeInstructions.Text
	resolution := skills.Resolve(turnMessage, catalog, sessionrole.MergeActivatedSkillNames(in.ActivatedSkillNames, in.SessionRole))
	visibleDefs := toolRuntime.VisibleDefinitions(toolExecCtx)
	activeSkills := append([]skills.ActivatedSkill(nil), resolution.Activated...)
	toolExecCtx.ActiveSkillNames = activatedSkillNames(activeSkills)
	toolExecCtx.InjectedEnv, err = loadActivatedSkillEnv(e.globalConfigRoot, activeSkills)
	if err != nil {
		if emitErr := emitFailed(err.Error()); emitErr != nil {
			return errors.Join(err, emitErr)
		}
		return err
	}
	instructionBundle := instructions.Compile(instructions.CompileInput{
		BaseText:     baseInstructionsText,
		Catalog:      catalog,
		Message:      turnMessage,
		ActiveSkills: activeSkills,
	})
	runtimeInstructions.Text = appendChildReportPromptSection(instructionBundle.Render(), childReports)
	runtimeInstructions.Notices = append(runtimeInstructions.Notices, instructionBundle.Notices...)
	req := e.runtime.PrepareRequest(
		working,
		cacheHead,
		caps,
		turnEntryUserItem(in),
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
	toolSteps := 0
	totalInputTokens := 0
	totalOutputTokens := 0
	totalCachedTokens := 0
	hasUsage := false

	if in.Resume == nil && turnKind == types.TurnKindUserMessage {
		userItem := model.UserMessageItem(in.Turn.UserMessage)
		if err := persistConversationItem(ctx, e.store, sessionID, in.Turn.ContextHeadID, in.Turn.ID, nextPosition, userItem); err != nil {
			if emitErr := emitFailed(err.Error()); emitErr != nil {
				return errors.Join(err, emitErr)
			}
			return err
		}
		nextPosition++
	} else if in.Resume != nil {
		toolResultItem, toolResult := resumeToolResultItem(in.Resume)
		if err := persistConversationItem(ctx, e.store, sessionID, in.Turn.ContextHeadID, in.Turn.ID, nextPosition, toolResultItem); err != nil {
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
			nextPosition, _, err = flushAssistantItems(ctx, e.store, sessionID, in.Turn.ContextHeadID, in.Turn.ID, nextPosition, orderedAssistantItems, 0, "", &req, nativeContinuation)
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
					ToolCallID:        call.ID,
					ToolName:          call.Name,
					Arguments:         marshalToolArguments(call.Input),
					ArgumentsRaw:      strings.TrimSpace(call.InputRaw),
					ArgumentsRecovery: strings.TrimSpace(call.InputRecovery),
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
				nextPosition, assistantCursor, err = flushAssistantItems(ctx, e.store, sessionID, in.Turn.ContextHeadID, in.Turn.ID, nextPosition, orderedAssistantItems, assistantCursor, call.ID, &req, nativeContinuation)
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
					ToolCallID:        call.ID,
					ToolName:          call.Name,
					Arguments:         marshalToolArguments(call.Input),
					ArgumentsRaw:      strings.TrimSpace(call.InputRaw),
					ArgumentsRecovery: strings.TrimSpace(call.InputRecovery),
					ResultPreview:     previewToolResult(toolResultText),
					IsError:           toolIsError,
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
					if err := persistConversationItem(ctx, e.store, sessionID, in.Turn.ContextHeadID, in.Turn.ID, nextPosition, toolResultItem); err != nil {
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
					if err := persistConversationItem(ctx, e.store, sessionID, in.Turn.ContextHeadID, in.Turn.ID, nextPosition, item); err != nil {
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
						visibleDefs = toolRuntime.VisibleDefinitions(toolExecCtx)
						req.Tools = buildToolSchemas(visibleDefs)
						req.Instructions = appendChildReportPromptSection(instructions.Compile(instructions.CompileInput{
							BaseText:     baseInstructionsText,
							Catalog:      catalog,
							Message:      turnMessage,
							ActiveSkills: activeSkills,
						}).Render(), childReports)
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
			nextPosition, _, err = flushAssistantItems(ctx, e.store, sessionID, in.Turn.ContextHeadID, in.Turn.ID, nextPosition, orderedAssistantItems, assistantCursor, "", &req, nativeContinuation)
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

func effectiveTurnMessage(turn types.Turn) string {
	if normalizeTurnKind(turn.Kind) == types.TurnKindChildReportBatch {
		return "Review the child reports and continue the main conversation."
	}
	return turn.UserMessage
}

func turnEntryUserItem(in Input) model.ConversationItem {
	if in.Resume != nil {
		return model.ConversationItem{}
	}
	return model.UserMessageItem(effectiveTurnMessage(in.Turn))
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

func loadConversationState(ctx context.Context, e *Engine, in Input, sessionID string, turnMessage string) (int, contextstate.WorkingSet, error) {
	if e.store == nil || e.ctxManager == nil {
		return 0, contextstate.WorkingSet{}, nil
	}

	contextHeadID, err := resolveConversationReadContextHeadID(ctx, e.store, in.Turn.ContextHeadID)
	if err != nil {
		return 0, contextstate.WorkingSet{}, err
	}
	items, err := loadPromptItemsForHead(ctx, e.store, sessionID, contextHeadID)
	if err != nil {
		return 0, contextstate.WorkingSet{}, err
	}
	totalItems := len(items)

	summaryBundle, compactions, err := loadHeadMemoryBundle(ctx, e.store, sessionID, contextHeadID)
	if err != nil {
		return 0, contextstate.WorkingSet{}, err
	}
	hasHeadMemory := summaryBundle.HeadMemory != nil
	headMemoryUpTo := loadHeadMemoryUpTo(ctx, e.store, sessionID, contextHeadID)

	entries, err := e.store.ListMemoryEntriesByWorkspace(ctx, in.Session.WorkspaceRoot)
	if err != nil {
		return 0, contextstate.WorkingSet{}, err
	}

	memoryRefs := buildMemoryRefs(entries, hasHeadMemory, in.Session.WorkspaceRoot, turnMessage)

	persistedMicroItems := activeMicrocompactItems(compactions)
	recentWindowItems, recentWindowOverride := recentRawItemsForCompactionWindow(items, compactions)
	working := e.ctxManager.Build(turnMessage, items, summaryBundle, memoryRefs)
	working = setPromptItems(working, persistedMicroItems, recentWindowItems, recentWindowOverride, turnMessage)
	if e.compactor != nil {
		working, summaryBundle, err = runCompactionPasses(
			ctx,
			e,
			sessionID,
			contextHeadID,
			turnMessage,
			items,
			summaryBundle,
			memoryRefs,
			compactions,
			recentWindowItems,
			headMemoryUpTo,
			working,
		)
		if err != nil {
			return 0, contextstate.WorkingSet{}, err
		}
	}

	return totalItems, working, nil
}

func runCompactionPasses(
	ctx context.Context,
	e *Engine,
	sessionID string,
	turnContextHeadID string,
	userMessage string,
	items []model.ConversationItem,
	summaryBundle SummaryBundle,
	memoryRefs []string,
	compactions []types.ConversationCompaction,
	recentWindowItems []model.ConversationItem,
	headMemoryUpTo int64,
	working contextstate.WorkingSet,
) (contextstate.WorkingSet, SummaryBundle, error) {
	switch working.Action.Kind {
	case contextstate.CompactionActionRolling:
		return applySummaryCompaction(
			ctx,
			e,
			sessionID,
			turnContextHeadID,
			userMessage,
			items,
			summaryBundle,
			memoryRefs,
			compactions,
			headMemoryUpTo,
			working,
			len(compactions)+1,
			types.ConversationCompactionKindRolling,
			"rolling_summary",
		)
	case contextstate.CompactionActionMicrocompact:
		nextWorking, nextBundle, nextCompactions, _, err := applyMicrocompactPass(
			ctx,
			e,
			sessionID,
			turnContextHeadID,
			userMessage,
			items,
			summaryBundle,
			memoryRefs,
			compactions,
			recentWindowItems,
			working,
		)
		if err != nil {
			return contextstate.WorkingSet{}, SummaryBundle{}, err
		}
		if !shouldApplyBoundaryCompaction(nextWorking, e.ctxManager.Config()) {
			return nextWorking, nextBundle, nil
		}
		return applySummaryCompaction(
			ctx,
			e,
			sessionID,
			turnContextHeadID,
			userMessage,
			items,
			nextBundle,
			memoryRefs,
			nextCompactions,
			headMemoryUpTo,
			nextWorking,
			len(nextCompactions)+1,
			types.ConversationCompactionKindRolling,
			"microcompact_escalated_to_rolling",
		)
	default:
		return working, summaryBundle, nil
	}
}

func applyMicrocompactPass(
	ctx context.Context,
	e *Engine,
	sessionID string,
	turnContextHeadID string,
	userMessage string,
	items []model.ConversationItem,
	summaryBundle SummaryBundle,
	memoryRefs []string,
	compactions []types.ConversationCompaction,
	recentWindowItems []model.ConversationItem,
	working contextstate.WorkingSet,
) (contextstate.WorkingSet, SummaryBundle, []types.ConversationCompaction, bool, error) {
	candidatePayload, _, ok := buildAppliedMicrocompact(items, working.Action.MicrocompactPositions, working.CompactionStart)
	if !ok {
		return working, summaryBundle, compactions, false, nil
	}
	contextHeadID, err := resolveConversationWriteContextHeadID(ctx, e.store, turnContextHeadID)
	if err != nil {
		return contextstate.WorkingSet{}, SummaryBundle{}, nil, false, err
	}
	startPosition := firstPayloadPosition(candidatePayload)
	endPosition := lastPayloadPosition(candidatePayload)
	startItemID, endItemID, err := resolveConversationCompactionItemIDBounds(ctx, e.store, sessionID, contextHeadID, startPosition, endPosition)
	if err != nil {
		return contextstate.WorkingSet{}, SummaryBundle{}, nil, false, err
	}
	if err := e.store.InsertConversationCompactionWithContextHead(ctx, types.ConversationCompaction{
		ID:              types.NewID("compact"),
		SessionID:       sessionID,
		ContextHeadID:   contextHeadID,
		Kind:            types.ConversationCompactionKindMicro,
		Generation:      len(compactions) + 1,
		StartItemID:     startItemID,
		EndItemID:       endItemID,
		StartPosition:   startPosition,
		EndPosition:     endPosition,
		SummaryPayload:  encodeMicrocompactPayload(candidatePayload),
		Reason:          "microcompact_tool_results",
		ProviderProfile: string(e.model.Capabilities().Profile),
		CreatedAt:       time.Now().UTC(),
	}); err != nil {
		return contextstate.WorkingSet{}, SummaryBundle{}, nil, false, err
	}

	nextBundle, nextCompactions, err := loadHeadMemoryBundle(ctx, e.store, sessionID, contextHeadID)
	if err != nil {
		return contextstate.WorkingSet{}, SummaryBundle{}, nil, false, err
	}
	persistedMicroItems := activeMicrocompactItems(nextCompactions)
	working = e.ctxManager.Build(userMessage, items, nextBundle, memoryRefs)
	working = setPromptItems(
		working,
		persistedMicroItems,
		recentRawItemsFromMicrocompact(items, candidatePayload.RecentStart, recentWindowItems),
		true,
		userMessage,
	)
	working.CompactionApplied = true
	return working, nextBundle, nextCompactions, true, nil
}

func shouldApplyBoundaryCompaction(working contextstate.WorkingSet, cfg contextstate.Config) bool {
	return working.EstimatedTokens > cfg.MaxEstimatedTokens
}

func applySummaryCompaction(
	ctx context.Context,
	e *Engine,
	sessionID string,
	turnContextHeadID string,
	userMessage string,
	items []model.ConversationItem,
	summaryBundle SummaryBundle,
	memoryRefs []string,
	compactions []types.ConversationCompaction,
	headMemoryUpTo int64,
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
	contextHeadID, err := resolveConversationWriteContextHeadID(ctx, e.store, turnContextHeadID)
	if err != nil {
		return contextstate.WorkingSet{}, SummaryBundle{}, err
	}
	startPosition := 0
	endPosition := cutoff
	startItemID, endItemID, err := resolveConversationCompactionItemIDBounds(ctx, e.store, sessionID, contextHeadID, startPosition, endPosition)
	if err != nil {
		return contextstate.WorkingSet{}, SummaryBundle{}, err
	}
	if err := e.store.InsertConversationCompactionWithContextHead(ctx, types.ConversationCompaction{
		ID:             types.NewID("compact"),
		SessionID:      sessionID,
		ContextHeadID:  contextHeadID,
		Kind:           kind,
		Generation:     generation,
		StartItemID:    startItemID,
		EndItemID:      endItemID,
		StartPosition:  startPosition,
		EndPosition:    endPosition,
		SummaryPayload: marshalCompactionSummary(summary),
		MetadataJSON: encodeBoundaryMetadata(newBoundaryMetadata(
			generation,
			cutoff,
			headMemoryUpTo,
			len(items),
			reason,
			string(e.model.Capabilities().Profile),
			len(activeMicrocompactItems(compactions)) > 0,
		)),
		Reason:          reason,
		ProviderProfile: string(e.model.Capabilities().Profile),
		CreatedAt:       time.Now().UTC(),
	}); err != nil {
		return contextstate.WorkingSet{}, SummaryBundle{}, err
	}

	summaryBundle, compactions, err = loadHeadMemoryBundle(ctx, e.store, sessionID, contextHeadID)
	if err != nil {
		return contextstate.WorkingSet{}, SummaryBundle{}, err
	}
	persistedMicroItems := activeMicrocompactItems(compactions)
	recentWindowItems, recentWindowOverride := recentRawItemsForCompactionWindow(items, compactions)
	working = e.ctxManager.Build(userMessage, items, summaryBundle, memoryRefs)
	working = setPromptItems(working, persistedMicroItems, recentWindowItems, recentWindowOverride, userMessage)
	working.CompactionApplied = true
	return working, summaryBundle, nil
}

func newBoundaryMetadata(generation int, cutoff int, headMemoryUpTo int64, sourceItemCount int, reason string, providerProfile string, hasRecentMicrocompact bool) types.CompactionBoundaryMetadata {
	return types.CompactionBoundaryMetadata{
		Version:               1,
		PromptLayoutVersion:   1,
		Generation:            generation,
		CompactedStart:        0,
		CompactedEnd:          cutoff,
		PreservedRecentStart:  cutoff,
		HeadMemoryUpTo:        headMemoryUpTo,
		SourceItemCount:       sourceItemCount,
		Reason:                reason,
		ProviderProfile:       providerProfile,
		HasRecentMicrocompact: hasRecentMicrocompact,
	}
}

func marshalCompactionSummary(summary model.Summary) string {
	raw, err := json.Marshal(summary)
	if err != nil {
		return summary.RangeLabel
	}
	return string(raw)
}

func setPromptItems(working contextstate.WorkingSet, carryForwardItems []model.ConversationItem, recentRawItems []model.ConversationItem, overrideRecentRaw bool, userMessage string) contextstate.WorkingSet {
	working.CarryForwardItems = cloneConversationItemsForPrompt(carryForwardItems)
	if overrideRecentRaw {
		working.RecentRawItems = cloneConversationItemsForPrompt(recentRawItems)
	}
	recentItems := working.RecentRawItems
	if len(recentItems) == 0 {
		recentItems = working.RecentItems
	}
	working.PromptItems = appendPromptItems(working.CarryForwardItems, recentItems)
	working.EstimatedTokens = contextstate.EstimatePromptTokens(userMessage, working.PromptItems, working.Summaries, working.MemoryRefs)
	return working
}

func loadHeadMemoryUpTo(ctx context.Context, store ConversationStore, sessionID, contextHeadID string) int64 {
	if store == nil || strings.TrimSpace(sessionID) == "" || strings.TrimSpace(contextHeadID) == "" {
		return 0
	}
	memory, ok, err := store.GetHeadMemory(ctx, sessionID, contextHeadID)
	if err != nil || !ok || memory.UpToItemID < 0 {
		return 0
	}
	return memory.UpToItemID
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

type pendingChildReportStore interface {
	ClaimPendingChildReportsForTurn(context.Context, string, string) ([]types.ChildReport, error)
}

func normalizeTurnKind(kind types.TurnKind) types.TurnKind {
	if kind == types.TurnKindChildReportBatch {
		return types.TurnKindChildReportBatch
	}
	return types.TurnKindUserMessage
}

func loadPendingChildReports(ctx context.Context, store ConversationStore, sessionID, turnID string) ([]types.ChildReport, error) {
	claimStore, ok := store.(pendingChildReportStore)
	if ok && strings.TrimSpace(sessionID) != "" && strings.TrimSpace(turnID) != "" {
		return claimStore.ClaimPendingChildReportsForTurn(ctx, sessionID, turnID)
	}

	legacyStore, ok := store.(pendingTaskCompletionStore)
	if !ok || strings.TrimSpace(sessionID) == "" || strings.TrimSpace(turnID) == "" {
		return nil, nil
	}
	completions, err := legacyStore.ClaimPendingTaskCompletionsForTurn(ctx, sessionID, turnID)
	if err != nil {
		return nil, err
	}
	reports := make([]types.ChildReport, len(completions))
	for i := range completions {
		reports[i] = types.ChildReport(completions[i])
	}
	return reports, nil
}

func buildChildReportPromptSection(reports []types.ChildReport) string {
	if len(reports) == 0 {
		return ""
	}
	lines := []string{"Child reports ready for parent review:"}
	for _, report := range reports {
		taskID := firstNonEmpty(report.TaskID, report.ID)
		if taskID == "" {
			continue
		}
		lines = append(lines, fmt.Sprintf("- task_id=%s status=%s source=%s objective=%q result_ready=%t observed_at=%s",
			taskID,
			report.Status,
			report.Source,
			report.Objective,
			report.ResultReady,
			report.ObservedAt.UTC().Format(time.RFC3339),
		))
		if preview := strings.TrimSpace(firstNonEmpty(report.ResultPreview, report.ResultText)); preview != "" {
			lines = append(lines, "  preview: "+preview)
		}
	}
	return strings.Join(lines, "\n")
}

func appendChildReportPromptSection(text string, reports []types.ChildReport) string {
	text = strings.TrimSpace(text)
	section := strings.TrimSpace(buildChildReportPromptSection(reports))
	if section == "" {
		return text
	}
	if text == "" {
		return section
	}
	return text + "\n\n" + section
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

func nextConversationPosition(ctx context.Context, store ConversationStore, sessionID string) (int, error) {
	if store == nil {
		return 1, nil
	}
	items, err := store.ListConversationItems(ctx, sessionID)
	if err != nil {
		return 0, err
	}
	return len(items) + 1, nil
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

func resolveConversationWriteContextHeadID(ctx context.Context, store ConversationStore, preferredContextHeadID string) (string, error) {
	if resolved := strings.TrimSpace(preferredContextHeadID); resolved != "" {
		return resolved, nil
	}
	if store == nil {
		return "", errors.New("context head id is required for head-scoped conversation writes")
	}
	current, ok, err := store.GetCurrentContextHeadID(ctx)
	if err != nil {
		return "", fmt.Errorf("load current context head id: %w", err)
	}
	current = strings.TrimSpace(current)
	if !ok || current == "" {
		return "", errors.New("context head id is required for head-scoped conversation writes")
	}
	return current, nil
}

func resolveConversationCompactionItemIDBounds(
	ctx context.Context,
	store ConversationStore,
	sessionID string,
	contextHeadID string,
	startPosition int,
	endPosition int,
) (int64, int64, error) {
	if store == nil {
		return 0, 0, errors.New("store is required to resolve compaction bounds")
	}
	timelineItems, err := store.ListConversationTimelineItemsByContextHead(ctx, sessionID, contextHeadID)
	if err != nil {
		return 0, 0, fmt.Errorf("list timeline items for context head %q: %w", contextHeadID, err)
	}

	startItemID := int64(0)
	if startPosition < 0 {
		return 0, 0, fmt.Errorf("resolve start item id at position %d: invalid negative position", startPosition)
	}
	if startPosition > 0 {
		startIndex := startPosition - 1
		if startIndex < 0 || startIndex >= len(timelineItems) {
			return 0, 0, fmt.Errorf("resolve start item id at position %d: not found in lineage timeline", startPosition)
		}
		startItemID = timelineItems[startIndex].ItemID
		if startItemID == 0 {
			return 0, 0, fmt.Errorf("resolve start item id at position %d: missing stable item id", startPosition)
		}
	}

	if endPosition <= 0 {
		return 0, 0, fmt.Errorf("resolve end item id at position %d: invalid non-positive position", endPosition)
	}
	endIndex := endPosition - 1
	if endIndex < 0 || endIndex >= len(timelineItems) {
		return 0, 0, fmt.Errorf("resolve end item id at position %d: not found in lineage timeline", endPosition)
	}
	endItemID := timelineItems[endIndex].ItemID
	if endItemID == 0 {
		return 0, 0, fmt.Errorf("resolve end item id at position %d: missing stable item id", endPosition)
	}
	return startItemID, endItemID, nil
}

func persistConversationItem(ctx context.Context, store ConversationStore, sessionID, turnContextHeadID, turnID string, position int, item model.ConversationItem) error {
	if store == nil {
		return nil
	}
	if (item.Kind == model.ConversationItemAssistantText || item.Kind == model.ConversationItemAssistantThinking) && strings.TrimSpace(item.Text) == "" {
		return nil
	}
	contextHeadID, err := resolveConversationWriteContextHeadID(ctx, store, turnContextHeadID)
	if err != nil {
		return fmt.Errorf("resolve context head id for conversation item write: %w", err)
	}
	return store.InsertConversationItemWithContextHead(ctx, sessionID, contextHeadID, turnID, position, item)
}

func flushAssistantItems(
	ctx context.Context,
	store ConversationStore,
	sessionID string,
	turnContextHeadID string,
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
		if err := persistConversationItem(ctx, store, sessionID, turnContextHeadID, turnID, nextPosition, item); err != nil {
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

	if e != nil && e.headMemoryAsync {
		if e.headMemoryWorker != nil {
			e.headMemoryWorker.Enqueue(ctx, e, in)
		} else {
			startAsyncHeadMemoryRefresh(ctx, e, in)
		}
	} else {
		_ = maybeRefreshHeadMemory(ctx, e, in)
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
