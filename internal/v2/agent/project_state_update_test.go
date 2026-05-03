package agent

import (
	"context"
	"strings"
	"testing"
	"time"

	"go-agent/internal/model"
	"go-agent/internal/v2/contracts"
	v2store "go-agent/internal/v2/store"
)

func TestProjectStateTurnTranscriptSkipsThinkingAndSummarizesToolCalls(t *testing.T) {
	got := projectStateTurnTranscript([]contracts.Message{
		{Role: "user", Content: "补长期项目上下文"},
		{Role: "assistant", Content: encodeThinkingBlock("private reasoning", "sig")},
		{Role: "assistant", Content: encodeToolCallMessage("shell.exec", map[string]any{"cmd": "go test"}), ToolCallID: "tool-1"},
		{Role: "tool", Content: "PASS"},
		{Role: "assistant", Content: "已完成。"},
	}, 1000)

	if strings.Contains(got, "private reasoning") {
		t.Fatalf("transcript leaked thinking block: %q", got)
	}
	if !strings.Contains(got, "user: 补长期项目上下文") {
		t.Fatalf("transcript missing user message: %q", got)
	}
	if !strings.Contains(got, "assistant: tool call: shell.exec") {
		t.Fatalf("transcript missing summarized tool call: %q", got)
	}
	if !strings.Contains(got, "tool: PASS") {
		t.Fatalf("transcript missing tool result: %q", got)
	}
}

func TestUpdateProjectStateWritesSummary(t *testing.T) {
	s, err := v2store.OpenInMemory()
	if err != nil {
		t.Fatalf("OpenInMemory: %v", err)
	}
	defer s.Close()

	ctx := context.Background()
	now := time.Now().UTC()
	if err := s.ProjectStates().Upsert(ctx, contracts.ProjectState{
		WorkspaceRoot: "/workspace",
		Summary:       "# Current Goal\nKeep V2 simple.",
		CreatedAt:     now,
		UpdatedAt:     now,
	}); err != nil {
		t.Fatalf("upsert project state: %v", err)
	}

	nextSummary := strings.Join([]string{
		"# Current Goal",
		"Ship long project context.",
		"# Current State",
		"Project State is auto-updated after turns.",
		"# Key Decisions",
		"Use one compact document instead of V1 context layers.",
		"# Open Threads",
		"",
		"# Changed Files",
		"internal/v2/agent/project_state_update.go",
		"# Validation",
		"go test ./internal/v2/agent",
		"# User Preferences",
		"Keep concepts lean.",
	}, "\n")
	client := &captureClient{events: []model.StreamEvent{
		{Kind: model.StreamEventTextDelta, TextDelta: nextSummary},
		{Kind: model.StreamEventMessageEnd},
	}}
	a := New(client, emptyRegistry{}, s)
	a.SetProjectStateAutoUpdate(false)

	err = a.updateProjectState(ctx, contracts.TurnInput{
		SessionID: "session-1",
		TurnID:    "turn-1",
	}, "/workspace", []contracts.Message{
		{Role: "user", Content: "继续补业务"},
		{Role: "assistant", Content: "已补 Project State 自动更新。"},
	})
	if err != nil {
		t.Fatalf("updateProjectState: %v", err)
	}

	state, ok, err := s.ProjectStates().Get(ctx, "/workspace")
	if err != nil {
		t.Fatalf("get project state: %v", err)
	}
	if !ok {
		t.Fatal("project state was not written")
	}
	if state.Summary != nextSummary {
		t.Fatalf("summary = %q, want %q", state.Summary, nextSummary)
	}
	if state.SourceSessionID != "session-1" || state.SourceTurnID != "turn-1" {
		t.Fatalf("source = (%q, %q), want session/turn ids", state.SourceSessionID, state.SourceTurnID)
	}

	req := client.firstRequest()
	if !strings.Contains(req.Instructions, "compact Project State") {
		t.Fatalf("request instructions missing updater role: %q", req.Instructions)
	}
	if !strings.Contains(req.Items[0].Text, "Keep V2 simple.") {
		t.Fatalf("request prompt missing current summary: %q", req.Items[0].Text)
	}
	if !strings.Contains(req.Items[0].Text, "继续补业务") {
		t.Fatalf("request prompt missing latest transcript: %q", req.Items[0].Text)
	}
}

