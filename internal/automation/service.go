package automation

import (
	"context"
	"errors"
	"strings"
	"time"

	"go-agent/internal/skills"
	"go-agent/internal/types"
)

var (
	errServiceNotConfigured         = errors.New("automation service is not configured")
	errMissingDispatchPlan          = &types.AutomationValidationError{Code: "missing_dispatch_plan", Message: "response_plan and delivery_policy are required"}
	errMissingRuntimePolicy         = &types.AutomationValidationError{Code: "missing_runtime_policy", Message: "runtime_policy is required"}
	errMissingAutomationID          = &types.AutomationValidationError{Code: "invalid_automation_spec", Message: "automation_id is required"}
	errMissingConfirmation          = &types.AutomationValidationError{Code: "missing_confirmation", Message: "automation_apply requires confirmed=true after user review"}
	errAutomationNotFound           = &types.AutomationValidationError{Code: "invalid_automation_spec", Message: "automation not found"}
	errInvalidControlAction         = &types.AutomationValidationError{Code: "invalid_automation_spec", Message: "action must be pause or resume"}
	errInvalidIncidentControlAction = &types.AutomationValidationError{Code: "invalid_automation_spec", Message: "action must be ack, close, reopen, or escalate"}
)

type Store interface {
	UpsertAutomation(context.Context, types.AutomationSpec) error
	GetAutomation(context.Context, string) (types.AutomationSpec, bool, error)
	ListAutomations(context.Context, types.AutomationListFilter) ([]types.AutomationSpec, error)
	DeleteAutomation(context.Context, string) (bool, error)
	UpsertTriggerEvent(context.Context, types.TriggerEvent) error
	UpsertAutomationIncident(context.Context, types.AutomationIncident) error
	GetAutomationIncident(context.Context, string) (types.AutomationIncident, bool, error)
	ListAutomationIncidents(context.Context, types.AutomationIncidentFilter) ([]types.AutomationIncident, error)
	UpsertIncidentPhaseState(context.Context, types.IncidentPhaseState) error
	UpsertAutomationHeartbeat(context.Context, types.AutomationHeartbeat) error
}

