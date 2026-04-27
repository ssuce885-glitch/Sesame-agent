package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go-agent/internal/automation"
	"go-agent/internal/roles"
	"go-agent/internal/runtimegraph"
	"go-agent/internal/store/sqlite"
	"go-agent/internal/task"
)

func newSimpleBuilderTestHarness(t *testing.T) (*automation.Service, ExecContext) {
	t.Helper()

	workspaceRoot := filepath.Join(t.TempDir(), "workspace")
	if err := os.MkdirAll(workspaceRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	store, err := sqlite.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})

	service := automation.NewService(store)
	return service, ExecContext{
		WorkspaceRoot:     workspaceRoot,
		AutomationService: service,
	}
}

func enableAutomationCreateSkills(execCtx *ExecContext) {
	execCtx.ActiveSkillNames = []string{"automation-standard-behavior", "automation-normalizer"}
}

func specialistAutomationContext(roleID string) context.Context {
	return roles.WithSpecialistRoleID(context.Background(), roleID)
}

func TestAutomationCreateSimpleRequiresAutomationSkills(t *testing.T) {
	_, execCtx := newSimpleBuilderTestHarness(t)
	tool := automationCreateSimpleTool{}

	_, err := tool.Execute(context.Background(), Call{
		Name: "automation_create_simple",
		Input: map[string]any{
			"automation_id":    "simple_requires_skills",
			"owner":            "role:doc_cleanup_operator",
			"watch_script":     `printf %s '{"status":"healthy","summary":"ok","facts":{}}'`,
			"interval_seconds": 30,
		},
	}, execCtx)
	if err == nil {
		t.Fatal("expected error when automation skills are not active")
	}
	if !strings.Contains(err.Error(), "skill_use") {
		t.Fatalf("error = %v, want skill_use guidance", err)
	}
}

