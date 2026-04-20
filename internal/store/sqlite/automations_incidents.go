package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"

	"go-agent/internal/types"
)

func (s *Store) UpsertAutomationIncident(ctx context.Context, incident types.AutomationIncident) error {
	return upsertAutomationIncidentWithExec(ctx, s.db, incident)
}

func (t runtimeTx) UpsertAutomationIncident(ctx context.Context, incident types.AutomationIncident) error {
	return upsertAutomationIncidentWithExec(ctx, t.tx, incident)
}

func upsertAutomationIncidentWithExec(ctx context.Context, execer execContexter, incident types.AutomationIncident) error {
	incident = normalizeAutomationIncidentForStore(incident)
	payload, err := json.Marshal(incident)
	if err != nil {
		return err
	}

	_, err = execer.ExecContext(ctx, `
		insert into automation_incidents (
			id, automation_id, workspace_root, status, observed_at, payload, created_at, updated_at
		)
		values (?, ?, ?, ?, ?, ?, ?, ?)
		on conflict(id) do update set
			automation_id = excluded.automation_id,
			workspace_root = excluded.workspace_root,
			status = excluded.status,
			observed_at = excluded.observed_at,
			payload = excluded.payload,
			updated_at = excluded.updated_at
	`,
		incident.ID,
		incident.AutomationID,
		incident.WorkspaceRoot,
		incident.Status,
		formatPendingOptionalTime(incident.ObservedAt),
		string(payload),
		incident.CreatedAt.UTC().Format(timeLayout),
		incident.UpdatedAt.UTC().Format(timeLayout),
	)
	return err
}

func (s *Store) GetAutomationIncident(ctx context.Context, id string) (types.AutomationIncident, bool, error) {
	return getAutomationIncidentWithQueryer(ctx, s.db, id)
}

func (t runtimeTx) GetAutomationIncident(ctx context.Context, id string) (types.AutomationIncident, bool, error) {
	return getAutomationIncidentWithQueryer(ctx, t.tx, id)
}

func getAutomationIncidentWithQueryer(ctx context.Context, queryer queryContexter, id string) (types.AutomationIncident, bool, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return types.AutomationIncident{}, false, nil
	}
	rows, err := queryer.QueryContext(ctx, `
		select payload, observed_at, created_at, updated_at
		from automation_incidents
		where id = ?
	`, id)
	if err != nil {
		return types.AutomationIncident{}, false, err
	}
	defer rows.Close()

	items, err := scanAutomationIncidents(rows)
	if err != nil {
		return types.AutomationIncident{}, false, err
	}
	if len(items) == 0 {
		return types.AutomationIncident{}, false, nil
	}
	return items[0], true, nil
}

func (s *Store) ListAutomationIncidents(ctx context.Context, filter types.AutomationIncidentFilter) ([]types.AutomationIncident, error) {
	return listAutomationIncidentsWithQueryer(ctx, s.db, filter)
}

func (t runtimeTx) ListAutomationIncidents(ctx context.Context, filter types.AutomationIncidentFilter) ([]types.AutomationIncident, error) {
	return listAutomationIncidentsWithQueryer(ctx, t.tx, filter)
}

