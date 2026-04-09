package tools

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"go-agent/internal/runtimegraph"
	"go-agent/internal/task"
	"go-agent/internal/types"
)

type enterWorktreeTool struct{}
type exitWorktreeTool struct{}

type EnterWorktreeInput struct {
	TaskID string `json:"task_id"`
	Branch string `json:"branch,omitempty"`
}

type EnterWorktreeOutput struct {
	WorktreeID   string `json:"worktree_id"`
	TaskID       string `json:"task_id"`
	WorktreePath string `json:"worktree_path"`
	Branch       string `json:"branch,omitempty"`
	State        string `json:"state"`
}

type ExitWorktreeInput struct {
	WorktreeID   string `json:"worktree_id"`
	WorktreePath string `json:"worktree_path"`
	TaskID       string `json:"task_id,omitempty"`
	Cleanup      bool   `json:"cleanup,omitempty"`
}

type ExitWorktreeOutput struct {
	WorktreeID   string `json:"worktree_id"`
	WorktreePath string `json:"worktree_path"`
	State        string `json:"state"`
}

func (enterWorktreeTool) IsEnabled(execCtx ExecContext) bool {
	return execCtx.TaskManager != nil && execCtx.RuntimeService != nil && execCtx.TurnContext != nil
}

func (exitWorktreeTool) IsEnabled(execCtx ExecContext) bool {
	return execCtx.RuntimeService != nil && execCtx.TurnContext != nil
}

func (enterWorktreeTool) Definition() Definition {
	return Definition{
		Name:        "enter_worktree",
		Description: "Create and register a git worktree for a task.",
		InputSchema: objectSchema(map[string]any{
			"task_id": map[string]any{
				"type":        "string",
				"description": "Task identifier.",
			},
			"branch": map[string]any{
				"type":        "string",
				"description": "Optional branch to create for the worktree.",
			},
		}, "task_id"),
		OutputSchema: objectSchema(map[string]any{
			"worktree_id":   map[string]any{"type": "string"},
			"task_id":       map[string]any{"type": "string"},
			"worktree_path": map[string]any{"type": "string"},
			"branch":        map[string]any{"type": "string"},
			"state":         map[string]any{"type": "string"},
		}, "worktree_id", "task_id", "worktree_path", "state"),
	}
}

func (exitWorktreeTool) Definition() Definition {
	return Definition{
		Name:        "exit_worktree",
		Description: "Detach or remove a previously created worktree.",
		InputSchema: objectSchema(map[string]any{
			"worktree_id": map[string]any{
				"type":        "string",
				"description": "Registered worktree identifier.",
			},
			"worktree_path": map[string]any{
				"type":        "string",
				"description": "Filesystem path of the worktree.",
			},
			"task_id": map[string]any{
				"type":        "string",
				"description": "Optional task identifier to update.",
			},
			"cleanup": map[string]any{
				"type":        "boolean",
				"description": "Remove the physical worktree directory.",
			},
		}, "worktree_id", "worktree_path"),
		OutputSchema: objectSchema(map[string]any{
			"worktree_id":   map[string]any{"type": "string"},
			"worktree_path": map[string]any{"type": "string"},
			"state":         map[string]any{"type": "string"},
		}, "worktree_id", "worktree_path", "state"),
	}
}

func (enterWorktreeTool) IsConcurrencySafe() bool { return false }
func (exitWorktreeTool) IsConcurrencySafe() bool  { return false }

func (enterWorktreeTool) Decode(call Call) (DecodedCall, error) {
	input := EnterWorktreeInput{
		TaskID: strings.TrimSpace(call.StringInput("task_id")),
		Branch: strings.TrimSpace(call.StringInput("branch")),
	}
	if input.TaskID == "" {
		return DecodedCall{}, fmt.Errorf("task_id is required")
	}
	return DecodedCall{
		Call: Call{
			ID:   call.ID,
			Name: call.Name,
			Input: map[string]any{
				"task_id": input.TaskID,
				"branch":  input.Branch,
			},
		},
		Input: input,
	}, nil
}

