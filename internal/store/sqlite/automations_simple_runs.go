package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"time"

	"go-agent/internal/types"
)

func (s *Store) UpsertSimpleAutomationRun(ctx context.Context, run types.SimpleAutomationRun) error {
	return upsertSimpleAutomationRunWithExec(ctx, s.db, run)
}

func (t runtimeTx) UpsertSimpleAutomationRun(ctx context.Context, run types.SimpleAutomationRun) error {
	return upsertSimpleAutomationRunWithExec(ctx, t.tx, run)
}

func (s *Store) ClaimSimpleAutomationRun(ctx context.Context, run types.SimpleAutomationRun) (bool, error) {
	return claimSimpleAutomationRunWithExec(ctx, s.db, run)
}

func (t runtimeTx) ClaimSimpleAutomationRun(ctx context.Context, run types.SimpleAutomationRun) (bool, error) {
	return claimSimpleAutomationRunWithExec(ctx, t.tx, run)
}

func claimSimpleAutomationRunWithExec(ctx context.Context, execer execContexter, run types.SimpleAutomationRun) (bool, error) {
	run = normalizeSimpleAutomationRunForStore(run)
	if run.AutomationID == "" || run.DedupeKey == "" {
		return false, nil
	}
	payload, err := json.Marshal(run)
	if err != nil {
		return false, err
	}

	result, err := execer.ExecContext(ctx, `
		insert into automation_simple_runs (
			automation_id, dedupe_key, owner, task_id, last_status, last_summary, payload, created_at, updated_at
		)
		values (?, ?, ?, ?, ?, ?, ?, ?, ?)
		on conflict(automation_id, dedupe_key) do nothing
	`,
		run.AutomationID,
		run.DedupeKey,
		run.Owner,
		run.TaskID,
		run.LastStatus,
		run.LastSummary,
		string(payload),
		run.CreatedAt.UTC().Format(timeLayout),
		run.UpdatedAt.UTC().Format(timeLayout),
	)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return rows > 0, nil
}

func upsertSimpleAutomationRunWithExec(ctx context.Context, execer execContexter, run types.SimpleAutomationRun) error {
	run = normalizeSimpleAutomationRunForStore(run)
	if run.AutomationID == "" || run.DedupeKey == "" {
		return nil
	}
	payload, err := json.Marshal(run)
	if err != nil {
		return err
	}

	_, err = execer.ExecContext(ctx, `
		insert into automation_simple_runs (
			automation_id, dedupe_key, owner, task_id, last_status, last_summary, payload, created_at, updated_at
		)
		values (?, ?, ?, ?, ?, ?, ?, ?, ?)
		on conflict(automation_id, dedupe_key) do update set
			owner = excluded.owner,
			task_id = excluded.task_id,
			last_status = excluded.last_status,
			last_summary = excluded.last_summary,
			payload = excluded.payload,
			updated_at = excluded.updated_at
	`,
		run.AutomationID,
		run.DedupeKey,
		run.Owner,
		run.TaskID,
		run.LastStatus,
		run.LastSummary,
		string(payload),
		run.CreatedAt.UTC().Format(timeLayout),
		run.UpdatedAt.UTC().Format(timeLayout),
	)
	return err
}

func (s *Store) GetSimpleAutomationRun(ctx context.Context, automationID, dedupeKey string) (types.SimpleAutomationRun, bool, error) {
	return getSimpleAutomationRunWithQueryer(ctx, s.db, automationID, dedupeKey)
}

func (t runtimeTx) GetSimpleAutomationRun(ctx context.Context, automationID, dedupeKey string) (types.SimpleAutomationRun, bool, error) {
	return getSimpleAutomationRunWithQueryer(ctx, t.tx, automationID, dedupeKey)
}

func (s *Store) GetSimpleAutomationRunByTaskID(ctx context.Context, taskID string) (types.SimpleAutomationRun, bool, error) {
	return getSimpleAutomationRunByTaskIDWithQueryer(ctx, s.db, taskID)
}

