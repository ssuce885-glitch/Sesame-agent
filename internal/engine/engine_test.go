package engine

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	contextstate "go-agent/internal/context"
	"go-agent/internal/model"
	"go-agent/internal/permissions"
	"go-agent/internal/tools"
	"go-agent/internal/types"
)

func TestRunTurnStreamsAssistantEventsIntoSink(t *testing.T) {
	fakeModel := model.NewFakeStreaming([][]model.StreamEvent{
		{
			{Kind: model.StreamEventTextDelta, TextDelta: "Hel"},
			{Kind: model.StreamEventTextDelta, TextDelta: "lo"},
			{Kind: model.StreamEventMessageEnd},
		},
	})
	sink := &recordingSink{}

	runner := New(fakeModel, tools.NewRegistry(), permissions.NewEngine(), nil, nil, nil, 0)
	err := runner.RunTurn(context.Background(), Input{
		Session: types.Session{ID: "sess_1", WorkspaceRoot: t.TempDir()},
		Turn:    types.Turn{ID: "turn_1", UserMessage: "inspect the readme"},
		Sink:    sink,
	})
	if err != nil {
		t.Fatalf("RunTurn() error = %v", err)
	}

	want := []string{
		types.EventTurnStarted,
		types.EventAssistantStarted,
		types.EventAssistantDelta,
		types.EventAssistantDelta,
		types.EventAssistantCompleted,
		types.EventTurnCompleted,
	}
	assertEventTypes(t, sink, want)
}

func TestRunTurnEmitsFailedWithoutCompletedWhenStreamErrorsAfterMessageEnd(t *testing.T) {
	streamErr := errors.New("stream failed after message end")
	sink := &recordingSink{}
	runner := New(scriptedStreamingClient{
		events: []model.StreamEvent{
			{Kind: model.StreamEventTextDelta, TextDelta: "Hello"},
			{Kind: model.StreamEventMessageEnd},
		},
		err: streamErr,
	}, tools.NewRegistry(), permissions.NewEngine(), nil, nil, nil, 0)

	err := runner.RunTurn(context.Background(), Input{
		Session: types.Session{ID: "sess_1", WorkspaceRoot: t.TempDir()},
		Turn:    types.Turn{ID: "turn_1", UserMessage: "hello"},
		Sink:    sink,
	})
	if !errors.Is(err, streamErr) {
		t.Fatalf("RunTurn() error = %v, want %v", err, streamErr)
	}

	assertEventTypes(t, sink, []string{
		types.EventTurnStarted,
		types.EventAssistantStarted,
		types.EventAssistantDelta,
		types.EventTurnFailed,
	})
	assertNoEventType(t, sink, types.EventAssistantCompleted)
	assertNoEventType(t, sink, types.EventTurnCompleted)
}

func TestRunTurnEmitsTurnFailedWhenToolExecutionFails(t *testing.T) {
	sink := &recordingSink{}
	runner := New(model.NewFakeStreaming([][]model.StreamEvent{
		{
			{Kind: model.StreamEventToolCallEnd, ToolCall: model.ToolCallChunk{
				ID:    "call_1",
				Name:  "missing_tool",
				Input: map[string]any{},
			}},
			{Kind: model.StreamEventMessageEnd},
		},
	}), tools.NewRegistry(), permissions.NewEngine(), nil, nil, nil, 0)

	err := runner.RunTurn(context.Background(), Input{
		Session: types.Session{ID: "sess_1", WorkspaceRoot: t.TempDir()},
		Turn:    types.Turn{ID: "turn_1", UserMessage: "hello"},
		Sink:    sink,
	})
	if err == nil {
		t.Fatal("RunTurn() error = nil, want non-nil")
	}

	assertEventTypes(t, sink, []string{
		types.EventTurnStarted,
		types.EventToolStarted,
		types.EventTurnFailed,
	})
	assertNoEventType(t, sink, types.EventTurnCompleted)
	assertNoEventType(t, sink, types.EventAssistantCompleted)
}

