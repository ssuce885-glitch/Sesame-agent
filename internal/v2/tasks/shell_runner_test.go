package tasks

import (
	"context"
	"os"
	"path/filepath"
	stdruntime "runtime"
	"testing"
	"time"

	"go-agent/internal/v2/contracts"
)

func TestShellRunnerKillsBackgroundProcessOnTimeout(t *testing.T) {
	if stdruntime.GOOS == "windows" {
		t.Skip("process group test is unix-specific")
	}

	root := t.TempDir()
	sentinel := filepath.Join(root, "child.txt")
	task := contracts.Task{
		ID:            "task_shell_timeout",
		WorkspaceRoot: root,
		Prompt:        "(sleep 2; printf leaked > child.txt) & sleep 10",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	err := (&ShellRunner{}).Run(ctx, task, &bufferSink{})
	if err == nil {
		t.Fatal("expected shell runner timeout error")
	}

	time.Sleep(2500 * time.Millisecond)
	if _, statErr := os.Stat(sentinel); !os.IsNotExist(statErr) {
		t.Fatalf("expected background child to be terminated, stat err=%v", statErr)
	}
}
