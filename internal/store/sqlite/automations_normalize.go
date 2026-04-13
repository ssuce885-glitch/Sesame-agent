package sqlite

import (
	"encoding/json"
	"strings"
	"time"

	"go-agent/internal/types"
)

func normalizeAutomationSpecForStore(spec types.AutomationSpec) types.AutomationSpec {
	now := time.Now().UTC()
	spec.ID = strings.TrimSpace(spec.ID)
	if spec.ID == "" {
		spec.ID = types.NewID("automation")
	}
	spec.Title = strings.TrimSpace(spec.Title)
	spec.WorkspaceRoot = strings.TrimSpace(spec.WorkspaceRoot)
	spec.Goal = strings.TrimSpace(spec.Goal)
	spec.State = normalizeAutomationStateForStore(spec.State)
	spec.Assumptions = types.NormalizeAutomationAssumptions(spec.Assumptions)

	spec.Context.Owner = strings.TrimSpace(spec.Context.Owner)
	spec.Context.Environment = strings.TrimSpace(spec.Context.Environment)
	spec.Context.Targets = normalizeAutomationStringList(spec.Context.Targets)
	labels := make(map[string]string, len(spec.Context.Labels))
	for key, value := range spec.Context.Labels {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		labels[key] = strings.TrimSpace(value)
	}
	spec.Context.Labels = labels

	signals := make([]types.AutomationSignal, 0, len(spec.Signals))
	for _, signal := range spec.Signals {
		signal.Kind = strings.TrimSpace(signal.Kind)
		signal.Source = strings.TrimSpace(signal.Source)
		signal.Selector = strings.TrimSpace(signal.Selector)
		signal.Payload = normalizeAutomationRawJSON(signal.Payload)
		if signal.Kind == "" && signal.Source == "" && signal.Selector == "" && len(signal.Payload) == 0 {
			continue
		}
		signals = append(signals, signal)
	}
	spec.Signals = signals
	spec.IncidentPolicy = normalizeAutomationObjectJSON(spec.IncidentPolicy)
	spec.ResponsePlan = normalizeAutomationResponsePlanForStore(spec.ResponsePlan)
	spec.VerificationPlan = normalizeAutomationObjectJSON(spec.VerificationPlan)
	spec.EscalationPolicy = normalizeAutomationObjectJSON(spec.EscalationPolicy)
	spec.DeliveryPolicy = normalizeAutomationRawJSON(spec.DeliveryPolicy)
	spec.RuntimePolicy = normalizeAutomationRawJSON(spec.RuntimePolicy)
	spec.WatcherLifecycle = normalizeAutomationObjectJSON(spec.WatcherLifecycle)
	spec.RetriggerPolicy = normalizeAutomationObjectJSON(spec.RetriggerPolicy)
	spec.RunPolicy = normalizeAutomationObjectJSON(spec.RunPolicy)

	if spec.CreatedAt.IsZero() {
		spec.CreatedAt = now
	} else {
		spec.CreatedAt = spec.CreatedAt.UTC()
	}
	if spec.UpdatedAt.IsZero() {
		spec.UpdatedAt = spec.CreatedAt
	} else {
		spec.UpdatedAt = spec.UpdatedAt.UTC()
	}
	if spec.UpdatedAt.Before(spec.CreatedAt) {
		spec.UpdatedAt = spec.CreatedAt
	}
	return spec
}

