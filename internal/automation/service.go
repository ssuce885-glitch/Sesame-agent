package automation

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"go-agent/internal/types"
)

var (
	errServiceNotConfigured = errors.New("automation service is not configured")
	errMissingDispatchPlan  = &types.AutomationValidationError{Code: "missing_dispatch_plan", Message: "response_plan and delivery_policy are required"}
	errMissingRuntimePolicy = &types.AutomationValidationError{Code: "missing_runtime_policy", Message: "runtime_policy is required"}
	errMissingAutomationID  = &types.AutomationValidationError{Code: "invalid_automation_spec", Message: "automation_id is required"}
	errMissingConfirmation  = &types.AutomationValidationError{Code: "missing_confirmation", Message: "automation_apply requires confirmed=true after user review"}
	errAutomationNotFound   = &types.AutomationValidationError{Code: "invalid_automation_spec", Message: "automation not found"}
	errInvalidControlAction = &types.AutomationValidationError{Code: "invalid_automation_spec", Message: "action must be pause or resume"}
)

type Store interface {
	UpsertAutomation(context.Context, types.AutomationSpec) error
	GetAutomation(context.Context, string) (types.AutomationSpec, bool, error)
	ListAutomations(context.Context, types.AutomationListFilter) ([]types.AutomationSpec, error)
	DeleteAutomation(context.Context, string) (bool, error)
	UpsertAutomationIncident(context.Context, types.AutomationIncident) error
	GetAutomationIncident(context.Context, string) (types.AutomationIncident, bool, error)
	ListAutomationIncidents(context.Context, types.AutomationIncidentFilter) ([]types.AutomationIncident, error)
	UpsertAutomationHeartbeat(context.Context, types.AutomationHeartbeat) error
}

type Service struct {
	store   Store
	watcher watcherRuntimeManager
	now     func() time.Time
}

type watcherRuntimeManager interface {
	Install(context.Context, types.AutomationSpec) (types.AutomationWatcherRuntime, error)
	Reinstall(context.Context, types.AutomationSpec) (types.AutomationWatcherRuntime, error)
	Get(context.Context, string) (types.AutomationWatcherRuntime, bool, error)
	Pause(context.Context, string) (types.AutomationWatcherRuntime, bool, error)
	Delete(context.Context, string) error
	Reconcile(context.Context) error
}

func NewService(store Store) *Service {
	return &Service{
		store: store,
		now:   func() time.Time { return time.Now().UTC() },
	}
}

func (s *Service) SetClock(now func() time.Time) {
	if s != nil && now != nil {
		s.now = now
	}
}

func (s *Service) SetWatcherService(watcher watcherRuntimeManager) {
	if s != nil {
		s.watcher = watcher
	}
}

func (s *Service) Apply(ctx context.Context, spec types.AutomationSpec) (types.AutomationSpec, error) {
	if s == nil || s.store == nil {
		return types.AutomationSpec{}, errServiceNotConfigured
	}
	spec = normalizeAutomationSpec(spec, s.currentTime())
	if err := validateAutomationSpec(spec); err != nil {
		return types.AutomationSpec{}, err
	}
	if err := s.store.UpsertAutomation(ctx, spec); err != nil {
		return types.AutomationSpec{}, err
	}
	if s.watcher != nil {
		switch spec.State {
		case types.AutomationStateActive:
			if _, err := s.watcher.Install(ctx, spec); err != nil {
				return types.AutomationSpec{}, err
			}
		case types.AutomationStatePaused:
			if _, ok, err := s.watcher.Pause(ctx, spec.ID); err != nil {
				return types.AutomationSpec{}, err
			} else if !ok {
				// No runtime exists yet; paused automation remains persisted without an active watcher.
			}
		}
	}
	return spec, nil
}

