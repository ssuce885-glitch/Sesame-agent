package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"go-agent/internal/task"
)

type taskCreateTool struct{}

type TaskCreateInput struct {
	Type        string `json:"type"`
	Command     string `json:"command"`
	Description string `json:"description,omitempty"`
}

type TaskCreateOutput struct {
	TaskID      string `json:"task_id"`
	Type        string `json:"type"`
	Command     string `json:"command"`
	Description string `json:"description,omitempty"`
}

type TaskIDInput struct {
	TaskID string `json:"task_id"`
}

type TaskGetOutput struct {
	Task task.Task `json:"task"`
}

type TaskListInput struct {
	Status string `json:"status,omitempty"`
}

type TaskListOutput struct {
	Tasks        []task.Task `json:"tasks"`
	StatusFilter string      `json:"status_filter,omitempty"`
}

type TaskOutputResult struct {
	TaskID string `json:"task_id"`
	Output string `json:"output"`
}

type TaskStopOutput struct {
	TaskID string `json:"task_id"`
}

type TaskUpdateInput struct {
	TaskID      string `json:"task_id"`
	Status      string `json:"status,omitempty"`
	Description string `json:"description,omitempty"`
}

type TaskUpdateOutput struct {
	Task task.Task `json:"task"`
}

func (taskCreateTool) IsEnabled(execCtx ExecContext) bool {
	return execCtx.TaskManager != nil
}

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
		OutputSchema: objectSchema(map[string]any{
			"task_id":     map[string]any{"type": "string"},
			"type":        map[string]any{"type": "string"},
			"command":     map[string]any{"type": "string"},
			"description": map[string]any{"type": "string"},
		}, "task_id", "type", "command"),
	}
}

func (taskCreateTool) IsConcurrencySafe() bool { return false }

func (taskCreateTool) Decode(call Call) (DecodedCall, error) {
	input := TaskCreateInput{
		Type:        strings.TrimSpace(call.StringInput("type")),
		Command:     strings.TrimSpace(call.StringInput("command")),
		Description: strings.TrimSpace(call.StringInput("description")),
	}
	if input.Type == "" {
		return DecodedCall{}, fmt.Errorf("type is required")
	}
	if input.Command == "" {
		return DecodedCall{}, fmt.Errorf("command is required")
	}
	normalized := Call{
		Name: call.Name,
		Input: map[string]any{
			"type":        input.Type,
			"command":     input.Command,
			"description": input.Description,
		},
	}
	return DecodedCall{Call: normalized, Input: input}, nil
}

func (t taskCreateTool) Execute(ctx context.Context, call Call, execCtx ExecContext) (Result, error) {
	decoded, err := t.Decode(call)
	if err != nil {
		return Result{}, err
	}
	output, err := t.ExecuteDecoded(ctx, decoded, execCtx)
	return output.Result, err
}

func (taskCreateTool) ExecuteDecoded(ctx context.Context, decoded DecodedCall, execCtx ExecContext) (ToolExecutionResult, error) {
	manager, err := requireTaskManager(execCtx)
	if err != nil {
		return ToolExecutionResult{}, err
	}

	input, _ := decoded.Input.(TaskCreateInput)
	taskType, err := decodeTaskType(input.Type)
	if err != nil {
		return ToolExecutionResult{}, err
	}

	created, err := manager.Create(ctx, task.CreateTaskInput{
		Type:          taskType,
		Command:       input.Command,
		Description:   input.Description,
		WorkspaceRoot: execCtx.WorkspaceRoot,
		Start:         true,
	})
	if err != nil {
		return ToolExecutionResult{}, err
	}

	modelText := fmt.Sprintf("Task created successfully with id: %s", created.ID)
	return ToolExecutionResult{
		Result: Result{
			Text:      created.ID,
			ModelText: modelText,
		},
		Data: TaskCreateOutput{
			TaskID:      created.ID,
			Type:        string(created.Type),
			Command:     created.Command,
			Description: created.Description,
		},
		PreviewText: modelText,
	}, nil
}

