package automation

import (
	"context"
	"strings"
	"time"

	"go-agent/internal/types"
)

type watcherHoldStore interface {
	GetAutomationWatcher(context.Context, string) (types.AutomationWatcherRuntime, bool, error)
	ListAutomationWatcherHolds(context.Context, string) ([]types.AutomationWatcherHold, error)
	ReplaceAutomationWatcherHolds(context.Context, string, string, []types.AutomationWatcherHold) error
}

type dispatchApprovalResumeStore interface {
	watcherHoldStore
	FindDispatchAttemptByBackgroundRun(context.Context, string, string) (types.DispatchAttempt, bool, error)
	FindDispatchAttemptByTaskID(context.Context, string) (types.DispatchAttempt, bool, error)
	UpsertDispatchAttempt(context.Context, types.DispatchAttempt) error
}

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

func SwapWatcherHold(holds []types.AutomationWatcherHold, fromKind types.AutomationWatcherHoldKind, fromOwner string, toKind types.AutomationWatcherHoldKind, toOwner, reason string, now time.Time) []types.AutomationWatcherHold {
	return AcquireWatcherHold(ReleaseWatcherHold(holds, fromKind, fromOwner), toKind, toOwner, reason, now)
}

func ReleaseWatcherHoldByOwner(ctx context.Context, store watcherHoldStore, automationID string, kind types.AutomationWatcherHoldKind, ownerID string) error {
	if store == nil {
		return nil
	}
	runtime, watcherID, holds, err := watcherHoldContext(ctx, store, automationID)
	if err != nil {
		return err
	}
	if runtime.AutomationID == "" {
		return nil
	}
	updated := ReleaseWatcherHold(holds, kind, ownerID)
	return store.ReplaceAutomationWatcherHolds(ctx, automationID, watcherID, updated)
}

func AcquireWatcherHoldByOwner(ctx context.Context, store watcherHoldStore, automationID string, kind types.AutomationWatcherHoldKind, ownerID, reason string, now time.Time) error {
	if store == nil {
		return nil
	}
	runtime, watcherID, holds, err := watcherHoldContext(ctx, store, automationID)
	if err != nil {
		return err
	}
	if runtime.AutomationID == "" {
		return nil
	}
	updated := AcquireWatcherHold(holds, kind, ownerID, reason, now)
	return store.ReplaceAutomationWatcherHolds(ctx, automationID, watcherID, updated)
}

func ReplaceDispatchHoldWithApprovalHold(ctx context.Context, store watcherHoldStore, automationID, dispatchID, requestID string, now time.Time) error {
	if store == nil {
		return nil
	}
	runtime, watcherID, holds, err := watcherHoldContext(ctx, store, automationID)
	if err != nil {
		return err
	}
	if runtime.AutomationID == "" {
		return nil
	}
	updated := SwapWatcherHold(holds, types.AutomationWatcherHoldKindDispatch, dispatchID, types.AutomationWatcherHoldKindApproval, requestID, "permission requested", now)
	return store.ReplaceAutomationWatcherHolds(ctx, automationID, watcherID, updated)
}

func ReplaceApprovalHoldWithDispatchHold(ctx context.Context, store watcherHoldStore, automationID, requestID, dispatchID string, now time.Time) error {
	if store == nil {
		return nil
	}
	runtime, watcherID, holds, err := watcherHoldContext(ctx, store, automationID)
	if err != nil {
		return err
	}
	if runtime.AutomationID == "" {
		return nil
	}
	updated := SwapWatcherHold(holds, types.AutomationWatcherHoldKindApproval, requestID, types.AutomationWatcherHoldKindDispatch, dispatchID, "dispatch resumed", now)
	return store.ReplaceAutomationWatcherHolds(ctx, automationID, watcherID, updated)
}

func RestoreDispatchAfterApprovalResume(ctx context.Context, store dispatchApprovalResumeStore, sessionID, turnID, taskID, requestID string, now time.Time) error {
	if store == nil {
		return nil
	}
	attempt, ok, err := store.FindDispatchAttemptByBackgroundRun(ctx, sessionID, turnID)
	if err != nil {
		return err
	}
	if !ok {
		attempt, ok, err = store.FindDispatchAttemptByTaskID(ctx, taskID)
		if err != nil || !ok {
			return err
		}
	}
	if strings.TrimSpace(attempt.PermissionRequestID) != strings.TrimSpace(requestID) {
		return nil
	}
	if err := ReplaceApprovalHoldWithDispatchHold(ctx, store, attempt.AutomationID, requestID, attempt.DispatchID, now); err != nil {
		return err
	}
	attempt.Status = types.DispatchAttemptStatusRunning
	attempt.UpdatedAt = now.UTC()
	return store.UpsertDispatchAttempt(ctx, attempt)
}

func watcherHoldContext(ctx context.Context, store watcherHoldStore, automationID string) (types.AutomationWatcherRuntime, string, []types.AutomationWatcherHold, error) {
	automationID = strings.TrimSpace(automationID)
	if automationID == "" {
		return types.AutomationWatcherRuntime{}, "", nil, nil
	}
	runtime, ok, err := store.GetAutomationWatcher(ctx, automationID)
	if err != nil || !ok {
		return types.AutomationWatcherRuntime{}, "", nil, err
	}
	holds, err := store.ListAutomationWatcherHolds(ctx, automationID)
	if err != nil {
		return types.AutomationWatcherRuntime{}, "", nil, err
	}
	watcherID := strings.TrimSpace(runtime.WatcherID)
	if watcherID == "" {
		watcherID = "watcher:" + automationID
	}
	return runtime, watcherID, holds, nil
}
