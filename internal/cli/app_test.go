package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go-agent/internal/cli/client"
	daemoncli "go-agent/internal/cli/daemon"
	"go-agent/internal/cli/repl"
	"go-agent/internal/config"
	"go-agent/internal/types"
)

func TestRunStatusModePrintsDaemonStatus(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	var stdout bytes.Buffer
	fakeClient := &fakeRuntimeClient{
		status: client.StatusResponse{Status: "ok", Model: "gpt-5", PermissionProfile: "trusted_local"},
	}
	ensureDaemonCalled := false

	app := App{
		Stdout: &stdout,
		LoadOptions: func([]string) (Options, error) {
			return Options{ShowStatus: true}, nil
		},
		LoadConfig: func(Options) (config.Config, error) {
			return config.Config{Addr: "127.0.0.1:4317", DataDir: "E:/tmp/sesame"}, nil
		},
		EnsureDaemon: func(context.Context, config.Config) error {
			ensureDaemonCalled = true
			return nil
		},
		NewClient: func(config.Config) RuntimeClient { return fakeClient },
		NewREPL: func(repl.Options) REPLRunner {
			t.Fatal("REPL should not be constructed for --status")
			return nil
		},
	}

	if err := app.Run(context.Background(), []string{"--status"}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !strings.Contains(stdout.String(), "gpt-5") {
		t.Fatalf("stdout = %q, want model", stdout.String())
	}
	if ensureDaemonCalled {
		t.Fatal("EnsureDaemon should not be called for --status")
	}
}

func TestRunInteractiveModeResumesSessionAndStartsREPL(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	var gotOptions repl.Options
	fakeClient := &fakeRuntimeClient{}
	stopCalled := false

	app := App{
		Stdout: &bytes.Buffer{},
		Stderr: &bytes.Buffer{},
		Stdin:  strings.NewReader(""),
		LoadOptions: func([]string) (Options, error) {
			return Options{ResumeID: "sess_resume", InitialPrompt: "hello"}, nil
		},
		LoadConfig: func(Options) (config.Config, error) {
			return config.Config{Addr: "127.0.0.1:4317", DataDir: "E:/tmp/sesame", ModelProvider: "fake", Model: "gpt-5", PermissionProfile: "trusted_local"}, nil
		},
		EnsureDaemon: func(context.Context, config.Config) error { return nil },
		StopDaemon: func(_ context.Context, cfg config.Config) error {
			stopCalled = true
			if cfg.Addr != "127.0.0.1:4317" {
				t.Fatalf("cfg.Addr = %q, want %q", cfg.Addr, "127.0.0.1:4317")
			}
			return nil
		},
		NewClient: func(config.Config) RuntimeClient { return fakeClient },
		NewREPL: func(opts repl.Options) REPLRunner {
			gotOptions = opts
			return stubRunner{}
		},
	}

	if err := app.Run(context.Background(), nil); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if fakeClient.selectedSessionID != "sess_resume" {
		t.Fatalf("selectedSessionID = %q, want %q", fakeClient.selectedSessionID, "sess_resume")
	}
	if gotOptions.SessionID != "sess_resume" {
		t.Fatalf("SessionID = %q, want %q", gotOptions.SessionID, "sess_resume")
	}
	if !stopCalled {
		t.Fatal("StopDaemon was not called")
	}
}

func TestRunListDaemonsPrintsHistoricalDaemonRecords(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	record := daemoncli.Record{
		ID:             "daemon_demo123",
		Addr:           "127.0.0.1:4321",
		DataDir:        home + "/.sesame/daemons/daemon_demo123",
		Model:          "gpt-5.4",
		PermissionMode: "trusted_local",
	}
	if err := daemoncli.SaveRecord(home+"/.sesame", record); err != nil {
		t.Fatalf("SaveRecord() error = %v", err)
	}

	var stdout bytes.Buffer
	app := App{
		Stdout: &stdout,
		LoadOptions: func([]string) (Options, error) {
			return Options{ListDaemons: true}, nil
		},
		LoadConfig: func(Options) (config.Config, error) {
			return config.Config{Paths: config.Paths{GlobalRoot: home + "/.sesame"}}, nil
		},
		EnsureDaemon: func(context.Context, config.Config) error {
			t.Fatal("EnsureDaemon should not be called for --list-daemons")
			return nil
		},
		NewClient: func(config.Config) RuntimeClient {
			t.Fatal("NewClient should not be called for --list-daemons")
			return nil
		},
	}

	if err := app.Run(context.Background(), []string{"--list-daemons"}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !strings.Contains(stdout.String(), "daemon_demo123") {
		t.Fatalf("stdout = %q, want daemon id", stdout.String())
	}
}

func TestPrepareDaemonConfigUsesHistoricalDaemonRecord(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	record := daemoncli.Record{
		ID:             "daemon_demo123",
		Addr:           "127.0.0.1:4555",
		DataDir:        home + "/.sesame/daemons/daemon_demo123",
		Model:          "historic-model",
		PermissionMode: "trusted_local",
	}
	if err := daemoncli.SaveRecord(home+"/.sesame", record); err != nil {
		t.Fatalf("SaveRecord() error = %v", err)
	}

	app := App{
		LoadConfig: func(opts Options) (config.Config, error) {
			return config.ResolveCLIStartupConfig(config.CLIStartupOverrides{
				DataDir:        opts.DataDir,
				Addr:           opts.Addr,
				Model:          opts.Model,
				PermissionMode: opts.PermissionMode,
			})
		},
	}

	base, err := config.ResolveCLIStartupConfig(config.CLIStartupOverrides{
		Model:          "current-model",
		PermissionMode: "read_only",
	})
	if err != nil {
		t.Fatalf("ResolveCLIStartupConfig(base) error = %v", err)
	}

	cfg, err := app.prepareDaemonConfig(Options{DaemonRef: record.ID}, base)
	if err != nil {
		t.Fatalf("prepareDaemonConfig() error = %v", err)
	}
	if cfg.Addr != record.Addr {
		t.Fatalf("cfg.Addr = %q, want %q", cfg.Addr, record.Addr)
	}
	if cfg.DataDir != record.DataDir {
		t.Fatalf("cfg.DataDir = %q, want %q", cfg.DataDir, record.DataDir)
	}
	if cfg.Model != record.Model {
		t.Fatalf("cfg.Model = %q, want %q", cfg.Model, record.Model)
	}
	if cfg.PermissionProfile != record.PermissionMode {
		t.Fatalf("cfg.PermissionProfile = %q, want %q", cfg.PermissionProfile, record.PermissionMode)
	}
	if cfg.DaemonID != record.ID {
		t.Fatalf("cfg.DaemonID = %q, want %q", cfg.DaemonID, record.ID)
	}
}

func TestRunSkillInstallBypassesDaemonStartup(t *testing.T) {
	home := t.TempDir()
	workspace := t.TempDir()
	source := filepath.Join(t.TempDir(), "demo-skill")
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	if err := os.MkdirAll(source, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(source, "SKILL.md"), []byte("---\nname: demo-skill\n---\nInstall me"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	prevWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	if err := os.Chdir(workspace); err != nil {
		t.Fatalf("Chdir() error = %v", err)
	}
	defer func() { _ = os.Chdir(prevWD) }()

	var stdout bytes.Buffer
	app := App{
		Stdout: &stdout,
		LoadConfig: func(Options) (config.Config, error) {
			t.Fatal("LoadConfig should not be called for skill commands")
			return config.Config{}, nil
		},
		EnsureDaemon: func(context.Context, config.Config) error {
			t.Fatal("EnsureDaemon should not be called for skill commands")
			return nil
		},
		NewClient: func(config.Config) RuntimeClient {
			t.Fatal("NewClient should not be called for skill commands")
			return nil
		},
		NewREPL: func(repl.Options) REPLRunner {
			t.Fatal("NewREPL should not be called for skill commands")
			return nil
		},
	}

	if err := app.Run(context.Background(), []string{"skill", "install", source, "--scope", "workspace"}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !strings.Contains(stdout.String(), "Installed skill demo-skill [workspace]") {
		t.Fatalf("stdout = %q, want install summary", stdout.String())
	}
	if _, err := os.Stat(filepath.Join(workspace, ".sesame", "skills", "demo-skill", "SKILL.md")); err != nil {
		t.Fatalf("installed skill missing: %v", err)
	}
}

type fakeRuntimeClient struct {
	status            client.StatusResponse
	selectedSessionID string
	workspaceSession  string
}

func (f *fakeRuntimeClient) Status(context.Context) (client.StatusResponse, error) {
	return f.status, nil
}

func (f *fakeRuntimeClient) ListSessions(context.Context) (types.ListSessionsResponse, error) {
	return types.ListSessionsResponse{}, nil
}

func (f *fakeRuntimeClient) SelectSession(_ context.Context, sessionID string) error {
	f.selectedSessionID = sessionID
	return nil
}

func (f *fakeRuntimeClient) SubmitTurn(context.Context, string, types.SubmitTurnRequest) (types.Turn, error) {
	return types.Turn{}, nil
}

func (f *fakeRuntimeClient) InterruptTurn(context.Context, string) error {
	return nil
}

func (f *fakeRuntimeClient) StreamEvents(context.Context, string, int64) (<-chan types.Event, error) {
	ch := make(chan types.Event)
	close(ch)
	return ch, nil
}

func (f *fakeRuntimeClient) GetTimeline(context.Context, string) (types.SessionTimelineResponse, error) {
	return types.SessionTimelineResponse{}, nil
}

func (f *fakeRuntimeClient) GetReportMailbox(context.Context, string) (types.SessionReportMailboxResponse, error) {
	return types.SessionReportMailboxResponse{}, nil
}

func (f *fakeRuntimeClient) GetRuntimeGraph(context.Context, string) (types.SessionRuntimeGraphResponse, error) {
	return types.SessionRuntimeGraphResponse{}, nil
}

func (f *fakeRuntimeClient) GetReportingOverview(context.Context, string) (types.ReportingOverview, error) {
	return types.ReportingOverview{}, nil
}

func (f *fakeRuntimeClient) ListCronJobs(context.Context, string) (types.ListScheduledJobsResponse, error) {
	return types.ListScheduledJobsResponse{}, nil
}

func (f *fakeRuntimeClient) GetCronJob(context.Context, string) (types.ScheduledJob, error) {
	return types.ScheduledJob{}, nil
}

func (f *fakeRuntimeClient) PauseCronJob(context.Context, string) (types.ScheduledJob, error) {
	return types.ScheduledJob{}, nil
}

func (f *fakeRuntimeClient) ResumeCronJob(context.Context, string) (types.ScheduledJob, error) {
	return types.ScheduledJob{}, nil
}

func (f *fakeRuntimeClient) DeleteCronJob(context.Context, string) error {
	return nil
}

func (f *fakeRuntimeClient) DecidePermission(context.Context, types.PermissionDecisionRequest) (types.PermissionDecisionResponse, error) {
	return types.PermissionDecisionResponse{}, nil
}

func (f *fakeRuntimeClient) FindOrCreateWorkspaceSession(context.Context, string) (string, bool, error) {
	if f.workspaceSession == "" {
		f.workspaceSession = "sess_workspace"
	}
	return f.workspaceSession, false, nil
}

type stubRunner struct{}

func (stubRunner) Run(context.Context, string) error {
	return nil
}
