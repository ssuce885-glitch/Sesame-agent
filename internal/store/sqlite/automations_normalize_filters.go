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

func normalizeAutomationHeartbeatFilterForStore(filter types.AutomationHeartbeatFilter) types.AutomationHeartbeatFilter {
	filter.WorkspaceRoot = strings.TrimSpace(filter.WorkspaceRoot)
	filter.AutomationID = strings.TrimSpace(filter.AutomationID)
	filter.WatcherID = strings.TrimSpace(filter.WatcherID)
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
	filter.DedupeKey = strings.TrimSpace(filter.DedupeKey)
	if filter.Limit < 0 {
		filter.Limit = 0
	}
	return filter
}

func normalizeAutomationWatcherHoldKindForStore(kind types.AutomationWatcherHoldKind) types.AutomationWatcherHoldKind {
	switch strings.ToLower(strings.TrimSpace(string(kind))) {
	case string(types.AutomationWatcherHoldKindManual):
		return types.AutomationWatcherHoldKindManual
	default:
		return ""
	}
}
