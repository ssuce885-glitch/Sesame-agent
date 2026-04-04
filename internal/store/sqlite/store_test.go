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

	firstItem := model.ConversationItem{
		Kind: model.ConversationItemUserMessage,
		Text: "inspect repository",
	}
	if err := store.InsertConversationItem(context.Background(), "sess_1", "turn_1", 2, firstItem); err != nil {
		t.Fatalf("InsertConversationItem() error = %v", err)
	}

	secondItem := model.ConversationItem{
		Kind: model.ConversationItemAssistantText,
		Text: "use glob first",
	}
	if err := store.InsertConversationItem(context.Background(), "sess_1", "turn_1", 1, secondItem); err != nil {
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
	if len(items) != 2 {
		t.Fatalf("len(items) = %d, want 2", len(items))
	}
	if items[0].Kind != model.ConversationItemAssistantText || items[0].Text != "use glob first" {
		t.Fatalf("first item = %#v, want assistant text round-trip", items[0])
	}
	if items[1].Kind != model.ConversationItemUserMessage || items[1].Text != "inspect repository" {
		t.Fatalf("second item = %#v, want user message round-trip", items[1])
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
	if len(summaries[0].ImportantChoices) != 1 || summaries[0].ImportantChoices[0] != "use glob first" {
		t.Fatalf("ImportantChoices = %#v, want [%q]", summaries[0].ImportantChoices, "use glob first")
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

func TestStoreDeleteTurnRemovesConversationItems(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "agentd.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	turn := types.Turn{
		ID:          "turn_cleanup",
		SessionID:   "sess_cleanup",
		State:       types.TurnStateCreated,
		UserMessage: "hello",
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}
	if err := store.InsertTurn(context.Background(), turn); err != nil {
		t.Fatalf("InsertTurn() error = %v", err)
	}
	if err := store.InsertConversationItem(context.Background(), turn.SessionID, turn.ID, 1, model.ConversationItem{
		Kind: model.ConversationItemUserMessage,
		Text: "hello",
	}); err != nil {
		t.Fatalf("InsertConversationItem() error = %v", err)
	}

	if err := store.DeleteTurn(context.Background(), turn.ID); err != nil {
		t.Fatalf("DeleteTurn() error = %v", err)
	}

	items, err := store.ListConversationItems(context.Background(), turn.SessionID)
	if err != nil {
		t.Fatalf("ListConversationItems() error = %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("len(items) = %d, want 0", len(items))
	}
}

func TestStoreListsMemoryEntriesByWorkspaceInUpdatedAtOrderWithUTCNormalization(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "agentd.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	older := time.Date(2024, 1, 1, 2, 0, 0, 0, time.UTC)
	newer := time.Date(2024, 1, 1, 0, 30, 0, 0, time.FixedZone("EST", -5*60*60))

	if err := store.InsertMemoryEntry(context.Background(), types.MemoryEntry{
		ID:          "mem_old",
		Scope:       types.MemoryScopeWorkspace,
		WorkspaceID: "ws_utc",
		Content:     "older",
		SourceRefs:  []string{"turn_old"},
		Confidence:  0.5,
		CreatedAt:   older,
		UpdatedAt:   older,
	}); err != nil {
		t.Fatalf("InsertMemoryEntry() error = %v", err)
	}

	if err := store.InsertMemoryEntry(context.Background(), types.MemoryEntry{
		ID:          "mem_new",
		Scope:       types.MemoryScopeWorkspace,
		WorkspaceID: "ws_utc",
		Content:     "newer",
		SourceRefs:  []string{"turn_new"},
		Confidence:  0.9,
		CreatedAt:   newer,
		UpdatedAt:   newer,
	}); err != nil {
		t.Fatalf("InsertMemoryEntry() error = %v", err)
	}

	entries, err := store.ListMemoryEntriesByWorkspace(context.Background(), "ws_utc")
	if err != nil {
		t.Fatalf("ListMemoryEntriesByWorkspace() error = %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("len(entries) = %d, want 2", len(entries))
	}
	if entries[0].ID != "mem_new" || entries[0].Scope != types.MemoryScopeWorkspace || len(entries[0].SourceRefs) != 1 || entries[0].SourceRefs[0] != "turn_new" {
		t.Fatalf("entries[0] = %#v, want newest entry round-trip", entries[0])
	}
	if !entries[0].UpdatedAt.Equal(newer.UTC()) || !entries[0].CreatedAt.Equal(newer.UTC()) {
		t.Fatalf("entries[0] times = %s/%s, want %s", entries[0].CreatedAt, entries[0].UpdatedAt, newer.UTC())
	}
	if entries[1].ID != "mem_old" {
		t.Fatalf("entries[1].ID = %q, want %q", entries[1].ID, "mem_old")
	}
}