func TestRunTurnExecutesToolAfterToolCallEnd(t *testing.T) {
	workspace := t.TempDir()
	readmePath := filepath.Join(workspace, "README.md")
	if err := os.WriteFile(readmePath, []byte("hello from readme"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	modelClient := &recordingStreamingClient{
		streams: [][]model.StreamEvent{
			{
				{Kind: model.StreamEventTextDelta, TextDelta: "Let me check the README."},
				{Kind: model.StreamEventToolCallStart, ToolCall: model.ToolCallChunk{ID: "call_1", Name: "file_read"}},
				{Kind: model.StreamEventToolCallDelta, ToolCall: model.ToolCallChunk{ID: "call_1", Name: "file_read", InputChunk: `{"path":"`}},
				{Kind: model.StreamEventToolCallEnd, ToolCall: model.ToolCallChunk{
					ID:    "call_1",
					Name:  "file_read",
					Input: map[string]any{"path": readmePath},
				}},
				{Kind: model.StreamEventMessageEnd},
			},
			{
				{Kind: model.StreamEventTextDelta, TextDelta: "README says hello from readme"},
				{Kind: model.StreamEventMessageEnd},
			},
		},
	}
	sink := &recordingSink{}
	store := &fakeConversationStore{}
	manager := contextstate.NewManager(contextstate.Config{
		MaxRecentItems:      8,
		MaxEstimatedTokens:  6000,
		CompactionThreshold: 16,
	})
	runner := New(modelClient, tools.NewRegistry(), permissions.NewEngine(), store, manager, nil, 8)

	err := runner.RunTurn(context.Background(), Input{
		Session: types.Session{ID: "sess_1", WorkspaceRoot: workspace},
		Turn:    types.Turn{ID: "turn_1", SessionID: "sess_1", UserMessage: "inspect the readme"},
		Sink:    sink,
	})
	if err != nil {
		t.Fatalf("RunTurn() error = %v", err)
	}

	assertEventTypes(t, sink, []string{
		types.EventTurnStarted,
		types.EventAssistantStarted,
		types.EventAssistantDelta,
		types.EventToolStarted,
		types.EventToolCompleted,
		types.EventAssistantDelta,
		types.EventAssistantCompleted,
		types.EventTurnCompleted,
	})

	if len(modelClient.requests) != 2 {
		t.Fatalf("len(requests) = %d, want 2", len(modelClient.requests))
	}

	first := modelClient.requests[0]
	if first.UserMessage != "inspect the readme" {
		t.Fatalf("first request message = %q, want %q", first.UserMessage, "inspect the readme")
	}
	if len(first.ToolResults) != 0 {
		t.Fatalf("len(first request tool results) = %d, want 0", len(first.ToolResults))
	}

	second := modelClient.requests[1]
	if second.UserMessage != "inspect the readme" {
		t.Fatalf("second request message = %q, want %q", second.UserMessage, "inspect the readme")
	}
	if len(second.Items) < 3 {
		t.Fatalf("len(second request items) = %d, want at least 3", len(second.Items))
	}
	foundUser := false
	foundToolResult := false
	for _, item := range second.Items {
		if item.Kind == model.ConversationItemUserMessage && item.Text == "inspect the readme" {
			foundUser = true
		}
		if item.Kind == model.ConversationItemToolResult {
			foundToolResult = true
			if item.Result == nil || item.Result.ToolCallID != "call_1" {
				t.Fatalf("tool result item = %#v, want call_1", item)
			}
		}
	}
	if !foundUser {
		t.Fatalf("second request items = %#v, want current user item", second.Items)
	}
	if !foundToolResult {
		t.Fatalf("second request items = %#v, want tool_result item", second.Items)
	}
	if len(second.ToolResults) != 1 {
		t.Fatalf("len(second request tool results) = %d, want 1", len(second.ToolResults))
	}

	got := second.ToolResults[0]
	if got.ToolCallID != "call_1" {
		t.Fatalf("tool result call id = %q, want %q", got.ToolCallID, "call_1")
	}
	if got.ToolName != "file_read" {
		t.Fatalf("tool result name = %q, want %q", got.ToolName, "file_read")
	}
	if got.Content != "hello from readme" {
		t.Fatalf("tool result content = %q, want %q", got.Content, "hello from readme")
	}
	if got.IsError {
		t.Fatal("tool result is_error = true, want false")
	}
}

func TestRunTurnBuildsProviderRequestFromStoredConversation(t *testing.T) {
	store := &fakeConversationStore{
		items: []model.ConversationItem{
			model.UserMessageItem("first request"),
			{Kind: model.ConversationItemAssistantText, Text: "first answer"},
		},
		summaries: []model.Summary{{
			RangeLabel:       "turns 1-1",
			ImportantChoices: []string{"used rg first"},
		}},
		memories: []types.MemoryEntry{{Content: "workspace prefers rg for searches"}},
	}
	client := model.NewFakeStreaming([][]model.StreamEvent{{
		{Kind: model.StreamEventTextDelta, TextDelta: "second answer"},
		{Kind: model.StreamEventMessageEnd},
	}})
	manager := contextstate.NewManager(contextstate.Config{
		MaxRecentItems:      1,
		MaxEstimatedTokens:  6000,
		CompactionThreshold: 16,
	})
	runner := NewWithRuntime(
		client,
		tools.NewRegistry(),
		permissions.NewEngine(),
		store,
		manager,
		contextstate.NewRuntime(86400, 3),
		nil,
		RuntimeMetadata{
			Provider: "openai_compatible",
			Model:    "glm-4.5",
		},
		8,
	)

	err := runner.RunTurn(context.Background(), Input{
		Session: types.Session{ID: "sess_1", WorkspaceRoot: t.TempDir()},
		Turn:    types.Turn{ID: "turn_2", SessionID: "sess_1", UserMessage: "workspace prefers rg for searches"},
		Sink:    &recordingSink{},
	})
	if err != nil {
		t.Fatalf("RunTurn() error = %v", err)
	}

	req := client.LastRequest()
	if len(req.Items) != 3 {
		t.Fatalf("len(req.Items) = %d, want 3", len(req.Items))
	}
	if !strings.Contains(req.Instructions, "workspace prefers rg for searches") {
		t.Fatalf("Instructions = %q, want recalled memory", req.Instructions)
	}
	if req.Items[0].Kind != model.ConversationItemSummary {
		t.Fatalf("first request item kind = %q, want %q", req.Items[0].Kind, model.ConversationItemSummary)
	}
	if req.Items[0].Summary == nil || req.Items[0].Summary.RangeLabel != "turns 1-1" {
		t.Fatalf("first summary item = %#v, want stored summary", req.Items[0])
	}
	if req.Items[1].Kind != model.ConversationItemAssistantText || req.Items[1].Text != "first answer" {
		t.Fatalf("second request item = %#v, want recent assistant item", req.Items[1])
	}
	if req.Items[2].Kind != model.ConversationItemUserMessage || req.Items[2].Text != "workspace prefers rg for searches" {
		t.Fatalf("last request item = %#v, want current user message", req.Items[2])
	}
	if len(store.insertedItems) != 2 {
		t.Fatalf("len(inserted items) = %d, want 2", len(store.insertedItems))
	}
	if store.insertedItems[0].Kind != model.ConversationItemUserMessage || store.insertedPositions[0] != 3 {
		t.Fatalf("first inserted item = %#v at %d, want current user at 3", store.insertedItems[0], store.insertedPositions[0])
	}
	if store.insertedItems[1].Kind != model.ConversationItemAssistantText || store.insertedPositions[1] != 4 {
		t.Fatalf("second inserted item = %#v at %d, want assistant at 4", store.insertedItems[1], store.insertedPositions[1])
	}
}

func TestRunTurnPersistsConversationItemsInStreamingOrderAcrossToolTurns(t *testing.T) {
	workspace := t.TempDir()
	readmePath := filepath.Join(workspace, "README.md")
	if err := os.WriteFile(readmePath, []byte("hello from readme"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	store := &fakeConversationStore{}
	client := model.NewFakeStreaming([][]model.StreamEvent{
		{
			{Kind: model.StreamEventTextDelta, TextDelta: "Let me check the README."},
			{Kind: model.StreamEventToolCallEnd, ToolCall: model.ToolCallChunk{
				ID:    "call_1",
				Name:  "file_read",
				Input: map[string]any{"path": readmePath},
			}},
			{Kind: model.StreamEventMessageEnd},
		},
		{
			{Kind: model.StreamEventTextDelta, TextDelta: "README says hello from readme"},
			{Kind: model.StreamEventMessageEnd},
		},
	})
	manager := contextstate.NewManager(contextstate.Config{
		MaxRecentItems:      8,
		MaxEstimatedTokens:  6000,
		CompactionThreshold: 16,
	})
	runner := NewWithRuntime(
		client,
		tools.NewRegistry(),
		permissions.NewEngine(),
		store,
		manager,
		contextstate.NewRuntime(86400, 3),
		nil,
		RuntimeMetadata{
			Provider: "openai_compatible",
			Model:    "glm-4.5",
		},
		8,
	)

	err := runner.RunTurn(context.Background(), Input{
		Session: types.Session{ID: "sess_1", WorkspaceRoot: workspace},
		Turn:    types.Turn{ID: "turn_1", SessionID: "sess_1", UserMessage: "inspect the readme"},
		Sink:    &recordingSink{},
	})
	if err != nil {
		t.Fatalf("RunTurn() error = %v", err)
	}

	if len(store.insertedItems) != 5 {
		t.Fatalf("len(inserted items) = %d, want 5", len(store.insertedItems))
	}
	wantKinds := []model.ConversationItemKind{
		model.ConversationItemUserMessage,
		model.ConversationItemAssistantText,
		model.ConversationItemToolCall,
		model.ConversationItemToolResult,
		model.ConversationItemAssistantText,
	}
	wantPositions := []int{1, 2, 3, 4, 5}
	for i := range wantKinds {
		if store.insertedItems[i].Kind != wantKinds[i] {
			t.Fatalf("inserted item kinds = %#v, want %#v", conversationItemKinds(store.insertedItems), wantKinds)
		}
		if store.insertedPositions[i] != wantPositions[i] {
			t.Fatalf("inserted positions = %v, want %v", store.insertedPositions, wantPositions)
		}
	}
}

func TestRunTurnFailsWhenToolStepLimitIsExceeded(t *testing.T) {
	workspace := t.TempDir()
	readmePath := filepath.Join(workspace, "README.md")
	if err := os.WriteFile(readmePath, []byte("hello"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	store := &fakeConversationStore{}
	client := &recordingStreamingClient{
		streams: [][]model.StreamEvent{{
			{Kind: model.StreamEventToolCallEnd, ToolCall: model.ToolCallChunk{
				ID:    "call_1",
				Name:  "file_read",
				Input: map[string]any{"path": readmePath},
			}},
			{Kind: model.StreamEventToolCallEnd, ToolCall: model.ToolCallChunk{
				ID:    "call_2",
				Name:  "file_read",
				Input: map[string]any{"path": readmePath},
			}},
			{Kind: model.StreamEventMessageEnd},
		}},
	}
	sink := &recordingSink{}
	manager := contextstate.NewManager(contextstate.Config{
		MaxRecentItems:      8,
		MaxEstimatedTokens:  6000,
		CompactionThreshold: 16,
	})
	runner := New(client, tools.NewRegistry(), permissions.NewEngine(), store, manager, nil, 1)

	err := runner.RunTurn(context.Background(), Input{
		Session: types.Session{ID: "sess_1", WorkspaceRoot: workspace},
		Turn:    types.Turn{ID: "turn_1", SessionID: "sess_1", UserMessage: "inspect the readme"},
		Sink:    sink,
	})
	if err == nil || err.Error() != "turn exceeded max tool steps (1)" {
		t.Fatalf("RunTurn() error = %v, want max tool step error", err)
	}

	assertEventTypes(t, sink, []string{
		types.EventTurnStarted,
		types.EventToolStarted,
		types.EventToolCompleted,
		types.EventTurnFailed,
	})
	if len(client.requests) != 1 {
		t.Fatalf("len(requests) = %d, want 1", len(client.requests))
	}
}

func TestRunTurnUsesStoredProviderCacheHeadAndPersistsNextHead(t *testing.T) {
	store := &fakeConversationStore{
		cacheHead: &types.ProviderCacheHead{
			SessionID:         "sess_1",
			Provider:          "openai_compatible",
			CapabilityProfile: "ark_responses",
			ActiveSessionRef:  "resp_prev",
			ActivePrefixRef:   "pref_prev",
			ActiveGeneration:  2,
			UpdatedAt:         time.Date(2026, 4, 4, 8, 0, 0, 0, time.UTC),
		},
	}
	client := &recordingStreamingClient{
		capabilities: model.ProviderCapabilities{
			Profile:              model.CapabilityProfileArkResponses,
			SupportsSessionCache: true,
			SupportsPrefixCache:  true,
			CachesToolResults:    true,
			RotatesSessionRef:    true,
		},
		streams: [][]model.StreamEvent{{
			{Kind: model.StreamEventResponseMetadata, ResponseMetadata: &model.ResponseMetadata{
				ResponseID:   "resp_next",
				InputTokens:  11,
				OutputTokens: 7,
				CachedTokens: 4,
			}},
			{Kind: model.StreamEventMessageEnd},
		}},
	}
	manager := contextstate.NewManager(contextstate.Config{
		MaxRecentItems:      8,
		MaxEstimatedTokens:  6000,
		CompactionThreshold: 16,
	})
	runner := NewWithRuntime(
		client,
		tools.NewRegistry(),
		permissions.NewEngine(),
		store,
		manager,
		contextstate.NewRuntime(86400, 3),
		nil,
		RuntimeMetadata{
			Provider: "openai_compatible",
			Model:    "glm-4.5",
		},
		8,
	)

	err := runner.RunTurn(context.Background(), Input{
		Session: types.Session{ID: "sess_1", WorkspaceRoot: t.TempDir()},
		Turn:    types.Turn{ID: "turn_1", SessionID: "sess_1", UserMessage: "hello"},
		Sink:    &recordingSink{},
	})
	if err != nil {
		t.Fatalf("RunTurn() error = %v", err)
	}

	if len(client.requests) != 1 {
		t.Fatalf("len(requests) = %d, want 1", len(client.requests))
	}
	if client.requests[0].Cache == nil {
		t.Fatal("request cache = nil, want stored cache head to be reused")
	}
	if client.requests[0].Cache.PreviousResponseID != "resp_prev" {
		t.Fatalf("request cache previous = %q, want %q", client.requests[0].Cache.PreviousResponseID, "resp_prev")
	}
	if len(client.requests[0].Items) != 1 {
		t.Fatalf("len(first request items) = %d, want 1 incremental user item", len(client.requests[0].Items))
	}
	if client.requests[0].Items[0].Kind != model.ConversationItemUserMessage || client.requests[0].Items[0].Text != "hello" {
		t.Fatalf("first request items = %#v, want only current user item", client.requests[0].Items)
	}
	if store.upsertedHead == nil {
		t.Fatal("upserted head = nil, want next cache head")
	}
	if store.upsertedHead.ActiveSessionRef != "resp_next" {
		t.Fatalf("upserted head session ref = %q, want %q", store.upsertedHead.ActiveSessionRef, "resp_next")
	}
	if store.upsertedHead.Provider != "openai_compatible" {
		t.Fatalf("upserted head provider = %q, want %q", store.upsertedHead.Provider, "openai_compatible")
	}
	if store.upsertedHead.CapabilityProfile != "ark_responses" {
		t.Fatalf("upserted head profile = %q, want %q", store.upsertedHead.CapabilityProfile, "ark_responses")
	}
}

func TestRunTurnUsesUpdatedCacheHeadWithinSameToolLoop(t *testing.T) {
	workspace := t.TempDir()
	readmePath := filepath.Join(workspace, "README.md")
	if err := os.WriteFile(readmePath, []byte("hello from readme"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	store := &fakeConversationStore{
		cacheHead: &types.ProviderCacheHead{
			SessionID:         "sess_1",
			Provider:          "openai_compatible",
			CapabilityProfile: "ark_responses",
			ActiveSessionRef:  "resp_prev",
			ActiveGeneration:  2,
			UpdatedAt:         time.Date(2026, 4, 4, 8, 0, 0, 0, time.UTC),
		},
	}
	client := &recordingStreamingClient{
		capabilities: model.ProviderCapabilities{
			Profile:              model.CapabilityProfileArkResponses,
			SupportsSessionCache: true,
			SupportsPrefixCache:  true,
			CachesToolResults:    true,
			RotatesSessionRef:    true,
		},
		streams: [][]model.StreamEvent{
			{
				{Kind: model.StreamEventResponseMetadata, ResponseMetadata: &model.ResponseMetadata{
					ResponseID:   "resp_next",
					InputTokens:  10,
					OutputTokens: 4,
					CachedTokens: 6,
				}},
				{Kind: model.StreamEventToolCallEnd, ToolCall: model.ToolCallChunk{
					ID:    "call_1",
					Name:  "file_read",
					Input: map[string]any{"path": readmePath},
				}},
				{Kind: model.StreamEventMessageEnd},
			},
			{
				{Kind: model.StreamEventMessageEnd},
			},
		},
	}
	manager := contextstate.NewManager(contextstate.Config{
		MaxRecentItems:      8,
		MaxEstimatedTokens:  6000,
		CompactionThreshold: 16,
	})
	runner := New(client, tools.NewRegistry(), permissions.NewEngine(), store, manager, nil, 8)

	err := runner.RunTurn(context.Background(), Input{
		Session: types.Session{ID: "sess_1", WorkspaceRoot: workspace},
		Turn:    types.Turn{ID: "turn_1", SessionID: "sess_1", UserMessage: "inspect the readme"},
		Sink:    &recordingSink{},
	})
	if err != nil {
		t.Fatalf("RunTurn() error = %v", err)
	}

	if len(client.requests) != 2 {
		t.Fatalf("len(requests) = %d, want 2", len(client.requests))
	}
	if client.requests[0].Cache == nil || client.requests[0].Cache.PreviousResponseID != "resp_prev" {
		t.Fatalf("first request cache = %#v, want resp_prev", client.requests[0].Cache)
	}
	if client.requests[1].Cache == nil {
		t.Fatal("second request cache = nil, want updated previous_response_id")
	}
	if client.requests[1].Cache.PreviousResponseID != "resp_next" {
		t.Fatalf("second request cache previous = %q, want %q", client.requests[1].Cache.PreviousResponseID, "resp_next")
	}
}

func TestRunTurnUsesIncrementalNativeContinuationAfterPrefixRotation(t *testing.T) {
	workspace := t.TempDir()
	readmePath := filepath.Join(workspace, "README.md")
	if err := os.WriteFile(readmePath, []byte("hello from readme"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	store := &fakeConversationStore{
		items: []model.ConversationItem{
			model.UserMessageItem("turn 1"),
			model.UserMessageItem("turn 2"),
			model.UserMessageItem("turn 3"),
			model.UserMessageItem("turn 4"),
			model.UserMessageItem("turn 5"),
		},
	}
	client := &recordingStreamingClient{
		capabilities: model.ProviderCapabilities{
			Profile:              model.CapabilityProfileArkResponses,
			SupportsSessionCache: true,
			SupportsPrefixCache:  true,
			CachesToolResults:    true,
			RotatesSessionRef:    true,
		},
		streams: [][]model.StreamEvent{
			{
				{Kind: model.StreamEventResponseMetadata, ResponseMetadata: &model.ResponseMetadata{
					ResponseID:   "resp_prefix",
					InputTokens:  12,
					OutputTokens: 5,
					CachedTokens: 8,
				}},
				{Kind: model.StreamEventToolCallEnd, ToolCall: model.ToolCallChunk{
					ID:    "call_1",
					Name:  "file_read",
					Input: map[string]any{"path": readmePath},
				}},
				{Kind: model.StreamEventMessageEnd},
			},
			{
				{Kind: model.StreamEventMessageEnd},
			},
		},
	}
	manager := contextstate.NewManager(contextstate.Config{
		MaxRecentItems:      2,
		MaxEstimatedTokens:  8,
		CompactionThreshold: 4,
	})
	runner := New(client, tools.NewRegistry(), permissions.NewEngine(), store, manager, nil, 8)

	err := runner.RunTurn(context.Background(), Input{
		Session: types.Session{ID: "sess_1", WorkspaceRoot: workspace},
		Turn:    types.Turn{ID: "turn_6", SessionID: "sess_1", UserMessage: "follow up"},
		Sink:    &recordingSink{},
	})
	if err != nil {
		t.Fatalf("RunTurn() error = %v", err)
	}

	if len(client.requests) != 2 {
		t.Fatalf("len(requests) = %d, want 2", len(client.requests))
	}
	if client.requests[0].Cache == nil || client.requests[0].Cache.Mode != model.CacheModePrefix {
		t.Fatalf("first request cache = %#v, want prefix rotation", client.requests[0].Cache)
	}
	if client.requests[1].Cache == nil {
		t.Fatal("second request cache = nil, want session continuation")
	}
	if client.requests[1].Cache.Mode != model.CacheModeSession {
		t.Fatalf("second request cache mode = %q, want %q", client.requests[1].Cache.Mode, model.CacheModeSession)
	}
	if client.requests[1].Cache.PreviousResponseID != "resp_prefix" {
		t.Fatalf("second request cache previous = %q, want %q", client.requests[1].Cache.PreviousResponseID, "resp_prefix")
	}
	if client.requests[1].UserMessage != "" {
		t.Fatalf("second request user message = %q, want empty native continuation", client.requests[1].UserMessage)
	}
	if len(client.requests[1].Items) != 2 {
		t.Fatalf("len(second request items) = %d, want 2 incremental items", len(client.requests[1].Items))
	}
	if client.requests[1].Items[0].Kind != model.ConversationItemToolCall || client.requests[1].Items[0].ToolCall.ID != "call_1" {
		t.Fatalf("second request items[0] = %#v, want tool call delta", client.requests[1].Items[0])
	}
	if client.requests[1].Items[1].Kind != model.ConversationItemToolResult || client.requests[1].Items[1].Result == nil || client.requests[1].Items[1].Result.ToolCallID != "call_1" {
		t.Fatalf("second request items[1] = %#v, want tool result delta", client.requests[1].Items[1])
	}
}

func TestRunTurnCompactsEvenWhenSummariesAlreadyExist(t *testing.T) {
	store := &fakeConversationStore{
		items: []model.ConversationItem{
			model.UserMessageItem("turn 1"),
			model.UserMessageItem("turn 2"),
			model.UserMessageItem("turn 3"),
			model.UserMessageItem("turn 4"),
			model.UserMessageItem("turn 5"),
			model.UserMessageItem("turn 6"),
		},
		summaries: []model.Summary{{
			RangeLabel: "turns 1-2",
		}},
	}
	client := &recordingStreamingClient{
		streams: [][]model.StreamEvent{{
			{Kind: model.StreamEventMessageEnd},
		}},
	}
	manager := contextstate.NewManager(contextstate.Config{
		MaxRecentItems:             2,
		MaxEstimatedTokens:         8,
		CompactionThreshold:        4,
		MicrocompactBytesThreshold: 9999,
	})
	compactor := &recordingCompactor{
		summary: model.Summary{
			RangeLabel: "turns 1-4",
		},
	}
	runner := New(client, tools.NewRegistry(), permissions.NewEngine(), store, manager, compactor, 8)

	err := runner.RunTurn(context.Background(), Input{
		Session: types.Session{ID: "sess_1", WorkspaceRoot: t.TempDir()},
		Turn:    types.Turn{ID: "turn_7", SessionID: "sess_1", UserMessage: "follow up"},
		Sink:    &recordingSink{},
	})
	if err != nil {
		t.Fatalf("RunTurn() error = %v", err)
	}

	if compactor.calls != 1 {
		t.Fatalf("compactor calls = %d, want 1", compactor.calls)
	}
	if len(store.insertedSummaries) != 1 {
		t.Fatalf("len(inserted summaries) = %d, want 1", len(store.insertedSummaries))
	}
	if store.insertedSummaries[0].RangeLabel != "turns 1-4" {
		t.Fatalf("inserted summary = %#v, want rolling compaction summary", store.insertedSummaries[0])
	}
}

func TestRunTurnPersistsToolCallAndEmitsTurnUsage(t *testing.T) {
	workspace := t.TempDir()
	readmePath := filepath.Join(workspace, "README.md")
	if err := os.WriteFile(readmePath, []byte("hello from readme"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	store := &fakeConversationStore{}
	client := model.NewFakeStreaming([][]model.StreamEvent{
		{
			{Kind: model.StreamEventResponseMetadata, ResponseMetadata: &model.ResponseMetadata{
				ResponseID:   "resp_1",
				InputTokens:  120,
				OutputTokens: 30,
				CachedTokens: 24,
			}},
			{Kind: model.StreamEventTextDelta, TextDelta: "checking readme"},
			{Kind: model.StreamEventToolCallEnd, ToolCall: model.ToolCallChunk{
				ID:    "call_1",
				Name:  "file_read",
				Input: map[string]any{"path": readmePath},
			}},
			{Kind: model.StreamEventMessageEnd},
		},
		{
			{Kind: model.StreamEventMessageEnd},
		},
	})
	sink := &recordingSink{}
	manager := contextstate.NewManager(contextstate.Config{
		MaxRecentItems:      8,
		MaxEstimatedTokens:  6000,
		CompactionThreshold: 16,
	})
	runner := NewWithRuntime(
		client,
		tools.NewRegistry(),
		permissions.NewEngine(),
		store,
		manager,
		contextstate.NewRuntime(86400, 3),
		nil,
		RuntimeMetadata{
			Provider: "openai_compatible",
			Model:    "glm-4.5",
		},
		8,
	)

	err := runner.RunTurn(context.Background(), Input{
		Session: types.Session{ID: "sess_1", WorkspaceRoot: workspace},
		Turn:    types.Turn{ID: "turn_1", SessionID: "sess_1", UserMessage: "inspect readme"},
		Sink:    sink,
	})
	if err != nil {
		t.Fatalf("RunTurn() error = %v", err)
	}

	foundToolCall := false
	for _, item := range store.insertedItems {
		if item.Kind != model.ConversationItemToolCall {
			continue
		}
		foundToolCall = true
		if item.ToolCall.ID != "call_1" || item.ToolCall.Name != "file_read" {
			t.Fatalf("tool call item = %#v, want call_1/file_read", item)
		}
		if item.ToolCall.Input["path"] != readmePath {
			t.Fatalf("tool call input path = %v, want %q", item.ToolCall.Input["path"], readmePath)
		}
	}
	if !foundToolCall {
		t.Fatalf("inserted items = %#v, want tool_call item", store.insertedItems)
	}

	if store.upsertedUsage == nil {
		t.Fatal("upserted usage = nil, want persisted turn usage")
	}
	if store.upsertedUsage.TurnID != "turn_1" || store.upsertedUsage.SessionID != "sess_1" {
		t.Fatalf("upserted usage identity = %#v, want turn_1/sess_1", *store.upsertedUsage)
	}
	if store.upsertedUsage.InputTokens != 120 || store.upsertedUsage.OutputTokens != 30 || store.upsertedUsage.CachedTokens != 24 {
		t.Fatalf("upserted usage tokens = %#v, want 120/30/24", *store.upsertedUsage)
	}
	if store.upsertedUsage.CacheHitRate != 0.2 {
		t.Fatalf("upserted usage cache hit rate = %v, want %v", store.upsertedUsage.CacheHitRate, 0.2)
	}
	if store.upsertedUsage.Provider != "openai_compatible" || store.upsertedUsage.Model != "glm-4.5" {
		t.Fatalf("upserted usage provider/model = %#v, want openai_compatible/glm-4.5", *store.upsertedUsage)
	}

	var usagePayload types.TurnUsagePayload
	foundUsageEvent := false
	for _, event := range sink.events {
		if event.Type != types.EventTurnUsage {
			continue
		}
		foundUsageEvent = true
		if err := json.Unmarshal(event.Payload, &usagePayload); err != nil {
			t.Fatalf("turn.usage payload unmarshal error = %v", err)
		}
	}
	if !foundUsageEvent {
		t.Fatalf("events = %v, want %q", sink.eventTypes(), types.EventTurnUsage)
	}
	if usagePayload.Provider != "openai_compatible" || usagePayload.Model != "glm-4.5" {
		t.Fatalf("turn.usage payload provider/model = %#v, want openai_compatible/glm-4.5", usagePayload)
	}
	if usagePayload.InputTokens != 120 || usagePayload.OutputTokens != 30 || usagePayload.CachedTokens != 24 || usagePayload.CacheHitRate != 0.2 {
		t.Fatalf("turn.usage payload tokens = %#v, want 120/30/24/0.2", usagePayload)
	}
}

func TestRunTurnAggregatesUsageAcrossMultipleResponseMetadata(t *testing.T) {
	workspace := t.TempDir()
	readmePath := filepath.Join(workspace, "README.md")
	if err := os.WriteFile(readmePath, []byte("hello from readme"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	store := &fakeConversationStore{}
	client := model.NewFakeStreaming([][]model.StreamEvent{
		{
			{Kind: model.StreamEventResponseMetadata, ResponseMetadata: &model.ResponseMetadata{
				ResponseID:   "resp_1",
				InputTokens:  100,
				OutputTokens: 20,
				CachedTokens: 10,
			}},
			{Kind: model.StreamEventToolCallEnd, ToolCall: model.ToolCallChunk{
				ID:    "call_1",
				Name:  "file_read",
				Input: map[string]any{"path": readmePath},
			}},
			{Kind: model.StreamEventMessageEnd},
		},
		{
			{Kind: model.StreamEventResponseMetadata, ResponseMetadata: &model.ResponseMetadata{
				ResponseID:   "resp_2",
				InputTokens:  50,
				OutputTokens: 10,
				CachedTokens: 5,
			}},
			{Kind: model.StreamEventMessageEnd},
		},
	})
	sink := &recordingSink{}
	manager := contextstate.NewManager(contextstate.Config{
		MaxRecentItems:      8,
		MaxEstimatedTokens:  6000,
		CompactionThreshold: 16,
	})
	runner := NewWithRuntime(
		client,
		tools.NewRegistry(),
		permissions.NewEngine(),
		store,
		manager,
		contextstate.NewRuntime(86400, 3),
		nil,
		RuntimeMetadata{
			Provider: "openai_compatible",
			Model:    "glm-4.5",
		},
		8,
	)

	err := runner.RunTurn(context.Background(), Input{
		Session: types.Session{ID: "sess_agg", WorkspaceRoot: workspace},
		Turn:    types.Turn{ID: "turn_agg", SessionID: "sess_agg", UserMessage: "inspect readme"},
		Sink:    sink,
	})
	if err != nil {
		t.Fatalf("RunTurn() error = %v", err)
	}

	if store.upsertUsageCalls != 1 {
		t.Fatalf("upsert usage calls = %d, want 1", store.upsertUsageCalls)
	}
	if store.upsertedUsage == nil {
		t.Fatal("upserted usage = nil, want aggregated usage")
	}
	if store.upsertedUsage.InputTokens != 150 || store.upsertedUsage.OutputTokens != 30 || store.upsertedUsage.CachedTokens != 15 {
		t.Fatalf("aggregated usage tokens = %#v, want 150/30/15", *store.upsertedUsage)
	}
	if store.upsertedUsage.CacheHitRate != 0.1 {
		t.Fatalf("aggregated cache hit rate = %v, want 0.1", store.upsertedUsage.CacheHitRate)
	}

	usageEvents := 0
	var payload types.TurnUsagePayload
	for _, event := range sink.events {
		if event.Type != types.EventTurnUsage {
			continue
		}
		usageEvents++
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			t.Fatalf("json.Unmarshal(turn.usage) error = %v", err)
		}
	}
	if usageEvents != 1 {
		t.Fatalf("turn.usage events = %d, want 1", usageEvents)
	}
	if payload.InputTokens != 150 || payload.OutputTokens != 30 || payload.CachedTokens != 15 || payload.CacheHitRate != 0.1 {
		t.Fatalf("turn.usage payload = %#v, want 150/30/15/0.1", payload)
	}
}

type scriptedStreamingClient struct {
	events []model.StreamEvent
	err    error
}

func (c scriptedStreamingClient) Stream(_ context.Context, _ model.Request) (<-chan model.StreamEvent, <-chan error) {
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

func (c scriptedStreamingClient) Capabilities() model.ProviderCapabilities {
	return model.ProviderCapabilities{Profile: model.CapabilityProfileNone}
}

type recordingStreamingClient struct {
	streams      [][]model.StreamEvent
	requests     []model.Request
	capabilities model.ProviderCapabilities
}

func (c *recordingStreamingClient) Stream(_ context.Context, req model.Request) (<-chan model.StreamEvent, <-chan error) {
	c.requests = append(c.requests, req)

	var batch []model.StreamEvent
	if len(c.streams) > 0 {
		batch = c.streams[0]
		c.streams = c.streams[1:]
	}

	events := make(chan model.StreamEvent, len(batch))
	errs := make(chan error, 1)

	for _, event := range batch {
		events <- event
	}
	close(events)

	errs <- nil
	close(errs)

	return events, errs
}

func (c *recordingStreamingClient) Capabilities() model.ProviderCapabilities {
	return c.capabilities
}

type recordingSink struct {
	events []types.Event
}

func (s *recordingSink) Emit(_ context.Context, event types.Event) error {
	s.events = append(s.events, event)
	return nil
}

func (s *recordingSink) eventTypes() []string {
	got := make([]string, 0, len(s.events))
	for _, event := range s.events {
		got = append(got, event.Type)
	}
	return got
}

func assertEventTypes(t *testing.T, sink *recordingSink, want []string) {
	t.Helper()

	got := sink.eventTypes()
	if len(got) != len(want) {
		t.Fatalf("len(event types) = %d, want %d; got %v", len(got), len(want), got)
	}

	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("event types = %v, want %v", got, want)
		}
	}
}

func assertNoEventType(t *testing.T, sink *recordingSink, unwanted string) {
	t.Helper()

	for _, eventType := range sink.eventTypes() {
		if eventType == unwanted {
			t.Fatalf("event types = %v, unexpected %q", sink.eventTypes(), unwanted)
		}
	}
}

type fakeConversationStore struct {
	items              []model.ConversationItem
	summaries          []model.Summary
	memories           []types.MemoryEntry
	cacheHead          *types.ProviderCacheHead
	upsertedUsage      *types.TurnUsage
	upsertUsageCalls   int
	upsertedHead       *types.ProviderCacheHead
	insertedItems      []model.ConversationItem
	insertedPositions  []int
	insertedSummaries  []model.Summary
	insertedSummaryPos []int
}

func (s *fakeConversationStore) ListConversationItems(context.Context, string) ([]model.ConversationItem, error) {
	return append([]model.ConversationItem(nil), s.items...), nil
}

func (s *fakeConversationStore) ListConversationSummaries(context.Context, string) ([]model.Summary, error) {
	return append([]model.Summary(nil), s.summaries...), nil
}

func (s *fakeConversationStore) InsertConversationItem(_ context.Context, _ string, _ string, position int, item model.ConversationItem) error {
	s.insertedItems = append(s.insertedItems, item)
	s.insertedPositions = append(s.insertedPositions, position)
	return nil
}

func (s *fakeConversationStore) InsertConversationSummary(_ context.Context, _ string, position int, summary model.Summary) error {
	s.insertedSummaries = append(s.insertedSummaries, summary)
	s.insertedSummaryPos = append(s.insertedSummaryPos, position)
	return nil
}

func (s *fakeConversationStore) ListMemoryEntriesByWorkspace(context.Context, string) ([]types.MemoryEntry, error) {
	return append([]types.MemoryEntry(nil), s.memories...), nil
}

func (s *fakeConversationStore) GetProviderCacheHead(context.Context, string, string, string) (types.ProviderCacheHead, bool, error) {
	if s.cacheHead == nil {
		return types.ProviderCacheHead{}, false, nil
	}
	return *s.cacheHead, true, nil
}

func (s *fakeConversationStore) UpsertProviderCacheHead(_ context.Context, head types.ProviderCacheHead) error {
	cloned := head
	s.upsertedHead = &cloned
	return nil
}

func (s *fakeConversationStore) UpsertTurnUsage(_ context.Context, usage types.TurnUsage) error {
	cloned := usage
	s.upsertedUsage = &cloned
	s.upsertUsageCalls++
	return nil
}

func (s *fakeConversationStore) InsertProviderCacheEntry(context.Context, types.ProviderCacheEntry) error {
	return nil
}

func (s *fakeConversationStore) InsertConversationCompaction(context.Context, types.ConversationCompaction) error {
	return nil
}

type recordingCompactor struct {
	summary model.Summary
	calls   int
}

func (c *recordingCompactor) Compact(context.Context, []model.ConversationItem) (model.Summary, error) {
	c.calls++
	return c.summary, nil
}

func conversationItemKinds(items []model.ConversationItem) []model.ConversationItemKind {
	kinds := make([]model.ConversationItemKind, 0, len(items))
	for _, item := range items {
		kinds = append(kinds, item.Kind)
	}
	return kinds
}
