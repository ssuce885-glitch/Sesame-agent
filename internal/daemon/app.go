package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	httpapi "go-agent/internal/api/http"
	"go-agent/internal/automation"
	"go-agent/internal/config"
	contextstate "go-agent/internal/context"
	"go-agent/internal/engine"
	"go-agent/internal/model"
	"go-agent/internal/permissions"
	"go-agent/internal/reporting"
	rolectx "go-agent/internal/roles"
	"go-agent/internal/scheduler"
	"go-agent/internal/session"
	"go-agent/internal/sessionbinding"
	"go-agent/internal/store/artifacts"
	"go-agent/internal/store/sqlite"
	"go-agent/internal/stream"
	"go-agent/internal/task"
	"go-agent/internal/tools"
	"go-agent/internal/types"
)

type storeAndBusSink struct {
	store *sqlite.Store
	bus   *stream.Bus
}

type runtimeWiring struct {
	contextManagerConfig contextstate.Config
	runtime              *contextstate.Runtime
	compactor            contextstate.Compactor
}

type agentTaskExecutor struct {
	runner  *engine.Engine
	store   *sqlite.Store
	manager *session.Manager
	now     func() time.Time
}

type taskTerminalNotifier struct {
	store     *sqlite.Store
	bus       *stream.Bus
	scheduler *scheduler.Service
	reporting *reporting.Service
	delivery  *automation.DeliveryService
	watcher   automation.DispatchWatcherSyncer
	manager   *session.Manager
	now       func() time.Time
}

func (s storeAndBusSink) Emit(ctx context.Context, event types.Event) error {
	persisted, err := s.store.AppendEventWithState(ctx, event)
	if err != nil {
		return err
	}
	s.bus.Publish(persisted)
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
	return nil
}

func buildAgentTaskExecutor(runner *engine.Engine, stores ...*sqlite.Store) *agentTaskExecutor {
	if runner == nil {
		return nil
	}
	var store *sqlite.Store
	if len(stores) > 0 {
		store = stores[0]
	}
	return &agentTaskExecutor{
		runner: runner,
		store:  store,
	}
}

func buildTaskTerminalNotifier(store *sqlite.Store, bus *stream.Bus, workspaceRoot string) *taskTerminalNotifier {
	if store == nil || bus == nil {
		return nil
	}
	reportingService := reporting.NewService(store)
	reportingService.SetWorkspaceRoot(workspaceRoot)
	reportingService.SetReportReadySink(func(ctx context.Context, sessionID, turnID string, item types.ReportMailboxItem) error {
		sessionID = strings.TrimSpace(sessionID)
		if sessionID == "" {
			return nil
		}
		eventSink := storeAndBusSink{store: store, bus: bus}
		event, err := types.NewEvent(sessionID, turnID, types.EventReportReady, item)
		if err != nil {
			return err
		}
		return eventSink.Emit(ctx, event)
	})
	return &taskTerminalNotifier{
		store:     store,
		bus:       bus,
		reporting: reportingService,
	}
}

func (s combinedEventSink) Emit(ctx context.Context, event types.Event) error {
	if s.primary != nil {
		if err := s.primary.Emit(ctx, event); err != nil {
			return err
		}
	}
	if s.observer != nil {
		if err := s.observer.Emit(ctx, event); err != nil {
			return err
		}
	}
	return nil
}

func (s combinedEventSink) FinalizeTurn(ctx context.Context, usage *types.TurnUsage, events []types.Event) error {
	if s.finalizer == nil {
		return nil
	}
	return s.finalizer.FinalizeTurn(ctx, usage, events)
}

func mustParseDetectorSignalForPrompt(incident types.AutomationIncident) types.AutomationDetectorSignal {
	detectorSignal, err := automation.ParseAutomationDetectorSignalPayload(incident.Payload)
	if err != nil {
		return types.AutomationDetectorSignal{
			Summary: strings.TrimSpace(incident.Summary),
			Facts:   map[string]any{},
		}
	}
	return detectorSignal
}

