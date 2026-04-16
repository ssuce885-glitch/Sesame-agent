package task

import (
	"encoding/json"
	"errors"
	"go-agent/internal/types"
	"os"
	"time"
)

type tasksFilePayload struct {
	Tasks []persistedTask `json:"tasks"`
}

type persistedTask struct {
	ID                   string                  `json:"id"`
	Type                 TaskType                `json:"type"`
	Status               TaskStatus              `json:"status"`
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
	FinalResultKind      FinalResultKind         `json:"final_result_kind,omitempty"`
	FinalResultText      string                  `json:"final_result_text,omitempty"`
	FinalResultReadyAt   *time.Time              `json:"final_result_ready_at,omitempty"`
	CompletionNotifiedAt *time.Time              `json:"completion_notified_at,omitempty"`
}

func toPersistedTask(task Task) persistedTask {
	return persistedTask{
		ID:                   task.ID,
		Type:                 task.Type,
		Status:               task.Status,
		Command:              task.Command,
		Description:          task.Description,
		ParentTaskID:         task.ParentTaskID,
		Owner:                task.Owner,
		Kind:                 task.Kind,
		ExecutionTaskID:      task.ExecutionTaskID,
		WorktreeID:           task.WorktreeID,
		ScheduledJobID:       task.ScheduledJobID,
		ActivatedSkillNames:  append([]string(nil), task.ActivatedSkillNames...),
		WorkspaceRoot:        task.WorkspaceRoot,
		Output:               task.Output,
		OutputPath:           task.OutputPath,
		Error:                task.Error,
		TimeoutSeconds:       task.TimeoutSeconds,
		StartTime:            task.StartTime,
		EndTime:              task.EndTime,
		ParentSessionID:      task.ParentSessionID,
		ParentTurnID:         task.ParentTurnID,
		Outcome:              task.Outcome,
		OutcomeSummary:       task.OutcomeSummary,
		FinalResultKind:      task.FinalResultKind,
		FinalResultText:      task.FinalResultText,
		FinalResultReadyAt:   task.FinalResultReadyAt,
		CompletionNotifiedAt: task.CompletionNotifiedAt,
	}
}

func (task persistedTask) toTask() Task {
	return Task{
		ID:                   task.ID,
		Type:                 task.Type,
		Status:               task.Status,
		Command:              task.Command,
		Description:          task.Description,
		ParentTaskID:         task.ParentTaskID,
		Owner:                task.Owner,
		Kind:                 task.Kind,
		ExecutionTaskID:      task.ExecutionTaskID,
		WorktreeID:           task.WorktreeID,
		ScheduledJobID:       task.ScheduledJobID,
		ActivatedSkillNames:  append([]string(nil), task.ActivatedSkillNames...),
		WorkspaceRoot:        task.WorkspaceRoot,
		Output:               task.Output,
		OutputPath:           task.OutputPath,
		Error:                task.Error,
		TimeoutSeconds:       task.TimeoutSeconds,
		StartTime:            task.StartTime,
		EndTime:              task.EndTime,
		ParentSessionID:      task.ParentSessionID,
		ParentTurnID:         task.ParentTurnID,
		Outcome:              task.Outcome,
		OutcomeSummary:       task.OutcomeSummary,
		FinalResultKind:      task.FinalResultKind,
		FinalResultText:      task.FinalResultText,
		FinalResultReadyAt:   task.FinalResultReadyAt,
		CompletionNotifiedAt: task.CompletionNotifiedAt,
	}
}

func loadTasksFile(path string) ([]Task, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var payload tasksFilePayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, err
	}

	tasks := make([]Task, 0, len(payload.Tasks))
	for _, persisted := range payload.Tasks {
		tasks = append(tasks, persisted.toTask())
	}
	return tasks, nil
}

func writeTasksFile(path string, tasks []Task) error {
	persisted := make([]persistedTask, 0, len(tasks))
	for _, task := range tasks {
		persisted = append(persisted, toPersistedTask(task))
	}
	payload := tasksFilePayload{Tasks: persisted}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

func writeTodosFile(path string, todos []TodoItem) error {
	data, err := json.MarshalIndent(todos, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, append(data, '\n'), 0o644)
}
