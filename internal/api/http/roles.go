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
				writeRoleError(w, err)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(struct {
				Roles       []roleSummaryResponse    `json:"roles"`
				Diagnostics []roleDiagnosticResponse `json:"diagnostics"`
			}{Roles: toRoleSummaryResponseList(catalog.Roles), Diagnostics: toRoleDiagnosticResponseList(catalog.Diagnostics)})
		case http.MethodPost:
			var input roles.UpsertInput
			if err := decodeStrictJSONBody(r, &input); err != nil {
				http.Error(w, "bad request", http.StatusBadRequest)
				return
			}
			spec, err := deps.RoleService.Create(deps.WorkspaceRoot, input)
			if err != nil {
				writeRoleError(w, err)
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
		rolePath := strings.TrimSpace(strings.TrimPrefix(r.URL.Path, "/v1/roles/"))
		if rolePath == "" {
			http.NotFound(w, r)
			return
		}
		if strings.HasSuffix(rolePath, "/versions") {
			roleID := strings.TrimSpace(strings.TrimSuffix(rolePath, "/versions"))
			roleID = strings.TrimSuffix(roleID, "/")
			if roleID == "" || strings.Contains(roleID, "/") {
				http.NotFound(w, r)
				return
			}
			switch r.Method {
			case http.MethodGet:
				versions, err := deps.RoleService.ListVersions(deps.WorkspaceRoot, roleID)
				if err != nil {
					writeRoleError(w, err)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(struct {
					Versions []roleResponse `json:"versions"`
				}{Versions: toRoleResponseList(versions)})
			default:
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			}
			return
		}
		roleID := rolePath
		if roleID == "" || strings.Contains(roleID, "/") {
			http.NotFound(w, r)
			return
		}
		switch r.Method {
		case http.MethodGet:
			spec, err := deps.RoleService.Get(deps.WorkspaceRoot, roleID)
			if err != nil {
				writeRoleError(w, err)
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
				writeRoleError(w, err)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(toRoleResponse(spec))
		case http.MethodDelete:
			if err := deps.RoleService.Delete(deps.WorkspaceRoot, roleID); err != nil {
				writeRoleError(w, err)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

type roleResponse struct {
	RoleID      string         `json:"role_id"`
	DisplayName string         `json:"display_name"`
	Description string         `json:"description"`
	Prompt      string         `json:"prompt"`
	SkillNames  []string       `json:"skills"`
	Policy      map[string]any `json:"policy"`
	Version     int            `json:"version"`
}

type roleSummaryResponse struct {
	RoleID      string         `json:"role_id"`
	DisplayName string         `json:"display_name"`
	Description string         `json:"description"`
	SkillNames  []string       `json:"skills"`
	Policy      map[string]any `json:"policy"`
	Version     int            `json:"version"`
}

type roleDiagnosticResponse struct {
	RoleID string `json:"role_id"`
	Path   string `json:"path"`
	Error  string `json:"error"`
}

func toRoleResponse(spec roles.Spec) roleResponse {
	return roleResponse{
		RoleID:      spec.RoleID,
		DisplayName: spec.DisplayName,
		Description: spec.Description,
		Prompt:      spec.Prompt,
		SkillNames:  normalizeSkillsForResponse(spec.SkillNames),
		Policy:      normalizePolicyForResponse(spec.Policy),
		Version:     spec.Version,
	}
}

func toRoleSummaryResponseList(specs []roles.Spec) []roleSummaryResponse {
	if len(specs) == 0 {
		return []roleSummaryResponse{}
	}
	out := make([]roleSummaryResponse, 0, len(specs))
	for _, spec := range specs {
		out = append(out, roleSummaryResponse{
			RoleID:      spec.RoleID,
			DisplayName: spec.DisplayName,
			Description: spec.Description,
			SkillNames:  normalizeSkillsForResponse(spec.SkillNames),
			Policy:      normalizePolicyForResponse(spec.Policy),
			Version:     spec.Version,
		})
	}
	return out
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

func toRoleDiagnosticResponseList(diagnostics []roles.Diagnostic) []roleDiagnosticResponse {
	if len(diagnostics) == 0 {
		return []roleDiagnosticResponse{}
	}
	out := make([]roleDiagnosticResponse, 0, len(diagnostics))
	for _, diagnostic := range diagnostics {
		out = append(out, roleDiagnosticResponse{
			RoleID: diagnostic.RoleID,
			Path:   diagnostic.Path,
			Error:  diagnostic.Error,
		})
	}
	return out
}

func normalizeSkillsForResponse(skills []string) []string {
	if skills == nil {
		return []string{}
	}
	return skills
}

func normalizePolicyForResponse(policy map[string]any) map[string]any {
	if len(policy) == 0 {
		return map[string]any{}
	}
	return policy
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

func writeRoleError(w http.ResponseWriter, err error) {
	switch {
	case roles.IsInvalidInput(err):
		http.Error(w, "bad request", http.StatusBadRequest)
	case roles.IsNotFound(err):
		http.Error(w, "not found", http.StatusNotFound)
	case roles.IsConflict(err):
		http.Error(w, "conflict", http.StatusConflict)
	default:
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}
}