type Service struct {
	store              Store
	watcher            watcherRuntimeManager
	now                func() time.Time
	skillCatalogLoader func(string) (skills.Catalog, error)
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

func (s *Service) SetSkillCatalogLoader(loader func(string) (skills.Catalog, error)) {
	if s != nil {
		s.skillCatalogLoader = loader
	}
}

func (s *Service) Apply(ctx context.Context, spec types.AutomationSpec) (types.AutomationSpec, error) {
	if s == nil || s.store == nil {
		return types.AutomationSpec{}, errServiceNotConfigured
	}
	if err := validateResponsePlanDraftMode(spec.ResponsePlan); err != nil {
		return types.AutomationSpec{}, err
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
	if err := validateResponsePlanDraftMode(req.Spec.ResponsePlan); err != nil {
		return types.AutomationSpec{}, err
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
	childAgentBundles, err := loadChildAgentTemplateBundles(spec)
	if err != nil {
		return types.AutomationSpec{}, err
	}
	if s.skillCatalogLoader != nil {
		catalog, err := s.skillCatalogLoader(spec.WorkspaceRoot)
		if err != nil {
			return types.AutomationSpec{}, err
		}
		if err := validateChildAgentTemplateBundleRequiredSkills(childAgentBundles, catalog); err != nil {
			return types.AutomationSpec{}, err
		}
	}
	spec, err = backfillResponsePlanChildAgentTemplateCache(spec, childAgentBundles)
	if err != nil {
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

func (s *Service) ControlIncident(ctx context.Context, id string, action types.IncidentControlAction) (types.AutomationIncident, bool, error) {
	if s == nil || s.store == nil {
		return types.AutomationIncident{}, false, errServiceNotConfigured
	}

	action = normalizeIncidentControlAction(action)
	switch action {
	case types.IncidentControlActionAck, types.IncidentControlActionClose, types.IncidentControlActionReopen, types.IncidentControlActionEscalate:
	default:
		return types.AutomationIncident{}, false, errInvalidIncidentControlAction
	}

	incident, ok, err := s.store.GetAutomationIncident(ctx, strings.TrimSpace(id))
	if err != nil || !ok {
		return types.AutomationIncident{}, ok, err
	}

	switch action {
	case types.IncidentControlActionAck:
		switch incident.Status {
		case types.AutomationIncidentStatusOpen, types.AutomationIncidentStatusSuppressed:
			incident.Status = types.AutomationIncidentStatusQueued
		}
	case types.IncidentControlActionClose:
		incident.Status = types.AutomationIncidentStatusClosed
	case types.IncidentControlActionReopen:
		incident.Status = types.AutomationIncidentStatusOpen
	case types.IncidentControlActionEscalate:
		incident.Status = types.AutomationIncidentStatusEscalated
	}
	incident.UpdatedAt = s.currentTime()
	if err := s.store.UpsertAutomationIncident(ctx, incident); err != nil {
		return types.AutomationIncident{}, false, err
	}
	return incident, true, nil
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
	trigger := buildTriggerEvent(spec, req, now)
	if trigger.SignalKind == "" {
		return types.AutomationIncident{}, &types.AutomationValidationError{
			Code:    "invalid_automation_spec",
			Message: "signal_kind is required",
		}
	}
	if len(trigger.Payload) > 0 && !isValidJSONValue(trigger.Payload) {
		return types.AutomationIncident{}, &types.AutomationValidationError{
			Code:    "invalid_automation_spec",
			Message: "payload must be valid JSON",
		}
	}
	if err := s.store.UpsertTriggerEvent(ctx, trigger); err != nil {
		return types.AutomationIncident{}, err
	}

	dedupeWindow := dedupeWindowForSpec(spec)
	if strings.TrimSpace(trigger.DedupeKey) != "" {
		incidents, err := s.store.ListAutomationIncidents(ctx, types.AutomationIncidentFilter{
			AutomationID: spec.ID,
			Status:       types.AutomationIncidentStatusOpen,
		})
		if err != nil {
			return types.AutomationIncident{}, err
		}
		for _, incident := range incidents {
			if !incidentMatchesTriggerDedupe(incident, trigger.DedupeKey, trigger.ObservedAt, dedupeWindow) {
				continue
			}
			incident.SignalKind = trigger.SignalKind
			incident.Source = trigger.Source
			if trigger.Summary != "" {
				incident.Summary = trigger.Summary
			}
			if len(trigger.Payload) > 0 {
				incident.Payload = trigger.Payload
			}
			incident.ObservedAt = trigger.ObservedAt
			incident.UpdatedAt = now
			if err := s.store.UpsertAutomationIncident(ctx, incident); err != nil {
				return types.AutomationIncident{}, err
			}
			trigger.IncidentID = incident.ID
			trigger.UpdatedAt = now
			if err := s.store.UpsertTriggerEvent(ctx, trigger); err != nil {
				return types.AutomationIncident{}, err
			}
			return incident, nil
		}
	}

	incident := types.AutomationIncident{
		ID:            types.NewID("incident"),
		AutomationID:  spec.ID,
		WorkspaceRoot: spec.WorkspaceRoot,
		Status:        types.AutomationIncidentStatusOpen,
		SignalKind:    trigger.SignalKind,
		Source:        trigger.Source,
		Summary:       trigger.Summary,
		Payload:       trigger.Payload,
		ObservedAt:    trigger.ObservedAt,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := s.store.UpsertAutomationIncident(ctx, incident); err != nil {
		return types.AutomationIncident{}, err
	}

	for _, phase := range buildIncidentPhaseStates(spec, incident, now) {
		if err := s.store.UpsertIncidentPhaseState(ctx, phase); err != nil {
			return types.AutomationIncident{}, err
		}
	}

	trigger.IncidentID = incident.ID
	trigger.UpdatedAt = now
	if err := s.store.UpsertTriggerEvent(ctx, trigger); err != nil {
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
