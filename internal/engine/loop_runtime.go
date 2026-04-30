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
	rolectx "go-agent/internal/roles"
	"go-agent/internal/skills"
	"go-agent/internal/tools"
	"go-agent/internal/types"
)

const (
	reportBodyRuneLimit       = 2000
	reportBodyTruncatedSuffix = "... [truncated]"
)

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

type queuedReportStore interface {
	ClaimQueuedReportDeliveriesForTurn(context.Context, string, string) ([]types.ReportDeliveryItem, error)
}

func normalizeTurnKind(kind types.TurnKind) types.TurnKind {
	if kind == types.TurnKindReportBatch {
		return types.TurnKindReportBatch
	}
	return types.TurnKindUserMessage
}

func loadQueuedReports(ctx context.Context, store ConversationStore, sessionID, turnID string) ([]types.ReportDeliveryItem, error) {
	claimStore, ok := store.(queuedReportStore)
	if !ok || strings.TrimSpace(sessionID) == "" || strings.TrimSpace(turnID) == "" {
		return nil, nil
	}
	return claimStore.ClaimQueuedReportDeliveriesForTurn(ctx, sessionID, turnID)
}

func buildReportPromptSection(reports []types.ReportDeliveryItem) string {
	if len(reports) == 0 {
		return ""
	}
	lines := []string{"Reports ready for review:"}
	for _, report := range reports {
		sourceID := firstNonEmpty(report.SourceID, report.ReportID, report.ID)
		if sourceID == "" {
			continue
		}
		envelope := report.Envelope
		lines = append(lines, fmt.Sprintf("- source_id=%s source=%s status=%s severity=%s title=%q observed_at=%s",
			sourceID,
			report.SourceKind,
			envelope.Status,
			envelope.Severity,
			envelope.Title,
			report.ObservedAt.UTC().Format(time.RFC3339),
		))
		if summary := strings.TrimSpace(envelope.Summary); summary != "" {
			lines = append(lines, "  summary: "+summary)
		}
	}
	return strings.Join(lines, "\n")
}

func buildReportConversationItems(reports []types.ReportDeliveryItem) []model.ConversationItem {
	if len(reports) == 0 {
		return nil
	}
	if len(reports) == 1 {
		return []model.ConversationItem{model.UserMessageItem(formatReport(reports[0]))}
	}

	items := []model.ConversationItem{model.UserMessageItem(formatReportDigest(reports))}
	for _, report := range reports {
		if len(reports) >= 6 && !isPriorityReportSeverity(report.Envelope.Severity) {
			continue
		}
		items = append(items, model.UserMessageItem(formatReport(report)))
	}
	return items
}

