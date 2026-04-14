package automation

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"go-agent/internal/types"
)

var errDispatcherStoreNotConfigured = errors.New("automation dispatcher store is not configured")
var errDispatcherTaskManagerNotConfigured = errors.New("automation dispatcher task manager is not configured")

type DispatcherStore interface {
	GetAutomation(context.Context, string) (types.AutomationSpec, bool, error)
	GetAutomationWatcher(context.Context, string) (types.AutomationWatcherRuntime, bool, error)
	ListAutomationIncidents(context.Context, types.AutomationIncidentFilter) ([]types.AutomationIncident, error)
	UpsertAutomationIncident(context.Context, types.AutomationIncident) error
	ListIncidentPhaseStates(context.Context, string) ([]types.IncidentPhaseState, error)
	UpsertIncidentPhaseState(context.Context, types.IncidentPhaseState) error
	ListDispatchAttempts(context.Context, types.DispatchAttemptFilter) ([]types.DispatchAttempt, error)
	UpsertDispatchAttempt(context.Context, types.DispatchAttempt) error
	ListAutomationWatcherHolds(context.Context, string) ([]types.AutomationWatcherHold, error)
	ReplaceAutomationWatcherHolds(context.Context, string, string, []types.AutomationWatcherHold) error
}

type DispatchTaskManager interface {
	StartAutomationDispatch(context.Context, types.DispatchAttempt, types.ChildAgentTemplate, types.AutomationIncident, ChildAgentRuntimeBundle) error
}

type DispatchWatcherSyncer interface {
	SyncAutomation(context.Context, string) error
}

type DispatcherConfig struct {
	Now     func() time.Time
	Watcher DispatchWatcherSyncer
}

type Dispatcher struct {
	store       DispatcherStore
	taskManager DispatchTaskManager
	watcher     DispatchWatcherSyncer
	now         func() time.Time
}

func NewDispatcher(store DispatcherStore, taskManager DispatchTaskManager, cfg DispatcherConfig) *Dispatcher {
	return &Dispatcher{
		store:       store,
		taskManager: taskManager,
		watcher:     cfg.Watcher,
		now:         firstNonNilClock(cfg.Now),
	}
}

func (d *Dispatcher) Tick(ctx context.Context) error {
	if d == nil || d.store == nil {
		return errDispatcherStoreNotConfigured
	}
	if d.taskManager == nil {
		return errDispatcherTaskManagerNotConfigured
	}

	incidents, err := d.listDispatchableIncidents(ctx)
	if err != nil {
		return err
	}
	for _, incident := range incidents {
		if err := d.dispatchIncident(ctx, incident); err != nil {
			return err
		}
	}
	return nil
}

func (d *Dispatcher) listDispatchableIncidents(ctx context.Context) ([]types.AutomationIncident, error) {
	filters := []types.AutomationIncidentFilter{
		{Status: types.AutomationIncidentStatusOpen},
		{Status: types.AutomationIncidentStatusQueued},
	}
	seen := make(map[string]struct{})
	out := make([]types.AutomationIncident, 0)
	for _, filter := range filters {
		incidents, err := d.store.ListAutomationIncidents(ctx, filter)
		if err != nil {
			return nil, err
		}
		for _, incident := range incidents {
			if _, ok := seen[incident.ID]; ok {
				continue
			}
			seen[incident.ID] = struct{}{}
			out = append(out, incident)
		}
	}
	return out, nil
}

