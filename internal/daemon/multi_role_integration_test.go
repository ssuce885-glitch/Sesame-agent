package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"go-agent/internal/config"
	"go-agent/internal/engine"
	"go-agent/internal/model"
	"go-agent/internal/reporting"
	"go-agent/internal/session"
	"go-agent/internal/skills"
	"go-agent/internal/store/sqlite"
	"go-agent/internal/stream"
	"go-agent/internal/types"
)

type multiRoleDef struct {
	ID          string
	DisplayName string
	Prompt      string
}

type multiRoleSink struct {
	mu     sync.Mutex
	events []types.Event
}

func (s *multiRoleSink) Emit(_ context.Context, e types.Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, e)
	return nil
}

func (s *multiRoleSink) Events() []types.Event {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]types.Event(nil), s.events...)
}

func setupMultiRoleRuntime(t *testing.T, roles []multiRoleDef) (*Runtime, *sqlite.Store, string) {
	t.Helper()
	workspaceRoot := t.TempDir()
	globalRoot := t.TempDir()
	dataDir := t.TempDir()

	rolesDir := filepath.Join(workspaceRoot, "roles")
	for _, r := range roles {
		dir := filepath.Join(rolesDir, r.ID)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("MkdirAll(%s): %v", dir, err)
		}
		yamlContent := fmt.Sprintf("display_name: %q\ndescription: %q\n", r.DisplayName, r.DisplayName)
		if err := os.WriteFile(filepath.Join(dir, "role.yaml"), []byte(yamlContent), 0o644); err != nil {
			t.Fatalf("WriteFile(role.yaml): %v", err)
		}
		if err := os.WriteFile(filepath.Join(dir, "prompt.md"), []byte(r.Prompt+"\n"), 0o644); err != nil {
			t.Fatalf("WriteFile(prompt.md): %v", err)
		}
	}

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	if cfg.ModelProvider == "fake" {
		t.Fatalf("config.Load: model provider %q is not a real model", cfg.ModelProvider)
	}
	cfg.Paths.WorkspaceRoot = workspaceRoot
	cfg.Paths.GlobalRoot = globalRoot
	cfg.DataDir = dataDir
	if _, err := skills.LoadCatalog(cfg.Paths.GlobalRoot, cfg.Paths.WorkspaceRoot); err != nil {
		t.Fatalf("skills.LoadCatalog: %v", err)
	}

	modelClient, err := model.NewFromConfig(cfg)
	if err != nil {
		t.Fatalf("model.NewFromConfig: %v", err)
	}

	store, err := sqlite.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("sqlite.Open: %v", err)
	}
	t.Cleanup(func() { store.Close() })

	runtime := buildRuntime(context.Background(), cfg, store, modelClient)
	return runtime, store, workspaceRoot
}

func waitForTurnCompleted(t *testing.T, store *sqlite.Store, turnID string, timeout time.Duration) types.Turn {
	t.Helper()
	ctx := context.Background()
	deadline := time.After(timeout)
	for {
		select {
		case <-deadline:
			t.Fatalf("timeout waiting for turn %s to complete", turnID)
		default:
		}
		turn, ok, err := store.GetTurn(ctx, turnID)
		if err != nil {
			t.Fatalf("GetTurn: %v", err)
		}
		if !ok {
			t.Fatalf("turn %s not found", turnID)
		}
		if isTurnTerminal(turn.State) {
			return turn
		}
		time.Sleep(500 * time.Millisecond)
	}
}

func collectEvents(bus *stream.Bus, sessionID string, timeout time.Duration) []types.Event {
	ch, unsubscribe := bus.Subscribe(sessionID)
	defer unsubscribe()
	var events []types.Event
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	for {
		select {
		case e, ok := <-ch:
			if !ok {
				return events
			}
			events = append(events, e)
		case <-timer.C:
			return events
		}
	}
}

