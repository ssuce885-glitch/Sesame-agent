package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"go-agent/internal/task"
)

type taskCreateTool struct{}

func (taskCreateTool) Definition() Definition {
	return Definition{
		Name:        "task_create",
		Description: "Create and start a background task.",
		InputSchema: objectSchema(map[string]any{
			"type": map[string]any{
				"type":        "string",
				"description": "Task type: shell, agent, or remote.",
			},
			"command": map[string]any{
				"type":        "string",
				"description": "Command or prompt to execute.",
			},
			"description": map[string]any{
				"type":        "string",
				"description": "Optional task description.",
			},
		}, "type", "command"),
	}
}

func (taskCreateTool) IsConcurrencySafe() bool { return false }

func (taskCreateTool) Execute(ctx context.Context, call Call, execCtx ExecContext) (Result, error) {
	manager, err := requireTaskManager(execCtx)
	if err != nil {
		return Result{}, err
	}

	taskType, err := decodeTaskType(call.StringInput("type"))
	if err != nil {
		return Result{}, err
	}
	command := strings.TrimSpace(call.StringInput("command"))
	if command == "" {
		return Result{}, fmt.Errorf("task command is required")
	}

	created, err := manager.Create(ctx, task.CreateTaskInput{
		Type:          taskType,
		Command:       command,
		Description:   strings.TrimSpace(call.StringInput("description")),
		WorkspaceRoot: execCtx.WorkspaceRoot,
		Start:         true,
	})
	if err != nil {
		return Result{}, err
	}

	return Result{Text: created.ID}, nil
}

type taskGetTool struct{}

func (taskGetTool) Definition() Definition {
	return Definition{
		Name:        "task_get",
		Description: "Read one task's details.",
		InputSchema: objectSchema(map[string]any{
			"task_id": map[string]any{
				"type":        "string",
				"description": "Task identifier.",
			},
		}, "task_id"),
	}
}

func (taskGetTool) IsConcurrencySafe() bool { return true }

func (taskGetTool) Execute(_ context.Context, call Call, execCtx ExecContext) (Result, error) {
	manager, err := requireTaskManager(execCtx)
	if err != nil {
		return Result{}, err
	}

	got, err := getTaskForWorkspace(manager, call.StringInput("task_id"), execCtx.WorkspaceRoot)
	if err != nil {
		return Result{}, err
	}

	return Result{Text: mustJSON(got)}, nil
}

type taskListTool struct{}

func (taskListTool) Definition() Definition {
	schema := objectSchema(map[string]any{
		"status": map[string]any{
			"type":        "string",
			"description": "Optional status filter.",
		},
	})
	schema["required"] = []string{}
	return Definition{
		Name:        "task_list",
		Description: "List workspace tasks.",
		InputSchema: schema,
	}
}

func (taskListTool) IsConcurrencySafe() bool { return true }

func (taskListTool) Execute(_ context.Context, call Call, execCtx ExecContext) (Result, error) {
	manager, err := requireTaskManager(execCtx)
	if err != nil {
		return Result{}, err
	}

	tasks, err := manager.List(execCtx.WorkspaceRoot)
	if err != nil {
		return Result{}, err
	}

	if rawStatus := strings.TrimSpace(call.StringInput("status")); rawStatus != "" {
		status, err := decodeTaskStatus(rawStatus)
		if err != nil {
			return Result{}, err
		}
		filtered := make([]task.Task, 0, len(tasks))
		for _, item := range tasks {
			if item.Status == status {
				filtered = append(filtered, item)
			}
		}
		tasks = filtered
	}

	return Result{Text: mustJSON(tasks)}, nil
}

type taskOutputTool struct{}

func (taskOutputTool) Definition() Definition {
	return Definition{
		Name:        "task_output",
		Description: "Read task output.",
		InputSchema: objectSchema(map[string]any{
			"task_id": map[string]any{
				"type":        "string",
				"description": "Task identifier.",
			},
		}, "task_id"),
	}
}

func (taskOutputTool) IsConcurrencySafe() bool { return true }

