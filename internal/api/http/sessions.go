package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
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
			title, lastPreview, err := deriveSessionText(r.Context(), deps, session.ID)
			if err != nil {
				http.Error(w, "internal server error", http.StatusInternalServerError)
				return
			}
			items = append(items, types.SessionListItem{
				ID:            session.ID,
				Title:         title,
				LastPreview:   lastPreview,
				WorkspaceRoot: session.WorkspaceRoot,
				SystemPrompt:  session.SystemPrompt,
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

func deriveSessionText(ctx context.Context, deps Dependencies, sessionID string) (string, string, error) {
	if deps.Store == nil {
		return "", "", nil
	}

	turns, err := deps.Store.ListTurnsBySession(ctx, sessionID)
	if err != nil {
		return "", "", err
	}

	title := ""
	lastPreview := ""
	for _, turn := range turns {
		text := clampPreview(turn.UserMessage)
		if text == "" {
			continue
		}
		if title == "" {
			title = text
		}
		lastPreview = text
	}

	return title, lastPreview, nil
}

func clampPreview(text string) string {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return ""
	}
	const maxLen = 120
	runes := []rune(trimmed)
	if len(runes) <= maxLen {
		return trimmed
	}
	return string(runes[:maxLen]) + "..."
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
			SystemPrompt:  req.SystemPrompt,
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
