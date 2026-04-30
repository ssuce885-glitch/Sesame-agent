package engine

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	contextstate "go-agent/internal/context"
	"go-agent/internal/model"
	"go-agent/internal/permissions"
	"go-agent/internal/store/sqlite"
	"go-agent/internal/tools"
	"go-agent/internal/types"
)

type recordingSink struct {
	events []types.Event
}

func (s *recordingSink) Emit(_ context.Context, e types.Event) error {
	s.events = append(s.events, e)
	return nil
}

func (s *recordingSink) eventTypes() []string {
	types := make([]string, len(s.events))
	for i, e := range s.events {
		types[i] = string(e.Type)
	}
	return types
}

type echoTool struct{}

func (echoTool) Definition() tools.Definition {
	return tools.Definition{
		Name:        "echo",
		Description: "Echoes back",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"message": map[string]any{"type": "string"},
			},
			"required": []string{"message"},
		},
	}
}

func (echoTool) IsConcurrencySafe() bool { return false }

func (echoTool) Execute(_ context.Context, call tools.Call, _ tools.ExecContext) (tools.Result, error) {
	return tools.Result{Text: "echo: " + call.StringInput("message")}, nil
}

type interruptTool struct{}

func (interruptTool) Definition() tools.Definition {
	return tools.Definition{
		Name:        "interrupt_tool",
		Description: "Interrupts the current turn",
		InputSchema: map[string]any{
			"type":                 "object",
			"properties":           map[string]any{},
			"additionalProperties": false,
		},
	}
}

func (interruptTool) IsConcurrencySafe() bool { return false }

func (interruptTool) Execute(_ context.Context, _ tools.Call, _ tools.ExecContext) (tools.Result, error) {
	return tools.Result{Text: "interrupted", ModelText: "interrupted"}, nil
}

func (interruptTool) ExecuteDecoded(_ context.Context, decoded tools.DecodedCall, execCtx tools.ExecContext) (tools.ToolExecutionResult, error) {
	result, err := interruptTool{}.Execute(context.Background(), decoded.Call, execCtx)
	if err != nil {
		return tools.ToolExecutionResult{}, err
	}
	return tools.ToolExecutionResult{
		Result:      result,
		PreviewText: "interrupted",
		Interrupt:   &tools.ToolInterrupt{Reason: "test_interrupt"},
	}, nil
}

type slowStreamingModel struct {
	streams  [][]model.StreamEvent
	index    int
	requests []model.Request
	delay    time.Duration
}

func (m *slowStreamingModel) Capabilities() model.ProviderCapabilities {
	return model.ProviderCapabilities{Profile: model.CapabilityProfileNone}
}

func (m *slowStreamingModel) Stream(ctx context.Context, req model.Request) (<-chan model.StreamEvent, <-chan error) {
	m.requests = append(m.requests, req)

	var batch []model.StreamEvent
	if m.index < len(m.streams) {
		batch = m.streams[m.index]
		m.index++
	}

	events := make(chan model.StreamEvent)
	errs := make(chan error, 1)
	go func() {
		defer close(events)
		defer close(errs)

		for _, event := range batch {
			select {
			case <-ctx.Done():
				errs <- ctx.Err()
				return
			case <-time.After(m.delay):
			}
			select {
			case <-ctx.Done():
				errs <- ctx.Err()
				return
			case events <- event:
			}
		}

		if ctx.Err() != nil {
			errs <- ctx.Err()
			return
		}
		errs <- nil
	}()

	return events, errs
}

func (m *slowStreamingModel) RequestCount() int {
	return len(m.requests)
}

