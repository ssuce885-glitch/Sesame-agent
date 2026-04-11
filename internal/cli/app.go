package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"go-agent/internal/cli/client"
	daemoncli "go-agent/internal/cli/daemon"
	"go-agent/internal/cli/repl"
	"go-agent/internal/config"
	"go-agent/internal/extensions"
)

const Version = "dev"

type RuntimeClient interface {
	repl.RuntimeClient
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

	opts, err := a.loadOptions(args)
	if err != nil {
		return err
	}

	if opts.ShowVersion {
		_, err := fmt.Fprintln(a.Stdout, Version)
		return err
	}
	if opts.Skill != nil {
		return runSkillCommand(a.Stdout, *opts.Skill)
	}

	cfg, err := a.loadRuntimeConfigWithSetup(opts)
	if err != nil {
		return err
	}
	if opts.ListDaemons {
		return renderDaemonHistory(a.Stdout, daemonGlobalRoot(cfg))
	}
	if opts.ShowStatus {
		return a.runStatus(ctx, opts, cfg)
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
	cliConfig, err := config.LoadCLIConfig()
	if err != nil {
		return err
	}
	workspaceRoot, err := os.Getwd()
	if err != nil {
		return err
	}
	catalog, err := extensions.LoadCatalog(cfg.Paths.GlobalRoot, workspaceRoot)
	if err != nil {
		return err
	}

	sessionID, err := resolveSessionID(ctx, runtimeClient, opts.ResumeID)
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
	})
	return runner.Run(ctx, opts.InitialPrompt)
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

func resolveSessionID(ctx context.Context, runtimeClient RuntimeClient, resumeID string) (string, error) {
	if strings.TrimSpace(resumeID) != "" {
		if err := runtimeClient.SelectSession(ctx, resumeID); err != nil {
			return "", err
		}
		return strings.TrimSpace(resumeID), nil
	}

	workspaceRoot, err := os.Getwd()
	if err != nil {
		return "", err
	}
	sessionID, _, err := runtimeClient.FindOrCreateWorkspaceSession(ctx, workspaceRoot)
	if err != nil {
		return "", err
	}
	return sessionID, nil
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
		})
	}
	return a.LoadConfig(opts)
}

func (a App) loadRuntimeConfigWithSetup(opts Options) (config.Config, error) {
	cfg, err := a.loadConfig(opts)
	if err == nil {
		return cfg, nil
	}
	if !isRecoverableSetupConfigError(err) {
		return config.Config{}, err
	}
	if err := ensureRuntimeConfigured(a.Stdin, a.Stdout, config.Config{}); err != nil {
		return config.Config{}, err
	}
	return a.loadConfig(opts)
}

func isRecoverableSetupConfigError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, config.ErrLegacyConfigFieldsUnsupported) {
		return false
	}
	if errors.Is(err, config.ErrActiveProfileRequired) {
		return true
	}
	if errors.Is(err, config.ErrActiveProfileNotFound) {
		return true
	}
	if errors.Is(err, config.ErrUnknownModelProvider) {
		return true
	}
	return false
}

func (a App) ensureDaemon(ctx context.Context, cfg config.Config) error {
	if a.EnsureDaemon == nil {
		return New().EnsureDaemon(ctx, cfg)
	}
	return a.EnsureDaemon(ctx, cfg)
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
