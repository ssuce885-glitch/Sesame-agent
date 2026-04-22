package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go-agent/internal/cli/client"
	daemoncli "go-agent/internal/cli/daemon"
	"go-agent/internal/cli/repl"
	"go-agent/internal/config"
	daemonapp "go-agent/internal/daemon"
	"go-agent/internal/extensions"
	"go-agent/internal/types"
	"go-agent/internal/workspace"
)

const Version = "dev"

type RuntimeClient interface {
	repl.RuntimeClient
	automationClient
	EnsureSession(context.Context, string) (types.Session, error)
}

type REPLRunner interface {
	Run(context.Context, string) error
}

type App struct {
	Stdout       io.Writer
	Stderr       io.Writer
	Stdin        io.Reader
	LoadOptions  func([]string) (Options, error)
	LoadConfig   func(Options) (config.Config, error)
	EnsureDaemon func(context.Context, config.Config) error
	StopDaemon   func(context.Context, config.Config) error
	RunSetup     func(context.Context, io.Reader, io.Writer, config.Config, string) error
	RunDaemon    func(context.Context) error
	NewClient    func(config.Config) RuntimeClient
	NewREPL      func(repl.Options) REPLRunner
}

func New() App {
	return App{
		Stdout:      os.Stdout,
		Stderr:      os.Stderr,
		Stdin:       os.Stdin,
		LoadOptions: ParseOptions,
		LoadConfig: func(opts Options) (config.Config, error) {
			return config.ResolveCLIStartupConfig(config.CLIStartupOverrides{
				DataDir:        opts.DataDir,
				Addr:           opts.Addr,
				Model:          opts.Model,
				PermissionMode: opts.PermissionMode,
				WorkspaceRoot:  opts.WorkspaceRoot,
			})
		},
		EnsureDaemon: func(ctx context.Context, cfg config.Config) error {
			manager := daemoncli.NewManager(daemoncli.Options{
				BaseURL: baseURLFromAddr(cfg.Addr),
				Config: daemoncli.LaunchConfig{
					Addr:           cfg.Addr,
					DataDir:        cfg.DataDir,
					Model:          cfg.Model,
					PermissionMode: cfg.PermissionProfile,
				},
				HTTPClient: &http.Client{},
			})
			return manager.EnsureRunning(ctx)
		},
		StopDaemon: func(ctx context.Context, cfg config.Config) error {
			manager := daemoncli.NewManager(daemoncli.Options{
				BaseURL: baseURLFromAddr(cfg.Addr),
				Config: daemoncli.LaunchConfig{
					Addr:           cfg.Addr,
					DataDir:        cfg.DataDir,
					Model:          cfg.Model,
					PermissionMode: cfg.PermissionProfile,
				},
				HTTPClient: &http.Client{},
			})
			return manager.Stop(ctx)
		},
		RunDaemon: daemonapp.Run,
		NewClient: func(cfg config.Config) RuntimeClient {
			return client.New(baseURLFromAddr(cfg.Addr), &http.Client{})
		},
		NewREPL: func(opts repl.Options) REPLRunner {
			return repl.New(opts)
		},
	}
}

