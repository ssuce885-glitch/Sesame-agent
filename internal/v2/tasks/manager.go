package tasks

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"go-agent/internal/types"
	"go-agent/internal/v2/contracts"
	"go-agent/internal/v2/observability"
)

// Runner executes a task. Each task kind (shell, agent, remote) has a runner.
type Runner interface {
	Run(ctx context.Context, task contracts.Task, sink OutputSink) error
}

// OutputSink receives streaming task output.
type OutputSink interface {
	Append(taskID string, data []byte) error
}

type Reporter interface {
	DeliverTaskReport(ctx context.Context, task contracts.Task) error
}

type Manager struct {
	mu         sync.RWMutex
	tasks      map[string]contracts.Task
	workspaces map[string]*workspaceState
	running    map[string]context.CancelFunc
	waiters    map[string]chan struct{}
	runners    map[string]Runner
	reporter   Reporter
	metrics    *observability.Collector
	store      contracts.Store
	outputsDir string
}

type workspaceState struct {
	outputsDir string
}

func NewManager(s contracts.Store, outputsDir string, metrics ...*observability.Collector) *Manager {
	if strings.TrimSpace(outputsDir) == "" {
		outputsDir = filepath.Join(os.TempDir(), "sesame-task-outputs")
	}
	m := &Manager{
		tasks:      make(map[string]contracts.Task),
		workspaces: make(map[string]*workspaceState),
		running:    make(map[string]context.CancelFunc),
		waiters:    make(map[string]chan struct{}),
		runners:    make(map[string]Runner),
		store:      s,
		outputsDir: outputsDir,
	}
	if len(metrics) > 0 {
		m.metrics = metrics[0]
	}
	m.runners["shell"] = &ShellRunner{}
	return m
}

func (m *Manager) RegisterRunner(kind string, runner Runner) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if strings.TrimSpace(kind) == "" || runner == nil {
		return
	}
	m.runners[strings.TrimSpace(kind)] = runner
}

func (m *Manager) SetReporter(reporter Reporter) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.reporter = reporter
}

func (m *Manager) SetMetrics(metrics *observability.Collector) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.metrics = metrics
}

// Create creates a new task and persists it.
func (m *Manager) Create(ctx context.Context, task contracts.Task) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	task = m.normalizeTaskLocked(task)
	if err := m.store.Tasks().Create(ctx, task); err != nil {
		return err
	}
	m.tasks[task.ID] = task
	m.waiterLocked(task.ID, isTerminalState(task.State))
	if m.metrics != nil {
		m.metrics.RecordTaskCreated()
	}
	return nil
}

// Start begins executing a pending task in a goroutine.
func (m *Manager) Start(ctx context.Context, taskID string) error {
	m.mu.Lock()
	task, ok := m.tasks[taskID]
	if !ok {
		var err error
		task, err = m.store.Tasks().Get(ctx, taskID)
		if err != nil {
			m.mu.Unlock()
			return err
		}
		task = m.normalizeTaskLocked(task)
		m.tasks[task.ID] = task
	}
	if task.State != "pending" {
		m.mu.Unlock()
		return fmt.Errorf("task %q is %s", taskID, task.State)
	}
	runner, ok := m.runners[task.Kind]
	if !ok {
		m.mu.Unlock()
		return fmt.Errorf("task kind %q is not supported", task.Kind)
	}

	runCtx, cancel := context.WithCancel(context.Background())
	task.State = "running"
	task.UpdatedAt = time.Now().UTC()
	m.tasks[task.ID] = task
	m.running[task.ID] = cancel
	m.waiterLocked(task.ID, false)
	if err := m.store.Tasks().Update(ctx, task); err != nil {
		delete(m.running, task.ID)
		cancel()
		task.State = "pending"
		m.tasks[task.ID] = task
		m.mu.Unlock()
		return err
	}
	m.mu.Unlock()

	go m.runTask(runCtx, task, runner)
	return nil
}

