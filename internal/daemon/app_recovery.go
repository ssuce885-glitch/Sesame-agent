package daemon

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	rolectx "go-agent/internal/roles"
	"go-agent/internal/session"
	"go-agent/internal/sessionrole"
	"go-agent/internal/store/sqlite"
	"go-agent/internal/types"
	"go-agent/internal/workspace"
)

func recoverRuntimeState(ctx context.Context, store *sqlite.Store, manager *session.Manager) error {
	sessions, err := store.ListSessions(ctx)
	if err != nil {
		return err
	}
	if manager != nil {
		for _, sessionRow := range sessions {
			manager.RegisterSession(sessionRow)
		}
	}
	resumedTurns, err := resumeResolvedContinuations(ctx, store, manager)
	if err != nil {
		return err
	}

	running, err := store.ListRunningTurns(ctx)
	if err != nil {
		return err
	}

	for _, turn := range running {
		if _, ok := resumedTurns[turn.ID]; ok {
			continue
		}
		if turn.State == types.TurnStateAwaitingPermission {
			continue
		}
		if turn.Kind == types.TurnKindChildReportBatch {
			if err := store.RequeueClaimedChildReportsForTurn(ctx, turn.ID); err != nil {
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

	return recoverQueuedCreatedTurns(ctx, store, manager, sessions)
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

func resumeResolvedContinuations(ctx context.Context, store *sqlite.Store, manager *session.Manager) (map[string]struct{}, error) {
	resumed := make(map[string]struct{})
	if store == nil || manager == nil {
		return resumed, nil
	}

	continuations, err := store.ListPendingTurnContinuations(ctx)
	if err != nil {
		return nil, err
	}

	for _, continuation := range continuations {
		if strings.TrimSpace(continuation.PermissionRequestID) == "" {
			continue
		}
		request, ok, err := store.GetPermissionRequest(ctx, continuation.PermissionRequestID)
		if err != nil {
			return nil, err
		}
		if !ok || request.Status == types.PermissionRequestStatusRequested || strings.TrimSpace(request.Decision) == "" {
			continue
		}

		turn, ok, err := store.GetTurn(ctx, continuation.TurnID)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		sessionRow, ok, err := store.GetSession(ctx, continuation.SessionID)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}

		effectiveProfile := sessionRow.PermissionProfile
		if types.PermissionDecisionGrantsProfile(request.Decision) && strings.TrimSpace(request.RequestedProfile) != "" {
			effectiveProfile = request.RequestedProfile
		}
		decisionScope := strings.TrimSpace(request.DecisionScope)
		if decisionScope == "" {
			decisionScope = request.Decision
		}
		resume := &types.TurnResume{
			ContinuationID:             continuation.ID,
			PermissionRequestID:        request.ID,
			ToolRunID:                  continuation.ToolRunID,
			ToolCallID:                 continuation.ToolCallID,
			ToolName:                   continuation.ToolName,
			RequestedProfile:           continuation.RequestedProfile,
			Reason:                     continuation.Reason,
			Decision:                   request.Decision,
			DecisionScope:              decisionScope,
			EffectivePermissionProfile: effectiveProfile,
			RunID:                      continuation.RunID,
			TaskID:                     continuation.TaskID,
		}
		specialistRoleID, err := store.ResolveSpecialistRoleID(ctx, sessionRow.ID, sessionRow.WorkspaceRoot)
		if err != nil {
			return nil, err
		}
		resumeCtx := workspace.WithWorkspaceRoot(
			rolectx.WithSpecialistRoleID(ctx, specialistRoleID),
			sessionRow.WorkspaceRoot,
		)
		if _, err := manager.ResumeTurn(resumeCtx, sessionRow.ID, session.ResumeTurnInput{
			Turn:   turn,
			Resume: resume,
		}); err != nil {
			return nil, err
		}

		now := time.Now().UTC()
		continuation.State = types.TurnContinuationStateResumed
		continuation.Decision = request.Decision
		continuation.DecisionScope = decisionScope
		continuation.UpdatedAt = now
		var resumedToolRun *types.ToolRun
		if strings.TrimSpace(continuation.ToolRunID) != "" {
			toolRun, found, err := store.GetToolRun(ctx, continuation.ToolRunID)
			if err != nil {
				return nil, err
			}
			if found {
				toolRun.PermissionRequestID = request.ID
				toolRun.UpdatedAt = now
				toolRun.CompletedAt = now
				toolRun.OutputJSON = marshalRecoveredPermissionToolRunOutput(request, effectiveProfile)
				if request.Decision == types.PermissionDecisionDeny {
					toolRun.State = types.ToolRunStateFailed
					toolRun.Error = "permission denied"
				} else {
					toolRun.State = types.ToolRunStateCompleted
					toolRun.Error = ""
				}
				resumedToolRun = &toolRun
			}
		}
		if err := store.CommitPermissionResume(ctx, sessionRow.ID, turn.ID, continuation, resumedToolRun); err != nil {
			manager.InterruptTurn(sessionRow.ID, turn.ID)
			return nil, err
		}

		resumed[turn.ID] = struct{}{}
	}

	return resumed, nil
}

func marshalRecoveredPermissionToolRunOutput(request types.PermissionRequest, effectiveProfile string) string {
	payload, _ := json.Marshal(map[string]any{
		"status":                       request.Status,
		"decision":                     request.Decision,
		"decision_scope":               request.DecisionScope,
		"requested_profile":            request.RequestedProfile,
		"effective_permission_profile": effectiveProfile,
		"reason":                       request.Reason,
	})
	return string(payload)
}
