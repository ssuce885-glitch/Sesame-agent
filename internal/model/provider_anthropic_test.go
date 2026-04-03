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

func TestAnthropicProviderStreamNormalizesTextAndMessageEnd(t *testing.T) {
	type requestBody struct {
		Model     string `json:"model"`
		MaxTokens int    `json:"max_tokens"`
		Stream    bool   `json:"stream"`
		Messages  []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
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
		UserMessage: "hello",
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

	if gotRequest.Model != "claude-sonnet-4-5" {
		t.Fatalf("request model = %q, want %q", gotRequest.Model, "claude-sonnet-4-5")
	}
	if gotRequest.MaxTokens != 1024 {
		t.Fatalf("request max_tokens = %d, want 1024", gotRequest.MaxTokens)
	}
	if !gotRequest.Stream {
		t.Fatal("request stream = false, want true")
	}
	if len(gotRequest.Messages) != 1 {
		t.Fatalf("len(request messages) = %d, want 1", len(gotRequest.Messages))
	}
	if gotRequest.Messages[0].Role != "user" {
		t.Fatalf("request message role = %q, want %q", gotRequest.Messages[0].Role, "user")
	}
	if gotRequest.Messages[0].Content != "hello" {
		t.Fatalf("request message content = %q, want %q", gotRequest.Messages[0].Content, "hello")
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
		UserMessage: "hello",
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