// Fail marks a task as failed and persists the terminal state.
func (m *Manager) Fail(ctx context.Context, taskID string, finalText string) error {
	m.mu.Lock()
	task, ok := m.tasks[taskID]
	if !ok {
		var err error
		task, err = m.store.Tasks().Get(ctx, taskID)
		if err != nil {
			m.mu.Unlock()
			return err
		}
		task = m.normalizeTaskLocked(task)
		m.tasks[task.ID] = task
	}
	if isTerminalState(task.State) {
		m.mu.Unlock()
		return nil
	}
	if cancel, ok := m.running[taskID]; ok {
		delete(m.running, taskID)
		cancel()
	}

	task.State = "failed"
	task.Outcome = "failure"
	task.FinalText = strings.TrimSpace(finalText)
	if task.FinalText == "" {
		task.FinalText = "Task failed."
	}
	task.UpdatedAt = time.Now().UTC()
	if err := m.store.Tasks().Update(ctx, task); err != nil {
		m.mu.Unlock()
		return err
	}
	reportErr := m.deliverReportLocked(context.WithoutCancel(ctx), task)
	m.tasks[task.ID] = task
	waiter := m.waiterLocked(task.ID, true)
	closeWaiter(waiter)
	if m.metrics != nil {
		m.metrics.RecordTaskDone(task.State)
	}
	m.mu.Unlock()
	return reportErr
}

// Cancel stops a running task.
func (m *Manager) Cancel(ctx context.Context, taskID string) error {
	m.mu.Lock()
	task, ok := m.tasks[taskID]
	if !ok {
		var err error
		task, err = m.store.Tasks().Get(ctx, taskID)
		if err != nil {
			m.mu.Unlock()
			return err
		}
		m.tasks[task.ID] = task
	}
	if isTerminalState(task.State) {
		m.mu.Unlock()
		return nil
	}
	if cancel, ok := m.running[taskID]; ok {
		m.mu.Unlock()
		cancel()
		return nil
	}
	task.State = "cancelled"
	task.Outcome = "cancelled"
	task.FinalText = "Task cancelled."
	task.UpdatedAt = time.Now().UTC()
	if err := m.store.Tasks().Update(ctx, task); err != nil {
		m.mu.Unlock()
		return err
	}
	reportErr := m.deliverReportLocked(context.Background(), task)
	m.tasks[task.ID] = task
	waiter := m.waiterLocked(task.ID, true)
	closeWaiter(waiter)
	if m.metrics != nil {
		m.metrics.RecordTaskDone(task.State)
	}
	m.mu.Unlock()
	return reportErr
}

// Get returns a task by ID.
func (m *Manager) Get(taskID string) (contracts.Task, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	task, ok := m.tasks[taskID]
	return task, ok
}

// ListByWorkspace returns all tasks for a workspace.
func (m *Manager) ListByWorkspace(workspaceRoot string) []contracts.Task {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var tasks []contracts.Task
	for _, task := range m.tasks {
		if task.WorkspaceRoot == workspaceRoot {
			tasks = append(tasks, task)
		}
	}
	sort.Slice(tasks, func(i, j int) bool {
		return tasks[i].CreatedAt.Before(tasks[j].CreatedAt)
	})
	return tasks
}

// Wait blocks until the task completes and returns the final task state.
func (m *Manager) Wait(ctx context.Context, taskID string) (contracts.Task, error) {
	for {
		m.mu.Lock()
		task, ok := m.tasks[taskID]
		if !ok {
			m.mu.Unlock()
			return contracts.Task{}, fmt.Errorf("task %q not found", taskID)
		}
		if isTerminalState(task.State) {
			m.mu.Unlock()
			return task, nil
		}
		waiter := m.waiterLocked(taskID, false)
		m.mu.Unlock()

		select {
		case <-waiter:
			continue
		case <-ctx.Done():
			if task, ok := m.Get(taskID); ok && isTerminalState(task.State) {
				return task, nil
			}
			return contracts.Task{}, ctx.Err()
		}
	}
}

func (m *Manager) runTask(ctx context.Context, task contracts.Task, runner Runner) {
	state := m.workspaceState(task.WorkspaceRoot)
	sink := NewFileSink(state.outputsDir)
	err := runner.Run(ctx, task, sink)
	if closeErr := sink.Close(task.ID); closeErr != nil {
		err = errors.Join(err, closeErr)
	}
	m.finishRun(task.ID, err, ctx.Err())
}

