package httpapi

import (
	"encoding/json"
	"net/http"
	"time"

	"go-agent/internal/types"
)

func registerSessionRoutes(mux *http.ServeMux, deps Dependencies) {
	mux.HandleFunc("/v1/sessions", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			handleListSessions(deps)(w, r)
		case http.MethodPost:
			handleCreateSession(deps)(w, r)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})
}

func handleListSessions(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if deps.Store == nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}

		sessions, err := deps.Store.ListSessions(r.Context())
		if err != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		selectedSessionID, ok, err := deps.Store.GetSelectedSessionID(r.Context())
		if err != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		if !ok {
			selectedSessionID = ""
		}

		items := make([]types.SessionListItem, 0, len(sessions))
		for _, session := range sessions {
			items = append(items, types.SessionListItem{
				ID:            session.ID,
				WorkspaceRoot: session.WorkspaceRoot,
				State:         session.State,
				ActiveTurnID:  session.ActiveTurnID,
				CreatedAt:     session.CreatedAt,
				UpdatedAt:     session.UpdatedAt,
				IsSelected:    session.ID == selectedSessionID,
			})
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(types.ListSessionsResponse{
			Sessions:          items,
			SelectedSessionID: selectedSessionID,
		})
	}
}

func handleCreateSession(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
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
	}
}