func newMultiRoleTurn(t *testing.T, store *sqlite.Store, sessionRow types.Session, head types.ContextHead, kind types.TurnKind, message string) types.Turn {
	t.Helper()
	now := time.Now().UTC()
	turn := types.Turn{
		ID:            types.NewID("turn"),
		SessionID:     sessionRow.ID,
		ContextHeadID: head.ID,
		Kind:          kind,
		State:         types.TurnStateCreated,
		UserMessage:   message,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := store.InsertTurn(context.Background(), turn); err != nil {
		t.Fatalf("InsertTurn: %v", err)
	}
	return turn
}

func hasMultiRoleEvent(events []types.Event, kind string) bool {
	for _, event := range events {
		if event.Type == kind {
			return true
		}
	}
	return false
}

func multiRoleEventTypes(events []types.Event) []string {
	out := make([]string, len(events))
	for i, event := range events {
		out[i] = event.Type
	}
	return out
}

func multiRoleAssistantDeltaText(events []types.Event) string {
	var builder strings.Builder
	for _, event := range events {
		if event.Type != types.EventAssistantDelta {
			continue
		}
		var payload types.AssistantDeltaPayload
		if err := json.Unmarshal(event.Payload, &payload); err == nil {
			builder.WriteString(payload.Text)
		}
	}
	return builder.String()
}

func hasMultiRoleToolEvent(events []types.Event, kind string, toolName string) bool {
	for _, event := range events {
		if event.Type != kind {
			continue
		}
		var payload types.ToolEventPayload
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			continue
		}
		if payload.ToolName == toolName {
			return true
		}
	}
	return false
}

func multiRoleToolNames(events []types.Event) []string {
	var names []string
	for _, event := range events {
		if event.Type != types.EventToolStarted {
			continue
		}
		var payload types.ToolEventPayload
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			continue
		}
		names = append(names, payload.ToolName)
	}
	return names
}

func delegationTaskID(events []types.Event) string {
	for _, event := range events {
		if event.Type != types.EventToolCompleted {
			continue
		}
		var payload types.ToolEventPayload
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			continue
		}
		if payload.ToolName != "delegate_to_role" {
			continue
		}
		preview := strings.TrimSpace(payload.ResultPreview)
		if taskID := taskIDFromText(preview); taskID != "" {
			return taskID
		}
		if idx := strings.Index(preview, "("); idx >= 0 {
			rest := preview[idx+1:]
			if idx2 := strings.LastIndex(rest, ")"); idx2 >= 0 {
				rest = rest[:idx2]
			}
			parts := strings.Fields(rest)
			for _, p := range parts {
				if strings.HasPrefix(p, "task_") {
					return p
				}
			}
		}
	}
	return ""
}

func taskIDFromText(text string) string {
	idx := strings.Index(text, "task_")
	if idx < 0 {
		return ""
	}
	end := idx
	for end < len(text) {
		c := text[end]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_' {
			end++
			continue
		}
		break
	}
	return text[idx:end]
}

func multiRoleDelegatedTaskID(t *testing.T, rt *Runtime, workspaceRoot string, parentSessionID string, parentTurnID string, targetRole string, events []types.Event) string {
	t.Helper()
	if taskID := delegationTaskID(events); taskID != "" {
		return taskID
	}
	tasks, err := rt.TaskManager.List(workspaceRoot)
	if err != nil {
		t.Fatalf("TaskManager.List: %v", err)
	}
	for _, taskRecord := range tasks {
		if taskRecord.ParentSessionID != parentSessionID {
			continue
		}
		if taskRecord.ParentTurnID != parentTurnID {
			continue
		}
		if targetRole != "" && taskRecord.TargetRole != targetRole {
			continue
		}
		return taskRecord.ID
	}
	return ""
}

