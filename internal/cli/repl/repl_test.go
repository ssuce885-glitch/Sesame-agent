package repl

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"go-agent/internal/cli/client"
	"go-agent/internal/extensions"
	"go-agent/internal/types"
)

func TestHandleSlashCommandSessionList(t *testing.T) {
	var stdout bytes.Buffer
	r := New(Options{
		Stdout: &stdout,
		Client: stubClient{
			listSessions: func(context.Context) (types.ListSessionsResponse, error) {
				return types.ListSessionsResponse{
					SelectedSessionID: "sess_1",
					Sessions: []types.SessionListItem{
						{ID: "sess_1", WorkspaceRoot: "E:/project/go-agent"},
					},
				}, nil
			},
		},
	})

	handled, err := r.HandleLine(context.Background(), "/session list")
	if err != nil {
		t.Fatalf("HandleLine() error = %v", err)
	}
	if !handled {
		t.Fatal("handled = false, want true")
	}
	if !strings.Contains(stdout.String(), "sess_1") {
		t.Fatalf("stdout = %q, want session id", stdout.String())
	}
}

func TestHandlePlainTextStreamsAssistantOutput(t *testing.T) {
	var stdout bytes.Buffer
	events := make(chan types.Event, 2)
	events <- types.Event{
		Seq:  1,
		Type: types.EventAssistantDelta,
		Payload: json.RawMessage(`{
			"text":"hello"
		}`),
	}
	events <- types.Event{Seq: 2, Type: types.EventTurnCompleted}
	close(events)

	r := New(Options{
		Stdout:    &stdout,
		SessionID: "sess_1",
		Client: stubClient{
			submitTurn: func(context.Context, string, types.SubmitTurnRequest) (types.Turn, error) {
				return types.Turn{ID: "turn_1"}, nil
			},
			streamEvents: func(context.Context, string, int64) (<-chan types.Event, error) {
				return events, nil
			},
		},
	})

	handled, err := r.HandleLine(context.Background(), "say hi")
	if err != nil {
		t.Fatalf("HandleLine() error = %v", err)
	}
	if handled {
		t.Fatal("handled = true, want false for plain prompt")
	}
	if !strings.Contains(stdout.String(), "hello") {
		t.Fatalf("stdout = %q, want streamed assistant text", stdout.String())
	}
}

func TestHandlePlainTextReturnsOnTurnInterrupted(t *testing.T) {
	var stdout bytes.Buffer
	events := make(chan types.Event, 2)
	events <- types.Event{
		Seq:  1,
		Type: types.EventAssistantDelta,
		Payload: json.RawMessage(`{
			"text":"hello"
		}`),
	}
	events <- types.Event{Seq: 2, Type: types.EventTurnInterrupted}
	close(events)

	r := New(Options{
		Stdout:    &stdout,
		SessionID: "sess_1",
		Client: stubClient{
			submitTurn: func(context.Context, string, types.SubmitTurnRequest) (types.Turn, error) {
				return types.Turn{ID: "turn_1"}, nil
			},
			streamEvents: func(context.Context, string, int64) (<-chan types.Event, error) {
				return events, nil
			},
		},
	})

	handled, err := r.HandleLine(context.Background(), "say hi")
	if err != nil {
		t.Fatalf("HandleLine() error = %v", err)
	}
	if handled {
		t.Fatal("handled = true, want false for plain prompt")
	}
	if !strings.Contains(stdout.String(), "hello") {
		t.Fatalf("stdout = %q, want streamed assistant text", stdout.String())
	}
}

func TestHandleSlashCommandSkills(t *testing.T) {
	var stdout bytes.Buffer
	r := New(Options{
		Stdout: &stdout,
		Client: stubClient{},
		Catalog: extensions.Catalog{
			Skills: []extensions.Skill{{
				Name:        "demo-skill",
				Scope:       "workspace",
				Description: "help with demos",
			}},
		},
	})

	handled, err := r.HandleLine(context.Background(), "/skills")
	if err != nil {
		t.Fatalf("HandleLine() error = %v", err)
	}
	if !handled {
		t.Fatal("handled = false, want true")
	}
	if !strings.Contains(stdout.String(), "demo-skill") {
		t.Fatalf("stdout = %q, want discovered skill", stdout.String())
	}
}

