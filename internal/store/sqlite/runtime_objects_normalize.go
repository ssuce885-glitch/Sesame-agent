package sqlite

import "go-agent/internal/types"

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
