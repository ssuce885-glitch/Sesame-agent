package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"go-agent/internal/model"
	"go-agent/internal/store/sqlite"
	"go-agent/internal/types"
)

const (
	historySessionID = "sess_history_v1"
	historyHeadID    = "head_history"
)

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintf(os.Stderr, "usage: go run ./cmd/prefill-test-db/ <db_path> <workspace_root>\n")
		os.Exit(2)
	}

	if err := run(context.Background(), os.Args[1], os.Args[2]); err != nil {
		fmt.Fprintf(os.Stderr, "prefill test db: %v\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, dbPath, workspaceRoot string) error {
	workspaceRoot = strings.TrimSpace(workspaceRoot)
	if strings.TrimSpace(dbPath) == "" {
		return fmt.Errorf("db_path is required")
	}
	if workspaceRoot == "" {
		return fmt.Errorf("workspace_root is required")
	}

	store, err := sqlite.Open(dbPath)
	if err != nil {
		return fmt.Errorf("open sqlite db: %w", err)
	}
	defer store.Close()

	now := time.Now().UTC()
	oneDayAgo := now.Add(-24 * time.Hour)

	if err := store.InsertSession(ctx, types.Session{
		ID:            historySessionID,
		WorkspaceRoot: workspaceRoot,
		State:         types.SessionStateIdle,
		CreatedAt:     now,
		UpdatedAt:     now,
	}); err != nil {
		return fmt.Errorf("insert session: %w", err)
	}

	if err := store.InsertContextHead(ctx, types.ContextHead{
		ID:         historyHeadID,
		SessionID:  historySessionID,
		SourceKind: types.ContextHeadSourceBootstrap,
		Title:      "Historical database optimization",
		Preview:    "Added an orders.user_id index and tuned connection pooling.",
		CreatedAt:  oneDayAgo,
		UpdatedAt:  oneDayAgo,
	}); err != nil {
		return fmt.Errorf("insert context head: %w", err)
	}

	conversationItems, err := historyConversationItems()
	if err != nil {
		return err
	}
	for idx, item := range conversationItems {
		position := idx + 1
		if err := store.InsertConversationItemWithContextHead(ctx, historySessionID, historyHeadID, "turn_history_1", position, item); err != nil {
			return fmt.Errorf("insert conversation item %d: %w", position, err)
		}
	}

	for _, turn := range []types.Turn{
		{
			ID:            "turn_history_1",
			SessionID:     historySessionID,
			ContextHeadID: historyHeadID,
			Kind:          types.TurnKindUserMessage,
			State:         types.TurnStateCompleted,
			UserMessage:   "database optimization work",
			CreatedAt:     oneDayAgo,
			UpdatedAt:     oneDayAgo,
		},
		{
			ID:            "turn_history_2",
			SessionID:     historySessionID,
			ContextHeadID: historyHeadID,
			Kind:          types.TurnKindUserMessage,
			State:         types.TurnStateCompleted,
			UserMessage:   "connection pooling configuration",
			CreatedAt:     oneDayAgo,
			UpdatedAt:     oneDayAgo,
		},
	} {
		if err := store.InsertTurn(ctx, turn); err != nil {
			return fmt.Errorf("insert turn %s: %w", turn.ID, err)
		}
	}

	startItemID, found, err := store.GetConversationItemIDByContextHeadAndPosition(ctx, historySessionID, historyHeadID, 1)
	if err != nil {
		return fmt.Errorf("get start item id: %w", err)
	}
	if !found {
		return fmt.Errorf("start conversation item not found")
	}
	endItemID, found, err := store.GetConversationItemIDByContextHeadAndPosition(ctx, historySessionID, historyHeadID, 25)
	if err != nil {
		return fmt.Errorf("get end item id: %w", err)
	}
	if !found {
		return fmt.Errorf("end conversation item not found")
	}

	if err := store.InsertConversationCompactionWithContextHead(ctx, types.ConversationCompaction{
		ID:              "compact_history_1",
		SessionID:       historySessionID,
		ContextHeadID:   historyHeadID,
		Kind:            types.ConversationCompactionKindArchive,
		Generation:      1,
		StartItemID:     startItemID,
		EndItemID:       endItemID,
		StartPosition:   1,
		EndPosition:     25,
		Reason:          "token threshold",
		SummaryPayload:  "{}",
		MetadataJSON:    "{}",
		ProviderProfile: "test",
		CreatedAt:       oneDayAgo,
	}); err != nil {
		return fmt.Errorf("insert archive compaction: %w", err)
	}

	if err := store.InsertColdIndexEntry(ctx, archiveColdIndexEntry(workspaceRoot, oneDayAgo)); err != nil {
		return fmt.Errorf("insert archive cold index entry: %w", err)
	}

	memories := memoryEntries(workspaceRoot, now)
	memoryIDs := make([]string, 0, len(memories))
	for _, entry := range memories {
		entry.Status = types.MemoryStatusActive
		if err := store.UpsertMemoryEntry(ctx, entry); err != nil {
			return fmt.Errorf("upsert memory %s: %w", entry.ID, err)
		}
		memoryIDs = append(memoryIDs, entry.ID)
	}
	if err := store.DeprecateMemoryEntries(ctx, memoryIDs); err != nil {
		return fmt.Errorf("deprecate memories: %w", err)
	}
	for _, entry := range memories {
		entry.Status = types.MemoryStatusDeprecated
		// DeprecateMemoryEntries stamps updated_at to now; restore fixture ages so
		// memory scoring and lifecycle tests see these as historical rows.
		if err := store.UpsertMemoryEntry(ctx, entry); err != nil {
			return fmt.Errorf("restore deprecated memory %s: %w", entry.ID, err)
		}
		if err := store.InsertColdIndexEntry(ctx, memoryColdIndexEntry(workspaceRoot, entry)); err != nil {
			return fmt.Errorf("insert memory cold index entry %s: %w", entry.ID, err)
		}
	}

	report := oldDatabaseAuditReport(workspaceRoot, now)
	if err := store.UpsertReport(ctx, report); err != nil {
		return fmt.Errorf("upsert report: %w", err)
	}
	if err := store.InsertColdIndexEntry(ctx, reportColdIndexEntry(workspaceRoot, report)); err != nil {
		return fmt.Errorf("insert report cold index entry: %w", err)
	}

	coldIndexCount, err := countColdIndexRows(ctx, store)
	if err != nil {
		return err
	}

	fmt.Println("Pre-filled DB with:")
	fmt.Println("  Session: sess_history_v1")
	fmt.Println("  Conversation items: 25")
	fmt.Println("  Turns: 2")
	fmt.Println("  Archive compaction: 1 (positions 1-25)")
	fmt.Println("  Cold index entries: 6 (1 archive + 4 memory_deprecated + 1 report)")
	fmt.Println("  Deprecated memories: 4")
	fmt.Println("  Reports: 1")
	fmt.Printf("  Total rows in cold_index: %d\n", coldIndexCount)

	return nil
}

func historyConversationItems() ([]model.ConversationItem, error) {
	return []model.ConversationItem{
		assistantText("Let me check the current database setup."),
		mustToolCall("call_history_2", "file_read", `{"path":"db/schema.sql"}`),
		toolResult("call_history_2", "file_read", "CREATE TABLE users (id UUID PRIMARY KEY, name TEXT, email TEXT); CREATE TABLE orders (id UUID PRIMARY KEY, user_id UUID REFERENCES users, total DECIMAL);", false),
		assistantText("I can see the schema. Users table has id, name, email. Orders reference users via user_id."),
		userText("We need to add an index on orders.user_id for performance."),
		assistantText("Good catch. Without an index, JOIN queries on orders.user_id will do full table scans."),
		mustToolCall("call_history_7", "file_read", `{"path":"db/migrations/004_add_order_index.sql"}`),
		toolResult("call_history_7", "file_read", "(file not found)", true),
		assistantText("The migration file doesn't exist yet. Let me create it."),
		mustToolCall("call_history_10", "file_write", `{"path":"db/migrations/004_add_order_index.sql","content":"CREATE INDEX idx_orders_user_id ON orders(user_id);"}`),
		toolResult("call_history_10", "file_write", "Wrote db/migrations/004_add_order_index.sql", false),
		assistantText("Created the migration. This should speed up user-order JOINs significantly."),
		userText("Also, we should set up connection pooling. What's the current config?"),
		assistantText("Let me check the database configuration."),
		mustToolCall("call_history_15", "file_read", `{"path":"config/database.yaml"}`),
		toolResult("call_history_15", "file_read", "database:\n  host: localhost\n  port: 5432\n  name: myapp\n  pool_size: 10\n  timeout: 30s", false),
		assistantText("Current pool_size is 10. For production load we typically want 25-50 connections."),
		userText("Change it to 40 and add a connection timeout of 5 seconds."),
		mustToolCall("call_history_19", "file_edit", `{"path":"config/database.yaml","old":"pool_size: 10","new":"pool_size: 40"}`),
		toolResult("call_history_19", "file_edit", "Updated config/database.yaml", false),
		assistantText("Updated pool_size to 40. Now about the timeout - there's already a 30s timeout. Did you want 5s for the connection timeout specifically?"),
		userText("Yes, connection timeout should be 5s, not query timeout."),
		mustToolCall("call_history_23", "file_edit", `{"path":"config/database.yaml","old":"timeout: 30s","new":"connection_timeout: 5s\n  query_timeout: 30s"}`),
		toolResult("call_history_23", "file_edit", "Updated config/database.yaml", false),
		assistantText("Done. Connection timeout is 5s, query timeout remains 30s, pool_size is 40."),
	}, nil
}

func assistantText(text string) model.ConversationItem {
	return model.ConversationItem{Kind: model.ConversationItemAssistantText, Text: text}
}

func userText(text string) model.ConversationItem {
	return model.ConversationItem{Kind: model.ConversationItemUserMessage, Text: text}
}

func mustToolCall(id, name, args string) model.ConversationItem {
	var input map[string]any
	if err := json.Unmarshal([]byte(args), &input); err != nil {
		panic(fmt.Sprintf("invalid fixture tool args for %s: %v", id, err))
	}
	return model.ConversationItem{
		Kind: model.ConversationItemToolCall,
		Text: name + " " + args,
		ToolCall: model.ToolCallChunk{
			ID:       id,
			Name:     name,
			Input:    input,
			InputRaw: args,
		},
	}
}

func toolResult(callID, toolName, content string, isError bool) model.ConversationItem {
	result := model.ToolResult{
		ToolCallID: callID,
		ToolName:   toolName,
		Content:    content,
		IsError:    isError,
	}
	return model.ConversationItem{
		Kind:   model.ConversationItemToolResult,
		Text:   content,
		Result: &result,
	}
}

func archiveColdIndexEntry(workspaceRoot string, occurredAt time.Time) types.ColdIndexEntry {
	return types.ColdIndexEntry{
		ID:          "cold_archive_compact_history_1",
		WorkspaceID: workspaceRoot,
		SourceType:  "archive",
		SourceID:    "compact_history_1",
		SearchText:  "database schema users orders user_id index performance connection pooling pool_size timeout PostgreSQL migration connection_timeout query_timeout config",
		SummaryLine: "[turns 1-2] Database optimization: added index on orders.user_id, increased pool_size to 40, set connection_timeout to 5s",
		FilesChanged: []string{
			"db/migrations/004_add_order_index.sql",
			"config/database.yaml",
		},
		ToolsUsed:  []string{"file_read", "file_write", "file_edit"},
		ErrorTypes: []string{},
		Visibility: types.MemoryVisibilityShared,
		ContextRef: types.ColdContextRef{
			SessionID:     historySessionID,
			ContextHeadID: historyHeadID,
			TurnStartPos:  1,
			TurnEndPos:    25,
			ItemCount:     25,
		},
		OccurredAt: occurredAt,
		CreatedAt:  occurredAt,
	}
}

func memoryEntries(workspaceRoot string, now time.Time) []types.MemoryEntry {
	return []types.MemoryEntry{
		memoryEntry(workspaceRoot, "mem_old_db_pattern", types.MemoryKindPattern, types.MemoryVisibilityShared, "", "Database migration pattern: always add indexes for foreign key columns used in JOIN queries. Use CREATE INDEX CONCURRENTLY for production tables.", 0.85, now.Add(-90*24*time.Hour)),
		memoryEntry(workspaceRoot, "mem_old_pool_config", types.MemoryKindDecision, types.MemoryVisibilityShared, "", "Connection pool sizing decision: use 40 connections for production PostgreSQL with 5s connection timeout and 30s query timeout", 0.80, now.Add(-60*24*time.Hour)),
		memoryEntry(workspaceRoot, "mem_old_schema_review", types.MemoryKindFact, types.MemoryVisibilityShared, "", "Schema review finding: orders table references users via user_id UUID foreign key. Consider adding ON DELETE CASCADE if order cleanup is needed.", 0.75, now.Add(-45*24*time.Hour)),
		memoryEntry(workspaceRoot, "mem_old_perf_notes", types.MemoryKindFact, types.MemoryVisibilityPrivate, "db_admin", "Performance benchmark notes: with pool_size=40 and the new index, user-order JOIN queries dropped from 850ms to 12ms", 0.90, now.Add(-30*24*time.Hour)),
	}
}

func memoryEntry(workspaceRoot, id string, kind types.MemoryKind, visibility types.MemoryVisibility, ownerRoleID, content string, confidence float64, timestamp time.Time) types.MemoryEntry {
	return types.MemoryEntry{
		ID:                  id,
		Kind:                kind,
		Scope:               types.MemoryScopeWorkspace,
		WorkspaceID:         workspaceRoot,
		Status:              types.MemoryStatusDeprecated,
		Visibility:          visibility,
		OwnerRoleID:         ownerRoleID,
		Content:             content,
		SourceSessionID:     historySessionID,
		SourceContextHeadID: historyHeadID,
		Confidence:          confidence,
		CreatedAt:           timestamp,
		UpdatedAt:           timestamp,
		LastUsedAt:          timestamp,
	}
}

func memoryColdIndexEntry(workspaceRoot string, entry types.MemoryEntry) types.ColdIndexEntry {
	return types.ColdIndexEntry{
		ID:          "cold_memory_deprecated_" + entry.ID,
		WorkspaceID: workspaceRoot,
		OwnerRoleID: entry.OwnerRoleID,
		Visibility:  entry.Visibility,
		SourceType:  "memory_deprecated",
		SourceID:    entry.ID,
		SearchText:  entry.Content,
		SummaryLine: truncateRunes(entry.Content, 200),
		ContextRef: types.ColdContextRef{
			SessionID:     entry.SourceSessionID,
			ContextHeadID: entry.SourceContextHeadID,
			ItemCount:     0,
		},
		OccurredAt: entry.CreatedAt,
		CreatedAt:  entry.CreatedAt,
	}
}

func oldDatabaseAuditReport(workspaceRoot string, now time.Time) types.ReportRecord {
	observedAt := now.Add(-7 * 24 * time.Hour)
	return types.ReportRecord{
		ID:            "report_old_db_audit",
		WorkspaceRoot: workspaceRoot,
		SessionID:     historySessionID,
		SourceKind:    types.ReportSourceTaskResult,
		SourceID:      "report_old_db_audit",
		Envelope: types.ReportEnvelope{
			Title:   "Database Audit Report",
			Summary: "Audit found missing indexes on foreign key columns and suboptimal connection pool settings. Recommendations: add index on orders.user_id, increase pool_size to 40, add connection timeout of 5s.",
			Sections: []types.ReportSectionContent{
				{Text: "Index audit", Items: []string{"Missing index orders.user_id", "Missing index order_items.order_id"}},
				{Text: "Connection pool audit", Items: []string{"pool_size too low at 10", "No connection timeout set"}},
			},
		},
		ObservedAt: observedAt,
		CreatedAt:  observedAt,
		UpdatedAt:  observedAt,
	}
}

func reportColdIndexEntry(workspaceRoot string, report types.ReportRecord) types.ColdIndexEntry {
	return types.ColdIndexEntry{
		ID:          "cold_report_" + report.ID,
		WorkspaceID: workspaceRoot,
		Visibility:  types.MemoryVisibilityShared,
		SourceType:  "report",
		SourceID:    report.ID,
		SearchText:  reportSearchText(report.Envelope),
		SummaryLine: report.Envelope.Summary,
		ContextRef: types.ColdContextRef{
			SessionID:     historySessionID,
			ContextHeadID: historyHeadID,
			TurnStartPos:  0,
			TurnEndPos:    1,
			ItemCount:     0,
		},
		OccurredAt: report.ObservedAt,
		CreatedAt:  report.CreatedAt,
	}
}

func reportSearchText(envelope types.ReportEnvelope) string {
	parts := []string{envelope.Title, envelope.Summary}
	for _, section := range envelope.Sections {
		parts = append(parts, section.Title, section.Text)
		parts = append(parts, section.Items...)
	}
	return strings.Join(nonEmptyStrings(parts), " ")
}

func nonEmptyStrings(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func truncateRunes(value string, max int) string {
	if max <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= max {
		return value
	}
	return string(runes[:max])
}

func countColdIndexRows(ctx context.Context, store *sqlite.Store) (int, error) {
	var count int
	if err := store.DB().QueryRowContext(ctx, `select count(*) from cold_index`).Scan(&count); err != nil {
		return 0, fmt.Errorf("count cold_index rows: %w", err)
	}
	return count, nil
}
