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
	if strings.TrimSpace(string(spec.Mode)) == "" {
		spec.Mode = types.AutomationModeSimple
	} else {
		spec.Mode = types.AutomationMode(strings.ToLower(strings.TrimSpace(string(spec.Mode))))
	}
	spec.Owner = strings.TrimSpace(spec.Owner)
	spec.ReportTarget = strings.TrimSpace(spec.ReportTarget)
	spec.EscalationTarget = strings.TrimSpace(spec.EscalationTarget)
	spec.SimplePolicy.OnSuccess = strings.ToLower(strings.TrimSpace(spec.SimplePolicy.OnSuccess))
	spec.SimplePolicy.OnFailure = strings.ToLower(strings.TrimSpace(spec.SimplePolicy.OnFailure))
	spec.SimplePolicy.OnBlocked = strings.ToLower(strings.TrimSpace(spec.SimplePolicy.OnBlocked))

	spec.Context.Owner = strings.TrimSpace(spec.Context.Owner)
	spec.Context.Environment = strings.TrimSpace(spec.Context.Environment)
	spec.Context.Targets = normalizeStringList(spec.Context.Targets)
	spec.Context.Labels = normalizeLabels(spec.Context.Labels)
	spec.Signals = normalizeSignals(spec.Signals)
	spec.WatcherLifecycle = normalizeObjectJSON(spec.WatcherLifecycle)
	spec.RetriggerPolicy = normalizeObjectJSON(spec.RetriggerPolicy)
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

