package tools

import (
	"context"
	"fmt"
	"strings"

	"go-agent/internal/task"
	"go-agent/internal/types"
)

type taskStopTool struct{}

func (taskStopTool) IsEnabled(execCtx ExecContext) bool { return execCtx.TaskManager != nil }

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

func (taskStopTool) ExecuteDecoded(ctx context.Context, decoded DecodedCall, execCtx ExecContext) (ToolExecutionResult, error) {
	manager, err := requireTaskManager(execCtx)
	if err != nil {
		return ToolExecutionResult{}, err
	}

	input, _ := decoded.Input.(TaskIDInput)
	taskID := input.TaskID
	if err := manager.Stop(taskID, execCtx.WorkspaceRoot); err != nil {
		return ToolExecutionResult{}, err
	}
	updated, err := getTaskForWorkspace(manager, taskID, execCtx.WorkspaceRoot)
	if err == nil {
		if execCtx.RuntimeService != nil && execCtx.TurnContext != nil {
			_ = execCtx.RuntimeService.UpdateTask(ctx, types.Task{
				ID:              updated.ID,
				RunID:           currentRunID(execCtx),
				ParentTaskID:    updated.ParentTaskID,
				State:           runtimeTaskStateFromTaskStatus(updated.Status),
				Title:           updated.Command,
				Description:     updated.Description,
				Owner:           updated.Owner,
				Kind:            updated.Kind,
				ExecutionTaskID: updated.ExecutionTaskID,
				WorktreeID:      updated.WorktreeID,
				CreatedAt:       updated.StartTime,
				UpdatedAt:       timeNowUTC(),
			})
		}
		emitTimelineBlockEvent(ctx, execCtx, types.EventTaskUpdated, timelineBlockFromManagerTask(updated, currentRunID(execCtx)))
	}

	modelText := fmt.Sprintf("Task stopped: %s", taskID)
	return ToolExecutionResult{
		Result:      Result{Text: taskID, ModelText: modelText},
		Data:        TaskStopOutput{TaskID: taskID},
		PreviewText: modelText,
	}, nil
}

func (taskStopTool) MapModelResult(output ToolExecutionResult) ModelToolResult {
	return defaultStructuredModelResult(output)
}

type taskUpdateTool struct{}

func (taskUpdateTool) IsEnabled(execCtx ExecContext) bool { return execCtx.TaskManager != nil }

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
				"enum":        []string{"pending", "running", "completed", "failed", "stopped"},
			},
			"description": map[string]any{
				"type":        "string",
				"description": "Optional description override.",
			},
			"owner": map[string]any{
				"type":        "string",
				"description": "Optional owner override.",
			},
			"worktree_id": map[string]any{
				"type":        "string",
				"description": "Optional worktree link override.",
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
		Owner:       strings.TrimSpace(call.StringInput("owner")),
		WorktreeID:  strings.TrimSpace(call.StringInput("worktree_id")),
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
			"owner":       input.Owner,
			"worktree_id": input.WorktreeID,
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

func (taskUpdateTool) ExecuteDecoded(ctx context.Context, decoded DecodedCall, execCtx ExecContext) (ToolExecutionResult, error) {
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
		Owner:       input.Owner,
		WorktreeID:  input.WorktreeID,
	}); err != nil {
		return ToolExecutionResult{}, err
	}

	updated, err := getTaskForWorkspace(manager, current.ID, execCtx.WorkspaceRoot)
	if err != nil {
		return ToolExecutionResult{}, err
	}

	text := mustJSON(updated)
	if execCtx.RuntimeService != nil && execCtx.TurnContext != nil {
		_ = execCtx.RuntimeService.UpdateTask(ctx, types.Task{
			ID:              updated.ID,
			RunID:           currentRunID(execCtx),
			ParentTaskID:    updated.ParentTaskID,
			State:           runtimeTaskStateFromTaskStatus(updated.Status),
			Title:           updated.Command,
			Description:     updated.Description,
			Owner:           updated.Owner,
			Kind:            updated.Kind,
			ExecutionTaskID: updated.ExecutionTaskID,
			WorktreeID:      updated.WorktreeID,
			CreatedAt:       updated.StartTime,
			UpdatedAt:       timeNowUTC(),
		})
	}
	emitTimelineBlockEvent(ctx, execCtx, types.EventTaskUpdated, timelineBlockFromManagerTask(updated, currentRunID(execCtx)))
	return ToolExecutionResult{
		Result:      Result{Text: text, ModelText: text},
		Data:        TaskUpdateOutput{Task: updated},
		PreviewText: fmt.Sprintf("Task %s updated to %s", updated.ID, updated.Status),
	}, nil
}

func (taskUpdateTool) MapModelResult(output ToolExecutionResult) ModelToolResult {
	return defaultStructuredModelResult(output)
}
