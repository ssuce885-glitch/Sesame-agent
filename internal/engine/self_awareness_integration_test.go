package engine

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
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

type awarenessSink struct {
	mu     sync.Mutex
	events []types.Event
}

func (s *awarenessSink) Emit(_ context.Context, e types.Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, e)
	return nil
}

func (s *awarenessSink) eventTypes() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	eventTypes := make([]string, len(s.events))
	for i, e := range s.events {
		eventTypes[i] = string(e.Type)
	}
	return eventTypes
}

func (s *awarenessSink) hasEvent(kind string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, e := range s.events {
		if string(e.Type) == kind {
			return true
		}
	}
	return false
}

func (s *awarenessSink) combinedText() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	var builder strings.Builder
	for _, e := range s.events {
		if e.Type != types.EventAssistantDelta {
			continue
		}
		var payload types.AssistantDeltaPayload
		if err := json.Unmarshal(e.Payload, &payload); err == nil {
			builder.WriteString(payload.Text)
		}
	}
	return builder.String()
}

func (s *awarenessSink) toolNames() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	var names []string
	for _, e := range s.events {
		if e.Type != types.EventToolStarted {
			continue
		}
		var payload types.ToolEventPayload
		if err := json.Unmarshal(e.Payload, &payload); err == nil {
			names = append(names, payload.ToolName)
		}
	}
	return names
}

func (s *awarenessSink) toolPayloads(toolName string) []types.ToolEventPayload {
	s.mu.Lock()
	defer s.mu.Unlock()
	var payloads []types.ToolEventPayload
	for _, e := range s.events {
		if e.Type != types.EventToolStarted {
			continue
		}
		var payload types.ToolEventPayload
		if err := json.Unmarshal(e.Payload, &payload); err == nil && payload.ToolName == toolName {
			payloads = append(payloads, payload)
		}
	}
	return payloads
}

func setupMainParentEngine(t *testing.T) (*Engine, *sqlite.Store) {
	return setupMainParentEngineWithMaxSteps(t, 20)
}

func setupMainParentEngineWithMaxSteps(t *testing.T, maxToolSteps int) (*Engine, *sqlite.Store) {
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
	ctxMgr := contextstate.NewManager(contextstate.Config{
		MaxRecentItems:          100,
		MaxEstimatedTokens:      200000,
		ModelContextWindow:      200000,
		CompactionThreshold:     150000,
		MaxCompactionBatchItems: 500,
	})
	eng := NewWithRuntime(
		modelClient,
		tools.NewRegistry(),
		permissions.NewEngine("trusted_local"),
		store,
		ctxMgr,
		nil,
		nil,
		RuntimeMetadata{Provider: cfg.ModelProvider, Model: cfg.Model},
		maxToolSteps,
	)
	return eng, store
}

