package tools

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go-agent/internal/task"
)

type taskGetTool struct{}

func (taskGetTool) IsEnabled(execCtx ExecContext) bool { return execCtx.TaskManager != nil }

func (taskGetTool) Definition() Definition {
	return Definition{
		Name:        "task_get",
		Description: "Read one task's current state and details. For agent tasks, use this only for a single status check when the user asks; if still running, answer with the current state and stop instead of polling.",
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
	modelText := text
	if got.Type == task.TaskTypeAgent {
		modelText = text + "\n\nThis is an agent task. If it is still running, do not call task_wait, repeat task_get/task_output/task_result, or use shell_command sleep to wait. Tell the user the current state and stop; the result returns through reports."
	}
	return ToolExecutionResult{
		Result: Result{Text: text, ModelText: modelText},
		Data:   TaskGetOutput{Task: got},
		PreviewText: fmt.Sprintf("Task %s (%s)",
			got.ID, got.Status),
	}, nil
}

func (taskGetTool) MapModelResult(output ToolExecutionResult) ModelToolResult {
	return defaultStructuredModelResult(output)
}

type taskListTool struct{}

func (taskListTool) IsEnabled(execCtx ExecContext) bool { return execCtx.TaskManager != nil }

func (taskListTool) Definition() Definition {
	schema := objectSchema(map[string]any{
		"status": map[string]any{
			"type":        "string",
			"description": "Optional status filter.",
			"enum":        []string{"pending", "running", "completed", "failed", "stopped"},
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
	return DecodedCall{Call: normalized, Input: TaskListInput{Status: status}}, nil
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
		Result: Result{Text: text, ModelText: text},
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

func (taskOutputTool) IsEnabled(execCtx ExecContext) bool { return execCtx.TaskManager != nil }

func (taskOutputTool) Definition() Definition {
	return Definition{
		Name:        "task_output",
		Description: "Read raw task output logs to inspect what a running task is doing. This is not a final-result channel; do not use it repeatedly to poll an agent task.",
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

	modelText := output
	if got, err := getTaskForWorkspace(manager, input.TaskID, execCtx.WorkspaceRoot); err == nil && got.Type == task.TaskTypeAgent && got.Status != task.TaskStatusCompleted {
		modelText = output + "\n\nThis is a running agent task. Do not poll it or sleep waiting for it. Report this progress and stop; the final result returns through reports."
	}

	return ToolExecutionResult{
		Result: Result{Text: output, ModelText: modelText},
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

type taskWaitTool struct{}

func (taskWaitTool) IsEnabled(execCtx ExecContext) bool { return execCtx.TaskManager != nil }

func (taskWaitTool) Definition() Definition {
	return Definition{
		Name:        "task_wait",
		Description: "Wait for a non-agent task as a transport-level poll. Never use task_wait for agent or delegated role tasks; their results return through reports.",
		InputSchema: objectSchema(map[string]any{
			"task_id": map[string]any{
				"type":        "string",
				"description": "Task identifier.",
			},
			"timeout_ms": map[string]any{
				"type":        "integer",
				"description": "Optional wait timeout in milliseconds. Defaults to 30000.",
			},
		}, "task_id"),
		OutputSchema: objectSchema(map[string]any{
			"task":         map[string]any{"type": "object", "additionalProperties": true},
			"timed_out":    map[string]any{"type": "boolean"},
			"result_ready": map[string]any{"type": "boolean"},
		}, "task", "timed_out", "result_ready"),
	}
}

func (taskWaitTool) IsConcurrencySafe() bool { return true }

func (taskWaitTool) Decode(call Call) (DecodedCall, error) {
	taskID := strings.TrimSpace(call.StringInput("task_id"))
	if taskID == "" {
		return DecodedCall{}, fmt.Errorf("task_id is required")
	}
	timeoutMS, err := decodeShellPositiveInt(call.Input["timeout_ms"], defaultTaskWaitTimeoutMS)
	if err != nil {
		return DecodedCall{}, fmt.Errorf("timeout_ms %w", err)
	}
	if timeoutMS > maxTaskWaitTimeoutMS {
		return DecodedCall{}, fmt.Errorf("timeout_ms exceeds max allowed (%d)", maxTaskWaitTimeoutMS)
	}
	normalized := Call{Name: call.Name, Input: map[string]any{"task_id": taskID, "timeout_ms": timeoutMS}}
	return DecodedCall{
		Call:  normalized,
		Input: TaskWaitInput{TaskID: taskID, TimeoutMS: timeoutMS},
	}, nil
}

func (t taskWaitTool) Execute(ctx context.Context, call Call, execCtx ExecContext) (Result, error) {
	decoded, err := t.Decode(call)
	if err != nil {
		return Result{}, err
	}
	output, err := t.ExecuteDecoded(ctx, decoded, execCtx)
	return output.Result, err
}

func (taskWaitTool) ExecuteDecoded(ctx context.Context, decoded DecodedCall, execCtx ExecContext) (ToolExecutionResult, error) {
	manager, err := requireTaskManager(execCtx)
	if err != nil {
		return ToolExecutionResult{}, err
	}

	input, _ := decoded.Input.(TaskWaitInput)
	current, err := getTaskForWorkspace(manager, input.TaskID, execCtx.WorkspaceRoot)
	if err != nil {
		return ToolExecutionResult{}, err
	}
	if current.Type == task.TaskTypeAgent {
		return ToolExecutionResult{}, fmt.Errorf(
			"task_wait is not supported for agent tasks; do not poll or sleep waiting for it. Tell the user the current state and stop because the result returns through reports",
		)
	}
	waitCtx, cancel := context.WithTimeout(ctx, time.Duration(input.TimeoutMS)*time.Millisecond)
	defer cancel()

	got, timedOut, err := manager.Wait(waitCtx, input.TaskID, execCtx.WorkspaceRoot)
	if err != nil {
		return ToolExecutionResult{}, err
	}

	output := TaskWaitOutput{Task: got, TimedOut: timedOut, ResultReady: got.ResultReady()}
	text := mustJSON(output)
	modelText := fmt.Sprintf("Task %s reached %s", got.ID, got.Status)
	previewText := modelText
	if timedOut {
		modelText = fmt.Sprintf(
			"Task %s is still %s. This wait call timed out; do not assume failure. Inspect with task_get/task_output and wait again only if needed.",
			got.ID,
			got.Status,
		)
		previewText = fmt.Sprintf("Task %s still %s (wait expired)", got.ID, got.Status)
	}
	return ToolExecutionResult{
		Result:      Result{Text: text, ModelText: modelText},
		Data:        output,
		PreviewText: previewText,
	}, nil
}

func (taskWaitTool) MapModelResult(output ToolExecutionResult) ModelToolResult {
	return defaultStructuredModelResult(output)
}

type taskResultTool struct{}

func (taskResultTool) IsEnabled(execCtx ExecContext) bool { return execCtx.TaskManager != nil }

func (taskResultTool) Definition() Definition {
	return Definition{
		Name:        "task_result",
		Description: "Read the final result of an agent task only when it is expected to be ready. If not ready, stop after reporting that the result is pending; do not poll.",
		InputSchema: objectSchema(map[string]any{
			"task_id": map[string]any{
				"type":        "string",
				"description": "Task identifier.",
			},
		}, "task_id"),
		OutputSchema: objectSchema(map[string]any{
			"task_id":     map[string]any{"type": "string"},
			"status":      map[string]any{"type": "string"},
			"kind":        map[string]any{"type": "string"},
			"text":        map[string]any{"type": "string"},
			"observed_at": map[string]any{"type": "string"},
		}, "task_id", "status"),
	}
}

func (taskResultTool) IsConcurrencySafe() bool { return true }

func (taskResultTool) Decode(call Call) (DecodedCall, error) {
	taskID := strings.TrimSpace(call.StringInput("task_id"))
	if taskID == "" {
		return DecodedCall{}, fmt.Errorf("task_id is required")
	}
	normalized := Call{Name: call.Name, Input: map[string]any{"task_id": taskID}}
	return DecodedCall{Call: normalized, Input: TaskIDInput{TaskID: taskID}}, nil
}

func (t taskResultTool) Execute(ctx context.Context, call Call, execCtx ExecContext) (Result, error) {
	decoded, err := t.Decode(call)
	if err != nil {
		return Result{}, err
	}
	output, err := t.ExecuteDecoded(ctx, decoded, execCtx)
	return output.Result, err
}

func (taskResultTool) ExecuteDecoded(_ context.Context, decoded DecodedCall, execCtx ExecContext) (ToolExecutionResult, error) {
	manager, err := requireTaskManager(execCtx)
	if err != nil {
		return ToolExecutionResult{}, err
	}

	input, _ := decoded.Input.(TaskIDInput)
	finalResult, ready, err := manager.ReadResult(input.TaskID, execCtx.WorkspaceRoot)
	if err != nil {
		return ToolExecutionResult{}, err
	}
	if !ready {
		output := TaskResultOutput{TaskID: input.TaskID, Status: taskResultStatusMissing}
		text := mustJSON(output)
		return ToolExecutionResult{
			Result:      Result{Text: text, ModelText: text + "\n\nThe final result is not ready. Do not poll or sleep waiting for it; report that it is pending and stop."},
			Data:        output,
			PreviewText: fmt.Sprintf("Task %s result not available", input.TaskID),
		}, nil
	}

	output := TaskResultOutput{
		TaskID:     input.TaskID,
		Status:     taskResultStatusReady,
		Kind:       string(finalResult.Kind),
		Text:       finalResult.Text,
		ObservedAt: &finalResult.ObservedAt,
	}
	modelText := finalResult.Text
	if strings.TrimSpace(modelText) == "" {
		modelText = mustJSON(output)
	}
	return ToolExecutionResult{
		Result:      Result{Text: finalResult.Text, ModelText: modelText},
		Data:        output,
		PreviewText: fmt.Sprintf("Task %s result ready", input.TaskID),
	}, nil
}

func (taskResultTool) MapModelResult(output ToolExecutionResult) ModelToolResult {
	return defaultStructuredModelResult(output)
}
