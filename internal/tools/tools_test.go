package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"go-agent/internal/permissions"
	"go-agent/internal/task"
)

type countingTool struct {
	definition Definition
	calls      int
}

type testNotebookFile struct {
	Cells         []testNotebookCell `json:"cells"`
	Metadata      map[string]any     `json:"metadata"`
	NBFormat      int                `json:"nbformat"`
	NBFormatMinor int                `json:"nbformat_minor"`
}

type testNotebookCell struct {
	CellType       string         `json:"cell_type"`
	ID             string         `json:"id"`
	Source         any            `json:"source"`
	Metadata       map[string]any `json:"metadata"`
	ExecutionCount any            `json:"execution_count"`
	Outputs        []any          `json:"outputs"`
}

func (t *countingTool) Definition() Definition {
	t.calls++
	return t.definition
}

func (t *countingTool) IsConcurrencySafe() bool { return true }

func (t *countingTool) Execute(context.Context, Call, ExecContext) (Result, error) {
	return Result{}, nil
}

func readNotebookFixture(t *testing.T, path string) testNotebookFile {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	var notebook testNotebookFile
	if err := json.Unmarshal(data, &notebook); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	return notebook
}

func TestFileReadToolRespectsWorkspaceBoundary(t *testing.T) {
	root := t.TempDir()
	allowed := filepath.Join(root, "allowed.txt")
	if err := os.WriteFile(allowed, []byte("hello"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	registry := NewRegistry()
	result, err := registry.Execute(context.Background(), Call{
		Name:  "file_read",
		Input: map[string]any{"path": allowed},
	}, ExecContext{
		WorkspaceRoot:    root,
		PermissionEngine: permissions.NewEngine(),
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Text != "hello" {
		t.Fatalf("result.Text = %q, want %q", result.Text, "hello")
	}
}

func TestRegistryDefinitionsExposePhase2ToolSchemas(t *testing.T) {
	registry := NewRegistry()

	defs := registry.Definitions()
	if len(defs) != 14 {
		t.Fatalf("len(Definitions) = %d, want 14", len(defs))
	}

	gotNames := make([]string, 0, len(defs))
	for _, def := range defs {
		gotNames = append(gotNames, def.Name)
		if def.Description == "" {
			t.Fatalf("definition %q missing description", def.Name)
		}
		if def.InputSchema == nil {
			t.Fatalf("definition %q missing schema", def.Name)
		}
	}
	wantNames := []string{
		"file_edit", "file_read", "file_write", "glob", "grep",
		"notebook_edit", "shell_command",
		"task_create", "task_get", "task_list", "task_output", "task_stop", "task_update",
		"todo_write",
	}
	if !reflect.DeepEqual(gotNames, wantNames) {
		t.Fatalf("Definitions() names = %v, want %v", gotNames, wantNames)
	}

	requireSchemaFields := func(name string, required []string, props ...string) {
		t.Helper()
		for _, def := range defs {
			if def.Name != name {
				continue
			}

			gotRequired, _ := def.InputSchema["required"].([]string)
			if !reflect.DeepEqual(gotRequired, required) {
				t.Fatalf("definition %q required = %v, want %v", name, gotRequired, required)
			}

			properties, _ := def.InputSchema["properties"].(map[string]any)
			for _, prop := range props {
				if _, ok := properties[prop]; !ok {
					t.Fatalf("definition %q missing property %q", name, prop)
				}
			}
			return
		}
		t.Fatalf("missing definition %q", name)
	}

	requireSchemaFields("file_read", []string{"path"}, "path")
	requireSchemaFields("file_write", []string{"path", "content"}, "path", "content")
	requireSchemaFields("file_edit", []string{"file_path", "old_string", "new_string"}, "file_path", "old_string", "new_string", "replace_all")
	requireSchemaFields("glob", []string{"pattern"}, "pattern")
	requireSchemaFields("grep", []string{"path", "pattern"}, "path", "pattern")
	requireSchemaFields("notebook_edit", []string{"notebook_path", "new_source"}, "notebook_path", "cell_id", "new_source", "cell_type", "edit_mode")
	requireSchemaFields("shell_command", []string{"command"}, "command")
	requireSchemaFields("todo_write", []string{"todos"}, "todos")
	requireSchemaFields("task_create", []string{"type", "command"}, "type", "command", "description")
	requireSchemaFields("task_get", []string{"task_id"}, "task_id")
	requireSchemaFields("task_list", []string{}, "status")
	requireSchemaFields("task_output", []string{"task_id"}, "task_id")
	requireSchemaFields("task_stop", []string{"task_id"}, "task_id")
	requireSchemaFields("task_update", []string{"task_id"}, "task_id", "status", "description")
}

func TestTodoWriteToolPersistsWorkspaceTodos(t *testing.T) {
	root := t.TempDir()
	manager := task.NewManager(task.Config{MaxConcurrentTasks: 8, TaskOutputMaxBytes: 1 << 20}, nil, nil)

	registry := NewRegistry()
	_, err := registry.Execute(context.Background(), Call{
		Name: "todo_write",
		Input: map[string]any{
			"todos": []any{
				map[string]any{"content": "write tests", "status": "pending", "activeForm": "Writing tests"},
			},
		},
	}, ExecContext{
		WorkspaceRoot:    root,
		PermissionEngine: permissions.NewEngine("trusted_local"),
		TaskManager:      manager,
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	data, err := os.ReadFile(filepath.Join(root, ".claude", "todos.json"))
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if !strings.Contains(string(data), "write tests") {
		t.Fatalf("todos.json = %q, want persisted todo", string(data))
	}
}

func TestTaskToolsDriveTaskLifecycle(t *testing.T) {
	root := t.TempDir()
	manager := task.NewManager(task.Config{MaxConcurrentTasks: 8, TaskOutputMaxBytes: 1 << 20}, nil, nil)
	registry := NewRegistry()
	execCtx := ExecContext{
		WorkspaceRoot:    root,
		PermissionEngine: permissions.NewEngine("trusted_local"),
		TaskManager:      manager,
	}

	created, err := registry.Execute(context.Background(), Call{
		Name: "task_create",
		Input: map[string]any{
			"type":        "shell",
			"command":     "echo tool-task",
			"description": "tool lifecycle",
		},
	}, execCtx)
	if err != nil {
		t.Fatalf("task_create error = %v", err)
	}
	if created.Text == "" {
		t.Fatal("task_create returned empty task id")
	}

	waitForToolTaskTerminal(t, manager, created.Text, root)

	listed, err := registry.Execute(context.Background(), Call{Name: "task_list", Input: map[string]any{}}, execCtx)
	if err != nil {
		t.Fatalf("task_list error = %v", err)
	}
	if !strings.Contains(listed.Text, created.Text) {
		t.Fatalf("task_list text = %q, want task id %q", listed.Text, created.Text)
	}

	got, err := registry.Execute(context.Background(), Call{
		Name:  "task_get",
		Input: map[string]any{"task_id": created.Text},
	}, execCtx)
	if err != nil {
		t.Fatalf("task_get error = %v", err)
	}
	if !strings.Contains(got.Text, "tool lifecycle") {
		t.Fatalf("task_get text = %q, want description", got.Text)
	}

	output, err := registry.Execute(context.Background(), Call{
		Name:  "task_output",
		Input: map[string]any{"task_id": created.Text},
	}, execCtx)
	if err != nil {
		t.Fatalf("task_output error = %v", err)
	}
	if !strings.Contains(output.Text, "tool-task") {
		t.Fatalf("task_output text = %q, want shell output", output.Text)
	}
}

func TestTaskToolsRequireTrustedLocalAndManager(t *testing.T) {
	root := t.TempDir()
	registry := NewRegistry()

	_, err := registry.Execute(context.Background(), Call{
		Name:  "task_list",
		Input: map[string]any{},
	}, ExecContext{
		WorkspaceRoot:    root,
		PermissionEngine: permissions.NewEngine("workspace_write"),
	})
	if err == nil || !strings.Contains(err.Error(), "denied") {
		t.Fatalf("task_list error = %v, want denied", err)
	}
}

func waitForToolTaskTerminal(t *testing.T, manager *task.Manager, taskID, workspaceRoot string) {
	t.Helper()

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		got, ok, err := manager.Get(taskID, workspaceRoot)
		if err != nil {
			t.Fatalf("Get() error = %v", err)
		}
		if ok && (got.Status == task.TaskStatusCompleted || got.Status == task.TaskStatusFailed || got.Status == task.TaskStatusStopped) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}

	t.Fatalf("task %q did not reach terminal state", taskID)
}

func TestRegistryDefinitionsUseFrozenSnapshots(t *testing.T) {
	tool := &countingTool{definition: Definition{
		Name:        "snapshot_tool",
		Description: "snapshot test tool",
		InputSchema: objectSchema(map[string]any{
			"value": map[string]any{"type": "string"},
		}, "value"),
	}}

	registry := &Registry{tools: make(map[string]Tool)}
	registry.Register(tool)
	if tool.calls != 1 {
		t.Fatalf("Definition() calls after Register = %d, want 1", tool.calls)
	}

	tool.definition.InputSchema["properties"].(map[string]any)["value"].(map[string]any)["type"] = "number"

	defs := registry.Definitions()
	if tool.calls != 1 {
		t.Fatalf("Definition() calls after Definitions() = %d, want 1", tool.calls)
	}
	if len(defs) != 1 || defs[0].Name != "snapshot_tool" {
		t.Fatalf("Definitions() = %+v, want frozen snapshot", defs)
	}
	if got := defs[0].InputSchema["properties"].(map[string]any)["value"].(map[string]any)["type"]; got != "string" {
		t.Fatalf("registry definition mutated from original tool change: got %v, want string", got)
	}

	defs[0].InputSchema["properties"].(map[string]any)["value"].(map[string]any)["type"] = "boolean"

	defsAgain := registry.Definitions()
	if tool.calls != 1 {
		t.Fatalf("Definition() calls after repeated Definitions() = %d, want 1", tool.calls)
	}
	if got := defsAgain[0].InputSchema["properties"].(map[string]any)["value"].(map[string]any)["type"]; got != "string" {
		t.Fatalf("registry definition mutated from returned Definitions() change: got %v, want string", got)
	}
}

func TestGlobToolRejectsEscapeOutsideWorkspace(t *testing.T) {
	root := t.TempDir()
	parent := filepath.Dir(root)
	outside := filepath.Join(parent, "outside-glob-escape.txt")
	if err := os.WriteFile(outside, []byte("outside"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	t.Cleanup(func() {
		_ = os.Remove(outside)
	})

	registry := NewRegistry()
	_, err := registry.Execute(context.Background(), Call{
		Name:  "glob",
		Input: map[string]any{"pattern": filepath.Join("..", filepath.Base(outside))},
	}, ExecContext{
		WorkspaceRoot:    root,
		PermissionEngine: permissions.NewEngine(),
	})
	if err == nil {
		t.Fatalf("Execute() error = nil, want escape rejection")
	}
}

func TestPermissionProfilesControlLocalTools(t *testing.T) {
	root := t.TempDir()
	registry := NewRegistry()

	t.Run("default profile denies write and shell", func(t *testing.T) {
		_, err := registry.Execute(context.Background(), Call{
			Name:  "file_write",
			Input: map[string]any{"path": filepath.Join(root, "blocked.txt"), "content": "blocked"},
		}, ExecContext{
			WorkspaceRoot:    root,
			PermissionEngine: permissions.NewEngine(),
		})
		if err == nil || !strings.Contains(err.Error(), "denied") {
			t.Fatalf("file_write error = %v, want denied", err)
		}

		_, err = registry.Execute(context.Background(), Call{
			Name: "file_edit",
			Input: map[string]any{
				"file_path":  filepath.Join(root, "blocked.txt"),
				"old_string": "hello",
				"new_string": "bye",
			},
		}, ExecContext{
			WorkspaceRoot:    root,
			PermissionEngine: permissions.NewEngine(),
		})
		if err == nil || !strings.Contains(err.Error(), "denied") {
			t.Fatalf("file_edit error = %v, want denied", err)
		}

		notebookPath := filepath.Join(root, "blocked.ipynb")
		if err := os.WriteFile(notebookPath, []byte(`{"cells":[],"metadata":{},"nbformat":4,"nbformat_minor":5}`), 0o644); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}
		_, err = registry.Execute(context.Background(), Call{
			Name: "notebook_edit",
			Input: map[string]any{
				"notebook_path": notebookPath,
				"new_source":    "print('x')\n",
				"cell_type":     "code",
				"edit_mode":     "insert",
			},
		}, ExecContext{
			WorkspaceRoot:    root,
			PermissionEngine: permissions.NewEngine(),
		})
		if err == nil || !strings.Contains(err.Error(), "denied") {
			t.Fatalf("notebook_edit error = %v, want denied", err)
		}

		_, err = registry.Execute(context.Background(), Call{
			Name:  "shell_command",
			Input: map[string]any{"command": "echo blocked"},
		}, ExecContext{
			WorkspaceRoot:    root,
			PermissionEngine: permissions.NewEngine(),
		})
		if err == nil || !strings.Contains(err.Error(), "denied") {
			t.Fatalf("shell_command error = %v, want denied", err)
		}
	})

	t.Run("trusted_local allows write and shell", func(t *testing.T) {
		writePath := filepath.Join(root, "allowed.txt")
		result, err := registry.Execute(context.Background(), Call{
			Name:  "file_write",
			Input: map[string]any{"path": writePath, "content": "hello"},
		}, ExecContext{
			WorkspaceRoot:    root,
			PermissionEngine: permissions.NewEngine("trusted_local"),
		})
		if err != nil {
			t.Fatalf("file_write error = %v", err)
		}
		if result.Text != writePath {
			t.Fatalf("file_write result.Text = %q, want %q", result.Text, writePath)
		}
		data, err := os.ReadFile(writePath)
		if err != nil {
			t.Fatalf("ReadFile() error = %v", err)
		}
		if string(data) != "hello" {
			t.Fatalf("written file = %q, want %q", string(data), "hello")
		}

		editResult, err := registry.Execute(context.Background(), Call{
			Name: "file_edit",
			Input: map[string]any{
				"file_path":  writePath,
				"old_string": "hello",
				"new_string": "world",
			},
		}, ExecContext{
			WorkspaceRoot:    root,
			PermissionEngine: permissions.NewEngine("trusted_local"),
		})
		if err != nil {
			t.Fatalf("file_edit error = %v", err)
		}
		if editResult.Text != writePath {
			t.Fatalf("file_edit result.Text = %q, want %q", editResult.Text, writePath)
		}
		data, err = os.ReadFile(writePath)
		if err != nil {
			t.Fatalf("ReadFile() error = %v", err)
		}
		if string(data) != "world" {
			t.Fatalf("edited file = %q, want %q", string(data), "world")
		}

		notebookPath := filepath.Join(root, "allowed.ipynb")
		if err := os.WriteFile(notebookPath, []byte(`{"cells":[],"metadata":{"language_info":{"name":"python"}},"nbformat":4,"nbformat_minor":5}`), 0o600); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}
		notebookResult, err := registry.Execute(context.Background(), Call{
			Name: "notebook_edit",
			Input: map[string]any{
				"notebook_path": notebookPath,
				"new_source":    "print('ok')\n",
				"cell_type":     "code",
				"edit_mode":     "insert",
			},
		}, ExecContext{
			WorkspaceRoot:    root,
			PermissionEngine: permissions.NewEngine("trusted_local"),
		})
		if err != nil {
			t.Fatalf("notebook_edit error = %v", err)
		}
		if notebookResult.Text != notebookPath {
			t.Fatalf("notebook_edit result.Text = %q, want %q", notebookResult.Text, notebookPath)
		}
		notebook := readNotebookFixture(t, notebookPath)
		if len(notebook.Cells) != 1 {
			t.Fatalf("len(notebook.Cells) = %d, want 1", len(notebook.Cells))
		}

		shellResult, err := registry.Execute(context.Background(), Call{
			Name:  "shell_command",
			Input: map[string]any{"command": "echo ok"},
		}, ExecContext{
			WorkspaceRoot:    root,
			PermissionEngine: permissions.NewEngine("trusted_local"),
		})
		if err != nil {
			t.Fatalf("shell_command error = %v", err)
		}
		if !strings.Contains(shellResult.Text, "ok") {
			t.Fatalf("shell_command result.Text = %q, want output", shellResult.Text)
		}
	})
}

func TestFileEditToolReplacesSingleUniqueMatchAndPreservesMode(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "note.txt")
	if err := os.WriteFile(path, []byte("hello world"), 0o640); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	beforeInfo, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}

	registry := NewRegistry()
	result, err := registry.Execute(context.Background(), Call{
		Name: "file_edit",
		Input: map[string]any{
			"file_path":  path,
			"old_string": "world",
			"new_string": "gophers",
		},
	}, ExecContext{
		WorkspaceRoot:    root,
		PermissionEngine: permissions.NewEngine("trusted_local"),
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Text != path {
		t.Fatalf("result.Text = %q, want %q", result.Text, path)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(data) != "hello gophers" {
		t.Fatalf("file contents = %q, want %q", string(data), "hello gophers")
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if info.Mode().Perm() != beforeInfo.Mode().Perm() {
		t.Fatalf("Mode().Perm() = %o, want unchanged %o", info.Mode().Perm(), beforeInfo.Mode().Perm())
	}
}

func TestFileEditToolRejectsAmbiguousMatchWithoutReplaceAll(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "note.txt")
	if err := os.WriteFile(path, []byte("alpha beta alpha"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	registry := NewRegistry()
	_, err := registry.Execute(context.Background(), Call{
		Name: "file_edit",
		Input: map[string]any{
			"file_path":  path,
			"old_string": "alpha",
			"new_string": "omega",
		},
	}, ExecContext{
		WorkspaceRoot:    root,
		PermissionEngine: permissions.NewEngine("trusted_local"),
	})
	if err == nil || !strings.Contains(err.Error(), "replace_all") {
		t.Fatalf("Execute() error = %v, want replace_all guidance", err)
	}
}

func TestFileEditToolReplaceAll(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "note.txt")
	if err := os.WriteFile(path, []byte("alpha beta alpha"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	registry := NewRegistry()
	_, err := registry.Execute(context.Background(), Call{
		Name: "file_edit",
		Input: map[string]any{
			"file_path":   path,
			"old_string":  "alpha",
			"new_string":  "omega",
			"replace_all": true,
		},
	}, ExecContext{
		WorkspaceRoot:    root,
		PermissionEngine: permissions.NewEngine("trusted_local"),
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(data) != "omega beta omega" {
		t.Fatalf("file contents = %q, want %q", string(data), "omega beta omega")
	}
}

func TestNotebookEditToolReplacesCellSourceAndClearsCodeOutputs(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "demo.ipynb")
	content := `{
  "cells": [
    {
      "cell_type": "code",
      "id": "cell-1",
      "source": ["print('hi')\n"],
      "metadata": {},
      "execution_count": 1,
      "outputs": [{"output_type":"stream","text":"hi\n"}]
    }
  ],
  "metadata": {"language_info":{"name":"python"},"kernelspec":{"name":"python3"}},
  "nbformat": 4,
  "nbformat_minor": 5
}`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	beforeInfo, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}

	registry := NewRegistry()
	result, err := registry.Execute(context.Background(), Call{
		Name: "notebook_edit",
		Input: map[string]any{
			"notebook_path": path,
			"cell_id":       "cell-1",
			"new_source":    "print('bye')\n",
			"edit_mode":     "replace",
		},
	}, ExecContext{
		WorkspaceRoot:    root,
		PermissionEngine: permissions.NewEngine("trusted_local"),
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Text != path {
		t.Fatalf("result.Text = %q, want %q", result.Text, path)
	}

	notebook := readNotebookFixture(t, path)
	if len(notebook.Cells) != 1 {
		t.Fatalf("len(notebook.Cells) = %d, want 1", len(notebook.Cells))
	}
	if notebook.Cells[0].Source != "print('bye')\n" {
		t.Fatalf("cell source = %#v, want %q", notebook.Cells[0].Source, "print('bye')\n")
	}
	if notebook.Cells[0].ExecutionCount != nil {
		t.Fatalf("ExecutionCount = %#v, want nil", notebook.Cells[0].ExecutionCount)
	}
	if len(notebook.Cells[0].Outputs) != 0 {
		t.Fatalf("len(Outputs) = %d, want 0", len(notebook.Cells[0].Outputs))
	}
	if notebook.Metadata["kernelspec"].(map[string]any)["name"] != "python3" {
		t.Fatalf("kernelspec.name = %#v, want %q", notebook.Metadata["kernelspec"], "python3")
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if info.Mode().Perm() != beforeInfo.Mode().Perm() {
		t.Fatalf("Mode().Perm() = %o, want unchanged %o", info.Mode().Perm(), beforeInfo.Mode().Perm())
	}
}

func TestNotebookEditToolInsertsAndDeletesCells(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "demo.ipynb")
	content := `{
  "cells": [
    {"cell_type":"markdown","id":"cell-1","source":"# Heading\n","metadata":{}},
    {"cell_type":"code","id":"cell-2","source":"print('hi')\n","metadata":{},"execution_count":null,"outputs":[]}
  ],
  "metadata": {"language_info":{"name":"python"}},
  "nbformat": 4,
  "nbformat_minor": 5
}`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	registry := NewRegistry()
	_, err := registry.Execute(context.Background(), Call{
		Name: "notebook_edit",
		Input: map[string]any{
			"notebook_path": path,
			"cell_id":       "cell-1",
			"new_source":    "## Notes\n",
			"cell_type":     "markdown",
			"edit_mode":     "insert",
		},
	}, ExecContext{
		WorkspaceRoot:    root,
		PermissionEngine: permissions.NewEngine("trusted_local"),
	})
	if err != nil {
		t.Fatalf("insert Execute() error = %v", err)
	}

	notebook := readNotebookFixture(t, path)
	if len(notebook.Cells) != 3 {
		t.Fatalf("len(notebook.Cells) = %d, want 3", len(notebook.Cells))
	}
	inserted := notebook.Cells[1]
	if inserted.CellType != "markdown" {
		t.Fatalf("inserted.CellType = %q, want %q", inserted.CellType, "markdown")
	}
	if inserted.Source != "## Notes\n" {
		t.Fatalf("inserted.Source = %#v, want %q", inserted.Source, "## Notes\n")
	}
	if inserted.ID == "" {
		t.Fatal("inserted.ID = empty, want generated id")
	}

	_, err = registry.Execute(context.Background(), Call{
		Name: "notebook_edit",
		Input: map[string]any{
			"notebook_path": path,
			"cell_id":       inserted.ID,
			"new_source":    "",
			"edit_mode":     "delete",
		},
	}, ExecContext{
		WorkspaceRoot:    root,
		PermissionEngine: permissions.NewEngine("trusted_local"),
	})
	if err != nil {
		t.Fatalf("delete Execute() error = %v", err)
	}

	notebook = readNotebookFixture(t, path)
	if len(notebook.Cells) != 2 {
		t.Fatalf("len(notebook.Cells) after delete = %d, want 2", len(notebook.Cells))
	}
	if notebook.Cells[0].ID != "cell-1" || notebook.Cells[1].ID != "cell-2" {
		t.Fatalf("cell order after delete = [%q %q], want [cell-1 cell-2]", notebook.Cells[0].ID, notebook.Cells[1].ID)
	}
}

func TestShellCommandTruncatesOutputAndUsesWorkspaceDir(t *testing.T) {
	workspace := t.TempDir()
	registry := NewRegistry()

	result, err := registry.Execute(context.Background(), Call{
		Name:  "shell_command",
		Input: map[string]any{"command": "echo %cd% && for /L %i in (1,1,200) do @echo x"},
	}, ExecContext{
		WorkspaceRoot:    workspace,
		PermissionEngine: permissions.NewEngine("trusted_local"),
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if len(result.Text) != shellCommandMaxOutputBytes {
		t.Fatalf("len(result.Text) = %d, want %d", len(result.Text), shellCommandMaxOutputBytes)
	}
	if !strings.Contains(result.Text, workspace) {
		t.Fatalf("result.Text = %q, want workspace root %q", result.Text, workspace)
	}
}

func TestFileWriteRejectsContentAboveLimit(t *testing.T) {
	root := t.TempDir()
	registry := NewRegistry()
	content := strings.Repeat("x", fileWriteMaxBytes+1)

	_, err := registry.Execute(context.Background(), Call{
		Name:  "file_write",
		Input: map[string]any{"path": filepath.Join(root, "too-big.txt"), "content": content},
	}, ExecContext{
		WorkspaceRoot:    root,
		PermissionEngine: permissions.NewEngine("trusted_local"),
	})
	if err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("Execute() error = %v, want size limit error", err)
	}
}
