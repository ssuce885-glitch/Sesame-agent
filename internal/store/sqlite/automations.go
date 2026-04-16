package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"

	"go-agent/internal/types"
)

func appendAutomationWorkspaceRootCondition(conditions *[]string, args *[]any, column, workspaceRoot string) {
	workspaceRoot = strings.TrimSpace(workspaceRoot)
	if workspaceRoot == "" {
		return
	}
	escaped := escapeSQLiteLikePattern(workspaceRoot)
	*conditions = append(*conditions, "("+column+" = ? or "+column+" like ? escape '\\' or "+column+" like ? escape '\\')")
	*args = append(*args,
		workspaceRoot,
		escaped+"/automations/%",
		escaped+"\\\\automations\\\\%",
	)
}

func escapeSQLiteLikePattern(value string) string {
	replacer := strings.NewReplacer(
		`\`, `\\`,
		`%`, `\%`,
		`_`, `\_`,
	)
	return replacer.Replace(value)
}

func (s *Store) UpsertAutomation(ctx context.Context, spec types.AutomationSpec) error {
	return upsertAutomationWithExec(ctx, s.db, spec)
}

func (t runtimeTx) UpsertAutomation(ctx context.Context, spec types.AutomationSpec) error {
	return upsertAutomationWithExec(ctx, t.tx, spec)
}

func upsertAutomationWithExec(ctx context.Context, execer execContexter, spec types.AutomationSpec) error {
	spec = normalizeAutomationSpecForStore(spec)
	payload, err := json.Marshal(spec)
	if err != nil {
		return err
	}

	_, err = execer.ExecContext(ctx, `
		insert into automations (
			id, workspace_root, state, payload, created_at, updated_at
		)
		values (?, ?, ?, ?, ?, ?)
		on conflict(id) do update set
			workspace_root = excluded.workspace_root,
			state = excluded.state,
			payload = excluded.payload,
			updated_at = excluded.updated_at
	`,
		spec.ID,
		spec.WorkspaceRoot,
		spec.State,
		string(payload),
		spec.CreatedAt.UTC().Format(timeLayout),
		spec.UpdatedAt.UTC().Format(timeLayout),
	)
	return err
}

func (s *Store) GetAutomation(ctx context.Context, id string) (types.AutomationSpec, bool, error) {
	return getAutomationWithQueryer(ctx, s.db, id)
}

func (t runtimeTx) GetAutomation(ctx context.Context, id string) (types.AutomationSpec, bool, error) {
	return getAutomationWithQueryer(ctx, t.tx, id)
}

func getAutomationWithQueryer(ctx context.Context, queryer queryContexter, id string) (types.AutomationSpec, bool, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return types.AutomationSpec{}, false, nil
	}
	rows, err := queryer.QueryContext(ctx, `
		select payload, created_at, updated_at
		from automations
		where id = ?
	`, id)
	if err != nil {
		return types.AutomationSpec{}, false, err
	}
	defer rows.Close()

	items, err := scanAutomationSpecs(rows)
	if err != nil {
		return types.AutomationSpec{}, false, err
	}
	if len(items) == 0 {
		return types.AutomationSpec{}, false, nil
	}
	return items[0], true, nil
}

func (s *Store) ListAutomations(ctx context.Context, filter types.AutomationListFilter) ([]types.AutomationSpec, error) {
	return listAutomationsWithQueryer(ctx, s.db, filter)
}

func (t runtimeTx) ListAutomations(ctx context.Context, filter types.AutomationListFilter) ([]types.AutomationSpec, error) {
	return listAutomationsWithQueryer(ctx, t.tx, filter)
}

func listAutomationsWithQueryer(ctx context.Context, queryer queryContexter, filter types.AutomationListFilter) ([]types.AutomationSpec, error) {
	filter = normalizeAutomationListFilterForStore(filter)
	query := `
		select payload, created_at, updated_at
		from automations
	`
	args := make([]any, 0, 3)
	conditions := make([]string, 0, 2)
	if filter.WorkspaceRoot != "" {
		appendAutomationWorkspaceRootCondition(&conditions, &args, "workspace_root", filter.WorkspaceRoot)
	}
	if filter.State != "" {
		conditions = append(conditions, "state = ?")
		args = append(args, filter.State)
	}
	if len(conditions) > 0 {
		query += " where " + strings.Join(conditions, " and ")
	}
	query += " order by updated_at desc, created_at desc, id asc"
	if filter.Limit > 0 {
		query += " limit ?"
		args = append(args, filter.Limit)
	}

	rows, err := queryer.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAutomationSpecs(rows)
}

func (s *Store) DeleteAutomation(ctx context.Context, id string) (bool, error) {
	return deleteAutomationWithExec(ctx, s.db, id)
}

func (t runtimeTx) DeleteAutomation(ctx context.Context, id string) (bool, error) {
	return deleteAutomationWithExec(ctx, t.tx, id)
}

func deleteAutomationWithExec(ctx context.Context, execer execContexter, id string) (bool, error) {
	result, err := execer.ExecContext(ctx, `
		delete from automations
		where id = ?
	`, strings.TrimSpace(id))
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return affected > 0, nil
}

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
			task_id, background_session_id, background_turn_id, payload, created_at, updated_at
		)
		values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		on conflict(dispatch_id) do update set
			workspace_root = excluded.workspace_root,
			automation_id = excluded.automation_id,
			incident_id = excluded.incident_id,
			phase = excluded.phase,
			status = excluded.status,
			task_id = excluded.task_id,
			background_session_id = excluded.background_session_id,
			background_turn_id = excluded.background_turn_id,
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
		attempt.BackgroundSessionID,
		attempt.BackgroundTurnID,
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

func (s *Store) FindDispatchAttemptByBackgroundRun(ctx context.Context, sessionID, turnID string) (types.DispatchAttempt, bool, error) {
	sessionID = strings.TrimSpace(sessionID)
	turnID = strings.TrimSpace(turnID)
	if sessionID == "" || turnID == "" {
		return types.DispatchAttempt{}, false, nil
	}
	rows, err := s.db.QueryContext(ctx, `
		select payload, created_at, updated_at
		from automation_dispatch_attempts
		where background_session_id = ? and background_turn_id = ?
		order by updated_at desc, created_at desc, dispatch_id asc
		limit 1
	`, sessionID, turnID)
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
			RequestID:           requestID,
			WorkspaceRoot:       attempt.WorkspaceRoot,
			AutomationID:        attempt.AutomationID,
			IncidentID:          attempt.IncidentID,
			DispatchID:          attempt.DispatchID,
			BackgroundSessionID: attempt.BackgroundSessionID,
			BackgroundTurnID:    attempt.BackgroundTurnID,
			PreferredSessionID:  attempt.PreferredSessionID,
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

func scanAutomationSpecs(rows *sql.Rows) ([]types.AutomationSpec, error) {
	out := make([]types.AutomationSpec, 0)
	for rows.Next() {
		var (
			payload   string
			createdAt string
			updatedAt string
		)
		if err := rows.Scan(&payload, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		var spec types.AutomationSpec
		if err := json.Unmarshal([]byte(payload), &spec); err != nil {
			return nil, err
		}
		if parsed, err := parsePendingOptionalTime(createdAt); err == nil && !parsed.IsZero() {
			spec.CreatedAt = parsed
		}
		if parsed, err := parsePendingOptionalTime(updatedAt); err == nil && !parsed.IsZero() {
			spec.UpdatedAt = parsed
		}
		out = append(out, normalizeAutomationSpecForStore(spec))
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
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
