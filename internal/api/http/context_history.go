package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"

	"go-agent/internal/types"
)

type contextHistoryStore interface {
	ListContextHistory(context.Context, string) ([]types.HistoryEntry, string, error)
	CreateReopenContextHead(context.Context, string) (types.ContextHead, error)
	LoadContextHead(context.Context, string, string) (types.ContextHead, error)
}

func handleListContextHistory(deps Dependencies, sessionID string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		store, ok := deps.Store.(contextHistoryStore)
		if !ok {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		entries, currentHeadID, err := store.ListContextHistory(r.Context(), sessionID)
		if err != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		writeJSON(w, types.ListContextHistoryResponse{
			Entries:       entries,
			CurrentHeadID: currentHeadID,
		})
	}
}

func handleReopenContext(deps Dependencies, sessionID string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		store, ok := deps.Store.(contextHistoryStore)
		if !ok {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		head, err := store.CreateReopenContextHead(r.Context(), sessionID)
		if err != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		writeJSON(w, head)
	}
}

func handleLoadContextHistory(deps Dependencies, sessionID string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		store, ok := deps.Store.(contextHistoryStore)
		if !ok {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}

		var req types.LoadContextHistoryRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		req.HeadID = strings.TrimSpace(req.HeadID)
		if req.HeadID == "" {
			http.Error(w, "head_id is required", http.StatusBadRequest)
			return
		}

		head, err := store.LoadContextHead(r.Context(), sessionID, req.HeadID)
		if err != nil {
			if err == sql.ErrNoRows {
				http.NotFound(w, r)
				return
			}
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		writeJSON(w, head)
	}
}
