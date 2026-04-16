package task

import (
	"context"
	"errors"
	"go-agent/internal/types"
)

var errAgentExecutorNotConfigured = errors.New("agent executor is not configured")
var errAgentFinalResultSinkNotConfigured = errors.New("agent final result sink is not configured")
var errAgentOutcomeSinkNotConfigured = errors.New("agent outcome sink is not configured")

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
	outcomeSink, ok := sink.(interface {
		SetOutcome(taskID string, outcome types.ChildAgentOutcome, summary string) error
	})
	if !ok {
		return errAgentOutcomeSinkNotConfigured
	}

	observer := outputSinkObserver{
		taskID:      task.ID,
		sink:        sink,
		resultSink:  resultSink,
		outcomeSink: outcomeSink,
	}
	return r.executor.RunTask(ctx, task.ID, task.WorkspaceRoot, task.Command, task.ActivatedSkillNames, observer)
}

type outputSinkObserver struct {
	taskID     string
	sink       OutputSink
	resultSink interface {
		SetFinalText(taskID, text string) error
	}
	outcomeSink interface {
		SetOutcome(taskID string, outcome types.ChildAgentOutcome, summary string) error
	}
}

func (w outputSinkObserver) AppendLog(chunk []byte) error {
	return w.sink.Append(w.taskID, chunk)
}

func (w outputSinkObserver) SetFinalText(text string) error {
	return w.resultSink.SetFinalText(w.taskID, text)
}

func (w outputSinkObserver) SetOutcome(outcome types.ChildAgentOutcome, summary string) error {
	return w.outcomeSink.SetOutcome(w.taskID, outcome, summary)
}

func (w outputSinkObserver) SetRunContext(_, _ string) error {
	return nil
}

func (w outputSinkObserver) Write(p []byte) (int, error) {
	if err := w.AppendLog(p); err != nil {
		return 0, err
	}
	return len(p), nil
}