func (t runtimeTx) GetSimpleAutomationRunByTaskID(ctx context.Context, taskID string) (types.SimpleAutomationRun, bool, error) {
	return getSimpleAutomationRunByTaskIDWithQueryer(ctx, t.tx, taskID)
}

func getSimpleAutomationRunWithQueryer(ctx context.Context, queryer queryContexter, automationID, dedupeKey string) (types.SimpleAutomationRun, bool, error) {
	automationID = strings.TrimSpace(automationID)
	dedupeKey = strings.TrimSpace(dedupeKey)
	if automationID == "" || dedupeKey == "" {
		return types.SimpleAutomationRun{}, false, nil
	}

	rows, err := queryer.QueryContext(ctx, `
		select payload, created_at, updated_at
		from automation_simple_runs
		where automation_id = ? and dedupe_key = ?
	`, automationID, dedupeKey)
	if err != nil {
		return types.SimpleAutomationRun{}, false, err
	}
	defer rows.Close()

	items, err := scanSimpleAutomationRuns(rows)
	if err != nil {
		return types.SimpleAutomationRun{}, false, err
	}
	if len(items) == 0 {
		return types.SimpleAutomationRun{}, false, nil
	}
	return items[0], true, nil
}

func getSimpleAutomationRunByTaskIDWithQueryer(ctx context.Context, queryer queryContexter, taskID string) (types.SimpleAutomationRun, bool, error) {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return types.SimpleAutomationRun{}, false, nil
	}

	rows, err := queryer.QueryContext(ctx, `
		select payload, created_at, updated_at
		from automation_simple_runs
		where task_id = ?
		order by updated_at desc, created_at desc, automation_id asc, dedupe_key asc
		limit 1
	`, taskID)
	if err != nil {
		return types.SimpleAutomationRun{}, false, err
	}
	defer rows.Close()

	items, err := scanSimpleAutomationRuns(rows)
	if err != nil {
		return types.SimpleAutomationRun{}, false, err
	}
	if len(items) == 0 {
		return types.SimpleAutomationRun{}, false, nil
	}
	return items[0], true, nil
}

func scanSimpleAutomationRuns(rows *sql.Rows) ([]types.SimpleAutomationRun, error) {
	out := make([]types.SimpleAutomationRun, 0)
	for rows.Next() {
		var (
			payload   string
			createdAt string
			updatedAt string
		)
		if err := rows.Scan(&payload, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		var run types.SimpleAutomationRun
		if err := json.Unmarshal([]byte(payload), &run); err != nil {
			return nil, err
		}
		if parsed, err := parsePendingOptionalTime(createdAt); err == nil && !parsed.IsZero() {
			run.CreatedAt = parsed
		}
		if parsed, err := parsePendingOptionalTime(updatedAt); err == nil && !parsed.IsZero() {
			run.UpdatedAt = parsed
		}
		out = append(out, normalizeSimpleAutomationRunForStore(run))
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func normalizeSimpleAutomationRunForStore(run types.SimpleAutomationRun) types.SimpleAutomationRun {
	run.AutomationID = strings.TrimSpace(run.AutomationID)
	run.DedupeKey = strings.TrimSpace(run.DedupeKey)
	run.Owner = strings.TrimSpace(run.Owner)
	run.TaskID = strings.TrimSpace(run.TaskID)
	run.LastStatus = strings.TrimSpace(run.LastStatus)
	run.LastSummary = strings.TrimSpace(run.LastSummary)

	now := time.Now().UTC()
	if run.CreatedAt.IsZero() {
		run.CreatedAt = now
	} else {
		run.CreatedAt = run.CreatedAt.UTC()
	}
	if run.UpdatedAt.IsZero() {
		run.UpdatedAt = run.CreatedAt
	} else {
		run.UpdatedAt = run.UpdatedAt.UTC()
	}
	if run.UpdatedAt.Before(run.CreatedAt) {
		run.UpdatedAt = run.CreatedAt
	}
	return run
}
