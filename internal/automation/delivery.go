package automation

import (
	"context"
	"strings"
	"time"

	"go-agent/internal/reporting"
	"go-agent/internal/types"
)

type DeliveryStore interface {
	UpsertDeliveryRecord(context.Context, types.DeliveryRecord) error
}

type DeliveryService struct {
	store     DeliveryStore
	reporting *reporting.Service
	now       func() time.Time
}

func NewDeliveryService(store DeliveryStore, reportingService *reporting.Service, now func() time.Time) *DeliveryService {
	return &DeliveryService{
		store:     store,
		reporting: reportingService,
		now:       firstNonNilClock(now),
	}
}

func (d *DeliveryService) DeliverDispatchResult(ctx context.Context, attempt types.DispatchAttempt, result types.ChildAgentResult) (types.DeliveryRecord, error) {
	now := d.currentTime()
	if strings.TrimSpace(result.SessionID) == "" {
		result.SessionID = strings.TrimSpace(attempt.PreferredSessionID)
	}
	if result.CreatedAt.IsZero() {
		result.CreatedAt = now
	}
	if result.UpdatedAt.IsZero() {
		result.UpdatedAt = result.CreatedAt
	}

	record := types.DeliveryRecord{
		DeliveryID:    types.NewID("delivery"),
		WorkspaceRoot: strings.TrimSpace(attempt.WorkspaceRoot),
		AutomationID:  strings.TrimSpace(attempt.AutomationID),
		IncidentID:    strings.TrimSpace(attempt.IncidentID),
		DispatchID:    strings.TrimSpace(attempt.DispatchID),
		SummaryRef:    "child_agent_result:" + strings.TrimSpace(result.ResultID),
		Channels: types.DeliveryChannelSet{
			Notice:    types.DeliveryChannelStatusRecord{Status: types.DeliveryChannelStatusPending},
			Mailbox:   types.DeliveryChannelStatusRecord{Status: types.DeliveryChannelStatusPending},
			Injection: types.DeliveryChannelStatusRecord{Status: types.DeliveryChannelStatusDisabled},
		},
		CreatedAt: now,
		UpdatedAt: now,
	}

	if d.reporting != nil {
		if _, err := d.reporting.EnqueueAutomationChildResult(ctx, attempt.WorkspaceRoot, result, now); err != nil {
			record.Channels.Mailbox.Status = types.DeliveryChannelStatusFailed
		} else {
			record.Channels.Mailbox.Status = types.DeliveryChannelStatusReady
		}
	} else {
		record.Channels.Mailbox.Status = types.DeliveryChannelStatusFailed
	}

	if d.store != nil {
		if err := d.store.UpsertDeliveryRecord(ctx, record); err != nil {
			return types.DeliveryRecord{}, err
		}
	}
	return record, nil
}

func (d *DeliveryService) currentTime() time.Time {
	if d != nil && d.now != nil {
		return d.now().UTC()
	}
	return time.Now().UTC()
}
