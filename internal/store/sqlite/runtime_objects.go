package sqlite

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"go-agent/internal/types"
)

var runtimeGraphReadHook func(string)

type RuntimeTx interface {
	InsertRun(context.Context, types.Run) error
	UpsertRun(context.Context, types.Run) error
	UpsertPlan(context.Context, types.Plan) error
	UpsertTaskRecord(context.Context, types.TaskRecord) error
	UpsertWorktree(context.Context, types.Worktree) error
	UpsertScheduledJob(context.Context, types.ScheduledJob) error
	GetScheduledJob(context.Context, string) (types.ScheduledJob, bool, error)
	ListScheduledJobs(context.Context) ([]types.ScheduledJob, error)
	ListScheduledJobsByWorkspace(context.Context, string) ([]types.ScheduledJob, error)
	ListDueScheduledJobs(context.Context, time.Time) ([]types.ScheduledJob, error)
	DeleteScheduledJob(context.Context, string) (bool, error)
	UpsertChildAgentSpec(context.Context, types.ChildAgentSpec) error
	DeleteChildAgentSpec(context.Context, string) (bool, error)
	UpsertOutputContract(context.Context, types.OutputContract) error
	GetReportGroup(context.Context, string) (types.ReportGroup, bool, error)
	UpsertReportGroup(context.Context, types.ReportGroup) error
	UpsertChildAgentResult(context.Context, types.ChildAgentResult) error
	UpsertDigestRecord(context.Context, types.DigestRecord) error
	ListActivePlansForSession(context.Context, string) ([]types.Plan, error)
}

type runtimeTx struct {
	tx *sql.Tx
}

func (s *Store) WithTx(ctx context.Context, fn func(tx RuntimeTx) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	adapter := runtimeTx{tx: tx}
	if err := fn(adapter); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) FinalizeTurn(ctx context.Context, usage *types.TurnUsage, events []types.Event) ([]types.Event, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	if usage != nil {
		if err := upsertTurnUsageWithExec(ctx, tx, *usage); err != nil {
			return nil, err
		}
	}

	persisted := make([]types.Event, 0, len(events))
	for _, event := range events {
		seq, err := appendEventWithExec(ctx, tx, event)
		if err != nil {
			return nil, err
		}
		event.Seq = seq
		if err := applyEventStateTransition(ctx, tx, event); err != nil {
			return nil, err
		}
		persisted = append(persisted, event)
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return persisted, nil
}

func (s *Store) InsertRun(ctx context.Context, run types.Run) error {
	return insertRunWithExec(ctx, s.db, run)
}

func (s *Store) UpsertRun(ctx context.Context, run types.Run) error {
	return upsertRunWithExec(ctx, s.db, run)
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

func (t runtimeTx) UpsertRun(ctx context.Context, run types.Run) error {
	return upsertRunWithExec(ctx, t.tx, run)
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
