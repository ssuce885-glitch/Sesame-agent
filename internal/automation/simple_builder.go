package automation

import (
	"encoding/json"
	"fmt"
	"strings"

	"go-agent/internal/types"
)

func CompileSimpleAutomationBuilder(input types.SimpleAutomationBuilderInput, workspaceRoot string) (types.AutomationSpec, error) {
	input.AutomationID = strings.TrimSpace(input.AutomationID)
	if input.AutomationID == "" {
		return types.AutomationSpec{}, nativeBuilderValidationError("automation_id is required")
	}
	if types.NormalizeAutomationID(input.AutomationID) == "" {
		return types.AutomationSpec{}, nativeBuilderValidationError("automation_id must match ^[a-z][a-z0-9_-]{0,127}$")
	}

	owner := types.NormalizeAutomationOwner(input.Owner)
	if owner == "" {
		return types.AutomationSpec{}, nativeBuilderValidationError("owner must be main_agent or role:<role_id>")
	}

	watchScript := strings.TrimSpace(input.WatchScript)
	if watchScript == "" {
		return types.AutomationSpec{}, nativeBuilderValidationError("watch_script is required")
	}
	if input.IntervalSeconds <= 0 {
		return types.AutomationSpec{}, nativeBuilderValidationError("interval_seconds must be greater than zero")
	}

	timeoutSeconds := input.TimeoutSeconds
	if timeoutSeconds <= 0 {
		timeoutSeconds = 30
	}

	title := strings.TrimSpace(input.Title)
	if title == "" {
		title = fmt.Sprintf("%s simple automation", input.AutomationID)
	}
	goal := strings.TrimSpace(input.Goal)
	if goal == "" {
		goal = "Run watcher script and dispatch deterministic owner task on match."
	}

	reportTarget := ""
	if strings.TrimSpace(input.ReportTarget) == "" {
		reportTarget = owner
	} else {
		reportTarget = types.NormalizeAutomationOwner(input.ReportTarget)
		if reportTarget == "" {
			return types.AutomationSpec{}, nativeBuilderValidationError("report_target must be main_agent or role:<role_id>")
		}
	}
	escalationTarget := ""
	if strings.TrimSpace(input.EscalationTarget) == "" {
		escalationTarget = "main_agent"
	} else {
		escalationTarget = types.NormalizeAutomationOwner(input.EscalationTarget)
		if escalationTarget == "" {
			return types.AutomationSpec{}, nativeBuilderValidationError("escalation_target must be main_agent or role:<role_id>")
		}
	}

	onSuccess, err := normalizeSimpleAutomationPolicyAction("on_success", input.SimplePolicy.OnSuccess, "continue")
	if err != nil {
		return types.AutomationSpec{}, err
	}
	onFailure, err := normalizeSimpleAutomationPolicyAction("on_failure", input.SimplePolicy.OnFailure, "pause")
	if err != nil {
		return types.AutomationSpec{}, err
	}
	onBlocked, err := normalizeSimpleAutomationPolicyAction("on_blocked", input.SimplePolicy.OnBlocked, "escalate")
	if err != nil {
		return types.AutomationSpec{}, err
	}
	policy := types.SimpleAutomationPolicy{
		OnSuccess: onSuccess,
		OnFailure: onFailure,
		OnBlocked: onBlocked,
	}

	payload := watcherPollSignalPayload{
		IntervalSeconds: input.IntervalSeconds,
		TimeoutSeconds:  timeoutSeconds,
		TriggerOn:       "script_status",
		SignalKind:      "simple_watcher",
		Summary:         "simple automation watcher match",
	}
	encodedPayload, err := json.Marshal(payload)
	if err != nil {
		encodedPayload = []byte(`{"interval_seconds":60,"timeout_seconds":30,"trigger_on":"script_status","signal_kind":"simple_watcher"}`)
	}

	spec := types.AutomationSpec{
		ID:               input.AutomationID,
		Title:            title,
		WorkspaceRoot:    strings.TrimSpace(workspaceRoot),
		Goal:             goal,
		State:            types.AutomationStateActive,
		Mode:             types.AutomationModeSimple,
		Owner:            owner,
		ReportTarget:     reportTarget,
		EscalationTarget: escalationTarget,
		SimplePolicy:     policy,
		Signals: []types.AutomationSignal{{
			Kind:     "poll",
			Source:   "simple_builder:watch_script",
			Selector: watchScript,
			Payload:  json.RawMessage(encodedPayload),
		}},
		WatcherLifecycle: marshalBuilderObject(map[string]any{
			"mode":           "continuous",
			"after_dispatch": "pause",
		}),
		RetriggerPolicy: marshalBuilderObject(map[string]any{"cooldown_seconds": 0}),
	}
	if err := ValidateWatcherCompilation(spec); err != nil {
		return types.AutomationSpec{}, err
	}
	return spec, nil
}

func normalizeSimpleAutomationPolicyAction(field, raw, fallback string) (string, error) {
	value := strings.ToLower(strings.TrimSpace(raw))
	switch value {
	case "continue", "pause", "escalate":
		return value, nil
	case "":
		return strings.ToLower(strings.TrimSpace(fallback)), nil
	default:
		return "", nativeBuilderValidationError(fmt.Sprintf("simple_policy.%s must be one of continue, pause, escalate", strings.TrimSpace(field)))
	}
}