func listAutomationIncidentsWithQueryer(ctx context.Context, queryer queryContexter, filter types.AutomationIncidentFilter) ([]types.AutomationIncident, error) {
	filter = normalizeAutomationIncidentFilterForStore(filter)
	query := `
		select payload, observed_at, created_at, updated_at
		from automation_incidents
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
	if filter.Status != "" {
		conditions = append(conditions, "status = ?")
		args = append(args, filter.Status)
	}
	if len(conditions) > 0 {
		query += " where " + strings.Join(conditions, " and ")
	}
	query += " order by observed_at desc, created_at desc, id asc"
	if filter.Limit > 0 {
		query += " limit ?"
		args = append(args, filter.Limit)
	}

	rows, err := queryer.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAutomationIncidents(rows)
}

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
			event_id, workspace_root, automation_id, incident_id, dedupe_key, signal_kind,
			source, observed_at, payload, created_at, updated_at
		)
		values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		on conflict(event_id) do update set
			workspace_root = excluded.workspace_root,
			automation_id = excluded.automation_id,
			incident_id = excluded.incident_id,
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
		event.IncidentID,
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
	if filter.IncidentID != "" {
		conditions = append(conditions, "incident_id = ?")
		args = append(args, filter.IncidentID)
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

func (s *Store) UpsertIncidentPhaseState(ctx context.Context, state types.IncidentPhaseState) error {
	return upsertIncidentPhaseStateWithExec(ctx, s.db, state)
}

func (t runtimeTx) UpsertIncidentPhaseState(ctx context.Context, state types.IncidentPhaseState) error {
	return upsertIncidentPhaseStateWithExec(ctx, t.tx, state)
}

func upsertIncidentPhaseStateWithExec(ctx context.Context, execer execContexter, state types.IncidentPhaseState) error {
	state = normalizeIncidentPhaseStateForStore(state)
	payload, err := json.Marshal(state)
	if err != nil {
		return err
	}

	_, err = execer.ExecContext(ctx, `
		insert into automation_incident_phase_states (
			incident_id, phase, automation_id, workspace_root, status, payload, created_at, updated_at
		)
		values (?, ?, ?, ?, ?, ?, ?, ?)
		on conflict(incident_id, phase) do update set
			automation_id = excluded.automation_id,
			workspace_root = excluded.workspace_root,
			status = excluded.status,
			payload = excluded.payload,
			updated_at = excluded.updated_at
	`,
		state.IncidentID,
		state.Phase,
		state.AutomationID,
		state.WorkspaceRoot,
		state.Status,
		string(payload),
		state.CreatedAt.UTC().Format(timeLayout),
		state.UpdatedAt.UTC().Format(timeLayout),
	)
	return err
}

func (s *Store) ListIncidentPhaseStates(ctx context.Context, incidentID string) ([]types.IncidentPhaseState, error) {
	return listIncidentPhaseStatesWithQueryer(ctx, s.db, incidentID)
}

func (t runtimeTx) ListIncidentPhaseStates(ctx context.Context, incidentID string) ([]types.IncidentPhaseState, error) {
	return listIncidentPhaseStatesWithQueryer(ctx, t.tx, incidentID)
}

func listIncidentPhaseStatesWithQueryer(ctx context.Context, queryer queryContexter, incidentID string) ([]types.IncidentPhaseState, error) {
	incidentID = strings.TrimSpace(incidentID)
	if incidentID == "" {
		return []types.IncidentPhaseState{}, nil
	}

	rows, err := queryer.QueryContext(ctx, `
		select payload, created_at, updated_at
		from automation_incident_phase_states
		where incident_id = ?
		order by created_at asc, phase asc
	`, incidentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanIncidentPhaseStates(rows)
}

func scanAutomationIncidents(rows *sql.Rows) ([]types.AutomationIncident, error) {
	out := make([]types.AutomationIncident, 0)
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
		var incident types.AutomationIncident
		if err := json.Unmarshal([]byte(payload), &incident); err != nil {
			return nil, err
		}
		if parsed, err := parsePendingOptionalTime(observedAt); err == nil && !parsed.IsZero() {
			incident.ObservedAt = parsed
		}
		if parsed, err := parsePendingOptionalTime(createdAt); err == nil && !parsed.IsZero() {
			incident.CreatedAt = parsed
		}
		if parsed, err := parsePendingOptionalTime(updatedAt); err == nil && !parsed.IsZero() {
			incident.UpdatedAt = parsed
		}
		out = append(out, normalizeAutomationIncidentForStore(incident))
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
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

func scanIncidentPhaseStates(rows *sql.Rows) ([]types.IncidentPhaseState, error) {
	out := make([]types.IncidentPhaseState, 0)
	for rows.Next() {
		var (
			payload   string
			createdAt string
			updatedAt string
		)
		if err := rows.Scan(&payload, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		var state types.IncidentPhaseState
		if err := json.Unmarshal([]byte(payload), &state); err != nil {
			return nil, err
		}
		if parsed, err := parsePendingOptionalTime(createdAt); err == nil && !parsed.IsZero() {
			state.CreatedAt = parsed
		}
		if parsed, err := parsePendingOptionalTime(updatedAt); err == nil && !parsed.IsZero() {
			state.UpdatedAt = parsed
		}
		out = append(out, normalizeIncidentPhaseStateForStore(state))
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}
