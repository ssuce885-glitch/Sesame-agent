package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
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
	return upsertPermissionRequestWithExec(ctx, s.db, request)
}

func upsertPermissionRequestWithExec(ctx context.Context, execer execContexter, request types.PermissionRequest) error {
	request = normalizePermissionRequest(request)
	payload, err := marshalRuntimePayload(request)
	if err != nil {
		return err
	}

	_, err = execer.ExecContext(ctx, `
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

func (s *Store) ListPermissionRequestsByTask(ctx context.Context, taskID string) ([]types.PermissionRequest, error) {
	return listRuntimeObjects[types.PermissionRequest](ctx, s.db, `
		select payload, created_at, updated_at
		from permission_requests
		where task_id = ?
		order by created_at asc, id asc
	`, taskID)
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
	return s.listRuntimeGraph(ctx, "")
}

func (s *Store) ListRuntimeGraphForWorkspace(ctx context.Context, workspaceRoot string) (types.RuntimeGraph, error) {
	workspaceRoot = strings.TrimSpace(workspaceRoot)
	if workspaceRoot == "" {
		return types.RuntimeGraph{}, nil
	}
	return s.listRuntimeGraph(ctx, workspaceRoot)
}

func (s *Store) listRuntimeGraph(ctx context.Context, workspaceRoot string) (types.RuntimeGraph, error) {
	workspaceRoot = strings.TrimSpace(workspaceRoot)

	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return types.RuntimeGraph{}, err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	var runs []types.Run
	if workspaceRoot == "" {
		runs, err = listRuntimeObjects[types.Run](ctx, tx, `select payload, created_at, updated_at from runs order by created_at asc, id asc`)
	} else {
		workspaceCondition, workspaceArgs := runtimeGraphWorkspaceCondition("s.workspace_root", workspaceRoot)
		runs, err = listRuntimeObjects[types.Run](ctx, tx, `
			select r.payload, r.created_at, r.updated_at
			from runs r
			join sessions s on s.id = r.session_id
			where `+workspaceCondition+`
			order by r.created_at asc, r.id asc
		`, workspaceArgs...)
	}
	if err != nil {
		return types.RuntimeGraph{}, err
	}
	if hook := runtimeGraphReadHook; hook != nil {
		hook("after_runs")
	}

	var plans []types.Plan
	if workspaceRoot == "" {
		plans, err = listRuntimeObjects[types.Plan](ctx, tx, `select payload, created_at, updated_at from plans order by created_at asc, id asc`)
	} else {
		workspaceCondition, workspaceArgs := runtimeGraphWorkspaceCondition("s.workspace_root", workspaceRoot)
		plans, err = listRuntimeObjects[types.Plan](ctx, tx, `
			select p.payload, p.created_at, p.updated_at
			from plans p
			join runs r on r.id = p.run_id
			join sessions s on s.id = r.session_id
			where `+workspaceCondition+`
			order by p.created_at asc, p.id asc
		`, workspaceArgs...)
	}
	if err != nil {
		return types.RuntimeGraph{}, err
	}

	var tasks []types.Task
	if workspaceRoot == "" {
		tasks, err = listRuntimeObjects[types.Task](ctx, tx, `select payload, created_at, updated_at from task_records order by created_at asc, id asc`)
	} else {
		workspaceCondition, workspaceArgs := runtimeGraphWorkspaceCondition("s.workspace_root", workspaceRoot)
		tasks, err = listRuntimeObjects[types.Task](ctx, tx, `
			select tr.payload, tr.created_at, tr.updated_at
			from task_records tr
			join runs r on r.id = tr.run_id
			join sessions s on s.id = r.session_id
			where `+workspaceCondition+`
			order by tr.created_at asc, tr.id asc
		`, workspaceArgs...)
	}
	if err != nil {
		return types.RuntimeGraph{}, err
	}

	var toolRuns []types.ToolRun
	if workspaceRoot == "" {
		toolRuns, err = listRuntimeObjects[types.ToolRun](ctx, tx, `select payload, created_at, updated_at from tool_runs order by created_at asc, id asc`)
	} else {
		workspaceCondition, workspaceArgs := runtimeGraphWorkspaceCondition("s.workspace_root", workspaceRoot)
		toolRuns, err = listRuntimeObjects[types.ToolRun](ctx, tx, `
			select tr.payload, tr.created_at, tr.updated_at
			from tool_runs tr
			join runs r on r.id = tr.run_id
			join sessions s on s.id = r.session_id
			where `+workspaceCondition+`
			order by tr.created_at asc, tr.id asc
		`, workspaceArgs...)
	}
	if err != nil {
		return types.RuntimeGraph{}, err
	}

	var worktrees []types.Worktree
	if workspaceRoot == "" {
		worktrees, err = listRuntimeObjects[types.Worktree](ctx, tx, `select payload, created_at, updated_at from worktrees order by created_at asc, id asc`)
	} else {
		workspaceCondition, workspaceArgs := runtimeGraphWorkspaceCondition("s.workspace_root", workspaceRoot)
		worktrees, err = listRuntimeObjects[types.Worktree](ctx, tx, `
			select w.payload, w.created_at, w.updated_at
			from worktrees w
			join runs r on r.id = w.run_id
			join sessions s on s.id = r.session_id
			where `+workspaceCondition+`
			order by w.created_at asc, w.id asc
		`, workspaceArgs...)
	}
	if err != nil {
		return types.RuntimeGraph{}, err
	}

	var incidents []types.AutomationIncident
	var dispatchAttempts []types.DispatchAttempt
	if workspaceRoot == "" {
		incidents, err = listAutomationIncidentsWithQueryer(ctx, tx, types.AutomationIncidentFilter{})
		if err != nil {
			return types.RuntimeGraph{}, err
		}
		dispatchAttempts, err = listDispatchAttemptsWithQueryer(ctx, tx, types.DispatchAttemptFilter{})
		if err != nil {
			return types.RuntimeGraph{}, err
		}
	} else {
		incidents, err = listAutomationIncidentsWithQueryer(ctx, tx, types.AutomationIncidentFilter{
			WorkspaceRoot: workspaceRoot,
		})
		if err != nil {
			return types.RuntimeGraph{}, err
		}
		dispatchAttempts, err = listDispatchAttemptsWithQueryer(ctx, tx, types.DispatchAttemptFilter{
			WorkspaceRoot: workspaceRoot,
		})
		if err != nil {
			return types.RuntimeGraph{}, err
		}
	}

	var permissionRequests []types.PermissionRequest
	if workspaceRoot == "" {
		permissionRequests, err = listRuntimeObjects[types.PermissionRequest](ctx, tx, `select payload, created_at, updated_at from permission_requests order by created_at asc, id asc`)
	} else {
		workspaceCondition, workspaceArgs := runtimeGraphWorkspaceCondition("s.workspace_root", workspaceRoot)
		permissionRequests, err = listRuntimeObjects[types.PermissionRequest](ctx, tx, `
			select pr.payload, pr.created_at, pr.updated_at
			from permission_requests pr
			join sessions s on s.id = pr.session_id
			where `+workspaceCondition+`
			order by pr.created_at asc, pr.id asc
		`, workspaceArgs...)
	}
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
		Incidents:          incidents,
		DispatchAttempts:   dispatchAttempts,
		PermissionRequests: permissionRequests,
	}, nil
}

func runtimeGraphWorkspaceCondition(column, workspaceRoot string) (string, []any) {
	conditions := make([]string, 0, 1)
	args := make([]any, 0, 2)
	appendAutomationWorkspaceRootCondition(&conditions, &args, column, workspaceRoot)
	if len(conditions) == 0 {
		return "1 = 0", args
	}
	return strings.Join(conditions, " and "), args
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
