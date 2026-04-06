package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	httpapi "go-agent/internal/api/http"
	"go-agent/internal/config"
	contextstate "go-agent/internal/context"
	"go-agent/internal/engine"
	"go-agent/internal/model"
	"go-agent/internal/permissions"
	sessionstate "go-agent/internal/session"
	"go-agent/internal/store/sqlite"
	"go-agent/internal/tools"
	"go-agent/internal/types"
)

type noopTestRunner struct{}

func (noopTestRunner) RunTurn(context.Context, sessionstate.RunInput) error { return nil }

func TestEnsureDataDirCreatesMissingDirectory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "runtime", "data")

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected %q to not exist before ensureDataDir, err = %v", path, err)
	}

	if err := ensureDataDir(path); err != nil {
		t.Fatalf("ensureDataDir() error = %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("%q is not a directory", path)
	}
}

func TestBuildRuntimeWiringUsesConfig(t *testing.T) {
	cfg := config.Config{
		PermissionProfile:          "trusted_local",
		MaxToolSteps:               11,
		MaxShellOutputBytes:        18,
		ShellTimeoutSeconds:        4,
		MaxFileWriteBytes:          23,
		MaxRecentItems:             2,
		CompactionThreshold:        7,
		MaxEstimatedTokens:         42,
		CacheExpirySeconds:         321,
		MaxCompactionPasses:        7,
		MicrocompactBytesThreshold: 13,
		Model:                      "compact-model",
	}

	permissionEngine := buildPermissionEngine(cfg)
	if permissionEngine.Decide("file_write") != permissions.DecisionAllow {
		t.Fatal("buildPermissionEngine() did not honor trusted_local profile for file_write")
	}
	if permissionEngine.Decide("shell_command") != permissions.DecisionAllow {
		t.Fatal("buildPermissionEngine() did not honor trusted_local profile for shell_command")
	}

	ctxCfg := buildContextManagerConfig(cfg)
	if ctxCfg != (contextstate.Config{
		MaxRecentItems:             2,
		MaxEstimatedTokens:         42,
		CompactionThreshold:        7,
		MicrocompactBytesThreshold: 13,
	}) {
		t.Fatalf("buildContextManagerConfig() = %#v, want cfg-derived context settings", ctxCfg)
	}

	wiring := buildRuntimeWiring(cfg, model.NewFakeStreaming(nil))
	if wiring.contextManagerConfig != ctxCfg {
		t.Fatalf("buildRuntimeWiring().contextManagerConfig = %#v, want %#v", wiring.contextManagerConfig, ctxCfg)
	}
	if wiring.runtime == nil {
		t.Fatal("buildRuntimeWiring().runtime is nil")
	}
	if _, ok := wiring.compactor.(*contextstate.PromptedCompactor); !ok {
		t.Fatalf("buildRuntimeWiring().compactor = %T, want *contextstate.PromptedCompactor", wiring.compactor)
	}

	if got := buildMaxToolSteps(cfg); got != 11 {
		t.Fatalf("buildMaxToolSteps() = %d, want 11", got)
	}
}

func TestBuildStatusPayloadIncludesProviderCacheProfile(t *testing.T) {
	cfg := config.Config{
		ModelProvider:        "openai_compatible",
		Model:                "glm-4-7-251222",
		PermissionProfile:    "trusted_local",
		ProviderCacheProfile: "ark_responses",
	}

	got := buildStatusPayload(cfg)
	want := httpapi.StatusPayload{
		Provider:             "openai_compatible",
		Model:                "glm-4-7-251222",
		PermissionProfile:    "trusted_local",
		ProviderCacheProfile: "ark_responses",
	}
	if got != want {
		t.Fatalf("buildStatusPayload() = %#v, want %#v", got, want)
	}
}

