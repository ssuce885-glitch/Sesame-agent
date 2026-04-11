package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"go-agent/internal/types"
)

func (s *Store) UpsertChildAgentSpec(ctx context.Context, spec types.ChildAgentSpec) error {
	return upsertSessionReportingObject(ctx, s.db, "child_agent_specs", spec.AgentID, spec.SessionID, &spec)
}

func (t runtimeTx) UpsertChildAgentSpec(ctx context.Context, spec types.ChildAgentSpec) error {
	return upsertSessionReportingObject(ctx, t.tx, "child_agent_specs", spec.AgentID, spec.SessionID, &spec)
}

func (s *Store) GetChildAgentSpec(ctx context.Context, agentID string) (types.ChildAgentSpec, bool, error) {
	var spec types.ChildAgentSpec
	ok, err := getReportingObject(ctx, s.db, "child_agent_specs", agentID, &spec)
	return spec, ok, err
}

func (s *Store) DeleteChildAgentSpec(ctx context.Context, agentID string) (bool, error) {
	return deleteReportingObject(ctx, s.db, "child_agent_specs", agentID)
}

func (t runtimeTx) DeleteChildAgentSpec(ctx context.Context, agentID string) (bool, error) {
	return deleteReportingObject(ctx, t.tx, "child_agent_specs", agentID)
}

func (s *Store) ListChildAgentSpecs(ctx context.Context) ([]types.ChildAgentSpec, error) {
	return listReportingObjectsAs[types.ChildAgentSpec](ctx, s.db, `
		select payload, created_at, updated_at
		from child_agent_specs
		order by updated_at desc, created_at desc, id asc
	`)
}

func (s *Store) ListChildAgentSpecsBySession(ctx context.Context, sessionID string) ([]types.ChildAgentSpec, error) {
	return listReportingObjectsAs[types.ChildAgentSpec](ctx, s.db, `
		select payload, created_at, updated_at
		from child_agent_specs
		where session_id = ?
		order by updated_at desc, created_at desc, id asc
	`, sessionID)
}

func (s *Store) UpsertOutputContract(ctx context.Context, contract types.OutputContract) error {
	return upsertReportingObject(ctx, s.db, "output_contracts", contract.ContractID, &contract)
}

func (t runtimeTx) UpsertOutputContract(ctx context.Context, contract types.OutputContract) error {
	return upsertReportingObject(ctx, t.tx, "output_contracts", contract.ContractID, &contract)
}

func (s *Store) GetOutputContract(ctx context.Context, contractID string) (types.OutputContract, bool, error) {
	var contract types.OutputContract
	ok, err := getReportingObject(ctx, s.db, "output_contracts", contractID, &contract)
	return contract, ok, err
}

func (s *Store) ListOutputContracts(ctx context.Context) ([]types.OutputContract, error) {
	return listReportingObjectsAs[types.OutputContract](ctx, s.db, `
		select payload, created_at, updated_at
		from output_contracts
		order by updated_at desc, created_at desc, id asc
	`)
}

func (s *Store) UpsertReportGroup(ctx context.Context, group types.ReportGroup) error {
	return upsertSessionReportingObject(ctx, s.db, "report_groups", group.GroupID, group.SessionID, &group)
}

func (t runtimeTx) UpsertReportGroup(ctx context.Context, group types.ReportGroup) error {
	return upsertSessionReportingObject(ctx, t.tx, "report_groups", group.GroupID, group.SessionID, &group)
}

func (s *Store) GetReportGroup(ctx context.Context, groupID string) (types.ReportGroup, bool, error) {
	var group types.ReportGroup
	ok, err := getReportingObject(ctx, s.db, "report_groups", groupID, &group)
	return group, ok, err
}

func (t runtimeTx) GetReportGroup(ctx context.Context, groupID string) (types.ReportGroup, bool, error) {
	var group types.ReportGroup
	ok, err := getReportingObject(ctx, t.tx, "report_groups", groupID, &group)
	return group, ok, err
}

func (s *Store) ListReportGroups(ctx context.Context) ([]types.ReportGroup, error) {
	return listReportingObjectsAs[types.ReportGroup](ctx, s.db, `
		select payload, created_at, updated_at
		from report_groups
		order by updated_at desc, created_at desc, id asc
	`)
}

func (s *Store) ListReportGroupsBySession(ctx context.Context, sessionID string) ([]types.ReportGroup, error) {
	return listReportingObjectsAs[types.ReportGroup](ctx, s.db, `
		select payload, created_at, updated_at
		from report_groups
		where session_id = ?
		order by updated_at desc, created_at desc, id asc
	`, sessionID)
}

