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

func TestOpenAICompatibleProviderStreamUsesResponsesAPIAndNormalizesToolCalls(t *testing.T) {
	type requestBody struct {
		Model        string           `json:"model"`
		Instructions string           `json:"instructions"`
		Input        []map[string]any `json:"input"`
		Tools        []map[string]any `json:"tools"`
		Stream       bool             `json:"stream"`
	}

	var gotRequest requestBody
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/v1/responses" {
			t.Fatalf("path = %s, want /v1/responses", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Fatalf("authorization = %q, want %q", got, "Bearer test-key")
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Fatalf("content-type = %q, want %q", got, "application/json")
		}
		if got := r.Header.Get("Accept"); got != "text/event-stream" {
			t.Fatalf("accept = %q, want %q", got, "text/event-stream")
		}

		if err := json.NewDecoder(r.Body).Decode(&gotRequest); err != nil {
			t.Fatalf("decode request: %v", err)
		}

		writeEvent := func(event string, data any) {
			t.Helper()
			payload, err := json.Marshal(data)
			if err != nil {
				t.Fatalf("marshal %s payload: %v", event, err)
			}
			_, _ = w.Write([]byte("event: " + event + "\n"))
			_, _ = w.Write([]byte("data: "))
			_, _ = w.Write(payload)
			_, _ = w.Write([]byte("\n\n"))
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		writeEvent("response.output_text.delta", map[string]any{"delta": "Hel"})
		writeEvent("response.output_item.added", map[string]any{
			"item": map[string]any{
				"id":      "fc_1",
				"call_id": "call_1",
				"type":    "function_call",
				"name":    "file_read",
			},
		})
		writeEvent("response.output_item.done", map[string]any{
			"item": map[string]any{
				"id":      "fc_1",
				"call_id": "call_1",
				"type":    "function_call",
			},
		})
		writeEvent("response.function_call_arguments.delta", map[string]any{
			"item_id": "fc_1",
			"delta":   `{"path":"README.`,
		})
		writeEvent("response.function_call_arguments.done", map[string]any{
			"item_id":   "fc_1",
			"arguments": `{"path":"README.md"}`,
		})
		writeEvent("response.completed", map[string]any{"status": "completed"})
		writeEvent("response.completed", map[string]any{"status": "completed"})
	}))
	defer server.Close()

	provider, err := NewOpenAICompatibleProvider(Config{
		APIKey:  "test-key",
		Model:   "fallback-model",
		BaseURL: server.URL + "/",
	})
	if err != nil {
		t.Fatalf("NewOpenAICompatibleProvider() error = %v", err)
	}

	events, errs := provider.Stream(context.Background(), Request{
		Stream:       true,
		Instructions: "system rules",
		Items: []ConversationItem{
			UserMessageItem("hello"),
			{
				Kind: ConversationItemAssistantText,
				Text: "done",
			},
			{
				Kind: ConversationItemToolResult,
				Result: &ToolResult{
					ToolCallID: "call_1",
					ToolName:   "file_read",
					Content:    "README contents",
					IsError:    true,
				},
			},
			{
				Kind: ConversationItemSummary,
				Summary: &Summary{
					RangeLabel: "ignored",
				},
			},
		},
		Tools: []ToolSchema{{
			Name:        "file_read",
			Description: "Read a file",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path": map[string]any{"type": "string"},
				},
			},
		}},
	})

	var got []StreamEvent
	for event := range events {
		got = append(got, event)
	}

	if len(got) != 4 {
		t.Fatalf("len(events) = %d, want 4", len(got))
	}
	if got[0].Kind != StreamEventTextDelta || got[0].TextDelta != "Hel" {
		t.Fatalf("first event = %+v, want text delta Hel", got[0])
	}
	if got[1].Kind != StreamEventToolCallDelta || got[1].ToolCall.ID != "call_1" || got[1].ToolCall.Name != "file_read" || got[1].ToolCall.InputChunk != `{"path":"README.` {
		t.Fatalf("second event = %+v, want tool call delta", got[1])
	}
	if got[2].Kind != StreamEventToolCallEnd || got[2].ToolCall.ID != "call_1" || got[2].ToolCall.Name != "file_read" || got[2].ToolCall.Input["path"] != "README.md" {
		t.Fatalf("third event = %+v, want tool call end", got[2])
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

	if gotRequest.Model != "fallback-model" {
		t.Fatalf("request model = %q, want %q", gotRequest.Model, "fallback-model")
	}
	if gotRequest.Instructions != "system rules" {
		t.Fatalf("request instructions = %q, want %q", gotRequest.Instructions, "system rules")
	}
	if !gotRequest.Stream {
		t.Fatal("request stream = false, want true")
	}
	if len(gotRequest.Input) != 3 {
		t.Fatalf("len(request input) = %d, want 3", len(gotRequest.Input))
	}
	if gotRequest.Input[0]["role"] != "user" {
		t.Fatalf("input[0].role = %v, want user", gotRequest.Input[0]["role"])
	}
	if content, ok := gotRequest.Input[0]["content"].([]any); !ok || len(content) != 1 || content[0].(map[string]any)["type"] != "input_text" || content[0].(map[string]any)["text"] != "hello" {
		t.Fatalf("input[0].content = %#v, want user input_text hello", gotRequest.Input[0]["content"])
	}
	if gotRequest.Input[1]["role"] != "assistant" {
		t.Fatalf("input[1].role = %v, want assistant", gotRequest.Input[1]["role"])
	}
	if gotRequest.Input[1]["type"] != "message" {
		t.Fatalf("input[1].type = %v, want message", gotRequest.Input[1]["type"])
	}
	if content, ok := gotRequest.Input[1]["content"].([]any); !ok || len(content) != 1 || content[0].(map[string]any)["type"] != "output_text" || content[0].(map[string]any)["text"] != "done" {
		t.Fatalf("input[1].content = %#v, want assistant output_text done", gotRequest.Input[1]["content"])
	}
	if gotRequest.Input[2]["type"] != "function_call_output" {
		t.Fatalf("input[2].type = %v, want function_call_output", gotRequest.Input[2]["type"])
	}
	if gotRequest.Input[2]["call_id"] != "call_1" {
		t.Fatalf("input[2].call_id = %v, want call_1", gotRequest.Input[2]["call_id"])
	}
	if gotRequest.Input[2]["output"] != "README contents" {
		t.Fatalf("input[2].output = %v, want README contents", gotRequest.Input[2]["output"])
	}
	if _, ok := gotRequest.Input[2]["is_error"]; ok {
		t.Fatalf("input[2].is_error = %v, want omitted", gotRequest.Input[2]["is_error"])
	}
	if _, ok := gotRequest.Input[2]["name_hint"]; ok {
		t.Fatalf("input[2].name_hint = %v, want omitted", gotRequest.Input[2]["name_hint"])
	}
	if len(gotRequest.Tools) != 1 {
		t.Fatalf("len(request tools) = %d, want 1", len(gotRequest.Tools))
	}
	if gotRequest.Tools[0]["type"] != "function" {
		t.Fatalf("tools[0].type = %v, want function", gotRequest.Tools[0]["type"])
	}
	if gotRequest.Tools[0]["name"] != "file_read" {
		t.Fatalf("tools[0].name = %v, want file_read", gotRequest.Tools[0]["name"])
	}
	if gotRequest.Tools[0]["description"] != "Read a file" {
		t.Fatalf("tools[0].description = %v, want Read a file", gotRequest.Tools[0]["description"])
	}
	if params, ok := gotRequest.Tools[0]["parameters"].(map[string]any); !ok || params["type"] != "object" {
		t.Fatalf("tools[0].parameters = %#v, want object schema", gotRequest.Tools[0]["parameters"])
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