func requireMultiRoleTaskCompleted(t *testing.T, rt *Runtime, workspaceRoot string, taskID string) {
	t.Helper()
	waitCtx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	taskRecord, timedOut, err := rt.TaskManager.Wait(waitCtx, taskID, workspaceRoot)
	if err != nil {
		t.Fatalf("TaskManager.Wait: %v", err)
	}
	if timedOut {
		t.Fatalf("timeout waiting for delegated task %s to complete", taskID)
	}
	if string(taskRecord.Status) != "completed" {
		t.Fatalf("delegated task state = %q, want completed", taskRecord.Status)
	}
}

func waitMultiRoleReportDeliveries(t *testing.T, store *sqlite.Store, sessionID string, minCount int, timeout time.Duration) []types.ReportDelivery {
	t.Helper()
	deadline := time.After(timeout)
	for {
		deliveries, err := store.ListReportDeliveries(context.Background(), sessionID, types.ReportChannelAgent)
		if err != nil {
			t.Fatalf("ListReportDeliveries: %v", err)
		}
		if len(deliveries) >= minCount {
			return deliveries
		}
		select {
		case <-deadline:
			t.Fatalf("report deliveries = %d, want at least %d", len(deliveries), minCount)
		case <-time.After(1 * time.Second):
		}
	}
}

func TestMultiRoleTwoSpecialistsConcurrentlyProduceAssistantText(t *testing.T) {
	ctx := context.Background()
	roles := []multiRoleDef{
		{
			ID:          "stock_checker",
			DisplayName: "Stock Checker",
			Prompt:      "You are a stock market analyst. Use shell_command or web_fetch to check a financial data source. Report findings concisely.",
		},
		{
			ID:          "headline_checker",
			DisplayName: "Headline Checker",
			Prompt:      "You are a news analyst. Use web_fetch to get headlines. Report findings concisely.",
		},
	}
	runtime, store, workspaceRoot := setupMultiRoleRuntime(t, roles)

	if _, _, _, err := store.EnsureRoleSession(ctx, workspaceRoot, types.SessionRoleMainParent); err != nil {
		t.Fatalf("EnsureRoleSession: %v", err)
	}

	stockSession, stockHead, _, err := store.EnsureSpecialistSession(ctx, workspaceRoot, roles[0].ID, roles[0].Prompt, nil)
	if err != nil {
		t.Fatalf("EnsureSpecialistSession(stock_checker): %v", err)
	}
	headlineSession, headlineHead, _, err := store.EnsureSpecialistSession(ctx, workspaceRoot, roles[1].ID, roles[1].Prompt, nil)
	if err != nil {
		t.Fatalf("EnsureSpecialistSession(headline_checker): %v", err)
	}

	stockTurn := newMultiRoleTurn(t, store, stockSession, stockHead, types.TurnKindUserMessage, "Use web_fetch to check https://example.com and report what you find. Keep it short.")
	headlineTurn := newMultiRoleTurn(t, store, headlineSession, headlineHead, types.TurnKindUserMessage, "Use web_fetch to check https://httpbin.org/get and report the response status. Keep it short.")

	stockSink := &multiRoleSink{}
	headlineSink := &multiRoleSink{}
	errCh := make(chan error, 2)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		errCh <- runtime.Engine.RunTurn(withRunnerSessionContext(ctx, stockSession, types.SessionRoleMainParent, roles[0].ID), engine.Input{
			Session:     stockSession,
			SessionRole: types.SessionRoleMainParent,
			Turn:        stockTurn,
			Sink:        stockSink,
		})
	}()
	go func() {
		defer wg.Done()
		errCh <- runtime.Engine.RunTurn(withRunnerSessionContext(ctx, headlineSession, types.SessionRoleMainParent, roles[1].ID), engine.Input{
			Session:     headlineSession,
			SessionRole: types.SessionRoleMainParent,
			Turn:        headlineTurn,
			Sink:        headlineSink,
		})
	}()
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			t.Fatalf("RunTurn: %v", err)
		}
	}

	for name, sink := range map[string]*multiRoleSink{"stock_checker": stockSink, "headline_checker": headlineSink} {
		events := sink.Events()
		if !hasMultiRoleEvent(events, types.EventAssistantDelta) {
			t.Fatalf("%s assistant.delta missing; events = %#v", name, multiRoleEventTypes(events))
		}
		if !hasMultiRoleEvent(events, types.EventTurnCompleted) {
			t.Fatalf("%s turn.completed missing; events = %#v", name, multiRoleEventTypes(events))
		}
		if hasMultiRoleEvent(events, types.EventTurnFailed) {
			t.Fatalf("%s turn.failed present; events = %#v", name, multiRoleEventTypes(events))
		}
		if got := strings.TrimSpace(multiRoleAssistantDeltaText(events)); got == "" {
			t.Fatalf("%s assistant delta text empty; events = %#v", name, multiRoleEventTypes(events))
		}
	}
}

