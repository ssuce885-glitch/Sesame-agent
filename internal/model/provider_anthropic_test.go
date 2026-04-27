package model

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func TestAnthropicThinkingBlockIncludesEmptyThinkingWithSignature(t *testing.T) {
	raw, err := json.Marshal(anthropicContentBlock{
		Type:      "thinking",
		Signature: "sig_123",
	})
	if err != nil {
		t.Fatalf("Marshal returned error: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("Unmarshal returned error: %v", err)
	}
	if _, ok := payload["thinking"]; !ok {
		t.Fatalf("thinking field missing from %s", raw)
	}
	if got := payload["thinking"]; got != "" {
		t.Fatalf("thinking = %#v, want empty string", got)
	}
	if got := payload["signature"]; got != "sig_123" {
		t.Fatalf("signature = %#v, want sig_123", got)
	}
}

func TestAnthropicStreamEmitsSignatureOnlyThinkingStart(t *testing.T) {
	provider := &AnthropicProvider{
		apiKey:  "test-key",
		model:   "test-model",
		baseURL: "https://example.test",
		httpClient: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			body := "event: content_block_start\n" +
				"data: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"thinking\",\"signature\":\"sig_123\"}}\n\n" +
				"event: message_stop\n" +
				"data: {\"type\":\"message_stop\"}\n\n"
			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Body:       io.NopCloser(strings.NewReader(body)),
				Header:     make(http.Header),
				Request:    req,
			}, nil
		})},
	}

	events, errs := provider.Stream(context.Background(), Request{
		Stream: true,
		Items:  []ConversationItem{UserMessageItem("hello")},
	})
	var got []StreamEvent
	for event := range events {
		got = append(got, event)
	}
	if err := <-errs; err != nil {
		t.Fatalf("stream returned error: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("len(events) = %d, want 3: %#v", len(got), got)
	}
	if got[0].Kind != StreamEventThinkingDelta || got[0].TextDelta != "" {
		t.Fatalf("first event = %#v, want empty thinking delta", got[0])
	}
	if got[1].Kind != StreamEventThinkingSignature || got[1].ThinkingSignature != "sig_123" {
		t.Fatalf("second event = %#v, want thinking signature", got[1])
	}
	if got[2].Kind != StreamEventMessageEnd {
		t.Fatalf("third event = %#v, want message end", got[2])
	}
}

func TestToAnthropicMessagesUsesFallbackForSignatureOnlyThinking(t *testing.T) {
	messages := toAnthropicMessages([]ConversationItem{
		AssistantThinkingBlockItem("", "sig_123"),
	})
	if len(messages) != 1 {
		t.Fatalf("len(messages) = %d, want 1", len(messages))
	}
	if got := messages[0].Role; got != "assistant" {
		t.Fatalf("role = %q, want assistant", got)
	}
	if len(messages[0].Content) != 1 {
		t.Fatalf("len(content) = %d, want 1", len(messages[0].Content))
	}
	block := messages[0].Content[0]
	if got := block.Thinking; got != "(signature only)" {
		t.Fatalf("thinking = %q, want fallback", got)
	}
	if got := block.Signature; got != "sig_123" {
		t.Fatalf("signature = %q, want sig_123", got)
	}
}

func TestFinalizeAnthropicToolInputFallsBackToInitialInputAfterInvalidDelta(t *testing.T) {
	state := &anthropicToolCallState{
		ID:              "call_1",
		Name:            "skill_use",
		InitialInput:    map[string]any{"name": "automation-standard-behavior"},
		HasInitialInput: true,
		HasDelta:        true,
	}
	state.Input.WriteString("{")

	input, raw, recovery := finalizeAnthropicToolInput(state)
	if got := input["name"]; got != "automation-standard-behavior" {
		t.Fatalf("input[name] = %#v, want automation-standard-behavior", got)
	}
	if raw != "{" {
		t.Fatalf("raw = %q, want malformed delta", raw)
	}
	if recovery != "used_initial_input_after_invalid_delta_json" {
		t.Fatalf("recovery = %q", recovery)
	}
}

func TestFinalizeAnthropicToolInputReturnsRawInvalidDeltaWithoutInitialInput(t *testing.T) {
	state := &anthropicToolCallState{
		ID:       "call_1",
		Name:     "skill_use",
		HasDelta: true,
	}
	state.Input.WriteString("{")

	input, raw, recovery := finalizeAnthropicToolInput(state)
	if len(input) != 0 {
		t.Fatalf("input = %#v, want empty input", input)
	}
	if raw != "{" {
		t.Fatalf("raw = %q, want malformed delta", raw)
	}
	if recovery != "invalid_delta_json" {
		t.Fatalf("recovery = %q", recovery)
	}
}
