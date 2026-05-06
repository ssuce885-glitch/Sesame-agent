package workflows

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"go-agent/internal/v2/contracts"
	v2store "go-agent/internal/v2/store"
)

func TestServiceTriggerSingleRoleTask(t *testing.T) {
	ctx := context.Background()
	store, err := v2store.OpenInMemory()
	if err != nil {
		t.Fatalf("OpenInMemory: %v", err)
	}
	defer store.Close()

	workspaceRoot := t.TempDir()
	now := time.Now().UTC()
	if err := store.Sessions().Create(ctx, contracts.Session{
		ID:            "main-session",
		WorkspaceRoot: workspaceRoot,
		State:         "idle",
		CreatedAt:     now,
		UpdatedAt:     now,
	}); err != nil {
		t.Fatalf("create session: %v", err)
	}
	workflow := contracts.Workflow{
		ID:            "workflow-review",
		WorkspaceRoot: workspaceRoot,
		Name:          "Review workflow",
		Trigger:       "manual",
		Steps:         `{"steps":[{"type":"role_task","role":"reviewer","prompt":"Review the patch.","name":"Review step"}]}`,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := store.Workflows().Create(ctx, workflow); err != nil {
		t.Fatalf("create workflow: %v", err)
	}

	manager := &fakeTaskManager{
		store: store,
		waitFn: func(ctx context.Context, task contracts.Task) (contracts.Task, error) {
			task.SessionID = "specialist-reviewer"
			task.State = "completed"
			task.Outcome = "success"
			task.FinalText = "Patch reviewed."
			task.UpdatedAt = time.Now().UTC()
			if err := store.Reports().Create(ctx, contracts.Report{
				ID:         "report-task-1",
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

	service := NewService(store, manager, "main-session")
	run, err := service.Trigger(ctx, workflow, TriggerInput{TriggerRef: "manual:test"})
	if err != nil {
		t.Fatalf("Trigger: %v", err)
	}
	if run.State != "completed" {
		t.Fatalf("run state = %q, want completed", run.State)
	}
	if run.WorkflowID != workflow.ID || run.WorkspaceRoot != workspaceRoot {
		t.Fatalf("run identity = %+v", run)
	}

	if len(manager.created) != 1 {
		t.Fatalf("created tasks = %d, want 1", len(manager.created))
	}
	task := manager.created[0]
	if task.Kind != "agent" || task.State != "pending" {
		t.Fatalf("created task = %+v", task)
	}
	if task.RoleID != "reviewer" || task.ParentSessionID != "main-session" || task.ReportSessionID != "main-session" {
		t.Fatalf("task routing = %+v", task)
	}

	taskIDs := decodeStringSlice(t, run.TaskIDs)
	if len(taskIDs) != 1 || taskIDs[0] != task.ID {
		t.Fatalf("task_ids = %v, want [%s]", taskIDs, task.ID)
	}
	reportIDs := decodeStringSlice(t, run.ReportIDs)
	if len(reportIDs) != 1 || reportIDs[0] != "report-task-1" {
		t.Fatalf("report_ids = %v", reportIDs)
	}
	trace := decodeTrace(t, run.Trace)
	for _, want := range []string{"run_created", "step_started", "task_created", "task_completed", "run_completed"} {
		if !hasTraceEvent(trace, want) {
			t.Fatalf("trace missing %q: %+v", want, trace)
		}
	}
	if !hasTraceState(trace, "task_completed", "completed") {
		t.Fatalf("trace missing completed task event: %+v", trace)
	}

	got, err := store.Workflows().GetRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if got.State != "completed" || got.ReportIDs != run.ReportIDs {
		t.Fatalf("persisted run = %+v", got)
	}
}

func TestServiceTriggerCreatesNewRunForRepeatedManualTriggerRef(t *testing.T) {
	ctx := context.Background()
	store, err := v2store.OpenInMemory()
	if err != nil {
		t.Fatalf("OpenInMemory: %v", err)
	}
	defer store.Close()

	workspaceRoot := t.TempDir()
	now := time.Now().UTC()
	if err := store.Sessions().Create(ctx, contracts.Session{
		ID:            "main-session",
		WorkspaceRoot: workspaceRoot,
		State:         "idle",
		CreatedAt:     now,
		UpdatedAt:     now,
	}); err != nil {
		t.Fatalf("create session: %v", err)
	}
	workflow := contracts.Workflow{
		ID:            "workflow-manual-repeat",
		WorkspaceRoot: workspaceRoot,
		Name:          "Repeated manual trigger workflow",
		Trigger:       "manual",
		Steps:         `{"steps":[{"type":"role_task","role":"reviewer","prompt":"Review the patch.","name":"Review step"}]}`,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := store.Workflows().Create(ctx, workflow); err != nil {
		t.Fatalf("create workflow: %v", err)
	}

	manager := &fakeTaskManager{
		store: store,
		waitFn: func(ctx context.Context, task contracts.Task) (contracts.Task, error) {
			task.State = "completed"
			task.Outcome = "success"
			task.FinalText = "Patch reviewed."
			task.UpdatedAt = time.Now().UTC()
			reportID := "report-" + task.ID
			if err := store.Reports().Create(ctx, contracts.Report{
				ID:         reportID,
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

	service := NewService(store, manager, "main-session")
	first, err := service.Trigger(ctx, workflow, TriggerInput{TriggerRef: "manual:web"})
	if err != nil {
		t.Fatalf("Trigger first: %v", err)
	}
	second, err := service.Trigger(ctx, workflow, TriggerInput{TriggerRef: "manual:web"})
	if err != nil {
		t.Fatalf("Trigger second: %v", err)
	}

	if first.ID == second.ID {
		t.Fatalf("run ids = %q and %q, want different runs", first.ID, second.ID)
	}
	if first.TriggerRef != "manual:web" || second.TriggerRef != "manual:web" {
		t.Fatalf("trigger refs = %q and %q, want manual:web", first.TriggerRef, second.TriggerRef)
	}
	if first.DedupeRef != "" || second.DedupeRef != "" {
		t.Fatalf("dedupe refs = %q and %q, want empty", first.DedupeRef, second.DedupeRef)
	}
	if len(manager.created) != 2 {
		t.Fatalf("created tasks = %d, want 2", len(manager.created))
	}
	if firstTaskIDs, secondTaskIDs := decodeStringSlice(t, first.TaskIDs), decodeStringSlice(t, second.TaskIDs); len(firstTaskIDs) != 1 || len(secondTaskIDs) != 1 || firstTaskIDs[0] == secondTaskIDs[0] {
		t.Fatalf("task ids = %v and %v, want distinct single task runs", firstTaskIDs, secondTaskIDs)
	}

	runs, err := store.Workflows().ListRunsByWorkspace(ctx, workspaceRoot, contracts.WorkflowRunListOptions{})
	if err != nil {
		t.Fatalf("ListRunsByWorkspace: %v", err)
	}
	if len(runs) != 2 {
		t.Fatalf("workflow runs = %+v, want two runs", runs)
	}
	for _, run := range runs {
		if run.TriggerRef != "manual:web" {
			t.Fatalf("stored trigger_ref = %q, want manual:web", run.TriggerRef)
		}
		if run.DedupeRef != "" {
			t.Fatalf("stored dedupe_ref = %q, want empty", run.DedupeRef)
		}
	}
}

func TestServiceTriggerMultiStepSerialOrdering(t *testing.T) {
	ctx := context.Background()
	store, err := v2store.OpenInMemory()
	if err != nil {
		t.Fatalf("OpenInMemory: %v", err)
	}
	defer store.Close()

	workspaceRoot := t.TempDir()
	now := time.Now().UTC()
	if err := store.Sessions().Create(ctx, contracts.Session{
		ID:            "main-session",
		WorkspaceRoot: workspaceRoot,
		State:         "idle",
		CreatedAt:     now,
		UpdatedAt:     now,
	}); err != nil {
		t.Fatalf("create session: %v", err)
	}
	workflow := contracts.Workflow{
		ID:            "workflow-multi-step",
		WorkspaceRoot: workspaceRoot,
		Name:          "Multi-step workflow",
		Trigger:       "manual",
		Steps:         `{"steps":[{"type":"role_task","role":"planner","prompt":"Plan the work.","name":"Plan"},{"type":"role_task","role":"reviewer","prompt":"Review the work.","name":"Review"}]}`,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := store.Workflows().Create(ctx, workflow); err != nil {
		t.Fatalf("create workflow: %v", err)
	}

	manager := &fakeTaskManager{
		store: store,
		waitFn: func(ctx context.Context, task contracts.Task) (contracts.Task, error) {
			task.State = "completed"
			task.Outcome = "success"
			task.UpdatedAt = time.Now().UTC()
			reportID := ""
			switch task.RoleID {
			case "planner":
				task.FinalText = "Plan complete."
				reportID = "report-step-1"
			case "reviewer":
				task.FinalText = "Review complete."
				reportID = "report-step-2"
			default:
				t.Fatalf("unexpected role_id %q", task.RoleID)
			}
			if err := store.Reports().Create(ctx, contracts.Report{
				ID:         reportID,
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

	service := NewService(store, manager, "main-session")
	run, err := service.Trigger(ctx, workflow, TriggerInput{TriggerRef: "manual:multi"})
	if err != nil {
		t.Fatalf("Trigger: %v", err)
	}
	if run.State != "completed" {
		t.Fatalf("run state = %q, want completed", run.State)
	}
	if len(manager.created) != 2 {
		t.Fatalf("created tasks = %d, want 2", len(manager.created))
	}

	taskIDs := decodeStringSlice(t, run.TaskIDs)
	wantTaskIDs := []string{manager.created[0].ID, manager.created[1].ID}
	if len(taskIDs) != len(wantTaskIDs) {
		t.Fatalf("task_ids = %v, want %v", taskIDs, wantTaskIDs)
	}
	for i := range wantTaskIDs {
		if taskIDs[i] != wantTaskIDs[i] {
			t.Fatalf("task_ids = %v, want %v", taskIDs, wantTaskIDs)
		}
	}

	reportIDs := decodeStringSlice(t, run.ReportIDs)
	wantReportIDs := []string{"report-step-1", "report-step-2"}
	if len(reportIDs) != len(wantReportIDs) {
		t.Fatalf("report_ids = %v, want %v", reportIDs, wantReportIDs)
	}
	for i := range wantReportIDs {
		if reportIDs[i] != wantReportIDs[i] {
			t.Fatalf("report_ids = %v, want %v", reportIDs, wantReportIDs)
		}
	}

	trace := decodeTrace(t, run.Trace)
	if len(trace) != 8 {
		t.Fatalf("trace length = %d, want 8 (%+v)", len(trace), trace)
	}
	if trace[0].Event != "run_created" || trace[1].Event != "step_started" || trace[2].Event != "task_created" || trace[3].Event != "task_completed" || trace[4].Event != "step_started" || trace[5].Event != "task_created" || trace[6].Event != "task_completed" || trace[7].Event != "run_completed" {
		t.Fatalf("trace events out of order: %+v", trace)
	}
	if trace[1].StepIndex == nil || *trace[1].StepIndex != 0 || trace[2].TaskID != manager.created[0].ID || trace[3].TaskID != manager.created[0].ID {
		t.Fatalf("first step trace mismatch: %+v", trace)
	}
	if trace[4].StepIndex == nil || *trace[4].StepIndex != 1 || trace[5].TaskID != manager.created[1].ID || trace[6].TaskID != manager.created[1].ID {
		t.Fatalf("second step trace mismatch: %+v", trace)
	}
}

func TestServiceTriggerApprovalStepWaitsForDecision(t *testing.T) {
	ctx := context.Background()
	store, err := v2store.OpenInMemory()
	if err != nil {
		t.Fatalf("OpenInMemory: %v", err)
	}
	defer store.Close()

	workspaceRoot := t.TempDir()
	now := time.Now().UTC()
	if err := store.Sessions().Create(ctx, contracts.Session{
		ID:            "main-session",
		WorkspaceRoot: workspaceRoot,
		State:         "idle",
		CreatedAt:     now,
		UpdatedAt:     now,
	}); err != nil {
		t.Fatalf("create session: %v", err)
	}
	workflow := contracts.Workflow{
		ID:            "workflow-approval",
		WorkspaceRoot: workspaceRoot,
		Name:          "Approval workflow",
		Trigger:       "manual",
		Steps:         `{"steps":[{"type":"role_task","role":"planner","prompt":"Draft the change.","name":"Draft"},{"type":"approval","action":"deploy release","risk":"high","summary":"Deploy to production","payload":{"environment":"prod"}},{"type":"role_task","role":"reviewer","prompt":"Should not run yet.","name":"Post approval"}]}`,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := store.Workflows().Create(ctx, workflow); err != nil {
		t.Fatalf("create workflow: %v", err)
	}

	manager := &fakeTaskManager{
		store: store,
		waitFn: func(ctx context.Context, task contracts.Task) (contracts.Task, error) {
			task.State = "completed"
			task.Outcome = "success"
			task.FinalText = "Draft complete."
			task.UpdatedAt = time.Now().UTC()
			if err := store.Reports().Create(ctx, contracts.Report{
				ID:         "report-approval-step-1",
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

	service := NewService(store, manager, "main-session")
	run, err := service.Trigger(ctx, workflow, TriggerInput{TriggerRef: "manual:approval"})
	if err != nil {
		t.Fatalf("Trigger: %v", err)
	}
	if run.State != "waiting_approval" {
		t.Fatalf("run state = %q, want waiting_approval", run.State)
	}
	if len(manager.created) != 1 {
		t.Fatalf("created tasks = %d, want 1", len(manager.created))
	}

	taskIDs := decodeStringSlice(t, run.TaskIDs)
	if len(taskIDs) != 1 || taskIDs[0] != manager.created[0].ID {
		t.Fatalf("task_ids = %v", taskIDs)
	}
	reportIDs := decodeStringSlice(t, run.ReportIDs)
	if len(reportIDs) != 1 || reportIDs[0] != "report-approval-step-1" {
		t.Fatalf("report_ids = %v", reportIDs)
	}
	approvalIDs := decodeStringSlice(t, run.ApprovalIDs)
	if len(approvalIDs) != 1 {
		t.Fatalf("approval_ids = %v, want one approval", approvalIDs)
	}

	approval, err := store.Workflows().GetApproval(ctx, approvalIDs[0])
	if err != nil {
		t.Fatalf("GetApproval: %v", err)
	}
	if approval.WorkflowRunID != run.ID || approval.State != "pending" || approval.RequestedAction != "deploy release" {
		t.Fatalf("approval = %+v", approval)
	}
	if approval.ProposedPayload != `{"environment":"prod"}` {
		t.Fatalf("approval payload = %q", approval.ProposedPayload)
	}

	trace := decodeTrace(t, run.Trace)
	if !hasTraceEvent(trace, "approval_requested") {
		t.Fatalf("trace missing approval_requested: %+v", trace)
	}
	if hasTraceEvent(trace, "run_completed") {
		t.Fatalf("trace should not complete after approval gate: %+v", trace)
	}
	if !hasTraceState(trace, "approval_requested", "pending") {
		t.Fatalf("trace missing pending approval state: %+v", trace)
	}

	got, err := store.Workflows().GetRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if got.State != "waiting_approval" || got.ApprovalIDs != run.ApprovalIDs {
		t.Fatalf("persisted run = %+v", got)
	}
}

func TestServiceResumeApprovedApprovalContinuesAfterGate(t *testing.T) {
	ctx := context.Background()
	store, err := v2store.OpenInMemory()
	if err != nil {
		t.Fatalf("OpenInMemory: %v", err)
	}
	defer store.Close()

	workspaceRoot := t.TempDir()
	now := time.Now().UTC()
	if err := store.Sessions().Create(ctx, contracts.Session{
		ID:            "main-session",
		WorkspaceRoot: workspaceRoot,
		State:         "idle",
		CreatedAt:     now,
		UpdatedAt:     now,
	}); err != nil {
		t.Fatalf("create session: %v", err)
	}
	workflow := contracts.Workflow{
		ID:            "workflow-resume-approval",
		WorkspaceRoot: workspaceRoot,
		Name:          "Resume approval workflow",
		Trigger:       "manual",
		Steps:         `{"steps":[{"type":"role_task","role":"planner","prompt":"Draft the change.","name":"Draft"},{"type":"approval","action":"deploy release","risk":"high","summary":"Deploy to production"},{"type":"role_task","role":"reviewer","prompt":"Continue after approval.","name":"Post approval"}]}`,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := store.Workflows().Create(ctx, workflow); err != nil {
		t.Fatalf("create workflow: %v", err)
	}

	manager := &fakeTaskManager{
		store: store,
		waitFn: func(ctx context.Context, task contracts.Task) (contracts.Task, error) {
			task.State = "completed"
			task.Outcome = "success"
			task.FinalText = "Completed " + task.RoleID
			task.UpdatedAt = time.Now().UTC()
			reportID := "report-" + task.RoleID
			if err := store.Reports().Create(ctx, contracts.Report{
				ID:         reportID,
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

	service := NewService(store, manager, "main-session")
	run, err := service.Trigger(ctx, workflow, TriggerInput{TriggerRef: "manual:resume-approval"})
	if err != nil {
		t.Fatalf("Trigger: %v", err)
	}
	if run.State != "waiting_approval" {
		t.Fatalf("run state = %q, want waiting_approval", run.State)
	}
	if len(manager.created) != 1 || manager.created[0].RoleID != "planner" {
		t.Fatalf("created before approval = %+v", manager.created)
	}
	approvalIDs := decodeStringSlice(t, run.ApprovalIDs)
	if len(approvalIDs) != 1 {
		t.Fatalf("approval_ids = %v, want one", approvalIDs)
	}
	approval, err := store.Workflows().GetApproval(ctx, approvalIDs[0])
	if err != nil {
		t.Fatalf("GetApproval: %v", err)
	}
	approval.State = "approved"
	approval.DecidedBy = "operator"
	approval.DecidedAt = time.Now().UTC()
	approval.UpdatedAt = approval.DecidedAt
	if err := store.Workflows().UpdateApproval(ctx, approval); err != nil {
		t.Fatalf("UpdateApproval: %v", err)
	}

	resumed, err := service.Resume(ctx, run.ID)
	if err != nil {
		t.Fatalf("Resume: %v", err)
	}
	if resumed.State != "completed" {
		t.Fatalf("resumed state = %q, want completed", resumed.State)
	}
	if len(manager.created) != 2 || manager.created[1].RoleID != "reviewer" {
		t.Fatalf("created after resume = %+v", manager.created)
	}
	taskIDs := decodeStringSlice(t, resumed.TaskIDs)
	if len(taskIDs) != 2 || taskIDs[0] != manager.created[0].ID || taskIDs[1] != manager.created[1].ID {
		t.Fatalf("task_ids = %v, created = %+v", taskIDs, manager.created)
	}
	reportIDs := decodeStringSlice(t, resumed.ReportIDs)
	if len(reportIDs) != 2 || reportIDs[0] != "report-planner" || reportIDs[1] != "report-reviewer" {
		t.Fatalf("report_ids = %v", reportIDs)
	}
	trace := decodeTrace(t, resumed.Trace)
	if !hasTraceState(trace, "run_resumed", "running") {
		t.Fatalf("trace missing run_resumed: %+v", trace)
	}
	if !hasTraceState(trace, "run_completed", "completed") {
		t.Fatalf("trace missing run_completed: %+v", trace)
	}
}

func TestServiceResumeRejectsPendingApproval(t *testing.T) {
	ctx := context.Background()
	store, err := v2store.OpenInMemory()
	if err != nil {
		t.Fatalf("OpenInMemory: %v", err)
	}
	defer store.Close()

	workspaceRoot := t.TempDir()
	now := time.Now().UTC()
	workflow := contracts.Workflow{
		ID:            "workflow-resume-pending",
		WorkspaceRoot: workspaceRoot,
		Name:          "Resume pending workflow",
		Trigger:       "manual",
		Steps:         `{"steps":[{"type":"approval","action":"deploy"},{"type":"role_task","role":"reviewer","prompt":"Continue"}]}`,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := store.Workflows().Create(ctx, workflow); err != nil {
		t.Fatalf("create workflow: %v", err)
	}
	service := NewService(store, &fakeTaskManager{store: store}, "main-session")
	run, err := service.Trigger(ctx, workflow, TriggerInput{})
	if err != nil {
		t.Fatalf("Trigger: %v", err)
	}

	if _, err := service.Resume(ctx, run.ID); !errors.Is(err, ErrInvalidWorkflow) {
		t.Fatalf("Resume error = %v, want ErrInvalidWorkflow", err)
	}
	got, err := store.Workflows().GetRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if got.State != "waiting_approval" {
		t.Fatalf("run state = %q, want waiting_approval", got.State)
	}
}

func TestServiceResumeStopsAtNextApprovalGate(t *testing.T) {
	ctx := context.Background()
	store, err := v2store.OpenInMemory()
	if err != nil {
		t.Fatalf("OpenInMemory: %v", err)
	}
	defer store.Close()

	workspaceRoot := t.TempDir()
	now := time.Now().UTC()
	if err := store.Sessions().Create(ctx, contracts.Session{
		ID:            "main-session",
		WorkspaceRoot: workspaceRoot,
		State:         "idle",
		CreatedAt:     now,
		UpdatedAt:     now,
	}); err != nil {
		t.Fatalf("create session: %v", err)
	}
	workflow := contracts.Workflow{
		ID:            "workflow-resume-next-approval",
		WorkspaceRoot: workspaceRoot,
		Name:          "Resume next approval workflow",
		Trigger:       "manual",
		Steps:         `{"steps":[{"type":"approval","action":"first approval"},{"type":"role_task","role":"reviewer","prompt":"Continue"},{"type":"approval","action":"second approval"},{"type":"role_task","role":"publisher","prompt":"Publish"}]}`,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := store.Workflows().Create(ctx, workflow); err != nil {
		t.Fatalf("create workflow: %v", err)
	}
	manager := &fakeTaskManager{
		store: store,
		waitFn: func(ctx context.Context, task contracts.Task) (contracts.Task, error) {
			task.State = "completed"
			task.Outcome = "success"
			task.FinalText = "Completed " + task.RoleID
			task.UpdatedAt = time.Now().UTC()
			return task, nil
		},
	}
	service := NewService(store, manager, "main-session")
	run, err := service.Trigger(ctx, workflow, TriggerInput{})
	if err != nil {
		t.Fatalf("Trigger: %v", err)
	}
	firstApprovalID := decodeStringSlice(t, run.ApprovalIDs)[0]
	firstApproval, err := store.Workflows().GetApproval(ctx, firstApprovalID)
	if err != nil {
		t.Fatalf("GetApproval: %v", err)
	}
	firstApproval.State = "approved"
	firstApproval.DecidedBy = "operator"
	firstApproval.DecidedAt = time.Now().UTC()
	firstApproval.UpdatedAt = firstApproval.DecidedAt
	if err := store.Workflows().UpdateApproval(ctx, firstApproval); err != nil {
		t.Fatalf("UpdateApproval: %v", err)
	}

	resumed, err := service.Resume(ctx, run.ID)
	if err != nil {
		t.Fatalf("Resume: %v", err)
	}
	if resumed.State != "waiting_approval" {
		t.Fatalf("resumed state = %q, want waiting_approval", resumed.State)
	}
	if len(manager.created) != 1 || manager.created[0].RoleID != "reviewer" {
		t.Fatalf("created tasks = %+v, want reviewer only", manager.created)
	}
	approvalIDs := decodeStringSlice(t, resumed.ApprovalIDs)
	if len(approvalIDs) != 2 || approvalIDs[0] != firstApprovalID || approvalIDs[1] == "" {
		t.Fatalf("approval_ids = %v", approvalIDs)
	}
	if hasTraceEvent(decodeTrace(t, resumed.Trace), "run_completed") {
		t.Fatalf("trace should not complete at second approval gate: %s", resumed.Trace)
	}
}

func TestServiceTriggerApprovalStepRollsBackOnRunUpdateFailure(t *testing.T) {
	ctx := context.Background()
	baseStore, err := v2store.OpenInMemory()
	if err != nil {
		t.Fatalf("OpenInMemory: %v", err)
	}
	defer baseStore.Close()

	workspaceRoot := t.TempDir()
	now := time.Now().UTC()
	if err := baseStore.Sessions().Create(ctx, contracts.Session{
		ID:            "main-session",
		WorkspaceRoot: workspaceRoot,
		State:         "idle",
		CreatedAt:     now,
		UpdatedAt:     now,
	}); err != nil {
		t.Fatalf("create session: %v", err)
	}
	workflow := contracts.Workflow{
		ID:            "workflow-approval-update-failure",
		WorkspaceRoot: workspaceRoot,
		Name:          "Approval update failure workflow",
		Trigger:       "manual",
		Steps:         `[{"kind":"approval","requested_action":"deploy release","summary":"Deploy to production"}]`,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := baseStore.Workflows().Create(ctx, workflow); err != nil {
		t.Fatalf("create workflow: %v", err)
	}

	service := NewService(&approvalUpdateFailureStore{Store: baseStore}, &fakeTaskManager{store: baseStore}, "main-session")
	run, err := service.Trigger(ctx, workflow, TriggerInput{TriggerRef: "manual:approval-update-failure"})
	if err != nil {
		t.Fatalf("Trigger: %v", err)
	}
	if run.State != "failed" {
		t.Fatalf("run state = %q, want failed", run.State)
	}
	if got := decodeStringSlice(t, run.ApprovalIDs); len(got) != 0 {
		t.Fatalf("approval_ids = %v, want none", got)
	}

	trace := decodeTrace(t, run.Trace)
	if hasTraceEvent(trace, "approval_requested") {
		t.Fatalf("trace should not record approval_requested after rollback: %+v", trace)
	}
	if !hasTraceState(trace, "run_failed", "failed") {
		t.Fatalf("trace missing run_failed state: %+v", trace)
	}

	approvals, err := baseStore.Workflows().ListApprovalsByWorkspace(ctx, workspaceRoot, contracts.ApprovalListOptions{})
	if err != nil {
		t.Fatalf("ListApprovalsByWorkspace: %v", err)
	}
	if len(approvals) != 0 {
		t.Fatalf("approvals = %+v, want none", approvals)
	}

	gotRun, err := baseStore.Workflows().GetRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if gotRun.State != "failed" {
		t.Fatalf("persisted run state = %q, want failed", gotRun.State)
	}
	if got := decodeStringSlice(t, gotRun.ApprovalIDs); len(got) != 0 {
		t.Fatalf("persisted approval_ids = %v, want none", got)
	}
}

func TestServiceTriggerRejectsBadSteps(t *testing.T) {
	ctx := context.Background()
	store, err := v2store.OpenInMemory()
	if err != nil {
		t.Fatalf("OpenInMemory: %v", err)
	}
	defer store.Close()

	workspaceRoot := t.TempDir()
	now := time.Now().UTC()
	workflow := contracts.Workflow{
		ID:            "workflow-bad",
		WorkspaceRoot: workspaceRoot,
		Name:          "Bad workflow",
		Trigger:       "manual",
		Steps:         `{"steps":[{"kind":"shell","prompt":"echo nope"}]}`,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := store.Workflows().Create(ctx, workflow); err != nil {
		t.Fatalf("create workflow: %v", err)
	}

	service := NewService(store, &fakeTaskManager{store: store}, "")
	if _, err := service.Trigger(ctx, workflow, TriggerInput{}); !errors.Is(err, ErrInvalidWorkflow) {
		t.Fatalf("Trigger error = %v, want ErrInvalidWorkflow", err)
	}

	runs, err := store.Workflows().ListRunsByWorkspace(ctx, workspaceRoot, contracts.WorkflowRunListOptions{})
	if err != nil {
		t.Fatalf("ListRunsByWorkspace: %v", err)
	}
	if len(runs) != 0 {
		t.Fatalf("runs = %+v, want none", runs)
	}
}

func TestServiceTriggerFailedTaskUpdatesRun(t *testing.T) {
	ctx := context.Background()
	store, err := v2store.OpenInMemory()
	if err != nil {
		t.Fatalf("OpenInMemory: %v", err)
	}
	defer store.Close()

	workspaceRoot := t.TempDir()
	now := time.Now().UTC()
	if err := store.Sessions().Create(ctx, contracts.Session{
		ID:            "main-session",
		WorkspaceRoot: workspaceRoot,
		State:         "idle",
		CreatedAt:     now,
		UpdatedAt:     now,
	}); err != nil {
		t.Fatalf("create session: %v", err)
	}
	workflow := contracts.Workflow{
		ID:            "workflow-failed",
		WorkspaceRoot: workspaceRoot,
		Name:          "Failed workflow",
		Trigger:       "manual",
		Steps:         `[{"kind":"role_task","role_id":"reviewer","prompt":"Handle failure"}]`,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := store.Workflows().Create(ctx, workflow); err != nil {
		t.Fatalf("create workflow: %v", err)
	}

	manager := &fakeTaskManager{
		store: store,
		waitFn: func(ctx context.Context, task contracts.Task) (contracts.Task, error) {
			task.SessionID = "specialist-reviewer"
			task.State = "failed"
			task.Outcome = "failure"
			task.FinalText = "Role execution failed."
			task.UpdatedAt = time.Now().UTC()
			if err := store.Reports().Create(ctx, contracts.Report{
				ID:         "report-task-failed",
				SessionID:  task.ReportSessionID,
				SourceKind: "task_result",
				SourceID:   task.ID,
				Status:     task.State,
				Severity:   "error",
				Title:      "Task result: agent",
				Summary:    task.FinalText,
				CreatedAt:  time.Now().UTC(),
			}); err != nil {
				return contracts.Task{}, err
			}
			return task, nil
		},
	}

	service := NewService(store, manager, "main-session")
	run, err := service.Trigger(ctx, workflow, TriggerInput{})
	if err != nil {
		t.Fatalf("Trigger: %v", err)
	}
	if run.State != "failed" {
		t.Fatalf("run state = %q, want failed", run.State)
	}
	if len(decodeStringSlice(t, run.TaskIDs)) != 1 {
		t.Fatalf("task_ids = %s, want one task", run.TaskIDs)
	}
	if len(decodeStringSlice(t, run.ReportIDs)) != 1 {
		t.Fatalf("report_ids = %s, want one report", run.ReportIDs)
	}
	trace := decodeTrace(t, run.Trace)
	if !hasTraceState(trace, "task_completed", "failed") {
		t.Fatalf("trace missing failed task completion: %+v", trace)
	}
	if !hasTraceState(trace, "run_failed", "failed") {
		t.Fatalf("trace missing run_failed state: %+v", trace)
	}
	if hasTraceEvent(trace, "run_interrupted") {
		t.Fatalf("trace should not mark failed run as interrupted: %+v", trace)
	}
}

func TestServiceTriggerStartFailureCollectsReportIDs(t *testing.T) {
	ctx := context.Background()
	store, err := v2store.OpenInMemory()
	if err != nil {
		t.Fatalf("OpenInMemory: %v", err)
	}
	defer store.Close()

	workspaceRoot := t.TempDir()
	now := time.Now().UTC()
	if err := store.Sessions().Create(ctx, contracts.Session{
		ID:            "main-session",
		WorkspaceRoot: workspaceRoot,
		State:         "idle",
		CreatedAt:     now,
		UpdatedAt:     now,
	}); err != nil {
		t.Fatalf("create session: %v", err)
	}
	workflow := contracts.Workflow{
		ID:            "workflow-start-failure",
		WorkspaceRoot: workspaceRoot,
		Name:          "Start failure workflow",
		Trigger:       "manual",
		Steps:         `[{"kind":"role_task","role_id":"reviewer","prompt":"Start the task"}]`,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := store.Workflows().Create(ctx, workflow); err != nil {
		t.Fatalf("create workflow: %v", err)
	}

	manager := &fakeTaskManager{
		store: store,
		startFn: func(_ context.Context, task contracts.Task) error {
			return errors.New("bootstrap failed")
		},
		failFn: func(ctx context.Context, task contracts.Task, finalText string) error {
			if finalText == "" {
				t.Fatalf("expected finalText on fail")
			}
			return store.Reports().Create(ctx, contracts.Report{
				ID:         "report-start-failure",
				SessionID:  task.ReportSessionID,
				SourceKind: "task_result",
				SourceID:   task.ID,
				Status:     "failed",
				Severity:   "error",
				Title:      "Task result: agent",
				Summary:    finalText,
				CreatedAt:  time.Now().UTC(),
			})
		},
	}

	service := NewService(store, manager, "main-session")
	run, err := service.Trigger(ctx, workflow, TriggerInput{})
	if err != nil {
		t.Fatalf("Trigger: %v", err)
	}
	if run.State != "failed" {
		t.Fatalf("run state = %q, want failed", run.State)
	}
	reportIDs := decodeStringSlice(t, run.ReportIDs)
	if len(reportIDs) != 1 || reportIDs[0] != "report-start-failure" {
		t.Fatalf("report_ids = %v, want report-start-failure", reportIDs)
	}
}

func TestServiceTriggerCreateCanceledMarksRunInterrupted(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	store, err := v2store.OpenInMemory()
	if err != nil {
		t.Fatalf("OpenInMemory: %v", err)
	}
	defer store.Close()

	workspaceRoot := t.TempDir()
	now := time.Now().UTC()
	if err := store.Sessions().Create(context.Background(), contracts.Session{
		ID:            "main-session",
		WorkspaceRoot: workspaceRoot,
		State:         "idle",
		CreatedAt:     now,
		UpdatedAt:     now,
	}); err != nil {
		t.Fatalf("create session: %v", err)
	}
	workflow := contracts.Workflow{
		ID:            "workflow-create-cancelled",
		WorkspaceRoot: workspaceRoot,
		Name:          "Create cancelled workflow",
		Trigger:       "manual",
		Steps:         `[{"kind":"role_task","role_id":"reviewer","prompt":"Create then cancel"}]`,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := store.Workflows().Create(context.Background(), workflow); err != nil {
		t.Fatalf("create workflow: %v", err)
	}

	manager := &fakeTaskManager{
		store: store,
		createFn: func(ctx context.Context, task contracts.Task) error {
			if !errors.Is(ctx.Err(), context.Canceled) {
				t.Fatalf("create ctx err = %v, want context.Canceled", ctx.Err())
			}
			if task.ID == "" {
				t.Fatalf("expected generated task id")
			}
			return context.Canceled
		},
	}

	service := NewService(store, manager, "main-session")
	run, err := service.Trigger(ctx, workflow, TriggerInput{})
	if err != nil {
		t.Fatalf("Trigger: %v", err)
	}
	if run.State != "interrupted" {
		t.Fatalf("run state = %q, want interrupted", run.State)
	}
	if got := decodeStringSlice(t, run.TaskIDs); len(got) != 0 {
		t.Fatalf("task_ids = %v, want none", got)
	}
	if got := decodeStringSlice(t, run.ReportIDs); len(got) != 0 {
		t.Fatalf("report_ids = %v, want none", got)
	}
	trace := decodeTrace(t, run.Trace)
	if !hasTraceState(trace, "run_interrupted", "interrupted") {
		t.Fatalf("trace missing run_interrupted state: %+v", trace)
	}
	if hasTraceEvent(trace, "run_failed") {
		t.Fatalf("trace should distinguish interrupted from failed: %+v", trace)
	}
	if hasTraceEvent(trace, "task_created") {
		t.Fatalf("trace should not include task_created after create cancellation: %+v", trace)
	}
}

func TestServiceTriggerStartCanceledCollectsReportIDsAndInterrupts(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	store, err := v2store.OpenInMemory()
	if err != nil {
		t.Fatalf("OpenInMemory: %v", err)
	}
	defer store.Close()

	workspaceRoot := t.TempDir()
	now := time.Now().UTC()
	if err := store.Sessions().Create(context.Background(), contracts.Session{
		ID:            "main-session",
		WorkspaceRoot: workspaceRoot,
		State:         "idle",
		CreatedAt:     now,
		UpdatedAt:     now,
	}); err != nil {
		t.Fatalf("create session: %v", err)
	}
	workflow := contracts.Workflow{
		ID:            "workflow-start-cancelled",
		WorkspaceRoot: workspaceRoot,
		Name:          "Start cancelled workflow",
		Trigger:       "manual",
		Steps:         `[{"kind":"role_task","role_id":"reviewer","prompt":"Start then cancel"}]`,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := store.Workflows().Create(context.Background(), workflow); err != nil {
		t.Fatalf("create workflow: %v", err)
	}

	manager := &fakeTaskManager{
		store: store,
		startFn: func(ctx context.Context, task contracts.Task) error {
			if !errors.Is(ctx.Err(), context.Canceled) {
				t.Fatalf("start ctx err = %v, want context.Canceled", ctx.Err())
			}
			return context.Canceled
		},
		cancelFn: func(ctx context.Context, task contracts.Task) error {
			return store.Reports().Create(ctx, contracts.Report{
				ID:         "report-start-cancelled",
				SessionID:  task.ReportSessionID,
				SourceKind: "task_result",
				SourceID:   task.ID,
				Status:     "cancelled",
				Severity:   "warning",
				Title:      "Task result: agent",
				Summary:    "Task cancelled while starting.",
				CreatedAt:  time.Now().UTC(),
			})
		},
	}

	service := NewService(store, manager, "main-session")
	run, err := service.Trigger(ctx, workflow, TriggerInput{})
	if err != nil {
		t.Fatalf("Trigger: %v", err)
	}
	if run.State != "interrupted" {
		t.Fatalf("run state = %q, want interrupted", run.State)
	}
	reportIDs := decodeStringSlice(t, run.ReportIDs)
	if len(reportIDs) != 1 || reportIDs[0] != "report-start-cancelled" {
		t.Fatalf("report_ids = %v, want report-start-cancelled", reportIDs)
	}
	trace := decodeTrace(t, run.Trace)
	if !hasTraceState(trace, "run_interrupted", "interrupted") {
		t.Fatalf("trace missing run_interrupted state: %+v", trace)
	}
	if hasTraceEvent(trace, "run_failed") {
		t.Fatalf("trace should distinguish interrupted from failed: %+v", trace)
	}
}

func TestServiceTriggerCanceledContextCollectsReportIDs(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	store, err := v2store.OpenInMemory()
	if err != nil {
		t.Fatalf("OpenInMemory: %v", err)
	}
	defer store.Close()

	workspaceRoot := t.TempDir()
	now := time.Now().UTC()
	if err := store.Sessions().Create(context.Background(), contracts.Session{
		ID:            "main-session",
		WorkspaceRoot: workspaceRoot,
		State:         "idle",
		CreatedAt:     now,
		UpdatedAt:     now,
	}); err != nil {
		t.Fatalf("create session: %v", err)
	}
	workflow := contracts.Workflow{
		ID:            "workflow-cancelled",
		WorkspaceRoot: workspaceRoot,
		Name:          "Cancelled workflow",
		Trigger:       "manual",
		Steps:         `[{"kind":"role_task","role_id":"reviewer","prompt":"Wait for cancellation"}]`,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := store.Workflows().Create(context.Background(), workflow); err != nil {
		t.Fatalf("create workflow: %v", err)
	}

	manager := &fakeTaskManager{
		store: store,
		waitFn: func(ctx context.Context, task contracts.Task) (contracts.Task, error) {
			if !errors.Is(ctx.Err(), context.Canceled) {
				t.Fatalf("wait ctx err = %v, want context.Canceled", ctx.Err())
			}
			return contracts.Task{}, context.Canceled
		},
		cancelFn: func(ctx context.Context, task contracts.Task) error {
			return store.Reports().Create(ctx, contracts.Report{
				ID:         "report-cancelled",
				SessionID:  task.ReportSessionID,
				SourceKind: "task_result",
				SourceID:   task.ID,
				Status:     "cancelled",
				Severity:   "warning",
				Title:      "Task result: agent",
				Summary:    "Task cancelled.",
				CreatedAt:  time.Now().UTC(),
			})
		},
	}

	service := NewService(store, manager, "main-session")
	run, err := service.Trigger(ctx, workflow, TriggerInput{})
	if err != nil {
		t.Fatalf("Trigger: %v", err)
	}
	if run.State != "interrupted" {
		t.Fatalf("run state = %q, want interrupted", run.State)
	}
	reportIDs := decodeStringSlice(t, run.ReportIDs)
	if len(reportIDs) != 1 || reportIDs[0] != "report-cancelled" {
		t.Fatalf("report_ids = %v, want report-cancelled", reportIDs)
	}
	trace := decodeTrace(t, run.Trace)
	if !hasTraceState(trace, "run_interrupted", "interrupted") {
		t.Fatalf("trace missing interrupted run state: %+v", trace)
	}
	if hasTraceEvent(trace, "run_failed") {
		t.Fatalf("trace should distinguish interrupted from failed: %+v", trace)
	}
}

func TestServiceTriggerWaitDeadlineExceededCollectsReportIDsAndInterrupts(t *testing.T) {
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()

	store, err := v2store.OpenInMemory()
	if err != nil {
		t.Fatalf("OpenInMemory: %v", err)
	}
	defer store.Close()

	workspaceRoot := t.TempDir()
	now := time.Now().UTC()
	if err := store.Sessions().Create(context.Background(), contracts.Session{
		ID:            "main-session",
		WorkspaceRoot: workspaceRoot,
		State:         "idle",
		CreatedAt:     now,
		UpdatedAt:     now,
	}); err != nil {
		t.Fatalf("create session: %v", err)
	}
	workflow := contracts.Workflow{
		ID:            "workflow-deadline-exceeded",
		WorkspaceRoot: workspaceRoot,
		Name:          "Deadline exceeded workflow",
		Trigger:       "manual",
		Steps:         `[{"kind":"role_task","role_id":"reviewer","prompt":"Wait for deadline"}]`,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := store.Workflows().Create(context.Background(), workflow); err != nil {
		t.Fatalf("create workflow: %v", err)
	}

	manager := &fakeTaskManager{
		store: store,
		waitFn: func(ctx context.Context, task contracts.Task) (contracts.Task, error) {
			if !errors.Is(ctx.Err(), context.DeadlineExceeded) {
				t.Fatalf("wait ctx err = %v, want context.DeadlineExceeded", ctx.Err())
			}
			return contracts.Task{}, context.DeadlineExceeded
		},
		cancelFn: func(ctx context.Context, task contracts.Task) error {
			return store.Reports().Create(ctx, contracts.Report{
				ID:         "report-deadline-exceeded",
				SessionID:  task.ReportSessionID,
				SourceKind: "task_result",
				SourceID:   task.ID,
				Status:     "cancelled",
				Severity:   "warning",
				Title:      "Task result: agent",
				Summary:    "Task interrupted by deadline.",
				CreatedAt:  time.Now().UTC(),
			})
		},
	}

	service := NewService(store, manager, "main-session")
	run, err := service.Trigger(ctx, workflow, TriggerInput{})
	if err != nil {
		t.Fatalf("Trigger: %v", err)
	}
	if run.State != "interrupted" {
		t.Fatalf("run state = %q, want interrupted", run.State)
	}
	reportIDs := decodeStringSlice(t, run.ReportIDs)
	if len(reportIDs) != 1 || reportIDs[0] != "report-deadline-exceeded" {
		t.Fatalf("report_ids = %v, want report-deadline-exceeded", reportIDs)
	}
	trace := decodeTrace(t, run.Trace)
	if !hasTraceState(trace, "run_interrupted", "interrupted") {
		t.Fatalf("trace missing interrupted run state: %+v", trace)
	}
	if hasTraceEvent(trace, "run_failed") {
		t.Fatalf("trace should distinguish interrupted from failed: %+v", trace)
	}
}

func TestServiceTriggerAsyncReusesRunForSameTriggerRef(t *testing.T) {
	ctx := context.Background()
	store, err := v2store.OpenInMemory()
	if err != nil {
		t.Fatalf("OpenInMemory: %v", err)
	}
	defer store.Close()

	workspaceRoot := t.TempDir()
	now := time.Now().UTC()
	if err := store.Sessions().Create(ctx, contracts.Session{
		ID:            "main-session",
		WorkspaceRoot: workspaceRoot,
		State:         "idle",
		CreatedAt:     now,
		UpdatedAt:     now,
	}); err != nil {
		t.Fatalf("create session: %v", err)
	}
	workflow := contracts.Workflow{
		ID:            "workflow-async-dedupe",
		WorkspaceRoot: workspaceRoot,
		Name:          "Async dedupe workflow",
		Trigger:       "manual",
		Steps:         `[{"kind":"role_task","role_id":"reviewer","prompt":"Review the change"}]`,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := store.Workflows().Create(ctx, workflow); err != nil {
		t.Fatalf("create workflow: %v", err)
	}

	waitStarted := make(chan string, 2)
	waitRelease := make(chan struct{})
	manager := &fakeTaskManager{
		store: store,
		waitFn: func(ctx context.Context, task contracts.Task) (contracts.Task, error) {
			waitStarted <- task.ID
			<-waitRelease
			task.State = "completed"
			task.Outcome = "success"
			task.FinalText = "Async dedupe complete."
			task.UpdatedAt = time.Now().UTC()
			return task, nil
		},
	}

	service := NewService(store, manager, "main-session")
	first, err := service.TriggerAsync(ctx, workflow, TriggerInput{TriggerRef: "automation:docs-stale"})
	if err != nil {
		t.Fatalf("TriggerAsync first: %v", err)
	}
	second, err := service.TriggerAsync(ctx, workflow, TriggerInput{TriggerRef: "automation:docs-stale"})
	if err != nil {
		t.Fatalf("TriggerAsync second: %v", err)
	}
	if second.ID != first.ID {
		t.Fatalf("second run id = %q, want %q", second.ID, first.ID)
	}

	select {
	case <-waitStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("workflow wait did not start")
	}
	select {
	case extra := <-waitStarted:
		t.Fatalf("duplicate workflow task started: %s", extra)
	case <-time.After(150 * time.Millisecond):
	}
	if len(manager.created) != 1 {
		t.Fatalf("created tasks = %d, want 1", len(manager.created))
	}

	runs, err := store.Workflows().ListRunsByWorkspace(ctx, workspaceRoot, contracts.WorkflowRunListOptions{})
	if err != nil {
		t.Fatalf("ListRunsByWorkspace: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("workflow runs = %+v, want one run", runs)
	}

	close(waitRelease)
	finalRun := waitForWorkflowRunState(t, store, first.ID, "completed")
	if finalRun.TriggerRef != "automation:docs-stale" {
		t.Fatalf("trigger_ref = %q, want automation:docs-stale", finalRun.TriggerRef)
	}
	if finalRun.DedupeRef != "automation:docs-stale" {
		t.Fatalf("dedupe_ref = %q, want automation:docs-stale", finalRun.DedupeRef)
	}
}

func TestServiceTriggerAsyncDeadlineMarksRunInterrupted(t *testing.T) {
	ctx := context.Background()
	store, err := v2store.OpenInMemory()
	if err != nil {
		t.Fatalf("OpenInMemory: %v", err)
	}
	defer store.Close()

	workspaceRoot := t.TempDir()
	now := time.Now().UTC()
	if err := store.Sessions().Create(ctx, contracts.Session{
		ID:            "main-session",
		WorkspaceRoot: workspaceRoot,
		State:         "idle",
		CreatedAt:     now,
		UpdatedAt:     now,
	}); err != nil {
		t.Fatalf("create session: %v", err)
	}
	workflow := contracts.Workflow{
		ID:            "workflow-async-deadline",
		WorkspaceRoot: workspaceRoot,
		Name:          "Async deadline workflow",
		Trigger:       "manual",
		Steps:         `[{"kind":"role_task","role_id":"reviewer","prompt":"Wait for timeout"}]`,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := store.Workflows().Create(ctx, workflow); err != nil {
		t.Fatalf("create workflow: %v", err)
	}

	manager := &fakeTaskManager{
		store: store,
		waitFn: func(ctx context.Context, task contracts.Task) (contracts.Task, error) {
			<-ctx.Done()
			return contracts.Task{}, ctx.Err()
		},
		cancelFn: func(ctx context.Context, task contracts.Task) error {
			return store.Reports().Create(ctx, contracts.Report{
				ID:         "report-async-deadline",
				SessionID:  task.ReportSessionID,
				SourceKind: "task_result",
				SourceID:   task.ID,
				Status:     "cancelled",
				Severity:   "warning",
				Title:      "Task result: agent",
				Summary:    "Task interrupted by async deadline.",
				CreatedAt:  time.Now().UTC(),
			})
		},
	}

	service := NewService(store, manager, "main-session")
	service.SetAsyncTimeout(50 * time.Millisecond)
	run, err := service.TriggerAsync(ctx, workflow, TriggerInput{TriggerRef: "manual:async-deadline"})
	if err != nil {
		t.Fatalf("TriggerAsync: %v", err)
	}

	finalRun := waitForWorkflowRunState(t, store, run.ID, "interrupted")
	reportIDs := decodeStringSlice(t, finalRun.ReportIDs)
	if len(reportIDs) != 1 || reportIDs[0] != "report-async-deadline" {
		t.Fatalf("report_ids = %v, want report-async-deadline", reportIDs)
	}
	trace := decodeTrace(t, finalRun.Trace)
	if !hasTraceState(trace, "run_interrupted", "interrupted") {
		t.Fatalf("trace missing run_interrupted: %+v", trace)
	}
	foundDeadline := false
	for _, event := range trace {
		if event.Event == "run_interrupted" && strings.Contains(event.Error, context.DeadlineExceeded.Error()) {
			foundDeadline = true
			break
		}
	}
	if !foundDeadline {
		t.Fatalf("trace missing deadline error: %+v", trace)
	}
}

func TestServiceTriggerAsyncPanicMarksRunFailed(t *testing.T) {
	ctx := context.Background()
	store, err := v2store.OpenInMemory()
	if err != nil {
		t.Fatalf("OpenInMemory: %v", err)
	}
	defer store.Close()

	workspaceRoot := t.TempDir()
	now := time.Now().UTC()
	if err := store.Sessions().Create(ctx, contracts.Session{
		ID:            "main-session",
		WorkspaceRoot: workspaceRoot,
		State:         "idle",
		CreatedAt:     now,
		UpdatedAt:     now,
	}); err != nil {
		t.Fatalf("create session: %v", err)
	}
	workflow := contracts.Workflow{
		ID:            "workflow-async-panic",
		WorkspaceRoot: workspaceRoot,
		Name:          "Async panic workflow",
		Trigger:       "manual",
		Steps:         `[{"kind":"role_task","role_id":"reviewer","prompt":"Panic"}]`,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := store.Workflows().Create(ctx, workflow); err != nil {
		t.Fatalf("create workflow: %v", err)
	}

	manager := &fakeTaskManager{
		store: store,
		waitFn: func(ctx context.Context, task contracts.Task) (contracts.Task, error) {
			panic("boom")
		},
	}

	service := NewService(store, manager, "main-session")
	run, err := service.TriggerAsync(ctx, workflow, TriggerInput{TriggerRef: "manual:async-panic"})
	if err != nil {
		t.Fatalf("TriggerAsync: %v", err)
	}

	finalRun := waitForWorkflowRunState(t, store, run.ID, "failed")
	trace := decodeTrace(t, finalRun.Trace)
	if !hasTraceState(trace, "run_failed", "failed") {
		t.Fatalf("trace missing run_failed: %+v", trace)
	}
	foundPanic := false
	for _, event := range trace {
		if event.Event == "run_failed" && strings.Contains(event.Error, "panic: boom") {
			foundPanic = true
			break
		}
	}
	if !foundPanic {
		t.Fatalf("trace missing panic error: %+v", trace)
	}
}

type fakeTaskManager struct {
	store    contracts.Store
	created  []contracts.Task
	tasks    map[string]contracts.Task
	createFn func(ctx context.Context, task contracts.Task) error
	startFn  func(ctx context.Context, task contracts.Task) error
	waitFn   func(ctx context.Context, task contracts.Task) (contracts.Task, error)
	cancelFn func(ctx context.Context, task contracts.Task) error
	failFn   func(ctx context.Context, task contracts.Task, finalText string) error
}

func (f *fakeTaskManager) Create(ctx context.Context, task contracts.Task) error {
	if f.createFn != nil {
		return f.createFn(ctx, task)
	}
	if f.tasks == nil {
		f.tasks = map[string]contracts.Task{}
	}
	f.tasks[task.ID] = task
	f.created = append(f.created, task)
	return nil
}

func (f *fakeTaskManager) Start(ctx context.Context, taskID string) error {
	task := f.tasks[taskID]
	if f.startFn != nil {
		return f.startFn(ctx, task)
	}
	task.State = "running"
	f.tasks[taskID] = task
	return nil
}

func (f *fakeTaskManager) Wait(ctx context.Context, taskID string) (contracts.Task, error) {
	task, ok := f.tasks[taskID]
	if !ok {
		return contracts.Task{}, errors.New("task not found")
	}
	if f.waitFn != nil {
		return f.waitFn(ctx, task)
	}
	task.State = "completed"
	task.Outcome = "success"
	task.FinalText = "completed"
	return task, nil
}

func (f *fakeTaskManager) Cancel(ctx context.Context, taskID string) error {
	task, ok := f.tasks[taskID]
	if !ok {
		return errors.New("task not found")
	}
	if f.cancelFn != nil {
		return f.cancelFn(ctx, task)
	}
	task.State = "cancelled"
	task.UpdatedAt = time.Now().UTC()
	f.tasks[taskID] = task
	return nil
}

func (f *fakeTaskManager) Fail(ctx context.Context, taskID string, finalText string) error {
	task, ok := f.tasks[taskID]
	if !ok {
		return errors.New("task not found")
	}
	if f.failFn != nil {
		return f.failFn(ctx, task, finalText)
	}
	task.State = "failed"
	task.FinalText = finalText
	task.UpdatedAt = time.Now().UTC()
	f.tasks[taskID] = task
	return nil
}

func decodeStringSlice(t *testing.T, raw string) []string {
	t.Helper()
	var out []string
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		t.Fatalf("decode string slice %q: %v", raw, err)
	}
	return out
}

func decodeTrace(t *testing.T, raw string) []workflowTraceEvent {
	t.Helper()
	var out []workflowTraceEvent
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		t.Fatalf("decode trace %q: %v", raw, err)
	}
	return out
}

func hasTraceEvent(trace []workflowTraceEvent, event string) bool {
	for _, item := range trace {
		if item.Event == event {
			return true
		}
	}
	return false
}

type approvalUpdateFailureStore struct {
	contracts.Store
}

func (s *approvalUpdateFailureStore) WithTx(ctx context.Context, fn func(tx contracts.Store) error) error {
	return s.Store.WithTx(ctx, func(tx contracts.Store) error {
		return fn(&approvalUpdateFailureTxStore{
			Store:     tx,
			workflows: &approvalUpdateFailureRepo{WorkflowRepository: tx.Workflows()},
		})
	})
}

type approvalUpdateFailureTxStore struct {
	contracts.Store
	workflows contracts.WorkflowRepository
}

func (s *approvalUpdateFailureTxStore) Workflows() contracts.WorkflowRepository {
	return s.workflows
}

type approvalUpdateFailureRepo struct {
	contracts.WorkflowRepository
}

func (r *approvalUpdateFailureRepo) UpdateRun(ctx context.Context, run contracts.WorkflowRun) error {
	if run.State == "waiting_approval" {
		return errors.New("forced workflow run update failure")
	}
	return r.WorkflowRepository.UpdateRun(ctx, run)
}

func hasTraceState(trace []workflowTraceEvent, event, state string) bool {
	for _, item := range trace {
		if item.Event == event && item.State == state {
			return true
		}
	}
	return false
}

func waitForWorkflowRunState(t *testing.T, s contracts.Store, runID, wantState string) contracts.WorkflowRun {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		run, err := s.Workflows().GetRun(context.Background(), runID)
		if err == nil && run.State == wantState {
			return run
		}
		time.Sleep(10 * time.Millisecond)
	}
	run, err := s.Workflows().GetRun(context.Background(), runID)
	if err != nil {
		t.Fatalf("GetRun after wait: %v", err)
	}
	t.Fatalf("workflow run state = %q, want %q", run.State, wantState)
	return contracts.WorkflowRun{}
}
