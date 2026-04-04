package sqlite

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"go-agent/internal/model"
	"go-agent/internal/types"
)

func TestStorePersistsSessionTurnAndEvent(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "agentd.db")

	store, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	session := types.Session{
		ID:            "sess_test",
		WorkspaceRoot: "D:/work/demo",
		State:         types.SessionStateIdle,
		CreatedAt:     time.Now().UTC(),
		UpdatedAt:     time.Now().UTC(),
	}
	if err := store.InsertSession(context.Background(), session); err != nil {
		t.Fatalf("InsertSession() error = %v", err)
	}

	turn := types.Turn{
		ID:          "turn_test",
		SessionID:   session.ID,
		State:       types.TurnStateCreated,
		UserMessage: "hello",
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}
	if err := store.InsertTurn(context.Background(), turn); err != nil {
		t.Fatalf("InsertTurn() error = %v", err)
	}

	event, err := types.NewEvent(session.ID, turn.ID, types.EventTurnStarted, types.TurnStartedPayload{
		WorkspaceRoot: session.WorkspaceRoot,
	})
	if err != nil {
		t.Fatalf("NewEvent() error = %v", err)
	}
	if _, err := store.AppendEvent(context.Background(), event); err != nil {
		t.Fatalf("AppendEvent() error = %v", err)
	}

	loaded, err := store.ListSessionEvents(context.Background(), session.ID, 0)
	if err != nil {
		t.Fatalf("ListSessionEvents() error = %v", err)
	}
	if len(loaded) != 1 {
		t.Fatalf("len(events) = %d, want 1", len(loaded))
	}
	if loaded[0].Seq != 1 {
		t.Fatalf("Seq = %d, want 1", loaded[0].Seq)
	}
}

func TestStoreDeleteTurnRemovesRow(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "agentd.db")

	store, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	turn := types.Turn{
		ID:          "turn_delete",
		SessionID:   "sess_test",
		State:       types.TurnStateCreated,
		UserMessage: "hello",
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}
	if err := store.InsertTurn(context.Background(), turn); err != nil {
		t.Fatalf("InsertTurn() error = %v", err)
	}
	if err := store.DeleteTurn(context.Background(), turn.ID); err != nil {
		t.Fatalf("DeleteTurn() error = %v", err)
	}

	var count int
	if err := store.db.QueryRowContext(context.Background(), `select count(*) from turns where id = ?`, turn.ID).Scan(&count); err != nil {
		t.Fatalf("count query error = %v", err)
	}
	if count != 0 {
		t.Fatalf("count = %d, want 0", count)
	}
}

func TestStorePersistsConversationItemsAndSummaries(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "agentd.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	item := model.ConversationItem{
		Kind: model.ConversationItemUserMessage,
		Text: "inspect repository",
	}
	if err := store.InsertConversationItem(context.Background(), "sess_1", "turn_1", 10, item); err != nil {
		t.Fatalf("InsertConversationItem() error = %v", err)
	}

	summary := model.Summary{
		RangeLabel:       "turns 1-4",
		UserGoals:        []string{"inspect repository"},
		ImportantChoices: []string{"use glob first"},
	}
	if err := store.InsertConversationSummary(context.Background(), "sess_1", 4, summary); err != nil {
		t.Fatalf("InsertConversationSummary() error = %v", err)
	}

	items, err := store.ListConversationItems(context.Background(), "sess_1")
	if err != nil {
		t.Fatalf("ListConversationItems() error = %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("len(items) = %d, want 1", len(items))
	}

	summaries, err := store.ListConversationSummaries(context.Background(), "sess_1")
	if err != nil {
		t.Fatalf("ListConversationSummaries() error = %v", err)
	}
	if len(summaries) != 1 {
		t.Fatalf("len(summaries) = %d, want 1", len(summaries))
	}
	if summaries[0].RangeLabel != "turns 1-4" {
		t.Fatalf("RangeLabel = %q, want %q", summaries[0].RangeLabel, "turns 1-4")
	}

	entry := types.MemoryEntry{
		ID:          "mem_1",
		Scope:       types.MemoryScopeWorkspace,
		WorkspaceID: "ws_1",
		Content:     "workspace prefers rg before grep fallback",
		SourceRefs:  []string{"turn_1"},
		Confidence:  0.9,
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}
	if err := store.InsertMemoryEntry(context.Background(), entry); err != nil {
		t.Fatalf("InsertMemoryEntry() error = %v", err)
	}

	entries, err := store.ListMemoryEntriesByWorkspace(context.Background(), "ws_1")
	if err != nil {
		t.Fatalf("ListMemoryEntriesByWorkspace() error = %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("len(entries) = %d, want 1", len(entries))
	}
}