func (a App) Run(ctx context.Context, args []string) error {
	ensureConsoleUTF8()

	if a.Stdout == nil {
		a.Stdout = io.Discard
	}
	if a.Stderr == nil {
		a.Stderr = io.Discard
	}

	scriptArgs := isScriptCommandArgs(args)
	opts, err := a.loadOptions(args)
	if err != nil {
		if scriptArgs {
			return newScriptCommandError(err, nil)
		}
		return err
	}
	scriptCommand := scriptArgs || opts.Automation != nil || opts.Trigger != nil || opts.Incident != nil

	if opts.ShowVersion {
		_, err := fmt.Fprintln(a.Stdout, Version)
		return err
	}
	if opts.Skill != nil {
		return runSkillCommand(a.Stdout, *opts.Skill)
	}
	if opts.Setup != nil {
		cfg, err := a.loadConfig(opts)
		if err != nil {
			return err
		}
		return a.runSetup(ctx, cfg, opts.Setup.Action)
	}
	if opts.Daemon != nil {
		cfg, err := a.loadConfig(opts)
		if err != nil {
			return err
		}
		if err := a.runSetup(ctx, cfg, ""); err != nil {
			return err
		}
		return a.runDaemon(ctx)
	}

	workspaceRoot, err := resolveWorkspaceRoot(opts.WorkspaceRoot)
	if err != nil {
		if scriptCommand {
			return newScriptCommandError(err, nil)
		}
		return err
	}
	opts.WorkspaceRoot = workspaceRoot
	if !opts.ShowStatus && !scriptCommand {
		if _, err := workspace.Ensure(workspaceRoot, ""); err != nil {
			return err
		}
	}

	cfg, err := a.loadConfig(opts)
	if err != nil {
		if scriptCommand {
			return newScriptCommandError(err, nil)
		}
		return err
	}
	if opts.ShowStatus {
		return a.runStatus(ctx, cfg)
	}
	if opts.Automation != nil || opts.Trigger != nil || opts.Incident != nil {
		return a.runScriptCommand(ctx, opts, cfg)
	}
	if err := ensureRuntimeConfigured(a.Stdin, a.Stdout, cfg); err != nil {
		return err
	}
	cfg, err = a.prepareDaemonConfig(opts, cfg)
	if err != nil {
		return err
	}
	if err := a.ensureDaemon(ctx, cfg); err != nil {
		return err
	}

	runtimeClient := a.newClient(cfg)
	sessionRow, err := runtimeClient.EnsureSession(ctx, workspaceRoot)
	if err != nil {
		return err
	}
	sessionID := sessionRow.ID
	if strings.TrimSpace(sessionRow.WorkspaceRoot) != "" {
		workspaceRoot = sessionRow.WorkspaceRoot
	}
	opts.WorkspaceRoot = workspaceRoot
	if !opts.ShowStatus && !scriptCommand {
		if _, err := workspace.Ensure(workspaceRoot, ""); err != nil {
			return err
		}
	}
	cliConfig, err := config.LoadCLIConfig()
	if err != nil {
		return err
	}
	catalogLoader := func() (extensions.Catalog, error) {
		return extensions.LoadCatalog(cfg.Paths.GlobalRoot, workspaceRoot)
	}
	catalog, err := catalogLoader()
	if err != nil {
		return err
	}

	stdin := a.Stdin
	if opts.PrintOnly {
		if strings.TrimSpace(opts.InitialPrompt) == "" {
			return fmt.Errorf("--print requires a prompt")
		}
		stdin = nil
	}

	runner := a.newREPL(repl.Options{
		Stdout:                a.Stdout,
		Stdin:                 stdin,
		SessionID:             sessionID,
		WorkspaceRoot:         workspaceRoot,
		ShowExtensionsSummary: cliConfig.ShowExtensionsOnStartup,
		Client:                runtimeClient,
		Catalog:               catalog,
		CatalogLoader:         catalogLoader,
	})
	runErr := runner.Run(ctx, opts.InitialPrompt)
	if !a.shouldStopDaemonAfterRun(opts) {
		return runErr
	}

	stopCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	stopErr := a.stopDaemon(stopCtx, cfg)
	if runErr != nil && stopErr != nil {
		return errors.Join(runErr, stopErr)
	}
	if runErr != nil {
		return runErr
	}
	return stopErr
}