func setupEngineTest(t *testing.T) (*sqlite.Store, *recordingSink, *contextstate.Manager) {
	t.Helper()
	store, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("sqlite.Open: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	sink := &recordingSink{}
	ctxMgr := contextstate.NewManager(contextstate.Config{
		MaxRecentItems:          100,
		MaxEstimatedTokens:      200000,
		ModelContextWindow:      200000,
		CompactionThreshold:     150000,
		MaxCompactionBatchItems: 500,
	})
	return store, sink, ctxMgr
}

func setupSessionAndTurn(t *testing.T, store *sqlite.Store, workspaceRoot string) (types.Session, types.Turn) {
	t.Helper()
	ctx := context.Background()
	session, head, _, err := store.EnsureRoleSession(ctx, workspaceRoot, types.SessionRoleMainParent)
	if err != nil {
		t.Fatalf("EnsureRoleSession: %v", err)
	}
	now := time.Now().UTC()
	turn := types.Turn{
		ID:            types.NewID("turn"),
		SessionID:     session.ID,
		ContextHeadID: head.ID,
		Kind:          types.TurnKindUserMessage,
		State:         types.TurnStateCreated,
		UserMessage:   "Hello, world",
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := store.InsertTurn(ctx, turn); err != nil {
		t.Fatalf("InsertTurn: %v", err)
	}
	return session, turn
}

func insertTurn(t *testing.T, store *sqlite.Store, turn types.Turn) {
	t.Helper()
	if err := store.InsertTurn(context.Background(), turn); err != nil {
		t.Fatalf("InsertTurn: %v", err)
	}
}

func assistantDeltaText(events []types.Event) string {
	var builder strings.Builder
	for _, event := range events {
		if event.Type != types.EventAssistantDelta {
			continue
		}
		var payload types.AssistantDeltaPayload
		if err := json.Unmarshal(event.Payload, &payload); err == nil {
			builder.WriteString(payload.Text)
		}
	}
	return builder.String()
}

func requestHasItemKind(req model.Request, kind model.ConversationItemKind) bool {
	for _, item := range req.Items {
		if item.Kind == kind {
			return true
		}
	}
	return false
}

func requestHasUserMessageText(req model.Request, text string) bool {
	for _, item := range req.Items {
		if item.Kind == model.ConversationItemUserMessage && item.Text == text {
			return true
		}
	}
	return false
}

func requestAssistantText(req model.Request) string {
	var builder strings.Builder
	for _, item := range req.Items {
		if item.Kind == model.ConversationItemAssistantText {
			builder.WriteString(item.Text)
		}
	}
	return builder.String()
}

func TestIntegrationSimpleTextResponse(t *testing.T) {
	store, sink, ctxMgr := setupEngineTest(t)
	fakeModel := model.NewFakeStreaming([][]model.StreamEvent{
		{
			{Kind: model.StreamEventTextDelta, TextDelta: "Hello"},
			{Kind: model.StreamEventTextDelta, TextDelta: " world!"},
			{Kind: model.StreamEventMessageEnd},
		},
	})
	eng := NewWithRuntime(fakeModel, tools.NewRegistry(), permissions.NewEngine("trusted_local"), store, ctxMgr, nil, nil, RuntimeMetadata{Provider: "fake", Model: "fake"}, 10)
	sess, turn := setupSessionAndTurn(t, store, t.TempDir())

	err := eng.RunTurn(context.Background(), Input{
		Session:     sess,
		SessionRole: types.SessionRoleMainParent,
		Turn:        turn,
		Sink:        sink,
	})
	if err != nil {
		t.Fatalf("RunTurn: %v", err)
	}
	if got := fakeModel.RequestCount(); got != 1 {
		t.Fatalf("RequestCount = %d, want 1", got)
	}

	req := fakeModel.LastRequest()
	if len(req.Items) == 0 {
		t.Fatalf("request items empty")
	}
	if req.Items[0].Kind != model.ConversationItemUserMessage {
		t.Fatalf("request item 0 kind = %q, want %q", req.Items[0].Kind, model.ConversationItemUserMessage)
	}
	if !hasEvent(sink.events, types.EventAssistantDelta) {
		t.Fatalf("assistant.delta missing; events = %#v", sink.eventTypes())
	}
	if !hasEvent(sink.events, types.EventTurnCompleted) {
		t.Fatalf("turn.completed missing; events = %#v", sink.eventTypes())
	}
	if got := assistantDeltaText(sink.events); !strings.Contains(got, "Hello world!") {
		t.Fatalf("assistant delta text = %q, want containing %q", got, "Hello world!")
	}
}

func TestIntegrationToolCallAndResult(t *testing.T) {
	store, sink, ctxMgr := setupEngineTest(t)
	registry := tools.NewRegistry()
	registry.Register(echoTool{})
	fakeModel := model.NewFakeStreaming([][]model.StreamEvent{
		{
			{Kind: model.StreamEventToolCallStart, ToolCall: model.ToolCallChunk{ID: "tc1", Name: "echo"}},
			{Kind: model.StreamEventToolCallEnd, ToolCall: model.ToolCallChunk{ID: "tc1", Name: "echo", Input: map[string]any{"message": "hi"}}},
			{Kind: model.StreamEventMessageEnd},
		},
		{
			{Kind: model.StreamEventTextDelta, TextDelta: "got echo: hi"},
			{Kind: model.StreamEventMessageEnd},
		},
	})
	eng := NewWithRuntime(fakeModel, registry, permissions.NewEngine("trusted_local"), store, ctxMgr, nil, nil, RuntimeMetadata{Provider: "fake", Model: "fake"}, 10)
	sess, turn := setupSessionAndTurn(t, store, t.TempDir())

	err := eng.RunTurn(context.Background(), Input{
		Session:     sess,
		SessionRole: types.SessionRoleMainParent,
		Turn:        turn,
		Sink:        sink,
	})
	if err != nil {
		t.Fatalf("RunTurn: %v", err)
	}
	if got := fakeModel.RequestCount(); got != 2 {
		t.Fatalf("RequestCount = %d, want 2", got)
	}

	secondReq := fakeModel.LastRequest()
	if !requestHasItemKind(secondReq, model.ConversationItemToolResult) {
		t.Fatalf("second request missing tool_result item: %#v", secondReq.Items)
	}
	if len(secondReq.ToolResults) != 1 {
		t.Fatalf("second request tool results = %d, want 1", len(secondReq.ToolResults))
	}
	if secondReq.ToolResults[0].ToolName != "echo" {
		t.Fatalf("tool result name = %q, want %q", secondReq.ToolResults[0].ToolName, "echo")
	}
	if !strings.Contains(secondReq.ToolResults[0].Content, "echo: hi") {
		t.Fatalf("tool result content = %q, want containing %q", secondReq.ToolResults[0].Content, "echo: hi")
	}
	if !hasEvent(sink.events, types.EventToolStarted) {
		t.Fatalf("tool.started missing; events = %#v", sink.eventTypes())
	}
	if !hasEvent(sink.events, types.EventToolCompleted) {
		t.Fatalf("tool.completed missing; events = %#v", sink.eventTypes())
	}
	if !hasEvent(sink.events, types.EventAssistantDelta) {
		t.Fatalf("assistant.delta missing; events = %#v", sink.eventTypes())
	}
	if !hasEvent(sink.events, types.EventTurnCompleted) {
		t.Fatalf("turn.completed missing; events = %#v", sink.eventTypes())
	}
}

func TestIntegrationMultiTurnConversation(t *testing.T) {
	store, sink, ctxMgr := setupEngineTest(t)
	fakeModel := model.NewFakeStreaming([][]model.StreamEvent{
		{
			{Kind: model.StreamEventTextDelta, TextDelta: "First reply"},
			{Kind: model.StreamEventMessageEnd},
		},
		{
			{Kind: model.StreamEventTextDelta, TextDelta: "Second reply"},
			{Kind: model.StreamEventMessageEnd},
		},
	})
	eng := NewWithRuntime(fakeModel, tools.NewRegistry(), permissions.NewEngine("trusted_local"), store, ctxMgr, nil, nil, RuntimeMetadata{Provider: "fake", Model: "fake"}, 10)
	sess, firstTurn := setupSessionAndTurn(t, store, t.TempDir())

	if err := eng.RunTurn(context.Background(), Input{
		Session:     sess,
		SessionRole: types.SessionRoleMainParent,
		Turn:        firstTurn,
		Sink:        sink,
	}); err != nil {
		t.Fatalf("first RunTurn: %v", err)
	}

	now := time.Now().UTC()
	secondTurn := types.Turn{
		ID:            types.NewID("turn"),
		SessionID:     sess.ID,
		ContextHeadID: firstTurn.ContextHeadID,
		Kind:          types.TurnKindUserMessage,
		State:         types.TurnStateCreated,
		UserMessage:   "Second turn",
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	insertTurn(t, store, secondTurn)

	if err := eng.RunTurn(context.Background(), Input{
		Session:     sess,
		SessionRole: types.SessionRoleMainParent,
		Turn:        secondTurn,
		Sink:        sink,
	}); err != nil {
		t.Fatalf("second RunTurn: %v", err)
	}

	if got := fakeModel.RequestCount(); got != 2 {
		t.Fatalf("RequestCount = %d, want 2", got)
	}

	secondReq := fakeModel.LastRequest()
	if len(secondReq.Items) <= 1 {
		t.Fatalf("second request items = %d, want accumulated conversation", len(secondReq.Items))
	}
	if !requestHasUserMessageText(secondReq, "Second turn") {
		t.Fatalf("second request missing second user message: %#v", secondReq.Items)
	}
	assistantText := requestAssistantText(secondReq)
	if assistantText == "" {
		t.Fatalf("second request missing assistant history: %#v", secondReq.Items)
	}
}

func TestIntegrationToolPermissionDenied(t *testing.T) {
	store, sink, ctxMgr := setupEngineTest(t)
	fakeModel := model.NewFakeStreaming([][]model.StreamEvent{
		{
			{Kind: model.StreamEventToolCallStart, ToolCall: model.ToolCallChunk{ID: "tc1", Name: "shell_command"}},
			{Kind: model.StreamEventToolCallEnd, ToolCall: model.ToolCallChunk{ID: "tc1", Name: "shell_command", Input: map[string]any{"command": "ls"}}},
			{Kind: model.StreamEventMessageEnd},
		},
		{
			{Kind: model.StreamEventTextDelta, TextDelta: "permission denied"},
			{Kind: model.StreamEventMessageEnd},
		},
	})
	eng := NewWithRuntime(fakeModel, tools.NewRegistry(), permissions.NewEngine("read_only"), store, ctxMgr, nil, nil, RuntimeMetadata{Provider: "fake", Model: "fake"}, 10)
	sess, turn := setupSessionAndTurn(t, store, t.TempDir())

	err := eng.RunTurn(context.Background(), Input{
		Session:     sess,
		SessionRole: types.SessionRoleMainParent,
		Turn:        turn,
		Sink:        sink,
	})
	if err != nil {
		t.Fatalf("RunTurn: %v", err)
	}
	if got := fakeModel.RequestCount(); got != 2 {
		t.Fatalf("RequestCount = %d, want 2", got)
	}

	secondReq := fakeModel.LastRequest()
	if !requestHasItemKind(secondReq, model.ConversationItemToolResult) {
		t.Fatalf("second request missing tool_result item: %#v", secondReq.Items)
	}
	if len(secondReq.ToolResults) != 1 {
		t.Fatalf("second request tool results = %d, want 1", len(secondReq.ToolResults))
	}
	if !secondReq.ToolResults[0].IsError {
		t.Fatalf("second request tool result IsError = false, want true")
	}
	if !strings.Contains(secondReq.ToolResults[0].Content, `tool "shell_command" denied`) {
		t.Fatalf("second request tool result content = %q", secondReq.ToolResults[0].Content)
	}
	if !hasEvent(sink.events, types.EventToolStarted) {
		t.Fatalf("tool.started missing; events = %#v", sink.eventTypes())
	}
	if !hasEvent(sink.events, types.EventToolCompleted) {
		t.Fatalf("tool.completed missing; events = %#v", sink.eventTypes())
	}
	if !hasEvent(sink.events, types.EventTurnCompleted) {
		t.Fatalf("turn.completed missing; events = %#v", sink.eventTypes())
	}
}

func TestIntegrationToolInterrupt(t *testing.T) {
	store, sink, ctxMgr := setupEngineTest(t)
	registry := tools.NewRegistry()
	registry.Register(interruptTool{})
	fakeModel := model.NewFakeStreaming([][]model.StreamEvent{
		{
			{Kind: model.StreamEventToolCallStart, ToolCall: model.ToolCallChunk{ID: "tc1", Name: "interrupt_tool"}},
			{Kind: model.StreamEventToolCallEnd, ToolCall: model.ToolCallChunk{ID: "tc1", Name: "interrupt_tool", Input: map[string]any{}}},
			{Kind: model.StreamEventMessageEnd},
		},
	})
	eng := NewWithRuntime(fakeModel, registry, permissions.NewEngine("trusted_local"), store, ctxMgr, nil, nil, RuntimeMetadata{Provider: "fake", Model: "fake"}, 10)
	sess, turn := setupSessionAndTurn(t, store, t.TempDir())

	err := eng.RunTurn(context.Background(), Input{
		Session:     sess,
		SessionRole: types.SessionRoleMainParent,
		Turn:        turn,
		Sink:        sink,
	})
	if err != nil {
		t.Fatalf("RunTurn: %v", err)
	}
	if got := fakeModel.RequestCount(); got != 1 {
		t.Fatalf("RequestCount = %d, want 1", got)
	}
	if !hasEvent(sink.events, types.EventTurnInterrupted) {
		t.Fatalf("turn.interrupted missing; events = %#v", sink.eventTypes())
	}
	if hasEvent(sink.events, types.EventTurnCompleted) {
		t.Fatalf("turn.completed present; events = %#v", sink.eventTypes())
	}
}

func TestIntegrationContextCancellation(t *testing.T) {
	store, sink, ctxMgr := setupEngineTest(t)
	events := make([]model.StreamEvent, 200)
	for i := range events {
		events[i] = model.StreamEvent{Kind: model.StreamEventTextDelta, TextDelta: "x"}
	}
	events = append(events, model.StreamEvent{Kind: model.StreamEventMessageEnd})
	slowModel := &slowStreamingModel{
		streams: [][]model.StreamEvent{events},
		delay:   5 * time.Millisecond,
	}
	eng := NewWithRuntime(slowModel, tools.NewRegistry(), permissions.NewEngine("trusted_local"), store, ctxMgr, nil, nil, RuntimeMetadata{Provider: "fake", Model: "fake"}, 10)
	sess, turn := setupSessionAndTurn(t, store, t.TempDir())

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- eng.RunTurn(ctx, Input{
			Session:     sess,
			SessionRole: types.SessionRoleMainParent,
			Turn:        turn,
			Sink:        sink,
		})
	}()

	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case err := <-errCh:
		if err == nil {
			t.Fatalf("expected error from cancelled context, got nil")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("RunTurn did not return after cancellation")
	}

	if got := slowModel.RequestCount(); got != 1 {
		t.Fatalf("RequestCount = %d, want 1", got)
	}
	if !hasEvent(sink.events, types.EventTurnFailed) {
		t.Fatalf("turn.failed missing; events = %#v", sink.eventTypes())
	}
}

func TestIntegrationComponentValidation(t *testing.T) {
	store, sink, ctxMgr := setupEngineTest(t)
	sess, turn := setupSessionAndTurn(t, store, t.TempDir())
	in := Input{
		Session:     sess,
		SessionRole: types.SessionRoleMainParent,
		Turn:        turn,
		Sink:        sink,
	}

	t.Run("nil model", func(t *testing.T) {
		eng := NewWithRuntime(nil, tools.NewRegistry(), permissions.NewEngine("trusted_local"), store, ctxMgr, nil, nil, RuntimeMetadata{}, 10)
		err := eng.RunTurn(context.Background(), in)
		if err == nil || !strings.Contains(err.Error(), "model client is required") {
			t.Fatalf("RunTurn error = %v, want model client is required", err)
		}
	})

	t.Run("nil registry", func(t *testing.T) {
		eng := NewWithRuntime(model.NewFakeStreaming(nil), nil, permissions.NewEngine("trusted_local"), store, ctxMgr, nil, nil, RuntimeMetadata{}, 10)
		err := eng.RunTurn(context.Background(), in)
		if err == nil || !strings.Contains(err.Error(), "tool registry is required") {
			t.Fatalf("RunTurn error = %v, want tool registry is required", err)
		}
	})

	t.Run("nil permission", func(t *testing.T) {
		eng := NewWithRuntime(model.NewFakeStreaming(nil), tools.NewRegistry(), nil, store, ctxMgr, nil, nil, RuntimeMetadata{}, 10)
		err := eng.RunTurn(context.Background(), in)
		if err == nil || !strings.Contains(err.Error(), "permission engine is required") {
			t.Fatalf("RunTurn error = %v, want permission engine is required", err)
		}
	})

	t.Run("nil sink", func(t *testing.T) {
		eng := NewWithRuntime(model.NewFakeStreaming(nil), tools.NewRegistry(), permissions.NewEngine("trusted_local"), store, ctxMgr, nil, nil, RuntimeMetadata{}, 10)
		err := eng.RunTurn(context.Background(), Input{
			Session:     sess,
			SessionRole: types.SessionRoleMainParent,
			Turn:        turn,
		})
		if err == nil || !strings.Contains(err.Error(), "event sink is required") {
			t.Fatalf("RunTurn error = %v, want event sink is required", err)
		}
	})
}
