package automation

import (
	"encoding/json"
	"strings"
	"time"

	"go-agent/internal/types"
)

func normalizeAutomationSpec(spec types.AutomationSpec, now time.Time) types.AutomationSpec {
	spec.ID = strings.TrimSpace(spec.ID)
	if spec.ID == "" {
		spec.ID = types.NewID("automation")
	}
	spec.Title = strings.TrimSpace(spec.Title)
	spec.WorkspaceRoot = strings.TrimSpace(spec.WorkspaceRoot)
	spec.Goal = strings.TrimSpace(spec.Goal)
	spec.State = normalizeAutomationState(spec.State)

	spec.Context.Owner = strings.TrimSpace(spec.Context.Owner)
	spec.Context.Environment = strings.TrimSpace(spec.Context.Environment)
	spec.Context.Targets = normalizeStringList(spec.Context.Targets)
	spec.Context.Labels = normalizeLabels(spec.Context.Labels)
	spec.Signals = normalizeSignals(spec.Signals)
	spec.IncidentPolicy = normalizeObjectJSON(spec.IncidentPolicy)
	spec.ResponsePlan = normalizeRawJSON(spec.ResponsePlan)
	spec.VerificationPlan = normalizeObjectJSON(spec.VerificationPlan)
	spec.EscalationPolicy = normalizeObjectJSON(spec.EscalationPolicy)
	spec.DeliveryPolicy = normalizeRawJSON(spec.DeliveryPolicy)
	spec.RuntimePolicy = normalizeRawJSON(spec.RuntimePolicy)
	spec.WatcherLifecycle = normalizeObjectJSON(spec.WatcherLifecycle)
	spec.RetriggerPolicy = normalizeObjectJSON(spec.RetriggerPolicy)
	spec.RunPolicy = normalizeObjectJSON(spec.RunPolicy)
	spec.ResponsePlan = types.NormalizeAutomationResponsePlanJSON(spec.ResponsePlan)
	spec.Assumptions = types.NormalizeAutomationAssumptions(spec.Assumptions)

	if spec.CreatedAt.IsZero() {
		spec.CreatedAt = now
	} else {
		spec.CreatedAt = spec.CreatedAt.UTC()
	}
	if spec.UpdatedAt.IsZero() {
		spec.UpdatedAt = spec.CreatedAt
	} else {
		spec.UpdatedAt = spec.UpdatedAt.UTC()
	}
	if spec.UpdatedAt.Before(spec.CreatedAt) {
		spec.UpdatedAt = spec.CreatedAt
	}
	return spec
}

func normalizeAutomationListFilter(filter types.AutomationListFilter) types.AutomationListFilter {
	filter.WorkspaceRoot = strings.TrimSpace(filter.WorkspaceRoot)
	filter.State = types.AutomationState(strings.ToLower(strings.TrimSpace(string(filter.State))))
	if filter.Limit < 0 {
		filter.Limit = 0
	}
	return filter
}

func normalizeIncidentFilter(filter types.AutomationIncidentFilter) types.AutomationIncidentFilter {
	filter.WorkspaceRoot = strings.TrimSpace(filter.WorkspaceRoot)
	filter.AutomationID = strings.TrimSpace(filter.AutomationID)
	filter.Status = types.AutomationIncidentStatus(strings.ToLower(strings.TrimSpace(string(filter.Status))))
	if filter.Limit < 0 {
		filter.Limit = 0
	}
	return filter
}

func normalizeTriggerRequest(req types.AutomationTriggerRequest) types.AutomationTriggerRequest {
	req.AutomationID = strings.TrimSpace(req.AutomationID)
	req.SignalKind = strings.TrimSpace(req.SignalKind)
	req.Source = strings.TrimSpace(req.Source)
	req.Summary = strings.TrimSpace(req.Summary)
	req.Payload = normalizeRawJSON(req.Payload)
	return req
}

func normalizeHeartbeatRequest(req types.AutomationHeartbeatRequest) types.AutomationHeartbeatRequest {
	req.AutomationID = strings.TrimSpace(req.AutomationID)
	req.WatcherID = strings.TrimSpace(req.WatcherID)
	req.Status = strings.TrimSpace(req.Status)
	req.Payload = normalizeRawJSON(req.Payload)
	return req
}

