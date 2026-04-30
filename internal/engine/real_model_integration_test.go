package engine

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"go-agent/internal/config"
	contextstate "go-agent/internal/context"
	"go-agent/internal/model"
	"go-agent/internal/permissions"
	"go-agent/internal/store/sqlite"
	"go-agent/internal/tools"
	"go-agent/internal/types"
)

type realModelSink struct {
	events []types.Event
}

func (s *realModelSink) Emit(_ context.Context, e types.Event) error {
	s.events = append(s.events, e)
	return nil
}

func (s *realModelSink) eventTypes() []string {
	types := make([]string, len(s.events))
	for i, e := range s.events {
		types[i] = string(e.Type)
	}
	return types
}

func hasRealEvent(events []types.Event, kind string) bool {
	for _, e := range events {
		if string(e.Type) == kind {
			return true
		}
	}
	return false
}

func realModelAssistantDeltaText(events []types.Event) string {
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

func setupRealModelEngine(t *testing.T) (*Engine, *sqlite.Store, *realModelSink) {
	t.Helper()
	return setupRealModelEngineWithRegistry(t, tools.NewRegistry())
}

func setupRealModelEngineWithRegistry(t *testing.T, registry *tools.Registry) (*Engine, *sqlite.Store, *realModelSink) {
	t.Helper()
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	if cfg.ModelProvider == "fake" {
		t.Fatalf("config.Load: model provider %q is not a real model", cfg.ModelProvider)
	}
	modelClient, err := model.NewFromConfig(cfg)
	if err != nil {
		t.Fatalf("model.NewFromConfig: %v", err)
	}
	store, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("sqlite.Open: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	sink := &realModelSink{}
	ctxMgr := contextstate.NewManager(contextstate.Config{
		MaxRecentItems:          100,
		MaxEstimatedTokens:      200000,
		ModelContextWindow:      200000,
		CompactionThreshold:     150000,
		MaxCompactionBatchItems: 500,
	})
	eng := NewWithRuntime(
		modelClient,
		registry,
		permissions.NewEngine("trusted_local"),
		store,
		ctxMgr,
		nil,
		nil,
		RuntimeMetadata{Provider: cfg.ModelProvider, Model: cfg.Model},
		10,
	)
	return eng, store, sink
}

func realModelSessionAndTurn(t *testing.T, store *sqlite.Store, workspaceRoot string) (types.Session, types.Turn) {
	t.Helper()
	return realModelSessionAndTurnWithMessage(t, store, workspaceRoot, "Reply with exactly 'pong' and nothing else. No explanation, no punctuation.")
}

func realModelSessionAndTurnWithMessage(t *testing.T, store *sqlite.Store, workspaceRoot, message string) (types.Session, types.Turn) {
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
		UserMessage:   message,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := store.InsertTurn(ctx, turn); err != nil {
		t.Fatalf("InsertTurn: %v", err)
	}
	return session, turn
}

type realEchoTool struct{}

func (realEchoTool) Definition() tools.Definition {
	return tools.Definition{
		Name:        "echo",
		Description: "Echoes the input message back",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"message": map[string]any{"type": "string"},
			},
			"required": []string{"message"},
		},
	}
}

func (realEchoTool) IsConcurrencySafe() bool { return false }

func (realEchoTool) Execute(_ context.Context, call tools.Call, _ tools.ExecContext) (tools.Result, error) {
	return tools.Result{Text: "echo: " + call.StringInput("message")}, nil
}

func TestRealModelSimpleTextResponse(t *testing.T) {
	eng, store, sink := setupRealModelEngine(t)
	sess, turn := realModelSessionAndTurn(t, store, t.TempDir())

	err := eng.RunTurn(context.Background(), Input{
		Session:     sess,
		SessionRole: types.SessionRoleMainParent,
		Turn:        turn,
		Sink:        sink,
	})
	if err != nil {
		t.Fatalf("RunTurn: %v", err)
	}
	if !hasRealEvent(sink.events, types.EventAssistantDelta) {
		t.Fatalf("assistant.delta missing; events = %#v", sink.eventTypes())
	}
	if !hasRealEvent(sink.events, types.EventTurnCompleted) {
		t.Fatalf("turn.completed missing; events = %#v", sink.eventTypes())
	}
	if hasRealEvent(sink.events, types.EventTurnFailed) {
		t.Fatalf("turn.failed present; events = %#v", sink.eventTypes())
	}
	if got := strings.TrimSpace(realModelAssistantDeltaText(sink.events)); got == "" {
		t.Fatalf("assistant delta text empty; events = %#v", sink.eventTypes())
	}
}

func TestRealModelToolCall(t *testing.T) {
	registry := &tools.Registry{}
	registry.Register(realEchoTool{})
	eng, store, sink := setupRealModelEngineWithRegistry(t, registry)
	sess, turn := realModelSessionAndTurnWithMessage(t, store, t.TempDir(), "Use the echo tool with message=hello. Do not answer until after you call the tool. Then report exactly what the tool returned.")

	err := eng.RunTurn(context.Background(), Input{
		Session:     sess,
		SessionRole: types.SessionRoleMainParent,
		Turn:        turn,
		Sink:        sink,
	})
	if err != nil {
		t.Fatalf("RunTurn: %v", err)
	}
	if !hasRealEvent(sink.events, types.EventToolStarted) {
		t.Fatalf("tool.started missing; events = %#v", sink.eventTypes())
	}
	if !hasRealEvent(sink.events, types.EventToolCompleted) {
		t.Fatalf("tool.completed missing; events = %#v", sink.eventTypes())
	}
	if !hasRealEvent(sink.events, types.EventAssistantDelta) {
		t.Fatalf("assistant.delta missing; events = %#v", sink.eventTypes())
	}
	if !hasRealEvent(sink.events, types.EventTurnCompleted) {
		t.Fatalf("turn.completed missing; events = %#v", sink.eventTypes())
	}
	if hasRealEvent(sink.events, types.EventTurnFailed) {
		t.Fatalf("turn.failed present; events = %#v", sink.eventTypes())
	}
}

func TestRealModelCancellation(t *testing.T) {
	eng, store, sink := setupRealModelEngine(t)
	sess, turn := realModelSessionAndTurnWithMessage(t, store, t.TempDir(), "Count from 1 to 100, saying each number on its own line.")

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

	time.Sleep(2 * time.Second)
	cancel()

	select {
	case err := <-errCh:
		if err != nil && !errors.Is(err, context.Canceled) {
			t.Fatalf("RunTurn: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("RunTurn did not return after cancellation")
	}

	if !hasRealEvent(sink.events, types.EventTurnFailed) {
		t.Fatalf("turn.failed missing; events = %#v", sink.eventTypes())
	}
}
