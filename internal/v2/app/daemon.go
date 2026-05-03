package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go-agent/internal/config"
	"go-agent/internal/model"
	"go-agent/internal/skillcatalog"
	"go-agent/internal/v2/agent"
	"go-agent/internal/v2/automation"
	"go-agent/internal/v2/contracts"
	"go-agent/internal/v2/memory"
	"go-agent/internal/v2/observability"
	"go-agent/internal/v2/reports"
	"go-agent/internal/v2/roles"
	v2session "go-agent/internal/v2/session"
	"go-agent/internal/v2/store"
	"go-agent/internal/v2/tasks"
	"go-agent/internal/v2/tools"
)

func Run(ctx context.Context, cfg config.Config) error {
	if err := os.MkdirAll(filepath.Dir(cfg.Paths.DatabaseFile), 0o755); err != nil {
		return fmt.Errorf("create data dir: %w", err)
	}

	s, err := store.Open(cfg.Paths.DatabaseFile)
	if err != nil {
		return fmt.Errorf("open v2 store: %w", err)
	}
	defer s.Close()

	if err := markInterrupted(ctx, s); err != nil {
		return fmt.Errorf("recover running turns: %w", err)
	}

	client, err := model.NewFromConfig(cfg)
	if err != nil {
		return fmt.Errorf("create model client: %w", err)
	}

	roleService := roles.NewService(cfg.Paths.WorkspaceRoot)
	catalog, err := skillcatalog.LoadCatalog(cfg.Paths.GlobalRoot, cfg.Paths.WorkspaceRoot)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: load skill catalog: %v\n", err)
	}
	if catalog.Skills == nil {
		catalog.Skills = []skillcatalog.SkillSpec{}
	}
	registry := tools.NewRegistry()
	tools.RegisterAllTools(registry, roleService, catalog)

	metrics := observability.New()
	ag := agent.New(client, registry, s, metrics)
	projectStateAuto, err := s.Settings().Get(ctx, "project_state_auto")
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("read project_state_auto setting: %w", err)
	}
	if strings.EqualFold(strings.TrimSpace(projectStateAuto), "false") {
		ag.SetProjectStateAutoUpdate(false)
	}
	systemPrompt, err := cfg.ResolveSystemPrompt()
	if err != nil {
		return fmt.Errorf("resolve system prompt: %w", err)
	}
	ag.SetSystemPrompt(systemPrompt)
	if cfg.MaxToolSteps > 0 {
		ag.SetMaxSteps(cfg.MaxToolSteps)
	}

	sessionMgr := v2session.NewManager(ag)
	taskManager := tasks.NewManager(s, filepath.Join(cfg.Paths.DataDir, "task_outputs"), metrics)
	taskManager.RegisterRunner("agent", tasks.NewAgentRunner(s, sessionMgr, roleService))
	delegateDeps := tools.DelegateToolDeps{
		SessionMgr:  sessionMgr,
		Store:       s,
		TaskManager: taskManager,
		RoleService: roleService,
	}
	registry.Register(contracts.NamespaceRoles, tools.NewDelegateToRoleTool(delegateDeps))
	reportService := reports.NewService(s)
	taskManager.SetReporter(reportService)
	memoryService := memory.NewService(s)
	automationService := automation.NewService(s, taskManager, roleService)
	ag.SetAutomationService(automationService)
	defaultSession, err := ensureSession(ctx, s, cfg.Paths.WorkspaceRoot, systemPrompt, cfg.PermissionProfile)
	if err != nil {
		return fmt.Errorf("ensure default session: %w", err)
	}
	sessionMgr.Register(defaultSession)
	if err := registerExistingSessions(ctx, s, sessionMgr, cfg.Paths.WorkspaceRoot); err != nil {
		return fmt.Errorf("register sessions: %w", err)
	}

	routes := &routes{
		cfg:               cfg,
		store:             s,
		sessionMgr:        sessionMgr,
		taskManager:       taskManager,
		memoryService:     memoryService,
		metrics:           metrics,
		roleService:       roleService,
		automationService: automationService,
		projectStateAuto:  ag,
		defaultSessionID:  defaultSession.ID,
	}

	go func() {
		ticker := time.NewTicker(60 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				_ = automationService.Reconcile(ctx)
				_, _ = memoryService.Cleanup(ctx, cfg.Paths.WorkspaceRoot, 1000, 500)
			}
		}
	}()

	server := &http.Server{
		Addr:              cfg.Addr,
		Handler:           routes.handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      0,
		IdleTimeout:       60 * time.Second,
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- server.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
		return ctx.Err()
	case err := <-errCh:
		if err == http.ErrServerClosed {
			return nil
		}
		return err
	}
}

func markInterrupted(ctx context.Context, s contracts.Store) error {
	running, err := s.Turns().ListRunning(ctx)
	if err != nil {
		return err
	}
	for _, turn := range running {
		if err := s.Turns().UpdateState(ctx, turn.ID, "interrupted"); err != nil {
			return err
		}
	}
	return nil
}

func registerExistingSessions(ctx context.Context, s contracts.Store, mgr contracts.SessionManager, workspaceRoot string) error {
	sessions, err := s.Sessions().ListByWorkspace(ctx, workspaceRoot)
	if err != nil {
		return err
	}
	for _, session := range sessions {
		mgr.Register(session)
	}
	return nil
}