func (taskCreateTool) MapModelResult(output ToolExecutionResult) ModelToolResult {
	return defaultStructuredModelResult(output)
}

type taskGetTool struct{}

func (taskGetTool) IsEnabled(execCtx ExecContext) bool {
	return execCtx.TaskManager != nil
}

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
		OutputSchema: objectSchema(map[string]any{
			"task": map[string]any{
				"type":                 "object",
				"additionalProperties": true,
			},
		}, "task"),
	}
}

func (taskGetTool) IsConcurrencySafe() bool { return true }

func (taskGetTool) Decode(call Call) (DecodedCall, error) {
	taskID := strings.TrimSpace(call.StringInput("task_id"))
	if taskID == "" {
		return DecodedCall{}, fmt.Errorf("task_id is required")
	}
	normalized := Call{Name: call.Name, Input: map[string]any{"task_id": taskID}}
	return DecodedCall{Call: normalized, Input: TaskIDInput{TaskID: taskID}}, nil
}

func (t taskGetTool) Execute(ctx context.Context, call Call, execCtx ExecContext) (Result, error) {
	decoded, err := t.Decode(call)
	if err != nil {
		return Result{}, err
	}
	output, err := t.ExecuteDecoded(ctx, decoded, execCtx)
	return output.Result, err
}

func (taskGetTool) ExecuteDecoded(_ context.Context, decoded DecodedCall, execCtx ExecContext) (ToolExecutionResult, error) {
	manager, err := requireTaskManager(execCtx)
	if err != nil {
		return ToolExecutionResult{}, err
	}

	input, _ := decoded.Input.(TaskIDInput)
	got, err := getTaskForWorkspace(manager, input.TaskID, execCtx.WorkspaceRoot)
	if err != nil {
		return ToolExecutionResult{}, err
	}

	text := mustJSON(got)
	return ToolExecutionResult{
		Result: Result{
			Text:      text,
			ModelText: text,
		},
		Data:        TaskGetOutput{Task: got},
		PreviewText: fmt.Sprintf("Task %s (%s)", got.ID, got.Status),
	}, nil
}

func (taskGetTool) MapModelResult(output ToolExecutionResult) ModelToolResult {
	return defaultStructuredModelResult(output)
}

type taskListTool struct{}

func (taskListTool) IsEnabled(execCtx ExecContext) bool {
	return execCtx.TaskManager != nil
}

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
		OutputSchema: objectSchema(map[string]any{
			"tasks": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type":                 "object",
					"additionalProperties": true,
				},
			},
			"status_filter": map[string]any{"type": "string"},
		}, "tasks"),
	}
}

func (taskListTool) IsConcurrencySafe() bool { return true }

func (taskListTool) Decode(call Call) (DecodedCall, error) {
	status := strings.TrimSpace(call.StringInput("status"))
	normalized := Call{Name: call.Name, Input: map[string]any{}}
	if status != "" {
		normalized.Input["status"] = status
	}
	return DecodedCall{
		Call:  normalized,
		Input: TaskListInput{Status: status},
	}, nil
}

func (t taskListTool) Execute(ctx context.Context, call Call, execCtx ExecContext) (Result, error) {
	decoded, err := t.Decode(call)
	if err != nil {
		return Result{}, err
	}
	output, err := t.ExecuteDecoded(ctx, decoded, execCtx)
	return output.Result, err
}

func (taskListTool) ExecuteDecoded(_ context.Context, decoded DecodedCall, execCtx ExecContext) (ToolExecutionResult, error) {
	manager, err := requireTaskManager(execCtx)
	if err != nil {
		return ToolExecutionResult{}, err
	}

	tasks, err := manager.List(execCtx.WorkspaceRoot)
	if err != nil {
		return ToolExecutionResult{}, err
	}

	input, _ := decoded.Input.(TaskListInput)
	if input.Status != "" {
		status, err := decodeTaskStatus(input.Status)
		if err != nil {
			return ToolExecutionResult{}, err
		}
		filtered := make([]task.Task, 0, len(tasks))
		for _, item := range tasks {
			if item.Status == status {
				filtered = append(filtered, item)
			}
		}
		tasks = filtered
	}

	text := mustJSON(tasks)
	return ToolExecutionResult{
		Result: Result{
			Text:      text,
			ModelText: text,
		},
		Data: TaskListOutput{
			Tasks:        tasks,
			StatusFilter: input.Status,
		},
		PreviewText: fmt.Sprintf("Listed %d task(s)", len(tasks)),
	}, nil
}

