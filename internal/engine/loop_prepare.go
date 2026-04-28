package engine

import (
	"context"
	"errors"
	"fmt"
	"strings"

	contextstate "go-agent/internal/context"
	"go-agent/internal/instructions"
	"go-agent/internal/model"
	"go-agent/internal/permissions"
	rolectx "go-agent/internal/roles"
	"go-agent/internal/runtimegraph"
	"go-agent/internal/sessionrole"
	"go-agent/internal/skills"
	"go-agent/internal/tools"
	"go-agent/internal/types"
)

type loopEmitter struct {
	sessionID string
	turnID    string
	sink      EventSink
}

func newLoopEmitter(in Input) loopEmitter {
	return loopEmitter{
		sessionID: in.Session.ID,
		turnID:    in.Turn.ID,
		sink:      in.Sink,
	}
}

func (e loopEmitter) Emit(ctx context.Context, eventType string, payload any) error {
	event, err := types.NewEvent(e.sessionID, e.turnID, eventType, payload)
	if err != nil {
		return err
	}
	return e.sink.Emit(ctx, event)
}

func (e loopEmitter) Fail(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}
	emitErr := e.Emit(ctx, types.EventTurnFailed, types.TurnFailedPayload{Message: err.Error()})
	if emitErr != nil {
		return errors.Join(err, emitErr)
	}
	return err
}

type preparedLoopState struct {
	caps               model.ProviderCapabilities
	providerName       string
	usageProvider      string
	usageModel         string
	sessionID          string
	turnMessage        string
	turnKind           types.TurnKind
	reports            []types.ReportDeliveryItem
	turnCtx            *runtimegraph.TurnContext
	toolExecCtx        tools.ExecContext
	toolRuntime        *tools.Runtime
	nextPosition       int
	req                model.Request
	cacheHead          *types.ProviderCacheHead
	nativeContinuation bool
	baseInstructions   string
	skillCatalog       skills.Catalog
	activeSkills       []skills.ActivatedSkill
	visibleDefs        []tools.Definition
}

