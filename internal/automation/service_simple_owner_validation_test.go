package automation

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"go-agent/internal/types"
)

type applyValidationStore struct {
	upsertCount int
}

func (s *applyValidationStore) UpsertAutomation(_ context.Context, _ types.AutomationSpec) error {
	s.upsertCount++
	return nil
}

func (s *applyValidationStore) GetAutomation(context.Context, string) (types.AutomationSpec, bool, error) {
	return types.AutomationSpec{}, false, nil
}

func (s *applyValidationStore) ListAutomations(context.Context, types.AutomationListFilter) ([]types.AutomationSpec, error) {
	return nil, nil
}

func (s *applyValidationStore) DeleteAutomation(context.Context, string) (bool, error) {
	return false, nil
}

func (s *applyValidationStore) UpsertTriggerEvent(context.Context, types.TriggerEvent) error {
	return nil
}

func (s *applyValidationStore) UpsertAutomationHeartbeat(context.Context, types.AutomationHeartbeat) error {
	return nil
}

func (s *applyValidationStore) UpsertSimpleAutomationRun(context.Context, types.SimpleAutomationRun) error {
	return nil
}

func (s *applyValidationStore) GetSimpleAutomationRun(context.Context, string, string) (types.SimpleAutomationRun, bool, error) {
	return types.SimpleAutomationRun{}, false, nil
}

func TestSimpleAutomationOwnerValidatedAtApplyTime(t *testing.T) {
	store := &applyValidationStore{}
	service := NewService(store)

	spec := minimalAutomationSpecForServiceValidation()
	spec.Mode = types.AutomationModeSimple
	spec.Owner = "log_repairer"

	_, err := service.Apply(context.Background(), spec)
	var validationErr *types.AutomationValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("Apply() error = %v", err)
	}
	if store.upsertCount != 0 {
		t.Fatalf("upsertCount = %d", store.upsertCount)
	}
}

func TestSimpleAutomationIDRejectsPathTraversalAtApplyTime(t *testing.T) {
	store := &applyValidationStore{}
	service := NewService(store)

	spec := minimalAutomationSpecForServiceValidation()
	spec.ID = "../escape"
	spec.Mode = types.AutomationModeSimple
	spec.Owner = "role:log_repairer"

	_, err := service.Apply(context.Background(), spec)
	var validationErr *types.AutomationValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("Apply() error = %v", err)
	}
	if validationErr.Code != "invalid_automation_spec" {
		t.Fatalf("validation code = %q", validationErr.Code)
	}
	if store.upsertCount != 0 {
		t.Fatalf("upsertCount = %d", store.upsertCount)
	}
}

func TestRoleBoundAutomationRejectsLegacyWatchScriptSelectorAtApplyTime(t *testing.T) {
	store := &applyValidationStore{}
	service := NewService(store)

	spec := minimalAutomationSpecForServiceValidation()
	spec.ID = "cleanup_docs_a"
	spec.Mode = types.AutomationModeSimple
	spec.Owner = "role:doc_cleanup_operator"
	spec.Signals = []types.AutomationSignal{{
		Kind:     "poll",
		Source:   "simple_builder:watch_script",
		Selector: filepath.ToSlash(filepath.Join("tmp", "detect.sh")),
		Payload:  json.RawMessage(`{"interval_seconds":5,"timeout_seconds":30,"trigger_on":"script_status","signal_kind":"simple_watcher","summary":"simple automation watcher match"}`),
	}}

	_, err := service.Apply(context.Background(), spec)
	var validationErr *types.AutomationValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("Apply() error = %v", err)
	}
	if validationErr.Code != "invalid_automation_spec" {
		t.Fatalf("validation code = %q", validationErr.Code)
	}
	if store.upsertCount != 0 {
		t.Fatalf("upsertCount = %d", store.upsertCount)
	}
}

func TestValidateWatcherContractErrorIncludesCopyableScriptStatusExample(t *testing.T) {
	err := ValidateWatcherContract(context.Background(), `printf %s '{"triggered":false}'`, "")
	if err == nil {
		t.Fatal("expected watcher contract error")
	}
	message := err.Error()
	required := []string{
		`{"status":"healthy","summary":"no .txt files found","facts":{"count":0}}`,
		`{"status":"needs_agent","summary":"found .txt files to clean","facts":{"count":2}}`,
		"healthy",
		"needs_agent",
	}
	for _, text := range required {
		if !strings.Contains(message, text) {
			t.Fatalf("error message missing %q:\n%s", text, message)
		}
	}
}

func minimalAutomationSpecForServiceValidation() types.AutomationSpec {
	object := json.RawMessage(`{}`)
	return types.AutomationSpec{
		Title:            "Simple Automation",
		WorkspaceRoot:    "/workspace",
		Goal:             "Do a deterministic task",
		WatcherLifecycle: object,
		RetriggerPolicy:  object,
	}
}
