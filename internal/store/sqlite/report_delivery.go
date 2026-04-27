package sqlite

import (
	"context"
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

func (s *Store) ListWorkspaceReportDeliveryItems(ctx context.Context, workspaceRoot string) ([]types.ReportDeliveryItem, error) {
	return listWorkspaceReportDeliveryItemsWithQuery(ctx, s.db, workspaceRoot, "")
}

func (s *Store) ListReports(ctx context.Context, sessionID string) ([]types.ReportRecord, error) {
	return listReportsWithQuery(ctx, s.db, sessionID)
}

func (s *Store) ListReportDeliveries(ctx context.Context, sessionID string, channel types.ReportChannel) ([]types.ReportDelivery, error) {
	return listReportDeliveriesWithQuery(ctx, s.db, sessionID, channel)
}

func (s *Store) UpsertReportDeliveryItem(ctx context.Context, item types.ReportDeliveryItem) error {
	if strings.TrimSpace(item.WorkspaceRoot) == "" && strings.TrimSpace(item.SessionID) != "" {
		item.WorkspaceRoot = s.workspaceRootForSession(ctx, item.SessionID)
	}
	report, delivery := reportDeliveryItemToRecordDelivery(item)
	if err := s.UpsertReport(ctx, report); err != nil {
		return err
	}
	return s.UpsertReportDelivery(ctx, delivery)
}

func (s *Store) ListReportDeliveryItems(ctx context.Context, sessionID string) ([]types.ReportDeliveryItem, error) {
	return listSessionReportDeliveryItemsWithQuery(ctx, s.db, sessionID, "")
}

func (s *Store) CountQueuedReportDeliveries(ctx context.Context, sessionID string) (int, error) {
	return countQueuedSessionReportDeliveriesWithQuery(ctx, s.db, sessionID)
}

func (s *Store) CountQueuedWorkspaceReportDeliveries(ctx context.Context, workspaceRoot string) (int, error) {
	return countQueuedWorkspaceReportDeliveriesWithQuery(ctx, s.db, workspaceRoot)
}

func (s *Store) ClaimQueuedReportDeliveriesForTurn(ctx context.Context, sessionID, turnID string) ([]types.ReportDeliveryItem, error) {
	sessionID = strings.TrimSpace(sessionID)
	turnID = strings.TrimSpace(turnID)
	if sessionID == "" || turnID == "" {
		return nil, nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	claimed, err := listSessionReportDeliveryItemsWithQuery(ctx, tx, sessionID, turnID)
	if err != nil {
		return nil, err
	}
	if len(claimed) > 0 {
		if err := tx.Commit(); err != nil {
			return nil, err
		}
		return claimed, nil
	}

	queued, err := listQueuedReportDeliveriesWithSessionQuery(ctx, tx, sessionID)
	if err != nil {
		return nil, err
	}
	if len(queued) == 0 {
		if err := tx.Commit(); err != nil {
			return nil, err
		}
		return nil, nil
	}

	now := time.Now().UTC()
	for index := range queued {
		queued[index].State = types.ReportDeliveryStateDelivered
		queued[index].InjectedTurnID = turnID
		queued[index].InjectedAt = now
		queued[index].UpdatedAt = now
		queued[index].SessionID = sessionID
		if err := upsertReportDeliveryWithExec(ctx, tx, queued[index]); err != nil {
			return nil, err
		}
	}

	claimed, err = listSessionReportDeliveryItemsWithQuery(ctx, tx, sessionID, turnID)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return claimed, nil
}

func (s *Store) RequeueClaimedReportDeliveriesForTurn(ctx context.Context, turnID string) error {
	turnID = strings.TrimSpace(turnID)
	if turnID == "" {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	rows, err := tx.QueryContext(ctx, `
		select payload, observed_at, injected_turn_id, injected_at, created_at, updated_at
		from report_deliveries
		where injected_turn_id = ?
		order by observed_at asc, created_at asc, id asc
	`, turnID)
	if err != nil {
		return err
	}
	deliveries, err := scanReportDeliveryRows(rows)
	if closeErr := rows.Close(); closeErr != nil && err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}

	now := time.Now().UTC()
	for index := range deliveries {
		deliveries[index].State = types.ReportDeliveryStateQueued
		deliveries[index].InjectedTurnID = ""
		deliveries[index].InjectedAt = time.Time{}
		deliveries[index].UpdatedAt = now
		if err := upsertReportDeliveryWithExec(ctx, tx, deliveries[index]); err != nil {
			return err
		}
	}
	return tx.Commit()
}
