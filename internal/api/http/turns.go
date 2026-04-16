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

type turnInterruptStore interface {
	GetSession(context.Context, string) (types.Session, bool, error)
	TryMarkTurnInterrupted(context.Context, string, string) (bool, error)
}

type currentContextHeadStore interface {
	GetCurrentContextHeadID(context.Context) (string, bool, error)
}

func handleSubmitTurn(deps Dependencies, sessionID string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req types.SubmitTurnRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		if strings.TrimSpace(req.Message) == "" {
			http.Error(w, "message is required", http.StatusBadRequest)
			return
		}
		if deps.Store == nil || deps.Manager == nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		sess, found, err := deps.Store.GetSession(r.Context(), sessionID)
		if err != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		if !found {
			http.NotFound(w, r)
			return
		}
		if sess.State == types.SessionStateAwaitingPermission {
			http.Error(w, "session is awaiting permission; use /approve or /deny before sending another prompt", http.StatusConflict)
			return
		}

		now := time.Now().UTC()
		turn := types.Turn{
			ID:           types.NewID("turn"),
			SessionID:    sessionID,
			ClientTurnID: req.ClientTurnID,
			State:        types.TurnStateCreated,
			UserMessage:  req.Message,
			CreatedAt:    now,
			UpdatedAt:    now,
		}
		if headStore, ok := deps.Store.(currentContextHeadStore); ok {
			headID, hasHead, err := headStore.GetCurrentContextHeadID(r.Context())
			if err != nil {
				http.Error(w, "internal server error", http.StatusInternalServerError)
				return
			}
			if hasHead {
				turn.ContextHeadID = strings.TrimSpace(headID)
			}
		}

		if err := deps.Store.InsertTurn(r.Context(), turn); err != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}

		if _, err := deps.Manager.SubmitTurn(r.Context(), sessionID, session.SubmitTurnInput{
			TurnID:       turn.ID,
			ClientTurnID: turn.ClientTurnID,
			Message:      turn.UserMessage,
		}); err != nil {
			if delErr := deps.Store.DeleteTurn(context.WithoutCancel(r.Context()), turn.ID); delErr != nil {
				http.Error(w, "internal server error", http.StatusInternalServerError)
				return
			}
			http.NotFound(w, r)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(turn)
	}
}

func handleInterruptTurn(deps Dependencies, sessionID string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		store, ok := deps.Store.(turnInterruptStore)
		if !ok {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}

		sess, found, err := store.GetSession(r.Context(), sessionID)
		if err != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		if !found {
			http.NotFound(w, r)
			return
		}

		turnID := strings.TrimSpace(sess.ActiveTurnID)
		if turnID == "" {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		if interrupter, ok := deps.Manager.(interface {
			InterruptTurn(string, string) bool
		}); ok {
			if !interrupter.InterruptTurn(sessionID, turnID) {
				w.WriteHeader(http.StatusNoContent)
				return
			}
		}

		bg := context.WithoutCancel(r.Context())
		interrupted, err := store.TryMarkTurnInterrupted(bg, sessionID, turnID)
		if err != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		if !interrupted {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if err := appendRuntimeTimelineEvent(bg, deps, sessionID, turnID, types.EventTurnInterrupted, map[string]string{
			"reason": "user_cancelled",
		}); err != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusAccepted)
	}
}
