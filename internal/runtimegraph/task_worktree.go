package runtimegraph

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go-agent/internal/types"
)

type CreateTaskInput struct {
	SessionID        string
	TurnID           string
	PlanID           string
	ParentTaskID     string
	Title            string
	Description      string
	Owner            string
	Kind             string
	ExecutionTaskID  string
	WorktreeID       string
}

type UpsertWorktreeInput struct {
	SessionID      string
	TurnID         string
	TaskID         string
	WorktreeID     string
	State          types.WorktreeState
	WorktreePath   string
	WorktreeBranch string
}

func (s *Service) CreateTask(ctx context.Context, turnCtx *TurnContext, in CreateTaskInput) (types.Task, error) {
	if s == nil || s.store == nil {
		return types.Task{}, fmt.Errorf("runtime service is not configured")
	}
	if turnCtx == nil {
		return types.Task{}, fmt.Errorf("turn runtime context is not configured")
	}
	if strings.TrimSpace(in.SessionID) == "" {
		return types.Task{}, fmt.Errorf("session id is required")
	}

	now := time.Now().UTC()
	taskID := strings.TrimSpace(in.ExecutionTaskID)
	if taskID == "" {
		taskID = types.NewID("task")
	}
	task := types.Task{
		ID:              taskID,
		PlanID:          strings.TrimSpace(in.PlanID),
		ParentTaskID:    strings.TrimSpace(in.ParentTaskID),
		State:           types.TaskStatePending,
		Title:           strings.TrimSpace(in.Title),
		Description:     strings.TrimSpace(in.Description),
		Owner:           strings.TrimSpace(in.Owner),
		Kind:            strings.TrimSpace(in.Kind),
		ExecutionTaskID: taskID,
		WorktreeID:      strings.TrimSpace(in.WorktreeID),
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	err := s.store.WithTx(ctx, func(tx RuntimeTx) error {
		runID, err := ensureRunLocked(ctx, tx, turnCtx, in.SessionID, in.TurnID, now)
		if err != nil {
			return err
		}
		task.RunID = runID
		return tx.UpsertTaskRecord(ctx, task)
	})
	if err != nil {
		return types.Task{}, err
	}

	turnCtx.CurrentTaskID = task.ID
	return task, nil
}

func (s *Service) UpdateTask(ctx context.Context, task types.Task) error {
	if s == nil || s.store == nil {
		return fmt.Errorf("runtime service is not configured")
	}
	task.UpdatedAt = time.Now().UTC()
	if task.CreatedAt.IsZero() {
		task.CreatedAt = task.UpdatedAt
	}
	return s.store.WithTx(ctx, func(tx RuntimeTx) error {
		return tx.UpsertTaskRecord(ctx, task)
	})
}

func (s *Service) UpsertWorktree(ctx context.Context, turnCtx *TurnContext, in UpsertWorktreeInput) (types.Worktree, error) {
	if s == nil || s.store == nil {
		return types.Worktree{}, fmt.Errorf("runtime service is not configured")
	}
	if turnCtx == nil {
		return types.Worktree{}, fmt.Errorf("turn runtime context is not configured")
	}
	if strings.TrimSpace(in.SessionID) == "" {
		return types.Worktree{}, fmt.Errorf("session id is required")
	}
	now := time.Now().UTC()
	worktree := types.Worktree{
		ID:             firstNonEmpty(strings.TrimSpace(in.WorktreeID), types.NewID("worktree")),
		TaskID:         strings.TrimSpace(in.TaskID),
		State:          in.State,
		WorktreePath:   strings.TrimSpace(in.WorktreePath),
		WorktreeBranch: strings.TrimSpace(in.WorktreeBranch),
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if worktree.State == "" {
		worktree.State = types.WorktreeStateActive
	}
	err := s.store.WithTx(ctx, func(tx RuntimeTx) error {
		runID, err := ensureRunLocked(ctx, tx, turnCtx, in.SessionID, in.TurnID, now)
		if err != nil {
			return err
		}
		worktree.RunID = runID
		return tx.UpsertWorktree(ctx, worktree)
	})
	if err != nil {
		return types.Worktree{}, err
	}
	return worktree, nil
}

func ensureRunLocked(ctx context.Context, tx RuntimeTx, turnCtx *TurnContext, sessionID, turnID string, now time.Time) (string, error) {
	if strings.TrimSpace(turnCtx.CurrentRunID) != "" {
		return turnCtx.CurrentRunID, nil
	}
	runID := types.NewID("run")
	if err := tx.InsertRun(ctx, types.Run{
		ID:        runID,
		SessionID: strings.TrimSpace(sessionID),
		TurnID:    strings.TrimSpace(turnID),
		State:     types.RunStateRunning,
		Objective: "Runtime orchestration",
		CreatedAt: now,
		UpdatedAt: now,
	}); err != nil {
		return "", err
	}
	turnCtx.CurrentRunID = runID
	return runID, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
