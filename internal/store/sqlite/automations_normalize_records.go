package sqlite

import (
	"strings"
	"time"

	"go-agent/internal/types"
)

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
	watcher.EffectiveState = ""
	watcher.Holds = nil
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

func normalizeAutomationWatcherHoldForStore(hold types.AutomationWatcherHold) types.AutomationWatcherHold {
	now := time.Now().UTC()
	hold.HoldID = strings.TrimSpace(hold.HoldID)
	if hold.HoldID == "" {
		hold.HoldID = types.NewID("watcher_hold")
	}
	hold.AutomationID = strings.TrimSpace(hold.AutomationID)
	hold.WatcherID = strings.TrimSpace(hold.WatcherID)
	hold.Kind = normalizeAutomationWatcherHoldKindForStore(hold.Kind)
	hold.OwnerID = strings.TrimSpace(hold.OwnerID)
	hold.Reason = strings.TrimSpace(hold.Reason)
	if hold.CreatedAt.IsZero() {
		hold.CreatedAt = now
	} else {
		hold.CreatedAt = hold.CreatedAt.UTC()
	}
	if hold.UpdatedAt.IsZero() {
		hold.UpdatedAt = hold.CreatedAt
	} else {
		hold.UpdatedAt = hold.UpdatedAt.UTC()
	}
	if hold.UpdatedAt.Before(hold.CreatedAt) {
		hold.UpdatedAt = hold.CreatedAt
	}
	return hold
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
	switch normalized := types.ChildAgentOutcome(strings.ToLower(strings.TrimSpace(string(attempt.Outcome)))); normalized {
	case types.ChildAgentOutcomeSuccess, types.ChildAgentOutcomeFailure, types.ChildAgentOutcomeBlocked:
		attempt.Outcome = normalized
	default:
		attempt.Outcome = ""
	}
	attempt.OutcomeSummary = strings.TrimSpace(attempt.OutcomeSummary)
	attempt.TaskID = strings.TrimSpace(attempt.TaskID)
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
