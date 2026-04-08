package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"go-agent/internal/permissions"
	"go-agent/internal/runtimegraph"
	"go-agent/internal/store/sqlite"
	"go-agent/internal/task"
	"go-agent/internal/types"
)

type countingTool struct {
	definition Definition
	calls      int
}

type aliasEnabledTool struct {
	definition Definition
	enabled    bool
}

type sleepTool struct {
	name            string
	delay           time.Duration
	concurrencySafe bool
}

type interruptibleTool struct {
	name  string
	delay time.Duration
	fail  bool
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

func (t aliasEnabledTool) Definition() Definition {
	return t.definition
}

func (t aliasEnabledTool) IsConcurrencySafe() bool { return true }

func (t aliasEnabledTool) Execute(_ context.Context, call Call, _ ExecContext) (Result, error) {
	return Result{Text: call.Name}, nil
}

func (t aliasEnabledTool) IsEnabled(ExecContext) bool {
	return t.enabled
}

func (t sleepTool) Definition() Definition {
	return Definition{
		Name:        t.name,
		Description: "sleep test tool",
		InputSchema: objectSchema(map[string]any{}),
	}
}

func (t sleepTool) IsConcurrencySafe() bool { return t.concurrencySafe }

func (t sleepTool) Execute(_ context.Context, _ Call, _ ExecContext) (Result, error) {
	time.Sleep(t.delay)
	return Result{Text: t.name}, nil
}

func (t interruptibleTool) Definition() Definition {
	return Definition{
		Name:        t.name,
		Description: "interruptible test tool",
		InputSchema: objectSchema(map[string]any{}),
	}
}

func (t interruptibleTool) IsConcurrencySafe() bool { return true }

func (t interruptibleTool) Execute(ctx context.Context, _ Call, _ ExecContext) (Result, error) {
	select {
	case <-ctx.Done():
		return Result{}, ctx.Err()
	case <-time.After(t.delay):
		if t.fail {
			return Result{}, fmt.Errorf("%s failed", t.name)
		}
		return Result{Text: t.name}, nil
	}
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
	if result.ModelText != "hello" {
		t.Fatalf("result.ModelText = %q, want %q", result.ModelText, "hello")
	}
}

func TestFileReadToolReturnsUnchangedStubForRepeatedReadInSameTurn(t *testing.T) {
	root := t.TempDir()
	allowed := filepath.Join(root, "allowed.txt")
	if err := os.WriteFile(allowed, []byte("hello"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	registry := NewRegistry()
	turnCtx := &runtimegraph.TurnContext{}
	execCtx := ExecContext{
		WorkspaceRoot:    root,
		PermissionEngine: permissions.NewEngine(),
		TurnContext:      turnCtx,
	}

	first, err := registry.Execute(context.Background(), Call{
		Name:  "file_read",
		Input: map[string]any{"path": "allowed.txt"},
	}, execCtx)
	if err != nil {
		t.Fatalf("first Execute() error = %v", err)
	}
	if first.Text != "hello" {
		t.Fatalf("first result.Text = %q, want %q", first.Text, "hello")
	}

	second, err := registry.Execute(context.Background(), Call{
		Name:  "file_read",
		Input: map[string]any{"path": allowed},
	}, execCtx)
	if err != nil {
		t.Fatalf("second Execute() error = %v", err)
	}
	if second.Text != fileReadUnchangedStub {
		t.Fatalf("second result.Text = %q, want unchanged stub", second.Text)
	}
	if second.ModelText != fileReadUnchangedStub {
		t.Fatalf("second result.ModelText = %q, want unchanged stub", second.ModelText)
	}
}

func TestRegistryDefinitionsExposePhase3PlanModeSchemas(t *testing.T) {
	registry := NewRegistry()

	defs := registry.Definitions()
	if len(defs) != 16 {
		t.Fatalf("len(Definitions) = %d, want 16", len(defs))
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
		if def.OutputSchema == nil {
			t.Fatalf("definition %q missing output schema", def.Name)
		}
	}
	wantNames := []string{
		"enter_plan_mode", "exit_plan_mode",
		"file_edit", "file_read", "file_write", "glob", "grep",
		"notebook_edit", "shell_command",
		"task_create", "task_get", "task_list", "task_output", "task_stop", "task_update",
		"todo_write",
	}
	if !reflect.DeepEqual(gotNames, wantNames) {
		t.Fatalf("Definitions() names = %v, want %v", gotNames, wantNames)
	}

	requireSchemaFields := func(name string, required []string, props ...string) Definition {
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
			return def
		}
		t.Fatalf("missing definition %q", name)
		return Definition{}
	}

	requireSchemaFields("enter_plan_mode", []string{"plan_file"}, "plan_file")
	exitDef := requireSchemaFields("exit_plan_mode", []string{}, "state")
	requireSchemaFields("file_read", []string{"path"}, "path")
	requireSchemaFields("file_write", []string{"path", "content"}, "path", "content")
	requireSchemaFields("file_edit", []string{"file_path", "old_string", "new_string"}, "file_path", "old_string", "new_string", "replace_all")
	requireSchemaFields("glob", []string{"pattern"}, "pattern")
	requireSchemaFields("grep", []string{"path", "pattern"}, "path", "pattern")
	requireSchemaFields("notebook_edit", []string{"notebook_path", "new_source"}, "notebook_path", "cell_id", "new_source", "cell_type", "edit_mode")
	requireSchemaFields("shell_command", []string{"command"}, "command", "timeout_seconds", "max_output_bytes")
	requireSchemaFields("todo_write", []string{"todos"}, "todos")
	requireSchemaFields("task_create", []string{"type", "command"}, "type", "command", "description")
	requireSchemaFields("task_get", []string{"task_id"}, "task_id")
	requireSchemaFields("task_list", []string{}, "status")
	requireSchemaFields("task_output", []string{"task_id"}, "task_id")
	requireSchemaFields("task_stop", []string{"task_id"}, "task_id")
	requireSchemaFields("task_update", []string{"task_id"}, "task_id", "status", "description")

	stateProp := exitDef.InputSchema["properties"].(map[string]any)["state"].(map[string]any)
	gotEnum, _ := stateProp["enum"].([]string)
	wantEnum := []string{"completed", "approved", "failed"}
	if !reflect.DeepEqual(gotEnum, wantEnum) {
		t.Fatalf("exit_plan_mode enum = %v, want %v", gotEnum, wantEnum)
	}

	requireOutputSchemaFields := func(name string, required []string, props ...string) {
		t.Helper()
		for _, def := range defs {
			if def.Name != name {
				continue
			}

			gotRequired, _ := def.OutputSchema["required"].([]string)
			if !reflect.DeepEqual(gotRequired, required) {
				t.Fatalf("definition %q output required = %v, want %v", name, gotRequired, required)
			}

			properties, _ := def.OutputSchema["properties"].(map[string]any)
			for _, prop := range props {
				if _, ok := properties[prop]; !ok {
					t.Fatalf("definition %q missing output property %q", name, prop)
				}
			}
			return
		}
		t.Fatalf("missing definition %q", name)
	}

	requireOutputSchemaFields("enter_plan_mode", []string{"plan_id", "run_id", "state", "plan_file"}, "plan_id", "run_id", "state", "plan_file")
	requireOutputSchemaFields("exit_plan_mode", []string{"plan_id", "state"}, "plan_id", "state")
	requireOutputSchemaFields("file_read", []string{"path", "content", "unchanged"}, "path", "content", "unchanged")
	requireOutputSchemaFields("file_write", []string{"path", "status", "bytes_written"}, "path", "status", "bytes_written")
	requireOutputSchemaFields("file_edit", []string{"file_path", "old_string", "new_string", "replace_all", "replaced_count"}, "file_path", "old_string", "new_string", "replace_all", "replaced_count")
	requireOutputSchemaFields("glob", []string{"pattern", "matches", "count"}, "pattern", "matches", "count")
	requireOutputSchemaFields("grep", []string{"path", "pattern", "matched", "match_count"}, "path", "pattern", "matched", "match_count")
	requireOutputSchemaFields("notebook_edit", []string{"notebook_path", "new_source", "cell_type", "edit_mode", "original_file", "updated_file"}, "notebook_path", "cell_id", "new_source", "cell_type", "language", "edit_mode", "original_file", "updated_file")
	requireOutputSchemaFields("shell_command", []string{"command", "output", "timeout_seconds", "max_output_bytes", "classification"}, "command", "output", "timeout_seconds", "max_output_bytes", "classification")
	requireOutputSchemaFields("todo_write", []string{"path", "old_todos", "new_todos", "count"}, "path", "old_todos", "new_todos", "count")
	requireOutputSchemaFields("task_create", []string{"task_id", "type", "command"}, "task_id", "type", "command", "description")
	requireOutputSchemaFields("task_get", []string{"task"}, "task")
	requireOutputSchemaFields("task_list", []string{"tasks"}, "tasks", "status_filter")
	requireOutputSchemaFields("task_output", []string{"task_id", "output"}, "task_id", "output")
	requireOutputSchemaFields("task_stop", []string{"task_id"}, "task_id")
	requireOutputSchemaFields("task_update", []string{"task"}, "task")
}

func TestRegistryVisibleDefinitionsRespectPermissions(t *testing.T) {
	registry := NewRegistry()
	defs := registry.VisibleDefinitions(ExecContext{
		PermissionEngine: permissions.NewEngine(),
	})

	gotNames := make([]string, 0, len(defs))
	for _, def := range defs {
		gotNames = append(gotNames, def.Name)
	}

	wantNames := []string{"file_read", "glob", "grep"}
	if !reflect.DeepEqual(gotNames, wantNames) {
		t.Fatalf("VisibleDefinitions() names = %v, want %v", gotNames, wantNames)
	}
}

func TestRegistryVisibleDefinitionsHideRuntimeDependentToolsWithoutContext(t *testing.T) {
	registry := NewRegistry()
	defs := registry.VisibleDefinitions(ExecContext{
		PermissionEngine: permissions.NewEngine("trusted_local"),
	})

	gotNames := make([]string, 0, len(defs))
	for _, def := range defs {
		gotNames = append(gotNames, def.Name)
	}

	wantNames := []string{
		"file_edit",
		"file_read",
		"file_write",
		"glob",
		"grep",
		"notebook_edit",
		"shell_command",
	}
	if !reflect.DeepEqual(gotNames, wantNames) {
		t.Fatalf("VisibleDefinitions(trusted_local) names = %v, want %v", gotNames, wantNames)
	}
}

func TestRegistryExecuteSupportsAliasesValidationAndEnablement(t *testing.T) {
	registry := &Registry{
		tools:       make(map[string]Tool),
		aliases:     make(map[string]string),
		definitions: make(map[string]Definition),
	}
	registry.Register(aliasEnabledTool{
		enabled: true,
		definition: Definition{
			Name:        "canonical_tool",
			Aliases:     []string{"legacy_tool"},
			Description: "alias test tool",
			InputSchema: objectSchema(map[string]any{
				"value": map[string]any{"type": "string"},
			}, "value"),
		},
	})
	registry.Register(aliasEnabledTool{
		enabled: false,
		definition: Definition{
			Name:        "disabled_tool",
			Description: "disabled test tool",
			InputSchema: objectSchema(map[string]any{}),
		},
	})

	result, err := registry.Execute(context.Background(), Call{
		Name:  "legacy_tool",
		Input: map[string]any{"value": "ok"},
	}, ExecContext{})
	if err != nil {
		t.Fatalf("Execute(alias) error = %v", err)
	}
	if result.Text != "canonical_tool" {
		t.Fatalf("Execute(alias) result.Text = %q, want canonical tool name", result.Text)
	}

	_, err = registry.Execute(context.Background(), Call{
		Name:  "legacy_tool",
		Input: map[string]any{"value": true},
	}, ExecContext{})
	if err == nil || !strings.Contains(err.Error(), "value must be a string") {
		t.Fatalf("Execute(validation) error = %v, want schema validation failure", err)
	}

	defs := registry.VisibleDefinitions(ExecContext{})
	for _, def := range defs {
		if def.Name == "disabled_tool" {
			t.Fatalf("VisibleDefinitions() unexpectedly included disabled_tool: %v", defs)
		}
	}
}

func TestPlanModeToolsRequireRuntimeServiceAndTurnContext(t *testing.T) {
	registry := NewRegistry()
	_, err := registry.Execute(context.Background(), Call{
		Name:  "enter_plan_mode",
		Input: map[string]any{"plan_file": "docs/superpowers/plans/demo.md"},
	}, ExecContext{
		WorkspaceRoot:    t.TempDir(),
		PermissionEngine: permissions.NewEngine("trusted_local"),
	})
	if err == nil || !strings.Contains(err.Error(), "runtime service is not configured") {
		t.Fatalf("enter_plan_mode error = %v, want runtime service is not configured", err)
	}

	store, err := sqlite.Open(filepath.Join(t.TempDir(), "agentd.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	_, err = registry.Execute(context.Background(), Call{
		Name:  "enter_plan_mode",
		Input: map[string]any{"plan_file": "docs/superpowers/plans/demo.md"},
	}, ExecContext{
		WorkspaceRoot:    t.TempDir(),
		PermissionEngine: permissions.NewEngine("trusted_local"),
		RuntimeService:   runtimegraph.NewService(store),
	})
	if err == nil || !strings.Contains(err.Error(), "turn runtime context is not configured") {
		t.Fatalf("enter_plan_mode missing turn context error = %v, want turn runtime context is not configured", err)
	}
}

func TestShellToolRequiresApprovalForDestructiveCommands(t *testing.T) {
	registry := NewRegistry()
	_, err := registry.Execute(context.Background(), Call{
		Name:  "shell_command",
		Input: map[string]any{"command": "del important.txt"},
	}, ExecContext{
		WorkspaceRoot:    t.TempDir(),
		PermissionEngine: permissions.NewEngine("trusted_local"),
	})
	if err == nil || !strings.Contains(err.Error(), "requires approval") {
		t.Fatalf("shell_command destructive error = %v, want approval requirement", err)
	}
}

func TestPlanModeToolsDriveLifecycle(t *testing.T) {
	store, err := sqlite.Open(filepath.Join(t.TempDir(), "agentd.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	service := runtimegraph.NewService(store)
	registry := NewRegistry()
	turnCtx := &runtimegraph.TurnContext{
		CurrentSessionID: "sess_plan_tools",
		CurrentTurnID:    "turn_plan_tools",
	}

	created, err := registry.Execute(context.Background(), Call{
		Name:  "enter_plan_mode",
		Input: map[string]any{"plan_file": "docs/superpowers/plans/phase3.md"},
	}, ExecContext{
		WorkspaceRoot:    t.TempDir(),
		PermissionEngine: permissions.NewEngine("trusted_local"),
		RuntimeService:   service,
		TurnContext:      turnCtx,
	})
	if err != nil {
		t.Fatalf("enter_plan_mode error = %v", err)
	}
	if !strings.Contains(created.Text, `"state": "active"`) {
		t.Fatalf("enter_plan_mode result = %q, want active payload", created.Text)
	}
	if turnCtx.CurrentRunID == "" {
		t.Fatal("TurnContext.CurrentRunID = empty, want lazy-created run id")
	}

	exited, err := registry.Execute(context.Background(), Call{
		Name:  "exit_plan_mode",
		Input: map[string]any{"state": "approved"},
	}, ExecContext{
		WorkspaceRoot:    t.TempDir(),
		PermissionEngine: permissions.NewEngine("trusted_local"),
		RuntimeService:   service,
		TurnContext:      turnCtx,
	})
	if err != nil {
		t.Fatalf("exit_plan_mode error = %v", err)
	}
	if !strings.Contains(exited.Text, `"state": "approved"`) {
		t.Fatalf("exit_plan_mode result = %q, want approved payload", exited.Text)
	}
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

func TestToolRuntimePersistsToolRunsWithRunContext(t *testing.T) {
	root := t.TempDir()
	readme := filepath.Join(root, "README.md")
	if err := os.WriteFile(readme, []byte("hello runtime"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	store, err := sqlite.Open(filepath.Join(t.TempDir(), "agentd.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	runtime := NewRuntime(NewRegistry(), store)
	result, execErr := runtime.Execute(context.Background(), Call{
		Name:  "file_read",
		Input: map[string]any{"path": "README.md"},
	}, ExecContext{
		WorkspaceRoot:    root,
		PermissionEngine: permissions.NewEngine(),
		TurnContext: &runtimegraph.TurnContext{
			CurrentRunID: "run_runtime_test",
		},
	})
	if execErr != nil {
		t.Fatalf("Runtime.Execute() error = %v", execErr)
	}
	if result.Text != "hello runtime" {
		t.Fatalf("Runtime.Execute() result.Text = %q, want %q", result.Text, "hello runtime")
	}

	graph, err := store.ListRuntimeGraph(context.Background())
	if err != nil {
		t.Fatalf("ListRuntimeGraph() error = %v", err)
	}
	if len(graph.ToolRuns) != 1 {
		t.Fatalf("len(graph.ToolRuns) = %d, want 1", len(graph.ToolRuns))
	}
	got := graph.ToolRuns[0]
	if got.RunID != "run_runtime_test" {
		t.Fatalf("tool run RunID = %q, want %q", got.RunID, "run_runtime_test")
	}
	if got.ToolName != "file_read" {
		t.Fatalf("tool run ToolName = %q, want %q", got.ToolName, "file_read")
	}
	if got.State != "completed" {
		t.Fatalf("tool run State = %q, want completed", got.State)
	}
	if !strings.Contains(got.OutputJSON, "hello runtime") {
		t.Fatalf("tool run OutputJSON = %q, want preview of result", got.OutputJSON)
	}
}

func TestToolRuntimeArtifactizesLargeResults(t *testing.T) {
	root := t.TempDir()
	largeContent := strings.Repeat("a", InlineResultLimit+256)
	path := filepath.Join(root, "large.txt")
	if err := os.WriteFile(path, []byte(largeContent), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	runtime := NewRuntime(NewRegistry(), nil)
	result, err := runtime.Execute(context.Background(), Call{
		Name:  "file_read",
		Input: map[string]any{"path": "large.txt"},
	}, ExecContext{
		WorkspaceRoot:    root,
		PermissionEngine: permissions.NewEngine(),
	})
	if err != nil {
		t.Fatalf("Runtime.Execute() error = %v", err)
	}
	if result.ArtifactPath == "" {
		t.Fatal("Runtime.Execute() ArtifactPath = empty, want persisted artifact")
	}
	if _, err := os.Stat(result.ArtifactPath); err != nil {
		t.Fatalf("os.Stat(ArtifactPath) error = %v", err)
	}
	if !strings.Contains(result.ModelText, ".runtime-data") {
		t.Fatalf("Runtime.Execute() ModelText = %q, want artifact summary", result.ModelText)
	}
}

func TestToolRuntimeExecuteRichReturnsStructuredOutput(t *testing.T) {
	root := t.TempDir()
	runtime := NewRuntime(NewRegistry(), nil)

	output, err := runtime.ExecuteRich(context.Background(), Call{
		Name:  "file_write",
		Input: map[string]any{"path": "structured.txt", "content": "hello"},
	}, ExecContext{
		WorkspaceRoot:    root,
		PermissionEngine: permissions.NewEngine("trusted_local"),
	})
	if err != nil {
		t.Fatalf("ExecuteRich() error = %v", err)
	}

	written, ok := output.Data.(FileWriteOutput)
	if !ok {
		t.Fatalf("output.Data = %#v, want FileWriteOutput", output.Data)
	}
	if written.Status != "created" {
		t.Fatalf("written.Status = %q, want created", written.Status)
	}
	modelResult := mapToolModelResult(fileWriteTool{}, output)
	if modelResult.Text == "" {
		t.Fatal("modelResult.Text = empty, want descriptive model text")
	}
	if modelResult.Structured == nil {
		t.Fatal("modelResult.Structured = nil, want structured payload")
	}
}

func TestSearchToolsExecuteRichReturnStructuredOutput(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "alpha.txt"), []byte("hello\nneedle here\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(alpha) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "beta.txt"), []byte("world\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(beta) error = %v", err)
	}

	runtime := NewRuntime(NewRegistry(), nil)
	execCtx := ExecContext{
		WorkspaceRoot:    root,
		PermissionEngine: permissions.NewEngine(),
	}

	globOutput, err := runtime.ExecuteRich(context.Background(), Call{
		Name:  "glob",
		Input: map[string]any{"pattern": "*.txt"},
	}, execCtx)
	if err != nil {
		t.Fatalf("glob ExecuteRich() error = %v", err)
	}
	globData, ok := globOutput.Data.(GlobOutput)
	if !ok {
		t.Fatalf("glob output.Data = %#v, want GlobOutput", globOutput.Data)
	}
	if globData.Count != 2 {
		t.Fatalf("glob Count = %d, want 2", globData.Count)
	}
	if len(globData.Matches) != 2 {
		t.Fatalf("len(glob Matches) = %d, want 2", len(globData.Matches))
	}

	grepOutput, err := runtime.ExecuteRich(context.Background(), Call{
		Name:  "grep",
		Input: map[string]any{"path": "alpha.txt", "pattern": "needle"},
	}, execCtx)
	if err != nil {
		t.Fatalf("grep ExecuteRich() error = %v", err)
	}
	grepData, ok := grepOutput.Data.(GrepOutput)
	if !ok {
		t.Fatalf("grep output.Data = %#v, want GrepOutput", grepOutput.Data)
	}
	if !grepData.Matched || grepData.MatchCount != 1 {
		t.Fatalf("grep output = %#v, want matched with single matching line", grepData)
	}
}

func TestTodoWriteToolExecuteRichReturnsStructuredOutput(t *testing.T) {
	root := t.TempDir()
	manager := task.NewManager(task.Config{MaxConcurrentTasks: 8, TaskOutputMaxBytes: 1 << 20}, nil, nil)
	runtime := NewRuntime(NewRegistry(), nil)

	output, err := runtime.ExecuteRich(context.Background(), Call{
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
		t.Fatalf("todo_write ExecuteRich() error = %v", err)
	}

	data, ok := output.Data.(TodoWriteOutput)
	if !ok {
		t.Fatalf("todo_write output.Data = %#v, want TodoWriteOutput", output.Data)
	}
	if data.Path == "" || data.Count != 1 {
		t.Fatalf("todo_write output = %#v, want persisted path and count", data)
	}
	if len(data.NewTodos) != 1 || data.NewTodos[0].Content != "write tests" {
		t.Fatalf("todo_write new todos = %#v, want persisted todo item", data.NewTodos)
	}
}

func TestNotebookEditToolExecuteRichReturnsStructuredOutput(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "demo.ipynb")
	content := `{
  "cells": [
    {"cell_type":"code","id":"cell-1","source":"print('hi')\n","metadata":{},"execution_count":1,"outputs":[{"output_type":"stream"}]}
  ],
  "metadata": {"language_info":{"name":"python"}},
  "nbformat": 4,
  "nbformat_minor": 5
}`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	runtime := NewRuntime(NewRegistry(), nil)
	output, err := runtime.ExecuteRich(context.Background(), Call{
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
		t.Fatalf("ExecuteRich() error = %v", err)
	}

	data, ok := output.Data.(NotebookEditOutput)
	if !ok {
		t.Fatalf("output.Data = %#v, want NotebookEditOutput", output.Data)
	}
	if data.CellID != "cell-1" || data.CellType != "code" || data.Language != "python" {
		t.Fatalf("notebook output = %#v, want cell-1/code/python", data)
	}
	if !strings.Contains(data.OriginalFile, "print('hi')") || !strings.Contains(data.UpdatedFile, "print('bye')") {
		t.Fatalf("notebook output files = %#v, want original and updated notebook snapshots", data)
	}
}

func TestPlanModeToolsExecuteRichReturnStructuredOutput(t *testing.T) {
	store, err := sqlite.Open(filepath.Join(t.TempDir(), "agentd.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	service := runtimegraph.NewService(store)
	turnCtx := &runtimegraph.TurnContext{
		CurrentSessionID: "sess_execute_rich_plan",
		CurrentTurnID:    "turn_execute_rich_plan",
	}
	runtime := NewRuntime(NewRegistry(), nil)
	execCtx := ExecContext{
		WorkspaceRoot:    t.TempDir(),
		PermissionEngine: permissions.NewEngine("trusted_local"),
		RuntimeService:   service,
		TurnContext:      turnCtx,
	}

	entered, err := runtime.ExecuteRich(context.Background(), Call{
		Name:  "enter_plan_mode",
		Input: map[string]any{"plan_file": "docs/superpowers/plans/demo.md"},
	}, execCtx)
	if err != nil {
		t.Fatalf("enter_plan_mode ExecuteRich() error = %v", err)
	}
	enterData, ok := entered.Data.(runtimegraph.EnterPlanModeOutput)
	if !ok {
		t.Fatalf("enter_plan_mode output.Data = %#v, want runtimegraph.EnterPlanModeOutput", entered.Data)
	}
	if enterData.PlanID == "" || enterData.RunID == "" || enterData.State != types.PlanStateActive {
		t.Fatalf("enter_plan_mode output = %#v, want populated active plan", enterData)
	}

	exited, err := runtime.ExecuteRich(context.Background(), Call{
		Name:  "exit_plan_mode",
		Input: map[string]any{"state": "approved"},
	}, execCtx)
	if err != nil {
		t.Fatalf("exit_plan_mode ExecuteRich() error = %v", err)
	}
	exitData, ok := exited.Data.(runtimegraph.ExitPlanModeOutput)
	if !ok {
		t.Fatalf("exit_plan_mode output.Data = %#v, want runtimegraph.ExitPlanModeOutput", exited.Data)
	}
	if exitData.PlanID == "" || exitData.State != types.PlanStateApproved {
		t.Fatalf("exit_plan_mode output = %#v, want approved plan result", exitData)
	}
}

func TestToolRuntimeExecuteCallsRunsParallelBatches(t *testing.T) {
	registry := &Registry{
		tools:       make(map[string]Tool),
		aliases:     make(map[string]string),
		definitions: make(map[string]Definition),
	}
	registry.Register(sleepTool{name: "sleep_a", delay: 200 * time.Millisecond, concurrencySafe: true})
	registry.Register(sleepTool{name: "sleep_b", delay: 200 * time.Millisecond, concurrencySafe: true})
	registry.Register(sleepTool{name: "sleep_c", delay: 50 * time.Millisecond, concurrencySafe: false})

	runtime := NewRuntime(registry, nil)
	batches := runtime.PlanBatches([]Call{
		{Name: "sleep_a", Input: map[string]any{}},
		{Name: "sleep_b", Input: map[string]any{}},
		{Name: "sleep_c", Input: map[string]any{}},
	}, ExecContext{})
	if len(batches) != 2 {
		t.Fatalf("len(PlanBatches) = %d, want 2", len(batches))
	}
	if !batches[0].Parallel || len(batches[0].Calls) != 2 {
		t.Fatalf("first batch = %#v, want 2-call parallel batch", batches[0])
	}
	if batches[1].Parallel || len(batches[1].Calls) != 1 || batches[1].Calls[0].Call.Name != "sleep_c" {
		t.Fatalf("second batch = %#v, want single serial sleep_c", batches[1])
	}

	start := time.Now()
	results, err := runtime.ExecuteCalls(context.Background(), []Call{
		{Name: "sleep_a", Input: map[string]any{}},
		{Name: "sleep_b", Input: map[string]any{}},
		{Name: "sleep_c", Input: map[string]any{}},
	}, ExecContext{})
	if err != nil {
		t.Fatalf("ExecuteCalls() error = %v", err)
	}
	elapsed := time.Since(start)

	if elapsed >= 430*time.Millisecond {
		t.Fatalf("ExecuteCalls() elapsed = %s, want parallel execution faster than serial total", elapsed)
	}
	if len(results) != 3 {
		t.Fatalf("len(ExecuteCalls results) = %d, want 3", len(results))
	}
	for i, want := range []string{"sleep_a", "sleep_b", "sleep_c"} {
		if results[i].Call.Name != want || results[i].Result.Text != want || results[i].Err != nil {
			t.Fatalf("results[%d] = %#v, want ordered success for %q", i, results[i], want)
		}
	}
}

func TestToolRuntimePlanBatchesUsesInputAwareShellConcurrency(t *testing.T) {
	runtime := NewRuntime(NewRegistry(), nil)
	batches := runtime.PlanBatches([]Call{
		{Name: "shell_command", Input: map[string]any{"command": "echo hello"}},
		{Name: "shell_command", Input: map[string]any{"command": "git status"}},
		{Name: "shell_command", Input: map[string]any{"command": "mkdir tmp-runtime"}},
	}, ExecContext{
		PermissionEngine: permissions.NewEngine("trusted_local"),
	})

	if len(batches) != 2 {
		t.Fatalf("len(PlanBatches shell) = %d, want 2", len(batches))
	}
	if !batches[0].Parallel || len(batches[0].Calls) != 2 {
		t.Fatalf("first shell batch = %#v, want 2-call parallel read-only batch", batches[0])
	}
	if batches[1].Parallel || len(batches[1].Calls) != 1 || batches[1].Calls[0].Call.Input["command"] != "mkdir tmp-runtime" {
		t.Fatalf("second shell batch = %#v, want mutating shell call isolated", batches[1])
	}
}

func TestToolRuntimeExecuteCallsCancelsSiblingParallelCallsOnError(t *testing.T) {
	registry := &Registry{
		tools:       make(map[string]Tool),
		aliases:     make(map[string]string),
		definitions: make(map[string]Definition),
	}
	registry.Register(interruptibleTool{name: "fail_fast", delay: 50 * time.Millisecond, fail: true})
	registry.Register(interruptibleTool{name: "wait_long", delay: time.Second})

	runtime := NewRuntime(registry, nil)
	start := time.Now()
	results, err := runtime.ExecuteCalls(context.Background(), []Call{
		{Name: "fail_fast", Input: map[string]any{}},
		{Name: "wait_long", Input: map[string]any{}},
	}, ExecContext{})
	if err != nil {
		t.Fatalf("ExecuteCalls() error = %v", err)
	}
	elapsed := time.Since(start)

	if elapsed >= 400*time.Millisecond {
		t.Fatalf("ExecuteCalls() elapsed = %s, want sibling cancellation after first error", elapsed)
	}
	if len(results) != 2 {
		t.Fatalf("len(ExecuteCalls results) = %d, want 2", len(results))
	}
	if results[0].Err == nil || !strings.Contains(results[0].Err.Error(), "fail_fast failed") {
		t.Fatalf("results[0].Err = %v, want fail_fast failure", results[0].Err)
	}
	if results[1].Err == nil || !strings.Contains(results[1].Err.Error(), "context canceled") {
		t.Fatalf("results[1].Err = %v, want canceled sibling error", results[1].Err)
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
		if !strings.Contains(result.Text, "wrote file") || !strings.Contains(result.Text, writePath) {
			t.Fatalf("file_write result.Text = %q, want clear success message containing %q", result.Text, writePath)
		}
		if result.ModelText != "File created successfully at: "+writePath {
			t.Fatalf("file_write result.ModelText = %q, want create success text", result.ModelText)
		}
		data, err := os.ReadFile(writePath)
		if err != nil {
			t.Fatalf("ReadFile() error = %v", err)
		}
		if string(data) != "hello" {
			t.Fatalf("written file = %q, want %q", string(data), "hello")
		}

		result, err = registry.Execute(context.Background(), Call{
			Name:  "file_write",
			Input: map[string]any{"path": writePath, "content": "hello"},
		}, ExecContext{
			WorkspaceRoot:    root,
			PermissionEngine: permissions.NewEngine("trusted_local"),
		})
		if err != nil {
			t.Fatalf("second file_write error = %v", err)
		}
		if !strings.Contains(result.Text, "already up to date") {
			t.Fatalf("second file_write result.Text = %q, want already-up-to-date message", result.Text)
		}
		if result.ModelText != "The file "+writePath+" is already up to date." {
			t.Fatalf("second file_write result.ModelText = %q, want up-to-date text", result.ModelText)
		}

		result, err = registry.Execute(context.Background(), Call{
			Name:  "file_write",
			Input: map[string]any{"path": writePath, "content": "updated"},
		}, ExecContext{
			WorkspaceRoot:    root,
			PermissionEngine: permissions.NewEngine("trusted_local"),
		})
		if err != nil {
			t.Fatalf("third file_write error = %v", err)
		}
		if result.ModelText != "The file "+writePath+" has been updated successfully." {
			t.Fatalf("third file_write result.ModelText = %q, want update success text", result.ModelText)
		}

		editResult, err := registry.Execute(context.Background(), Call{
			Name: "file_edit",
			Input: map[string]any{
				"file_path":  writePath,
				"old_string": "updated",
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