func normalizeAutomationIncidentForStore(incident types.AutomationIncident) types.AutomationIncident {
	now := time.Now().UTC()
	incident.ID = strings.TrimSpace(incident.ID)
	if incident.ID == "" {
		incident.ID = types.NewID("incident")
	}
	incident.AutomationID = strings.TrimSpace(incident.AutomationID)
	incident.WorkspaceRoot = strings.TrimSpace(incident.WorkspaceRoot)
	incident.Status = types.AutomationIncidentStatus(strings.ToLower(strings.TrimSpace(string(incident.Status))))
	if incident.Status == "" {
		incident.Status = types.AutomationIncidentStatusOpen
	}
	incident.SignalKind = strings.TrimSpace(incident.SignalKind)
	incident.Source = strings.TrimSpace(incident.Source)
	incident.Summary = strings.TrimSpace(incident.Summary)
	incident.Payload = normalizeAutomationRawJSON(incident.Payload)
	if incident.ObservedAt.IsZero() {
		incident.ObservedAt = now
	} else {
		incident.ObservedAt = incident.ObservedAt.UTC()
	}
	if incident.CreatedAt.IsZero() {
		incident.CreatedAt = now
	} else {
		incident.CreatedAt = incident.CreatedAt.UTC()
	}
	if incident.UpdatedAt.IsZero() {
		incident.UpdatedAt = incident.CreatedAt
	} else {
		incident.UpdatedAt = incident.UpdatedAt.UTC()
	}
	if incident.UpdatedAt.Before(incident.CreatedAt) {
		incident.UpdatedAt = incident.CreatedAt
	}
	return incident
}

func normalizeAutomationHeartbeatForStore(heartbeat types.AutomationHeartbeat) types.AutomationHeartbeat {
	now := time.Now().UTC()
	heartbeat.AutomationID = strings.TrimSpace(heartbeat.AutomationID)
	heartbeat.WatcherID = strings.TrimSpace(heartbeat.WatcherID)
	heartbeat.WorkspaceRoot = strings.TrimSpace(heartbeat.WorkspaceRoot)
	heartbeat.Status = strings.TrimSpace(heartbeat.Status)
	heartbeat.Payload = normalizeAutomationRawJSON(heartbeat.Payload)
	if heartbeat.ObservedAt.IsZero() {
		heartbeat.ObservedAt = now
	} else {
		heartbeat.ObservedAt = heartbeat.ObservedAt.UTC()
	}
	if heartbeat.CreatedAt.IsZero() {
		heartbeat.CreatedAt = now
	} else {
		heartbeat.CreatedAt = heartbeat.CreatedAt.UTC()
	}
	if heartbeat.UpdatedAt.IsZero() {
		heartbeat.UpdatedAt = heartbeat.CreatedAt
	} else {
		heartbeat.UpdatedAt = heartbeat.UpdatedAt.UTC()
	}
	if heartbeat.UpdatedAt.Before(heartbeat.CreatedAt) {
		heartbeat.UpdatedAt = heartbeat.CreatedAt
	}
	return heartbeat
}

func normalizeAutomationWatcherForStore(watcher types.AutomationWatcherRuntime) types.AutomationWatcherRuntime {
	now := time.Now().UTC()
	watcher.ID = strings.TrimSpace(watcher.ID)
	if watcher.ID == "" {
		watcher.ID = types.NewID("watcher")
	}
	watcher.AutomationID = strings.TrimSpace(watcher.AutomationID)
	watcher.WorkspaceRoot = strings.TrimSpace(watcher.WorkspaceRoot)
	watcher.WatcherID = strings.TrimSpace(watcher.WatcherID)
	if watcher.WatcherID == "" {
		watcher.WatcherID = watcher.ID
	}
	watcher.State = normalizeAutomationWatcherStateForStore(watcher.State)
	watcher.ScriptPath = strings.TrimSpace(watcher.ScriptPath)
	watcher.StatePath = strings.TrimSpace(watcher.StatePath)
	watcher.TaskID = strings.TrimSpace(watcher.TaskID)
	watcher.Command = strings.TrimSpace(watcher.Command)
	watcher.LastError = strings.TrimSpace(watcher.LastError)
	if watcher.CreatedAt.IsZero() {
		watcher.CreatedAt = now
	} else {
		watcher.CreatedAt = watcher.CreatedAt.UTC()
	}
	if watcher.UpdatedAt.IsZero() {
		watcher.UpdatedAt = watcher.CreatedAt
	} else {
		watcher.UpdatedAt = watcher.UpdatedAt.UTC()
	}
	if watcher.UpdatedAt.Before(watcher.CreatedAt) {
		watcher.UpdatedAt = watcher.CreatedAt
	}
	return watcher
}