func TestHandleSlashCommandSkillsUsesCatalogLoader(t *testing.T) {
	var stdout bytes.Buffer
	r := New(Options{
		Stdout: &stdout,
		Client: stubClient{},
		Catalog: extensions.Catalog{
			Skills: []extensions.Skill{{
				Name:        "stale-skill",
				Scope:       "workspace",
				Description: "stale startup snapshot",
			}},
		},
		CatalogLoader: func() (extensions.Catalog, error) {
			return extensions.Catalog{
				Skills: []extensions.Skill{{
					Name:        "fresh-skill",
					Scope:       "workspace",
					Description: "freshly loaded",
				}},
			}, nil
		},
	})

	handled, err := r.HandleLine(context.Background(), "/skills")
	if err != nil {
		t.Fatalf("HandleLine() error = %v", err)
	}
	if !handled {
		t.Fatal("handled = false, want true")
	}
	output := stdout.String()
	if !strings.Contains(output, "fresh-skill") {
		t.Fatalf("stdout = %q, want freshly loaded skill", output)
	}
	if strings.Contains(output, "stale-skill") {
		t.Fatalf("stdout = %q, want loader result instead of startup snapshot", output)
	}
}

func TestHandleSlashCommandApproveUsesLatestPermissionRequestAndStreamsResume(t *testing.T) {
	var stdout bytes.Buffer
	events := make(chan types.Event, 2)
	events <- types.Event{
		Seq:  4,
		Type: types.EventAssistantDelta,
		Payload: json.RawMessage(`{
			"text":"resumed"
		}`),
	}
	events <- types.Event{Seq: 5, Type: types.EventTurnCompleted}
	close(events)

	var decided types.PermissionDecisionRequest
	r := New(Options{
		Stdout:    &stdout,
		SessionID: "sess_1",
		Client: stubClient{
			decidePermission: func(_ context.Context, req types.PermissionDecisionRequest) (types.PermissionDecisionResponse, error) {
				decided = req
				return types.PermissionDecisionResponse{
					Request: types.PermissionRequest{ID: req.RequestID},
					TurnID:  "turn_1",
					Resumed: true,
				}, nil
			},
			streamEvents: func(_ context.Context, sessionID string, afterSeq int64) (<-chan types.Event, error) {
				if sessionID != "sess_1" {
					t.Fatalf("sessionID = %q, want sess_1", sessionID)
				}
				if afterSeq != 3 {
					t.Fatalf("afterSeq = %d, want 3", afterSeq)
				}
				return events, nil
			},
		},
	})
	r.lastSeq = 3
	r.lastPermissionRequestID = "perm_123"

	handled, err := r.HandleLine(context.Background(), "/approve")
	if err != nil {
		t.Fatalf("HandleLine() error = %v", err)
	}
	if !handled {
		t.Fatal("handled = false, want true")
	}
	if decided.RequestID != "perm_123" {
		t.Fatalf("RequestID = %q, want perm_123", decided.RequestID)
	}
	if decided.Decision != types.PermissionDecisionAllowOnce {
		t.Fatalf("Decision = %q, want %q", decided.Decision, types.PermissionDecisionAllowOnce)
	}
	if !strings.Contains(stdout.String(), "resumed") {
		t.Fatalf("stdout = %q, want resumed assistant output", stdout.String())
	}
}

func TestBuildPermissionDecisionRequestUsesLatestRequestForScopeOnlyApprove(t *testing.T) {
	req, err := buildPermissionDecisionRequest("approve", []string{"session"}, "perm_123")
	if err != nil {
		t.Fatalf("buildPermissionDecisionRequest() error = %v", err)
	}
	if req.RequestID != "perm_123" {
		t.Fatalf("RequestID = %q, want perm_123", req.RequestID)
	}
	if req.Decision != types.PermissionDecisionAllowSession {
		t.Fatalf("Decision = %q, want %q", req.Decision, types.PermissionDecisionAllowSession)
	}
}

