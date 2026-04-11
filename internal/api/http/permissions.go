package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"go-agent/internal/session"
	"go-agent/internal/types"
)

type permissionStore interface {
	GetPermissionRequest(context.Context, string) (types.PermissionRequest, bool, error)
	GetTurnContinuationByPermissionRequest(context.Context, string) (types.TurnContinuation, bool, error)
	GetTurn(context.Context, string) (types.Turn, bool, error)
	GetSession(context.Context, string) (types.Session, bool, error)
	GetToolRun(context.Context, string) (types.ToolRun, bool, error)
	UpsertPermissionRequest(context.Context, types.PermissionRequest) error
	UpsertTurnContinuation(context.Context, types.TurnContinuation) error
	UpsertToolRun(context.Context, types.ToolRun) error
	UpdateSessionPermissionProfile(context.Context, string, string) (types.Session, bool, error)
	UpdateTurnState(context.Context, string, types.TurnState) error
	UpdateSessionState(context.Context, string, types.SessionState, string) error
}

type eventPublisher interface {
	Publish(types.Event)
}

func registerPermissionRoutes(mux *http.ServeMux, deps Dependencies) {
	mux.HandleFunc("/v1/permissions/decide", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		handlePermissionDecision(deps)(w, r)
	})
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

		if err := store.UpdateTurnState(r.Context(), turn.ID, types.TurnStateLoopContinue); err != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		if err := store.UpdateSessionState(r.Context(), sessionRow.ID, types.SessionStateRunning, turn.ID); err != nil {
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
			ActivatedSkillNames:        append([]string(nil), continuation.ActivatedSkillNames...),
		}
		if _, err := deps.Manager.ResumeTurn(r.Context(), sessionRow.ID, session.ResumeTurnInput{
			TurnID:  turn.ID,
			Message: turn.UserMessage,
			Resume:  resume,
		}); err != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}

		continuation.State = types.TurnContinuationStateResumed
		continuation.Decision = req.Decision
		continuation.DecisionScope = req.Decision
		continuation.UpdatedAt = now
		if err := store.UpsertTurnContinuation(r.Context(), continuation); err != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}

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
		if err := store.UpsertToolRun(r.Context(), toolRun); err != nil {
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
