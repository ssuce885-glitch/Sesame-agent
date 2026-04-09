package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	httpapi "go-agent/internal/api/http"
	"go-agent/internal/config"
	contextstate "go-agent/internal/context"
	"go-agent/internal/engine"
	"go-agent/internal/model"
	"go-agent/internal/permissions"
	"go-agent/internal/runtimegraph"
	"go-agent/internal/session"
	"go-agent/internal/store/artifacts"
	"go-agent/internal/store/sqlite"
	"go-agent/internal/stream"
	"go-agent/internal/task"
	"go-agent/internal/tools"
	"go-agent/internal/types"
)

type sessionRunnerAdapter struct {
	engine *engine.Engine
	sink   engine.EventSink
}

type storeAndBusSink struct {
	store *sqlite.Store
	bus   *stream.Bus
}

type runtimeWiring struct {
	contextManagerConfig contextstate.Config
	runtime              *contextstate.Runtime
	compactor            contextstate.Compactor
}

type taskEventSink struct {
	writer io.Writer
}

type agentTaskExecutor struct {
	runner *engine.Engine
}

func (s storeAndBusSink) Emit(ctx context.Context, event types.Event) error {
	seq, err := s.store.AppendEvent(ctx, event)
	if err != nil {
		return err
	}
	event.Seq = seq
	s.bus.Publish(event)
	switch event.Type {
	case types.EventTurnStarted:
		_ = s.store.UpdateTurnState(ctx, event.TurnID, types.TurnStateBuildingContext)
		_ = s.store.UpdateSessionState(ctx, event.SessionID, types.SessionStateRunning, event.TurnID)
	case types.EventTurnFailed:
		_ = s.store.UpdateTurnState(ctx, event.TurnID, types.TurnStateFailed)
		_ = s.store.UpdateSessionState(ctx, event.SessionID, types.SessionStateIdle, "")
	case types.EventTurnInterrupted:
		var payload map[string]string
		_ = json.Unmarshal(event.Payload, &payload)
		if payload["reason"] != "permission_requested" {
			_ = s.store.UpdateTurnState(ctx, event.TurnID, types.TurnStateInterrupted)
			_ = s.store.UpdateSessionState(ctx, event.SessionID, types.SessionStateIdle, "")
		}
	}
	return nil
}

func (s storeAndBusSink) FinalizeTurn(ctx context.Context, usage *types.TurnUsage, events []types.Event) error {
	persisted, err := s.store.FinalizeTurn(ctx, usage, events)
	if err != nil {
		return err
	}
	for _, event := range persisted {
		s.bus.Publish(event)
	}
	if len(persisted) > 0 {
		last := persisted[len(persisted)-1]
		_ = s.store.UpdateTurnState(ctx, last.TurnID, types.TurnStateCompleted)
		_ = s.store.UpdateSessionState(ctx, last.SessionID, types.SessionStateIdle, "")
	}
	return nil
}

func (a sessionRunnerAdapter) RunTurn(ctx context.Context, in session.RunInput) error {
	err := a.engine.RunTurn(ctx, engine.Input{
		Session: in.Session,
		Turn: types.Turn{
			ID:           in.TurnID,
			SessionID:    in.Session.ID,
			ClientTurnID: "",
			UserMessage:  in.Message,
		},
		Sink:   a.sink,
		Resume: in.Resume,
	})
	return err
}

func (s taskEventSink) Emit(_ context.Context, event types.Event) error {
	switch event.Type {
	case types.EventAssistantDelta:
		if s.writer == nil {
			return nil
		}

		var payload types.AssistantDeltaPayload
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			return err
		}
		_, err := io.WriteString(s.writer, payload.Text)
		return err
	case types.EventTurnFailed:
		var payload types.TurnFailedPayload
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			return err
		}
		if payload.Message == "" {
			return errors.New("turn failed")
		}
		return errors.New(payload.Message)
	default:
		return nil
	}
}