func (taskOutputTool) Execute(_ context.Context, call Call, execCtx ExecContext) (Result, error) {
	manager, err := requireTaskManager(execCtx)
	if err != nil {
		return Result{}, err
	}

	output, err := manager.ReadOutput(call.StringInput("task_id"), execCtx.WorkspaceRoot)
	if err != nil {
		return Result{}, err
	}

	return Result{Text: output}, nil
}

type taskStopTool struct{}

func (taskStopTool) Definition() Definition {
	return Definition{
		Name:        "task_stop",
		Description: "Stop a running task.",
		InputSchema: objectSchema(map[string]any{
			"task_id": map[string]any{
				"type":        "string",
				"description": "Task identifier.",
			},
		}, "task_id"),
	}
}

func (taskStopTool) IsConcurrencySafe() bool { return false }

func (taskStopTool) Execute(_ context.Context, call Call, execCtx ExecContext) (Result, error) {
	manager, err := requireTaskManager(execCtx)
	if err != nil {
		return Result{}, err
	}

	taskID := call.StringInput("task_id")
	if err := manager.Stop(taskID, execCtx.WorkspaceRoot); err != nil {
		return Result{}, err
	}

	return Result{Text: taskID}, nil
}

type taskUpdateTool struct{}

func (taskUpdateTool) Definition() Definition {
	return Definition{
		Name:        "task_update",
		Description: "Update task metadata or status.",
		InputSchema: objectSchema(map[string]any{
			"task_id": map[string]any{
				"type":        "string",
				"description": "Task identifier.",
			},
			"status": map[string]any{
				"type":        "string",
				"description": "Optional status override.",
			},
			"description": map[string]any{
				"type":        "string",
				"description": "Optional description override.",
			},
		}, "task_id"),
	}
}

func (taskUpdateTool) IsConcurrencySafe() bool { return false }

func (taskUpdateTool) Execute(_ context.Context, call Call, execCtx ExecContext) (Result, error) {
	manager, err := requireTaskManager(execCtx)
	if err != nil {
		return Result{}, err
	}

	current, err := getTaskForWorkspace(manager, call.StringInput("task_id"), execCtx.WorkspaceRoot)
	if err != nil {
		return Result{}, err
	}

	nextStatus := current.Status
	if rawStatus := strings.TrimSpace(call.StringInput("status")); rawStatus != "" {
		nextStatus, err = decodeTaskStatus(rawStatus)
		if err != nil {
			return Result{}, err
		}
	}

	if err := manager.Update(current.ID, execCtx.WorkspaceRoot, task.UpdateTaskInput{
		Status:      nextStatus,
		Description: strings.TrimSpace(call.StringInput("description")),
	}); err != nil {
		return Result{}, err
	}

	updated, err := getTaskForWorkspace(manager, current.ID, execCtx.WorkspaceRoot)
	if err != nil {
		return Result{}, err
	}

	return Result{Text: mustJSON(updated)}, nil
}

func decodeTaskType(raw string) (task.TaskType, error) {
	switch taskType := task.TaskType(strings.TrimSpace(raw)); taskType {
	case task.TaskTypeShell, task.TaskTypeAgent, task.TaskTypeRemote:
		return taskType, nil
	default:
		return "", fmt.Errorf("invalid task type %q", raw)
	}
}

func decodeTaskStatus(raw string) (task.TaskStatus, error) {
	switch status := task.TaskStatus(strings.TrimSpace(raw)); status {
	case task.TaskStatusPending, task.TaskStatusRunning, task.TaskStatusCompleted, task.TaskStatusFailed, task.TaskStatusStopped:
		return status, nil
	default:
		return "", fmt.Errorf("invalid task status %q", raw)
	}
}

func getTaskForWorkspace(manager *task.Manager, taskID, workspaceRoot string) (task.Task, error) {
	got, ok, err := manager.Get(taskID, workspaceRoot)
	if err != nil {
		return task.Task{}, err
	}
	if !ok {
		return task.Task{}, fmt.Errorf("task %q not found", taskID)
	}
	return got, nil
}

func mustJSON(v any) string {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Sprintf("%v", v)
	}
	return string(data)
}
