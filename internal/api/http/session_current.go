package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"

	"go-agent/internal/sessionbinding"
	"go-agent/internal/types"
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
		if req.WorkspaceRoot == "" {
			http.Error(w, "workspace_root is required", http.StatusBadRequest)
			return
		}
		if deps.Store == nil || deps.Manager == nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}

		session, _, _, err := deps.Store.EnsureCanonicalSession(r.Context(), req.WorkspaceRoot)
		if err != nil {
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
		sessionID, ok := resolveCurrentSessionID(w, r, deps)
		if !ok {
			return
		}
		next(deps, sessionID)(w, r)
	}
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
	session, _, _, err := deps.Store.EnsureCanonicalSession(r.Context(), workspaceRoot)
	if err != nil {
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
