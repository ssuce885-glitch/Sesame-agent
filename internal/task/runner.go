package task

import (
	"context"
	"errors"
)

var errAgentExecutorNotConfigured = errors.New("agent executor is not configured")
var errAgentFinalResultSinkNotConfigured = errors.New("agent final result sink is not configured")

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
	resultSink, ok := sink.(interface {
		SetFinalText(taskID, text string) error
	})
	if !ok {
		return errAgentFinalResultSinkNotConfigured
	}

	observer := outputSinkObserver{
		taskID:     task.ID,
		sink:       sink,
		resultSink: resultSink,
	}
	return r.executor.RunTask(ctx, task.WorkspaceRoot, task.Command, task.ActivatedSkillNames, observer)
}

type outputSinkObserver struct {
	taskID     string
	sink       OutputSink
	resultSink interface {
		SetFinalText(taskID, text string) error
	}
}

func (w outputSinkObserver) AppendLog(chunk []byte) error {
	return w.sink.Append(w.taskID, chunk)
}

func (w outputSinkObserver) SetFinalText(text string) error {
	return w.resultSink.SetFinalText(w.taskID, text)
}

func (w outputSinkObserver) Write(p []byte) (int, error) {
	if err := w.AppendLog(p); err != nil {
		return 0, err
	}
	return len(p), nil
}
