package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"

	"go-agent/internal/roles"
)

func registerRoleRoutes(mux *http.ServeMux, deps Dependencies) {
	mux.HandleFunc("/v1/roles", handleRoles(deps))
	mux.HandleFunc("/v1/roles/", handleRoleByID(deps))
}

func handleRoles(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if deps.RoleService == nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		switch r.Method {
		case http.MethodGet:
			catalog, err := deps.RoleService.List(deps.WorkspaceRoot)
			if err != nil {
				http.Error(w, "internal server error", http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(struct {
				Roles []roles.Spec `json:"roles"`
			}{Roles: catalog.Roles})
		case http.MethodPost:
			var input roles.UpsertInput
			if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
				http.Error(w, "bad request", http.StatusBadRequest)
				return
			}
			spec, err := deps.RoleService.Create(deps.WorkspaceRoot, input)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(spec)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

func handleRoleByID(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if deps.RoleService == nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		roleID := strings.TrimSpace(strings.TrimPrefix(r.URL.Path, "/v1/roles/"))
		if roleID == "" {
			http.NotFound(w, r)
			return
		}
		switch r.Method {
		case http.MethodGet:
			spec, err := deps.RoleService.Get(deps.WorkspaceRoot, roleID)
			if err != nil {
				http.Error(w, "not found", http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(spec)
		case http.MethodPut:
			var input roles.UpsertInput
			if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
				http.Error(w, "bad request", http.StatusBadRequest)
				return
			}
			input.RoleID = roleID
			spec, err := deps.RoleService.Update(deps.WorkspaceRoot, input)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(spec)
		case http.MethodDelete:
			if err := deps.RoleService.Delete(deps.WorkspaceRoot, roleID); err != nil {
				http.Error(w, "not found", http.StatusNotFound)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}
}
