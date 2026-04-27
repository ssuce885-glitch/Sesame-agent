package automation

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"go-agent/internal/task"
	"go-agent/internal/types"
)

func TestBuildSimpleAutomationPromptPinsOwnerTaskMode(t *testing.T) {
	prompt := buildSimpleAutomationPrompt(types.AutomationSpec{
		ID:    "box_txt_cleaner",
		Title: "Box Cleaner",
		Goal:  "Clean .txt files from ./box.",
	}, "found files", "box_txt_cleaner|simple", map[string]any{"count": 2})

	required := []string{
		"# Current Mode: Owner Task Mode",
		"Do not call automation_create_simple or automation_control.",
		"Execute automation_goal using the detector facts.",
		"Return the result as your final assistant response; the runtime delivers that response to the main agent report stream.",
		"Do not call delegate_to_role to report the result.",
	}
	for _, text := range required {
		if !strings.Contains(prompt, text) {
			t.Fatalf("prompt missing %q:\n%s", text, prompt)
		}
	}
}

func TestSimpleRuntimeDoesNotRedispatchRunningDedupe(t *testing.T) {
	store := &simpleRuntimeFakeStore{
		existing: types.SimpleAutomationRun{
			AutomationID: "reddit_ai_scanner_daily",
			DedupeKey:    "reddit-2026-04-27-4",
			LastStatus:   "running",
			TaskID:       "task_existing",
		},
	}
	tasks := &simpleRuntimeFakeTaskManager{}
	runtime := NewSimpleRuntime(store, tasks, SimpleRuntimeConfig{})

	payload := map[string]any{
		"status":     "needs_agent",
		"summary":    "scheduled scan UTC+8 12:00/20:00",
		"dedupe_key": "reddit-2026-04-27-4",
		"facts":      map[string]any{"hour_utc": 4},
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}

	err = runtime.HandleMatch(context.Background(), types.AutomationSpec{
		ID:            "reddit_ai_scanner_daily",
		WorkspaceRoot: "/tmp/workspace",
		Owner:         "role:reddit_ai_scanner",
		SimplePolicy: types.SimpleAutomationPolicy{
			OnSuccess: "continue",
			OnFailure: "pause",
			OnBlocked: "escalate",
		},
	}, types.TriggerEvent{
		AutomationID: "reddit_ai_scanner_daily",
		SignalKind:   "simple_watcher",
		Source:       "simple_builder:watch_script",
		Summary:      "scheduled scan UTC+8 12:00/20:00",
		DedupeKey:    "reddit-2026-04-27-4",
		Payload:      raw,
	})
	if err != nil {
		t.Fatal(err)
	}
	if tasks.created != 0 {
		t.Fatalf("expected existing dedupe to skip task creation, created %d tasks", tasks.created)
	}
	if store.upserts != 0 {
		t.Fatalf("expected existing dedupe to skip run upsert, got %d upserts", store.upserts)
	}
}

func TestSimpleRuntimeRedispatchesCompletedDedupe(t *testing.T) {
	store := &simpleRuntimeFakeStore{
		existing: types.SimpleAutomationRun{
			AutomationID: "reddit_ai_scanner_daily",
			DedupeKey:    "reddit-2026-04-27-4",
			LastStatus:   "success",
			TaskID:       "task_existing",
		},
	}
	tasks := &simpleRuntimeFakeTaskManager{}
	runtime := NewSimpleRuntime(store, tasks, SimpleRuntimeConfig{})

	payload := map[string]any{
		"status":     "needs_agent",
		"summary":    "scheduled scan UTC+8 12:00/20:00",
		"dedupe_key": "reddit-2026-04-27-4",
		"facts":      map[string]any{"hour_utc": 4},
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}

	err = runtime.HandleMatch(context.Background(), types.AutomationSpec{
		ID:            "reddit_ai_scanner_daily",
		WorkspaceRoot: "/tmp/workspace",
		Owner:         "role:reddit_ai_scanner",
		SimplePolicy: types.SimpleAutomationPolicy{
			OnSuccess: "continue",
			OnFailure: "pause",
			OnBlocked: "escalate",
		},
	}, types.TriggerEvent{
		AutomationID: "reddit_ai_scanner_daily",
		SignalKind:   "simple_watcher",
		Source:       "simple_builder:watch_script",
		Summary:      "scheduled scan UTC+8 12:00/20:00",
		DedupeKey:    "reddit-2026-04-27-4",
		Payload:      raw,
	})
	if err != nil {
		t.Fatal(err)
	}
	if tasks.created != 1 {
		t.Fatalf("created tasks = %d, want 1", tasks.created)
	}
	if store.upserts != 1 {
		t.Fatalf("run upserts = %d, want 1", store.upserts)
	}
	if store.existing.LastStatus != "running" {
		t.Fatalf("LastStatus = %q, want running", store.existing.LastStatus)
	}
	if store.existing.TaskID != "task_created" {
		t.Fatalf("TaskID = %q, want task_created", store.existing.TaskID)
	}
}

type simpleRuntimeFakeStore struct {
	existing types.SimpleAutomationRun
	upserts  int
}

func (s *simpleRuntimeFakeStore) ClaimSimpleAutomationRun(_ context.Context, run types.SimpleAutomationRun) (bool, error) {
	if s.existing.AutomationID == run.AutomationID &&
		s.existing.DedupeKey == run.DedupeKey &&
		strings.EqualFold(strings.TrimSpace(s.existing.LastStatus), "running") {
		return false, nil
	}
	s.existing = run
	return true, nil
}

func (s *simpleRuntimeFakeStore) GetSimpleAutomationRun(_ context.Context, automationID, dedupeKey string) (types.SimpleAutomationRun, bool, error) {
	if s.existing.AutomationID == automationID && s.existing.DedupeKey == dedupeKey {
		return s.existing, true, nil
	}
	return types.SimpleAutomationRun{}, false, nil
}

func (s *simpleRuntimeFakeStore) UpsertSimpleAutomationRun(_ context.Context, run types.SimpleAutomationRun) error {
	s.upserts++
	s.existing = run
	return nil
}

type simpleRuntimeFakeTaskManager struct {
	created int
}

func (m *simpleRuntimeFakeTaskManager) Create(_ context.Context, input task.CreateTaskInput) (task.Task, error) {
	m.created++
	return task.Task{
		ID:            "task_created",
		Type:          input.Type,
		Command:       input.Command,
		Description:   input.Description,
		Owner:         input.Owner,
		Kind:          input.Kind,
		TargetRole:    input.TargetRole,
		WorkspaceRoot: input.WorkspaceRoot,
	}, nil
}