func buildAgentTaskExecutor(runner *engine.Engine) task.AgentExecutor {
	if runner == nil {
		return nil
	}
	return agentTaskExecutor{runner: runner}
}

func (a agentTaskExecutor) RunTask(ctx context.Context, workspaceRoot string, prompt string, sink io.Writer) error {
	if a.runner == nil {
		return errors.New("engine runner is not configured")
	}

	sessionID := types.NewID("task_session")
	turnID := types.NewID("task_turn")
	return a.runner.RunTurn(ctx, engine.Input{
		Session: types.Session{
			ID:            sessionID,
			WorkspaceRoot: workspaceRoot,
		},
		Turn: types.Turn{
			ID:          turnID,
			SessionID:   sessionID,
			UserMessage: prompt,
		},
		Sink: taskEventSink{writer: sink},
	})
}

func ensureDataDir(path string) error {
	return os.MkdirAll(path, 0o755)
}

func writePIDFile(path string) error {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	return os.WriteFile(path, []byte(fmt.Sprintf("%d\n", os.Getpid())), 0o644)
}

func Run(ctx context.Context) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if missing := config.MissingSetupFields(cfg); len(missing) > 0 {
		configPath, _ := config.GlobalConfigPath()
		return fmt.Errorf("sesame daemon is not configured: missing %s in %s", strings.Join(missing, ", "), configPath)
	}

	basePrompt, err := cfg.ResolveSystemPrompt()
	if err != nil {
		return err
	}

	if err := ensureDataDir(cfg.DataDir); err != nil {
		return err
	}
	if err := writePIDFile(cfg.Paths.PIDFile); err != nil {
		return err
	}
	defer os.Remove(cfg.Paths.PIDFile)

	store, err := sqlite.Open(cfg.Paths.DatabaseFile)
	if err != nil {
		return err
	}
	defer store.Close()
	runtimeService := runtimegraph.NewService(store)

	_, err = artifacts.New(filepath.Join(cfg.DataDir, "artifacts"))
	if err != nil {
		return err
	}

	bus := stream.NewBus()
	registry := tools.NewRegistry()
	configureRuntimeGuardrails(cfg)
	permissionEngine := buildPermissionEngine(cfg)
	modelClient, err := model.NewFromConfig(cfg)
	if err != nil {
		return err
	}
	wiring := buildRuntimeWiring(cfg, modelClient)
	runner := engine.NewWithRuntime(
		modelClient,
		registry,
		permissionEngine,
		store,
		contextstate.NewManager(wiring.contextManagerConfig),
		wiring.runtime,
		wiring.compactor,
		engine.RuntimeMetadata{
			Provider: cfg.ModelProvider,
			Model:    cfg.Model,
		},
		buildMaxToolSteps(cfg),
	)
	runner.SetBaseSystemPrompt(basePrompt)
	runner.SetGlobalConfigRoot(cfg.Paths.GlobalRoot)
	runner.SetSessionMemoryAsync(true)
	runner.SetMaxWorkspacePromptBytes(cfg.MaxWorkspacePromptBytes)
	runner.SetRuntimeService(runtimeService)
	taskManager := task.NewManager(task.Config{
		MaxConcurrentTasks: cfg.MaxConcurrentTasks,
		TaskOutputMaxBytes: cfg.TaskOutputMaxBytes,
	}, nil, buildAgentTaskExecutor(runner))
	taskManager.SetRemoteConfig(task.RemoteExecutorConfig{
		ShimCommand:    cfg.RemoteExecutorShimCommand,
		TimeoutSeconds: cfg.RemoteExecutorTimeoutSeconds,
	})
	runner.SetTaskManager(taskManager)
	manager := session.NewManager(sessionRunnerAdapter{
		engine: runner,
		sink: storeAndBusSink{
			store: store,
			bus:   bus,
		},
	})
	if err := recoverRuntimeState(ctx, store, manager); err != nil {
		return err
	}

	handler := httpapi.NewRouter(httpapi.Dependencies{
		Bus:         bus,
		Store:       store,
		Manager:     manager,
		Status:      buildStatusPayload(cfg),
		ConsoleRoot: filepath.Join("web", "console", "dist"),
	})

	slog.Info("sesame daemon listening", "addr", cfg.Addr)
	return http.ListenAndServe(cfg.Addr, handler)
}

