package daemon

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go-agent/internal/automation"
	"go-agent/internal/store/sqlite"
	"go-agent/internal/task"
	"go-agent/internal/types"
)

type watcherOwnerFlowTaskManager struct {
	creates []task.CreateTaskInput
}

func (m *watcherOwnerFlowTaskManager) Create(_ context.Context, in task.CreateTaskInput) (task.Task, error) {
	m.creates = append(m.creates, in)
	return task.Task{ID: fmt.Sprintf("task_%d", len(m.creates))}, nil
}

type watcherOwnerFlowClient struct {
	store   *sqlite.Store
	service *automation.Service
}

func (c watcherOwnerFlowClient) GetAutomation(ctx context.Context, id string) (types.AutomationSpec, error) {
	spec, ok, err := c.store.GetAutomation(ctx, id)
	if err != nil {
		return types.AutomationSpec{}, err
	}
	if !ok {
		return types.AutomationSpec{}, fmt.Errorf("automation %q not found", id)
	}
	return spec, nil
}

func (c watcherOwnerFlowClient) EmitTrigger(ctx context.Context, req types.TriggerEmitRequest) (types.TriggerEvent, error) {
	return c.service.EmitTrigger(ctx, req)
}

func (c watcherOwnerFlowClient) RecordHeartbeat(ctx context.Context, req types.TriggerHeartbeatRequest) (types.AutomationHeartbeat, error) {
	return c.service.RecordHeartbeat(ctx, req)
}

