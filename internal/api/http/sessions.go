package httpapi

import (
	"encoding/json"
	"net/http"
	"time"

	"go-agent/internal/types"
)

func registerSessionRoutes(mux *http.ServeMux, deps Dependencies) {
	mux.HandleFunc("/v1/sessions", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req types.CreateSessionRequest
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

		now := time.Now().UTC()
		session := types.Session{
			ID:            types.NewID("sess"),
			WorkspaceRoot: req.WorkspaceRoot,
			State:         types.SessionStateIdle,
			CreatedAt:     now,
			UpdatedAt:     now,
		}

		if err := deps.Store.InsertSession(r.Context(), session); err != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		deps.Manager.RegisterSession(session)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(session)
	})
}
