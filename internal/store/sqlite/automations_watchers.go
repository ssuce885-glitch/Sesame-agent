package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"

	"go-agent/internal/types"
)

func (s *Store) UpsertAutomationHeartbeat(ctx context.Context, heartbeat types.AutomationHeartbeat) error {
	return upsertAutomationHeartbeatWithExec(ctx, s.db, heartbeat)
}

func (t runtimeTx) UpsertAutomationHeartbeat(ctx context.Context, heartbeat types.AutomationHeartbeat) error {
	return upsertAutomationHeartbeatWithExec(ctx, t.tx, heartbeat)
}

func upsertAutomationHeartbeatWithExec(ctx context.Context, execer execContexter, heartbeat types.AutomationHeartbeat) error {
	heartbeat = normalizeAutomationHeartbeatForStore(heartbeat)
	payload, err := json.Marshal(heartbeat)
	if err != nil {
		return err
	}

	_, err = execer.ExecContext(ctx, `
		insert into automation_heartbeats (
			automation_id, watcher_id, workspace_root, status, observed_at, payload, created_at, updated_at
		)
		values (?, ?, ?, ?, ?, ?, ?, ?)
		on conflict(automation_id, watcher_id) do update set
			workspace_root = excluded.workspace_root,
			status = excluded.status,
			observed_at = excluded.observed_at,
			payload = excluded.payload,
			updated_at = excluded.updated_at
	`,
		heartbeat.AutomationID,
		heartbeat.WatcherID,
		heartbeat.WorkspaceRoot,
		heartbeat.Status,
		formatPendingOptionalTime(heartbeat.ObservedAt),
		string(payload),
		heartbeat.CreatedAt.UTC().Format(timeLayout),
		heartbeat.UpdatedAt.UTC().Format(timeLayout),
	)
	return err
}

func (s *Store) UpsertAutomationWatcher(ctx context.Context, watcher types.AutomationWatcherRuntime) error {
	return upsertAutomationWatcherWithExec(ctx, s.db, watcher)
}

func (t runtimeTx) UpsertAutomationWatcher(ctx context.Context, watcher types.AutomationWatcherRuntime) error {
	return upsertAutomationWatcherWithExec(ctx, t.tx, watcher)
}

func upsertAutomationWatcherWithExec(ctx context.Context, execer execContexter, watcher types.AutomationWatcherRuntime) error {
	watcher = normalizeAutomationWatcherForStore(watcher)
	payload, err := json.Marshal(watcher)
	if err != nil {
		return err
	}

	_, err = execer.ExecContext(ctx, `
		insert into automation_watchers (
			id, automation_id, workspace_root, state, payload, created_at, updated_at
		)
		values (?, ?, ?, ?, ?, ?, ?)
		on conflict(automation_id) do update set
			id = excluded.id,
			workspace_root = excluded.workspace_root,
			state = excluded.state,
			payload = excluded.payload,
			updated_at = excluded.updated_at
	`,
		watcher.ID,
		watcher.AutomationID,
		watcher.WorkspaceRoot,
		watcher.State,
		string(payload),
		watcher.CreatedAt.UTC().Format(timeLayout),
		watcher.UpdatedAt.UTC().Format(timeLayout),
	)
	return err
}

func (s *Store) GetAutomationWatcher(ctx context.Context, automationID string) (types.AutomationWatcherRuntime, bool, error) {
	return getAutomationWatcherWithQueryer(ctx, s.db, automationID)
}

func (t runtimeTx) GetAutomationWatcher(ctx context.Context, automationID string) (types.AutomationWatcherRuntime, bool, error) {
	return getAutomationWatcherWithQueryer(ctx, t.tx, automationID)
}

func getAutomationWatcherWithQueryer(ctx context.Context, queryer queryContexter, automationID string) (types.AutomationWatcherRuntime, bool, error) {
	automationID = strings.TrimSpace(automationID)
	if automationID == "" {
		return types.AutomationWatcherRuntime{}, false, nil
	}
	rows, err := queryer.QueryContext(ctx, `
		select payload, created_at, updated_at
		from automation_watchers
		where automation_id = ?
	`, automationID)
	if err != nil {
		return types.AutomationWatcherRuntime{}, false, err
	}
	defer rows.Close()

	items, err := scanAutomationWatchers(rows)
	if err != nil {
		return types.AutomationWatcherRuntime{}, false, err
	}
	if len(items) == 0 {
		return types.AutomationWatcherRuntime{}, false, nil
	}
	watcher, err := hydrateAutomationWatcherRuntime(ctx, queryer, items[0])
	if err != nil {
		return types.AutomationWatcherRuntime{}, false, err
	}
	return watcher, true, nil
}

