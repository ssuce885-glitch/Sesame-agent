package automation

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	runtimex "go-agent/internal/runtime"
	"go-agent/internal/types"
)

type watcherRuntimeClient interface {
	GetAutomation(context.Context, string) (types.AutomationSpec, error)
	EmitTrigger(context.Context, types.TriggerEmitRequest) (types.TriggerEvent, error)
	RecordHeartbeat(context.Context, types.TriggerHeartbeatRequest) (types.AutomationHeartbeat, error)
}

type WatcherRunnerConfig struct {
	Now   func() time.Time
	Exec  func(context.Context, watcherCommandRequest) (watcherCommandResult, error)
	Sleep func(context.Context, time.Duration) error
}

type WatcherRunner struct {
	client watcherRuntimeClient
	now    func() time.Time
	exec   func(context.Context, watcherCommandRequest) (watcherCommandResult, error)
	sleep  func(context.Context, time.Duration) error
}

type watcherCommandRequest struct {
	Command    string
	WorkingDir string
	Timeout    time.Duration
	Env        map[string]string
}

type watcherCommandResult struct {
	ExitCode int
	Stdout   string
	Stderr   string
}

type compiledWatcherSignal struct {
	Key             string
	Command         string
	WorkingDir      string
	Interval        time.Duration
	Timeout         time.Duration
	TriggerOn       string
	Match           string
	SignalKind      string
	Summary         string
	CooldownSeconds int
	Source          string
	Env             map[string]string
	UsesScriptJSON  bool
}

type watcherLifecycleConfig struct {
	Mode          string `json:"mode"`
	AfterDispatch string `json:"after_dispatch"`
}

type watcherPollSignalPayload struct {
	IntervalSeconds int               `json:"interval_seconds"`
	TimeoutSeconds  int               `json:"timeout_seconds"`
	TriggerOn       string            `json:"trigger_on"`
	Match           string            `json:"match"`
	SignalKind      string            `json:"signal_kind"`
	Summary         string            `json:"summary"`
	WorkingDir      string            `json:"working_dir"`
	CooldownSeconds int               `json:"cooldown_seconds"`
	Env             map[string]string `json:"env"`
	ScriptPath      string            `json:"script_path"`
}

type watcherRetriggerPolicy struct {
	CooldownSeconds int `json:"cooldown_seconds"`
}

func NewWatcherRunner(client watcherRuntimeClient, cfg WatcherRunnerConfig) *WatcherRunner {
	return &WatcherRunner{
		client: client,
		now:    firstNonNilClock(cfg.Now),
		exec:   firstNonNilExec(cfg.Exec),
		sleep:  firstNonNilSleep(cfg.Sleep),
	}
}

