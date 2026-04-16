package task

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"go-agent/internal/config"
	"go-agent/internal/types"
)

type runningTask struct {
	workspaceRoot string
	cancel        context.CancelFunc
}

type workspaceState struct {
	outputsDir string
	todos      []TodoItem
	loaded     bool
}

type Config struct {
	MaxConcurrentTasks int
	TaskOutputMaxBytes int
	TerminalNotifier   TerminalNotifier
	WorkspaceStore     WorkspaceStore
}

type Manager struct {
	mu               sync.RWMutex
	tasks            map[string]*Task
	workspaces       map[string]*workspaceState
	running          map[string]*runningTask
	waiters          map[string]chan struct{}
	runners          map[TaskType]Runner
	cfg              Config
	remote           RemoteExecutorConfig
	terminalNotifier TerminalNotifier
	workspaceStore   WorkspaceStore
}

func NewManager(cfg Config, runners map[TaskType]Runner, agentExecutor AgentExecutor) *Manager {
	manager := &Manager{
		tasks:            make(map[string]*Task),
		workspaces:       make(map[string]*workspaceState),
		running:          make(map[string]*runningTask),
		waiters:          make(map[string]chan struct{}),
		runners:          make(map[TaskType]Runner),
		cfg:              cfg,
		terminalNotifier: cfg.TerminalNotifier,
		workspaceStore:   cfg.WorkspaceStore,
	}
	manager.registerDefaultRunners(agentExecutor)
	for taskType, runner := range runners {
		manager.runners[taskType] = runner
	}
	return manager
}

func (m *Manager) registerDefaultRunners(agentExecutor AgentExecutor) {
	m.runners[TaskTypeShell] = ShellRunner{}
	if agentExecutor != nil {
		m.runners[TaskTypeAgent] = NewAgentRunner(agentExecutor)
	}
	m.runners[TaskTypeRemote] = RemoteRunner{config: m.remote}
}

func (m *Manager) Create(_ context.Context, in CreateTaskInput) (Task, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	workspaceRoot := normalizeWorkspaceRoot(in.WorkspaceRoot)
	state, err := m.ensureWorkspaceLocked(workspaceRoot)
	if err != nil {
		return Task{}, err
	}

	taskID := strings.TrimSpace(in.ID)
	if taskID == "" {
		taskID = types.NewID("task")
	}
	task := &Task{
		ID:                  taskID,
		Type:                in.Type,
		Status:              TaskStatusPending,
		Command:             in.Command,
		Description:         in.Description,
		ParentTaskID:        in.ParentTaskID,
		ParentSessionID:     in.ParentSessionID,
		ParentTurnID:        in.ParentTurnID,
		Owner:               in.Owner,
		Kind:                in.Kind,
		ExecutionTaskID:     taskID,
		WorktreeID:          in.WorktreeID,
		ScheduledJobID:      in.ScheduledJobID,
		ActivatedSkillNames: append([]string(nil), in.ActivatedSkillNames...),
		WorkspaceRoot:       workspaceRoot,
		OutputPath:          filepath.Join(state.outputsDir, taskID+".log"),
		TimeoutSeconds:      in.TimeoutSeconds,
		StartTime:           time.Now().UTC(),
	}
	m.tasks[task.ID] = task
	m.waiterLocked(task)
	if !in.Start {
		if err := m.saveTaskLocked(task); err != nil {
			delete(m.tasks, task.ID)
			delete(m.waiters, task.ID)
			return Task{}, err
		}
		return *task, nil
	}
	if err := m.startLocked(task); err != nil {
		delete(m.tasks, task.ID)
		delete(m.waiters, task.ID)
		return Task{}, err
	}
	return *task, nil
}

func (m *Manager) List(workspaceRoot string) ([]Task, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	workspaceRoot = normalizeWorkspaceRoot(workspaceRoot)
	if _, err := m.ensureWorkspaceLocked(workspaceRoot); err != nil {
		return nil, err
	}

	tasks := make([]Task, 0)
	for _, task := range m.tasks {
		if task.WorkspaceRoot == workspaceRoot {
			tasks = append(tasks, *task)
		}
	}
	sort.Slice(tasks, func(i, j int) bool {
		return tasks[i].StartTime.After(tasks[j].StartTime)
	})
	return tasks, nil
}

func (m *Manager) Get(taskID, workspaceRoot string) (Task, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	workspaceRoot = normalizeWorkspaceRoot(workspaceRoot)
	if _, err := m.ensureWorkspaceLocked(workspaceRoot); err != nil {
		return Task{}, false, err
	}
	task, ok := m.tasks[taskID]
	if !ok || task.WorkspaceRoot != workspaceRoot {
		return Task{}, false, nil
	}
	return *task, true, nil
}

