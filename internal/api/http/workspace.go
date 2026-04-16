package httpapi

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"

	"go-agent/internal/types"
	"go-agent/internal/workspace"
)

func registerWorkspaceRoutes(mux *http.ServeMux, deps Dependencies) {
	mux.HandleFunc("/v1/workspace", handleGetWorkspace(deps))
}

func handleGetWorkspace(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		root := strings.TrimSpace(deps.WorkspaceRoot)
		if root == "" {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}

		meta, ok, err := workspace.Load(root)
		if err != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		if !ok {
			meta.Name = strings.TrimSpace(filepath.Base(root))
			if meta.Name == "" || meta.Name == "." || meta.Name == string(filepath.Separator) {
				meta.Name = "workspace"
			}
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(types.WorkspaceResponse{
			ID:                   meta.ID,
			Name:                 meta.Name,
			WorkspaceRoot:        root,
			Provider:             deps.Status.Provider,
			Model:                deps.Status.Model,
			PermissionProfile:    deps.Status.PermissionProfile,
			ProviderCacheProfile: deps.Status.ProviderCacheProfile,
		})
	}
}
