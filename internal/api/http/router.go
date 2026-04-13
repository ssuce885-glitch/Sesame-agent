package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"

	"go-agent/internal/automation"
	"go-agent/internal/session"
	"go-agent/internal/store/sqlite"
	"go-agent/internal/types"
)

type Dependencies struct {
	Bus           Bus
	Store         Store
	Manager       Manager
	Scheduler     CronScheduler
	Automation    AutomationService
	Status        StatusPayload
	ConsoleRoot   string
	WorkspaceRoot string
}

type noopRunner struct{}

func (noopRunner) RunTurn(ctx context.Context, in session.RunInput) error {
	return nil
}

func NewTestDependencies(t interface {
	Helper()
	Fatalf(string, ...any)
	Cleanup(func())
	TempDir() string
}) Dependencies {
	t.Helper()

	store, err := sqlite.Open(filepath.Join(t.TempDir(), "agentd.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})

	return Dependencies{
		Store:      store,
		Manager:    session.NewManager(noopRunner{}),
		Automation: automation.NewService(store),
	}
}

func NewRouter(deps Dependencies) http.Handler {
	mux := http.NewServeMux()
	registerStatusRoutes(mux, deps.Status)
	registerSessionRoutes(mux, deps)
	registerSessionScopedRoutes(mux, deps)
	registerWorkspaceMailboxRoutes(mux, deps)
	registerMetricsRoutes(mux, deps)
	registerPermissionRoutes(mux, deps)
	registerMemoryRoutes(mux, deps)
	registerReportingRoutes(mux, deps)
	registerCronRoutes(mux, deps)
	registerAutomationRoutes(mux, deps)
	registerConsoleRoutes(mux, deps)

	return mux
}

type workspaceMailboxStore interface {
	ListWorkspaceReportMailboxItems(context.Context, string) ([]types.ReportMailboxItem, error)
	CountPendingWorkspaceReportMailboxItems(context.Context, string) (int, error)
}

func registerWorkspaceMailboxRoutes(mux *http.ServeMux, deps Dependencies) {
	mux.HandleFunc("/v1/mailbox", handleGetWorkspaceMailbox(deps))
}

func handleGetWorkspaceMailbox(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if deps.Store == nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		root := strings.TrimSpace(deps.WorkspaceRoot)
		if root == "" {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		store, ok := deps.Store.(workspaceMailboxStore)
		if !ok {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		items, err := store.ListWorkspaceReportMailboxItems(r.Context(), root)
		if err != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		pendingCount, err := store.CountPendingWorkspaceReportMailboxItems(r.Context(), root)
		if err != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(types.WorkspaceReportMailboxResponse{
			WorkspaceRoot: root,
			Items:         items,
			PendingCount:  pendingCount,
		})
	}
}
