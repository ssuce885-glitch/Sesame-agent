package model

import (
	"context"
	"testing"
)

func TestFakeStreamingClientEmitsEventsInOrder(t *testing.T) {
	client := NewFakeStreaming([][]StreamEvent{
		{
			{Kind: StreamEventTextDelta, TextDelta: "Hel"},
			{Kind: StreamEventToolCallStart, ToolCall: ToolCallChunk{ID: "call_1", Name: "search"}},
			{Kind: StreamEventToolCallDelta, ToolCall: ToolCallChunk{ID: "call_1", InputChunk: `{"query":"go`}},
			{Kind: StreamEventToolCallEnd, ToolCall: ToolCallChunk{ID: "call_1", Input: map[string]any{"query": "go"}}},
			{Kind: StreamEventUsage, Usage: &Usage{InputTokens: 12, OutputTokens: 3}},
			{Kind: StreamEventMessageEnd},
		},
	})

	events, errs := client.Stream(context.Background(), Request{UserMessage: "hello"})

	var got []StreamEvent
	for event := range events {
		got = append(got, event)
	}

	if len(got) != 6 {
		t.Fatalf("expected 6 events, got %d", len(got))
	}

	if got[0].Kind != StreamEventTextDelta || got[0].TextDelta != "Hel" {
		t.Fatalf("unexpected first event: %+v", got[0])
	}

	if got[1].Kind != StreamEventToolCallStart || got[1].ToolCall.ID != "call_1" || got[1].ToolCall.Name != "search" {
		t.Fatalf("unexpected second event: %+v", got[1])
	}

	if got[2].Kind != StreamEventToolCallDelta || got[2].ToolCall.InputChunk != `{"query":"go` {
		t.Fatalf("unexpected third event: %+v", got[2])
	}

	if got[3].Kind != StreamEventToolCallEnd || got[3].ToolCall.Input["query"] != "go" {
		t.Fatalf("unexpected fourth event: %+v", got[3])
	}

	if got[4].Kind != StreamEventUsage || got[4].Usage == nil || got[4].Usage.InputTokens != 12 || got[4].Usage.OutputTokens != 3 {
		t.Fatalf("unexpected fifth event: %+v", got[4])
	}

	if got[5].Kind != StreamEventMessageEnd {
		t.Fatalf("unexpected sixth event: %+v", got[5])
	}

	if err, ok := <-errs; !ok {
		t.Fatal("expected nil error to be sent before the errors channel closed")
	} else if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	if _, ok := <-errs; ok {
		t.Fatal("expected errors channel to be closed after the nil error")
	}
}
