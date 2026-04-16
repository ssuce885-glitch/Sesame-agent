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

type dispatchResultDelivery interface {
	DeliverDispatchResult(context.Context, types.DispatchAttempt, types.ChildAgentResult) (types.DeliveryRecord, error)
}

type DispatchTaskObserver struct {
	store      dispatchAttemptObserverStore
	delivery   dispatchResultDelivery
	dispatchID string
	now        func() time.Time
}

func NewDispatchTaskObserver(store dispatchAttemptObserverStore, dispatchID string, now func() time.Time) *DispatchTaskObserver {
	return NewDispatchTaskObserverWithDelivery(store, nil, dispatchID, now)
}

func NewDispatchTaskObserverWithDelivery(store dispatchAttemptObserverStore, delivery dispatchResultDelivery, dispatchID string, now func() time.Time) *DispatchTaskObserver {
	return &DispatchTaskObserver{
		store:      store,
		delivery:   delivery,
		dispatchID: strings.TrimSpace(dispatchID),
		now:        firstNonNilClock(now),
	}
}

func (o *DispatchTaskObserver) AppendLog(_ []byte) error {
	return nil
}

func (o *DispatchTaskObserver) SetFinalText(text string) error {
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
	if err := o.store.UpsertDispatchAttempt(context.Background(), attempt); err != nil {
		return err
	}
	if o.delivery == nil || strings.TrimSpace(text) == "" {
		return nil
	}
	_, err = o.delivery.DeliverDispatchResult(context.Background(), attempt, types.ChildAgentResult{
		AgentID:    strings.TrimSpace(attempt.ChildAgentID),
		ContractID: strings.TrimSpace(attempt.OutputContractRef),
		Envelope: types.ReportEnvelope{
			Source:  string(types.ReportMailboxSourceChildAgentResult),
			Status:  "completed",
			Title:   firstNonEmptyObserverString(attempt.ChildAgentID, "Automation dispatch result"),
			Summary: summarizeObserverText(text),
			Sections: []types.ReportSectionContent{{
				ID:    "report_body",
				Title: "Result",
				Text:  strings.TrimSpace(text),
			}},
		},
		ObservedAt: now,
		CreatedAt:  now,
		UpdatedAt:  now,
	})
	return err
}

func (o *DispatchTaskObserver) SetOutcome(outcome types.ChildAgentOutcome, summary string) error {
	if o == nil || o.store == nil || strings.TrimSpace(o.dispatchID) == "" {
		return nil
	}
	attempt, ok, err := o.store.GetDispatchAttempt(context.Background(), o.dispatchID)
	if err != nil || !ok {
		return err
	}
	now := o.currentTime()
	attempt.Outcome = normalizeObserverOutcome(outcome)
	attempt.OutcomeSummary = strings.TrimSpace(summary)
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

func firstNonEmptyObserverString(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func summarizeObserverText(text string) string {
	text = strings.TrimSpace(text)
	if len(text) <= 160 {
		return text
	}
	return strings.TrimSpace(text[:157]) + "..."
}

func normalizeObserverOutcome(outcome types.ChildAgentOutcome) types.ChildAgentOutcome {
	normalized := types.ChildAgentOutcome(strings.ToLower(strings.TrimSpace(string(outcome))))
	switch normalized {
	case types.ChildAgentOutcomeSuccess, types.ChildAgentOutcomeFailure, types.ChildAgentOutcomeBlocked:
		return normalized
	default:
		return ""
	}
}
