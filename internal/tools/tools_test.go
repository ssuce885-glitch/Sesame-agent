package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"go-agent/internal/permissions"
	"go-agent/internal/runtimegraph"
	"go-agent/internal/scheduler"
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

type blockingTaskRunner struct {
	started chan struct{}
	release chan struct{}
}

type fakeTaskAgentExecutor struct {
	output                 string
	finalText              string
	gotActivatedSkillNames []string
	err                    error
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

func (r blockingTaskRunner) Run(ctx context.Context, _ *task.Task, _ task.OutputSink) error {
	if r.started != nil {
		select {
		case r.started <- struct{}{}:
		default:
		}
	}
	select {
	case <-r.release:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (f fakeTaskAgentExecutor) RunTask(ctx context.Context, workspaceRoot string, prompt string, activatedSkillNames []string, observer task.AgentTaskObserver) error {
	_ = ctx
	_ = workspaceRoot
	_ = prompt
	f.gotActivatedSkillNames = append([]string(nil), activatedSkillNames...)
	if f.err != nil {
		return f.err
	}
	if f.output != "" {
		if err := observer.AppendLog([]byte(f.output)); err != nil {
			return err
		}
	}
	if f.finalText != "" {
		if err := observer.SetFinalText(f.finalText); err != nil {
			return err
		}
	}
	return nil
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

func TestFileReadToolAllowsSesameGlobalConfigRoot(t *testing.T) {
	home := t.TempDir()
	workspace := t.TempDir()
	globalRoot := filepath.Join(home, ".sesame")
	configPath := filepath.Join(globalRoot, "config.json")
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(configPath, []byte(`{"model":"demo"}`), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	registry := NewRegistry()
	result, err := registry.Execute(context.Background(), Call{
		Name:  "file_read",
		Input: map[string]any{"path": "~/.sesame/config.json"},
	}, ExecContext{
		WorkspaceRoot:    workspace,
		GlobalConfigRoot: globalRoot,
		PermissionEngine: permissions.NewEngine(),
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Text != `{"model":"demo"}` {
		t.Fatalf("result.Text = %q, want config contents", result.Text)
	}
}

func TestRegistryDefinitionsExposePhase3PlanModeSchemas(t *testing.T) {
	registry := NewRegistry()

	defs := registry.Definitions()
	wantNames := []string{
		"apply_patch",
		"enter_plan_mode", "enter_worktree", "exit_plan_mode",
		"exit_worktree",
		"file_edit", "file_read", "file_write", "glob", "grep", "list_dir",
		"notebook_edit", "request_permissions", "request_user_input", "schedule_report", "shell_command", "skill_use",
		"task_create", "task_get", "task_list", "task_output", "task_result", "task_stop", "task_update", "task_wait",
		"todo_write", "view_image", "web_fetch",
	}
	if len(defs) != len(wantNames) {
		t.Fatalf("len(Definitions) = %d, want %d", len(defs), len(wantNames))
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

	requireSchemaFields("apply_patch", []string{}, "patch", "input")
	requireSchemaFields("enter_plan_mode", []string{"plan_file"}, "plan_file")
	requireSchemaFields("enter_worktree", []string{"task_id"}, "task_id", "branch")
	exitDef := requireSchemaFields("exit_plan_mode", []string{}, "state")
	requireSchemaFields("exit_worktree", []string{"worktree_id", "worktree_path"}, "worktree_id", "worktree_path", "task_id", "cleanup")
	requireSchemaFields("file_read", []string{"path"}, "path")
	requireSchemaFields("file_write", []string{"path", "content"}, "path", "content")
	requireSchemaFields("file_edit", []string{"file_path", "old_string", "new_string"}, "file_path", "old_string", "new_string", "replace_all")
	requireSchemaFields("glob", []string{"pattern"}, "pattern")
	requireSchemaFields("grep", []string{"path", "pattern"}, "path", "pattern")
	requireSchemaFields("list_dir", []string{}, "path", "dir_path", "offset", "limit", "depth")
	requireSchemaFields("notebook_edit", []string{"notebook_path", "new_source"}, "notebook_path", "cell_id", "new_source", "cell_type", "edit_mode")
	requireSchemaFields("request_permissions", []string{"profile"}, "profile", "reason")
	requireSchemaFields("request_user_input", []string{"questions"}, "questions")
	requireSchemaFields("schedule_report", []string{"prompt"}, "name", "prompt", "report_group_id", "report_group_title", "report_group_run_at", "report_group_every_minutes", "report_group_cron", "report_group_timezone", "delay_minutes", "run_at", "every_minutes", "cron", "timezone", "timeout_seconds", "skip_if_running")
	requireSchemaFields("shell_command", []string{"command"}, "command", "workdir", "timeout_seconds", "max_output_bytes")
	requireSchemaFields("skill_use", []string{"name"}, "name")
	requireSchemaFields("todo_write", []string{"todos"}, "todos")
	requireSchemaFields("task_create", []string{"type", "command"}, "type", "command", "description", "plan_id", "parent_task_id", "owner", "kind", "worktree_id", "start")
	requireSchemaFields("task_get", []string{"task_id"}, "task_id")
	requireSchemaFields("task_list", []string{}, "status")
	requireSchemaFields("task_output", []string{"task_id"}, "task_id")
	requireSchemaFields("task_result", []string{"task_id"}, "task_id")
	requireSchemaFields("task_stop", []string{"task_id"}, "task_id")
	requireSchemaFields("task_update", []string{"task_id"}, "task_id", "status", "description", "owner", "worktree_id")
	requireSchemaFields("task_wait", []string{"task_id"}, "task_id", "timeout_ms")
	requireSchemaFields("view_image", []string{"path"}, "path")
	requireSchemaFields("web_fetch", []string{"url"}, "url", "timeout_seconds", "max_bytes")

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

	requireOutputSchemaFields("apply_patch", []string{"status", "change_count", "changes", "summary"}, "status", "change_count", "changes", "summary")
	requireOutputSchemaFields("enter_plan_mode", []string{"plan_id", "run_id", "state", "plan_file"}, "plan_id", "run_id", "state", "plan_file")
	requireOutputSchemaFields("enter_worktree", []string{"worktree_id", "task_id", "worktree_path", "state"}, "worktree_id", "task_id", "worktree_path", "branch", "state")
	requireOutputSchemaFields("exit_plan_mode", []string{"plan_id", "state"}, "plan_id", "state")
	requireOutputSchemaFields("exit_worktree", []string{"worktree_id", "worktree_path", "state"}, "worktree_id", "worktree_path", "state")
	requireOutputSchemaFields("file_read", []string{"path", "content", "unchanged"}, "path", "content", "unchanged")
	requireOutputSchemaFields("file_write", []string{"path", "status", "bytes_written"}, "path", "status", "bytes_written")
	requireOutputSchemaFields("file_edit", []string{"file_path", "old_string", "new_string", "replace_all", "replaced_count"}, "file_path", "old_string", "new_string", "replace_all", "replaced_count")
	requireOutputSchemaFields("glob", []string{"pattern", "matches", "count"}, "pattern", "matches", "count")
	requireOutputSchemaFields("grep", []string{"path", "pattern", "matched", "match_count"}, "path", "pattern", "matched", "match_count")
	requireOutputSchemaFields("list_dir", []string{"path", "entries", "offset", "limit", "depth", "total_count", "returned_count", "has_more"}, "path", "entries", "offset", "limit", "depth", "total_count", "returned_count", "has_more")
	requireOutputSchemaFields("notebook_edit", []string{"notebook_path", "new_source", "cell_type", "edit_mode", "original_file", "updated_file"}, "notebook_path", "cell_id", "new_source", "cell_type", "language", "edit_mode", "original_file", "updated_file")
	requireOutputSchemaFields("request_permissions", []string{"permission_request_id", "status", "profile"}, "permission_request_id", "status", "profile", "reason")
	requireOutputSchemaFields("request_user_input", []string{"status", "questions", "prompt_text"}, "status", "questions", "prompt_text")
	requireOutputSchemaFields("schedule_report", []string{"job"}, "job")
	requireOutputSchemaFields("shell_command", []string{"command", "output", "stdout", "stderr", "exit_code", "timed_out", "duration_ms", "truncated", "timeout_seconds", "max_output_bytes", "classification"}, "command", "workdir", "output", "stdout", "stderr", "exit_code", "timed_out", "duration_ms", "truncated", "timeout_seconds", "max_output_bytes", "classification")
	requireOutputSchemaFields("skill_use", []string{"name", "scope", "path", "body_injected"}, "name", "scope", "path", "description", "granted_tools", "body_injected")
	requireOutputSchemaFields("todo_write", []string{"path", "old_todos", "new_todos", "count"}, "path", "old_todos", "new_todos", "count")
	requireOutputSchemaFields("task_create", []string{"task_id", "type", "command"}, "task_id", "type", "command", "description")
	requireOutputSchemaFields("task_get", []string{"task"}, "task")
	requireOutputSchemaFields("task_list", []string{"tasks"}, "tasks", "status_filter")
	requireOutputSchemaFields("task_output", []string{"task_id", "output"}, "task_id", "output")
	requireOutputSchemaFields("task_result", []string{"task_id", "status"}, "task_id", "status", "kind", "text", "observed_at")
	requireOutputSchemaFields("task_stop", []string{"task_id"}, "task_id")
	requireOutputSchemaFields("task_update", []string{"task"}, "task")
	requireOutputSchemaFields("task_wait", []string{"task", "timed_out", "result_ready"}, "task", "timed_out", "result_ready")
	requireOutputSchemaFields("view_image", []string{"path", "mime_type", "size_bytes"}, "path", "mime_type", "width", "height", "size_bytes")
	requireOutputSchemaFields("web_fetch", []string{"url", "final_url", "status_code", "status", "content_type", "content_kind", "readable", "content", "bytes_read", "truncated"}, "url", "final_url", "status_code", "status", "content_type", "content_kind", "readable", "title", "content", "bytes_read", "truncated")
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

	wantNames := []string{"file_read", "glob", "grep", "list_dir", "request_permissions", "request_user_input", "skill_use", "view_image", "web_fetch"}
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
		"apply_patch",
		"file_edit",
		"file_read",
		"file_write",
		"glob",
		"grep",
		"list_dir",
		"notebook_edit",
		"request_permissions",
		"request_user_input",
		"shell_command",
		"skill_use",
		"view_image",
		"web_fetch",
	}
	if !reflect.DeepEqual(gotNames, wantNames) {
		t.Fatalf("VisibleDefinitions(trusted_local) names = %v, want %v", gotNames, wantNames)
	}
}

func TestRegistryVisibleDefinitionsIncludeScheduleReportWithSchedulerContext(t *testing.T) {
	store, err := sqlite.Open(filepath.Join(t.TempDir(), "agentd.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	registry := NewRegistry()
	defs := registry.VisibleDefinitions(ExecContext{
		WorkspaceRoot:    t.TempDir(),
		PermissionEngine: permissions.NewEngine("trusted_local"),
		SchedulerService: scheduler.NewService(store, nil),
		TurnContext: &runtimegraph.TurnContext{
			CurrentSessionID: "sess_schedule_visible",
			CurrentTurnID:    "turn_schedule_visible",
		},
	})

	found := false
	for _, def := range defs {
		if def.Name == "schedule_report" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("VisibleDefinitions() missing schedule_report: %v", defs)
	}
}

func TestRuntimeVisibleDefinitionsIncludeCustomToolsForTrustedLocal(t *testing.T) {
	home := t.TempDir()
	workspace := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	writeCustomToolManifest(t, filepath.Join(home, ".sesame", "tools", "global-demo", "tool.json"), map[string]any{
		"name":        "global_demo",
		"description": "global demo tool",
		"command":     os.Args[0],
		"args":        []string{"-test.run=TestCustomToolHelperProcess", "--", "global-json"},
		"input_schema": map[string]any{
			"type":                 "object",
			"properties":           map[string]any{},
			"additionalProperties": false,
		},
	})
	writeCustomToolManifest(t, filepath.Join(workspace, ".sesame", "tools", "workspace-demo", "tool.json"), map[string]any{
		"name":        "workspace_demo",
		"description": "workspace demo tool",
		"command":     os.Args[0],
		"args":        []string{"-test.run=TestCustomToolHelperProcess", "--", "workspace-json"},
		"input_schema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"value": map[string]any{"type": "string"},
			},
			"required":             []string{"value"},
			"additionalProperties": false,
		},
		"output_schema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"input":     map[string]any{"type": "object"},
				"tool_name": map[string]any{"type": "string"},
				"scope":     map[string]any{"type": "string"},
			},
			"required":             []string{"input", "tool_name", "scope"},
			"additionalProperties": false,
		},
	})

	runtime := NewRuntime(NewRegistry(), nil)
	trustedDefs := runtime.VisibleDefinitions(ExecContext{
		WorkspaceRoot:    workspace,
		PermissionEngine: permissions.NewEngine("trusted_local"),
	})

	gotTrusted := make([]string, 0, len(trustedDefs))
	for _, def := range trustedDefs {
		gotTrusted = append(gotTrusted, def.Name)
	}
	if !containsString(gotTrusted, "global_demo") {
		t.Fatalf("trusted definitions missing global_demo: %v", gotTrusted)
	}
	if !containsString(gotTrusted, "workspace_demo") {
		t.Fatalf("trusted definitions missing workspace_demo: %v", gotTrusted)
	}

	readOnlyDefs := runtime.VisibleDefinitions(ExecContext{
		WorkspaceRoot:    workspace,
		PermissionEngine: permissions.NewEngine(),
	})
	for _, def := range readOnlyDefs {
		if def.Name == "global_demo" || def.Name == "workspace_demo" {
			t.Fatalf("read-only definitions unexpectedly exposed custom tool %q", def.Name)
		}
	}
}

func TestRuntimeExecuteCustomToolPrefersWorkspaceOverride(t *testing.T) {
	home := t.TempDir()
	workspace := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	writeCustomToolManifest(t, filepath.Join(home, ".sesame", "tools", "shared-tool", "tool.json"), map[string]any{
		"name":        "shared_tool",
		"description": "global shared tool",
		"command":     os.Args[0],
		"args":        []string{"-test.run=TestCustomToolHelperProcess", "--", "global-json"},
		"input_schema": map[string]any{
			"type":                 "object",
			"properties":           map[string]any{},
			"additionalProperties": false,
		},
		"output_schema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"source": map[string]any{"type": "string"},
			},
			"required":             []string{"source"},
			"additionalProperties": false,
		},
	})
	writeCustomToolManifest(t, filepath.Join(workspace, ".sesame", "tools", "shared-tool", "tool.json"), map[string]any{
		"name":        "shared_tool",
		"description": "workspace shared tool",
		"command":     os.Args[0],
		"args":        []string{"-test.run=TestCustomToolHelperProcess", "--", "workspace-override"},
		"input_schema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"value": map[string]any{"type": "string"},
			},
			"required":             []string{"value"},
			"additionalProperties": false,
		},
		"output_schema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"source": map[string]any{"type": "string"},
			},
			"required":             []string{"source"},
			"additionalProperties": false,
		},
	})

	runtime := NewRuntime(NewRegistry(), nil)
	output, err := runtime.ExecuteRich(context.Background(), Call{
		Name:  "shared_tool",
		Input: map[string]any{"value": "Ada"},
	}, ExecContext{
		WorkspaceRoot:    workspace,
		PermissionEngine: permissions.NewEngine("trusted_local"),
	})
	if err != nil {
		t.Fatalf("ExecuteRich(custom workspace override) error = %v", err)
	}
	if output.Text != "workspace override tool" {
		t.Fatalf("output.Text = %q, want workspace override tool", output.Text)
	}
	data, ok := output.Data.(map[string]any)
	if !ok {
		t.Fatalf("output.Data type = %T, want map[string]any", output.Data)
	}
	if data["source"] != "workspace" {
		t.Fatalf("output.Data[source] = %v, want workspace", data["source"])
	}
}

