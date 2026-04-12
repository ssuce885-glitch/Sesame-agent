package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"time"

	"go-agent/internal/types"
)

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
		conditions = append(conditions, "workspace_root = ?")
		args = append(args, filter.WorkspaceRoot)
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
		conditions = append(conditions, "workspace_root = ?")
		args = append(args, filter.WorkspaceRoot)
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
		heartbeat.ID,
		heartbeat.AutomationID,
		heartbeat.WorkspaceRoot,
		heartbeat.Status,
		formatPendingOptionalTime(heartbeat.ObservedAt),
		string(payload),
		heartbeat.CreatedAt.UTC().Format(timeLayout),
		heartbeat.UpdatedAt.UTC().Format(timeLayout),
	)
	return err
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
		if parsed, err := parsePendingOptionalTime(createdAt); err == nil {
			spec.CreatedAt = parsed
		}
		if parsed, err := parsePendingOptionalTime(updatedAt); err == nil {
			spec.UpdatedAt = parsed
		}
		out = append(out, normalizeAutomationSpecForStore(spec))
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
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
		if parsed, err := parsePendingOptionalTime(observedAt); err == nil {
			incident.ObservedAt = parsed
		}
		if parsed, err := parsePendingOptionalTime(createdAt); err == nil {
			incident.CreatedAt = parsed
		}
		if parsed, err := parsePendingOptionalTime(updatedAt); err == nil {
			incident.UpdatedAt = parsed
		}
		out = append(out, normalizeAutomationIncidentForStore(incident))
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func normalizeAutomationSpecForStore(spec types.AutomationSpec) types.AutomationSpec {
	now := time.Now().UTC()
	spec.ID = strings.TrimSpace(spec.ID)
	if spec.ID == "" {
		spec.ID = types.NewID("automation")
	}
	spec.Title = strings.TrimSpace(spec.Title)
	spec.WorkspaceRoot = strings.TrimSpace(spec.WorkspaceRoot)
	spec.Goal = strings.TrimSpace(spec.Goal)
	spec.State = normalizeAutomationStateForStore(spec.State)
	spec.Assumptions = normalizeAutomationStringList(spec.Assumptions)

	spec.Context.Owner = strings.TrimSpace(spec.Context.Owner)
	spec.Context.Environment = strings.TrimSpace(spec.Context.Environment)
	spec.Context.Targets = normalizeAutomationStringList(spec.Context.Targets)
	if len(spec.Context.Labels) > 0 {
		labels := make(map[string]string, len(spec.Context.Labels))
		for key, value := range spec.Context.Labels {
			key = strings.TrimSpace(key)
			if key == "" {
				continue
			}
			labels[key] = strings.TrimSpace(value)
		}
		spec.Context.Labels = labels
	}

	for index := range spec.Signals {
		spec.Signals[index].Kind = strings.TrimSpace(spec.Signals[index].Kind)
		spec.Signals[index].Source = strings.TrimSpace(spec.Signals[index].Source)
		spec.Signals[index].Selector = strings.TrimSpace(spec.Signals[index].Selector)
		spec.Signals[index].Payload = normalizeAutomationRawJSON(spec.Signals[index].Payload)
	}
	spec.IncidentPolicy = normalizeAutomationRawJSON(spec.IncidentPolicy)
	spec.ResponsePlan = normalizeAutomationRawJSON(spec.ResponsePlan)
	spec.VerificationPlan = normalizeAutomationRawJSON(spec.VerificationPlan)
	spec.EscalationPolicy = normalizeAutomationRawJSON(spec.EscalationPolicy)
	spec.DeliveryPolicy = normalizeAutomationRawJSON(spec.DeliveryPolicy)
	spec.RuntimePolicy = normalizeAutomationRawJSON(spec.RuntimePolicy)
	spec.WatcherLifecycle = normalizeAutomationRawJSON(spec.WatcherLifecycle)
	spec.RetriggerPolicy = normalizeAutomationRawJSON(spec.RetriggerPolicy)
	spec.RunPolicy = normalizeAutomationRawJSON(spec.RunPolicy)

	if spec.CreatedAt.IsZero() {
		spec.CreatedAt = now
	} else {
		spec.CreatedAt = spec.CreatedAt.UTC()
	}
	if spec.UpdatedAt.IsZero() {
		spec.UpdatedAt = spec.CreatedAt
	} else {
		spec.UpdatedAt = spec.UpdatedAt.UTC()
	}
	if spec.UpdatedAt.Before(spec.CreatedAt) {
		spec.UpdatedAt = spec.CreatedAt
	}
	return spec
}

