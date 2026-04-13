package automation

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go-agent/internal/task"
	"go-agent/internal/types"
)

const watcherTaskDescription = "automation watcher runtime"

type watcherStore interface {
	GetAutomation(context.Context, string) (types.AutomationSpec, bool, error)
	UpsertAutomationWatcher(context.Context, types.AutomationWatcherRuntime) error
	GetAutomationWatcher(context.Context, string) (types.AutomationWatcherRuntime, bool, error)
	ListAutomationWatchers(context.Context, types.AutomationWatcherFilter) ([]types.AutomationWatcherRuntime, error)
	DeleteAutomationWatcher(context.Context, string) (bool, error)
}

type watcherTaskManager interface {
	Create(context.Context, task.CreateTaskInput) (task.Task, error)
	Get(string, string) (task.Task, bool, error)
	Stop(string, string) error
}

type WatcherConfig struct {
	DataRoot       string
	ExecutablePath string
	DataDir        string
	Addr           string
	ReconcileEvery time.Duration
	Now            func() time.Time
}

type WatcherService struct {
	store          watcherStore
	taskManager    watcherTaskManager
	dataRoot       string
	executablePath string
	dataDir        string
	addr           string
	reconcileEvery time.Duration
	now            func() time.Time
}

type watcherRunnerState struct {
	DesiredState string                        `json:"desired_state,omitempty"`
	Signals      map[string]watcherSignalState `json:"signals,omitempty"`
	UpdatedAt    time.Time                     `json:"updated_at,omitempty"`
}

type watcherSignalState struct {
	LastTriggeredAt time.Time `json:"last_triggered_at,omitempty"`
}

func NewWatcherService(store watcherStore, taskManager watcherTaskManager, cfg WatcherConfig) *WatcherService {
	dataRoot := strings.TrimSpace(cfg.DataRoot)
	if dataRoot == "" {
		dataRoot = filepath.Join(os.TempDir(), "sesame-automation")
	}
	return &WatcherService{
		store:          store,
		taskManager:    taskManager,
		dataRoot:       dataRoot,
		executablePath: strings.TrimSpace(cfg.ExecutablePath),
		dataDir:        strings.TrimSpace(cfg.DataDir),
		addr:           strings.TrimSpace(cfg.Addr),
		reconcileEvery: firstPositiveDuration(cfg.ReconcileEvery, 5*time.Second),
		now:            firstNonNilClock(cfg.Now),
	}
}

func (s *WatcherService) Install(ctx context.Context, spec types.AutomationSpec) (types.AutomationWatcherRuntime, error) {
	if s == nil || s.store == nil || s.taskManager == nil {
		return types.AutomationWatcherRuntime{}, errServiceNotConfigured
	}
	spec = normalizeAutomationSpec(spec, s.currentTime())
	if err := validateWatcherSignals(spec); err != nil {
		return types.AutomationWatcherRuntime{}, err
	}
	runtime, ok, err := s.loadOrInitRuntime(ctx, spec)
	if err != nil {
		return types.AutomationWatcherRuntime{}, err
	}
	runtime, err = s.startRuntime(ctx, spec, runtime, ok)
	return runtime, err
}

func (s *WatcherService) Reinstall(ctx context.Context, spec types.AutomationSpec) (types.AutomationWatcherRuntime, error) {
	if s == nil || s.store == nil || s.taskManager == nil {
		return types.AutomationWatcherRuntime{}, errServiceNotConfigured
	}
	spec = normalizeAutomationSpec(spec, s.currentTime())
	if err := validateWatcherSignals(spec); err != nil {
		return types.AutomationWatcherRuntime{}, err
	}
	runtime, _, err := s.loadOrInitRuntime(ctx, spec)
	if err != nil {
		return types.AutomationWatcherRuntime{}, err
	}
	runtime, err = s.startRuntime(ctx, spec, runtime, true)
	return runtime, err
}

func (s *WatcherService) Get(ctx context.Context, automationID string) (types.AutomationWatcherRuntime, bool, error) {
	if s == nil || s.store == nil {
		return types.AutomationWatcherRuntime{}, false, errServiceNotConfigured
	}
	return s.store.GetAutomationWatcher(ctx, strings.TrimSpace(automationID))
}

