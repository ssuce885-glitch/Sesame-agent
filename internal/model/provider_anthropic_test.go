package model

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestAnthropicProviderRejectsMissingAPIKey(t *testing.T) {
	_, err := NewAnthropicProvider(Config{
		APIKey: "",
		Model:  "claude-sonnet-4-5",
	})
	if err == nil {
		t.Fatal("expected error for missing API key")
	}
}

func TestAnthropicProviderCapabilitiesDefaultToNone(t *testing.T) {
	provider, err := NewAnthropicProvider(Config{
		APIKey: "test-key",
		Model:  "claude-sonnet-4-5",
	})
	if err != nil {
		t.Fatalf("NewAnthropicProvider() error = %v", err)
	}

	if caps := provider.Capabilities(); caps.Profile != CapabilityProfileNone {
		t.Fatalf("Capabilities().Profile = %q, want %q", caps.Profile, CapabilityProfileNone)
	}
}

func TestAnthropicProviderStreamNormalizesTextAndMessageEnd(t *testing.T) {
	type requestBody struct {
		Model     string `json:"model"`
		MaxTokens int    `json:"max_tokens"`
		Stream    bool   `json:"stream"`
		System    string `json:"system"`
		Tools     []struct {
			Name        string         `json:"name"`
			Description string         `json:"description"`
			InputSchema map[string]any `json:"input_schema"`
		} `json:"tools"`
		Messages []struct {
			Role    string `json:"role"`
			Content []struct {
				Type      string `json:"type"`
				Text      string `json:"text,omitempty"`
				ToolUseID string `json:"tool_use_id,omitempty"`
				Content   string `json:"content,omitempty"`
				IsError   bool   `json:"is_error,omitempty"`
			} `json:"content"`
		} `json:"messages"`
	}

	var gotRequest requestBody
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/v1/messages" {
			t.Fatalf("path = %s, want /v1/messages", r.URL.Path)
		}
		if got := r.Header.Get("x-api-key"); got != "test-key" {
			t.Fatalf("x-api-key = %q, want %q", got, "test-key")
		}
		if got := r.Header.Get("anthropic-version"); got != "2023-06-01" {
			t.Fatalf("anthropic-version = %q, want %q", got, "2023-06-01")
		}
		if got := r.Header.Get("content-type"); got != "application/json" {
			t.Fatalf("content-type = %q, want %q", got, "application/json")
		}

		if err := json.NewDecoder(r.Body).Decode(&gotRequest); err != nil {
			t.Fatalf("decode request: %v", err)
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("event: message_start\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"message_start\"}\n\n"))
		_, _ = w.Write([]byte("event: content_block_delta\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":\"Hel\"}}\n\n"))
		_, _ = w.Write([]byte("event: ping\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"ping\"}\n\n"))
		_, _ = w.Write([]byte("event: content_block_delta\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":\"lo\"}}\n\n"))
		_, _ = w.Write([]byte("event: message_stop\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"message_stop\"}\n\n"))
	}))
	defer server.Close()

	provider, err := NewAnthropicProvider(Config{
		APIKey:  "test-key",
		Model:   "claude-sonnet-4-5",
		BaseURL: server.URL,
	})
	if err != nil {
		t.Fatalf("NewAnthropicProvider() error = %v", err)
	}

	events, errs := provider.Stream(context.Background(), Request{
		Model:        "claude-override",
		Instructions: "system rules",
		Stream:       true,
		Items: []ConversationItem{
			UserMessageItem("hello"),
			{
				Kind: ConversationItemAssistantText,
				Text: "prior answer",
			},
			ToolResultItem(ToolResult{
				ToolCallID: "tool_1",
				ToolName:   "glob",
				Content:    "[\"main.go\"]",
			}),
		},
		Tools: []ToolSchema{{
			Name:        "glob",
			Description: "List files matching a glob pattern",
			InputSchema: map[string]any{"type": "object"},
		}},
	})

	var got []StreamEvent
	for event := range events {
		got = append(got, event)
	}

	if len(got) != 3 {
		t.Fatalf("len(events) = %d, want 3", len(got))
	}
	if got[0].Kind != StreamEventTextDelta || got[0].TextDelta != "Hel" {
		t.Fatalf("first event = %+v, want text delta Hel", got[0])
	}
	if got[1].Kind != StreamEventTextDelta || got[1].TextDelta != "lo" {
		t.Fatalf("second event = %+v, want text delta lo", got[1])
	}
	if got[2].Kind != StreamEventMessageEnd {
		t.Fatalf("third event = %+v, want message end", got[2])
	}

	select {
	case err := <-errs:
		if err != nil {
			t.Fatalf("stream error = %v, want nil", err)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timed out waiting for stream error result")
	}

	if gotRequest.Model != "claude-override" {
		t.Fatalf("request model = %q, want %q", gotRequest.Model, "claude-override")
	}
	if gotRequest.MaxTokens != 1024 {
		t.Fatalf("request max_tokens = %d, want 1024", gotRequest.MaxTokens)
	}
	if !gotRequest.Stream {
		t.Fatal("request stream = false, want true")
	}
	if gotRequest.System != "system rules" {
		t.Fatalf("request system = %q, want %q", gotRequest.System, "system rules")
	}
	if len(gotRequest.Tools) != 1 {
		t.Fatalf("len(request tools) = %d, want 1", len(gotRequest.Tools))
	}
	if gotRequest.Tools[0].Name != "glob" || gotRequest.Tools[0].Description == "" {
		t.Fatalf("request tool = %#v, want glob definition", gotRequest.Tools[0])
	}
	if len(gotRequest.Messages) != 3 {
		t.Fatalf("len(request messages) = %d, want 3", len(gotRequest.Messages))
	}
	if gotRequest.Messages[0].Role != "user" || len(gotRequest.Messages[0].Content) != 1 || gotRequest.Messages[0].Content[0].Text != "hello" {
		t.Fatalf("request user message = %#v, want text content block", gotRequest.Messages[0])
	}
	if gotRequest.Messages[1].Role != "assistant" || len(gotRequest.Messages[1].Content) != 1 || gotRequest.Messages[1].Content[0].Text != "prior answer" {
		t.Fatalf("request assistant message = %#v, want assistant text content block", gotRequest.Messages[1])
	}
	if gotRequest.Messages[2].Role != "user" || len(gotRequest.Messages[2].Content) != 1 {
		t.Fatalf("request tool result message = %#v, want user tool_result block", gotRequest.Messages[2])
	}
	if gotRequest.Messages[2].Content[0].Type != "tool_result" || gotRequest.Messages[2].Content[0].ToolUseID != "tool_1" || gotRequest.Messages[2].Content[0].Content != "[\"main.go\"]" {
		t.Fatalf("request tool result block = %#v, want tool_result round-trip", gotRequest.Messages[2].Content[0])
	}
}

