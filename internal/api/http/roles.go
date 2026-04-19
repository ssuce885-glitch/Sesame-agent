package httpapi

import (
	"encoding/json"
	"io"
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
				writeRoleServiceError(w, err)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(struct {
				Roles []roleResponse `json:"roles"`
			}{Roles: toRoleResponseList(catalog.Roles)})
		case http.MethodPost:
			var input roles.UpsertInput
			if err := decodeStrictJSONBody(r, &input); err != nil {
				http.Error(w, "bad request", http.StatusBadRequest)
				return
			}
			spec, err := deps.RoleService.Create(deps.WorkspaceRoot, input)
			if err != nil {
				writeRoleServiceError(w, err)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(toRoleResponse(spec))
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
				writeRoleServiceError(w, err)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(toRoleResponse(spec))
		case http.MethodPut:
			var input roles.UpsertInput
			if err := decodeStrictJSONBody(r, &input); err != nil {
				http.Error(w, "bad request", http.StatusBadRequest)
				return
			}
			input.RoleID = roleID
			spec, err := deps.RoleService.Update(deps.WorkspaceRoot, input)
			if err != nil {
				writeRoleServiceError(w, err)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(toRoleResponse(spec))
		case http.MethodDelete:
			if err := deps.RoleService.Delete(deps.WorkspaceRoot, roleID); err != nil {
				writeRoleServiceError(w, err)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

type roleResponse struct {
	RoleID      string   `json:"role_id"`
	DisplayName string   `json:"display_name"`
	Description string   `json:"description"`
	Prompt      string   `json:"prompt"`
	SkillNames  []string `json:"skills"`
}

func toRoleResponse(spec roles.Spec) roleResponse {
	return roleResponse{
		RoleID:      spec.RoleID,
		DisplayName: spec.DisplayName,
		Description: spec.Description,
		Prompt:      spec.Prompt,
		SkillNames:  spec.SkillNames,
	}
}

func toRoleResponseList(specs []roles.Spec) []roleResponse {
	if len(specs) == 0 {
		return []roleResponse{}
	}
	out := make([]roleResponse, 0, len(specs))
	for _, spec := range specs {
		out = append(out, toRoleResponse(spec))
	}
	return out
}

func decodeStrictJSONBody(r *http.Request, dst any) error {
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return io.ErrUnexpectedEOF
	}
	return nil
}

func writeRoleServiceError(w http.ResponseWriter, err error) {
	switch roles.KindOf(err) {
	case roles.ErrorKindInvalidInput:
		http.Error(w, "bad request", http.StatusBadRequest)
	case roles.ErrorKindNotFound:
		http.Error(w, "not found", http.StatusNotFound)
	case roles.ErrorKindConflict:
		http.Error(w, "conflict", http.StatusConflict)
	default:
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}
}
