package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"

	"go-agent/internal/runtime"
	"go-agent/internal/types"
)

type runtimeGraphStore interface {
	ListRuntimeGraph(context.Context) (types.RuntimeGraph, error)
}

func handleGetRuntimeGraph(deps Dependencies, sessionID string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		store, ok := deps.Store.(runtimeGraphStore)
		if !ok {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		graph, err := store.ListRuntimeGraph(r.Context())
		if err != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"graph": filterRuntimeGraphForSession(graph, sessionID),
		})
	}
}

func filterRuntimeGraphForSession(graph types.RuntimeGraph, sessionID string) types.RuntimeGraph {
	if sessionID == "" {
		return graph
	}
	runIDs := make(map[string]struct{})
	for _, run := range graph.Runs {
		if run.SessionID == sessionID {
			runIDs[run.ID] = struct{}{}
		}
	}
	filtered := types.RuntimeGraph{
		Runs: make([]types.Run, 0, len(runIDs)),
	}
	for _, run := range graph.Runs {
		if _, ok := runIDs[run.ID]; ok {
			filtered.Runs = append(filtered.Runs, run)
		}
	}
	for _, plan := range graph.Plans {
		if _, ok := runIDs[plan.RunID]; ok {
			filtered.Plans = append(filtered.Plans, plan)
		}
	}
	for _, task := range graph.Tasks {
		if _, ok := runIDs[task.RunID]; ok {
			filtered.Tasks = append(filtered.Tasks, task)
		}
	}
	for _, toolRun := range graph.ToolRuns {
		if _, ok := runIDs[toolRun.RunID]; ok {
			filtered.ToolRuns = append(filtered.ToolRuns, toolRun)
		}
	}
	for _, worktree := range graph.Worktrees {
		if _, ok := runIDs[worktree.RunID]; ok {
			filtered.Worktrees = append(filtered.Worktrees, worktree)
		}
	}
	for _, request := range graph.PermissionRequests {
		if _, ok := runIDs[request.RunID]; ok || request.SessionID == sessionID {
			filtered.PermissionRequests = append(filtered.PermissionRequests, request)
		}
	}
	return filtered
}

func handleGetSessionFileContent(deps Dependencies, sessionID string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		path := r.URL.Query().Get("path")
		if path == "" {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		sessionRow, ok, err := deps.Store.GetSession(r.Context(), sessionID)
		if err != nil || !ok {
			http.NotFound(w, r)
			return
		}
		resolved := path
		if !filepath.IsAbs(resolved) {
			resolved = filepath.Join(sessionRow.WorkspaceRoot, resolved)
		}
		resolved = filepath.Clean(resolved)
		if err := runtime.WithinWorkspace(sessionRow.WorkspaceRoot, resolved); err != nil {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		http.ServeFile(w, r, resolved)
	}
}