func (s *WatcherService) Pause(ctx context.Context, automationID string) (types.AutomationWatcherRuntime, bool, error) {
	if s == nil || s.store == nil {
		return types.AutomationWatcherRuntime{}, false, errServiceNotConfigured
	}
	runtime, ok, err := s.store.GetAutomationWatcher(ctx, strings.TrimSpace(automationID))
	if err != nil || !ok {
		return types.AutomationWatcherRuntime{}, ok, err
	}
	if err := s.stopRuntimeTask(runtime); err != nil {
		return types.AutomationWatcherRuntime{}, false, err
	}
	runtime.State = types.AutomationWatcherStatePaused
	runtime.LastError = ""
	runtime.UpdatedAt = s.currentTime()
	if err := writeWatcherRunnerState(runtime.StatePath, watcherRunnerState{
		DesiredState: string(types.AutomationWatcherStatePaused),
		UpdatedAt:    runtime.UpdatedAt,
	}); err != nil {
		return types.AutomationWatcherRuntime{}, false, err
	}
	if err := s.store.UpsertAutomationWatcher(ctx, runtime); err != nil {
		return types.AutomationWatcherRuntime{}, false, err
	}
	return runtime, true, nil
}

func (s *WatcherService) Delete(ctx context.Context, automationID string) error {
	if s == nil || s.store == nil {
		return errServiceNotConfigured
	}
	runtime, ok, err := s.store.GetAutomationWatcher(ctx, strings.TrimSpace(automationID))
	if err != nil {
		return err
	}
	if ok {
		if err := s.stopRuntimeTask(runtime); err != nil {
			return err
		}
		if _, err := s.store.DeleteAutomationWatcher(ctx, automationID); err != nil {
			return err
		}
		if dir := filepath.Dir(runtime.ScriptPath); dir != "" && dir != "." {
			_ = os.RemoveAll(dir)
		}
	}
	return nil
}

func (s *WatcherService) Reconcile(ctx context.Context) error {
	if s == nil || s.store == nil || s.taskManager == nil {
		return errServiceNotConfigured
	}
	watchers, err := s.store.ListAutomationWatchers(ctx, types.AutomationWatcherFilter{})
	if err != nil {
		return err
	}
	for _, watcher := range watchers {
		current, ok, err := s.store.GetAutomationWatcher(ctx, watcher.AutomationID)
		if err != nil {
			return err
		}
		if !ok {
			continue
		}
		if err := s.reconcileWatcher(ctx, current); err != nil {
			return err
		}
	}
	return nil
}

func (s *WatcherService) Run(ctx context.Context) error {
	if s == nil {
		return nil
	}
	if err := s.Reconcile(ctx); err != nil {
		return err
	}
	timer := time.NewTicker(s.reconcileEvery)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
			if err := s.Reconcile(ctx); err != nil {
				return err
			}
		}
	}
}

func (s *WatcherService) reconcileWatcher(ctx context.Context, runtime types.AutomationWatcherRuntime) error {
	spec, ok, err := s.store.GetAutomation(ctx, runtime.AutomationID)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	if spec.State == types.AutomationStatePaused {
		runtime.State = types.AutomationWatcherStatePaused
		runtime.UpdatedAt = s.currentTime()
		return s.store.UpsertAutomationWatcher(ctx, runtime)
	}

	status, shouldRestart, err := s.inspectRuntime(spec, runtime)
	if err != nil {
		return err
	}
	if !shouldRestart {
		status.UpdatedAt = s.currentTime()
		return s.store.UpsertAutomationWatcher(ctx, status)
	}
	_, err = s.startRuntime(ctx, spec, status, false)
	return err
}

func (s *WatcherService) loadOrInitRuntime(ctx context.Context, spec types.AutomationSpec) (types.AutomationWatcherRuntime, bool, error) {
	runtime, ok, err := s.store.GetAutomationWatcher(ctx, spec.ID)
	if err != nil {
		return types.AutomationWatcherRuntime{}, false, err
	}
	if !ok {
		runtime = types.AutomationWatcherRuntime{
			ID:            types.NewID("watcher"),
			AutomationID:  spec.ID,
			WorkspaceRoot: spec.WorkspaceRoot,
			WatcherID:     "watcher:" + spec.ID,
			State:         types.AutomationWatcherStatePending,
			CreatedAt:     s.currentTime(),
		}
	}
	return runtime, ok, nil
}

