package daemon

import (
	"context"
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
	Addr           string
	DataDir        string
	Model          string
	PermissionMode string
}

type Options struct {
	BaseURL    string
	HTTPClient *http.Client
	Probe      func(context.Context, string) error
	Launcher   func(context.Context, LaunchConfig) error
	Config     LaunchConfig
	Retries    int
}

type Manager struct {
	baseURL    string
	probe      func(context.Context, string) error
	launcher   func(context.Context, LaunchConfig) error
	config     LaunchConfig
	retries    int
	httpClient *http.Client
}

func NewManager(opts Options) *Manager {
	client := opts.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second}
	}

	retries := opts.Retries
	if retries <= 0 {
		retries = 3
	}

	manager := &Manager{
		baseURL:    strings.TrimRight(opts.BaseURL, "/"),
		launcher:   opts.Launcher,
		config:     opts.Config,
		retries:    retries,
		httpClient: client,
	}
	if opts.Probe != nil {
		manager.probe = opts.Probe
	} else {
		manager.probe = manager.probeStatus
	}
	if manager.launcher == nil {
		manager.launcher = LaunchAgentD
	}
	return manager
}

func (m *Manager) EnsureRunning(ctx context.Context) error {
	if err := m.probe(ctx, m.baseURL); err == nil {
		return nil
	}

	if err := m.launcher(ctx, m.config); err != nil {
		return err
	}

	var lastErr error
	for attempt := 0; attempt < m.retries; attempt++ {
		if attempt > 0 {
			delay := time.Duration(attempt) * 200 * time.Millisecond
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case <-timer.C:
			}
		}

		if err := m.probe(ctx, m.baseURL); err == nil {
			return nil
		} else {
			lastErr = err
		}
	}

	if lastErr == nil {
		lastErr = fmt.Errorf("daemon did not become ready")
	}
	return fmt.Errorf("ensure daemon running: %w", lastErr)
}

func (m *Manager) probeStatus(ctx context.Context, baseURL string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(baseURL, "/")+"/v1/status", nil)
	if err != nil {
		return err
	}

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code %d", resp.StatusCode)
	}
	io.Copy(io.Discard, resp.Body)
	return nil
}

func LaunchAgentD(ctx context.Context, cfg LaunchConfig) error {
	binary, err := findAgentDBinary()
	if err != nil {
		return err
	}

	cmd := exec.CommandContext(ctx, binary)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	cmd.Env = append(os.Environ(),
		"AGENTD_ADDR="+cfg.Addr,
		"AGENTD_DATA_DIR="+cfg.DataDir,
	)
	if strings.TrimSpace(cfg.Model) != "" {
		cmd.Env = append(cmd.Env, "AGENTD_MODEL="+cfg.Model)
	}
	if strings.TrimSpace(cfg.PermissionMode) != "" {
		cmd.Env = append(cmd.Env, "AGENTD_PERMISSION_PROFILE="+cfg.PermissionMode)
	}

	return cmd.Start()
}

func findAgentDBinary() (string, error) {
	name := "agentd"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}

	if exePath, err := os.Executable(); err == nil {
		sibling := filepath.Join(filepath.Dir(exePath), name)
		if _, statErr := os.Stat(sibling); statErr == nil {
			return sibling, nil
		}
	}

	path, err := exec.LookPath(name)
	if err != nil {
		return "", fmt.Errorf("find agentd binary: %w", err)
	}
	return path, nil
}
