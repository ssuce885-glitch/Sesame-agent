package tools

import (
	"context"
	"strings"
	"testing"

	rolectx "go-agent/internal/roles"
	"go-agent/internal/types"
)

type memoryWriteStore struct {
	entries []types.MemoryEntry
}

func (s *memoryWriteStore) UpsertMemoryEntry(_ context.Context, entry types.MemoryEntry) error {
	s.entries = append(s.entries, entry)
	return nil
}

func TestMemoryWriteToolDefaults(t *testing.T) {
	store := &memoryWriteStore{}
	output, err := (memoryWriteTool{}).ExecuteDecoded(context.Background(), DecodedCall{
		Input: MemoryWriteInput{Content: "Important implementation note"},
	}, ExecContext{
		WorkspaceRoot: "/workspace",
		MemoryStore:   store,
	})
	if err != nil {
		t.Fatalf("memory_write: %v", err)
	}
	if len(store.entries) != 1 {
		t.Fatalf("stored entries = %d, want 1", len(store.entries))
	}
	entry := store.entries[0]
	if !strings.HasPrefix(entry.ID, "mem_") {
		t.Fatalf("entry ID = %q, want mem_ prefix", entry.ID)
	}
	if entry.Scope != types.MemoryScopeWorkspace {
		t.Fatalf("Scope = %q", entry.Scope)
	}
	if entry.Kind != types.MemoryKindFact {
		t.Fatalf("Kind = %q", entry.Kind)
	}
	if entry.WorkspaceID != "/workspace" {
		t.Fatalf("WorkspaceID = %q", entry.WorkspaceID)
	}
	if entry.OwnerRoleID != "" {
		t.Fatalf("OwnerRoleID = %q, want empty for main agent", entry.OwnerRoleID)
	}
	if entry.Visibility != types.MemoryVisibilityShared {
		t.Fatalf("Visibility = %q", entry.Visibility)
	}
	if entry.Status != types.MemoryStatusActive {
		t.Fatalf("Status = %q", entry.Status)
	}
	if entry.Content != "Important implementation note" {
		t.Fatalf("Content = %q", entry.Content)
	}
	if entry.CreatedAt.IsZero() || entry.UpdatedAt.IsZero() || entry.LastUsedAt.IsZero() {
		t.Fatalf("timestamps were not populated: %#v", entry)
	}
	if entry.Confidence <= 0 {
		t.Fatalf("Confidence = %v, want positive", entry.Confidence)
	}

	data := output.Data.(MemoryWriteOutput)
	if data.ID != entry.ID || data.Kind != "fact" || data.Scope != "workspace" {
		t.Fatalf("output = %#v, entry = %#v", data, entry)
	}
}

func TestMemoryWriteToolSpecialistOwnerAndCustomFields(t *testing.T) {
	store := &memoryWriteStore{}
	ctx := rolectx.WithSpecialistRoleID(context.Background(), "analyst")

	_, err := (memoryWriteTool{}).ExecuteDecoded(ctx, DecodedCall{
		Input: MemoryWriteInput{
			Content:    "Keep generated release notes concise.",
			Kind:       "preference",
			Scope:      "global",
			Visibility: "private",
		},
	}, ExecContext{
		WorkspaceRoot: "/workspace",
		MemoryStore:   store,
	})
	if err != nil {
		t.Fatalf("memory_write: %v", err)
	}
	if len(store.entries) != 1 {
		t.Fatalf("stored entries = %d, want 1", len(store.entries))
	}
	entry := store.entries[0]
	if entry.OwnerRoleID != "analyst" {
		t.Fatalf("OwnerRoleID = %q", entry.OwnerRoleID)
	}
	if entry.Kind != types.MemoryKindPreference {
		t.Fatalf("Kind = %q", entry.Kind)
	}
	if entry.Scope != types.MemoryScopeGlobal {
		t.Fatalf("Scope = %q", entry.Scope)
	}
	if entry.Visibility != types.MemoryVisibilityPrivate {
		t.Fatalf("Visibility = %q", entry.Visibility)
	}
}

func TestMemoryWriteToolRolePolicyForcesRoleOnlyPrivateWorkspace(t *testing.T) {
	store := &memoryWriteStore{}
	ctx := rolectx.WithSpecialistRoleID(context.Background(), "analyst")

	_, err := (memoryWriteTool{}).ExecuteDecoded(ctx, DecodedCall{
		Input: MemoryWriteInput{
			Content:    "Private role note.",
			Scope:      "global",
			Visibility: "shared",
		},
	}, ExecContext{
		WorkspaceRoot: "/workspace",
		MemoryStore:   store,
		RoleSpec: &rolectx.Spec{
			RoleID: "analyst",
			Policy: &rolectx.RolePolicyConfig{
				MemoryWriteScope: "role_only",
			},
		},
	})
	if err != nil {
		t.Fatalf("memory_write: %v", err)
	}
	entry := store.entries[0]
	if entry.Scope != types.MemoryScopeWorkspace {
		t.Fatalf("Scope = %q, want workspace", entry.Scope)
	}
	if entry.Visibility != types.MemoryVisibilityPrivate {
		t.Fatalf("Visibility = %q, want private", entry.Visibility)
	}
}

func TestMemoryWriteToolRolePolicyAppliesDefaultVisibility(t *testing.T) {
	store := &memoryWriteStore{}

	_, err := (memoryWriteTool{}).ExecuteDecoded(context.Background(), DecodedCall{
		Input: MemoryWriteInput{Content: "Default private note."},
	}, ExecContext{
		WorkspaceRoot: "/workspace",
		MemoryStore:   store,
		RoleSpec: &rolectx.Spec{
			RoleID: "analyst",
			Policy: &rolectx.RolePolicyConfig{
				DefaultVisibility: "private",
			},
		},
	})
	if err != nil {
		t.Fatalf("memory_write: %v", err)
	}
	if got := store.entries[0].Visibility; got != types.MemoryVisibilityPrivate {
		t.Fatalf("Visibility = %q, want private", got)
	}
}