func TestConfigureRuntimeGuardrailsAffectsTools(t *testing.T) {
	t.Cleanup(func() {
		tools.SetShellCommandGuardrails(256, 30)
		tools.SetFileWriteMaxBytes(1 << 20)
	})

	configureRuntimeGuardrails(config.Config{
		MaxShellOutputBytes: 12,
		ShellTimeoutSeconds: 30,
		MaxFileWriteBytes:   7,
	})

	workspace := t.TempDir()
	registry := tools.NewRegistry()

	t.Run("file write respects configured limit", func(t *testing.T) {
		_, err := registry.Execute(context.Background(), tools.Call{
			Name:  "file_write",
			Input: map[string]any{"path": filepath.Join(workspace, "too-big.txt"), "content": "12345678"},
		}, tools.ExecContext{
			WorkspaceRoot:    workspace,
			PermissionEngine: permissions.NewEngine("trusted_local"),
		})
		if err == nil || !strings.Contains(err.Error(), "exceeds max size") {
			t.Fatalf("file_write error = %v, want size limit error", err)
		}
	})

	t.Run("shell command respects configured output limit and workspace", func(t *testing.T) {
		tools.SetShellCommandGuardrails(128, 30)
		result, err := registry.Execute(context.Background(), tools.Call{
			Name:  "shell_command",
			Input: map[string]any{"command": "echo %cd%"},
		}, tools.ExecContext{
			WorkspaceRoot:    workspace,
			PermissionEngine: permissions.NewEngine("trusted_local"),
		})
		if err != nil {
			t.Fatalf("shell_command error = %v", err)
		}
		if !strings.Contains(result.Text, workspace) {
			t.Fatalf("shell_command output = %q, want workspace root %q", result.Text, workspace)
		}

		tools.SetShellCommandGuardrails(4, 30)
		result, err = registry.Execute(context.Background(), tools.Call{
			Name:  "shell_command",
			Input: map[string]any{"command": "echo 1234567890"},
		}, tools.ExecContext{
			WorkspaceRoot:    workspace,
			PermissionEngine: permissions.NewEngine("trusted_local"),
		})
		if err != nil {
			t.Fatalf("shell_command error = %v", err)
		}
		if len(result.Text) != 4 {
			t.Fatalf("len(shell_command output) = %d, want 4", len(result.Text))
		}
	})
}

func TestTaskEventSinkRoutesAssistantDeltaAndTurnFailures(t *testing.T) {
	var output bytes.Buffer
	sink := taskEventSink{writer: &output}

	assistantEvent, err := types.NewEvent("sess_1", "turn_1", types.EventAssistantDelta, types.AssistantDeltaPayload{
		Text: "agent output",
	})
	if err != nil {
		t.Fatalf("NewEvent(assistant.delta) error = %v", err)
	}
	if err := sink.Emit(context.Background(), assistantEvent); err != nil {
		t.Fatalf("Emit(assistant.delta) error = %v", err)
	}
	if output.String() != "agent output" {
		t.Fatalf("assistant output = %q, want %q", output.String(), "agent output")
	}

	failedEvent, err := types.NewEvent("sess_1", "turn_1", types.EventTurnFailed, types.TurnFailedPayload{
		Message: "task failed",
	})
	if err != nil {
		t.Fatalf("NewEvent(turn.failed) error = %v", err)
	}
	err = sink.Emit(context.Background(), failedEvent)
	if err == nil || !strings.Contains(err.Error(), "task failed") {
		t.Fatalf("Emit(turn.failed) error = %v, want task failed", err)
	}
}

func TestBuildAgentTaskExecutorRunsPromptThroughEngine(t *testing.T) {
	modelClient := model.NewFakeStreaming([][]model.StreamEvent{{
		{Kind: model.StreamEventTextDelta, TextDelta: "agent reply"},
		{Kind: model.StreamEventMessageEnd},
	}})
	runner := engine.New(
		modelClient,
		tools.NewRegistry(),
		permissions.NewEngine("trusted_local"),
		nil,
		nil,
		nil,
		8,
	)

	executor := buildAgentTaskExecutor(runner)
	if executor == nil {
		t.Fatal("buildAgentTaskExecutor() = nil, want executor")
	}

	var output bytes.Buffer
	workspace := t.TempDir()
	if err := executor.RunTask(context.Background(), workspace, "summarize workspace", &output); err != nil {
		t.Fatalf("RunTask() error = %v", err)
	}
	if output.String() != "agent reply" {
		t.Fatalf("RunTask() output = %q, want %q", output.String(), "agent reply")
	}

	lastRequest := modelClient.LastRequest()
	if lastRequest.UserMessage != "summarize workspace" {
		t.Fatalf("model request message = %q, want %q", lastRequest.UserMessage, "summarize workspace")
	}
}

