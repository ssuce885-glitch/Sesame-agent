package task

import (
	"context"
	"io"
	"sync"

	runtimex "go-agent/internal/runtime"
)

type ShellRunner struct{}

func (ShellRunner) Run(ctx context.Context, task *Task, sink OutputSink) error {
	cmd := runtimex.NewShellCommandContext(ctx, task.Command)
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
	streamPipe := func(reader io.Reader) {
		defer wg.Done()
		buf := make([]byte, 4096)
		for {
			n, readErr := reader.Read(buf)
			if n > 0 {
				_ = sink.Append(task.ID, buf[:n])
			}
			if readErr != nil {
				return
			}
		}
	}

	wg.Add(2)
	go streamPipe(stdout)
	go streamPipe(stderr)
	err = cmd.Wait()
	wg.Wait()
	return err
}
