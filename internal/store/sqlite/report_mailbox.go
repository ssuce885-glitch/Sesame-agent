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

func (s *Store) ListWorkspaceReportMailboxItems(ctx context.Context, workspaceRoot string) ([]types.ReportMailboxItem, error) {
	return listWorkspaceReportMailboxItemsWithQuery(ctx, s.db, workspaceRoot, "")
}

func (s *Store) ListReports(ctx context.Context, sessionID string) ([]types.ReportRecord, error) {
	return listReportsWithQuery(ctx, s.db, sessionID)
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
	if workspaceRoot == "" {
		return nil, nil
	}
	return s.ListWorkspaceReportMailboxItems(ctx, workspaceRoot)
}

func (s *Store) CountPendingReportMailboxItems(ctx context.Context, sessionID string) (int, error) {
	workspaceRoot := s.workspaceRootForSession(ctx, sessionID)
	if workspaceRoot == "" {
		return 0, nil
	}
	return s.CountPendingWorkspaceReportMailboxItems(ctx, workspaceRoot)
}

func (s *Store) CountPendingWorkspaceReportMailboxItems(ctx context.Context, workspaceRoot string) (int, error) {
	return countPendingWorkspaceReportMailboxItemsWithQuery(ctx, s.db, workspaceRoot)
}

func (s *Store) ClaimPendingReportMailboxItemsForTurn(ctx context.Context, sessionID, turnID string) ([]types.ReportMailboxItem, error) {
	sessionID = strings.TrimSpace(sessionID)
	turnID = strings.TrimSpace(turnID)
	if sessionID == "" || turnID == "" {
		return nil, nil
	}

	workspaceRoot := s.workspaceRootForSession(ctx, sessionID)
	if workspaceRoot == "" {
		return nil, nil
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
