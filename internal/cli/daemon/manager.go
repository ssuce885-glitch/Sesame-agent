package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

type LaunchConfig struct {
	DaemonID          string
	Addr              string
	DataDir           string
	Model             string
	PermissionMode    string
	ConfigFingerprint string
}

type StatusInfo struct {
	Status            string `json:"status,omitempty"`
	DaemonID          string `json:"daemon_id,omitempty"`
	Model             string `json:"model,omitempty"`
	PermissionProfile string `json:"permission_profile,omitempty"`
	ConfigFingerprint string `json:"config_fingerprint,omitempty"`
	PID               int    `json:"pid,omitempty"`
}

type Options struct {
	BaseURL      string
	HTTPClient   *http.Client
	Status       func(context.Context, string) (StatusInfo, error)
	Launcher     func(context.Context, LaunchConfig) error
	Config       LaunchConfig
	PollInterval time.Duration
	ReadyTimeout time.Duration
}

type Manager struct {
	baseURL      string
	status       func(context.Context, string) (StatusInfo, error)
	launcher     func(context.Context, LaunchConfig) error
	config       LaunchConfig
	pollInterval time.Duration
	readyTimeout time.Duration
	httpClient   *http.Client
}

type launchTarget struct {
	executable string
	args       []string
	workdir    string
}

func NewManager(opts Options) *Manager {
	client := opts.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second}
	}

	pollInterval := opts.PollInterval
	if pollInterval <= 0 {
		pollInterval = 200 * time.Millisecond
	}
	readyTimeout := opts.ReadyTimeout
	if readyTimeout <= 0 {
		readyTimeout = 5 * time.Second
	}

	manager := &Manager{
		baseURL:      strings.TrimRight(opts.BaseURL, "/"),
		launcher:     opts.Launcher,
		config:       opts.Config,
		pollInterval: pollInterval,
		readyTimeout: readyTimeout,
		httpClient:   client,
	}
	if opts.Status != nil {
		manager.status = opts.Status
	} else {
		manager.status = manager.fetchStatus
	}
	if manager.launcher == nil {
		manager.launcher = LaunchSesameDaemon
	}
	return manager
}

func (m *Manager) EnsureRunning(ctx context.Context) error {
	status, err := m.status(ctx, m.baseURL)
	if err == nil && m.matchesConfig(status) {
		return nil
	}
	if err == nil && !m.matchesConfig(status) {
		if err := restartPID(status.PID); err != nil {
			return err
		}
		if err := m.waitForUnavailable(ctx); err != nil {
			return err
		}
	}

	if err := m.launcher(ctx, m.config); err != nil {
		return err
	}
	return m.waitForReady(ctx)
}

func (m *Manager) matchesConfig(status StatusInfo) bool {
	wantDaemonID := strings.TrimSpace(m.config.DaemonID)
	gotDaemonID := strings.TrimSpace(status.DaemonID)
	if wantDaemonID != "" && wantDaemonID != gotDaemonID {
		return false
	}

	want := strings.TrimSpace(m.config.ConfigFingerprint)
	got := strings.TrimSpace(status.ConfigFingerprint)
	if want == "" || got == "" {
		return false
	}
	return want == got
}

func (m *Manager) waitForUnavailable(ctx context.Context) error {
	deadline := time.Now().Add(m.readyTimeout)
	for {
		if _, err := m.status(ctx, m.baseURL); err != nil {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out waiting for sesame daemon shutdown")
		}
		timer := time.NewTimer(m.pollInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

func (m *Manager) waitForReady(ctx context.Context) error {
	deadline := time.Now().Add(m.readyTimeout)
	var lastErr error
	for {
		status, err := m.status(ctx, m.baseURL)
		if err == nil && m.matchesConfig(status) {
			return nil
		}
		if err != nil {
			lastErr = err
		} else {
			lastErr = fmt.Errorf("running sesame daemon config fingerprint mismatch")
		}

		if time.Now().After(deadline) {
			break
		}

		timer := time.NewTimer(m.pollInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
	return fmt.Errorf("ensure daemon running: %w", lastErr)
}

func (m *Manager) fetchStatus(ctx context.Context, baseURL string) (StatusInfo, error) {
	return FetchStatus(ctx, baseURL, m.httpClient)
}

func FetchStatus(ctx context.Context, baseURL string, httpClient *http.Client) (StatusInfo, error) {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 5 * time.Second}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(baseURL, "/")+"/v1/status", nil)
	if err != nil {
		return StatusInfo{}, err
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return StatusInfo{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return StatusInfo{}, fmt.Errorf("unexpected status code %d", resp.StatusCode)
	}
	var payload StatusInfo
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return StatusInfo{}, err
	}
	io.Copy(io.Discard, resp.Body)
	return payload, nil
}

func restartPID(pid int) error {
	if pid <= 0 {
		return fmt.Errorf("cannot restart sesame daemon: missing pid")
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return process.Kill()
}

func LaunchSesameDaemon(ctx context.Context, cfg LaunchConfig) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	target, err := resolveSesameLaunchTarget(cwd, exec.LookPath)
	if err != nil {
		return err
	}

	cmd := exec.CommandContext(ctx, target.executable, target.args...)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if target.workdir != "" {
		cmd.Dir = target.workdir
	}
	cmd.Env = append(os.Environ(),
		"SESAME_DAEMON_ID="+cfg.DaemonID,
		"SESAME_ADDR="+cfg.Addr,
		"SESAME_DATA_DIR="+cfg.DataDir,
	)
	if strings.TrimSpace(cfg.Model) != "" {
		cmd.Env = append(cmd.Env, "SESAME_MODEL="+cfg.Model)
	}
	if strings.TrimSpace(cfg.PermissionMode) != "" {
		cmd.Env = append(cmd.Env, "SESAME_PERMISSION_PROFILE="+cfg.PermissionMode)
	}

	return cmd.Start()
}

func resolveSesameLaunchTarget(cwd string, lookPath func(string) (string, error)) (launchTarget, error) {
	name := "sesame"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}

	if exePath, err := os.Executable(); err == nil {
		sibling := filepath.Join(filepath.Dir(exePath), name)
		if _, statErr := os.Stat(sibling); statErr == nil {
			return launchTarget{executable: sibling, args: []string{"daemon"}}, nil
		}
	}

	path, err := lookPath(name)
	if err == nil {
		return launchTarget{executable: path, args: []string{"daemon"}}, nil
	}

	if root, ok := findRepoRootWithSesame(cwd); ok {
		return launchTarget{
			executable: "go",
			args:       []string{"run", "./cmd/sesame", "daemon"},
			workdir:    root,
		}, nil
	}

	return launchTarget{}, fmt.Errorf("find sesame binary: %w", err)
}

func findRepoRootWithSesame(start string) (string, bool) {
	current := start
	for {
		if current == "" {
			return "", false
		}

		goMod := filepath.Join(current, "go.mod")
		sesameMain := filepath.Join(current, "cmd", "sesame", "main.go")
		if fileExists(goMod) && fileExists(sesameMain) {
			return current, true
		}

		parent := filepath.Dir(current)
		if parent == current {
			return "", false
		}
		current = parent
	}
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir()
}