func TestWatcherRunnerCheapDetectionRunsOwnerRoleAndReportsToMainAgent(t *testing.T) {
	ctx := context.Background()
	store := newDaemonTestStore(t)

	workspaceRoot := filepath.Join(t.TempDir(), "workspace")
	if err := os.MkdirAll(workspaceRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	writeRoleForNotifierTest(t, workspaceRoot, "log_repairer", "Repair logs")

	scriptDir := filepath.Join(workspaceRoot, "roles", "log_repairer", "automations", "cheap_scan_main_report")
	if err := os.MkdirAll(scriptDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	scriptPath := filepath.Join(scriptDir, "watch.sh")
	scriptBody := strings.Join([]string{
		"#!/usr/bin/env bash",
		"set -euo pipefail",
		"printf '%s' '{\"status\":\"needs_agent\",\"summary\":\"cheap hit\",\"facts\":{\"file_path\":\"automation-test/docs/a.txt\"},\"dedupe_key\":\"file:a.txt\"}'",
		"",
	}, "\n")
	if err := os.WriteFile(scriptPath, []byte(scriptBody), 0o755); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", scriptPath, err)
	}

	statePath := filepath.Join(t.TempDir(), "watcher-state.json")
	if err := os.WriteFile(statePath, []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", statePath, err)
	}

	now := time.Date(2026, time.April, 23, 9, 0, 0, 0, time.UTC)
	spec := types.AutomationSpec{
		ID:            "cheap_scan_main_report",
		Title:         "Cheap Scan",
		WorkspaceRoot: workspaceRoot,
		Goal:          "Detect simple file issues and dispatch owner task",
		State:         types.AutomationStateActive,
		Mode:          types.AutomationModeSimple,
		Owner:         "role:log_repairer",
		ReportTarget:  "main_agent",
		Signals: []types.AutomationSignal{{
			Kind:     "poll",
			Source:   "test:cheap_detector",
			Selector: filepath.ToSlash(filepath.Join("roles", "log_repairer", "automations", "cheap_scan_main_report", "watch.sh")),
			Payload:  []byte(`{"interval_seconds":1,"timeout_seconds":5,"trigger_on":"script_status","signal_kind":"simple_watcher","summary":"cheap watcher match"}`),
		}},
		WatcherLifecycle: []byte(`{"mode":"once"}`),
		RetriggerPolicy:  []byte(`{"cooldown_seconds":0}`),
	}

	taskManager := &watcherOwnerFlowTaskManager{}
	service := automation.NewService(store)
	service.SetClock(func() time.Time { return now })
	service.SetSimpleRuntime(automation.NewSimpleRuntime(store, taskManager, automation.SimpleRuntimeConfig{
		Now: func() time.Time { return now },
	}))
	if _, err := service.Apply(ctx, spec); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}

	runner := automation.NewWatcherRunner(watcherOwnerFlowClient{
		store:   store,
		service: service,
	}, automation.WatcherRunnerConfig{
		Now: func() time.Time { return now },
	})
	if err := runner.Run(ctx, spec.ID, "watcher:"+spec.ID, statePath); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if len(taskManager.creates) != 1 {
		t.Fatalf("task creates = %d", len(taskManager.creates))
	}
	created := taskManager.creates[0]
	if created.Kind != "automation_simple" {
		t.Fatalf("Kind = %q", created.Kind)
	}
	if created.Owner != "role:log_repairer" {
		t.Fatalf("Owner = %q", created.Owner)
	}
	if created.TargetRole != "log_repairer" {
		t.Fatalf("TargetRole = %q", created.TargetRole)
	}
	if !strings.Contains(created.Command, "detector_summary: cheap hit") {
		t.Fatalf("Command = %q, want detector summary", created.Command)
	}
	if !strings.Contains(created.Command, `"file_path":"automation-test/docs/a.txt"`) {
		t.Fatalf("Command = %q, want detector facts", created.Command)
	}

	run, ok, err := store.GetSimpleAutomationRun(ctx, spec.ID, "file:a.txt")
	if err != nil || !ok {
		t.Fatalf("GetSimpleAutomationRun() ok=%v err=%v", ok, err)
	}
	if run.TaskID != "task_1" {
		t.Fatalf("TaskID = %q", run.TaskID)
	}
	if run.LastStatus != "running" {
		t.Fatalf("LastStatus = %q", run.LastStatus)
	}
	if run.LastSummary != "cheap hit" {
		t.Fatalf("LastSummary = %q", run.LastSummary)
	}

	heartbeats, err := store.ListAutomationHeartbeats(ctx, types.AutomationHeartbeatFilter{
		AutomationID: spec.ID,
		WatcherID:    "watcher:" + spec.ID,
		Limit:        1,
	})
	if err != nil {
		t.Fatalf("ListAutomationHeartbeats() error = %v", err)
	}
	if len(heartbeats) != 1 {
		t.Fatalf("heartbeats = %d", len(heartbeats))
	}
	if heartbeats[0].Status != "paused" {
		t.Fatalf("Heartbeat Status = %q", heartbeats[0].Status)
	}

	notifier := taskTerminalNotifier{
		store: store,
		now:   func() time.Time { return now.Add(time.Minute) },
	}
	completed := completedSimpleAutomationTask(
		"task_1",
		workspaceRoot,
		task.TaskStatusCompleted,
		types.ChildAgentOutcomeSuccess,
		"repaired automation-test/docs/a.txt",
		"repaired automation-test/docs/a.txt",
		now.Add(time.Minute),
	)
	completed.Command = created.Command
	completed.Description = created.Description
	completed.TargetRole = created.TargetRole
	if err := notifier.NotifyTaskTerminal(ctx, completed); err != nil {
		t.Fatalf("NotifyTaskTerminal() error = %v", err)
	}

	mainSessionID, ok, err := store.GetRoleSessionID(ctx, workspaceRoot, types.SessionRoleMainParent)
	if err != nil || !ok {
		t.Fatalf("GetRoleSessionID(main_parent) ok=%v err=%v", ok, err)
	}
	reports, err := store.ListReportDeliveryItems(ctx, mainSessionID)
	if err != nil {
		t.Fatalf("ListReportDeliveryItems() error = %v", err)
	}
	if len(reports) != 1 {
		t.Fatalf("main queued reports = %d", len(reports))
	}
	if reports[0].Envelope.Status != "completed" {
		t.Fatalf("Status = %q", reports[0].Envelope.Status)
	}
	if reports[0].SourceID != "task_1" {
		t.Fatalf("TaskID = %q", reports[0].SourceID)
	}
	if !strings.Contains(reports[0].Envelope.Summary, "repaired automation-test/docs/a.txt") {
		t.Fatalf("Summary = %q", reports[0].Envelope.Summary)
	}
}
