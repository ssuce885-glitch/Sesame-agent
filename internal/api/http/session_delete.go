package httpapi

import (
	"encoding/json"
	"net/http"

	"go-agent/internal/types"
)

func handleDeleteSession(deps Dependencies, sessionID string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if deps.Store == nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}

		sessionRow, ok, err := deps.Store.GetSession(r.Context(), sessionID)
		if err != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		if !ok {
			http.NotFound(w, r)
			return
		}
		if sessionRow.State != types.SessionStateIdle {
			http.Error(w, "session is not idle", http.StatusConflict)
			return
		}

		nextSelected, deleted, err := deps.Store.DeleteSession(r.Context(), sessionID)
		if err != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		if !deleted {
			http.NotFound(w, r)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(types.DeleteSessionResponse{
			DeletedSessionID:  sessionID,
			SelectedSessionID: nextSelected,
		})
	}
}
