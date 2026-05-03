package automation

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"go-agent/internal/model"
	"go-agent/internal/v2/agent"
	"go-agent/internal/v2/contracts"
	"go-agent/internal/v2/reports"
	"go-agent/internal/v2/roles"
	v2session "go-agent/internal/v2/session"
	"go-agent/internal/v2/store"
	"go-agent/internal/v2/tasks"
	"go-agent/internal/v2/tools"
)

func TestAutomationRoleReportTransaction(t *testing.T) {
	ctx := context.Background()
	s, err := store.OpenInMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	workspaceRoot := t.TempDir()
	mainSession := createTransactionSession(t, s, "main_session", workspaceRoot)

	roleService := roles.NewService(workspaceRoot)
	roleSpec, err := roleService.Create(ctx, roles.SaveInput{
		ID:                "reddit_monitor",
		Name:              "Reddit Monitor",
		Description:       "Reviews watcher signals and reports important posts.",
		SystemPrompt:      "Inspect watcher signals and return a concise final report.",
		PermissionProfile: "default",
		Model:             "role-model",
		MaxToolCalls:      4,
		MaxRuntime:        30,
		AutomationOwners:  []string{"reddit_monitor"},
	})
	if err != nil {
		t.Fatalf("create role: %v", err)
	}

	watcherPath := filepath.Join(workspaceRoot, "roles", "reddit_monitor", "automations", "hot_post", "watcher.sh")
	writeTransactionWatcher(t, watcherPath)

	modelClient := newTransactionModelClient([][]model.StreamEvent{
		textStream("Owner role inspected Reddit signal and prepared email draft."),
		textStream("I received the role report and informed the user."),
	})
	registry := tools.NewRegistry()
	ag := agent.New(modelClient, registry, s)
	ag.SetSystemPrompt("Main agent prompt.")
	ag.SetProjectStateAutoUpdate(false)

	sessionMgr := v2session.NewManager(ag)
	sessionMgr.Register(mainSession)

	taskManager := tasks.NewManager(s, t.TempDir())
	taskManager.RegisterRunner("agent", tasks.NewAgentRunner(s, sessionMgr, roleService))
	reportService := reports.NewService(s, sessionMgr)
	taskManager.SetReporter(reportService)

	automationService := NewService(s, taskManager, roleService)
	automation := contracts.Automation{
		ID:            "reddit_hot_post",
		WorkspaceRoot: workspaceRoot,
		Title:         "Watch hot Reddit posts",
		Goal:          "Review high-signal Reddit AI posts and notify the user.",
		Owner:         "role:reddit_monitor",
		WatcherPath:   watcherPath,
		State:         "active",
	}
	if err := automationService.Create(ctx, automation); err != nil {
		t.Fatalf("create automation: %v", err)
	}

	if err := automationService.Reconcile(ctx); err != nil {
		t.Fatalf("reconcile automation: %v", err)
	}

	run, err := s.Automations().GetRunByDedupeKey(ctx, automation.ID, "reddit-hot-post-1")
	if err != nil {
		t.Fatalf("load automation run: %v", err)
	}
	if run.TaskID == "" || run.Status != "needs_agent" {
		t.Fatalf("unexpected automation run: %+v", run)
	}

	task, err := taskManager.Wait(ctxWithTimeout(t, 5*time.Second), run.TaskID)
	if err != nil {
		t.Fatalf("wait task: %v", err)
	}
	if task.State != "completed" || task.Outcome != "success" {
		t.Fatalf("unexpected task terminal state: %+v", task)
	}
	if task.RoleID != roleSpec.ID {
		t.Fatalf("task role_id = %q, want %q", task.RoleID, roleSpec.ID)
	}
	if task.ReportSessionID != mainSession.ID || task.ParentSessionID != mainSession.ID {
		t.Fatalf("task should report to main session: %+v", task)
	}
	if task.SessionID != v2session.SpecialistSessionID(roleSpec.ID, workspaceRoot) {
		t.Fatalf("task session_id = %q, want specialist session", task.SessionID)
	}
	if !strings.Contains(task.FinalText, "prepared email draft") {
		t.Fatalf("task final text missing specialist output: %q", task.FinalText)
	}

	reportBatch := waitForReportBatchTurn(t, s, mainSession.ID)
	if reportBatch.State != "completed" {
		t.Fatalf("report batch did not complete: %+v", reportBatch)
	}

	reportsForMain, err := s.Reports().ListBySession(ctx, mainSession.ID)
	if err != nil {
		t.Fatalf("list reports: %v", err)
	}
	if len(reportsForMain) != 1 {
		t.Fatalf("expected one main-session report, got %+v", reportsForMain)
	}
	report := reportsForMain[0]
	if !report.Delivered || report.SourceID != task.ID || !strings.Contains(report.Summary, "prepared email draft") {
		t.Fatalf("unexpected delivered report: %+v", report)
	}

	trace, err := tasks.BuildTrace(ctx, s, task, tasks.TraceOptions{})
	if err != nil {
		t.Fatalf("build trace: %v", err)
	}
	if len(trace.Reports) != 1 || trace.Reports[0].ID != report.ID {
		t.Fatalf("trace should include delivered report: %+v", trace.Reports)
	}

	messages, err := s.Messages().List(ctx, mainSession.ID, contracts.MessageListOptions{})
	if err != nil {
		t.Fatalf("list main messages: %v", err)
	}
	if !containsMessage(messages, "Review these completed task reports") || !containsMessage(messages, "I received the role report") {
		t.Fatalf("main session messages do not show report processing: %+v", messages)
	}
	if modelClient.RequestCount() != 2 {
		t.Fatalf("model request count = %d, want 2", modelClient.RequestCount())
	}
}

