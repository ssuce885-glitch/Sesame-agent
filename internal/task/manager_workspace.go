package task

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"go-agent/internal/config"
)

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
