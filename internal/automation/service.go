package automation

import (
	"context"
	"errors"
	"strings"
	"time"

	"go-agent/internal/types"
)

var (
	errServiceNotConfigured      = errors.New("automation service is not configured")
	errMissingAutomationID       = &types.AutomationValidationError{Code: "invalid_automation_spec", Message: "automation_id is required"}
	errMissingConfirmation       = &types.AutomationValidationError{Code: "missing_confirmation", Message: "automation_apply requires confirmed=true after user review"}
	errAutomationNotFound        = &types.AutomationValidationError{Code: "invalid_automation_spec", Message: "automation not found"}
	errInvalidControlAction      = &types.AutomationValidationError{Code: "invalid_automation_spec", Message: "action must be pause or resume"}
	errManagedRuntimeUnsupported = &types.AutomationValidationError{Code: "unsupported_automation_mode", Message: "managed automation runtime is removed; only simple mode automations are supported"}
)

type Store interface {
	UpsertAutomation(context.Context, types.AutomationSpec) error
	GetAutomation(context.Context, string) (types.AutomationSpec, bool, error)
	ListAutomations(context.Context, types.AutomationListFilter) ([]types.AutomationSpec, error)
	DeleteAutomation(context.Context, string) (bool, error)
	UpsertTriggerEvent(context.Context, types.TriggerEvent) error
	UpsertAutomationHeartbeat(context.Context, types.AutomationHeartbeat) error
	UpsertSimpleAutomationRun(context.Context, types.SimpleAutomationRun) error
	GetSimpleAutomationRun(context.Context, string, string) (types.SimpleAutomationRun, bool, error)
}

type Service struct {
	store         Store
	watcher       watcherRuntimeManager
	simpleRuntime *SimpleRuntime
	now           func() time.Time
}

type automationHeartbeatStore interface {
	ListAutomationHeartbeats(context.Context, types.AutomationHeartbeatFilter) ([]types.AutomationHeartbeat, error)
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

func (s *Service) SetSimpleRuntime(runtime *SimpleRuntime) {
	if s != nil {
		s.simpleRuntime = runtime
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
	spec, ok, err := s.store.GetAutomation(ctx, id)
	if err != nil || !ok {
		return false, err
	}
	if s.watcher != nil {
		if err := s.watcher.Delete(ctx, id); err != nil {
			return false, err
		}
	}
	deleted, err := s.store.DeleteAutomation(ctx, id)
	if err != nil || !deleted {
		return deleted, err
	}
	if err := RemoveRoleBoundAutomationSource(spec.WorkspaceRoot, spec.Owner, spec.ID); err != nil {
		return false, err
	}
	return true, nil
}

func (s *Service) EmitTrigger(ctx context.Context, req types.AutomationTriggerRequest) (types.TriggerEvent, error) {
	if s == nil || s.store == nil {
		return types.TriggerEvent{}, errServiceNotConfigured
	}
	req = normalizeTriggerRequest(req)
	if req.AutomationID == "" {
		return types.TriggerEvent{}, errMissingAutomationID
	}
	spec, ok, err := s.store.GetAutomation(ctx, req.AutomationID)
	if err != nil {
		return types.TriggerEvent{}, err
	}
	if !ok {
		return types.TriggerEvent{}, errAutomationNotFound
	}
	if !strings.EqualFold(strings.TrimSpace(string(spec.Mode)), string(types.AutomationModeSimple)) {
		return types.TriggerEvent{}, errManagedRuntimeUnsupported
	}

	now := s.currentTime()
	trigger := buildTriggerEvent(spec, req, now)
	if trigger.SignalKind == "" {
		return types.TriggerEvent{}, &types.AutomationValidationError{
			Code:    "invalid_automation_spec",
			Message: "signal_kind is required",
		}
	}
	if len(trigger.Payload) > 0 && !isValidJSONValue(trigger.Payload) {
		return types.TriggerEvent{}, &types.AutomationValidationError{
			Code:    "invalid_automation_spec",
			Message: "payload must be valid JSON",
		}
	}
	if err := s.store.UpsertTriggerEvent(ctx, trigger); err != nil {
		return types.TriggerEvent{}, err
	}
	if s.simpleRuntime == nil {
		return types.TriggerEvent{}, errServiceNotConfigured
	}
	if err := s.simpleRuntime.HandleMatch(ctx, spec, trigger); err != nil {
		return types.TriggerEvent{}, err
	}
	return trigger, nil
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

func (s *Service) GetWatcher(ctx context.Context, id string) (types.AutomationWatcherRuntime, bool, error) {
	if s == nil || s.watcher == nil {
		return types.AutomationWatcherRuntime{}, false, nil
	}
	return s.watcher.Get(ctx, strings.TrimSpace(id))
}

func (s *Service) ListHeartbeats(ctx context.Context, filter types.AutomationHeartbeatFilter) ([]types.AutomationHeartbeat, error) {
	if s == nil || s.store == nil {
		return nil, errServiceNotConfigured
	}
	heartbeatStore, ok := s.store.(automationHeartbeatStore)
	if !ok {
		return nil, errServiceNotConfigured
	}
	return heartbeatStore.ListAutomationHeartbeats(ctx, normalizeHeartbeatFilter(filter))
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
	if types.NormalizeAutomationID(spec.ID) == "" {
		return &types.AutomationValidationError{
			Code:    "invalid_automation_spec",
			Message: "automation_id must match ^[a-z][a-z0-9_-]{0,127}$",
		}
	}
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
	if !strings.EqualFold(strings.TrimSpace(string(spec.Mode)), string(types.AutomationModeSimple)) {
		return errManagedRuntimeUnsupported
	}
	if types.NormalizeRoleAutomationOwner(spec.Owner) == "" {
		return &types.AutomationValidationError{
			Code:    "invalid_automation_spec",
			Message: "owner must be role:<role_id> for simple mode automations",
		}
	}
	if _, err := normalizeSimpleAutomationMainAgentTarget("report_target", spec.ReportTarget); err != nil {
		return err
	}
	if _, err := normalizeSimpleAutomationMainAgentTarget("escalation_target", spec.EscalationTarget); err != nil {
		return err
	}
	if !isJSONObject(spec.WatcherLifecycle) || !isJSONObject(spec.RetriggerPolicy) {
		return &types.AutomationValidationError{
			Code:    "invalid_automation_spec",
			Message: "watcher_lifecycle and retrigger_policy must be JSON objects",
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
	if err := ValidateRoleBoundAutomationSpec(spec); err != nil {
		return &types.AutomationValidationError{
			Code:    "invalid_automation_spec",
			Message: err.Error(),
		}
	}
	return nil
}
