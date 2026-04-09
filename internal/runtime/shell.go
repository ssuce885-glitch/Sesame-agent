package runtime

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	stdruntime "runtime"
	"time"
)

type CommandOutput struct {
	Stdout           string
	Stderr           string
	AggregatedOutput string
	ExitCode         int
	TimedOut         bool
	Duration         time.Duration
	Truncated        bool
}

type cappedBuffer struct {
	buf       bytes.Buffer
	maxBytes  int
	truncated bool
}

func (w *cappedBuffer) Write(p []byte) (int, error) {
	if w == nil {
		return len(p), nil
	}
	if w.maxBytes <= 0 {
		w.truncated = w.truncated || len(p) > 0
		return len(p), nil
	}
	remaining := w.maxBytes - w.buf.Len()
	if remaining <= 0 {
		w.truncated = w.truncated || len(p) > 0
		return len(p), nil
	}
	if len(p) > remaining {
		_, _ = w.buf.Write(p[:remaining])
		w.truncated = true
		return len(p), nil
	}
	_, _ = w.buf.Write(p)
	return len(p), nil
}

type teeCapture struct {
	aggregate *cappedBuffer
	stream    *cappedBuffer
}

func (w teeCapture) Write(p []byte) (int, error) {
	if w.aggregate != nil {
		_, _ = w.aggregate.Write(p)
	}
	if w.stream != nil {
		_, _ = w.stream.Write(p)
	}
	return len(p), nil
}

func RunCommand(ctx context.Context, workdir, command string, maxOutputBytes int) (CommandOutput, error) {
	cmd := NewShellCommandContext(ctx, command)
	if workdir != "" {
		cmd.Dir = workdir
	}

	startedAt := time.Now()
	stdout := &cappedBuffer{maxBytes: maxOutputBytes}
	stderr := &cappedBuffer{maxBytes: maxOutputBytes}
	aggregate := &cappedBuffer{maxBytes: maxOutputBytes}
	cmd.Stdout = teeCapture{aggregate: aggregate, stream: stdout}
	cmd.Stderr = teeCapture{aggregate: aggregate, stream: stderr}

	runErr := cmd.Run()
	result := CommandOutput{
		Stdout:           stdout.buf.String(),
		Stderr:           stderr.buf.String(),
		AggregatedOutput: aggregate.buf.String(),
		Duration:         time.Since(startedAt),
		Truncated:        stdout.truncated || stderr.truncated || aggregate.truncated,
	}

	switch {
	case runErr == nil:
		result.ExitCode = 0
		return result, nil
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		result.ExitCode = -1
		result.TimedOut = true
		return result, nil
	case errors.Is(ctx.Err(), context.Canceled):
		return result, ctx.Err()
	}

	var exitErr *exec.ExitError
	if errors.As(runErr, &exitErr) {
		result.ExitCode = exitErr.ExitCode()
		return result, nil
	}
	return result, runErr
}

func NewShellCommandContext(ctx context.Context, command string) *exec.Cmd {
	if stdruntime.GOOS == "windows" {
		return exec.CommandContext(ctx, "cmd", "/c", command)
	}
	return exec.CommandContext(ctx, "sh", "-lc", command)
}