func (exitWorktreeTool) Decode(call Call) (DecodedCall, error) {
	input := ExitWorktreeInput{
		WorktreeID:   strings.TrimSpace(call.StringInput("worktree_id")),
		WorktreePath: strings.TrimSpace(call.StringInput("worktree_path")),
		TaskID:       strings.TrimSpace(call.StringInput("task_id")),
	}
	if cleanup, ok := call.Input["cleanup"].(bool); ok {
		input.Cleanup = cleanup
	}
	if input.WorktreeID == "" || input.WorktreePath == "" {
		return DecodedCall{}, fmt.Errorf("worktree_id and worktree_path are required")
	}
	return DecodedCall{
		Call: Call{
			ID:   call.ID,
			Name: call.Name,
			Input: map[string]any{
				"worktree_id":   input.WorktreeID,
				"worktree_path": input.WorktreePath,
				"task_id":       input.TaskID,
				"cleanup":       input.Cleanup,
			},
		},
		Input: input,
	}, nil
}

func (t enterWorktreeTool) Execute(ctx context.Context, call Call, execCtx ExecContext) (Result, error) {
	decoded, err := t.Decode(call)
	if err != nil {
		return Result{}, err
	}
	output, err := t.ExecuteDecoded(ctx, decoded, execCtx)
	return output.Result, err
}

func (t exitWorktreeTool) Execute(ctx context.Context, call Call, execCtx ExecContext) (Result, error) {
	decoded, err := t.Decode(call)
	if err != nil {
		return Result{}, err
	}
	output, err := t.ExecuteDecoded(ctx, decoded, execCtx)
	return output.Result, err
}

func (enterWorktreeTool) ExecuteDecoded(ctx context.Context, decoded DecodedCall, execCtx ExecContext) (ToolExecutionResult, error) {
	manager, err := requireTaskManager(execCtx)
	if err != nil {
		return ToolExecutionResult{}, err
	}
	input, _ := decoded.Input.(EnterWorktreeInput)
	taskRow, err := getTaskForWorkspace(manager, input.TaskID, execCtx.WorkspaceRoot)
	if err != nil {
		return ToolExecutionResult{}, err
	}
	repoRoot, err := gitTopLevel(ctx, execCtx.WorkspaceRoot)
	if err != nil {
		return ToolExecutionResult{}, err
	}
	worktreePath := filepath.Join(repoRoot, ".worktrees", sanitizeWorktreeSlug(taskRow.ID))
	if err := os.MkdirAll(filepath.Dir(worktreePath), 0o755); err != nil {
		return ToolExecutionResult{}, err
	}
	args := []string{"-C", repoRoot, "worktree", "add"}
	branch := strings.TrimSpace(input.Branch)
	if branch != "" {
		args = append(args, "-b", branch)
	} else {
		args = append(args, "--detach")
	}
	args = append(args, worktreePath, "HEAD")
	if _, err := runGit(ctx, args...); err != nil {
		return ToolExecutionResult{}, err
	}
	worktree, err := execCtx.RuntimeService.UpsertWorktree(ctx, execCtx.TurnContext, runtimegraph.UpsertWorktreeInput{
		SessionID:      execCtx.TurnContext.CurrentSessionID,
		TurnID:         execCtx.TurnContext.CurrentTurnID,
		TaskID:         taskRow.ID,
		State:          types.WorktreeStateActive,
		WorktreePath:   worktreePath,
		WorktreeBranch: branch,
	})
	if err != nil {
		return ToolExecutionResult{}, err
	}
	_ = manager.Update(taskRow.ID, execCtx.WorkspaceRoot, task.UpdateTaskInput{
		Status:     taskRow.Status,
		WorktreeID: worktree.ID,
	})
	emitTimelineBlockEvent(ctx, execCtx, types.EventWorktreeUpdated, types.TimelineBlockFromWorktree(worktree))
	if updatedTask, err := getTaskForWorkspace(manager, taskRow.ID, execCtx.WorkspaceRoot); err == nil {
		emitTimelineBlockEvent(ctx, execCtx, types.EventTaskUpdated, timelineBlockFromManagerTask(updatedTask, currentRunID(execCtx)))
	}
	text := fmt.Sprintf("Created worktree %s for task %s", worktree.WorktreePath, taskRow.ID)
	return ToolExecutionResult{
		Result: Result{Text: text, ModelText: text},
		Data: EnterWorktreeOutput{
			WorktreeID:   worktree.ID,
			TaskID:       taskRow.ID,
			WorktreePath: worktree.WorktreePath,
			Branch:       worktree.WorktreeBranch,
			State:        string(worktree.State),
		},
		PreviewText: text,
	}, nil
}