func (a agentTaskExecutor) RunTask(ctx context.Context, taskID string, workspaceRoot string, prompt string, activatedSkillNames []string, observer task.AgentTaskObserver) error {
	if a.runner == nil {
		return errors.New("engine runner is not configured")
	}

	sessionID := types.NewID("task_session")
	turnID := types.NewID("task_turn")
	taskCtx := sessionbinding.WithContextBinding(ctx, taskContextBinding(sessionID))
	if err := a.prepareTaskRun(taskCtx, sessionID, turnID, workspaceRoot, prompt); err != nil {
		return err
	}
	sink := &taskEventSink{observer: observer}
	if observer != nil {
		if err := observer.SetRunContext(sessionID, turnID); err != nil {
			return err
		}
	}
	if err := a.runner.RunTurn(taskCtx, engine.Input{
		Session: types.Session{
			ID:            sessionID,
			WorkspaceRoot: workspaceRoot,
		},
		Turn: types.Turn{
			ID:          turnID,
			SessionID:   sessionID,
			Kind:        types.TurnKindUserMessage,
			UserMessage: prompt,
		},
		TaskID:              strings.TrimSpace(taskID),
		Sink:                sink,
		ActivatedSkillNames: append([]string(nil), activatedSkillNames...),
	}); err != nil {
		return err
	}
	if observer == nil {
		return nil
	}
	finalText := sink.FinalText()
	if strings.TrimSpace(finalText) == "" {
		return nil
	}
	return observer.SetFinalText(finalText)
}

