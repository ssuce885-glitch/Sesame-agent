package contextsvc

import (
	"context"
	"os"
	"testing"
	"time"

	"go-agent/internal/v2/contracts"
	v2store "go-agent/internal/v2/store"
)

func TestPreviewIncludesPromptAndContextBlocks(t *testing.T) {
	ctx := context.Background()
	s, err := v2store.OpenInMemory()
	if err != nil {
		t.Fatalf("OpenInMemory: %v", err)
	}
	defer s.Close()

	now := time.Date(2026, 5, 4, 8, 0, 0, 0, time.UTC)
	workspaceRoot := t.TempDir()
	if err := os.WriteFile(workspaceRoot+"/AGENTS.md", []byte("- Keep baseline visible in preview."), 0o644); err != nil {
		t.Fatalf("write AGENTS.md: %v", err)
	}
	session := contracts.Session{
		ID:                "session-ctx",
		WorkspaceRoot:     workspaceRoot,
		SystemPrompt:      "Session prompt.",
		PermissionProfile: "workspace",
		State:             "idle",
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	if err := s.Sessions().Create(ctx, session); err != nil {
		t.Fatalf("create session: %v", err)
	}
	if err := s.ProjectStates().Upsert(ctx, contracts.ProjectState{
		WorkspaceRoot: workspaceRoot,
		Summary:       "Ship context governance.",
		CreatedAt:     now,
		UpdatedAt:     now,
	}); err != nil {
		t.Fatalf("upsert project state: %v", err)
	}
	if err := s.Messages().Append(ctx, []contracts.Message{
		{SessionID: session.ID, TurnID: "turn-0", Role: "system", Content: "Compacted summary.", Position: 0, CreatedAt: now},
		{SessionID: session.ID, TurnID: "turn-1", Role: "user", Content: "What is next?", Position: 1, CreatedAt: now},
		{SessionID: session.ID, TurnID: "turn-1", Role: "assistant", Content: "Context service.", Position: 2, CreatedAt: now},
	}); err != nil {
		t.Fatalf("append messages: %v", err)
	}
	expiresAt := now.Add(-time.Minute)
	for _, block := range []contracts.ContextBlock{
		{
			ID:              "ctx-available",
			WorkspaceRoot:   workspaceRoot,
			Type:            "decision",
			Owner:           "workspace",
			Visibility:      "global",
			SourceRef:       "message:1",
			Summary:         "Keep ContextBlock as an index.",
			Confidence:      1,
			ImportanceScore: 0.9,
			CreatedAt:       now,
			UpdatedAt:       now,
		},
		{
			ID:              "ctx-expired",
			WorkspaceRoot:   workspaceRoot,
			Type:            "fact",
			Owner:           "workspace",
			Visibility:      "global",
			SourceRef:       "memory:old",
			Summary:         "Old context.",
			Confidence:      1,
			ImportanceScore: 0.5,
			ExpiresAt:       &expiresAt,
			CreatedAt:       now,
			UpdatedAt:       now,
		},
	} {
		if err := s.ContextBlocks().Create(ctx, block); err != nil {
			t.Fatalf("create context block %s: %v", block.ID, err)
		}
	}
	if err := s.Memories().Create(ctx, contracts.Memory{
		ID:            "memory-1",
		WorkspaceRoot: workspaceRoot,
		Kind:          "decision",
		Content:       "Memory remains available.",
		Confidence:    0.8,
		CreatedAt:     now,
		UpdatedAt:     now,
	}); err != nil {
		t.Fatalf("create memory: %v", err)
	}
	if err := s.Reports().Create(ctx, contracts.Report{
		ID:         "report-1",
		SessionID:  session.ID,
		SourceKind: "task_result",
		SourceID:   "task-1",
		Status:     "completed",
		Severity:   "info",
		Title:      "Report",
		Summary:    "Report remains available.",
		CreatedAt:  now,
	}); err != nil {
		t.Fatalf("create report: %v", err)
	}

	svc := New(s, nil)
	svc.now = func() time.Time { return now }
	preview, err := svc.Preview(ctx, PreviewInput{
		SessionID:    session.ID,
		SystemPrompt: "You are Sesame.",
	})
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	if preview.SessionID != session.ID || preview.WorkspaceRoot != workspaceRoot || preview.ApproxTokens <= 0 || len(preview.Prompt) < 3 {
		t.Fatalf("preview = %+v", preview)
	}
	assertPreviewBlock(t, preview.Blocks, "project_state", "included")
	assertPreviewBlock(t, preview.Blocks, "workspace_instructions", "included")
	assertPreviewBlock(t, preview.Blocks, "ctx-available", "available")
	assertPreviewBlock(t, preview.Blocks, "ctx-expired", "excluded")
	assertPreviewBlock(t, preview.Blocks, "memory-1", "available")
	assertPreviewBlock(t, preview.Blocks, "report-1", "available")
	if countPreviewBlock(preview.Blocks, "system_prompt") != 1 {
		t.Fatalf("system prompt block count = %d, blocks %+v", countPreviewBlock(preview.Blocks, "system_prompt"), preview.Blocks)
	}
}

func TestUpdateBlockMergesPartialInput(t *testing.T) {
	ctx := context.Background()
	s, err := v2store.OpenInMemory()
	if err != nil {
		t.Fatalf("OpenInMemory: %v", err)
	}
	defer s.Close()

	now := time.Date(2026, 5, 4, 9, 0, 0, 0, time.UTC)
	svc := New(s, nil)
	svc.now = func() time.Time { return now }
	created, err := svc.CreateBlock(ctx, "/workspace", BlockInput{
		ID:              strPtr("client-id"),
		WorkspaceRoot:   strPtr("/other-workspace"),
		Type:            strPtr("decision"),
		Owner:           strPtr("workspace"),
		Visibility:      strPtr("global"),
		SourceRef:       strPtr("message:1"),
		Title:           strPtr("Original"),
		Summary:         strPtr("Keep existing summary."),
		ImportanceScore: floatPtr(0.7),
	}, func(string) string { return "ctx-1" })
	if err != nil {
		t.Fatalf("create block: %v", err)
	}
	if created.ID != "ctx-1" || created.WorkspaceRoot != "/workspace" {
		t.Fatalf("created block accepted client identity override: %+v", created)
	}

	updatedAt := now.Add(time.Hour)
	svc.now = func() time.Time { return updatedAt }
	updated, err := svc.UpdateBlock(ctx, created.ID, BlockInput{
		Title:      strPtr("Patched"),
		Confidence: floatPtr(0),
	})
	if err != nil {
		t.Fatalf("update block: %v", err)
	}
	if updated.Title != "Patched" || updated.Summary != "Keep existing summary." || updated.Type != "decision" || updated.Confidence != 0 {
		t.Fatalf("updated block = %+v", updated)
	}
	if !updated.UpdatedAt.Equal(updatedAt) || !updated.CreatedAt.Equal(now) {
		t.Fatalf("timestamps = created %s updated %s", updated.CreatedAt, updated.UpdatedAt)
	}
}

func assertPreviewBlock(t *testing.T, blocks []PreviewBlock, id, status string) {
	t.Helper()
	for _, block := range blocks {
		if block.ID == id {
			if block.Status != status {
				t.Fatalf("block %s status = %s, want %s", id, block.Status, status)
			}
			return
		}
	}
	t.Fatalf("missing block %s in %+v", id, blocks)
}

func countPreviewBlock(blocks []PreviewBlock, id string) int {
	count := 0
	for _, block := range blocks {
		if block.ID == id {
			count++
		}
	}
	return count
}

func strPtr(value string) *string { return &value }

func floatPtr(value float64) *float64 { return &value }
