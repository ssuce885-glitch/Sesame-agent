package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"go-agent/internal/task"
	"go-agent/internal/types"
)

type persistedWorkspaceTask struct {
	ID                   string                  `json:"id"`
	Type                 task.TaskType           `json:"type"`
	Status               task.TaskStatus         `json:"status"`
	Command              string                  `json:"command"`
	Description          string                  `json:"description,omitempty"`
	ParentTaskID         string                  `json:"parent_task_id,omitempty"`
	Owner                string                  `json:"owner,omitempty"`
	Kind                 string                  `json:"kind,omitempty"`
	ExecutionTaskID      string                  `json:"execution_task_id,omitempty"`
	WorktreeID           string                  `json:"worktree_id,omitempty"`
	ScheduledJobID       string                  `json:"scheduled_job_id,omitempty"`
	ActivatedSkillNames  []string                `json:"activated_skill_names,omitempty"`
	WorkspaceRoot        string                  `json:"workspace_root"`
	Output               string                  `json:"output,omitempty"`
	OutputPath           string                  `json:"output_path,omitempty"`
	Error                string                  `json:"error,omitempty"`
	TimeoutSeconds       int                     `json:"timeout_seconds,omitempty"`
	StartTime            time.Time               `json:"start_time"`
	EndTime              *time.Time              `json:"end_time,omitempty"`
	ParentSessionID      string                  `json:"parent_session_id,omitempty"`
	ParentTurnID         string                  `json:"parent_turn_id,omitempty"`
	Outcome              types.ChildAgentOutcome `json:"outcome,omitempty"`
	OutcomeSummary       string                  `json:"outcome_summary,omitempty"`
	FinalResultKind      task.FinalResultKind    `json:"final_result_kind,omitempty"`
	FinalResultText      string                  `json:"final_result_text,omitempty"`
	FinalResultReadyAt   *time.Time              `json:"final_result_ready_at,omitempty"`
	CompletionNotifiedAt *time.Time              `json:"completion_notified_at,omitempty"`
}

func toPersistedWorkspaceTask(item task.Task) persistedWorkspaceTask {
	return persistedWorkspaceTask{
		ID:                   item.ID,
		Type:                 item.Type,
		Status:               item.Status,
		Command:              item.Command,
		Description:          item.Description,
		ParentTaskID:         item.ParentTaskID,
		Owner:                item.Owner,
		Kind:                 item.Kind,
		ExecutionTaskID:      item.ExecutionTaskID,
		WorktreeID:           item.WorktreeID,
		ScheduledJobID:       item.ScheduledJobID,
		ActivatedSkillNames:  append([]string(nil), item.ActivatedSkillNames...),
		WorkspaceRoot:        item.WorkspaceRoot,
		Output:               item.Output,
		OutputPath:           item.OutputPath,
		Error:                item.Error,
		TimeoutSeconds:       item.TimeoutSeconds,
		StartTime:            item.StartTime,
		EndTime:              item.EndTime,
		ParentSessionID:      item.ParentSessionID,
		ParentTurnID:         item.ParentTurnID,
		Outcome:              item.Outcome,
		OutcomeSummary:       item.OutcomeSummary,
		FinalResultKind:      item.FinalResultKind,
		FinalResultText:      item.FinalResultText,
		FinalResultReadyAt:   item.FinalResultReadyAt,
		CompletionNotifiedAt: item.CompletionNotifiedAt,
	}
}

func (item persistedWorkspaceTask) toTask() task.Task {
	return task.Task{
		ID:                   item.ID,
		Type:                 item.Type,
		Status:               item.Status,
		Command:              item.Command,
		Description:          item.Description,
		ParentTaskID:         item.ParentTaskID,
		Owner:                item.Owner,
		Kind:                 item.Kind,
		ExecutionTaskID:      item.ExecutionTaskID,
		WorktreeID:           item.WorktreeID,
		ScheduledJobID:       item.ScheduledJobID,
		ActivatedSkillNames:  append([]string(nil), item.ActivatedSkillNames...),
		WorkspaceRoot:        item.WorkspaceRoot,
		Output:               item.Output,
		OutputPath:           item.OutputPath,
		Error:                item.Error,
		TimeoutSeconds:       item.TimeoutSeconds,
		StartTime:            item.StartTime,
		EndTime:              item.EndTime,
		ParentSessionID:      item.ParentSessionID,
		ParentTurnID:         item.ParentTurnID,
		Outcome:              item.Outcome,
		OutcomeSummary:       item.OutcomeSummary,
		FinalResultKind:      item.FinalResultKind,
		FinalResultText:      item.FinalResultText,
		FinalResultReadyAt:   item.FinalResultReadyAt,
		CompletionNotifiedAt: item.CompletionNotifiedAt,
	}
}

func (s *Store) UpsertWorkspaceTask(ctx context.Context, item task.Task) error {
	payload, err := json.Marshal(toPersistedWorkspaceTask(item))
	if err != nil {
		return err
	}
	now := time.Now().UTC().Format(timeLayout)
	_, err = s.db.ExecContext(ctx, `
		insert into workspace_tasks (workspace_root, task_id, payload, created_at, updated_at)
		values (?, ?, ?, ?, ?)
		on conflict(workspace_root, task_id) do update set
			payload = excluded.payload,
			updated_at = excluded.updated_at
	`, item.WorkspaceRoot, item.ID, string(payload), item.StartTime.UTC().Format(timeLayout), now)
	return err
}

func (s *Store) ListWorkspaceTasks(ctx context.Context, workspaceRoot string) ([]task.Task, error) {
	rows, err := s.db.QueryContext(ctx, `
		select payload
		from workspace_tasks
		where workspace_root = ?
		order by created_at asc, task_id asc
	`, workspaceRoot)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]task.Task, 0)
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var persisted persistedWorkspaceTask
		if err := json.Unmarshal([]byte(raw), &persisted); err != nil {
			return nil, err
		}
		out = append(out, persisted.toTask())
	}
	return out, rows.Err()
}

func (s *Store) GetWorkspaceTodos(ctx context.Context, workspaceRoot string) ([]task.TodoItem, error) {
	var raw string
	err := s.db.QueryRowContext(ctx, `
		select payload
		from workspace_todos
		where workspace_root = ?
	`, workspaceRoot).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var todos []task.TodoItem
	if err := json.Unmarshal([]byte(raw), &todos); err != nil {
		return nil, err
	}
	return todos, nil
}

func (s *Store) ReplaceWorkspaceTodos(ctx context.Context, workspaceRoot string, todos []task.TodoItem) error {
	payload, err := json.Marshal(todos)
	if err != nil {
		return err
	}
	now := time.Now().UTC().Format(timeLayout)
	_, err = s.db.ExecContext(ctx, `
		insert into workspace_todos (workspace_root, payload, updated_at)
		values (?, ?, ?)
		on conflict(workspace_root) do update set
			payload = excluded.payload,
			updated_at = excluded.updated_at
	`, workspaceRoot, string(payload), now)
	return err
}
