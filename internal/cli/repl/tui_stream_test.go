package repl

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"go-agent/internal/types"
)

func TestForwardTUIStreamEventsBatchesAssistantDeltas(t *testing.T) {
	events := make(chan types.Event, 2)
	events <- newAssistantDeltaEvent(1, "sess_1", "turn_1", "hello ")
	events <- newAssistantDeltaEvent(2, "sess_1", "turn_1", "world")
	close(events)

	out := make(chan tea.Msg, 2)
	lastSeq := int64(0)

	if ok := forwardTUIStreamEvents(context.Background(), "sess_1", events, out, &lastSeq); !ok {
		t.Fatal("forwardTUIStreamEvents() = false, want true")
	}

	msgs := drainTUIStreamMessages(out)
	if len(msgs) != 1 {
		t.Fatalf("len(msgs) = %d, want 1", len(msgs))
	}
	if lastSeq != 2 {
		t.Fatalf("lastSeq = %d, want 2", lastSeq)
	}
	if msgs[0].sessionID != "sess_1" {
		t.Fatalf("msgs[0].sessionID = %q, want sess_1", msgs[0].sessionID)
	}
	if msgs[0].event.Seq != 2 {
		t.Fatalf("msgs[0].event.Seq = %d, want 2", msgs[0].event.Seq)
	}

	var payload types.AssistantDeltaPayload
	if err := json.Unmarshal(msgs[0].event.Payload, &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if payload.Text != "hello world" {
		t.Fatalf("payload.Text = %q, want %q", payload.Text, "hello world")
	}
}

func TestForwardTUIStreamEventsFlushesBeforeDifferentTurns(t *testing.T) {
	events := make(chan types.Event, 2)
	events <- newAssistantDeltaEvent(1, "sess_1", "turn_1", "alpha")
	events <- newAssistantDeltaEvent(2, "sess_1", "turn_2", "beta")
	close(events)

	out := make(chan tea.Msg, 3)
	lastSeq := int64(0)

	if ok := forwardTUIStreamEvents(context.Background(), "sess_1", events, out, &lastSeq); !ok {
		t.Fatal("forwardTUIStreamEvents() = false, want true")
	}

	msgs := drainTUIStreamMessages(out)
	if len(msgs) != 2 {
		t.Fatalf("len(msgs) = %d, want 2", len(msgs))
	}
	if got := assistantDeltaText(t, msgs[0].event); got != "alpha" {
		t.Fatalf("assistantDeltaText(msgs[0]) = %q, want alpha", got)
	}
	if got := assistantDeltaText(t, msgs[1].event); got != "beta" {
		t.Fatalf("assistantDeltaText(msgs[1]) = %q, want beta", got)
	}
}

func newAssistantDeltaEvent(seq int64, sessionID, turnID, text string) types.Event {
	raw, err := json.Marshal(types.AssistantDeltaPayload{Text: text})
	if err != nil {
		panic(err)
	}
	return types.Event{
		ID:        "evt",
		Seq:       seq,
		SessionID: sessionID,
		TurnID:    turnID,
		Type:      types.EventAssistantDelta,
		Time:      time.Unix(0, 0).UTC(),
		Payload:   raw,
	}
}

func assistantDeltaText(t *testing.T, event types.Event) string {
	t.Helper()
	var payload types.AssistantDeltaPayload
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	return payload.Text
}

func drainTUIStreamMessages(out <-chan tea.Msg) []tuiStreamEventMsg {
	msgs := []tuiStreamEventMsg{}
	for {
		select {
		case msg := <-out:
			streamMsg, ok := msg.(tuiStreamEventMsg)
			if !ok {
				continue
			}
			msgs = append(msgs, streamMsg)
		default:
			return msgs
		}
	}
}
