package engine

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"go-agent/internal/model"
	"go-agent/internal/permissions"
	"go-agent/internal/tools"
	"go-agent/internal/types"
)

type completeTurnTool struct{}
type failingTool struct{}

func (completeTurnTool) Definition() tools.Definition {
	return tools.Definition{
		Name:        "complete_turn_tool",
		Description: "test tool that completes the current turn",
	}
}

func (completeTurnTool) IsConcurrencySafe() bool { return false }

func (completeTurnTool) CompletesTurnOnSuccess() bool { return true }

func (completeTurnTool) Execute(context.Context, tools.Call, tools.ExecContext) (tools.Result, error) {
	return tools.Result{Text: "completed"}, nil
}

func (completeTurnTool) ExecuteDecoded(context.Context, tools.DecodedCall, tools.ExecContext) (tools.ToolExecutionResult, error) {
	return tools.ToolExecutionResult{
		Result:       tools.Result{Text: "completed", ModelText: "completed"},
		PreviewText:  "completed",
		CompleteTurn: true,
	}, nil
}

func (failingTool) Definition() tools.Definition {
	return tools.Definition{
		Name:        "failing_tool",
		Description: "test tool that returns an execution error",
	}
}

func (failingTool) IsConcurrencySafe() bool { return false }

func (failingTool) Execute(context.Context, tools.Call, tools.ExecContext) (tools.Result, error) {
	return tools.Result{}, errors.New("planned failure")
}

type captureSink struct {
	events []types.Event
}

func (s *captureSink) Emit(_ context.Context, event types.Event) error {
	s.events = append(s.events, event)
	return nil
}

func TestClearPreviousResponseWhenVisibleToolSetChanges(t *testing.T) {
	req := model.Request{
		Cache: &model.CacheDirective{PreviousResponseID: "resp_123"},
	}

	clearPreviousResponseWhenVisibleToolsChange(&req,
		[]tools.Definition{{Name: "skill_use"}},
		[]tools.Definition{{Name: "automation_create_simple"}, {Name: "skill_use"}},
	)

	if got := req.Cache.PreviousResponseID; got != "" {
		t.Fatalf("PreviousResponseID = %q, want cleared after tool set change", got)
	}
}

func TestKeepPreviousResponseWhenVisibleToolSetIsSame(t *testing.T) {
	req := model.Request{
		Cache: &model.CacheDirective{PreviousResponseID: "resp_123"},
	}

	clearPreviousResponseWhenVisibleToolsChange(&req,
		[]tools.Definition{{Name: "skill_use"}, {Name: "automation_list"}},
		[]tools.Definition{{Name: "automation_list"}, {Name: "skill_use"}},
	)

	if got := req.Cache.PreviousResponseID; got != "resp_123" {
		t.Fatalf("PreviousResponseID = %q, want unchanged", got)
	}
}

func TestCompleteTurnToolFinalizesWithoutSecondModelRequest(t *testing.T) {
	modelClient := model.NewFakeStreaming([][]model.StreamEvent{
		{
			{Kind: model.StreamEventTextDelta, TextDelta: "Delegating now."},
			{Kind: model.StreamEventToolCallEnd, ToolCall: model.ToolCallChunk{
				ID:    "call_1",
				Name:  "complete_turn_tool",
				Input: map[string]any{},
			}},
			{Kind: model.StreamEventMessageEnd},
		},
	})
	registry := tools.NewRegistry()
	registry.Register(completeTurnTool{})
	engine := NewWithRuntime(modelClient, registry, permissions.NewEngine("trusted_local"), nil, nil, nil, nil, RuntimeMetadata{}, 4)
	sink := &captureSink{}

	err := engine.RunTurn(context.Background(), Input{
		Session: types.Session{
			ID:                "session_1",
			WorkspaceRoot:     t.TempDir(),
			PermissionProfile: "trusted_local",
		},
		SessionRole: types.SessionRoleMainParent,
		Turn: types.Turn{
			ID:          "turn_1",
			SessionID:   "session_1",
			Kind:        types.TurnKindUserMessage,
			UserMessage: "delegate work",
		},
		Sink: sink,
	})
	if err != nil {
		t.Fatalf("RunTurn returned error: %v", err)
	}

	for _, event := range sink.events {
		if event.Type == types.EventTurnInterrupted {
			t.Fatalf("turn was interrupted, want completed")
		}
	}
	if !hasEvent(sink.events, types.EventTurnCompleted) {
		t.Fatalf("turn.completed event missing; events = %#v", sink.events)
	}
	if got := modelClient.RequestCount(); got != 1 {
		t.Fatalf("model request count = %d, want 1", got)
	}
	if len(modelClient.LastRequest().ToolResults) != 0 {
		t.Fatalf("last request unexpectedly includes continuation tool results")
	}
}

