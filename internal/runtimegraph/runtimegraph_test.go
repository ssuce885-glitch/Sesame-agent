package runtimegraph

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go-agent/internal/store/sqlite"
	"go-agent/internal/types"
)

func TestServiceEnterPlanModeCreatesRunArchivesSessionPlansAndUpdatesTurnContext(t *testing.T) {
	store, err := sqlite.Open(filepath.Join(t.TempDir(), "agentd.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	now := time.Date(2026, 4, 6, 11, 0, 0, 0, time.UTC)
	if err := store.InsertRun(context.Background(), types.Run{
		ID:        "run_existing",
		SessionID: "sess_plan",
		State:     types.RunStateRunning,
		CreatedAt: now,
		UpdatedAt: now,
	}); err != nil {
		t.Fatalf("InsertRun(existing) error = %v", err)
	}
	if err := store.UpsertPlan(context.Background(), types.Plan{
		ID:        "plan_existing",
		RunID:     "run_existing",
		State:     types.PlanStateActive,
		PlanFile:  "docs/superpowers/plans/old.md",
		CreatedAt: now,
		UpdatedAt: now,
	}); err != nil {
		t.Fatalf("UpsertPlan(existing) error = %v", err)
	}

	service := NewService(store)
	turnCtx := &TurnContext{CurrentSessionID: "sess_plan", CurrentTurnID: "turn_plan"}
	got, err := service.EnterPlanMode(context.Background(), turnCtx, EnterPlanModeInput{
		SessionID: "sess_plan",
		TurnID:    "turn_plan",
		PlanFile:  "docs/superpowers/plans/new.md",
	})
	if err != nil {
		t.Fatalf("EnterPlanMode() error = %v", err)
	}
	if got.PlanID == "" || got.RunID == "" {
		t.Fatalf("EnterPlanMode() = %#v, want non-empty ids", got)
	}
	if turnCtx.CurrentRunID != got.RunID {
		t.Fatalf("TurnContext.CurrentRunID = %q, want %q", turnCtx.CurrentRunID, got.RunID)
	}

	active, err := store.ListActivePlansForSession(context.Background(), "sess_plan")
	if err != nil {
		t.Fatalf("ListActivePlansForSession() error = %v", err)
	}
	if len(active) != 1 || active[0].ID != got.PlanID || active[0].PlanFile != "docs/superpowers/plans/new.md" {
		t.Fatalf("active plans = %#v, want only the newly created active plan", active)
	}

	graph, err := store.ListRuntimeGraph(context.Background())
	if err != nil {
		t.Fatalf("ListRuntimeGraph() error = %v", err)
	}
	var archived bool
	for _, plan := range graph.Plans {
		if plan.ID == "plan_existing" && plan.State == types.PlanStateCompleted {
			archived = true
		}
	}
	if !archived {
		t.Fatalf("graph.Plans = %#v, want existing active plan archived to completed", graph.Plans)
	}
}

func TestServiceExitPlanModeFinalizesActivePlan(t *testing.T) {
	store, err := sqlite.Open(filepath.Join(t.TempDir(), "agentd.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	now := time.Date(2026, 4, 6, 11, 30, 0, 0, time.UTC)
	if err := store.InsertRun(context.Background(), types.Run{
		ID:        "run_exit",
		SessionID: "sess_exit",
		State:     types.RunStateRunning,
		CreatedAt: now,
		UpdatedAt: now,
	}); err != nil {
		t.Fatalf("InsertRun() error = %v", err)
	}
	if err := store.UpsertPlan(context.Background(), types.Plan{
		ID:        "plan_exit",
		RunID:     "run_exit",
		State:     types.PlanStateActive,
		PlanFile:  "docs/superpowers/plans/exit.md",
		CreatedAt: now,
		UpdatedAt: now,
	}); err != nil {
		t.Fatalf("UpsertPlan() error = %v", err)
	}

	service := NewService(store)
	got, err := service.ExitPlanMode(context.Background(), ExitPlanModeInput{
		SessionID:  "sess_exit",
		FinalState: types.PlanStateApproved,
	})
	if err != nil {
		t.Fatalf("ExitPlanMode() error = %v", err)
	}
	if got.PlanID != "plan_exit" || got.State != types.PlanStateApproved {
		t.Fatalf("ExitPlanMode() = %#v, want plan_exit approved", got)
	}

	active, err := store.ListActivePlansForSession(context.Background(), "sess_exit")
	if err != nil {
		t.Fatalf("ListActivePlansForSession() error = %v", err)
	}
	if len(active) != 0 {
		t.Fatalf("active plans = %#v, want none after exit", active)
	}
}

func TestServiceExitPlanModeReturnsNotFoundWithoutActivePlan(t *testing.T) {
	store, err := sqlite.Open(filepath.Join(t.TempDir(), "agentd.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	service := NewService(store)
	_, err = service.ExitPlanMode(context.Background(), ExitPlanModeInput{
		SessionID:  "sess_missing",
		FinalState: types.PlanStateCompleted,
	})
	if err == nil || !strings.Contains(err.Error(), "no active plan found") {
		t.Fatalf("ExitPlanMode() error = %v, want no active plan found", err)
	}
}