func (s *Service) ApplyRequest(ctx context.Context, req types.ApplyAutomationRequest) (types.AutomationSpec, error) {
	if !req.Confirmed {
		return types.AutomationSpec{}, errMissingConfirmation
	}
	if s == nil || s.store == nil {
		return types.AutomationSpec{}, errServiceNotConfigured
	}
	spec := normalizeAutomationSpec(req.Spec, s.currentTime())
	if err := validateAutomationSpec(spec); err != nil {
		return types.AutomationSpec{}, err
	}
	if err := PersistAutomationAssets(spec.WorkspaceRoot, spec.ID, req.Assets); err != nil {
		return types.AutomationSpec{}, err
	}
	if err := ValidateAutomationScriptAssets(spec); err != nil {
		return types.AutomationSpec{}, err
	}
	return s.Apply(ctx, spec)
}

func (s *Service) Get(ctx context.Context, id string) (types.AutomationSpec, bool, error) {
	if s == nil || s.store == nil {
		return types.AutomationSpec{}, false, errServiceNotConfigured
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return types.AutomationSpec{}, false, nil
	}
	return s.store.GetAutomation(ctx, id)
}

func (s *Service) List(ctx context.Context, filter types.AutomationListFilter) ([]types.AutomationSpec, error) {
	if s == nil || s.store == nil {
		return nil, errServiceNotConfigured
	}
	return s.store.ListAutomations(ctx, normalizeAutomationListFilter(filter))
}

func (s *Service) Control(ctx context.Context, id string, action types.AutomationControlAction) (types.AutomationSpec, bool, error) {
	if s == nil || s.store == nil {
		return types.AutomationSpec{}, false, errServiceNotConfigured
	}

	action = normalizeControlAction(action)
	switch action {
	case types.AutomationControlActionPause, types.AutomationControlActionResume:
	default:
		return types.AutomationSpec{}, false, errInvalidControlAction
	}

	spec, ok, err := s.Get(ctx, id)
	if err != nil || !ok {
		return types.AutomationSpec{}, ok, err
	}
	switch action {
	case types.AutomationControlActionPause:
		spec.State = types.AutomationStatePaused
	case types.AutomationControlActionResume:
		spec.State = types.AutomationStateActive
	}
	spec.UpdatedAt = s.currentTime()
	if err := s.store.UpsertAutomation(ctx, spec); err != nil {
		return types.AutomationSpec{}, false, err
	}
	if s.watcher != nil {
		switch action {
		case types.AutomationControlActionPause:
			if _, _, err := s.watcher.Pause(ctx, spec.ID); err != nil {
				return types.AutomationSpec{}, false, err
			}
		case types.AutomationControlActionResume:
			if _, err := s.watcher.Install(ctx, spec); err != nil {
				return types.AutomationSpec{}, false, err
			}
		}
	}
	return spec, true, nil
}

func (s *Service) Delete(ctx context.Context, id string) (bool, error) {
	if s == nil || s.store == nil {
		return false, errServiceNotConfigured
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return false, nil
	}
	if s.watcher != nil {
		if err := s.watcher.Delete(ctx, id); err != nil {
			return false, err
		}
	}
	return s.store.DeleteAutomation(ctx, id)
}