func (s *Store) ListAutomationWatchers(ctx context.Context, filter types.AutomationWatcherFilter) ([]types.AutomationWatcherRuntime, error) {
	return listAutomationWatchersWithQueryer(ctx, s.db, filter)
}

func (t runtimeTx) ListAutomationWatchers(ctx context.Context, filter types.AutomationWatcherFilter) ([]types.AutomationWatcherRuntime, error) {
	return listAutomationWatchersWithQueryer(ctx, t.tx, filter)
}

func listAutomationWatchersWithQueryer(ctx context.Context, queryer queryContexter, filter types.AutomationWatcherFilter) ([]types.AutomationWatcherRuntime, error) {
	filter = normalizeAutomationWatcherFilterForStore(filter)
	query := `
		select payload, created_at, updated_at
		from automation_watchers
	`
	args := make([]any, 0, 4)
	conditions := make([]string, 0, 3)
	if filter.WorkspaceRoot != "" {
		appendAutomationWorkspaceRootCondition(&conditions, &args, "workspace_root", filter.WorkspaceRoot)
	}
	if filter.AutomationID != "" {
		conditions = append(conditions, "automation_id = ?")
		args = append(args, filter.AutomationID)
	}
	if filter.State != "" {
		conditions = append(conditions, "state = ?")
		args = append(args, filter.State)
	}
	if len(conditions) > 0 {
		query += " where " + strings.Join(conditions, " and ")
	}
	query += " order by updated_at desc, created_at desc, automation_id asc"
	if filter.Limit > 0 {
		query += " limit ?"
		args = append(args, filter.Limit)
	}

	rows, err := queryer.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items, err := scanAutomationWatchers(rows)
	if err != nil {
		return nil, err
	}
	for index := range items {
		items[index], err = hydrateAutomationWatcherRuntime(ctx, queryer, items[index])
		if err != nil {
			return nil, err
		}
	}
	return items, nil
}

func (s *Store) DeleteAutomationWatcher(ctx context.Context, automationID string) (bool, error) {
	return deleteAutomationWatcherWithExec(ctx, s.db, automationID)
}

func (t runtimeTx) DeleteAutomationWatcher(ctx context.Context, automationID string) (bool, error) {
	return deleteAutomationWatcherWithExec(ctx, t.tx, automationID)
}

func deleteAutomationWatcherWithExec(ctx context.Context, execer execContexter, automationID string) (bool, error) {
	if _, err := execer.ExecContext(ctx, `
		delete from automation_watcher_holds
		where automation_id = ?
	`, strings.TrimSpace(automationID)); err != nil {
		return false, err
	}
	result, err := execer.ExecContext(ctx, `
		delete from automation_watchers
		where automation_id = ?
	`, strings.TrimSpace(automationID))
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return affected > 0, nil
}

func (s *Store) ListAutomationWatcherHolds(ctx context.Context, automationID string) ([]types.AutomationWatcherHold, error) {
	return listAutomationWatcherHoldsWithQueryer(ctx, s.db, automationID)
}

func (t runtimeTx) ListAutomationWatcherHolds(ctx context.Context, automationID string) ([]types.AutomationWatcherHold, error) {
	return listAutomationWatcherHoldsWithQueryer(ctx, t.tx, automationID)
}

func listAutomationWatcherHoldsWithQueryer(ctx context.Context, queryer queryContexter, automationID string) ([]types.AutomationWatcherHold, error) {
	automationID = strings.TrimSpace(automationID)
	if automationID == "" {
		return []types.AutomationWatcherHold{}, nil
	}
	rows, err := queryer.QueryContext(ctx, `
		select payload, created_at, updated_at
		from automation_watcher_holds
		where automation_id = ?
		order by created_at asc, hold_id asc
	`, automationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAutomationWatcherHolds(rows)
}

func (s *Store) ReplaceAutomationWatcherHolds(ctx context.Context, automationID, watcherID string, holds []types.AutomationWatcherHold) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback()
	}()
	if err := replaceAutomationWatcherHoldsWithExec(ctx, tx, automationID, watcherID, holds); err != nil {
		return err
	}
	return tx.Commit()
}

