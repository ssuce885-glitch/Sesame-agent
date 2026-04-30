package httpapi_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	httpapi "go-agent/internal/api/http"
	"go-agent/internal/session"
	"go-agent/internal/store/sqlite"
	"go-agent/internal/stream"
	"go-agent/internal/types"
)

type mockCronStore struct {
	jobs map[string]types.ScheduledJob
}

func newMockCronStore() *mockCronStore {
	return &mockCronStore{jobs: make(map[string]types.ScheduledJob)}
}

func (m *mockCronStore) ListJobs(_ context.Context, workspaceRoot string) ([]types.ScheduledJob, error) {
	var out []types.ScheduledJob
	for _, j := range m.jobs {
		if workspaceRoot == "" || j.WorkspaceRoot == workspaceRoot {
			out = append(out, j)
		}
	}
	if out == nil {
		out = []types.ScheduledJob{}
	}
	return out, nil
}

func (m *mockCronStore) GetJob(_ context.Context, id string) (types.ScheduledJob, bool, error) {
	j, ok := m.jobs[id]
	return j, ok, nil
}

func (m *mockCronStore) SetJobEnabled(_ context.Context, id string, enabled bool) (types.ScheduledJob, bool, error) {
	j, ok := m.jobs[id]
	if !ok {
		return types.ScheduledJob{}, false, nil
	}
	j.Enabled = enabled
	m.jobs[id] = j
	return j, true, nil
}

func (m *mockCronStore) DeleteJob(_ context.Context, id string) (bool, error) {
	_, ok := m.jobs[id]
	delete(m.jobs, id)
	return ok, nil
}

func setupHTTPTest(t *testing.T) (*httptest.Server, *sqlite.Store, *stream.Bus, *mockCronStore, string) {
	t.Helper()

	store, err := sqlite.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("sqlite.Open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	bus := stream.NewBus()
	cronStore := newMockCronStore()
	workspaceRoot := filepath.ToSlash(t.TempDir())
	consoleRoot := t.TempDir()

	deps := httpapi.Dependencies{
		Store:         store,
		Manager:       session.NewManager(noopRunner{}),
		Bus:           bus,
		Scheduler:     cronStore,
		Status:        httpapi.StatusPayload{Provider: "test", Model: "test-model"},
		WorkspaceRoot: workspaceRoot,
		ConsoleRoot:   consoleRoot,
	}

	srv := httptest.NewServer(httpapi.NewRouter(deps))
	t.Cleanup(func() { srv.Close() })

	return srv, store, bus, cronStore, workspaceRoot
}

type noopRunner struct{}

func (noopRunner) RunTurn(_ context.Context, _ session.RunInput) error { return nil }

func readSSEEvents(ctx context.Context, t *testing.T, body io.Reader, events chan<- string) {
	t.Helper()
	defer close(events)

	scanner := bufio.NewScanner(body)
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return
		default:
		}
		line := scanner.Text()
		if strings.HasPrefix(line, "data: ") {
			events <- strings.TrimPrefix(line, "data: ")
		}
	}
}

