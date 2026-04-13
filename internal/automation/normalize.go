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

func buildTriggerEvent(spec types.AutomationSpec, req types.AutomationTriggerRequest, now time.Time) types.TriggerEvent {
	observedAt := normalizeOptionalTime(req.ObservedAt, now)
	payload := normalizeRawJSON(req.Payload)
	return types.TriggerEvent{
		EventID:       types.NewID("trigger"),
		AutomationID:  spec.ID,
		WorkspaceRoot: spec.WorkspaceRoot,
		SignalKind:    req.SignalKind,
		Source:        req.Source,
		Summary:       req.Summary,
		Payload:       payload,
		DedupeKey:     extractTriggerDedupeKey(payload, req.SignalKind, req.Source, req.Summary),
		ObservedAt:    observedAt,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
}

func loadResponsePlanV2(raw json.RawMessage) types.ResponsePlanV2 {
	raw = normalizeRawJSON(raw)
	if len(raw) == 0 {
		return types.ResponsePlanV2{}
	}
	var plan types.ResponsePlanV2
	if err := json.Unmarshal(raw, &plan); err != nil {
		return types.ResponsePlanV2{}
	}
	return plan
}

func buildIncidentPhaseStates(spec types.AutomationSpec, incident types.AutomationIncident, now time.Time) []types.IncidentPhaseState {
	plan := loadResponsePlanV2(spec.ResponsePlan)
	if len(plan.Phases) == 0 {
		return []types.IncidentPhaseState{}
	}
	states := make([]types.IncidentPhaseState, 0, len(plan.Phases))
	for _, phase := range plan.Phases {
		if phase.Phase == "" {
			continue
		}
		states = append(states, types.IncidentPhaseState{
			IncidentID:             incident.ID,
			AutomationID:           incident.AutomationID,
			WorkspaceRoot:          incident.WorkspaceRoot,
			Phase:                  phase.Phase,
			Reduction:              types.IncidentPhaseReductionAllMustSucceed,
			Status:                 types.IncidentPhaseStatusPending,
			DispatchIDs:            []string{},
			ActiveDispatchCount:    0,
			CompletedDispatchCount: 0,
			FailedDispatchCount:    0,
			CreatedAt:              now,
			UpdatedAt:              now,
		})
	}
	return states
}

func dedupeWindowForSpec(spec types.AutomationSpec) time.Duration {
	cfg := struct {
		DedupeWindowSeconds int `json:"dedupe_window_seconds"`
	}{}
	_ = json.Unmarshal(spec.RetriggerPolicy, &cfg)
	if cfg.DedupeWindowSeconds <= 0 {
		cfg.DedupeWindowSeconds = 600
	}
	return time.Duration(cfg.DedupeWindowSeconds) * time.Second
}

func incidentMatchesTriggerDedupe(incident types.AutomationIncident, dedupeKey string, observedAt time.Time, window time.Duration) bool {
	if strings.TrimSpace(dedupeKey) == "" {
		return false
	}
	if strings.TrimSpace(incidentDedupeKey(incident)) != strings.TrimSpace(dedupeKey) {
		return false
	}
	if observedAt.IsZero() || incident.ObservedAt.IsZero() || window <= 0 {
		return true
	}
	delta := observedAt.Sub(incident.ObservedAt)
	if delta < 0 {
		delta = -delta
	}
	return delta <= window
}

func incidentDedupeKey(incident types.AutomationIncident) string {
	if dedupeKey := incidentPayloadDedupeKey(incident.Payload); dedupeKey != "" {
		return dedupeKey
	}
	return extractTriggerDedupeKey(nil, incident.SignalKind, incident.Source, incident.Summary)
}

func incidentPayloadDedupeKey(raw json.RawMessage) string {
	raw = normalizeRawJSON(raw)
	if len(raw) == 0 || !json.Valid(raw) {
		return ""
	}
	var object map[string]any
	if err := json.Unmarshal(raw, &object); err != nil {
		return ""
	}
	text, _ := object["dedupe_key"].(string)
	return strings.TrimSpace(text)
}

func extractTriggerDedupeKey(raw json.RawMessage, signalKind string, source string, summary string) string {
	if dedupeKey := incidentPayloadDedupeKey(raw); dedupeKey != "" {
		return dedupeKey
	}
	parts := []string{
		strings.TrimSpace(signalKind),
		strings.TrimSpace(source),
		strings.TrimSpace(summary),
	}
	nonEmpty := make([]string, 0, len(parts))
	for _, part := range parts {
		if part != "" {
			nonEmpty = append(nonEmpty, part)
		}
	}
	return strings.Join(nonEmpty, "|")
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
