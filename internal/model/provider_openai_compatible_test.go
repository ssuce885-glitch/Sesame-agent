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
		ToolChoice   any              `json:"tool_choice"`
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
				Kind: ConversationItemToolCall,
				ToolCall: ToolCallChunk{
					ID:    "call_1",
					Name:  "file_read",
					Input: map[string]any{"path": "README.md"},
				},
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
	if gotRequest.ToolChoice != nil {
		if choice, ok := gotRequest.ToolChoice.(string); !ok || choice != "auto" {
			t.Fatalf("request tool_choice = %#v, want auto", gotRequest.ToolChoice)
		}
	}
	if len(gotRequest.Input) != 5 {
		t.Fatalf("len(request input) = %d, want 5", len(gotRequest.Input))
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
	if gotRequest.Input[1]["status"] != "completed" {
		t.Fatalf("input[1].status = %v, want completed", gotRequest.Input[1]["status"])
	}
	if content, ok := gotRequest.Input[1]["content"].([]any); !ok || len(content) != 1 || content[0].(map[string]any)["type"] != "output_text" || content[0].(map[string]any)["text"] != "done" {
		t.Fatalf("input[1].content = %#v, want assistant output_text done", gotRequest.Input[1]["content"])
	}
	if gotRequest.Input[2]["type"] != "function_call" {
		t.Fatalf("input[2].type = %v, want function_call", gotRequest.Input[2]["type"])
	}
	if gotRequest.Input[2]["call_id"] != "call_1" {
		t.Fatalf("input[2].call_id = %v, want call_1", gotRequest.Input[2]["call_id"])
	}
	if gotRequest.Input[2]["name"] != "file_read" {
		t.Fatalf("input[2].name = %v, want file_read", gotRequest.Input[2]["name"])
	}
	if gotRequest.Input[2]["arguments"] != `{"path":"README.md"}` {
		t.Fatalf("input[2].arguments = %v, want serialized tool arguments", gotRequest.Input[2]["arguments"])
	}
	if gotRequest.Input[3]["type"] != "function_call_output" {
		t.Fatalf("input[3].type = %v, want function_call_output", gotRequest.Input[3]["type"])
	}
	if gotRequest.Input[3]["call_id"] != "call_1" {
		t.Fatalf("input[3].call_id = %v, want call_1", gotRequest.Input[3]["call_id"])
	}
	if gotRequest.Input[3]["output"] != "README contents" {
		t.Fatalf("input[3].output = %v, want README contents", gotRequest.Input[3]["output"])
	}
	if _, ok := gotRequest.Input[3]["is_error"]; ok {
		t.Fatalf("input[3].is_error = %v, want omitted", gotRequest.Input[3]["is_error"])
	}
	if _, ok := gotRequest.Input[3]["name_hint"]; ok {
		t.Fatalf("input[3].name_hint = %v, want omitted", gotRequest.Input[3]["name_hint"])
	}
	if gotRequest.Input[4]["role"] != "assistant" {
		t.Fatalf("input[4].role = %v, want assistant summary", gotRequest.Input[4]["role"])
	}
	if content, ok := gotRequest.Input[4]["content"].([]any); !ok || len(content) != 1 || !strings.Contains(content[0].(map[string]any)["text"].(string), "[Conversation summary]") {
		t.Fatalf("input[4].content = %#v, want rendered summary", gotRequest.Input[4]["content"])
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

func TestToResponsesToolChoiceSupportsSpecificToolAndNone(t *testing.T) {
	tools := []ToolSchema{{Name: "list_dir"}}

	got := toResponsesToolChoice("list_dir", tools)
	choice, ok := got.(map[string]any)
	if !ok {
		t.Fatalf("specific tool choice = %#v, want object", got)
	}
	if choice["type"] != "function" || choice["name"] != "list_dir" {
		t.Fatalf("specific tool choice = %#v, want function/list_dir", choice)
	}

	got = toResponsesToolChoice("none", tools)
	if got != "none" {
		t.Fatalf("none tool choice = %#v, want none", got)
	}
}

func TestOpenAICompatibleProviderFallsBackToDeltaArgumentsWhenDonePayloadIsMalformed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
		writeEvent("response.output_item.added", map[string]any{
			"item": map[string]any{
				"id":      "fc_1",
				"call_id": "call_1",
				"type":    "function_call",
				"name":    "file_read",
			},
		})
		writeEvent("response.function_call_arguments.delta", map[string]any{
			"item_id": "fc_1",
			"delta":   `{"path":"README.md"}`,
		})
		writeEvent("response.function_call_arguments.done", map[string]any{
			"item_id":   "fc_1",
			"arguments": `{`,
		})
		writeEvent("response.completed", map[string]any{"status": "completed"})
	}))
	defer server.Close()

	provider, err := NewOpenAICompatibleProvider(Config{
		APIKey:  "test-key",
		Model:   "fallback-model",
		BaseURL: server.URL,
	})
	if err != nil {
		t.Fatalf("NewOpenAICompatibleProvider() error = %v", err)
	}

	events, errs := provider.Stream(context.Background(), Request{
		Stream: true,
		Items:  []ConversationItem{UserMessageItem("hello")},
	})

	var got []StreamEvent
	for event := range events {
		got = append(got, event)
	}

	if len(got) != 3 {
		t.Fatalf("len(events) = %d, want 3", len(got))
	}
	if got[1].Kind != StreamEventToolCallEnd {
		t.Fatalf("second event = %+v, want tool call end", got[1])
	}
	if got[1].ToolCall.Input["path"] != "README.md" {
		t.Fatalf("tool call input = %#v, want parsed fallback delta args", got[1].ToolCall.Input)
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

func TestOpenAICompatibleProviderDefersMalformedDoneUntilLaterDeltaCompletesArguments(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
		writeEvent("response.output_item.added", map[string]any{
			"item": map[string]any{
				"id":      "fc_1",
				"call_id": "call_1",
				"type":    "function_call",
				"name":    "shell_command",
			},
		})
		writeEvent("response.function_call_arguments.delta", map[string]any{
			"item_id": "fc_1",
			"delta":   `{`,
		})
		writeEvent("response.function_call_arguments.done", map[string]any{
			"item_id":   "fc_1",
			"arguments": `{`,
		})
		writeEvent("response.function_call_arguments.delta", map[string]any{
			"item_id": "fc_1",
			"delta":   `"command":"echo hi"}`,
		})
		writeEvent("response.completed", map[string]any{"status": "completed"})
	}))
	defer server.Close()

	provider, err := NewOpenAICompatibleProvider(Config{
		APIKey:  "test-key",
		Model:   "fallback-model",
		BaseURL: server.URL,
	})
	if err != nil {
		t.Fatalf("NewOpenAICompatibleProvider() error = %v", err)
	}

	events, errs := provider.Stream(context.Background(), Request{
		Stream: true,
		Items:  []ConversationItem{UserMessageItem("hello")},
	})

	var got []StreamEvent
	for event := range events {
		got = append(got, event)
	}

	if len(got) != 4 {
		t.Fatalf("len(events) = %d, want 4", len(got))
	}
	if got[0].Kind != StreamEventToolCallDelta || got[0].ToolCall.InputChunk != "{" {
		t.Fatalf("first event = %+v, want opening tool call delta", got[0])
	}
	if got[1].Kind != StreamEventToolCallDelta || got[1].ToolCall.InputChunk != `"command":"echo hi"}` {
		t.Fatalf("second event = %+v, want completing tool call delta", got[1])
	}
	if got[2].Kind != StreamEventToolCallEnd {
		t.Fatalf("third event = %+v, want tool call end", got[2])
	}
	if got[2].ToolCall.Input["command"] != "echo hi" {
		t.Fatalf("tool call input = %#v, want parsed command", got[2].ToolCall.Input)
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

func TestOpenAICompatibleProviderFinalizesToolCallFromDeltaOnlyAtResponseCompleted(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
		writeEvent("response.output_item.added", map[string]any{
			"item": map[string]any{
				"id":      "fc_1",
				"call_id": "call_1",
				"type":    "function_call",
				"name":    "shell_command",
			},
		})
		writeEvent("response.function_call_arguments.delta", map[string]any{
			"item_id": "fc_1",
			"delta":   `{"command":"echo hi"}`,
		})
		writeEvent("response.completed", map[string]any{"status": "completed"})
	}))
	defer server.Close()

	provider, err := NewOpenAICompatibleProvider(Config{
		APIKey:  "test-key",
		Model:   "fallback-model",
		BaseURL: server.URL,
	})
	if err != nil {
		t.Fatalf("NewOpenAICompatibleProvider() error = %v", err)
	}

	events, errs := provider.Stream(context.Background(), Request{
		Stream: true,
		Items:  []ConversationItem{UserMessageItem("hello")},
	})

	var got []StreamEvent
	for event := range events {
		got = append(got, event)
	}

	if len(got) != 3 {
		t.Fatalf("len(events) = %d, want 3", len(got))
	}
	if got[0].Kind != StreamEventToolCallDelta {
		t.Fatalf("first event = %+v, want tool call delta", got[0])
	}
	if got[1].Kind != StreamEventToolCallEnd {
		t.Fatalf("second event = %+v, want tool call end", got[1])
	}
	if got[1].ToolCall.Input["command"] != "echo hi" {
		t.Fatalf("tool call input = %#v, want parsed command", got[1].ToolCall.Input)
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
}