func TestMultiRoleSpecialistMultiToolChain(t *testing.T) {
	ctx := context.Background()
	role := multiRoleDef{
		ID:          "data_collector",
		DisplayName: "Data Collector",
		Prompt:      "You collect system facts. You must call shell_command first and memory_write second before your final response.",
	}
	runtime, store, workspaceRoot := setupMultiRoleRuntime(t, []multiRoleDef{role})

	if _, _, _, err := store.EnsureRoleSession(ctx, workspaceRoot, types.SessionRoleMainParent); err != nil {
		t.Fatalf("EnsureRoleSession: %v", err)
	}
	specialistSession, head, _, err := store.EnsureSpecialistSession(ctx, workspaceRoot, role.ID, role.Prompt, nil)
	if err != nil {
		t.Fatalf("EnsureSpecialistSession: %v", err)
	}
	turn := newMultiRoleTurn(t, store, specialistSession, head, types.TurnKindUserMessage, "Run shell_command with `echo 'collected: system has 8GB RAM, 4 CPUs'`. Then use memory_write to store this system info with a clear title. Report what you did.")
	sink := &multiRoleSink{}

	if err := runtime.Engine.RunTurn(withRunnerSessionContext(ctx, specialistSession, types.SessionRoleMainParent, role.ID), engine.Input{
		Session:     specialistSession,
		SessionRole: types.SessionRoleMainParent,
		Turn:        turn,
		Sink:        sink,
	}); err != nil {
		t.Fatalf("RunTurn: %v", err)
	}

	events := sink.Events()
	for _, toolName := range []string{"shell_command", "memory_write"} {
		if !hasMultiRoleToolEvent(events, types.EventToolStarted, toolName) {
			t.Fatalf("%s tool.started missing; events = %#v", toolName, multiRoleEventTypes(events))
		}
		if !hasMultiRoleToolEvent(events, types.EventToolCompleted, toolName) {
			t.Fatalf("%s tool.completed missing; events = %#v", toolName, multiRoleEventTypes(events))
		}
	}
	if !hasMultiRoleEvent(events, types.EventTurnCompleted) {
		t.Fatalf("turn.completed missing; events = %#v", multiRoleEventTypes(events))
	}
	if got := strings.TrimSpace(multiRoleAssistantDeltaText(events)); got == "" {
		t.Fatalf("assistant delta text empty; events = %#v", multiRoleEventTypes(events))
	}
}