func (m *Manager) Update(taskID, workspaceRoot string, in UpdateTaskInput) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	workspaceRoot = normalizeWorkspaceRoot(workspaceRoot)
	if _, err := m.ensureWorkspaceLocked(workspaceRoot); err != nil {
		return err
	}
	task, ok := m.tasks[taskID]
	if !ok || task.WorkspaceRoot != workspaceRoot {
		return fmt.Errorf("task %q not found", taskID)
	}
	if err := validateStatusTransition(task.Status, in.Status); err != nil {
		return err
	}

	task.Status = in.Status
	if in.Description != "" {
		task.Description = in.Description
	}
	if in.Owner != "" {
		task.Owner = in.Owner
	}
	if in.WorktreeID != "" {
		task.WorktreeID = in.WorktreeID
	}
	if isTerminalStatus(task.Status) {
		now := time.Now().UTC()
		task.EndTime = &now
		m.markTerminalLocked(task)
	}
	return m.saveTaskLocked(task)
}

func (m *Manager) Stop(taskID, workspaceRoot string) error {
	m.mu.Lock()
	workspaceRoot = normalizeWorkspaceRoot(workspaceRoot)
	if _, err := m.ensureWorkspaceLocked(workspaceRoot); err != nil {
		m.mu.Unlock()
		return err
	}

	task, ok := m.tasks[taskID]
	if !ok || task.WorkspaceRoot != workspaceRoot {
		m.mu.Unlock()
		return fmt.Errorf("task %q not found", taskID)
	}
	if isTerminalStatus(task.Status) {
		m.mu.Unlock()
		return nil
	}
	if task.Status == TaskStatusPending {
		task.Status = TaskStatusStopped
		now := time.Now().UTC()
		task.EndTime = &now
		m.markTerminalLocked(task)
		err := m.saveTaskLocked(task)
		m.mu.Unlock()
		return err
	}

	handle, ok := m.running[taskID]
	if !ok {
		m.mu.Unlock()
		return fmt.Errorf("task %q is not running", taskID)
	}
	cancel := handle.cancel
	m.mu.Unlock()
	cancel()
	return nil
}

func (m *Manager) ReadOutput(taskID, workspaceRoot string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	workspaceRoot = normalizeWorkspaceRoot(workspaceRoot)
	if _, err := m.ensureWorkspaceLocked(workspaceRoot); err != nil {
		return "", err
	}
	task, ok := m.tasks[taskID]
	if !ok || task.WorkspaceRoot != workspaceRoot {
		return "", fmt.Errorf("task %q not found", taskID)
	}

	if task.OutputPath != "" {
		data, err := os.ReadFile(task.OutputPath)
		if err == nil {
			return string(data), nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
	}
	return task.Output, nil
}

func (m *Manager) Wait(ctx context.Context, taskID, workspaceRoot string) (Task, bool, error) {
	workspaceRoot = normalizeWorkspaceRoot(workspaceRoot)
	for {
		m.mu.Lock()
		if _, err := m.ensureWorkspaceLocked(workspaceRoot); err != nil {
			m.mu.Unlock()
			return Task{}, false, err
		}
		task, ok := m.tasks[taskID]
		if !ok || task.WorkspaceRoot != workspaceRoot {
			m.mu.Unlock()
			return Task{}, false, fmt.Errorf("task %q not found", taskID)
		}
		snapshot := copyTask(*task)
		if isTerminalStatus(task.Status) {
			m.mu.Unlock()
			return snapshot, false, nil
		}
		waiter := m.waiterLocked(task)
		m.mu.Unlock()

		select {
		case <-waiter:
			continue
		case <-ctx.Done():
			current, ok, err := m.Get(taskID, workspaceRoot)
			if err != nil {
				return Task{}, false, err
			}
			if !ok {
				return Task{}, false, fmt.Errorf("task %q not found", taskID)
			}
			if isTerminalStatus(current.Status) {
				return current, false, nil
			}
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				return current, true, nil
			}
			return Task{}, false, ctx.Err()
		}
	}
}

func (m *Manager) WriteTodos(workspaceRoot string, todos []TodoItem) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	workspaceRoot = normalizeWorkspaceRoot(workspaceRoot)
	state, err := m.ensureWorkspaceLocked(workspaceRoot)
	if err != nil {
		return err
	}

	state.todos = copyTodoItems(todos)
	if m.workspaceStore == nil {
		return nil
	}
	return m.workspaceStore.ReplaceWorkspaceTodos(context.Background(), workspaceRoot, state.todos)
}

func (m *Manager) ReadTodos(workspaceRoot string) ([]TodoItem, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	workspaceRoot = normalizeWorkspaceRoot(workspaceRoot)
	state, err := m.ensureWorkspaceLocked(workspaceRoot)
	if err != nil {
		return nil, err
	}
	return copyTodoItems(state.todos), nil
}

