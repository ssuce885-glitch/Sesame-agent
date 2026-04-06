package task

import (
	"context"
	"errors"
)

var errAgentExecutorNotConfigured = errors.New("agent executor is not configured")

type AgentRunner struct {
	executor AgentExecutor
}

func NewAgentRunner(executor AgentExecutor) Runner {
	return AgentRunner{
		executor: executor,
	}
}

func (r AgentRunner) Run(ctx context.Context, task *Task, sink OutputSink) error {
	if r.executor == nil {
		return errAgentExecutorNotConfigured
	}

	writer := outputSinkWriter{
		taskID: task.ID,
		sink:   sink,
	}
	return r.executor.RunTask(ctx, task.WorkspaceRoot, task.Command, writer)
}

type outputSinkWriter struct {
	taskID string
	sink   OutputSink
}

func (w outputSinkWriter) Write(p []byte) (int, error) {
	if err := w.sink.Append(w.taskID, p); err != nil {
		return 0, err
	}
	return len(p), nil
}