func TestMultiRoleReportBatchTurnProcessesMultipleReports(t *testing.T) {
	ctx := context.Background()
	role := multiRoleDef{
		ID:          "report_helper",
		DisplayName: "Report Helper",
		Prompt:      "You help summarize reports.",
	}
	runtime, store, workspaceRoot := setupMultiRoleRuntime(t, []multiRoleDef{role})
	mainSession, head, _, err := store.EnsureRoleSession(ctx, workspaceRoot, types.SessionRoleMainParent)
	if err != nil {
		t.Fatalf("EnsureRoleSession: %v", err)
	}

	now := time.Now().UTC()
	reports := []types.ReportRecord{
		{
			ID:              "report_stock_alert",
			WorkspaceRoot:   workspaceRoot,
			SessionID:       mainSession.ID,
			TargetSessionID: mainSession.ID,
			Audience:        types.ReportAudienceMainAgent,
			SourceKind:      types.ReportSourceTaskResult,
			SourceID:        "task_stock_alert",
			Envelope: types.ReportEnvelope{
				Source:   "task_result",
				Status:   "completed",
				Severity: "info",
				Title:    "Stock Alert",
				Summary:  "AAPL up 5% today",
			},
			ObservedAt: now,
			CreatedAt:  now,
			UpdatedAt:  now,
		},
		{
			ID:              "report_news_alert",
			WorkspaceRoot:   workspaceRoot,
			SessionID:       mainSession.ID,
			TargetSessionID: mainSession.ID,
			Audience:        types.ReportAudienceMainAgent,
			SourceKind:      types.ReportSourceTaskResult,
			SourceID:        "task_news_alert",
			Envelope: types.ReportEnvelope{
				Source:   "task_result",
				Status:   "completed",
				Severity: "warning",
				Title:    "News Alert",
				Summary:  "Breaking: new AI regulation announced",
			},
			ObservedAt: now,
			CreatedAt:  now,
			UpdatedAt:  now,
		},
	}
	for _, report := range reports {
		delivery := reporting.DeliveryFromReport(report, now)
		item := types.ReportDeliveryItemFromRecordDelivery(report, delivery)
		if err := store.UpsertReportDeliveryItem(ctx, item); err != nil {
			t.Fatalf("UpsertReportDeliveryItem: %v", err)
		}
	}

	turn := newMultiRoleTurn(t, store, mainSession, head, types.TurnKindReportBatch, "")
	sink := &multiRoleSink{}
	if err := runtime.Engine.RunTurn(withRunnerSessionContext(ctx, mainSession, types.SessionRoleMainParent, ""), engine.Input{
		Session:     mainSession,
		SessionRole: types.SessionRoleMainParent,
		Turn:        turn,
		Sink:        sink,
	}); err != nil {
		t.Fatalf("RunTurn: %v", err)
	}

	events := sink.Events()
	if !hasMultiRoleEvent(events, types.EventTurnCompleted) {
		t.Fatalf("turn.completed missing; events = %#v", multiRoleEventTypes(events))
	}
	if got := strings.TrimSpace(multiRoleAssistantDeltaText(events)); got == "" {
		t.Fatalf("assistant delta text empty; events = %#v", multiRoleEventTypes(events))
	}
	deliveries, err := store.ListReportDeliveries(ctx, mainSession.ID, types.ReportChannelAgent)
	if err != nil {
		t.Fatalf("ListReportDeliveries: %v", err)
	}
	if len(deliveries) != 2 {
		t.Fatalf("ListReportDeliveries length = %d, want 2", len(deliveries))
	}
	for _, delivery := range deliveries {
		if delivery.State != types.ReportDeliveryStateDelivered {
			t.Fatalf("delivery %s state = %q, want %q", delivery.ID, delivery.State, types.ReportDeliveryStateDelivered)
		}
		if delivery.InjectedTurnID != turn.ID {
			t.Fatalf("delivery %s injected turn = %q, want %q", delivery.ID, delivery.InjectedTurnID, turn.ID)
		}
	}
}

