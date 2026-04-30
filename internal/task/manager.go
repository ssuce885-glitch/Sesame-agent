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
		TargetRole:          strings.TrimSpace(in.TargetRole),
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