func TestRecoverRunningTurnsMarksInterruptedOnStartup(t *testing.T) {
	store, err := sqlite.Open(filepath.Join(t.TempDir(), "agentd.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	session := types.Session{
		ID:            "sess_1",
		WorkspaceRoot: t.TempDir(),
		State:         types.SessionStateRunning,
		ActiveTurnID:  "turn_running",
		CreatedAt:     time.Now().UTC(),
		UpdatedAt:     time.Now().UTC(),
	}
	if err := store.InsertSession(context.Background(), session); err != nil {
		t.Fatalf("InsertSession() error = %v", err)
	}

	turn := types.Turn{
		ID:          "turn_running",
		SessionID:   "sess_1",
		State:       types.TurnStateModelStreaming,
		UserMessage: "continue",
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}
	if err := store.InsertTurn(context.Background(), turn); err != nil {
		t.Fatalf("InsertTurn() error = %v", err)
	}

	if err := recoverRuntimeState(context.Background(), store, sessionstate.NewManager(noopTestRunner{})); err != nil {
		t.Fatalf("recoverRuntimeState() error = %v", err)
	}

	events, err := store.ListSessionEvents(context.Background(), "sess_1", 0)
	if err != nil {
		t.Fatalf("ListSessionEvents() error = %v", err)
	}

	found := false
	for _, event := range events {
		if event.Type == types.EventTurnInterrupted {
			found = true
			if !strings.Contains(string(event.Payload), "daemon_restart") {
				t.Fatalf("payload = %s, want daemon_restart reason", event.Payload)
			}
		}
	}
	if !found {
		t.Fatalf("events = %+v, want turn.interrupted", events)
	}
}

func TestRecoverRuntimeStateRegistersPersistedSessions(t *testing.T) {
	store, err := sqlite.Open(filepath.Join(t.TempDir(), "agentd.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	now := time.Now().UTC()
	sessionRow := types.Session{
		ID:            "sess_restore",
		WorkspaceRoot: "D:/work/demo",
		State:         types.SessionStateIdle,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := store.InsertSession(context.Background(), sessionRow); err != nil {
		t.Fatalf("InsertSession() error = %v", err)
	}

	manager := sessionstate.NewManager(noopTestRunner{})
	if err := recoverRuntimeState(context.Background(), store, manager); err != nil {
		t.Fatalf("recoverRuntimeState() error = %v", err)
	}

	if _, ok := manager.GetRuntimeState("sess_restore"); !ok {
		t.Fatal("restored session was not registered in manager")
	}
}

func TestRecoverRuntimeStateBackfillsSelectedSessionToMostRecent(t *testing.T) {
	store, err := sqlite.Open(filepath.Join(t.TempDir(), "agentd.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	older := time.Date(2026, 4, 4, 8, 0, 0, 0, time.UTC)
	newer := older.Add(5 * time.Minute)
	if err := store.InsertSession(context.Background(), types.Session{
		ID:            "sess_old",
		WorkspaceRoot: "D:/work/old",
		State:         types.SessionStateIdle,
		CreatedAt:     older,
		UpdatedAt:     older,
	}); err != nil {
		t.Fatalf("InsertSession(old) error = %v", err)
	}
	if err := store.InsertSession(context.Background(), types.Session{
		ID:            "sess_new",
		WorkspaceRoot: "D:/work/new",
		State:         types.SessionStateIdle,
		CreatedAt:     newer,
		UpdatedAt:     newer,
	}); err != nil {
		t.Fatalf("InsertSession(new) error = %v", err)
	}

	manager := sessionstate.NewManager(noopTestRunner{})
	if err := recoverRuntimeState(context.Background(), store, manager); err != nil {
		t.Fatalf("recoverRuntimeState() error = %v", err)
	}

	selected, ok, err := store.GetSelectedSessionID(context.Background())
	if err != nil {
		t.Fatalf("GetSelectedSessionID() error = %v", err)
	}
	if !ok || selected != "sess_new" {
		t.Fatalf("selected = %q, %v, want sess_new true", selected, ok)
	}
}
