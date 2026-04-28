package automation

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"go-agent/internal/task"
	"go-agent/internal/types"
)

type simpleRuntimeStore interface {
	ClaimSimpleAutomationRun(context.Context, types.SimpleAutomationRun) (bool, error)
	UpsertSimpleAutomationRun(context.Context, types.SimpleAutomationRun) error
}

type simpleRuntimeTaskManager interface {
	Create(context.Context, task.CreateTaskInput) (task.Task, error)
}

type SimpleRuntimeConfig struct {
	Now func() time.Time
}

type SimpleRuntime struct {
	store       simpleRuntimeStore
	taskManager simpleRuntimeTaskManager
	now         func() time.Time
}

func NewSimpleRuntime(store simpleRuntimeStore, taskManager simpleRuntimeTaskManager, cfg SimpleRuntimeConfig) *SimpleRuntime {
	return &SimpleRuntime{
		store:       store,
		taskManager: taskManager,
		now:         firstNonNilClock(cfg.Now),
	}
}

func (r *SimpleRuntime) HandleMatch(ctx context.Context, spec types.AutomationSpec, trigger types.TriggerEvent) error {
	if r == nil || r.store == nil || r.taskManager == nil {
		return errServiceNotConfigured
	}
	automationID := strings.TrimSpace(spec.ID)
	if automationID == "" {
		return errMissingAutomationID
	}

	owner := types.NormalizeRoleAutomationOwner(spec.Owner)
	if owner == "" {
		return &types.AutomationValidationError{
			Code:    "invalid_automation_spec",
			Message: "owner must be role:<role_id> for simple mode automations",
		}
	}
	targetRole, err := resolveSimpleAutomationTargetRole(owner)
	if err != nil {
		return err
	}

	detector, hasDetector := parseSimpleRuntimeDetectorSignal(trigger.Payload)
	dedupeKey := resolveSimpleRunDedupeKey(trigger, detector, hasDetector, automationID)
	summary, facts := resolveSimpleRunSignalData(trigger, detector, hasDetector)
	now := r.currentTime()
	claimed, err := r.store.ClaimSimpleAutomationRun(ctx, types.SimpleAutomationRun{
		AutomationID: automationID,
		DedupeKey:    dedupeKey,
		Owner:        owner,
		LastStatus:   "running",
		LastSummary:  summary,
		CreatedAt:    now,
		UpdatedAt:    now,
	})
	if err != nil {
		return err
	}
	if !claimed {
		return nil
	}

	taskPrompt := buildSimpleAutomationPrompt(spec, summary, dedupeKey, facts)
	createdTask, err := r.taskManager.Create(ctx, task.CreateTaskInput{
		Type:          task.TaskTypeAgent,
		Command:       taskPrompt,
		Description:   fmt.Sprintf("simple automation match: %s", automationID),
		Owner:         owner,
		Kind:          "automation_simple",
		TargetRole:    targetRole,
		WorkspaceRoot: strings.TrimSpace(spec.WorkspaceRoot),
		Start:         true,
	})
	if err != nil {
		_ = r.store.UpsertSimpleAutomationRun(ctx, types.SimpleAutomationRun{
			AutomationID: automationID,
			DedupeKey:    dedupeKey,
			Owner:        owner,
			LastStatus:   "failure",
			LastSummary:  err.Error(),
			CreatedAt:    now,
			UpdatedAt:    r.currentTime(),
		})
		return err
	}

	return r.store.UpsertSimpleAutomationRun(ctx, types.SimpleAutomationRun{
		AutomationID: automationID,
		DedupeKey:    dedupeKey,
		Owner:        owner,
		TaskID:       strings.TrimSpace(createdTask.ID),
		LastStatus:   "running",
		LastSummary:  summary,
		CreatedAt:    now,
		UpdatedAt:    now,
	})
}

func (r *SimpleRuntime) currentTime() time.Time {
	if r == nil || r.now == nil {
		return time.Now().UTC()
	}
	return r.now().UTC()
}

func parseSimpleRuntimeDetectorSignal(payload json.RawMessage) (types.AutomationDetectorSignal, bool) {
	if len(strings.TrimSpace(string(payload))) == 0 {
		return types.AutomationDetectorSignal{}, false
	}
	signal, err := ParseAutomationDetectorSignalPayload(payload)
	if err != nil {
		return types.AutomationDetectorSignal{}, false
	}
	return signal, true
}

func resolveSimpleRunDedupeKey(trigger types.TriggerEvent, detector types.AutomationDetectorSignal, hasDetector bool, automationID string) string {
	if hasDetector {
		if dedupe := strings.TrimSpace(detector.DedupeKey); dedupe != "" {
			return dedupe
		}
	}
	if dedupe := strings.TrimSpace(trigger.DedupeKey); dedupe != "" {
		return dedupe
	}
	dedupe := strings.TrimSpace(extractTriggerDedupeKey(trigger.Payload, trigger.SignalKind, trigger.Source, trigger.Summary))
	if dedupe != "" {
		return dedupe
	}
	return strings.TrimSpace(automationID) + "|simple"
}

func resolveSimpleRunSignalData(trigger types.TriggerEvent, detector types.AutomationDetectorSignal, hasDetector bool) (string, map[string]any) {
	summary := strings.TrimSpace(trigger.Summary)
	facts := map[string]any{}
	if hasDetector {
		if detectorSummary := strings.TrimSpace(detector.Summary); detectorSummary != "" {
			summary = detectorSummary
		}
		if detector.Facts != nil {
			facts = detector.Facts
		}
	}
	if summary == "" {
		summary = "automation watcher match"
	}
	return summary, facts
}

func resolveSimpleAutomationTargetRole(owner string) (string, error) {
	if strings.HasPrefix(owner, "role:") {
		roleID := strings.TrimSpace(strings.TrimPrefix(owner, "role:"))
		if roleID != "" {
			return roleID, nil
		}
	}
	return "", &types.AutomationValidationError{
		Code:    "invalid_automation_spec",
		Message: "owner must be role:<role_id> for simple mode automations",
	}
}

func buildSimpleAutomationPrompt(spec types.AutomationSpec, summary, dedupeKey string, facts map[string]any) string {
	factsJSON, err := json.Marshal(facts)
	if err != nil {
		factsJSON = []byte("{}")
	}

	return strings.Join([]string{
		"# Current Mode: Owner Task Mode",
		"You are executing a watcher-triggered owner task.",
		"Do not create, update, pause, resume, or reinstall automations.",
		"Do not call automation_create_simple or automation_control.",
		"Execute automation_goal using the detector facts.",
		"Return the result as your final assistant response; the runtime delivers that response to the main agent report stream.",
		"Keep your final response concise: summarize findings and actions taken.",
		"Do not call delegate_to_role to report the result.",
		"",
		"Simple automation task",
		"automation_title: " + strings.TrimSpace(spec.Title),
		"automation_goal: " + strings.TrimSpace(spec.Goal),
		"detector_summary: " + strings.TrimSpace(summary),
		"dedupe_key: " + strings.TrimSpace(dedupeKey),
		"facts_json: " + string(factsJSON),
	}, "\n")
}