func (m *Manager) ReadResult(taskID, workspaceRoot string) (FinalResult, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	workspaceRoot = normalizeWorkspaceRoot(workspaceRoot)
	if _, err := m.ensureWorkspaceLocked(workspaceRoot); err != nil {
		return FinalResult{}, false, err
	}
	task, ok := m.tasks[taskID]
	if !ok || task.WorkspaceRoot != workspaceRoot {
		return FinalResult{}, false, fmt.Errorf("task %q not found", taskID)
	}
	result, ready := task.FinalResult()
	if !ready {
		return FinalResult{}, false, nil
	}
	return result, true, nil
}

func (m *Manager) SetRemoteConfig(cfg RemoteExecutorConfig) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.remote = cfg
	if runner, ok := m.runners[TaskTypeRemote]; ok {
		remoteRunner, ok := runner.(RemoteRunner)
		if ok {
			remoteRunner.config = cfg
			m.runners[TaskTypeRemote] = remoteRunner
		}
	}
}

func (m *Manager) SetFinalText(taskID, text string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	task, ok := m.tasks[taskID]
	if !ok {
		return fmt.Errorf("task %q not found", taskID)
	}
	now := time.Now().UTC()
	task.FinalResultKind = FinalResultKindAssistantText
	task.FinalResultText = text
	task.FinalResultReadyAt = &now
	return m.saveTaskLocked(task)
}

func (m *Manager) SetOutcome(taskID string, outcome types.ChildAgentOutcome, summary string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	task, ok := m.tasks[taskID]
	if !ok {
		return fmt.Errorf("task %q not found", taskID)
	}
	task.Outcome = normalizeChildAgentOutcome(outcome)
	task.OutcomeSummary = strings.TrimSpace(summary)
	return m.saveTaskLocked(task)
}

func normalizeChildAgentOutcome(outcome types.ChildAgentOutcome) types.ChildAgentOutcome {
	normalized := types.ChildAgentOutcome(strings.ToLower(strings.TrimSpace(string(outcome))))
	switch normalized {
	case types.ChildAgentOutcomeSuccess, types.ChildAgentOutcomeFailure, types.ChildAgentOutcomeBlocked:
		return normalized
	default:
		return ""
	}
}

func (m *Manager) Append(taskID string, chunk []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	task, ok := m.tasks[taskID]
	if !ok {
		return fmt.Errorf("task %q not found", taskID)
	}

	next := task.Output + string(chunk)
	if m.cfg.TaskOutputMaxBytes > 0 && len(next) > m.cfg.TaskOutputMaxBytes {
		next = next[:m.cfg.TaskOutputMaxBytes]
	}
	task.Output = next
	return os.WriteFile(task.OutputPath, []byte(task.Output), 0o644)
}

func (m *Manager) ensureWorkspaceLocked(workspaceRoot string) (*workspaceState, error) {
	workspaceRoot = normalizeWorkspaceRoot(workspaceRoot)
	state, ok := m.workspaces[workspaceRoot]
	if !ok {
		workspaceStateDir := filepath.Join(workspaceRoot, config.DirName)
		outputsDir := filepath.Join(workspaceStateDir, "tasks")
		if err := os.MkdirAll(outputsDir, 0o755); err != nil {
			return nil, err
		}
		state = &workspaceState{
			outputsDir: outputsDir,
		}
		m.workspaces[workspaceRoot] = state
	}
	if state.loaded {
		return state, nil
	}

	persistedTasks := make([]Task, 0)
	if m.workspaceStore != nil {
		var err error
		persistedTasks, err = m.workspaceStore.ListWorkspaceTasks(context.Background(), workspaceRoot)
		if err != nil {
			return nil, err
		}
		state.todos, err = m.workspaceStore.GetWorkspaceTodos(context.Background(), workspaceRoot)
		if err != nil {
			return nil, err
		}
	}

	pendingSaves := make([]*Task, 0)
	for _, persistedTask := range persistedTasks {
		taskCopy := persistedTask
		if taskCopy.Status == TaskStatusRunning {
			taskCopy.Status = TaskStatusFailed
			taskCopy.Error = "task interrupted by process restart"
			now := time.Now().UTC()
			taskCopy.EndTime = &now
			pendingSaves = append(pendingSaves, &taskCopy)
		}
		m.tasks[taskCopy.ID] = &taskCopy
		waiter := m.waiterLocked(&taskCopy)
		if isTerminalStatus(taskCopy.Status) {
			closeWaiter(waiter)
		}
	}

	state.loaded = true
	for _, persistedTask := range pendingSaves {
		if err := m.saveTaskLocked(persistedTask); err != nil {
			return nil, err
		}
	}
	return state, nil
}

