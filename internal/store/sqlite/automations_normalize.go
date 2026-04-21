package sqlite

import (
	"strings"
	"time"

	"go-agent/internal/types"
)

func normalizeAutomationSpecForStore(spec types.AutomationSpec) types.AutomationSpec {
	now := time.Now().UTC()
	spec.ID = strings.TrimSpace(spec.ID)
	if spec.ID == "" {
		spec.ID = types.NewID("automation")
	}
	spec.Title = strings.TrimSpace(spec.Title)
	spec.WorkspaceRoot = strings.TrimSpace(spec.WorkspaceRoot)
	spec.Goal = strings.TrimSpace(spec.Goal)
	spec.State = normalizeAutomationStateForStore(spec.State)
	spec.Assumptions = types.NormalizeAutomationAssumptions(spec.Assumptions)

	spec.Context.Owner = strings.TrimSpace(spec.Context.Owner)
	spec.Context.Environment = strings.TrimSpace(spec.Context.Environment)
	spec.Context.Targets = normalizeAutomationStringList(spec.Context.Targets)
	labels := make(map[string]string, len(spec.Context.Labels))
	for key, value := range spec.Context.Labels {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		labels[key] = strings.TrimSpace(value)
	}
	spec.Context.Labels = labels

	signals := make([]types.AutomationSignal, 0, len(spec.Signals))
	for _, signal := range spec.Signals {
		signal.Kind = strings.TrimSpace(signal.Kind)
		signal.Source = strings.TrimSpace(signal.Source)
		signal.Selector = strings.TrimSpace(signal.Selector)
		signal.Payload = normalizeAutomationRawJSON(signal.Payload)
		if signal.Kind == "" && signal.Source == "" && signal.Selector == "" && len(signal.Payload) == 0 {
			continue
		}
		signals = append(signals, signal)
	}
	spec.Signals = signals
	spec.IncidentPolicy = normalizeAutomationObjectJSON(spec.IncidentPolicy)
	spec.ResponsePlan = normalizeAutomationResponsePlanForStore(spec.ResponsePlan)
	spec.VerificationPlan = normalizeAutomationObjectJSON(spec.VerificationPlan)
	spec.EscalationPolicy = normalizeAutomationObjectJSON(spec.EscalationPolicy)
	spec.DeliveryPolicy = normalizeAutomationRawJSON(spec.DeliveryPolicy)
	spec.RuntimePolicy = normalizeAutomationRawJSON(spec.RuntimePolicy)
	spec.WatcherLifecycle = normalizeAutomationObjectJSON(spec.WatcherLifecycle)
	spec.RetriggerPolicy = normalizeAutomationObjectJSON(spec.RetriggerPolicy)
	spec.RunPolicy = normalizeAutomationObjectJSON(spec.RunPolicy)

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

func normalizeAutomationIncidentForStore(incident types.AutomationIncident) types.AutomationIncident {
	now := time.Now().UTC()
	incident.ID = strings.TrimSpace(incident.ID)
	if incident.ID == "" {
		incident.ID = types.NewID("incident")
	}
	incident.AutomationID = strings.TrimSpace(incident.AutomationID)
	incident.WorkspaceRoot = strings.TrimSpace(incident.WorkspaceRoot)
	incident.Status = types.AutomationIncidentStatus(strings.ToLower(strings.TrimSpace(string(incident.Status))))
	if incident.Status == "" {
		incident.Status = types.AutomationIncidentStatusOpen
	}
	incident.SignalKind = strings.TrimSpace(incident.SignalKind)
	incident.Source = strings.TrimSpace(incident.Source)
	incident.Summary = strings.TrimSpace(incident.Summary)
	incident.Payload = normalizeAutomationRawJSON(incident.Payload)
	if incident.ObservedAt.IsZero() {
		incident.ObservedAt = now
	} else {
		incident.ObservedAt = incident.ObservedAt.UTC()
	}
	if incident.CreatedAt.IsZero() {
		incident.CreatedAt = now
	} else {
		incident.CreatedAt = incident.CreatedAt.UTC()
	}
	if incident.UpdatedAt.IsZero() {
		incident.UpdatedAt = incident.CreatedAt
	} else {
		incident.UpdatedAt = incident.UpdatedAt.UTC()
	}
	if incident.UpdatedAt.Before(incident.CreatedAt) {
		incident.UpdatedAt = incident.CreatedAt
	}
	return incident
}
