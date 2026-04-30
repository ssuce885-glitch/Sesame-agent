package tools

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"go-agent/internal/task"
	"go-agent/internal/types"
)

func runtimeTaskStateFromTaskStatus(status task.TaskStatus) types.TaskState {
	switch status {
	case task.TaskStatusRunning:
		return types.TaskStateRunning
	case task.TaskStatusCompleted:
		return types.TaskStateCompleted
	case task.TaskStatusStopped:
		return types.TaskStateCancelled
	case task.TaskStatusFailed:
		return types.TaskStateFailed
	default:
		return types.TaskStatePending
	}
}

func timeNowUTC() time.Time {
	return time.Now().UTC()
}

func currentSessionID(execCtx ExecContext) string {
	return strings.TrimSpace(execCtx.TurnContext.CurrentSessionID)
}

func currentTurnID(execCtx ExecContext) string {
	return strings.TrimSpace(execCtx.TurnContext.CurrentTurnID)
}

func currentRunID(execCtx ExecContext) string {
	return strings.TrimSpace(execCtx.TurnContext.CurrentRunID)
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
