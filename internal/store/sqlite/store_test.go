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

func TestStoreListsSessionsInUpdatedAtDescOrder(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "agentd.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	older := time.Date(2026, 4, 4, 8, 0, 0, 0, time.UTC)
	newer := older.Add(5 * time.Minute)

	if err := store.InsertSession(context.Background(), types.Session{
		ID:            "sess_old",
		WorkspaceRoot: "D:/work/old",
		State:         types.SessionStateIdle,
		CreatedAt:     older,
		UpdatedAt:     older,
	}); err != nil {
		t.Fatalf("InsertSession(old) error = %v", err)
	}
	if err := store.InsertSession(context.Background(), types.Session{
		ID:            "sess_new",
		WorkspaceRoot: "D:/work/new",
		State:         types.SessionStateIdle,
		CreatedAt:     newer,
		UpdatedAt:     newer,
	}); err != nil {
		t.Fatalf("InsertSession(new) error = %v", err)
	}

	sessions, err := store.ListSessions(context.Background())
	if err != nil {
		t.Fatalf("ListSessions() error = %v", err)
	}
	if len(sessions) != 2 {
		t.Fatalf("len(sessions) = %d, want 2", len(sessions))
	}
	if sessions[0].ID != "sess_new" || sessions[1].ID != "sess_old" {
		t.Fatalf("sessions = %#v, want newest first", sessions)
	}
}

func TestStorePersistsSelectedSessionID(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "agentd.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	selected, ok, err := store.GetSelectedSessionID(context.Background())
	if err != nil {
		t.Fatalf("GetSelectedSessionID(initial) error = %v", err)
	}
	if ok || selected != "" {
		t.Fatalf("initial selected = %q, %v, want empty false", selected, ok)
	}

	if err := store.SetSelectedSessionID(context.Background(), "sess_focus"); err != nil {
		t.Fatalf("SetSelectedSessionID() error = %v", err)
	}

	selected, ok, err = store.GetSelectedSessionID(context.Background())
	if err != nil {
		t.Fatalf("GetSelectedSessionID(saved) error = %v", err)
	}
	if !ok || selected != "sess_focus" {
		t.Fatalf("selected = %q, %v, want %q true", selected, ok, "sess_focus")
	}
}

func TestStoreListsRunningTurnsOldestFirst(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "agentd.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	older := time.Date(2026, 4, 4, 8, 0, 0, 0, time.UTC)
	newer := older.Add(5 * time.Minute)

	if err := store.InsertTurn(context.Background(), types.Turn{
		ID:          "turn_created",
		SessionID:   "sess_1",
		State:       types.TurnStateCreated,
		UserMessage: "ignore me",
		CreatedAt:   older.Add(2 * time.Minute),
		UpdatedAt:   older.Add(2 * time.Minute),
	}); err != nil {
		t.Fatalf("InsertTurn(created) error = %v", err)
	}

	if err := store.InsertTurn(context.Background(), types.Turn{
		ID:          "turn_running_old",
		SessionID:   "sess_1",
		State:       types.TurnStateModelStreaming,
		UserMessage: "first running",
		CreatedAt:   older,
		UpdatedAt:   older,
	}); err != nil {
		t.Fatalf("InsertTurn(running old) error = %v", err)
	}

	if err := store.InsertTurn(context.Background(), types.Turn{
		ID:          "turn_running_new",
		SessionID:   "sess_2",
		State:       types.TurnStateToolRunning,
		UserMessage: "second running",
		CreatedAt:   newer,
		UpdatedAt:   newer,
	}); err != nil {
		t.Fatalf("InsertTurn(running new) error = %v", err)
	}

	turns, err := store.ListRunningTurns(context.Background())
	if err != nil {
		t.Fatalf("ListRunningTurns() error = %v", err)
	}
	if len(turns) != 2 {
		t.Fatalf("len(turns) = %d, want 2", len(turns))
	}
	if turns[0].ID != "turn_running_old" || turns[0].State != types.TurnStateModelStreaming {
		t.Fatalf("turns[0] = %#v, want oldest running turn", turns[0])
	}
	if turns[1].ID != "turn_running_new" || turns[1].State != types.TurnStateToolRunning {
		t.Fatalf("turns[1] = %#v, want newest running turn", turns[1])
	}
}

