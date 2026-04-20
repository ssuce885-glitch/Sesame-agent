package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"

	"go-agent/internal/types"
)

func (s *Store) UpsertDispatchAttempt(ctx context.Context, attempt types.DispatchAttempt) error {
	return upsertDispatchAttemptWithExec(ctx, s.db, attempt)
}

func (t runtimeTx) UpsertDispatchAttempt(ctx context.Context, attempt types.DispatchAttempt) error {
	return upsertDispatchAttemptWithExec(ctx, t.tx, attempt)
}

func upsertDispatchAttemptWithExec(ctx context.Context, execer execContexter, attempt types.DispatchAttempt) error {
	attempt = normalizeDispatchAttemptForStore(attempt)
	payload, err := json.Marshal(attempt)
	if err != nil {
		return err
	}

	_, err = execer.ExecContext(ctx, `
		insert into automation_dispatch_attempts (
			dispatch_id, workspace_root, automation_id, incident_id, phase, status,
			task_id, payload, created_at, updated_at
		)
		values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		on conflict(dispatch_id) do update set
			workspace_root = excluded.workspace_root,
			automation_id = excluded.automation_id,
			incident_id = excluded.incident_id,
			phase = excluded.phase,
			status = excluded.status,
			task_id = excluded.task_id,
			payload = excluded.payload,
			updated_at = excluded.updated_at
	`,
		attempt.DispatchID,
		attempt.WorkspaceRoot,
		attempt.AutomationID,
		attempt.IncidentID,
		attempt.Phase,
		attempt.Status,
		attempt.TaskID,
		string(payload),
		attempt.CreatedAt.UTC().Format(timeLayout),
		attempt.UpdatedAt.UTC().Format(timeLayout),
	)
	return err
}

func (s *Store) GetDispatchAttempt(ctx context.Context, dispatchID string) (types.DispatchAttempt, bool, error) {
	return getDispatchAttemptWithQueryer(ctx, s.db, dispatchID)
}

func (t runtimeTx) GetDispatchAttempt(ctx context.Context, dispatchID string) (types.DispatchAttempt, bool, error) {
	return getDispatchAttemptWithQueryer(ctx, t.tx, dispatchID)
}

func getDispatchAttemptWithQueryer(ctx context.Context, queryer queryContexter, dispatchID string) (types.DispatchAttempt, bool, error) {
	dispatchID = strings.TrimSpace(dispatchID)
	if dispatchID == "" {
		return types.DispatchAttempt{}, false, nil
	}
	rows, err := queryer.QueryContext(ctx, `
		select payload, created_at, updated_at
		from automation_dispatch_attempts
		where dispatch_id = ?
	`, dispatchID)
	if err != nil {
		return types.DispatchAttempt{}, false, err
	}
	defer rows.Close()

	items, err := scanDispatchAttempts(rows)
	if err != nil {
		return types.DispatchAttempt{}, false, err
	}
	if len(items) == 0 {
		return types.DispatchAttempt{}, false, nil
	}
	return items[0], true, nil
}

func (s *Store) FindDispatchAttemptByTaskID(ctx context.Context, taskID string) (types.DispatchAttempt, bool, error) {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return types.DispatchAttempt{}, false, nil
	}
	rows, err := s.db.QueryContext(ctx, `
		select payload, created_at, updated_at
		from automation_dispatch_attempts
		where task_id = ?
		order by updated_at desc, created_at desc, dispatch_id asc
		limit 1
	`, taskID)
	if err != nil {
		return types.DispatchAttempt{}, false, err
	}
	defer rows.Close()

	items, err := scanDispatchAttempts(rows)
	if err != nil {
		return types.DispatchAttempt{}, false, err
	}
	if len(items) == 0 {
		return types.DispatchAttempt{}, false, nil
	}
	return items[0], true, nil
}

func (s *Store) ListDispatchAttempts(ctx context.Context, filter types.DispatchAttemptFilter) ([]types.DispatchAttempt, error) {
	return listDispatchAttemptsWithQueryer(ctx, s.db, filter)
}

func (t runtimeTx) ListDispatchAttempts(ctx context.Context, filter types.DispatchAttemptFilter) ([]types.DispatchAttempt, error) {
	return listDispatchAttemptsWithQueryer(ctx, t.tx, filter)
}