func TestTUIModelMailboxCommandLoadsStructuredView(t *testing.T) {
	model := newTUIModel(tuiModelOptions{
		Context:   context.Background(),
		SessionID: "sess_1",
		Client: stubClient{
			getMailbox: func(context.Context, string) (types.SessionReportMailboxResponse, error) {
				return types.SessionReportMailboxResponse{
					PendingCount: 1,
					Items: []types.ReportMailboxItem{{
						ID:         "mail_1",
						SessionID:  "sess_1",
						SourceKind: types.ReportMailboxSourceTaskResult,
						ObservedAt: time.Unix(0, 0).UTC(),
						Envelope: types.ReportEnvelope{
							Title:   "Shanghai weather",
							Summary: "18C and cloudy",
						},
					}},
				}, nil
			},
		},
	})

	updated, cmd := model.handleCommand("/mailbox")
	got, ok := updated.(*tuiModel)
	if !ok {
		t.Fatalf("updated type = %T, want *tuiModel", updated)
	}
	if got.activeView != tuiViewMailbox {
		t.Fatalf("activeView = %q, want mailbox", got.activeView)
	}
	if cmd == nil {
		t.Fatal("cmd = nil, want mailbox load")
	}

	next, _ := got.Update(cmd())
	loaded := next.(tuiModel)
	if !loaded.mailboxLoaded {
		t.Fatal("mailboxLoaded = false, want true")
	}
	if loaded.pendingReportCount != 1 {
		t.Fatalf("pendingReportCount = %d, want 1", loaded.pendingReportCount)
	}
	if len(loaded.entries) != 0 {
		t.Fatalf("len(entries) = %d, want chat stream unchanged", len(loaded.entries))
	}
	if !strings.Contains(loaded.renderViewportContent(), "Shanghai weather") {
		t.Fatalf("mailbox view = %q, want mailbox content", loaded.renderViewportContent())
	}
}

func TestTUIModelAgentsCommandLoadsRuntimeGraph(t *testing.T) {
	model := newTUIModel(tuiModelOptions{
		Context:   context.Background(),
		SessionID: "sess_1",
		Client: stubClient{
			getRuntimeGraph: func(context.Context, string) (types.SessionRuntimeGraphResponse, error) {
				return types.SessionRuntimeGraphResponse{
					Graph: types.RuntimeGraph{
						Runs: []types.Run{{
							ID:        "run_1",
							SessionID: "sess_1",
							State:     types.RunStateRunning,
							Objective: "Summarize child agent output",
						}},
						Tasks: []types.Task{{
							ID:    "task_1",
							RunID: "run_1",
							State: types.TaskStateRunning,
							Title: "Collect worker results",
						}},
					},
				}, nil
			},
			getReporting: func(_ context.Context, sessionID string) (types.ReportingOverview, error) {
				if sessionID != "sess_1" {
					t.Fatalf("sessionID = %q, want sess_1", sessionID)
				}
				return types.ReportingOverview{
					ChildAgents: []types.ChildAgentSpec{{
						AgentID:   "cron_weather",
						SessionID: "sess_1",
						Purpose:   "Hefei weather worker",
						Mode:      types.ChildAgentModeBackgroundWorker,
						Schedule: types.ScheduleSpec{
							Kind:         types.ScheduleKindEvery,
							EveryMinutes: 5,
						},
					}},
					ChildResults: []types.ChildAgentResult{{
						ResultID:  "result_1",
						SessionID: "sess_1",
						AgentID:   "cron_weather",
						Envelope: types.ReportEnvelope{
							Title:   "Hefei weather worker",
							Status:  "completed",
							Summary: "22C and cloudy",
						},
					}},
					Digests: []types.DigestRecord{{
						DigestID:  "digest_1",
						SessionID: "sess_1",
						GroupID:   "weather-daily",
						Envelope: types.ReportEnvelope{
							Title:   "Weather digest",
							Status:  "completed",
							Summary: "1 worker reported in.",
						},
					}},
				}, nil
			},
		},
	})

	updated, cmd := model.handleCommand("/agents")
	got, ok := updated.(*tuiModel)
	if !ok {
		t.Fatalf("updated type = %T, want *tuiModel", updated)
	}
	if got.activeView != tuiViewAgents {
		t.Fatalf("activeView = %q, want agents", got.activeView)
	}
	if cmd == nil {
		t.Fatal("cmd = nil, want runtime graph load")
	}

	nextModel := tea.Model(*got)
	switch msg := cmd().(type) {
	case tea.BatchMsg:
		for _, sub := range msg {
			if sub == nil {
				continue
			}
			nextModel, _ = nextModel.Update(sub())
		}
	default:
		nextModel, _ = nextModel.Update(msg)
	}
	loaded := nextModel.(tuiModel)
	if !loaded.runtimeGraphLoaded {
		t.Fatal("runtimeGraphLoaded = false, want true")
	}
	if !loaded.reportingLoaded {
		t.Fatal("reportingLoaded = false, want true")
	}
	rendered := loaded.renderViewportContent()
	if !strings.Contains(rendered, "Summarize child agent output") || !strings.Contains(rendered, "Collect worker results") || !strings.Contains(rendered, "Hefei weather worker") || !strings.Contains(rendered, "Weather digest") {
		t.Fatalf("agents view = %q, want runtime and reporting content", rendered)
	}
}

