package task

import (
	"context"
	"errors"
)

var errAgentExecutorNotConfigured = errors.New("agent executor is not configured")

type defaultRunner struct {
	executor     AgentExecutor
	remoteConfig RemoteExecutorConfig
}

func NewRunner(executor AgentExecutor, remoteConfig RemoteExecutorConfig) Runner {
	return &defaultRunner{
		executor:     executor,
		remoteConfig: remoteConfig,
	}
}

func (r *defaultRunner) Run(ctx context.Context, task Task, sink OutputSink) error {
	if r.executor == nil {
		return errAgentExecutorNotConfigured
	}

	return r.executor.Execute(ctx, task, sink, r.remoteConfig)
}