func (s *WatcherService) startRuntime(ctx context.Context, spec types.AutomationSpec, runtime types.AutomationWatcherRuntime, stopExisting bool) (types.AutomationWatcherRuntime, error) {
	if stopExisting {
		if err := s.stopRuntimeTask(runtime); err != nil {
			return types.AutomationWatcherRuntime{}, err
		}
	}
	runtime.WorkspaceRoot = spec.WorkspaceRoot
	runtime.State = types.AutomationWatcherStatePending
	runtime.LastError = ""
	runtime.UpdatedAt = s.currentTime()

	scriptPath, statePath, command, err := s.prepareFiles(spec, runtime)
	if err != nil {
		return types.AutomationWatcherRuntime{}, err
	}
	runtime.ScriptPath = scriptPath
	runtime.StatePath = statePath
	runtime.Command = command

	taskRecord, err := s.taskManager.Create(ctx, task.CreateTaskInput{
		Type:           task.TaskTypeShell,
		Command:        command,
		Description:    watcherTaskDescription,
		Kind:           "automation_watcher",
		Owner:          runtime.WatcherID,
		WorkspaceRoot:  spec.WorkspaceRoot,
		TimeoutSeconds: 0,
		Start:          true,
	})
	if err != nil {
		runtime.State = types.AutomationWatcherStateFailed
		runtime.LastError = err.Error()
		_ = s.store.UpsertAutomationWatcher(ctx, runtime)
		return types.AutomationWatcherRuntime{}, err
	}
	runtime.TaskID = taskRecord.ID
	runtime.State = types.AutomationWatcherStateRunning
	if err := s.store.UpsertAutomationWatcher(ctx, runtime); err != nil {
		return types.AutomationWatcherRuntime{}, err
	}
	return runtime, nil
}

func (s *WatcherService) inspectRuntime(spec types.AutomationSpec, runtime types.AutomationWatcherRuntime) (types.AutomationWatcherRuntime, bool, error) {
	if strings.TrimSpace(runtime.TaskID) == "" {
		if desired := readWatcherDesiredState(runtime.StatePath); desired != "" {
			runtime.State = types.AutomationWatcherState(desired)
			return runtime, desired == string(types.AutomationWatcherStateRunning), nil
		}
		return runtime, isContinuousWatcher(spec), nil
	}
	taskRecord, ok, err := s.taskManager.Get(runtime.TaskID, runtime.WorkspaceRoot)
	if err != nil {
		return runtime, false, err
	}
	if !ok {
		if desired := readWatcherDesiredState(runtime.StatePath); desired != "" {
			runtime.State = types.AutomationWatcherState(desired)
			return runtime, desired == string(types.AutomationWatcherStateRunning), nil
		}
		runtime.TaskID = ""
		return runtime, isContinuousWatcher(spec), nil
	}
	switch taskRecord.Status {
	case task.TaskStatusRunning:
		runtime.State = types.AutomationWatcherStateRunning
		runtime.LastError = ""
		return runtime, false, nil
	case task.TaskStatusStopped:
		runtime.TaskID = ""
		runtime.State = types.AutomationWatcherStateStopped
		if desired := readWatcherDesiredState(runtime.StatePath); desired == string(types.AutomationWatcherStatePaused) {
			runtime.State = types.AutomationWatcherStatePaused
			return runtime, false, nil
		}
		return runtime, isContinuousWatcher(spec), nil
	case task.TaskStatusFailed:
		runtime.TaskID = ""
		runtime.State = types.AutomationWatcherStateFailed
		runtime.LastError = strings.TrimSpace(taskRecord.Error)
		if desired := readWatcherDesiredState(runtime.StatePath); desired == string(types.AutomationWatcherStatePaused) || desired == string(types.AutomationWatcherStateStopped) {
			runtime.State = types.AutomationWatcherState(desired)
			runtime.LastError = ""
			return runtime, false, nil
		}
		return runtime, isContinuousWatcher(spec), nil
	case task.TaskStatusCompleted:
		runtime.TaskID = ""
		runtime.State = types.AutomationWatcherStateStopped
		if desired := readWatcherDesiredState(runtime.StatePath); desired == string(types.AutomationWatcherStatePaused) || desired == string(types.AutomationWatcherStateStopped) {
			runtime.State = types.AutomationWatcherState(desired)
			return runtime, false, nil
		}
		return runtime, isContinuousWatcher(spec), nil
	default:
		return runtime, isContinuousWatcher(spec), nil
	}
}

