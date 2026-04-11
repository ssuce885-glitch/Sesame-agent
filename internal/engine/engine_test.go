package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	contextstate "go-agent/internal/context"
	"go-agent/internal/model"
	"go-agent/internal/permissions"
	"go-agent/internal/runtimegraph"
	"go-agent/internal/scheduler"
	"go-agent/internal/store/sqlite"
	"go-agent/internal/task"
	"go-agent/internal/tools"
	"go-agent/internal/types"
)

func TestRunTurnStreamsAssistantEventsIntoSink(t *testing.T) {
	fakeModel := model.NewFakeStreaming([][]model.StreamEvent{
		{
			{Kind: model.StreamEventTextDelta, TextDelta: "Hel"},
			{Kind: model.StreamEventTextDelta, TextDelta: "lo"},
			{Kind: model.StreamEventMessageEnd},
		},
	})
	sink := &recordingSink{}

	runner := New(fakeModel, tools.NewRegistry(), permissions.NewEngine(), nil, nil, nil, 0)
	err := runner.RunTurn(context.Background(), Input{
		Session: types.Session{ID: "sess_1", WorkspaceRoot: t.TempDir()},
		Turn:    types.Turn{ID: "turn_1", UserMessage: "inspect the readme"},
		Sink:    sink,
	})
	if err != nil {
		t.Fatalf("RunTurn() error = %v", err)
	}

	want := []string{
		types.EventTurnStarted,
		types.EventAssistantStarted,
		types.EventAssistantDelta,
		types.EventAssistantDelta,
		types.EventAssistantCompleted,
		types.EventTurnCompleted,
	}
	assertEventTypes(t, sink, want)
}

func TestRunTurnExposesOnlyVisibleToolsForPermissionProfile(t *testing.T) {
	fakeModel := model.NewFakeStreaming([][]model.StreamEvent{
		{
			{Kind: model.StreamEventTextDelta, TextDelta: "ok"},
			{Kind: model.StreamEventMessageEnd},
		},
	})
	runner := New(fakeModel, tools.NewRegistry(), permissions.NewEngine(), nil, nil, nil, 0)

	err := runner.RunTurn(context.Background(), Input{
		Session: types.Session{ID: "sess_visible_tools", WorkspaceRoot: t.TempDir()},
		Turn:    types.Turn{ID: "turn_visible_tools", SessionID: "sess_visible_tools", UserMessage: "hello"},
		Sink:    &recordingSink{},
	})
	if err != nil {
		t.Fatalf("RunTurn() error = %v", err)
	}

	req := fakeModel.LastRequest()
	gotNames := make([]string, 0, len(req.Tools))
	for _, tool := range req.Tools {
		gotNames = append(gotNames, tool.Name)
	}

	wantNames := []string{"file_read", "glob", "grep", "list_dir", "request_permissions", "request_user_input", "skill_use", "view_image", "web_fetch"}
	if !reflect.DeepEqual(gotNames, wantNames) {
		t.Fatalf("request tool names = %v, want %v", gotNames, wantNames)
	}
}

func TestRunTurnInterruptsWhenRequestUserInputToolIsCalled(t *testing.T) {
	client := model.NewFakeStreaming([][]model.StreamEvent{
		{
			{Kind: model.StreamEventToolCallEnd, ToolCall: model.ToolCallChunk{
				ID:   "call_1",
				Name: "request_user_input",
				Input: map[string]any{
					"questions": []any{
						map[string]any{
							"id":       "mode",
							"header":   "Decision",
							"question": "Should I proceed with the faster path?",
							"options": []any{
								map[string]any{"label": "Yes (Recommended)", "description": "Proceed now."},
								map[string]any{"label": "No", "description": "Choose the safer path."},
							},
						},
					},
				},
			}},
			{Kind: model.StreamEventMessageEnd},
		},
	})
	sink := &recordingSink{}
	store := &fakeConversationStore{}
	runner := New(client, tools.NewRegistry(), permissions.NewEngine(), store, contextstate.NewManager(contextstate.Config{
		MaxRecentItems:      8,
		MaxEstimatedTokens:  6000,
		CompactionThreshold: 16,
	}), nil, 8)

	err := runner.RunTurn(context.Background(), Input{
		Session: types.Session{ID: "sess_interrupt", WorkspaceRoot: t.TempDir()},
		Turn:    types.Turn{ID: "turn_interrupt", SessionID: "sess_interrupt", UserMessage: "continue"},
		Sink:    sink,
	})
	if err != nil {
		t.Fatalf("RunTurn() error = %v", err)
	}

	assertEventTypes(t, sink, []string{
		types.EventTurnStarted,
		types.EventToolStarted,
		types.EventToolCompleted,
		types.EventSystemNotice,
		types.EventTurnInterrupted,
	})
	if len(client.LastRequest().Items) == 0 && len(store.insertedItems) == 0 {
		t.Fatal("expected conversation items to be persisted before interruption")
	}
	if len(client.LastRequest().Tools) == 0 {
		t.Fatal("expected tool definitions in first request")
	}
	if len(client.LastRequest().ToolResults) != 0 {
		t.Fatalf("first request tool results = %d, want 0", len(client.LastRequest().ToolResults))
	}
	if len(store.insertedItems) < 2 {
		t.Fatalf("len(store.insertedItems) = %d, want at least user + tool_result", len(store.insertedItems))
	}
	last := store.insertedItems[len(store.insertedItems)-1]
	if last.Kind != model.ConversationItemToolResult || last.Result == nil {
		t.Fatalf("last inserted item = %#v, want tool result", last)
	}
	if !strings.Contains(last.Result.Content, "paused the current turn") {
		t.Fatalf("tool result content = %q, want pause guidance", last.Result.Content)
	}
}

func TestRunTurnEmitsPermissionRequestedWhenRequestPermissionsToolIsCalled(t *testing.T) {
	client := model.NewFakeStreaming([][]model.StreamEvent{
		{
			{Kind: model.StreamEventToolCallEnd, ToolCall: model.ToolCallChunk{
				ID:   "call_1",
				Name: "request_permissions",
				Input: map[string]any{
					"profile": "trusted_local",
					"reason":  "Need shell access to inspect the workspace.",
				},
			}},
			{Kind: model.StreamEventMessageEnd},
		},
	})
	sink := &recordingSink{}
	runner := New(client, tools.NewRegistry(), permissions.NewEngine(), &fakeConversationStore{}, contextstate.NewManager(contextstate.Config{
		MaxRecentItems:      8,
		MaxEstimatedTokens:  6000,
		CompactionThreshold: 16,
	}), nil, 8)

	err := runner.RunTurn(context.Background(), Input{
		Session: types.Session{ID: "sess_perm", WorkspaceRoot: t.TempDir()},
		Turn:    types.Turn{ID: "turn_perm", SessionID: "sess_perm", UserMessage: "continue"},
		Sink:    sink,
	})
	if err != nil {
		t.Fatalf("RunTurn() error = %v", err)
	}

	assertEventTypes(t, sink, []string{
		types.EventTurnStarted,
		types.EventToolStarted,
		types.EventToolCompleted,
		types.EventPermissionRequested,
		types.EventSystemNotice,
		types.EventTurnInterrupted,
	})
}

func TestRunTurnEmitsPermissionRequestedWhenShellCommandNeedsApproval(t *testing.T) {
	client := model.NewFakeStreaming([][]model.StreamEvent{
		{
			{Kind: model.StreamEventToolCallEnd, ToolCall: model.ToolCallChunk{
				ID:   "call_shell_1",
				Name: "shell_command",
				Input: map[string]any{
					"command": "del important.txt",
				},
			}},
			{Kind: model.StreamEventMessageEnd},
		},
	})
	sink := &recordingSink{}
	runner := New(client, tools.NewRegistry(), permissions.NewEngine("trusted_local"), &fakeConversationStore{}, contextstate.NewManager(contextstate.Config{
		MaxRecentItems:      8,
		MaxEstimatedTokens:  6000,
		CompactionThreshold: 16,
	}), nil, 8)

	err := runner.RunTurn(context.Background(), Input{
		Session: types.Session{ID: "sess_perm_shell", WorkspaceRoot: t.TempDir()},
		Turn:    types.Turn{ID: "turn_perm_shell", SessionID: "sess_perm_shell", UserMessage: "continue"},
		Sink:    sink,
	})
	if err != nil {
		t.Fatalf("RunTurn() error = %v", err)
	}

	assertEventTypes(t, sink, []string{
		types.EventTurnStarted,
		types.EventToolStarted,
		types.EventToolCompleted,
		types.EventPermissionRequested,
		types.EventSystemNotice,
		types.EventTurnInterrupted,
	})
}

func TestRunTurnInjectsPendingTaskCompletionNotices(t *testing.T) {
	client := model.NewFakeStreaming([][]model.StreamEvent{
		{
			{Kind: model.StreamEventTextDelta, TextDelta: "received"},
			{Kind: model.StreamEventMessageEnd},
		},
	})
	store := &fakeConversationStore{
		pendingTaskCompletions: []types.PendingTaskCompletion{
			{
				ID:            "task_child_1",
				SessionID:     "sess_completion",
				ParentTurnID:  "turn_parent",
				TaskID:        "task_child_1",
				TaskType:      "agent",
				ResultKind:    "assistant_text",
				ResultText:    "subtask finished with final answer",
				ResultPreview: "subtask finished with final answer",
				ObservedAt:    time.Date(2026, 4, 10, 8, 30, 0, 0, time.UTC),
			},
		},
	}
	sink := &recordingSink{}
	runner := New(
		client,
		tools.NewRegistry(),
		permissions.NewEngine(),
		store,
		contextstate.NewManager(contextstate.Config{
			MaxRecentItems:      8,
			MaxEstimatedTokens:  6000,
			CompactionThreshold: 16,
		}),
		nil,
		8,
	)

	err := runner.RunTurn(context.Background(), Input{
		Session: types.Session{ID: "sess_completion", WorkspaceRoot: t.TempDir()},
		Turn:    types.Turn{ID: "turn_child_delivery", SessionID: "sess_completion", UserMessage: "继续"},
		Sink:    sink,
	})
	if err != nil {
		t.Fatalf("RunTurn() error = %v", err)
	}

	req := client.LastRequest()
	if !strings.Contains(req.Instructions, "Pending child task completions:") {
		t.Fatalf("request instructions = %q, want completion prompt section", req.Instructions)
	}
	if !strings.Contains(req.Instructions, "task_child_1") || !strings.Contains(req.Instructions, "task_result") {
		t.Fatalf("request instructions = %q, want task id and task_result guidance", req.Instructions)
	}
	if store.claimedCompletionSession != "sess_completion" || store.claimedCompletionTurn != "turn_child_delivery" {
		t.Fatalf("claimed completion = %s/%s, want sess_completion/turn_child_delivery", store.claimedCompletionSession, store.claimedCompletionTurn)
	}
	assertHasEventType(t, sink, types.EventSystemNotice)
}

func TestRunTurnInjectsPendingReportMailboxItemsIntoInstructionsOnly(t *testing.T) {
	client := model.NewFakeStreaming([][]model.StreamEvent{
		{
			{Kind: model.StreamEventTextDelta, TextDelta: "received"},
			{Kind: model.StreamEventMessageEnd},
		},
	})
	store := &fakeConversationStore{
		pendingReportMailboxItems: []types.ReportMailboxItem{
			{
				ID:         "task_result:task_report_1",
				SessionID:  "sess_report_mailbox",
				SourceKind: types.ReportMailboxSourceTaskResult,
				SourceID:   "task_report_1",
				ObservedAt: time.Date(2026, 4, 10, 8, 45, 0, 0, time.UTC),
				Envelope: types.ReportEnvelope{
					Source:  "task_result",
					Status:  "completed",
					Title:   "Shanghai weather",
					Summary: "22C, cloudy",
					Sections: []types.ReportSectionContent{{
						ID:    "body",
						Title: "Report",
						Text:  "22C, cloudy, humidity 61%",
					}},
				},
			},
		},
	}
	sink := &recordingSink{}
	runner := New(
		client,
		tools.NewRegistry(),
		permissions.NewEngine(),
		store,
		contextstate.NewManager(contextstate.Config{
			MaxRecentItems:      8,
			MaxEstimatedTokens:  6000,
			CompactionThreshold: 16,
		}),
		nil,
		8,
	)

	err := runner.RunTurn(context.Background(), Input{
		Session: types.Session{ID: "sess_report_mailbox", WorkspaceRoot: t.TempDir()},
		Turn:    types.Turn{ID: "turn_report_delivery", SessionID: "sess_report_mailbox", UserMessage: "继续"},
		Sink:    sink,
	})
	if err != nil {
		t.Fatalf("RunTurn() error = %v", err)
	}

	req := client.LastRequest()
	if !strings.Contains(req.Instructions, "Pending reports delivered for this turn:") {
		t.Fatalf("request instructions = %q, want report mailbox prompt section", req.Instructions)
	}
	if !strings.Contains(req.Instructions, "Shanghai weather") || !strings.Contains(req.Instructions, "22C, cloudy") {
		t.Fatalf("request instructions = %q, want report title and summary", req.Instructions)
	}
	if store.claimedReportSession != "sess_report_mailbox" || store.claimedReportTurn != "turn_report_delivery" {
		t.Fatalf("claimed report mailbox = %s/%s, want sess_report_mailbox/turn_report_delivery", store.claimedReportSession, store.claimedReportTurn)
	}
	assertNoEventType(t, sink, types.EventSystemNotice)
}

func TestRunTurnEmitsFailedWithoutCompletedWhenStreamErrorsAfterMessageEnd(t *testing.T) {
	streamErr := errors.New("stream failed after message end")
	sink := &recordingSink{}
	runner := New(scriptedStreamingClient{
		events: []model.StreamEvent{
			{Kind: model.StreamEventTextDelta, TextDelta: "Hello"},
			{Kind: model.StreamEventMessageEnd},
		},
		err: streamErr,
	}, tools.NewRegistry(), permissions.NewEngine(), nil, nil, nil, 0)

	err := runner.RunTurn(context.Background(), Input{
		Session: types.Session{ID: "sess_1", WorkspaceRoot: t.TempDir()},
		Turn:    types.Turn{ID: "turn_1", UserMessage: "hello"},
		Sink:    sink,
	})
	if !errors.Is(err, streamErr) {
		t.Fatalf("RunTurn() error = %v, want %v", err, streamErr)
	}

	assertEventTypes(t, sink, []string{
		types.EventTurnStarted,
		types.EventAssistantStarted,
		types.EventAssistantDelta,
		types.EventTurnFailed,
	})
	assertNoEventType(t, sink, types.EventAssistantCompleted)
	assertNoEventType(t, sink, types.EventTurnCompleted)
}

func TestRunTurnEmitsTurnFailedWhenToolExecutionFails(t *testing.T) {
	sink := &recordingSink{}
	runner := New(model.NewFakeStreaming([][]model.StreamEvent{
		{
			{Kind: model.StreamEventToolCallEnd, ToolCall: model.ToolCallChunk{
				ID:    "call_1",
				Name:  "missing_tool",
				Input: map[string]any{},
			}},
			{Kind: model.StreamEventMessageEnd},
		},
		// second stream: model responds after receiving the error tool result
		{
			{Kind: model.StreamEventTextDelta, TextDelta: "done"},
			{Kind: model.StreamEventMessageEnd},
		},
	}), tools.NewRegistry(), permissions.NewEngine(), nil, nil, nil, 0)

	err := runner.RunTurn(context.Background(), Input{
		Session: types.Session{ID: "sess_1", WorkspaceRoot: t.TempDir()},
		Turn:    types.Turn{ID: "turn_1", UserMessage: "hello"},
		Sink:    sink,
	})
	if err != nil {
		t.Fatalf("RunTurn() error = %v, want nil", err)
	}

	assertEventTypes(t, sink, []string{
		types.EventTurnStarted,
		types.EventToolStarted,
		types.EventToolCompleted,
		types.EventAssistantStarted,
		types.EventAssistantDelta,
		types.EventAssistantCompleted,
		types.EventTurnCompleted,
	})
}