func (s *Service) EmitTrigger(ctx context.Context, req types.AutomationTriggerRequest) (types.AutomationIncident, error) {
	if s == nil || s.store == nil {
		return types.AutomationIncident{}, errServiceNotConfigured
	}
	req = normalizeTriggerRequest(req)
	if req.AutomationID == "" {
		return types.AutomationIncident{}, errMissingAutomationID
	}
	spec, ok, err := s.store.GetAutomation(ctx, req.AutomationID)
	if err != nil {
		return types.AutomationIncident{}, err
	}
	if !ok {
		return types.AutomationIncident{}, errAutomationNotFound
	}

	now := s.currentTime()
	incident := types.AutomationIncident{
		ID:            types.NewID("incident"),
		AutomationID:  spec.ID,
		WorkspaceRoot: spec.WorkspaceRoot,
		Status:        types.AutomationIncidentStatusOpen,
		SignalKind:    req.SignalKind,
		Source:        req.Source,
		Summary:       req.Summary,
		Payload:       normalizeRawJSON(req.Payload),
		ObservedAt:    normalizeOptionalTime(req.ObservedAt, now),
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if incident.SignalKind == "" {
		return types.AutomationIncident{}, &types.AutomationValidationError{
			Code:    "invalid_automation_spec",
			Message: "signal_kind is required",
		}
	}
	if len(incident.Payload) > 0 && !isValidJSONValue(incident.Payload) {
		return types.AutomationIncident{}, &types.AutomationValidationError{
			Code:    "invalid_automation_spec",
			Message: "payload must be valid JSON",
		}
	}
	if err := s.store.UpsertAutomationIncident(ctx, incident); err != nil {
		return types.AutomationIncident{}, err
	}
	return incident, nil
}

func (s *Service) RecordHeartbeat(ctx context.Context, req types.AutomationHeartbeatRequest) (types.AutomationHeartbeat, error) {
	if s == nil || s.store == nil {
		return types.AutomationHeartbeat{}, errServiceNotConfigured
	}
	req = normalizeHeartbeatRequest(req)
	if req.AutomationID == "" {
		return types.AutomationHeartbeat{}, errMissingAutomationID
	}
	spec, ok, err := s.store.GetAutomation(ctx, req.AutomationID)
	if err != nil {
		return types.AutomationHeartbeat{}, err
	}
	if !ok {
		return types.AutomationHeartbeat{}, errAutomationNotFound
	}

	now := s.currentTime()
	heartbeat := types.AutomationHeartbeat{
		AutomationID:  spec.ID,
		WatcherID:     req.WatcherID,
		WorkspaceRoot: spec.WorkspaceRoot,
		Status:        req.Status,
		Payload:       normalizeRawJSON(req.Payload),
		ObservedAt:    normalizeOptionalTime(req.ObservedAt, now),
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if heartbeat.WatcherID == "" {
		return types.AutomationHeartbeat{}, &types.AutomationValidationError{
			Code:    "invalid_automation_spec",
			Message: "watcher_id is required",
		}
	}
	if heartbeat.Status == "" {
		return types.AutomationHeartbeat{}, &types.AutomationValidationError{
			Code:    "invalid_automation_spec",
			Message: "status is required",
		}
	}
	if len(heartbeat.Payload) > 0 && !isValidJSONValue(heartbeat.Payload) {
		return types.AutomationHeartbeat{}, &types.AutomationValidationError{
			Code:    "invalid_automation_spec",
			Message: "payload must be valid JSON",
		}
	}
	if err := s.store.UpsertAutomationHeartbeat(ctx, heartbeat); err != nil {
		return types.AutomationHeartbeat{}, err
	}
	return heartbeat, nil
}

func (s *Service) GetIncident(ctx context.Context, id string) (types.AutomationIncident, bool, error) {
	if s == nil || s.store == nil {
		return types.AutomationIncident{}, false, errServiceNotConfigured
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return types.AutomationIncident{}, false, nil
	}
	return s.store.GetAutomationIncident(ctx, id)
}

func (s *Service) ListIncidents(ctx context.Context, filter types.AutomationIncidentFilter) ([]types.AutomationIncident, error) {
	if s == nil || s.store == nil {
		return nil, errServiceNotConfigured
	}
	return s.store.ListAutomationIncidents(ctx, normalizeIncidentFilter(filter))
}

func (s *Service) GetWatcher(ctx context.Context, id string) (types.AutomationWatcherRuntime, bool, error) {
	if s == nil || s.watcher == nil {
		return types.AutomationWatcherRuntime{}, false, nil
	}
	return s.watcher.Get(ctx, strings.TrimSpace(id))
}

func (s *Service) ReinstallWatcher(ctx context.Context, id string) (types.AutomationWatcherRuntime, bool, error) {
	if s == nil || s.store == nil || s.watcher == nil {
		return types.AutomationWatcherRuntime{}, false, errServiceNotConfigured
	}
	spec, ok, err := s.Get(ctx, id)
	if err != nil || !ok {
		return types.AutomationWatcherRuntime{}, ok, err
	}
	watcher, err := s.watcher.Reinstall(ctx, spec)
	if err != nil {
		return types.AutomationWatcherRuntime{}, false, err
	}
	return watcher, true, nil
}

func (s *Service) InstallWatcher(ctx context.Context, id string) (types.AutomationWatcherRuntime, bool, error) {
	return s.ReinstallWatcher(ctx, id)
}

func (s *Service) currentTime() time.Time {
	if s == nil || s.now == nil {
		return time.Now().UTC()
	}
	return s.now().UTC()
}

func validateAutomationSpec(spec types.AutomationSpec) error {
	if strings.TrimSpace(spec.Title) == "" {
		return &types.AutomationValidationError{
			Code:    "invalid_automation_spec",
			Message: "title is required",
		}
	}
	if strings.TrimSpace(spec.WorkspaceRoot) == "" {
		return &types.AutomationValidationError{
			Code:    "invalid_automation_spec",
			Message: "workspace_root is required",
		}
	}
	if strings.TrimSpace(spec.Goal) == "" {
		return &types.AutomationValidationError{
			Code:    "invalid_automation_spec",
			Message: "goal is required",
		}
	}
	if !isValidAutomationState(spec.State) {
		return &types.AutomationValidationError{
			Code:    "invalid_automation_spec",
			Message: "state must be active or paused",
		}
	}
	if !isPresentJSON(spec.ResponsePlan) || !isPresentJSON(spec.DeliveryPolicy) {
		return errMissingDispatchPlan
	}
	if !isPresentJSON(spec.RuntimePolicy) {
		return errMissingRuntimePolicy
	}
	if !isJSONObject(spec.ResponsePlan) || !isJSONObject(spec.DeliveryPolicy) || !isJSONObject(spec.RuntimePolicy) {
		return &types.AutomationValidationError{
			Code:    "invalid_automation_spec",
			Message: "response_plan, delivery_policy, and runtime_policy must be JSON objects",
		}
	}
	if !isJSONObject(spec.IncidentPolicy) || !isJSONObject(spec.VerificationPlan) || !isJSONObject(spec.EscalationPolicy) || !isJSONObject(spec.WatcherLifecycle) || !isJSONObject(spec.RetriggerPolicy) || !isJSONObject(spec.RunPolicy) {
		return &types.AutomationValidationError{
			Code:    "invalid_automation_spec",
			Message: "incident_policy, verification_plan, escalation_policy, watcher_lifecycle, retrigger_policy, and run_policy must be JSON objects",
		}
	}
	for _, signal := range spec.Signals {
		if len(signal.Payload) > 0 && !isValidJSONValue(signal.Payload) {
			return &types.AutomationValidationError{
				Code:    "invalid_automation_spec",
				Message: "signals payloads must be valid JSON",
			}
		}
	}
	return nil
}

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
	spec.ResponsePlan = normalizeResponsePlanJSON(spec.ResponsePlan)
	spec.VerificationPlan = normalizeObjectJSON(spec.VerificationPlan)
	spec.EscalationPolicy = normalizeObjectJSON(spec.EscalationPolicy)
	spec.DeliveryPolicy = normalizeRawJSON(spec.DeliveryPolicy)
	spec.RuntimePolicy = normalizeRawJSON(spec.RuntimePolicy)
	spec.WatcherLifecycle = normalizeObjectJSON(spec.WatcherLifecycle)
	spec.RetriggerPolicy = normalizeObjectJSON(spec.RetriggerPolicy)
	spec.RunPolicy = normalizeObjectJSON(spec.RunPolicy)
	spec.Assumptions = normalizeAssumptions(spec.Assumptions)

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
	if !req.ObservedAt.IsZero() {
		req.ObservedAt = req.ObservedAt.UTC()
	}
	return req
}

func normalizeHeartbeatRequest(req types.AutomationHeartbeatRequest) types.AutomationHeartbeatRequest {
	req.AutomationID = strings.TrimSpace(req.AutomationID)
	req.WatcherID = strings.TrimSpace(req.WatcherID)
	req.Status = strings.TrimSpace(req.Status)
	req.Payload = normalizeRawJSON(req.Payload)
	if !req.ObservedAt.IsZero() {
		req.ObservedAt = req.ObservedAt.UTC()
	}
	return req
}

func normalizeAutomationState(state types.AutomationState) types.AutomationState {
	normalized := strings.ToLower(strings.TrimSpace(string(state)))
	if normalized == "" {
		return types.AutomationStateActive
	}
	return types.AutomationState(normalized)
}

func isValidAutomationState(state types.AutomationState) bool {
	switch state {
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
		value = strings.TrimSpace(value)
		if key == "" {
			continue
		}
		out[key] = value
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

func normalizeAssumptions(values []types.AutomationAssumption) []types.AutomationAssumption {
	if len(values) == 0 {
		return []types.AutomationAssumption{}
	}
	out := make([]types.AutomationAssumption, 0, len(values))
	for _, value := range values {
		value.Field = strings.TrimSpace(value.Field)
		value.Value = strings.TrimSpace(value.Value)
		value.Reason = strings.TrimSpace(value.Reason)
		if value.Field == "" && value.Value == "" && value.Reason == "" {
			continue
		}
		out = append(out, value)
	}
	if len(out) == 0 {
		return []types.AutomationAssumption{}
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

func normalizeResponsePlanJSON(raw json.RawMessage) json.RawMessage {
	raw = normalizeRawJSON(raw)
	if len(raw) == 0 || string(raw) == "null" {
		return raw
	}
	var plan types.ResponsePlanV2
	if err := json.Unmarshal(raw, &plan); err != nil {
		return raw
	}
	plan.SchemaVersion = strings.TrimSpace(plan.SchemaVersion)
	plan.Mode = strings.TrimSpace(plan.Mode)
	plan.ChildAgentTemplateRefs = normalizeStringList(plan.ChildAgentTemplateRefs)
	plan.Phases = normalizeResponsePlanPhases(plan.Phases)
	if plan.SchemaVersion == "" && (plan.Mode != "" || len(plan.Phases) > 0 || len(plan.ChildAgentTemplateRefs) > 0) {
		plan.SchemaVersion = types.ResponsePlanV2Schema
	}
	if len(plan.Phases) == 0 && plan.Mode != "" {
		plan.Phases = defaultResponsePlanPhases(plan.Mode, plan.ChildAgentTemplateRefs)
	}
	normalized, err := json.Marshal(plan)
	if err != nil {
		return raw
	}
	return normalized
}

func normalizeResponsePlanPhases(phases []types.ResponsePlanPhase) []types.ResponsePlanPhase {
	if len(phases) == 0 {
		return []types.ResponsePlanPhase{}
	}
	out := make([]types.ResponsePlanPhase, 0, len(phases))
	for _, phase := range phases {
		phase.Phase = types.AutomationPhase(strings.ToLower(strings.TrimSpace(string(phase.Phase))))
		phase.ChildAgentTemplateRefs = normalizeStringList(phase.ChildAgentTemplateRefs)
		if phase.Phase == "" && len(phase.ChildAgentTemplateRefs) == 0 {
			continue
		}
		out = append(out, phase)
	}
	if len(out) == 0 {
		return []types.ResponsePlanPhase{}
	}
	return out
}

func defaultResponsePlanPhases(mode string, templateRefs []string) []types.ResponsePlanPhase {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "investigate_then_remediate":
		diagnoseRefs := []string{}
		remediateRefs := []string{}
		if len(templateRefs) > 0 {
			diagnoseRefs = append(diagnoseRefs, templateRefs[0])
		}
		if len(templateRefs) > 1 {
			remediateRefs = append(remediateRefs, templateRefs[1:]...)
		}
		phases := []types.ResponsePlanPhase{{
			Phase:                  types.AutomationPhaseDiagnose,
			ChildAgentTemplateRefs: diagnoseRefs,
		}}
		if len(remediateRefs) > 0 {
			phases = append(phases, types.ResponsePlanPhase{
				Phase:                  types.AutomationPhaseRemediate,
				ChildAgentTemplateRefs: remediateRefs,
			})
		}
		return phases
	default:
		return []types.ResponsePlanPhase{}
	}
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