func (s *WatcherService) prepareFiles(spec types.AutomationSpec, runtime types.AutomationWatcherRuntime) (string, string, string, error) {
	dir := filepath.Join(s.dataRoot, "watchers", spec.ID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", "", "", err
	}
	scriptPath := filepath.Join(dir, "run.sh")
	statePath := filepath.Join(dir, "state.json")
	if err := writeWatcherRunnerState(statePath, watcherRunnerState{
		UpdatedAt: s.currentTime(),
	}); err != nil {
		return "", "", "", err
	}
	scriptBody := renderWatcherScript(spec.ID, runtime.WatcherID, statePath, s.dataDir, s.addr, s.resolveExecutablePath())
	if err := os.WriteFile(scriptPath, []byte(scriptBody), 0o755); err != nil {
		return "", "", "", err
	}
	return scriptPath, statePath, shellQuote(scriptPath), nil
}

func (s *WatcherService) stopRuntimeTask(runtime types.AutomationWatcherRuntime) error {
	if s == nil || s.taskManager == nil || strings.TrimSpace(runtime.TaskID) == "" {
		return nil
	}
	err := s.taskManager.Stop(runtime.TaskID, runtime.WorkspaceRoot)
	if err != nil && !strings.Contains(err.Error(), "not found") {
		return err
	}
	return nil
}

func (s *WatcherService) resolveExecutablePath() string {
	if strings.TrimSpace(s.executablePath) != "" {
		return s.executablePath
	}
	if path, err := os.Executable(); err == nil && strings.TrimSpace(path) != "" {
		return path
	}
	return "sesame"
}

func (s *WatcherService) currentTime() time.Time {
	if s == nil || s.now == nil {
		return time.Now().UTC()
	}
	return s.now().UTC()
}

func renderWatcherScript(automationID, watcherID, statePath, dataDir, addr, executablePath string) string {
	lines := []string{
		"#!/usr/bin/env bash",
		"set -euo pipefail",
	}
	if strings.TrimSpace(dataDir) != "" {
		lines = append(lines, "export SESAME_DATA_DIR="+shellQuote(dataDir))
	}
	if strings.TrimSpace(addr) != "" {
		lines = append(lines, "export SESAME_ADDR="+shellQuote(addr))
	}
	lines = append(lines,
		"exec "+shellQuote(executablePath)+
			" automation run"+
			" --watcher-id "+shellQuote(watcherID)+
			" --state-file "+shellQuote(statePath)+
			" "+shellQuote(automationID),
	)
	return strings.Join(lines, "\n") + "\n"
}

func validateWatcherSignals(spec types.AutomationSpec) error {
	if len(spec.Signals) == 0 {
		return &types.AutomationValidationError{
			Code:    "missing_signal_spec",
			Message: "active automation watchers require at least one runnable signal",
		}
	}
	for _, signal := range spec.Signals {
		if strings.EqualFold(strings.TrimSpace(signal.Kind), "poll") && strings.TrimSpace(signal.Selector) != "" {
			return nil
		}
	}
	return &types.AutomationValidationError{
		Code:    "missing_signal_spec",
		Message: "at least one poll signal with a selector command is required",
	}
}

func isContinuousWatcher(spec types.AutomationSpec) bool {
	type lifecycle struct {
		Mode string `json:"mode"`
	}
	var cfg lifecycle
	_ = json.Unmarshal(spec.WatcherLifecycle, &cfg)
	switch strings.ToLower(strings.TrimSpace(cfg.Mode)) {
	case "", "continuous":
		return true
	default:
		return false
	}
}

func writeWatcherRunnerState(path string, state watcherRunnerState) error {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	state.UpdatedAt = time.Now().UTC()
	raw, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0o644)
}

func readWatcherDesiredState(path string) string {
	if strings.TrimSpace(path) == "" {
		return ""
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	var state watcherRunnerState
	if err := json.Unmarshal(raw, &state); err != nil {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(state.DesiredState))
}

func shellQuote(value string) string {
	value = strings.ReplaceAll(value, `'`, `'"'"'`)
	return "'" + value + "'"
}

func firstNonNilClock(now func() time.Time) func() time.Time {
	if now != nil {
		return now
	}
	return func() time.Time { return time.Now().UTC() }
}

func firstPositiveDuration(value time.Duration, fallback time.Duration) time.Duration {
	if value > 0 {
		return value
	}
	return fallback
}
