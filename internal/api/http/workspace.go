package httpapi

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"

	"go-agent/internal/types"
	"go-agent/internal/workspace"
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

		meta, ok, err := workspace.Load(sessionRow.WorkspaceRoot)
		if err != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		if !ok {
			meta.Name = strings.TrimSpace(filepath.Base(sessionRow.WorkspaceRoot))
			if meta.Name == "" || meta.Name == "." || meta.Name == string(filepath.Separator) {
				meta.Name = "workspace"
			}
		}

		w.Header().Set("Content-Type", "application/json")
		permissionProfile := deps.Status.PermissionProfile
		if sessionRow.PermissionProfile != "" {
			permissionProfile = sessionRow.PermissionProfile
		}
		_ = json.NewEncoder(w).Encode(types.WorkspaceResponse{
			ID:                   meta.ID,
			Name:                 meta.Name,
			WorkspaceRoot:        sessionRow.WorkspaceRoot,
			Provider:             deps.Status.Provider,
			Model:                deps.Status.Model,
			PermissionProfile:    permissionProfile,
			ProviderCacheProfile: deps.Status.ProviderCacheProfile,
		})
	}
}
