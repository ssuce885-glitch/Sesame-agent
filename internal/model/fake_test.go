package model

import (
	"context"
	"testing"
	"time"
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

func TestFakeStreamingClientReturnsErrWhenExhausted(t *testing.T) {
	client := NewFakeStreaming(nil)

	events, errs := client.Stream(context.Background(), Request{UserMessage: "hello"})

	for range events {
		t.Fatal("expected no events when the fake streaming client is exhausted")
	}

	select {
	case err := <-errs:
		if err != errNoMoreResponses {
			t.Fatalf("expected errNoMoreResponses, got %v", err)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("expected exhaustion error from errs channel")
	}
}

func TestFakeStreamingClientReturnsContextErrorWhenCancelled(t *testing.T) {
	client := NewFakeStreaming([][]StreamEvent{
		{
			{Kind: StreamEventTextDelta, TextDelta: "blocked"},
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	events, errs := client.Stream(ctx, Request{UserMessage: "hello"})

	select {
	case event, ok := <-events:
		if ok {
			t.Fatalf("expected no events when context is cancelled, got %+v", event)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("expected events channel to close promptly on cancellation")
	}

	select {
	case err := <-errs:
		if err != context.Canceled {
			t.Fatalf("expected context.Canceled, got %v", err)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("expected context error from errs channel")
	}
}

func TestFakeStreamingClientCapturesNeutralRequest(t *testing.T) {
	client := NewFakeStreaming([][]StreamEvent{{
		{Kind: StreamEventTextDelta, TextDelta: "hello"},
		{Kind: StreamEventMessageEnd},
	}})

	req := Request{
		Model:        "provider-model",
		Instructions: "system rules",
		Stream:       true,
		ToolChoice:   "auto",
		Items: []ConversationItem{
			UserMessageItem("inspect workspace"),
			ToolResultItem(ToolResult{
				ToolCallID: "tool_1",
				ToolName:   "file_read",
				Content:    "README contents",
			}),
		},
		Tools: []ToolSchema{{
			Name:        "file_read",
			Description: "Read a file inside the workspace",
			InputSchema: map[string]any{"type": "object"},
		}},
	}

	stream, errs := client.Stream(context.Background(), req)
	for range stream {
	}
	if err := <-errs; err != nil {
		t.Fatalf("Stream() error = %v", err)
	}

	got := client.LastRequest()
	if got.Model != "provider-model" {
		t.Fatalf("Model = %q, want %q", got.Model, "provider-model")
	}
	if got.Instructions != "system rules" {
		t.Fatalf("Instructions = %q, want %q", got.Instructions, "system rules")
	}
	if !got.Stream {
		t.Fatal("Stream = false, want true")
	}
	if got.ToolChoice != "auto" {
		t.Fatalf("ToolChoice = %q, want %q", got.ToolChoice, "auto")
	}
	if len(got.Items) != 2 {
		t.Fatalf("len(Items) = %d, want 2", len(got.Items))
	}
	if got.Items[0].Kind != ConversationItemUserMessage || got.Items[0].Text != "inspect workspace" {
		t.Fatalf("first item = %+v, want user message", got.Items[0])
	}
	if got.Items[1].Kind != ConversationItemToolResult || got.Items[1].Result == nil {
		t.Fatalf("second item = %+v, want tool result", got.Items[1])
	}
	if got.Items[1].Result.ToolCallID != "tool_1" || got.Items[1].Result.ToolName != "file_read" || got.Items[1].Result.Content != "README contents" {
		t.Fatalf("second item result = %+v, want tool_1/file_read/README contents", got.Items[1].Result)
	}
	if len(got.Tools) != 1 || got.Tools[0].Name != "file_read" {
		t.Fatalf("Tools = %+v, want file_read schema", got.Tools)
	}
	if got.Tools[0].InputSchema["type"] != "object" {
		t.Fatalf("Tools[0].InputSchema[type] = %v, want object", got.Tools[0].InputSchema["type"])
	}
}

func TestFakeStreamingClientSnapshotsNeutralRequest(t *testing.T) {
	client := NewFakeStreaming([][]StreamEvent{{
		{Kind: StreamEventMessageEnd},
	}})

	req := Request{
		Model:        "provider-model",
		Instructions: "system rules",
		Stream:       true,
		ToolChoice:   "auto",
		Items: []ConversationItem{
			UserMessageItem("inspect workspace"),
			{
				Kind: ConversationItemToolCall,
				ToolCall: ToolCallChunk{
					ID:   "call_1",
					Name: "file_read",
					Input: map[string]any{
						"path": "README.md",
					},
				},
			},
			ToolResultItem(ToolResult{
				ToolCallID: "tool_1",
				ToolName:   "file_read",
				Content:    "README contents",
			}),
		},
		Tools: []ToolSchema{{
			Name:        "file_read",
			Description: "Read a file inside the workspace",
			InputSchema: map[string]any{
				"type":     "object",
				"required": []string{"path"},
				"properties": map[string]any{
					"path": map[string]any{"type": "string"},
				},
				"oneOf": []map[string]any{
					{"type": "string"},
					{"type": "object", "properties": map[string]any{"path": map[string]any{"type": "string"}}},
				},
			},
		}},
		ToolResults: []ToolResult{{
			ToolCallID: "legacy_1",
			ToolName:   "legacy_tool",
			Content:    "legacy contents",
		}},
	}

	stream, errs := client.Stream(context.Background(), req)
	for range stream {
	}
	if err := <-errs; err != nil {
		t.Fatalf("Stream() error = %v", err)
	}

	req.Model = "mutated-model"
	req.Instructions = "mutated rules"
	req.Stream = false
	req.ToolChoice = "none"
	req.Items[0].Text = "mutated"
	req.Items[1].ToolCall.Input["path"] = "CHANGED.md"
	req.Items[2].Result.Content = "mutated contents"
	req.Tools[0].Description = "mutated description"
	req.Tools[0].InputSchema["type"] = "mutated"
	req.Tools[0].InputSchema["required"].([]string)[0] = "mutated-path"
	req.Tools[0].InputSchema["properties"].(map[string]any)["path"].(map[string]any)["type"] = "mutated"
	req.Tools[0].InputSchema["oneOf"].([]map[string]any)[0]["type"] = "mutated"
	req.ToolResults[0].Content = "mutated legacy contents"

	got := client.LastRequest()
	if got.Model != "provider-model" {
		t.Fatalf("Model = %q, want %q", got.Model, "provider-model")
	}
	if got.Instructions != "system rules" {
		t.Fatalf("Instructions = %q, want %q", got.Instructions, "system rules")
	}
	if !got.Stream {
		t.Fatal("Stream = false, want true")
	}
	if got.ToolChoice != "auto" {
		t.Fatalf("ToolChoice = %q, want %q", got.ToolChoice, "auto")
	}
	if got.Items[0].Text != "inspect workspace" {
		t.Fatalf("Items[0].Text = %q, want %q", got.Items[0].Text, "inspect workspace")
	}
	if got.Items[1].ToolCall.Input["path"] != "README.md" {
		t.Fatalf("Items[1].ToolCall.Input[path] = %v, want README.md", got.Items[1].ToolCall.Input["path"])
	}
	if got.Items[2].Result.Content != "README contents" {
		t.Fatalf("Items[2].Result.Content = %q, want %q", got.Items[2].Result.Content, "README contents")
	}
	if got.Tools[0].Description != "Read a file inside the workspace" {
		t.Fatalf("Tools[0].Description = %q, want %q", got.Tools[0].Description, "Read a file inside the workspace")
	}
	if got.Tools[0].InputSchema["type"] != "object" {
		t.Fatalf("Tools[0].InputSchema[type] = %v, want object", got.Tools[0].InputSchema["type"])
	}
	if got.Tools[0].InputSchema["required"].([]string)[0] != "path" {
		t.Fatalf("Tools[0].InputSchema[required][0] = %v, want path", got.Tools[0].InputSchema["required"].([]string)[0])
	}
	if got.Tools[0].InputSchema["properties"].(map[string]any)["path"].(map[string]any)["type"] != "string" {
		t.Fatalf("Tools[0].InputSchema[properties][path][type] = %v, want string", got.Tools[0].InputSchema["properties"].(map[string]any)["path"].(map[string]any)["type"])
	}
	if got.Tools[0].InputSchema["oneOf"].([]map[string]any)[0]["type"] != "string" {
		t.Fatalf("Tools[0].InputSchema[oneOf][0][type] = %v, want string", got.Tools[0].InputSchema["oneOf"].([]map[string]any)[0]["type"])
	}
	if got.ToolResults[0].Content != "legacy contents" {
		t.Fatalf("ToolResults[0].Content = %q, want %q", got.ToolResults[0].Content, "legacy contents")
	}
}

func TestFakeStreamingClientSnapshotsCacheAwareRequest(t *testing.T) {
	client := NewFakeStreaming([][]StreamEvent{{
		{Kind: StreamEventResponseMetadata, ResponseMetadata: &ResponseMetadata{
			ResponseID:   "resp_1",
			CachedTokens: 7,
		}},
		{Kind: StreamEventMessageEnd},
	}})

	req := Request{
		Cache: &CacheDirective{
			Mode:               CacheModePrefix,
			Store:              true,
			PreviousResponseID: "resp_prev",
			ExpireAt:           1735689600,
		},
	}

	stream, errs := client.Stream(context.Background(), req)

	req.Cache.Mode = CacheModeSession
	req.Cache.Store = false
	req.Cache.PreviousResponseID = "mutated"
	req.Cache.ExpireAt = 0

	var got []StreamEvent
	for event := range stream {
		got = append(got, event)
	}
	if err := <-errs; err != nil {
		t.Fatalf("Stream() error = %v", err)
	}

	if len(got) != 2 {
		t.Fatalf("len(events) = %d, want 2", len(got))
	}
	if got[0].Kind != StreamEventResponseMetadata || got[0].ResponseMetadata == nil {
		t.Fatalf("first event = %+v, want response metadata", got[0])
	}
	if got[0].ResponseMetadata.ResponseID != "resp_1" || got[0].ResponseMetadata.CachedTokens != 7 {
		t.Fatalf("response metadata = %+v, want resp_1/7", got[0].ResponseMetadata)
	}
	if got[1].Kind != StreamEventMessageEnd {
		t.Fatalf("second event = %+v, want message end", got[1])
	}

	last := client.LastRequest()
	if last.Cache == nil {
		t.Fatal("Cache = nil, want snapshot")
	}
	if last.Cache.Mode != CacheModePrefix {
		t.Fatalf("Cache.Mode = %q, want %q", last.Cache.Mode, CacheModePrefix)
	}
	if !last.Cache.Store {
		t.Fatal("Cache.Store = false, want true")
	}
	if last.Cache.PreviousResponseID != "resp_prev" {
		t.Fatalf("Cache.PreviousResponseID = %q, want %q", last.Cache.PreviousResponseID, "resp_prev")
	}
	if last.Cache.ExpireAt != 1735689600 {
		t.Fatalf("Cache.ExpireAt = %d, want %d", last.Cache.ExpireAt, 1735689600)
	}
	if caps := client.Capabilities(); caps.Profile != CapabilityProfileNone {
		t.Fatalf("Capabilities().Profile = %q, want %q", caps.Profile, CapabilityProfileNone)
	}
}

func TestCacheContractShapesCompileAndHoldValues(t *testing.T) {
	caps := ProviderCapabilities{
		Profile:              CapabilityProfileArkResponses,
		SupportsSessionCache: true,
		SupportsPrefixCache:  true,
		CachesToolResults:    true,
		RotatesSessionRef:    true,
	}

	if !caps.SupportsSessionCache || !caps.SupportsPrefixCache || !caps.CachesToolResults || !caps.RotatesSessionRef {
		t.Fatalf("capabilities = %+v, want all cache flags true", caps)
	}

	meta := ResponseMetadata{
		ResponseID:   "resp_2",
		CachedTokens: 5,
		InputTokens:  13,
		OutputTokens: 8,
	}

	if meta.ResponseID != "resp_2" || meta.CachedTokens != 5 || meta.InputTokens != 13 || meta.OutputTokens != 8 {
		t.Fatalf("response metadata = %+v, want resp_2/5/13/8", meta)
	}
}