func TestTUIModelPermissionRequestedNoticeIncludesApproveHint(t *testing.T) {
	model := newTUIModel(tuiModelOptions{
		Context:   context.Background(),
		SessionID: "sess_1",
		Client:    stubClient{},
	})

	payload, err := json.Marshal(types.PermissionRequestedPayload{
		RequestID:        "perm_123",
		ToolName:         "request_permissions",
		RequestedProfile: "trusted_local",
		Reason:           "need to edit ~/.sesame/skills/agent-browser/SKILL.md",
	})
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	model.applyEvent(types.Event{
		Seq:     1,
		Type:    types.EventPermissionRequested,
		Payload: payload,
	})

	if model.lastPermissionRequestID != "perm_123" {
		t.Fatalf("lastPermissionRequestID = %q, want perm_123", model.lastPermissionRequestID)
	}
	if len(model.entries) == 0 {
		t.Fatal("len(entries) = 0, want notice entry")
	}
	body := model.entries[len(model.entries)-1].Body
	if !strings.Contains(body, "/approve perm_123") || !strings.Contains(body, "/deny perm_123") {
		t.Fatalf("notice body = %q, want approval hint", body)
	}
}

func TestTUIModelTabCyclesViews(t *testing.T) {
	model := newTUIModel(tuiModelOptions{
		Context: context.Background(),
		Client:  stubClient{},
	})

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyTab})
	got, ok := updated.(*tuiModel)
	if !ok {
		t.Fatalf("updated type = %T, want *tuiModel", updated)
	}
	if got.activeView != tuiViewAgents {
		t.Fatalf("activeView after tab = %q, want agents", got.activeView)
	}

	updated, _ = got.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	back, ok := updated.(*tuiModel)
	if !ok {
		t.Fatalf("updated type = %T, want *tuiModel", updated)
	}
	if back.activeView != tuiViewChat {
		t.Fatalf("activeView after shift+tab = %q, want chat", back.activeView)
	}
}

func TestTUIModelReportReadyRefreshesMailboxCache(t *testing.T) {
	model := newTUIModel(tuiModelOptions{
		Context:   context.Background(),
		SessionID: "sess_1",
		Client: stubClient{
			getMailbox: func(context.Context, string) (types.SessionReportMailboxResponse, error) {
				return types.SessionReportMailboxResponse{
					PendingCount: 2,
					Items: []types.ReportMailboxItem{{
						ID:         "mail_1",
						SessionID:  "sess_1",
						SourceKind: types.ReportMailboxSourceDigest,
						Envelope: types.ReportEnvelope{
							Title: "Daily digest",
						},
					}},
				}, nil
			},
		},
	})

	raw, err := json.Marshal(types.ReportMailboxItem{
		ID:         "mail_1",
		SessionID:  "sess_1",
		SourceKind: types.ReportMailboxSourceDigest,
		Envelope:   types.ReportEnvelope{Title: "Daily digest"},
	})
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	cmd := model.applyEvent(types.Event{
		Seq:       1,
		SessionID: "sess_1",
		Type:      types.EventReportReady,
		Payload:   raw,
	})
	if model.pendingReportCount != 1 {
		t.Fatalf("pendingReportCount after event = %d, want optimistic increment", model.pendingReportCount)
	}
	if model.statusFlash != "Mailbox updated" {
		t.Fatalf("statusFlash = %q, want mailbox update notice", model.statusFlash)
	}
	if cmd == nil {
		t.Fatal("cmd = nil, want mailbox refresh")
	}

	updated, _ := model.Update(cmd())
	loaded := updated.(tuiModel)
	if !loaded.mailboxLoaded {
		t.Fatal("mailboxLoaded = false, want true after refresh")
	}
	if loaded.pendingReportCount != 2 {
		t.Fatalf("pendingReportCount = %d, want refreshed value 2", loaded.pendingReportCount)
	}
}

