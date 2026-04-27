package sqlite

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"go-agent/internal/types"
)

func upsertReportWithExec(ctx context.Context, execer execContexter, report types.ReportRecord) error {
	report = normalizeReport(report)
	payload, err := json.Marshal(report)
	if err != nil {
		return err
	}

	_, err = execer.ExecContext(ctx, `
		insert into reports (
			id, workspace_root, session_id, source_kind, source_id, severity, observed_at, payload, created_at, updated_at
		)
		values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		on conflict(id) do update set
			workspace_root = excluded.workspace_root,
			session_id = excluded.session_id,
			source_kind = excluded.source_kind,
			source_id = excluded.source_id,
			severity = excluded.severity,
			observed_at = excluded.observed_at,
			payload = excluded.payload,
			updated_at = excluded.updated_at
	`,
		report.ID,
		strings.TrimSpace(report.WorkspaceRoot),
		report.SessionID,
		report.SourceKind,
		report.SourceID,
		strings.TrimSpace(report.Envelope.Severity),
		formatPendingOptionalTime(report.ObservedAt),
		string(payload),
		report.CreatedAt.Format(timeLayout),
		report.UpdatedAt.Format(timeLayout),
	)
	return err
}

func upsertReportDeliveryWithExec(ctx context.Context, execer execContexter, delivery types.ReportDelivery) error {
	delivery = normalizeReportDelivery(delivery)
	payload, err := json.Marshal(delivery)
	if err != nil {
		return err
	}

	_, err = execer.ExecContext(ctx, `
		insert into report_deliveries (
			id, workspace_root, session_id, report_id, channel, state, observed_at, injected_turn_id, injected_at, payload, created_at, updated_at
		)
		values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		on conflict(id) do update set
			workspace_root = excluded.workspace_root,
			session_id = excluded.session_id,
			report_id = excluded.report_id,
			channel = excluded.channel,
			state = excluded.state,
			observed_at = excluded.observed_at,
			injected_turn_id = excluded.injected_turn_id,
			injected_at = excluded.injected_at,
			payload = excluded.payload,
			updated_at = excluded.updated_at
	`,
		delivery.ID,
		strings.TrimSpace(delivery.WorkspaceRoot),
		delivery.SessionID,
		delivery.ReportID,
		delivery.Channel,
		delivery.State,
		formatPendingOptionalTime(delivery.ObservedAt),
		delivery.InjectedTurnID,
		formatPendingOptionalTime(delivery.InjectedAt),
		string(payload),
		delivery.CreatedAt.Format(timeLayout),
		delivery.UpdatedAt.Format(timeLayout),
	)
	return err
}

func normalizeReport(report types.ReportRecord) types.ReportRecord {
	now := time.Now().UTC()
	report.WorkspaceRoot = strings.TrimSpace(report.WorkspaceRoot)
	report.SessionID = strings.TrimSpace(report.SessionID)
	report.SourceSessionID = strings.TrimSpace(report.SourceSessionID)
	report.SourceTurnID = strings.TrimSpace(report.SourceTurnID)
	report.SourceRoleID = strings.TrimSpace(report.SourceRoleID)
	report.SourceKind = types.ReportSourceKind(strings.TrimSpace(string(report.SourceKind)))
	report.SourceID = strings.TrimSpace(report.SourceID)
	report.TargetRoleID = strings.TrimSpace(report.TargetRoleID)
	report.TargetSessionID = strings.TrimSpace(report.TargetSessionID)
	report.Envelope.Source = strings.TrimSpace(report.Envelope.Source)
	report.Envelope.Status = strings.TrimSpace(report.Envelope.Status)
	report.Envelope.Severity = strings.TrimSpace(report.Envelope.Severity)
	report.Envelope.Title = strings.TrimSpace(report.Envelope.Title)
	report.Envelope.Summary = strings.TrimSpace(report.Envelope.Summary)
	if strings.TrimSpace(report.ID) == "" {
		switch {
		case report.SourceKind != "" && report.SourceID != "":
			report.ID = fmt.Sprintf("%s:%s", report.SourceKind, report.SourceID)
		case report.SourceID != "":
			report.ID = report.SourceID
		default:
			report.ID = types.NewID("report")
		}
	}
	if report.CreatedAt.IsZero() {
		report.CreatedAt = now
	} else {
		report.CreatedAt = report.CreatedAt.UTC()
	}
	if report.UpdatedAt.IsZero() {
		report.UpdatedAt = report.CreatedAt
	} else {
		report.UpdatedAt = report.UpdatedAt.UTC()
	}
	if report.ObservedAt.IsZero() {
		report.ObservedAt = report.UpdatedAt
	} else {
		report.ObservedAt = report.ObservedAt.UTC()
	}
	return report
}

