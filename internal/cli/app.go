package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"go-agent/internal/cli/client"
	daemoncli "go-agent/internal/cli/daemon"
	"go-agent/internal/cli/repl"
	"go-agent/internal/config"
	"go-agent/internal/extensions"
	"go-agent/internal/workspace"
)

const Version = "dev"

type RuntimeClient interface {
	repl.RuntimeClient
	automationClient
	FindOrCreateWorkspaceSession(context.Context, string) (string, bool, error)
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
					DaemonID:          cfg.DaemonID,
					Addr:              cfg.Addr,
					DataDir:           cfg.DataDir,
					Model:             cfg.Model,
					PermissionMode:    cfg.PermissionProfile,
					ConfigFingerprint: cfg.ConfigFingerprint,
				},
				HTTPClient: &http.Client{},
			})
			return manager.EnsureRunning(ctx)
		},
		StopDaemon: func(ctx context.Context, cfg config.Config) error {
			manager := daemoncli.NewManager(daemoncli.Options{
				BaseURL: baseURLFromAddr(cfg.Addr),
				Config: daemoncli.LaunchConfig{
					DaemonID:          cfg.DaemonID,
					Addr:              cfg.Addr,
					DataDir:           cfg.DataDir,
					Model:             cfg.Model,
					PermissionMode:    cfg.PermissionProfile,
					ConfigFingerprint: cfg.ConfigFingerprint,
				},
				HTTPClient: &http.Client{},
			})
			return manager.Stop(ctx)
		},
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

	workspaceRoot, err := resolveWorkspaceRoot(opts.WorkspaceRoot)
	if err != nil {
		if scriptCommand {
			return newScriptCommandError(err, nil)
		}
		return err
	}
	opts.WorkspaceRoot = workspaceRoot
	if !opts.ListDaemons && !opts.ShowStatus && !scriptCommand && strings.TrimSpace(opts.ResumeID) == "" {
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
	if opts.ListDaemons {
		return renderDaemonHistory(a.Stdout, daemonGlobalRoot(cfg))
	}
	if opts.ShowStatus {
		return a.runStatus(ctx, opts, cfg)
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
	sessionID, sessionWorkspaceRoot, err := resolveSessionBinding(ctx, runtimeClient, opts.ResumeID, workspaceRoot)
	if err != nil {
		return err
	}
	workspaceRoot = sessionWorkspaceRoot
	opts.WorkspaceRoot = workspaceRoot
	if !opts.ListDaemons && !opts.ShowStatus && !scriptCommand {
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
	default:
		runErr = runIncidentCommand(ctx, a.Stdout, runtimeClient, *opts.Incident)
	}

	if opts.Automation != nil && strings.EqualFold(strings.TrimSpace(opts.Automation.Action), "run") {
		return runErr
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
	case "automation", "trigger", "incident":
		return true
	default:
		return false
	}
}

func (a App) runStatus(ctx context.Context, opts Options, base config.Config) error {
	cfg, err := a.prepareStatusConfig(opts, base)
	if err != nil {
		return err
	}
	runtimeClient := a.newClient(cfg)
	status, err := runtimeClient.Status(ctx)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(
		a.Stdout,
		"status=%s daemon=%s model=%s permission=%s\n",
		status.Status,
		firstNonEmpty(status.DaemonID, cfg.DaemonID, "unknown"),
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

func resolveSessionBinding(ctx context.Context, runtimeClient RuntimeClient, resumeID string, workspaceRoot string) (string, string, error) {
	if strings.TrimSpace(resumeID) != "" {
		resp, err := runtimeClient.ListSessions(ctx)
		if err != nil {
			return "", "", err
		}
		for _, item := range resp.Sessions {
			if item.ID != strings.TrimSpace(resumeID) {
				continue
			}
			if err := runtimeClient.SelectSession(ctx, resumeID); err != nil {
				return "", "", err
			}
			boundWorkspaceRoot := strings.TrimSpace(item.WorkspaceRoot)
			if boundWorkspaceRoot == "" {
				boundWorkspaceRoot = workspaceRoot
			}
			return strings.TrimSpace(resumeID), boundWorkspaceRoot, nil
		}
		return "", "", fmt.Errorf("session %q not found", strings.TrimSpace(resumeID))
	}

	sessionID, _, err := runtimeClient.FindOrCreateWorkspaceSession(ctx, workspaceRoot)
	if err != nil {
		return "", "", err
	}
	return sessionID, workspaceRoot, nil
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
	daemonOpts := opts
	globalRoot := daemonGlobalRoot(base)

	ref := strings.TrimSpace(opts.DaemonRef)

	var (
		record daemoncli.Record
		err    error
	)
	if ref == "" || strings.EqualFold(ref, "new") {
		record, err = daemoncli.CreateRecord(globalRoot, daemoncli.LaunchConfig{
			Addr:           base.Addr,
			Model:          base.Model,
			PermissionMode: base.PermissionProfile,
		})
	} else {
		record, err = daemoncli.ResolveRecord(globalRoot, ref)
	}
	if err != nil {
		return config.Config{}, err
	}

	if strings.TrimSpace(daemonOpts.Model) == "" && strings.TrimSpace(record.Model) != "" {
		daemonOpts.Model = record.Model
	}
	if strings.TrimSpace(daemonOpts.PermissionMode) == "" && strings.TrimSpace(record.PermissionMode) != "" {
		daemonOpts.PermissionMode = record.PermissionMode
	}
	daemonOpts.Addr = record.Addr
	daemonOpts.DataDir = record.DataDir

	cfg, err := a.loadConfig(daemonOpts)
	if err != nil {
		return config.Config{}, err
	}
	cfg.DaemonID = record.ID
	cfg.ConfigFingerprint = cfg.Fingerprint()

	record.Model = cfg.Model
	record.PermissionMode = cfg.PermissionProfile
	if _, err := daemoncli.TouchRecord(globalRoot, record); err != nil {
		return config.Config{}, err
	}
	return cfg, nil
}

func (a App) prepareStatusConfig(opts Options, base config.Config) (config.Config, error) {
	daemonOpts := opts
	globalRoot := daemonGlobalRoot(base)

	ref := strings.TrimSpace(opts.DaemonRef)
	if ref == "" {
		if latest, err := daemoncli.ResolveRecord(globalRoot, "latest"); err == nil {
			ref = latest.ID
		}
	}
	if ref == "" || strings.EqualFold(ref, "new") {
		return base, nil
	}

	record, err := daemoncli.ResolveRecord(globalRoot, ref)
	if err != nil {
		return config.Config{}, err
	}
	if strings.TrimSpace(daemonOpts.Model) == "" && strings.TrimSpace(record.Model) != "" {
		daemonOpts.Model = record.Model
	}
	if strings.TrimSpace(daemonOpts.PermissionMode) == "" && strings.TrimSpace(record.PermissionMode) != "" {
		daemonOpts.PermissionMode = record.PermissionMode
	}
	daemonOpts.Addr = record.Addr
	daemonOpts.DataDir = record.DataDir

	cfg, err := a.loadConfig(daemonOpts)
	if err != nil {
		return config.Config{}, err
	}
	cfg.DaemonID = record.ID
	return cfg, nil
}

func renderDaemonHistory(out io.Writer, globalRoot string) error {
	if out == nil {
		out = io.Discard
	}
	records, err := daemoncli.ListRecords(globalRoot)
	if err != nil {
		return err
	}
	if len(records) == 0 {
		_, err := fmt.Fprintln(out, "No historical daemons.")
		return err
	}

	clientHTTP := &http.Client{Timeout: 300 * time.Millisecond}
	type rendered struct {
		record  daemoncli.Record
		running bool
		pid     int
	}
	items := make([]rendered, 0, len(records))
	for _, record := range records {
		status, statusErr := daemoncli.FetchStatus(context.Background(), baseURLFromAddr(record.Addr), clientHTTP)
		items = append(items, rendered{
			record:  record,
			running: statusErr == nil && status.Status == "ok",
			pid:     status.PID,
		})
	}
	sort.SliceStable(items, func(i, j int) bool {
		return items[i].record.LastUsedAt.After(items[j].record.LastUsedAt)
	})
	for _, item := range items {
		state := "stopped"
		if item.running {
			state = "running"
		}
		line := fmt.Sprintf("%s  %s  %s", item.record.ID, item.record.Addr, state)
		if item.pid > 0 {
			line += fmt.Sprintf(" pid=%d", item.pid)
		}
		if strings.TrimSpace(item.record.Model) != "" {
			line += " model=" + item.record.Model
		}
		if strings.TrimSpace(item.record.PermissionMode) != "" {
			line += " permission=" + item.record.PermissionMode
		}
		if !item.record.LastUsedAt.IsZero() {
			line += " last_used=" + item.record.LastUsedAt.Format("2006-01-02 15:04:05")
		}
		_, _ = fmt.Fprintln(out, line)
	}
	return nil
}

func baseURLFromAddr(addr string) string {
	return "http://" + strings.TrimSpace(addr)
}

func daemonGlobalRoot(cfg config.Config) string {
	if strings.TrimSpace(cfg.Paths.GlobalRoot) != "" {
		return strings.TrimSpace(cfg.Paths.GlobalRoot)
	}
	paths, err := config.ResolvePaths("", "")
	if err != nil {
		return ""
	}
	return paths.GlobalRoot
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
