package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"go-agent/internal/config"
	v2app "go-agent/internal/v2/app"
	v2client "go-agent/internal/v2/client"
	"go-agent/internal/v2/tui"
)

const defaultV2Addr = "127.0.0.1:8421"

func main() {
	workspace := flag.String("workspace", "", "workspace root")
	daemon := flag.Bool("daemon", false, "run daemon")
	dataDir := flag.String("data-dir", "", "sesame data directory")
	addr := flag.String("addr", defaultV2Addr, "daemon listen address")
	flag.Parse()

	ctx := context.Background()
	if *workspace == "" {
		if wd, err := os.Getwd(); err == nil {
			*workspace = wd
		}
	}

	cfg, err := config.ResolveCLIStartupConfig(config.CLIStartupOverrides{
		WorkspaceRoot: *workspace,
		DataDir:       *dataDir,
		Addr:          *addr,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	if *daemon {
		if err := v2app.Run(ctx, cfg); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}

	baseURL := fmt.Sprintf("http://%s", connectAddr(cfg.Addr))
	if err := ensureDaemon(ctx, baseURL, cfg); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	client := v2client.New(baseURL)
	if err := tui.Run(ctx, client, *workspace); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func connectAddr(listenAddr string) string {
	host, port, err := net.SplitHostPort(listenAddr)
	if err != nil {
		return listenAddr
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	return net.JoinHostPort(host, port)
}

func ensureDaemon(ctx context.Context, baseURL string, cfg config.Config) error {
	if daemonReady(ctx, baseURL) {
		return nil
	}
	for i := 0; i < 3; i++ {
		time.Sleep(300 * time.Millisecond)
		if daemonReady(ctx, baseURL) {
			return nil
		}
	}

	exe, err := os.Executable()
	if err != nil {
		return err
	}
	args := []string{"-daemon", "-workspace", cfg.Paths.WorkspaceRoot, "-addr", cfg.Addr}
	if cfg.DataDir != "" {
		args = append(args, "-data-dir", cfg.DataDir)
	}
	cmd := exec.Command(exe, args...)
	cmd.Stdout = nil
	cmd.Stderr = nil
	var logFile *os.File
	if cfg.DataDir != "" {
		_ = os.MkdirAll(cfg.DataDir, 0o755)
		if f, openErr := os.OpenFile(filepath.Join(cfg.DataDir, "sesame-v2.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644); openErr == nil {
			logFile = f
			cmd.Stdout = logFile
			cmd.Stderr = logFile
		}
	}
	if err := cmd.Start(); err != nil {
		if logFile != nil {
			_ = logFile.Close()
		}
		return err
	}
	if logFile != nil {
		_ = logFile.Close()
	}
	if err := cmd.Process.Release(); err != nil {
		return err
	}
	return waitForDaemon(ctx, baseURL, 5*time.Second)
}

func waitForDaemon(ctx context.Context, baseURL string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		if daemonReady(ctx, baseURL) {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out waiting for v2 daemon at %s", baseURL)
		}
		timer := time.NewTimer(200 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

func daemonReady(ctx context.Context, baseURL string) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/v2/status", nil)
	if err != nil {
		return false
	}
	client := &http.Client{Timeout: 500 * time.Millisecond}
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}