func (d *Dispatcher) dispatchIncident(ctx context.Context, incident types.AutomationIncident) error {
	spec, ok, err := d.store.GetAutomation(ctx, incident.AutomationID)
	if err != nil || !ok {
		return err
	}
	if spec.State != types.AutomationStateActive {
		return nil
	}

	phases, err := d.store.ListIncidentPhaseStates(ctx, incident.ID)
	if err != nil {
		return err
	}
	phase, ok := nextDispatchablePhaseState(phases)
	if !ok {
		return nil
	}

	planPhase, template, ok := selectDispatchTemplate(spec.ResponsePlan, phase.Phase)
	if !ok {
		return nil
	}
	bundle, err := loadChildAgentRuntimeBundle(spec, phase.Phase, template.AgentID)
	if err != nil {
		return err
	}
	detectorSignal, err := ParseAutomationDetectorSignalPayload(incident.Payload)
	if err != nil {
		return err
	}
	if !detectorStatusAllowed(bundle.Strategy.EscalationCondition.WhenStatus, detectorSignal.Status) {
		return nil
	}

	attempts, err := d.store.ListDispatchAttempts(ctx, types.DispatchAttemptFilter{IncidentID: incident.ID})
	if err != nil {
		return err
	}
	if hasActiveDispatchAttempt(attempts, phase.Phase) {
		return nil
	}

	if template.AllowElevation && !approvalBindingResolvable(spec.RuntimePolicy) {
		return d.failApprovalUnroutable(ctx, incident, phase, template, bundle, attempts)
	}

	now := d.currentTime()
	approvalQueueKey, preferredSessionID := approvalBindingRouting(spec.RuntimePolicy)
	attempt := types.DispatchAttempt{
		DispatchID:          types.NewID("dispatch"),
		IncidentID:          incident.ID,
		AutomationID:        incident.AutomationID,
		WorkspaceRoot:       incident.WorkspaceRoot,
		Phase:               phase.Phase,
		Attempt:             nextDispatchAttemptNumber(attempts, phase.Phase),
		Status:              types.DispatchAttemptStatusRunning,
		ChildAgentID:        template.AgentID,
		ActivatedSkillNames: append([]string(nil), bundle.Skills.Required...),
		OutputContractRef:   template.OutputContractRef,
		ApprovalQueueKey:    approvalQueueKey,
		PreferredSessionID:  preferredSessionID,
		StartedAt:           now,
		CreatedAt:           now,
		UpdatedAt:           now,
	}
	if err := d.store.UpsertDispatchAttempt(ctx, attempt); err != nil {
		return err
	}
	if err := AcquireWatcherHoldByOwner(ctx, d.store, attempt.AutomationID, types.AutomationWatcherHoldKindDispatch, attempt.DispatchID, "dispatch active", now); err != nil {
		return err
	}
	if err := d.syncWatcher(ctx, attempt.AutomationID); err != nil {
		return err
	}

	phase.DispatchIDs = append(append([]string(nil), phase.DispatchIDs...), attempt.DispatchID)
	phase.ActiveDispatchCount++
	phase.Status = types.IncidentPhaseStatusRunning
	phase.UpdatedAt = now
	if err := d.store.UpsertIncidentPhaseState(ctx, phase); err != nil {
		return err
	}

	incident.Status = nextIncidentStatusForDispatch(planPhase, incident.Status)
	incident.UpdatedAt = now
	if err := d.store.UpsertAutomationIncident(ctx, incident); err != nil {
		return err
	}

	if err := d.taskManager.StartAutomationDispatch(ctx, attempt, template, incident, bundle); err != nil {
		_ = ReleaseWatcherHoldByOwner(ctx, d.store, attempt.AutomationID, types.AutomationWatcherHoldKindDispatch, attempt.DispatchID)
		_ = d.syncWatcher(ctx, attempt.AutomationID)
		attempt.Status = types.DispatchAttemptStatusFailed
		attempt.Error = strings.TrimSpace(err.Error())
		attempt.FinishedAt = now
		attempt.UpdatedAt = now
		if upsertErr := d.store.UpsertDispatchAttempt(ctx, attempt); upsertErr != nil {
			return upsertErr
		}
		phase.ActiveDispatchCount = 0
		phase.FailedDispatchCount++
		phase.Status = types.IncidentPhaseStatusFailed
		phase.UpdatedAt = now
		if upsertErr := d.store.UpsertIncidentPhaseState(ctx, phase); upsertErr != nil {
			return upsertErr
		}
		incident.Status = types.AutomationIncidentStatusFailed
		incident.UpdatedAt = now
		return d.store.UpsertAutomationIncident(ctx, incident)
	}

	return nil
}

func (d *Dispatcher) syncWatcher(ctx context.Context, automationID string) error {
	if d == nil || d.watcher == nil {
		return nil
	}
	return d.watcher.SyncAutomation(ctx, automationID)
}

