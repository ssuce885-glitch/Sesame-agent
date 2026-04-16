package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"go-agent/internal/config"
	"go-agent/internal/task"
)

type todoWriteTool struct{}

type TodoWriteInput struct {
	Todos []task.TodoItem `json:"todos"`
}

type TodoWriteOutput struct {
	Path     string          `json:"path"`
	OldTodos []task.TodoItem `json:"old_todos"`
	NewTodos []task.TodoItem `json:"new_todos"`
	Count    int             `json:"count"`
}

func (todoWriteTool) IsEnabled(execCtx ExecContext) bool {
	return execCtx.TaskManager != nil
}

func (todoWriteTool) Definition() Definition {
	return Definition{
		Name:        "todo_write",
		Description: "Persist workspace todo items.",
		InputSchema: objectSchema(map[string]any{
			"todos": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type":                 "object",
					"additionalProperties": false,
					"properties": map[string]any{
						"content":    map[string]any{"type": "string"},
						"status":     map[string]any{"type": "string", "enum": []string{"pending", "in_progress", "completed"}},
						"activeForm": map[string]any{"type": "string"},
					},
					"required": []string{"content", "status"},
				},
			},
		}, "todos"),
		OutputSchema: objectSchema(map[string]any{
			"path": map[string]any{"type": "string"},
			"old_todos": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type":                 "object",
					"additionalProperties": true,
				},
			},
			"new_todos": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type":                 "object",
					"additionalProperties": true,
				},
			},
			"count": map[string]any{"type": "integer"},
		}, "path", "old_todos", "new_todos", "count"),
	}
}

func (todoWriteTool) IsConcurrencySafe() bool { return false }

func (todoWriteTool) Decode(call Call) (DecodedCall, error) {
	todos, err := decodeTodoItems(call.Input["todos"])
	if err != nil {
		return DecodedCall{}, err
	}

	normalized := Call{
		Name: call.Name,
		Input: map[string]any{
			"todos": encodeTodoItems(todos),
		},
	}
	return DecodedCall{Call: normalized, Input: TodoWriteInput{Todos: todos}}, nil
}

func (t todoWriteTool) Execute(ctx context.Context, call Call, execCtx ExecContext) (Result, error) {
	decoded, err := t.Decode(call)
	if err != nil {
		return Result{}, err
	}
	output, err := t.ExecuteDecoded(ctx, decoded, execCtx)
	return output.Result, err
}

func (todoWriteTool) ExecuteDecoded(_ context.Context, decoded DecodedCall, execCtx ExecContext) (ToolExecutionResult, error) {
	manager, err := requireTaskManager(execCtx)
	if err != nil {
		return ToolExecutionResult{}, err
	}

	input, _ := decoded.Input.(TodoWriteInput)
	path := filepath.Join(execCtx.WorkspaceRoot, config.DirName, "todos.json")
	oldTodos, err := readExistingTodos(path)
	if err != nil {
		return ToolExecutionResult{}, err
	}
	if err := manager.WriteTodos(execCtx.WorkspaceRoot, input.Todos); err != nil {
		return ToolExecutionResult{}, err
	}

	modelText := fmt.Sprintf("Updated todo list with %d item(s).", len(input.Todos))
	if len(input.Todos) == 0 {
		modelText = "Cleared the workspace todo list."
	}
	return ToolExecutionResult{
		Result: Result{
			Text:      path,
			ModelText: modelText,
		},
		Data: TodoWriteOutput{
			Path:     path,
			OldTodos: oldTodos,
			NewTodos: input.Todos,
			Count:    len(input.Todos),
		},
		PreviewText: modelText,
		Metadata: map[string]any{
			"count": len(input.Todos),
		},
	}, nil
}

func (todoWriteTool) MapModelResult(output ToolExecutionResult) ModelToolResult {
	return defaultStructuredModelResult(output)
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

func encodeTodoItems(todos []task.TodoItem) []any {
	encoded := make([]any, 0, len(todos))
	for _, todo := range todos {
		encoded = append(encoded, map[string]any{
			"content":    todo.Content,
			"status":     todo.Status,
			"activeForm": todo.ActiveForm,
		})
	}
	return encoded
}

func readExistingTodos(path string) ([]task.TodoItem, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var todos []task.TodoItem
	if err := json.Unmarshal(data, &todos); err != nil {
		return nil, err
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