func (a App) runScriptCommand(ctx context.Context, opts Options, cfg config.Config) error {
	cfg, err := a.prepareDaemonConfig(opts, cfg)
	if err != nil {
		return newScriptCommandError(err, nil)
	}
	if err := a.ensureDaemon(ctx, cfg); err != nil {
		stopCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		stopErr := a.stopDaemon(stopCtx, cfg)
		return newScriptCommandError(err, stopErr)
	}

	runtimeClient := a.newClient(cfg)
	var runErr error
	switch {
	case opts.Automation != nil:
		runErr = runAutomationCommand(ctx, a.Stdout, runtimeClient, *opts.Automation)
	case opts.Trigger != nil:
		runErr = runTriggerCommand(ctx, a.Stdout, runtimeClient, *opts.Trigger)
	case opts.Permissions != nil:
		runErr = runPermissionsCommand(ctx, a.Stdout, runtimeClient, *opts.Permissions)
	default:
		runErr = runIncidentCommand(ctx, a.Stdout, runtimeClient, *opts.Incident)
	}

	if !a.shouldStopDaemonAfterRun(opts) {
		if runErr != nil {
			return newScriptCommandError(runErr, nil)
		}
		return nil
	}

	stopCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	stopErr := a.stopDaemon(stopCtx, cfg)
	if runErr != nil || stopErr != nil {
		return newScriptCommandError(runErr, stopErr)
	}
	return nil
}

func isScriptCommandArgs(args []string) bool {
	if len(args) == 0 {
		return false
	}
	switch strings.TrimSpace(args[0]) {
	case "automation", "trigger", "incident", "permissions":
		return true
	default:
		return false
	}
}

func (a App) runStatus(ctx context.Context, cfg config.Config) error {
	runtimeClient := a.newClient(cfg)
	status, err := runtimeClient.Status(ctx)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(
		a.Stdout,
		"status=%s model=%s permission=%s\n",
		status.Status,
		status.Model,
		status.PermissionProfile,
	)
	return err
}

func resolveWorkspaceRoot(explicitRoot string) (string, error) {
	if trimmed := strings.TrimSpace(explicitRoot); trimmed != "" {
		return filepath.Abs(trimmed)
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return filepath.Abs(cwd)
}

func (a App) loadOptions(args []string) (Options, error) {
	if a.LoadOptions == nil {
		return ParseOptions(args)
	}
	return a.LoadOptions(args)
}

func (a App) loadConfig(opts Options) (config.Config, error) {
	if a.LoadConfig == nil {
		return config.ResolveCLIStartupConfig(config.CLIStartupOverrides{
			DataDir:        opts.DataDir,
			Addr:           opts.Addr,
			Model:          opts.Model,
			PermissionMode: opts.PermissionMode,
			WorkspaceRoot:  opts.WorkspaceRoot,
		})
	}
	return a.LoadConfig(opts)
}

func (a App) ensureDaemon(ctx context.Context, cfg config.Config) error {
	if a.EnsureDaemon == nil {
		return New().EnsureDaemon(ctx, cfg)
	}
	return a.EnsureDaemon(ctx, cfg)
}

func (a App) stopDaemon(ctx context.Context, cfg config.Config) error {
	if a.StopDaemon == nil {
		return New().StopDaemon(ctx, cfg)
	}
	return a.StopDaemon(ctx, cfg)
}

func (a App) shouldStopDaemonAfterRun(_ Options) bool {
	// Phase 3 default lifecycle: attach to the workspace daemon and leave it
	// running after CLI/TUI/script completion. Startup-failure cleanup stays
	// handled at the ensureDaemon call sites.
	return false
}

func (a App) runSetup(ctx context.Context, cfg config.Config, action string) error {
	if a.RunSetup != nil {
		return a.RunSetup(ctx, a.Stdin, a.Stdout, cfg, action)
	}
	return ensureRuntimeConfiguredAction(a.Stdin, a.Stdout, cfg, action)
}

func (a App) runDaemon(ctx context.Context) error {
	if a.RunDaemon != nil {
		return a.RunDaemon(ctx)
	}
	return daemonapp.Run(ctx)
}

func (a App) newClient(cfg config.Config) RuntimeClient {
	if a.NewClient == nil {
		return client.New(baseURLFromAddr(cfg.Addr), &http.Client{})
	}
	return a.NewClient(cfg)
}

func (a App) newREPL(opts repl.Options) REPLRunner {
	if a.NewREPL == nil {
		return repl.New(opts)
	}
	return a.NewREPL(opts)
}

func (a App) prepareDaemonConfig(opts Options, base config.Config) (config.Config, error) {
	_ = opts
	return base, nil
}

func baseURLFromAddr(addr string) string {
	return "http://" + strings.TrimSpace(addr)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
