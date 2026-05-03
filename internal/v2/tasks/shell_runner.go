package tasks

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	runtimex "go-agent/internal/runtime"
	"go-agent/internal/v2/contracts"
)

type ShellRunner struct{}

const defaultShellTaskTimeout = 30 * time.Minute

func (r *ShellRunner) Run(ctx context.Context, task contracts.Task, sink OutputSink) error {
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, defaultShellTaskTimeout)
		defer cancel()
	}
	cmd := runtimex.NewShellCommandContext(ctx, task.Prompt)
	cmd.Dir = task.WorkspaceRoot

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}

	var wg sync.WaitGroup
	var appendMu sync.Mutex
	var appendErr error
	streamPipe := func(reader io.Reader) {
		defer wg.Done()
		buf := make([]byte, 4096)
		for {
			n, readErr := reader.Read(buf)
			if n > 0 {
				if err := sink.Append(task.ID, buf[:n]); err != nil {
					appendMu.Lock()
					appendErr = errors.Join(appendErr, err)
					appendMu.Unlock()
				}
			}
			if readErr != nil {
				return
			}
		}
	}

	wg.Add(2)
	go streamPipe(stdout)
	go streamPipe(stderr)
	waitErr := cmd.Wait()
	wg.Wait()
	if ctx.Err() == context.DeadlineExceeded {
		waitErr = errors.Join(waitErr, fmt.Errorf("shell task timed out after %s", defaultShellTaskTimeout))
	}

	exitCode := -1
	if cmd.ProcessState != nil {
		exitCode = cmd.ProcessState.ExitCode()
	}
	if err := sink.Append(task.ID, []byte(fmt.Sprintf("\n[exit code: %d]\n", exitCode))); err != nil {
		appendErr = errors.Join(appendErr, err)
	}
	return errors.Join(waitErr, appendErr)
}
