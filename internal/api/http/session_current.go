package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	rolectx "go-agent/internal/roles"
	"go-agent/internal/sessionbinding"
	"go-agent/internal/sessionrole"
	"go-agent/internal/types"
	"go-agent/internal/workspace"
)

var errRoleServiceUnavailable = errors.New("role service is required")

func registerCurrentSessionRoutes(mux *http.ServeMux, deps Dependencies) {
	mux.HandleFunc("/v1/session/ensure", handleEnsureSession(deps))
	mux.HandleFunc("/v1/session/turns", handleCurrentSession(deps, handleSubmitTurn))
	mux.HandleFunc("/v1/session/interrupt", handleCurrentSession(deps, handleInterruptTurn))
	mux.HandleFunc("/v1/session/events", handleCurrentSession(deps, handleStreamEvents))
	mux.HandleFunc("/v1/session/timeline", handleCurrentSession(deps, handleGetTimeline))
	mux.HandleFunc("/v1/session/history", handleCurrentSession(deps, handleListContextHistory))
	mux.HandleFunc("/v1/session/history/load", handleCurrentSession(deps, handleLoadContextHistory))
	mux.HandleFunc("/v1/session/reopen", handleCurrentSession(deps, handleReopenContext))
}

func handleEnsureSession(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		r = r.WithContext(sessionbinding.WithContextBinding(r.Context(), r.Header.Get(sessionbinding.HeaderName)))

		var req types.EnsureSessionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		workspaceRoot := strings.TrimSpace(req.WorkspaceRoot)
		if workspaceRoot == "" {
			http.Error(w, "workspace_root is required", http.StatusBadRequest)
			return
		}
		r = r.WithContext(workspace.WithWorkspaceRoot(r.Context(), workspaceRoot))
		if deps.Store == nil || deps.Manager == nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}

		role, roleOK := resolveRequestedSessionRole(r, req.SessionRole)
		if !roleOK {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		r = r.WithContext(sessionrole.WithSessionRole(r.Context(), role))
		r = r.WithContext(rolectx.WithSpecialistRoleID(r.Context(), req.SpecialistRoleID))

		session, status, err := ensureSession(r.Context(), deps, workspaceRoot, role, req.SpecialistRoleID)
		if err != nil {
			if status == http.StatusBadRequest {
				http.Error(w, "bad request", http.StatusBadRequest)
				return
			}
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		if !deps.Manager.UpdateSession(session) {
			deps.Manager.RegisterSession(session)
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(session)
	}
}

type sessionScopedHandlerFactory func(Dependencies, string) http.HandlerFunc

func handleCurrentSession(deps Dependencies, next sessionScopedHandlerFactory) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(sessionbinding.WithContextBinding(r.Context(), r.Header.Get(sessionbinding.HeaderName)))
		if workspaceRoot := strings.TrimSpace(deps.WorkspaceRoot); workspaceRoot != "" {
			r = r.WithContext(workspace.WithWorkspaceRoot(r.Context(), workspaceRoot))
		}
		role, roleOK := resolveRequestedSessionRole(r, "")
		if !roleOK {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		r = r.WithContext(sessionrole.WithSessionRole(r.Context(), role))
		r = r.WithContext(rolectx.WithSpecialistRoleID(r.Context(), r.URL.Query().Get("specialist_role_id")))
		sessionID, ok := resolveCurrentSessionID(w, r, deps)
		if !ok {
			return
		}
		next(deps, sessionID)(w, r)
	}
}

func resolveRequestedSessionRole(r *http.Request, fallback string) (types.SessionRole, bool) {
	role := sessionrole.RequestRole(r, fallback)
	if role != "" {
		return role, true
	}
	headerRole := ""
	if r != nil {
		headerRole = strings.TrimSpace(r.Header.Get(sessionrole.HeaderName))
	}
	if headerRole != "" || strings.TrimSpace(fallback) != "" {
		return "", false
	}
	return types.SessionRoleMainParent, true
}

func resolveCurrentSessionID(w http.ResponseWriter, r *http.Request, deps Dependencies) (string, bool) {
	if deps.Store == nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return "", false
	}
	workspaceRoot := strings.TrimSpace(deps.WorkspaceRoot)
	if workspaceRoot == "" {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return "", false
	}
	role := sessionrole.FromContext(r.Context())
	specialistRoleID := rolectx.SpecialistRoleIDFromContext(r.Context())
	session, status, err := ensureSession(r.Context(), deps, workspaceRoot, role, specialistRoleID)
	if err != nil {
		if status == http.StatusBadRequest {
			http.Error(w, "bad request", http.StatusBadRequest)
			return "", false
		}
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return "", false
	}
	if deps.Manager != nil {
		if !deps.Manager.UpdateSession(session) {
			deps.Manager.RegisterSession(session)
		}
	}
	return session.ID, true
}

func ensureSession(ctx context.Context, deps Dependencies, workspaceRoot string, role types.SessionRole, specialistRoleID string) (types.Session, int, error) {
	workspaceRoot = strings.TrimSpace(workspaceRoot)
	specialistRoleID = strings.TrimSpace(specialistRoleID)

	if specialistRoleID == string(types.SessionRoleMainParent) {
		return types.Session{}, http.StatusBadRequest, errors.New("specialist role id cannot be main_parent")
	}

	if specialistRoleID == "" {
		session, _, _, err := deps.Store.EnsureRoleSession(ctx, workspaceRoot, role)
		return session, 0, err
	}

	if deps.RoleService == nil {
		return types.Session{}, http.StatusInternalServerError, errRoleServiceUnavailable
	}

	spec, err := deps.RoleService.Get(workspaceRoot, specialistRoleID)
	if err != nil {
		switch rolectx.KindOf(err) {
		case rolectx.ErrorKindInvalidInput, rolectx.ErrorKindNotFound:
			return types.Session{}, http.StatusBadRequest, err
		default:
			return types.Session{}, http.StatusInternalServerError, err
		}
	}

	specialistCtx := rolectx.WithSpecialistRoleID(
		sessionrole.WithSessionRole(ctx, types.SessionRoleMainParent),
		spec.RoleID,
	)
	session, _, _, err := deps.Store.EnsureSpecialistSession(
		specialistCtx,
		workspaceRoot,
		spec.RoleID,
		spec.Prompt,
		spec.SkillNames,
	)
	if err != nil {
		return types.Session{}, http.StatusInternalServerError, err
	}
	return session, 0, nil
}
