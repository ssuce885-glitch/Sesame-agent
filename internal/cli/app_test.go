package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"go-agent/internal/cli/client"
	"go-agent/internal/cli/repl"
	"go-agent/internal/config"
	"go-agent/internal/types"
)

func TestRunStatusModePrintsDaemonStatus(t *testing.T) {
	var stdout bytes.Buffer
	fakeClient := &fakeRuntimeClient{
		status: client.StatusResponse{Status: "ok", Model: "gpt-5", PermissionProfile: "trusted_local"},
	}

	app := App{
		Stdout: &stdout,
		LoadOptions: func([]string) (Options, error) {
			return Options{ShowStatus: true}, nil
		},
		LoadConfig: func(Options) (config.Config, error) {
			return config.Config{Addr: "127.0.0.1:4317", DataDir: "E:/tmp/agentd", Model: "gpt-5", PermissionProfile: "trusted_local"}, nil
		},
		EnsureDaemon: func(context.Context, config.Config) error { return nil },
		NewClient:    func(config.Config) RuntimeClient { return fakeClient },
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
}

func TestRunInteractiveModeResumesSessionAndStartsREPL(t *testing.T) {
	var gotOptions repl.Options
	fakeClient := &fakeRuntimeClient{}

	app := App{
		Stdout: &bytes.Buffer{},
		Stderr: &bytes.Buffer{},
		Stdin:  strings.NewReader(""),
		LoadOptions: func([]string) (Options, error) {
			return Options{ResumeID: "sess_resume", InitialPrompt: "hello"}, nil
		},
		LoadConfig: func(Options) (config.Config, error) {
			return config.Config{Addr: "127.0.0.1:4317", DataDir: "E:/tmp/agentd", Model: "gpt-5", PermissionProfile: "trusted_local"}, nil
		},
		EnsureDaemon: func(context.Context, config.Config) error { return nil },
		NewClient:    func(config.Config) RuntimeClient { return fakeClient },
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

func (f *fakeRuntimeClient) StreamEvents(context.Context, string, int64) (<-chan types.Event, error) {
	ch := make(chan types.Event)
	close(ch)
	return ch, nil
}

func (f *fakeRuntimeClient) GetTimeline(context.Context, string) (types.SessionTimelineResponse, error) {
	return types.SessionTimelineResponse{}, nil
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
