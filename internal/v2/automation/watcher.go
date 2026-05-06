package automation

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
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

type validationError struct {
	message string
}

func (e validationError) Error() string {
	return e.message
}

func newValidationError(format string, args ...any) error {
	return validationError{message: fmt.Sprintf(format, args...)}
}

func IsValidationError(err error) bool {
	var target validationError
	return errors.As(err, &target)
}

func runWatcher(scriptPath string, workspaceRoot string) (*WatcherResult, error) {
	var err error
	scriptPath, err = resolveWatcherPath(scriptPath, workspaceRoot, true)
	if err != nil {
		return nil, err
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

func resolveWatcherPath(scriptPath string, workspaceRoot string, requireExisting bool) (string, error) {
	scriptPath = strings.TrimSpace(scriptPath)
	if scriptPath == "" {
		return "", newValidationError("watcher path is required")
	}
	root, err := watcherWorkspaceRoot(workspaceRoot)
	if err != nil {
		return "", err
	}
	if !filepath.IsAbs(scriptPath) {
		scriptPath = filepath.Join(root, scriptPath)
	}
	scriptPath, err = filepath.Abs(scriptPath)
	if err != nil {
		return "", err
	}
	if err := ensureWatcherPathWithinWorkspace(root, scriptPath); err != nil {
		return "", err
	}

	if _, err := os.Lstat(scriptPath); err != nil {
		if !requireExisting && os.IsNotExist(err) {
			return scriptPath, nil
		}
		return "", err
	}
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", err
	}
	realPath, err := filepath.EvalSymlinks(scriptPath)
	if err != nil {
		return "", err
	}
	if err := ensureWatcherPathWithinWorkspace(realRoot, realPath); err != nil {
		return "", err
	}
	return scriptPath, nil
}

func resolveRoleOwnedWatcherPath(scriptPath string, workspaceRoot string, roleID string, requireExisting bool) (string, error) {
	resolved, err := resolveWatcherPath(scriptPath, workspaceRoot, requireExisting)
	if err != nil {
		return "", err
	}
	roleRoot, err := roleAutomationRoot(workspaceRoot, roleID)
	if err != nil {
		return "", err
	}
	if err := ensureRoleOwnedWatcherPath(roleRoot, resolved, roleID); err != nil {
		return "", err
	}

	if _, err := os.Lstat(resolved); err != nil {
		if !requireExisting && os.IsNotExist(err) {
			return resolved, nil
		}
		return "", err
	}
	realRoleRoot, err := filepath.EvalSymlinks(roleRoot)
	if err != nil {
		return "", err
	}
	realPath, err := filepath.EvalSymlinks(resolved)
	if err != nil {
		return "", err
	}
	if err := ensureRoleOwnedWatcherPath(realRoleRoot, realPath, roleID); err != nil {
		return "", err
	}
	return resolved, nil
}

func watcherWorkspaceRoot(workspaceRoot string) (string, error) {
	workspaceRoot = strings.TrimSpace(workspaceRoot)
	if workspaceRoot == "" {
		return os.Getwd()
	}
	return filepath.Abs(workspaceRoot)
}

func roleAutomationRoot(workspaceRoot string, roleID string) (string, error) {
	roleID = strings.TrimSpace(roleID)
	if roleID == "" || roleID == "." || roleID == ".." || filepath.Clean(roleID) != roleID || strings.ContainsAny(roleID, `/\`) {
		return "", newValidationError("automation owner role id is invalid")
	}
	root, err := watcherWorkspaceRoot(workspaceRoot)
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "roles", roleID, "automations"), nil
}

func ensureWatcherPathWithinWorkspace(root, candidate string) error {
	rel, err := filepath.Rel(root, candidate)
	if err != nil {
		return err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return newValidationError("watcher path escapes workspace root")
	}
	return nil
}

func ensureRoleOwnedWatcherPath(root, candidate string, roleID string) error {
	if err := ensureWatcherPathWithinWorkspace(root, candidate); err != nil {
		return newValidationError("watcher path must be under roles/%s/automations", roleID)
	}
	return nil
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
