package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"slices"
	"strings"

	"go-agent/internal/runtime"
	"go-agent/internal/types"
)

type runtimeGraphStore interface {
	ListRuntimeGraph(context.Context) (types.RuntimeGraph, error)
	ListRuntimeGraphForWorkspace(context.Context, string) (types.RuntimeGraph, error)
}

type runtimeGraphSessionStore interface {
	ListSessions(context.Context) ([]types.Session, error)
	ListSessionEvents(context.Context, string, int64) ([]types.Event, error)
}

func registerRuntimeGraphRoutes(mux *http.ServeMux, deps Dependencies) {
	mux.HandleFunc("/v1/runtime_graph", handleGetRuntimeGraph(deps))
}

func handleGetRuntimeGraph(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		workspaceRoot := strings.TrimSpace(deps.WorkspaceRoot)
		if workspaceRoot == "" {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		store, ok := deps.Store.(runtimeGraphStore)
		if !ok {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		graph, err := store.ListRuntimeGraphForWorkspace(r.Context(), workspaceRoot)
		if err != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		diagnostics, err := collectRuntimeDiagnostics(r.Context(), deps.Store, workspaceRoot)
		if err != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		graph.Diagnostics = diagnostics
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(types.WorkspaceRuntimeGraphResponse{
			WorkspaceRoot: workspaceRoot,
			Graph:         graph,
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
	return filtered
}

type interruptedReasonPayload struct {
	Reason string `json:"reason"`
}

type runtimeDiagnosticDescriptor struct {
	Category   string
	Severity   string
	Summary    string
	RepairHint string
}

var runtimeDiagnosticReasons = []string{
	"task_session_replay_unsupported",
	"unmapped_session",
}

func collectRuntimeDiagnostics(ctx context.Context, store any, workspaceRoot string) ([]types.RuntimeDiagnostic, error) {
	sessionStore, ok := store.(runtimeGraphSessionStore)
	if !ok {
		return []types.RuntimeDiagnostic{}, nil
	}

	sessions, err := sessionStore.ListSessions(ctx)
	if err != nil {
		return nil, err
	}

	var diagnostics []types.RuntimeDiagnostic
	for _, sessionRow := range sessions {
		if strings.TrimSpace(sessionRow.WorkspaceRoot) != workspaceRoot {
			continue
		}
		events, err := sessionStore.ListSessionEvents(ctx, sessionRow.ID, 0)
		if err != nil {
			return nil, err
		}
		for _, event := range events {
			if event.Type != types.EventTurnInterrupted {
				continue
			}
			var payload interruptedReasonPayload
			if err := json.Unmarshal(event.Payload, &payload); err != nil {
				return nil, err
			}
			if !slices.Contains(runtimeDiagnosticReasons, strings.TrimSpace(payload.Reason)) {
				continue
			}
			descriptor := describeRuntimeDiagnostic(payload.Reason)
			diagnostics = append(diagnostics, types.RuntimeDiagnostic{
				ID:         event.ID,
				SessionID:  event.SessionID,
				TurnID:     event.TurnID,
				EventType:  event.Type,
				Category:   descriptor.Category,
				Severity:   descriptor.Severity,
				Reason:     payload.Reason,
				Summary:    descriptor.Summary,
				RepairHint: descriptor.RepairHint,
				AssetKind:  "turn",
				AssetID:    event.TurnID,
				CreatedAt:  event.Time.UTC(),
			})
		}
	}

	slices.SortFunc(diagnostics, func(a, b types.RuntimeDiagnostic) int {
		return b.CreatedAt.Compare(a.CreatedAt)
	})
	return diagnostics, nil
}

func describeRuntimeDiagnostic(reason string) runtimeDiagnosticDescriptor {
	switch strings.TrimSpace(reason) {
	case "task_session_replay_unsupported":
		return runtimeDiagnosticDescriptor{
			Category:   "task_recovery",
			Severity:   "warning",
			Summary:    "Task-session replay was quarantined during daemon recovery.",
			RepairHint: "Review the interrupted task, then restart or recreate it from the workspace runtime view.",
		}
	case "unmapped_session":
		return runtimeDiagnosticDescriptor{
			Category:   "session_binding",
			Severity:   "warning",
			Summary:    "A created turn was quarantined because its session binding was no longer canonical.",
			RepairHint: "Inspect workspace session bindings and reopen the parent context if needed.",
		}
	default:
		return runtimeDiagnosticDescriptor{
			Category:   "runtime_recovery",
			Severity:   "warning",
			Summary:    "Runtime quarantine diagnostic.",
			RepairHint: "Inspect the affected runtime asset and decide whether it should be restarted or left quarantined.",
		}
	}
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
