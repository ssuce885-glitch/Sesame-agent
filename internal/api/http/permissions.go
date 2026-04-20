package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"go-agent/internal/automation"
	rolectx "go-agent/internal/roles"
	"go-agent/internal/session"
	"go-agent/internal/types"
)

type permissionStore interface {
	GetPermissionRequest(context.Context, string) (types.PermissionRequest, bool, error)
	GetTurnContinuationByPermissionRequest(context.Context, string) (types.TurnContinuation, bool, error)
	GetTurn(context.Context, string) (types.Turn, bool, error)
	GetSession(context.Context, string) (types.Session, bool, error)
	ResolveSpecialistRoleID(context.Context, string, string) (string, error)
	GetToolRun(context.Context, string) (types.ToolRun, bool, error)
	ListDispatchAttempts(context.Context, types.DispatchAttemptFilter) ([]types.DispatchAttempt, error)
	ListPendingAutomationPermissions(context.Context, string) ([]types.PendingAutomationPermission, error)
	FindDispatchAttemptByTaskID(context.Context, string) (types.DispatchAttempt, bool, error)
	GetAutomationWatcher(context.Context, string) (types.AutomationWatcherRuntime, bool, error)
	ListAutomationWatcherHolds(context.Context, string) ([]types.AutomationWatcherHold, error)
	UpsertPermissionRequest(context.Context, types.PermissionRequest) error
	UpsertDispatchAttempt(context.Context, types.DispatchAttempt) error
	UpsertTurnContinuation(context.Context, types.TurnContinuation) error
	UpsertToolRun(context.Context, types.ToolRun) error
	ReplaceAutomationWatcherHolds(context.Context, string, string, []types.AutomationWatcherHold) error
	CommitPermissionResume(context.Context, string, string, types.TurnContinuation, *types.ToolRun) error
	UpdateSessionPermissionProfile(context.Context, string, string) (types.Session, bool, error)
	UpdateTurnState(context.Context, string, types.TurnState) error
	UpdateSessionState(context.Context, string, types.SessionState, string) error
}

type eventPublisher interface {
	Publish(types.Event)
}

func registerPermissionRoutes(mux *http.ServeMux, deps Dependencies) {
	mux.HandleFunc("GET /v1/permissions/pending", handleListPendingPermissions(deps))
	mux.HandleFunc("GET /v1/permissions/pending/{request_id}", handleGetPendingPermission(deps))
	mux.HandleFunc("POST /v1/permissions/decide", handlePermissionDecision(deps))
}

func handleListPendingPermissions(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		store, ok := deps.Store.(permissionStore)
		if !ok {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}

		workspaceRoot := strings.TrimSpace(deps.WorkspaceRoot)
		if workspaceRoot == "" {
			workspaceRoot = strings.TrimSpace(r.URL.Query().Get("workspace_root"))
		}
		if workspaceRoot == "" {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}

		items, err := store.ListPendingAutomationPermissions(r.Context(), workspaceRoot)
		if err != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(types.ListPendingAutomationPermissionsResponse{
			Pending: items,
		})
	}
}

func handleGetPendingPermission(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		store, ok := deps.Store.(permissionStore)
		if !ok {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}

		requestID := strings.TrimSpace(r.PathValue("request_id"))
		if requestID == "" || strings.Contains(requestID, "/") {
			http.NotFound(w, r)
			return
		}

		item, ok, err := findPendingAutomationPermission(r.Context(), store, requestID)
		if err != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(item)
	}
}

