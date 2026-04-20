package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"go-agent/internal/types"
)

func (s *Store) workspaceRootForSession(ctx context.Context, sessionID string) string {
	if s == nil {
		return ""
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return ""
	}
	var root string
	if err := s.db.QueryRowContext(ctx, `
		select workspace_root
		from sessions
		where id = ?
	`, sessionID).Scan(&root); err != nil {
		// Backwards-compatible fallback: some code paths/tests write mailbox data without
		// persisting a session row. Treat session_id as a stable scope key in that case.
		if err == sql.ErrNoRows {
			return sessionID
		}
		return ""
	}
	return strings.TrimSpace(root)
}

func (s *Store) UpsertReport(ctx context.Context, report types.ReportRecord) error {
	return upsertReportWithExec(ctx, s.db, report)
}

func (s *Store) UpsertReportDelivery(ctx context.Context, delivery types.ReportDelivery) error {
	return upsertReportDeliveryWithExec(ctx, s.db, delivery)
}

func (s *Store) ListWorkspaceReportMailboxItems(ctx context.Context, workspaceRoot string) ([]types.ReportMailboxItem, error) {
	rows, err := s.db.QueryContext(ctx, `
		select
			d.payload,
			d.observed_at,
			d.injected_turn_id,
			d.injected_at,
			d.created_at,
			d.updated_at,
			r.payload,
			r.observed_at,
			r.created_at,
			r.updated_at,
			d.workspace_root
		from report_deliveries d
		join reports r on r.id = d.report_id
		where d.workspace_root = ? and d.channel = ?
		order by d.observed_at desc, d.created_at desc, d.id asc
	`, strings.TrimSpace(workspaceRoot), string(types.ReportChannelMailbox))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanReportMailboxRows(rows)
}