func (taskListTool) MapModelResult(output ToolExecutionResult) ModelToolResult {
	return defaultStructuredModelResult(output)
}

type taskOutputTool struct{}

func (taskOutputTool) IsEnabled(execCtx ExecContext) bool {
	return execCtx.TaskManager != nil
}

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
		OutputSchema: objectSchema(map[string]any{
			"task_id": map[string]any{"type": "string"},
			"output":  map[string]any{"type": "string"},
		}, "task_id", "output"),
	}
}

func (taskOutputTool) IsConcurrencySafe() bool { return true }

func (taskOutputTool) Decode(call Call) (DecodedCall, error) {
	taskID := strings.TrimSpace(call.StringInput("task_id"))
	if taskID == "" {
		return DecodedCall{}, fmt.Errorf("task_id is required")
	}
	normalized := Call{Name: call.Name, Input: map[string]any{"task_id": taskID}}
	return DecodedCall{Call: normalized, Input: TaskIDInput{TaskID: taskID}}, nil
}

func (t taskOutputTool) Execute(ctx context.Context, call Call, execCtx ExecContext) (Result, error) {
	decoded, err := t.Decode(call)
	if err != nil {
		return Result{}, err
	}
	output, err := t.ExecuteDecoded(ctx, decoded, execCtx)
	return output.Result, err
}

func (taskOutputTool) ExecuteDecoded(_ context.Context, decoded DecodedCall, execCtx ExecContext) (ToolExecutionResult, error) {
	manager, err := requireTaskManager(execCtx)
	if err != nil {
		return ToolExecutionResult{}, err
	}

	input, _ := decoded.Input.(TaskIDInput)
	output, err := manager.ReadOutput(input.TaskID, execCtx.WorkspaceRoot)
	if err != nil {
		return ToolExecutionResult{}, err
	}

	return ToolExecutionResult{
		Result: Result{
			Text:      output,
			ModelText: output,
		},
		Data: TaskOutputResult{
			TaskID: input.TaskID,
			Output: output,
		},
		PreviewText: PreviewText(output, 256),
	}, nil
}

func (taskOutputTool) MapModelResult(output ToolExecutionResult) ModelToolResult {
	return defaultStructuredModelResult(output)
}

type taskStopTool struct{}

func (taskStopTool) IsEnabled(execCtx ExecContext) bool {
	return execCtx.TaskManager != nil
}

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
		OutputSchema: objectSchema(map[string]any{
			"task_id": map[string]any{"type": "string"},
		}, "task_id"),
	}
}

func (taskStopTool) IsConcurrencySafe() bool { return false }

func (taskStopTool) Decode(call Call) (DecodedCall, error) {
	taskID := strings.TrimSpace(call.StringInput("task_id"))
	if taskID == "" {
		return DecodedCall{}, fmt.Errorf("task_id is required")
	}
	normalized := Call{Name: call.Name, Input: map[string]any{"task_id": taskID}}
	return DecodedCall{Call: normalized, Input: TaskIDInput{TaskID: taskID}}, nil
}

func (t taskStopTool) Execute(ctx context.Context, call Call, execCtx ExecContext) (Result, error) {
	decoded, err := t.Decode(call)
	if err != nil {
		return Result{}, err
	}
	output, err := t.ExecuteDecoded(ctx, decoded, execCtx)
	return output.Result, err
}