func TestTurnCompletingToolSkipsLaterToolCallsInSameAssistantMessage(t *testing.T) {
	modelClient := model.NewFakeStreaming([][]model.StreamEvent{
		{
			{Kind: model.StreamEventTextDelta, TextDelta: "Delegating now."},
			{Kind: model.StreamEventToolCallEnd, ToolCall: model.ToolCallChunk{
				ID:    "call_1",
				Name:  "complete_turn_tool",
				Input: map[string]any{},
			}},
			{Kind: model.StreamEventToolCallEnd, ToolCall: model.ToolCallChunk{
				ID:    "call_2",
				Name:  "failing_tool",
				Input: map[string]any{},
			}},
			{Kind: model.StreamEventMessageEnd},
		},
	})
	registry := tools.NewRegistry()
	registry.Register(completeTurnTool{})
	registry.Register(failingTool{})
	engine := NewWithRuntime(modelClient, registry, permissions.NewEngine("trusted_local"), nil, nil, nil, nil, RuntimeMetadata{}, 4)
	sink := &captureSink{}

	err := engine.RunTurn(context.Background(), Input{
		Session: types.Session{
			ID:                "session_1",
			WorkspaceRoot:     t.TempDir(),
			PermissionProfile: "trusted_local",
		},
		SessionRole: types.SessionRoleMainParent,
		Turn: types.Turn{
			ID:          "turn_1",
			SessionID:   "session_1",
			Kind:        types.TurnKindUserMessage,
			UserMessage: "delegate work",
		},
		Sink: sink,
	})
	if err != nil {
		t.Fatalf("RunTurn returned error: %v", err)
	}

	if hasToolEvent(sink.events, types.EventToolStarted, "failing_tool") {
		t.Fatalf("failing_tool was executed after the turn-completing handoff; events = %#v", sink.events)
	}
	if !hasEvent(sink.events, types.EventTurnCompleted) {
		t.Fatalf("turn.completed event missing; events = %#v", sink.events)
	}
	if got := modelClient.RequestCount(); got != 1 {
		t.Fatalf("model request count = %d, want 1", got)
	}
}

func TestToolErrorContinuesModelLoopForFinalReply(t *testing.T) {
	modelClient := model.NewFakeStreaming([][]model.StreamEvent{
		{
			{Kind: model.StreamEventTextDelta, TextDelta: "Checking."},
			{Kind: model.StreamEventToolCallEnd, ToolCall: model.ToolCallChunk{
				ID:    "call_1",
				Name:  "failing_tool",
				Input: map[string]any{},
			}},
			{Kind: model.StreamEventMessageEnd},
		},
		{
			{Kind: model.StreamEventTextDelta, TextDelta: "The tool failed, but here is the final explanation."},
			{Kind: model.StreamEventMessageEnd},
		},
	})
	registry := tools.NewRegistry()
	registry.Register(failingTool{})
	engine := NewWithRuntime(modelClient, registry, permissions.NewEngine("trusted_local"), nil, nil, nil, nil, RuntimeMetadata{}, 4)
	sink := &captureSink{}

	err := engine.RunTurn(context.Background(), Input{
		Session: types.Session{
			ID:                "session_1",
			WorkspaceRoot:     t.TempDir(),
			PermissionProfile: "trusted_local",
		},
		SessionRole: types.SessionRoleMainParent,
		Turn: types.Turn{
			ID:          "turn_1",
			SessionID:   "session_1",
			Kind:        types.TurnKindUserMessage,
			UserMessage: "run failing tool",
		},
		Sink: sink,
	})
	if err != nil {
		t.Fatalf("RunTurn returned error: %v", err)
	}
	if got := modelClient.RequestCount(); got != 2 {
		t.Fatalf("model request count = %d, want 2", got)
	}
	results := modelClient.LastRequest().ToolResults
	if len(results) != 1 {
		t.Fatalf("last request tool results = %d, want 1", len(results))
	}
	if !results[0].IsError {
		t.Fatalf("tool result IsError = false, want true")
	}
	if !hasEvent(sink.events, types.EventTurnCompleted) {
		t.Fatalf("turn.completed event missing; events = %#v", sink.events)
	}
}

func hasEvent(events []types.Event, eventType string) bool {
	for _, event := range events {
		if event.Type == eventType {
			return true
		}
	}
	return false
}

func hasToolEvent(events []types.Event, eventType, toolName string) bool {
	for _, event := range events {
		if event.Type != eventType {
			continue
		}
		var payload types.ToolEventPayload
		if err := json.Unmarshal(event.Payload, &payload); err == nil && payload.ToolName == toolName {
			return true
		}
	}
	return false
}
