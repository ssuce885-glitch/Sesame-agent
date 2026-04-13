package automation

import (
	"context"
	"strings"
	"time"

	"go-agent/internal/types"
)

type dispatchAttemptObserverStore interface {
	GetDispatchAttempt(context.Context, string) (types.DispatchAttempt, bool, error)
	UpsertDispatchAttempt(context.Context, types.DispatchAttempt) error
}

type DispatchTaskObserver struct {
	store      dispatchAttemptObserverStore
	dispatchID string
	now        func() time.Time
}

func NewDispatchTaskObserver(store dispatchAttemptObserverStore, dispatchID string, now func() time.Time) *DispatchTaskObserver {
	return &DispatchTaskObserver{
		store:      store,
		dispatchID: strings.TrimSpace(dispatchID),
		now:        firstNonNilClock(now),
	}
}

func (o *DispatchTaskObserver) AppendLog(_ []byte) error {
	return nil
}

func (o *DispatchTaskObserver) SetFinalText(_ string) error {
	if o == nil || o.store == nil || strings.TrimSpace(o.dispatchID) == "" {
		return nil
	}
	attempt, ok, err := o.store.GetDispatchAttempt(context.Background(), o.dispatchID)
	if err != nil || !ok {
		return err
	}
	now := o.currentTime()
	attempt.Status = types.DispatchAttemptStatusCompleted
	attempt.FinishedAt = now
	attempt.UpdatedAt = now
	return o.store.UpsertDispatchAttempt(context.Background(), attempt)
}

func (o *DispatchTaskObserver) SetRunContext(sessionID, turnID string) error {
	if o == nil || o.store == nil || strings.TrimSpace(o.dispatchID) == "" {
		return nil
	}
	attempt, ok, err := o.store.GetDispatchAttempt(context.Background(), o.dispatchID)
	if err != nil || !ok {
		return err
	}
	now := o.currentTime()
	attempt.BackgroundSessionID = strings.TrimSpace(sessionID)
	attempt.BackgroundTurnID = strings.TrimSpace(turnID)
	if attempt.StartedAt.IsZero() {
		attempt.StartedAt = now
	}
	attempt.Status = types.DispatchAttemptStatusRunning
	attempt.UpdatedAt = now
	return o.store.UpsertDispatchAttempt(context.Background(), attempt)
}

func (o *DispatchTaskObserver) currentTime() time.Time {
	if o != nil && o.now != nil {
		return o.now().UTC()
	}
	return time.Now().UTC()
}