func (m *Manager) saveTaskLocked(task *Task) error {
	if task == nil || m.workspaceStore == nil {
		return nil
	}
	return m.workspaceStore.UpsertWorkspaceTask(context.Background(), copyTask(*task))
}

func (m *Manager) startLocked(task *Task) error {
	runner, ok := m.runners[task.Type]
	if !ok {
		return fmt.Errorf("task type %q is not supported", task.Type)
	}
	if remoteRunner, ok := runner.(RemoteRunner); ok && strings.TrimSpace(remoteRunner.config.ShimCommand) == "" {
		return fmt.Errorf("remote runner is not configured")
	}

	baseCtx := context.Background()
	var (
		runCtx context.Context
		cancel context.CancelFunc
	)
	if task.TimeoutSeconds > 0 {
		runCtx, cancel = context.WithTimeout(baseCtx, time.Duration(task.TimeoutSeconds)*time.Second)
	} else {
		runCtx, cancel = context.WithCancel(baseCtx)
	}
	handle := &runningTask{
		workspaceRoot: task.WorkspaceRoot,
		cancel:        cancel,
	}
	m.running[task.ID] = handle
	task.Status = TaskStatusRunning
	if err := m.saveTaskLocked(task); err != nil {
		delete(m.running, task.ID)
		cancel()
		return err
	}

	go func(snapshot Task, run Runner) {
		err := run.Run(runCtx, &snapshot, m)
		m.finishRun(snapshot.ID, snapshot.WorkspaceRoot, err, runCtx.Err())
	}(copyTask(*task), runner)
	return nil
}

func (m *Manager) finishRun(taskID, workspaceRoot string, runErr error, ctxErr error) {
	m.mu.Lock()
	task, ok := m.tasks[taskID]
	if !ok {
		m.mu.Unlock()
		return
	}
	delete(m.running, taskID)

	now := time.Now().UTC()
	task.EndTime = &now
	switch {
	case errors.Is(ctxErr, context.Canceled):
		task.Status = TaskStatusStopped
	case runErr != nil:
		task.Status = TaskStatusFailed
		task.Error = runErr.Error()
	default:
		task.Status = TaskStatusCompleted
	}
	m.markTerminalLocked(task)
	snapshot := copyTask(*task)
	notifier := m.terminalNotifier
	_ = m.saveTaskLocked(task)
	m.mu.Unlock()

	if notifier == nil {
		return
	}
	if err := notifier.NotifyTaskTerminal(context.Background(), snapshot); err != nil {
		return
	}
	if !shouldMarkCompletionNotified(snapshot) {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	current, ok := m.tasks[taskID]
	if !ok || current.CompletionNotifiedAt != nil {
		return
	}
	notifiedAt := time.Now().UTC()
	current.CompletionNotifiedAt = &notifiedAt
	_ = m.saveTaskLocked(current)
}

func (m *Manager) waiterLocked(task *Task) chan struct{} {
	if task == nil {
		return nil
	}
	if waiter, ok := m.waiters[task.ID]; ok {
		return waiter
	}
	waiter := make(chan struct{})
	if isTerminalStatus(task.Status) {
		close(waiter)
	}
	m.waiters[task.ID] = waiter
	return waiter
}

func (m *Manager) markTerminalLocked(task *Task) {
	if task == nil {
		return
	}
	closeWaiter(m.waiterLocked(task))
}

func closeWaiter(waiter chan struct{}) {
	if waiter == nil {
		return
	}
	select {
	case <-waiter:
		return
	default:
		close(waiter)
	}
}

func normalizeWorkspaceRoot(workspaceRoot string) string {
	return filepath.ToSlash(filepath.Clean(workspaceRoot))
}

func copyTask(task Task) Task {
	copy := task
	copy.ActivatedSkillNames = append([]string(nil), task.ActivatedSkillNames...)
	if task.EndTime != nil {
		end := *task.EndTime
		copy.EndTime = &end
	}
	if task.FinalResultReadyAt != nil {
		readyAt := *task.FinalResultReadyAt
		copy.FinalResultReadyAt = &readyAt
	}
	if task.CompletionNotifiedAt != nil {
		notifiedAt := *task.CompletionNotifiedAt
		copy.CompletionNotifiedAt = &notifiedAt
	}
	return copy
}

func copyTodoItems(todos []TodoItem) []TodoItem {
	if len(todos) == 0 {
		return nil
	}
	out := make([]TodoItem, len(todos))
	copy(out, todos)
	return out
}

func shouldMarkCompletionNotified(task Task) bool {
	return task.Status == TaskStatusCompleted &&
		task.ResultReady() &&
		task.ParentSessionID != "" &&
		task.CompletionNotifiedAt == nil
}