func TestTUIModelScrollsViewportOnPageDown(t *testing.T) {
	model := newTUIModel(tuiModelOptions{
		Context: context.Background(),
		Client:  stubClient{},
	})
	model.width = 100
	model.height = 20
	for i := 0; i < 40; i++ {
		model.appendEntry(tuiEntry{
			Kind:  tuiEntryAssistant,
			Title: "Sesame",
			Body:  "line " + strings.Repeat("content ", 8),
		})
	}
	model.layout()
	model.viewport.GotoTop()

	if model.viewport.YOffset != 0 {
		t.Fatalf("initial YOffset = %d, want 0", model.viewport.YOffset)
	}

	updatedModel, _ := model.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	got := updatedModel.(tuiModel)
	if got.viewport.YOffset <= 0 {
		t.Fatalf("YOffset after pgdown = %d, want > 0", got.viewport.YOffset)
	}
}

func TestTUIModelScrollsViewportOnArrowDown(t *testing.T) {
	model := newTUIModel(tuiModelOptions{
		Context: context.Background(),
		Client:  stubClient{},
	})
	model.width = 100
	model.height = 20
	for i := 0; i < 40; i++ {
		model.appendEntry(tuiEntry{
			Kind:  tuiEntryAssistant,
			Title: "Sesame",
			Body:  "line " + strings.Repeat("content ", 8),
		})
	}
	model.layout()
	model.viewport.GotoTop()

	updatedModel, _ := model.Update(tea.KeyMsg{Type: tea.KeyDown})
	got := updatedModel.(tuiModel)
	if got.viewport.YOffset <= 0 {
		t.Fatalf("YOffset after down = %d, want > 0", got.viewport.YOffset)
	}
}

func TestTUIModelInterruptsTurnOnEsc(t *testing.T) {
	called := 0
	model := newTUIModel(tuiModelOptions{
		Context:   context.Background(),
		SessionID: "sess_1",
		Client: stubClient{
			interruptTurn: func(context.Context, string) error {
				called++
				return nil
			},
		},
	})
	model.busy = true

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd == nil {
		t.Fatal("cmd = nil, want interrupt command")
	}

	msg := cmd()
	updated, _ = updated.(tuiModel).Update(msg)
	got := updated.(tuiModel)
	if called != 1 {
		t.Fatalf("interrupt calls = %d, want 1", called)
	}
	if len(got.entries) == 0 || got.entries[len(got.entries)-1].Body != "interrupt requested" {
		t.Fatalf("last entry = %+v, want interrupt requested notice", got.entries)
	}
}

func TestShouldUseTUIAltScreenDefaultsToTrue(t *testing.T) {
	if !shouldUseTUIAltScreen(func(string) (string, bool) { return "", false }) {
		t.Fatal("shouldUseTUIAltScreen() = false, want true when ZELLIJ is unset")
	}
}

func TestShouldUseTUIAltScreenDisablesForZellij(t *testing.T) {
	if shouldUseTUIAltScreen(func(key string) (string, bool) {
		if key == "ZELLIJ" {
			return "0", true
		}
		return "", false
	}) {
		t.Fatal("shouldUseTUIAltScreen() = true, want false when ZELLIJ is set")
	}
}