func TestRuntimeExecuteCustomToolRequiresTrustedLocal(t *testing.T) {
	home := t.TempDir()
	workspace := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	writeCustomToolManifest(t, filepath.Join(workspace, ".sesame", "tools", "workspace-demo", "tool.json"), map[string]any{
		"name":        "workspace_demo",
		"description": "workspace demo tool",
		"command":     os.Args[0],
		"args":        []string{"-test.run=TestCustomToolHelperProcess", "--", "workspace-json"},
		"input_schema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"value": map[string]any{"type": "string"},
			},
			"required":             []string{"value"},
			"additionalProperties": false,
		},
	})

	runtime := NewRuntime(NewRegistry(), nil)
	_, err := runtime.ExecuteRich(context.Background(), Call{
		Name:  "workspace_demo",
		Input: map[string]any{"value": "Ada"},
	}, ExecContext{
		WorkspaceRoot:    workspace,
		PermissionEngine: permissions.NewEngine(),
	})
	if err == nil || !strings.Contains(err.Error(), `tool "workspace_demo" denied`) {
		t.Fatalf("ExecuteRich(custom read_only) error = %v, want deny", err)
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

func TestScheduleReportToolCreatesPersistedJob(t *testing.T) {
	root := t.TempDir()
	store, err := sqlite.Open(filepath.Join(t.TempDir(), "agentd.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	service := scheduler.NewService(store, nil)
	registry := NewRegistry()
	execCtx := ExecContext{
		WorkspaceRoot:    root,
		ActiveSkillNames: []string{"send-email", " send-email "},
		PermissionEngine: permissions.NewEngine("trusted_local"),
		SchedulerService: service,
		TurnContext: &runtimegraph.TurnContext{
			CurrentSessionID: "sess_schedule_report",
			CurrentTurnID:    "turn_schedule_report",
		},
	}

	output, err := registry.executePreparedRich(context.Background(), registry.prepareCall(Call{
		Name: "schedule_report",
		Input: map[string]any{
			"name":                  "Shanghai weather",
			"prompt":                "五分钟后汇报上海天气",
			"report_group_id":       "weather-daily",
			"report_group_title":    "Weather Daily Digest",
			"report_group_cron":     "0 18 * * *",
			"report_group_timezone": "Asia/Shanghai",
			"delay_minutes":         5,
		},
	}), execCtx)
	if err != nil {
		t.Fatalf("schedule_report error = %v", err)
	}

	data, ok := output.Data.(ScheduleReportOutput)
	if !ok {
		t.Fatalf("schedule_report data = %#v, want ScheduleReportOutput", output.Data)
	}
	if data.Job.ID == "" || data.Job.Kind != types.ScheduleKindAt {
		t.Fatalf("scheduled job = %#v, want populated one-shot job", data.Job)
	}
	if data.Job.OwnerSessionID != "sess_schedule_report" {
		t.Fatalf("owner session = %q, want sess_schedule_report", data.Job.OwnerSessionID)
	}
	if got := data.Job.ActivatedSkillNames; len(got) != 1 || got[0] != "send-email" {
		t.Fatalf("job activated skills = %v, want [send-email]", got)
	}
	if data.Job.NextRunAt.IsZero() {
		t.Fatalf("NextRunAt = %v, want populated next run", data.Job.NextRunAt)
	}

	persisted, ok, err := store.GetScheduledJob(context.Background(), data.Job.ID)
	if err != nil {
		t.Fatalf("GetScheduledJob() error = %v", err)
	}
	if !ok {
		t.Fatal("scheduled job was not persisted")
	}
	if persisted.Prompt != "五分钟后汇报上海天气" {
		t.Fatalf("persisted prompt = %q, want original prompt", persisted.Prompt)
	}
	if persisted.ReportGroupID != "weather-daily" {
		t.Fatalf("persisted report group = %q, want weather-daily", persisted.ReportGroupID)
	}
	if got := persisted.ActivatedSkillNames; len(got) != 1 || got[0] != "send-email" {
		t.Fatalf("persisted activated skills = %v, want [send-email]", got)
	}

	spec, ok, err := store.GetChildAgentSpec(context.Background(), data.Job.ID)
	if err != nil {
		t.Fatalf("GetChildAgentSpec() error = %v", err)
	}
	if !ok {
		t.Fatal("child agent spec was not persisted")
	}
	if spec.SessionID != "sess_schedule_report" {
		t.Fatalf("spec session = %q, want sess_schedule_report", spec.SessionID)
	}
	if got := spec.ActivatedSkillNames; len(got) != 1 || got[0] != "send-email" {
		t.Fatalf("spec activated skills = %v, want [send-email]", got)
	}
	if len(spec.ReportGroups) != 1 || spec.ReportGroups[0] != "weather-daily" {
		t.Fatalf("spec report groups = %#v, want weather-daily", spec.ReportGroups)
	}

	group, ok, err := store.GetReportGroup(context.Background(), "weather-daily")
	if err != nil {
		t.Fatalf("GetReportGroup() error = %v", err)
	}
	if !ok {
		t.Fatal("report group was not auto-created")
	}
	if group.SessionID != "sess_schedule_report" {
		t.Fatalf("group session = %q, want sess_schedule_report", group.SessionID)
	}
	if group.Schedule.Kind != types.ScheduleKindCron || group.Schedule.Expr != "0 18 * * *" || group.Schedule.Timezone != "Asia/Shanghai" {
		t.Fatalf("group schedule = %#v, want cron 0 18 * * * Asia/Shanghai", group.Schedule)
	}
}

func TestScheduleReportToolInfersExecutionSkillsFromPrompt(t *testing.T) {
	home := t.TempDir()
	root := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	skillDir := filepath.Join(home, ".sesame", "skills", "send-email")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(`---
name: send-email
description: Send emails via SMTP.
---
Run the local email sender script with shell_command.`), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	store, err := sqlite.Open(filepath.Join(t.TempDir(), "agentd.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	service := scheduler.NewService(store, nil)
	registry := NewRegistry()
	execCtx := ExecContext{
		WorkspaceRoot:    root,
		PermissionEngine: permissions.NewEngine("trusted_local"),
		SchedulerService: service,
		TurnContext: &runtimegraph.TurnContext{
			CurrentSessionID: "sess_schedule_report_infer",
			CurrentTurnID:    "turn_schedule_report_infer",
		},
	}

	output, err := registry.executePreparedRich(context.Background(), registry.prepareCall(Call{
		Name: "schedule_report",
		Input: map[string]any{
			"prompt":        "查询合肥市当前天气信息，并发送到邮箱 demo@example.com",
			"delay_minutes": 1,
		},
	}), execCtx)
	if err != nil {
		t.Fatalf("schedule_report error = %v", err)
	}

	data, ok := output.Data.(ScheduleReportOutput)
	if !ok {
		t.Fatalf("schedule_report data = %#v, want ScheduleReportOutput", output.Data)
	}
	if got := data.Job.ActivatedSkillNames; len(got) != 1 || got[0] != "send-email" {
		t.Fatalf("job activated skills = %v, want [send-email]", got)
	}

	persisted, ok, err := store.GetScheduledJob(context.Background(), data.Job.ID)
	if err != nil {
		t.Fatalf("GetScheduledJob() error = %v", err)
	}
	if !ok {
		t.Fatal("scheduled job was not persisted")
	}
	if got := persisted.ActivatedSkillNames; len(got) != 1 || got[0] != "send-email" {
		t.Fatalf("persisted activated skills = %v, want [send-email]", got)
	}

	spec, ok, err := store.GetChildAgentSpec(context.Background(), data.Job.ID)
	if err != nil {
		t.Fatalf("GetChildAgentSpec() error = %v", err)
	}
	if !ok {
		t.Fatal("child agent spec was not persisted")
	}
	if got := spec.ActivatedSkillNames; len(got) != 1 || got[0] != "send-email" {
		t.Fatalf("spec activated skills = %v, want [send-email]", got)
	}
}

func TestShellToolRequestsApprovalForDestructiveCommands(t *testing.T) {
	root := t.TempDir()
	store, err := sqlite.Open(filepath.Join(root, "agentd.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	output, err := NewRuntime(NewRegistry(), store).ExecuteRich(context.Background(), Call{
		ID:    "call_shell_1",
		Name:  "shell_command",
		Input: map[string]any{"command": "del important.txt"},
	}, ExecContext{
		WorkspaceRoot:    root,
		PermissionEngine: permissions.NewEngine("trusted_local"),
		TurnContext: &runtimegraph.TurnContext{
			CurrentRunID:     "run_shell_approval",
			CurrentSessionID: "sess_shell_approval",
			CurrentTurnID:    "turn_shell_approval",
		},
	})
	if err != nil {
		t.Fatalf("ExecuteRich() error = %v, want interrupt result", err)
	}
	if output.Interrupt == nil || strings.TrimSpace(output.Interrupt.EventType) != types.EventPermissionRequested {
		t.Fatalf("output.Interrupt = %#v, want permission requested interrupt", output.Interrupt)
	}
	if !strings.Contains(output.Result.Text, "Approval requested for shell_command") {
		t.Fatalf("output.Result.Text = %q, want approval notice", output.Result.Text)
	}

	graph, err := store.ListRuntimeGraph(context.Background())
	if err != nil {
		t.Fatalf("ListRuntimeGraph() error = %v", err)
	}
	if len(graph.ToolRuns) != 1 {
		t.Fatalf("len(graph.ToolRuns) = %d, want 1", len(graph.ToolRuns))
	}
	if graph.ToolRuns[0].State != types.ToolRunStateWaitingPermission {
		t.Fatalf("tool run state = %q, want waiting_permission", graph.ToolRuns[0].State)
	}
}

func TestListDirToolReturnsStructuredDirectoryEntries(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("hello"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if err := os.Mkdir(filepath.Join(root, "docs"), 0o755); err != nil {
		t.Fatalf("Mkdir() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "docs", "guide.md"), []byte("guide"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	registry := NewRegistry()
	result, err := NewRuntime(registry, nil).ExecuteRich(context.Background(), Call{
		Name:  "list_dir",
		Input: map[string]any{"path": ".", "depth": 2},
	}, ExecContext{
		WorkspaceRoot:    root,
		PermissionEngine: permissions.NewEngine(),
	})
	if err != nil {
		t.Fatalf("ExecuteRich() error = %v", err)
	}
	if !strings.Contains(result.Text, "README.md") || !strings.Contains(result.Text, "docs/guide.md") {
		t.Fatalf("result.Text = %q, want listed entries", result.Text)
	}
	data, ok := result.Data.(ListDirOutput)
	if !ok {
		t.Fatalf("result.Data type = %T, want ListDirOutput", result.Data)
	}
	if data.TotalCount != 3 {
		t.Fatalf("data.TotalCount = %d, want 3", data.TotalCount)
	}
	if len(data.Entries) != 3 {
		t.Fatalf("len(data.Entries) = %d, want 3", len(data.Entries))
	}
	if data.Entries[0].Path != "README.md" || data.Entries[1].Path != "docs" || data.Entries[2].Path != "docs/guide.md" {
		t.Fatalf("entries = %+v, want sorted relative paths", data.Entries)
	}
}

func TestListDirToolAllowsSesameGlobalConfigRoot(t *testing.T) {
	home := t.TempDir()
	workspace := t.TempDir()
	globalRoot := filepath.Join(home, ".sesame")
	skillsRoot := filepath.Join(globalRoot, "skills")
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	if err := os.MkdirAll(filepath.Join(skillsRoot, "demo"), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillsRoot, "demo", "SKILL.md"), []byte("demo"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	registry := NewRegistry()
	result, err := NewRuntime(registry, nil).ExecuteRich(context.Background(), Call{
		Name:  "list_dir",
		Input: map[string]any{"path": "~/.sesame/skills", "depth": 2},
	}, ExecContext{
		WorkspaceRoot:    workspace,
		GlobalConfigRoot: globalRoot,
		PermissionEngine: permissions.NewEngine(),
	})
	if err != nil {
		t.Fatalf("ExecuteRich() error = %v", err)
	}
	if !strings.Contains(result.Text, "demo/SKILL.md") {
		t.Fatalf("result.Text = %q, want global skill entries", result.Text)
	}
}

func TestApplyPatchToolAddsUpdatesMovesAndDeletesFiles(t *testing.T) {
	root := t.TempDir()
	oldPath := filepath.Join(root, "old.txt")
	deletePath := filepath.Join(root, "delete.txt")
	if err := os.WriteFile(oldPath, []byte("hello\nworld\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(old) error = %v", err)
	}
	if err := os.WriteFile(deletePath, []byte("remove me\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(delete) error = %v", err)
	}

	patch := strings.Join([]string{
		"*** Begin Patch",
		"*** Add File: added.txt",
		"+new file",
		"*** Update File: old.txt",
		"*** Move to: moved.txt",
		"@@",
		" hello",
		"-world",
		"+gophers",
		"*** Delete File: delete.txt",
		"*** End Patch",
	}, "\n")

	registry := NewRegistry()
	result, err := NewRuntime(registry, nil).ExecuteRich(context.Background(), Call{
		Name:  "apply_patch",
		Input: map[string]any{"patch": patch},
	}, ExecContext{
		WorkspaceRoot:    root,
		PermissionEngine: permissions.NewEngine("trusted_local"),
	})
	if err != nil {
		t.Fatalf("ExecuteRich() error = %v", err)
	}

	addedData, err := os.ReadFile(filepath.Join(root, "added.txt"))
	if err != nil {
		t.Fatalf("ReadFile(added) error = %v", err)
	}
	if string(addedData) != "new file\n" {
		t.Fatalf("added.txt = %q, want %q", string(addedData), "new file\n")
	}
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Fatalf("old.txt stat err = %v, want not exists", err)
	}
	movedData, err := os.ReadFile(filepath.Join(root, "moved.txt"))
	if err != nil {
		t.Fatalf("ReadFile(moved) error = %v", err)
	}
	if string(movedData) != "hello\ngophers\n" {
		t.Fatalf("moved.txt = %q, want %q", string(movedData), "hello\ngophers\n")
	}
	if _, err := os.Stat(deletePath); !os.IsNotExist(err) {
		t.Fatalf("delete.txt stat err = %v, want not exists", err)
	}
	data, ok := result.Data.(ApplyPatchOutput)
	if !ok {
		t.Fatalf("result.Data type = %T, want ApplyPatchOutput", result.Data)
	}
	if data.ChangeCount != 3 {
		t.Fatalf("data.ChangeCount = %d, want 3", data.ChangeCount)
	}
}

func TestApplyPatchRejectsPathTraversal(t *testing.T) {
	root := t.TempDir()
	registry := NewRegistry()
	_, err := NewRuntime(registry, nil).ExecuteRich(context.Background(), Call{
		Name: "apply_patch",
		Input: map[string]any{
			"patch": strings.Join([]string{
				"*** Begin Patch",
				"*** Add File: ../escape.txt",
				"+nope",
				"*** End Patch",
			}, "\n"),
		},
	}, ExecContext{
		WorkspaceRoot:    root,
		PermissionEngine: permissions.NewEngine("trusted_local"),
	})
	if err == nil || !strings.Contains(err.Error(), "workspace") {
		t.Fatalf("ExecuteRich() error = %v, want workspace boundary failure", err)
	}
}

func TestRequestUserInputToolProducesInterrupt(t *testing.T) {
	registry := NewRegistry()
	result, err := NewRuntime(registry, nil).ExecuteRich(context.Background(), Call{
		Name: "request_user_input",
		Input: map[string]any{
			"questions": []any{
				map[string]any{
					"id":       "approval",
					"header":   "Need input",
					"question": "Which direction should I take?",
					"options": []any{
						map[string]any{"label": "Option A (Recommended)", "description": "Faster but narrower."},
						map[string]any{"label": "Option B", "description": "Broader but slower."},
					},
				},
			},
		},
	}, ExecContext{
		WorkspaceRoot:    t.TempDir(),
		PermissionEngine: permissions.NewEngine(),
	})
	if err != nil {
		t.Fatalf("ExecuteRich() error = %v", err)
	}
	if result.Interrupt == nil {
		t.Fatal("result.Interrupt = nil, want interruption metadata")
	}
	if result.Interrupt.Reason != "user_input_requested" {
		t.Fatalf("result.Interrupt.Reason = %q, want user_input_requested", result.Interrupt.Reason)
	}
	if !strings.Contains(result.Interrupt.Notice, "Which direction should I take?") {
		t.Fatalf("result.Interrupt.Notice = %q, want question text", result.Interrupt.Notice)
	}
}

func TestRequestPermissionsToolProducesInterrupt(t *testing.T) {
	registry := NewRegistry()
	result, err := NewRuntime(registry, nil).ExecuteRich(context.Background(), Call{
		Name: "request_permissions",
		Input: map[string]any{
			"profile": "trusted_local",
			"reason":  "Need shell access to run tests.",
		},
	}, ExecContext{
		WorkspaceRoot:    t.TempDir(),
		PermissionEngine: permissions.NewEngine(),
		TurnContext: &runtimegraph.TurnContext{
			CurrentTurnID: "turn_perm",
		},
	})
	if err != nil {
		t.Fatalf("ExecuteRich() error = %v", err)
	}
	if result.Interrupt == nil {
		t.Fatal("result.Interrupt = nil, want interruption metadata")
	}
	if result.Interrupt.EventType != types.EventPermissionRequested {
		t.Fatalf("result.Interrupt.EventType = %q, want permission.requested", result.Interrupt.EventType)
	}
}

func TestShellCommandReturnsStructuredExitStatusForNonZeroCommand(t *testing.T) {
	registry := NewRegistry()
	command := "echo stdout && echo stderr 1>&2 && exit /b 7"
	if runtime.GOOS != "windows" {
		command = "echo stdout; echo stderr 1>&2; exit 7"
	}
	result, err := NewRuntime(registry, nil).ExecuteRich(context.Background(), Call{
		Name:  "shell_command",
		Input: map[string]any{"command": command},
	}, ExecContext{
		WorkspaceRoot:    t.TempDir(),
		PermissionEngine: permissions.NewEngine("trusted_local"),
	})
	if err != nil {
		t.Fatalf("ExecuteRich() error = %v, want nil for non-zero exit status", err)
	}
	data, ok := result.Data.(ShellCommandOutput)
	if !ok {
		t.Fatalf("result.Data type = %T, want ShellCommandOutput", result.Data)
	}
	if data.ExitCode != 7 {
		t.Fatalf("data.ExitCode = %d, want 7", data.ExitCode)
	}
	if !strings.Contains(data.Stdout, "stdout") {
		t.Fatalf("data.Stdout = %q, want stdout", data.Stdout)
	}
	if !strings.Contains(data.Stderr, "stderr") {
		t.Fatalf("data.Stderr = %q, want stderr", data.Stderr)
	}
	if result.Result.Text == "" {
		t.Fatal("result.Result.Text = empty, want aggregated output")
	}
	if !strings.Contains(result.Result.ModelText, "Exit code: 7") {
		t.Fatalf("result.Result.ModelText = %q, want exit code summary", result.Result.ModelText)
	}
}

func TestShellCommandInjectsExecContextEnv(t *testing.T) {
	registry := NewRegistry()
	command := "printf '%s' \"$SKILL_TOKEN\""
	result, err := NewRuntime(registry, nil).ExecuteRich(context.Background(), Call{
		Name:  "shell_command",
		Input: map[string]any{"command": command},
	}, ExecContext{
		WorkspaceRoot:    t.TempDir(),
		PermissionEngine: permissions.NewEngine("trusted_local"),
		InjectedEnv: map[string]string{
			"SKILL_TOKEN": "from-skill-env",
		},
	})
	if err != nil {
		t.Fatalf("ExecuteRich() error = %v", err)
	}
	data, ok := result.Data.(ShellCommandOutput)
	if !ok {
		t.Fatalf("result.Data type = %T, want ShellCommandOutput", result.Data)
	}
	if data.Stdout != "from-skill-env" {
		t.Fatalf("data.Stdout = %q, want injected env value", data.Stdout)
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

	result, err := registry.executePreparedRich(context.Background(), registry.prepareCall(Call{
		Name:  "task_result",
		Input: map[string]any{"task_id": created.Text},
	}), execCtx)
	if err != nil {
		t.Fatalf("task_result error = %v", err)
	}
	missing, ok := result.Data.(TaskResultOutput)
	if !ok {
		t.Fatalf("task_result data = %#v, want TaskResultOutput", result.Data)
	}
	if missing.Status != taskResultStatusMissing {
		t.Fatalf("task_result status = %q, want %q", missing.Status, taskResultStatusMissing)
	}
}

func TestTaskCreateToolCarriesActiveSkillsToAgentTask(t *testing.T) {
	root := t.TempDir()
	manager := task.NewManager(task.Config{MaxConcurrentTasks: 8, TaskOutputMaxBytes: 1 << 20}, nil, nil)
	registry := NewRegistry()
	execCtx := ExecContext{
		WorkspaceRoot:    root,
		ActiveSkillNames: []string{"send-email", " send-email "},
		PermissionEngine: permissions.NewEngine("trusted_local"),
		TaskManager:      manager,
	}

	created, err := registry.executePreparedRich(context.Background(), registry.prepareCall(Call{
		Name: "task_create",
		Input: map[string]any{
			"type":    "agent",
			"command": "查询天气并发送邮件",
			"start":   false,
		},
	}), execCtx)
	if err != nil {
		t.Fatalf("task_create error = %v", err)
	}

	data, ok := created.Data.(TaskCreateOutput)
	if !ok {
		t.Fatalf("task_create data = %#v, want TaskCreateOutput", created.Data)
	}
	got, ok, err := manager.Get(data.TaskID, root)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if !ok {
		t.Fatalf("task %q not found", data.TaskID)
	}
	if want := []string{"send-email"}; !reflect.DeepEqual(got.ActivatedSkillNames, want) {
		t.Fatalf("activated skills = %v, want %v", got.ActivatedSkillNames, want)
	}
}

func TestTaskWaitToolUsesTerminalStatusInsteadOfOutputGuessing(t *testing.T) {
	root := t.TempDir()
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	manager := task.NewManager(
		task.Config{MaxConcurrentTasks: 8, TaskOutputMaxBytes: 1 << 20},
		map[task.TaskType]task.Runner{
			task.TaskTypeShell: blockingTaskRunner{started: started, release: release},
		},
		nil,
	)
	registry := NewRegistry()
	execCtx := ExecContext{
		WorkspaceRoot:    root,
		PermissionEngine: permissions.NewEngine("trusted_local"),
		TaskManager:      manager,
	}

	created, err := registry.Execute(context.Background(), Call{
		Name: "task_create",
		Input: map[string]any{
			"type":    "shell",
			"command": "blocked-task",
		},
	}, execCtx)
	if err != nil {
		t.Fatalf("task_create error = %v", err)
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("runner did not start")
	}

	waitTimedOut, err := registry.executePreparedRich(context.Background(), registry.prepareCall(Call{
		Name: "task_wait",
		Input: map[string]any{
			"task_id":    created.Text,
			"timeout_ms": 25,
		},
	}), execCtx)
	if err != nil {
		t.Fatalf("task_wait timeout error = %v", err)
	}
	timedOut, ok := waitTimedOut.Data.(TaskWaitOutput)
	if !ok {
		t.Fatalf("task_wait timeout data = %#v, want TaskWaitOutput", waitTimedOut.Data)
	}
	if !timedOut.TimedOut {
		t.Fatal("task_wait timed_out = false, want true")
	}
	if timedOut.Task.Status != task.TaskStatusRunning {
		t.Fatalf("task_wait status = %q, want %q", timedOut.Task.Status, task.TaskStatusRunning)
	}

	close(release)

	waitCompleted, err := registry.executePreparedRich(context.Background(), registry.prepareCall(Call{
		Name: "task_wait",
		Input: map[string]any{
			"task_id":    created.Text,
			"timeout_ms": 1000,
		},
	}), execCtx)
	if err != nil {
		t.Fatalf("task_wait completion error = %v", err)
	}
	completed, ok := waitCompleted.Data.(TaskWaitOutput)
	if !ok {
		t.Fatalf("task_wait completion data = %#v, want TaskWaitOutput", waitCompleted.Data)
	}
	if completed.TimedOut {
		t.Fatal("task_wait timed_out = true, want false")
	}
	if completed.Task.Status != task.TaskStatusCompleted {
		t.Fatalf("task_wait status = %q, want %q", completed.Task.Status, task.TaskStatusCompleted)
	}
	if completed.ResultReady {
		t.Fatal("task_wait result_ready = true, want false for shell task")
	}
}

func TestTaskResultToolSeparatesAgentFinalTextFromLogs(t *testing.T) {
	root := t.TempDir()
	manager := task.NewManager(
		task.Config{MaxConcurrentTasks: 8, TaskOutputMaxBytes: 1 << 20},
		nil,
		fakeTaskAgentExecutor{
			output:    "我将先检查当前工作区。",
			finalText: "检查完成，未发现阻塞问题。",
		},
	)
	registry := NewRegistry()
	execCtx := ExecContext{
		WorkspaceRoot:    root,
		PermissionEngine: permissions.NewEngine("trusted_local"),
		TaskManager:      manager,
	}

	created, err := registry.Execute(context.Background(), Call{
		Name: "task_create",
		Input: map[string]any{
			"type":    "agent",
			"command": "检查当前工作区并汇报结论",
		},
	}, execCtx)
	if err != nil {
		t.Fatalf("task_create error = %v", err)
	}

	waitForToolTaskTerminal(t, manager, created.Text, root)

	output, err := registry.Execute(context.Background(), Call{
		Name:  "task_output",
		Input: map[string]any{"task_id": created.Text},
	}, execCtx)
	if err != nil {
		t.Fatalf("task_output error = %v", err)
	}
	if output.Text != "我将先检查当前工作区。" {
		t.Fatalf("task_output text = %q, want planning log", output.Text)
	}

	waitResult, err := registry.executePreparedRich(context.Background(), registry.prepareCall(Call{
		Name:  "task_wait",
		Input: map[string]any{"task_id": created.Text},
	}), execCtx)
	if err != nil {
		t.Fatalf("task_wait error = %v", err)
	}
	waited, ok := waitResult.Data.(TaskWaitOutput)
	if !ok {
		t.Fatalf("task_wait data = %#v, want TaskWaitOutput", waitResult.Data)
	}
	if !waited.ResultReady {
		t.Fatal("task_wait result_ready = false, want true")
	}

	result, err := registry.executePreparedRich(context.Background(), registry.prepareCall(Call{
		Name:  "task_result",
		Input: map[string]any{"task_id": created.Text},
	}), execCtx)
	if err != nil {
		t.Fatalf("task_result error = %v", err)
	}
	ready, ok := result.Data.(TaskResultOutput)
	if !ok {
		t.Fatalf("task_result data = %#v, want TaskResultOutput", result.Data)
	}
	if ready.Status != taskResultStatusReady {
		t.Fatalf("task_result status = %q, want %q", ready.Status, taskResultStatusReady)
	}
	if ready.Kind != string(task.FinalResultKindAssistantText) {
		t.Fatalf("task_result kind = %q, want %q", ready.Kind, task.FinalResultKindAssistantText)
	}
	if ready.Text != "检查完成，未发现阻塞问题。" {
		t.Fatalf("task_result text = %q, want final result", ready.Text)
	}

	got, err := registry.Execute(context.Background(), Call{
		Name:  "task_get",
		Input: map[string]any{"task_id": created.Text},
	}, execCtx)
	if err != nil {
		t.Fatalf("task_get error = %v", err)
	}
	if strings.Contains(got.Text, ready.Text) {
		t.Fatalf("task_get leaked final result text: %q", got.Text)
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

func TestSearchToolsAllowSesameGlobalConfigRoot(t *testing.T) {
	home := t.TempDir()
	workspace := t.TempDir()
	globalRoot := filepath.Join(home, ".sesame")
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	if err := os.MkdirAll(globalRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(globalRoot, "alpha.txt"), []byte("hello\nneedle here\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(alpha) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(globalRoot, "beta.txt"), []byte("world\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(beta) error = %v", err)
	}

	runtime := NewRuntime(NewRegistry(), nil)
	execCtx := ExecContext{
		WorkspaceRoot:    workspace,
		GlobalConfigRoot: globalRoot,
		PermissionEngine: permissions.NewEngine(),
	}

	globOutput, err := runtime.ExecuteRich(context.Background(), Call{
		Name:  "glob",
		Input: map[string]any{"pattern": "~/.sesame/*.txt"},
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

	grepOutput, err := runtime.ExecuteRich(context.Background(), Call{
		Name:  "grep",
		Input: map[string]any{"path": "~/.sesame/alpha.txt", "pattern": "needle"},
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

func TestReadOnlyToolsRejectPathsOutsideWorkspaceAndGlobalConfig(t *testing.T) {
	workspace := t.TempDir()
	globalRoot := t.TempDir()
	outside := t.TempDir()
	secretPath := filepath.Join(outside, "secret.txt")
	if err := os.WriteFile(secretPath, []byte("secret"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	registry := NewRegistry()
	_, err := registry.Execute(context.Background(), Call{
		Name:  "file_read",
		Input: map[string]any{"path": secretPath},
	}, ExecContext{
		WorkspaceRoot:    workspace,
		GlobalConfigRoot: globalRoot,
		PermissionEngine: permissions.NewEngine(),
	})
	if err == nil || !strings.Contains(err.Error(), "outside allowed read roots") {
		t.Fatalf("file_read error = %v, want allowed-root rejection", err)
	}

	_, err = NewRuntime(registry, nil).ExecuteRich(context.Background(), Call{
		Name:  "list_dir",
		Input: map[string]any{"path": outside},
	}, ExecContext{
		WorkspaceRoot:    workspace,
		GlobalConfigRoot: globalRoot,
		PermissionEngine: permissions.NewEngine(),
	})
	if err == nil || !strings.Contains(err.Error(), "outside allowed read roots") {
		t.Fatalf("list_dir error = %v, want allowed-root rejection", err)
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

	command := `echo %cd% && for /L %i in (1,1,200) do @echo x`
	if runtime.GOOS != "windows" {
		command = `pwd; i=1; while [ "$i" -le 200 ]; do echo x; i=$((i+1)); done`
	}

	result, err := registry.Execute(context.Background(), Call{
		Name:  "shell_command",
		Input: map[string]any{"command": command},
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

func TestCustomToolHelperProcess(t *testing.T) {
	if strings.TrimSpace(os.Getenv("SESAME_TOOL_NAME")) == "" {
		t.Skip("helper process only")
	}

	mode := ""
	for i, arg := range os.Args {
		if arg == "--" && i+1 < len(os.Args) {
			mode = os.Args[i+1]
			break
		}
	}

	inputBytes, err := io.ReadAll(os.Stdin)
	if err != nil {
		fmt.Fprint(os.Stderr, err.Error())
		os.Exit(2)
	}
	rawInput := strings.TrimSpace(string(inputBytes))
	if rawInput == "" {
		rawInput = "null"
	}

	switch mode {
	case "global-json":
		fmt.Printf(`{"text":"global custom tool","data":{"source":"global"}}`)
		os.Exit(0)
	case "workspace-json":
		fmt.Printf(`{"text":"workspace custom tool","model_text":"workspace custom tool model","data":{"input":%s,"tool_name":"%s","scope":"%s"}}`,
			rawInput,
			os.Getenv("SESAME_TOOL_NAME"),
			os.Getenv("SESAME_TOOL_SCOPE"),
		)
		os.Exit(0)
	case "workspace-override":
		fmt.Printf(`{"text":"workspace override tool","data":{"source":"workspace"}}`)
		os.Exit(0)
	case "stderr-fail":
		fmt.Fprint(os.Stderr, "boom")
		os.Exit(17)
	default:
		fmt.Printf(`{"text":"unknown helper mode","data":{"mode":"%s"}}`, mode)
		os.Exit(0)
	}
}

func writeCustomToolManifest(t *testing.T, path string, manifest map[string]any) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatalf("MarshalIndent() error = %v", err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