func normalizeTriggerEventForStore(event types.TriggerEvent) types.TriggerEvent {
	now := time.Now().UTC()
	event.EventID = strings.TrimSpace(event.EventID)
	if event.EventID == "" {
		event.EventID = types.NewID("trigger")
	}
	event.AutomationID = strings.TrimSpace(event.AutomationID)
	event.WorkspaceRoot = strings.TrimSpace(event.WorkspaceRoot)
	event.IncidentID = strings.TrimSpace(event.IncidentID)
	event.SignalKind = strings.TrimSpace(event.SignalKind)
	event.Source = strings.TrimSpace(event.Source)
	event.Summary = strings.TrimSpace(event.Summary)
	event.Payload = normalizeAutomationRawJSON(event.Payload)
	event.DedupeKey = strings.TrimSpace(event.DedupeKey)
	if event.ObservedAt.IsZero() {
		event.ObservedAt = now
	} else {
		event.ObservedAt = event.ObservedAt.UTC()
	}
	if event.CreatedAt.IsZero() {
		event.CreatedAt = now
	} else {
		event.CreatedAt = event.CreatedAt.UTC()
	}
	if event.UpdatedAt.IsZero() {
		event.UpdatedAt = event.CreatedAt
	} else {
		event.UpdatedAt = event.UpdatedAt.UTC()
	}
	if event.UpdatedAt.Before(event.CreatedAt) {
		event.UpdatedAt = event.CreatedAt
	}
	return event
}

func normalizeIncidentPhaseStateForStore(state types.IncidentPhaseState) types.IncidentPhaseState {
	now := time.Now().UTC()
	state.IncidentID = strings.TrimSpace(state.IncidentID)
	state.AutomationID = strings.TrimSpace(state.AutomationID)
	state.WorkspaceRoot = strings.TrimSpace(state.WorkspaceRoot)
	state.Phase = normalizeAutomationPhaseNameForStore(state.Phase)
	state.Reduction = normalizeIncidentPhaseReductionForStore(state.Reduction)
	state.Status = normalizeIncidentPhaseStatusForStore(state.Status)
	state.DispatchIDs = normalizeAutomationStringList(state.DispatchIDs)
	if state.ActiveDispatchCount < 0 {
		state.ActiveDispatchCount = 0
	}
	if state.CompletedDispatchCount < 0 {
		state.CompletedDispatchCount = 0
	}
	if state.FailedDispatchCount < 0 {
		state.FailedDispatchCount = 0
	}
	if state.CreatedAt.IsZero() {
		state.CreatedAt = now
	} else {
		state.CreatedAt = state.CreatedAt.UTC()
	}
	if state.UpdatedAt.IsZero() {
		state.UpdatedAt = state.CreatedAt
	} else {
		state.UpdatedAt = state.UpdatedAt.UTC()
	}
	if state.UpdatedAt.Before(state.CreatedAt) {
		state.UpdatedAt = state.CreatedAt
	}
	return state
}