func TestOpenAICompatibleProviderUsesArkCacheFieldsAndMetadata(t *testing.T) {
	type requestBody map[string]any

	var gotRequest requestBody
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
		writeEvent("response.completed", map[string]any{
			"id": "resp_123",
			"usage": map[string]any{
				"input_tokens":  17,
				"output_tokens": 9,
				"prompt_tokens_details": map[string]any{
					"cached_tokens": 4,
				},
			},
		})
	}))
	defer server.Close()

	provider, err := NewOpenAICompatibleProvider(Config{
		APIKey:       "test-key",
		Model:        "fallback-model",
		BaseURL:      server.URL,
		CacheProfile: CapabilityProfileArkResponses,
	})
	if err != nil {
		t.Fatalf("NewOpenAICompatibleProvider() error = %v", err)
	}

	if caps := provider.Capabilities(); caps.Profile != CapabilityProfileArkResponses || !caps.SupportsSessionCache || caps.SupportsPrefixCache || !caps.CachesToolResults || !caps.RotatesSessionRef {
		t.Fatalf("Capabilities() = %+v, want ark cache capabilities", caps)
	}

	events, errs := provider.Stream(context.Background(), Request{
		Stream:       true,
		Instructions: "system rules",
		Items:        []ConversationItem{UserMessageItem("hello")},
		Cache: &CacheDirective{
			Mode:               CacheModeSession,
			Store:              true,
			PreviousResponseID: "resp_prev",
			ExpireAt:           1735689600,
		},
	})

	var got []StreamEvent
	for event := range events {
		got = append(got, event)
	}

	if len(got) != 2 {
		t.Fatalf("len(events) = %d, want 2", len(got))
	}
	if got[0].Kind != StreamEventResponseMetadata || got[0].ResponseMetadata == nil {
		t.Fatalf("first event = %+v, want response metadata", got[0])
	}
	if got[0].ResponseMetadata.ResponseID != "resp_123" || got[0].ResponseMetadata.InputTokens != 17 || got[0].ResponseMetadata.OutputTokens != 9 || got[0].ResponseMetadata.CachedTokens != 4 {
		t.Fatalf("response metadata = %+v, want resp_123/17/9/4", got[0].ResponseMetadata)
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

	if gotRequest["model"] != "fallback-model" {
		t.Fatalf("request model = %v, want %q", gotRequest["model"], "fallback-model")
	}
	if _, ok := gotRequest["instructions"]; ok {
		t.Fatalf("request instructions = %#v, want omitted for ark cache", gotRequest["instructions"])
	}
	if gotRequest["stream"] != true {
		t.Fatalf("request stream = %v, want true", gotRequest["stream"])
	}
	if gotRequest["store"] != true {
		t.Fatalf("request store = %#v, want true", gotRequest["store"])
	}
	if gotRequest["previous_response_id"] != "resp_prev" {
		t.Fatalf("request previous_response_id = %#v, want resp_prev", gotRequest["previous_response_id"])
	}
	caching, ok := gotRequest["caching"].(map[string]any)
	if !ok {
		t.Fatal("request caching = nil, want enabled cache body")
	}
	if caching["type"] != "enabled" {
		t.Fatalf("request caching.type = %v, want enabled", caching["type"])
	}
	if _, ok := caching["prefix"]; ok {
		t.Fatalf("request caching.prefix = %#v, want omitted for session cache", caching["prefix"])
	}
	if _, ok := caching["expire_at"]; ok {
		t.Fatalf("request caching.expire_at = %#v, want omitted for ark responses", caching["expire_at"])
	}
	input, ok := gotRequest["input"].([]any)
	if !ok || len(input) != 2 {
		t.Fatalf("request input = %#v, want 2 items (system + user)", gotRequest["input"])
	}
	systemItem, ok := input[0].(map[string]any)
	if !ok {
		t.Fatalf("input[0] = %#v, want object", input[0])
	}
	if systemItem["role"] != "system" {
		t.Fatalf("input[0].role = %v, want system", systemItem["role"])
	}
	systemContent, ok := systemItem["content"].([]any)
	if !ok || len(systemContent) != 1 {
		t.Fatalf("input[0].content = %#v, want single input_text", systemItem["content"])
	}
	systemText, ok := systemContent[0].(map[string]any)
	if !ok || systemText["type"] != "input_text" || systemText["text"] != "system rules" {
		t.Fatalf("input[0].content[0] = %#v, want system input_text", systemContent[0])
	}
	userItem, ok := input[1].(map[string]any)
	if !ok {
		t.Fatalf("input[1] = %#v, want object", input[1])
	}
	if userItem["role"] != "user" {
		t.Fatalf("input[1].role = %v, want user", userItem["role"])
	}
}

