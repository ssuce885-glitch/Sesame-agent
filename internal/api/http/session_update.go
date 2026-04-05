package httpapi

import (
	"encoding/json"
	"net/http"

	"go-agent/internal/types"
)

func handlePatchSession(deps Dependencies, sessionID string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if deps.Store == nil || deps.Manager == nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}

		var req types.PatchSessionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		if req.SystemPrompt == nil {
			http.Error(w, "system_prompt is required", http.StatusBadRequest)
			return
		}

		session, ok, err := deps.Store.UpdateSessionSystemPrompt(r.Context(), sessionID, *req.SystemPrompt)
		if err != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		if !ok {
			http.NotFound(w, r)
			return
		}
		deps.Manager.UpdateSession(session)

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(session)
	}
}