func normalizeDispatchAttemptForStore(attempt types.DispatchAttempt) types.DispatchAttempt {
	now := time.Now().UTC()
	attempt.DispatchID = strings.TrimSpace(attempt.DispatchID)
	if attempt.DispatchID == "" {
		attempt.DispatchID = types.NewID("dispatch")
	}
	attempt.IncidentID = strings.TrimSpace(attempt.IncidentID)
	attempt.AutomationID = strings.TrimSpace(attempt.AutomationID)
	attempt.WorkspaceRoot = strings.TrimSpace(attempt.WorkspaceRoot)
	attempt.Phase = normalizeAutomationPhaseNameForStore(attempt.Phase)
	attempt.Status = normalizeDispatchAttemptStatusForStore(attempt.Status)
	if attempt.Attempt <= 0 {
		attempt.Attempt = 1
	}
	attempt.TaskID = strings.TrimSpace(attempt.TaskID)
	attempt.BackgroundSessionID = strings.TrimSpace(attempt.BackgroundSessionID)
	attempt.BackgroundTurnID = strings.TrimSpace(attempt.BackgroundTurnID)
	attempt.ChildAgentID = strings.TrimSpace(attempt.ChildAgentID)
	attempt.PromptHash = strings.TrimSpace(attempt.PromptHash)
	attempt.ActivatedSkillNames = normalizeAutomationStringList(attempt.ActivatedSkillNames)
	attempt.OutputContractRef = strings.TrimSpace(attempt.OutputContractRef)
	attempt.ContinuationID = strings.TrimSpace(attempt.ContinuationID)
	attempt.PermissionRequestID = strings.TrimSpace(attempt.PermissionRequestID)
	attempt.ApprovalQueueKey = strings.TrimSpace(attempt.ApprovalQueueKey)
	attempt.PreferredSessionID = strings.TrimSpace(attempt.PreferredSessionID)
	attempt.Error = strings.TrimSpace(attempt.Error)
	if !attempt.StartedAt.IsZero() {
		attempt.StartedAt = attempt.StartedAt.UTC()
	}
	if !attempt.FinishedAt.IsZero() {
		attempt.FinishedAt = attempt.FinishedAt.UTC()
	}
	if attempt.CreatedAt.IsZero() {
		attempt.CreatedAt = now
	} else {
		attempt.CreatedAt = attempt.CreatedAt.UTC()
	}
	if attempt.UpdatedAt.IsZero() {
		attempt.UpdatedAt = attempt.CreatedAt
	} else {
		attempt.UpdatedAt = attempt.UpdatedAt.UTC()
	}
	if attempt.UpdatedAt.Before(attempt.CreatedAt) {
		attempt.UpdatedAt = attempt.CreatedAt
	}
	return attempt
}

func normalizeDeliveryRecordForStore(delivery types.DeliveryRecord) types.DeliveryRecord {
	now := time.Now().UTC()
	delivery.DeliveryID = strings.TrimSpace(delivery.DeliveryID)
	if delivery.DeliveryID == "" {
		delivery.DeliveryID = types.NewID("delivery")
	}
	delivery.WorkspaceRoot = strings.TrimSpace(delivery.WorkspaceRoot)
	delivery.AutomationID = strings.TrimSpace(delivery.AutomationID)
	delivery.IncidentID = strings.TrimSpace(delivery.IncidentID)
	delivery.DispatchID = strings.TrimSpace(delivery.DispatchID)
	delivery.SummaryRef = strings.TrimSpace(delivery.SummaryRef)
	delivery.Channels = normalizeDeliveryChannelsForStore(delivery.Channels)
	if delivery.CreatedAt.IsZero() {
		delivery.CreatedAt = now
	} else {
		delivery.CreatedAt = delivery.CreatedAt.UTC()
	}
	if delivery.UpdatedAt.IsZero() {
		delivery.UpdatedAt = delivery.CreatedAt
	} else {
		delivery.UpdatedAt = delivery.UpdatedAt.UTC()
	}
	if delivery.UpdatedAt.Before(delivery.CreatedAt) {
		delivery.UpdatedAt = delivery.CreatedAt
	}
	return delivery
}

func normalizeAutomationStateForStore(state types.AutomationState) types.AutomationState {
	state = types.AutomationState(strings.ToLower(strings.TrimSpace(string(state))))
	if state == "" {
		return types.AutomationStateActive
	}
	return state
}

func normalizeAutomationListFilterForStore(filter types.AutomationListFilter) types.AutomationListFilter {
	filter.WorkspaceRoot = strings.TrimSpace(filter.WorkspaceRoot)
	filter.State = types.AutomationState(strings.ToLower(strings.TrimSpace(string(filter.State))))
	if filter.Limit < 0 {
		filter.Limit = 0
	}
	return filter
}

func normalizeAutomationIncidentFilterForStore(filter types.AutomationIncidentFilter) types.AutomationIncidentFilter {
	filter.WorkspaceRoot = strings.TrimSpace(filter.WorkspaceRoot)
	filter.AutomationID = strings.TrimSpace(filter.AutomationID)
	filter.Status = types.AutomationIncidentStatus(strings.ToLower(strings.TrimSpace(string(filter.Status))))
	if filter.Limit < 0 {
		filter.Limit = 0
	}
	return filter
}

