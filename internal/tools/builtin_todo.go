package tools

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"go-agent/internal/task"
)

type todoWriteTool struct{}

func (todoWriteTool) Definition() Definition {
	return Definition{
		Name:        "todo_write",
		Description: "Persist workspace todo items.",
		InputSchema: objectSchema(map[string]any{
			"todos": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"content":    map[string]any{"type": "string"},
						"status":     map[string]any{"type": "string"},
						"activeForm": map[string]any{"type": "string"},
					},
				},
			},
		}, "todos"),
	}
}

func (todoWriteTool) IsConcurrencySafe() bool { return false }

func (todoWriteTool) Execute(_ context.Context, call Call, execCtx ExecContext) (Result, error) {
	manager, err := requireTaskManager(execCtx)
	if err != nil {
		return Result{}, err
	}

	todos, err := decodeTodoItems(call.Input["todos"])
	if err != nil {
		return Result{}, err
	}
	if err := manager.WriteTodos(execCtx.WorkspaceRoot, todos); err != nil {
		return Result{}, err
	}

	return Result{Text: filepath.Join(execCtx.WorkspaceRoot, ".claude", "todos.json")}, nil
}

func decodeTodoItems(raw any) ([]task.TodoItem, error) {
	items, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("todos must be an array")
	}

	todos := make([]task.TodoItem, 0, len(items))
	for _, item := range items {
		mapped, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("todo items must be objects")
		}

		todo := task.TodoItem{
			Content:    strings.TrimSpace(asString(mapped["content"])),
			Status:     strings.TrimSpace(asString(mapped["status"])),
			ActiveForm: strings.TrimSpace(asString(mapped["activeForm"])),
		}
		if todo.Content == "" {
			return nil, fmt.Errorf("todo content is required")
		}
		switch todo.Status {
		case "pending", "in_progress", "completed":
		default:
			return nil, fmt.Errorf("invalid todo status %q", todo.Status)
		}
		todos = append(todos, todo)
	}

	return todos, nil
}

func asString(v any) string {
	s, _ := v.(string)
	return s
}

func requireTaskManager(execCtx ExecContext) (*task.Manager, error) {
	if execCtx.TaskManager == nil {
		return nil, fmt.Errorf("task manager is not configured")
	}
	return execCtx.TaskManager, nil
}