func configureRuntimeGuardrails(cfg config.Config) {
	tools.SetShellCommandGuardrails(cfg.MaxShellOutputBytes, cfg.ShellTimeoutSeconds)
	tools.SetFileWriteMaxBytes(cfg.MaxFileWriteBytes)
}

func buildPermissionEngine(cfg config.Config) *permissions.Engine {
	return permissions.NewEngine(cfg.PermissionProfile)
}

func buildContextManagerConfig(cfg config.Config) contextstate.Config {
	return contextstate.Config{
		MaxRecentItems:             cfg.MaxRecentItems,
		MaxEstimatedTokens:         cfg.MaxEstimatedTokens,
		CompactionThreshold:        cfg.CompactionThreshold,
		MicrocompactBytesThreshold: cfg.MicrocompactBytesThreshold,
	}
}

func buildMaxToolSteps(cfg config.Config) int {
	return cfg.MaxToolSteps
}

func buildStatusPayload(cfg config.Config) httpapi.StatusPayload {
	return httpapi.StatusPayload{
		DaemonID:             cfg.DaemonID,
		Provider:             cfg.ModelProvider,
		Model:                cfg.Model,
		PermissionProfile:    cfg.PermissionProfile,
		ProviderCacheProfile: cfg.ProviderCacheProfile,
		ConfigFingerprint:    cfg.ConfigFingerprint,
		PID:                  os.Getpid(),
	}
}

func buildRuntimeWiring(cfg config.Config, modelClient model.StreamingClient) runtimeWiring {
	return runtimeWiring{
		contextManagerConfig: buildContextManagerConfig(cfg),
		runtime:              contextstate.NewRuntime(cfg.CacheExpirySeconds, cfg.MaxCompactionPasses),
		compactor:            contextstate.NewPromptedCompactor(modelClient, cfg.Model),
	}
}

func recoverRuntimeState(ctx context.Context, store *sqlite.Store, manager *session.Manager) error {
	sessions, err := store.ListSessions(ctx)
	if err != nil {
		return err
	}
	if manager != nil {
		for _, sessionRow := range sessions {
			manager.RegisterSession(sessionRow)
		}
	}
	if err := ensureSelectedSession(ctx, store, sessions); err != nil {
		return err
	}
	resumedTurns, err := resumeResolvedContinuations(ctx, store, manager)
	if err != nil {
		return err
	}

	running, err := store.ListRunningTurns(ctx)
	if err != nil {
		return err
	}

	for _, turn := range running {
		if _, ok := resumedTurns[turn.ID]; ok {
			continue
		}
		if turn.State == types.TurnStateAwaitingPermission {
			continue
		}
		if err := store.MarkTurnInterrupted(ctx, turn.ID); err != nil {
			return err
		}

		event, err := types.NewEvent(turn.SessionID, turn.ID, types.EventTurnInterrupted, map[string]string{
			"reason": "daemon_restart",
		})
		if err != nil {
			return err
		}
		if _, err := store.AppendEvent(ctx, event); err != nil {
			return err
		}
	}

	return nil
}

