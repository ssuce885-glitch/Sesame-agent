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