func TestIntegrationStatusEndpoint(t *testing.T) {
	srv, _, _, _, _ := setupHTTPTest(t)

	resp, err := srv.Client().Get(srv.URL + "/v1/status")
	if err != nil {
		t.Fatalf("GET /v1/status: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var body httpapi.StatusPayload
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Status != "ok" {
		t.Fatalf("status = %q, want %q", body.Status, "ok")
	}
	if body.Provider != "test" {
		t.Fatalf("provider = %q, want %q", body.Provider, "test")
	}
	if body.Model != "test-model" {
		t.Fatalf("model = %q, want %q", body.Model, "test-model")
	}
}

func TestIntegrationSessionEnsure(t *testing.T) {
	srv, _, _, _, _ := setupHTTPTest(t)

	body := bytes.NewBufferString(`{"workspace_root":"/tmp/test-workspace"}`)
	resp, err := srv.Client().Post(srv.URL+"/v1/session/ensure", "application/json", body)
	if err != nil {
		t.Fatalf("POST /v1/session/ensure: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var session types.Session
	if err := json.NewDecoder(resp.Body).Decode(&session); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if session.ID == "" {
		t.Fatal("session.ID is empty")
	}
	if session.WorkspaceRoot != "/tmp/test-workspace" {
		t.Fatalf("workspace_root = %q, want %q", session.WorkspaceRoot, "/tmp/test-workspace")
	}
}

func TestIntegrationSessionEnsureBadRequest(t *testing.T) {
	srv, _, _, _, _ := setupHTTPTest(t)

	body := bytes.NewBufferString(`{}`)
	resp, err := srv.Client().Post(srv.URL+"/v1/session/ensure", "application/json", body)
	if err != nil {
		t.Fatalf("POST /v1/session/ensure: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}
}

func TestIntegrationTurnSubmit(t *testing.T) {
	srv, _, _, _, _ := setupHTTPTest(t)

	ensureBody := bytes.NewBufferString(`{"workspace_root":"/tmp/test-workspace"}`)
	ensureResp, err := srv.Client().Post(srv.URL+"/v1/session/ensure", "application/json", ensureBody)
	if err != nil {
		t.Fatalf("POST /v1/session/ensure: %v", err)
	}
	ensureResp.Body.Close()

	turnBody := bytes.NewBufferString(`{"message":"hello world"}`)
	turnResp, err := srv.Client().Post(srv.URL+"/v1/session/turns", "application/json", turnBody)
	if err != nil {
		t.Fatalf("POST /v1/session/turns: %v", err)
	}
	defer turnResp.Body.Close()

	if turnResp.StatusCode != http.StatusAccepted {
		t.Fatalf("status = %d, want %d", turnResp.StatusCode, http.StatusAccepted)
	}

	var turn types.Turn
	if err := json.NewDecoder(turnResp.Body).Decode(&turn); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if turn.ID == "" {
		t.Fatal("turn.ID is empty")
	}
	if turn.UserMessage != "hello world" {
		t.Fatalf("user_message = %q, want %q", turn.UserMessage, "hello world")
	}
	if turn.Kind != types.TurnKindUserMessage {
		t.Fatalf("kind = %q, want %q", turn.Kind, types.TurnKindUserMessage)
	}
}

func TestIntegrationTurnSubmitEmptyMessage(t *testing.T) {
	srv, _, _, _, _ := setupHTTPTest(t)

	ensureBody := bytes.NewBufferString(`{"workspace_root":"/tmp/test-workspace"}`)
	ensureResp, err := srv.Client().Post(srv.URL+"/v1/session/ensure", "application/json", ensureBody)
	if err != nil {
		t.Fatalf("POST /v1/session/ensure: %v", err)
	}
	ensureResp.Body.Close()

	turnBody := bytes.NewBufferString(`{"message":""}`)
	turnResp, err := srv.Client().Post(srv.URL+"/v1/session/turns", "application/json", turnBody)
	if err != nil {
		t.Fatalf("POST /v1/session/turns: %v", err)
	}
	defer turnResp.Body.Close()

	if turnResp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", turnResp.StatusCode, http.StatusBadRequest)
	}
}

func TestIntegrationSSEEvents(t *testing.T) {
	srv, store, bus, _, _ := setupHTTPTest(t)

	reqCtx, cancelReq := context.WithCancel(context.Background())
	defer cancelReq()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, srv.URL+"/v1/session/events?after=0", nil)
	if err != nil {
		t.Fatalf("NewRequestWithContext: %v", err)
	}

	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("GET /v1/session/events: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if contentType := resp.Header.Get("Content-Type"); !strings.HasPrefix(contentType, "text/event-stream") {
		t.Fatalf("Content-Type = %q, want prefix %q", contentType, "text/event-stream")
	}

	sessions, err := store.ListSessions(context.Background())
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("len(sessions) = %d, want 1", len(sessions))
	}

	readCtx, cancelRead := context.WithCancel(context.Background())
	defer cancelRead()

	events := make(chan string, 16)
	go readSSEEvents(readCtx, t, resp.Body, events)

	event, err := types.NewEvent(
		sessions[0].ID,
		"",
		types.EventAssistantDelta,
		types.AssistantDeltaPayload{Text: "hello"},
	)
	if err != nil {
		t.Fatalf("NewEvent: %v", err)
	}

	persisted, err := store.AppendEventWithState(context.Background(), event)
	if err != nil {
		t.Fatalf("AppendEventWithState: %v", err)
	}

	time.Sleep(100 * time.Millisecond)
	bus.Publish(persisted)

	timeout := time.NewTimer(5 * time.Second)
	defer timeout.Stop()

	for {
		select {
		case data, ok := <-events:
			if !ok {
				t.Fatal("event stream closed before event was received")
			}

			var streamed types.Event
			if err := json.Unmarshal([]byte(data), &streamed); err != nil {
				t.Fatalf("decode SSE event: %v", err)
			}
			if streamed.Type != types.EventAssistantDelta {
				continue
			}
			if streamed.Seq != persisted.Seq {
				t.Fatalf("seq = %d, want %d", streamed.Seq, persisted.Seq)
			}

			var payload types.AssistantDeltaPayload
			if err := json.Unmarshal(streamed.Payload, &payload); err != nil {
				t.Fatalf("decode payload: %v", err)
			}
			if payload.Text != "hello" {
				t.Fatalf("payload.text = %q, want %q", payload.Text, "hello")
			}
			return
		case <-timeout.C:
			t.Fatal("timed out waiting for SSE event")
		}
	}
}

func TestIntegrationCronList(t *testing.T) {
	srv, _, _, cronStore, workspaceRoot := setupHTTPTest(t)

	cronStore.jobs["job-1"] = types.ScheduledJob{
		ID:            "job-1",
		Name:          "test-job",
		WorkspaceRoot: workspaceRoot,
		Kind:          types.ScheduleKindEvery,
		EveryMinutes:  15,
		Enabled:       true,
	}

	resp, err := srv.Client().Get(srv.URL + "/v1/cron")
	if err != nil {
		t.Fatalf("GET /v1/cron: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var body types.ListScheduledJobsResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Jobs) != 1 {
		t.Fatalf("len(jobs) = %d, want 1", len(body.Jobs))
	}
	if body.Jobs[0].Name != "test-job" {
		t.Fatalf("name = %q, want %q", body.Jobs[0].Name, "test-job")
	}
}

func TestIntegrationCronGetAndDelete(t *testing.T) {
	srv, _, _, cronStore, workspaceRoot := setupHTTPTest(t)

	cronStore.jobs["job-1"] = types.ScheduledJob{
		ID:            "job-1",
		Name:          "test-job",
		WorkspaceRoot: workspaceRoot,
		Enabled:       true,
	}

	resp, err := srv.Client().Get(srv.URL + "/v1/cron/job-1")
	if err != nil {
		t.Fatalf("GET /v1/cron/job-1: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	var job types.ScheduledJob
	if err := json.NewDecoder(resp.Body).Decode(&job); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if job.ID != "job-1" {
		t.Fatalf("job.ID = %q, want %q", job.ID, "job-1")
	}
	if job.Name != "test-job" {
		t.Fatalf("job.Name = %q, want %q", job.Name, "test-job")
	}
	resp.Body.Close()

	resp, err = srv.Client().Get(srv.URL + "/v1/cron/nonexistent")
	if err != nil {
		t.Fatalf("GET /v1/cron/nonexistent: %v", err)
	}
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("GET nonexistent status = %d, want %d", resp.StatusCode, http.StatusNotFound)
	}
	resp.Body.Close()

	req, err := http.NewRequest(http.MethodDelete, srv.URL+"/v1/cron/job-1", nil)
	if err != nil {
		t.Fatalf("NewRequest DELETE: %v", err)
	}
	resp, err = srv.Client().Do(req)
	if err != nil {
		t.Fatalf("DELETE /v1/cron/job-1: %v", err)
	}
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("DELETE status = %d, want %d", resp.StatusCode, http.StatusNoContent)
	}
	resp.Body.Close()

	if _, ok := cronStore.jobs["job-1"]; ok {
		t.Fatal("job still exists after delete")
	}
}

func TestIntegrationCronPauseResume(t *testing.T) {
	srv, _, _, cronStore, workspaceRoot := setupHTTPTest(t)

	cronStore.jobs["job-1"] = types.ScheduledJob{
		ID:            "job-1",
		Name:          "test-job",
		WorkspaceRoot: workspaceRoot,
		Enabled:       true,
	}

	resp, err := srv.Client().Post(srv.URL+"/v1/cron/job-1/pause", "application/json", nil)
	if err != nil {
		t.Fatalf("POST /v1/cron/job-1/pause: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("pause status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	resp.Body.Close()

	if cronStore.jobs["job-1"].Enabled {
		t.Fatal("job still enabled after pause")
	}

	resp, err = srv.Client().Post(srv.URL+"/v1/cron/job-1/resume", "application/json", nil)
	if err != nil {
		t.Fatalf("POST /v1/cron/job-1/resume: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("resume status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	resp.Body.Close()

	if !cronStore.jobs["job-1"].Enabled {
		t.Fatal("job not enabled after resume")
	}
}

func TestIntegrationCronNotFound(t *testing.T) {
	srv, _, _, _, _ := setupHTTPTest(t)

	resp, err := srv.Client().Get(srv.URL + "/v1/cron/no-such-job")
	if err != nil {
		t.Fatalf("GET /v1/cron/no-such-job: %v", err)
	}
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("GET status = %d, want %d", resp.StatusCode, http.StatusNotFound)
	}
	resp.Body.Close()

	resp, err = srv.Client().Post(srv.URL+"/v1/cron/no-such-job/pause", "application/json", nil)
	if err != nil {
		t.Fatalf("POST /v1/cron/no-such-job/pause: %v", err)
	}
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("pause status = %d, want %d", resp.StatusCode, http.StatusNotFound)
	}
	resp.Body.Close()

	req, err := http.NewRequest(http.MethodDelete, srv.URL+"/v1/cron/no-such-job", nil)
	if err != nil {
		t.Fatalf("NewRequest DELETE: %v", err)
	}
	resp, err = srv.Client().Do(req)
	if err != nil {
		t.Fatalf("DELETE /v1/cron/no-such-job: %v", err)
	}
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("DELETE status = %d, want %d", resp.StatusCode, http.StatusNotFound)
	}
	resp.Body.Close()
}
