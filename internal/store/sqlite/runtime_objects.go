package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"go-agent/internal/types"
)

var runtimeGraphReadHook func(string)

func (s *Store) InsertRun(ctx context.Context, run types.Run) error {
	return insertRunWithExec(ctx, s.db, run)
}

func (s *Store) UpsertPlan(ctx context.Context, plan types.Plan) error {
	return upsertPlanWithExec(ctx, s.db, plan)
}

func (s *Store) ListActivePlansForSession(ctx context.Context, sessionID string) ([]types.Plan, error) {
	return listActivePlansForSession(ctx, s.db, sessionID)
}

func (t runtimeTx) InsertRun(ctx context.Context, run types.Run) error {
	return insertRunWithExec(ctx, t.tx, run)
}

func (t runtimeTx) UpsertPlan(ctx context.Context, plan types.Plan) error {
	return upsertPlanWithExec(ctx, t.tx, plan)
}

func (t runtimeTx) UpsertTaskRecord(ctx context.Context, task types.TaskRecord) error {
	task = normalizeTask(task)
	payload, err := marshalRuntimePayload(task)
	if err != nil {
		return err
	}
	_, err = t.tx.ExecContext(ctx, `
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

func (t runtimeTx) UpsertWorktree(ctx context.Context, worktree types.Worktree) error {
	worktree = normalizeWorktree(worktree)
	payload, err := marshalRuntimePayload(worktree)
	if err != nil {
		return err
	}
	_, err = t.tx.ExecContext(ctx, `
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

func (t runtimeTx) ListActivePlansForSession(ctx context.Context, sessionID string) ([]types.Plan, error) {
	return listActivePlansForSession(ctx, t.tx, sessionID)
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

func (s *Store) GetTaskRecord(ctx context.Context, id string) (types.TaskRecord, bool, error) {
	items, err := listRuntimeObjects[types.TaskRecord](ctx, s.db, `
		select payload, created_at, updated_at
		from task_records
		where id = ?
	`, id)
	if err != nil {
		return types.TaskRecord{}, false, err
	}
	if len(items) == 0 {
		return types.TaskRecord{}, false, nil
	}
	return items[0], true, nil
}

func (s *Store) UpsertToolRun(ctx context.Context, toolRun types.ToolRun) error {
	return upsertToolRunWithExec(ctx, s.db, toolRun)
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

func (s *Store) UpsertPermissionRequest(ctx context.Context, request types.PermissionRequest) error {
	request = normalizePermissionRequest(request)
	payload, err := marshalRuntimePayload(request)
	if err != nil {
		return err
	}

	_, err = s.db.ExecContext(ctx, `
		insert into permission_requests (id, session_id, turn_id, run_id, task_id, tool_run_id, status, payload, created_at, updated_at)
		values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		on conflict(id) do update set
			session_id = excluded.session_id,
			turn_id = excluded.turn_id,
			run_id = excluded.run_id,
			task_id = excluded.task_id,
			tool_run_id = excluded.tool_run_id,
			status = excluded.status,
			payload = excluded.payload,
			updated_at = excluded.updated_at
	`,
		request.ID,
		request.SessionID,
		request.TurnID,
		request.RunID,
		request.TaskID,
		request.ToolRunID,
		request.Status,
		payload,
		request.CreatedAt.UTC().Format(timeLayout),
		request.UpdatedAt.UTC().Format(timeLayout),
	)
	return err
}

func (s *Store) UpsertTurnContinuation(ctx context.Context, continuation types.TurnContinuation) error {
	return upsertTurnContinuationWithExec(ctx, s.db, continuation)
}

func upsertTurnContinuationWithExec(ctx context.Context, execer execContexter, continuation types.TurnContinuation) error {
	continuation = normalizeTurnContinuation(continuation)
	payload, err := marshalRuntimePayload(continuation)
	if err != nil {
		return err
	}

	_, err = execer.ExecContext(ctx, `
		insert into turn_continuations (id, session_id, turn_id, permission_request_id, state, payload, created_at, updated_at)
		values (?, ?, ?, ?, ?, ?, ?, ?)
		on conflict(id) do update set
			session_id = excluded.session_id,
			turn_id = excluded.turn_id,
			permission_request_id = excluded.permission_request_id,
			state = excluded.state,
			payload = excluded.payload,
			updated_at = excluded.updated_at
	`,
		continuation.ID,
		continuation.SessionID,
		continuation.TurnID,
		continuation.PermissionRequestID,
		continuation.State,
		payload,
		continuation.CreatedAt.UTC().Format(timeLayout),
		continuation.UpdatedAt.UTC().Format(timeLayout),
	)
	return err
}

func (s *Store) GetPermissionRequest(ctx context.Context, id string) (types.PermissionRequest, bool, error) {
	items, err := listRuntimeObjects[types.PermissionRequest](ctx, s.db, `
		select payload, created_at, updated_at
		from permission_requests
		where id = ?
	`, id)
	if err != nil {
		return types.PermissionRequest{}, false, err
	}
	if len(items) == 0 {
		return types.PermissionRequest{}, false, nil
	}
	return items[0], true, nil
}

func (s *Store) ListPermissionRequestsBySession(ctx context.Context, sessionID string) ([]types.PermissionRequest, error) {
	return listRuntimeObjects[types.PermissionRequest](ctx, s.db, `
		select payload, created_at, updated_at
		from permission_requests
		where session_id = ?
		order by created_at asc, id asc
	`, sessionID)
}

func (s *Store) GetTurnContinuationByPermissionRequest(ctx context.Context, requestID string) (types.TurnContinuation, bool, error) {
	items, err := listRuntimeObjects[types.TurnContinuation](ctx, s.db, `
		select payload, created_at, updated_at
		from turn_continuations
		where permission_request_id = ?
		order by created_at desc, id desc
		limit 1
	`, requestID)
	if err != nil {
		return types.TurnContinuation{}, false, err
	}
	if len(items) == 0 {
		return types.TurnContinuation{}, false, nil
	}
	return items[0], true, nil
}

func (s *Store) ListPendingTurnContinuations(ctx context.Context) ([]types.TurnContinuation, error) {
	return listRuntimeObjects[types.TurnContinuation](ctx, s.db, `
		select payload, created_at, updated_at
		from turn_continuations
		where state = ?
		order by created_at asc, id asc
	`, string(types.TurnContinuationStatePending))
}

func (s *Store) GetToolRun(ctx context.Context, id string) (types.ToolRun, bool, error) {
	items, err := listRuntimeObjects[types.ToolRun](ctx, s.db, `
		select payload, created_at, updated_at
		from tool_runs
		where id = ?
	`, id)
	if err != nil {
		return types.ToolRun{}, false, err
	}
	if len(items) == 0 {
		return types.ToolRun{}, false, nil
	}
	return items[0], true, nil
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
	if hook := runtimeGraphReadHook; hook != nil {
		hook("after_runs")
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

	permissionRequests, err := listRuntimeObjects[types.PermissionRequest](ctx, tx, `select payload, created_at, updated_at from permission_requests order by created_at asc, id asc`)
	if err != nil {
		return types.RuntimeGraph{}, err
	}

	if err := tx.Commit(); err != nil {
		return types.RuntimeGraph{}, err
	}

	return types.RuntimeGraph{
		Runs:               runs,
		Plans:              plans,
		Tasks:              tasks,
		ToolRuns:           toolRuns,
		Worktrees:          worktrees,
		PermissionRequests: permissionRequests,
	}, nil
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
	case *types.PermissionRequest:
		v.CreatedAt = created.UTC()
		v.UpdatedAt = updated.UTC()
	case *types.TurnContinuation:
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

func normalizePermissionRequest(request types.PermissionRequest) types.PermissionRequest {
	request.CreatedAt = request.CreatedAt.UTC()
	request.UpdatedAt = request.UpdatedAt.UTC()
	request.ResolvedAt = request.ResolvedAt.UTC()
	return request
}

func normalizeTurnContinuation(continuation types.TurnContinuation) types.TurnContinuation {
	continuation.CreatedAt = continuation.CreatedAt.UTC()
	continuation.UpdatedAt = continuation.UpdatedAt.UTC()
	return continuation
}
