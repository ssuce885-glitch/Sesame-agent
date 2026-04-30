package engine

import (
	"strings"
	"testing"

	"go-agent/internal/roles"
	"go-agent/internal/tools"
)

func TestRoleBudgetTrackerEnforcesToolAndContextLimits(t *testing.T) {
	tracker := NewRoleBudgetTracker(roles.Spec{
		RoleID: "analyst",
		Budget: &roles.RoleBudgetConfig{
			MaxToolCalls:     2,
			MaxContextTokens: 10,
			MaxConcurrent:    1,
		},
	}, roles.RoleBudgetConfig{})

	if err := tracker.CanStartTurn(); err != nil {
		t.Fatalf("CanStartTurn() error = %v", err)
	}
	defer tracker.FinishTurn()

	if err := tracker.RecordToolCall(2); err != nil {
		t.Fatalf("RecordToolCall(2) error = %v", err)
	}
	if err := tracker.RecordToolCall(1); err == nil || !strings.Contains(err.Error(), "max tool calls") {
		t.Fatalf("RecordToolCall over limit error = %v, want max tool calls", err)
	}
	if err := tracker.CheckContextTokens(11); err == nil || !strings.Contains(err.Error(), "max context tokens") {
		t.Fatalf("CheckContextTokens over limit error = %v, want max context tokens", err)
	}
}

func TestRoleBudgetTrackerUsesRoleValuesWhenSet(t *testing.T) {
	budget := effectiveRoleBudget(&roles.RoleBudgetConfig{
		MaxRuntime:       "1h",
		MaxToolCalls:     100,
		MaxContextTokens: 200000,
		MaxTurnsPerHour:  1000,
		MaxConcurrent:    10,
	}, roles.RoleBudgetConfig{
		MaxRuntime:       "30m",
		MaxToolCalls:     20,
		MaxContextTokens: 16000,
		MaxTurnsPerHour:  60,
		MaxConcurrent:    1,
	})

	if budget.MaxRuntime != "1h" || budget.MaxToolCalls != 100 || budget.MaxContextTokens != 200000 || budget.MaxTurnsPerHour != 1000 || budget.MaxConcurrent != 10 {
		t.Fatalf("budget did not use role values: %#v", budget)
	}
}

func TestRoleBudgetTrackerFallsBackToDefaultWhenRoleNotSet(t *testing.T) {
	budget := effectiveRoleBudget(&roles.RoleBudgetConfig{}, roles.RoleBudgetConfig{
		MaxRuntime:       "30m",
		MaxToolCalls:     20,
		MaxContextTokens: 16000,
		MaxTurnsPerHour:  60,
		MaxConcurrent:    1,
	})

	if budget.MaxRuntime != "30m" || budget.MaxToolCalls != 20 || budget.MaxContextTokens != 16000 || budget.MaxTurnsPerHour != 60 || budget.MaxConcurrent != 1 {
		t.Fatalf("budget did not fall back to defaults: %#v", budget)
	}
}

func TestApplyRolePolicyToToolDefinitionsFiltersDenied(t *testing.T) {
	defs := []tools.Definition{
		{Name: "file_read"},
		{Name: "shell_command"},
		{Name: "delegate_to_role"},
	}
	spec := &roles.Spec{Policy: &roles.RolePolicyConfig{
		DeniedTools: []string{"shell_command", "delegate_to_role"},
	}}

	filtered := applyRolePolicyToToolDefinitions(spec, defs)
	if len(filtered) != 1 || filtered[0].Name != "file_read" {
		t.Fatalf("filtered = %#v, want only file_read", filtered)
	}
}
