package sqlite

import (
	"context"
	"database/sql"
	"strings"

	"go-agent/internal/types"
)

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
