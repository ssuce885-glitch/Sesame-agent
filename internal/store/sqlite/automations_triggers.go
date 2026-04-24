package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"

	"go-agent/internal/types"
)

func (s *Store) UpsertTriggerEvent(ctx context.Context, event types.TriggerEvent) error {
	return upsertTriggerEventWithExec(ctx, s.db, event)
}

func (t runtimeTx) UpsertTriggerEvent(ctx context.Context, event types.TriggerEvent) error {
	return upsertTriggerEventWithExec(ctx, t.tx, event)
}

func upsertTriggerEventWithExec(ctx context.Context, execer execContexter, event types.TriggerEvent) error {
	event = normalizeTriggerEventForStore(event)
	payload, err := json.Marshal(event)
	if err != nil {
		return err
	}

	_, err = execer.ExecContext(ctx, `
		insert into automation_trigger_events (
			event_id, workspace_root, automation_id, dedupe_key, signal_kind,
			source, observed_at, payload, created_at, updated_at
		)
		values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		on conflict(event_id) do update set
			workspace_root = excluded.workspace_root,
			automation_id = excluded.automation_id,
			dedupe_key = excluded.dedupe_key,
			signal_kind = excluded.signal_kind,
			source = excluded.source,
			observed_at = excluded.observed_at,
			payload = excluded.payload,
			updated_at = excluded.updated_at
	`,
		event.EventID,
		event.WorkspaceRoot,
		event.AutomationID,
		event.DedupeKey,
		event.SignalKind,
		event.Source,
		formatPendingOptionalTime(event.ObservedAt),
		string(payload),
		event.CreatedAt.UTC().Format(timeLayout),
		event.UpdatedAt.UTC().Format(timeLayout),
	)
	return err
}

func (s *Store) ListTriggerEvents(ctx context.Context, filter types.TriggerEventFilter) ([]types.TriggerEvent, error) {
	return listTriggerEventsWithQueryer(ctx, s.db, filter)
}

func (t runtimeTx) ListTriggerEvents(ctx context.Context, filter types.TriggerEventFilter) ([]types.TriggerEvent, error) {
	return listTriggerEventsWithQueryer(ctx, t.tx, filter)
}

func listTriggerEventsWithQueryer(ctx context.Context, queryer queryContexter, filter types.TriggerEventFilter) ([]types.TriggerEvent, error) {
	filter = normalizeTriggerEventFilterForStore(filter)
	query := `
		select payload, observed_at, created_at, updated_at
		from automation_trigger_events
	`
	args := make([]any, 0, 5)
	conditions := make([]string, 0, 4)
	if filter.WorkspaceRoot != "" {
		appendAutomationWorkspaceRootCondition(&conditions, &args, "workspace_root", filter.WorkspaceRoot)
	}
	if filter.AutomationID != "" {
		conditions = append(conditions, "automation_id = ?")
		args = append(args, filter.AutomationID)
	}
	if filter.DedupeKey != "" {
		conditions = append(conditions, "dedupe_key = ?")
		args = append(args, filter.DedupeKey)
	}
	if len(conditions) > 0 {
		query += " where " + strings.Join(conditions, " and ")
	}
	query += " order by observed_at desc, created_at desc, event_id asc"
	if filter.Limit > 0 {
		query += " limit ?"
		args = append(args, filter.Limit)
	}

	rows, err := queryer.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanTriggerEvents(rows)
}

func scanTriggerEvents(rows *sql.Rows) ([]types.TriggerEvent, error) {
	out := make([]types.TriggerEvent, 0)
	for rows.Next() {
		var (
			payload    string
			observedAt string
			createdAt  string
			updatedAt  string
		)
		if err := rows.Scan(&payload, &observedAt, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		var event types.TriggerEvent
		if err := json.Unmarshal([]byte(payload), &event); err != nil {
			return nil, err
		}
		if parsed, err := parsePendingOptionalTime(observedAt); err == nil && !parsed.IsZero() {
			event.ObservedAt = parsed
		}
		if parsed, err := parsePendingOptionalTime(createdAt); err == nil && !parsed.IsZero() {
			event.CreatedAt = parsed
		}
		if parsed, err := parsePendingOptionalTime(updatedAt); err == nil && !parsed.IsZero() {
			event.UpdatedAt = parsed
		}
		out = append(out, normalizeTriggerEventForStore(event))
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}