func normalizeAutomationWatcherFilterForStore(filter types.AutomationWatcherFilter) types.AutomationWatcherFilter {
	filter.WorkspaceRoot = strings.TrimSpace(filter.WorkspaceRoot)
	filter.AutomationID = strings.TrimSpace(filter.AutomationID)
	if strings.TrimSpace(string(filter.State)) != "" {
		filter.State = normalizeAutomationWatcherStateForStore(filter.State)
	}
	if filter.Limit < 0 {
		filter.Limit = 0
	}
	return filter
}

func normalizeAutomationWatcherStateForStore(state types.AutomationWatcherState) types.AutomationWatcherState {
	state = types.AutomationWatcherState(strings.ToLower(strings.TrimSpace(string(state))))
	if state == "" {
		return types.AutomationWatcherStatePending
	}
	return state
}

func normalizeTriggerEventFilterForStore(filter types.TriggerEventFilter) types.TriggerEventFilter {
	filter.WorkspaceRoot = strings.TrimSpace(filter.WorkspaceRoot)
	filter.AutomationID = strings.TrimSpace(filter.AutomationID)
	filter.IncidentID = strings.TrimSpace(filter.IncidentID)
	filter.DedupeKey = strings.TrimSpace(filter.DedupeKey)
	if filter.Limit < 0 {
		filter.Limit = 0
	}
	return filter
}

func normalizeAutomationStringList(values []string) []string {
	if len(values) == 0 {
		return []string{}
	}
	out := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
	if len(out) == 0 {
		return []string{}
	}
	return out
}

func normalizeAutomationRawJSON(raw json.RawMessage) json.RawMessage {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" {
		return nil
	}
	return json.RawMessage(trimmed)
}

func normalizeAutomationResponsePlanForStore(raw json.RawMessage) json.RawMessage {
	raw = normalizeAutomationRawJSON(raw)
	if len(raw) == 0 || !json.Valid(raw) {
		return raw
	}

	var object map[string]any
	if err := json.Unmarshal(raw, &object); err != nil {
		return raw
	}
	if strings.EqualFold(strings.TrimSpace(normalizeAutomationResponsePlanModeString(object["schema_version"])), types.ResponsePlanSchemaVersionV2) {
		return types.NormalizeAutomationResponsePlanJSON(raw)
	}

	mode := strings.TrimSpace(normalizeAutomationResponsePlanModeString(object["mode"]))
	refs := normalizeAutomationAnyToStringSlice(object["child_agent_template_refs"])
	switch mode {
	case "summary", "digest", "investigate_only":
		object["mode"] = "investigate"
	case "investigate_then_remediate":
		object["mode"] = "investigate_then_act"
	case "remediate_only":
		object["mode"] = "act_only"
	case "verify":
		object = map[string]any{
			"schema_version": types.ResponsePlanSchemaVersionV2,
			"phases": []any{
				legacyAutomationResponsePlanPhaseObject(string(types.AutomationPhaseVerify), "", refs, 0),
			},
		}
	case "investigate_then_remediate_then_verify", "investigate_then_act_then_verify":
		object = map[string]any{
			"schema_version": types.ResponsePlanSchemaVersionV2,
			"phases": []any{
				legacyAutomationResponsePlanPhaseObject(string(types.AutomationPhaseDiagnose), string(types.AutomationPhaseTransitionNextPhase), refs, 0),
				legacyAutomationResponsePlanPhaseObject(string(types.AutomationPhaseRemediate), string(types.AutomationPhaseTransitionNextPhase), refs, 1),
				legacyAutomationResponsePlanPhaseObject(string(types.AutomationPhaseVerify), "", refs, 2),
			},
		}
	}

	normalized, err := json.Marshal(object)
	if err != nil {
		return raw
	}
	return types.NormalizeAutomationResponsePlanJSON(normalized)
}

