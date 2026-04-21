package sqlite

import (
	"strings"

	"go-agent/internal/types"
)

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

func normalizeAutomationWatcherHoldKindForStore(kind types.AutomationWatcherHoldKind) types.AutomationWatcherHoldKind {
	switch strings.ToLower(strings.TrimSpace(string(kind))) {
	case string(types.AutomationWatcherHoldKindManual):
		return types.AutomationWatcherHoldKindManual
	case string(types.AutomationWatcherHoldKindDispatch):
		return types.AutomationWatcherHoldKindDispatch
	case string(types.AutomationWatcherHoldKindApproval):
		return types.AutomationWatcherHoldKindApproval
	default:
		return ""
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