func findPendingAutomationPermission(ctx context.Context, store permissionStore, requestID string) (types.PendingAutomationPermission, bool, error) {
	requestID = strings.TrimSpace(requestID)
	if requestID == "" {
		return types.PendingAutomationPermission{}, false, nil
	}
	request, found, err := store.GetPermissionRequest(ctx, requestID)
	if err != nil {
		return types.PendingAutomationPermission{}, false, err
	}
	if !found || request.Status != types.PermissionRequestStatusRequested {
		return types.PendingAutomationPermission{}, false, nil
	}
	continuation, found, err := store.GetTurnContinuationByPermissionRequest(ctx, requestID)
	if err != nil {
		return types.PendingAutomationPermission{}, false, err
	}
	if !found || continuation.State != types.TurnContinuationStatePending {
		return types.PendingAutomationPermission{}, false, nil
	}
	attempts, err := store.ListDispatchAttempts(ctx, types.DispatchAttemptFilter{
		Status: types.DispatchAttemptStatusAwaitingApproval,
	})
	if err != nil {
		return types.PendingAutomationPermission{}, false, err
	}
	for _, attempt := range attempts {
		if strings.TrimSpace(attempt.PermissionRequestID) != requestID {
			continue
		}
		return types.PendingAutomationPermission{
			RequestID:          requestID,
			WorkspaceRoot:      attempt.WorkspaceRoot,
			AutomationID:       attempt.AutomationID,
			IncidentID:         attempt.IncidentID,
			DispatchID:         attempt.DispatchID,
			PreferredSessionID: attempt.PreferredSessionID,
		}, true, nil
	}
	return types.PendingAutomationPermission{}, false, nil
}

func handlePermissionDecision(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		store, ok := deps.Store.(permissionStore)
		if !ok || deps.Manager == nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}

		var req types.PermissionDecisionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		req.RequestID = strings.TrimSpace(req.RequestID)
		req.Decision = strings.TrimSpace(req.Decision)
		if req.RequestID == "" || !types.IsValidPermissionDecision(req.Decision) {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		permissionRequest, found, err := store.GetPermissionRequest(r.Context(), req.RequestID)
		if err != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		if !found {
			http.NotFound(w, r)
			return
		}

		continuation, found, err := store.GetTurnContinuationByPermissionRequest(r.Context(), req.RequestID)
		if err != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		if !found {
			http.NotFound(w, r)
			return
		}

		turn, found, err := store.GetTurn(r.Context(), continuation.TurnID)
		if err != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		if !found {
			http.NotFound(w, r)
			return
		}

		sessionRow, found, err := store.GetSession(r.Context(), continuation.SessionID)
		if err != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		if !found {
			http.NotFound(w, r)
			return
		}

		toolRun, toolRunFound, err := store.GetToolRun(r.Context(), continuation.ToolRunID)
		if err != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		if !toolRunFound {
			toolRun = types.ToolRun{
				ID:         continuation.ToolRunID,
				RunID:      continuation.RunID,
				TaskID:     continuation.TaskID,
				ToolCallID: continuation.ToolCallID,
				ToolName:   continuation.ToolName,
			}
		}

		now := time.Now().UTC()
		permissionRequest.Decision = req.Decision
		permissionRequest.DecisionScope = req.Decision
		permissionRequest.Status = types.PermissionRequestStatusApproved
		if req.Decision == types.PermissionDecisionDeny {
			permissionRequest.Status = types.PermissionRequestStatusDenied
		}
		permissionRequest.ResolvedAt = now
		permissionRequest.UpdatedAt = now
		if err := store.UpsertPermissionRequest(r.Context(), permissionRequest); err != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}

		effectiveProfile := strings.TrimSpace(sessionRow.PermissionProfile)
		if types.PermissionDecisionGrantsProfile(req.Decision) {
			effectiveProfile = permissionRequest.RequestedProfile
			if req.Decision == types.PermissionDecisionAllowSession {
				updatedSession, ok, err := store.UpdateSessionPermissionProfile(r.Context(), sessionRow.ID, effectiveProfile)
				if err != nil {
					http.Error(w, "internal server error", http.StatusInternalServerError)
					return
				}
				if ok {
					sessionRow = updatedSession
					_ = deps.Manager.UpdateSession(updatedSession)
				}
			}
		}

		if err := appendPermissionResolvedEvent(r.Context(), deps, permissionRequest, continuation, effectiveProfile); err != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}

		resume := &types.TurnResume{
			ContinuationID:             continuation.ID,
			PermissionRequestID:        permissionRequest.ID,
			ToolRunID:                  continuation.ToolRunID,
			ToolCallID:                 continuation.ToolCallID,
			ToolName:                   continuation.ToolName,
			RequestedProfile:           continuation.RequestedProfile,
			Reason:                     continuation.Reason,
			Decision:                   req.Decision,
			DecisionScope:              req.Decision,
			EffectivePermissionProfile: effectiveProfile,
			RunID:                      continuation.RunID,
			TaskID:                     continuation.TaskID,
		}
		specialistRoleID, err := store.ResolveSpecialistRoleID(r.Context(), sessionRow.ID, sessionRow.WorkspaceRoot)
		if err != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		resumeCtx := rolectx.WithSpecialistRoleID(r.Context(), specialistRoleID)
		if _, err := deps.Manager.ResumeTurn(resumeCtx, sessionRow.ID, session.ResumeTurnInput{
			Turn:   turn,
			Resume: resume,
		}); err != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}

		continuation.State = types.TurnContinuationStateResumed
		continuation.Decision = req.Decision
		continuation.DecisionScope = req.Decision
		continuation.UpdatedAt = now

		toolRun.PermissionRequestID = permissionRequest.ID
		toolRun.UpdatedAt = now
		toolRun.CompletedAt = now
		toolRun.OutputJSON = marshalPermissionToolRunOutput(permissionRequest, effectiveProfile)
		if req.Decision == types.PermissionDecisionDeny {
			toolRun.State = types.ToolRunStateFailed
			toolRun.Error = "permission denied"
		} else {
			toolRun.State = types.ToolRunStateCompleted
			toolRun.Error = ""
		}
		if err := store.CommitPermissionResume(r.Context(), sessionRow.ID, turn.ID, continuation, &toolRun); err != nil {
			if interrupter, ok := deps.Manager.(interface {
				InterruptTurn(string, string) bool
			}); ok {
				interrupter.InterruptTurn(sessionRow.ID, turn.ID)
			}
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		if err := automation.RestoreDispatchAfterApprovalResume(r.Context(), store, continuation.TaskID, permissionRequest.ID, now); err != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		if err := appendRuntimeTimelineEvent(r.Context(), deps, turn.SessionID, turn.ID, types.EventToolRunUpdated, types.TimelineBlockFromToolRun(toolRun)); err != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(types.PermissionDecisionResponse{
			Request: permissionRequest,
			TurnID:  turn.ID,
			Resumed: true,
		})
	}
}