func TestShouldUpdateProjectStateUsesDeltaThresholds(t *testing.T) {
	current := contracts.ProjectState{
		Summary:      "# Current Goal\nKeep current.",
		SourceTurnID: "turn-2",
	}
	tokenText := func(tokens int) string {
		return strings.Repeat("abcd", tokens)
	}
	message := func(turnID string, tokens int) contracts.Message {
		return contracts.Message{TurnID: turnID, Role: "assistant", Content: tokenText(tokens)}
	}

	if shouldUpdateProjectState(true, current, nil, []contracts.Message{message("turn-3", 100)}) {
		t.Fatal("small turn below context threshold should not update")
	}
	if !shouldUpdateProjectState(false, contracts.ProjectState{}, nil, []contracts.Message{message("turn-1", 100)}) {
		t.Fatal("missing project state should create initial state")
	}
	if !shouldUpdateProjectState(true, current, nil, []contracts.Message{message("turn-3", projectStateSignificantTurnTokens)}) {
		t.Fatal("large single turn should update")
	}

	priorBelowDelta := []contracts.Message{
		message("turn-1", 6000),
		message("turn-2", 10),
		message("turn-3", 3000),
	}
	if shouldUpdateProjectState(true, current, priorBelowDelta, []contracts.Message{message("turn-4", 1000)}) {
		t.Fatal("context above init threshold but below delta threshold should not update")
	}

	priorAtDelta := []contracts.Message{
		message("turn-1", 6000),
		message("turn-2", 10),
		message("turn-3", 4000),
	}
	if !shouldUpdateProjectState(true, current, priorAtDelta, []contracts.Message{message("turn-4", 1000)}) {
		t.Fatal("context above init threshold and delta threshold should update")
	}
}

func TestRunTurnSchedulesProjectStateUpdate(t *testing.T) {
	s, err := v2store.OpenInMemory()
	if err != nil {
		t.Fatalf("OpenInMemory: %v", err)
	}
	defer s.Close()

	ctx := context.Background()
	now := time.Now().UTC()
	session := contracts.Session{
		ID:                "session-1",
		WorkspaceRoot:     "/workspace",
		SystemPrompt:      "Base prompt.",
		PermissionProfile: "trusted_local",
		State:             "idle",
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	if err := s.Sessions().Create(ctx, session); err != nil {
		t.Fatalf("create session: %v", err)
	}
	turn := contracts.Turn{
		ID:          "turn-1",
		SessionID:   session.ID,
		Kind:        "user_message",
		State:       "created",
		UserMessage: "What changed?",
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := s.Turns().Create(ctx, turn); err != nil {
		t.Fatalf("create turn: %v", err)
	}

	client := &captureClient{events: []model.StreamEvent{
		{Kind: model.StreamEventTextDelta, TextDelta: "Updated project state."},
		{Kind: model.StreamEventMessageEnd},
	}}
	a := New(client, emptyRegistry{}, s)
	if err := a.RunTurn(ctx, contracts.TurnInput{
		SessionID: session.ID,
		TurnID:    turn.ID,
		Messages:  []contracts.Message{{SessionID: session.ID, TurnID: turn.ID, Role: "user", Content: turn.UserMessage}},
	}); err != nil {
		t.Fatalf("RunTurn: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for {
		state, ok, err := s.ProjectStates().Get(ctx, "/workspace")
		if err != nil {
			t.Fatalf("get project state: %v", err)
		}
		if ok && state.Summary == "Updated project state." && client.requestCount() >= 2 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("project state was not updated; requests=%d", client.requestCount())
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestRunTurnSkipsProjectStateUpdateForSmallRecentTurn(t *testing.T) {
	s, err := v2store.OpenInMemory()
	if err != nil {
		t.Fatalf("OpenInMemory: %v", err)
	}
	defer s.Close()

	ctx := context.Background()
	now := time.Now().UTC()
	session := contracts.Session{
		ID:                "session-1",
		WorkspaceRoot:     "/workspace",
		SystemPrompt:      "Base prompt.",
		PermissionProfile: "trusted_local",
		State:             "idle",
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	if err := s.Sessions().Create(ctx, session); err != nil {
		t.Fatalf("create session: %v", err)
	}
	if err := s.ProjectStates().Upsert(ctx, contracts.ProjectState{
		WorkspaceRoot:   session.WorkspaceRoot,
		Summary:         "# Current Goal\nAlready current.",
		SourceSessionID: session.ID,
		SourceTurnID:    "turn-0",
		CreatedAt:       now,
		UpdatedAt:       now,
	}); err != nil {
		t.Fatalf("upsert project state: %v", err)
	}
	turn := contracts.Turn{
		ID:          "turn-1",
		SessionID:   session.ID,
		Kind:        "user_message",
		State:       "created",
		UserMessage: "ok",
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := s.Turns().Create(ctx, turn); err != nil {
		t.Fatalf("create turn: %v", err)
	}

	client := &captureClient{events: []model.StreamEvent{
		{Kind: model.StreamEventTextDelta, TextDelta: "Done."},
		{Kind: model.StreamEventMessageEnd},
	}}
	a := New(client, emptyRegistry{}, s)
	if err := a.RunTurn(ctx, contracts.TurnInput{
		SessionID: session.ID,
		TurnID:    turn.ID,
		Messages:  []contracts.Message{{SessionID: session.ID, TurnID: turn.ID, Role: "user", Content: turn.UserMessage}},
	}); err != nil {
		t.Fatalf("RunTurn: %v", err)
	}

	time.Sleep(100 * time.Millisecond)
	if got := client.requestCount(); got != 1 {
		t.Fatalf("request count = %d, want only main turn request", got)
	}
}
