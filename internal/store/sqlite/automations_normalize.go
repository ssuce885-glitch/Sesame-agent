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
	spec.WatcherLifecycle = normalizeAutomationObjectJSON(spec.WatcherLifecycle)
	spec.RetriggerPolicy = normalizeAutomationObjectJSON(spec.RetriggerPolicy)

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
