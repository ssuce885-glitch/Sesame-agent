package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"go-agent/internal/types"
)

func (t runtimeTx) UpsertTaskRecord(ctx context.Context, task types.TaskRecord) error {
	return upsertTaskRecordWithExec(ctx, t.tx, task)
}

func (t runtimeTx) UpsertWorktree(ctx context.Context, worktree types.Worktree) error {
	return upsertWorktreeWithExec(ctx, t.tx, worktree)
}

func insertRunWithExec(ctx context.Context, execer interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}, run types.Run) error {
	run = normalizeRun(run)
	payload, err := marshalRuntimePayload(run)
	if err != nil {
		return err
	}

	_, err = execer.ExecContext(ctx, `
		insert into runs (id, session_id, turn_id, state, payload, created_at, updated_at)
		values (?, ?, ?, ?, ?, ?, ?)
	`,
		run.ID,
		run.SessionID,
		run.TurnID,
		run.State,
		payload,
		run.CreatedAt.UTC().Format(timeLayout),
		run.UpdatedAt.UTC().Format(timeLayout),
	)
	return err
}

func upsertRunWithExec(ctx context.Context, execer interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}, run types.Run) error {
	run = normalizeRun(run)
	payload, err := marshalRuntimePayload(run)
	if err != nil {
		return err
	}

	_, err = execer.ExecContext(ctx, `
		insert into runs (id, session_id, turn_id, state, payload, created_at, updated_at)
		values (?, ?, ?, ?, ?, ?, ?)
		on conflict(id) do update set
			session_id = excluded.session_id,
			turn_id = excluded.turn_id,
			state = excluded.state,
			payload = excluded.payload,
			updated_at = excluded.updated_at
	`,
		run.ID,
		run.SessionID,
		run.TurnID,
		run.State,
		payload,
		run.CreatedAt.UTC().Format(timeLayout),
		run.UpdatedAt.UTC().Format(timeLayout),
	)
	return err
}

func upsertPlanWithExec(ctx context.Context, execer interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}, plan types.Plan) error {
	plan = normalizePlan(plan)
	payload, err := marshalRuntimePayload(plan)
	if err != nil {
		return err
	}

	_, err = execer.ExecContext(ctx, `
		insert into plans (id, run_id, state, payload, created_at, updated_at)
		values (?, ?, ?, ?, ?, ?)
		on conflict(id) do update set
			run_id = excluded.run_id,
			state = excluded.state,
			payload = excluded.payload,
			updated_at = excluded.updated_at
	`,
		plan.ID,
		plan.RunID,
		plan.State,
		payload,
		plan.CreatedAt.UTC().Format(timeLayout),
		plan.UpdatedAt.UTC().Format(timeLayout),
	)
	return err
}

func upsertTaskRecordWithExec(ctx context.Context, execer execContexter, task types.TaskRecord) error {
	task = normalizeTask(task)
	payload, err := marshalRuntimePayload(task)
	if err != nil {
		return err
	}

	_, err = execer.ExecContext(ctx, `
		insert into task_records (id, run_id, plan_id, state, payload, created_at, updated_at)
		values (?, ?, ?, ?, ?, ?, ?)
		on conflict(id) do update set
			run_id = excluded.run_id,
			plan_id = excluded.plan_id,
			state = excluded.state,
			payload = excluded.payload,
			updated_at = excluded.updated_at
	`,
		task.ID,
		task.RunID,
		task.PlanID,
		task.State,
		payload,
		task.CreatedAt.UTC().Format(timeLayout),
		task.UpdatedAt.UTC().Format(timeLayout),
	)
	return err
}

func upsertToolRunWithExec(ctx context.Context, execer execContexter, toolRun types.ToolRun) error {
	toolRun = normalizeToolRun(toolRun)
	payload, err := marshalRuntimePayload(toolRun)
	if err != nil {
		return err
	}

	_, err = execer.ExecContext(ctx, `
		insert into tool_runs (id, run_id, task_id, state, payload, created_at, updated_at)
		values (?, ?, ?, ?, ?, ?, ?)
		on conflict(id) do update set
			run_id = excluded.run_id,
			task_id = excluded.task_id,
			state = excluded.state,
			payload = excluded.payload,
			updated_at = excluded.updated_at
	`,
		toolRun.ID,
		toolRun.RunID,
		toolRun.TaskID,
		toolRun.State,
		payload,
		toolRun.CreatedAt.UTC().Format(timeLayout),
		toolRun.UpdatedAt.UTC().Format(timeLayout),
	)
	return err
}

func upsertWorktreeWithExec(ctx context.Context, execer execContexter, worktree types.Worktree) error {
	worktree = normalizeWorktree(worktree)
	payload, err := marshalRuntimePayload(worktree)
	if err != nil {
		return err
	}

	_, err = execer.ExecContext(ctx, `
		insert into worktrees (id, run_id, task_id, state, payload, created_at, updated_at)
		values (?, ?, ?, ?, ?, ?, ?)
		on conflict(id) do update set
			run_id = excluded.run_id,
			task_id = excluded.task_id,
			state = excluded.state,
			payload = excluded.payload,
			updated_at = excluded.updated_at
	`,
		worktree.ID,
		worktree.RunID,
		worktree.TaskID,
		worktree.State,
		payload,
		worktree.CreatedAt.UTC().Format(timeLayout),
		worktree.UpdatedAt.UTC().Format(timeLayout),
	)
	return err
}

type runtimeObjectQueryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func listRuntimeObjects[T any](ctx context.Context, queryer runtimeObjectQueryer, query string, args ...any) ([]T, error) {
	rows, err := queryer.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]T, 0)
	for rows.Next() {
		var payload string
		var createdAt string
		var updatedAt string
		if err := rows.Scan(&payload, &createdAt, &updatedAt); err != nil {
			return nil, err
		}

		var item T
		if err := json.Unmarshal([]byte(payload), &item); err != nil {
			return nil, err
		}
		if err := applyRuntimeTimestamps(&item, createdAt, updatedAt); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return out, nil
}

func listActivePlansForSession(ctx context.Context, queryer runtimeObjectQueryer, sessionID string) ([]types.Plan, error) {
	return listRuntimeObjects[types.Plan](ctx, queryer, `
		select p.payload, p.created_at, p.updated_at
		from plans p
		join runs r on p.run_id = r.id
		where r.session_id = ? and p.state = ?
		order by p.created_at desc, p.id desc
	`, sessionID, types.PlanStateActive)
}

func marshalRuntimePayload(v any) (string, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func applyRuntimeTimestamps[T any](item *T, createdAt, updatedAt string) error {
	created, err := time.Parse(timeLayout, createdAt)
	if err != nil {
		return err
	}
	updated, err := time.Parse(timeLayout, updatedAt)
	if err != nil {
		return err
	}

	switch v := any(item).(type) {
	case *types.Run:
		v.CreatedAt = created.UTC()
		v.UpdatedAt = updated.UTC()
	case *types.Plan:
		v.CreatedAt = created.UTC()
		v.UpdatedAt = updated.UTC()
	case *types.Task:
		v.CreatedAt = created.UTC()
		v.UpdatedAt = updated.UTC()
	case *types.ToolRun:
		v.CreatedAt = created.UTC()
		v.UpdatedAt = updated.UTC()
	case *types.Worktree:
		v.CreatedAt = created.UTC()
		v.UpdatedAt = updated.UTC()
	}

	return nil
}