func formatReportDigest(reports []types.ReportDeliveryItem) string {
	sourceIDs := map[string]struct{}{}
	for _, report := range reports {
		if sourceID := firstNonEmpty(report.SourceID, report.ReportID, report.ID); sourceID != "" {
			sourceIDs[sourceID] = struct{}{}
		}
	}
	lines := []string{fmt.Sprintf("Digest: %d reports from %d sources", len(reports), len(sourceIDs))}
	for _, report := range reports {
		envelope := report.Envelope
		sourceID := firstNonEmpty(report.SourceID, report.ReportID, report.ID)
		line := fmt.Sprintf("- source_id=%s severity=%s status=%s title=%q",
			sourceID,
			envelope.Severity,
			envelope.Status,
			envelope.Title,
		)
		if summary := strings.TrimSpace(envelope.Summary); summary != "" {
			line += " summary: " + summary
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func formatReport(report types.ReportDeliveryItem) string {
	envelope := report.Envelope
	title := firstNonEmpty(envelope.Title, report.SourceID, report.ReportID, "Report")
	lines := []string{
		fmt.Sprintf("--- Report: %s ---", title),
		"Source: " + string(report.SourceKind),
		"Status: " + envelope.Status,
		"Severity: " + envelope.Severity,
	}
	if sourceID := firstNonEmpty(report.SourceID, report.ReportID, report.ID); sourceID != "" {
		lines = append(lines, "Source ID: "+sourceID)
	}
	if summary := strings.TrimSpace(envelope.Summary); summary != "" {
		lines = append(lines, "Summary: "+summary)
	}
	for _, section := range envelope.Sections {
		if line := formatReportSection(section); line != "" {
			lines = append(lines, line)
		}
	}
	return strings.Join(lines, "\n")
}

func formatReportSection(section types.ReportSectionContent) string {
	text := clampReportBody(section.Text)
	if text == "" {
		return ""
	}
	return firstNonEmpty(section.Title, "Details") + ": " + text
}

func isPriorityReportSeverity(severity string) bool {
	switch strings.ToLower(strings.TrimSpace(severity)) {
	case "warning", "error":
		return true
	default:
		return false
	}
}

func clampReportBody(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	runes := []rune(text)
	if len(runes) <= reportBodyRuneLimit {
		return text
	}
	suffixRunes := []rune(reportBodyTruncatedSuffix)
	keep := reportBodyRuneLimit - len(suffixRunes)
	if keep < 0 {
		keep = 0
	}
	return strings.TrimSpace(string(runes[:keep])) + reportBodyTruncatedSuffix
}

func appendReportPromptSection(text string, reports []types.ReportDeliveryItem) string {
	text = strings.TrimSpace(text)
	section := strings.TrimSpace(buildReportPromptSection(reports))
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
	pos, ok, err := store.MaxConversationPosition(ctx, sessionID)
	if err != nil {
		return 0, err
	}
	if !ok {
		return 1, nil
	}
	return pos + 1, nil
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
	if item.Kind == model.ConversationItemAssistantText && strings.TrimSpace(item.Text) == "" {
		return nil
	}
	if item.Kind == model.ConversationItemAssistantThinking && strings.TrimSpace(item.Text) == "" && strings.TrimSpace(item.ThinkingSignature) == "" {
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

type turnCostStore interface {
	UpsertTurnCost(context.Context, types.TurnCost) error
}

func persistTurnCost(ctx context.Context, store ConversationStore, in Input, usage *types.TurnUsage) error {
	if usage == nil {
		return nil
	}
	costStore, ok := store.(turnCostStore)
	if !ok {
		return nil
	}
	return costStore.UpsertTurnCost(ctx, types.TurnCost{
		TurnID:       in.Turn.ID,
		SessionID:    usage.SessionID,
		OwnerRoleID:  rolectx.SpecialistRoleIDFromContext(ctx),
		InputTokens:  usage.InputTokens,
		OutputTokens: usage.OutputTokens,
		CostUSD:      usage.CostUSD,
		CreatedAt:    usage.CreatedAt,
	})
}

func buildTurnUsage(hasUsage bool, turnID, sessionID, provider, modelName string, inputTokens, outputTokens, cachedTokens int, costUSD float64) *types.TurnUsage {
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
		CostUSD:      costUSD,
		CacheHitRate: cacheHitRate,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
}

func finalizeTurn(ctx context.Context, e *Engine, in Input, usage *types.TurnUsage, parentReplyCommitted *types.ParentReplyCommittedPayload) error {
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

	if parentReplyCommitted != nil {
		committedEvent, err := types.NewEvent(in.Session.ID, in.Turn.ID, types.EventParentReplyCommitted, *parentReplyCommitted)
		if err != nil {
			return err
		}
		finalEvents = append(finalEvents, committedEvent)
	}

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
	if e != nil && e.store != nil {
		if err := persistTurnCost(context.WithoutCancel(ctx), e.store, in, usage); err != nil {
			return err
		}
		if _, err := e.store.DeleteTurnCheckpoints(context.WithoutCancel(ctx), in.Turn.ID); err != nil {
			return err
		}
	}

	if e != nil && e.contextHeadSummaryAsync {
		if e.contextHeadSummaryWorker != nil {
			e.contextHeadSummaryWorker.Enqueue(ctx, e, in)
		} else {
			startAsyncContextHeadSummaryRefresh(ctx, e, in)
		}
	} else {
		_ = maybeRefreshContextHeadSummary(ctx, e, in)
	}
	return nil
}

func buildParentReplyCommittedPayload(
	ctx context.Context,
	store ConversationStore,
	session types.Session,
	turn types.Turn,
	contextHeadID string,
	nextPositionBeforeFlush int,
	orderedAssistantItems []model.ConversationItem,
	reports []types.ReportDeliveryItem,
) (*types.ParentReplyCommittedPayload, error) {
	if store == nil {
		return nil, nil
	}

	var builder strings.Builder
	lastAssistantTextPosition := 0
	for i, item := range orderedAssistantItems {
		if item.Kind != model.ConversationItemAssistantText || strings.TrimSpace(item.Text) == "" {
			continue
		}
		builder.WriteString(item.Text)
		lastAssistantTextPosition = nextPositionBeforeFlush + i
	}

	text := builder.String()
	if strings.TrimSpace(text) == "" || lastAssistantTextPosition == 0 {
		return nil, nil
	}

	if strings.TrimSpace(contextHeadID) == "" {
		return nil, fmt.Errorf("resolve committed parent reply item id: context head id is empty")
	}

	itemID, ok, err := store.GetConversationItemIDByContextHeadAndPosition(ctx, session.ID, contextHeadID, lastAssistantTextPosition)
	if err != nil {
		return nil, fmt.Errorf("resolve committed parent reply item id: %w", err)
	}
	if !ok || itemID == 0 {
		return nil, fmt.Errorf("resolve committed parent reply item id: position %d not found", lastAssistantTextPosition)
	}

	return &types.ParentReplyCommittedPayload{
		WorkspaceRoot:       session.WorkspaceRoot,
		SessionID:           session.ID,
		TurnID:              turn.ID,
		TurnKind:            normalizeTurnKind(turn.Kind),
		SourceParentTurnIDs: reportSourceTurnIDs(reports),
		SourceTaskIDs:       reportTaskIDs(reports),
		ItemID:              itemID,
		Text:                text,
		CreatedAt:           time.Now().UTC().Format(time.RFC3339),
	}, nil
}

func reportSourceTurnIDs(reports []types.ReportDeliveryItem) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, report := range reports {
		turnID := strings.TrimSpace(report.SourceTurnID)
		if turnID == "" {
			continue
		}
		if _, ok := seen[turnID]; ok {
			continue
		}
		seen[turnID] = struct{}{}
		out = append(out, turnID)
	}
	return out
}

func reportTaskIDs(reports []types.ReportDeliveryItem) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, report := range reports {
		taskID := strings.TrimSpace(report.SourceID)
		if report.SourceKind != types.ReportSourceTaskResult {
			taskID = ""
		}
		if taskID == "" {
			continue
		}
		if _, ok := seen[taskID]; ok {
			continue
		}
		seen[taskID] = struct{}{}
		out = append(out, taskID)
	}
	return out
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

func coldIndexStoreFromConversationStore(store ConversationStore) tools.ColdIndexStore {
	if store == nil {
		return nil
	}
	coldStore, ok := any(store).(tools.ColdIndexStore)
	if !ok {
		return nil
	}
	return coldStore
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