func prepareLoopState(ctx context.Context, e *Engine, in Input, emitter loopEmitter) (*preparedLoopState, error) {
	caps := e.model.Capabilities()
	providerName := providerCacheOwnerForCapabilities(caps)
	usageProvider := strings.TrimSpace(e.meta.Provider)
	if usageProvider == "" {
		usageProvider = providerName
	}
	usageModel := strings.TrimSpace(e.meta.Model)

	if err := emitter.Emit(ctx, types.EventTurnStarted, types.TurnStartedPayload{
		WorkspaceRoot: in.Session.WorkspaceRoot,
	}); err != nil {
		return nil, err
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

	toolExecCtx := tools.ExecContext{
		WorkspaceRoot:            in.Session.WorkspaceRoot,
		GlobalConfigRoot:         e.globalConfigRoot,
		PermissionEngine:         effectivePermissionEngine(e.permission, in),
		AutomationService:        e.automationService,
		RoleService:              e.roleService,
		SessionDelegationService: e.sessionDelegationService,
		TaskManager:              e.taskManager,
		RuntimeService:           e.runtimeService,
		SchedulerService:         e.schedulerService,
		ArchiveStore:             e.store,
		ColdIndexStore:           coldIndexStoreFromConversationStore(e.store),
		MemoryStore:              e.store,
		TurnContext:              turnCtx,
		EventSink:                in.Sink,
	}
	toolRuntime := tools.NewRuntime(e.registry, toolRunStoreFromConversationStore(e.store))

	_, working, err := loadConversationState(ctx, e, in, sessionID, turnMessage)
	if err != nil {
		return nil, emitter.Fail(ctx, err)
	}
	turnKind := normalizeTurnKind(in.Turn.Kind)
	var reports []types.ReportDeliveryItem
	if turnKind == types.TurnKindReportBatch {
		reports, err = loadQueuedReports(ctx, e.store, sessionID, in.Turn.ID)
		if err != nil {
			return nil, emitter.Fail(ctx, err)
		}
	}
	nextPosition, err := nextConversationPosition(ctx, e.store, sessionID)
	if err != nil {
		return nil, emitter.Fail(ctx, err)
	}

	cacheHeadValue, cacheHeadOK, err := loadProviderCacheHead(ctx, e.store, sessionID, providerName, string(caps.Profile))
	if err != nil {
		return nil, emitter.Fail(ctx, err)
	}
	var cacheHead *types.ProviderCacheHead
	if cacheHeadOK {
		cacheHead = &cacheHeadValue
	}

	roleCatalog, err := rolectx.LoadCatalog(in.Session.WorkspaceRoot)
	if err != nil {
		return nil, emitter.Fail(ctx, err)
	}
	specialistSpec, err := resolveSpecialistSpec(roleCatalog, rolectx.SpecialistRoleIDFromContext(ctx))
	if err != nil {
		return nil, emitter.Fail(ctx, err)
	}

	runtimeInstructions, baseInstructions, err := buildLoopInstructions(e, in, working, roleCatalog, specialistSpec)
	if err != nil {
		return nil, emitter.Fail(ctx, err)
	}

	skillCatalog, activeSkills, visibleDefs, injectedEnv, renderedInstructions, notices, err := buildLoopSkillsAndInstructions(
		e,
		toolRuntime,
		toolExecCtx,
		in,
		turnMessage,
		baseInstructions,
		specialistSpec,
		reports,
	)
	if err != nil {
		return nil, emitter.Fail(ctx, err)
	}
	toolExecCtx.ActiveSkillNames = activatedSkillNames(activeSkills)
	toolExecCtx.InjectedEnv = injectedEnv
	runtimeInstructions.Text = renderedInstructions
	runtimeInstructions.Notices = append(runtimeInstructions.Notices, notices...)

	req := e.runtime.PrepareRequest(
		working,
		cacheHead,
		caps,
		turnEntryUserItem(in),
		runtimeInstructions.Text,
	)
	for _, notice := range runtimeInstructions.Notices {
		if err := emitter.Emit(ctx, types.EventSystemNotice, types.NoticePayload{Text: notice}); err != nil {
			return nil, err
		}
	}
	req.Stream = true
	req.Tools = buildToolSchemas(visibleDefs)
	req.ToolChoice = "auto"

	return &preparedLoopState{
		caps:               caps,
		providerName:       providerName,
		usageProvider:      usageProvider,
		usageModel:         usageModel,
		sessionID:          sessionID,
		turnMessage:        turnMessage,
		turnKind:           turnKind,
		reports:            reports,
		turnCtx:            turnCtx,
		toolExecCtx:        toolExecCtx,
		toolRuntime:        toolRuntime,
		nextPosition:       nextPosition,
		req:                req,
		cacheHead:          cacheHead,
		nativeContinuation: req.Cache != nil && caps.Profile != model.CapabilityProfileNone,
		baseInstructions:   baseInstructions,
		skillCatalog:       skillCatalog,
		activeSkills:       activeSkills,
		visibleDefs:        visibleDefs,
	}, nil
}

func resolveSpecialistSpec(roleCatalog rolectx.Catalog, specialistRoleID string) (*rolectx.Spec, error) {
	specialistRoleID = strings.TrimSpace(specialistRoleID)
	if specialistRoleID == "" {
		return nil, nil
	}
	spec, ok := roleCatalog.ByID[specialistRoleID]
	if !ok {
		return nil, fmt.Errorf("specialist role %q is not installed", specialistRoleID)
	}
	return &spec, nil
}

func buildLoopInstructions(e *Engine, in Input, working contextstate.WorkingSet, roleCatalog rolectx.Catalog, specialistSpec *rolectx.Spec) (RuntimeInstructions, string, error) {
	runtimeSession := in.Session
	if specialistSpec != nil {
		runtimeSession.SystemPrompt = sessionrole.SpecialistSystemPrompt(*specialistSpec)
	}
	runtimeInstructions, err := buildRuntimeInstructionsWithMaxBytes(runtimeSession, e.basePrompt, working.MemoryRefs, e.maxWorkspacePromptBytes)
	if err != nil {
		return RuntimeInstructions{}, "", err
	}
	baseInstructions := runtimeInstructions.Text
	if in.SessionRole == types.SessionRoleMainParent && specialistSpec == nil {
		if registrySummary := strings.TrimSpace(rolectx.RenderRegistrySummary(roleCatalog)); registrySummary != "" {
			baseInstructions = strings.TrimSpace(baseInstructions + "\n\n" + registrySummary)
		}
	}
	return runtimeInstructions, baseInstructions, nil
}

func buildLoopSkillsAndInstructions(
	e *Engine,
	toolRuntime *tools.Runtime,
	toolExecCtx tools.ExecContext,
	in Input,
	turnMessage string,
	baseInstructions string,
	specialistSpec *rolectx.Spec,
	reports []types.ReportDeliveryItem,
) (skills.Catalog, []skills.ActivatedSkill, []tools.Definition, map[string]string, string, []string, error) {
	skillCatalog, err := skills.LoadCatalog(e.globalConfigRoot, in.Session.WorkspaceRoot)
	if err != nil {
		return skills.Catalog{}, nil, nil, nil, "", nil, err
	}
	resolution := skills.Resolve(turnMessage, skillCatalog, in.ActivatedSkillNames)
	activeSkills := append([]skills.ActivatedSkill(nil), resolution.Activated...)
	toolExecCtx.ActiveSkillNames = activatedSkillNames(activeSkills)
	visibleDefs := toolRuntime.VisibleDefinitions(toolExecCtx)
	injectedEnv, err := loadActivatedSkillEnv(e.globalConfigRoot, activeSkills)
	if err != nil {
		return skills.Catalog{}, nil, nil, nil, "", nil, err
	}
	instructionBundle := instructions.Render(instructions.RenderInput{
		BaseText:     baseInstructions,
		Catalog:      skillCatalog,
		Message:      turnMessage,
		ActiveSkills: activeSkills,
	})
	renderedInstructions := appendReportPromptSection(instructionBundle.Render(), reports)
	return skillCatalog, activeSkills, visibleDefs, injectedEnv, renderedInstructions, instructionBundle.Notices, nil
}

func persistInitialLoopItems(ctx context.Context, e *Engine, in Input, emitter loopEmitter, state *preparedLoopState) error {
	if state.turnKind == types.TurnKindUserMessage {
		userItem := model.UserMessageItem(in.Turn.UserMessage)
		if err := persistConversationItem(ctx, e.store, state.sessionID, in.Turn.ContextHeadID, in.Turn.ID, state.nextPosition, userItem); err != nil {
			return emitter.Fail(ctx, err)
		}
		state.nextPosition++
	}
	if state.turnKind == types.TurnKindReportBatch {
		reportItems := buildReportConversationItems(state.reports)
		for _, item := range reportItems {
			if err := persistConversationItem(ctx, e.store, state.sessionID, in.Turn.ContextHeadID, in.Turn.ID, state.nextPosition, item); err != nil {
				return emitter.Fail(ctx, err)
			}
			state.nextPosition++
		}
		insertReportItemsBeforeTurnEntry(&state.req, reportItems)
	}
	return nil
}

func insertReportItemsBeforeTurnEntry(req *model.Request, reportItems []model.ConversationItem) {
	if req == nil || len(reportItems) == 0 {
		return
	}
	insertAt := len(req.Items)
	if insertAt > 0 {
		insertAt--
	}
	items := make([]model.ConversationItem, 0, len(req.Items)+len(reportItems))
	items = append(items, req.Items[:insertAt]...)
	items = append(items, reportItems...)
	items = append(items, req.Items[insertAt:]...)
	req.Items = items
}

func effectivePermissionEngine(base *permissions.Engine, in Input) *permissions.Engine {
	if strings.TrimSpace(in.Session.PermissionProfile) != "" {
		return permissions.NewEngine(in.Session.PermissionProfile)
	}
	return base
}
