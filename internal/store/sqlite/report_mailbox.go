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

func (s *Store) UpsertReportMailboxItem(ctx context.Context, item types.ReportMailboxItem) error {
	return upsertReportMailboxItemWithExec(ctx, s.db, item)
}

func (s *Store) ListReportMailboxItems(ctx context.Context, sessionID string) ([]types.ReportMailboxItem, error) {
	rows, err := s.db.QueryContext(ctx, `
		select payload, observed_at, injected_turn_id, injected_at, created_at, updated_at
		from report_mailbox_items
		where session_id = ?
		order by observed_at desc, created_at desc, id asc
	`, strings.TrimSpace(sessionID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanReportMailboxRows(rows)
}

func (s *Store) CountPendingReportMailboxItems(ctx context.Context, sessionID string) (int, error) {
	var count int
	if err := s.db.QueryRowContext(ctx, `
		select count(*)
		from report_mailbox_items
		where session_id = ? and injected_turn_id = ''
	`, strings.TrimSpace(sessionID)).Scan(&count); err != nil {
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

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	claimed, err := listReportMailboxItemsWithQuery(ctx, tx, sessionID, turnID)
	if err != nil {
		return nil, err
	}
	if len(claimed) > 0 {
		if err := tx.Commit(); err != nil {
			return nil, err
		}
		return claimed, nil
	}

	pending, err := listReportMailboxItemsWithQuery(ctx, tx, sessionID, "")
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
		pending[index].InjectedTurnID = turnID
		pending[index].InjectedAt = now
		pending[index].UpdatedAt = now
		if err := upsertReportMailboxItemWithExec(ctx, tx, pending[index]); err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return pending, nil
}

type reportMailboxQueryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func listReportMailboxItemsWithQuery(ctx context.Context, queryer reportMailboxQueryer, sessionID, injectedTurnID string) ([]types.ReportMailboxItem, error) {
	rows, err := queryer.QueryContext(ctx, `
		select payload, observed_at, injected_turn_id, injected_at, created_at, updated_at
		from report_mailbox_items
		where session_id = ? and injected_turn_id = ?
		order by observed_at asc, created_at asc, id asc
	`, strings.TrimSpace(sessionID), strings.TrimSpace(injectedTurnID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanReportMailboxRows(rows)
}

func scanReportMailboxRows(rows *sql.Rows) ([]types.ReportMailboxItem, error) {
	out := make([]types.ReportMailboxItem, 0)
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

		var item types.ReportMailboxItem
		if err := json.Unmarshal([]byte(rawPayload), &item); err != nil {
			return nil, err
		}
		applyReportMailboxTimes(&item, observedAt, injectedTurnID, injectedAt, createdAt, updatedAt)
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func upsertReportMailboxItemWithExec(ctx context.Context, execer execContexter, item types.ReportMailboxItem) error {
	item = normalizeReportMailboxItem(item)
	payload, err := json.Marshal(item)
	if err != nil {
		return err
	}

	_, err = execer.ExecContext(ctx, `
		insert into report_mailbox_items (
			id, session_id, source_kind, source_id, severity, observed_at, injected_turn_id, injected_at, payload, created_at, updated_at
		)
		values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		on conflict(id) do update set
			session_id = excluded.session_id,
			source_kind = excluded.source_kind,
			source_id = excluded.source_id,
			severity = excluded.severity,
			observed_at = excluded.observed_at,
			injected_turn_id = excluded.injected_turn_id,
			injected_at = excluded.injected_at,
			payload = excluded.payload,
			updated_at = excluded.updated_at
	`,
		item.ID,
		item.SessionID,
		item.SourceKind,
		item.SourceID,
		strings.TrimSpace(item.Envelope.Severity),
		formatPendingOptionalTime(item.ObservedAt),
		item.InjectedTurnID,
		formatPendingOptionalTime(item.InjectedAt),
		string(payload),
		item.CreatedAt.Format(timeLayout),
		item.UpdatedAt.Format(timeLayout),
	)
	return err
}

func normalizeReportMailboxItem(item types.ReportMailboxItem) types.ReportMailboxItem {
	now := time.Now().UTC()
	item.SessionID = strings.TrimSpace(item.SessionID)
	item.SourceKind = types.ReportMailboxSourceKind(strings.TrimSpace(string(item.SourceKind)))
	item.SourceID = strings.TrimSpace(item.SourceID)
	item.InjectedTurnID = strings.TrimSpace(item.InjectedTurnID)
	item.Envelope.Source = strings.TrimSpace(item.Envelope.Source)
	item.Envelope.Status = strings.TrimSpace(item.Envelope.Status)
	item.Envelope.Severity = strings.TrimSpace(item.Envelope.Severity)
	item.Envelope.Title = strings.TrimSpace(item.Envelope.Title)
	item.Envelope.Summary = strings.TrimSpace(item.Envelope.Summary)
	if strings.TrimSpace(item.ID) == "" {
		switch {
		case item.SourceKind != "" && item.SourceID != "":
			item.ID = fmt.Sprintf("%s:%s", item.SourceKind, item.SourceID)
		case item.SourceID != "":
			item.ID = item.SourceID
		default:
			item.ID = types.NewID("report_mailbox")
		}
	}
	if item.CreatedAt.IsZero() {
		item.CreatedAt = now
	} else {
		item.CreatedAt = item.CreatedAt.UTC()
	}
	if item.UpdatedAt.IsZero() {
		item.UpdatedAt = item.CreatedAt
	} else {
		item.UpdatedAt = item.UpdatedAt.UTC()
	}
	if item.ObservedAt.IsZero() {
		item.ObservedAt = item.UpdatedAt
	} else {
		item.ObservedAt = item.ObservedAt.UTC()
	}
	if !item.InjectedAt.IsZero() {
		item.InjectedAt = item.InjectedAt.UTC()
	}
	return item
}

func applyReportMailboxTimes(item *types.ReportMailboxItem, observedAtRaw, injectedTurnID, injectedAtRaw, createdAtRaw, updatedAtRaw string) {
	if item == nil {
		return
	}
	if parsed, err := parsePendingOptionalTime(observedAtRaw); err == nil {
		item.ObservedAt = parsed
	}
	item.InjectedTurnID = strings.TrimSpace(injectedTurnID)
	if parsed, err := parsePendingOptionalTime(injectedAtRaw); err == nil {
		item.InjectedAt = parsed
	}
	if parsed, err := parsePendingOptionalTime(createdAtRaw); err == nil {
		item.CreatedAt = parsed
	}
	if parsed, err := parsePendingOptionalTime(updatedAtRaw); err == nil {
		item.UpdatedAt = parsed
	}
}
