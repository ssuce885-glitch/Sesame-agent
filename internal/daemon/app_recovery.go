package daemon

import (
	"context"
	"strings"
	"time"

	rolectx "go-agent/internal/roles"
	"go-agent/internal/session"
	"go-agent/internal/sessionrole"
	"go-agent/internal/store/sqlite"
	"go-agent/internal/types"
	"go-agent/internal/workspace"
)

func recoverRuntimeState(ctx context.Context, store *sqlite.Store, manager *session.Manager, reportEnqueue ...reportTurnEnqueuer) error {
	sessions, err := store.ListSessions(ctx)
	if err != nil {
		return err
	}
	if manager != nil {
		for _, sessionRow := range sessions {
			manager.RegisterSession(sessionRow)
		}
	}
	running, err := store.ListRunningTurns(ctx)
	if err != nil {
		return err
	}

	resumedTurns := make(map[string]struct{})
	checkpointedTurns, err := store.ListInterruptedTurnsWithCheckpoints(ctx)
	if err != nil {
		return err
	}
	for _, turnID := range checkpointedTurns {
		checkpoint, found, err := store.GetLatestTurnCheckpoint(ctx, turnID)
		if err != nil {
			return err
		}
		if !found || checkpoint.State != types.TurnCheckpointStatePostToolBatch {
			continue
		}
		resumed, err := resumeTurnFromCheckpoint(ctx, store, manager, checkpoint)
		if err != nil {
			return err
		}
		if resumed {
			resumedTurns[turnID] = struct{}{}
		}
	}

	for _, turn := range running {
		if _, ok := resumedTurns[turn.ID]; ok {
			continue
		}
		if turn.Kind == types.TurnKindReportBatch {
			if _, err := store.RequeueClaimedReportDeliveriesForTurn(ctx, turn.ID); err != nil {
				return err
			}
		}
		if err := store.MarkTurnInterrupted(ctx, turn.ID); err != nil {
			return err
		}

		event, err := types.NewEvent(turn.SessionID, turn.ID, types.EventTurnInterrupted, map[string]string{
			"reason": "daemon_restart",
		})
		if err != nil {
			return err
		}
		if _, err := store.AppendEvent(ctx, event); err != nil {
			return err
		}
	}

	if err := recoverQueuedCreatedTurns(ctx, store, manager, sessions); err != nil {
		return err
	}
	var enqueuer reportTurnEnqueuer
	if len(reportEnqueue) > 0 {
		enqueuer = reportEnqueue[0]
	}
	return recoverQueuedReportTurns(ctx, store, manager, sessions, enqueuer)
}

func recoverQueuedCreatedTurns(ctx context.Context, store *sqlite.Store, manager *session.Manager, sessions []types.Session) error {
	if store == nil || manager == nil {
		return nil
	}

	for _, sessionRow := range sessions {
		sessionID := strings.TrimSpace(sessionRow.ID)
		if sessionID == "" {
			continue
		}

		turns, err := store.ListTurnsBySession(ctx, sessionID)
		if err != nil {
			return err
		}
		createdTurns := make([]types.Turn, 0, len(turns))
		for _, turn := range turns {
			if turn.State == types.TurnStateCreated {
				createdTurns = append(createdTurns, turn)
			}
		}
		if len(createdTurns) == 0 {
			continue
		}
		if strings.HasPrefix(sessionID, "task_session_") {
			for _, turn := range createdTurns {
				if err := interruptUnrecoverableCreatedTurn(ctx, store, turn, "task_session_replay_unsupported"); err != nil {
					return err
				}
			}
			continue
		}

		specialistRoleID, err := store.ResolveSpecialistRoleID(ctx, sessionID, sessionRow.WorkspaceRoot)
		if err != nil {
			return err
		}
		role, err := store.ResolveSessionRole(ctx, sessionID, sessionRow.WorkspaceRoot)
		if err != nil {
			return err
		}

		if specialistRoleID == "" && role != types.SessionRoleMainParent {
			for _, turn := range createdTurns {
				if err := interruptUnrecoverableCreatedTurn(ctx, store, turn, "unmapped_session"); err != nil {
					return err
				}
			}
			continue
		}

		replayRole := role
		if specialistRoleID != "" {
			replayRole = types.SessionRoleMainParent
		}
		replayCtx := workspace.WithWorkspaceRoot(
			rolectx.WithSpecialistRoleID(sessionrole.WithSessionRole(ctx, replayRole), specialistRoleID),
			sessionRow.WorkspaceRoot,
		)

		for _, turn := range createdTurns {
			if _, err := manager.SubmitTurn(replayCtx, sessionID, session.SubmitTurnInput{Turn: turn}); err != nil {
				return err
			}
		}
	}
	return nil
}

func interruptUnrecoverableCreatedTurn(ctx context.Context, store *sqlite.Store, turn types.Turn, reason string) error {
	if store == nil {
		return nil
	}
	if err := store.MarkTurnInterrupted(ctx, turn.ID); err != nil {
		return err
	}
	event, err := types.NewEvent(turn.SessionID, turn.ID, types.EventTurnInterrupted, map[string]string{
		"reason": strings.TrimSpace(reason),
	})
	if err != nil {
		return err
	}
	_, err = store.AppendEvent(ctx, event)
	return err
}

func recoverQueuedReportTurns(ctx context.Context, store *sqlite.Store, manager *session.Manager, sessions []types.Session, enqueuer reportTurnEnqueuer) error {
	if store == nil || manager == nil || enqueuer == nil {
		return nil
	}
	for _, sessionRow := range sessions {
		sessionID := strings.TrimSpace(sessionRow.ID)
		if sessionID == "" {
			continue
		}
		queuedCount, err := store.CountQueuedReportDeliveries(ctx, sessionID)
		if err != nil {
			return err
		}
		if queuedCount == 0 {
			continue
		}
		if hasRuntimeReportTurn(manager, sessionID) {
			continue
		}
		hasCreatedReportTurn, err := hasCreatedReportBatchTurn(ctx, store, sessionID)
		if err != nil {
			return err
		}
		if hasCreatedReportTurn {
			continue
		}
		if err := enqueuer.EnqueueSyntheticReportTurn(ctx, sessionID); err != nil {
			return err
		}
	}
	return nil
}

func hasRuntimeReportTurn(manager *session.Manager, sessionID string) bool {
	if manager == nil {
		return false
	}
	state, ok := manager.GetRuntimeState(sessionID)
	if !ok {
		return false
	}
	return state.ActiveTurnKind == types.TurnKindReportBatch || state.QueuedReportBatches > 0
}

func hasCreatedReportBatchTurn(ctx context.Context, store *sqlite.Store, sessionID string) (bool, error) {
	turns, err := store.ListTurnsBySession(ctx, sessionID)
	if err != nil {
		return false, err
	}
	now := time.Now().UTC()
	for _, turn := range turns {
		if turn.Kind != types.TurnKindReportBatch {
			continue
		}
		if !isTurnTerminal(turn.State) {
			return true, nil
		}
		if turn.State == types.TurnStateCompleted && now.Sub(turn.UpdatedAt) < reportBatchCooldown {
			return true, nil
		}
	}
	return false, nil
}
