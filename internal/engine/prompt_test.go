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

func TestBuildReportPromptSectionOmitsSectionBodies(t *testing.T) {
	report := reportDeliveryItemForPromptTest("task_1", "info", "Task finished", "short summary", "full body text")

	section := buildReportPromptSection([]types.ReportDeliveryItem{report})

	if !strings.Contains(section, "summary: short summary") {
		t.Fatalf("report prompt missing summary:\n%s", section)
	}
	if strings.Contains(section, "full body text") {
		t.Fatalf("report prompt should not include section body:\n%s", section)
	}
}

func TestBuildReportConversationItemsDigestThresholds(t *testing.T) {
	single := buildReportConversationItems([]types.ReportDeliveryItem{
		reportDeliveryItemForPromptTest("task_1", "info", "One", "summary one", "body one"),
	})
	if len(single) != 1 {
		t.Fatalf("single report items = %d, want 1", len(single))
	}
	if !strings.Contains(single[0].Text, "--- Report: One ---") || !strings.Contains(single[0].Text, "Details: body one") {
		t.Fatalf("single report item missing full report content:\n%s", single[0].Text)
	}

	few := buildReportConversationItems([]types.ReportDeliveryItem{
		reportDeliveryItemForPromptTest("task_1", "info", "One", "summary one", "body one"),
		reportDeliveryItemForPromptTest("task_2", "warning", "Two", "summary two", "body two"),
	})
	if len(few) != 3 {
		t.Fatalf("few report items = %d, want digest plus two reports", len(few))
	}
	if !strings.HasPrefix(few[0].Text, "Digest: 2 reports from 2 sources") {
		t.Fatalf("few digest header = %q", few[0].Text)
	}

	many := buildReportConversationItems([]types.ReportDeliveryItem{
		reportDeliveryItemForPromptTest("task_1", "info", "One", "summary one", "body one"),
		reportDeliveryItemForPromptTest("task_2", "warning", "Two", "summary two", "body two"),
		reportDeliveryItemForPromptTest("task_3", "success", "Three", "summary three", "body three"),
		reportDeliveryItemForPromptTest("task_4", "error", "Four", "summary four", "body four"),
		reportDeliveryItemForPromptTest("task_5", "blocked", "Five", "summary five", "body five"),
		reportDeliveryItemForPromptTest("task_6", "info", "Six", "summary six", "body six"),
	})
	if len(many) != 3 {
		t.Fatalf("many report items = %d, want digest plus warning/error reports", len(many))
	}
	if !strings.Contains(many[1].Text, "Severity: warning") || !strings.Contains(many[2].Text, "Severity: error") {
		t.Fatalf("many report detail items should keep warning/error reports:\n%q\n%q", many[1].Text, many[2].Text)
	}
}

func TestBuildReportConversationItemsClampsSectionBodies(t *testing.T) {
	items := buildReportConversationItems([]types.ReportDeliveryItem{
		reportDeliveryItemForPromptTest("task_1", "info", "Long", "summary", strings.Repeat("界", reportBodyRuneLimit+10)),
	})
	if len(items) != 1 {
		t.Fatalf("items = %d, want 1", len(items))
	}
	if !strings.Contains(items[0].Text, reportBodyTruncatedSuffix) {
		t.Fatalf("report item missing truncation suffix:\n%s", items[0].Text)
	}
	if len([]rune(items[0].Text)) >= reportBodyRuneLimit+200 {
		t.Fatalf("report item was not clamped enough, rune length %d", len([]rune(items[0].Text)))
	}
}

func TestInsertReportItemsBeforeTurnEntry(t *testing.T) {
	req := model.Request{Items: []model.ConversationItem{
		{Kind: model.ConversationItemSummary},
		model.UserMessageItem("Review the reports and continue the conversation."),
	}}
	insertReportItemsBeforeTurnEntry(&req, []model.ConversationItem{
		model.UserMessageItem("report one"),
		model.UserMessageItem("report two"),
	})

	got := []string{req.Items[1].Text, req.Items[2].Text, req.Items[3].Text}
	want := []string{"report one", "report two", "Review the reports and continue the conversation."}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("inserted items = %#v, want %#v", got, want)
	}
}

func reportDeliveryItemForPromptTest(sourceID, severity, title, summary, body string) types.ReportDeliveryItem {
	return types.ReportDeliveryItem{
		ID:         "delivery_" + sourceID,
		ReportID:   "report_" + sourceID,
		SourceKind: types.ReportSourceTaskResult,
		SourceID:   sourceID,
		Envelope: types.ReportEnvelope{
			Status:   "completed",
			Severity: severity,
			Title:    title,
			Summary:  summary,
			Sections: []types.ReportSectionContent{{
				Title: "Details",
				Text:  body,
			}},
		},
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
