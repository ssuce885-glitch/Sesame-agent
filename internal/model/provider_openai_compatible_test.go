package model

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"go-agent/internal/config"
)

func TestNewOpenAICompatibleProviderRejectsMissingAPIKey(t *testing.T) {
	_, err := NewOpenAICompatibleProvider(Config{
		Model: "gpt-4.1-mini",
	})
	if err == nil {
		t.Fatal("expected error for missing API key")
	}
}

func TestNewOpenAICompatibleProviderRejectsMissingModel(t *testing.T) {
	_, err := NewOpenAICompatibleProvider(Config{
		APIKey: "test-key",
	})
	if err == nil {
		t.Fatal("expected error for missing model")
	}
}

func TestOpenAICompatibleProviderStreamNormalizesTextAndMessageEnd(t *testing.T) {
	type requestBody struct {
		Model    string `json:"model"`
		Stream   bool   `json:"stream"`
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
	}

	var gotRequest requestBody
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("path = %s, want /v1/chat/completions", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Fatalf("authorization = %q, want %q", got, "Bearer test-key")
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Fatalf("content-type = %q, want %q", got, "application/json")
		}

		if err := json.NewDecoder(r.Body).Decode(&gotRequest); err != nil {
			t.Fatalf("decode request: %v", err)
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"Hel\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"lo\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	provider, err := NewOpenAICompatibleProvider(Config{
		APIKey:  "test-key",
		Model:   "gpt-4.1-mini",
		BaseURL: server.URL + "/",
	})
	if err != nil {
		t.Fatalf("NewOpenAICompatibleProvider() error = %v", err)
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

	if gotRequest.Model != "gpt-4.1-mini" {
		t.Fatalf("request model = %q, want %q", gotRequest.Model, "gpt-4.1-mini")
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

func TestOpenAICompatibleProviderStreamReturnsBodyOnError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "invalid api key", http.StatusUnauthorized)
	}))
	defer server.Close()

	provider, err := NewOpenAICompatibleProvider(Config{
		APIKey:  "test-key",
		Model:   "gpt-4.1-mini",
		BaseURL: server.URL,
	})
	if err != nil {
		t.Fatalf("NewOpenAICompatibleProvider() error = %v", err)
	}

	events, errs := provider.Stream(context.Background(), Request{
		UserMessage: "hello",
	})

	for range events {
		t.Fatal("unexpected stream event on error response")
	}

	select {
	case err := <-errs:
		if err == nil {
			t.Fatal("stream error = nil, want error")
		}
		if !strings.Contains(err.Error(), "401") {
			t.Fatalf("stream error = %q, want status code", err.Error())
		}
		if !strings.Contains(err.Error(), "invalid api key") {
			t.Fatalf("stream error = %q, want body snippet", err.Error())
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timed out waiting for stream error result")
	}
}

func TestOpenAICompatibleProviderStreamTreatsFinishReasonAsMessageEnd(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"Hi\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: {\"choices\":[{\"finish_reason\":\"stop\"}]}\n\n"))
	}))
	defer server.Close()

	provider, err := NewOpenAICompatibleProvider(Config{
		APIKey:  "test-key",
		Model:   "gpt-4.1-mini",
		BaseURL: server.URL,
	})
	if err != nil {
		t.Fatalf("NewOpenAICompatibleProvider() error = %v", err)
	}

	events, errs := provider.Stream(context.Background(), Request{
		UserMessage: "hello",
	})

	var got []StreamEvent
	for event := range events {
		got = append(got, event)
	}

	if len(got) != 2 {
		t.Fatalf("len(events) = %d, want 2", len(got))
	}
	if got[0].Kind != StreamEventTextDelta || got[0].TextDelta != "Hi" {
		t.Fatalf("first event = %+v, want text delta Hi", got[0])
	}
	if got[1].Kind != StreamEventMessageEnd {
		t.Fatalf("second event = %+v, want message end", got[1])
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

func TestNewFromConfigSupportsOpenAICompatible(t *testing.T) {
	got, err := NewFromConfig(config.Config{
		ModelProvider: "openai_compatible",
		Model:         "gpt-4.1-mini",
		OpenAIAPIKey:  "test-key",
		OpenAIBaseURL: "https://example.com",
	})
	if err != nil {
		t.Fatalf("NewFromConfig() error = %v", err)
	}
	if got == nil {
		t.Fatal("NewFromConfig() returned nil client")
	}
}
