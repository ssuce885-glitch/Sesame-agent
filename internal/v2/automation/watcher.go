package automation

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"path"
	"path/filepath"
	"strings"
	"time"

	runtimex "go-agent/internal/runtime"
)

const (
	defaultWatcherTimeout = 30 * time.Second
	maxWatcherOutputBytes = 64 * 1024
)

// WatcherResult is the JSON output from a watcher script.
type WatcherResult struct {
	Status     string `json:"status"` // "ok", "needs_agent", "needs_human", "error"
	Summary    string `json:"summary"`
	DedupeKey  string `json:"dedupe_key"`
	SignalKind string `json:"signal_kind"`
}

func runWatcher(scriptPath string, workspaceRoot string) (*WatcherResult, error) {
	scriptPath = strings.TrimSpace(scriptPath)
	if scriptPath == "" {
		return nil, fmt.Errorf("watcher path is required")
	}
	if !filepath.IsAbs(scriptPath) {
		scriptPath = filepath.Join(workspaceRoot, scriptPath)
	}

	runCtx, cancel := context.WithTimeout(context.Background(), defaultWatcherTimeout)
	defer cancel()
	name := scriptPath
	args := []string{}
	if strings.EqualFold(path.Ext(scriptPath), ".py") {
		name = "python3"
		args = []string{scriptPath}
	}
	cmd := runtimex.NewCommandContext(runCtx, name, args...)
	cmd.Dir = workspaceRoot
	cmd.Env = append(cmd.Environ(), "WORKSPACE_ROOT="+workspaceRoot)
	var stdout, stderr limitedBuffer
	stdout.limit = maxWatcherOutputBytes
	stderr.limit = maxWatcherOutputBytes
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if runCtx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("run watcher %s: timed out after %s", scriptPath, defaultWatcherTimeout)
		}
		return nil, fmt.Errorf("run watcher %s: %w: %s", scriptPath, err, strings.TrimSpace(stderr.String()))
	}
	if stdout.truncated {
		return nil, fmt.Errorf("watcher output exceeded %d bytes", maxWatcherOutputBytes)
	}

	var result WatcherResult
	if err := json.Unmarshal(bytes.TrimSpace(stdout.Bytes()), &result); err != nil {
		return nil, fmt.Errorf("parse watcher output: %w", err)
	}
	result.Status = strings.TrimSpace(result.Status)
	if result.Status == "" {
		result.Status = "ok"
	}
	if !validWatcherStatus(result.Status) {
		return nil, fmt.Errorf("invalid watcher status %q", result.Status)
	}
	return &result, nil
}

type limitedBuffer struct {
	bytes.Buffer
	limit     int
	truncated bool
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if b.limit <= 0 {
		_, _ = b.Buffer.Write(p)
		return len(p), nil
	}
	remaining := b.limit - b.Buffer.Len()
	if remaining <= 0 {
		b.truncated = true
		return len(p), nil
	}
	if len(p) > remaining {
		_, _ = b.Buffer.Write(p[:remaining])
		b.truncated = true
		return len(p), nil
	}
	_, _ = b.Buffer.Write(p)
	return len(p), nil
}

func validWatcherStatus(status string) bool {
	switch status {
	case "ok", "needs_agent", "needs_human", "error":
		return true
	default:
		return false
	}
}

func watcherRunKey(result *WatcherResult) string {
	if result == nil {
		return fmt.Sprintf("run-%d", time.Now().UTC().UnixNano())
	}
	for _, value := range []string{result.DedupeKey, result.SignalKind, result.Summary} {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return "default"
}
