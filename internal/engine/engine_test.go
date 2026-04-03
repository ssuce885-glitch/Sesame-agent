package engine

import (
	"context"
	"errors"
	"testing"

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

	runner := New(fakeModel, tools.NewRegistry(), permissions.NewEngine())
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
	}, tools.NewRegistry(), permissions.NewEngine())

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

func TestRunTurnEmitsTurnFailedForToolCallStreamEvent(t *testing.T) {
	sink := &recordingSink{}
	runner := New(model.NewFakeStreaming([][]model.StreamEvent{
		{
			{Kind: model.StreamEventToolCallStart, ToolCall: model.ToolCallChunk{ID: "call_1", Name: "search"}},
		},
	}), tools.NewRegistry(), permissions.NewEngine())

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
		types.EventTurnFailed,
	})
	assertNoEventType(t, sink, types.EventTurnCompleted)
	assertNoEventType(t, sink, types.EventAssistantCompleted)
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