func (m *Manager) finishRun(taskID string, runErr error, ctxErr error) {
	m.mu.Lock()
	task, ok := m.tasks[taskID]
	if !ok {
		m.mu.Unlock()
		return
	}
	if persisted, err := m.store.Tasks().Get(context.Background(), taskID); err == nil {
		task = persisted
	}
	switch {
	case errors.Is(ctxErr, context.Canceled):
		task.State = "cancelled"
		task.Outcome = "cancelled"
		if strings.TrimSpace(task.FinalText) == "" {
			task.FinalText = "Task cancelled."
		}
	case runErr != nil:
		task.State = "failed"
		task.Outcome = "failure"
		if strings.TrimSpace(task.FinalText) == "" {
			task.FinalText = "Task failed: " + runErr.Error()
		}
	default:
		task.State = "completed"
		task.Outcome = "success"
		if strings.TrimSpace(task.FinalText) == "" {
			task.FinalText = "Task completed successfully."
		}
	}
	task.UpdatedAt = time.Now().UTC()
	if err := retryDatabaseBusy(context.Background(), func(ctx context.Context) error {
		return m.store.Tasks().Update(ctx, task)
	}); err != nil {
		slog.Warn("failed to persist v2 task state", "task_id", task.ID, "error", err)
	}
	if err := retryDatabaseBusy(context.Background(), func(ctx context.Context) error {
		return m.deliverReportLocked(ctx, task)
	}); err != nil {
		slog.Warn("failed to deliver v2 task report", "task_id", task.ID, "error", err)
	}

	delete(m.running, taskID)
	m.tasks[task.ID] = task
	waiter := m.waiterLocked(task.ID, true)
	closeWaiter(waiter)
	if m.metrics != nil {
		m.metrics.RecordTaskDone(task.State)
	}
	m.mu.Unlock()
}

func (m *Manager) normalizeTaskLocked(task contracts.Task) contracts.Task {
	if strings.TrimSpace(task.ID) == "" {
		task.ID = types.NewID("task")
	}
	task.WorkspaceRoot = strings.TrimSpace(task.WorkspaceRoot)
	if strings.TrimSpace(task.Kind) == "" {
		task.Kind = "shell"
	}
	if strings.TrimSpace(task.State) == "" {
		task.State = "pending"
	}
	now := time.Now().UTC()
	if task.CreatedAt.IsZero() {
		task.CreatedAt = now
	}
	if task.UpdatedAt.IsZero() {
		task.UpdatedAt = task.CreatedAt
	}
	if strings.TrimSpace(task.OutputPath) == "" {
		state := m.workspaceStateLocked(task.WorkspaceRoot)
		task.OutputPath = filepath.Join(state.outputsDir, task.ID+".log")
	}
	return task
}

func (m *Manager) workspaceState(workspaceRoot string) *workspaceState {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.workspaceStateLocked(workspaceRoot)
}

func (m *Manager) workspaceStateLocked(workspaceRoot string) *workspaceState {
	if state, ok := m.workspaces[workspaceRoot]; ok {
		return state
	}
	sum := sha256.Sum256([]byte(workspaceRoot))
	dir := filepath.Join(m.outputsDir, hex.EncodeToString(sum[:8]))
	state := &workspaceState{outputsDir: dir}
	m.workspaces[workspaceRoot] = state
	return state
}

func (m *Manager) waiterLocked(taskID string, closed bool) chan struct{} {
	waiter, ok := m.waiters[taskID]
	if !ok {
		waiter = make(chan struct{})
		m.waiters[taskID] = waiter
	}
	if closed {
		closeWaiter(waiter)
	}
	return waiter
}

func closeWaiter(waiter chan struct{}) {
	select {
	case <-waiter:
	default:
		close(waiter)
	}
}

func isTerminalState(state string) bool {
	switch state {
	case "completed", "failed", "cancelled":
		return true
	default:
		return false
	}
}

func (m *Manager) deliverReportLocked(ctx context.Context, task contracts.Task) error {
	sessionID := firstNonEmpty(task.ReportSessionID, task.SessionID)
	if strings.TrimSpace(sessionID) == "" {
		return nil
	}
	reporter := m.reporter
	if reporter != nil {
		return reporter.DeliverTaskReport(ctx, task)
	}
	report := contracts.Report{
		ID:         types.NewID("report"),
		SessionID:  sessionID,
		SourceKind: "task_result",
		SourceID:   task.ID,
		Status:     task.State,
		Severity:   severityFromOutcome(task.Outcome),
		Title:      "Task result: " + task.Kind,
		Summary:    task.FinalText,
		CreatedAt:  time.Now().UTC(),
	}
	return m.store.Reports().Create(ctx, report)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func severityFromOutcome(outcome string) string {
	switch outcome {
	case "failure":
		return "error"
	case "cancelled":
		return "warning"
	default:
		return "info"
	}
}