func (s *Store) UpsertChildAgentResult(ctx context.Context, result types.ChildAgentResult) error {
	return upsertChildAgentResultWithExec(ctx, s.db, result)
}

func (t runtimeTx) UpsertChildAgentResult(ctx context.Context, result types.ChildAgentResult) error {
	return upsertChildAgentResultWithExec(ctx, t.tx, result)
}

func (s *Store) GetChildAgentResult(ctx context.Context, resultID string) (types.ChildAgentResult, bool, error) {
	var result types.ChildAgentResult
	ok, err := getReportingObject(ctx, s.db, "child_agent_results", resultID, &result)
	return result, ok, err
}

func (s *Store) ListChildAgentResults(ctx context.Context) ([]types.ChildAgentResult, error) {
	return listReportingObjectsAs[types.ChildAgentResult](ctx, s.db, `
		select payload, created_at, updated_at
		from child_agent_results
		order by observed_at desc, created_at desc, id asc
	`)
}

func (s *Store) ListChildAgentResultsBySession(ctx context.Context, sessionID string) ([]types.ChildAgentResult, error) {
	return listReportingObjectsAs[types.ChildAgentResult](ctx, s.db, `
		select payload, created_at, updated_at
		from child_agent_results
		where session_id = ?
		order by observed_at desc, created_at desc, id asc
	`, sessionID)
}

func (s *Store) ListChildAgentResultsByAgent(ctx context.Context, agentID string) ([]types.ChildAgentResult, error) {
	return listReportingObjectsAs[types.ChildAgentResult](ctx, s.db, `
		select payload, created_at, updated_at
		from child_agent_results
		where agent_id = ?
		order by observed_at desc, created_at desc, id asc
	`, agentID)
}

func (s *Store) ListChildAgentResultsByReportGroup(ctx context.Context, groupID string) ([]types.ChildAgentResult, error) {
	all, err := s.ListChildAgentResults(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]types.ChildAgentResult, 0, len(all))
	for _, result := range all {
		if containsReportingString(result.ReportGroupRefs, groupID) {
			out = append(out, result)
		}
	}
	return out, nil
}

func (s *Store) ListChildAgentResultsBySessionAndReportGroup(ctx context.Context, sessionID, groupID string) ([]types.ChildAgentResult, error) {
	all, err := s.ListChildAgentResultsBySession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	out := make([]types.ChildAgentResult, 0, len(all))
	for _, result := range all {
		if containsReportingString(result.ReportGroupRefs, groupID) {
			out = append(out, result)
		}
	}
	return out, nil
}

func (s *Store) UpsertDigestRecord(ctx context.Context, digest types.DigestRecord) error {
	return upsertDigestRecordWithExec(ctx, s.db, digest)
}

func (t runtimeTx) UpsertDigestRecord(ctx context.Context, digest types.DigestRecord) error {
	return upsertDigestRecordWithExec(ctx, t.tx, digest)
}

func (s *Store) GetDigestRecord(ctx context.Context, digestID string) (types.DigestRecord, bool, error) {
	var digest types.DigestRecord
	ok, err := getReportingObject(ctx, s.db, "digest_records", digestID, &digest)
	return digest, ok, err
}

func (s *Store) ListDigestRecords(ctx context.Context) ([]types.DigestRecord, error) {
	return listReportingObjectsAs[types.DigestRecord](ctx, s.db, `
		select payload, created_at, updated_at
		from digest_records
		order by window_end desc, created_at desc, id asc
	`)
}

func (s *Store) ListDigestRecordsBySession(ctx context.Context, sessionID string) ([]types.DigestRecord, error) {
	return listReportingObjectsAs[types.DigestRecord](ctx, s.db, `
		select payload, created_at, updated_at
		from digest_records
		where session_id = ?
		order by window_end desc, created_at desc, id asc
	`, sessionID)
}

func (s *Store) ListDigestRecordsByGroup(ctx context.Context, groupID string) ([]types.DigestRecord, error) {
	return listReportingObjectsAs[types.DigestRecord](ctx, s.db, `
		select payload, created_at, updated_at
		from digest_records
		where group_id = ?
		order by window_end desc, created_at desc, id asc
	`, groupID)
}

func (s *Store) ListDigestRecordsBySessionAndGroup(ctx context.Context, sessionID, groupID string) ([]types.DigestRecord, error) {
	return listReportingObjectsAs[types.DigestRecord](ctx, s.db, `
		select payload, created_at, updated_at
		from digest_records
		where session_id = ? and group_id = ?
		order by window_end desc, created_at desc, id asc
	`, sessionID, groupID)
}