func (taskStopTool) ExecuteDecoded(_ context.Context, decoded DecodedCall, execCtx ExecContext) (ToolExecutionResult, error) {
	manager, err := requireTaskManager(execCtx)
	if err != nil {
		return ToolExecutionResult{}, err
	}

	input, _ := decoded.Input.(TaskIDInput)
	taskID := input.TaskID
	if err := manager.Stop(taskID, execCtx.WorkspaceRoot); err != nil {
		return ToolExecutionResult{}, err
	}

	modelText := fmt.Sprintf("Task stopped: %s", taskID)
	return ToolExecutionResult{
		Result: Result{
			Text:      taskID,
			ModelText: modelText,
		},
		Data:        TaskStopOutput{TaskID: taskID},
		PreviewText: modelText,
	}, nil
}

func (taskStopTool) MapModelResult(output ToolExecutionResult) ModelToolResult {
	return defaultStructuredModelResult(output)
}

type taskUpdateTool struct{}

func (taskUpdateTool) IsEnabled(execCtx ExecContext) bool {
	return execCtx.TaskManager != nil
}

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
		OutputSchema: objectSchema(map[string]any{
			"task": map[string]any{
				"type":                 "object",
				"additionalProperties": true,
			},
		}, "task"),
	}
}

func (taskUpdateTool) IsConcurrencySafe() bool { return false }

func (taskUpdateTool) Decode(call Call) (DecodedCall, error) {
	input := TaskUpdateInput{
		TaskID:      strings.TrimSpace(call.StringInput("task_id")),
		Status:      strings.TrimSpace(call.StringInput("status")),
		Description: strings.TrimSpace(call.StringInput("description")),
	}
	if input.TaskID == "" {
		return DecodedCall{}, fmt.Errorf("task_id is required")
	}
	normalized := Call{
		Name: call.Name,
		Input: map[string]any{
			"task_id":     input.TaskID,
			"status":      input.Status,
			"description": input.Description,
		},
	}
	return DecodedCall{Call: normalized, Input: input}, nil
}

func (t taskUpdateTool) Execute(ctx context.Context, call Call, execCtx ExecContext) (Result, error) {
	decoded, err := t.Decode(call)
	if err != nil {
		return Result{}, err
	}
	output, err := t.ExecuteDecoded(ctx, decoded, execCtx)
	return output.Result, err
}

func (taskUpdateTool) ExecuteDecoded(_ context.Context, decoded DecodedCall, execCtx ExecContext) (ToolExecutionResult, error) {
	manager, err := requireTaskManager(execCtx)
	if err != nil {
		return ToolExecutionResult{}, err
	}

	input, _ := decoded.Input.(TaskUpdateInput)
	current, err := getTaskForWorkspace(manager, input.TaskID, execCtx.WorkspaceRoot)
	if err != nil {
		return ToolExecutionResult{}, err
	}

	nextStatus := current.Status
	if input.Status != "" {
		nextStatus, err = decodeTaskStatus(input.Status)
		if err != nil {
			return ToolExecutionResult{}, err
		}
	}

	if err := manager.Update(current.ID, execCtx.WorkspaceRoot, task.UpdateTaskInput{
		Status:      nextStatus,
		Description: input.Description,
	}); err != nil {
		return ToolExecutionResult{}, err
	}

	updated, err := getTaskForWorkspace(manager, current.ID, execCtx.WorkspaceRoot)
	if err != nil {
		return ToolExecutionResult{}, err
	}

	text := mustJSON(updated)
	return ToolExecutionResult{
		Result: Result{
			Text:      text,
			ModelText: text,
		},
		Data:        TaskUpdateOutput{Task: updated},
		PreviewText: fmt.Sprintf("Task %s updated to %s", updated.ID, updated.Status),
	}, nil
}

func (taskUpdateTool) MapModelResult(output ToolExecutionResult) ModelToolResult {
	return defaultStructuredModelResult(output)
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

func defaultStructuredModelResult(output ToolExecutionResult) ModelToolResult {
	text := output.ModelText
	if strings.TrimSpace(text) == "" {
		text = output.Text
	}
	return ModelToolResult{
		Text:       text,
		Structured: output.Data,
	}
}