func (r *WatcherRunner) Run(ctx context.Context, automationID, watcherID, statePath string) error {
	if r == nil || r.client == nil {
		return errServiceNotConfigured
	}
	automationID = strings.TrimSpace(automationID)
	watcherID = strings.TrimSpace(watcherID)
	if automationID == "" {
		return &types.AutomationValidationError{
			Code:    "missing_signal_spec",
			Message: "automation_id is required",
		}
	}
	if watcherID == "" {
		watcherID = "watcher:" + automationID
	}

	spec, err := r.client.GetAutomation(ctx, automationID)
	if err != nil {
		return err
	}
	signals, lifecycle, err := compileWatcherSignals(spec)
	if err != nil {
		return err
	}

	state, err := loadWatcherRunnerState(statePath)
	if err != nil {
		return err
	}
	if state.Signals == nil {
		state.Signals = make(map[string]watcherSignalState, len(signals))
	}

	for {
		now := r.currentTime()
		heartbeatStatus := "healthy"
		triggered := false

		for _, signal := range signals {
			result, execErr := r.exec(ctx, watcherCommandRequest{
				Command:    signal.Command,
				WorkingDir: signal.WorkingDir,
				Timeout:    signal.Timeout,
				Env:        signal.Env,
			})
			if execErr != nil && ctx.Err() != nil {
				return ctx.Err()
			}

			matched, summary, payload, evalErr := evaluateWatcherSignal(signal, result, execErr)
			if evalErr != nil {
				return evalErr
			}
			if execErr != nil && !matched {
				heartbeatStatus = "degraded"
			}
			if !matched {
				continue
			}

			last := state.Signals[signal.Key]
			if signal.CooldownSeconds > 0 && !last.LastTriggeredAt.IsZero() {
				if now.Sub(last.LastTriggeredAt) < time.Duration(signal.CooldownSeconds)*time.Second {
					heartbeatStatus = "triggered"
					triggered = true
					continue
				}
			}

			emitReq := types.TriggerEmitRequest{
				AutomationID: automationID,
				SignalKind:   signal.SignalKind,
				Source:       signal.Source,
				Summary:      summary,
				Payload:      payload,
				ObservedAt:   now,
			}
			if _, err := r.client.EmitTrigger(ctx, emitReq); err != nil {
				return err
			}

			last.LastTriggeredAt = now
			state.Signals[signal.Key] = last
			heartbeatStatus = "triggered"
			triggered = true

			switch strings.ToLower(strings.TrimSpace(lifecycle.AfterDispatch)) {
			case "stop":
				state.DesiredState = string(types.AutomationWatcherStateStopped)
				if err := writeWatcherRunnerState(statePath, state); err != nil {
					return err
				}
				_, err := r.client.RecordHeartbeat(ctx, types.TriggerHeartbeatRequest{
					AutomationID: automationID,
					WatcherID:    watcherID,
					Status:       "stopped",
					ObservedAt:   now,
				})
				return err
			case "pause":
				state.DesiredState = string(types.AutomationWatcherStatePaused)
				if err := writeWatcherRunnerState(statePath, state); err != nil {
					return err
				}
				_, err := r.client.RecordHeartbeat(ctx, types.TriggerHeartbeatRequest{
					AutomationID: automationID,
					WatcherID:    watcherID,
					Status:       "paused",
					ObservedAt:   now,
				})
				return err
			}
		}

		state.DesiredState = ""
		if err := writeWatcherRunnerState(statePath, state); err != nil {
			return err
		}
		if _, err := r.client.RecordHeartbeat(ctx, types.TriggerHeartbeatRequest{
			AutomationID: automationID,
			WatcherID:    watcherID,
			Status:       firstNonEmptyWatcherStatus(heartbeatStatus, triggered),
			ObservedAt:   now,
		}); err != nil {
			return err
		}

		if !isContinuousLifecycle(lifecycle) {
			return nil
		}
		if err := r.sleep(ctx, minWatcherInterval(signals)); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return err
		}
	}
}

func (r *WatcherRunner) currentTime() time.Time {
	if r == nil || r.now == nil {
		return time.Now().UTC()
	}
	return r.now().UTC()
}

