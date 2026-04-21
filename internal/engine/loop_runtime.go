package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"go-agent/internal/config"
	"go-agent/internal/model"
	"go-agent/internal/runtimegraph"
	"go-agent/internal/skills"
	"go-agent/internal/tools"
	"go-agent/internal/types"
)

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

func buildTurnUsage(hasUsage bool, turnID, sessionID, provider, modelName string, inputTokens, outputTokens, cachedTokens int) *types.TurnUsage {
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
		Model:        modelName,
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
