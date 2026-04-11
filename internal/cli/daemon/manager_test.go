package daemon

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestEnsureRunningReusesHealthyDaemonWithMatchingConfig(t *testing.T) {
	var launches atomic.Int32

	mgr := NewManager(Options{
		BaseURL: "http://127.0.0.1:4317",
		Status: func(context.Context, string) (StatusInfo, error) {
			return StatusInfo{ConfigFingerprint: "match", PID: 123}, nil
		},
		Launcher: func(context.Context, LaunchConfig) error {
			launches.Add(1)
			return nil
		},
		Config: LaunchConfig{ConfigFingerprint: "match"},
	})

	if err := mgr.EnsureRunning(context.Background()); err != nil {
		t.Fatalf("EnsureRunning() error = %v", err)
	}
	if launches.Load() != 0 {
		t.Fatalf("launches = %d, want 0", launches.Load())
	}
}

func TestEnsureRunningLaunchesWhenStatusUnavailable(t *testing.T) {
	var launches atomic.Int32

	mgr := NewManager(Options{
		BaseURL: "http://127.0.0.1:4317",
		Status: func(context.Context, string) (StatusInfo, error) {
			if launches.Load() == 0 {
				return StatusInfo{}, errors.New("connection refused")
			}
			return StatusInfo{ConfigFingerprint: "match", PID: 123}, nil
		},
		Launcher: func(context.Context, LaunchConfig) error {
			launches.Add(1)
			return nil
		},
		Config:       LaunchConfig{ConfigFingerprint: "match"},
		PollInterval: time.Millisecond,
		ReadyTimeout: 20 * time.Millisecond,
	})

	if err := mgr.EnsureRunning(context.Background()); err != nil {
		t.Fatalf("EnsureRunning() error = %v", err)
	}
	if launches.Load() != 1 {
		t.Fatalf("launches = %d, want 1", launches.Load())
	}
}

func TestStopStopsMatchingDaemon(t *testing.T) {
	var stops atomic.Int32

	mgr := NewManager(Options{
		BaseURL: "http://127.0.0.1:4317",
		Status: func(context.Context, string) (StatusInfo, error) {
			if stops.Load() > 0 {
				return StatusInfo{}, errors.New("connection refused")
			}
			return StatusInfo{
				DaemonID:          "daemon_demo123",
				ConfigFingerprint: "match",
				PID:               123,
			}, nil
		},
		Stopper: func(pid int) error {
			if pid != 123 {
				t.Fatalf("pid = %d, want 123", pid)
			}
			stops.Add(1)
			return nil
		},
		Config:       LaunchConfig{DaemonID: "daemon_demo123", ConfigFingerprint: "match"},
		PollInterval: time.Millisecond,
		ReadyTimeout: 20 * time.Millisecond,
	})

	if err := mgr.Stop(context.Background()); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	if stops.Load() != 1 {
		t.Fatalf("stops = %d, want 1", stops.Load())
	}
}

func TestStopIgnoresDifferentDaemon(t *testing.T) {
	var stops atomic.Int32

	mgr := NewManager(Options{
		BaseURL: "http://127.0.0.1:4317",
		Status: func(context.Context, string) (StatusInfo, error) {
			return StatusInfo{
				DaemonID:          "daemon_other",
				ConfigFingerprint: "other",
				PID:               456,
			}, nil
		},
		Stopper: func(int) error {
			stops.Add(1)
			return nil
		},
		Config: LaunchConfig{DaemonID: "daemon_demo123", ConfigFingerprint: "match"},
	})

	if err := mgr.Stop(context.Background()); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	if stops.Load() != 0 {
		t.Fatalf("stops = %d, want 0", stops.Load())
	}
}

func TestResolveSesameLaunchTargetFallsBackToGoRunInRepo(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module go-agent\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(go.mod) error = %v", err)
	}
	sesameMain := filepath.Join(root, "cmd", "sesame", "main.go")
	if err := os.MkdirAll(filepath.Dir(sesameMain), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(sesameMain, []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(main.go) error = %v", err)
	}

	subdir := filepath.Join(root, "internal", "cli")
	if err := os.MkdirAll(subdir, 0o755); err != nil {
		t.Fatalf("MkdirAll(subdir) error = %v", err)
	}

	target, err := resolveSesameLaunchTarget(subdir, func(string) (string, error) {
		return "", errors.New("not found")
	})
	if err != nil {
		t.Fatalf("resolveSesameLaunchTarget() error = %v", err)
	}
	if target.executable != "go" {
		t.Fatalf("executable = %q, want %q", target.executable, "go")
	}
	if len(target.args) != 3 || target.args[0] != "run" || target.args[1] != "./cmd/sesame" || target.args[2] != "daemon" {
		t.Fatalf("args = %#v, want []string{\"run\", \"./cmd/sesame\", \"daemon\"}", target.args)
	}
	if target.workdir != root {
		t.Fatalf("workdir = %q, want %q", target.workdir, root)
	}
}
