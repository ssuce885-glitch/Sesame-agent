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
	runner := New(modelClient, tools.NewRegistry(), permissions.NewEngine(), nil, nil, nil, 0)

	err := runner.RunTurn(context.Background(), Input{
		Session: types.Session{ID: "sess_1", WorkspaceRoot: workspace},
		Turn:    types.Turn{ID: "turn_1", UserMessage: "inspect the readme"},
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
	if len(req.Items) < 3 {
		t.Fatalf("len(req.Items) = %d, want at least 3", len(req.Items))
	}
	if !strings.Contains(req.Instructions, "workspace prefers rg for searches") {
		t.Fatalf("Instructions = %q, want recalled memory", req.Instructions)
	}
	var summaryItems int
	for _, item := range req.Items {
		if item.Kind == model.ConversationItemSummary {
			summaryItems++
			if item.Summary == nil || item.Summary.RangeLabel != "turns 1-1" {
				t.Fatalf("summary item = %#v, want stored summary", item)
			}
		}
	}
	if summaryItems != 1 {
		t.Fatalf("summary item count = %d, want 1", summaryItems)
	}
	if len(store.insertedPositions) != 1 || store.insertedPositions[0] != 3 {
		t.Fatalf("inserted positions = %v, want [3]", store.insertedPositions)
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
	insertedPositions []int
}

func (s *fakeConversationStore) ListConversationItems(context.Context, string) ([]model.ConversationItem, error) {
	return append([]model.ConversationItem(nil), s.items...), nil
}

func (s *fakeConversationStore) ListConversationSummaries(context.Context, string) ([]model.Summary, error) {
	return append([]model.Summary(nil), s.summaries...), nil
}

func (s *fakeConversationStore) InsertConversationItem(_ context.Context, _ string, _ string, position int, _ model.ConversationItem) error {
	s.insertedPositions = append(s.insertedPositions, position)
	return nil
}

func (s *fakeConversationStore) InsertConversationSummary(context.Context, string, int, model.Summary) error {
	return nil
}

func (s *fakeConversationStore) ListMemoryEntriesByWorkspace(context.Context, string) ([]types.MemoryEntry, error) {
	return append([]types.MemoryEntry(nil), s.memories...), nil
}
