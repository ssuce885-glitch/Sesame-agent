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

func TestSplitMessagesForCompactionKeepsWholeRecentTurns(t *testing.T) {
	tokenText := func(tokens int) string { return strings.Repeat("abcd", tokens) }
	messages := []contracts.Message{
		{TurnID: "turn-1", Role: "user", Content: tokenText(1000)},
		{TurnID: "turn-1", Role: "assistant", Content: tokenText(1000)},
		{TurnID: "turn-2", Role: "user", Content: tokenText(1000)},
		{TurnID: "turn-2", Role: "assistant", Content: tokenText(1000)},
		{TurnID: "turn-3", Role: "user", Content: tokenText(1000)},
	}

	summarize, keep := splitMessagesForCompaction(messages, 3500)
	if len(summarize) != 2 {
		t.Fatalf("summarize count = %d, want first turn", len(summarize))
	}
	if summarize[0].TurnID != "turn-1" || summarize[1].TurnID != "turn-1" {
		t.Fatalf("summarize split a turn: %+v", summarize)
	}
	if len(keep) != 3 || keep[0].TurnID != "turn-2" || keep[2].TurnID != "turn-3" {
		t.Fatalf("unexpected kept messages: %+v", keep)
	}
}

func TestCompactPriorIfNeededPersistsBoundarySummaryAndSnapshot(t *testing.T) {
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
		ID:          "turn-new",
		SessionID:   session.ID,
		Kind:        "user_message",
		State:       "created",
		UserMessage: "continue",
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := s.Turns().Create(ctx, turn); err != nil {
		t.Fatalf("create turn: %v", err)
	}

	tokenText := func(tokens int) string { return strings.Repeat("abcd", tokens) }
	prior := []contracts.Message{
		{SessionID: session.ID, TurnID: "turn-old", Role: "user", Content: tokenText(145000), Position: 1, CreatedAt: now},
		{SessionID: session.ID, TurnID: "turn-recent", Role: "assistant", Content: tokenText(30000), Position: 2, CreatedAt: now},
	}
	if err := s.Messages().Append(ctx, prior); err != nil {
		t.Fatalf("append prior: %v", err)
	}
	client := &captureClient{events: []model.StreamEvent{
		{Kind: model.StreamEventTextDelta, TextDelta: "# Summary\nOld work summarized."},
		{Kind: model.StreamEventMessageEnd},
	}}
	a := New(client, emptyRegistry{}, s)

	got, err := a.compactPriorIfNeeded(ctx, contracts.TurnInput{
		SessionID: session.ID,
		TurnID:    turn.ID,
	}, session.SystemPrompt, prior, []contracts.Message{{Role: "user", Content: "continue"}}, 0)
	if err != nil {
		t.Fatalf("compactPriorIfNeeded: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("compacted prior count = %d, want boundary + summary + kept", len(got))
	}
	if !isCompactBoundaryMessage(got[0]) {
		t.Fatalf("first message is not compact boundary: %+v", got[0])
	}
	if !isCompactSummaryMessage(got[1]) || !strings.Contains(compactSummaryContent(got[1]), "Old work summarized.") {
		t.Fatalf("second message is not compact summary: %+v", got[1])
	}
	if got[2].TurnID != "turn-recent" {
		t.Fatalf("kept message = %+v, want recent turn", got[2])
	}

	persisted, err := s.Messages().List(ctx, session.ID, contracts.MessageListOptions{})
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}
	if len(persisted) != 4 {
		t.Fatalf("persisted message count = %d, want original 2 + compact 2", len(persisted))
	}
	if !isCompactBoundaryMessage(persisted[2]) || !isCompactSummaryMessage(persisted[3]) {
		t.Fatalf("compact messages were not persisted at tail: %+v", persisted)
	}
	snapshotID := strings.TrimPrefix(persisted[2].Content, compactBoundaryPrefix)
	snapshot, err := s.Messages().LoadSnapshot(ctx, snapshotID)
	if err != nil {
		t.Fatalf("load snapshot: %v", err)
	}
	if len(snapshot) != 1 || snapshot[0].TurnID != "turn-old" {
		t.Fatalf("snapshot = %+v, want summarized old turn", snapshot)
	}
}
