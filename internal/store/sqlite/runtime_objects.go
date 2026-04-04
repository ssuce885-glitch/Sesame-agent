package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"

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
		on conflict(id) do update set
			session_id = excluded.session_id,
			turn_id = excluded.turn_id,
			state = excluded.state,
			payload = excluded.payload,
			created_at = excluded.created_at,
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
			created_at = excluded.created_at,
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
			created_at = excluded.created_at,
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
			created_at = excluded.created_at,
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
			created_at = excluded.created_at,
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
	runs, err := listRuntimeObjects[types.Run](ctx, s.db, `select payload from runs order by created_at asc, id asc`)
	if err != nil {
		return types.RuntimeGraph{}, err
	}

	plans, err := listRuntimeObjects[types.Plan](ctx, s.db, `select payload from plans order by created_at asc, id asc`)
	if err != nil {
		return types.RuntimeGraph{}, err
	}

	tasks, err := listRuntimeObjects[types.Task](ctx, s.db, `select payload from task_records order by created_at asc, id asc`)
	if err != nil {
		return types.RuntimeGraph{}, err
	}

	toolRuns, err := listRuntimeObjects[types.ToolRun](ctx, s.db, `select payload from tool_runs order by created_at asc, id asc`)
	if err != nil {
		return types.RuntimeGraph{}, err
	}

	worktrees, err := listRuntimeObjects[types.Worktree](ctx, s.db, `select payload from worktrees order by created_at asc, id asc`)
	if err != nil {
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

func listRuntimeObjects[T any](ctx context.Context, db *sql.DB, query string) ([]T, error) {
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]T, 0)
	for rows.Next() {
		var payload string
		if err := rows.Scan(&payload); err != nil {
			return nil, err
		}

		var item T
		if err := json.Unmarshal([]byte(payload), &item); err != nil {
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
