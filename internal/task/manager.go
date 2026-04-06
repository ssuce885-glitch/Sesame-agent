package task

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"go-agent/internal/types"
)

type runningTask struct {
	workspaceRoot string
	cancel        context.CancelFunc
	done          chan struct{}
}

type workspaceState struct {
	tasksFile  string
	todosFile  string
	outputsDir string
	loaded     bool
}

type Config struct {
	MaxConcurrentTasks int
	TaskOutputMaxBytes int
}

type Manager struct {
	mu         sync.RWMutex
	tasks      map[string]*Task
	workspaces map[string]*workspaceState
	running    map[string]*runningTask
	runners    map[TaskType]Runner
	cfg        Config
	remote     RemoteExecutorConfig
}

func NewManager(cfg Config, runners map[TaskType]Runner, agentExecutor AgentExecutor) *Manager {
	manager := &Manager{
		tasks:      make(map[string]*Task),
		workspaces: make(map[string]*workspaceState),
		running:    make(map[string]*runningTask),
		runners:    make(map[TaskType]Runner),
		cfg:        cfg,
	}
	manager.registerDefaultRunners(agentExecutor)
	for taskType, runner := range runners {
		manager.runners[taskType] = runner
	}
	return manager
}

func (m *Manager) registerDefaultRunners(agentExecutor AgentExecutor) {
	if agentExecutor != nil {
		m.runners[TaskTypeAgent] = NewAgentRunner(agentExecutor)
	}
}

func (m *Manager) Create(_ context.Context, in CreateTaskInput) (Task, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	workspaceRoot := normalizeWorkspaceRoot(in.WorkspaceRoot)
	state, err := m.ensureWorkspaceLocked(workspaceRoot)
	if err != nil {
		return Task{}, err
	}

	taskID := types.NewID("task")
	task := &Task{
		ID:            taskID,
		Type:          in.Type,
		Status:        TaskStatusPending,
		Command:       in.Command,
		Description:   in.Description,
		WorkspaceRoot: workspaceRoot,
		OutputPath:    filepath.Join(state.outputsDir, taskID+".log"),
		StartTime:     time.Now().UTC(),
	}
	m.tasks[task.ID] = task
	if err := m.saveWorkspaceLocked(workspaceRoot); err != nil {
		delete(m.tasks, task.ID)
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
	if isTerminalStatus(task.Status) {
		now := time.Now().UTC()
		task.EndTime = &now
	}
	return m.saveWorkspaceLocked(workspaceRoot)
}

func (m *Manager) ensureWorkspaceLocked(workspaceRoot string) (*workspaceState, error) {
	workspaceRoot = normalizeWorkspaceRoot(workspaceRoot)
	state, ok := m.workspaces[workspaceRoot]
	if !ok {
		claudeDir := filepath.Join(workspaceRoot, ".claude")
		outputsDir := filepath.Join(claudeDir, "tasks")
		if err := os.MkdirAll(outputsDir, 0o755); err != nil {
			return nil, err
		}
		state = &workspaceState{
			tasksFile:  filepath.Join(claudeDir, "tasks.json"),
			todosFile:  filepath.Join(claudeDir, "todos.json"),
			outputsDir: outputsDir,
		}
		m.workspaces[workspaceRoot] = state
	}
	if state.loaded {
		return state, nil
	}

	persistedTasks, err := loadTasksFile(state.tasksFile)
	if err != nil {
		return nil, err
	}

	needsSave := false
	for _, persistedTask := range persistedTasks {
		taskCopy := persistedTask
		if taskCopy.Status == TaskStatusRunning {
			taskCopy.Status = TaskStatusFailed
			taskCopy.Error = "task interrupted by process restart"
			now := time.Now().UTC()
			taskCopy.EndTime = &now
			needsSave = true
		}
		m.tasks[taskCopy.ID] = &taskCopy
	}

	state.loaded = true
	if needsSave {
		if err := m.saveWorkspaceLocked(workspaceRoot); err != nil {
			return nil, err
		}
	}
	return state, nil
}

func (m *Manager) saveWorkspaceLocked(workspaceRoot string) error {
	workspaceRoot = normalizeWorkspaceRoot(workspaceRoot)
	state := m.workspaces[workspaceRoot]
	tasks := make([]Task, 0)
	for _, task := range m.tasks {
		if task.WorkspaceRoot == workspaceRoot {
			tasks = append(tasks, *task)
		}
	}
	sort.Slice(tasks, func(i, j int) bool {
		return tasks[i].StartTime.Before(tasks[j].StartTime)
	})
	return writeTasksFile(state.tasksFile, tasks)
}

func normalizeWorkspaceRoot(workspaceRoot string) string {
	return filepath.ToSlash(filepath.Clean(workspaceRoot))
}
