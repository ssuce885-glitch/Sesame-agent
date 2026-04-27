package engine

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"go-agent/internal/model"
	"go-agent/internal/types"
)

func TestDefaultGlobalPromptRequiresSkillBeforeAutomationControl(t *testing.T) {
	required := []string{
		"Before creating, modifying, pausing, or resuming automations",
		"automation_control",
	}
	for _, text := range required {
		if !strings.Contains(defaultGlobalSystemPrompt, text) {
			t.Fatalf("default global prompt missing %q:\n%s", text, defaultGlobalSystemPrompt)
		}
	}
}

func TestDefaultGlobalPromptUsesPersonalAssistantIdentity(t *testing.T) {
	required := []string{
		"local personal assistant",
		"Do not present yourself as a generic software engineering or coding assistant",
	}
	for _, text := range required {
		if !strings.Contains(defaultGlobalSystemPrompt, text) {
			t.Fatalf("default global prompt missing %q:\n%s", text, defaultGlobalSystemPrompt)
		}
	}
	forbidden := []string{
		"local software engineering assistant",
	}
	for _, text := range forbidden {
		if strings.Contains(defaultGlobalSystemPrompt, text) {
			t.Fatalf("default global prompt should not contain %q:\n%s", text, defaultGlobalSystemPrompt)
		}
	}
}

func TestDefaultGlobalPromptForbidsRoleTaskPolling(t *testing.T) {
	required := []string{
		"After delegate_to_role succeeds",
		"do not wait, sleep, poll, or inspect the delegated task",
		"at most inspect current state once",
		"Do not use task_wait, repeated task_get/task_output/task_result calls, or shell_command sleep loops",
		"Do not create a replacement task unless the user explicitly asks to rerun or retry",
	}
	for _, text := range required {
		if !strings.Contains(defaultGlobalSystemPrompt, text) {
			t.Fatalf("default global prompt missing %q:\n%s", text, defaultGlobalSystemPrompt)
		}
	}
}

func TestDurableWorkspaceMemoryIDsIncludeOwnerRole(t *testing.T) {
	const workspaceRoot = "/tmp/project"
	const detail = "Review auth flow"

	unownedOverview := durableWorkspaceOverviewID(workspaceRoot, "")
	opsOverview := durableWorkspaceOverviewID(workspaceRoot, "ops")
	researchOverview := durableWorkspaceOverviewID(workspaceRoot, "research")

	if unownedOverview == "" || opsOverview == "" || researchOverview == "" {
		t.Fatal("expected durable overview IDs to be non-empty")
	}
	if unownedOverview == opsOverview || opsOverview == researchOverview || unownedOverview == researchOverview {
		t.Fatalf("overview IDs should be owner-scoped, got unowned=%q ops=%q research=%q", unownedOverview, opsOverview, researchOverview)
	}

	opsDetail := durableWorkspaceDetailID(workspaceRoot, "ops", "thread", detail)
	researchDetail := durableWorkspaceDetailID(workspaceRoot, "research", "thread", detail)
	if opsDetail == researchDetail {
		t.Fatalf("detail IDs should be owner-scoped, got %q", opsDetail)
	}
	if !strings.HasPrefix(opsDetail, durableWorkspaceMemoryPrefix(workspaceRoot)) {
		t.Fatalf("detail ID %q should keep workspace durable prefix", opsDetail)
	}
}

func TestBuildGlobalDurableMemoriesAreUnowned(t *testing.T) {
	record := types.ContextHeadSummary{
		SessionID:     "sess_role",
		ContextHeadID: "head_role",
		SourceTurnID:  "turn_role",
	}
	summary := model.Summary{
		UserGoals: []string{"I prefer concise answers."},
	}

	entries := buildGlobalDurableMemories(record, summary, "research")
	if len(entries) == 0 {
		t.Fatal("expected at least one global memory entry")
	}
	for _, entry := range entries {
		if entry.Scope != types.MemoryScopeGlobal {
			t.Fatalf("Scope = %q, want global", entry.Scope)
		}
		if entry.OwnerRoleID != "" {
			t.Fatalf("OwnerRoleID = %q, want empty for global memory", entry.OwnerRoleID)
		}
	}
}

func TestPruneWorkspaceDurableMemoriesOnlyDeletesSameOwner(t *testing.T) {
	const workspaceRoot = "/tmp/project"
	const roleID = "ops"

	desired := []types.MemoryEntry{{
		ID:          durableWorkspaceOverviewID(workspaceRoot, roleID),
		Scope:       types.MemoryScopeWorkspace,
		WorkspaceID: workspaceRoot,
		OwnerRoleID: roleID,
	}}

	opsStaleID := durableWorkspaceDetailID(workspaceRoot, roleID, "thread", "old ops thread")
	peerStaleID := durableWorkspaceDetailID(workspaceRoot, "research", "thread", "old research thread")
	unownedStaleID := durableWorkspaceDetailID(workspaceRoot, "", "thread", "old shared thread")
	store := &pruneMemoryStore{entries: []types.MemoryEntry{
		desired[0],
		{
			ID:          opsStaleID,
			Scope:       types.MemoryScopeWorkspace,
			WorkspaceID: workspaceRoot,
			OwnerRoleID: roleID,
		},
		{
			ID:          peerStaleID,
			Scope:       types.MemoryScopeWorkspace,
			WorkspaceID: workspaceRoot,
			OwnerRoleID: "research",
		},
		{
			ID:          unownedStaleID,
			Scope:       types.MemoryScopeWorkspace,
			WorkspaceID: workspaceRoot,
			OwnerRoleID: "",
		},
	}}

	pruned, err := pruneWorkspaceDurableMemories(context.Background(), store, workspaceRoot, roleID, desired)
	if err != nil {
		t.Fatalf("pruneWorkspaceDurableMemories returned error: %v", err)
	}
	if pruned != 1 {
		t.Fatalf("pruned = %d, want 1", pruned)
	}
	if want := []string{opsStaleID}; !reflect.DeepEqual(store.deleted, want) {
		t.Fatalf("deleted = %#v, want %#v", store.deleted, want)
	}
}

type pruneMemoryStore struct {
	entries []types.MemoryEntry
	deleted []string
}

func (s *pruneMemoryStore) ListVisibleMemoryEntries(context.Context, string, string) ([]types.MemoryEntry, error) {
	return append([]types.MemoryEntry(nil), s.entries...), nil
}

func (s *pruneMemoryStore) DeleteMemoryEntries(_ context.Context, ids []string) error {
	s.deleted = append(s.deleted, ids...)
	return nil
}
