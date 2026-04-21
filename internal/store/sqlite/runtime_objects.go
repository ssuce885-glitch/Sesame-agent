package sqlite

import (
	"context"
	"strings"

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

func (t runtimeTx) ListActivePlansForSession(ctx context.Context, sessionID string) ([]types.Plan, error) {
	return listActivePlansForSession(ctx, t.tx, sessionID)
}

func (s *Store) UpsertTaskRecord(ctx context.Context, task types.TaskRecord) error {
	return upsertTaskRecordWithExec(ctx, s.db, task)
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

func (s *Store) UpsertWorktree(ctx context.Context, worktree types.Worktree) error {
	return upsertWorktreeWithExec(ctx, s.db, worktree)
}

func (s *Store) UpsertPermissionRequest(ctx context.Context, request types.PermissionRequest) error {
	return upsertPermissionRequestWithExec(ctx, s.db, request)
}

func (s *Store) UpsertTurnContinuation(ctx context.Context, continuation types.TurnContinuation) error {
	return upsertTurnContinuationWithExec(ctx, s.db, continuation)
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
