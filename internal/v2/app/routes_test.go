package app

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"go-agent/internal/config"
	"go-agent/internal/v2/automation"
	"go-agent/internal/v2/contextsvc"
	"go-agent/internal/v2/contracts"
	"go-agent/internal/v2/memory"
	"go-agent/internal/v2/roles"
	v2store "go-agent/internal/v2/store"
	"go-agent/internal/v2/workflows"
)

func TestRoutesStatusAndRoleCRUD(t *testing.T) {
	ctx := context.Background()
	s, err := v2store.OpenInMemory()
	if err != nil {
		t.Fatalf("OpenInMemory: %v", err)
	}
	defer s.Close()

	workspaceRoot := t.TempDir()
	now := time.Now().UTC()
	session := contracts.Session{
		ID:                "session-1",
		WorkspaceRoot:     workspaceRoot,
		SystemPrompt:      "You are Sesame.",
		PermissionProfile: "workspace",
		State:             "idle",
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	if err := s.Sessions().Create(ctx, session); err != nil {
		t.Fatalf("create session: %v", err)
	}
	sessionMgr := &testSessionManager{}
	sessionMgr.Register(session)

	handler := (&routes{
		cfg: config.Config{
			Addr:              "127.0.0.1:8421",
			Model:             "test-model",
			PermissionProfile: "workspace",
			Paths: config.Paths{
				WorkspaceRoot: workspaceRoot,
				DataDir:       filepath.Join(workspaceRoot, ".sesame"),
			},
		},
		store:            s,
		sessionMgr:       sessionMgr,
		roleService:      roles.NewService(workspaceRoot),
		defaultSessionID: session.ID,
	}).handler()

	status := decodeJSON[map[string]any](t, handler, http.MethodGet, "/v2/status", nil, http.StatusOK)
	if status["default_session_id"] != session.ID {
		t.Fatalf("default_session_id = %v, want %s", status["default_session_id"], session.ID)
	}

	rolePayload := map[string]any{
		"id":                 "smoke_role",
		"name":               "Smoke Role",
		"system_prompt":      "You are a smoke role.",
		"permission_profile": "workspace",
		"model":              "test-role-model",
		"max_tool_calls":     3,
		"max_runtime":        60,
		"skill_names":        []string{"email"},
	}
	created := decodeJSON[roles.RoleSpec](t, handler, http.MethodPost, "/v2/roles", rolePayload, http.StatusCreated)
	if created.ID != "smoke_role" || created.Name != "Smoke Role" || created.Model != "test-role-model" {
		t.Fatalf("created role = %+v", created)
	}

	fetched := decodeJSON[roles.RoleSpec](t, handler, http.MethodGet, "/v2/roles/smoke_role", nil, http.StatusOK)
	if fetched.ID != created.ID || fetched.SystemPrompt != "You are a smoke role." {
		t.Fatalf("fetched role = %+v", fetched)
	}

	rolePayload["name"] = "Smoke Role Updated"
	rolePayload["system_prompt"] = "Updated role prompt."
	updated := decodeJSON[roles.RoleSpec](t, handler, http.MethodPut, "/v2/roles/smoke_role", rolePayload, http.StatusOK)
	if updated.Name != "Smoke Role Updated" || updated.SystemPrompt != "Updated role prompt." || updated.Version != 2 {
		t.Fatalf("updated role = %+v", updated)
	}
}

func TestContextPreviewIncludesPromptAndAvailableBlocks(t *testing.T) {
	ctx := context.Background()
	s, err := v2store.OpenInMemory()
	if err != nil {
		t.Fatalf("OpenInMemory: %v", err)
	}
	defer s.Close()

	workspaceRoot := t.TempDir()
	now := time.Date(2026, 5, 3, 1, 2, 3, 0, time.UTC)
	session := contracts.Session{
		ID:                "session-ctx",
		WorkspaceRoot:     workspaceRoot,
		SystemPrompt:      "Session prompt.",
		PermissionProfile: "workspace",
		State:             "idle",
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	if err := s.Sessions().Create(ctx, session); err != nil {
		t.Fatalf("create session: %v", err)
	}
	if err := s.ProjectStates().Upsert(ctx, contracts.ProjectState{
		WorkspaceRoot: workspaceRoot,
		Summary:       "Ship context inspector.",
		CreatedAt:     now,
		UpdatedAt:     now,
	}); err != nil {
		t.Fatalf("upsert project state: %v", err)
	}
	if err := s.Messages().Append(ctx, []contracts.Message{
		{SessionID: session.ID, TurnID: "turn-1", Role: "user", Content: "What is active?", Position: 1, CreatedAt: now},
		{SessionID: session.ID, TurnID: "turn-1", Role: "assistant", Content: "Context work is active.", Position: 2, CreatedAt: now},
	}); err != nil {
		t.Fatalf("append messages: %v", err)
	}
	if err := s.Memories().Create(ctx, contracts.Memory{
		ID:            "memory-1",
		WorkspaceRoot: workspaceRoot,
		Kind:          "decision",
		Content:       "Keep preview read-only first.",
		Source:        "manual",
		Confidence:    0.9,
		CreatedAt:     now,
		UpdatedAt:     now,
	}); err != nil {
		t.Fatalf("create memory: %v", err)
	}
	if err := s.ContextBlocks().Create(ctx, contracts.ContextBlock{
		ID:              "ctxblk-1",
		WorkspaceRoot:   workspaceRoot,
		Type:            "decision",
		Owner:           "workspace",
		Visibility:      "global",
		SourceRef:       "message:1",
		Title:           "Indexed decision",
		Summary:         "Context blocks stay out of prompt until selected.",
		Confidence:      0.9,
		ImportanceScore: 0.8,
		CreatedAt:       now,
		UpdatedAt:       now,
	}); err != nil {
		t.Fatalf("create context block: %v", err)
	}
	if err := s.Reports().Create(ctx, contracts.Report{
		ID:         "report-1",
		SessionID:  session.ID,
		SourceKind: "task_result",
		SourceID:   "task-1",
		Status:     "completed",
		Severity:   "info",
		Title:      "Task report",
		Summary:    "Context inspector research completed.",
		Delivered:  false,
		CreatedAt:  now,
	}); err != nil {
		t.Fatalf("create report: %v", err)
	}
	sessionMgr := &testSessionManager{}
	sessionMgr.Register(session)

	handler := (&routes{
		cfg: config.Config{
			SystemPrompt:      "You are Sesame.",
			PermissionProfile: "workspace",
			Paths: config.Paths{
				WorkspaceRoot: workspaceRoot,
				DataDir:       filepath.Join(workspaceRoot, ".sesame"),
			},
		},
		store:            s,
		sessionMgr:       sessionMgr,
		memoryService:    memory.NewService(s),
		defaultSessionID: session.ID,
	}).handler()

	preview := decodeJSON[contextsvc.PreviewResponse](t, handler, http.MethodGet, "/v2/context/preview?session_id=session-ctx", nil, http.StatusOK)
	if preview.SessionID != session.ID || preview.WorkspaceRoot != workspaceRoot {
		t.Fatalf("preview identity = %+v", preview)
	}
	if preview.ApproxTokens <= 0 || len(preview.Prompt) < 3 {
		t.Fatalf("preview prompt = %+v, tokens %d", preview.Prompt, preview.ApproxTokens)
	}
	if !hasContextBlock(preview.Blocks, "project_state", "included") {
		t.Fatalf("missing included project state block: %+v", preview.Blocks)
	}
	if !hasContextBlock(preview.Blocks, "ctxblk-1", "available") {
		t.Fatalf("missing available context block: %+v", preview.Blocks)
	}
	if !hasContextBlock(preview.Blocks, "memory-1", "available") {
		t.Fatalf("missing available memory block: %+v", preview.Blocks)
	}
	if !hasContextBlock(preview.Blocks, "report-1", "available") {
		t.Fatalf("missing available report block: %+v", preview.Blocks)
	}
}

func TestAutomationRoutesIncludeWorkflowFields(t *testing.T) {
	ctx := context.Background()
	s, err := v2store.OpenInMemory()
	if err != nil {
		t.Fatalf("OpenInMemory: %v", err)
	}
	defer s.Close()

	workspaceRoot := t.TempDir()
	handler := (&routes{
		cfg: config.Config{
			PermissionProfile: "workspace",
			Paths: config.Paths{
				WorkspaceRoot: workspaceRoot,
				DataDir:       filepath.Join(workspaceRoot, ".sesame"),
			},
		},
		store:             s,
		automationService: automation.NewService(s, nil, nil),
	}).handler()

	created := decodeJSON[contracts.Automation](t, handler, http.MethodPost, "/v2/automations", map[string]any{
		"title":        "Watch docs",
		"goal":         "Keep docs fresh",
		"owner":        "main",
		"watcher_path": "watcher.sh",
		"workflow_id":  "workflow-docs",
	}, http.StatusCreated)
	if created.ID == "" || created.WorkspaceRoot != workspaceRoot || created.WorkflowID != "workflow-docs" {
		t.Fatalf("created automation = %+v", created)
	}

	list := decodeJSON[[]contracts.Automation](t, handler, http.MethodGet, "/v2/automations?workspace_root="+workspaceRoot, nil, http.StatusOK)
	if len(list) != 1 || list[0].ID != created.ID || list[0].WorkflowID != "workflow-docs" {
		t.Fatalf("listed automations = %+v", list)
	}

	now := time.Now().UTC()
	if err := s.Automations().CreateRun(ctx, contracts.AutomationRun{
		AutomationID:  created.ID,
		DedupeKey:     "docs-stale",
		WorkflowRunID: "wfrun-1",
		Status:        "workflow:completed",
		Summary:       "Docs workflow completed",
		CreatedAt:     now,
	}); err != nil {
		t.Fatalf("create automation run: %v", err)
	}

	runs := decodeJSON[[]contracts.AutomationRun](t, handler, http.MethodGet, "/v2/automations/"+created.ID+"/runs?limit=10", nil, http.StatusOK)
	if len(runs) != 1 || runs[0].WorkflowRunID != "wfrun-1" || runs[0].TaskID != "" {
		t.Fatalf("listed automation runs = %+v", runs)
	}
}

func TestContextBlockRoutesCRUD(t *testing.T) {
	ctx := context.Background()
	s, err := v2store.OpenInMemory()
	if err != nil {
		t.Fatalf("OpenInMemory: %v", err)
	}
	defer s.Close()

	workspaceRoot := t.TempDir()
	session := contracts.Session{
		ID:                "session-blocks",
		WorkspaceRoot:     workspaceRoot,
		SystemPrompt:      "You are Sesame.",
		PermissionProfile: "workspace",
		State:             "idle",
		CreatedAt:         time.Now().UTC(),
		UpdatedAt:         time.Now().UTC(),
	}
	if err := s.Sessions().Create(ctx, session); err != nil {
		t.Fatalf("create session: %v", err)
	}
	sessionMgr := &testSessionManager{}
	sessionMgr.Register(session)
	handler := (&routes{
		cfg: config.Config{
			PermissionProfile: "workspace",
			Paths: config.Paths{
				WorkspaceRoot: workspaceRoot,
				DataDir:       filepath.Join(workspaceRoot, ".sesame"),
			},
		},
		store:            s,
		sessionMgr:       sessionMgr,
		defaultSessionID: session.ID,
	}).handler()

	created := decodeJSON[contracts.ContextBlock](t, handler, http.MethodPost, "/v2/context/blocks", map[string]any{
		"type":             "decision",
		"source_ref":       "message:7",
		"title":            "Decision",
		"summary":          "Use ContextBlock as an index.",
		"importance_score": 0.75,
	}, http.StatusCreated)
	if created.ID == "" || created.WorkspaceRoot != workspaceRoot || created.Owner != "workspace" {
		t.Fatalf("created context block = %+v", created)
	}

	updated := decodeJSON[contracts.ContextBlock](t, handler, http.MethodPut, "/v2/context/blocks/"+created.ID, map[string]any{
		"workspace_root":     workspaceRoot,
		"type":               "warning",
		"owner":              "main_session",
		"visibility":         "session",
		"source_ref":         "report:1",
		"title":              "Updated",
		"evidence":           "Check stale context.",
		"confidence":         0.6,
		"importance_score":   0.5,
		"expiry_policy":      "manual",
		"unknown_ignored_ok": true,
	}, http.StatusOK)
	if updated.Type != "warning" || updated.Owner != "main_session" || updated.Summary != "Use ContextBlock as an index." || updated.Evidence == "" {
		t.Fatalf("updated context block = %+v", updated)
	}

	patched := decodeJSON[contracts.ContextBlock](t, handler, http.MethodPut, "/v2/context/blocks/"+created.ID, map[string]any{
		"title":      "Patched",
		"confidence": 0,
	}, http.StatusOK)
	if patched.Title != "Patched" || patched.Type != "warning" || patched.Owner != "main_session" || patched.Evidence == "" || patched.Confidence != 0 {
		t.Fatalf("patched context block = %+v", patched)
	}

	list := decodeJSON[[]contracts.ContextBlock](t, handler, http.MethodGet, "/v2/context/blocks?workspace_root="+workspaceRoot+"&owner=main_session", nil, http.StatusOK)
	if len(list) != 1 || list[0].ID != created.ID {
		t.Fatalf("list context blocks = %+v", list)
	}

	deleted := decodeJSON[map[string]string](t, handler, http.MethodDelete, "/v2/context/blocks/"+created.ID, nil, http.StatusOK)
	if deleted["status"] != "deleted" {
		t.Fatalf("delete response = %+v", deleted)
	}
}

func TestContextRoutesErrorMapping(t *testing.T) {
	ctx := context.Background()
	s, err := v2store.OpenInMemory()
	if err != nil {
		t.Fatalf("OpenInMemory: %v", err)
	}
	defer s.Close()

	workspaceRoot := t.TempDir()
	session := contracts.Session{
		ID:                "session-errors",
		WorkspaceRoot:     workspaceRoot,
		SystemPrompt:      "You are Sesame.",
		PermissionProfile: "workspace",
		State:             "idle",
		CreatedAt:         time.Now().UTC(),
		UpdatedAt:         time.Now().UTC(),
	}
	if err := s.Sessions().Create(ctx, session); err != nil {
		t.Fatalf("create session: %v", err)
	}
	sessionMgr := &testSessionManager{}
	sessionMgr.Register(session)
	handler := (&routes{
		cfg: config.Config{
			PermissionProfile: "workspace",
			Paths: config.Paths{
				WorkspaceRoot: workspaceRoot,
				DataDir:       filepath.Join(workspaceRoot, ".sesame"),
			},
		},
		store:            s,
		sessionMgr:       sessionMgr,
		defaultSessionID: session.ID,
	}).handler()

	_ = decodeJSON[map[string]string](t, handler, http.MethodGet, "/v2/context/preview?session_id=missing", nil, http.StatusNotFound)
	_ = decodeJSON[map[string]string](t, handler, http.MethodPost, "/v2/context/blocks", map[string]any{
		"title": "Missing content",
	}, http.StatusBadRequest)
	_ = decodeJSON[map[string]string](t, handler, http.MethodPut, "/v2/context/blocks/missing", map[string]any{
		"title": "No block",
	}, http.StatusNotFound)

	badPromptHandler := (&routes{
		cfg: config.Config{
			SystemPromptFile: filepath.Join(workspaceRoot, "missing-prompt.md"),
			Paths: config.Paths{
				WorkspaceRoot: workspaceRoot,
				DataDir:       filepath.Join(workspaceRoot, ".sesame"),
			},
		},
		store:            s,
		sessionMgr:       sessionMgr,
		defaultSessionID: session.ID,
	}).handler()
	_ = decodeJSON[map[string]string](t, badPromptHandler, http.MethodGet, "/v2/context/preview?session_id="+session.ID, nil, http.StatusInternalServerError)
}

func TestWorkflowRoutesCRUDAndRuns(t *testing.T) {
	s, err := v2store.OpenInMemory()
	if err != nil {
		t.Fatalf("OpenInMemory: %v", err)
	}
	defer s.Close()

	workspaceRoot := t.TempDir()
	handler := (&routes{
		cfg: config.Config{
			Paths: config.Paths{
				WorkspaceRoot: workspaceRoot,
				DataDir:       filepath.Join(workspaceRoot, ".sesame"),
			},
		},
		store:      s,
		sessionMgr: &testSessionManager{},
	}).handler()

	created := decodeJSON[contracts.Workflow](t, handler, http.MethodPost, "/v2/workflows", map[string]any{
		"id":             "client-workflow",
		"workspace_root": "/other",
		"name":           "Review flow",
		"trigger":        "manual",
		"owner_role":     "reviewer",
		"steps":          `[{"kind":"task","name":"review"}]`,
		"required_tools": `["git"]`,
	}, http.StatusCreated)
	if created.ID == "" || created.ID == "client-workflow" || created.WorkspaceRoot != workspaceRoot {
		t.Fatalf("created workflow identity = %+v", created)
	}
	if created.Name != "Review flow" || created.Trigger != "manual" {
		t.Fatalf("created workflow = %+v", created)
	}

	fetched := decodeJSON[contracts.Workflow](t, handler, http.MethodGet, "/v2/workflows/"+created.ID, nil, http.StatusOK)
	if fetched.ID != created.ID || fetched.Steps == "" {
		t.Fatalf("fetched workflow = %+v", fetched)
	}

	updated := decodeJSON[contracts.Workflow](t, handler, http.MethodPut, "/v2/workflows/"+created.ID, map[string]any{
		"id":              "client-update",
		"workspace_root":  "/other",
		"name":            "Review flow updated",
		"trigger":         "file_change",
		"approval_policy": `{"required":true}`,
	}, http.StatusOK)
	if updated.ID != created.ID || updated.WorkspaceRoot != workspaceRoot || updated.Name != "Review flow updated" {
		t.Fatalf("updated workflow = %+v", updated)
	}

	workflows := decodeJSON[[]contracts.Workflow](t, handler, http.MethodGet, "/v2/workflows?workspace_root="+workspaceRoot, nil, http.StatusOK)
	if len(workflows) != 1 || workflows[0].ID != created.ID {
		t.Fatalf("list workflows = %+v", workflows)
	}

	run := decodeJSON[contracts.WorkflowRun](t, handler, http.MethodPost, "/v2/workflow_runs", map[string]any{
		"id":             "client-run",
		"workflow_id":    created.ID,
		"workspace_root": "/other",
		"state":          "running",
		"trigger_ref":    "manual:1",
		"task_ids":       `["task-1"]`,
		"report_ids":     `["report-1"]`,
		"approval_ids":   `["approval-1"]`,
		"trace":          `[{"event":"started"}]`,
	}, http.StatusCreated)
	if run.ID == "" || run.ID == "client-run" || run.WorkflowID != created.ID || run.WorkspaceRoot != workspaceRoot {
		t.Fatalf("created workflow run identity = %+v", run)
	}
	if run.State != "running" || run.Trace == "" {
		t.Fatalf("created workflow run = %+v", run)
	}

	updatedRun := decodeJSON[contracts.WorkflowRun](t, handler, http.MethodPut, "/v2/workflow_runs/"+run.ID, map[string]any{
		"id":             "client-update-run",
		"workflow_id":    "other-workflow",
		"workspace_root": "/other",
		"state":          "completed",
		"trace":          `[{"event":"completed"}]`,
	}, http.StatusOK)
	if updatedRun.ID != run.ID || updatedRun.WorkflowID != created.ID || updatedRun.WorkspaceRoot != workspaceRoot || updatedRun.State != "completed" {
		t.Fatalf("updated workflow run = %+v", updatedRun)
	}

	runs := decodeJSON[[]contracts.WorkflowRun](t, handler, http.MethodGet, "/v2/workflow_runs?workspace_root="+workspaceRoot+"&workflow_id="+created.ID+"&state=completed", nil, http.StatusOK)
	if len(runs) != 1 || runs[0].ID != run.ID {
		t.Fatalf("list workflow runs = %+v", runs)
	}
}

func TestApprovalRoutesCRUDAndWorkspaceBoundary(t *testing.T) {
	ctx := context.Background()
	s, err := v2store.OpenInMemory()
	if err != nil {
		t.Fatalf("OpenInMemory: %v", err)
	}
	defer s.Close()

	workspaceRoot := t.TempDir()
	now := time.Now().UTC()
	handler := (&routes{
		cfg: config.Config{
			Paths: config.Paths{
				WorkspaceRoot: workspaceRoot,
				DataDir:       filepath.Join(workspaceRoot, ".sesame"),
			},
		},
		store:      s,
		sessionMgr: &testSessionManager{},
	}).handler()

	workflow := contracts.Workflow{
		ID:            "workflow-approval-local",
		WorkspaceRoot: workspaceRoot,
		Name:          "Approval local workflow",
		Trigger:       "manual",
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := s.Workflows().Create(ctx, workflow); err != nil {
		t.Fatalf("create workflow: %v", err)
	}
	run := contracts.WorkflowRun{
		ID:            "run-approval-local",
		WorkflowID:    workflow.ID,
		WorkspaceRoot: workspaceRoot,
		State:         "waiting_approval",
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := s.Workflows().CreateRun(ctx, run); err != nil {
		t.Fatalf("create run: %v", err)
	}

	created := decodeJSON[contracts.Approval](t, handler, http.MethodPost, "/v2/approvals", map[string]any{
		"id":               "client-approval",
		"workflow_run_id":  run.ID,
		"workspace_root":   "/other",
		"requested_action": "deploy release",
		"risk_level":       "high",
		"summary":          "Deploy to production",
		"proposed_payload": `{"environment":"prod"}`,
		"state":            "pending",
	}, http.StatusCreated)
	if created.ID == "" || created.ID == "client-approval" {
		t.Fatalf("created approval id = %q", created.ID)
	}
	if created.WorkflowRunID != run.ID || created.WorkspaceRoot != workspaceRoot || created.RequestedAction != "deploy release" {
		t.Fatalf("created approval = %+v", created)
	}

	fetched := decodeJSON[contracts.Approval](t, handler, http.MethodGet, "/v2/approvals/"+created.ID, nil, http.StatusOK)
	if fetched.ID != created.ID || fetched.State != "pending" {
		t.Fatalf("fetched approval = %+v", fetched)
	}

	decidedAt := now.Add(time.Hour).Format(time.RFC3339Nano)
	updated := decodeJSON[contracts.Approval](t, handler, http.MethodPut, "/v2/approvals/"+created.ID, map[string]any{
		"id":               "other-approval",
		"workflow_run_id":  "other-run",
		"workspace_root":   "/other",
		"requested_action": "deploy release",
		"state":            "approved",
		"decided_by":       "operator",
		"decided_at":       decidedAt,
	}, http.StatusOK)
	if updated.ID != created.ID || updated.WorkflowRunID != run.ID || updated.WorkspaceRoot != workspaceRoot {
		t.Fatalf("updated approval identity = %+v", updated)
	}
	if updated.State != "approved" || updated.DecidedBy != "operator" {
		t.Fatalf("updated approval = %+v", updated)
	}

	list := decodeJSON[[]contracts.Approval](t, handler, http.MethodGet, "/v2/approvals?workspace_root="+workspaceRoot+"&workflow_run_id="+run.ID+"&state=approved", nil, http.StatusOK)
	if len(list) != 1 || list[0].ID != created.ID {
		t.Fatalf("list approvals = %+v", list)
	}

	_ = decodeJSON[map[string]string](t, handler, http.MethodGet, "/v2/approvals?workspace_root="+workspaceRoot+"&state=bogus", nil, http.StatusBadRequest)
	_ = decodeJSON[map[string]string](t, handler, http.MethodPost, "/v2/approvals", map[string]any{
		"workflow_run_id":  run.ID,
		"requested_action": "deploy release",
		"state":            "bogus",
	}, http.StatusBadRequest)
	_ = decodeJSON[map[string]string](t, handler, http.MethodPut, "/v2/approvals/"+created.ID, map[string]any{
		"state": "bogus",
	}, http.StatusBadRequest)
	_ = decodeJSON[map[string]string](t, handler, http.MethodPost, "/v2/approvals", map[string]any{
		"requested_action": "deploy release",
	}, http.StatusBadRequest)
	_ = decodeJSON[map[string]string](t, handler, http.MethodGet, "/v2/approvals?workspace_root=/other", nil, http.StatusBadRequest)

	otherWorkflow := contracts.Workflow{
		ID:            "workflow-approval-other",
		WorkspaceRoot: "/other",
		Name:          "Approval other workflow",
		Trigger:       "manual",
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := s.Workflows().Create(ctx, otherWorkflow); err != nil {
		t.Fatalf("create other workflow: %v", err)
	}
	otherRun := contracts.WorkflowRun{
		ID:            "run-approval-other",
		WorkflowID:    otherWorkflow.ID,
		WorkspaceRoot: otherWorkflow.WorkspaceRoot,
		State:         "waiting_approval",
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := s.Workflows().CreateRun(ctx, otherRun); err != nil {
		t.Fatalf("create other run: %v", err)
	}
	otherApproval := contracts.Approval{
		ID:              "approval-other",
		WorkflowRunID:   otherRun.ID,
		WorkspaceRoot:   otherWorkflow.WorkspaceRoot,
		RequestedAction: "deploy elsewhere",
		State:           "pending",
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if err := s.Workflows().CreateApproval(ctx, otherApproval); err != nil {
		t.Fatalf("create other approval: %v", err)
	}

	_ = decodeJSON[map[string]string](t, handler, http.MethodPost, "/v2/approvals", map[string]any{
		"workflow_run_id":  otherRun.ID,
		"requested_action": "deploy elsewhere",
	}, http.StatusNotFound)
	_ = decodeJSON[map[string]string](t, handler, http.MethodGet, "/v2/approvals/"+otherApproval.ID, nil, http.StatusNotFound)
	_ = decodeJSON[map[string]string](t, handler, http.MethodPut, "/v2/approvals/"+otherApproval.ID, map[string]any{
		"state": "approved",
	}, http.StatusNotFound)
}

func TestApprovalRoutesEnforceStateMachine(t *testing.T) {
	ctx := context.Background()
	s, err := v2store.OpenInMemory()
	if err != nil {
		t.Fatalf("OpenInMemory: %v", err)
	}
	defer s.Close()

	workspaceRoot := t.TempDir()
	now := time.Now().UTC()
	handler := (&routes{
		cfg: config.Config{
			Paths: config.Paths{
				WorkspaceRoot: workspaceRoot,
				DataDir:       filepath.Join(workspaceRoot, ".sesame"),
			},
		},
		store:      s,
		sessionMgr: &testSessionManager{},
	}).handler()

	workflow := contracts.Workflow{
		ID:            "workflow-approval-state-machine",
		WorkspaceRoot: workspaceRoot,
		Name:          "Approval state machine workflow",
		Trigger:       "manual",
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := s.Workflows().Create(ctx, workflow); err != nil {
		t.Fatalf("create workflow: %v", err)
	}
	run := contracts.WorkflowRun{
		ID:            "run-approval-state-machine",
		WorkflowID:    workflow.ID,
		WorkspaceRoot: workspaceRoot,
		State:         "waiting_approval",
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := s.Workflows().CreateRun(ctx, run); err != nil {
		t.Fatalf("create run: %v", err)
	}

	_ = decodeJSON[map[string]string](t, handler, http.MethodPost, "/v2/approvals", map[string]any{
		"workflow_run_id":  run.ID,
		"requested_action": "deploy release",
		"state":            "approved",
		"decided_by":       "operator",
	}, http.StatusBadRequest)

	created := decodeJSON[contracts.Approval](t, handler, http.MethodPost, "/v2/approvals", map[string]any{
		"workflow_run_id":  run.ID,
		"requested_action": "deploy release",
		"risk_level":       "high",
		"summary":          "Deploy to production",
	}, http.StatusCreated)
	if created.State != "pending" || !created.DecidedAt.IsZero() {
		t.Fatalf("created approval = %+v", created)
	}

	_ = decodeJSON[map[string]string](t, handler, http.MethodPut, "/v2/approvals/"+created.ID, map[string]any{
		"state": "approved",
	}, http.StatusBadRequest)

	approved := decodeJSON[contracts.Approval](t, handler, http.MethodPut, "/v2/approvals/"+created.ID, map[string]any{
		"state":      "approved",
		"decided_by": "operator",
	}, http.StatusOK)
	if approved.State != "approved" || approved.DecidedBy != "operator" || approved.DecidedAt.IsZero() {
		t.Fatalf("approved approval = %+v", approved)
	}

	invalidTimestampApproval := decodeJSON[contracts.Approval](t, handler, http.MethodPost, "/v2/approvals", map[string]any{
		"workflow_run_id":  run.ID,
		"requested_action": "deploy release",
	}, http.StatusCreated)
	_ = decodeJSON[map[string]string](t, handler, http.MethodPut, "/v2/approvals/"+invalidTimestampApproval.ID, map[string]any{
		"state":      "approved",
		"decided_by": "operator",
		"decided_at": "not-a-timestamp",
	}, http.StatusBadRequest)

	pendingUpdateApproval := decodeJSON[contracts.Approval](t, handler, http.MethodPost, "/v2/approvals", map[string]any{
		"workflow_run_id":  run.ID,
		"requested_action": "deploy release",
	}, http.StatusCreated)
	_ = decodeJSON[map[string]string](t, handler, http.MethodPut, "/v2/approvals/"+pendingUpdateApproval.ID, map[string]any{
		"summary":    "Still pending",
		"decided_at": now.Add(time.Hour).Format(time.RFC3339Nano),
	}, http.StatusBadRequest)

	_ = decodeJSON[map[string]string](t, handler, http.MethodPut, "/v2/approvals/"+created.ID, map[string]any{
		"state": "pending",
	}, http.StatusBadRequest)
	_ = decodeJSON[map[string]string](t, handler, http.MethodPut, "/v2/approvals/"+created.ID, map[string]any{
		"requested_action": "deploy release v2",
	}, http.StatusBadRequest)
	_ = decodeJSON[map[string]string](t, handler, http.MethodPut, "/v2/approvals/"+created.ID, map[string]any{
		"decided_by": "other-operator",
	}, http.StatusBadRequest)

	fetched := decodeJSON[contracts.Approval](t, handler, http.MethodGet, "/v2/approvals/"+created.ID, nil, http.StatusOK)
	if fetched.State != "approved" || fetched.RequestedAction != "deploy release" || fetched.DecidedBy != "operator" || !fetched.DecidedAt.Equal(approved.DecidedAt) {
		t.Fatalf("fetched approval = %+v", fetched)
	}
}

func TestWorkflowEntityRoutesRequireWorkspaceRootWithoutDaemonWorkspace(t *testing.T) {
	ctx := context.Background()
	s, err := v2store.OpenInMemory()
	if err != nil {
		t.Fatalf("OpenInMemory: %v", err)
	}
	defer s.Close()

	workspaceRoot := t.TempDir()
	now := time.Now().UTC()
	handler := (&routes{
		cfg: config.Config{
			Paths: config.Paths{
				DataDir: filepath.Join(workspaceRoot, ".sesame"),
			},
		},
		store:      s,
		sessionMgr: &testSessionManager{},
		workflowService: workflowTriggerServiceStub{
			triggerFn: func(ctx context.Context, workflow contracts.Workflow, input workflows.TriggerInput) (contracts.WorkflowRun, error) {
				return contracts.WorkflowRun{
					ID:            "triggered-unbound-run",
					WorkflowID:    workflow.ID,
					WorkspaceRoot: workflow.WorkspaceRoot,
					State:         "completed",
					TriggerRef:    input.TriggerRef,
					TaskIDs:       "[]",
					ReportIDs:     "[]",
					Trace:         "[]",
					CreatedAt:     now,
					UpdatedAt:     now,
				}, nil
			},
		},
	}).handler()

	workflow := contracts.Workflow{
		ID:            "workflow-unbound",
		WorkspaceRoot: workspaceRoot,
		Name:          "Unbound workflow",
		Trigger:       "manual",
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := s.Workflows().Create(ctx, workflow); err != nil {
		t.Fatalf("create workflow: %v", err)
	}
	run := contracts.WorkflowRun{
		ID:            "run-unbound",
		WorkflowID:    workflow.ID,
		WorkspaceRoot: workspaceRoot,
		State:         "waiting_approval",
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := s.Workflows().CreateRun(ctx, run); err != nil {
		t.Fatalf("create run: %v", err)
	}
	approval := contracts.Approval{
		ID:              "approval-unbound",
		WorkflowRunID:   run.ID,
		WorkspaceRoot:   workspaceRoot,
		RequestedAction: "deploy release",
		State:           "pending",
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if err := s.Workflows().CreateApproval(ctx, approval); err != nil {
		t.Fatalf("create approval: %v", err)
	}

	_ = decodeJSON[map[string]string](t, handler, http.MethodGet, "/v2/workflows/"+workflow.ID, nil, http.StatusBadRequest)
	_ = decodeJSON[map[string]string](t, handler, http.MethodGet, "/v2/workflows/"+workflow.ID+"?workspace_root=/other", nil, http.StatusNotFound)
	gotWorkflow := decodeJSON[contracts.Workflow](t, handler, http.MethodGet, "/v2/workflows/"+workflow.ID+"?workspace_root="+workspaceRoot, nil, http.StatusOK)
	if gotWorkflow.ID != workflow.ID {
		t.Fatalf("got workflow = %+v", gotWorkflow)
	}
	_ = decodeJSON[map[string]string](t, handler, http.MethodPut, "/v2/workflows/"+workflow.ID, map[string]any{
		"name": "Updated without workspace",
	}, http.StatusBadRequest)
	_ = decodeJSON[map[string]string](t, handler, http.MethodPut, "/v2/workflows/"+workflow.ID+"?workspace_root=/other", map[string]any{
		"name": "Updated with wrong workspace",
	}, http.StatusNotFound)
	updatedWorkflow := decodeJSON[contracts.Workflow](t, handler, http.MethodPut, "/v2/workflows/"+workflow.ID+"?workspace_root="+workspaceRoot, map[string]any{
		"name": "Updated workflow",
	}, http.StatusOK)
	if updatedWorkflow.Name != "Updated workflow" {
		t.Fatalf("updated workflow = %+v", updatedWorkflow)
	}
	_ = decodeJSON[map[string]string](t, handler, http.MethodPost, "/v2/workflow_runs", map[string]any{
		"workflow_id": workflow.ID,
	}, http.StatusBadRequest)
	_ = decodeJSON[map[string]string](t, handler, http.MethodPost, "/v2/workflow_runs?workspace_root=/other", map[string]any{
		"workflow_id": workflow.ID,
	}, http.StatusNotFound)
	createdRun := decodeJSON[contracts.WorkflowRun](t, handler, http.MethodPost, "/v2/workflow_runs?workspace_root="+workspaceRoot, map[string]any{
		"workflow_id": workflow.ID,
	}, http.StatusCreated)
	if createdRun.ID == "" || createdRun.WorkflowID != workflow.ID || createdRun.WorkspaceRoot != workspaceRoot {
		t.Fatalf("created run = %+v", createdRun)
	}
	_ = decodeJSON[map[string]string](t, handler, http.MethodPost, "/v2/workflows/"+workflow.ID+"/trigger", map[string]any{
		"trigger_ref": "manual:missing-workspace",
	}, http.StatusBadRequest)
	_ = decodeJSON[map[string]string](t, handler, http.MethodPost, "/v2/workflows/"+workflow.ID+"/trigger?workspace_root=/other", map[string]any{
		"trigger_ref": "manual:wrong-workspace",
	}, http.StatusNotFound)
	triggeredRun := decodeJSON[contracts.WorkflowRun](t, handler, http.MethodPost, "/v2/workflows/"+workflow.ID+"/trigger?workspace_root="+workspaceRoot, map[string]any{
		"trigger_ref": "manual:matched-workspace",
	}, http.StatusCreated)
	if triggeredRun.WorkflowID != workflow.ID || triggeredRun.WorkspaceRoot != workspaceRoot || triggeredRun.State != "completed" || triggeredRun.TriggerRef != "manual:matched-workspace" {
		t.Fatalf("triggered run = %+v", triggeredRun)
	}

	_ = decodeJSON[map[string]string](t, handler, http.MethodGet, "/v2/workflow_runs/"+run.ID, nil, http.StatusBadRequest)
	_ = decodeJSON[map[string]string](t, handler, http.MethodGet, "/v2/workflow_runs/"+run.ID+"?workspace_root=/other", nil, http.StatusNotFound)
	gotRun := decodeJSON[contracts.WorkflowRun](t, handler, http.MethodGet, "/v2/workflow_runs/"+run.ID+"?workspace_root="+workspaceRoot, nil, http.StatusOK)
	if gotRun.ID != run.ID {
		t.Fatalf("got run = %+v", gotRun)
	}
	_ = decodeJSON[map[string]string](t, handler, http.MethodPost, "/v2/approvals", map[string]any{
		"workflow_run_id":  run.ID,
		"workspace_root":   workspaceRoot,
		"requested_action": "deploy release",
	}, http.StatusBadRequest)
	_ = decodeJSON[map[string]string](t, handler, http.MethodPost, "/v2/approvals?workspace_root=/other", map[string]any{
		"workflow_run_id":  run.ID,
		"workspace_root":   workspaceRoot,
		"requested_action": "deploy release",
	}, http.StatusNotFound)
	createdApproval := decodeJSON[contracts.Approval](t, handler, http.MethodPost, "/v2/approvals?workspace_root="+workspaceRoot, map[string]any{
		"workflow_run_id":  run.ID,
		"workspace_root":   "/other",
		"requested_action": "deploy release",
		"summary":          "Create with matching query workspace",
	}, http.StatusCreated)
	if createdApproval.ID == "" || createdApproval.WorkflowRunID != run.ID || createdApproval.WorkspaceRoot != workspaceRoot || createdApproval.RequestedAction != "deploy release" || createdApproval.State != "pending" {
		t.Fatalf("created approval = %+v", createdApproval)
	}
	_ = decodeJSON[map[string]string](t, handler, http.MethodPut, "/v2/workflow_runs/"+run.ID, map[string]any{
		"state": "completed",
	}, http.StatusBadRequest)
	_ = decodeJSON[map[string]string](t, handler, http.MethodPut, "/v2/workflow_runs/"+run.ID+"?workspace_root=/other", map[string]any{
		"state": "completed",
	}, http.StatusNotFound)
	updatedRun := decodeJSON[contracts.WorkflowRun](t, handler, http.MethodPut, "/v2/workflow_runs/"+run.ID+"?workspace_root="+workspaceRoot, map[string]any{
		"state": "completed",
	}, http.StatusOK)
	if updatedRun.State != "completed" {
		t.Fatalf("updated run = %+v", updatedRun)
	}

	_ = decodeJSON[map[string]string](t, handler, http.MethodGet, "/v2/approvals/"+approval.ID, nil, http.StatusBadRequest)
	_ = decodeJSON[map[string]string](t, handler, http.MethodGet, "/v2/approvals/"+approval.ID+"?workspace_root=/other", nil, http.StatusNotFound)
	gotApproval := decodeJSON[contracts.Approval](t, handler, http.MethodGet, "/v2/approvals/"+approval.ID+"?workspace_root="+workspaceRoot, nil, http.StatusOK)
	if gotApproval.ID != approval.ID {
		t.Fatalf("got approval = %+v", gotApproval)
	}
	_ = decodeJSON[map[string]string](t, handler, http.MethodPut, "/v2/approvals/"+approval.ID, map[string]any{
		"summary": "Needs review",
	}, http.StatusBadRequest)
	_ = decodeJSON[map[string]string](t, handler, http.MethodPut, "/v2/approvals/"+approval.ID+"?workspace_root=/other", map[string]any{
		"summary": "Wrong workspace",
	}, http.StatusNotFound)
	updatedApproval := decodeJSON[contracts.Approval](t, handler, http.MethodPut, "/v2/approvals/"+approval.ID+"?workspace_root="+workspaceRoot, map[string]any{
		"summary": "Needs review",
	}, http.StatusOK)
	if updatedApproval.Summary != "Needs review" || updatedApproval.State != "pending" {
		t.Fatalf("updated approval = %+v", updatedApproval)
	}
}

func TestWorkflowTriggerRoute(t *testing.T) {
	ctx := context.Background()
	s, err := v2store.OpenInMemory()
	if err != nil {
		t.Fatalf("OpenInMemory: %v", err)
	}
	defer s.Close()

	workspaceRoot := t.TempDir()
	now := time.Now().UTC()
	session := contracts.Session{
		ID:                "session-workflow",
		WorkspaceRoot:     workspaceRoot,
		SystemPrompt:      "You are Sesame.",
		PermissionProfile: "workspace",
		State:             "idle",
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	if err := s.Sessions().Create(ctx, session); err != nil {
		t.Fatalf("create session: %v", err)
	}
	sessionMgr := &testSessionManager{}
	sessionMgr.Register(session)

	manager := &routeWorkflowTaskManager{
		store: s,
		waitFn: func(ctx context.Context, task contracts.Task) (contracts.Task, error) {
			task.SessionID = "specialist-reviewer"
			task.State = "completed"
			task.Outcome = "success"
			task.FinalText = "Workflow step complete."
			task.UpdatedAt = time.Now().UTC()
			if err := s.Reports().Create(ctx, contracts.Report{
				ID:         "report-route-1",
				SessionID:  task.ReportSessionID,
				SourceKind: "task_result",
				SourceID:   task.ID,
				Status:     task.State,
				Severity:   "info",
				Title:      "Task result: agent",
				Summary:    task.FinalText,
				CreatedAt:  time.Now().UTC(),
			}); err != nil {
				return contracts.Task{}, err
			}
			return task, nil
		},
	}

	workflowService := workflows.NewService(s, manager, session.ID)
	handler := (&routes{
		cfg: config.Config{
			Paths: config.Paths{
				WorkspaceRoot: workspaceRoot,
				DataDir:       filepath.Join(workspaceRoot, ".sesame"),
			},
		},
		store:            s,
		sessionMgr:       sessionMgr,
		workflowService:  workflowService,
		defaultSessionID: session.ID,
	}).handler()

	localWorkflow := contracts.Workflow{
		ID:            "workflow-trigger-local",
		WorkspaceRoot: workspaceRoot,
		Name:          "Local workflow",
		Trigger:       "manual",
		Steps:         `[{"kind":"role_task","role_id":"reviewer","prompt":"Review the change","title":"Review"}]`,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := s.Workflows().Create(ctx, localWorkflow); err != nil {
		t.Fatalf("create local workflow: %v", err)
	}

	run := decodeJSON[contracts.WorkflowRun](t, handler, http.MethodPost, "/v2/workflows/"+localWorkflow.ID+"/trigger", map[string]any{
		"id":             "client-run",
		"workflow_id":    "other-workflow",
		"workspace_root": "/other",
		"trigger_ref":    "manual:ui",
	}, http.StatusCreated)
	if run.ID == "" || run.ID == "client-run" {
		t.Fatalf("run id = %q", run.ID)
	}
	if run.WorkflowID != localWorkflow.ID || run.WorkspaceRoot != workspaceRoot {
		t.Fatalf("triggered run = %+v", run)
	}
	if run.State != "completed" || run.TriggerRef != "manual:ui" {
		t.Fatalf("run completion = %+v", run)
	}
	if len(decodeStringSliceJSON(t, run.TaskIDs)) != 1 || len(decodeStringSliceJSON(t, run.ReportIDs)) != 1 {
		t.Fatalf("run linkage = %+v", run)
	}

	otherWorkflow := contracts.Workflow{
		ID:            "workflow-trigger-other",
		WorkspaceRoot: "/other",
		Name:          "Other workflow",
		Trigger:       "manual",
		Steps:         localWorkflow.Steps,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := s.Workflows().Create(ctx, otherWorkflow); err != nil {
		t.Fatalf("create other workflow: %v", err)
	}
	_ = decodeJSON[map[string]string](t, handler, http.MethodPost, "/v2/workflows/"+otherWorkflow.ID+"/trigger", nil, http.StatusNotFound)

	nonManualWorkflow := contracts.Workflow{
		ID:            "workflow-trigger-auto",
		WorkspaceRoot: workspaceRoot,
		Name:          "Auto workflow",
		Trigger:       "file_change",
		Steps:         localWorkflow.Steps,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := s.Workflows().Create(ctx, nonManualWorkflow); err != nil {
		t.Fatalf("create non-manual workflow: %v", err)
	}
	_ = decodeJSON[map[string]string](t, handler, http.MethodPost, "/v2/workflows/"+nonManualWorkflow.ID+"/trigger", nil, http.StatusBadRequest)

	badStepsWorkflow := contracts.Workflow{
		ID:            "workflow-trigger-bad-steps",
		WorkspaceRoot: workspaceRoot,
		Name:          "Bad steps workflow",
		Trigger:       "manual",
		Steps:         `[{"kind":"shell","prompt":"echo nope"}]`,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := s.Workflows().Create(ctx, badStepsWorkflow); err != nil {
		t.Fatalf("create bad steps workflow: %v", err)
	}
	_ = decodeJSON[map[string]string](t, handler, http.MethodPost, "/v2/workflows/"+badStepsWorkflow.ID+"/trigger", nil, http.StatusBadRequest)
}

func TestWorkflowTriggerRouteDetachesRequestCancellation(t *testing.T) {
	ctx := context.Background()
	s, err := v2store.OpenInMemory()
	if err != nil {
		t.Fatalf("OpenInMemory: %v", err)
	}
	defer s.Close()

	workspaceRoot := t.TempDir()
	now := time.Now().UTC()
	workflow := contracts.Workflow{
		ID:            "workflow-trigger-detached",
		WorkspaceRoot: workspaceRoot,
		Name:          "Detached workflow",
		Trigger:       "manual",
		Steps:         `[{"kind":"role_task","role_id":"reviewer","prompt":"Review"}]`,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := s.Workflows().Create(ctx, workflow); err != nil {
		t.Fatalf("create workflow: %v", err)
	}

	type requestMarker string

	called := false
	handler := (&routes{
		cfg: config.Config{
			Paths: config.Paths{
				WorkspaceRoot: workspaceRoot,
				DataDir:       filepath.Join(workspaceRoot, ".sesame"),
			},
		},
		store:      s,
		sessionMgr: &testSessionManager{},
		workflowService: workflowTriggerServiceStub{
			triggerFn: func(ctx context.Context, workflow contracts.Workflow, input workflows.TriggerInput) (contracts.WorkflowRun, error) {
				called = true
				if ctx.Done() != nil {
					t.Fatalf("trigger ctx should be detached from request cancellation")
				}
				if got := ctx.Value(requestMarker("marker")); got != "request-value" {
					t.Fatalf("trigger ctx value = %v, want request-value", got)
				}
				return contracts.WorkflowRun{
					ID:            "run-detached",
					WorkflowID:    workflow.ID,
					WorkspaceRoot: workflow.WorkspaceRoot,
					State:         "completed",
					TriggerRef:    input.TriggerRef,
					TaskIDs:       "[]",
					ReportIDs:     "[]",
					Trace:         "[]",
					CreatedAt:     now,
					UpdatedAt:     now,
				}, nil
			},
		},
	}).handler()

	reqCtx, cancel := context.WithCancel(context.WithValue(context.Background(), requestMarker("marker"), "request-value"))
	defer cancel()
	req := httptest.NewRequest(http.MethodPost, "/v2/workflows/"+workflow.ID+"/trigger", bytes.NewReader([]byte(`{"trigger_ref":"manual:ui"}`))).WithContext(reqCtx)
	req.Header.Set("Content-Type", "application/json")
	if req.Context().Done() == nil {
		t.Fatalf("request context should support cancellation")
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if !called {
		t.Fatalf("workflow trigger service was not called")
	}
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body %s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	var run contracts.WorkflowRun
	if err := json.NewDecoder(rec.Body).Decode(&run); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if run.ID != "run-detached" || run.TriggerRef != "manual:ui" {
		t.Fatalf("run = %+v", run)
	}
}

func TestWorkflowTriggerRouteReturnsFailedRunBody(t *testing.T) {
	ctx := context.Background()
	s, err := v2store.OpenInMemory()
	if err != nil {
		t.Fatalf("OpenInMemory: %v", err)
	}
	defer s.Close()

	workspaceRoot := t.TempDir()
	now := time.Now().UTC()
	workflow := contracts.Workflow{
		ID:            "workflow-trigger-failed",
		WorkspaceRoot: workspaceRoot,
		Name:          "Failed workflow",
		Trigger:       "manual",
		Steps:         `[{"kind":"role_task","role_id":"reviewer","prompt":"Review"}]`,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := s.Workflows().Create(ctx, workflow); err != nil {
		t.Fatalf("create workflow: %v", err)
	}

	run := decodeJSON[contracts.WorkflowRun](t, (&routes{
		cfg: config.Config{
			Paths: config.Paths{
				WorkspaceRoot: workspaceRoot,
				DataDir:       filepath.Join(workspaceRoot, ".sesame"),
			},
		},
		store:      s,
		sessionMgr: &testSessionManager{},
		workflowService: workflowTriggerServiceStub{
			triggerFn: func(ctx context.Context, workflow contracts.Workflow, input workflows.TriggerInput) (contracts.WorkflowRun, error) {
				return contracts.WorkflowRun{
					ID:            "run-failed",
					WorkflowID:    workflow.ID,
					WorkspaceRoot: workflow.WorkspaceRoot,
					State:         "failed",
					TriggerRef:    input.TriggerRef,
					TaskIDs:       `["task-failed-1"]`,
					ReportIDs:     `["report-failed-1"]`,
					Trace:         `[{"event":"run_failed","state":"failed"}]`,
					CreatedAt:     now,
					UpdatedAt:     now,
				}, nil
			},
		},
	}).handler(), http.MethodPost, "/v2/workflows/"+workflow.ID+"/trigger", map[string]any{
		"trigger_ref": "manual:failure",
	}, http.StatusOK)

	if run.ID != "run-failed" || run.State != "failed" {
		t.Fatalf("run = %+v", run)
	}
	if run.TriggerRef != "manual:failure" {
		t.Fatalf("trigger_ref = %q, want manual:failure", run.TriggerRef)
	}
	if len(decodeStringSliceJSON(t, run.TaskIDs)) != 1 || len(decodeStringSliceJSON(t, run.ReportIDs)) != 1 {
		t.Fatalf("run linkage = %+v", run)
	}
	if run.Trace == "" || run.Trace == "[]" {
		t.Fatalf("expected failed run trace in body: %+v", run)
	}
}

func TestWorkflowTriggerRouteReturnsInterruptedRunBody(t *testing.T) {
	ctx := context.Background()
	s, err := v2store.OpenInMemory()
	if err != nil {
		t.Fatalf("OpenInMemory: %v", err)
	}
	defer s.Close()

	workspaceRoot := t.TempDir()
	now := time.Now().UTC()
	workflow := contracts.Workflow{
		ID:            "workflow-trigger-interrupted",
		WorkspaceRoot: workspaceRoot,
		Name:          "Interrupted workflow",
		Trigger:       "manual",
		Steps:         `[{"kind":"role_task","role_id":"reviewer","prompt":"Review"}]`,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := s.Workflows().Create(ctx, workflow); err != nil {
		t.Fatalf("create workflow: %v", err)
	}

	run := decodeJSON[contracts.WorkflowRun](t, (&routes{
		cfg: config.Config{
			Paths: config.Paths{
				WorkspaceRoot: workspaceRoot,
				DataDir:       filepath.Join(workspaceRoot, ".sesame"),
			},
		},
		store:      s,
		sessionMgr: &testSessionManager{},
		workflowService: workflowTriggerServiceStub{
			triggerFn: func(ctx context.Context, workflow contracts.Workflow, input workflows.TriggerInput) (contracts.WorkflowRun, error) {
				return contracts.WorkflowRun{
					ID:            "run-interrupted",
					WorkflowID:    workflow.ID,
					WorkspaceRoot: workflow.WorkspaceRoot,
					State:         "interrupted",
					TriggerRef:    input.TriggerRef,
					TaskIDs:       `["task-interrupted-1"]`,
					ReportIDs:     `["report-interrupted-1"]`,
					Trace:         `[{"event":"run_interrupted","state":"interrupted"}]`,
					CreatedAt:     now,
					UpdatedAt:     now,
				}, nil
			},
		},
	}).handler(), http.MethodPost, "/v2/workflows/"+workflow.ID+"/trigger", map[string]any{
		"trigger_ref": "manual:interrupt",
	}, http.StatusOK)

	if run.ID != "run-interrupted" || run.State != "interrupted" {
		t.Fatalf("run = %+v", run)
	}
	if run.TriggerRef != "manual:interrupt" {
		t.Fatalf("trigger_ref = %q, want manual:interrupt", run.TriggerRef)
	}
	if len(decodeStringSliceJSON(t, run.TaskIDs)) != 1 || len(decodeStringSliceJSON(t, run.ReportIDs)) != 1 {
		t.Fatalf("run linkage = %+v", run)
	}
	if run.Trace == "" || run.Trace == "[]" {
		t.Fatalf("expected interrupted run trace in body: %+v", run)
	}
}

func TestWorkflowTriggerRouteReturnsWaitingApprovalRunBody(t *testing.T) {
	ctx := context.Background()
	s, err := v2store.OpenInMemory()
	if err != nil {
		t.Fatalf("OpenInMemory: %v", err)
	}
	defer s.Close()

	workspaceRoot := t.TempDir()
	now := time.Now().UTC()
	workflow := contracts.Workflow{
		ID:            "workflow-trigger-waiting-approval",
		WorkspaceRoot: workspaceRoot,
		Name:          "Waiting approval workflow",
		Trigger:       "manual",
		Steps:         `[{"kind":"approval","requested_action":"deploy"}]`,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := s.Workflows().Create(ctx, workflow); err != nil {
		t.Fatalf("create workflow: %v", err)
	}

	handler := (&routes{
		cfg: config.Config{
			Paths: config.Paths{
				WorkspaceRoot: workspaceRoot,
				DataDir:       filepath.Join(workspaceRoot, ".sesame"),
			},
		},
		store:      s,
		sessionMgr: &testSessionManager{},
		workflowService: workflowTriggerServiceStub{
			triggerFn: func(ctx context.Context, workflow contracts.Workflow, input workflows.TriggerInput) (contracts.WorkflowRun, error) {
				return contracts.WorkflowRun{
					ID:            "run-waiting-approval",
					WorkflowID:    workflow.ID,
					WorkspaceRoot: workflow.WorkspaceRoot,
					State:         "waiting_approval",
					TriggerRef:    input.TriggerRef,
					TaskIDs:       `["task-before-approval"]`,
					ReportIDs:     `["report-before-approval"]`,
					ApprovalIDs:   `["approval-1"]`,
					Trace:         `[{"event":"approval_requested","state":"pending"}]`,
					CreatedAt:     now,
					UpdatedAt:     now,
				}, nil
			},
		},
	}).handler()

	run := decodeJSON[contracts.WorkflowRun](t, handler, http.MethodPost, "/v2/workflows/"+workflow.ID+"/trigger", map[string]any{
		"trigger_ref": "manual:approval",
	}, http.StatusAccepted)

	if run.ID != "run-waiting-approval" || run.State != "waiting_approval" {
		t.Fatalf("run = %+v", run)
	}
	if run.TriggerRef != "manual:approval" {
		t.Fatalf("trigger_ref = %q, want manual:approval", run.TriggerRef)
	}
	if len(decodeStringSliceJSON(t, run.ApprovalIDs)) != 1 {
		t.Fatalf("approval linkage = %+v", run)
	}
	if run.Trace == "" || run.Trace == "[]" {
		t.Fatalf("expected waiting approval trace in body: %+v", run)
	}
}

func TestWorkflowRoutesErrorMapping(t *testing.T) {
	ctx := context.Background()
	s, err := v2store.OpenInMemory()
	if err != nil {
		t.Fatalf("OpenInMemory: %v", err)
	}
	defer s.Close()

	workspaceRoot := t.TempDir()
	handler := (&routes{
		cfg: config.Config{
			Paths: config.Paths{
				WorkspaceRoot: workspaceRoot,
				DataDir:       filepath.Join(workspaceRoot, ".sesame"),
			},
		},
		store:      s,
		sessionMgr: &testSessionManager{},
	}).handler()

	_ = decodeJSON[map[string]string](t, handler, http.MethodPost, "/v2/workflows", map[string]any{
		"trigger": "manual",
	}, http.StatusBadRequest)
	_ = decodeJSON[map[string]string](t, handler, http.MethodGet, "/v2/workflows?workspace_root=/other", nil, http.StatusBadRequest)
	_ = decodeJSON[map[string]string](t, handler, http.MethodGet, "/v2/workflows/missing", nil, http.StatusNotFound)
	_ = decodeJSON[map[string]string](t, handler, http.MethodPost, "/v2/workflow_runs", map[string]any{
		"state": "queued",
	}, http.StatusBadRequest)
	_ = decodeJSON[map[string]string](t, handler, http.MethodPost, "/v2/workflow_runs", map[string]any{
		"workflow_id": "missing",
	}, http.StatusNotFound)
	_ = decodeJSON[map[string]string](t, handler, http.MethodGet, "/v2/workflow_runs?workspace_root=/other", nil, http.StatusBadRequest)
	_ = decodeJSON[map[string]string](t, handler, http.MethodGet, "/v2/workflow_runs/missing", nil, http.StatusNotFound)

	now := time.Now().UTC()
	localWorkflow := contracts.Workflow{
		ID:            "workflow-local",
		WorkspaceRoot: workspaceRoot,
		Name:          "Local workflow",
		Trigger:       "manual",
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := s.Workflows().Create(ctx, localWorkflow); err != nil {
		t.Fatalf("create local workflow: %v", err)
	}
	_ = decodeJSON[map[string]string](t, handler, http.MethodPost, "/v2/workflow_runs", map[string]any{
		"workflow_id": localWorkflow.ID,
		"state":       "bogus",
	}, http.StatusBadRequest)

	otherWorkflow := contracts.Workflow{
		ID:            "workflow-other",
		WorkspaceRoot: "/other",
		Name:          "Other workflow",
		Trigger:       "manual",
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := s.Workflows().Create(ctx, otherWorkflow); err != nil {
		t.Fatalf("create other workflow: %v", err)
	}
	if err := s.Workflows().CreateRun(ctx, contracts.WorkflowRun{
		ID:            "run-other",
		WorkflowID:    otherWorkflow.ID,
		WorkspaceRoot: otherWorkflow.WorkspaceRoot,
		State:         "queued",
		CreatedAt:     now,
		UpdatedAt:     now,
	}); err != nil {
		t.Fatalf("create other workflow run: %v", err)
	}
	_ = decodeJSON[map[string]string](t, handler, http.MethodGet, "/v2/workflows/"+otherWorkflow.ID, nil, http.StatusNotFound)
	_ = decodeJSON[map[string]string](t, handler, http.MethodPut, "/v2/workflows/"+otherWorkflow.ID, map[string]any{
		"name": "Other updated",
	}, http.StatusNotFound)
	_ = decodeJSON[map[string]string](t, handler, http.MethodPost, "/v2/workflow_runs", map[string]any{
		"workflow_id": otherWorkflow.ID,
	}, http.StatusNotFound)
	_ = decodeJSON[map[string]string](t, handler, http.MethodGet, "/v2/workflow_runs/run-other", nil, http.StatusNotFound)
	_ = decodeJSON[map[string]string](t, handler, http.MethodPut, "/v2/workflow_runs/run-other", map[string]any{
		"state": "completed",
	}, http.StatusNotFound)
	_ = decodeJSON[map[string]string](t, handler, http.MethodPost, "/v2/workflows/"+localWorkflow.ID+"/trigger", nil, http.StatusInternalServerError)
}

func decodeJSON[T any](t *testing.T, handler http.Handler, method, path string, body any, wantStatus int) T {
	t.Helper()
	var reader *bytes.Reader
	if body == nil {
		reader = bytes.NewReader(nil)
	} else {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		reader = bytes.NewReader(raw)
	}
	req := httptest.NewRequest(method, path, reader)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != wantStatus {
		t.Fatalf("%s %s status = %d, want %d, body %s", method, path, rec.Code, wantStatus, rec.Body.String())
	}
	var out T
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return out
}

func hasContextBlock(blocks []contextsvc.PreviewBlock, id, status string) bool {
	for _, block := range blocks {
		if block.ID == id && block.Status == status {
			return true
		}
	}
	return false
}

func decodeStringSliceJSON(t *testing.T, raw string) []string {
	t.Helper()
	var out []string
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		t.Fatalf("decode string slice %q: %v", raw, err)
	}
	return out
}

type routeWorkflowTaskManager struct {
	store  contracts.Store
	tasks  map[string]contracts.Task
	waitFn func(ctx context.Context, task contracts.Task) (contracts.Task, error)
}

type workflowTriggerServiceStub struct {
	triggerFn func(ctx context.Context, workflow contracts.Workflow, input workflows.TriggerInput) (contracts.WorkflowRun, error)
}

func (s workflowTriggerServiceStub) Trigger(ctx context.Context, workflow contracts.Workflow, input workflows.TriggerInput) (contracts.WorkflowRun, error) {
	if s.triggerFn == nil {
		return contracts.WorkflowRun{}, nil
	}
	return s.triggerFn(ctx, workflow, input)
}

func (m *routeWorkflowTaskManager) Create(_ context.Context, task contracts.Task) error {
	if m.tasks == nil {
		m.tasks = map[string]contracts.Task{}
	}
	m.tasks[task.ID] = task
	return nil
}

func (m *routeWorkflowTaskManager) Start(_ context.Context, taskID string) error {
	task := m.tasks[taskID]
	task.State = "running"
	m.tasks[taskID] = task
	return nil
}

func (m *routeWorkflowTaskManager) Wait(ctx context.Context, taskID string) (contracts.Task, error) {
	task, ok := m.tasks[taskID]
	if !ok {
		return contracts.Task{}, context.Canceled
	}
	if m.waitFn != nil {
		return m.waitFn(ctx, task)
	}
	task.State = "completed"
	task.Outcome = "success"
	task.FinalText = "done"
	return task, nil
}

type testSessionManager struct {
	sessions map[string]contracts.Session
}

func (m *testSessionManager) Register(session contracts.Session) {
	if m.sessions == nil {
		m.sessions = map[string]contracts.Session{}
	}
	m.sessions[session.ID] = session
}

func (m *testSessionManager) SubmitTurn(context.Context, string, contracts.SubmitTurnInput) (string, error) {
	return "", nil
}

func (m *testSessionManager) CancelTurn(string, string) bool { return false }

func (m *testSessionManager) QueuePayload(sessionID string) (contracts.QueuePayload, bool) {
	if _, ok := m.sessions[sessionID]; !ok {
		return contracts.QueuePayload{}, false
	}
	return contracts.QueuePayload{}, true
}
