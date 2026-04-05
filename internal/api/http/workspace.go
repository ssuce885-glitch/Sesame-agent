package httpapi

import (
	"encoding/json"
	"net/http"

	"go-agent/internal/types"
)

func handleGetWorkspace(deps Dependencies, sessionID string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
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

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(types.SessionWorkspaceResponse{
			SessionID:            sessionRow.ID,
			WorkspaceRoot:        sessionRow.WorkspaceRoot,
			Provider:             deps.Status.Provider,
			Model:                deps.Status.Model,
			PermissionProfile:    deps.Status.PermissionProfile,
			ProviderCacheProfile: deps.Status.ProviderCacheProfile,
		})
	}
}