func TestToResponsesInputIncludesStructuredJSONForCompactToolResults(t *testing.T) {
	input := toResponsesInput([]ConversationItem{
		ToolResultItem(ToolResult{
			ToolCallID:     "call_1",
			ToolName:       "task_get",
			Content:        "Task found.",
			StructuredJSON: `{"task_id":"task_1","status":"running"}`,
		}),
	})

	if len(input) != 1 {
		t.Fatalf("len(input) = %d, want 1", len(input))
	}
	output, _ := input[0]["output"].(string)
	if !strings.Contains(output, "Task found.") || !strings.Contains(output, `"status":"running"`) {
		t.Fatalf("output = %q, want text plus structured json", output)
	}
}

func TestOpenAICompatibleProviderRetriesWithoutArkCachingWhenCacheServiceUnavailable(t *testing.T) {
	type requestBody map[string]any

	var requests []requestBody
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var got requestBody
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		requests = append(requests, got)

		if len(requests) == 1 {
			if _, ok := got["caching"]; !ok {
				t.Fatal("first request missing caching block")
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"error":{"code":"AccessDenied.CacheService","message":"cache service unavailable","type":"Forbidden"}}`))
			return
		}

		if _, ok := got["caching"]; ok {
			t.Fatalf("retry request caching = %#v, want omitted", got["caching"])
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
		writeEvent("response.output_text.delta", map[string]any{"delta": "PONG"})
		writeEvent("response.completed", map[string]any{
			"id": "resp_retry",
			"usage": map[string]any{
				"input_tokens":  5,
				"output_tokens": 1,
				"prompt_tokens_details": map[string]any{
					"cached_tokens": 0,
				},
			},
		})
	}))
	defer server.Close()

	provider, err := NewOpenAICompatibleProvider(Config{
		APIKey:       "test-key",
		Model:        "glm-4-7-251222",
		BaseURL:      server.URL,
		CacheProfile: CapabilityProfileArkResponses,
	})
	if err != nil {
		t.Fatalf("NewOpenAICompatibleProvider() error = %v", err)
	}

	events, errs := provider.Stream(context.Background(), Request{
		Stream:       true,
		Instructions: "system rules",
		Items:        []ConversationItem{UserMessageItem("hello")},
		Cache: &CacheDirective{
			Mode:  CacheModeSession,
			Store: true,
		},
	})

	var got []StreamEvent
	for event := range events {
		got = append(got, event)
	}

	if len(got) != 2 {
		t.Fatalf("len(events) = %d, want 2", len(got))
	}
	if got[0].Kind != StreamEventTextDelta || got[0].TextDelta != "PONG" {
		t.Fatalf("first event = %+v, want text delta PONG", got[0])
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

	if len(requests) != 2 {
		t.Fatalf("len(requests) = %d, want 2", len(requests))
	}
}

func TestOpenAICompatibleProviderOmitsToolsForArkCachedContinuation(t *testing.T) {
	type requestBody map[string]any

	var gotRequest requestBody
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&gotRequest); err != nil {
			t.Fatalf("decode request: %v", err)
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("event: response.completed\n"))
		_, _ = w.Write([]byte("data: {\"id\":\"resp_456\",\"usage\":{\"input_tokens\":3,\"output_tokens\":1,\"prompt_tokens_details\":{\"cached_tokens\":2}}}\n\n"))
	}))
	defer server.Close()

	provider, err := NewOpenAICompatibleProvider(Config{
		APIKey:       "test-key",
		Model:        "fallback-model",
		BaseURL:      server.URL,
		CacheProfile: CapabilityProfileArkResponses,
	})
	if err != nil {
		t.Fatalf("NewOpenAICompatibleProvider() error = %v", err)
	}

	events, errs := provider.Stream(context.Background(), Request{
		Stream:       true,
		Instructions: "system rules",
		Items:        []ConversationItem{UserMessageItem("hello")},
		Tools: []ToolSchema{{
			Name:        "file_read",
			Description: "Read a file",
			InputSchema: map[string]any{"type": "object"},
		}},
		Cache: &CacheDirective{
			Mode:               CacheModeSession,
			Store:              true,
			PreviousResponseID: "resp_prev",
		},
	})

	for range events {
	}

	select {
	case err := <-errs:
		if err != nil {
			t.Fatalf("stream error = %v, want nil", err)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timed out waiting for stream error result")
	}

	if _, ok := gotRequest["tools"]; ok {
		t.Fatalf("request tools = %#v, want omitted for ark cached continuation", gotRequest["tools"])
	}
}

func TestOpenAICompatibleProviderUsesArkResponsesPathForOfficialBaseURL(t *testing.T) {
	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("event: response.completed\n"))
		_, _ = w.Write([]byte("data: {\"id\":\"resp_123\",\"usage\":{\"input_tokens\":1,\"output_tokens\":1,\"prompt_tokens_details\":{\"cached_tokens\":0}}}\n\n"))
	}))
	defer server.Close()

	provider, err := NewOpenAICompatibleProvider(Config{
		APIKey:       "test-key",
		Model:        "glm-4-7-251222",
		BaseURL:      server.URL + "/api/v3",
		CacheProfile: CapabilityProfileArkResponses,
	})
	if err != nil {
		t.Fatalf("NewOpenAICompatibleProvider() error = %v", err)
	}

	events, errs := provider.Stream(context.Background(), Request{
		Stream:       true,
		Instructions: "system rules",
		Items:        []ConversationItem{UserMessageItem("hello")},
	})

	for range events {
	}

	select {
	case err := <-errs:
		if err != nil {
			t.Fatalf("stream error = %v, want nil", err)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timed out waiting for stream error result")
	}

	if gotPath != "/api/v3/responses" {
		t.Fatalf("request path = %q, want %q", gotPath, "/api/v3/responses")
	}
}

func TestOpenAICompatibleProviderOmitsEmptyArkPreviousResponseID(t *testing.T) {
	type requestBody map[string]any

	var gotRequest requestBody
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&gotRequest); err != nil {
			t.Fatalf("decode request: %v", err)
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("event: response.completed\n"))
		_, _ = w.Write([]byte("data: {\"id\":\"resp_123\",\"usage\":{\"input_tokens\":1,\"output_tokens\":1,\"prompt_tokens_details\":{\"cached_tokens\":0}}}\n\n"))
	}))
	defer server.Close()

	provider, err := NewOpenAICompatibleProvider(Config{
		APIKey:       "test-key",
		Model:        "fallback-model",
		BaseURL:      server.URL,
		CacheProfile: CapabilityProfileArkResponses,
	})
	if err != nil {
		t.Fatalf("NewOpenAICompatibleProvider() error = %v", err)
	}

	events, errs := provider.Stream(context.Background(), Request{
		Stream: true,
		Cache: &CacheDirective{
			Mode:               CacheModePrefix,
			Store:              true,
			PreviousResponseID: "",
		},
	})

	for range events {
	}

	select {
	case err := <-errs:
		if err != nil {
			t.Fatalf("stream error = %v, want nil", err)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timed out waiting for stream error result")
	}

	if _, ok := gotRequest["previous_response_id"]; ok {
		t.Fatalf("request previous_response_id = %#v, want omitted", gotRequest["previous_response_id"])
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
