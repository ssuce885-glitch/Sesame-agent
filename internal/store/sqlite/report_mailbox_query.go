package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"

	"go-agent/internal/types"
)

type reportMailboxQueryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

type reportMailboxRowQueryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func listWorkspaceReportMailboxItemsWithQuery(ctx context.Context, queryer reportMailboxQueryer, workspaceRoot, injectedTurnID string) ([]types.ReportMailboxItem, error) {
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
		where d.workspace_root = ? and d.channel = ?
	`
	args := []any{strings.TrimSpace(workspaceRoot), string(types.ReportChannelMailbox)}
	if trimmed := strings.TrimSpace(injectedTurnID); trimmed != "" {
		query += ` and d.injected_turn_id = ?`
		args = append(args, trimmed)
		query += ` order by d.observed_at asc, d.created_at asc, d.id asc`
	} else {
		query += ` order by d.observed_at desc, d.created_at desc, d.id asc`
	}
	rows, err := queryer.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanReportMailboxRows(rows)
}

func listReportsWithQuery(ctx context.Context, queryer reportMailboxQueryer, sessionID string) ([]types.ReportRecord, error) {
	rows, err := queryer.QueryContext(ctx, `
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

func countPendingWorkspaceReportMailboxItemsWithQuery(ctx context.Context, queryer reportMailboxRowQueryer, workspaceRoot string) (int, error) {
	var count int
	if err := queryer.QueryRowContext(ctx, `
		select count(*)
		from report_deliveries
		where workspace_root = ? and channel = ? and state = ?
	`, strings.TrimSpace(workspaceRoot), string(types.ReportChannelMailbox), string(types.ReportDeliveryStatePending)).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
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