func compileWatcherSignals(spec types.AutomationSpec) ([]compiledWatcherSignal, watcherLifecycleConfig, error) {
	lifecycle := watcherLifecycleConfig{}
	_ = json.Unmarshal(spec.WatcherLifecycle, &lifecycle)
	if strings.EqualFold(strings.TrimSpace(string(spec.Mode)), string(types.AutomationModeSimple)) && strings.TrimSpace(lifecycle.AfterDispatch) == "" {
		lifecycle.AfterDispatch = "pause"
	}
	retrigger := watcherRetriggerPolicy{}
	_ = json.Unmarshal(spec.RetriggerPolicy, &retrigger)

	out := make([]compiledWatcherSignal, 0, len(spec.Signals))
	for idx, signal := range spec.Signals {
		if !strings.EqualFold(strings.TrimSpace(signal.Kind), "poll") {
			continue
		}
		payload := watcherPollSignalPayload{}
		if len(signal.Payload) > 0 {
			if err := json.Unmarshal(signal.Payload, &payload); err != nil {
				return nil, watcherLifecycleConfig{}, err
			}
		}
		command := strings.TrimSpace(signal.Selector)
		workingDir := firstNonEmptyString(strings.TrimSpace(payload.WorkingDir), spec.WorkspaceRoot)
		usesScriptJSON := normalizeTriggerOn(payload.TriggerOn) == "script_status"
		if strings.EqualFold(command, "automation_script") {
			resolvedPath, err := ResolveAutomationAssetPath(spec.WorkspaceRoot, spec.ID, payload.ScriptPath)
			if err != nil {
				return nil, watcherLifecycleConfig{}, err
			}
			command = shellQuote(resolvedPath)
			workingDir = firstNonEmptyString(strings.TrimSpace(payload.WorkingDir), filepath.Dir(resolvedPath))
			usesScriptJSON = true
		}
		if command == "" {
			continue
		}
		interval := time.Duration(payload.IntervalSeconds) * time.Second
		if interval <= 0 {
			interval = time.Minute
		}
		timeout := time.Duration(payload.TimeoutSeconds) * time.Second
		if timeout <= 0 {
			timeout = 30 * time.Second
		}
		cooldown := payload.CooldownSeconds
		if cooldown < 0 {
			cooldown = 0
		}
		if cooldown == 0 && retrigger.CooldownSeconds > 0 {
			cooldown = retrigger.CooldownSeconds
		}
		out = append(out, compiledWatcherSignal{
			Key:             fmt.Sprintf("%s:%d:%s", strings.TrimSpace(signal.Kind), idx, command),
			Command:         command,
			WorkingDir:      workingDir,
			Interval:        interval,
			Timeout:         timeout,
			TriggerOn:       normalizeTriggerOn(payload.TriggerOn),
			Match:           strings.TrimSpace(payload.Match),
			SignalKind:      firstNonEmptyString(strings.TrimSpace(payload.SignalKind), strings.TrimSpace(signal.Kind)),
			Summary:         strings.TrimSpace(payload.Summary),
			CooldownSeconds: cooldown,
			Source:          firstNonEmptyString(strings.TrimSpace(signal.Source), "watcher:"+spec.ID),
			Env:             payload.Env,
			UsesScriptJSON:  usesScriptJSON,
		})
	}
	if len(out) == 0 {
		return nil, watcherLifecycleConfig{}, &types.AutomationValidationError{
			Code:    "missing_signal_spec",
			Message: "at least one poll signal with a selector command is required",
		}
	}
	return out, lifecycle, nil
}

func evaluateWatcherSignal(signal compiledWatcherSignal, result watcherCommandResult, execErr error) (bool, string, json.RawMessage, error) {
	if signal.UsesScriptJSON {
		return evaluateWatcherScriptSignal(signal, result, execErr)
	}
	switch signal.TriggerOn {
	case "stdout_contains":
		if strings.Contains(result.Stdout, signal.Match) {
			payload, err := watcherCommandPayload(signal, result)
			return true, watcherSummary(signal, result, execErr), payload, err
		}
		return false, "", nil, nil
	case "stderr_contains":
		if strings.Contains(result.Stderr, signal.Match) {
			payload, err := watcherCommandPayload(signal, result)
			return true, watcherSummary(signal, result, execErr), payload, err
		}
		return false, "", nil, nil
	case "output_contains":
		if strings.Contains(result.Stdout+"\n"+result.Stderr, signal.Match) {
			payload, err := watcherCommandPayload(signal, result)
			return true, watcherSummary(signal, result, execErr), payload, err
		}
		return false, "", nil, nil
	default:
		if result.ExitCode != 0 || execErr != nil {
			payload, err := watcherCommandPayload(signal, result)
			return true, watcherSummary(signal, result, execErr), payload, err
		}
		return false, "", nil, nil
	}
}

func evaluateWatcherScriptSignal(signal compiledWatcherSignal, result watcherCommandResult, execErr error) (bool, string, json.RawMessage, error) {
	parsed, payload, err := parseAutomationScriptResult(result.Stdout)
	if err != nil {
		return false, "", nil, err
	}
	switch parsed.Status {
	case types.AutomationDetectorStatusHealthy, types.AutomationDetectorStatusRecovered:
		return false, "", payload, nil
	case types.AutomationDetectorStatusNeedsAgent, types.AutomationDetectorStatusNeedsHuman:
		return true, firstNonEmptyString(strings.TrimSpace(parsed.Summary), watcherSummary(signal, result, execErr)), payload, nil
	default:
		return false, "", nil, invalidSignalOutput("detector signal status is unsupported")
	}
}

func parseAutomationScriptResult(stdout string) (types.AutomationDetectorSignal, json.RawMessage, error) {
	raw := json.RawMessage(bytes.TrimSpace([]byte(stdout)))
	if len(raw) == 0 {
		return types.AutomationDetectorSignal{}, nil, invalidSignalOutput("script watcher stdout must be non-empty JSON")
	}
	return parseAutomationDetectorSignalPayload(raw)
}

