package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestEnsureRunningReusesHealthyDaemon(t *testing.T) {
	statusServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/status" {
			t.Fatalf("path = %q, want %q", r.URL.Path, "/v1/status")
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}))
	defer statusServer.Close()

	mgr := NewManager(Options{
		BaseURL: statusServer.URL,
		Launcher: func(context.Context, LaunchConfig) error {
			t.Fatal("launcher should not run")
			return nil
		},
	})

	if err := mgr.EnsureRunning(context.Background()); err != nil {
		t.Fatalf("EnsureRunning() error = %v", err)
	}
}

func TestEnsureRunningLaunchesDaemonAfterProbeFailure(t *testing.T) {
	var launches atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}))
	defer server.Close()

	mgr := NewManager(Options{
		BaseURL: server.URL,
		Probe: func(context.Context, string) error {
			if launches.Load() == 0 {
				return errors.New("connection refused")
			}
			return nil
		},
		Launcher: func(context.Context, LaunchConfig) error {
			launches.Add(1)
			return nil
		},
		Retries: 1,
	})

	if err := mgr.EnsureRunning(context.Background()); err != nil {
		t.Fatalf("EnsureRunning() error = %v", err)
	}
	if launches.Load() != 1 {
		t.Fatalf("launches = %d, want 1", launches.Load())
	}
}