func TestAutomationCreateSimpleRejectsOwnerTaskModeMutation(t *testing.T) {
	_, execCtx := newSimpleBuilderTestHarness(t)
	enableAutomationCreateSkills(&execCtx)
	manager := task.NewManager(task.Config{WorkspaceStore: nil}, nil, nil)
	created, err := manager.Create(context.Background(), task.CreateTaskInput{
		Type:          task.TaskTypeAgent,
		Command:       "Simple automation task",
		Kind:          "automation_simple",
		Owner:         "role:doc_cleanup_operator",
		WorkspaceRoot: execCtx.WorkspaceRoot,
		Start:         false,
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	execCtx.TaskManager = manager
	execCtx.TurnContext = &runtimegraph.TurnContext{CurrentTaskID: created.ID}
	tool := automationCreateSimpleTool{}

	_, err = tool.Execute(specialistAutomationContext("doc_cleanup_operator"), Call{
		Name: "automation_create_simple",
		Input: map[string]any{
			"automation_id":    "owner_task_must_not_mutate",
			"owner":            "role:doc_cleanup_operator",
			"watch_script":     `printf %s '{"status":"healthy","summary":"ok","facts":{}}'`,
			"interval_seconds": 30,
		},
	}, execCtx)
	if err == nil {
		t.Fatal("expected owner task mode mutation error")
	}
	if !strings.Contains(err.Error(), "Owner Task Mode") {
		t.Fatalf("error = %v, want Owner Task Mode guidance", err)
	}
}

func TestAutomationMutationToolsHiddenInOwnerTaskMode(t *testing.T) {
	_, execCtx := newSimpleBuilderTestHarness(t)
	enableAutomationCreateSkills(&execCtx)
	manager := task.NewManager(task.Config{WorkspaceStore: nil}, nil, nil)
	created, err := manager.Create(context.Background(), task.CreateTaskInput{
		Type:          task.TaskTypeAgent,
		Command:       "Simple automation task",
		Kind:          "automation_simple",
		Owner:         "role:doc_cleanup_operator",
		WorkspaceRoot: execCtx.WorkspaceRoot,
		Start:         false,
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	execCtx.TaskManager = manager
	execCtx.TurnContext = &runtimegraph.TurnContext{CurrentTaskID: created.ID}

	defs := NewRegistry().VisibleDefinitions(execCtx)
	seen := visibleToolNames(defs)
	if seen["automation_create_simple"] {
		t.Fatal("automation_create_simple visible in Owner Task Mode")
	}
	if seen["automation_control"] {
		t.Fatal("automation_control visible in Owner Task Mode")
	}
	if !seen["automation_query"] {
		t.Fatal("automation_query should remain visible in Owner Task Mode")
	}
}

func TestAutomationControlRejectsOwnerTaskModeMutation(t *testing.T) {
	_, execCtx := newSimpleBuilderTestHarness(t)
	execCtx.ActiveSkillNames = []string{"automation-standard-behavior"}
	manager := task.NewManager(task.Config{WorkspaceStore: nil}, nil, nil)
	created, err := manager.Create(context.Background(), task.CreateTaskInput{
		Type:          task.TaskTypeAgent,
		Command:       "Simple automation task",
		Kind:          "automation_simple",
		Owner:         "role:doc_cleanup_operator",
		WorkspaceRoot: execCtx.WorkspaceRoot,
		Start:         false,
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	execCtx.TaskManager = manager
	execCtx.TurnContext = &runtimegraph.TurnContext{CurrentTaskID: created.ID}

	_, err = automationControlTool{}.Execute(context.Background(), Call{
		Name: "automation_control",
		Input: map[string]any{
			"automation_id": "box_txt_cleaner",
			"action":        "resume",
		},
	}, execCtx)
	if err == nil {
		t.Fatal("expected owner task mode control error")
	}
	if !strings.Contains(err.Error(), "Owner Task Mode") {
		t.Fatalf("error = %v, want Owner Task Mode guidance", err)
	}
}

func TestAutomationCreateSimpleRejectsRoleOwnedAutomationFromMainParent(t *testing.T) {
	_, execCtx := newSimpleBuilderTestHarness(t)
	enableAutomationCreateSkills(&execCtx)
	tool := automationCreateSimpleTool{}

	_, err := tool.Execute(context.Background(), Call{
		Name: "automation_create_simple",
		Input: map[string]any{
			"automation_id":    "role_owned_from_parent",
			"owner":            "role:doc_cleanup_operator",
			"watch_script":     `printf %s '{"status":"healthy","summary":"ok","facts":{}}'`,
			"interval_seconds": 30,
		},
	}, execCtx)
	if err == nil {
		t.Fatal("expected role ownership boundary error")
	}
	if !strings.Contains(err.Error(), "owning specialist role") {
		t.Fatalf("error = %v, want owning specialist role guidance", err)
	}
}

func TestAutomationCreateSimpleRejectsWatcherOutputMissingSummary(t *testing.T) {
	_, execCtx := newSimpleBuilderTestHarness(t)
	execCtx.ActiveSkillNames = []string{"automation-standard-behavior", "automation-normalizer"}
	tool := automationCreateSimpleTool{}

	_, err := tool.Execute(specialistAutomationContext("doc_cleanup_operator"), Call{
		Name: "automation_create_simple",
		Input: map[string]any{
			"automation_id":    "simple_missing_summary",
			"owner":            "role:doc_cleanup_operator",
			"watch_script":     `printf %s '{"status":"healthy","facts":{}}'`,
			"interval_seconds": 30,
		},
	}, execCtx)
	if err == nil {
		t.Fatal("expected watcher contract error")
	}
	if !strings.Contains(err.Error(), "summary") {
		t.Fatalf("error = %v, want summary contract failure", err)
	}
}

func TestAutomationCreateSimpleRoleOwnerMaterializesRoleBoundSourceLayout(t *testing.T) {
	service, execCtx := newSimpleBuilderTestHarness(t)
	enableAutomationCreateSkills(&execCtx)
	tool := automationCreateSimpleTool{}
	ctx := specialistAutomationContext("doc_cleanup_operator")

	scriptPath := filepath.Join(execCtx.WorkspaceRoot, "tmp", "detect.sh")
	if err := os.MkdirAll(filepath.Dir(scriptPath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(scriptPath, []byte("#!/usr/bin/env bash\nprintf '%s\\n' '{\"status\":\"healthy\",\"summary\":\"ok\",\"facts\":{}}'\n"), 0o755); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	_, err := tool.Execute(ctx, Call{
		Name: "automation_create_simple",
		Input: map[string]any{
			"automation_id":    "cleanup_docs_a",
			"owner":            "role:doc_cleanup_operator",
			"watch_script":     scriptPath,
			"interval_seconds": 5,
			"title":            "Cleanup Docs A",
			"goal":             "Delete docs/a.txt when it appears.",
		},
	}, execCtx)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	spec, ok, err := service.Get(ctx, "cleanup_docs_a")
	if err != nil || !ok {
		t.Fatalf("Get() ok=%v err=%v", ok, err)
	}
	wantSelector := filepath.ToSlash(filepath.Join("roles", "doc_cleanup_operator", "automations", "cleanup_docs_a", "watch.sh"))
	if got := filepath.ToSlash(spec.Signals[0].Selector); got != wantSelector {
		t.Fatalf("Selector = %q, want %q", got, wantSelector)
	}

	sourceDir := filepath.Join(execCtx.WorkspaceRoot, "roles", "doc_cleanup_operator", "automations", "cleanup_docs_a")
	if _, err := os.Stat(filepath.Join(sourceDir, "watch.sh")); err != nil {
		t.Fatalf("watch.sh missing: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(sourceDir, "automation.yaml"))
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if !strings.Contains(string(raw), "automation_id: cleanup_docs_a") {
		t.Fatalf("automation.yaml = %s", raw)
	}
	if !strings.Contains(string(raw), "owner: role:doc_cleanup_operator") {
		t.Fatalf("automation.yaml = %s", raw)
	}
	if !strings.Contains(string(raw), "report_target: main_agent") {
		t.Fatalf("automation.yaml = %s", raw)
	}
	if !strings.Contains(string(raw), "escalation_target: main_agent") {
		t.Fatalf("automation.yaml = %s", raw)
	}
	if !strings.Contains(string(raw), "state: active") {
		t.Fatalf("automation.yaml = %s", raw)
	}
	if !strings.Contains(string(raw), "kind: poll") {
		t.Fatalf("automation.yaml = %s", raw)
	}
	if !strings.Contains(string(raw), "source: simple_builder:watch_script") {
		t.Fatalf("automation.yaml = %s", raw)
	}
	if !strings.Contains(string(raw), "trigger_on: script_status") {
		t.Fatalf("automation.yaml = %s", raw)
	}
	if !strings.Contains(string(raw), "signal_kind: simple_watcher") {
		t.Fatalf("automation.yaml = %s", raw)
	}
	if !strings.Contains(string(raw), "summary: simple automation watcher match") {
		t.Fatalf("automation.yaml = %s", raw)
	}
	if !strings.Contains(string(raw), "watcher_lifecycle:") {
		t.Fatalf("automation.yaml = %s", raw)
	}
	if !strings.Contains(string(raw), "after_dispatch: pause") {
		t.Fatalf("automation.yaml = %s", raw)
	}
	if !strings.Contains(string(raw), "retrigger_policy:") {
		t.Fatalf("automation.yaml = %s", raw)
	}
	if _, err := os.Stat(scriptPath); !os.IsNotExist(err) {
		t.Fatalf("legacy watch_script still exists, err=%v", err)
	}
	if _, err := os.Stat(filepath.Dir(scriptPath)); !os.IsNotExist(err) {
		t.Fatalf("legacy watch_script directory still exists, err=%v", err)
	}
}

func TestAutomationCreateSimpleRoleOwnerDoesNotMaterializeSourceLayoutOnWatcherFailure(t *testing.T) {
	_, execCtx := newSimpleBuilderTestHarness(t)
	enableAutomationCreateSkills(&execCtx)
	tool := automationCreateSimpleTool{}

	_, err := tool.Execute(specialistAutomationContext("doc_cleanup_operator"), Call{
		Name: "automation_create_simple",
		Input: map[string]any{
			"automation_id":    "bad_role_watcher",
			"owner":            "role:doc_cleanup_operator",
			"watch_script":     `printf %s '{"triggered":false}'`,
			"interval_seconds": 5,
		},
	}, execCtx)
	if err == nil {
		t.Fatal("expected watcher contract error")
	}

	sourceDir := filepath.Join(execCtx.WorkspaceRoot, "roles", "doc_cleanup_operator", "automations", "bad_role_watcher")
	if _, statErr := os.Stat(sourceDir); !os.IsNotExist(statErr) {
		t.Fatalf("source layout was materialized despite watcher failure, stat err=%v", statErr)
	}
}

func TestAutomationCreateSimpleRejectsSelfReferentialRoleWatcher(t *testing.T) {
	service, execCtx := newSimpleBuilderTestHarness(t)
	enableAutomationCreateSkills(&execCtx)
	tool := automationCreateSimpleTool{}

	automationID := "self_recursive_watcher"
	canonicalPath := filepath.Join(execCtx.WorkspaceRoot, "roles", "doc_cleanup_operator", "automations", automationID, "watch.sh")
	_, err := tool.Execute(specialistAutomationContext("doc_cleanup_operator"), Call{
		Name: "automation_create_simple",
		Input: map[string]any{
			"automation_id":    automationID,
			"owner":            "role:doc_cleanup_operator",
			"watch_script":     "bash " + canonicalPath,
			"interval_seconds": 5,
		},
	}, execCtx)
	if err == nil {
		t.Fatal("expected self-referential watcher rejection")
	}
	if !strings.Contains(err.Error(), "must not invoke or copy its own canonical role-owned watch.sh path") {
		t.Fatalf("error = %v, want self-reference guidance", err)
	}

	spec, ok, err := service.Get(context.Background(), automationID)
	if err != nil || ok {
		t.Fatalf("Get() ok=%v err=%v spec=%#v", ok, err, spec)
	}
	sourceDir := filepath.Dir(canonicalPath)
	if _, statErr := os.Stat(sourceDir); !os.IsNotExist(statErr) {
		t.Fatalf("source layout was materialized despite self-reference, stat err=%v", statErr)
	}
}

func TestAutomationCreateSimpleRejectsMainAgentOwner(t *testing.T) {
	_, execCtx := newSimpleBuilderTestHarness(t)
	enableAutomationCreateSkills(&execCtx)
	tool := automationCreateSimpleTool{}

	_, err := tool.Execute(specialistAutomationContext("log_repairer"), Call{
		Name: "automation_create_simple",
		Input: map[string]any{
			"automation_id":    "simple_main",
			"owner":            "main_agent",
			"watch_script":     `printf %s '{"status":"healthy","summary":"ok","facts":{}}'`,
			"interval_seconds": 30,
		},
	}, execCtx)
	if err == nil {
		t.Fatal("expected main_agent owner rejection")
	}
	if !strings.Contains(err.Error(), "role:<role_id>") {
		t.Fatalf("error = %v, want role owner guidance", err)
	}
}

func TestAutomationCreateSimpleRejectedMainAgentOwnerDoesNotMaterializeRoleBoundLayout(t *testing.T) {
	service, execCtx := newSimpleBuilderTestHarness(t)
	enableAutomationCreateSkills(&execCtx)
	tool := automationCreateSimpleTool{}

	command := `printf %s '{"status":"healthy","summary":"ok","facts":{}}'`
	_, err := tool.Execute(specialistAutomationContext("log_repairer"), Call{
		Name: "automation_create_simple",
		Input: map[string]any{
			"automation_id":    "simple_main",
			"owner":            "main_agent",
			"watch_script":     command,
			"interval_seconds": 30,
		},
	}, execCtx)
	if err == nil {
		t.Fatal("expected main_agent owner rejection")
	}

	spec, ok, err := service.Get(context.Background(), "simple_main")
	if err != nil || ok {
		t.Fatalf("Get() ok=%v err=%v spec=%#v", ok, err, spec)
	}
	if _, err := os.Stat(filepath.Join(execCtx.WorkspaceRoot, "roles")); !os.IsNotExist(err) {
		t.Fatalf("unexpected role-owned source layout created, err=%v", err)
	}
}

func TestAutomationCreateSimpleRoleOwnerDefaultsReportTargetToMainAgent(t *testing.T) {
	service, execCtx := newSimpleBuilderTestHarness(t)
	enableAutomationCreateSkills(&execCtx)
	tool := automationCreateSimpleTool{}
	ctx := specialistAutomationContext("log_repairer")

	_, err := tool.Execute(ctx, Call{
		Name: "automation_create_simple",
		Input: map[string]any{
			"automation_id":    "simple_role",
			"owner":            "role:log_repairer",
			"watch_script":     `echo '{"status":"needs_agent","summary":"repair","facts":{"path":"a.txt"},"dedupe_key":"file:a.txt"}'`,
			"interval_seconds": 45,
		},
	}, execCtx)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	spec, ok, err := service.Get(ctx, "simple_role")
	if err != nil || !ok {
		t.Fatalf("Get() ok=%v err=%v", ok, err)
	}
	if spec.Owner != "role:log_repairer" {
		t.Fatalf("Owner = %q", spec.Owner)
	}
	if spec.ReportTarget != "main_agent" {
		t.Fatalf("ReportTarget = %q", spec.ReportTarget)
	}
	if spec.EscalationTarget != "main_agent" {
		t.Fatalf("EscalationTarget = %q", spec.EscalationTarget)
	}
	if spec.SimplePolicy.OnSuccess != "continue" {
		t.Fatalf("SimplePolicy.OnSuccess = %q", spec.SimplePolicy.OnSuccess)
	}
	if spec.SimplePolicy.OnFailure != "pause" {
		t.Fatalf("SimplePolicy.OnFailure = %q", spec.SimplePolicy.OnFailure)
	}
	if spec.SimplePolicy.OnBlocked != "escalate" {
		t.Fatalf("SimplePolicy.OnBlocked = %q", spec.SimplePolicy.OnBlocked)
	}
}

func TestAutomationCreateSimpleRejectsInvalidPolicyValue(t *testing.T) {
	_, execCtx := newSimpleBuilderTestHarness(t)
	enableAutomationCreateSkills(&execCtx)
	tool := automationCreateSimpleTool{}

	_, err := tool.Execute(specialistAutomationContext("log_repairer"), Call{
		Name: "automation_create_simple",
		Input: map[string]any{
			"automation_id":    "simple_bad_policy",
			"owner":            "role:log_repairer",
			"watch_script":     `echo '{"status":"healthy","summary":"ok","facts":{}}'`,
			"interval_seconds": 60,
			"simple_policy": map[string]any{
				"on_failure": "invalid_action",
			},
		},
	}, execCtx)
	if err == nil {
		t.Fatal("expected error for invalid simple_policy.on_failure")
	}
	if !strings.Contains(err.Error(), "simple_policy.on_failure must be one of continue, pause, escalate") {
		t.Fatalf("error = %v", err)
	}
}

func TestAutomationCreateSimpleRejectsInvalidOnSuccessPolicyValue(t *testing.T) {
	_, execCtx := newSimpleBuilderTestHarness(t)
	enableAutomationCreateSkills(&execCtx)
	tool := automationCreateSimpleTool{}

	_, err := tool.Execute(specialistAutomationContext("log_repairer"), Call{
		Name: "automation_create_simple",
		Input: map[string]any{
			"automation_id":    "simple_bad_policy_on_success",
			"owner":            "role:log_repairer",
			"watch_script":     `echo '{"status":"healthy","summary":"ok","facts":{}}'`,
			"interval_seconds": 60,
			"simple_policy": map[string]any{
				"on_success": "invalid_action",
			},
		},
	}, execCtx)
	if err == nil {
		t.Fatal("expected error for invalid simple_policy.on_success")
	}
	if !strings.Contains(err.Error(), "simple_policy.on_success must be one of continue, pause, escalate") {
		t.Fatalf("error = %v", err)
	}
}

func TestAutomationCreateSimpleRejectsInvalidOnBlockedPolicyValue(t *testing.T) {
	_, execCtx := newSimpleBuilderTestHarness(t)
	enableAutomationCreateSkills(&execCtx)
	tool := automationCreateSimpleTool{}

	_, err := tool.Execute(specialistAutomationContext("log_repairer"), Call{
		Name: "automation_create_simple",
		Input: map[string]any{
			"automation_id":    "simple_bad_policy_on_blocked",
			"owner":            "role:log_repairer",
			"watch_script":     `echo '{"status":"healthy","summary":"ok","facts":{}}'`,
			"interval_seconds": 60,
			"simple_policy": map[string]any{
				"on_blocked": "invalid_action",
			},
		},
	}, execCtx)
	if err == nil {
		t.Fatal("expected error for invalid simple_policy.on_blocked")
	}
	if !strings.Contains(err.Error(), "simple_policy.on_blocked must be one of continue, pause, escalate") {
		t.Fatalf("error = %v", err)
	}
}

func TestAutomationCreateSimpleRejectsInvalidReportTarget(t *testing.T) {
	_, execCtx := newSimpleBuilderTestHarness(t)
	enableAutomationCreateSkills(&execCtx)
	tool := automationCreateSimpleTool{}

	_, err := tool.Execute(specialistAutomationContext("log_repairer"), Call{
		Name: "automation_create_simple",
		Input: map[string]any{
			"automation_id":    "simple_bad_report_target",
			"owner":            "role:log_repairer",
			"watch_script":     `echo '{"status":"healthy","summary":"ok","facts":{}}'`,
			"interval_seconds": 60,
			"report_target":    "role:supervisor",
		},
	}, execCtx)
	if err == nil {
		t.Fatal("expected error for invalid report_target")
	}
	if !strings.Contains(err.Error(), "report_target must be main_agent") {
		t.Fatalf("error = %v", err)
	}
}

func TestAutomationCreateSimpleRejectsInvalidEscalationTarget(t *testing.T) {
	_, execCtx := newSimpleBuilderTestHarness(t)
	enableAutomationCreateSkills(&execCtx)
	tool := automationCreateSimpleTool{}

	_, err := tool.Execute(specialistAutomationContext("log_repairer"), Call{
		Name: "automation_create_simple",
		Input: map[string]any{
			"automation_id":     "simple_bad_escalation_target",
			"owner":             "role:log_repairer",
			"watch_script":      `echo '{"status":"healthy","summary":"ok","facts":{}}'`,
			"interval_seconds":  60,
			"escalation_target": "role:supervisor",
		},
	}, execCtx)
	if err == nil {
		t.Fatal("expected error for invalid escalation_target")
	}
	if !strings.Contains(err.Error(), "escalation_target must be main_agent") {
		t.Fatalf("error = %v", err)
	}
}

func TestAutomationCreateSimpleRejectsMalformedOwner(t *testing.T) {
	_, execCtx := newSimpleBuilderTestHarness(t)
	enableAutomationCreateSkills(&execCtx)
	tool := automationCreateSimpleTool{}

	_, err := tool.Execute(context.Background(), Call{
		Name: "automation_create_simple",
		Input: map[string]any{
			"automation_id":    "simple_bad_owner",
			"owner":            "role:",
			"watch_script":     `echo '{"status":"healthy","summary":"ok","facts":{}}'`,
			"interval_seconds": 60,
		},
	}, execCtx)
	if err == nil {
		t.Fatal("expected error for malformed owner")
	}
	if !strings.Contains(err.Error(), "owner must be role:<role_id>") {
		t.Fatalf("error = %v", err)
	}
}

func newRoleToolExecContext(t *testing.T) ExecContext {
	t.Helper()
	return ExecContext{
		WorkspaceRoot: t.TempDir(),
		RoleService:   roles.NewService(),
	}
}

func TestRoleCreateAddsAutomationBoundaryBaselineWithoutBlanketAutomationBan(t *testing.T) {
	execCtx := newRoleToolExecContext(t)
	output, err := roleCreateTool{}.ExecuteDecoded(context.Background(), DecodedCall{
		Input: RoleUpsertInput{
			RoleID: "doc_cleanup_operator",
			Prompt: "Clean up documents when assigned.",
		},
	}, execCtx)
	if err != nil {
		t.Fatalf("ExecuteDecoded() error = %v", err)
	}

	roleOutput := output.Data.(RoleOutput)
	assertAutomationBoundaryBaseline(t, roleOutput.Prompt)
}

func TestRoleUpdateAddsAutomationBoundaryBaselineWithoutBlanketAutomationBan(t *testing.T) {
	execCtx := newRoleToolExecContext(t)
	service := execCtx.RoleService.(*roles.Service)
	if _, err := service.Create(execCtx.WorkspaceRoot, roles.UpsertInput{
		RoleID: "doc_cleanup_operator",
		Prompt: "Initial prompt.",
	}); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	output, err := roleUpdateTool{}.ExecuteDecoded(context.Background(), DecodedCall{
		Input: RoleUpsertInput{
			RoleID: "doc_cleanup_operator",
			Prompt: "Updated prompt.",
		},
	}, execCtx)
	if err != nil {
		t.Fatalf("ExecuteDecoded() error = %v", err)
	}

	roleOutput := output.Data.(RoleOutput)
	assertAutomationBoundaryBaseline(t, roleOutput.Prompt)
}

func assertAutomationBoundaryBaseline(t *testing.T, prompt string) {
	t.Helper()
	required := []string{
		"# Automation boundaries",
		"Create Automation Mode",
		"Owner Task Mode",
		"Status/Report Mode",
		"activate automation-standard-behavior and automation-normalizer",
		"automation_control",
	}
	for _, text := range required {
		if !strings.Contains(prompt, text) {
			t.Fatalf("prompt missing %q:\n%s", text, prompt)
		}
	}
	if strings.Contains(prompt, "Do not create or modify automations") {
		t.Fatalf("prompt contains blanket automation ban:\n%s", prompt)
	}
}
