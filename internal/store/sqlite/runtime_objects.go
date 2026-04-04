package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"go-agent/internal/types"
)

func (s *Store) InsertRun(ctx context.Context, run types.Run) error {
	run = normalizeRun(run)
	payload, err := marshalRuntimePayload(run)
	if err != nil {
		return err
	}

	_, err = s.db.ExecContext(ctx, `
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

func (s *Store) UpsertPlan(ctx context.Context, plan types.Plan) error {
	plan = normalizePlan(plan)
	payload, err := marshalRuntimePayload(plan)
	if err != nil {
		return err
	}

	_, err = s.db.ExecContext(ctx, `
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

func (s *Store) UpsertTaskRecord(ctx context.Context, task types.TaskRecord) error {
	task = normalizeTask(task)
	payload, err := marshalRuntimePayload(task)
	if err != nil {
		return err
	}

	_, err = s.db.ExecContext(ctx, `
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

func (s *Store) UpsertToolRun(ctx context.Context, toolRun types.ToolRun) error {
	toolRun = normalizeToolRun(toolRun)
	payload, err := marshalRuntimePayload(toolRun)
	if err != nil {
		return err
	}

	_, err = s.db.ExecContext(ctx, `
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

func (s *Store) UpsertWorktree(ctx context.Context, worktree types.Worktree) error {
	worktree = normalizeWorktree(worktree)
	payload, err := marshalRuntimePayload(worktree)
	if err != nil {
		return err
	}

	_, err = s.db.ExecContext(ctx, `
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

func (s *Store) ListRuntimeGraph(ctx context.Context) (types.RuntimeGraph, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return types.RuntimeGraph{}, err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	runs, err := listRuntimeObjects[types.Run](ctx, tx, `select payload, created_at, updated_at from runs order by created_at asc, id asc`)
	if err != nil {
		return types.RuntimeGraph{}, err
	}

	plans, err := listRuntimeObjects[types.Plan](ctx, tx, `select payload, created_at, updated_at from plans order by created_at asc, id asc`)
	if err != nil {
		return types.RuntimeGraph{}, err
	}

	tasks, err := listRuntimeObjects[types.Task](ctx, tx, `select payload, created_at, updated_at from task_records order by created_at asc, id asc`)
	if err != nil {
		return types.RuntimeGraph{}, err
	}

	toolRuns, err := listRuntimeObjects[types.ToolRun](ctx, tx, `select payload, created_at, updated_at from tool_runs order by created_at asc, id asc`)
	if err != nil {
		return types.RuntimeGraph{}, err
	}

	worktrees, err := listRuntimeObjects[types.Worktree](ctx, tx, `select payload, created_at, updated_at from worktrees order by created_at asc, id asc`)
	if err != nil {
		return types.RuntimeGraph{}, err
	}

	if err := tx.Commit(); err != nil {
		return types.RuntimeGraph{}, err
	}

	return types.RuntimeGraph{
		Runs:      runs,
		Plans:     plans,
		Tasks:     tasks,
		ToolRuns:  toolRuns,
		Worktrees: worktrees,
	}, nil
}

type runtimeObjectQueryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func listRuntimeObjects[T any](ctx context.Context, queryer runtimeObjectQueryer, query string) ([]T, error) {
	rows, err := queryer.QueryContext(ctx, query)
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

func normalizeRun(run types.Run) types.Run {
	run.CreatedAt = run.CreatedAt.UTC()
	run.UpdatedAt = run.UpdatedAt.UTC()
	return run
}

func normalizePlan(plan types.Plan) types.Plan {
	plan.CreatedAt = plan.CreatedAt.UTC()
	plan.UpdatedAt = plan.UpdatedAt.UTC()
	return plan
}

func normalizeTask(task types.TaskRecord) types.TaskRecord {
	task.CreatedAt = task.CreatedAt.UTC()
	task.UpdatedAt = task.UpdatedAt.UTC()
	return task
}

func normalizeToolRun(toolRun types.ToolRun) types.ToolRun {
	toolRun.CreatedAt = toolRun.CreatedAt.UTC()
	toolRun.UpdatedAt = toolRun.UpdatedAt.UTC()
	toolRun.StartedAt = toolRun.StartedAt.UTC()
	toolRun.CompletedAt = toolRun.CompletedAt.UTC()
	return toolRun
}

func normalizeWorktree(worktree types.Worktree) types.Worktree {
	worktree.CreatedAt = worktree.CreatedAt.UTC()
	worktree.UpdatedAt = worktree.UpdatedAt.UTC()
	return worktree
}