func TestTUIModelCoalescesRepeatedTaskPolling(t *testing.T) {
	model := newTUIModel(tuiModelOptions{
		Context: context.Background(),
		Client:  stubClient{},
	})

	model.upsertToolEntry(types.ToolEventPayload{
		ToolCallID: "call_1",
		ToolName:   "task_get",
		Arguments:  `{"task_id":"task_123"}`,
	}, false)
	model.upsertToolEntry(types.ToolEventPayload{
		ToolCallID:    "call_1",
		ToolName:      "task_get",
		Arguments:     `{"task_id":"task_123"}`,
		ResultPreview: "Task task_123 (running)",
	}, true)
	model.upsertToolEntry(types.ToolEventPayload{
		ToolCallID: "call_2",
		ToolName:   "shell_command",
		Arguments:  `{"command":"sleep 5"}`,
	}, false)
	model.upsertToolEntry(types.ToolEventPayload{
		ToolCallID:    "call_2",
		ToolName:      "shell_command",
		Arguments:     `{"command":"sleep 5"}`,
		ResultPreview: "slept",
	}, true)
	model.upsertToolEntry(types.ToolEventPayload{
		ToolCallID: "call_3",
		ToolName:   "task_get",
		Arguments:  `{"task_id":"task_123"}`,
	}, false)
	model.upsertToolEntry(types.ToolEventPayload{
		ToolCallID:    "call_3",
		ToolName:      "task_get",
		Arguments:     `{"task_id":"task_123"}`,
		ResultPreview: "Task task_123 (completed)",
	}, true)

	if len(model.entries) != 2 {
		t.Fatalf("len(entries) = %d, want 2", len(model.entries))
	}
	if model.entries[0].Title != "task status" {
		t.Fatalf("entries[0].Title = %q, want task status", model.entries[0].Title)
	}
	if model.entries[0].Body != "task_123 (completed)" {
		t.Fatalf("entries[0].Body = %q, want task_123 (completed)", model.entries[0].Body)
	}
	if model.entries[1].Title != "shell" {
		t.Fatalf("entries[1].Title = %q, want shell", model.entries[1].Title)
	}
}

func TestRenderTUIEntryActivityWithoutBody(t *testing.T) {
	got := renderTUIEntry(tuiEntry{
		Kind:  tuiEntryActivity,
		Title: "task_123",
	}, 80)
	if strings.Contains(got, "\n") {
		t.Fatalf("renderTUIEntry() = %q, want single-line activity", got)
	}
}

func TestToolEntryStatusLabelUsesTaskStateForTaskStatusRows(t *testing.T) {
	status := toolEntryStatusLabel(tuiEntry{
		Kind:   tuiEntryTool,
		Title:  "task status",
		Body:   "task_123 (running)",
		Status: "completed",
	})
	if status != "…" {
		t.Fatalf("toolEntryStatusLabel() = %q, want ellipsis for running task", status)
	}
}

type stubClient struct {
	status           func(context.Context) (client.StatusResponse, error)
	listSessions     func(context.Context) (types.ListSessionsResponse, error)
	selectSession    func(context.Context, string) error
	submitTurn       func(context.Context, string, types.SubmitTurnRequest) (types.Turn, error)
	interruptTurn    func(context.Context, string) error
	decidePermission func(context.Context, types.PermissionDecisionRequest) (types.PermissionDecisionResponse, error)
	streamEvents     func(context.Context, string, int64) (<-chan types.Event, error)
	getTimeline      func(context.Context, string) (types.SessionTimelineResponse, error)
	getMailbox       func(context.Context, string) (types.SessionReportMailboxResponse, error)
	getRuntimeGraph  func(context.Context, string) (types.SessionRuntimeGraphResponse, error)
	getReporting     func(context.Context, string) (types.ReportingOverview, error)
	listCronJobs     func(context.Context, string) (types.ListScheduledJobsResponse, error)
	getCronJob       func(context.Context, string) (types.ScheduledJob, error)
	pauseCronJob     func(context.Context, string) (types.ScheduledJob, error)
	resumeCronJob    func(context.Context, string) (types.ScheduledJob, error)
	deleteCronJob    func(context.Context, string) error
}