func listDispatchAttemptsWithQueryer(ctx context.Context, queryer queryContexter, filter types.DispatchAttemptFilter) ([]types.DispatchAttempt, error) {
	filter = normalizeDispatchAttemptFilterForStore(filter)
	query := `
		select payload, created_at, updated_at
		from automation_dispatch_attempts
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
	if filter.IncidentID != "" {
		conditions = append(conditions, "incident_id = ?")
		args = append(args, filter.IncidentID)
	}
	if filter.Status != "" {
		conditions = append(conditions, "status = ?")
		args = append(args, filter.Status)
	}
	if len(conditions) > 0 {
		query += " where " + strings.Join(conditions, " and ")
	}
	query += " order by updated_at desc, created_at desc, dispatch_id asc"
	if filter.Limit > 0 {
		query += " limit ?"
		args = append(args, filter.Limit)
	}

	rows, err := queryer.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanDispatchAttempts(rows)
}

func (s *Store) ListPendingAutomationPermissions(ctx context.Context, workspaceRoot string) ([]types.PendingAutomationPermission, error) {
	attempts, err := s.ListDispatchAttempts(ctx, types.DispatchAttemptFilter{
		WorkspaceRoot: workspaceRoot,
		Status:        types.DispatchAttemptStatusAwaitingApproval,
	})
	if err != nil {
		return nil, err
	}
	items := make([]types.PendingAutomationPermission, 0, len(attempts))
	for _, attempt := range attempts {
		requestID := strings.TrimSpace(attempt.PermissionRequestID)
		if requestID == "" {
			continue
		}
		request, ok, err := s.GetPermissionRequest(ctx, requestID)
		if err != nil {
			return nil, err
		}
		if !ok || request.Status != types.PermissionRequestStatusRequested {
			continue
		}
		continuation, ok, err := s.GetTurnContinuationByPermissionRequest(ctx, requestID)
		if err != nil {
			return nil, err
		}
		if !ok || continuation.State != types.TurnContinuationStatePending {
			continue
		}
		items = append(items, types.PendingAutomationPermission{
			RequestID:          requestID,
			WorkspaceRoot:      attempt.WorkspaceRoot,
			AutomationID:       attempt.AutomationID,
			IncidentID:         attempt.IncidentID,
			DispatchID:         attempt.DispatchID,
			PreferredSessionID: attempt.PreferredSessionID,
		})
	}
	return items, nil
}

func (s *Store) UpsertDeliveryRecord(ctx context.Context, delivery types.DeliveryRecord) error {
	return upsertDeliveryRecordWithExec(ctx, s.db, delivery)
}

func (t runtimeTx) UpsertDeliveryRecord(ctx context.Context, delivery types.DeliveryRecord) error {
	return upsertDeliveryRecordWithExec(ctx, t.tx, delivery)
}

func upsertDeliveryRecordWithExec(ctx context.Context, execer execContexter, delivery types.DeliveryRecord) error {
	delivery = normalizeDeliveryRecordForStore(delivery)
	payload, err := json.Marshal(delivery)
	if err != nil {
		return err
	}

	_, err = execer.ExecContext(ctx, `
		insert into automation_delivery_records (
			delivery_id, workspace_root, automation_id, incident_id, dispatch_id, payload, created_at, updated_at
		)
		values (?, ?, ?, ?, ?, ?, ?, ?)
		on conflict(delivery_id) do update set
			workspace_root = excluded.workspace_root,
			automation_id = excluded.automation_id,
			incident_id = excluded.incident_id,
			dispatch_id = excluded.dispatch_id,
			payload = excluded.payload,
			updated_at = excluded.updated_at
	`,
		delivery.DeliveryID,
		delivery.WorkspaceRoot,
		delivery.AutomationID,
		delivery.IncidentID,
		delivery.DispatchID,
		string(payload),
		delivery.CreatedAt.UTC().Format(timeLayout),
		delivery.UpdatedAt.UTC().Format(timeLayout),
	)
	return err
}

func (s *Store) GetDeliveryRecord(ctx context.Context, deliveryID string) (types.DeliveryRecord, bool, error) {
	return getDeliveryRecordWithQueryer(ctx, s.db, deliveryID)
}

func (t runtimeTx) GetDeliveryRecord(ctx context.Context, deliveryID string) (types.DeliveryRecord, bool, error) {
	return getDeliveryRecordWithQueryer(ctx, t.tx, deliveryID)
}

func getDeliveryRecordWithQueryer(ctx context.Context, queryer queryContexter, deliveryID string) (types.DeliveryRecord, bool, error) {
	deliveryID = strings.TrimSpace(deliveryID)
	if deliveryID == "" {
		return types.DeliveryRecord{}, false, nil
	}
	rows, err := queryer.QueryContext(ctx, `
		select payload, created_at, updated_at
		from automation_delivery_records
		where delivery_id = ?
	`, deliveryID)
	if err != nil {
		return types.DeliveryRecord{}, false, err
	}
	defer rows.Close()

	items, err := scanDeliveryRecords(rows)
	if err != nil {
		return types.DeliveryRecord{}, false, err
	}
	if len(items) == 0 {
		return types.DeliveryRecord{}, false, nil
	}
	return items[0], true, nil
}

func (s *Store) ListDeliveryRecords(ctx context.Context, filter types.DeliveryRecordFilter) ([]types.DeliveryRecord, error) {
	return listDeliveryRecordsWithQueryer(ctx, s.db, filter)
}

func (t runtimeTx) ListDeliveryRecords(ctx context.Context, filter types.DeliveryRecordFilter) ([]types.DeliveryRecord, error) {
	return listDeliveryRecordsWithQueryer(ctx, t.tx, filter)
}

func listDeliveryRecordsWithQueryer(ctx context.Context, queryer queryContexter, filter types.DeliveryRecordFilter) ([]types.DeliveryRecord, error) {
	filter = normalizeDeliveryRecordFilterForStore(filter)
	query := `
		select payload, created_at, updated_at
		from automation_delivery_records
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
	if filter.IncidentID != "" {
		conditions = append(conditions, "incident_id = ?")
		args = append(args, filter.IncidentID)
	}
	if filter.DispatchID != "" {
		conditions = append(conditions, "dispatch_id = ?")
		args = append(args, filter.DispatchID)
	}
	if len(conditions) > 0 {
		query += " where " + strings.Join(conditions, " and ")
	}
	query += " order by updated_at desc, created_at desc, delivery_id asc"
	if filter.Limit > 0 {
		query += " limit ?"
		args = append(args, filter.Limit)
	}

	rows, err := queryer.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanDeliveryRecords(rows)
}