func TestAnthropicProviderStreamNormalizesToolUseEvents(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("event: content_block_start\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"tool_use\",\"id\":\"toolu_1\",\"name\":\"glob\"}}\n\n"))
		_, _ = w.Write([]byte("event: content_block_delta\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"input_json_delta\",\"partial_json\":\"{\\\"pattern\\\":\\\"\"}}\n\n"))
		_, _ = w.Write([]byte("event: content_block_delta\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"input_json_delta\",\"partial_json\":\"*.go\\\"}\"}}\n\n"))
		_, _ = w.Write([]byte("event: content_block_stop\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"content_block_stop\",\"index\":0}\n\n"))
		_, _ = w.Write([]byte("event: message_stop\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"message_stop\"}\n\n"))
	}))
	defer server.Close()

	provider, err := NewAnthropicProvider(Config{
		APIKey:  "test-key",
		Model:   "claude-sonnet-4-5",
		BaseURL: server.URL,
	})
	if err != nil {
		t.Fatalf("NewAnthropicProvider() error = %v", err)
	}

	events, errs := provider.Stream(context.Background(), Request{
		Items: []ConversationItem{UserMessageItem("list go files")},
	})

	var got []StreamEvent
	for event := range events {
		got = append(got, event)
	}

	if len(got) != 4 {
		t.Fatalf("len(events) = %d, want 4", len(got))
	}
	if got[0].Kind != StreamEventToolCallDelta || got[0].ToolCall.Name != "glob" || got[0].ToolCall.ID != "toolu_1" {
		t.Fatalf("first event = %+v, want first tool-call delta", got[0])
	}
	if got[1].Kind != StreamEventToolCallDelta || got[1].ToolCall.InputChunk != "*.go\"}" {
		t.Fatalf("second event = %+v, want second tool-call delta", got[1])
	}
	if got[2].Kind != StreamEventToolCallEnd || got[2].ToolCall.Input["pattern"] != "*.go" {
		t.Fatalf("third event = %+v, want tool-call end with parsed JSON", got[2])
	}
	if got[3].Kind != StreamEventMessageEnd {
		t.Fatalf("fourth event = %+v, want message end", got[3])
	}

	select {
	case err := <-errs:
		if err != nil {
			t.Fatalf("stream error = %v, want nil", err)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timed out waiting for stream error result")
	}
}

func TestAnthropicProviderStreamFailsWhenEOFBeforeMessageStop(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("event: content_block_delta\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":\"Hel\"}}\n\n"))
	}))
	defer server.Close()

	provider, err := NewAnthropicProvider(Config{
		APIKey:  "test-key",
		Model:   "claude-sonnet-4-5",
		BaseURL: server.URL,
	})
	if err != nil {
		t.Fatalf("NewAnthropicProvider() error = %v", err)
	}

	events, errs := provider.Stream(context.Background(), Request{
		Items: []ConversationItem{UserMessageItem("hello")},
	})

	var got []StreamEvent
	for event := range events {
		got = append(got, event)
	}

	if len(got) != 1 {
		t.Fatalf("len(events) = %d, want 1", len(got))
	}
	if got[0].Kind != StreamEventTextDelta || got[0].TextDelta != "Hel" {
		t.Fatalf("first event = %+v, want text delta Hel", got[0])
	}

	select {
	case err := <-errs:
		if err == nil {
			t.Fatal("stream error = nil, want error")
		}
		if !strings.Contains(err.Error(), "message_stop") {
			t.Fatalf("stream error = %q, want mention of message_stop", err.Error())
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timed out waiting for stream error result")
	}
}