func TestMultiRoleSessionManagerSubmitTurnCompletes(t *testing.T) {
	ctx := context.Background()
	role := multiRoleDef{
		ID:          "quick_reporter",
		DisplayName: "Quick Reporter",
		Prompt:      "You are a fast reporter.",
	}
	runtime, store, workspaceRoot := setupMultiRoleRuntime(t, []multiRoleDef{role})

	if _, _, _, err := store.EnsureRoleSession(ctx, workspaceRoot, types.SessionRoleMainParent); err != nil {
		t.Fatalf("EnsureRoleSession: %v", err)
	}
	specialistSession, head, _, err := store.EnsureSpecialistSession(ctx, workspaceRoot, role.ID, role.Prompt, nil)
	if err != nil {
		t.Fatalf("EnsureSpecialistSession: %v", err)
	}
	runtime.SessionManager.RegisterSession(specialistSession)
	turn := newMultiRoleTurn(t, store, specialistSession, head, types.TurnKindUserMessage, "Run shell_command with `echo 'report: all clear'`. Report the result.")

	done := make(chan error, 1)
	turnID, err := runtime.SessionManager.SubmitTurn(ctx, specialistSession.ID, session.SubmitTurnInput{
		Turn: turn,
		Run:  session.RunMetadata{Done: done},
	})
	if err != nil {
		t.Fatalf("SubmitTurn: %v", err)
	}
	if turnID != turn.ID {
		t.Fatalf("SubmitTurn turnID = %q, want %q", turnID, turn.ID)
	}

	select {
	case runErr := <-done:
		if runErr != nil {
			t.Fatalf("RunTurn: %v", runErr)
		}
	case <-time.After(5 * time.Minute):
		t.Fatal("timeout waiting for session manager turn")
	}

	completed := waitForTurnCompleted(t, store, turn.ID, 30*time.Second)
	if completed.State != types.TurnStateCompleted {
		t.Fatalf("turn state = %q, want %q", completed.State, types.TurnStateCompleted)
	}
}

func TestMultiRoleBuildRuntimeCreatesComponents(t *testing.T) {
	roles := []multiRoleDef{
		{
			ID:          "stock_checker",
			DisplayName: "Stock Checker",
			Prompt:      "You are a stock market analyst.",
		},
		{
			ID:          "headline_checker",
			DisplayName: "Headline Checker",
			Prompt:      "You are a news analyst.",
		},
	}
	runtime, store, _ := setupMultiRoleRuntime(t, roles)

	if runtime.Store != store {
		t.Fatalf("runtime.Store mismatch")
	}
	checks := []struct {
		name    string
		missing bool
	}{
		{"Engine", runtime.Engine == nil},
		{"SessionManager", runtime.SessionManager == nil},
		{"TaskManager", runtime.TaskManager == nil},
		{"SchedulerService", runtime.SchedulerService == nil},
		{"Store", runtime.Store == nil},
		{"Bus", runtime.Bus == nil},
		{"FileCheckpoints", runtime.FileCheckpoints == nil},
		{"RuntimeService", runtime.RuntimeService == nil},
		{"AutomationService", runtime.AutomationService == nil},
		{"WatcherService", runtime.WatcherService == nil},
		{"ReportingService", runtime.ReportingService == nil},
		{"TaskNotifier", runtime.TaskNotifier == nil},
	}
	for _, check := range checks {
		if check.missing {
			t.Fatalf("%s is nil", check.name)
		}
	}
}

func TestMultiRoleDelegationChainProducesReport(t *testing.T) {
	ctx := context.Background()
	role := multiRoleDef{
		ID:          "disk_checker",
		DisplayName: "Disk Checker",
		Prompt:      "You check disk space using shell_command. Always use shell_command to run `df -h`. Report the output concisely.",
	}
	runtime, store, workspaceRoot := setupMultiRoleRuntime(t, []multiRoleDef{role})

	mainSession, head, _, err := store.EnsureRoleSession(ctx, workspaceRoot, types.SessionRoleMainParent)
	if err != nil {
		t.Fatalf("EnsureRoleSession: %v", err)
	}
	runtime.SessionManager.RegisterSession(mainSession)

	turn := newMultiRoleTurn(t, store, mainSession, head, types.TurnKindUserMessage, "Use delegate_to_role to delegate to disk_checker: run df -h and report the disk space. Keep it short.")
	sink := &multiRoleSink{}

	if err := runtime.Engine.RunTurn(withRunnerSessionContext(ctx, mainSession, types.SessionRoleMainParent, ""), engine.Input{
		Session:     mainSession,
		SessionRole: types.SessionRoleMainParent,
		Turn:        turn,
		Sink:        sink,
	}); err != nil {
		t.Fatalf("RunTurn: %v", err)
	}

	events := sink.Events()
	if !hasMultiRoleEvent(events, types.EventTurnCompleted) {
		t.Fatalf("turn.completed missing; events = %#v", multiRoleEventTypes(events))
	}
	if !hasMultiRoleToolEvent(events, types.EventToolStarted, "delegate_to_role") {
		t.Fatalf("delegate_to_role was not called; events = %#v", multiRoleEventTypes(events))
	}

	taskID := multiRoleDelegatedTaskID(t, runtime, workspaceRoot, mainSession.ID, turn.ID, role.ID, events)
	if taskID == "" {
		t.Fatalf("could not extract task ID from delegate_to_role output")
	}

	requireMultiRoleTaskCompleted(t, runtime, workspaceRoot, taskID)

	deliveries := waitMultiRoleReportDeliveries(t, store, mainSession.ID, 1, 60*time.Second)
	if len(deliveries) == 0 {
		t.Fatalf("no report deliveries found for main parent session")
	}

	t.Logf("delegation chain complete: task %s, %d report deliveries", taskID, len(deliveries))
}

