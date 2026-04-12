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
	errMissingTitle         = errors.New("missing_title")
	errMissingWorkspaceRoot = errors.New("missing_workspace_root")
	errMissingGoal          = errors.New("missing_goal")
	errMissingDispatchPlan  = errors.New("missing_dispatch_plan")
	errMissingRuntimePolicy = errors.New("missing_runtime_policy")
	errInvalidSpec          = errors.New("invalid_automation_spec")
	errMissingAutomationID  = errors.New("missing_automation_id")
	errAutomationNotFound   = errors.New("automation_not_found")
	errInvalidControlAction = errors.New("invalid_control_action")
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
	store Store
	now   func() time.Time
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
	return spec, nil
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
		TriggerKind:   req.TriggerKind,
		Source:        req.Source,
		Title:         req.Title,
		Summary:       req.Summary,
		Payload:       normalizeRawJSON(req.Payload),
		ObservedAt:    normalizeOptionalTime(req.ObservedAt, now),
		CreatedAt:     now,
		UpdatedAt:     now,
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
		ID:            types.NewID("heartbeat"),
		AutomationID:  spec.ID,
		WorkspaceRoot: spec.WorkspaceRoot,
		Status:        req.Status,
		Message:       req.Message,
		Payload:       normalizeRawJSON(req.Payload),
		ObservedAt:    normalizeOptionalTime(req.ObservedAt, now),
		CreatedAt:     now,
		UpdatedAt:     now,
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

func (s *Service) currentTime() time.Time {
	if s == nil || s.now == nil {
		return time.Now().UTC()
	}
	return s.now().UTC()
}

func validateAutomationSpec(spec types.AutomationSpec) error {
	if strings.TrimSpace(spec.Title) == "" {
		return errMissingTitle
	}
	if strings.TrimSpace(spec.WorkspaceRoot) == "" {
		return errMissingWorkspaceRoot
	}
	if strings.TrimSpace(spec.Goal) == "" {
		return errMissingGoal
	}
	if !isPresentJSON(spec.ResponsePlan) || !isPresentJSON(spec.DeliveryPolicy) {
		return errMissingDispatchPlan
	}
	if !isPresentJSON(spec.RuntimePolicy) {
		return errMissingRuntimePolicy
	}
	if !isValidOptionalJSON(spec.ResponsePlan) || !isValidOptionalJSON(spec.DeliveryPolicy) || !isValidOptionalJSON(spec.RuntimePolicy) {
		return errInvalidSpec
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
	if len(spec.Context.Labels) == 0 {
		spec.Context.Labels = nil
	}

	spec.Signals = normalizeSignals(spec.Signals)
	spec.IncidentPolicy = normalizeRawJSON(spec.IncidentPolicy)
	spec.ResponsePlan = normalizeRawJSON(spec.ResponsePlan)
	spec.VerificationPlan = normalizeRawJSON(spec.VerificationPlan)
	spec.EscalationPolicy = normalizeRawJSON(spec.EscalationPolicy)
	spec.DeliveryPolicy = normalizeRawJSON(spec.DeliveryPolicy)
	spec.RuntimePolicy = normalizeRawJSON(spec.RuntimePolicy)
	spec.WatcherLifecycle = normalizeRawJSON(spec.WatcherLifecycle)
	spec.RetriggerPolicy = normalizeRawJSON(spec.RetriggerPolicy)
	spec.RunPolicy = normalizeRawJSON(spec.RunPolicy)
	spec.Assumptions = normalizeStringList(spec.Assumptions)

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
	req.TriggerKind = strings.TrimSpace(req.TriggerKind)
	req.Source = strings.TrimSpace(req.Source)
	req.Title = strings.TrimSpace(req.Title)
	req.Summary = strings.TrimSpace(req.Summary)
	req.Payload = normalizeRawJSON(req.Payload)
	if !req.ObservedAt.IsZero() {
		req.ObservedAt = req.ObservedAt.UTC()
	}
	return req
}

func normalizeHeartbeatRequest(req types.AutomationHeartbeatRequest) types.AutomationHeartbeatRequest {
	req.AutomationID = strings.TrimSpace(req.AutomationID)
	req.Status = strings.TrimSpace(req.Status)
	req.Message = strings.TrimSpace(req.Message)
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

func normalizeControlAction(action types.AutomationControlAction) types.AutomationControlAction {
	return types.AutomationControlAction(strings.ToLower(strings.TrimSpace(string(action))))
}

func normalizeSignals(signals []types.AutomationSignal) []types.AutomationSignal {
	if len(signals) == 0 {
		return nil
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
		return nil
	}
	return out
}

func normalizeLabels(labels map[string]string) map[string]string {
	if len(labels) == 0 {
		return nil
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
		return nil
	}
	return out
}

func normalizeStringList(values []string) []string {
	if len(values) == 0 {
		return nil
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
		return nil
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

func isValidOptionalJSON(raw json.RawMessage) bool {
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
