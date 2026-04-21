package tools

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go-agent/internal/runtimegraph"
	"go-agent/internal/task"
	"go-agent/internal/types"
)

type taskCreateTool struct{}

type TaskCreateInput struct {
	Type         string `json:"type"`
	Command      string `json:"command"`
	Description  string `json:"description,omitempty"`
	PlanID       string `json:"plan_id,omitempty"`
	ParentTaskID string `json:"parent_task_id,omitempty"`
	Owner        string `json:"owner,omitempty"`
	Kind         string `json:"kind,omitempty"`
	WorktreeID   string `json:"worktree_id,omitempty"`
	Start        *bool  `json:"start,omitempty"`
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

type TaskWaitInput struct {
	TaskID    string `json:"task_id"`
	TimeoutMS int    `json:"timeout_ms"`
}

type TaskWaitOutput struct {
	Task        task.Task `json:"task"`
	TimedOut    bool      `json:"timed_out"`
	ResultReady bool      `json:"result_ready"`
}

type TaskResultOutput struct {
	TaskID     string     `json:"task_id"`
	Status     string     `json:"status"`
	Kind       string     `json:"kind,omitempty"`
	Text       string     `json:"text,omitempty"`
	ObservedAt *time.Time `json:"observed_at,omitempty"`
}

type TaskStopOutput struct {
	TaskID string `json:"task_id"`
}

type TaskUpdateInput struct {
	TaskID      string `json:"task_id"`
	Status      string `json:"status,omitempty"`
	Description string `json:"description,omitempty"`
	Owner       string `json:"owner,omitempty"`
	WorktreeID  string `json:"worktree_id,omitempty"`
}

type TaskUpdateOutput struct {
	Task task.Task `json:"task"`
}

const (
	defaultTaskWaitTimeoutMS = 30000
	maxTaskWaitTimeoutMS     = 300000
	taskResultStatusReady    = "ready"
	taskResultStatusMissing  = "not_available"
)

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
				"enum":        []string{"shell", "agent", "remote"},
			},
			"command": map[string]any{
				"type":        "string",
				"description": "Command or prompt to execute.",
			},
			"description": map[string]any{
				"type":        "string",
				"description": "Optional task description.",
			},
			"plan_id": map[string]any{
				"type":        "string",
				"description": "Optional runtime plan identifier.",
			},
			"parent_task_id": map[string]any{
				"type":        "string",
				"description": "Optional parent task identifier.",
			},
			"owner": map[string]any{
				"type":        "string",
				"description": "Optional task owner label.",
			},
			"kind": map[string]any{
				"type":        "string",
				"description": "Optional orchestration kind label.",
			},
			"worktree_id": map[string]any{
				"type":        "string",
				"description": "Optional linked worktree id.",
			},
			"start": map[string]any{
				"type":        "boolean",
				"description": "Whether to start immediately. Defaults to true.",
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
		Type:         strings.TrimSpace(call.StringInput("type")),
		Command:      strings.TrimSpace(call.StringInput("command")),
		Description:  strings.TrimSpace(call.StringInput("description")),
		PlanID:       strings.TrimSpace(call.StringInput("plan_id")),
		ParentTaskID: strings.TrimSpace(call.StringInput("parent_task_id")),
		Owner:        strings.TrimSpace(call.StringInput("owner")),
		Kind:         strings.TrimSpace(call.StringInput("kind")),
		WorktreeID:   strings.TrimSpace(call.StringInput("worktree_id")),
	}
	if start, ok := call.Input["start"].(bool); ok {
		input.Start = &start
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
			"type":           input.Type,
			"command":        input.Command,
			"description":    input.Description,
			"plan_id":        input.PlanID,
			"parent_task_id": input.ParentTaskID,
			"owner":          input.Owner,
			"kind":           input.Kind,
			"worktree_id":    input.WorktreeID,
			"start":          input.Start,
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
	start := true
	if input.Start != nil {
		start = *input.Start
	}

	taskKind := input.Kind
	if taskKind == "" {
		taskKind = string(taskType)
	}
	activatedSkillNames := []string(nil)
	if taskType == task.TaskTypeAgent {
		activatedSkillNames, err = resolveChildTaskSkillNames(execCtx, input.Command)
		if err != nil {
			return ToolExecutionResult{}, err
		}
	}
	created, err := manager.Create(ctx, task.CreateTaskInput{
		Type:                taskType,
		Command:             input.Command,
		Description:         input.Description,
		ParentTaskID:        input.ParentTaskID,
		ParentSessionID:     currentSessionID(execCtx),
		ParentTurnID:        currentTurnID(execCtx),
		Owner:               input.Owner,
		Kind:                taskKind,
		WorktreeID:          input.WorktreeID,
		ActivatedSkillNames: activatedSkillNames,
		WorkspaceRoot:       execCtx.WorkspaceRoot,
		Start:               start,
	})
	if err != nil {
		return ToolExecutionResult{}, err
	}
	emittedRuntimeBlock := false
	if execCtx.RuntimeService != nil && execCtx.TurnContext != nil {
		if runtimeTask, err := execCtx.RuntimeService.CreateTask(ctx, execCtx.TurnContext, runtimegraph.CreateTaskInput{
			SessionID:       execCtx.TurnContext.CurrentSessionID,
			TurnID:          execCtx.TurnContext.CurrentTurnID,
			PlanID:          input.PlanID,
			ParentTaskID:    input.ParentTaskID,
			Title:           input.Command,
			Description:     input.Description,
			Owner:           input.Owner,
			Kind:            taskKind,
			ExecutionTaskID: created.ID,
			WorktreeID:      input.WorktreeID,
		}); err == nil {
			runtimeTask.State = runtimeTaskStateFromTaskStatus(created.Status)
			runtimeTask.Description = firstNonEmptyString(created.Description, runtimeTask.Description)
			runtimeTask.Owner = firstNonEmptyString(created.Owner, runtimeTask.Owner)
			runtimeTask.Kind = firstNonEmptyString(created.Kind, runtimeTask.Kind)
			runtimeTask.ExecutionTaskID = firstNonEmptyString(created.ExecutionTaskID, runtimeTask.ExecutionTaskID, created.ID)
			runtimeTask.WorktreeID = firstNonEmptyString(created.WorktreeID, runtimeTask.WorktreeID)
			runtimeTask.UpdatedAt = timeNowUTC()
			_ = execCtx.RuntimeService.UpdateTask(ctx, runtimeTask)
			execCtx.TurnContext.CurrentTaskID = runtimeTask.ID
			emitTimelineBlockEvent(ctx, execCtx, types.EventTaskUpdated, types.TimelineBlockFromTask(runtimeTask))
			emittedRuntimeBlock = true
		}
	}
	if !emittedRuntimeBlock {
		emitTimelineBlockEvent(ctx, execCtx, types.EventTaskUpdated, timelineBlockFromManagerTask(created, currentRunID(execCtx)))
	}

	modelText := fmt.Sprintf("Task created successfully with id: %s", created.ID)
	if created.Type == task.TaskTypeAgent {
		modelText = fmt.Sprintf(
			"Task created successfully with id: %s. Agent tasks run in the background; do not call task_wait. Continue the turn and wait for child reports unless you need to inspect state with task_get/task_output/task_result.",
			created.ID,
		)
	}
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