func resumeResolvedContinuations(ctx context.Context, store *sqlite.Store, manager *session.Manager) (map[string]struct{}, error) {
	resumed := make(map[string]struct{})
	if store == nil || manager == nil {
		return resumed, nil
	}

	continuations, err := store.ListPendingTurnContinuations(ctx)
	if err != nil {
		return nil, err
	}

	for _, continuation := range continuations {
		if strings.TrimSpace(continuation.PermissionRequestID) == "" {
			continue
		}
		request, ok, err := store.GetPermissionRequest(ctx, continuation.PermissionRequestID)
		if err != nil {
			return nil, err
		}
		if !ok || request.Status == types.PermissionRequestStatusRequested || strings.TrimSpace(request.Decision) == "" {
			continue
		}

		turn, ok, err := store.GetTurn(ctx, continuation.TurnID)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		sessionRow, ok, err := store.GetSession(ctx, continuation.SessionID)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}

		effectiveProfile := sessionRow.PermissionProfile
		if types.PermissionDecisionGrantsProfile(request.Decision) && strings.TrimSpace(request.RequestedProfile) != "" {
			effectiveProfile = request.RequestedProfile
		}
		decisionScope := strings.TrimSpace(request.DecisionScope)
		if decisionScope == "" {
			decisionScope = request.Decision
		}
		if err := store.UpdateTurnState(ctx, turn.ID, types.TurnStateLoopContinue); err != nil {
			return nil, err
		}
		if err := store.UpdateSessionState(ctx, sessionRow.ID, types.SessionStateRunning, turn.ID); err != nil {
			return nil, err
		}

		resume := &types.TurnResume{
			ContinuationID:             continuation.ID,
			PermissionRequestID:        request.ID,
			ToolRunID:                  continuation.ToolRunID,
			ToolCallID:                 continuation.ToolCallID,
			ToolName:                   continuation.ToolName,
			RequestedProfile:           continuation.RequestedProfile,
			Reason:                     continuation.Reason,
			Decision:                   request.Decision,
			DecisionScope:              decisionScope,
			EffectivePermissionProfile: effectiveProfile,
			RunID:                      continuation.RunID,
			TaskID:                     continuation.TaskID,
		}
		if _, err := manager.ResumeTurn(ctx, sessionRow.ID, session.ResumeTurnInput{
			TurnID:  turn.ID,
			Message: turn.UserMessage,
			Resume:  resume,
		}); err != nil {
			return nil, err
		}

		now := time.Now().UTC()
		continuation.State = types.TurnContinuationStateResumed
		continuation.Decision = request.Decision
		continuation.DecisionScope = decisionScope
		continuation.UpdatedAt = now
		if err := store.UpsertTurnContinuation(ctx, continuation); err != nil {
			return nil, err
		}
		if strings.TrimSpace(continuation.ToolRunID) != "" {
			toolRun, found, err := store.GetToolRun(ctx, continuation.ToolRunID)
			if err != nil {
				return nil, err
			}
			if found {
				toolRun.PermissionRequestID = request.ID
				toolRun.UpdatedAt = now
				toolRun.CompletedAt = now
				toolRun.OutputJSON = marshalRecoveredPermissionToolRunOutput(request, effectiveProfile)
				if request.Decision == types.PermissionDecisionDeny {
					toolRun.State = types.ToolRunStateFailed
					toolRun.Error = "permission denied"
				} else {
					toolRun.State = types.ToolRunStateCompleted
					toolRun.Error = ""
				}
				if err := store.UpsertToolRun(ctx, toolRun); err != nil {
					return nil, err
				}
			}
		}

		resumed[turn.ID] = struct{}{}
	}

	return resumed, nil
}

func marshalRecoveredPermissionToolRunOutput(request types.PermissionRequest, effectiveProfile string) string {
	payload, _ := json.Marshal(map[string]any{
		"status":                       request.Status,
		"decision":                     request.Decision,
		"decision_scope":               request.DecisionScope,
		"requested_profile":            request.RequestedProfile,
		"effective_permission_profile": effectiveProfile,
		"reason":                       request.Reason,
	})
	return string(payload)
}

func ensureSelectedSession(ctx context.Context, store *sqlite.Store, sessions []types.Session) error {
	if len(sessions) == 0 {
		return nil
	}

	selected, ok, err := store.GetSelectedSessionID(ctx)
	if err != nil {
		return err
	}
	if ok {
		for _, sessionRow := range sessions {
			if sessionRow.ID == selected {
				return nil
			}
		}
	}

	return store.SetSelectedSessionID(ctx, sessions[0].ID)
}