func (exitWorktreeTool) ExecuteDecoded(ctx context.Context, decoded DecodedCall, execCtx ExecContext) (ToolExecutionResult, error) {
	input, _ := decoded.Input.(ExitWorktreeInput)
	state := types.WorktreeStateDetached
	if input.Cleanup {
		repoRoot, err := gitTopLevel(ctx, execCtx.WorkspaceRoot)
		if err != nil {
			return ToolExecutionResult{}, err
		}
		if _, err := runGit(ctx, "-C", repoRoot, "worktree", "remove", "--force", input.WorktreePath); err != nil {
			return ToolExecutionResult{}, err
		}
		state = types.WorktreeStateRemoved
	}
	worktree, err := execCtx.RuntimeService.UpsertWorktree(ctx, execCtx.TurnContext, runtimegraph.UpsertWorktreeInput{
		SessionID:    execCtx.TurnContext.CurrentSessionID,
		TurnID:       execCtx.TurnContext.CurrentTurnID,
		TaskID:       input.TaskID,
		WorktreeID:   input.WorktreeID,
		State:        state,
		WorktreePath: input.WorktreePath,
	})
	if err != nil {
		return ToolExecutionResult{}, err
	}
	emitTimelineBlockEvent(ctx, execCtx, types.EventWorktreeUpdated, types.TimelineBlockFromWorktree(worktree))
	if input.TaskID != "" {
		if manager, err := requireTaskManager(execCtx); err == nil {
			if updatedTask, err := getTaskForWorkspace(manager, input.TaskID, execCtx.WorkspaceRoot); err == nil {
				emitTimelineBlockEvent(ctx, execCtx, types.EventTaskUpdated, timelineBlockFromManagerTask(updatedTask, currentRunID(execCtx)))
			}
		}
	}
	text := fmt.Sprintf("Worktree %s marked %s", worktree.WorktreePath, worktree.State)
	return ToolExecutionResult{
		Result: Result{Text: text, ModelText: text},
		Data: ExitWorktreeOutput{
			WorktreeID:   worktree.ID,
			WorktreePath: worktree.WorktreePath,
			State:        string(worktree.State),
		},
		PreviewText: text,
	}, nil
}

func (enterWorktreeTool) MapModelResult(output ToolExecutionResult) ModelToolResult {
	return defaultStructuredModelResult(output)
}

func (exitWorktreeTool) MapModelResult(output ToolExecutionResult) ModelToolResult {
	return defaultStructuredModelResult(output)
}

func gitTopLevel(ctx context.Context, workspaceRoot string) (string, error) {
	out, err := runGit(ctx, "-C", workspaceRoot, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

func runGit(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = err.Error()
		}
		return "", fmt.Errorf("git %s: %s", strings.Join(args, " "), message)
	}
	return strings.TrimSpace(stdout.String()), nil
}

func sanitizeWorktreeSlug(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "worktree"
	}
	replacer := strings.NewReplacer("/", "-", "\\", "-", " ", "-", ":", "-", "@", "-")
	return replacer.Replace(value)
}
