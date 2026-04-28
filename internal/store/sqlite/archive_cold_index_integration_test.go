package sqlite

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go-agent/internal/model"
	"go-agent/internal/types"
)

func TestArchiveCompactionToColdIndexToSearch(t *testing.T) {
	ctx := context.Background()
	store, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	now := time.Now().UTC()
	if err := store.InsertSession(ctx, types.Session{
		ID:            "sess_ar",
		WorkspaceRoot: "/ws",
		State:         types.SessionStateIdle,
		CreatedAt:     now,
		UpdatedAt:     now,
	}); err != nil {
		t.Fatalf("InsertSession() error = %v", err)
	}

	items := []model.ConversationItem{
		{Kind: model.ConversationItemAssistantText, Text: "Let me check the handler code"},
		archiveTestToolCall("read", "read handler.go"),
		archiveTestToolResult("read", "package handler ...", false),
		{Kind: model.ConversationItemAssistantText, Text: "I see a compile error in handler.go"},
		archiveTestToolCall("go build", "go build"),
		archiveTestToolResult("go build", "compile error: undefined: context", true),
		{Kind: model.ConversationItemAssistantText, Text: "Need to import context in handler.go"},
		archiveTestToolCall("edit", "edit handler.go"),
		archiveTestToolResult("edit", "edit successful", false),
		{Kind: model.ConversationItemUserMessage, Text: "thanks"},
	}
	for i, item := range items {
		position := i + 1
		if err := store.InsertConversationItemWithContextHead(ctx, "sess_ar", "head_ar", "turn_1", position, item); err != nil {
			t.Fatalf("InsertConversationItemWithContextHead(%d) error = %v", position, err)
		}
	}

	startItemID, found, err := store.GetConversationItemIDByContextHeadAndPosition(ctx, "sess_ar", "head_ar", 1)
	if err != nil {
		t.Fatalf("GetConversationItemIDByContextHeadAndPosition(1) error = %v", err)
	}
	if !found {
		t.Fatalf("conversation item at position 1 missing")
	}
	endItemID, found, err := store.GetConversationItemIDByContextHeadAndPosition(ctx, "sess_ar", "head_ar", 8)
	if err != nil {
		t.Fatalf("GetConversationItemIDByContextHeadAndPosition(8) error = %v", err)
	}
	if !found {
		t.Fatalf("conversation item at position 8 missing")
	}
	if err := store.InsertConversationCompactionWithContextHead(ctx, types.ConversationCompaction{
		ID:              "arc1",
		SessionID:       "sess_ar",
		ContextHeadID:   "head_ar",
		Kind:            types.ConversationCompactionKindArchive,
		Generation:      1,
		StartItemID:     startItemID,
		EndItemID:       endItemID,
		StartPosition:   1,
		EndPosition:     8,
		SummaryPayload:  "{}",
		MetadataJSON:    "{}",
		Reason:          "test",
		ProviderProfile: "test",
		CreatedAt:       now,
	}); err != nil {
		t.Fatalf("InsertConversationCompactionWithContextHead() error = %v", err)
	}

	if err := store.InsertColdIndexEntry(ctx, types.ColdIndexEntry{
		ID:           "cold_archive_arc1",
		WorkspaceID:  "/ws",
		SourceType:   "archive",
		SourceID:     "arc1",
		SearchText:   "compile error handler missing import context fix",
		SummaryLine:  "[turns 1-8] Fixed compile error in handler.go by adding missing context import",
		FilesChanged: []string{"pkg/handler.go", "pkg/model.go"},
		ToolsUsed:    []string{"go build", "read", "edit"},
		ErrorTypes:   []string{"compile_error"},
		OccurredAt:   now,
		CreatedAt:    now,
		ContextRef: types.ColdContextRef{
			SessionID:     "sess_ar",
			ContextHeadID: "head_ar",
			TurnStartPos:  1,
			TurnEndPos:    8,
			ItemCount:     8,
		},
	}); err != nil {
		t.Fatalf("InsertColdIndexEntry() error = %v", err)
	}

	compileMatches := assertColdSearchCount(t, ctx, store, types.ColdSearchQuery{
		WorkspaceID: "/ws",
		TextQuery:   "compile handler",
		Limit:       10,
	}, 1)
	if !strings.Contains(compileMatches[0].SummaryLine, "compile error") {
		t.Fatalf("SummaryLine = %q, want compile error", compileMatches[0].SummaryLine)
	}
	assertColdSearchCount(t, ctx, store, types.ColdSearchQuery{
		WorkspaceID: "/ws",
		TextQuery:   "model",
		Limit:       10,
	}, 1)
	assertColdSearchCount(t, ctx, store, types.ColdSearchQuery{
		WorkspaceID: "/ws",
		TextQuery:   "database",
		Limit:       10,
	}, 0)
	assertColdSearchCount(t, ctx, store, types.ColdSearchQuery{
		WorkspaceID:  "/ws",
		FilesTouched: []string{"pkg/handler.go"},
		Limit:        10,
	}, 1)
	assertColdSearchCount(t, ctx, store, types.ColdSearchQuery{
		WorkspaceID: "/ws",
		SourceTypes: []string{"archive"},
		Limit:       10,
	}, 1)
	assertColdSearchCount(t, ctx, store, types.ColdSearchQuery{
		WorkspaceID: "/ws",
		ToolsUsed:   []string{"go build"},
		Limit:       10,
	}, 1)
	assertColdSearchCount(t, ctx, store, types.ColdSearchQuery{
		WorkspaceID: "/ws",
		ErrorTypes:  []string{"compile_error"},
		Limit:       10,
	}, 1)
	assertColdSearchCount(t, ctx, store, types.ColdSearchQuery{
		WorkspaceID: "/ws",
		SourceTypes: []string{"memory_deprecated"},
		Limit:       10,
	}, 0)

	entry, found, err := store.GetColdIndexEntry(ctx, "cold_archive_arc1")
	if err != nil {
		t.Fatalf("GetColdIndexEntry() error = %v", err)
	}
	if !found {
		t.Fatalf("GetColdIndexEntry() found = false, want true")
	}
	if entry.ContextRef.TurnStartPos != 1 || entry.ContextRef.TurnEndPos != 8 || entry.ContextRef.ItemCount != 8 {
		t.Fatalf("ContextRef = %#v, want turn range 1-8 with 8 items", entry.ContextRef)
	}
}

func archiveTestToolCall(name, text string) model.ConversationItem {
	return model.ConversationItem{
		Kind: model.ConversationItemToolCall,
		Text: text,
		ToolCall: model.ToolCallChunk{
			ID:       "call_" + strings.ReplaceAll(name, " ", "_"),
			Name:     name,
			InputRaw: text,
		},
	}
}

func archiveTestToolResult(toolName, content string, isError bool) model.ConversationItem {
	result := model.ToolResult{
		ToolName: toolName,
		Content:  content,
		IsError:  isError,
	}
	return model.ConversationItem{
		Kind:   model.ConversationItemToolResult,
		Text:   content,
		Result: &result,
	}
}

func assertColdSearchCount(t *testing.T, ctx context.Context, store *Store, query types.ColdSearchQuery, want int) []types.ColdIndexEntry {
	t.Helper()
	entries, total, err := store.SearchColdIndex(ctx, query)
	if err != nil {
		t.Fatalf("SearchColdIndex(%#v) error = %v", query, err)
	}
	if total != want || len(entries) != want {
		t.Fatalf("SearchColdIndex(%#v) returned %d/%d entries, want %d/%d", query, len(entries), total, want, want)
	}
	return entries
}