func (a agentTaskExecutor) prepareTaskRun(ctx context.Context, sessionID, turnID, workspaceRoot, prompt string) error {
	if a.store == nil {
		return nil
	}
	now := a.currentTime()
	sessionRow := types.Session{
		ID:                sessionID,
		WorkspaceRoot:     strings.TrimSpace(workspaceRoot),
		PermissionProfile: "read_only",
		State:             types.SessionStateIdle,
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	if existing, ok, err := a.store.GetSession(ctx, sessionRow.ID); err != nil {
		return err
	} else if ok {
		sessionRow = existing
	} else if err := a.store.InsertSession(ctx, sessionRow); err != nil {
		return err
	}
	turnRow := types.Turn{
		ID:          turnID,
		SessionID:   sessionRow.ID,
		Kind:        types.TurnKindUserMessage,
		State:       types.TurnStateCreated,
		UserMessage: prompt,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if _, ok, err := a.store.GetTurn(ctx, turnRow.ID); err != nil {
		return err
	} else if !ok {
		if err := a.store.InsertTurn(ctx, turnRow); err != nil {
			return err
		}
	}
	if err := a.ensureTaskContextHead(ctx, sessionRow); err != nil {
		return err
	}
	if a.manager != nil {
		a.manager.RegisterSession(sessionRow)
	}
	return nil
}

func (a agentTaskExecutor) ensureTaskContextHead(ctx context.Context, sessionRow types.Session) error {
	if a.store == nil {
		return nil
	}
	if headID, ok, err := a.store.GetCurrentContextHeadID(ctx); err != nil {
		return err
	} else if ok {
		head, found, err := a.store.GetContextHead(ctx, headID)
		if err != nil {
			return err
		}
		if found && head.SessionID == sessionRow.ID {
			return a.store.AssignTurnsWithoutHead(ctx, sessionRow.ID, head.ID)
		}
	}

	now := a.currentTime()
	head := types.ContextHead{
		ID:         types.NewID("head"),
		SessionID:  sessionRow.ID,
		SourceKind: types.ContextHeadSourceBootstrap,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if err := a.store.InsertContextHead(ctx, head); err != nil {
		return err
	}
	if err := a.store.AssignTurnsWithoutHead(ctx, sessionRow.ID, head.ID); err != nil {
		return err
	}
	return a.store.SetCurrentContextHeadID(ctx, head.ID)
}

func taskContextBinding(sessionID string) string {
	return "task:" + strings.TrimSpace(sessionID)
}

func (a agentTaskExecutor) currentTime() time.Time {
	if a.now != nil {
		return a.now().UTC()
	}
	return time.Now().UTC()
}

func (n taskTerminalNotifier) NotifyTaskTerminal(ctx context.Context, completed task.Task) error {
	if n.store == nil || strings.TrimSpace(completed.ID) == "" {
		return nil
	}

	now := n.currentTime()
	runtimeTask, ok, err := n.store.GetTaskRecord(ctx, completed.ID)
	if err != nil {
		return err
	}
	if ok {
		runtimeTask.State = runtimeTaskStateFromTaskStatus(completed.Status)
		runtimeTask.Title = firstNonEmptyTrimmed(runtimeTask.Title, completed.Command, completed.ExecutionTaskID, completed.ID)
		runtimeTask.Description = firstNonEmptyTrimmed(completed.Description, runtimeTask.Description)
		runtimeTask.Owner = firstNonEmptyTrimmed(completed.Owner, runtimeTask.Owner)
		runtimeTask.Kind = firstNonEmptyTrimmed(completed.Kind, runtimeTask.Kind)
		runtimeTask.ExecutionTaskID = firstNonEmptyTrimmed(runtimeTask.ExecutionTaskID, completed.ExecutionTaskID, completed.ID)
		runtimeTask.WorktreeID = firstNonEmptyTrimmed(completed.WorktreeID, runtimeTask.WorktreeID)
		runtimeTask.UpdatedAt = now
		if err := n.store.UpsertTaskRecord(ctx, runtimeTask); err != nil {
			return err
		}
	}
	if n.scheduler != nil {
		if err := n.scheduler.RecordTaskTerminal(ctx, completed); err != nil {
			return err
		}
	}
	if err := n.reconcileAutomationDispatchTask(ctx, completed); err != nil {
		return err
	}

	updatedBlock := timelineBlockFromCompletedTask(completed, runtimeTask, ok)
	eventSink := storeAndBusSink{store: n.store, bus: n.bus}
	if strings.TrimSpace(completed.ParentSessionID) != "" {
		taskEvent, err := types.NewEvent(completed.ParentSessionID, completed.ParentTurnID, types.EventTaskUpdated, updatedBlock)
		if err != nil {
			return err
		}
		if err := eventSink.Emit(ctx, taskEvent); err != nil {
			return err
		}
	}

	if reporting.ShouldQueueTaskReport(completed) {
		var (
			reportItems []types.ReportMailboxItem
			ok          bool
			err         error
		)
		if strings.TrimSpace(completed.ScheduledJobID) != "" {
			_, reportItems, ok, err = n.reporting.EnqueueScheduledJobReport(ctx, completed, now)
		} else {
			var reportItem types.ReportMailboxItem
			_, _, reportItem, ok, err = n.reporting.EnqueueTaskReport(ctx, completed, now)
			if ok {
				reportItems = append(reportItems, reportItem)
			}
		}
		if err != nil {
			return err
		}
		if !ok {
			return nil
		}
		for _, reportItem := range reportItems {
			targetSessionID := strings.TrimSpace(reportItem.SessionID)
			if targetSessionID == "" {
				continue
			}
			reportEvent, err := types.NewEvent(targetSessionID, completed.ParentTurnID, types.EventReportReady, reportItem)
			if err != nil {
				return err
			}
			if err := eventSink.Emit(ctx, reportEvent); err != nil {
				return err
			}
		}
		return nil
	}

	if strings.TrimSpace(completed.ParentSessionID) == "" {
		return nil
	}

	report, ok := childReportFromTask(completed, now)
	if !ok {
		return nil
	}
	if err := n.store.UpsertPendingChildReport(ctx, report); err != nil {
		return err
	}
	if err := n.enqueueSyntheticChildReportTurn(ctx, completed.ParentSessionID); err != nil {
		return err
	}

	pendingCount, err := n.store.CountPendingChildReports(ctx, completed.ParentSessionID)
	if err != nil {
		return err
	}
	noticeText := "child report queued"
	if pendingCount > 1 {
		noticeText = fmt.Sprintf("%d child reports queued", pendingCount)
	}
	noticeEvent, err := types.NewEvent(completed.ParentSessionID, completed.ParentTurnID, types.EventSystemNotice, types.NoticePayload{
		Text: noticeText,
	})
	if err != nil {
		return err
	}
	if err := eventSink.Emit(ctx, noticeEvent); err != nil {
		return err
	}
	return nil
}

func (n taskTerminalNotifier) currentTime() time.Time {
	if n.now != nil {
		return n.now().UTC()
	}
	return time.Now().UTC()
}

func timelineBlockFromCompletedTask(completed task.Task, runtimeTask types.Task, hasRuntimeTask bool) types.TimelineBlock {
	if hasRuntimeTask {
		block := types.TimelineBlockFromTask(runtimeTask)
		if block.Title == "" {
			block.Title = firstNonEmptyTrimmed(completed.Command, completed.ExecutionTaskID, completed.ID)
		}
		if block.Text == "" {
			block.Text = firstNonEmptyTrimmed(completed.Description, completed.Owner)
		}
		return block
	}
	return types.TimelineBlock{
		ID:         completed.ID,
		TurnID:     completed.ParentTurnID,
		Kind:       "task_block",
		Status:     string(runtimeTaskStateFromTaskStatus(completed.Status)),
		Title:      firstNonEmptyTrimmed(completed.Command, completed.ExecutionTaskID, completed.ID),
		Text:       firstNonEmptyTrimmed(completed.Description, completed.Owner),
		TaskID:     completed.ID,
		WorktreeID: completed.WorktreeID,
	}
}

func childReportFromTask(completed task.Task, now time.Time) (types.ChildReport, bool) {
	result, ready := completed.FinalResult()
	if !ready && strings.TrimSpace(completed.OutcomeSummary) == "" && strings.TrimSpace(completed.Error) == "" && completed.Status == task.TaskStatusRunning {
		return types.ChildReport{}, false
	}
	report := types.ChildReport{
		ID:            completed.ID,
		SessionID:     completed.ParentSessionID,
		ParentTurnID:  completed.ParentTurnID,
		TaskID:        completed.ID,
		TaskType:      string(completed.Type),
		TaskKind:      completed.Kind,
		Source:        childReportSourceFromTask(completed),
		Status:        childReportStatusFromTask(completed),
		Objective:     firstNonEmptyTrimmed(completed.Description, completed.Command),
		ResultReady:   ready,
		Command:       completed.Command,
		Description:   completed.Description,
		ResultKind:    string(result.Kind),
		ResultText:    result.Text,
		ResultPreview: clampTaskResultPreview(firstNonEmptyTrimmed(result.Text, completed.OutcomeSummary, completed.Error)),
		ObservedAt:    firstNonZeroTime(result.ObservedAt, now),
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	return report, true
}

func runtimeTaskStateFromTaskStatus(status task.TaskStatus) types.TaskState {
	switch status {
	case task.TaskStatusRunning:
		return types.TaskStateRunning
	case task.TaskStatusCompleted:
		return types.TaskStateCompleted
	case task.TaskStatusStopped:
		return types.TaskStateCancelled
	case task.TaskStatusFailed:
		return types.TaskStateFailed
	default:
		return types.TaskStatePending
	}
}

func childReportSourceFromTask(completed task.Task) types.ChildReportSource {
	if strings.TrimSpace(completed.ScheduledJobID) != "" || strings.EqualFold(strings.TrimSpace(completed.Kind), "scheduled_report") {
		return types.ChildReportSourceCron
	}
	if strings.TrimSpace(completed.ParentSessionID) != "" {
		return types.ChildReportSourceChat
	}
	return types.ChildReportSourceAutomation
}

func childReportStatusFromTask(completed task.Task) types.ChildReportStatus {
	switch completed.Outcome {
	case types.ChildAgentOutcomeBlocked:
		return types.ChildReportStatusBlocked
	case types.ChildAgentOutcomeFailure:
		return types.ChildReportStatusFailure
	case types.ChildAgentOutcomeSuccess:
		return types.ChildReportStatusSuccess
	}
	switch completed.Status {
	case task.TaskStatusFailed, task.TaskStatusStopped:
		return types.ChildReportStatusFailure
	default:
		return types.ChildReportStatusSuccess
	}
}

func (n taskTerminalNotifier) enqueueSyntheticChildReportTurn(ctx context.Context, sessionID string) error {
	if n.store == nil || n.manager == nil || strings.TrimSpace(sessionID) == "" {
		return nil
	}
	if state, ok := n.manager.GetRuntimeState(sessionID); ok {
		if state.QueuedChildReportBatches > 0 {
			return nil
		}
	}

	now := n.currentTime()
	turn := types.Turn{
		ID:        types.NewID("turn"),
		SessionID: sessionID,
		Kind:      types.TurnKindChildReportBatch,
		State:     types.TurnStateCreated,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if headID, ok, err := n.store.GetCurrentContextHeadID(ctx); err == nil && ok {
		turn.ContextHeadID = strings.TrimSpace(headID)
	}
	if err := n.store.InsertTurn(ctx, turn); err != nil {
		return err
	}
	_, err := n.manager.SubmitTurn(ctx, sessionID, session.SubmitTurnInput{Turn: turn})
	return err
}

func clampTaskResultPreview(text string) string {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return ""
	}
	const maxLen = 480
	runes := []rune(trimmed)
	if len(runes) <= maxLen {
		return trimmed
	}
	return string(runes[:maxLen]) + "..."
}

func firstNonZeroTime(values ...time.Time) time.Time {
	for _, value := range values {
		if !value.IsZero() {
			return value.UTC()
		}
	}
	return time.Time{}
}

func firstNonEmptyTrimmed(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func ensureDataDir(path string) error {
	return os.MkdirAll(path, 0o755)
}

func writePIDFile(path string, daemonID string, fingerprint string) error {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	raw, err := json.Marshal(map[string]any{
		"pid":                os.Getpid(),
		"daemon_id":          strings.TrimSpace(daemonID),
		"config_fingerprint": strings.TrimSpace(fingerprint),
	})
	if err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0o644)
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
	if err := writePIDFile(cfg.Paths.PIDFile, cfg.DaemonID, cfg.ConfigFingerprint); err != nil {
		return err
	}
	defer os.Remove(cfg.Paths.PIDFile)

	store, err := sqlite.Open(cfg.Paths.DatabaseFile)
	if err != nil {
		return err
	}
	defer store.Close()

	_, err = artifacts.New(filepath.Join(cfg.DataDir, "artifacts"))
	if err != nil {
		return err
	}

	configureRuntimeGuardrails(cfg)
	modelClient, err := model.NewFromConfig(cfg)
	if err != nil {
		return err
	}
	runtime, err := buildRuntime(ctx, cfg, store, modelClient)
	if err != nil {
		return err
	}
	if err := validateRuntime(runtime); err != nil {
		return err
	}
	if runtime.Engine != nil {
		runtime.Engine.SetBaseSystemPrompt(basePrompt)
	}

	if err := recoverRuntimeState(ctx, runtime.Store, runtime.SessionManager); err != nil {
		return err
	}
	dispatcher := automation.NewDispatcher(runtime.Store, automationTaskLauncher{
		store:   runtime.Store,
		manager: runtime.TaskManager,
	}, automation.DispatcherConfig{Watcher: runtime.WatcherService})
	go func() {
		if err := runtime.SchedulerService.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
			slog.Error("scheduler loop exited", "error", err)
		}
	}()
	if runtime.WatcherService != nil {
		go func() {
			runSupervisedLoop(ctx, "watcher", runtime.WatcherService.ReconcileInterval(), runtime.WatcherService.Reconcile, func(_ context.Context, err error) {
				slog.Error("watcher tick failed", "error", err)
			})
		}()
	}
	if runtime.ReportingService != nil {
		go func() {
			runSupervisedLoop(ctx, "reporting", runtime.ReportingService.PollInterval(), runtime.ReportingService.Tick, func(_ context.Context, err error) {
				slog.Error("reporting tick failed", "error", err)
			})
		}()
	}
	go func() {
		runSupervisedLoop(ctx, "automation_dispatcher", time.Second, dispatcher.Tick, func(_ context.Context, err error) {
			slog.Error("automation dispatcher tick failed", "error", err)
		})
	}()

	handler := httpapi.NewRouter(buildHTTPDependencies(cfg, runtime.Store, runtime.Bus, runtime.SessionManager, runtime.SchedulerService, runtime.AutomationService))

	slog.Info("sesame daemon listening", "addr", cfg.Addr)
	return http.ListenAndServe(cfg.Addr, handler)
}

func configureRuntimeGuardrails(cfg config.Config) {
	tools.SetShellCommandGuardrails(cfg.MaxShellOutputBytes, cfg.ShellTimeoutSeconds)
	tools.SetFileWriteMaxBytes(cfg.MaxFileWriteBytes)
}

func buildHTTPDependencies(cfg config.Config, store *sqlite.Store, bus *stream.Bus, manager *session.Manager, schedulerService *scheduler.Service, automationService *automation.Service) httpapi.Dependencies {
	if automationService == nil && store != nil {
		automationService = automation.NewService(store)
	}
	return httpapi.Dependencies{
		Bus:           bus,
		Store:         store,
		Manager:       manager,
		Scheduler:     schedulerService,
		Automation:    automationService,
		RoleService:   rolectx.NewService(),
		Status:        buildStatusPayload(cfg),
		ConsoleRoot:   filepath.Join("web", "console", "dist"),
		WorkspaceRoot: cfg.Paths.WorkspaceRoot,
	}
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
		Provider:             cfg.ModelProvider,
		Model:                cfg.Model,
		PermissionProfile:    cfg.PermissionProfile,
		ProviderCacheProfile: cfg.ProviderCacheProfile,
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

func runAutomationDispatcherLoop(ctx context.Context, dispatcher *automation.Dispatcher) error {
	if dispatcher == nil {
		return nil
	}
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	if err := dispatcher.Tick(ctx); err != nil {
		return err
	}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := dispatcher.Tick(ctx); err != nil {
				return err
			}
		}
	}
}

func recoverRuntimeState(ctx context.Context, store *sqlite.Store, manager *session.Manager) error {
	now := time.Now().UTC()
	if err := failLegacyDispatchAttempts(ctx, store, now); err != nil {
		return err
	}

	sessions, err := store.ListSessions(ctx)
	if err != nil {
		return err
	}
	if manager != nil {
		for _, sessionRow := range sessions {
			manager.RegisterSession(sessionRow)
		}
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
		if turn.Kind == types.TurnKindChildReportBatch {
			if err := store.RequeueClaimedChildReportsForTurn(ctx, turn.ID); err != nil {
				return err
			}
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

	attempts, err := store.ListDispatchAttempts(ctx, types.DispatchAttemptFilter{
		Status: types.DispatchAttemptStatusRunning,
	})
	if err != nil {
		return err
	}
	for _, attempt := range attempts {
		attempt.Status = types.DispatchAttemptStatusInterrupted
		attempt.Error = firstNonEmptyTrimmed(attempt.Error, "daemon_restart")
		attempt.UpdatedAt = time.Now().UTC()
		if err := store.UpsertDispatchAttempt(ctx, attempt); err != nil {
			return err
		}
		if err := updateIncidentPhaseState(ctx, store, attempt.IncidentID, attempt.Phase, attempt.UpdatedAt, func(phase *types.IncidentPhaseState) {
			if phase.ActiveDispatchCount > 0 {
				phase.ActiveDispatchCount--
			}
			phase.Status = types.IncidentPhaseStatusPending
		}); err != nil {
			return err
		}
		if err := updateAutomationIncidentStatus(ctx, store, attempt.IncidentID, types.AutomationIncidentStatusQueued, attempt.UpdatedAt); err != nil {
			return err
		}
	}

	if err := recoverQueuedCanonicalTurns(ctx, store, manager); err != nil {
		return err
	}

	return nil
}

func recoverQueuedCanonicalTurns(ctx context.Context, store *sqlite.Store, manager *session.Manager) error {
	if store == nil || manager == nil {
		return nil
	}

	canonicalSessionID, ok, err := store.GetCanonicalSessionID(ctx)
	if err != nil || !ok {
		return err
	}

	turns, err := store.ListTurnsBySession(ctx, canonicalSessionID)
	if err != nil {
		return err
	}
	for _, turn := range turns {
		if turn.State != types.TurnStateCreated {
			continue
		}
		if _, err := manager.SubmitTurn(ctx, canonicalSessionID, session.SubmitTurnInput{Turn: turn}); err != nil {
			return err
		}
	}
	return nil
}

func failLegacyDispatchAttempts(ctx context.Context, store *sqlite.Store, now time.Time) error {
	if store == nil {
		return nil
	}

	attempts, err := store.ListDispatchAttempts(ctx, types.DispatchAttemptFilter{})
	if err != nil {
		return err
	}

	reconciler := sessionRunnerAdapter{store: store}
	for _, attempt := range attempts {
		if strings.TrimSpace(attempt.TaskID) != "" {
			continue
		}
		if attempt.Status != types.DispatchAttemptStatusRunning && attempt.Status != types.DispatchAttemptStatusAwaitingApproval {
			continue
		}
		if err := clearLegacyDispatchWatcherHold(ctx, store, attempt, now); err != nil {
			return err
		}
		attempt.Status = types.DispatchAttemptStatusFailed
		attempt.Error = "legacy background-run dispatch no longer supported"
		attempt.FinishedAt = now
		attempt.UpdatedAt = now
		if err := store.UpsertDispatchAttempt(ctx, attempt); err != nil {
			return err
		}
		if err := reconciler.applyDispatchOutcome(ctx, attempt, dispatchOutcomeFailed, now); err != nil {
			return err
		}
	}
	return nil
}

func clearLegacyDispatchWatcherHold(ctx context.Context, store *sqlite.Store, attempt types.DispatchAttempt, now time.Time) error {
	if store == nil || strings.TrimSpace(attempt.AutomationID) == "" {
		return nil
	}

	runtime, ok, err := store.GetAutomationWatcher(ctx, attempt.AutomationID)
	if err != nil || !ok {
		return err
	}
	holds, err := store.ListAutomationWatcherHolds(ctx, attempt.AutomationID)
	if err != nil {
		return err
	}

	updated := holds
	if requestID := strings.TrimSpace(attempt.PermissionRequestID); requestID != "" {
		updated = automation.ReleaseWatcherHold(holds, types.AutomationWatcherHoldKindApproval, requestID)
	} else {
		updated = automation.ReleaseWatcherHold(holds, types.AutomationWatcherHoldKindDispatch, attempt.DispatchID)
	}

	watcherID := strings.TrimSpace(runtime.WatcherID)
	if watcherID == "" {
		watcherID = "watcher:" + attempt.AutomationID
	}
	if err := store.ReplaceAutomationWatcherHolds(ctx, attempt.AutomationID, watcherID, updated); err != nil {
		return err
	}

	runtime.WatcherID = watcherID
	runtime.Holds = updated
	runtime.EffectiveState = automation.EffectiveWatcherState(runtime.State, updated)
	runtime.UpdatedAt = now
	return store.UpsertAutomationWatcher(ctx, runtime)
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
		specialistRoleID, err := store.ResolveSpecialistRoleID(ctx, sessionRow.ID, sessionRow.WorkspaceRoot)
		if err != nil {
			return nil, err
		}
		resumeCtx := rolectx.WithSpecialistRoleID(ctx, specialistRoleID)
		if _, err := manager.ResumeTurn(resumeCtx, sessionRow.ID, session.ResumeTurnInput{
			Turn:   turn,
			Resume: resume,
		}); err != nil {
			return nil, err
		}

		now := time.Now().UTC()
		continuation.State = types.TurnContinuationStateResumed
		continuation.Decision = request.Decision
		continuation.DecisionScope = decisionScope
		continuation.UpdatedAt = now
		var resumedToolRun *types.ToolRun
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
				resumedToolRun = &toolRun
			}
		}
		if err := store.CommitPermissionResume(ctx, sessionRow.ID, turn.ID, continuation, resumedToolRun); err != nil {
			manager.InterruptTurn(sessionRow.ID, turn.ID)
			return nil, err
		}
		if err := automation.RestoreDispatchAfterApprovalResume(ctx, store, continuation.TaskID, request.ID, now); err != nil {
			return nil, err
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