func normalizeHeartbeatFilter(filter types.AutomationHeartbeatFilter) types.AutomationHeartbeatFilter {
	filter.WorkspaceRoot = strings.TrimSpace(filter.WorkspaceRoot)
	filter.AutomationID = strings.TrimSpace(filter.AutomationID)
	filter.WatcherID = strings.TrimSpace(filter.WatcherID)
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

type detectorSignalPayloadDecode struct {
	Status       *string         `json:"status"`
	Summary      *string         `json:"summary"`
	Facts        json.RawMessage `json:"facts"`
	ActionsTaken json.RawMessage `json:"actions_taken"`
	Hints        json.RawMessage `json:"hints"`
	DedupeKey    *string         `json:"dedupe_key"`
}

type detectorActionObjectDecode struct {
	Name    *string `json:"name"`
	Result  *string `json:"result"`
	Summary *string `json:"summary"`
}

func ParseAutomationDetectorSignalPayload(raw json.RawMessage) (types.AutomationDetectorSignal, error) {
	signal, _, err := parseAutomationDetectorSignalPayload(raw)
	return signal, err
}

func parseAutomationDetectorSignalPayload(raw json.RawMessage) (types.AutomationDetectorSignal, json.RawMessage, error) {
	raw = normalizeRawJSON(raw)
	if len(raw) == 0 {
		return types.AutomationDetectorSignal{}, nil, invalidSignalOutput("detector signal payload must be non-empty JSON")
	}

	var decoded detectorSignalPayloadDecode
	if err := decodeStrictJSON(raw, &decoded); err != nil {
		return types.AutomationDetectorSignal{}, nil, invalidSignalOutput("detector signal payload must be valid JSON")
	}
	if decoded.Status == nil {
		return types.AutomationDetectorSignal{}, nil, invalidSignalOutput("detector signal status is required")
	}
	normalizedStatus, ok := normalizeDetectorStatus(*decoded.Status)
	if !ok {
		return types.AutomationDetectorSignal{}, nil, invalidSignalOutput("detector signal status is unsupported")
	}
	if decoded.Summary == nil {
		return types.AutomationDetectorSignal{}, nil, invalidSignalOutput("detector signal summary is required")
	}
	summary := strings.TrimSpace(*decoded.Summary)
	if summary == "" {
		return types.AutomationDetectorSignal{}, nil, invalidSignalOutput("detector signal summary is required")
	}
	factsRaw := normalizeRawJSON(decoded.Facts)
	if len(factsRaw) == 0 || string(factsRaw) == "null" {
		factsRaw = json.RawMessage("{}")
	}
	if !isJSONObject(factsRaw) {
		return types.AutomationDetectorSignal{}, nil, invalidSignalOutput("detector signal facts must be a JSON object")
	}
	facts := map[string]any{}
	if err := json.Unmarshal(factsRaw, &facts); err != nil {
		return types.AutomationDetectorSignal{}, nil, invalidSignalOutput("detector signal facts must be a JSON object")
	}

	actionsTaken, err := normalizeDetectorActions(decoded.ActionsTaken)
	if err != nil {
		return types.AutomationDetectorSignal{}, nil, err
	}
	hints, err := normalizeDetectorStringList(decoded.Hints, "hints")
	if err != nil {
		return types.AutomationDetectorSignal{}, nil, err
	}

	signal := types.AutomationDetectorSignal{
		Status:       types.AutomationDetectorStatus(normalizedStatus),
		Summary:      summary,
		Facts:        facts,
		ActionsTaken: actionsTaken,
		Hints:        hints,
	}
	if decoded.DedupeKey != nil {
		signal.DedupeKey = strings.TrimSpace(*decoded.DedupeKey)
	}

	normalizedPayload, err := json.Marshal(signal)
	if err != nil {
		return types.AutomationDetectorSignal{}, nil, invalidSignalOutput("detector signal payload normalization failed")
	}
	return signal, normalizedPayload, nil
}

func normalizeDetectorActions(raw json.RawMessage) ([]string, error) {
	raw = normalizeRawJSON(raw)
	if len(raw) == 0 || string(raw) == "null" {
		return []string{}, nil
	}

	var actionStrings []string
	if err := json.Unmarshal(raw, &actionStrings); err == nil {
		return normalizeStringList(actionStrings), nil
	}

	var actionObjects []detectorActionObjectDecode
	if err := decodeStrictJSON(raw, &actionObjects); err != nil {
		return nil, invalidSignalOutput("detector signal actions_taken must be an array of strings or action objects")
	}
	normalized := make([]string, 0, len(actionObjects))
	for _, action := range actionObjects {
		parts := make([]string, 0, 3)
		if action.Name != nil {
			if name := strings.TrimSpace(*action.Name); name != "" {
				parts = append(parts, name)
			}
		}
		if action.Result != nil {
			if result := strings.TrimSpace(*action.Result); result != "" {
				parts = append(parts, result)
			}
		}
		if action.Summary != nil {
			if summary := strings.TrimSpace(*action.Summary); summary != "" {
				parts = append(parts, summary)
			}
		}
		if len(parts) == 0 {
			continue
		}
		normalized = append(normalized, strings.Join(parts, " | "))
	}
	return normalizeStringList(normalized), nil
}

func normalizeDetectorStringList(raw json.RawMessage, field string) ([]string, error) {
	raw = normalizeRawJSON(raw)
	if len(raw) == 0 || string(raw) == "null" {
		return []string{}, nil
	}

	var values []string
	if err := json.Unmarshal(raw, &values); err != nil {
		return nil, invalidSignalOutput("detector signal " + strings.TrimSpace(field) + " must be an array of strings")
	}
	return normalizeStringList(values), nil
}

func normalizeDetectorStatus(value string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case string(types.AutomationDetectorStatusHealthy):
		return string(types.AutomationDetectorStatusHealthy), true
	case string(types.AutomationDetectorStatusRecovered):
		return string(types.AutomationDetectorStatusRecovered), true
	case string(types.AutomationDetectorStatusNeedsAgent):
		return string(types.AutomationDetectorStatusNeedsAgent), true
	case string(types.AutomationDetectorStatusNeedsHuman):
		return string(types.AutomationDetectorStatusNeedsHuman), true
	default:
		return "", false
	}
}

func invalidSignalOutput(message string) error {
	return &types.AutomationValidationError{
		Code:    "invalid_signal_output",
		Message: strings.TrimSpace(message),
	}
}

func decodeStrictJSON(raw []byte, out any) error {
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(out); err != nil {
		return err
	}
	if decoder.More() {
		return invalidSignalOutput("detector signal payload must be a single JSON value")
	}
	return nil
}
