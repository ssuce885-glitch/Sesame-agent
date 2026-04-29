package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"go-agent/internal/sessionbinding"
	"go-agent/internal/sessionrole"
	"go-agent/internal/types"
	"go-agent/internal/workspace"
)

func registerCurrentSessionRoutes(mux *http.ServeMux, deps Dependencies) {
	mux.HandleFunc("/v1/session/ensure", handleEnsureSession(deps))
	mux.HandleFunc("/v1/session/turns", handleCurrentSession(deps, handleSubmitTurn))
	mux.HandleFunc("/v1/session/interrupt", handleCurrentSession(deps, handleInterruptTurn))
	mux.HandleFunc("/v1/session/events", handleCurrentSession(deps, handleStreamEvents))
	mux.HandleFunc("/v1/session/timeline", handleCurrentSession(deps, handleGetTimeline))
	mux.HandleFunc("/v1/session/history", handleCurrentSession(deps, handleListContextHistory))
	mux.HandleFunc("/v1/session/history/load", handleCurrentSession(deps, handleLoadContextHistory))
	mux.HandleFunc("/v1/session/reopen", handleCurrentSession(deps, handleReopenContext))
	mux.HandleFunc("/v1/session/checkpoints", handleCurrentSession(deps, handleListFileCheckpoints))
	mux.HandleFunc("/v1/session/checkpoints/", handleCurrentSession(deps, handleFileCheckpointAction))
}

func handleEnsureSession(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		r = r.WithContext(sessionbinding.WithContextBinding(r.Context(), resolveRequestBinding(r)))

		req, err := decodeEnsureSessionRequest(r)
		if err != nil {
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

		session, status, err := ensureSession(r.Context(), deps, workspaceRoot, role)
		if err != nil {
			switch status {
			case http.StatusBadRequest:
				http.Error(w, "bad request", http.StatusBadRequest)
			case http.StatusConflict:
				http.Error(w, "conflict", http.StatusConflict)
			default:
				http.Error(w, "internal server error", http.StatusInternalServerError)
			}
			return
		}
		if !deps.Manager.UpdateSession(session) {
			deps.Manager.RegisterSession(session)
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(session)
	}
}

func decodeEnsureSessionRequest(r *http.Request) (types.EnsureSessionRequest, error) {
	var raw map[string]json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		return types.EnsureSessionRequest{}, err
	}
	if _, ok := raw["specialist_role_id"]; ok {
		return types.EnsureSessionRequest{}, errors.New("specialist_role_id is not supported")
	}
	payload, err := json.Marshal(raw)
	if err != nil {
		return types.EnsureSessionRequest{}, err
	}
	var req types.EnsureSessionRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		return types.EnsureSessionRequest{}, err
	}
	return req, nil
}

type sessionScopedHandlerFactory func(Dependencies, string) http.HandlerFunc

func handleCurrentSession(deps Dependencies, next sessionScopedHandlerFactory) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(sessionbinding.WithContextBinding(r.Context(), resolveRequestBinding(r)))
		if workspaceRoot := strings.TrimSpace(deps.WorkspaceRoot); workspaceRoot != "" {
			r = r.WithContext(workspace.WithWorkspaceRoot(r.Context(), workspaceRoot))
		}
		if strings.TrimSpace(r.URL.Query().Get("specialist_role_id")) != "" {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		role, roleOK := resolveRequestedSessionRole(r, "")
		if !roleOK {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		r = r.WithContext(sessionrole.WithSessionRole(r.Context(), role))
		sessionID, ok := resolveCurrentSessionID(w, r, deps)
		if !ok {
			return
		}
		next(deps, sessionID)(w, r)
	}
}

func resolveRequestBinding(r *http.Request) string {
	if r == nil {
		return ""
	}
	if binding := strings.TrimSpace(r.URL.Query().Get("binding")); binding != "" {
		return binding
	}
	return r.Header.Get(sessionbinding.HeaderName)
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
	session, status, err := ensureSession(r.Context(), deps, workspaceRoot, role)
	if err != nil {
		switch status {
		case http.StatusBadRequest:
			http.Error(w, "bad request", http.StatusBadRequest)
		case http.StatusConflict:
			http.Error(w, "conflict", http.StatusConflict)
		default:
			http.Error(w, "internal server error", http.StatusInternalServerError)
		}
		return "", false
	}
	if deps.Manager != nil {
		if !deps.Manager.UpdateSession(session) {
			deps.Manager.RegisterSession(session)
		}
	}
	return session.ID, true
}

func ensureSession(ctx context.Context, deps Dependencies, workspaceRoot string, role types.SessionRole) (types.Session, int, error) {
	workspaceRoot = strings.TrimSpace(workspaceRoot)
	session, _, _, err := deps.Store.EnsureRoleSession(ctx, workspaceRoot, role)
	if err != nil {
		return types.Session{}, http.StatusInternalServerError, err
	}
	return session, 0, nil
}
