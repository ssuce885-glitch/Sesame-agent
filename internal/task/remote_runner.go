package task

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	runtimex "go-agent/internal/runtime"
)

type RemoteRunner struct {
	config RemoteExecutorConfig
}

func (r RemoteRunner) Run(ctx context.Context, task *Task, sink OutputSink) error {
	if strings.TrimSpace(r.config.ShimCommand) == "" {
		return fmt.Errorf("remote runner is not configured")
	}

	runCtx := ctx
	cancel := func() {}
	if r.config.TimeoutSeconds > 0 {
		runCtx, cancel = context.WithTimeout(ctx, time.Duration(r.config.TimeoutSeconds)*time.Second)
	}
	defer cancel()

	shimCommand := strings.TrimSpace(r.config.ShimCommand)
	var cmd *exec.Cmd
	if info, err := os.Stat(shimCommand); err == nil && !info.IsDir() {
		cmd = exec.CommandContext(runCtx, shimCommand, task.Command)
	} else {
		escapedCommand := strings.ReplaceAll(task.Command, `"`, `""`)
		command := fmt.Sprintf(`%s "%s"`, shimCommand, escapedCommand)
		cmd = runtimex.NewShellCommandContext(runCtx, command)
	}
	cmd.Dir = task.WorkspaceRoot

	output, err := cmd.CombinedOutput()
	var appendErr error
	if len(output) > 0 {
		appendErr = sink.Append(task.ID, output)
	}
	return errors.Join(err, appendErr)
}