func TestRunTurnExecutesToolAfterToolCallEnd(t *testing.T) {
	workspace := t.TempDir()
	readmePath := filepath.Join(workspace, "README.md")
	if err := os.WriteFile(readmePath, []byte("hello from readme"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	modelClient := &recordingStreamingClient{
		streams: [][]model.StreamEvent{
			{
				{Kind: model.StreamEventTextDelta, TextDelta: "Let me check the README."},
				{Kind: model.StreamEventToolCallStart, ToolCall: model.ToolCallChunk{ID: "call_1", Name: "file_read"}},
				{Kind: model.StreamEventToolCallDelta, ToolCall: model.ToolCallChunk{ID: "call_1", Name: "file_read", InputChunk: `{"path":"`}},
				{Kind: model.StreamEventToolCallEnd, ToolCall: model.ToolCallChunk{
					ID:    "call_1",
					Name:  "file_read",
					Input: map[string]any{"path": readmePath},
				}},
				{Kind: model.StreamEventMessageEnd},
			},
			{
				{Kind: model.StreamEventTextDelta, TextDelta: "README says hello from readme"},
				{Kind: model.StreamEventMessageEnd},
			},
		},
	}
	sink := &recordingSink{}
	store := &fakeConversationStore{}
	manager := contextstate.NewManager(contextstate.Config{
		MaxRecentItems:      8,
		MaxEstimatedTokens:  6000,
		CompactionThreshold: 16,
	})
	runner := New(modelClient, tools.NewRegistry(), permissions.NewEngine(), store, manager, nil, 8)

	err := runner.RunTurn(context.Background(), Input{
		Session: types.Session{ID: "sess_1", WorkspaceRoot: workspace},
		Turn:    types.Turn{ID: "turn_1", SessionID: "sess_1", UserMessage: "inspect the readme"},
		Sink:    sink,
	})
	if err != nil {
		t.Fatalf("RunTurn() error = %v", err)
	}

	assertEventTypes(t, sink, []string{
		types.EventTurnStarted,
		types.EventAssistantStarted,
		types.EventAssistantDelta,
		types.EventToolStarted,
		types.EventToolCompleted,
		types.EventAssistantDelta,
		types.EventAssistantCompleted,
		types.EventTurnCompleted,
	})

	if len(modelClient.requests) != 2 {
		t.Fatalf("len(requests) = %d, want 2", len(modelClient.requests))
	}

	first := modelClient.requests[0]
	if first.UserMessage != "inspect the readme" {
		t.Fatalf("first request message = %q, want %q", first.UserMessage, "inspect the readme")
	}
	if len(first.ToolResults) != 0 {
		t.Fatalf("len(first request tool results) = %d, want 0", len(first.ToolResults))
	}

	second := modelClient.requests[1]
	if second.UserMessage != "inspect the readme" {
		t.Fatalf("second request message = %q, want %q", second.UserMessage, "inspect the readme")
	}
	if len(second.Items) < 3 {
		t.Fatalf("len(second request items) = %d, want at least 3", len(second.Items))
	}
	foundUser := false
	foundToolResult := false
	for _, item := range second.Items {
		if item.Kind == model.ConversationItemUserMessage && item.Text == "inspect the readme" {
			foundUser = true
		}
		if item.Kind == model.ConversationItemToolResult {
			foundToolResult = true
			if item.Result == nil || item.Result.ToolCallID != "call_1" {
				t.Fatalf("tool result item = %#v, want call_1", item)
			}
		}
	}
	if !foundUser {
		t.Fatalf("second request items = %#v, want current user item", second.Items)
	}
	if !foundToolResult {
		t.Fatalf("second request items = %#v, want tool_result item", second.Items)
	}
	if len(second.ToolResults) != 1 {
		t.Fatalf("len(second request tool results) = %d, want 1", len(second.ToolResults))
	}

	got := second.ToolResults[0]
	if got.ToolCallID != "call_1" {
		t.Fatalf("tool result call id = %q, want %q", got.ToolCallID, "call_1")
	}
	if got.ToolName != "file_read" {
		t.Fatalf("tool result name = %q, want %q", got.ToolName, "file_read")
	}
	if got.Content != "hello from readme" {
		t.Fatalf("tool result content = %q, want %q", got.Content, "hello from readme")
	}
	if !strings.Contains(got.StructuredJSON, "\"path\"") {
		t.Fatalf("tool result structured_json = %q, want file_read structured payload", got.StructuredJSON)
	}
	if got.IsError {
		t.Fatal("tool result is_error = true, want false")
	}
}

func TestRunTurnPassesTaskManagerIntoToolExecContext(t *testing.T) {
	capturingTool := &taskManagerCapturingTool{}
	registry := tools.NewRegistry()
	registry.Register(capturingTool)

	manager := task.NewManager(task.Config{MaxConcurrentTasks: 8, TaskOutputMaxBytes: 1 << 20}, nil, nil)
	runner := New(model.NewFakeStreaming([][]model.StreamEvent{
		{
			{Kind: model.StreamEventToolCallEnd, ToolCall: model.ToolCallChunk{
				ID:    "call_1",
				Name:  "glob",
				Input: map[string]any{"pattern": "*"},
			}},
			{Kind: model.StreamEventMessageEnd},
		},
		{
			{Kind: model.StreamEventTextDelta, TextDelta: "captured"},
			{Kind: model.StreamEventMessageEnd},
		},
	}), registry, permissions.NewEngine(), nil, nil, nil, 8)
	runner.SetTaskManager(manager)

	err := runner.RunTurn(context.Background(), Input{
		Session: types.Session{ID: "sess_1", WorkspaceRoot: t.TempDir()},
		Turn:    types.Turn{ID: "turn_1", SessionID: "sess_1", UserMessage: "capture manager"},
		Sink:    &recordingSink{},
	})
	if err != nil {
		t.Fatalf("RunTurn() error = %v", err)
	}
	if capturingTool.gotManager != manager {
		t.Fatalf("tool exec manager = %p, want %p", capturingTool.gotManager, manager)
	}
}

func TestRunTurnSharesMutableTurnContextAcrossToolExecutions(t *testing.T) {
	store, err := sqlite.Open(filepath.Join(t.TempDir(), "agentd.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	service := runtimegraph.NewService(store)
	capturingTool := &runtimeContextCapturingTool{}
	registry := tools.NewRegistry()
	registry.Register(capturingTool)

	runner := New(model.NewFakeStreaming([][]model.StreamEvent{
		{
			{Kind: model.StreamEventToolCallEnd, ToolCall: model.ToolCallChunk{
				ID:    "call_1",
				Name:  "glob",
				Input: map[string]any{"pattern": "first"},
			}},
			{Kind: model.StreamEventMessageEnd},
		},
		{
			{Kind: model.StreamEventToolCallEnd, ToolCall: model.ToolCallChunk{
				ID:    "call_2",
				Name:  "glob",
				Input: map[string]any{"pattern": "second"},
			}},
			{Kind: model.StreamEventMessageEnd},
		},
		{
			{Kind: model.StreamEventTextDelta, TextDelta: "done"},
			{Kind: model.StreamEventMessageEnd},
		},
	}), registry, permissions.NewEngine(), nil, nil, nil, 8)
	runner.SetRuntimeService(service)

	err = runner.RunTurn(context.Background(), Input{
		Session: types.Session{ID: "sess_runtime_ctx", WorkspaceRoot: t.TempDir()},
		Turn:    types.Turn{ID: "turn_runtime_ctx", SessionID: "sess_runtime_ctx", UserMessage: "capture runtime context"},
		Sink:    &recordingSink{},
	})
	if err != nil {
		t.Fatalf("RunTurn() error = %v", err)
	}

	if len(capturingTool.turnContexts) != 2 {
		t.Fatalf("len(turnContexts) = %d, want 2", len(capturingTool.turnContexts))
	}
	if capturingTool.turnContexts[0] != capturingTool.turnContexts[1] {
		t.Fatal("turn contexts differ, want the same pointer shared across tool executions")
	}
	if capturingTool.turnContexts[0].CurrentSessionID != "sess_runtime_ctx" || capturingTool.turnContexts[0].CurrentTurnID != "turn_runtime_ctx" {
		t.Fatalf("turn context = %#v, want current session/turn ids populated", capturingTool.turnContexts[0])
	}
	if capturingTool.seenRunIDs[0] == "" || capturingTool.seenRunIDs[1] != "run_from_first" {
		t.Fatalf("seen run ids = %v, want lazy-created run id then propagated run_from_first", capturingTool.seenRunIDs)
	}
	if len(capturingTool.runtimeServices) != 2 || capturingTool.runtimeServices[0] != service || capturingTool.runtimeServices[1] != service {
		t.Fatalf("runtime services = %#v, want injected runtime service on both tool calls", capturingTool.runtimeServices)
	}
}

func TestRunTurnPersistsToolRunForNormalToolCalls(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("hello"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	store, err := sqlite.Open(filepath.Join(root, "agentd.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	runner := New(model.NewFakeStreaming([][]model.StreamEvent{
		{
			{Kind: model.StreamEventToolCallEnd, ToolCall: model.ToolCallChunk{
				ID:    "call_read_1",
				Name:  "file_read",
				Input: map[string]any{"path": "README.md"},
			}},
			{Kind: model.StreamEventMessageEnd},
		},
		{
			{Kind: model.StreamEventTextDelta, TextDelta: "done"},
			{Kind: model.StreamEventMessageEnd},
		},
	}), tools.NewRegistry(), permissions.NewEngine(), store, contextstate.NewManager(contextstate.Config{
		MaxRecentItems:      8,
		MaxEstimatedTokens:  6000,
		CompactionThreshold: 16,
	}), nil, 8)
	runner.SetRuntimeService(runtimegraph.NewService(store))

	err = runner.RunTurn(context.Background(), Input{
		Session: types.Session{ID: "sess_tool_run", WorkspaceRoot: root},
		Turn:    types.Turn{ID: "turn_tool_run", SessionID: "sess_tool_run", UserMessage: "read the readme"},
		Sink:    &recordingSink{},
	})
	if err != nil {
		t.Fatalf("RunTurn() error = %v", err)
	}

	graph, err := store.ListRuntimeGraph(context.Background())
	if err != nil {
		t.Fatalf("ListRuntimeGraph() error = %v", err)
	}
	if len(graph.ToolRuns) != 1 {
		t.Fatalf("len(graph.ToolRuns) = %d, want 1", len(graph.ToolRuns))
	}
	if graph.ToolRuns[0].RunID == "" {
		t.Fatal("tool run RunID = empty, want lazy-created runtime run")
	}
	if graph.ToolRuns[0].ToolCallID != "call_read_1" || graph.ToolRuns[0].ToolName != "file_read" {
		t.Fatalf("tool run = %#v, want call_read_1/file_read", graph.ToolRuns[0])
	}
}

func TestRunTurnBuildsProviderRequestFromStoredConversation(t *testing.T) {
	store := &fakeConversationStore{
		items: []model.ConversationItem{
			model.UserMessageItem("first request"),
			{Kind: model.ConversationItemAssistantText, Text: "first answer"},
		},
		summaries: []model.Summary{{
			RangeLabel:       "turns 1-1",
			ImportantChoices: []string{"used rg first"},
		}},
		memories: []types.MemoryEntry{{Content: "workspace prefers rg for searches"}},
	}
	client := model.NewFakeStreaming([][]model.StreamEvent{{
		{Kind: model.StreamEventTextDelta, TextDelta: "second answer"},
		{Kind: model.StreamEventMessageEnd},
	}})
	manager := contextstate.NewManager(contextstate.Config{
		MaxRecentItems:      1,
		MaxEstimatedTokens:  6000,
		CompactionThreshold: 16,
	})
	runner := NewWithRuntime(
		client,
		tools.NewRegistry(),
		permissions.NewEngine(),
		store,
		manager,
		contextstate.NewRuntime(86400, 3),
		nil,
		RuntimeMetadata{
			Provider: "openai_compatible",
			Model:    "glm-4.5",
		},
		8,
	)

	err := runner.RunTurn(context.Background(), Input{
		Session: types.Session{ID: "sess_1", WorkspaceRoot: t.TempDir()},
		Turn:    types.Turn{ID: "turn_2", SessionID: "sess_1", UserMessage: "workspace prefers rg for searches"},
		Sink:    &recordingSink{},
	})
	if err != nil {
		t.Fatalf("RunTurn() error = %v", err)
	}

	req := client.LastRequest()
	if len(req.Items) != 3 {
		t.Fatalf("len(req.Items) = %d, want 3", len(req.Items))
	}
	if !strings.Contains(req.Instructions, "workspace prefers rg for searches") {
		t.Fatalf("Instructions = %q, want recalled memory", req.Instructions)
	}
	if req.Items[0].Kind != model.ConversationItemSummary {
		t.Fatalf("first request item kind = %q, want %q", req.Items[0].Kind, model.ConversationItemSummary)
	}
	if req.Items[0].Summary == nil || req.Items[0].Summary.RangeLabel != "turns 1-1" {
		t.Fatalf("first summary item = %#v, want stored summary", req.Items[0])
	}
	if req.Items[1].Kind != model.ConversationItemAssistantText || req.Items[1].Text != "first answer" {
		t.Fatalf("second request item = %#v, want recent assistant item", req.Items[1])
	}
	if req.Items[2].Kind != model.ConversationItemUserMessage || req.Items[2].Text != "workspace prefers rg for searches" {
		t.Fatalf("last request item = %#v, want current user message", req.Items[2])
	}
	if len(store.insertedItems) != 2 {
		t.Fatalf("len(inserted items) = %d, want 2", len(store.insertedItems))
	}
	if store.insertedItems[0].Kind != model.ConversationItemUserMessage || store.insertedPositions[0] != 3 {
		t.Fatalf("first inserted item = %#v at %d, want current user at 3", store.insertedItems[0], store.insertedPositions[0])
	}
	if store.insertedItems[1].Kind != model.ConversationItemAssistantText || store.insertedPositions[1] != 4 {
		t.Fatalf("second inserted item = %#v at %d, want assistant at 4", store.insertedItems[1], store.insertedPositions[1])
	}
}

func TestRunTurnDoesNotInjectAmbientSkillSummaryForWebLookup(t *testing.T) {
	home := t.TempDir()
	workspace := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	writeFile(t, filepath.Join(home, ".sesame", "skills", "browser-helper", "SKILL.md"), `---
name: browser-helper
description: Open sites and click buttons.
policy:
  capability_tags:
    - browser_automation
---
which browser-cli
which playwright`)

	client := model.NewFakeStreaming([][]model.StreamEvent{
		{
			{Kind: model.StreamEventTextDelta, TextDelta: "ok"},
			{Kind: model.StreamEventMessageEnd},
		},
	})
	runner := New(client, tools.NewRegistry(), permissions.NewEngine(), nil, nil, nil, 0)

	err := runner.RunTurn(context.Background(), Input{
		Session: types.Session{ID: "sess_skill_dirs", WorkspaceRoot: workspace},
		Turn:    types.Turn{ID: "turn_skill_dirs", SessionID: "sess_skill_dirs", UserMessage: "帮我查今天热门新闻"},
		Sink:    &recordingSink{},
	})
	if err != nil {
		t.Fatalf("RunTurn() error = %v", err)
	}

	instructions := client.LastRequest().Instructions
	for _, unwanted := range []string{
		"Sesame skill directories:",
		"browser-helper",
		"which browser-cli",
		"which playwright",
		"source candidates only",
	} {
		if strings.Contains(instructions, unwanted) {
			t.Fatalf("Instructions = %q, do not want %q", instructions, unwanted)
		}
	}
	if !strings.Contains(instructions, "Profile: web_lookup") {
		t.Fatalf("Instructions = %q, want web_lookup profile", instructions)
	}
}

func TestRunTurnSelectsBrowserAutomationSkillByProfile(t *testing.T) {
	home := t.TempDir()
	workspace := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	writeFile(t, filepath.Join(home, ".sesame", "skills", "browser-helper", "SKILL.md"), `---
name: browser-helper
description: Open sites and click buttons.
policy:
  capability_tags:
    - browser_automation
---
Use the browser helper workflow.`)

	client := model.NewFakeStreaming([][]model.StreamEvent{
		{
			{Kind: model.StreamEventTextDelta, TextDelta: "ok"},
			{Kind: model.StreamEventMessageEnd},
		},
	})
	runner := New(client, tools.NewRegistry(), permissions.NewEngine(), nil, nil, nil, 0)

	err := runner.RunTurn(context.Background(), Input{
		Session: types.Session{ID: "sess_browser_skill", WorkspaceRoot: workspace},
		Turn:    types.Turn{ID: "turn_browser_skill", SessionID: "sess_browser_skill", UserMessage: "打开 https://example.com 并点击按钮"},
		Sink:    &recordingSink{},
	})
	if err != nil {
		t.Fatalf("RunTurn() error = %v", err)
	}

	instructions := client.LastRequest().Instructions
	for _, want := range []string{
		"Profile: browser_automation",
		"Use the browser helper workflow.",
	} {
		if !strings.Contains(instructions, want) {
			t.Fatalf("Instructions = %q, want %q", instructions, want)
		}
	}
}

func TestRunTurnSkillUseRecomputesVisibleToolsForNextRequest(t *testing.T) {
	home := t.TempDir()
	workspace := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	writeFile(t, filepath.Join(home, ".sesame", "skills", "send-email", "SKILL.md"), `---
name: send-email
description: Send emails via SMTP.
---
Run the local email sender script with shell_command.`)
	writeFile(t, filepath.Join(home, ".sesame", "config.json"), `{
  "skills": {
    "entries": {
      "send-email": {
        "enabled": true,
        "env": {
          "EMAIL_SENDER": "bot@example.com"
        }
      }
    }
  }
}`)

	client := &recordingStreamingClient{
		streams: [][]model.StreamEvent{
			{
				{Kind: model.StreamEventToolCallEnd, ToolCall: model.ToolCallChunk{
					ID:   "call_skill_1",
					Name: "skill_use",
					Input: map[string]any{
						"name": "send-email",
					},
				}},
				{Kind: model.StreamEventMessageEnd},
			},
			{
				{Kind: model.StreamEventToolCallEnd, ToolCall: model.ToolCallChunk{
					ID:   "call_shell_1",
					Name: "shell_command",
					Input: map[string]any{
						"command": "printf '%s' \"$EMAIL_SENDER\"",
					},
				}},
				{Kind: model.StreamEventMessageEnd},
			},
			{
				{Kind: model.StreamEventTextDelta, TextDelta: "ok"},
				{Kind: model.StreamEventMessageEnd},
			},
		},
	}
	runner := New(client, tools.NewRegistry(), permissions.NewEngine("trusted_local"), nil, nil, nil, 8)

	err := runner.RunTurn(context.Background(), Input{
		Session: types.Session{ID: "sess_skill_use", WorkspaceRoot: workspace},
		Turn:    types.Turn{ID: "turn_skill_use", SessionID: "sess_skill_use", UserMessage: "帮我查今天合肥天气"},
		Sink:    &recordingSink{},
	})
	if err != nil {
		t.Fatalf("RunTurn() error = %v", err)
	}
	if len(client.requests) != 3 {
		t.Fatalf("len(client.requests) = %d, want 3", len(client.requests))
	}

	firstNames := toolSchemaNames(client.requests[0].Tools)
	secondNames := toolSchemaNames(client.requests[1].Tools)
	if containsString(firstNames, "shell_command") {
		t.Fatalf("first request tools = %v, do not want shell_command before skill activation", firstNames)
	}
	if !containsString(firstNames, "skill_use") {
		t.Fatalf("first request tools = %v, want skill_use", firstNames)
	}
	if !containsString(secondNames, "shell_command") {
		t.Fatalf("second request tools = %v, want shell_command after skill activation", secondNames)
	}
	if len(client.requests[1].ToolResults) != 1 {
		t.Fatalf("len(second request tool results) = %d, want 1", len(client.requests[1].ToolResults))
	}
	if !strings.Contains(client.requests[1].ToolResults[0].Content, "Activated skill: send-email") {
		t.Fatalf("tool result content = %q, want activated skill guidance", client.requests[1].ToolResults[0].Content)
	}
	if len(client.requests[2].ToolResults) < 1 {
		t.Fatalf("len(third request tool results) = %d, want at least 1", len(client.requests[2].ToolResults))
	}
	lastToolResult := client.requests[2].ToolResults[len(client.requests[2].ToolResults)-1]
	if !strings.Contains(lastToolResult.Content, "bot@example.com") {
		t.Fatalf("shell tool result content = %q, want injected env value", lastToolResult.Content)
	}
}

func TestRunTurnPreselectsSendEmailSkillForCompoundTask(t *testing.T) {
	home := t.TempDir()
	workspace := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	writeFile(t, filepath.Join(home, ".sesame", "skills", "send-email", "SKILL.md"), `---
name: send-email
description: Send emails via SMTP.
---
Run the local email sender script with shell_command.`)

	client := model.NewFakeStreaming([][]model.StreamEvent{
		{
			{Kind: model.StreamEventTextDelta, TextDelta: "ok"},
			{Kind: model.StreamEventMessageEnd},
		},
	})
	runner := New(client, tools.NewRegistry(), permissions.NewEngine("trusted_local"), nil, nil, nil, 8)

	err := runner.RunTurn(context.Background(), Input{
		Session: types.Session{ID: "sess_preselect_email", WorkspaceRoot: workspace},
		Turn:    types.Turn{ID: "turn_preselect_email", SessionID: "sess_preselect_email", UserMessage: "帮我查今天合肥天气，然后发邮件给我"},
		Sink:    &recordingSink{},
	})
	if err != nil {
		t.Fatalf("RunTurn() error = %v", err)
	}

	req := client.LastRequest()
	toolNames := toolSchemaNames(req.Tools)
	if !containsString(toolNames, "shell_command") {
		t.Fatalf("request tools = %v, want shell_command from retrieved send-email skill", toolNames)
	}
	if !strings.Contains(req.Instructions, "send-email") {
		t.Fatalf("instructions = %q, want send-email activation", req.Instructions)
	}
}

func TestRunTurnPreselectsBrowserSkillWithCompactSummary(t *testing.T) {
	home := t.TempDir()
	workspace := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	writeFile(t, filepath.Join(home, ".sesame", "skills", "agent-browser", "SKILL.md"), `---
name: agent-browser
description: Browser automation helper for websites, forms, clicks, login, and screenshots.
policy:
  allow_implicit_activation: true
  allow_full_injection: false
allowed-tools:
  - shell_command
---
npm i -g agent-browser
agent-browser open https://example.com`)

	client := model.NewFakeStreaming([][]model.StreamEvent{
		{
			{Kind: model.StreamEventTextDelta, TextDelta: "ok"},
			{Kind: model.StreamEventMessageEnd},
		},
	})
	runner := New(client, tools.NewRegistry(), permissions.NewEngine("trusted_local"), nil, nil, nil, 8)

	err := runner.RunTurn(context.Background(), Input{
		Session: types.Session{ID: "sess_preselect_browser", WorkspaceRoot: workspace},
		Turn:    types.Turn{ID: "turn_preselect_browser", SessionID: "sess_preselect_browser", UserMessage: "打开 https://example.com 并点击登录按钮"},
		Sink:    &recordingSink{},
	})
	if err != nil {
		t.Fatalf("RunTurn() error = %v", err)
	}

	req := client.LastRequest()
	toolNames := toolSchemaNames(req.Tools)
	if !containsString(toolNames, "shell_command") {
		t.Fatalf("request tools = %v, want shell_command from retrieved browser skill", toolNames)
	}
	for _, want := range []string{
		"Profile: browser_automation",
		"Summary: Browser automation helper for websites, forms, clicks, login, and screenshots.",
		"Use `skill_use` with its name if you need the full local instructions.",
	} {
		if !strings.Contains(req.Instructions, want) {
			t.Fatalf("instructions = %q, want %q", req.Instructions, want)
		}
	}
	for _, unwanted := range []string{
		"npm i -g agent-browser",
		"agent-browser open https://example.com",
	} {
		if strings.Contains(req.Instructions, unwanted) {
			t.Fatalf("instructions = %q, do not want %q", req.Instructions, unwanted)
		}
	}
}

func TestRunTurnDoesNotPreselectExecutionSkillDuringSchedulingTurn(t *testing.T) {
	home := t.TempDir()
	workspace := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	writeFile(t, filepath.Join(home, ".sesame", "skills", "send-email", "SKILL.md"), `---
name: send-email
description: Send emails via SMTP.
---
Run the local email sender script with shell_command.`)

	client := model.NewFakeStreaming([][]model.StreamEvent{
		{
			{Kind: model.StreamEventTextDelta, TextDelta: "ok"},
			{Kind: model.StreamEventMessageEnd},
		},
	})
	runner := New(client, tools.NewRegistry(), permissions.NewEngine("trusted_local"), nil, nil, nil, 8)
	store, err := sqlite.Open(filepath.Join(t.TempDir(), "agentd.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()
	runner.SetSchedulerService(scheduler.NewService(store, nil))

	err = runner.RunTurn(context.Background(), Input{
		Session: types.Session{ID: "sess_scheduled_email", WorkspaceRoot: workspace},
		Turn:    types.Turn{ID: "turn_scheduled_email", SessionID: "sess_scheduled_email", UserMessage: "两分钟后给我发一封邮件，内容是今天合肥天气"},
		Sink:    &recordingSink{},
	})
	if err != nil {
		t.Fatalf("RunTurn() error = %v", err)
	}

	req := client.LastRequest()
	toolNames := toolSchemaNames(req.Tools)
	if containsString(toolNames, "shell_command") {
		t.Fatalf("request tools = %v, do not want shell_command during scheduling turn", toolNames)
	}
	if !reflect.DeepEqual(toolNames, []string{"schedule_report"}) {
		t.Fatalf("request tools = %v, want only schedule_report", toolNames)
	}
	if strings.Contains(req.Instructions, "send-email") {
		t.Fatalf("instructions = %q, do not want execution skill injected during scheduling turn", req.Instructions)
	}
}

func TestRunTurnInheritsActivatedSkillsForScheduledExecutionTurn(t *testing.T) {
	home := t.TempDir()
	workspace := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	writeFile(t, filepath.Join(home, ".sesame", "skills", "send-email", "SKILL.md"), `---
name: send-email
description: Send emails via SMTP.
---
Run the local email sender script with shell_command.`)

	client := model.NewFakeStreaming([][]model.StreamEvent{
		{
			{Kind: model.StreamEventTextDelta, TextDelta: "ok"},
			{Kind: model.StreamEventMessageEnd},
		},
	})
	runner := New(client, tools.NewRegistry(), permissions.NewEngine("trusted_local"), nil, nil, nil, 8)

	err := runner.RunTurn(context.Background(), Input{
		Session:             types.Session{ID: "sess_inherited_skill", WorkspaceRoot: workspace},
		Turn:                types.Turn{ID: "turn_inherited_skill", SessionID: "sess_inherited_skill", UserMessage: "帮我查今天合肥天气"},
		Sink:                &recordingSink{},
		ActivatedSkillNames: []string{"send-email"},
	})
	if err != nil {
		t.Fatalf("RunTurn() error = %v", err)
	}

	req := client.LastRequest()
	toolNames := toolSchemaNames(req.Tools)
	if !containsString(toolNames, "shell_command") {
		t.Fatalf("request tools = %v, want shell_command from inherited send-email skill", toolNames)
	}
	if !strings.Contains(req.Instructions, "send-email") {
		t.Fatalf("instructions = %q, want inherited send-email activation", req.Instructions)
	}
}

func TestRunTurnPersistsConversationItemsInStreamingOrderAcrossToolTurns(t *testing.T) {
	workspace := t.TempDir()
	readmePath := filepath.Join(workspace, "README.md")
	if err := os.WriteFile(readmePath, []byte("hello from readme"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	store := &fakeConversationStore{}
	client := model.NewFakeStreaming([][]model.StreamEvent{
		{
			{Kind: model.StreamEventTextDelta, TextDelta: "Let me check the README."},
			{Kind: model.StreamEventToolCallEnd, ToolCall: model.ToolCallChunk{
				ID:    "call_1",
				Name:  "file_read",
				Input: map[string]any{"path": readmePath},
			}},
			{Kind: model.StreamEventMessageEnd},
		},
		{
			{Kind: model.StreamEventTextDelta, TextDelta: "README says hello from readme"},
			{Kind: model.StreamEventMessageEnd},
		},
	})
	manager := contextstate.NewManager(contextstate.Config{
		MaxRecentItems:      8,
		MaxEstimatedTokens:  6000,
		CompactionThreshold: 16,
	})
	runner := NewWithRuntime(
		client,
		tools.NewRegistry(),
		permissions.NewEngine(),
		store,
		manager,
		contextstate.NewRuntime(86400, 3),
		nil,
		RuntimeMetadata{
			Provider: "openai_compatible",
			Model:    "glm-4.5",
		},
		8,
	)

	err := runner.RunTurn(context.Background(), Input{
		Session: types.Session{ID: "sess_1", WorkspaceRoot: workspace},
		Turn:    types.Turn{ID: "turn_1", SessionID: "sess_1", UserMessage: "inspect the readme"},
		Sink:    &recordingSink{},
	})
	if err != nil {
		t.Fatalf("RunTurn() error = %v", err)
	}

	if len(store.insertedItems) != 5 {
		t.Fatalf("len(inserted items) = %d, want 5", len(store.insertedItems))
	}
	wantKinds := []model.ConversationItemKind{
		model.ConversationItemUserMessage,
		model.ConversationItemAssistantText,
		model.ConversationItemToolCall,
		model.ConversationItemToolResult,
		model.ConversationItemAssistantText,
	}
	wantPositions := []int{1, 2, 3, 4, 5}
	for i := range wantKinds {
		if store.insertedItems[i].Kind != wantKinds[i] {
			t.Fatalf("inserted item kinds = %#v, want %#v", conversationItemKinds(store.insertedItems), wantKinds)
		}
		if store.insertedPositions[i] != wantPositions[i] {
			t.Fatalf("inserted positions = %v, want %v", store.insertedPositions, wantPositions)
		}
	}
}

func TestRunTurnPersistsAssistantSegmentsAndToolCallsInStreamOrder(t *testing.T) {
	workspace := t.TempDir()
	readmePath := filepath.Join(workspace, "README.md")
	if err := os.WriteFile(readmePath, []byte("hello from readme"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	store := &fakeConversationStore{}
	client := model.NewFakeStreaming([][]model.StreamEvent{
		{
			{Kind: model.StreamEventTextDelta, TextDelta: "Before tool. "},
			{Kind: model.StreamEventToolCallEnd, ToolCall: model.ToolCallChunk{
				ID:    "call_1",
				Name:  "file_read",
				Input: map[string]any{"path": readmePath},
			}},
			{Kind: model.StreamEventTextDelta, TextDelta: "After tool."},
			{Kind: model.StreamEventMessageEnd},
		},
		{
			{Kind: model.StreamEventTextDelta, TextDelta: "Final answer."},
			{Kind: model.StreamEventMessageEnd},
		},
	})
	manager := contextstate.NewManager(contextstate.Config{
		MaxRecentItems:      8,
		MaxEstimatedTokens:  6000,
		CompactionThreshold: 16,
	})
	runner := NewWithRuntime(
		client,
		tools.NewRegistry(),
		permissions.NewEngine(),
		store,
		manager,
		contextstate.NewRuntime(86400, 3),
		nil,
		RuntimeMetadata{
			Provider: "openai_compatible",
			Model:    "glm-4.5",
		},
		8,
	)

	err := runner.RunTurn(context.Background(), Input{
		Session: types.Session{ID: "sess_1", WorkspaceRoot: workspace},
		Turn:    types.Turn{ID: "turn_1", SessionID: "sess_1", UserMessage: "inspect the readme"},
		Sink:    &recordingSink{},
	})
	if err != nil {
		t.Fatalf("RunTurn() error = %v", err)
	}

	if len(store.insertedItems) != 6 {
		t.Fatalf("len(inserted items) = %d, want 6", len(store.insertedItems))
	}
	wantKinds := []model.ConversationItemKind{
		model.ConversationItemUserMessage,
		model.ConversationItemAssistantText,
		model.ConversationItemToolCall,
		model.ConversationItemToolResult,
		model.ConversationItemAssistantText,
		model.ConversationItemAssistantText,
	}
	wantTexts := []string{
		"",
		"Before tool. ",
		"",
		"",
		"After tool.",
		"Final answer.",
	}
	for i, wantKind := range wantKinds {
		if store.insertedItems[i].Kind != wantKind {
			t.Fatalf("inserted item kinds = %#v, want %#v", conversationItemKinds(store.insertedItems), wantKinds)
		}
		if wantTexts[i] != "" && store.insertedItems[i].Text != wantTexts[i] {
			t.Fatalf("inserted item %d text = %q, want %q", i, store.insertedItems[i].Text, wantTexts[i])
		}
	}
}

func TestRunTurnFailsWhenToolStepLimitIsExceeded(t *testing.T) {
	workspace := t.TempDir()
	readmePath := filepath.Join(workspace, "README.md")
	if err := os.WriteFile(readmePath, []byte("hello"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	store := &fakeConversationStore{}
	client := &recordingStreamingClient{
		streams: [][]model.StreamEvent{{
			{Kind: model.StreamEventToolCallEnd, ToolCall: model.ToolCallChunk{
				ID:    "call_1",
				Name:  "file_read",
				Input: map[string]any{"path": readmePath},
			}},
			{Kind: model.StreamEventToolCallEnd, ToolCall: model.ToolCallChunk{
				ID:    "call_2",
				Name:  "file_read",
				Input: map[string]any{"path": readmePath},
			}},
			{Kind: model.StreamEventMessageEnd},
		}},
	}
	sink := &recordingSink{}
	manager := contextstate.NewManager(contextstate.Config{
		MaxRecentItems:      8,
		MaxEstimatedTokens:  6000,
		CompactionThreshold: 16,
	})
	runner := New(client, tools.NewRegistry(), permissions.NewEngine(), store, manager, nil, 1)

	err := runner.RunTurn(context.Background(), Input{
		Session: types.Session{ID: "sess_1", WorkspaceRoot: workspace},
		Turn:    types.Turn{ID: "turn_1", SessionID: "sess_1", UserMessage: "inspect the readme"},
		Sink:    sink,
	})
	if err == nil || err.Error() != "turn exceeded max tool steps (1)" {
		t.Fatalf("RunTurn() error = %v, want max tool step error", err)
	}

	assertEventTypes(t, sink, []string{
		types.EventTurnStarted,
		types.EventToolStarted,
		types.EventToolCompleted,
		types.EventTurnFailed,
	})
	if len(client.requests) != 1 {
		t.Fatalf("len(requests) = %d, want 1", len(client.requests))
	}

	var persistedToolCalls []string
	for _, item := range store.insertedItems {
		if item.Kind == model.ConversationItemToolCall {
			persistedToolCalls = append(persistedToolCalls, item.ToolCall.ID)
		}
	}
	if !reflect.DeepEqual(persistedToolCalls, []string{"call_1"}) {
		t.Fatalf("persisted tool calls = %v, want only executed prefix [call_1]", persistedToolCalls)
	}
}

func TestRunTurnUsesStoredProviderCacheHeadAndPersistsNextHead(t *testing.T) {
	store := &fakeConversationStore{
		cacheHead: &types.ProviderCacheHead{
			SessionID:         "sess_1",
			Provider:          "openai_compatible",
			CapabilityProfile: "ark_responses",
			ActiveSessionRef:  "resp_prev",
			ActivePrefixRef:   "pref_prev",
			ActiveGeneration:  2,
			UpdatedAt:         time.Date(2026, 4, 4, 8, 0, 0, 0, time.UTC),
		},
	}
	client := &recordingStreamingClient{
		capabilities: model.ProviderCapabilities{
			Profile:              model.CapabilityProfileArkResponses,
			SupportsSessionCache: true,
			SupportsPrefixCache:  true,
			CachesToolResults:    true,
			RotatesSessionRef:    true,
		},
		streams: [][]model.StreamEvent{{
			{Kind: model.StreamEventResponseMetadata, ResponseMetadata: &model.ResponseMetadata{
				ResponseID:   "resp_next",
				InputTokens:  11,
				OutputTokens: 7,
				CachedTokens: 4,
			}},
			{Kind: model.StreamEventMessageEnd},
		}},
	}
	manager := contextstate.NewManager(contextstate.Config{
		MaxRecentItems:      8,
		MaxEstimatedTokens:  6000,
		CompactionThreshold: 16,
	})
	runner := NewWithRuntime(
		client,
		tools.NewRegistry(),
		permissions.NewEngine(),
		store,
		manager,
		contextstate.NewRuntime(86400, 3),
		nil,
		RuntimeMetadata{
			Provider: "openai_compatible",
			Model:    "glm-4.5",
		},
		8,
	)

	err := runner.RunTurn(context.Background(), Input{
		Session: types.Session{ID: "sess_1", WorkspaceRoot: t.TempDir()},
		Turn:    types.Turn{ID: "turn_1", SessionID: "sess_1", UserMessage: "hello"},
		Sink:    &recordingSink{},
	})
	if err != nil {
		t.Fatalf("RunTurn() error = %v", err)
	}

	if len(client.requests) != 1 {
		t.Fatalf("len(requests) = %d, want 1", len(client.requests))
	}
	if client.requests[0].Cache == nil {
		t.Fatal("request cache = nil, want stored cache head to be reused")
	}
	if client.requests[0].Cache.PreviousResponseID != "resp_prev" {
		t.Fatalf("request cache previous = %q, want %q", client.requests[0].Cache.PreviousResponseID, "resp_prev")
	}
	if len(client.requests[0].Items) != 1 {
		t.Fatalf("len(first request items) = %d, want 1 incremental user item", len(client.requests[0].Items))
	}
	if client.requests[0].Items[0].Kind != model.ConversationItemUserMessage || client.requests[0].Items[0].Text != "hello" {
		t.Fatalf("first request items = %#v, want only current user item", client.requests[0].Items)
	}
	if store.upsertedHead == nil {
		t.Fatal("upserted head = nil, want next cache head")
	}
	if store.upsertedHead.ActiveSessionRef != "resp_next" {
		t.Fatalf("upserted head session ref = %q, want %q", store.upsertedHead.ActiveSessionRef, "resp_next")
	}
	if store.upsertedHead.Provider != "openai_compatible" {
		t.Fatalf("upserted head provider = %q, want %q", store.upsertedHead.Provider, "openai_compatible")
	}
	if store.upsertedHead.CapabilityProfile != "ark_responses" {
		t.Fatalf("upserted head profile = %q, want %q", store.upsertedHead.CapabilityProfile, "ark_responses")
	}
}

func TestRunTurnUsesUpdatedCacheHeadWithinSameToolLoop(t *testing.T) {
	workspace := t.TempDir()
	readmePath := filepath.Join(workspace, "README.md")
	if err := os.WriteFile(readmePath, []byte("hello from readme"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	store := &fakeConversationStore{
		cacheHead: &types.ProviderCacheHead{
			SessionID:         "sess_1",
			Provider:          "openai_compatible",
			CapabilityProfile: "ark_responses",
			ActiveSessionRef:  "resp_prev",
			ActiveGeneration:  2,
			UpdatedAt:         time.Date(2026, 4, 4, 8, 0, 0, 0, time.UTC),
		},
	}
	client := &recordingStreamingClient{
		capabilities: model.ProviderCapabilities{
			Profile:              model.CapabilityProfileArkResponses,
			SupportsSessionCache: true,
			SupportsPrefixCache:  true,
			CachesToolResults:    true,
			RotatesSessionRef:    true,
		},
		streams: [][]model.StreamEvent{
			{
				{Kind: model.StreamEventResponseMetadata, ResponseMetadata: &model.ResponseMetadata{
					ResponseID:   "resp_next",
					InputTokens:  10,
					OutputTokens: 4,
					CachedTokens: 6,
				}},
				{Kind: model.StreamEventToolCallEnd, ToolCall: model.ToolCallChunk{
					ID:    "call_1",
					Name:  "file_read",
					Input: map[string]any{"path": readmePath},
				}},
				{Kind: model.StreamEventMessageEnd},
			},
			{
				{Kind: model.StreamEventMessageEnd},
			},
		},
	}
	manager := contextstate.NewManager(contextstate.Config{
		MaxRecentItems:      8,
		MaxEstimatedTokens:  6000,
		CompactionThreshold: 16,
	})
	runner := New(client, tools.NewRegistry(), permissions.NewEngine(), store, manager, nil, 8)

	err := runner.RunTurn(context.Background(), Input{
		Session: types.Session{ID: "sess_1", WorkspaceRoot: workspace},
		Turn:    types.Turn{ID: "turn_1", SessionID: "sess_1", UserMessage: "inspect the readme"},
		Sink:    &recordingSink{},
	})
	if err != nil {
		t.Fatalf("RunTurn() error = %v", err)
	}

	if len(client.requests) != 2 {
		t.Fatalf("len(requests) = %d, want 2", len(client.requests))
	}
	if client.requests[0].Cache == nil || client.requests[0].Cache.PreviousResponseID != "resp_prev" {
		t.Fatalf("first request cache = %#v, want resp_prev", client.requests[0].Cache)
	}
	if client.requests[1].Cache == nil {
		t.Fatal("second request cache = nil, want updated previous_response_id")
	}
	if client.requests[1].Cache.PreviousResponseID != "resp_next" {
		t.Fatalf("second request cache previous = %q, want %q", client.requests[1].Cache.PreviousResponseID, "resp_next")
	}
}

func TestRunTurnUsesIncrementalNativeContinuationAfterPrefixRotation(t *testing.T) {
	workspace := t.TempDir()
	readmePath := filepath.Join(workspace, "README.md")
	if err := os.WriteFile(readmePath, []byte("hello from readme"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	store := &fakeConversationStore{
		items: []model.ConversationItem{
			model.UserMessageItem("turn 1"),
			model.UserMessageItem("turn 2"),
			model.UserMessageItem("turn 3"),
			model.UserMessageItem("turn 4"),
			model.UserMessageItem("turn 5"),
		},
	}
	client := &recordingStreamingClient{
		capabilities: model.ProviderCapabilities{
			Profile:              model.CapabilityProfileArkResponses,
			SupportsSessionCache: true,
			SupportsPrefixCache:  true,
			CachesToolResults:    true,
			RotatesSessionRef:    true,
		},
		streams: [][]model.StreamEvent{
			{
				{Kind: model.StreamEventResponseMetadata, ResponseMetadata: &model.ResponseMetadata{
					ResponseID:   "resp_prefix",
					InputTokens:  12,
					OutputTokens: 5,
					CachedTokens: 8,
				}},
				{Kind: model.StreamEventToolCallEnd, ToolCall: model.ToolCallChunk{
					ID:    "call_1",
					Name:  "file_read",
					Input: map[string]any{"path": readmePath},
				}},
				{Kind: model.StreamEventMessageEnd},
			},
			{
				{Kind: model.StreamEventMessageEnd},
			},
		},
	}
	manager := contextstate.NewManager(contextstate.Config{
		MaxRecentItems:      2,
		MaxEstimatedTokens:  8,
		CompactionThreshold: 4,
	})
	runner := New(client, tools.NewRegistry(), permissions.NewEngine(), store, manager, nil, 8)

	err := runner.RunTurn(context.Background(), Input{
		Session: types.Session{ID: "sess_1", WorkspaceRoot: workspace},
		Turn:    types.Turn{ID: "turn_6", SessionID: "sess_1", UserMessage: "follow up"},
		Sink:    &recordingSink{},
	})
	if err != nil {
		t.Fatalf("RunTurn() error = %v", err)
	}

	if len(client.requests) != 2 {
		t.Fatalf("len(requests) = %d, want 2", len(client.requests))
	}
	if client.requests[0].Cache == nil || client.requests[0].Cache.Mode != model.CacheModePrefix {
		t.Fatalf("first request cache = %#v, want prefix rotation", client.requests[0].Cache)
	}
	if client.requests[1].Cache == nil {
		t.Fatal("second request cache = nil, want session continuation")
	}
	if client.requests[1].Cache.Mode != model.CacheModeSession {
		t.Fatalf("second request cache mode = %q, want %q", client.requests[1].Cache.Mode, model.CacheModeSession)
	}
	if client.requests[1].Cache.PreviousResponseID != "resp_prefix" {
		t.Fatalf("second request cache previous = %q, want %q", client.requests[1].Cache.PreviousResponseID, "resp_prefix")
	}
	if client.requests[1].UserMessage != "" {
		t.Fatalf("second request user message = %q, want empty native continuation", client.requests[1].UserMessage)
	}
	if len(client.requests[1].Items) != 2 {
		t.Fatalf("len(second request items) = %d, want 2 incremental items", len(client.requests[1].Items))
	}
	if client.requests[1].Items[0].Kind != model.ConversationItemToolCall || client.requests[1].Items[0].ToolCall.ID != "call_1" {
		t.Fatalf("second request items[0] = %#v, want tool call delta", client.requests[1].Items[0])
	}
	if client.requests[1].Items[1].Kind != model.ConversationItemToolResult || client.requests[1].Items[1].Result == nil || client.requests[1].Items[1].Result.ToolCallID != "call_1" {
		t.Fatalf("second request items[1] = %#v, want tool result delta", client.requests[1].Items[1])
	}
}

func TestRunTurnCompactsEvenWhenSummariesAlreadyExist(t *testing.T) {
	store := &fakeConversationStore{
		items: []model.ConversationItem{
			model.UserMessageItem("turn 1"),
			model.UserMessageItem("turn 2"),
			model.UserMessageItem("turn 3"),
			model.UserMessageItem("turn 4"),
			model.UserMessageItem("turn 5"),
			model.UserMessageItem("turn 6"),
		},
		summaries: []model.Summary{{
			RangeLabel: "turns 1-2",
		}},
	}
	client := &recordingStreamingClient{
		streams: [][]model.StreamEvent{{
			{Kind: model.StreamEventMessageEnd},
		}},
	}
	manager := contextstate.NewManager(contextstate.Config{
		MaxRecentItems:             2,
		MaxEstimatedTokens:         8,
		CompactionThreshold:        4,
		MicrocompactBytesThreshold: 9999,
	})
	compactor := &recordingCompactor{
		summary: model.Summary{
			RangeLabel: "turns 1-4",
		},
	}
	runner := New(client, tools.NewRegistry(), permissions.NewEngine(), store, manager, compactor, 8)

	err := runner.RunTurn(context.Background(), Input{
		Session: types.Session{ID: "sess_1", WorkspaceRoot: t.TempDir()},
		Turn:    types.Turn{ID: "turn_7", SessionID: "sess_1", UserMessage: "follow up"},
		Sink:    &recordingSink{},
	})
	if err != nil {
		t.Fatalf("RunTurn() error = %v", err)
	}

	if compactor.calls != 1 {
		t.Fatalf("compactor calls = %d, want 1", compactor.calls)
	}
	if len(store.insertedSummaries) != 1 {
		t.Fatalf("len(inserted summaries) = %d, want 1", len(store.insertedSummaries))
	}
	if store.insertedSummaries[0].RangeLabel != "turns 1-4" {
		t.Fatalf("inserted summary = %#v, want rolling compaction summary", store.insertedSummaries[0])
	}
}

func TestLoadConversationStatePrependsSessionMemorySummary(t *testing.T) {
	store := &fakeConversationStore{
		summaries: []model.Summary{{
			RangeLabel: "turns 1-2",
		}},
		sessionMemory: &types.SessionMemory{
			SessionID: "sess_mem",
			SummaryPayload: encodeSessionMemorySummary(model.Summary{
				RangeLabel:       "session memory",
				ImportantChoices: []string{"prefer rg for search"},
			}),
			CreatedAt: time.Now().UTC(),
			UpdatedAt: time.Now().UTC(),
		},
	}
	manager := contextstate.NewManager(contextstate.Config{
		MaxRecentItems:      8,
		MaxEstimatedTokens:  6000,
		CompactionThreshold: 16,
	})
	runner := New(model.NewFakeStreaming(nil), tools.NewRegistry(), permissions.NewEngine(), store, manager, nil, 8)

	_, working, _, err := loadConversationState(context.Background(), runner, Input{
		Session: types.Session{ID: "sess_mem", WorkspaceRoot: t.TempDir()},
		Turn:    types.Turn{ID: "turn_mem", SessionID: "sess_mem", UserMessage: "continue"},
	}, "sess_mem")
	if err != nil {
		t.Fatalf("loadConversationState() error = %v", err)
	}

	if len(working.Summaries) != 1 {
		t.Fatalf("len(working summaries) = %d, want 1 compact session summary", len(working.Summaries))
	}
	if working.Summaries[0].RangeLabel != "session memory" {
		t.Fatalf("working summaries[0] = %#v, want session memory first", working.Summaries[0])
	}
	if len(working.Summaries[0].ImportantChoices) != 1 || working.Summaries[0].ImportantChoices[0] != "prefer rg for search" {
		t.Fatalf("session memory summary = %#v, want injected memory choice", working.Summaries[0])
	}
}

func TestDedupeSummaryStringsPrefersMoreSpecificSemanticVariant(t *testing.T) {
	got := dedupeSummaryStrings([]string{
		"prefer rg",
		"Prefer rg for searches",
		"PREFER RG",
	})
	if len(got) != 1 {
		t.Fatalf("len(dedupeSummaryStrings) = %d, want 1", len(got))
	}
	if got[0] != "Prefer rg for searches" {
		t.Fatalf("dedupeSummaryStrings = %#v, want more specific semantic variant", got)
	}
}

func TestBuildWorkspaceDetailMemoriesSplitsSummaryFields(t *testing.T) {
	memories := buildWorkspaceDetailMemories(types.SessionMemory{
		SessionID:     "sess_detail",
		WorkspaceRoot: "/tmp/demo",
		SourceTurnID:  "turn_detail",
	}, model.Summary{
		ImportantChoices: []string{"prefer rg", "Prefer rg for searches"},
		FilesTouched:     []string{"README.md"},
		OpenThreads:      []string{"verify cache rotation"},
	})
	if len(memories) != 3 {
		t.Fatalf("len(buildWorkspaceDetailMemories) = %d, want 3 split durable memories", len(memories))
	}
	seen := map[string]bool{}
	for _, memory := range memories {
		seen[memory.Content] = true
	}
	if !seen["[Workspace durable memory] Choice: Prefer rg for searches"] {
		t.Fatalf("workspace detail memories = %#v, want split choice memory", memories)
	}
	if !seen["[Workspace durable memory] File focus: README.md"] {
		t.Fatalf("workspace detail memories = %#v, want file memory", memories)
	}
	if !seen["[Workspace durable memory] Open thread: verify cache rotation"] {
		t.Fatalf("workspace detail memories = %#v, want open thread memory", memories)
	}
}

func TestMaybeRefreshSessionMemoryPrunesStaleWorkspaceDurableEntries(t *testing.T) {
	workspaceRoot := t.TempDir()
	staleID := durableWorkspaceDetailID(workspaceRoot, "thread", "obsolete thread")
	store := &fakeConversationStore{
		items: []model.ConversationItem{
			model.UserMessageItem("turn 1"),
			{Kind: model.ConversationItemAssistantText, Text: "assistant 1"},
			model.UserMessageItem("turn 2"),
			{Kind: model.ConversationItemAssistantText, Text: "assistant 2"},
			model.UserMessageItem("turn 3"),
			{Kind: model.ConversationItemAssistantText, Text: "assistant 3"},
			model.UserMessageItem("turn 4"),
			{Kind: model.ConversationItemAssistantText, Text: "assistant 4"},
		},
		memories: []types.MemoryEntry{{
			ID:          staleID,
			Scope:       types.MemoryScopeWorkspace,
			WorkspaceID: workspaceRoot,
			Content:     "[Workspace durable memory] Open thread: obsolete thread",
		}},
	}
	compactor := &recordingCompactor{
		summary: model.Summary{
			RangeLabel:       "session memory",
			ImportantChoices: []string{"prefer rg"},
		},
	}
	runner := &Engine{
		store:     store,
		compactor: compactor,
	}

	if err := maybeRefreshSessionMemory(context.Background(), runner, Input{
		Session: types.Session{ID: "sess_prune", WorkspaceRoot: workspaceRoot},
		Turn:    types.Turn{ID: "turn_prune", SessionID: "sess_prune"},
	}); err != nil {
		t.Fatalf("maybeRefreshSessionMemory() error = %v", err)
	}

	if len(store.deletedMemoryIDs) != 1 || store.deletedMemoryIDs[0] != staleID {
		t.Fatalf("deleted memory ids = %#v, want stale workspace durable memory pruned", store.deletedMemoryIDs)
	}
	if findMemoryByID(store.memories, staleID) != nil {
		t.Fatalf("memories = %#v, want stale workspace durable memory removed", store.memories)
	}
}

func TestLoadConversationStateAppliesMemoryInjectionBudgets(t *testing.T) {
	workspaceRoot := t.TempDir()
	store := &fakeConversationStore{
		memories: []types.MemoryEntry{
			{
				ID:          durableWorkspaceOverviewID(workspaceRoot),
				Scope:       types.MemoryScopeWorkspace,
				WorkspaceID: workspaceRoot,
				Content:     "[Workspace durable memory]\nChoices: prefer rg",
			},
			{
				ID:          durableWorkspaceDetailID(workspaceRoot, "choice", "prefer rg for searches"),
				Scope:       types.MemoryScopeWorkspace,
				WorkspaceID: workspaceRoot,
				Content:     "[Workspace durable memory] Choice: prefer rg for searches",
				UpdatedAt:   time.Now().UTC(),
			},
			{
				ID:          durableWorkspaceDetailID(workspaceRoot, "thread", "verify prompt compaction"),
				Scope:       types.MemoryScopeWorkspace,
				WorkspaceID: workspaceRoot,
				Content:     "[Workspace durable memory] Open thread: verify prompt compaction",
				UpdatedAt:   time.Now().UTC().Add(-time.Second),
			},
			{
				ID:          durableWorkspaceDetailID(workspaceRoot, "tool", "shell command produced focused diff"),
				Scope:       types.MemoryScopeWorkspace,
				WorkspaceID: workspaceRoot,
				Content:     "[Workspace durable memory] Tool outcome: shell command produced focused diff",
				UpdatedAt:   time.Now().UTC().Add(-2 * time.Second),
			},
			{
				ID:         durableGlobalMemoryID("I prefer answers in Chinese"),
				Scope:      types.MemoryScopeGlobal,
				Content:    "[Global durable memory] I prefer answers in Chinese",
				UpdatedAt:  time.Now().UTC(),
				Confidence: 0.9,
			},
			{
				ID:         durableGlobalMemoryID("keep replies concise"),
				Scope:      types.MemoryScopeGlobal,
				Content:    "[Global durable memory] keep replies concise",
				UpdatedAt:  time.Now().UTC().Add(-time.Second),
				Confidence: 0.8,
			},
		},
	}
	manager := contextstate.NewManager(contextstate.Config{
		MaxRecentItems:      8,
		MaxEstimatedTokens:  6000,
		CompactionThreshold: 16,
	})
	runner := New(model.NewFakeStreaming(nil), tools.NewRegistry(), permissions.NewEngine(), store, manager, nil, 8)

	_, working, _, err := loadConversationState(context.Background(), runner, Input{
		Session: types.Session{ID: "sess_budget", WorkspaceRoot: workspaceRoot},
		Turn:    types.Turn{ID: "turn_budget", SessionID: "sess_budget", UserMessage: "用 rg 搜索，并继续处理 prompt compaction，回答用中文"},
	}, "sess_budget")
	if err != nil {
		t.Fatalf("loadConversationState() error = %v", err)
	}

	if len(working.MemoryRefs) != 4 {
		t.Fatalf("working memory refs = %#v, want overview + 2 workspace details + 1 global", working.MemoryRefs)
	}
	if !strings.Contains(working.MemoryRefs[0], "[Workspace durable memory]") || !strings.Contains(working.MemoryRefs[0], "Choices: prefer rg") {
		t.Fatalf("working memory refs[0] = %q, want workspace overview first", working.MemoryRefs[0])
	}
	globalCount := 0
	for _, ref := range working.MemoryRefs {
		if strings.Contains(ref, "[Global durable memory]") {
			globalCount++
		}
	}
	if globalCount != 1 {
		t.Fatalf("working memory refs = %#v, want global memories capped to 1", working.MemoryRefs)
	}
}

func TestLoadConversationStateSkipsDurableWorkspaceMemoryWhenSessionMemoryExists(t *testing.T) {
	workspaceRoot := t.TempDir()
	store := &fakeConversationStore{
		memories: []types.MemoryEntry{{
			ID:          durableWorkspaceMemoryID(workspaceRoot),
			Scope:       types.MemoryScopeWorkspace,
			WorkspaceID: workspaceRoot,
			Content:     "[Workspace durable memory]\nChoices: prefer rg",
		}},
		sessionMemory: &types.SessionMemory{
			SessionID: "sess_mem",
			SummaryPayload: encodeSessionMemorySummary(model.Summary{
				RangeLabel:       "session memory",
				ImportantChoices: []string{"prefer rg"},
			}),
			CreatedAt: time.Now().UTC(),
			UpdatedAt: time.Now().UTC(),
		},
	}
	manager := contextstate.NewManager(contextstate.Config{
		MaxRecentItems:      8,
		MaxEstimatedTokens:  6000,
		CompactionThreshold: 16,
	})
	runner := New(model.NewFakeStreaming(nil), tools.NewRegistry(), permissions.NewEngine(), store, manager, nil, 8)

	_, working, _, err := loadConversationState(context.Background(), runner, Input{
		Session: types.Session{ID: "sess_mem", WorkspaceRoot: workspaceRoot},
		Turn:    types.Turn{ID: "turn_mem", SessionID: "sess_mem", UserMessage: "search with rg"},
	}, "sess_mem")
	if err != nil {
		t.Fatalf("loadConversationState() error = %v", err)
	}

	if len(working.MemoryRefs) != 0 {
		t.Fatalf("working memory refs = %#v, want durable workspace memory skipped when session memory exists", working.MemoryRefs)
	}
}

func TestLoadConversationStateKeepsMatchingWorkspaceDetailMemoryWhenSessionMemoryExists(t *testing.T) {
	workspaceRoot := t.TempDir()
	store := &fakeConversationStore{
		memories: []types.MemoryEntry{
			{
				ID:          durableWorkspaceOverviewID(workspaceRoot),
				Scope:       types.MemoryScopeWorkspace,
				WorkspaceID: workspaceRoot,
				Content:     "[Workspace durable memory]\nChoices: prefer rg",
			},
			{
				ID:          durableWorkspaceDetailID(workspaceRoot, "thread", "verify prompt compaction"),
				Scope:       types.MemoryScopeWorkspace,
				WorkspaceID: workspaceRoot,
				Content:     "[Workspace durable memory] Open thread: verify prompt compaction",
				UpdatedAt:   time.Now().UTC(),
			},
		},
		sessionMemory: &types.SessionMemory{
			SessionID: "sess_mem",
			SummaryPayload: encodeSessionMemorySummary(model.Summary{
				RangeLabel:       "session memory",
				ImportantChoices: []string{"prefer rg"},
			}),
			CreatedAt: time.Now().UTC(),
			UpdatedAt: time.Now().UTC(),
		},
	}
	manager := contextstate.NewManager(contextstate.Config{
		MaxRecentItems:      8,
		MaxEstimatedTokens:  6000,
		CompactionThreshold: 16,
	})
	runner := New(model.NewFakeStreaming(nil), tools.NewRegistry(), permissions.NewEngine(), store, manager, nil, 8)

	_, working, _, err := loadConversationState(context.Background(), runner, Input{
		Session: types.Session{ID: "sess_mem", WorkspaceRoot: workspaceRoot},
		Turn:    types.Turn{ID: "turn_mem", SessionID: "sess_mem", UserMessage: "继续处理 prompt compaction"},
	}, "sess_mem")
	if err != nil {
		t.Fatalf("loadConversationState() error = %v", err)
	}

	if len(working.MemoryRefs) != 1 {
		t.Fatalf("working memory refs = %#v, want only matching workspace detail memory", working.MemoryRefs)
	}
	if !strings.Contains(working.MemoryRefs[0], "verify prompt compaction") {
		t.Fatalf("working memory refs = %#v, want matching workspace detail memory retained", working.MemoryRefs)
	}
}

func TestRunTurnAppliesTargetedMicrocompactCarryForward(t *testing.T) {
	store := &fakeConversationStore{
		items: []model.ConversationItem{
			model.UserMessageItem("turn 1"),
			{
				Kind: model.ConversationItemToolCall,
				ToolCall: model.ToolCallChunk{
					ID:    "call_1",
					Name:  "shell_command",
					Input: map[string]any{"command": "pwd"},
				},
			},
			model.ToolResultItem(model.ToolResult{
				ToolCallID: "call_1",
				ToolName:   "shell_command",
				Content:    strings.Repeat("x", 80),
			}),
			{Kind: model.ConversationItemAssistantText, Text: "recent assistant"},
			model.UserMessageItem("recent user"),
		},
	}
	client := &recordingStreamingClient{
		streams: [][]model.StreamEvent{{
			{Kind: model.StreamEventMessageEnd},
		}},
	}
	manager := contextstate.NewManager(contextstate.Config{
		MaxRecentItems:             2,
		MaxEstimatedTokens:         9999,
		CompactionThreshold:        2,
		MicrocompactBytesThreshold: 16,
	})
	compactor := &recordingCompactor{
		summary: model.Summary{
			RangeLabel: "turns 1-2",
		},
	}
	runner := New(client, tools.NewRegistry(), permissions.NewEngine(), store, manager, compactor, 8)

	err := runner.RunTurn(context.Background(), Input{
		Session: types.Session{ID: "sess_1", WorkspaceRoot: t.TempDir()},
		Turn:    types.Turn{ID: "turn_5", SessionID: "sess_1", UserMessage: "follow up"},
		Sink:    &recordingSink{},
	})
	if err != nil {
		t.Fatalf("RunTurn() error = %v", err)
	}

	if len(store.insertedSummaries) != 0 {
		t.Fatalf("len(inserted summaries) = %d, want 0 for targeted microcompact", len(store.insertedSummaries))
	}
	if len(store.insertedCompactions) != 1 {
		t.Fatalf("len(inserted compactions) = %d, want 1", len(store.insertedCompactions))
	}
	if store.insertedCompactions[0].Kind != types.ConversationCompactionKindMicro {
		t.Fatalf("inserted compaction = %#v, want micro kind", store.insertedCompactions[0])
	}
	payload, err := decodeMicrocompactPayload(store.insertedCompactions[0].SummaryPayload)
	if err != nil {
		t.Fatalf("decodeMicrocompactPayload() error = %v", err)
	}
	if len(payload.Items) != 3 {
		t.Fatalf("len(payload items) = %d, want boundary + tool call + compacted result", len(payload.Items))
	}
	if payload.Items[0].Kind != model.ConversationItemSummary {
		t.Fatalf("payload items[0] = %#v, want boundary summary", payload.Items[0])
	}
	if payload.Items[1].Kind != model.ConversationItemToolCall {
		t.Fatalf("payload items[1] = %#v, want carried-forward tool call", payload.Items[1])
	}
	if payload.Items[2].Kind != model.ConversationItemToolResult || payload.Items[2].Result == nil || !strings.Contains(payload.Items[2].Result.Content, "[Microcompacted historical tool result]") {
		t.Fatalf("payload items[2] = %#v, want compacted tool result", payload.Items[2])
	}
	if len(client.requests) != 1 {
		t.Fatalf("len(requests) = %d, want 1", len(client.requests))
	}
	if len(client.requests[0].Items) != 6 {
		t.Fatalf("len(request items) = %d, want 6 carry-forward + recent + user", len(client.requests[0].Items))
	}
	if client.requests[0].Items[0].Kind != model.ConversationItemSummary {
		t.Fatalf("request items[0] = %#v, want microcompact boundary summary", client.requests[0].Items[0])
	}
	if client.requests[0].Items[1].Kind != model.ConversationItemToolCall {
		t.Fatalf("request items[1] = %#v, want carried-forward tool call", client.requests[0].Items[1])
	}
	if client.requests[0].Items[2].Kind != model.ConversationItemToolResult || client.requests[0].Items[2].Result == nil || !strings.Contains(client.requests[0].Items[2].Result.Content, "Original size") {
		t.Fatalf("request items[2] = %#v, want compacted tool result", client.requests[0].Items[2])
	}
}

func TestMaybeRefreshSessionMemoryMergesPriorSummaryWithFreshDelta(t *testing.T) {
	createdAt := time.Now().UTC().Add(-10 * time.Minute)
	store := &fakeConversationStore{
		items: []model.ConversationItem{
			model.UserMessageItem("turn 1"),
			{Kind: model.ConversationItemAssistantText, Text: "assistant 1"},
			model.UserMessageItem("turn 2"),
			{Kind: model.ConversationItemAssistantText, Text: "assistant 2"},
			model.UserMessageItem("turn 3"),
			{Kind: model.ConversationItemAssistantText, Text: "assistant 3"},
			model.UserMessageItem("turn 4"),
			{Kind: model.ConversationItemAssistantText, Text: "assistant 4"},
		},
		sessionMemory: &types.SessionMemory{
			SessionID:    "sess_merge",
			UpToPosition: 2,
			ItemCount:    2,
			SummaryPayload: encodeSessionMemorySummary(model.Summary{
				RangeLabel:  "session memory",
				OpenThreads: []string{"remember previous architecture decision"},
			}),
			CreatedAt: createdAt,
			UpdatedAt: createdAt,
		},
	}
	compactor := &recordingCompactor{
		summary: model.Summary{
			RangeLabel:  "session memory",
			OpenThreads: []string{"merged outcome"},
		},
	}
	runner := &Engine{
		store:     store,
		compactor: compactor,
	}

	err := maybeRefreshSessionMemory(context.Background(), runner, Input{
		Session: types.Session{ID: "sess_merge", WorkspaceRoot: t.TempDir()},
		Turn:    types.Turn{ID: "turn_merge", SessionID: "sess_merge"},
	})
	if err != nil {
		t.Fatalf("maybeRefreshSessionMemory() error = %v", err)
	}

	if compactor.calls != 1 {
		t.Fatalf("compactor calls = %d, want 1", compactor.calls)
	}
	if len(compactor.inputs) != 1 {
		t.Fatalf("len(compactor inputs) = %d, want 1", len(compactor.inputs))
	}
	if len(compactor.inputs[0]) != 7 {
		t.Fatalf("len(compactor input items) = %d, want 7 (prior summary + 6 fresh items)", len(compactor.inputs[0]))
	}
	if compactor.inputs[0][0].Kind != model.ConversationItemSummary || compactor.inputs[0][0].Summary == nil {
		t.Fatalf("compactor input[0] = %#v, want prior session summary item", compactor.inputs[0][0])
	}
	if store.upsertedSessionMemory == nil {
		t.Fatal("upserted session memory = nil, want refreshed memory")
	}
	if store.upsertedSessionMemory.UpToPosition != len(store.items) {
		t.Fatalf("session memory up_to_position = %d, want %d", store.upsertedSessionMemory.UpToPosition, len(store.items))
	}
	if store.upsertedSessionMemory.CreatedAt != createdAt {
		t.Fatalf("session memory created_at = %s, want preserved %s", store.upsertedSessionMemory.CreatedAt, createdAt)
	}
	if store.upsertedSessionMemory.SourceTurnID != "turn_merge" {
		t.Fatalf("session memory source_turn_id = %q, want %q", store.upsertedSessionMemory.SourceTurnID, "turn_merge")
	}
	workspaceMemory := findMemoryByID(store.upsertedMemoryEntries, durableWorkspaceOverviewID(store.sessionMemory.WorkspaceRoot))
	if workspaceMemory == nil {
		t.Fatal("workspace durable overview memory = nil, want promotion from session memory")
	}
	if workspaceMemory.Scope != types.MemoryScopeWorkspace {
		t.Fatalf("workspace durable memory scope = %q, want %q", workspaceMemory.Scope, types.MemoryScopeWorkspace)
	}
	if !strings.Contains(workspaceMemory.Content, "Open threads: merged outcome") {
		t.Fatalf("workspace durable memory content = %q, want summarized open thread", workspaceMemory.Content)
	}
	detailFound := false
	for _, entry := range store.upsertedMemoryEntries {
		if entry.Scope == types.MemoryScopeWorkspace && strings.Contains(entry.Content, "Open thread: merged outcome") {
			detailFound = true
			break
		}
	}
	if !detailFound {
		t.Fatalf("workspace detail memories = %#v, want split open-thread durable memory", store.upsertedMemoryEntries)
	}
}

func TestMaybeRefreshSessionMemoryPromotesGlobalPreferenceMemory(t *testing.T) {
	store := &fakeConversationStore{
		items: []model.ConversationItem{
			model.UserMessageItem("turn 1"),
			{Kind: model.ConversationItemAssistantText, Text: "assistant 1"},
			model.UserMessageItem("turn 2"),
			{Kind: model.ConversationItemAssistantText, Text: "assistant 2"},
			model.UserMessageItem("turn 3"),
			{Kind: model.ConversationItemAssistantText, Text: "assistant 3"},
			model.UserMessageItem("turn 4"),
			{Kind: model.ConversationItemAssistantText, Text: "assistant 4"},
		},
	}
	compactor := &recordingCompactor{
		summary: model.Summary{
			RangeLabel:       "session memory",
			ImportantChoices: []string{"I prefer answers in Chinese"},
		},
	}
	runner := &Engine{
		store:     store,
		compactor: compactor,
	}

	err := maybeRefreshSessionMemory(context.Background(), runner, Input{
		Session: types.Session{ID: "sess_global_pref", WorkspaceRoot: t.TempDir()},
		Turn:    types.Turn{ID: "turn_global_pref", SessionID: "sess_global_pref"},
	})
	if err != nil {
		t.Fatalf("maybeRefreshSessionMemory() error = %v", err)
	}

	foundGlobal := false
	for _, entry := range store.memories {
		if entry.Scope != types.MemoryScopeGlobal {
			continue
		}
		foundGlobal = true
		if !strings.Contains(entry.Content, "I prefer answers in Chinese") {
			t.Fatalf("global durable memory content = %q, want preference", entry.Content)
		}
		break
	}
	if !foundGlobal {
		t.Fatal("global durable memory not promoted from session summary")
	}
}

func TestRunTurnIgnoresSessionMemoryRefreshFailure(t *testing.T) {
	store := &fakeConversationStore{
		items: []model.ConversationItem{
			model.UserMessageItem("turn 1"),
			{Kind: model.ConversationItemAssistantText, Text: "assistant 1"},
			model.UserMessageItem("turn 2"),
			{Kind: model.ConversationItemAssistantText, Text: "assistant 2"},
			model.UserMessageItem("turn 3"),
			{Kind: model.ConversationItemAssistantText, Text: "assistant 3"},
		},
	}
	client := model.NewFakeStreaming([][]model.StreamEvent{{
		{Kind: model.StreamEventTextDelta, TextDelta: "done"},
		{Kind: model.StreamEventMessageEnd},
	}})
	manager := contextstate.NewManager(contextstate.Config{
		MaxRecentItems:      16,
		MaxEstimatedTokens:  6000,
		CompactionThreshold: 99,
	})
	runner := New(client, tools.NewRegistry(), permissions.NewEngine(), store, manager, &erroringCompactor{err: errors.New("compact failed")}, 8)
	sink := &recordingSink{}

	err := runner.RunTurn(context.Background(), Input{
		Session: types.Session{ID: "sess_refresh_fail", WorkspaceRoot: t.TempDir()},
		Turn:    types.Turn{ID: "turn_refresh_fail", SessionID: "sess_refresh_fail", UserMessage: "continue"},
		Sink:    sink,
	})
	if err != nil {
		t.Fatalf("RunTurn() error = %v, want nil even when session memory refresh fails", err)
	}

	assertEventTypes(t, sink, []string{
		types.EventTurnStarted,
		types.EventAssistantStarted,
		types.EventAssistantDelta,
		types.EventAssistantCompleted,
		types.EventTurnCompleted,
	})
	if store.upsertedSessionMemory != nil {
		t.Fatalf("upserted session memory = %#v, want nil when refresh fails", store.upsertedSessionMemory)
	}
}

func TestRunTurnCanRefreshSessionMemoryAsynchronously(t *testing.T) {
	workspaceRoot := t.TempDir()
	store := &fakeConversationStore{
		items: []model.ConversationItem{
			model.UserMessageItem("turn 1"),
			{Kind: model.ConversationItemAssistantText, Text: "assistant 1"},
			model.UserMessageItem("turn 2"),
			{Kind: model.ConversationItemAssistantText, Text: "assistant 2"},
			model.UserMessageItem("turn 3"),
			{Kind: model.ConversationItemAssistantText, Text: "assistant 3"},
		},
	}
	blocker := make(chan struct{})
	client := model.NewFakeStreaming([][]model.StreamEvent{{
		{Kind: model.StreamEventTextDelta, TextDelta: "done"},
		{Kind: model.StreamEventMessageEnd},
	}})
	manager := contextstate.NewManager(contextstate.Config{
		MaxRecentItems:      16,
		MaxEstimatedTokens:  6000,
		CompactionThreshold: 99,
	})
	runner := New(client, tools.NewRegistry(), permissions.NewEngine(), store, manager, &blockingCompactor{
		summary: model.Summary{
			RangeLabel:       "session memory",
			ImportantChoices: []string{"prefer rg"},
		},
		block: blocker,
	}, 8)
	runner.SetSessionMemoryAsync(true)
	sink := &recordingSink{}

	done := make(chan error, 1)
	go func() {
		done <- runner.RunTurn(context.Background(), Input{
			Session: types.Session{ID: "sess_async", WorkspaceRoot: workspaceRoot},
			Turn:    types.Turn{ID: "turn_async", SessionID: "sess_async", UserMessage: "continue"},
			Sink:    sink,
		})
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("RunTurn() error = %v", err)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("RunTurn() did not return before async session memory compaction completed")
	}

	if store.upsertedSessionMemory != nil {
		t.Fatalf("upserted session memory before unblock = %#v, want nil", store.upsertedSessionMemory)
	}

	close(blocker)
	runner.waitBackgroundTasks()

	if store.upsertedSessionMemory == nil {
		t.Fatal("upserted session memory = nil after waiting for background refresh")
	}
}

func TestRunTurnAsyncSessionMemoryWorkerEmitsLifecycleEvents(t *testing.T) {
	workspaceRoot := t.TempDir()
	store := &fakeConversationStore{
		items: []model.ConversationItem{
			model.UserMessageItem("turn 1"),
			{Kind: model.ConversationItemAssistantText, Text: "assistant 1"},
			model.UserMessageItem("turn 2"),
			{Kind: model.ConversationItemAssistantText, Text: "assistant 2"},
			model.UserMessageItem("turn 3"),
			{Kind: model.ConversationItemAssistantText, Text: "assistant 3"},
		},
	}
	client := model.NewFakeStreaming([][]model.StreamEvent{{
		{Kind: model.StreamEventTextDelta, TextDelta: "done"},
		{Kind: model.StreamEventMessageEnd},
	}})
	manager := contextstate.NewManager(contextstate.Config{
		MaxRecentItems:      16,
		MaxEstimatedTokens:  6000,
		CompactionThreshold: 99,
	})
	runner := New(client, tools.NewRegistry(), permissions.NewEngine(), store, manager, &recordingCompactor{
		summary: model.Summary{
			RangeLabel:       "session memory",
			ImportantChoices: []string{"I prefer answers in Chinese"},
		},
	}, 8)
	runner.SetSessionMemoryAsync(true)
	sink := &recordingSink{}

	if err := runner.RunTurn(context.Background(), Input{
		Session: types.Session{ID: "sess_async_events", WorkspaceRoot: workspaceRoot},
		Turn:    types.Turn{ID: "turn_async_events", SessionID: "sess_async_events", UserMessage: "continue"},
		Sink:    sink,
	}); err != nil {
		t.Fatalf("RunTurn() error = %v", err)
	}
	runner.waitBackgroundTasks()

	seenStarted := false
	seenCompleted := false
	for _, event := range sink.events {
		switch event.Type {
		case types.EventSessionMemoryStarted:
			seenStarted = true
		case types.EventSessionMemoryCompleted:
			seenCompleted = true
			var payload types.SessionMemoryEventPayload
			if err := json.Unmarshal(event.Payload, &payload); err != nil {
				t.Fatalf("json.Unmarshal(session memory payload) error = %v", err)
			}
			if !payload.Async || !payload.Updated {
				t.Fatalf("session memory completed payload = %#v, want async updated result", payload)
			}
		}
	}
	if !seenStarted || !seenCompleted {
		t.Fatalf("event types = %v, want async session memory lifecycle events", sink.eventTypes())
	}
}

func TestRunTurnUsesConfiguredSessionMemoryWorker(t *testing.T) {
	client := model.NewFakeStreaming([][]model.StreamEvent{{
		{Kind: model.StreamEventTextDelta, TextDelta: "done"},
		{Kind: model.StreamEventMessageEnd},
	}})
	worker := &recordingSessionMemoryWorker{}
	runner := New(client, tools.NewRegistry(), permissions.NewEngine(), &fakeConversationStore{}, contextstate.NewManager(contextstate.Config{
		MaxRecentItems:      8,
		MaxEstimatedTokens:  6000,
		CompactionThreshold: 16,
	}), nil, 8)
	runner.SetSessionMemoryWorker(worker)

	err := runner.RunTurn(context.Background(), Input{
		Session: types.Session{ID: "sess_worker", WorkspaceRoot: t.TempDir()},
		Turn:    types.Turn{ID: "turn_worker", SessionID: "sess_worker", UserMessage: "continue"},
		Sink:    &recordingSink{},
	})
	if err != nil {
		t.Fatalf("RunTurn() error = %v", err)
	}
	if worker.enqueueCalls != 1 {
		t.Fatalf("worker enqueue calls = %d, want 1", worker.enqueueCalls)
	}
	if worker.lastInput.Turn.ID != "turn_worker" {
		t.Fatalf("worker last turn = %#v, want turn_worker", worker.lastInput.Turn)
	}
}

func TestAsyncSessionMemoryRefreshCoalescesLatestInputForSession(t *testing.T) {
	workspaceRoot := t.TempDir()
	store := &fakeConversationStore{
		items: []model.ConversationItem{
			model.UserMessageItem("turn 1"),
			{Kind: model.ConversationItemAssistantText, Text: "assistant 1"},
			model.UserMessageItem("turn 2"),
			{Kind: model.ConversationItemAssistantText, Text: "assistant 2"},
			model.UserMessageItem("turn 3"),
			{Kind: model.ConversationItemAssistantText, Text: "assistant 3"},
			model.UserMessageItem("turn 4"),
			{Kind: model.ConversationItemAssistantText, Text: "assistant 4"},
		},
	}
	appended := false
	store.afterUpsertSessionMemory = func() {
		if appended {
			return
		}
		appended = true
		store.items = append(store.items,
			model.UserMessageItem("turn 5"),
			model.ConversationItem{Kind: model.ConversationItemAssistantText, Text: "assistant 5"},
			model.UserMessageItem("turn 6"),
			model.ConversationItem{Kind: model.ConversationItemAssistantText, Text: "assistant 6"},
			model.UserMessageItem("turn 7"),
			model.ConversationItem{Kind: model.ConversationItemAssistantText, Text: "assistant 7"},
		)
	}
	started := make(chan struct{}, 2)
	blocker := make(chan struct{})
	runner := &Engine{
		store: store,
		compactor: &blockingCompactor{
			summary: model.Summary{
				RangeLabel:       "session memory",
				ImportantChoices: []string{"prefer rg"},
			},
			block:   blocker,
			started: started,
		},
	}
	runner.SetSessionMemoryAsync(true)

	first := Input{
		Session: types.Session{ID: "sess_async_merge", WorkspaceRoot: workspaceRoot},
		Turn:    types.Turn{ID: "turn_async_1", SessionID: "sess_async_merge"},
	}
	startAsyncSessionMemoryRefresh(context.Background(), runner, first)
	<-started
	second := Input{
		Session: types.Session{ID: "sess_async_merge", WorkspaceRoot: workspaceRoot},
		Turn:    types.Turn{ID: "turn_async_2", SessionID: "sess_async_merge"},
	}
	startAsyncSessionMemoryRefresh(context.Background(), runner, second)

	close(blocker)
	runner.waitBackgroundTasks()

	if store.upsertedSessionMemory == nil {
		t.Fatal("upserted session memory = nil, want coalesced refresh result")
	}
	if store.upsertedSessionMemory.SourceTurnID != "turn_async_2" {
		t.Fatalf("session memory source_turn_id = %q, want latest queued turn", store.upsertedSessionMemory.SourceTurnID)
	}
}

func TestRunTurnReusesPersistedMicrocompactCarryForward(t *testing.T) {
	payload, err := buildMicrocompactPayload([]model.ConversationItem{
		model.UserMessageItem("turn 1"),
		{
			Kind: model.ConversationItemToolCall,
			ToolCall: model.ToolCallChunk{
				ID:    "call_1",
				Name:  "shell_command",
				Input: map[string]any{"command": "pwd"},
			},
		},
		model.ToolResultItem(model.ToolResult{
			ToolCallID: "call_1",
			ToolName:   "shell_command",
			Content:    strings.Repeat("x", 80),
		}),
	}, []int{2}, 2)
	if err != nil {
		t.Fatalf("buildMicrocompactPayload() error = %v", err)
	}

	store := &fakeConversationStore{
		items: []model.ConversationItem{
			model.UserMessageItem("turn 1"),
			{
				Kind: model.ConversationItemToolCall,
				ToolCall: model.ToolCallChunk{
					ID:    "call_1",
					Name:  "shell_command",
					Input: map[string]any{"command": "pwd"},
				},
			},
			model.ToolResultItem(model.ToolResult{
				ToolCallID: "call_1",
				ToolName:   "shell_command",
				Content:    strings.Repeat("x", 80),
			}),
			{Kind: model.ConversationItemAssistantText, Text: "recent assistant"},
			model.UserMessageItem("recent user"),
		},
		compactions: []types.ConversationCompaction{{
			ID:             "compact_1",
			SessionID:      "sess_1",
			Kind:           types.ConversationCompactionKindMicro,
			Generation:     1,
			StartPosition:  firstPayloadPosition(payload),
			EndPosition:    lastPayloadPosition(payload),
			SummaryPayload: encodeMicrocompactPayload(payload),
			Reason:         "microcompact_tool_results",
		}},
	}
	client := &recordingStreamingClient{
		streams: [][]model.StreamEvent{{
			{Kind: model.StreamEventMessageEnd},
		}},
	}
	manager := contextstate.NewManager(contextstate.Config{
		MaxRecentItems:             2,
		MaxEstimatedTokens:         9999,
		CompactionThreshold:        99,
		MicrocompactBytesThreshold: 16,
	})
	runner := New(client, tools.NewRegistry(), permissions.NewEngine(), store, manager, nil, 8)

	err = runner.RunTurn(context.Background(), Input{
		Session: types.Session{ID: "sess_1", WorkspaceRoot: t.TempDir()},
		Turn:    types.Turn{ID: "turn_5", SessionID: "sess_1", UserMessage: "follow up"},
		Sink:    &recordingSink{},
	})
	if err != nil {
		t.Fatalf("RunTurn() error = %v", err)
	}

	if len(store.insertedCompactions) != 0 {
		t.Fatalf("len(inserted compactions) = %d, want 0 when reusing persisted carry-forward", len(store.insertedCompactions))
	}
	if len(client.requests) != 1 {
		t.Fatalf("len(requests) = %d, want 1", len(client.requests))
	}
	if len(client.requests[0].Items) != 6 {
		t.Fatalf("len(request items) = %d, want 6 persisted carry-forward + recent + user", len(client.requests[0].Items))
	}
	if client.requests[0].Items[0].Kind != model.ConversationItemSummary {
		t.Fatalf("request items[0] = %#v, want persisted boundary", client.requests[0].Items[0])
	}
}

func TestActiveMicrocompactItemsClearedByLaterRollingCompaction(t *testing.T) {
	payload, err := buildMicrocompactPayload([]model.ConversationItem{
		model.UserMessageItem("turn 1"),
		{
			Kind: model.ConversationItemToolCall,
			ToolCall: model.ToolCallChunk{
				ID:    "call_1",
				Name:  "shell_command",
				Input: map[string]any{"command": "pwd"},
			},
		},
		model.ToolResultItem(model.ToolResult{
			ToolCallID: "call_1",
			ToolName:   "shell_command",
			Content:    strings.Repeat("x", 80),
		}),
	}, []int{2}, 2)
	if err != nil {
		t.Fatalf("buildMicrocompactPayload() error = %v", err)
	}

	got := activeMicrocompactItems([]types.ConversationCompaction{
		{
			ID:             "compact_micro",
			SessionID:      "sess_1",
			Kind:           types.ConversationCompactionKindMicro,
			StartPosition:  firstPayloadPosition(payload),
			EndPosition:    lastPayloadPosition(payload),
			SummaryPayload: encodeMicrocompactPayload(payload),
		},
		{
			ID:            "compact_roll",
			SessionID:     "sess_1",
			Kind:          types.ConversationCompactionKindRolling,
			StartPosition: 0,
			EndPosition:   4,
		},
	})

	if len(got) != 0 {
		t.Fatalf("activeMicrocompactItems() = %#v, want cleared by rolling summary", got)
	}
}

func TestRunTurnStartsFreshNativeSessionAfterCompaction(t *testing.T) {
	store := &fakeConversationStore{
		items: []model.ConversationItem{
			model.UserMessageItem("turn 1"),
			model.UserMessageItem("turn 2"),
			model.UserMessageItem("turn 3"),
			model.UserMessageItem("turn 4"),
			model.UserMessageItem("turn 5"),
		},
		cacheHead: &types.ProviderCacheHead{
			SessionID:         "sess_1",
			Provider:          "openai_compatible",
			CapabilityProfile: string(model.CapabilityProfileArkResponses),
			ActiveSessionRef:  "resp_prev",
		},
	}
	client := &recordingStreamingClient{
		capabilities: model.ProviderCapabilities{
			Profile:              model.CapabilityProfileArkResponses,
			SupportsSessionCache: true,
			CachesToolResults:    true,
			RotatesSessionRef:    true,
		},
		streams: [][]model.StreamEvent{{
			{Kind: model.StreamEventResponseMetadata, ResponseMetadata: &model.ResponseMetadata{
				ResponseID:   "resp_new",
				InputTokens:  10,
				OutputTokens: 2,
			}},
			{Kind: model.StreamEventMessageEnd},
		}},
	}
	manager := contextstate.NewManager(contextstate.Config{
		MaxRecentItems:             2,
		MaxEstimatedTokens:         8,
		CompactionThreshold:        4,
		MicrocompactBytesThreshold: 9999,
	})
	compactor := &recordingCompactor{
		summary: model.Summary{
			RangeLabel: "turns 1-3",
		},
	}
	runner := New(client, tools.NewRegistry(), permissions.NewEngine(), store, manager, compactor, 8)

	err := runner.RunTurn(context.Background(), Input{
		Session: types.Session{ID: "sess_1", WorkspaceRoot: t.TempDir()},
		Turn:    types.Turn{ID: "turn_6", SessionID: "sess_1", UserMessage: "follow up"},
		Sink:    &recordingSink{},
	})
	if err != nil {
		t.Fatalf("RunTurn() error = %v", err)
	}

	if len(client.requests) != 1 {
		t.Fatalf("len(requests) = %d, want 1", len(client.requests))
	}
	if client.requests[0].Cache == nil {
		t.Fatal("request cache = nil, want fresh native session rebuild")
	}
	if client.requests[0].Cache.Mode != model.CacheModeSession {
		t.Fatalf("request cache mode = %q, want %q", client.requests[0].Cache.Mode, model.CacheModeSession)
	}
	if client.requests[0].Cache.PreviousResponseID != "" {
		t.Fatalf("request previous_response_id = %q, want empty after compaction", client.requests[0].Cache.PreviousResponseID)
	}
	if len(client.requests[0].Items) != 4 {
		t.Fatalf("len(request items) = %d, want 4 compacted items", len(client.requests[0].Items))
	}
	if client.requests[0].Items[0].Kind != model.ConversationItemSummary {
		t.Fatalf("request items[0] = %#v, want summary item", client.requests[0].Items[0])
	}
}

func TestRunTurnPersistsToolCallAndEmitsTurnUsage(t *testing.T) {
	workspace := t.TempDir()
	readmePath := filepath.Join(workspace, "README.md")
	if err := os.WriteFile(readmePath, []byte("hello from readme"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	store := &fakeConversationStore{}
	client := model.NewFakeStreaming([][]model.StreamEvent{
		{
			{Kind: model.StreamEventResponseMetadata, ResponseMetadata: &model.ResponseMetadata{
				ResponseID:   "resp_1",
				InputTokens:  120,
				OutputTokens: 30,
				CachedTokens: 24,
			}},
			{Kind: model.StreamEventTextDelta, TextDelta: "checking readme"},
			{Kind: model.StreamEventToolCallEnd, ToolCall: model.ToolCallChunk{
				ID:    "call_1",
				Name:  "file_read",
				Input: map[string]any{"path": readmePath},
			}},
			{Kind: model.StreamEventMessageEnd},
		},
		{
			{Kind: model.StreamEventMessageEnd},
		},
	})
	sink := &recordingSink{}
	manager := contextstate.NewManager(contextstate.Config{
		MaxRecentItems:      8,
		MaxEstimatedTokens:  6000,
		CompactionThreshold: 16,
	})
	runner := NewWithRuntime(
		client,
		tools.NewRegistry(),
		permissions.NewEngine(),
		store,
		manager,
		contextstate.NewRuntime(86400, 3),
		nil,
		RuntimeMetadata{
			Provider: "openai_compatible",
			Model:    "glm-4.5",
		},
		8,
	)

	err := runner.RunTurn(context.Background(), Input{
		Session: types.Session{ID: "sess_1", WorkspaceRoot: workspace},
		Turn:    types.Turn{ID: "turn_1", SessionID: "sess_1", UserMessage: "inspect readme"},
		Sink:    sink,
	})
	if err != nil {
		t.Fatalf("RunTurn() error = %v", err)
	}

	foundToolCall := false
	for _, item := range store.insertedItems {
		if item.Kind != model.ConversationItemToolCall {
			continue
		}
		foundToolCall = true
		if item.ToolCall.ID != "call_1" || item.ToolCall.Name != "file_read" {
			t.Fatalf("tool call item = %#v, want call_1/file_read", item)
		}
		if item.ToolCall.Input["path"] != readmePath {
			t.Fatalf("tool call input path = %v, want %q", item.ToolCall.Input["path"], readmePath)
		}
	}
	if !foundToolCall {
		t.Fatalf("inserted items = %#v, want tool_call item", store.insertedItems)
	}

	if store.upsertedUsage == nil {
		t.Fatal("upserted usage = nil, want persisted turn usage")
	}
	if store.upsertedUsage.TurnID != "turn_1" || store.upsertedUsage.SessionID != "sess_1" {
		t.Fatalf("upserted usage identity = %#v, want turn_1/sess_1", *store.upsertedUsage)
	}
	if store.upsertedUsage.InputTokens != 120 || store.upsertedUsage.OutputTokens != 30 || store.upsertedUsage.CachedTokens != 24 {
		t.Fatalf("upserted usage tokens = %#v, want 120/30/24", *store.upsertedUsage)
	}
	if store.upsertedUsage.CacheHitRate != 0.2 {
		t.Fatalf("upserted usage cache hit rate = %v, want %v", store.upsertedUsage.CacheHitRate, 0.2)
	}
	if store.upsertedUsage.Provider != "openai_compatible" || store.upsertedUsage.Model != "glm-4.5" {
		t.Fatalf("upserted usage provider/model = %#v, want openai_compatible/glm-4.5", *store.upsertedUsage)
	}

	var usagePayload types.TurnUsagePayload
	foundUsageEvent := false
	for _, event := range sink.events {
		if event.Type != types.EventTurnUsage {
			continue
		}
		foundUsageEvent = true
		if err := json.Unmarshal(event.Payload, &usagePayload); err != nil {
			t.Fatalf("turn.usage payload unmarshal error = %v", err)
		}
	}
	if !foundUsageEvent {
		t.Fatalf("events = %v, want %q", sink.eventTypes(), types.EventTurnUsage)
	}
	if usagePayload.Provider != "openai_compatible" || usagePayload.Model != "glm-4.5" {
		t.Fatalf("turn.usage payload provider/model = %#v, want openai_compatible/glm-4.5", usagePayload)
	}
	if usagePayload.InputTokens != 120 || usagePayload.OutputTokens != 30 || usagePayload.CachedTokens != 24 || usagePayload.CacheHitRate != 0.2 {
		t.Fatalf("turn.usage payload tokens = %#v, want 120/30/24/0.2", usagePayload)
	}
}

func TestRunTurnAggregatesUsageAcrossMultipleResponseMetadata(t *testing.T) {
	workspace := t.TempDir()
	readmePath := filepath.Join(workspace, "README.md")
	if err := os.WriteFile(readmePath, []byte("hello from readme"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	store := &fakeConversationStore{}
	client := model.NewFakeStreaming([][]model.StreamEvent{
		{
			{Kind: model.StreamEventResponseMetadata, ResponseMetadata: &model.ResponseMetadata{
				ResponseID:   "resp_1",
				InputTokens:  100,
				OutputTokens: 20,
				CachedTokens: 10,
			}},
			{Kind: model.StreamEventToolCallEnd, ToolCall: model.ToolCallChunk{
				ID:    "call_1",
				Name:  "file_read",
				Input: map[string]any{"path": readmePath},
			}},
			{Kind: model.StreamEventMessageEnd},
		},
		{
			{Kind: model.StreamEventResponseMetadata, ResponseMetadata: &model.ResponseMetadata{
				ResponseID:   "resp_2",
				InputTokens:  50,
				OutputTokens: 10,
				CachedTokens: 5,
			}},
			{Kind: model.StreamEventMessageEnd},
		},
	})
	sink := &recordingSink{}
	manager := contextstate.NewManager(contextstate.Config{
		MaxRecentItems:      8,
		MaxEstimatedTokens:  6000,
		CompactionThreshold: 16,
	})
	runner := NewWithRuntime(
		client,
		tools.NewRegistry(),
		permissions.NewEngine(),
		store,
		manager,
		contextstate.NewRuntime(86400, 3),
		nil,
		RuntimeMetadata{
			Provider: "openai_compatible",
			Model:    "glm-4.5",
		},
		8,
	)

	err := runner.RunTurn(context.Background(), Input{
		Session: types.Session{ID: "sess_agg", WorkspaceRoot: workspace},
		Turn:    types.Turn{ID: "turn_agg", SessionID: "sess_agg", UserMessage: "inspect readme"},
		Sink:    sink,
	})
	if err != nil {
		t.Fatalf("RunTurn() error = %v", err)
	}

	if store.upsertUsageCalls != 1 {
		t.Fatalf("upsert usage calls = %d, want 1", store.upsertUsageCalls)
	}
	if store.upsertedUsage == nil {
		t.Fatal("upserted usage = nil, want aggregated usage")
	}
	if store.upsertedUsage.InputTokens != 150 || store.upsertedUsage.OutputTokens != 30 || store.upsertedUsage.CachedTokens != 15 {
		t.Fatalf("aggregated usage tokens = %#v, want 150/30/15", *store.upsertedUsage)
	}
	if store.upsertedUsage.CacheHitRate != 0.1 {
		t.Fatalf("aggregated cache hit rate = %v, want 0.1", store.upsertedUsage.CacheHitRate)
	}

	usageEvents := 0
	var payload types.TurnUsagePayload
	for _, event := range sink.events {
		if event.Type != types.EventTurnUsage {
			continue
		}
		usageEvents++
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			t.Fatalf("json.Unmarshal(turn.usage) error = %v", err)
		}
	}
	if usageEvents != 1 {
		t.Fatalf("turn.usage events = %d, want 1", usageEvents)
	}
	if payload.InputTokens != 150 || payload.OutputTokens != 30 || payload.CachedTokens != 15 || payload.CacheHitRate != 0.1 {
		t.Fatalf("turn.usage payload = %#v, want 150/30/15/0.1", payload)
	}
}

func TestRunTurnUsesFinalizingSinkForTurnCompletion(t *testing.T) {
	store := &fakeConversationStore{}
	client := model.NewFakeStreaming([][]model.StreamEvent{
		{
			{Kind: model.StreamEventResponseMetadata, ResponseMetadata: &model.ResponseMetadata{
				ResponseID:   "resp_1",
				InputTokens:  80,
				OutputTokens: 20,
				CachedTokens: 8,
			}},
			{Kind: model.StreamEventTextDelta, TextDelta: "done"},
			{Kind: model.StreamEventMessageEnd},
		},
	})
	sink := &recordingFinalizingSink{}
	manager := contextstate.NewManager(contextstate.Config{
		MaxRecentItems:      8,
		MaxEstimatedTokens:  6000,
		CompactionThreshold: 16,
	})
	runner := NewWithRuntime(
		client,
		tools.NewRegistry(),
		permissions.NewEngine(),
		store,
		manager,
		contextstate.NewRuntime(86400, 3),
		nil,
		RuntimeMetadata{
			Provider: "openai_compatible",
			Model:    "glm-4.5",
		},
		8,
	)

	err := runner.RunTurn(context.Background(), Input{
		Session: types.Session{ID: "sess_final", WorkspaceRoot: t.TempDir()},
		Turn:    types.Turn{ID: "turn_final", SessionID: "sess_final", UserMessage: "hello"},
		Sink:    sink,
	})
	if err != nil {
		t.Fatalf("RunTurn() error = %v", err)
	}

	if sink.finalizeCalls != 1 {
		t.Fatalf("finalize calls = %d, want 1", sink.finalizeCalls)
	}
	if sink.finalUsage == nil {
		t.Fatal("final usage = nil, want aggregated usage")
	}
	if sink.finalUsage.InputTokens != 80 || sink.finalUsage.OutputTokens != 20 || sink.finalUsage.CachedTokens != 8 {
		t.Fatalf("final usage tokens = %#v, want 80/20/8", *sink.finalUsage)
	}
	if store.upsertUsageCalls != 0 {
		t.Fatalf("store upsert usage calls = %d, want 0 when finalizing sink is used", store.upsertUsageCalls)
	}

	finalTypes := make([]string, 0, len(sink.finalEvents))
	for _, event := range sink.finalEvents {
		finalTypes = append(finalTypes, event.Type)
	}
	wantFinal := []string{
		types.EventAssistantCompleted,
		types.EventTurnUsage,
		types.EventTurnCompleted,
	}
	if len(finalTypes) != len(wantFinal) {
		t.Fatalf("len(final events) = %d, want %d (%v)", len(finalTypes), len(wantFinal), finalTypes)
	}
	for i := range wantFinal {
		if finalTypes[i] != wantFinal[i] {
			t.Fatalf("final events = %v, want %v", finalTypes, wantFinal)
		}
	}

	emittedTypes := sink.eventTypes()
	assertNoEventType(t, sink.recordingSink, types.EventAssistantCompleted)
	assertNoEventType(t, sink.recordingSink, types.EventTurnUsage)
	assertNoEventType(t, sink.recordingSink, types.EventTurnCompleted)
	if len(emittedTypes) == 0 || emittedTypes[0] != types.EventTurnStarted {
		t.Fatalf("emitted events = %v, want non-final events through Emit", emittedTypes)
	}
}

func TestRunTurnReturnsErrorWhenFinalizingSinkFails(t *testing.T) {
	store := &fakeConversationStore{}
	client := model.NewFakeStreaming([][]model.StreamEvent{
		{
			{Kind: model.StreamEventResponseMetadata, ResponseMetadata: &model.ResponseMetadata{
				ResponseID:   "resp_1",
				InputTokens:  10,
				OutputTokens: 3,
				CachedTokens: 1,
			}},
			{Kind: model.StreamEventMessageEnd},
		},
	})
	sink := &recordingFinalizingSink{
		finalizeErr: errors.New("finalize failed"),
	}
	runner := NewWithRuntime(
		client,
		tools.NewRegistry(),
		permissions.NewEngine(),
		store,
		contextstate.NewManager(contextstate.Config{MaxRecentItems: 8, MaxEstimatedTokens: 6000, CompactionThreshold: 16}),
		contextstate.NewRuntime(86400, 3),
		nil,
		RuntimeMetadata{Provider: "openai_compatible", Model: "glm-4.5"},
		8,
	)

	err := runner.RunTurn(context.Background(), Input{
		Session: types.Session{ID: "sess_final_fail", WorkspaceRoot: t.TempDir()},
		Turn:    types.Turn{ID: "turn_final_fail", SessionID: "sess_final_fail", UserMessage: "hello"},
		Sink:    sink,
	})
	if err == nil || err.Error() != "finalize failed" {
		t.Fatalf("RunTurn() error = %v, want finalize failed", err)
	}
	if sink.finalizeCalls != 1 {
		t.Fatalf("finalize calls = %d, want 1", sink.finalizeCalls)
	}
	if store.upsertUsageCalls != 0 {
		t.Fatalf("store upsert usage calls = %d, want 0 when finalization fails", store.upsertUsageCalls)
	}
}

type scriptedStreamingClient struct {
	events []model.StreamEvent
	err    error
}

func (c scriptedStreamingClient) Stream(_ context.Context, _ model.Request) (<-chan model.StreamEvent, <-chan error) {
	events := make(chan model.StreamEvent, len(c.events))
	errs := make(chan error, 1)

	for _, event := range c.events {
		events <- event
	}
	close(events)

	errs <- c.err
	close(errs)

	return events, errs
}

func (c scriptedStreamingClient) Capabilities() model.ProviderCapabilities {
	return model.ProviderCapabilities{Profile: model.CapabilityProfileNone}
}

type recordingStreamingClient struct {
	streams      [][]model.StreamEvent
	requests     []model.Request
	capabilities model.ProviderCapabilities
}

func (c *recordingStreamingClient) Stream(_ context.Context, req model.Request) (<-chan model.StreamEvent, <-chan error) {
	c.requests = append(c.requests, req)

	var batch []model.StreamEvent
	if len(c.streams) > 0 {
		batch = c.streams[0]
		c.streams = c.streams[1:]
	}

	events := make(chan model.StreamEvent, len(batch))
	errs := make(chan error, 1)

	for _, event := range batch {
		events <- event
	}
	close(events)

	errs <- nil
	close(errs)

	return events, errs
}

func (c *recordingStreamingClient) Capabilities() model.ProviderCapabilities {
	return c.capabilities
}

type taskManagerCapturingTool struct {
	gotManager *task.Manager
}

func toolSchemaNames(tools []model.ToolSchema) []string {
	names := make([]string, 0, len(tools))
	for _, tool := range tools {
		names = append(names, tool.Name)
	}
	return names
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func (t *taskManagerCapturingTool) Definition() tools.Definition {
	return tools.Definition{
		Name:        "glob",
		Description: "capture task manager",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"pattern": map[string]any{"type": "string"},
			},
		},
	}
}

func (t *taskManagerCapturingTool) IsConcurrencySafe() bool { return true }

func (t *taskManagerCapturingTool) Execute(_ context.Context, _ tools.Call, execCtx tools.ExecContext) (tools.Result, error) {
	t.gotManager = execCtx.TaskManager
	return tools.Result{Text: "ok"}, nil
}

type runtimeContextCapturingTool struct {
	turnContexts    []*runtimegraph.TurnContext
	seenRunIDs      []string
	runtimeServices []*runtimegraph.Service
}

func (t *runtimeContextCapturingTool) Definition() tools.Definition {
	return tools.Definition{
		Name:        "glob",
		Description: "capture runtimegraph wiring",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"pattern": map[string]any{"type": "string"},
			},
		},
	}
}

func (t *runtimeContextCapturingTool) IsConcurrencySafe() bool { return true }

func (t *runtimeContextCapturingTool) Execute(_ context.Context, call tools.Call, execCtx tools.ExecContext) (tools.Result, error) {
	if execCtx.RuntimeService == nil {
		return tools.Result{}, fmt.Errorf("runtime service missing")
	}
	if execCtx.TurnContext == nil {
		return tools.Result{}, fmt.Errorf("turn context missing")
	}

	t.runtimeServices = append(t.runtimeServices, execCtx.RuntimeService)
	t.turnContexts = append(t.turnContexts, execCtx.TurnContext)
	t.seenRunIDs = append(t.seenRunIDs, execCtx.TurnContext.CurrentRunID)

	if call.StringInput("pattern") == "first" {
		execCtx.TurnContext.CurrentRunID = "run_from_first"
	}

	return tools.Result{Text: "ok"}, nil
}

type recordingSink struct {
	events []types.Event
}

func (s *recordingSink) Emit(_ context.Context, event types.Event) error {
	s.events = append(s.events, event)
	return nil
}

func (s *recordingSink) eventTypes() []string {
	got := make([]string, 0, len(s.events))
	for _, event := range s.events {
		got = append(got, event.Type)
	}
	return got
}

type recordingFinalizingSink struct {
	*recordingSink
	finalizeCalls int
	finalUsage    *types.TurnUsage
	finalEvents   []types.Event
	finalizeErr   error
}

func (s *recordingFinalizingSink) Emit(ctx context.Context, event types.Event) error {
	if s.recordingSink == nil {
		s.recordingSink = &recordingSink{}
	}
	return s.recordingSink.Emit(ctx, event)
}

func (s *recordingFinalizingSink) FinalizeTurn(_ context.Context, usage *types.TurnUsage, events []types.Event) error {
	s.finalizeCalls++
	if usage != nil {
		cloned := *usage
		s.finalUsage = &cloned
	}
	s.finalEvents = append([]types.Event(nil), events...)
	return s.finalizeErr
}

func assertEventTypes(t *testing.T, sink *recordingSink, want []string) {
	t.Helper()

	got := sink.eventTypes()
	if len(got) != len(want) {
		t.Fatalf("len(event types) = %d, want %d; got %v", len(got), len(want), got)
	}

	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("event types = %v, want %v", got, want)
		}
	}
}

func assertNoEventType(t *testing.T, sink *recordingSink, unwanted string) {
	t.Helper()

	for _, eventType := range sink.eventTypes() {
		if eventType == unwanted {
			t.Fatalf("event types = %v, unexpected %q", sink.eventTypes(), unwanted)
		}
	}
}

func assertHasEventType(t *testing.T, sink *recordingSink, wanted string) {
	t.Helper()

	for _, eventType := range sink.eventTypes() {
		if eventType == wanted {
			return
		}
	}
	t.Fatalf("event types = %v, want %q", sink.eventTypes(), wanted)
}

type fakeConversationStore struct {
	items                     []model.ConversationItem
	summaries                 []model.Summary
	memories                  []types.MemoryEntry
	pendingTaskCompletions    []types.PendingTaskCompletion
	pendingReportMailboxItems []types.ReportMailboxItem
	claimedTaskCompletions    []types.PendingTaskCompletion
	claimedCompletionSession  string
	claimedCompletionTurn     string
	claimedReportMailboxItems []types.ReportMailboxItem
	claimedReportSession      string
	claimedReportTurn         string
	deletedMemoryIDs          []string
	sessionMemory             *types.SessionMemory
	cacheHead                 *types.ProviderCacheHead
	compactions               []types.ConversationCompaction
	upsertedUsage             *types.TurnUsage
	upsertedSessionMemory     *types.SessionMemory
	upsertedMemoryEntry       *types.MemoryEntry
	upsertedMemoryEntries     []types.MemoryEntry
	upsertUsageCalls          int
	upsertedHead              *types.ProviderCacheHead
	insertedItems             []model.ConversationItem
	insertedPositions         []int
	insertedSummaries         []model.Summary
	insertedSummaryPos        []int
	insertedCompactions       []types.ConversationCompaction
	afterUpsertSessionMemory  func()
}

func (s *fakeConversationStore) ListConversationItems(context.Context, string) ([]model.ConversationItem, error) {
	return append([]model.ConversationItem(nil), s.items...), nil
}

func (s *fakeConversationStore) ListConversationSummaries(context.Context, string) ([]model.Summary, error) {
	return append([]model.Summary(nil), s.summaries...), nil
}

func (s *fakeConversationStore) ListConversationCompactions(context.Context, string) ([]types.ConversationCompaction, error) {
	return append([]types.ConversationCompaction(nil), s.compactions...), nil
}

func (s *fakeConversationStore) GetSessionMemory(context.Context, string) (types.SessionMemory, bool, error) {
	if s.sessionMemory == nil {
		return types.SessionMemory{}, false, nil
	}
	return *s.sessionMemory, true, nil
}

func (s *fakeConversationStore) InsertConversationItem(_ context.Context, _ string, _ string, position int, item model.ConversationItem) error {
	s.insertedItems = append(s.insertedItems, item)
	s.insertedPositions = append(s.insertedPositions, position)
	s.items = append(s.items, item)
	return nil
}

func (s *fakeConversationStore) InsertConversationSummary(_ context.Context, _ string, position int, summary model.Summary) error {
	s.insertedSummaries = append(s.insertedSummaries, summary)
	s.insertedSummaryPos = append(s.insertedSummaryPos, position)
	s.summaries = append(s.summaries, summary)
	return nil
}

func (s *fakeConversationStore) ListMemoryEntriesByWorkspace(context.Context, string) ([]types.MemoryEntry, error) {
	return append([]types.MemoryEntry(nil), s.memories...), nil
}

func (s *fakeConversationStore) GetProviderCacheHead(context.Context, string, string, string) (types.ProviderCacheHead, bool, error) {
	if s.cacheHead == nil {
		return types.ProviderCacheHead{}, false, nil
	}
	return *s.cacheHead, true, nil
}

func (s *fakeConversationStore) UpsertProviderCacheHead(_ context.Context, head types.ProviderCacheHead) error {
	cloned := head
	s.upsertedHead = &cloned
	return nil
}

func (s *fakeConversationStore) UpsertTurnUsage(_ context.Context, usage types.TurnUsage) error {
	cloned := usage
	s.upsertedUsage = &cloned
	s.upsertUsageCalls++
	return nil
}

func (s *fakeConversationStore) UpsertSessionMemory(_ context.Context, memory types.SessionMemory) error {
	cloned := memory
	s.upsertedSessionMemory = &cloned
	s.sessionMemory = &cloned
	if s.afterUpsertSessionMemory != nil {
		s.afterUpsertSessionMemory()
	}
	return nil
}

func (s *fakeConversationStore) UpsertMemoryEntry(_ context.Context, entry types.MemoryEntry) error {
	cloned := entry
	s.upsertedMemoryEntry = &cloned
	s.upsertedMemoryEntries = append(s.upsertedMemoryEntries, cloned)
	replaced := false
	for i := range s.memories {
		if s.memories[i].ID == entry.ID {
			s.memories[i] = cloned
			replaced = true
			break
		}
	}
	if !replaced {
		s.memories = append(s.memories, cloned)
	}
	return nil
}

func (s *fakeConversationStore) DeleteMemoryEntries(_ context.Context, ids []string) error {
	for _, id := range ids {
		s.deletedMemoryIDs = append(s.deletedMemoryIDs, id)
		filtered := s.memories[:0]
		for _, entry := range s.memories {
			if entry.ID == id {
				continue
			}
			filtered = append(filtered, entry)
		}
		s.memories = filtered
	}
	return nil
}

func (s *fakeConversationStore) ClaimPendingTaskCompletionsForTurn(_ context.Context, sessionID, turnID string) ([]types.PendingTaskCompletion, error) {
	if s.claimedCompletionSession == sessionID && s.claimedCompletionTurn == turnID {
		return clonePendingTaskCompletions(s.claimedTaskCompletions), nil
	}
	s.claimedCompletionSession = sessionID
	s.claimedCompletionTurn = turnID
	if len(s.pendingTaskCompletions) == 0 {
		s.claimedTaskCompletions = nil
		return nil, nil
	}
	claimed := clonePendingTaskCompletions(s.pendingTaskCompletions)
	now := time.Now().UTC()
	for index := range claimed {
		claimed[index].InjectedTurnID = turnID
		if claimed[index].InjectedAt.IsZero() {
			claimed[index].InjectedAt = now
		}
	}
	s.claimedTaskCompletions = claimed
	s.pendingTaskCompletions = nil
	return clonePendingTaskCompletions(claimed), nil
}

func (s *fakeConversationStore) ClaimPendingReportMailboxItemsForTurn(_ context.Context, sessionID, turnID string) ([]types.ReportMailboxItem, error) {
	if s.claimedReportSession == sessionID && s.claimedReportTurn == turnID {
		return cloneReportMailboxItems(s.claimedReportMailboxItems), nil
	}
	s.claimedReportSession = sessionID
	s.claimedReportTurn = turnID
	if len(s.pendingReportMailboxItems) == 0 {
		s.claimedReportMailboxItems = nil
		return nil, nil
	}
	claimed := cloneReportMailboxItems(s.pendingReportMailboxItems)
	now := time.Now().UTC()
	for index := range claimed {
		claimed[index].InjectedTurnID = turnID
		if claimed[index].InjectedAt.IsZero() {
			claimed[index].InjectedAt = now
		}
	}
	s.claimedReportMailboxItems = claimed
	s.pendingReportMailboxItems = nil
	return cloneReportMailboxItems(claimed), nil
}

func findMemoryByID(entries []types.MemoryEntry, id string) *types.MemoryEntry {
	for i := range entries {
		if entries[i].ID == id {
			return &entries[i]
		}
	}
	return nil
}

func clonePendingTaskCompletions(items []types.PendingTaskCompletion) []types.PendingTaskCompletion {
	if len(items) == 0 {
		return nil
	}
	out := make([]types.PendingTaskCompletion, len(items))
	copy(out, items)
	return out
}

func cloneReportMailboxItems(items []types.ReportMailboxItem) []types.ReportMailboxItem {
	if len(items) == 0 {
		return nil
	}
	out := make([]types.ReportMailboxItem, len(items))
	copy(out, items)
	return out
}

func (s *fakeConversationStore) InsertProviderCacheEntry(context.Context, types.ProviderCacheEntry) error {
	return nil
}

func (s *fakeConversationStore) InsertConversationCompaction(_ context.Context, compaction types.ConversationCompaction) error {
	s.insertedCompactions = append(s.insertedCompactions, compaction)
	s.compactions = append(s.compactions, compaction)
	return nil
}

type recordingCompactor struct {
	summary model.Summary
	calls   int
	inputs  [][]model.ConversationItem
}

func (c *recordingCompactor) Compact(_ context.Context, items []model.ConversationItem) (model.Summary, error) {
	c.calls++
	c.inputs = append(c.inputs, cloneConversationItemsForPrompt(items))
	return c.summary, nil
}

type erroringCompactor struct {
	err error
}

func (c *erroringCompactor) Compact(context.Context, []model.ConversationItem) (model.Summary, error) {
	return model.Summary{}, c.err
}

type blockingCompactor struct {
	summary model.Summary
	block   <-chan struct{}
	started chan<- struct{}
}

func (c *blockingCompactor) Compact(context.Context, []model.ConversationItem) (model.Summary, error) {
	if c.started != nil {
		select {
		case c.started <- struct{}{}:
		default:
		}
	}
	<-c.block
	return c.summary, nil
}

type recordingSessionMemoryWorker struct {
	enqueueCalls int
	lastInput    Input
}

func (w *recordingSessionMemoryWorker) Enqueue(_ context.Context, _ *Engine, in Input) {
	w.enqueueCalls++
	w.lastInput = in
}

func (w *recordingSessionMemoryWorker) Wait() {}

func conversationItemKinds(items []model.ConversationItem) []model.ConversationItemKind {
	kinds := make([]model.ConversationItemKind, 0, len(items))
	for _, item := range items {
		kinds = append(kinds, item.Kind)
	}
	return kinds
}

func writeFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
}
