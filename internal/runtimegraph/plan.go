package runtimegraph

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go-agent/internal/types"
)

type EnterPlanModeInput struct {
	SessionID string
	TurnID    string
	RunID     string
	PlanFile  string
}

type EnterPlanModeOutput struct {
	PlanID   string          `json:"plan_id"`
	RunID    string          `json:"run_id"`
	State    types.PlanState `json:"state"`
	PlanFile string          `json:"plan_file"`
}

type ExitPlanModeInput struct {
	SessionID  string
	FinalState types.PlanState
}

type ExitPlanModeOutput struct {
	PlanID string          `json:"plan_id"`
	State  types.PlanState `json:"state"`
}

func (s *Service) EnterPlanMode(ctx context.Context, turnCtx *TurnContext, in EnterPlanModeInput) (EnterPlanModeOutput, error) {
	if s == nil || s.store == nil {
		return EnterPlanModeOutput{}, fmt.Errorf("runtime service is not configured")
	}
	if turnCtx == nil {
		return EnterPlanModeOutput{}, fmt.Errorf("turn runtime context is not configured")
	}
	if strings.TrimSpace(in.SessionID) == "" {
		return EnterPlanModeOutput{}, fmt.Errorf("session id is required")
	}

	planFile := strings.TrimSpace(in.PlanFile)
	if planFile == "" {
		return EnterPlanModeOutput{}, fmt.Errorf("plan_file is required")
	}

	now := time.Now().UTC()
	runID := strings.TrimSpace(in.RunID)
	turnID := strings.TrimSpace(in.TurnID)
	var planID string

	err := s.store.WithTx(ctx, func(tx RuntimeTx) error {
		if runID == "" {
			runID = types.NewID("run")
			if err := tx.InsertRun(ctx, types.Run{
				ID:        runID,
				SessionID: strings.TrimSpace(in.SessionID),
				TurnID:    turnID,
				State:     types.RunStateRunning,
				Objective: "Plan mode session",
				CreatedAt: now,
				UpdatedAt: now,
			}); err != nil {
				return err
			}
		}

		activePlans, err := tx.ListActivePlansForSession(ctx, in.SessionID)
		if err != nil {
			return err
		}
		for _, plan := range activePlans {
			plan.State = types.PlanStateCompleted
			plan.UpdatedAt = now
			if err := tx.UpsertPlan(ctx, plan); err != nil {
				return err
			}
		}

		planID = types.NewID("plan")
		return tx.UpsertPlan(ctx, types.Plan{
			ID:        planID,
			RunID:     runID,
			State:     types.PlanStateActive,
			PlanFile:  planFile,
			CreatedAt: now,
			UpdatedAt: now,
		})
	})
	if err != nil {
		return EnterPlanModeOutput{}, err
	}

	turnCtx.CurrentRunID = runID
	return EnterPlanModeOutput{
		PlanID:   planID,
		RunID:    runID,
		State:    types.PlanStateActive,
		PlanFile: planFile,
	}, nil
}

func (s *Service) ExitPlanMode(ctx context.Context, in ExitPlanModeInput) (ExitPlanModeOutput, error) {
	if s == nil || s.store == nil {
		return ExitPlanModeOutput{}, fmt.Errorf("runtime service is not configured")
	}
	if strings.TrimSpace(in.SessionID) == "" {
		return ExitPlanModeOutput{}, fmt.Errorf("session id is required")
	}

	finalState := in.FinalState
	if finalState == "" {
		finalState = types.PlanStateCompleted
	}
	switch finalState {
	case types.PlanStateCompleted, types.PlanStateApproved, types.PlanStateFailed:
	default:
		return ExitPlanModeOutput{}, fmt.Errorf("invalid plan state %q", finalState)
	}

	now := time.Now().UTC()
	var updatedPlanID string
	err := s.store.WithTx(ctx, func(tx RuntimeTx) error {
		activePlans, err := tx.ListActivePlansForSession(ctx, in.SessionID)
		if err != nil {
			return err
		}
		if len(activePlans) == 0 {
			return fmt.Errorf("no active plan found")
		}

		for i, plan := range activePlans {
			plan.State = finalState
			plan.UpdatedAt = now
			if err := tx.UpsertPlan(ctx, plan); err != nil {
				return err
			}
			if i == 0 {
				updatedPlanID = plan.ID
			}
		}
		return nil
	})
	if err != nil {
		return ExitPlanModeOutput{}, err
	}

	return ExitPlanModeOutput{
		PlanID: updatedPlanID,
		State:  finalState,
	}, nil
}