func createTransactionSession(t *testing.T, s contracts.Store, id, workspaceRoot string) contracts.Session {
	t.Helper()
	now := time.Now().UTC()
	session := contracts.Session{
		ID:                id,
		WorkspaceRoot:     workspaceRoot,
		SystemPrompt:      "Main system prompt.",
		PermissionProfile: "default",
		State:             "idle",
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	if err := s.Sessions().Create(context.Background(), session); err != nil {
		t.Fatal(err)
	}
	return session
}

func writeTransactionWatcher(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	body := "#!/bin/sh\nprintf '%s\\n' '{\"status\":\"needs_agent\",\"summary\":\"Hot Reddit AI post crossed threshold\",\"dedupe_key\":\"reddit-hot-post-1\",\"signal_kind\":\"reddit_hot_post\"}'\n"
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
}

func textStream(text string) []model.StreamEvent {
	return []model.StreamEvent{
		{Kind: model.StreamEventTextDelta, TextDelta: text},
		{Kind: model.StreamEventMessageEnd},
	}
}

type transactionModelClient struct {
	mu       sync.Mutex
	streams  [][]model.StreamEvent
	index    int
	requests []model.Request
}

func newTransactionModelClient(streams [][]model.StreamEvent) *transactionModelClient {
	return &transactionModelClient{streams: streams}
}

func (c *transactionModelClient) Stream(ctx context.Context, req model.Request) (<-chan model.StreamEvent, <-chan error) {
	c.mu.Lock()
	c.requests = append(c.requests, req)
	var batch []model.StreamEvent
	if c.index < len(c.streams) {
		batch = append([]model.StreamEvent(nil), c.streams[c.index]...)
		c.index++
	}
	c.mu.Unlock()

	events := make(chan model.StreamEvent, len(batch))
	errs := make(chan error, 1)
	go func() {
		defer close(events)
		defer close(errs)
		if len(batch) == 0 {
			errs <- context.Canceled
			return
		}
		for _, event := range batch {
			select {
			case <-ctx.Done():
				errs <- ctx.Err()
				return
			case events <- event:
			}
		}
		errs <- nil
	}()
	return events, errs
}

func (c *transactionModelClient) Capabilities() model.ProviderCapabilities {
	return model.ProviderCapabilities{}
}

func (c *transactionModelClient) RequestCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.requests)
}

func waitForReportBatchTurn(t *testing.T, s contracts.Store, sessionID string) contracts.Turn {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		turns, err := s.Turns().ListBySession(context.Background(), sessionID)
		if err != nil {
			t.Fatal(err)
		}
		for _, turn := range turns {
			if turn.Kind == "report_batch" {
				if turn.State == "completed" {
					return turn
				}
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	turns, _ := s.Turns().ListBySession(context.Background(), sessionID)
	t.Fatalf("timed out waiting for completed report_batch turn: %+v", turns)
	return contracts.Turn{}
}

func containsMessage(messages []contracts.Message, text string) bool {
	for _, msg := range messages {
		if strings.Contains(msg.Content, text) {
			return true
		}
	}
	return false
}

func ctxWithTimeout(t *testing.T, timeout time.Duration) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	t.Cleanup(cancel)
	return ctx
}