func watcherCommandPayload(signal compiledWatcherSignal, result watcherCommandResult) (json.RawMessage, error) {
	return json.Marshal(map[string]any{
		"command":   signal.Command,
		"exit_code": result.ExitCode,
		"stdout":    truncateWatcherText(result.Stdout),
		"stderr":    truncateWatcherText(result.Stderr),
	})
}

func watcherSummary(signal compiledWatcherSignal, result watcherCommandResult, execErr error) string {
	if strings.TrimSpace(signal.Summary) != "" {
		return signal.Summary
	}
	if execErr != nil {
		return fmt.Sprintf("%s failed: %v", signal.SignalKind, execErr)
	}
	return fmt.Sprintf("%s matched with exit code %d", signal.SignalKind, result.ExitCode)
}

func executeWatcherCommand(ctx context.Context, req watcherCommandRequest) (watcherCommandResult, error) {
	runCtx := ctx
	cancel := func() {}
	if req.Timeout > 0 {
		runCtx, cancel = context.WithTimeout(ctx, req.Timeout)
	}
	defer cancel()

	cmd := runtimex.NewShellCommandContext(runCtx, req.Command)
	if strings.TrimSpace(req.WorkingDir) != "" {
		cmd.Dir = req.WorkingDir
	}
	if len(req.Env) > 0 {
		env := os.Environ()
		for key, value := range req.Env {
			if strings.TrimSpace(key) == "" {
				continue
			}
			env = append(env, key+"="+value)
		}
		cmd.Env = env
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	result := watcherCommandResult{
		ExitCode: 0,
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
	}
	if err == nil {
		return result, nil
	}

	result.ExitCode = -1
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		result.ExitCode = exitErr.ExitCode()
		return result, nil
	}
	return result, err
}

func loadWatcherRunnerState(path string) (watcherRunnerState, error) {
	if strings.TrimSpace(path) == "" {
		return watcherRunnerState{Signals: map[string]watcherSignalState{}}, nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return watcherRunnerState{Signals: map[string]watcherSignalState{}}, nil
		}
		return watcherRunnerState{}, err
	}
	var state watcherRunnerState
	if len(bytes.TrimSpace(raw)) == 0 {
		return watcherRunnerState{Signals: map[string]watcherSignalState{}}, nil
	}
	if err := json.Unmarshal(raw, &state); err != nil {
		return watcherRunnerState{}, err
	}
	if state.Signals == nil {
		state.Signals = map[string]watcherSignalState{}
	}
	return state, nil
}

func minWatcherInterval(signals []compiledWatcherSignal) time.Duration {
	interval := time.Minute
	for _, signal := range signals {
		if signal.Interval > 0 && signal.Interval < interval {
			interval = signal.Interval
		}
	}
	if interval <= 0 {
		return time.Minute
	}
	return interval
}

func normalizeTriggerOn(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "stdout_contains", "stderr_contains", "output_contains", "script_status":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "nonzero_exit"
	}
}

func isContinuousLifecycle(lifecycle watcherLifecycleConfig) bool {
	switch strings.ToLower(strings.TrimSpace(lifecycle.Mode)) {
	case "", "continuous":
		return true
	default:
		return false
	}
}

func firstNonEmptyWatcherStatus(status string, triggered bool) string {
	status = strings.TrimSpace(status)
	if status != "" {
		return status
	}
	if triggered {
		return "triggered"
	}
	return "healthy"
}

func truncateWatcherText(value string) string {
	value = strings.TrimSpace(value)
	if len(value) <= 1024 {
		return value
	}
	return value[:1024]
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func firstNonNilExec(execFn func(context.Context, watcherCommandRequest) (watcherCommandResult, error)) func(context.Context, watcherCommandRequest) (watcherCommandResult, error) {
	if execFn != nil {
		return execFn
	}
	return executeWatcherCommand
}

func firstNonNilSleep(sleepFn func(context.Context, time.Duration) error) func(context.Context, time.Duration) error {
	if sleepFn != nil {
		return sleepFn
	}
	return func(ctx context.Context, duration time.Duration) error {
		timer := time.NewTimer(duration)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
			return nil
		}
	}
}