func TestStorePersistsProviderCacheHeadsAndCompactions(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "agentd.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	now := time.Date(2026, 4, 4, 8, 0, 0, 0, time.UTC)

	head := types.ProviderCacheHead{
		SessionID:         "sess_cache",
		Provider:          "openai_compatible",
		CapabilityProfile: "ark_responses",
		ActiveSessionRef:  "resp_active",
		ActivePrefixRef:   "resp_prefix",
		ActiveGeneration:  3,
		UpdatedAt:         now,
	}
	if err := store.UpsertProviderCacheHead(context.Background(), head); err != nil {
		t.Fatalf("UpsertProviderCacheHead() error = %v", err)
	}

	entry := types.ProviderCacheEntry{
		ID:                "cache_entry_1",
		SessionID:         head.SessionID,
		Provider:          head.Provider,
		CapabilityProfile: head.CapabilityProfile,
		CacheKind:         "session",
		ExternalRef:       "resp_active",
		Generation:        3,
		Status:            "active",
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	if err := store.InsertProviderCacheEntry(context.Background(), entry); err != nil {
		t.Fatalf("InsertProviderCacheEntry() error = %v", err)
	}

	compaction := types.ConversationCompaction{
		ID:              "compaction_1",
		SessionID:       head.SessionID,
		Kind:            "rolling",
		Generation:      3,
		StartPosition:   1,
		EndPosition:     4,
		SummaryPayload:  `{"range_label":"turns 1-4"}`,
		Reason:          "token_budget",
		ProviderProfile: head.CapabilityProfile,
		CreatedAt:       now,
	}
	if err := store.InsertConversationCompaction(context.Background(), compaction); err != nil {
		t.Fatalf("InsertConversationCompaction() error = %v", err)
	}

	gotHead, ok, err := store.GetProviderCacheHead(context.Background(), head.SessionID, head.Provider, head.CapabilityProfile)
	if err != nil {
		t.Fatalf("GetProviderCacheHead() error = %v", err)
	}
	if !ok {
		t.Fatal("GetProviderCacheHead() ok = false, want true")
	}
	if gotHead.ActiveSessionRef != head.ActiveSessionRef || gotHead.ActivePrefixRef != head.ActivePrefixRef || gotHead.ActiveGeneration != head.ActiveGeneration {
		t.Fatalf("GetProviderCacheHead() = %#v, want %#v", gotHead, head)
	}

	var count int
	if err := store.db.QueryRowContext(context.Background(), `select count(*) from provider_cache_entries where id = ?`, entry.ID).Scan(&count); err != nil {
		t.Fatalf("count provider_cache_entries error = %v", err)
	}
	if count != 1 {
		t.Fatalf("provider_cache_entries count = %d, want 1", count)
	}

	if err := store.db.QueryRowContext(context.Background(), `select count(*) from conversation_compactions where id = ?`, compaction.ID).Scan(&count); err != nil {
		t.Fatalf("count conversation_compactions error = %v", err)
	}
	if count != 1 {
		t.Fatalf("conversation_compactions count = %d, want 1", count)
	}
}

func TestStoreProviderCacheHeadsAreScopedByCapabilityProfile(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "agentd.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	now := time.Date(2026, 4, 4, 8, 0, 0, 0, time.UTC)

	arkHead := types.ProviderCacheHead{
		SessionID:         "sess_cache",
		Provider:          "openai_compatible",
		CapabilityProfile: "ark_responses",
		ActiveSessionRef:  "resp_ark",
		ActivePrefixRef:   "pref_ark",
		ActiveGeneration:  2,
		UpdatedAt:         now,
	}
	if err := store.UpsertProviderCacheHead(context.Background(), arkHead); err != nil {
		t.Fatalf("UpsertProviderCacheHead(ark) error = %v", err)
	}

	otherHead := types.ProviderCacheHead{
		SessionID:         "sess_cache",
		Provider:          "openai_compatible",
		CapabilityProfile: "anthropic_native",
		ActiveSessionRef:  "resp_other",
		ActivePrefixRef:   "pref_other",
		ActiveGeneration:  5,
		UpdatedAt:         now.Add(time.Minute),
	}
	if err := store.UpsertProviderCacheHead(context.Background(), otherHead); err != nil {
		t.Fatalf("UpsertProviderCacheHead(other) error = %v", err)
	}

	gotArk, ok, err := store.GetProviderCacheHead(context.Background(), arkHead.SessionID, arkHead.Provider, arkHead.CapabilityProfile)
	if err != nil {
		t.Fatalf("GetProviderCacheHead(ark) error = %v", err)
	}
	if !ok {
		t.Fatal("GetProviderCacheHead(ark) ok = false, want true")
	}
	if gotArk.ActiveSessionRef != arkHead.ActiveSessionRef || gotArk.ActivePrefixRef != arkHead.ActivePrefixRef || gotArk.ActiveGeneration != arkHead.ActiveGeneration {
		t.Fatalf("GetProviderCacheHead(ark) = %#v, want %#v", gotArk, arkHead)
	}

	gotOther, ok, err := store.GetProviderCacheHead(context.Background(), otherHead.SessionID, otherHead.Provider, otherHead.CapabilityProfile)
	if err != nil {
		t.Fatalf("GetProviderCacheHead(other) error = %v", err)
	}
	if !ok {
		t.Fatal("GetProviderCacheHead(other) ok = false, want true")
	}
	if gotOther.ActiveSessionRef != otherHead.ActiveSessionRef || gotOther.ActivePrefixRef != otherHead.ActivePrefixRef || gotOther.ActiveGeneration != otherHead.ActiveGeneration {
		t.Fatalf("GetProviderCacheHead(other) = %#v, want %#v", gotOther, otherHead)
	}

	var count int
	if err := store.db.QueryRowContext(context.Background(), `
		select count(*)
		from provider_cache_heads
		where session_id = ? and provider = ?
	`, arkHead.SessionID, arkHead.Provider).Scan(&count); err != nil {
		t.Fatalf("count provider_cache_heads error = %v", err)
	}
	if count != 2 {
		t.Fatalf("provider_cache_heads count = %d, want 2", count)
	}
}