func scanDispatchAttempts(rows *sql.Rows) ([]types.DispatchAttempt, error) {
	out := make([]types.DispatchAttempt, 0)
	for rows.Next() {
		var (
			payload   string
			createdAt string
			updatedAt string
		)
		if err := rows.Scan(&payload, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		var attempt types.DispatchAttempt
		if err := json.Unmarshal([]byte(payload), &attempt); err != nil {
			return nil, err
		}
		if parsed, err := parsePendingOptionalTime(createdAt); err == nil && !parsed.IsZero() {
			attempt.CreatedAt = parsed
		}
		if parsed, err := parsePendingOptionalTime(updatedAt); err == nil && !parsed.IsZero() {
			attempt.UpdatedAt = parsed
		}
		out = append(out, normalizeDispatchAttemptForStore(attempt))
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func scanDeliveryRecords(rows *sql.Rows) ([]types.DeliveryRecord, error) {
	out := make([]types.DeliveryRecord, 0)
	for rows.Next() {
		var (
			payload   string
			createdAt string
			updatedAt string
		)
		if err := rows.Scan(&payload, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		var delivery types.DeliveryRecord
		if err := json.Unmarshal([]byte(payload), &delivery); err != nil {
			return nil, err
		}
		if parsed, err := parsePendingOptionalTime(createdAt); err == nil && !parsed.IsZero() {
			delivery.CreatedAt = parsed
		}
		if parsed, err := parsePendingOptionalTime(updatedAt); err == nil && !parsed.IsZero() {
			delivery.UpdatedAt = parsed
		}
		out = append(out, normalizeDeliveryRecordForStore(delivery))
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}