func appendPermissionResolvedEvent(ctx context.Context, deps Dependencies, request types.PermissionRequest, continuation types.TurnContinuation, effectiveProfile string) error {
	return appendRuntimeTimelineEvent(ctx, deps, request.SessionID, request.TurnID, types.EventPermissionResolved, types.PermissionResolvedPayload{
		RequestID:        request.ID,
		ToolRunID:        continuation.ToolRunID,
		ToolCallID:       continuation.ToolCallID,
		ToolName:         continuation.ToolName,
		RequestedProfile: request.RequestedProfile,
		Decision:         request.Decision,
		DecisionScope:    request.DecisionScope,
		EffectiveProfile: effectiveProfile,
		TurnID:           request.TurnID,
	})
}

func appendRuntimeTimelineEvent(ctx context.Context, deps Dependencies, sessionID, turnID, eventType string, payload any) error {
	appendStore, ok := deps.Store.(interface {
		AppendEvent(context.Context, types.Event) (int64, error)
	})
	if !ok {
		return nil
	}
	event, err := types.NewEvent(sessionID, turnID, eventType, payload)
	if err != nil {
		return err
	}
	seq, err := appendStore.AppendEvent(ctx, event)
	if err != nil {
		return err
	}
	event.Seq = seq
	if publisher, ok := deps.Bus.(eventPublisher); ok {
		publisher.Publish(event)
	}
	return nil
}

func marshalPermissionToolRunOutput(request types.PermissionRequest, effectiveProfile string) string {
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