func normalizeAutomationIncidentForStore(incident types.AutomationIncident) types.AutomationIncident {
	now := time.Now().UTC()
	incident.ID = strings.TrimSpace(incident.ID)
	if incident.ID == "" {
		incident.ID = types.NewID("incident")
	}
	incident.AutomationID = strings.TrimSpace(incident.AutomationID)
	incident.WorkspaceRoot = strings.TrimSpace(incident.WorkspaceRoot)
	incident.Status = types.AutomationIncidentStatus(strings.ToLower(strings.TrimSpace(string(incident.Status))))
	if incident.Status == "" {
		incident.Status = types.AutomationIncidentStatusOpen
	}
	incident.TriggerKind = strings.TrimSpace(incident.TriggerKind)
	incident.Source = strings.TrimSpace(incident.Source)
	incident.Title = strings.TrimSpace(incident.Title)
	incident.Summary = strings.TrimSpace(incident.Summary)
	incident.Payload = normalizeAutomationRawJSON(incident.Payload)
	if incident.ObservedAt.IsZero() {
		incident.ObservedAt = now
	} else {
		incident.ObservedAt = incident.ObservedAt.UTC()
	}
	if incident.CreatedAt.IsZero() {
		incident.CreatedAt = now
	} else {
		incident.CreatedAt = incident.CreatedAt.UTC()
	}
	if incident.UpdatedAt.IsZero() {
		incident.UpdatedAt = incident.CreatedAt
	} else {
		incident.UpdatedAt = incident.UpdatedAt.UTC()
	}
	if incident.UpdatedAt.Before(incident.CreatedAt) {
		incident.UpdatedAt = incident.CreatedAt
	}
	return incident
}

func normalizeAutomationHeartbeatForStore(heartbeat types.AutomationHeartbeat) types.AutomationHeartbeat {
	now := time.Now().UTC()
	heartbeat.ID = strings.TrimSpace(heartbeat.ID)
	if heartbeat.ID == "" {
		heartbeat.ID = types.NewID("heartbeat")
	}
	heartbeat.AutomationID = strings.TrimSpace(heartbeat.AutomationID)
	heartbeat.WorkspaceRoot = strings.TrimSpace(heartbeat.WorkspaceRoot)
	heartbeat.Status = strings.TrimSpace(heartbeat.Status)
	heartbeat.Message = strings.TrimSpace(heartbeat.Message)
	heartbeat.Payload = normalizeAutomationRawJSON(heartbeat.Payload)
	if heartbeat.ObservedAt.IsZero() {
		heartbeat.ObservedAt = now
	} else {
		heartbeat.ObservedAt = heartbeat.ObservedAt.UTC()
	}
	if heartbeat.CreatedAt.IsZero() {
		heartbeat.CreatedAt = now
	} else {
		heartbeat.CreatedAt = heartbeat.CreatedAt.UTC()
	}
	if heartbeat.UpdatedAt.IsZero() {
		heartbeat.UpdatedAt = heartbeat.CreatedAt
	} else {
		heartbeat.UpdatedAt = heartbeat.UpdatedAt.UTC()
	}
	if heartbeat.UpdatedAt.Before(heartbeat.CreatedAt) {
		heartbeat.UpdatedAt = heartbeat.CreatedAt
	}
	return heartbeat
}

func normalizeAutomationStateForStore(state types.AutomationState) types.AutomationState {
	state = types.AutomationState(strings.ToLower(strings.TrimSpace(string(state))))
	if state == "" {
		return types.AutomationStateActive
	}
	return state
}

func normalizeAutomationListFilterForStore(filter types.AutomationListFilter) types.AutomationListFilter {
	filter.WorkspaceRoot = strings.TrimSpace(filter.WorkspaceRoot)
	filter.State = types.AutomationState(strings.ToLower(strings.TrimSpace(string(filter.State))))
	if filter.Limit < 0 {
		filter.Limit = 0
	}
	return filter
}

func normalizeAutomationIncidentFilterForStore(filter types.AutomationIncidentFilter) types.AutomationIncidentFilter {
	filter.WorkspaceRoot = strings.TrimSpace(filter.WorkspaceRoot)
	filter.AutomationID = strings.TrimSpace(filter.AutomationID)
	filter.Status = types.AutomationIncidentStatus(strings.ToLower(strings.TrimSpace(string(filter.Status))))
	if filter.Limit < 0 {
		filter.Limit = 0
	}
	return filter
}

func normalizeAutomationStringList(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func normalizeAutomationRawJSON(raw json.RawMessage) json.RawMessage {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" {
		return nil
	}
	return json.RawMessage(trimmed)
}