func normalizeAutomationObjectJSON(raw json.RawMessage) json.RawMessage {
	raw = normalizeAutomationRawJSON(raw)
	if len(raw) == 0 || string(raw) == "null" {
		return json.RawMessage("{}")
	}
	return raw
}

func normalizeAutomationResponsePlanModeString(value any) string {
	text, _ := value.(string)
	return text
}

func normalizeAutomationAnyToStringSlice(value any) []string {
	items, ok := value.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		text, _ := item.(string)
		out = append(out, text)
	}
	return normalizeAutomationStringList(out)
}

func legacyAutomationResponsePlanPhaseObject(phase string, onSuccess string, refs []string, index int) map[string]any {
	phaseObject := map[string]any{"phase": phase}
	if strings.TrimSpace(onSuccess) != "" {
		phaseObject["on_success"] = onSuccess
	}
	if ref := legacyAutomationResponsePlanRefForIndex(refs, index); ref != "" {
		phaseObject["child_agents"] = []any{
			map[string]any{"agent_id": ref},
		}
	}
	return phaseObject
}

func legacyAutomationResponsePlanRefForIndex(refs []string, index int) string {
	if index < 0 || index >= len(refs) {
		return ""
	}
	return strings.TrimSpace(refs[index])
}

func normalizeAutomationPhaseNameForStore(phase types.AutomationPhaseName) types.AutomationPhaseName {
	switch strings.ToLower(strings.TrimSpace(string(phase))) {
	case string(types.AutomationPhaseRemediate):
		return types.AutomationPhaseRemediate
	case string(types.AutomationPhaseVerify):
		return types.AutomationPhaseVerify
	case string(types.AutomationPhaseEscalate):
		return types.AutomationPhaseEscalate
	default:
		return types.AutomationPhaseDiagnose
	}
}

func normalizeIncidentPhaseReductionForStore(reduction types.IncidentPhaseReduction) types.IncidentPhaseReduction {
	switch strings.ToLower(strings.TrimSpace(string(reduction))) {
	case string(types.IncidentPhaseReductionAnySuccess):
		return types.IncidentPhaseReductionAnySuccess
	case string(types.IncidentPhaseReductionBestEffort):
		return types.IncidentPhaseReductionBestEffort
	default:
		return types.IncidentPhaseReductionAllMustSucceed
	}
}

func normalizeIncidentPhaseStatusForStore(status types.IncidentPhaseStatus) types.IncidentPhaseStatus {
	switch strings.ToLower(strings.TrimSpace(string(status))) {
	case string(types.IncidentPhaseStatusRunning):
		return types.IncidentPhaseStatusRunning
	case string(types.IncidentPhaseStatusAwaitingApproval):
		return types.IncidentPhaseStatusAwaitingApproval
	case string(types.IncidentPhaseStatusCompleted):
		return types.IncidentPhaseStatusCompleted
	case string(types.IncidentPhaseStatusFailed):
		return types.IncidentPhaseStatusFailed
	case string(types.IncidentPhaseStatusCanceled):
		return types.IncidentPhaseStatusCanceled
	default:
		return types.IncidentPhaseStatusPending
	}
}

func normalizeDispatchAttemptStatusForStore(status types.DispatchAttemptStatus) types.DispatchAttemptStatus {
	switch strings.ToLower(strings.TrimSpace(string(status))) {
	case string(types.DispatchAttemptStatusAwaitingApproval):
		return types.DispatchAttemptStatusAwaitingApproval
	case string(types.DispatchAttemptStatusRunning):
		return types.DispatchAttemptStatusRunning
	case string(types.DispatchAttemptStatusInterrupted):
		return types.DispatchAttemptStatusInterrupted
	case string(types.DispatchAttemptStatusCompleted):
		return types.DispatchAttemptStatusCompleted
	case string(types.DispatchAttemptStatusFailed):
		return types.DispatchAttemptStatusFailed
	case string(types.DispatchAttemptStatusTimedOut):
		return types.DispatchAttemptStatusTimedOut
	case string(types.DispatchAttemptStatusCanceled):
		return types.DispatchAttemptStatusCanceled
	default:
		return types.DispatchAttemptStatusPlanned
	}
}