func TestMultiRoleConcurrentDelegation(t *testing.T) {
	ctx := context.Background()
	roles := []multiRoleDef{
		{
			ID:          "stock_checker",
			DisplayName: "Stock Checker",
			Prompt:      "You are a stock analyst. Use web_fetch to check https://httpbin.org/get and report the response status. Be concise.",
		},
		{
			ID:          "weather_checker",
			DisplayName: "Weather Checker",
			Prompt:      "You are a weather analyst. Use shell_command to run `echo 'weather: sunny, 72F'`. Report the output. Be concise.",
		},
	}
	runtime, store, workspaceRoot := setupMultiRoleRuntime(t, roles)

	mainSession, head, _, err := store.EnsureRoleSession(ctx, workspaceRoot, types.SessionRoleMainParent)
	if err != nil {
		t.Fatalf("EnsureRoleSession: %v", err)
	}
	runtime.SessionManager.RegisterSession(mainSession)

	delegationMessages := []string{
		"Use delegate_to_role to delegate to stock_checker: check https://httpbin.org/get and report the status. Be concise.",
		"Use delegate_to_role to delegate to weather_checker: run shell_command `echo 'weather: sunny, 72F'`. Report the output. Be concise.",
	}
	turns := make([]types.Turn, len(delegationMessages))
	for i := range delegationMessages {
		turns[i] = newMultiRoleTurn(t, store, mainSession, head, types.TurnKindUserMessage, delegationMessages[i])
	}

	eventCh, unsubscribe := runtime.Bus.Subscribe(mainSession.ID)
	defer unsubscribe()
	stopEvents := make(chan struct{})
	defer close(stopEvents)
	eventsCh := make(chan []types.Event, 1)
	go func() {
		var events []types.Event
		completed := make(map[string]bool)
		for {
			select {
			case event, ok := <-eventCh:
				if !ok {
					eventsCh <- events
					return
				}
				events = append(events, event)
				if event.Type != types.EventTurnCompleted {
					continue
				}
				for _, turn := range turns {
					if event.TurnID == turn.ID {
						completed[turn.ID] = true
					}
				}
				if len(completed) == len(turns) {
					eventsCh <- events
					return
				}
			case <-stopEvents:
				eventsCh <- events
				return
			}
		}
	}()

	var wg sync.WaitGroup
	errCh := make(chan error, len(turns))
	doneChs := make([]chan error, len(turns))
	for i := range turns {
		doneChs[i] = make(chan error, 1)
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_, submitErr := runtime.SessionManager.SubmitTurn(ctx, mainSession.ID, session.SubmitTurnInput{
				Turn: turns[idx],
				Run:  session.RunMetadata{Done: doneChs[idx]},
			})
			errCh <- submitErr
		}(i)
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			t.Fatalf("RunTurn: %v", err)
		}
	}

	for i, doneCh := range doneChs {
		select {
		case runErr := <-doneCh:
			if runErr != nil {
				t.Fatalf("delegation %d: RunTurn: %v", i, runErr)
			}
		case <-time.After(5 * time.Minute):
			t.Fatalf("delegation %d: timeout waiting for turn", i)
		}
	}

	var events []types.Event
	select {
	case events = <-eventsCh:
	case <-time.After(30 * time.Second):
		t.Fatal("timeout waiting for delegation events")
	}

	var taskIDs []string
	for i, turn := range turns {
		var turnEvents []types.Event
		for _, event := range events {
			if event.TurnID == turn.ID {
				turnEvents = append(turnEvents, event)
			}
		}
		if !hasMultiRoleEvent(turnEvents, types.EventTurnCompleted) {
			t.Fatalf("delegation %d: turn.completed missing; events = %#v", i, multiRoleEventTypes(turnEvents))
		}
		if !hasMultiRoleToolEvent(turnEvents, types.EventToolStarted, "delegate_to_role") {
			t.Fatalf("delegation %d: delegate_to_role was not called; events = %#v tools = %#v", i, multiRoleEventTypes(turnEvents), multiRoleToolNames(turnEvents))
		}
		if taskID := multiRoleDelegatedTaskID(t, runtime, workspaceRoot, mainSession.ID, turn.ID, roles[i].ID, turnEvents); taskID != "" {
			taskIDs = append(taskIDs, taskID)
		}
	}

	for _, taskID := range taskIDs {
		requireMultiRoleTaskCompleted(t, runtime, workspaceRoot, taskID)
	}

	deliveries := waitMultiRoleReportDeliveries(t, store, mainSession.ID, 1, 60*time.Second)
	t.Logf("concurrent delegation: %d report deliveries for main parent", len(deliveries))
}

