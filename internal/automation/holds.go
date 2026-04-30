package automation

import (
	"strings"
	"time"

	"go-agent/internal/types"
)

func EffectiveWatcherState(current types.AutomationWatcherState, holds []types.AutomationWatcherHold) types.AutomationWatcherState {
	if len(holds) > 0 {
		return types.AutomationWatcherStatePaused
	}
	if current == "" {
		return types.AutomationWatcherStateRunning
	}
	return current
}

func AcquireWatcherHold(holds []types.AutomationWatcherHold, kind types.AutomationWatcherHoldKind, ownerID, reason string, now time.Time) []types.AutomationWatcherHold {
	updated := ReleaseWatcherHold(holds, kind, ownerID)
	updated = append(updated, types.AutomationWatcherHold{
		HoldID:    types.NewID("watcher_hold"),
		Kind:      kind,
		OwnerID:   strings.TrimSpace(ownerID),
		Reason:    strings.TrimSpace(reason),
		CreatedAt: now.UTC(),
		UpdatedAt: now.UTC(),
	})
	return updated
}

func ReleaseWatcherHold(holds []types.AutomationWatcherHold, kind types.AutomationWatcherHoldKind, ownerID string) []types.AutomationWatcherHold {
	ownerID = strings.TrimSpace(ownerID)
	out := make([]types.AutomationWatcherHold, 0, len(holds))
	for _, hold := range holds {
		if hold.Kind == kind && strings.TrimSpace(hold.OwnerID) == ownerID {
			continue
		}
		out = append(out, hold)
	}
	return out
}