func normalizeReportDelivery(delivery types.ReportDelivery) types.ReportDelivery {
	now := time.Now().UTC()
	delivery.WorkspaceRoot = strings.TrimSpace(delivery.WorkspaceRoot)
	delivery.SessionID = strings.TrimSpace(delivery.SessionID)
	delivery.ReportID = strings.TrimSpace(delivery.ReportID)
	delivery.TargetRoleID = strings.TrimSpace(delivery.TargetRoleID)
	delivery.TargetSessionID = strings.TrimSpace(delivery.TargetSessionID)
	delivery.Channel = types.ReportChannel(strings.TrimSpace(string(delivery.Channel)))
	delivery.State = types.ReportDeliveryState(strings.TrimSpace(string(delivery.State)))
	delivery.InjectedTurnID = strings.TrimSpace(delivery.InjectedTurnID)
	if delivery.Channel == "" {
		delivery.Channel = types.ReportChannelAgent
	}
	if delivery.State == "" {
		delivery.State = types.ReportDeliveryStateQueued
	}
	if strings.TrimSpace(delivery.ID) == "" {
		delivery.ID = firstNonEmptyReportString(delivery.ReportID, types.NewID("report_delivery"))
	}
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
	if delivery.ObservedAt.IsZero() {
		delivery.ObservedAt = delivery.UpdatedAt
	} else {
		delivery.ObservedAt = delivery.ObservedAt.UTC()
	}
	if !delivery.InjectedAt.IsZero() {
		delivery.InjectedAt = delivery.InjectedAt.UTC()
	}
	return delivery
}

func applyReportTimes(report *types.ReportRecord, observedAtRaw, createdAtRaw, updatedAtRaw string) {
	if report == nil {
		return
	}
	if parsed, err := parsePendingOptionalTime(observedAtRaw); err == nil {
		report.ObservedAt = parsed
	}
	if parsed, err := parsePendingOptionalTime(createdAtRaw); err == nil {
		report.CreatedAt = parsed
	}
	if parsed, err := parsePendingOptionalTime(updatedAtRaw); err == nil {
		report.UpdatedAt = parsed
	}
}

func applyReportDeliveryTimes(delivery *types.ReportDelivery, observedAtRaw, injectedTurnID, injectedAtRaw, createdAtRaw, updatedAtRaw string) {
	if delivery == nil {
		return
	}
	if parsed, err := parsePendingOptionalTime(observedAtRaw); err == nil {
		delivery.ObservedAt = parsed
	}
	delivery.InjectedTurnID = strings.TrimSpace(injectedTurnID)
	if parsed, err := parsePendingOptionalTime(injectedAtRaw); err == nil {
		delivery.InjectedAt = parsed
	}
	if parsed, err := parsePendingOptionalTime(createdAtRaw); err == nil {
		delivery.CreatedAt = parsed
	}
	if parsed, err := parsePendingOptionalTime(updatedAtRaw); err == nil {
		delivery.UpdatedAt = parsed
	}
}

func reportDeliveryItemToRecordDelivery(item types.ReportDeliveryItem) (types.ReportRecord, types.ReportDelivery) {
	report := normalizeReport(types.ReportRecord{
		ID:              firstNonEmptyReportString(item.ReportID, item.ID),
		WorkspaceRoot:   strings.TrimSpace(item.WorkspaceRoot),
		SessionID:       item.SessionID,
		SourceSessionID: item.SourceSessionID,
		SourceTurnID:    item.SourceTurnID,
		SourceRoleID:    item.SourceRoleID,
		SourceKind:      item.SourceKind,
		SourceID:        item.SourceID,
		TargetRoleID:    item.TargetRoleID,
		TargetSessionID: item.TargetSessionID,
		Audience:        item.Audience,
		Envelope:        item.Envelope,
		ObservedAt:      item.ObservedAt,
		CreatedAt:       item.CreatedAt,
		UpdatedAt:       item.UpdatedAt,
	})
	delivery := normalizeReportDelivery(types.ReportDelivery{
		ID:              firstNonEmptyReportString(item.DeliveryID, item.ID, report.ID),
		WorkspaceRoot:   strings.TrimSpace(item.WorkspaceRoot),
		SessionID:       item.SessionID,
		ReportID:        report.ID,
		TargetRoleID:    item.TargetRoleID,
		TargetSessionID: item.TargetSessionID,
		Audience:        item.Audience,
		Channel:         firstNonEmptyReportChannel(item.Channel, types.ReportChannelAgent),
		State:           firstNonEmptyReportState(item.DeliveryState, reportDeliveryStateFromInjection(item.InjectedTurnID)),
		ObservedAt:      firstNonEmptyReportTime(item.ObservedAt, report.ObservedAt),
		InjectedTurnID:  item.InjectedTurnID,
		InjectedAt:      item.InjectedAt,
		CreatedAt:       item.CreatedAt,
		UpdatedAt:       item.UpdatedAt,
	})
	return report, delivery
}

func reportDeliveryStateFromInjection(injectedTurnID string) types.ReportDeliveryState {
	if strings.TrimSpace(injectedTurnID) != "" {
		return types.ReportDeliveryStateDelivered
	}
	return types.ReportDeliveryStateQueued
}

func firstNonEmptyReportString(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func firstNonEmptyReportChannel(values ...types.ReportChannel) types.ReportChannel {
	for _, value := range values {
		if trimmed := strings.TrimSpace(string(value)); trimmed != "" {
			return types.ReportChannel(trimmed)
		}
	}
	return ""
}

func firstNonEmptyReportState(values ...types.ReportDeliveryState) types.ReportDeliveryState {
	for _, value := range values {
		if trimmed := strings.TrimSpace(string(value)); trimmed != "" {
			return types.ReportDeliveryState(trimmed)
		}
	}
	return ""
}

func firstNonEmptyReportTime(values ...time.Time) time.Time {
	for _, value := range values {
		if !value.IsZero() {
			return value.UTC()
		}
	}
	return time.Time{}
}