func normalizeDispatchAttemptFilterForStore(filter types.DispatchAttemptFilter) types.DispatchAttemptFilter {
	filter.WorkspaceRoot = strings.TrimSpace(filter.WorkspaceRoot)
	filter.AutomationID = strings.TrimSpace(filter.AutomationID)
	filter.IncidentID = strings.TrimSpace(filter.IncidentID)
	if strings.TrimSpace(string(filter.Status)) != "" {
		filter.Status = normalizeDispatchAttemptStatusForStore(filter.Status)
	}
	if filter.Limit < 0 {
		filter.Limit = 0
	}
	return filter
}

func normalizeDeliveryChannelsForStore(channels types.DeliveryChannelSet) types.DeliveryChannelSet {
	channels.Notice.Status = normalizeNoticeDeliveryChannelStatusForStore(channels.Notice.Status)
	channels.Mailbox.Status = normalizeMailboxDeliveryChannelStatusForStore(channels.Mailbox.Status)
	channels.Injection.Status = normalizeInjectionDeliveryChannelStatusForStore(channels.Injection.Status)
	return channels
}

func normalizeNoticeDeliveryChannelStatusForStore(status types.DeliveryChannelStatus) types.DeliveryChannelStatus {
	switch strings.ToLower(strings.TrimSpace(string(status))) {
	case string(types.DeliveryChannelStatusPending):
		return types.DeliveryChannelStatusPending
	case string(types.DeliveryChannelStatusEmitted):
		return types.DeliveryChannelStatusEmitted
	case string(types.DeliveryChannelStatusSkipped):
		return types.DeliveryChannelStatusSkipped
	case string(types.DeliveryChannelStatusFailed):
		return types.DeliveryChannelStatusFailed
	default:
		return types.DeliveryChannelStatusPending
	}
}

func normalizeMailboxDeliveryChannelStatusForStore(status types.DeliveryChannelStatus) types.DeliveryChannelStatus {
	switch strings.ToLower(strings.TrimSpace(string(status))) {
	case string(types.DeliveryChannelStatusPending):
		return types.DeliveryChannelStatusPending
	case string(types.DeliveryChannelStatusReady):
		return types.DeliveryChannelStatusReady
	case string(types.DeliveryChannelStatusSkipped):
		return types.DeliveryChannelStatusSkipped
	case string(types.DeliveryChannelStatusFailed):
		return types.DeliveryChannelStatusFailed
	default:
		return types.DeliveryChannelStatusPending
	}
}

func normalizeInjectionDeliveryChannelStatusForStore(status types.DeliveryChannelStatus) types.DeliveryChannelStatus {
	switch strings.ToLower(strings.TrimSpace(string(status))) {
	case string(types.DeliveryChannelStatusQueued):
		return types.DeliveryChannelStatusQueued
	case string(types.DeliveryChannelStatusInjected):
		return types.DeliveryChannelStatusInjected
	case string(types.DeliveryChannelStatusDisabled):
		return types.DeliveryChannelStatusDisabled
	case string(types.DeliveryChannelStatusSkipped):
		return types.DeliveryChannelStatusSkipped
	case string(types.DeliveryChannelStatusFailed):
		return types.DeliveryChannelStatusFailed
	default:
		return types.DeliveryChannelStatusDisabled
	}
}

func normalizeDeliveryRecordFilterForStore(filter types.DeliveryRecordFilter) types.DeliveryRecordFilter {
	filter.WorkspaceRoot = strings.TrimSpace(filter.WorkspaceRoot)
	filter.AutomationID = strings.TrimSpace(filter.AutomationID)
	filter.IncidentID = strings.TrimSpace(filter.IncidentID)
	filter.DispatchID = strings.TrimSpace(filter.DispatchID)
	if filter.Limit < 0 {
		filter.Limit = 0
	}
	return filter
}
