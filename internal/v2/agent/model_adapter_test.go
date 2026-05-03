package agent

import (
	"context"
	"strings"
	"testing"

	"go-agent/internal/model"
	"go-agent/internal/v2/contracts"
)

func TestThinkingBlockRoundTrip(t *testing.T) {
	items := []model.ConversationItem{{
		Kind:              model.ConversationItemAssistantThinking,
		Text:              "reasoning text",
		ThinkingSignature: "sig_123",
	}}

	msgs := fromModelItems(items, "turn_1")
	if len(msgs) != 1 {
		t.Fatalf("fromModelItems returned %d messages, want 1", len(msgs))
	}
	if msgs[0].Role != "assistant" {
		t.Fatalf("message role = %q, want assistant", msgs[0].Role)
	}
	if !strings.HasPrefix(msgs[0].Content, thinkingBlockPrefix) {
		t.Fatalf("message content = %q, want thinking prefix", msgs[0].Content)
	}

	got := toModelMessages(msgs)
	if len(got) != 1 {
		t.Fatalf("toModelMessages returned %d items, want 1", len(got))
	}
	if got[0].Kind != model.ConversationItemAssistantThinking {
		t.Fatalf("item kind = %q, want assistant thinking", got[0].Kind)
	}
	if got[0].Text != "reasoning text" {
		t.Fatalf("thinking text = %q, want reasoning text", got[0].Text)
	}
	if got[0].ThinkingSignature != "sig_123" {
		t.Fatalf("thinking signature = %q, want sig_123", got[0].ThinkingSignature)
	}
}

func TestCompactSummarySystemMessageIsSentAsContext(t *testing.T) {
	got := toModelMessages([]contracts.Message{
		{Role: "system", Content: compactBoundaryPrefix + "snapshot-1"},
		{Role: "system", Content: compactSummaryPrefix + "Compacted context."},
	})
	if len(got) != 1 {
		t.Fatalf("toModelMessages returned %d items, want 1", len(got))
	}
	if got[0].Kind != model.ConversationItemUserMessage || got[0].Text != "Compacted context." {
		t.Fatalf("unexpected compact summary item: %+v", got[0])
	}
}

func TestThinkingBlockRoundTripSignatureOnly(t *testing.T) {
	items := []model.ConversationItem{{
		Kind:              model.ConversationItemAssistantThinking,
		ThinkingSignature: "sig_only",
	}}

	msgs := fromModelItems(items, "turn_1")
	if len(msgs) != 1 {
		t.Fatalf("fromModelItems returned %d messages, want 1", len(msgs))
	}

	got := toModelMessages(msgs)
	if len(got) != 1 {
		t.Fatalf("toModelMessages returned %d items, want 1", len(got))
	}
	if got[0].Kind != model.ConversationItemAssistantThinking {
		t.Fatalf("item kind = %q, want assistant thinking", got[0].Kind)
	}
	if got[0].Text != "" {
		t.Fatalf("thinking text = %q, want empty", got[0].Text)
	}
	if got[0].ThinkingSignature != "sig_only" {
		t.Fatalf("thinking signature = %q, want sig_only", got[0].ThinkingSignature)
	}
}

func TestStreamOnceCollectsThinkingBlock(t *testing.T) {
	client := streamEventsClient{events: []model.StreamEvent{
		{Kind: model.StreamEventThinkingDelta, TextDelta: "step one"},
		{Kind: model.StreamEventThinkingDelta, TextDelta: " step two"},
		{Kind: model.StreamEventThinkingSignature, ThinkingSignature: "sig_123"},
		{Kind: model.StreamEventMessageEnd},
	}}
	agent := &Agent{client: client}

	items, calls, _, err := agent.streamOnceWithUsage(context.Background(), contracts.TurnInput{}, model.Request{})
	if err != nil {
		t.Fatalf("streamOnce returned error: %v", err)
	}
	if len(calls) != 0 {
		t.Fatalf("streamOnce returned %d tool calls, want 0", len(calls))
	}
	if len(items) != 1 {
		t.Fatalf("streamOnce returned %d items, want 1", len(items))
	}
	if items[0].Kind != model.ConversationItemAssistantThinking {
		t.Fatalf("item kind = %q, want assistant thinking", items[0].Kind)
	}
	if items[0].Text != "step one step two" {
		t.Fatalf("thinking text = %q, want combined text", items[0].Text)
	}
	if items[0].ThinkingSignature != "sig_123" {
		t.Fatalf("thinking signature = %q, want sig_123", items[0].ThinkingSignature)
	}
}

type streamEventsClient struct {
	events []model.StreamEvent
	err    error
}

func (c streamEventsClient) Stream(ctx context.Context, req model.Request) (<-chan model.StreamEvent, <-chan error) {
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

func (c streamEventsClient) Capabilities() model.ProviderCapabilities {
	return model.ProviderCapabilities{}
}