func normalizeAutomationState(state types.AutomationState) types.AutomationState {
	state = types.AutomationState(strings.ToLower(strings.TrimSpace(string(state))))
	if state == "" {
		return types.AutomationStateActive
	}
	return state
}

func isValidAutomationState(state types.AutomationState) bool {
	switch normalizeAutomationState(state) {
	case types.AutomationStateActive, types.AutomationStatePaused:
		return true
	default:
		return false
	}
}

func normalizeControlAction(action types.AutomationControlAction) types.AutomationControlAction {
	return types.AutomationControlAction(strings.ToLower(strings.TrimSpace(string(action))))
}

func normalizeSignals(signals []types.AutomationSignal) []types.AutomationSignal {
	if len(signals) == 0 {
		return []types.AutomationSignal{}
	}
	out := make([]types.AutomationSignal, 0, len(signals))
	for _, signal := range signals {
		signal.Kind = strings.TrimSpace(signal.Kind)
		signal.Source = strings.TrimSpace(signal.Source)
		signal.Selector = strings.TrimSpace(signal.Selector)
		signal.Payload = normalizeRawJSON(signal.Payload)
		if signal.Kind == "" && signal.Source == "" && signal.Selector == "" && len(signal.Payload) == 0 {
			continue
		}
		out = append(out, signal)
	}
	if len(out) == 0 {
		return []types.AutomationSignal{}
	}
	return out
}

func normalizeLabels(labels map[string]string) map[string]string {
	if len(labels) == 0 {
		return map[string]string{}
	}
	out := make(map[string]string, len(labels))
	for key, value := range labels {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		out[key] = strings.TrimSpace(value)
	}
	if len(out) == 0 {
		return map[string]string{}
	}
	return out
}

func normalizeStringList(values []string) []string {
	if len(values) == 0 {
		return []string{}
	}
	out := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
	if len(out) == 0 {
		return []string{}
	}
	return out
}

func normalizeRawJSON(raw json.RawMessage) json.RawMessage {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" {
		return nil
	}
	return json.RawMessage(trimmed)
}

func isPresentJSON(raw json.RawMessage) bool {
	raw = normalizeRawJSON(raw)
	if len(raw) == 0 {
		return false
	}
	return string(raw) != "null"
}

func normalizeObjectJSON(raw json.RawMessage) json.RawMessage {
	raw = normalizeRawJSON(raw)
	if len(raw) == 0 || string(raw) == "null" {
		return json.RawMessage("{}")
	}
	return raw
}

func isJSONObject(raw json.RawMessage) bool {
	raw = normalizeRawJSON(raw)
	if len(raw) == 0 || !json.Valid(raw) {
		return false
	}
	var decoded any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return false
	}
	_, ok := decoded.(map[string]any)
	return ok
}

func isValidJSONValue(raw json.RawMessage) bool {
	raw = normalizeRawJSON(raw)
	if len(raw) == 0 {
		return true
	}
	return json.Valid(raw)
}

func normalizeOptionalTime(value, fallback time.Time) time.Time {
	if value.IsZero() {
		return fallback.UTC()
	}
	return value.UTC()
}

func validateResponsePlanDraftMode(raw json.RawMessage) error {
	raw = normalizeRawJSON(raw)
	if len(raw) == 0 || !json.Valid(raw) {
		return nil
	}

	var object map[string]any
	if err := json.Unmarshal(raw, &object); err != nil {
		return nil
	}
	if strings.EqualFold(strings.TrimSpace(asStringForDraftValidation(object["schema_version"])), types.ResponsePlanSchemaVersionV2) {
		return nil
	}

	mode := strings.TrimSpace(asStringForDraftValidation(object["mode"]))
	switch mode {
	case "", "notify", "investigate", "investigate_then_act", "act_only":
		return nil
	default:
		return &types.AutomationValidationError{
			Code:    "invalid_automation_spec",
			Message: "response_plan.mode must be one of notify, investigate, investigate_then_act, act_only",
		}
	}
}

func asStringForDraftValidation(value any) string {
	text, _ := value.(string)
	return text
}
