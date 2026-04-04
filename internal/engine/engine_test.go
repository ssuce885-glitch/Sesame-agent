package engine

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	contextstate "go-agent/internal/context"
	"go-agent/internal/model"
	"go-agent/internal/permissions"
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
	}), tools.NewRegistry(), permissions.NewEngine(), nil, nil, nil, 0)

	err := runner.RunTurn(context.Background(), Input{
		Session: types.Session{ID: "sess_1", WorkspaceRoot: t.TempDir()},
		Turn:    types.Turn{ID: "turn_1", UserMessage: "hello"},
		Sink:    sink,
	})
	if err == nil {
		t.Fatal("RunTurn() error = nil, want non-nil")
	}

	assertEventTypes(t, sink, []string{
		types.EventTurnStarted,
		types.EventToolStarted,
		types.EventTurnFailed,
	})
	assertNoEventType(t, sink, types.EventTurnCompleted)
	assertNoEventType(t, sink, types.EventAssistantCompleted)
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
	if got.IsError {
		t.Fatal("tool result is_error = true, want false")
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
	runner := New(client, tools.NewRegistry(), permissions.NewEngine(), store, manager, nil, 8)

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
	runner := New(client, tools.NewRegistry(), permissions.NewEngine(), store, manager, nil, 8)

	err := runner.RunTurn(context.Background(), Input{
		Session: types.Session{ID: "sess_1", WorkspaceRoot: workspace},
		Turn:    types.Turn{ID: "turn_1", SessionID: "sess_1", UserMessage: "inspect the readme"},
		Sink:    &recordingSink{},
	})
	if err != nil {
		t.Fatalf("RunTurn() error = %v", err)
	}

	if len(store.insertedItems) != 4 {
		t.Fatalf("len(inserted items) = %d, want 4", len(store.insertedItems))
	}
	wantKinds := []model.ConversationItemKind{
		model.ConversationItemUserMessage,
		model.ConversationItemAssistantText,
		model.ConversationItemToolResult,
		model.ConversationItemAssistantText,
	}
	wantPositions := []int{1, 2, 3, 4}
	for i := range wantKinds {
		if store.insertedItems[i].Kind != wantKinds[i] {
			t.Fatalf("inserted item kinds = %#v, want %#v", conversationItemKinds(store.insertedItems), wantKinds)
		}
		if store.insertedPositions[i] != wantPositions[i] {
			t.Fatalf("inserted positions = %v, want %v", store.insertedPositions, wantPositions)
		}
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

type recordingStreamingClient struct {
	streams  [][]model.StreamEvent
	requests []model.Request
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

type fakeConversationStore struct {
	items             []model.ConversationItem
	summaries         []model.Summary
	memories          []types.MemoryEntry
	insertedItems     []model.ConversationItem
	insertedPositions []int
}

func (s *fakeConversationStore) ListConversationItems(context.Context, string) ([]model.ConversationItem, error) {
	return append([]model.ConversationItem(nil), s.items...), nil
}

func (s *fakeConversationStore) ListConversationSummaries(context.Context, string) ([]model.Summary, error) {
	return append([]model.Summary(nil), s.summaries...), nil
}

func (s *fakeConversationStore) InsertConversationItem(_ context.Context, _ string, _ string, position int, item model.ConversationItem) error {
	s.insertedItems = append(s.insertedItems, item)
	s.insertedPositions = append(s.insertedPositions, position)
	return nil
}

func (s *fakeConversationStore) InsertConversationSummary(context.Context, string, int, model.Summary) error {
	return nil
}

func (s *fakeConversationStore) ListMemoryEntriesByWorkspace(context.Context, string) ([]types.MemoryEntry, error) {
	return append([]types.MemoryEntry(nil), s.memories...), nil
}

func conversationItemKinds(items []model.ConversationItem) []model.ConversationItemKind {
	kinds := make([]model.ConversationItemKind, 0, len(items))
	for _, item := range items {
		kinds = append(kinds, item.Kind)
	}
	return kinds
}