func upsertReportingObject(ctx context.Context, execer execContexter, table, id string, value any) error {
	createdAt, updatedAt := normalizeReportingTimestamps(value)
	applyReportingTimestamps(value, createdAt, updatedAt)

	payload, err := json.Marshal(value)
	if err != nil {
		return err
	}

	_, err = execer.ExecContext(ctx, `
		insert into `+table+` (id, payload, created_at, updated_at)
		values (?, ?, ?, ?)
		on conflict(id) do update set
			payload = excluded.payload,
			updated_at = excluded.updated_at
	`, id, string(payload), createdAt.Format(timeLayout), updatedAt.Format(timeLayout))
	return err
}

func upsertSessionReportingObject(ctx context.Context, execer execContexter, table, id, sessionID string, value any) error {
	createdAt, updatedAt := normalizeReportingTimestamps(value)
	applyReportingTimestamps(value, createdAt, updatedAt)

	payload, err := json.Marshal(value)
	if err != nil {
		return err
	}

	_, err = execer.ExecContext(ctx, `
		insert into `+table+` (id, session_id, payload, created_at, updated_at)
		values (?, ?, ?, ?, ?)
		on conflict(id) do update set
			session_id = excluded.session_id,
			payload = excluded.payload,
			updated_at = excluded.updated_at
	`, id, sessionID, string(payload), createdAt.Format(timeLayout), updatedAt.Format(timeLayout))
	return err
}

func upsertChildAgentResultWithExec(ctx context.Context, execer execContexter, result types.ChildAgentResult) error {
	createdAt, updatedAt := normalizeReportingTimestamps(&result)
	result.ObservedAt = normalizeObservedAt(result.ObservedAt, updatedAt)
	applyReportingTimestamps(&result, createdAt, updatedAt)

	payload, err := json.Marshal(result)
	if err != nil {
		return err
	}

	_, err = execer.ExecContext(ctx, `
		insert into child_agent_results (id, session_id, agent_id, status, severity, observed_at, payload, created_at, updated_at)
		values (?, ?, ?, ?, ?, ?, ?, ?, ?)
		on conflict(id) do update set
			session_id = excluded.session_id,
			agent_id = excluded.agent_id,
			status = excluded.status,
			severity = excluded.severity,
			observed_at = excluded.observed_at,
			payload = excluded.payload,
			updated_at = excluded.updated_at
	`,
		result.ResultID,
		result.SessionID,
		result.AgentID,
		result.Envelope.Status,
		result.Envelope.Severity,
		result.ObservedAt.Format(timeLayout),
		string(payload),
		createdAt.Format(timeLayout),
		updatedAt.Format(timeLayout),
	)
	return err
}

func upsertDigestRecordWithExec(ctx context.Context, execer execContexter, digest types.DigestRecord) error {
	createdAt, updatedAt := normalizeReportingTimestamps(&digest)
	digest.WindowStart, digest.WindowEnd = normalizeDigestWindow(digest.WindowStart, digest.WindowEnd, createdAt, updatedAt)
	applyReportingTimestamps(&digest, createdAt, updatedAt)

	payload, err := json.Marshal(digest)
	if err != nil {
		return err
	}

	_, err = execer.ExecContext(ctx, `
		insert into digest_records (id, session_id, group_id, status, severity, window_start, window_end, payload, created_at, updated_at)
		values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		on conflict(id) do update set
			session_id = excluded.session_id,
			group_id = excluded.group_id,
			status = excluded.status,
			severity = excluded.severity,
			window_start = excluded.window_start,
			window_end = excluded.window_end,
			payload = excluded.payload,
			updated_at = excluded.updated_at
	`,
		digest.DigestID,
		digest.SessionID,
		digest.GroupID,
		digest.Envelope.Status,
		digest.Envelope.Severity,
		digest.WindowStart.Format(timeLayout),
		digest.WindowEnd.Format(timeLayout),
		string(payload),
		createdAt.Format(timeLayout),
		updatedAt.Format(timeLayout),
	)
	return err
}

