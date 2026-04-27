package automation

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go-agent/internal/task"
	"go-agent/internal/types"
)

type cutoverServiceStore struct {
	spec              types.AutomationSpec
	automationUpserts int
	triggerEvents     []types.TriggerEvent
	runs              map[string]types.SimpleAutomationRun
	deleteCalls       []string
}

func newCutoverServiceStore(spec types.AutomationSpec) *cutoverServiceStore {
	return &cutoverServiceStore{
		spec: spec,
		runs: map[string]types.SimpleAutomationRun{},
	}
}

func (s *cutoverServiceStore) UpsertAutomation(context.Context, types.AutomationSpec) error {
	s.automationUpserts++
	return nil
}

func (s *cutoverServiceStore) GetAutomation(_ context.Context, id string) (types.AutomationSpec, bool, error) {
	if id != s.spec.ID {
		return types.AutomationSpec{}, false, nil
	}
	return s.spec, true, nil
}

func (s *cutoverServiceStore) ListAutomations(context.Context, types.AutomationListFilter) ([]types.AutomationSpec, error) {
	return nil, nil
}

func (s *cutoverServiceStore) DeleteAutomation(context.Context, string) (bool, error) {
	return true, nil
}

func (s *cutoverServiceStore) UpsertTriggerEvent(_ context.Context, event types.TriggerEvent) error {
	s.triggerEvents = append(s.triggerEvents, event)
	return nil
}

func (s *cutoverServiceStore) UpsertAutomationHeartbeat(context.Context, types.AutomationHeartbeat) error {
	return nil
}

func (s *cutoverServiceStore) UpsertSimpleAutomationRun(_ context.Context, run types.SimpleAutomationRun) error {
	s.runs[s.runKey(run.AutomationID, run.DedupeKey)] = run
	return nil
}

func (s *cutoverServiceStore) ClaimSimpleAutomationRun(_ context.Context, run types.SimpleAutomationRun) (bool, error) {
	key := s.runKey(run.AutomationID, run.DedupeKey)
	if existing, ok := s.runs[key]; ok && strings.EqualFold(strings.TrimSpace(existing.LastStatus), "running") {
		return false, nil
	}
	s.runs[key] = run
	return true, nil
}

func (s *cutoverServiceStore) GetSimpleAutomationRun(_ context.Context, automationID, dedupeKey string) (types.SimpleAutomationRun, bool, error) {
	run, ok := s.runs[s.runKey(automationID, dedupeKey)]
	return run, ok, nil
}

func (s *cutoverServiceStore) runKey(automationID, dedupeKey string) string {
	return automationID + "|" + dedupeKey
}

type cutoverTaskManager struct {
	creates int
	last    task.CreateTaskInput
}

func (m *cutoverTaskManager) Create(_ context.Context, input task.CreateTaskInput) (task.Task, error) {
	m.creates++
	m.last = input
	return task.Task{ID: fmt.Sprintf("task_%d", m.creates)}, nil
}

func TestEmitTriggerRejectsManagedMode(t *testing.T) {
	store := newCutoverServiceStore(types.AutomationSpec{
		ID:            "auto_managed",
		Title:         "Managed",
		Goal:          "legacy",
		WorkspaceRoot: "/workspace",
		Mode:          types.AutomationMode("managed"),
	})
	service := NewService(store)

	_, err := service.EmitTrigger(context.Background(), types.AutomationTriggerRequest{
		AutomationID: "auto_managed",
		SignalKind:   "manual",
		Source:       "test",
		Summary:      "trigger",
	})
	var validationErr *types.AutomationValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("EmitTrigger() err = %v", err)
	}
	if validationErr.Code != "unsupported_automation_mode" {
		t.Fatalf("validation code = %q", validationErr.Code)
	}
	if len(store.triggerEvents) != 0 {
		t.Fatalf("trigger event writes = %d", len(store.triggerEvents))
	}
}

func TestEmitTriggerSimpleModeRoutesToSimpleRuntimeOnly(t *testing.T) {
	store := newCutoverServiceStore(types.AutomationSpec{
		ID:            "auto_simple",
		Title:         "Simple",
		Goal:          "run owner task",
		WorkspaceRoot: "/workspace",
		Mode:          types.AutomationModeSimple,
		Owner:         "role:log_repairer",
	})
	taskManager := &cutoverTaskManager{}
	runtime := NewSimpleRuntime(store, taskManager, SimpleRuntimeConfig{})
	service := NewService(store)
	service.SetSimpleRuntime(runtime)

	trigger, err := service.EmitTrigger(context.Background(), types.AutomationTriggerRequest{
		AutomationID: "auto_simple",
		SignalKind:   "manual",
		Source:       "test",
		Summary:      "trigger",
	})
	if err != nil {
		t.Fatalf("EmitTrigger() err = %v", err)
	}
	if trigger.EventID == "" {
		t.Fatal("expected non-empty trigger event id")
	}
	if len(store.triggerEvents) != 1 {
		t.Fatalf("trigger event writes = %d", len(store.triggerEvents))
	}
	if taskManager.creates != 1 {
		t.Fatalf("simple runtime task creates = %d", taskManager.creates)
	}
	if taskManager.last.Owner != "role:log_repairer" {
		t.Fatalf("created Owner = %q", taskManager.last.Owner)
	}
	if taskManager.last.TargetRole != "log_repairer" {
		t.Fatalf("created TargetRole = %q", taskManager.last.TargetRole)
	}
}

func TestApplyRejectsManagedMode(t *testing.T) {
	store := newCutoverServiceStore(types.AutomationSpec{ID: "auto_simple"})
	service := NewService(store)

	_, err := service.Apply(context.Background(), types.AutomationSpec{
		Title:         "Managed",
		WorkspaceRoot: "/workspace",
		Goal:          "legacy",
		State:         types.AutomationStateActive,
		Mode:          types.AutomationMode("managed"),
		Owner:         "role:log_repairer",
	})
	var validationErr *types.AutomationValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("Apply() err = %v", err)
	}
	if validationErr.Code != "unsupported_automation_mode" {
		t.Fatalf("validation code = %q", validationErr.Code)
	}
	if store.automationUpserts != 0 {
		t.Fatalf("automation upserts = %d", store.automationUpserts)
	}
}

func TestDeleteRemovesRoleBoundAutomationSourceDirectory(t *testing.T) {
	workspaceRoot := t.TempDir()
	sourceDir := filepath.Join(workspaceRoot, "roles", "doc_cleanup_operator", "automations", "cleanup_docs_a")
	if err := os.MkdirAll(sourceDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(sourceDir, "watch.sh"), []byte("#!/usr/bin/env bash\n"), 0o755); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(sourceDir, "automation.yaml"), []byte("automation_id: cleanup_docs_a\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	store := newCutoverServiceStore(types.AutomationSpec{
		ID:            "cleanup_docs_a",
		Title:         "Cleanup Docs A",
		Goal:          "Delete the file",
		WorkspaceRoot: workspaceRoot,
		Mode:          types.AutomationModeSimple,
		Owner:         "role:doc_cleanup_operator",
	})
	service := NewService(store)

	deleted, err := service.Delete(context.Background(), "cleanup_docs_a")
	if err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if !deleted {
		t.Fatal("Delete() = false, want true")
	}
	if _, err := os.Stat(sourceDir); !os.IsNotExist(err) {
		t.Fatalf("sourceDir still exists, err=%v", err)
	}
}
