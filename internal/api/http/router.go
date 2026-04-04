package httpapi

import (
	"context"
	"net/http"
	"path/filepath"

	"go-agent/internal/session"
	"go-agent/internal/store/sqlite"
)

type Dependencies struct {
	Bus         Bus
	Store       Store
	Manager     Manager
	Status      StatusPayload
	ConsoleRoot string
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
		Store:   store,
		Manager: session.NewManager(noopRunner{}),
	}
}

func NewRouter(deps Dependencies) http.Handler {
	mux := http.NewServeMux()
	registerStatusRoutes(mux, deps.Status)
	registerSessionRoutes(mux, deps)
	registerSessionScopedRoutes(mux, deps)
	registerMetricsRoutes(mux, deps)
	registerPermissionRoutes(mux, deps)
	registerMemoryRoutes(mux, deps)
	registerConsoleRoutes(mux, deps)

	return mux
}
