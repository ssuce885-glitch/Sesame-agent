package cli

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	daemoncli "go-agent/internal/cli/daemon"
	"go-agent/internal/cli/client"
	"go-agent/internal/cli/repl"
	"go-agent/internal/config"
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
		Stdout: os.Stdout,
		Stderr: os.Stderr,
		Stdin:  os.Stdin,
		LoadOptions: ParseOptions,
		LoadConfig: func(opts Options) (config.Config, error) {
			return config.ResolveCLIStartupConfig(config.CLIStartupOverrides{
				DataDir:        opts.DataDir,
				Model:          opts.Model,
				PermissionMode: opts.PermissionMode,
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
		NewClient: func(cfg config.Config) RuntimeClient {
			return client.New(baseURLFromAddr(cfg.Addr), &http.Client{})
		},
		NewREPL: func(opts repl.Options) REPLRunner {
			return repl.New(opts)
		},
	}
}

func (a App) Run(ctx context.Context, args []string) error {
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

	cfg, err := a.loadConfig(opts)
	if err != nil {
		return err
	}
	if err := a.ensureDaemon(ctx, cfg); err != nil {
		return err
	}

	runtimeClient := a.newClient(cfg)
	if opts.ShowStatus {
		status, err := runtimeClient.Status(ctx)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(a.Stdout, "status=%s model=%s permission=%s\n", status.Status, status.Model, status.PermissionProfile)
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
		Stdout:    a.Stdout,
		Stdin:     stdin,
		SessionID: sessionID,
		Client:    runtimeClient,
	})
	return runner.Run(ctx, opts.InitialPrompt)
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
			Model:          opts.Model,
			PermissionMode: opts.PermissionMode,
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

func baseURLFromAddr(addr string) string {
	return "http://" + strings.TrimSpace(addr)
}
