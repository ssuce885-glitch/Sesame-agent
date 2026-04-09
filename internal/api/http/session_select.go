package httpapi

import (
	"encoding/json"
	"net/http"

	"go-agent/internal/types"
)

func handleSelectSession(deps Dependencies, sessionID string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if deps.Store == nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}

		sessions, err := deps.Store.ListSessions(r.Context())
		if err != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}

		found := false
		for _, session := range sessions {
			if session.ID == sessionID {
				found = true
				break
			}
		}
		if !found {
			http.NotFound(w, r)
			return
		}

		if err := deps.Store.SetSelectedSessionID(r.Context(), sessionID); err != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(types.SelectSessionResponse{
			SelectedSessionID: sessionID,
		})
	}
}