func (s stubClient) Status(ctx context.Context) (client.StatusResponse, error) {
	if s.status == nil {
		return client.StatusResponse{}, nil
	}
	return s.status(ctx)
}

func (s stubClient) ListSessions(ctx context.Context) (types.ListSessionsResponse, error) {
	if s.listSessions == nil {
		return types.ListSessionsResponse{}, nil
	}
	return s.listSessions(ctx)
}

func (s stubClient) SelectSession(ctx context.Context, sessionID string) error {
	if s.selectSession == nil {
		return nil
	}
	return s.selectSession(ctx, sessionID)
}

func (s stubClient) SubmitTurn(ctx context.Context, sessionID string, req types.SubmitTurnRequest) (types.Turn, error) {
	if s.submitTurn == nil {
		return types.Turn{}, nil
	}
	return s.submitTurn(ctx, sessionID, req)
}

func (s stubClient) InterruptTurn(ctx context.Context, sessionID string) error {
	if s.interruptTurn == nil {
		return nil
	}
	return s.interruptTurn(ctx, sessionID)
}

func (s stubClient) DecidePermission(ctx context.Context, req types.PermissionDecisionRequest) (types.PermissionDecisionResponse, error) {
	if s.decidePermission == nil {
		return types.PermissionDecisionResponse{}, nil
	}
	return s.decidePermission(ctx, req)
}

func (s stubClient) StreamEvents(ctx context.Context, sessionID string, afterSeq int64) (<-chan types.Event, error) {
	if s.streamEvents == nil {
		ch := make(chan types.Event)
		close(ch)
		return ch, nil
	}
	return s.streamEvents(ctx, sessionID, afterSeq)
}

func (s stubClient) GetTimeline(ctx context.Context, sessionID string) (types.SessionTimelineResponse, error) {
	if s.getTimeline == nil {
		return types.SessionTimelineResponse{}, nil
	}
	return s.getTimeline(ctx, sessionID)
}

func (s stubClient) GetReportMailbox(ctx context.Context, sessionID string) (types.SessionReportMailboxResponse, error) {
	if s.getMailbox == nil {
		return types.SessionReportMailboxResponse{}, nil
	}
	return s.getMailbox(ctx, sessionID)
}

func (s stubClient) GetRuntimeGraph(ctx context.Context, sessionID string) (types.SessionRuntimeGraphResponse, error) {
	if s.getRuntimeGraph == nil {
		return types.SessionRuntimeGraphResponse{}, nil
	}
	return s.getRuntimeGraph(ctx, sessionID)
}

func (s stubClient) GetReportingOverview(ctx context.Context, sessionID string) (types.ReportingOverview, error) {
	if s.getReporting == nil {
		return types.ReportingOverview{}, nil
	}
	return s.getReporting(ctx, sessionID)
}

func (s stubClient) ListCronJobs(ctx context.Context, workspaceRoot string) (types.ListScheduledJobsResponse, error) {
	if s.listCronJobs == nil {
		return types.ListScheduledJobsResponse{}, nil
	}
	return s.listCronJobs(ctx, workspaceRoot)
}

func (s stubClient) GetCronJob(ctx context.Context, jobID string) (types.ScheduledJob, error) {
	if s.getCronJob == nil {
		return types.ScheduledJob{}, nil
	}
	return s.getCronJob(ctx, jobID)
}

func (s stubClient) PauseCronJob(ctx context.Context, jobID string) (types.ScheduledJob, error) {
	if s.pauseCronJob == nil {
		return types.ScheduledJob{}, nil
	}
	return s.pauseCronJob(ctx, jobID)
}

func (s stubClient) ResumeCronJob(ctx context.Context, jobID string) (types.ScheduledJob, error) {
	if s.resumeCronJob == nil {
		return types.ScheduledJob{}, nil
	}
	return s.resumeCronJob(ctx, jobID)
}

func (s stubClient) DeleteCronJob(ctx context.Context, jobID string) error {
	if s.deleteCronJob == nil {
		return nil
	}
	return s.deleteCronJob(ctx, jobID)
}