func (d *Dispatcher) failApprovalUnroutable(ctx context.Context, incident types.AutomationIncident, phase types.IncidentPhaseState, template types.ChildAgentTemplate, bundle ChildAgentRuntimeBundle, attempts []types.DispatchAttempt) error {
	now := d.currentTime()
	attempt := types.DispatchAttempt{
		DispatchID:          types.NewID("dispatch"),
		IncidentID:          incident.ID,
		AutomationID:        incident.AutomationID,
		WorkspaceRoot:       incident.WorkspaceRoot,
		Phase:               phase.Phase,
		Attempt:             nextDispatchAttemptNumber(attempts, phase.Phase),
		Status:              types.DispatchAttemptStatusFailed,
		ChildAgentID:        template.AgentID,
		ActivatedSkillNames: append([]string(nil), bundle.Skills.Required...),
		OutputContractRef:   template.OutputContractRef,
		Error:               "approval_unroutable",
		FinishedAt:          now,
		CreatedAt:           now,
		UpdatedAt:           now,
	}
	if err := d.store.UpsertDispatchAttempt(ctx, attempt); err != nil {
		return err
	}

	phase.DispatchIDs = append(append([]string(nil), phase.DispatchIDs...), attempt.DispatchID)
	phase.FailedDispatchCount++
	phase.Status = types.IncidentPhaseStatusFailed
	phase.UpdatedAt = now
	if err := d.store.UpsertIncidentPhaseState(ctx, phase); err != nil {
		return err
	}

	incident.Status = types.AutomationIncidentStatusEscalated
	incident.UpdatedAt = now
	return d.store.UpsertAutomationIncident(ctx, incident)
}

func nextDispatchablePhaseState(phases []types.IncidentPhaseState) (types.IncidentPhaseState, bool) {
	for _, phase := range phases {
		if phase.Status == types.IncidentPhaseStatusPending {
			return phase, true
		}
	}
	return types.IncidentPhaseState{}, false
}

func selectDispatchTemplate(raw json.RawMessage, phaseName types.AutomationPhaseName) (types.AutomationPhasePlan, types.ChildAgentTemplate, bool) {
	plan := loadResponsePlanV2(raw)
	for _, phase := range plan.Phases {
		if phase.Phase != phaseName {
			continue
		}
		if len(phase.ChildAgents) == 0 {
			return phase, types.ChildAgentTemplate{}, false
		}
		return phase, phase.ChildAgents[0], true
	}
	return types.AutomationPhasePlan{}, types.ChildAgentTemplate{}, false
}

func nextDispatchAttemptNumber(attempts []types.DispatchAttempt, phase types.AutomationPhaseName) int {
	next := 1
	for _, attempt := range attempts {
		if attempt.Phase != phase {
			continue
		}
		if attempt.Attempt >= next {
			next = attempt.Attempt + 1
		}
	}
	return next
}

func hasActiveDispatchAttempt(attempts []types.DispatchAttempt, phase types.AutomationPhaseName) bool {
	for _, attempt := range attempts {
		if attempt.Phase != phase {
			continue
		}
		switch attempt.Status {
		case types.DispatchAttemptStatusPlanned, types.DispatchAttemptStatusAwaitingApproval, types.DispatchAttemptStatusRunning:
			return true
		}
	}
	return false
}

func approvalBindingResolvable(raw json.RawMessage) bool {
	cfg := approvalBindingConfig(raw)
	return strings.TrimSpace(cfg.WorkspaceBinding) != "" && strings.TrimSpace(cfg.OwnerKey) != ""
}

func approvalBindingRouting(raw json.RawMessage) (string, string) {
	cfg := approvalBindingConfig(raw)
	return strings.TrimSpace(cfg.WorkspaceBinding), strings.TrimSpace(cfg.PreferredSessionID)
}

type approvalBinding struct {
	WorkspaceBinding   string `json:"workspace_binding"`
	OwnerKey           string `json:"owner_key"`
	PreferredSessionID string `json:"preferred_session_id"`
}

func approvalBindingConfig(raw json.RawMessage) approvalBinding {
	cfg := struct {
		ApprovalBinding approvalBinding `json:"approval_binding"`
	}{}
	_ = json.Unmarshal(raw, &cfg)
	return cfg.ApprovalBinding
}

func nextIncidentStatusForDispatch(_ types.AutomationPhasePlan, current types.AutomationIncidentStatus) types.AutomationIncidentStatus {
	switch current {
	case types.AutomationIncidentStatusQueued, types.AutomationIncidentStatusOpen:
		return types.AutomationIncidentStatusActive
	default:
		return current
	}
}

func (d *Dispatcher) currentTime() time.Time {
	if d != nil && d.now != nil {
		return d.now().UTC()
	}
	return time.Now().UTC()
}