func (t runtimeTx) ReplaceAutomationWatcherHolds(ctx context.Context, automationID, watcherID string, holds []types.AutomationWatcherHold) error {
	return replaceAutomationWatcherHoldsWithExec(ctx, t.tx, automationID, watcherID, holds)
}

func replaceAutomationWatcherHoldsWithExec(ctx context.Context, execer execContexter, automationID, watcherID string, holds []types.AutomationWatcherHold) error {
	automationID = strings.TrimSpace(automationID)
	watcherID = strings.TrimSpace(watcherID)
	if automationID == "" {
		return nil
	}
	if watcherID == "" {
		watcherID = "watcher:" + automationID
	}
	if _, err := execer.ExecContext(ctx, `
		delete from automation_watcher_holds
		where automation_id = ?
	`, automationID); err != nil {
		return err
	}
	for _, hold := range holds {
		hold = normalizeAutomationWatcherHoldForStore(hold)
		hold.AutomationID = automationID
		hold.WatcherID = watcherID
		payload, err := json.Marshal(hold)
		if err != nil {
			return err
		}
		if _, err := execer.ExecContext(ctx, `
			insert into automation_watcher_holds (
				hold_id, automation_id, watcher_id, kind, owner_id, payload, created_at, updated_at
			)
			values (?, ?, ?, ?, ?, ?, ?, ?)
		`,
			hold.HoldID,
			hold.AutomationID,
			hold.WatcherID,
			hold.Kind,
			hold.OwnerID,
			string(payload),
			hold.CreatedAt.UTC().Format(timeLayout),
			hold.UpdatedAt.UTC().Format(timeLayout),
		); err != nil {
			return err
		}
	}
	return nil
}

func scanAutomationWatchers(rows *sql.Rows) ([]types.AutomationWatcherRuntime, error) {
	out := make([]types.AutomationWatcherRuntime, 0)
	for rows.Next() {
		var (
			payload   string
			createdAt string
			updatedAt string
		)
		if err := rows.Scan(&payload, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		var watcher types.AutomationWatcherRuntime
		if err := json.Unmarshal([]byte(payload), &watcher); err != nil {
			return nil, err
		}
		if parsed, err := parsePendingOptionalTime(createdAt); err == nil && !parsed.IsZero() {
			watcher.CreatedAt = parsed
		}
		if parsed, err := parsePendingOptionalTime(updatedAt); err == nil && !parsed.IsZero() {
			watcher.UpdatedAt = parsed
		}
		out = append(out, normalizeAutomationWatcherForStore(watcher))
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func scanAutomationWatcherHolds(rows *sql.Rows) ([]types.AutomationWatcherHold, error) {
	out := make([]types.AutomationWatcherHold, 0)
	for rows.Next() {
		var (
			payload   string
			createdAt string
			updatedAt string
		)
		if err := rows.Scan(&payload, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		var hold types.AutomationWatcherHold
		if err := json.Unmarshal([]byte(payload), &hold); err != nil {
			return nil, err
		}
		if parsed, err := parsePendingOptionalTime(createdAt); err == nil && !parsed.IsZero() {
			hold.CreatedAt = parsed
		}
		if parsed, err := parsePendingOptionalTime(updatedAt); err == nil && !parsed.IsZero() {
			hold.UpdatedAt = parsed
		}
		out = append(out, normalizeAutomationWatcherHoldForStore(hold))
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func hydrateAutomationWatcherRuntime(ctx context.Context, queryer queryContexter, watcher types.AutomationWatcherRuntime) (types.AutomationWatcherRuntime, error) {
	holds, err := listAutomationWatcherHoldsWithQueryer(ctx, queryer, watcher.AutomationID)
	if err != nil {
		return types.AutomationWatcherRuntime{}, err
	}
	watcher.Holds = append([]types.AutomationWatcherHold(nil), holds...)
	watcher.EffectiveState = effectiveAutomationWatcherState(watcher.State, holds)
	return watcher, nil
}

func effectiveAutomationWatcherState(current types.AutomationWatcherState, holds []types.AutomationWatcherHold) types.AutomationWatcherState {
	if len(holds) > 0 {
		return types.AutomationWatcherStatePaused
	}
	if current == "" {
		return types.AutomationWatcherStateRunning
	}
	return current
}
