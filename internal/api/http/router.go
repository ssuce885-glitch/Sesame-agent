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
	Bus             Bus
	Store           Store
	Manager         Manager
	Scheduler       CronScheduler
	Automation      AutomationService
	RoleService     RoleService
	FileCheckpoints FileCheckpointService
	Status          StatusPayload
	ConsoleRoot     string
	WorkspaceRoot   string
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
	registerCurrentSessionRoutes(mux, deps)
	registerWorkspaceRoutes(mux, deps)
	registerRuntimeGraphRoutes(mux, deps)
	registerWorkspaceReportsRoutes(mux, deps)
	registerMetricsRoutes(mux, deps)
	registerMemoryRoutes(mux, deps)
	registerReportingRoutes(mux, deps)
	registerCronRoutes(mux, deps)
	registerAutomationRoutes(mux, deps)
	registerRoleRoutes(mux, deps)
	registerConsoleRoutes(mux, deps)

	return mux
}

type workspaceReportsStore interface {
	ListWorkspaceReportDeliveryItems(context.Context, string) ([]types.ReportDeliveryItem, error)
	CountQueuedWorkspaceReportDeliveries(context.Context, string) (int, error)
}

func registerWorkspaceReportsRoutes(mux *http.ServeMux, deps Dependencies) {
	mux.HandleFunc("/v1/reports", handleGetWorkspaceReports(deps))
}

func handleGetWorkspaceReports(deps Dependencies) http.HandlerFunc {
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
		store, ok := deps.Store.(workspaceReportsStore)
		if !ok {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		items, err := store.ListWorkspaceReportDeliveryItems(r.Context(), root)
		if err != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		queuedCount, err := store.CountQueuedWorkspaceReportDeliveries(r.Context(), root)
		if err != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(types.WorkspaceReportsResponse{
			WorkspaceRoot: root,
			Items:         items,
			QueuedCount:   queuedCount,
		})
	}
}
