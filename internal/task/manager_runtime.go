package task

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

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
