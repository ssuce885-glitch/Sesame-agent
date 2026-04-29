package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"go-agent/internal/types"
)

type fileCheckpointStore interface {
	GetFileCheckpoint(context.Context, string) (types.FileCheckpoint, bool, error)
	ListFileCheckpointsBySession(context.Context, string, int) ([]types.FileCheckpoint, error)
	GetLatestFileCheckpoint(context.Context, string) (types.FileCheckpoint, bool, error)
}

type fileCheckpointListResponse struct {
	Checkpoints []types.FileCheckpoint `json:"checkpoints"`
}

type fileCheckpointDiffResponse struct {
	Checkpoint types.FileCheckpoint  `json:"checkpoint"`
	Parent     *types.FileCheckpoint `json:"parent,omitempty"`
	Diff       string                `json:"diff"`
}

type fileCheckpointRollbackResponse struct {
	Status     string                `json:"status"`
	Checkpoint *types.FileCheckpoint `json:"checkpoint,omitempty"`
}

func handleListFileCheckpoints(deps Dependencies, sessionID string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		store, ok := deps.Store.(fileCheckpointStore)
		if deps.Store == nil || !ok {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		limit := parseCheckpointLimit(r.URL.Query().Get("limit"))
		checkpoints, err := store.ListFileCheckpointsBySession(r.Context(), sessionID, limit)
		if err != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(fileCheckpointListResponse{Checkpoints: checkpoints})
	}
}

func handleFileCheckpointAction(deps Dependencies, sessionID string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		checkpointID, action, ok := parseCheckpointActionPath(r.URL.Path)
		if !ok {
			http.NotFound(w, r)
			return
		}
		switch action {
		case "diff":
			handleGetFileCheckpointDiff(deps, sessionID, checkpointID)(w, r)
		case "rollback":
			handleRollbackFileCheckpoint(deps, sessionID, checkpointID)(w, r)
		default:
			http.NotFound(w, r)
		}
	}
}

func handleGetFileCheckpointDiff(deps Dependencies, sessionID, checkpointID string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		store, ok := deps.Store.(fileCheckpointStore)
		if deps.Store == nil || !ok || deps.FileCheckpoints == nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		checkpoint, found, err := getSessionFileCheckpoint(r.Context(), store, sessionID, checkpointID)
		if err != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		if !found {
			http.NotFound(w, r)
			return
		}
		diff, err := deps.FileCheckpoints.GetDiff("", checkpoint.ID)
		if err != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		var parent *types.FileCheckpoint
		if parentID := strings.TrimSpace(checkpoint.ParentCheckpointID); parentID != "" {
			if parentCheckpoint, parentFound, err := getSessionFileCheckpoint(r.Context(), store, sessionID, parentID); err != nil {
				http.Error(w, "internal server error", http.StatusInternalServerError)
				return
			} else if parentFound {
				parent = &parentCheckpoint
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(fileCheckpointDiffResponse{
			Checkpoint: checkpoint,
			Parent:     parent,
			Diff:       diff,
		})
	}
}

func handleRollbackFileCheckpoint(deps Dependencies, sessionID, checkpointID string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		store, ok := deps.Store.(fileCheckpointStore)
		if deps.Store == nil || !ok || deps.FileCheckpoints == nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		if _, found, err := getSessionFileCheckpoint(r.Context(), store, sessionID, checkpointID); err != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		} else if !found {
			http.NotFound(w, r)
			return
		}
		if err := deps.FileCheckpoints.RollbackTo(r.Context(), checkpointID); err != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		var latest *types.FileCheckpoint
		if checkpoint, found, err := store.GetLatestFileCheckpoint(r.Context(), sessionID); err != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		} else if found {
			latest = &checkpoint
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(fileCheckpointRollbackResponse{
			Status:     "rolled_back",
			Checkpoint: latest,
		})
	}
}

func getSessionFileCheckpoint(ctx context.Context, store fileCheckpointStore, sessionID, checkpointID string) (types.FileCheckpoint, bool, error) {
	checkpoint, found, err := store.GetFileCheckpoint(ctx, strings.TrimSpace(checkpointID))
	if err != nil || !found {
		return checkpoint, found, err
	}
	if checkpoint.SessionID != sessionID {
		return types.FileCheckpoint{}, false, nil
	}
	return checkpoint, true, nil
}

func parseCheckpointLimit(raw string) int {
	limit, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || limit <= 0 {
		return 100
	}
	if limit > 500 {
		return 500
	}
	return limit
}

func parseCheckpointActionPath(path string) (string, string, bool) {
	rest := strings.TrimPrefix(path, "/v1/session/checkpoints/")
	rest = strings.Trim(rest, "/")
	parts := strings.Split(rest, "/")
	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
		return "", "", false
	}
	id, err := url.PathUnescape(parts[0])
	if err != nil {
		return "", "", false
	}
	action, err := url.PathUnescape(parts[1])
	if err != nil {
		return "", "", false
	}
	return strings.TrimSpace(id), strings.TrimSpace(action), true
}