func (s *Store) ListReports(ctx context.Context, sessionID string) ([]types.ReportRecord, error) {
	rows, err := s.db.QueryContext(ctx, `
		select payload, observed_at, created_at, updated_at
		from reports
		where session_id = ?
		order by observed_at desc, created_at desc, id asc
	`, strings.TrimSpace(sessionID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanReportRows(rows)
}

func (s *Store) ListReportDeliveries(ctx context.Context, sessionID string, channel types.ReportChannel) ([]types.ReportDelivery, error) {
	return listReportDeliveriesWithQuery(ctx, s.db, sessionID, channel)
}

func (s *Store) UpsertReportMailboxItem(ctx context.Context, item types.ReportMailboxItem) error {
	if strings.TrimSpace(item.WorkspaceRoot) == "" && strings.TrimSpace(item.SessionID) != "" {
		item.WorkspaceRoot = s.workspaceRootForSession(ctx, item.SessionID)
	}
	report, delivery := mailboxItemToRecordDelivery(item)
	if err := s.UpsertReport(ctx, report); err != nil {
		return err
	}
	return s.UpsertReportDelivery(ctx, delivery)
}

func (s *Store) ListReportMailboxItems(ctx context.Context, sessionID string) ([]types.ReportMailboxItem, error) {
	workspaceRoot := s.workspaceRootForSession(ctx, sessionID)
	if workspaceRoot == "" || workspaceRoot == strings.TrimSpace(sessionID) {
		return listSessionReportMailboxItemsWithQuery(ctx, s.db, sessionID, "")
	}
	return s.ListWorkspaceReportMailboxItems(ctx, workspaceRoot)
}

func (s *Store) CountPendingReportMailboxItems(ctx context.Context, sessionID string) (int, error) {
	workspaceRoot := s.workspaceRootForSession(ctx, sessionID)
	if workspaceRoot == "" || workspaceRoot == strings.TrimSpace(sessionID) {
		return countPendingSessionReportMailboxItemsWithQuery(ctx, s.db, sessionID)
	}
	return s.CountPendingWorkspaceReportMailboxItems(ctx, workspaceRoot)
}

func (s *Store) CountPendingWorkspaceReportMailboxItems(ctx context.Context, workspaceRoot string) (int, error) {
	var count int
	if err := s.db.QueryRowContext(ctx, `
		select count(*)
		from report_deliveries
		where workspace_root = ? and channel = ? and state = ?
	`, strings.TrimSpace(workspaceRoot), string(types.ReportChannelMailbox), string(types.ReportDeliveryStatePending)).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func (s *Store) ClaimPendingReportMailboxItemsForTurn(ctx context.Context, sessionID, turnID string) ([]types.ReportMailboxItem, error) {
	sessionID = strings.TrimSpace(sessionID)
	turnID = strings.TrimSpace(turnID)
	if sessionID == "" || turnID == "" {
		return nil, nil
	}

	workspaceRoot := s.workspaceRootForSession(ctx, sessionID)
	if workspaceRoot == "" || workspaceRoot == strings.TrimSpace(sessionID) {
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return nil, err
		}
		defer func() {
			_ = tx.Rollback()
		}()

		claimed, err := listSessionReportMailboxItemsWithQuery(ctx, tx, sessionID, turnID)
		if err != nil {
			return nil, err
		}
		if len(claimed) > 0 {
			if err := tx.Commit(); err != nil {
				return nil, err
			}
			return claimed, nil
		}

		pending, err := listPendingMailboxDeliveriesWithSessionQuery(ctx, tx, sessionID)
		if err != nil {
			return nil, err
		}
		if len(pending) == 0 {
			if err := tx.Commit(); err != nil {
				return nil, err
			}
			return nil, nil
		}

		now := time.Now().UTC()
		for index := range pending {
			pending[index].State = types.ReportDeliveryStateDelivered
			pending[index].InjectedTurnID = turnID
			pending[index].InjectedAt = now
			pending[index].UpdatedAt = now
			pending[index].SessionID = sessionID
			if err := upsertReportDeliveryWithExec(ctx, tx, pending[index]); err != nil {
				return nil, err
			}
		}

		claimed, err = listSessionReportMailboxItemsWithQuery(ctx, tx, sessionID, turnID)
		if err != nil {
			return nil, err
		}
		if err := tx.Commit(); err != nil {
			return nil, err
		}
		return claimed, nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	claimed, err := listWorkspaceReportMailboxItemsWithQuery(ctx, tx, workspaceRoot, turnID)
	if err != nil {
		return nil, err
	}
	if len(claimed) > 0 {
		if err := tx.Commit(); err != nil {
			return nil, err
		}
		return claimed, nil
	}

	pending, err := listPendingMailboxDeliveriesWithWorkspaceQuery(ctx, tx, workspaceRoot)
	if err != nil {
		return nil, err
	}
	if len(pending) == 0 {
		if err := tx.Commit(); err != nil {
			return nil, err
		}
		return nil, nil
	}

	now := time.Now().UTC()
	for index := range pending {
		pending[index].State = types.ReportDeliveryStateDelivered
		pending[index].InjectedTurnID = turnID
		pending[index].InjectedAt = now
		pending[index].UpdatedAt = now
		pending[index].SessionID = sessionID
		if err := upsertReportDeliveryWithExec(ctx, tx, pending[index]); err != nil {
			return nil, err
		}
	}

	claimed, err = listWorkspaceReportMailboxItemsWithQuery(ctx, tx, workspaceRoot, turnID)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return claimed, nil
}

type reportMailboxQueryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

type reportMailboxRowQueryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func listWorkspaceReportMailboxItemsWithQuery(ctx context.Context, queryer reportMailboxQueryer, workspaceRoot, injectedTurnID string) ([]types.ReportMailboxItem, error) {
	rows, err := queryer.QueryContext(ctx, `
		select
			d.payload,
			d.observed_at,
			d.injected_turn_id,
			d.injected_at,
			d.created_at,
			d.updated_at,
			r.payload,
			r.observed_at,
			r.created_at,
			r.updated_at,
			d.workspace_root
		from report_deliveries d
		join reports r on r.id = d.report_id
		where d.workspace_root = ? and d.channel = ? and d.injected_turn_id = ?
		order by d.observed_at asc, d.created_at asc, d.id asc
	`, strings.TrimSpace(workspaceRoot), string(types.ReportChannelMailbox), strings.TrimSpace(injectedTurnID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanReportMailboxRows(rows)
}

func listSessionReportMailboxItemsWithQuery(ctx context.Context, queryer reportMailboxQueryer, sessionID, injectedTurnID string) ([]types.ReportMailboxItem, error) {
	query := `
		select
			d.payload,
			d.observed_at,
			d.injected_turn_id,
			d.injected_at,
			d.created_at,
			d.updated_at,
			r.payload,
			r.observed_at,
			r.created_at,
			r.updated_at,
			d.workspace_root
		from report_deliveries d
		join reports r on r.id = d.report_id
		where d.session_id = ? and d.channel = ?
	`
	args := []any{strings.TrimSpace(sessionID), string(types.ReportChannelMailbox)}
	if trimmed := strings.TrimSpace(injectedTurnID); trimmed != "" {
		query += ` and d.injected_turn_id = ?`
		args = append(args, trimmed)
	}
	query += ` order by d.observed_at asc, d.created_at asc, d.id asc`
	rows, err := queryer.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanReportMailboxRows(rows)
}

func listPendingMailboxDeliveriesWithWorkspaceQuery(ctx context.Context, queryer reportMailboxQueryer, workspaceRoot string) ([]types.ReportDelivery, error) {
	rows, err := queryer.QueryContext(ctx, `
		select payload, observed_at, injected_turn_id, injected_at, created_at, updated_at
		from report_deliveries
		where workspace_root = ? and channel = ? and state = ?
		order by observed_at asc, created_at asc, id asc
	`, strings.TrimSpace(workspaceRoot), string(types.ReportChannelMailbox), string(types.ReportDeliveryStatePending))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanReportDeliveryRows(rows)
}

func listPendingMailboxDeliveriesWithSessionQuery(ctx context.Context, queryer reportMailboxQueryer, sessionID string) ([]types.ReportDelivery, error) {
	rows, err := queryer.QueryContext(ctx, `
		select payload, observed_at, injected_turn_id, injected_at, created_at, updated_at
		from report_deliveries
		where session_id = ? and channel = ? and state = ?
		order by observed_at asc, created_at asc, id asc
	`, strings.TrimSpace(sessionID), string(types.ReportChannelMailbox), string(types.ReportDeliveryStatePending))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanReportDeliveryRows(rows)
}

func countPendingSessionReportMailboxItemsWithQuery(ctx context.Context, queryer reportMailboxRowQueryer, sessionID string) (int, error) {
	var count int
	if err := queryer.QueryRowContext(ctx, `
		select count(*)
		from report_deliveries
		where session_id = ? and channel = ? and state = ?
	`, strings.TrimSpace(sessionID), string(types.ReportChannelMailbox), string(types.ReportDeliveryStatePending)).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func scanReportMailboxRows(rows *sql.Rows) ([]types.ReportMailboxItem, error) {
	out := make([]types.ReportMailboxItem, 0)
	for rows.Next() {
		var (
			rawDelivery      string
			deliveryObserved string
			injectedTurnID   string
			injectedAt       string
			deliveryCreated  string
			deliveryUpdated  string
			rawReport        string
			reportObserved   string
			reportCreated    string
			reportUpdated    string
			workspaceRoot    string
		)
		if err := rows.Scan(
			&rawDelivery,
			&deliveryObserved,
			&injectedTurnID,
			&injectedAt,
			&deliveryCreated,
			&deliveryUpdated,
			&rawReport,
			&reportObserved,
			&reportCreated,
			&reportUpdated,
			&workspaceRoot,
		); err != nil {
			return nil, err
		}

		var delivery types.ReportDelivery
		if err := json.Unmarshal([]byte(rawDelivery), &delivery); err != nil {
			return nil, err
		}
		applyReportDeliveryTimes(&delivery, deliveryObserved, injectedTurnID, injectedAt, deliveryCreated, deliveryUpdated)
		if strings.TrimSpace(delivery.WorkspaceRoot) == "" {
			delivery.WorkspaceRoot = strings.TrimSpace(workspaceRoot)
		}

		var report types.ReportRecord
		if err := json.Unmarshal([]byte(rawReport), &report); err != nil {
			return nil, err
		}
		applyReportTimes(&report, reportObserved, reportCreated, reportUpdated)
		if strings.TrimSpace(report.WorkspaceRoot) == "" {
			report.WorkspaceRoot = strings.TrimSpace(workspaceRoot)
		}

		out = append(out, types.ReportMailboxItemFromRecordDelivery(report, delivery))
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func scanReportRows(rows *sql.Rows) ([]types.ReportRecord, error) {
	out := make([]types.ReportRecord, 0)
	for rows.Next() {
		var (
			rawPayload string
			observedAt string
			createdAt  string
			updatedAt  string
		)
		if err := rows.Scan(&rawPayload, &observedAt, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		var report types.ReportRecord
		if err := json.Unmarshal([]byte(rawPayload), &report); err != nil {
			return nil, err
		}
		applyReportTimes(&report, observedAt, createdAt, updatedAt)
		out = append(out, report)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func scanReportDeliveryRows(rows *sql.Rows) ([]types.ReportDelivery, error) {
	out := make([]types.ReportDelivery, 0)
	for rows.Next() {
		var (
			rawPayload     string
			observedAt     string
			injectedTurnID string
			injectedAt     string
			createdAt      string
			updatedAt      string
		)
		if err := rows.Scan(&rawPayload, &observedAt, &injectedTurnID, &injectedAt, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		var delivery types.ReportDelivery
		if err := json.Unmarshal([]byte(rawPayload), &delivery); err != nil {
			return nil, err
		}
		applyReportDeliveryTimes(&delivery, observedAt, injectedTurnID, injectedAt, createdAt, updatedAt)
		out = append(out, delivery)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

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

func listReportDeliveriesWithQuery(ctx context.Context, queryer reportMailboxQueryer, sessionID string, channel types.ReportChannel) ([]types.ReportDelivery, error) {
	query := `
		select payload, observed_at, injected_turn_id, injected_at, created_at, updated_at
		from report_deliveries
		where session_id = ?
	`
	args := []any{strings.TrimSpace(sessionID)}
	if strings.TrimSpace(string(channel)) != "" {
		query += ` and channel = ?`
		args = append(args, string(channel))
	}
	query += ` order by observed_at desc, created_at desc, id asc`
	rows, err := queryer.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanReportDeliveryRows(rows)
}

func normalizeReport(report types.ReportRecord) types.ReportRecord {
	now := time.Now().UTC()
	report.WorkspaceRoot = strings.TrimSpace(report.WorkspaceRoot)
	report.SessionID = strings.TrimSpace(report.SessionID)
	report.SourceSessionID = strings.TrimSpace(report.SourceSessionID)
	report.SourceRoleID = strings.TrimSpace(report.SourceRoleID)
	report.SourceKind = types.ReportMailboxSourceKind(strings.TrimSpace(string(report.SourceKind)))
	report.SourceID = strings.TrimSpace(report.SourceID)
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
	delivery.Channel = types.ReportChannel(strings.TrimSpace(string(delivery.Channel)))
	delivery.State = types.ReportDeliveryState(strings.TrimSpace(string(delivery.State)))
	delivery.InjectedTurnID = strings.TrimSpace(delivery.InjectedTurnID)
	if delivery.Channel == "" {
		delivery.Channel = types.ReportChannelMailbox
	}
	if delivery.State == "" {
		delivery.State = types.ReportDeliveryStatePending
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

func mailboxItemToRecordDelivery(item types.ReportMailboxItem) (types.ReportRecord, types.ReportDelivery) {
	report := normalizeReport(types.ReportRecord{
		ID:              firstNonEmptyReportString(item.ReportID, item.ID),
		WorkspaceRoot:   strings.TrimSpace(item.WorkspaceRoot),
		SessionID:       item.SessionID,
		SourceSessionID: item.SourceSessionID,
		SourceRoleID:    item.SourceRoleID,
		SourceKind:      item.SourceKind,
		SourceID:        item.SourceID,
		Envelope:        item.Envelope,
		ObservedAt:      item.ObservedAt,
		CreatedAt:       item.CreatedAt,
		UpdatedAt:       item.UpdatedAt,
	})
	delivery := normalizeReportDelivery(types.ReportDelivery{
		ID:             firstNonEmptyReportString(item.DeliveryID, item.ID, report.ID),
		WorkspaceRoot:  strings.TrimSpace(item.WorkspaceRoot),
		SessionID:      item.SessionID,
		ReportID:       report.ID,
		Channel:        firstNonEmptyReportChannel(item.Channel, types.ReportChannelMailbox),
		State:          firstNonEmptyReportState(item.DeliveryState, reportDeliveryStateFromInjection(item.InjectedTurnID)),
		ObservedAt:     firstNonEmptyReportTime(item.ObservedAt, report.ObservedAt),
		InjectedTurnID: item.InjectedTurnID,
		InjectedAt:     item.InjectedAt,
		CreatedAt:      item.CreatedAt,
		UpdatedAt:      item.UpdatedAt,
	})
	return report, delivery
}

func reportDeliveryStateFromInjection(injectedTurnID string) types.ReportDeliveryState {
	if strings.TrimSpace(injectedTurnID) != "" {
		return types.ReportDeliveryStateDelivered
	}
	return types.ReportDeliveryStatePending
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
