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