func TestMultiRoleSpecialistCannotDelegate(t *testing.T) {
	ctx := context.Background()
	role := multiRoleDef{
		ID:          "boundary_tester",
		DisplayName: "Boundary Tester",
		Prompt:      "You are a specialist role tester. You should report what tools you have available and whether you can delegate work to other roles.",
	}
	runtime, store, workspaceRoot := setupMultiRoleRuntime(t, []multiRoleDef{role})

	if _, _, _, err := store.EnsureRoleSession(ctx, workspaceRoot, types.SessionRoleMainParent); err != nil {
		t.Fatalf("EnsureRoleSession: %v", err)
	}
	specialistSession, head, _, err := store.EnsureSpecialistSession(ctx, workspaceRoot, role.ID, role.Prompt, nil)
	if err != nil {
		t.Fatalf("EnsureSpecialistSession: %v", err)
	}
	turn := newMultiRoleTurn(t, store, specialistSession, head, types.TurnKindUserMessage, "List your available tools. Then tell me whether you can use delegate_to_role to delegate work to another role. Just check your tool list and report what you find.")
	sink := &multiRoleSink{}

	if err := runtime.Engine.RunTurn(withRunnerSessionContext(ctx, specialistSession, types.SessionRoleMainParent, role.ID), engine.Input{
		Session:     specialistSession,
		SessionRole: types.SessionRoleMainParent,
		Turn:        turn,
		Sink:        sink,
	}); err != nil {
		t.Fatalf("RunTurn: %v", err)
	}

	events := sink.Events()
	if !hasMultiRoleEvent(events, types.EventTurnCompleted) {
		t.Fatalf("turn.completed missing; events = %#v", multiRoleEventTypes(events))
	}
	if hasMultiRoleToolEvent(events, types.EventToolStarted, "delegate_to_role") {
		t.Fatalf("delegate_to_role was called by specialist; this should be hidden from specialist context")
	}
	if got := strings.TrimSpace(multiRoleAssistantDeltaText(events)); got == "" {
		t.Fatalf("assistant delta text empty; events = %#v", multiRoleEventTypes(events))
	}
}