func getReportingObject(ctx context.Context, queryer queryRowContexter, table, id string, target any) (bool, error) {
	var (
		rawPayload string
		createdAt  string
		updatedAt  string
	)
	err := queryer.QueryRowContext(ctx, `
		select payload, created_at, updated_at
		from `+table+`
		where id = ?
	`, id).Scan(&rawPayload, &createdAt, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	if err := json.Unmarshal([]byte(rawPayload), target); err != nil {
		return false, err
	}
	createdParsed, err := time.Parse(timeLayout, createdAt)
	if err != nil {
		return false, err
	}
	updatedParsed, err := time.Parse(timeLayout, updatedAt)
	if err != nil {
		return false, err
	}
	applyReportingTimestamps(target, createdParsed, updatedParsed)
	return true, nil
}

func deleteReportingObject(ctx context.Context, execer execContexter, table, id string) (bool, error) {
	result, err := execer.ExecContext(ctx, `delete from `+table+` where id = ?`, id)
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return affected > 0, nil
}

func listReportingObjectsAs[T any](ctx context.Context, queryer queryContexter, query string, args ...any) ([]T, error) {
	rows, err := queryer.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]T, 0)
	for rows.Next() {
		var (
			rawPayload string
			createdAt  string
			updatedAt  string
		)
		if err := rows.Scan(&rawPayload, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		createdParsed, err := time.Parse(timeLayout, createdAt)
		if err != nil {
			return nil, err
		}
		updatedParsed, err := time.Parse(timeLayout, updatedAt)
		if err != nil {
			return nil, err
		}

		var item T
		if err := json.Unmarshal([]byte(rawPayload), &item); err != nil {
			return nil, err
		}
		applyReportingTimestamps(&item, createdParsed, updatedParsed)
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

type queryContexter interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func normalizeReportingTimestamps(value any) (time.Time, time.Time) {
	now := time.Now().UTC()
	switch v := value.(type) {
	case *types.ChildAgentSpec:
		createdAt := v.CreatedAt.UTC()
		if createdAt.IsZero() {
			createdAt = now
		}
		updatedAt := v.UpdatedAt.UTC()
		if updatedAt.IsZero() {
			updatedAt = now
		}
		return createdAt, updatedAt
	case *types.OutputContract:
		createdAt := v.CreatedAt.UTC()
		if createdAt.IsZero() {
			createdAt = now
		}
		updatedAt := v.UpdatedAt.UTC()
		if updatedAt.IsZero() {
			updatedAt = now
		}
		return createdAt, updatedAt
	case *types.ReportGroup:
		createdAt := v.CreatedAt.UTC()
		if createdAt.IsZero() {
			createdAt = now
		}
		updatedAt := v.UpdatedAt.UTC()
		if updatedAt.IsZero() {
			updatedAt = now
		}
		return createdAt, updatedAt
	case *types.ChildAgentResult:
		createdAt := v.CreatedAt.UTC()
		if createdAt.IsZero() {
			createdAt = now
		}
		updatedAt := v.UpdatedAt.UTC()
		if updatedAt.IsZero() {
			updatedAt = now
		}
		return createdAt, updatedAt
	case *types.DigestRecord:
		createdAt := v.CreatedAt.UTC()
		if createdAt.IsZero() {
			createdAt = now
		}
		updatedAt := v.UpdatedAt.UTC()
		if updatedAt.IsZero() {
			updatedAt = now
		}
		return createdAt, updatedAt
	default:
		return now, now
	}
}

func applyReportingTimestamps(target any, createdAt, updatedAt time.Time) {
	switch v := target.(type) {
	case *types.ChildAgentSpec:
		v.CreatedAt = createdAt.UTC()
		v.UpdatedAt = updatedAt.UTC()
	case *types.OutputContract:
		v.CreatedAt = createdAt.UTC()
		v.UpdatedAt = updatedAt.UTC()
	case *types.ReportGroup:
		v.CreatedAt = createdAt.UTC()
		v.UpdatedAt = updatedAt.UTC()
	case *types.ChildAgentResult:
		v.CreatedAt = createdAt.UTC()
		v.UpdatedAt = updatedAt.UTC()
	case *types.DigestRecord:
		v.CreatedAt = createdAt.UTC()
		v.UpdatedAt = updatedAt.UTC()
	}
}

func normalizeObservedAt(observedAt, fallback time.Time) time.Time {
	observedAt = observedAt.UTC()
	if observedAt.IsZero() {
		return fallback.UTC()
	}
	return observedAt
}

func normalizeDigestWindow(windowStart, windowEnd, createdAt, updatedAt time.Time) (time.Time, time.Time) {
	windowStart = windowStart.UTC()
	windowEnd = windowEnd.UTC()
	if windowEnd.IsZero() {
		windowEnd = updatedAt.UTC()
	}
	if windowStart.IsZero() {
		windowStart = createdAt.UTC()
	}
	if windowStart.After(windowEnd) {
		windowStart = windowEnd
	}
	return windowStart, windowEnd
}

func containsReportingString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