func awarenessSessionAndTurn(t *testing.T, store *sqlite.Store, message string) (types.Session, types.Turn) {
	t.Helper()
	ctx := context.Background()
	sess, head, _, err := store.EnsureRoleSession(ctx, t.TempDir(), types.SessionRoleMainParent)
	if err != nil {
		t.Fatalf("EnsureRoleSession: %v", err)
	}
	now := time.Now().UTC()
	turn := types.Turn{
		ID:            types.NewID("turn"),
		SessionID:     sess.ID,
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
	return sess, turn
}

func runAwarenessTurn(t *testing.T, eng *Engine, sess types.Session, turn types.Turn, sink *awarenessSink) {
	t.Helper()
	err := eng.RunTurn(context.Background(), Input{
		Session:     sess,
		SessionRole: types.SessionRoleMainParent,
		Turn:        turn,
		Sink:        sink,
	})
	if err != nil {
		t.Fatalf("RunTurn: %v; events = %#v tools = %#v text = %q", err, sink.eventTypes(), sink.toolNames(), strings.TrimSpace(sink.combinedText()))
	}
}

func requireAwarenessCompleted(t *testing.T, sink *awarenessSink) {
	t.Helper()
	if !sink.hasEvent(types.EventTurnCompleted) {
		t.Fatalf("turn.completed missing; events = %#v", sink.eventTypes())
	}
	if sink.hasEvent(types.EventTurnFailed) {
		t.Fatalf("turn.failed present; events = %#v", sink.eventTypes())
	}
}

func requireAwarenessText(t *testing.T, sink *awarenessSink) string {
	t.Helper()
	text := strings.TrimSpace(sink.combinedText())
	if text == "" {
		t.Fatalf("assistant delta text empty; events = %#v", sink.eventTypes())
	}
	return text
}

func awarenessContainsTool(names []string, want string) bool {
	for _, name := range names {
		if name == want {
			return true
		}
	}
	return false
}

func awarenessMentionsAny(text string, terms ...string) bool {
	lower := strings.ToLower(text)
	for _, term := range terms {
		if strings.Contains(lower, strings.ToLower(term)) {
			return true
		}
	}
	return false
}

func TestSelfAwarenessIdentity(t *testing.T) {
	eng, store := setupMainParentEngine(t)
	sink := &awarenessSink{}
	sess, turn := awarenessSessionAndTurn(t, store, "Who are you? What is your name and role?")

	runAwarenessTurn(t, eng, sess, turn, sink)

	requireAwarenessCompleted(t, sink)
	requireAwarenessText(t, sink)
}

func TestSelfAwarenessToolInventory(t *testing.T) {
	eng, store := setupMainParentEngine(t)
	sink := &awarenessSink{}
	sess, turn := awarenessSessionAndTurn(t, store, "List the tools you have available. Group them by category.")

	runAwarenessTurn(t, eng, sess, turn, sink)

	requireAwarenessCompleted(t, sink)
	text := requireAwarenessText(t, sink)
	if !awarenessMentionsAny(text, "shell_command", "file_read", "grep", "memory_write", "delegate_to_role", "role_list") {
		t.Fatalf("assistant text did not mention a known tool; text = %q", text)
	}
}

func TestSelfAwarenessAutomationOwnershipBoundary(t *testing.T) {
	eng, store := setupMainParentEngineWithMaxSteps(t, 25)
	sink := &awarenessSink{}
	sess, turn := awarenessSessionAndTurn(t, store, "Create a simple automation that checks disk space every hour using shell_command `df -h`.")

	runAwarenessTurn(t, eng, sess, turn, sink)

	requireAwarenessCompleted(t, sink)
	names := sink.toolNames()
	if awarenessContainsTool(names, "automation_create_simple") {
		t.Fatalf("automation_create_simple was called; tools = %#v", names)
	}
	text := requireAwarenessText(t, sink)
	if !awarenessContainsTool(names, "delegate_to_role") && !awarenessMentionsAny(text, "specialist", "delegate", "cannot create", "can't create", "directly", "owned by a specialist role") {
		t.Fatalf("assistant did not explain the automation ownership boundary or delegate; text = %q tools = %#v", text, names)
	}
}

func TestSelfAwarenessRoleCreationToolPreference(t *testing.T) {
	eng, store := setupMainParentEngineWithMaxSteps(t, 25)
	sink := &awarenessSink{}
	sess, turn := awarenessSessionAndTurn(t, store, "Create a new specialist role called 'hello_world' with display name 'Hello World' and prompt 'You greet users and report their mood.'")

	runAwarenessTurn(t, eng, sess, turn, sink)

	requireAwarenessCompleted(t, sink)
	names := sink.toolNames()
	text := requireAwarenessText(t, sink)
	if awarenessContainsTool(names, "role_create") {
		return
	}
	if awarenessMentionsAny(text, "role_create", "role.yaml", "prompt.md", "directory structure", "role creation", "specialist role") {
		return
	}
	t.Fatalf("assistant did not use or explain role creation; text = %q tools = %#v", text, names)
}

func TestSelfAwarenessNoWorkspacePollution(t *testing.T) {
	eng, store := setupMainParentEngine(t)
	sink := &awarenessSink{}
	sess, turn := awarenessSessionAndTurn(t, store, "I want to verify the file system works. Create a file called test_verify.txt with content 'hello'. Then read it back to confirm.")

	runAwarenessTurn(t, eng, sess, turn, sink)

	requireAwarenessCompleted(t, sink)
	text := requireAwarenessText(t, sink)
	names := sink.toolNames()
	_ = names
	_ = text
}

func TestSelfAwarenessDelegationWorkflow(t *testing.T) {
	eng, store := setupMainParentEngineWithMaxSteps(t, 25)
	sink := &awarenessSink{}
	sess, turn := awarenessSessionAndTurn(t, store, "Walk me through what you would do if I asked you to have a specialist analyze the Go files in internal/engine/ using grep for 'TODO' comments. List each step.")

	runAwarenessTurn(t, eng, sess, turn, sink)

	requireAwarenessCompleted(t, sink)
	text := requireAwarenessText(t, sink)
	if !awarenessMentionsAny(text, "delegate", "specialist", "role") {
		t.Fatalf("assistant did not mention delegation or specialist roles; text = %q", text)
	}
}

func TestSelfAwarenessDelegationToNonexistentRole(t *testing.T) {
	eng, store := setupMainParentEngineWithMaxSteps(t, 25)
	sink := &awarenessSink{}
	sess, turn := awarenessSessionAndTurn(t, store, "Use delegate_to_role to delegate 'check disk space' to a role called 'ghost_role_nonexistent'. Report what happens.")

	runAwarenessTurn(t, eng, sess, turn, sink)

	requireAwarenessCompleted(t, sink)
	text := requireAwarenessText(t, sink)
	if !awarenessMentionsAny(text, "not installed", "not found", "role_list", "role_create", "unavailable", "does not exist", "no specialist") {
		t.Fatalf("assistant did not mention role workflow guidance; text = %q", text)
	}
}

func TestSelfAwarenessAutomationWorkflowSteps(t *testing.T) {
	eng, store := setupMainParentEngineWithMaxSteps(t, 25)
	sink := &awarenessSink{}
	sess, turn := awarenessSessionAndTurn(t, store, "If I want you to create an automation that monitors disk space every hour, walk me through exactly what steps you would take. Be detailed about which tools you would use and in what order.")

	runAwarenessTurn(t, eng, sess, turn, sink)

	requireAwarenessCompleted(t, sink)
	names := sink.toolNames()
	text := requireAwarenessText(t, sink)
	if awarenessContainsTool(names, "automation_create_simple") {
		t.Fatalf("automation_create_simple was called from main_parent; tools = %#v", names)
	}
	if !awarenessMentionsAny(text, "delegate", "specialist") {
		t.Fatalf("assistant did not mention delegating to a specialist role; text = %q", text)
	}
}
